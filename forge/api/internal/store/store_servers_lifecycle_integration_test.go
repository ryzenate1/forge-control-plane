package store

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func lifecycleTestStore(t *testing.T) *Store {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := "lifecycle_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_, _ = admin.Exec(cleanupCtx, `DROP SCHEMA `+schema+` CASCADE`)
		admin.Close()
	})

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	s := &Store{db: pool, secrets: newTestKeyring()}
	if err := s.RunMigrations(ctx, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestServerOrphanRemediationListAndResolve(t *testing.T) {
	s := lifecycleTestStore(t)
	ctx := context.Background()
	serverRemediationID, databaseRemediationID := uuid.NewString(), uuid.NewString()
	serverID, databaseID, hostID, actorID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, 'admin@example.test', 'hash', 'admin')`, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO server_orphan_remediations (id, server_id, node_url, daemon_error)
		VALUES ($1, $2, 'https://beacon.example.test', 'remote deletion failed')
	`, serverRemediationID, serverID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO database_orphan_remediations
			(id, server_database_id, server_id, database_host_id, engine, host, port, database_name, username, remote, reason)
		VALUES ($1, $2, $3, $4, 'postgresql', 'db.example.test', 5432, 'game', 'game_user', '%', 'remote deletion failed')
	`, databaseRemediationID, databaseID, serverID, hostID); err != nil {
		t.Fatal(err)
	}

	servers, err := s.ListServerOrphanRemediations(ctx, OrphanRemediationStatusPending)
	if err != nil || len(servers) != 1 || servers[0].ID != serverRemediationID {
		t.Fatalf("pending server remediations = %#v, %v", servers, err)
	}
	databases, err := s.ListDatabaseOrphanRemediations(ctx, OrphanRemediationStatusPending)
	if err != nil || len(databases) != 1 || databases[0].ID != databaseRemediationID {
		t.Fatalf("pending database remediations = %#v, %v", databases, err)
	}

	if _, err := s.ResolveServerOrphanRemediation(ctx, serverRemediationID, &actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveDatabaseOrphanRemediation(ctx, databaseRemediationID, &actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveServerOrphanRemediation(ctx, serverRemediationID, &actorID); !errors.Is(err, ErrOrphanRemediationResolved) {
		t.Fatalf("resolving already resolved remediation error = %v", err)
	}

	servers, err = s.ListServerOrphanRemediations(ctx, OrphanRemediationStatusResolved)
	if err != nil || len(servers) != 1 || servers[0].ResolvedAt == nil {
		t.Fatalf("resolved server remediations = %#v, %v", servers, err)
	}
	databases, err = s.ListDatabaseOrphanRemediations(ctx, OrphanRemediationStatusResolved)
	if err != nil || len(databases) != 1 || databases[0].ResolvedAt == nil {
		t.Fatalf("resolved database remediations = %#v, %v", databases, err)
	}
	var auditCount int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE target_type = 'orphan_remediation' AND actor_id = $1`, actorID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 2 {
		t.Fatalf("resolution audit events = %d, want 2", auditCount)
	}
}

func TestServerInventoriesSupportEmailSearchAndNewestFirstOrdering(t *testing.T) {
	s := lifecycleTestStore(t)
	ctx := context.Background()
	ownerID, nodeID, templateID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	allocationOneID, allocationTwoID := uuid.NewString(), uuid.NewString()
	ownerEmail := "inventory-owner@example.test"

	if _, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'user')`, ownerID, ownerEmail); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO nodes (id, name, region, base_url, token_hash) VALUES ($1, 'inventory-node', 'test', 'http://beacon.test', 'hash')`, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO eggs (id, nest_id, name, docker_images, startup, default_memory_mb)
		SELECT $1, id, 'inventory-template', '{"Busybox":"busybox:latest"}'::jsonb, 'sleep 60', 512
		FROM nests WHERE name = 'Games'
	`, templateID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO allocations (id, node_id, ip, port) VALUES ($1, $2, '127.0.0.1', 25565), ($3, $2, '127.0.0.1', 25566)`, allocationOneID, nodeID, allocationTwoID); err != nil {
		t.Fatal(err)
	}

	create := func(name, allocationID string) Server {
		t.Helper()
		server, err := s.CreateServer(ctx, CreateServerRequest{
			Name: name, NodeID: nodeID, OwnerID: ownerID, TemplateID: templateID, AllocationID: allocationID,
			MemoryMB: 512, CPUShares: 512, DiskMB: 1024, IOWeight: 500,
		})
		if err != nil {
			t.Fatal(err)
		}
		return server
	}
	older := create("older", allocationOneID)
	newer := create("newer", allocationTwoID)
	if _, err := s.db.Exec(ctx, `UPDATE servers SET created_at = CASE id WHEN $1 THEN now() - interval '1 hour' WHEN $2 THEN now() END`, older.ID, newer.ID); err != nil {
		t.Fatal(err)
	}

	userServers, userTotal, err := s.ListServersForUser(ctx, ownerID, "user", 1, 10, ownerEmail)
	if err != nil {
		t.Fatal(err)
	}
	if userTotal != 2 || len(userServers) != 2 || userServers[0].ID != newer.ID || userServers[1].ID != older.ID {
		t.Fatalf("user inventory = %#v, total %d; want newest-first two-server inventory", userServers, userTotal)
	}

	adminServers, adminTotal, err := s.ListServersPaginated(ctx, 1, 10, ownerEmail)
	if err != nil {
		t.Fatal(err)
	}
	if adminTotal != 2 || len(adminServers) != 2 || adminServers[0].ID != newer.ID || adminServers[1].ID != older.ID {
		t.Fatalf("admin inventory = %#v, total %d; want newest-first two-server inventory", adminServers, adminTotal)
	}
}

func TestServerPatchAndHardDeleteAreTransactional(t *testing.T) {
	s := lifecycleTestStore(t)
	ctx := context.Background()
	ownerID, nodeID, templateID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	primaryID, alternateID := uuid.NewString(), uuid.NewString()
	if _, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, ownerID, ownerID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO nodes (id, name, region, base_url, token_hash) VALUES ($1, 'node', 'test', 'http://beacon.test', 'hash')`, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO eggs (id, nest_id, name, docker_images, startup, default_memory_mb)
		SELECT $1, id, 'template', '{"Busybox":"busybox:latest"}'::jsonb, 'sleep 60', 512
		FROM nests WHERE name = 'Games'
	`, templateID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO allocations (id, node_id, ip, port) VALUES ($1, $2, '127.0.0.1', 25565), ($3, $2, '127.0.0.1', 25566)`, primaryID, nodeID, alternateID); err != nil {
		t.Fatal(err)
	}

	server, err := s.CreateServer(ctx, CreateServerRequest{
		Name: "original", NodeID: nodeID, OwnerID: ownerID, TemplateID: templateID, AllocationID: primaryID,
		MemoryMB: 768, CPUShares: 512, DiskMB: 2048, IOWeight: 500, CPULimit: 0,
		DatabaseLimit: 2, BackupLimit: 3, AllocationLimit: 2, SwapMB: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	renamed := "renamed"
	updated, err := s.UpdateServer(ctx, server.ID, UpdateServerRequest{Name: &renamed}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.MemoryMB != 768 || updated.DiskMB != 2048 || updated.DatabaseLimit != 2 || updated.BackupLimit != 3 || updated.AllocationLimit != 2 {
		t.Fatalf("omitted patch fields changed: %+v", updated)
	}
	if _, err := s.UpdateServer(ctx, server.ID, UpdateServerRequest{PrimaryAllocationID: &alternateID}, nil); err == nil {
		t.Fatal("unassigned allocation was accepted as primary")
	}
	if _, err := s.db.Exec(ctx, `UPDATE allocations SET server_id = $1, assigned_at = now() WHERE id = $2`, server.ID, alternateID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateServer(ctx, server.ID, UpdateServerRequest{PrimaryAllocationID: &alternateID}, nil); err != nil {
		t.Fatal(err)
	}

	if err := s.RecordOrphanAndHardDeleteServer(ctx, server.ID, "http://beacon.test", "delete failed"); err != nil {
		t.Fatal(err)
	}
	var assigned int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE server_id = $1`, server.ID).Scan(&assigned); err != nil {
		t.Fatal(err)
	}
	if assigned != 0 {
		t.Fatalf("%d allocations remain assigned after hard delete", assigned)
	}
	var remediation int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM server_orphan_remediations WHERE server_id = $1 AND status = 'pending'`, server.ID).Scan(&remediation); err != nil {
		t.Fatal(err)
	}
	if remediation != 1 {
		t.Fatalf("orphan remediation rows = %d, want 1", remediation)
	}
}
