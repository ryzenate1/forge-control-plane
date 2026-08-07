package store

import (
	"context"
	"encoding/json"
	"testing"
)

func TestListDBContainers_DecryptsEncryptedFleetView(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	serverID := "44444444-4444-4444-8444-444444444444"
	// Ensure server exists minimally for FK? Try to ensure with dummy insert if needed, but CreateDBContainer does not FK check server.
	// We attempt to create a server row if not exists to avoid FK violation.
	// Check if server exists, if not create minimal.
	var exists bool
	_ = s.GetDB().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM servers WHERE id = $1)`, serverID).Scan(&exists)
	if !exists {
		// Create minimal required rows: user, node, nest, egg, allocation
		// Use existing seed data if present; otherwise create via SQL.
		// Try to create via direct inserts; ignore errors if FKs missing due to migration not requiring them.
		_, _ = s.GetDB().Exec(ctx, `INSERT INTO nodes (id, name, region, base_url, token_hash) VALUES ('22222222-2222-2222-2222-222222222222','test-node','test','http://127.0.0.1:9090','hash') ON CONFLICT (id) DO NOTHING`)
		_, _ = s.GetDB().Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ('11111111-1111-1111-1111-111111111111','a@example.test','hash','admin') ON CONFLICT (id) DO NOTHING`)
		// Need allocation and egg for server; try to use existing egg if any
		var nestID string
		_ = s.GetDB().QueryRow(ctx, `SELECT id::text FROM nests WHERE name='Games' LIMIT 1`).Scan(&nestID)
		if nestID == "" {
			nestID = "dddddddd-dddd-dddd-dddd-dddddddddddd"
			_, _ = s.GetDB().Exec(ctx, `INSERT INTO nests (id, name) VALUES ($1,'Games') ON CONFLICT (name) DO NOTHING`, nestID)
		}
		var eggID string
		_ = s.GetDB().QueryRow(ctx, `SELECT id::text FROM eggs WHERE nest_id = $1 LIMIT 1`, nestID).Scan(&eggID)
		if eggID == "" {
			eggID = "33333333-3333-3333-3333-333333333333"
			_, _ = s.GetDB().Exec(ctx, `INSERT INTO eggs (id, nest_id, name, docker_images, startup, config) VALUES ($1,$2,'TestEgg','{}','startup','{}') ON CONFLICT (nest_id, name) DO NOTHING`, eggID, nestID)
			_ = s.GetDB().QueryRow(ctx, `SELECT id::text FROM eggs WHERE nest_id=$1 AND name='TestEgg'`, nestID).Scan(&eggID)
		}
		allocID := "55555555-5555-5555-5555-555555555555"
		_, _ = s.GetDB().Exec(ctx, `INSERT INTO allocations (id, node_id, ip, port) VALUES ($1,'22222222-2222-2222-2222-222222222222','127.0.0.1',25565) ON CONFLICT (id) DO NOTHING`, allocID)
		_, _ = s.GetDB().Exec(ctx, `INSERT INTO servers (id, node_id, owner_id, egg_id, template_id, name, memory_mb, cpu_shares, disk_mb, allocation_id) VALUES ($1,'22222222-2222-2222-2222-222222222222','11111111-1111-1111-1111-111111111111',$2,$2,'test-server',1024,512,2048,$3) ON CONFLICT (id) DO NOTHING`, serverID, eggID, allocID)
	}

	container, err := s.CreateDBContainer(ctx, CreateDBContainerRequest{
		ServerID: serverID,
		Engine:   "postgresql",
		Version:  "16",
		MemoryMB: 256,
	})
	if err != nil {
		t.Fatalf("CreateDBContainer: %v", err)
	}

	connStr := "postgres://user:pass@localhost:5432/db"
	creds := json.RawMessage(`{"username":"user","password":"pass"}`)
	if err := s.SetDBContainerStatus(ctx, container.ID, "container-123", "running", 5432, "vol-1", connStr, creds); err != nil {
		t.Fatalf("SetDBContainerStatus: %v", err)
	}

	// Fleet view via List should return decrypted secrets, not blank.
	list, err := s.ListDBContainers(ctx, serverID)
	if err != nil {
		t.Fatalf("ListDBContainers: %v", err)
	}
	if len(list) == 0 {
		t.Fatalf("expected at least 1 container, got 0")
	}
	var found *DBContainer
	for i := range list {
		if list[i].ID == container.ID {
			found = &list[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("created container not found in list")
	}
	if found.ConnectionString != connStr {
		t.Fatalf("ListDBContainers ConnectionString = %q, want %q (fleet view should decrypt)", found.ConnectionString, connStr)
	}
	if string(found.Credentials) == "" || string(found.Credentials) == "{}" {
		t.Fatalf("ListDBContainers Credentials blank after encryption: %s", string(found.Credentials))
	}
	var credMap map[string]string
	if err := json.Unmarshal(found.Credentials, &credMap); err != nil {
		t.Fatalf("credentials json: %v", err)
	}
	if credMap["username"] != "user" || credMap["password"] != "pass" {
		t.Fatalf("credentials mismatch: %v", credMap)
	}

	// Global fleet view via ListAll should also decrypt.
	all, err := s.ListAllDBContainers(ctx, 10)
	if err != nil {
		t.Fatalf("ListAllDBContainers: %v", err)
	}
	found = nil
	for i := range all {
		if all[i].ID == container.ID {
			found = &all[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("container not found in ListAll")
	}
	if found.ConnectionString != connStr {
		t.Fatalf("ListAll ConnectionString = %q, want %q", found.ConnectionString, connStr)
	}

	// Single get should also work (baseline)
	single, err := s.GetDBContainer(ctx, container.ID)
	if err != nil {
		t.Fatalf("GetDBContainer: %v", err)
	}
	if single.ConnectionString != connStr {
		t.Fatalf("GetDBContainer ConnectionString = %q, want %q", single.ConnectionString, connStr)
	}
}
