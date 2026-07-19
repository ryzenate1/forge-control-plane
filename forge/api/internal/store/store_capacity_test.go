//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestNodeCapacitySnapshot(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	nodeID, _, regionID := setupTestNode(t, s)

	snapshot, err := s.NodeCapacitySnapshot(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.NodeID != nodeID {
		t.Fatalf("expected node %s, got %s", nodeID, snapshot.NodeID)
	}
	if snapshot.RegionID != regionID {
		t.Fatalf("expected region %s, got %s", regionID, snapshot.RegionID)
	}
	if snapshot.TotalCPU != 8192 {
		t.Fatalf("expected TotalCPU 8192, got %d", snapshot.TotalCPU)
	}
	if snapshot.TotalMemory != 16384 {
		t.Fatalf("expected TotalMemory 16384, got %d", snapshot.TotalMemory)
	}
	if snapshot.TotalDisk != 102400 {
		t.Fatalf("expected TotalDisk 102400, got %d", snapshot.TotalDisk)
	}
	if snapshot.AllocatedCPU != 0 {
		t.Fatalf("expected AllocatedCPU 0, got %d", snapshot.AllocatedCPU)
	}
	if snapshot.AvailableCPU != 8192 {
		t.Fatalf("expected AvailableCPU 8192, got %d", snapshot.AvailableCPU)
	}
	if snapshot.ServerCount != 0 {
		t.Fatalf("expected ServerCount 0, got %d", snapshot.ServerCount)
	}
}

func TestNodeCapacitySnapshotWithServers(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	nodeID, _, _ := setupTestNode(t, s)

	ownerID := uuid.NewString()
	templateID := uuid.NewString()
	serverID := uuid.NewString()
	if _, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, 'test@test.com', 'hash', 'admin')`, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO server_templates (id, name, image, startup_command, default_memory_mb) VALUES ($1, 'test', 'test:latest', 'test', 1024)`, templateID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO servers (id, node_id, owner_id, template_id, name, memory_mb, cpu_shares, disk_mb)
		VALUES ($1, $2, $3, $4, 'test-server', 4096, 2048, 10240)
	`, serverID, nodeID, ownerID, templateID); err != nil {
		t.Fatal(err)
	}

	snapshot, err := s.NodeCapacitySnapshot(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ServerCount != 1 {
		t.Fatalf("expected ServerCount 1, got %d", snapshot.ServerCount)
	}
	if snapshot.AllocatedCPU != 2048 {
		t.Fatalf("expected AllocatedCPU 2048, got %d", snapshot.AllocatedCPU)
	}
	if snapshot.AllocatedMemory != 4096 {
		t.Fatalf("expected AllocatedMemory 4096, got %d", snapshot.AllocatedMemory)
	}
	if snapshot.AllocatedDisk != 10240 {
		t.Fatalf("expected AllocatedDisk 10240, got %d", snapshot.AllocatedDisk)
	}
	if snapshot.AvailableCPU != 8192-2048 {
		t.Fatalf("expected AvailableCPU %d, got %d", 8192-2048, snapshot.AvailableCPU)
	}
}

func TestNodeCapacitySnapshotWithReservations(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	nodeID, _, _ := setupTestNode(t, s)

	if _, err := s.CreatePlacementReservation(ctx, CreatePlacementReservationRequest{
		NodeID: nodeID,
		CPU:    1024,
		Memory: 2048,
		Disk:   5120,
	}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := s.NodeCapacitySnapshot(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.AvailableCPU != 8192-1024 {
		t.Fatalf("expected AvailableCPU %d (accounting for reservation), got %d", 8192-1024, snapshot.AvailableCPU)
	}
	if snapshot.AvailableMemory != 16384-2048 {
		t.Fatalf("expected AvailableMemory %d (accounting for reservation), got %d", 16384-2048, snapshot.AvailableMemory)
	}
	if snapshot.AvailableDisk != 102400-5120 {
		t.Fatalf("expected AvailableDisk %d (accounting for reservation), got %d", 102400-5120, snapshot.AvailableDisk)
	}
}

func TestNodeCapacitySnapshotNodeNotFound(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	_, err := s.NodeCapacitySnapshot(ctx, uuid.NewString())
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows, got %v", err)
	}
}

func TestRegionCapacitySnapshots(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	locationID := uuid.NewString()
	regionID := uuid.NewString()
	if _, err := s.db.Exec(ctx, `INSERT INTO locations (id, short, long) VALUES ($1, 'test', 'Test Location')`, locationID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO regions (id, uuid, name, slug) VALUES ($1, $1, 'Test Region', 'test-region')`, regionID); err != nil {
		t.Fatal(err)
	}

	nodeIDs := make([]string, 2)
	for i := 0; i < 2; i++ {
		nodeID := uuid.NewString()
		if _, err := s.db.Exec(ctx, `
			INSERT INTO nodes (id, name, region, base_url, token_hash, location_id, region_id, cpu_threads, memory_mb, disk_mb)
			VALUES ($1, $2::text, 'test', 'http://node.test', 'hash', $3, $4, 4, 8192, 51200)
		`, nodeID, "Node-"+string(rune('0'+i)), locationID, regionID); err != nil {
			t.Fatal(err)
		}
		nodeIDs[i] = nodeID
	}

	snapshots, err := s.RegionCapacitySnapshots(ctx, regionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapshots))
	}
	seen := map[string]bool{}
	for _, snap := range snapshots {
		seen[snap.NodeID] = true
		if snap.TotalCPU != 4096 {
			t.Fatalf("node %s: expected TotalCPU 4096, got %d", snap.NodeID, snap.TotalCPU)
		}
	}
	for _, nid := range nodeIDs {
		if !seen[nid] {
			t.Fatalf("missing snapshot for node %s", nid)
		}
	}
}

func TestRegionCapacitySnapshotsEmptyRegion(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	snapshots, err := s.RegionCapacitySnapshots(ctx, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 0 {
		t.Fatalf("expected 0 snapshots, got %d", len(snapshots))
	}
}

func TestAvailableResource(t *testing.T) {
	tests := []struct {
		name      string
		total     int
		allocated int
		want      int
	}{
		{name: "full available", total: 100, allocated: 0, want: 100},
		{name: "partial available", total: 100, allocated: 30, want: 70},
		{name: "fully allocated", total: 100, allocated: 100, want: 0},
		{name: "over allocated", total: 100, allocated: 150, want: 0},
		{name: "zero total", total: 0, allocated: 0, want: 0},
		{name: "negative total", total: -1, allocated: 0, want: 0},
		{name: "negative allocated", total: 100, allocated: -10, want: 110},
		{name: "both zero", total: 0, allocated: 0, want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := availableResource(tc.total, tc.allocated)
			if got != tc.want {
				t.Fatalf("availableResource(%d, %d) = %d; want %d", tc.total, tc.allocated, got, tc.want)
			}
		})
	}
}

func TestFindAvailableAllocation(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	nodeID := uuid.NewString()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO nodes (id, name, region, base_url, token_hash, memory_mb, disk_mb)
		VALUES ($1, 'Alloc Node', 'test', 'http://node.test', 'hash', 8192, 51200)
	`, nodeID); err != nil {
		t.Fatal(err)
	}

	allocationID := uuid.NewString()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO allocations (id, node_id, ip, port)
		VALUES ($1, $2, '192.168.1.1', 25565)
	`, allocationID, nodeID); err != nil {
		t.Fatal(err)
	}

	allocation, err := s.FindAvailableAllocation(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if allocation.ID != allocationID {
		t.Fatalf("expected allocation %s, got %s", allocationID, allocation.ID)
	}
	if allocation.Node != "Alloc Node" {
		t.Fatalf("expected node 'Alloc Node', got %s", allocation.Node)
	}
	if allocation.IP != "192.168.1.1" {
		t.Fatalf("expected IP '192.168.1.1', got %s", allocation.IP)
	}
	if allocation.Port != 25565 {
		t.Fatalf("expected port 25565, got %d", allocation.Port)
	}
}

func TestFindAvailableAllocationSkipsAssigned(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	nodeID := uuid.NewString()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO nodes (id, name, region, base_url, token_hash, memory_mb, disk_mb)
		VALUES ($1, 'Alloc Node', 'test', 'http://node.test', 'hash', 8192, 51200)
	`, nodeID); err != nil {
		t.Fatal(err)
	}

	ownerID := uuid.NewString()
	templateID := uuid.NewString()
	serverID := uuid.NewString()
	if _, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, 'test2@test.com', 'hash', 'admin')`, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO server_templates (id, name, image, startup_command, default_memory_mb) VALUES ($1, 'test', 'test:latest', 'test', 1024)`, templateID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO servers (id, node_id, owner_id, template_id, name, memory_mb, cpu_shares, disk_mb)
		VALUES ($1, $2, $3, $4, 'test-server', 1024, 256, 4096)
	`, serverID, nodeID, ownerID, templateID); err != nil {
		t.Fatal(err)
	}

	assignedID := uuid.NewString()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO allocations (id, node_id, ip, port, server_id)
		VALUES ($1, $2, '192.168.1.1', 25565, $3)
	`, assignedID, nodeID, serverID); err != nil {
		t.Fatal(err)
	}

	freeID := uuid.NewString()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO allocations (id, node_id, ip, port)
		VALUES ($1, $2, '192.168.1.2', 25566)
	`, freeID, nodeID); err != nil {
		t.Fatal(err)
	}

	allocation, err := s.FindAvailableAllocation(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if allocation.ID != freeID {
		t.Fatalf("expected free allocation %s, got %s", freeID, allocation.ID)
	}
}

func TestFindAvailableAllocationNoAllocations(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	nodeID := uuid.NewString()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO nodes (id, name, region, base_url, token_hash, memory_mb, disk_mb)
		VALUES ($1, 'Empty Node', 'test', 'http://node.test', 'hash', 8192, 51200)
	`, nodeID); err != nil {
		t.Fatal(err)
	}

	_, err := s.FindAvailableAllocation(ctx, nodeID)
	if err == nil {
		t.Fatal("expected error when no allocations exist")
	}
}
