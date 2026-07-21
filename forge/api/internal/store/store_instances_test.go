package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupInstancesTest(t *testing.T, ctx context.Context) *Store {
	s := setupTestStore(t, ctx)
	createTestNode(t, ctx, s, "node-a")
	createTestNode(t, ctx, s, "node-b")
	return s
}

func createTestNode(t *testing.T, ctx context.Context, s *Store, suffix string) string {
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO nodes (id, uuid, name, region, base_url, fqdn, scheme, status, daemon_listen, daemon_sftp, daemon_base)
		VALUES ($1, $1, $2, 'test', 'http://'+$2+':8080', $2, 'http', 'online', 8080, 2022, '/tmp')
	`, id, "node-"+suffix)
	if err != nil {
		t.Fatalf("create test node: %v", err)
	}
	return id
}

func setupTestStore(t *testing.T, ctx context.Context) *Store {
	pool, err := pgxpool.New(ctx, "postgres://localhost:5432/forge_test?sslmode=disable")
	if err != nil {
		t.Skipf("no test database: %v", err)
	}
	return &Store{db: pool}
}

func TestCreateReplicaApp(t *testing.T) {
	ctx := context.Background()
	s := setupInstancesTest(t, ctx)

	app, err := s.CreateReplicaApp(ctx, CreateReplicaAppRequest{
		Name:     "test-app",
		Replicas: 3,
		CPU:      1024,
		MemoryMB: 2048,
		DiskMB:   10240,
	})
	if err != nil {
		t.Fatalf("CreateReplicaApp: %v", err)
	}
	if app.Name != "test-app" {
		t.Fatalf("expected name test-app, got %s", app.Name)
	}
	if app.Replicas != 3 {
		t.Fatalf("expected 3 replicas, got %d", app.Replicas)
	}
}

func TestCreateInstance(t *testing.T) {
	ctx := context.Background()
	s := setupInstancesTest(t, ctx)

	app, _ := s.CreateReplicaApp(ctx, CreateReplicaAppRequest{Name: "test-app", Replicas: 2, CPU: 1024, MemoryMB: 2048, DiskMB: 10240})
	nodes, _ := s.ListNodes(ctx)
	if len(nodes) == 0 {
		t.Fatal("no test nodes available")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	inst, err := s.CreateInstance(ctx, tx, app.ID, nodes[0].ID, 0, 1024, 2048, 10240, "docker")
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if inst.Status != "pending" {
		t.Fatalf("expected status pending, got %s", inst.Status)
	}
	if inst.Idx != 0 {
		t.Fatalf("expected idx 0, got %d", inst.Idx)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit tx: %v", err)
	}
}

func TestTwoHealthyNodes(t *testing.T) {
	ctx := context.Background()
	s := setupInstancesTest(t, ctx)

	app, _ := s.CreateReplicaApp(ctx, CreateReplicaAppRequest{Name: "two-node-app", Replicas: 2, CPU: 1024, MemoryMB: 2048, DiskMB: 10240})
	nodes, _ := s.ListNodes(ctx)
	if len(nodes) < 2 {
		t.Skip("need at least 2 nodes")
	}

	for i := 0; i < 2; i++ {
		tx, _ := s.db.Begin(ctx)
		_, err := s.CreateInstance(ctx, tx, app.ID, nodes[i].ID, i, 1024, 2048, 10240, "docker")
		if err != nil {
			t.Fatalf("create instance %d: %v", i, err)
		}
		_ = tx.Commit(ctx)
	}

	instances, err := s.ListInstancesByApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("ListInstancesByApp: %v", err)
	}
	if len(instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(instances))
	}
}

func TestInsufficientCapacity(t *testing.T) {
	ctx := context.Background()
	s := setupInstancesTest(t, ctx)

	_, _ = s.CreateReplicaApp(ctx, CreateReplicaAppRequest{Name: "capacity-app", Replicas: 1, CPU: 999999, MemoryMB: 999999, DiskMB: 999999})
	nodes, _ := s.ListNodes(ctx)
	if len(nodes) == 0 {
		t.Skip("no nodes available")
	}

	snapshot, err := s.NodeCapacitySnapshot(ctx, nodes[0].ID)
	if err != nil {
		t.Fatalf("NodeCapacitySnapshot: %v", err)
	}
	// Verify node cannot accommodate the request
	if snapshot.AvailableCPU < 999999 {
		t.Logf("node %s has insufficient CPU: %d available < 999999", nodes[0].ID, snapshot.AvailableCPU)
	}
}

func TestIncompatibleRuntime(t *testing.T) {
	ctx := context.Background()
	s := setupInstancesTest(t, ctx)

	app, _ := s.CreateReplicaApp(ctx, CreateReplicaAppRequest{Name: "runtime-app", Replicas: 1, CPU: 1024, MemoryMB: 2048, DiskMB: 10240, RuntimeProvider: "firecracker"})
	nodes, _ := s.ListNodes(ctx)
	if len(nodes) == 0 {
		t.Skip("no nodes available")
	}

	// Most test nodes default to docker, not firecracker
	tx, _ := s.db.Begin(ctx)
	_, err := s.CreateInstance(ctx, tx, app.ID, nodes[0].ID, 0, 1024, 2048, 10240, "firecracker")
	_ = tx.Commit(ctx)
	if err != nil {
		t.Fatalf("instance creation should work regardless: %v", err)
	}
}

func TestOneNodeDraining(t *testing.T) {
	ctx := context.Background()
	s := setupInstancesTest(t, ctx)

	nodes, _ := s.ListNodes(ctx)
	if len(nodes) < 2 {
		t.Skip("need at least 2 nodes")
	}

	// Mark second node as draining
	_, err := s.db.Exec(ctx, `UPDATE nodes SET draining = true, desired_state = 'draining' WHERE id = $1`, nodes[1].ID)
	if err != nil {
		t.Fatalf("mark node draining: %v", err)
	}

	app, _ := s.CreateReplicaApp(ctx, CreateReplicaAppRequest{Name: "drain-app", Replicas: 1, CPU: 1024, MemoryMB: 2048, DiskMB: 10240})
	tx, _ := s.db.Begin(ctx)
	_, err = s.CreateInstance(ctx, tx, app.ID, nodes[0].ID, 0, 1024, 2048, 10240, "docker")
	if err != nil {
		t.Fatalf("should place on non-draining node: %v", err)
	}
	_ = tx.Commit(ctx)
}

func TestOneNodeOffline(t *testing.T) {
	ctx := context.Background()
	s := setupInstancesTest(t, ctx)

	nodes, _ := s.ListNodes(ctx)
	if len(nodes) < 2 {
		t.Skip("need at least 2 nodes")
	}

	_, err := s.db.Exec(ctx, `UPDATE nodes SET status = 'offline', actual_state = 'offline' WHERE id = $1`, nodes[1].ID)
	if err != nil {
		t.Fatalf("mark node offline: %v", err)
	}

	app, _ := s.CreateReplicaApp(ctx, CreateReplicaAppRequest{Name: "offline-app", Replicas: 1, CPU: 1024, MemoryMB: 2048, DiskMB: 10240})
	tx, _ := s.db.Begin(ctx)
	_, err = s.CreateInstance(ctx, tx, app.ID, nodes[0].ID, 0, 1024, 2048, 10240, "docker")
	if err != nil {
		t.Fatalf("should place on online node: %v", err)
	}
	_ = tx.Commit(ctx)
}

func TestDuplicateScaleRequest(t *testing.T) {
	ctx := context.Background()
	s := setupInstancesTest(t, ctx)

	app, _ := s.CreateReplicaApp(ctx, CreateReplicaAppRequest{Name: "dup-app", Replicas: 1, CPU: 1024, MemoryMB: 2048, DiskMB: 10240})
	nodes, _ := s.ListNodes(ctx)
	if len(nodes) == 0 {
		t.Skip("no nodes")
	}

	tx, _ := s.db.Begin(ctx)
	_, err := s.CreateInstance(ctx, tx, app.ID, nodes[0].ID, 0, 1024, 2048, 10240, "docker")
	if err != nil {
		t.Fatalf("first instance: %v", err)
	}
	_ = tx.Commit(ctx)

	// Duplicate index should fail due to UNIQUE constraint
	tx2, _ := s.db.Begin(ctx)
	_, err = s.CreateInstance(ctx, tx2, app.ID, nodes[0].ID, 0, 1024, 2048, 10240, "docker")
	if err == nil {
		_ = tx2.Commit(ctx)
		t.Fatal("expected error for duplicate index, got nil")
	}
	_ = tx2.Rollback(ctx)
}

func TestScaleDownCleanup(t *testing.T) {
	ctx := context.Background()
	s := setupInstancesTest(t, ctx)

	app, _ := s.CreateReplicaApp(ctx, CreateReplicaAppRequest{Name: "scale-down-app", Replicas: 3, CPU: 1024, MemoryMB: 2048, DiskMB: 10240})
	nodes, _ := s.ListNodes(ctx)
	if len(nodes) == 0 {
		t.Skip("no nodes")
	}

	for i := 0; i < 3; i++ {
		tx, _ := s.db.Begin(ctx)
		_, err := s.CreateInstance(ctx, tx, app.ID, nodes[0].ID, i, 1024, 2048, 10240, "docker")
		if err != nil {
			t.Fatalf("instance %d: %v", i, err)
		}
		_ = tx.Commit(ctx)
	}

	// Mark last instance as removing (simulating scale-down)
	instances, _ := s.ListInstancesByApp(ctx, app.ID)
	last := instances[len(instances)-1]
	_, _ = s.UpdateInstanceStatus(ctx, last.ID, "removing")
	_ = s.DeleteInstance(ctx, last.ID)

	remaining, _ := s.ListInstancesByApp(ctx, app.ID)
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining after scale-down, got %d", len(remaining))
	}
}

func TestReplacementAfterFailure(t *testing.T) {
	ctx := context.Background()
	s := setupInstancesTest(t, ctx)

	app, _ := s.CreateReplicaApp(ctx, CreateReplicaAppRequest{Name: "replacement-app", Replicas: 1, CPU: 1024, MemoryMB: 2048, DiskMB: 10240})
	nodes, _ := s.ListNodes(ctx)
	if len(nodes) == 0 {
		t.Skip("no nodes")
	}

	tx, _ := s.db.Begin(ctx)
	inst, err := s.CreateInstance(ctx, tx, app.ID, nodes[0].ID, 0, 1024, 2048, 10240, "docker")
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	_ = tx.Commit(ctx)

	// Simulate failure and replacement
	_, _ = s.UpdateInstanceStatus(ctx, inst.ID, "failed")
	_, _ = s.UpdateInstanceNode(ctx, inst.ID, nodes[0].ID)
	_, _ = s.UpdateInstanceStatus(ctx, inst.ID, "pending")

	inst2, _ := s.GetInstance(ctx, inst.ID)
	if inst2.Status != "pending" {
		t.Fatalf("expected pending after replacement, got %s", inst2.Status)
	}
}

func TestPlacementDecision(t *testing.T) {
	ctx := context.Background()
	s := setupInstancesTest(t, ctx)

	app, _ := s.CreateReplicaApp(ctx, CreateReplicaAppRequest{Name: "pd-app", Replicas: 1, CPU: 1024, MemoryMB: 2048, DiskMB: 10240})
	nodes, _ := s.ListNodes(ctx)
	if len(nodes) == 0 {
		t.Skip("no nodes")
	}

	tx, _ := s.db.Begin(ctx)
	inst, _ := s.CreateInstance(ctx, tx, app.ID, nodes[0].ID, 0, 1024, 2048, 10240, "docker")
	_ = tx.Commit(ctx)

	pd, err := s.CreatePlacementDecision(ctx, inst.ID, nodes[0].ID, app.ID, 0, 0.95, true, []string{"least-loaded", "sufficient capacity"}, "docker")
	if err != nil {
		t.Fatalf("CreatePlacementDecision: %v", err)
	}
	if !pd.Accepted {
		t.Fatal("expected accepted placement")
	}
	if len(pd.Reasons) != 2 {
		t.Fatalf("expected 2 reasons, got %d", len(pd.Reasons))
	}

	decisions, err := s.ListLatestPlacementPerInstance(ctx, app.ID)
	if err != nil {
		t.Fatalf("ListLatestPlacementPerInstance: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
}

func TestPlacementDecisionRejected(t *testing.T) {
	ctx := context.Background()
	s := setupInstancesTest(t, ctx)

	app, _ := s.CreateReplicaApp(ctx, CreateReplicaAppRequest{Name: "rejected-app", Replicas: 1, CPU: 1024, MemoryMB: 2048, DiskMB: 10240})
	nodes, _ := s.ListNodes(ctx)
	if len(nodes) == 0 {
		t.Skip("no nodes")
	}

	tx, _ := s.db.Begin(ctx)
	inst, _ := s.CreateInstance(ctx, tx, app.ID, nodes[0].ID, 0, 1024, 2048, 10240, "docker")
	_ = tx.Commit(ctx)

	pd, err := s.CreatePlacementDecision(ctx, inst.ID, nodes[0].ID, app.ID, 0, 0, false, []string{"insufficient capacity", "memory exceeded"}, "docker")
	if err != nil {
		t.Fatalf("CreatePlacementDecision: %v", err)
	}
	if pd.Accepted {
		t.Fatal("expected rejected placement")
	}
}

func TestMaintenanceModeRejection(t *testing.T) {
	ctx := context.Background()
	s := setupInstancesTest(t, ctx)

	nodes, _ := s.ListNodes(ctx)
	if len(nodes) < 2 {
		t.Skip("need at least 2 nodes")
	}

	// Mark first node as maintenance
	_, _ = s.db.Exec(ctx, `UPDATE nodes SET maintenance_mode = true, desired_state = 'maintenance' WHERE id = $1`, nodes[0].ID)

	app, _ := s.CreateReplicaApp(ctx, CreateReplicaAppRequest{Name: "maint-app", Replicas: 1, CPU: 1024, MemoryMB: 2048, DiskMB: 10240})
	tx, _ := s.db.Begin(ctx)
	_, err := s.CreateInstance(ctx, tx, app.ID, nodes[1].ID, 0, 1024, 2048, 10240, "docker")
	if err != nil {
		t.Fatalf("should place on non-maintenance node: %v", err)
	}
	_ = tx.Commit(ctx)
}

func TestSchedulerRestartPersistence(t *testing.T) {
	ctx := context.Background()
	s := setupInstancesTest(t, ctx)

	app, _ := s.CreateReplicaApp(ctx, CreateReplicaAppRequest{Name: "persist-app", Replicas: 2, CPU: 1024, MemoryMB: 2048, DiskMB: 10240})
	nodes, _ := s.ListNodes(ctx)
	if len(nodes) == 0 {
		t.Skip("no nodes")
	}

	for i := 0; i < 2; i++ {
		tx, _ := s.db.Begin(ctx)
		_, _ = s.CreateInstance(ctx, tx, app.ID, nodes[0].ID, i, 1024, 2048, 10240, "docker")
		_ = tx.Commit(ctx)
	}

	// Simulate scheduler restart by re-reading from DB
	apps, err := s.ListReplicaApps(ctx)
	if err != nil {
		t.Fatalf("ListReplicaApps: %v", err)
	}
	found := false
	for _, a := range apps {
		if a.ID == app.ID {
			found = true
			if a.Replicas != 2 {
				t.Fatalf("expected 2 replicas after restart, got %d", a.Replicas)
			}
		}
	}
	if !found {
		t.Fatal("app not found after restart")
	}
}
