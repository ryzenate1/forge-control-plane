package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestDeleteMountRejectsWhenServersAttached(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()
	ownerID, nodeID, allocationID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, ownerID, ownerID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO nodes (id, name, region, base_url, token_hash, daemon_token_id, daemon_token) VALUES ($1, 'node', 'test', 'http://daemon.test', 'hash', 'token-id', 'token-secret')`, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO allocations (id, node_id, ip, port) VALUES ($1, $2, '127.0.0.1', 25565)`, allocationID, nodeID); err != nil {
		t.Fatal(err)
	}
	var nestID string
	if err := s.db.QueryRow(ctx, `SELECT id::text FROM nests WHERE name = 'Games'`).Scan(&nestID); err != nil {
		t.Fatal(err)
	}
	images, _ := json.Marshal(map[string]string{"Game": "example/game:2"})
	egg, err := s.CreateEgg(ctx, CreateEggRequest{
		NestID: nestID, Name: "Delete Mount Game", DockerImages: images,
		Startup: "./game --port {{PORT}}", Config: json.RawMessage(`{"stop":"quit"}`),
		DefaultMemoryMB: 1536, InstallScript: "install", InstallContainer: "alpine:3.21",
		InstallEntrypoint: "sh",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	mount, err := s.CreateMount(ctx, CreateMountRequest{
		Name: "delete test mount", Source: "/srv/delete-test", Target: "/data",
		NodeIDs: []string{nodeID}, TemplateIDs: []string{egg.ID},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	server, err := s.CreateServer(ctx, CreateServerRequest{
		Name: "delete test server", NodeID: nodeID, OwnerID: ownerID, TemplateID: egg.ID,
		AllocationID: allocationID, MemoryMB: 1024, CPUShares: 512, DiskMB: 2048,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AssignMountToServer(ctx, server.ID, mount.ID, nil); err != nil {
		t.Fatal(err)
	}

	count, err := s.CountServersUsingMount(ctx, mount.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 server using mount, got %d", count)
	}
}

func TestDeleteMountSucceedsWhenNoServersAttached(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()
	nodeID := uuid.NewString()
	if _, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, uuid.NewString(), "admin@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO nodes (id, name, region, base_url, token_hash, daemon_token_id, daemon_token) VALUES ($1, 'node', 'test', 'http://daemon.test', 'hash', 'token-id', 'token-secret')`, nodeID); err != nil {
		t.Fatal(err)
	}
	var nestID string
	if err := s.db.QueryRow(ctx, `SELECT id::text FROM nests WHERE name = 'Games'`).Scan(&nestID); err != nil {
		t.Fatal(err)
	}
	images, _ := json.Marshal(map[string]string{"Game": "example/game:2"})
	egg, err := s.CreateEgg(ctx, CreateEggRequest{
		NestID: nestID, Name: "Solo Mount Game", DockerImages: images,
		Startup: "./game", Config: json.RawMessage(`{"stop":"quit"}`),
		DefaultMemoryMB: 1024, InstallScript: "install",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	mount, err := s.CreateMount(ctx, CreateMountRequest{
		Name: "solo mount", Source: "/srv/solo", Target: "/solo",
		NodeIDs: []string{nodeID}, TemplateIDs: []string{egg.ID},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	count1, err := s.CountServersUsingMount(ctx, mount.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count1 != 0 {
		t.Fatalf("expected 0 servers using mount before deletion, got %d", count1)
	}

	if err := s.DeleteMount(ctx, mount.ID, nil); err != nil {
		t.Fatal(err)
	}

	_, err = s.GetMount(ctx, mount.ID)
	if err == nil {
		t.Fatal("expected mount not found after deletion")
	}

	count2, err := s.CountServersUsingMount(ctx, mount.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count2 != 0 {
		t.Fatalf("expected 0 servers after mount deletion, got %d", count2)
	}
}

func TestCountServersUsingMountWithMultipleServers(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()
	ownerID, nodeID, allocationID1, allocationID2 := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, ownerID, ownerID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO nodes (id, name, region, base_url, token_hash, daemon_token_id, daemon_token) VALUES ($1, 'node', 'test', 'http://daemon.test', 'hash', 'token-id', 'token-secret')`, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO allocations (id, node_id, ip, port) VALUES ($1, $2, '127.0.0.1', 25565)`, allocationID1, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO allocations (id, node_id, ip, port) VALUES ($1, $2, '127.0.0.1', 25566)`, allocationID2, nodeID); err != nil {
		t.Fatal(err)
	}
	var nestID string
	if err := s.db.QueryRow(ctx, `SELECT id::text FROM nests WHERE name = 'Games'`).Scan(&nestID); err != nil {
		t.Fatal(err)
	}
	images, _ := json.Marshal(map[string]string{"Game": "example/game:2"})
	egg, err := s.CreateEgg(ctx, CreateEggRequest{
		NestID: nestID, Name: "Multi Mount Game", DockerImages: images,
		Startup: "./game", Config: json.RawMessage(`{"stop":"quit"}`),
		DefaultMemoryMB: 1024, InstallScript: "install",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	mount, err := s.CreateMount(ctx, CreateMountRequest{
		Name: "shared mount", Source: "/srv/shared", Target: "/shared",
		NodeIDs: []string{nodeID}, TemplateIDs: []string{egg.ID},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	countBefore, err := s.CountServersUsingMount(ctx, mount.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countBefore != 0 {
		t.Fatalf("expected 0 servers before assignment, got %d", countBefore)
	}

	server1, err := s.CreateServer(ctx, CreateServerRequest{
		Name: "server-1", NodeID: nodeID, OwnerID: ownerID, TemplateID: egg.ID,
		AllocationID: allocationID1, MemoryMB: 1024, CPUShares: 512, DiskMB: 2048,
	})
	if err != nil {
		t.Fatal(err)
	}
	server2, err := s.CreateServer(ctx, CreateServerRequest{
		Name: "server-2", NodeID: nodeID, OwnerID: ownerID, TemplateID: egg.ID,
		AllocationID: allocationID2, MemoryMB: 1024, CPUShares: 512, DiskMB: 2048,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AssignMountToServer(ctx, server1.ID, mount.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.AssignMountToServer(ctx, server2.ID, mount.ID, nil); err != nil {
		t.Fatal(err)
	}

	countAfter, err := s.CountServersUsingMount(ctx, mount.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countAfter != 2 {
		t.Fatalf("expected 2 servers using mount, got %d", countAfter)
	}

	if err := s.RemoveMountFromServer(ctx, server1.ID, mount.ID, nil); err != nil {
		t.Fatal(err)
	}
	countAfterOne, err := s.CountServersUsingMount(ctx, mount.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countAfterOne != 1 {
		t.Fatalf("expected 1 server after detach, got %d", countAfterOne)
	}

	if err := s.RemoveMountFromServer(ctx, server2.ID, mount.ID, nil); err != nil {
		t.Fatal(err)
	}
	countAfterNone, err := s.CountServersUsingMount(ctx, mount.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countAfterNone != 0 {
		t.Fatalf("expected 0 servers after all detach, got %d", countAfterNone)
	}
}

func TestDeleteMountPreservesAuditLog(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()
	nodeID := uuid.NewString()
	if _, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, uuid.NewString(), "admin@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO nodes (id, name, region, base_url, token_hash, daemon_token_id, daemon_token) VALUES ($1, 'node', 'test', 'http://daemon.test', 'hash', 'token-id', 'token-secret')`, nodeID); err != nil {
		t.Fatal(err)
	}
	var nestID string
	if err := s.db.QueryRow(ctx, `SELECT id::text FROM nests WHERE name = 'Games'`).Scan(&nestID); err != nil {
		t.Fatal(err)
	}
	images, _ := json.Marshal(map[string]string{"Game": "example/game:2"})
	egg, err := s.CreateEgg(ctx, CreateEggRequest{
		NestID: nestID, Name: "Audit Mount Game", DockerImages: images,
		Startup: "./game", Config: json.RawMessage(`{"stop":"quit"}`),
		DefaultMemoryMB: 1024, InstallScript: "install",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	mount, err := s.CreateMount(ctx, CreateMountRequest{
		Name: "audit mount", Source: "/srv/audit", Target: "/audit",
		NodeIDs: []string{nodeID}, TemplateIDs: []string{egg.ID},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	actorID := uuid.NewString()
	if err := s.DeleteMount(ctx, mount.ID, &actorID); err != nil {
		t.Fatal(err)
	}
	var auditCount int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE entity_type = 'mount' AND entity_id = $1`, mount.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount < 1 {
		t.Fatalf("expected audit log entry for mount deletion, got %d", auditCount)
	}
}
