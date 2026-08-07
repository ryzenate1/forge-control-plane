//go:build integration

package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// seedBackupServer inserts a minimal user/node/server chain for backup tests.
func seedBackupServer(t *testing.T, s *Store) string {
	t.Helper()
	ctx := context.Background()
	ownerID, nodeID, allocationID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, ownerID, ownerID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO nodes (id, name, region, base_url, token_hash, daemon_token_id, daemon_token) VALUES ($1, 'bk-node', 'test', 'http://daemon.test', 'hash', 'token-id', 'token-secret')`, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO allocations (id, node_id, ip, port) VALUES ($1, $2, '127.0.0.1', 25599)`, allocationID, nodeID); err != nil {
		t.Fatal(err)
	}
	var nestID string
	if err := s.db.QueryRow(ctx, `SELECT id::text FROM nests WHERE name = 'Games'`).Scan(&nestID); err != nil {
		t.Fatal(err)
	}
	var eggID string
	images := []byte(`{"Game": "example/game:2"}`)
	if err := s.db.QueryRow(ctx, `
		INSERT INTO eggs (id, nest_id, name, docker_images, startup, config)
		VALUES ($1, $2, 'Backup Lock Egg', $3, './game', '{}')
		RETURNING id::text
	`, uuid.NewString(), nestID, images).Scan(&eggID); err != nil {
		t.Fatal(err)
	}
	server, err := s.CreateServer(ctx, CreateServerRequest{
		Name: "backup-lock-" + uuid.NewString()[:8], NodeID: nodeID, OwnerID: ownerID,
		TemplateID: eggID, AllocationID: allocationID,
		MemoryMB: 1024, CPUShares: 512, DiskMB: 2048,
	})
	if err != nil {
		t.Fatal(err)
	}
	return server.ID
}

// TestUpsertBackupIsLocked verifies the create-backup contract end to end at
// the store layer:
//  1. a pending row is created with is_locked = true when requested;
//  2. the asynchronous completion upsert (same UUID/name, status completed)
//     does NOT clobber the lock flag;
//  3. an unlocked backup stays unlocked.
func TestUpsertBackupIsLocked(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()
	serverID := seedBackupServer(t, s)

	completedAt := time.Now().UTC().Add(-time.Minute)

	stored, err := s.UpsertBackup(ctx, serverID, UpsertBackupRequest{
		Name:     "lockme-" + uuid.NewString()[:8],
		Status:   "pending",
		IsLocked: true,
	}, nil)
	if err != nil {
		t.Fatalf("upsert locked pending backup: %v", err)
	}
	if !stored.IsLocked {
		t.Fatal("pending backup lost is_locked=true")
	}

	completed, err := s.UpsertBackup(ctx, serverID, UpsertBackupRequest{
		UUID:        stored.UUID,
		Name:        stored.Name,
		Checksum:    "abc123",
		Size:        4096,
		Status:      "completed",
		CompletedAt: &completedAt,
	}, nil)
	if err != nil {
		t.Fatalf("completion upsert: %v", err)
	}
	if completed.Status != "completed" {
		t.Fatalf("status = %q, want completed", completed.Status)
	}
	if !completed.IsLocked {
		t.Fatal("completion upsert clobbered is_locked=true")
	}

	plain, err := s.UpsertBackup(ctx, serverID, UpsertBackupRequest{
		Name:   "plain-" + uuid.NewString()[:8],
		Status: "completed",
	}, nil)
	if err != nil {
		t.Fatalf("upsert plain backup: %v", err)
	}
	if plain.IsLocked {
		t.Fatal("default backup should not be locked")
	}
}
