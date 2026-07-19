//go:build integration

package evacuationplanner

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gamepanel/forge/internal/store"
)

func plannerTestSvc(t *testing.T) (*Service, *pgxpool.Pool) {
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
	schema := "evacplanner_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = admin.Exec(cleanCtx, `DROP SCHEMA `+schema+` CASCADE`)
		admin.Close()
	})

	schemaURL := databaseURL
	if strings.Contains(databaseURL, "?") {
		schemaURL += "&search_path=" + schema
	} else {
		schemaURL += "?search_path=" + schema
	}

	s, err := store.ConnectWithKeyring(ctx, schemaURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)

	if err := s.RunMigrations(ctx, "../../../migrations"); err != nil {
		t.Fatal(err)
	}

	rawCfg, err := pgxpool.ParseConfig(schemaURL)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := pgxpool.NewWithConfig(ctx, rawCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(raw.Close)

	return New(s, nil), raw
}

func insertNodeRaw(t *testing.T, ctx context.Context, raw *pgxpool.Pool, id, locationID, regionID string, cpuThreads, memoryMB, diskMB int) {
	t.Helper()
	location := "NULL"
	if locationID != "" {
		location = "'" + locationID + "'"
	}
	rgn := "NULL"
	if regionID != "" {
		rgn = "'" + regionID + "'"
	}
	q := `INSERT INTO nodes (id, name, region, base_url, token_hash, location_id, region_id, status, desired_state, actual_state, cpu_threads, memory_mb, disk_mb, heartbeat_state)
	      VALUES ('` + id + `', '` + id + `', 'test', 'http://node.test:8080', 'hash', ` + location + `, ` + rgn + `, 'online', 'active', 'online', ` + itoa(cpuThreads) + `, ` + itoa(memoryMB) + `, ` + itoa(diskMB) + `, 'offline')`
	if _, err := raw.Exec(ctx, q); err != nil {
		t.Fatalf("insert node %s: %v", id, err)
	}
}

func insertServerRaw(t *testing.T, ctx context.Context, raw *pgxpool.Pool, id, nodeID string, cpu, memory, disk int) {
	t.Helper()
	q := `INSERT INTO servers (id, name, node_id, owner_id, egg_id, status, cpu_shares, memory_mb, disk_mb)
	      VALUES ('` + id + `', 'test', '` + nodeID + `', '` + uuid.NewString() + `', '` + uuid.NewString() + `', 'installing', ` + itoa(cpu) + `, ` + itoa(memory) + `, ` + itoa(disk) + `)`
	if _, err := raw.Exec(ctx, q); err != nil {
		t.Fatalf("insert server %s: %v", id, err)
	}
}

func insertLocationRaw(t *testing.T, ctx context.Context, raw *pgxpool.Pool, id string) {
	t.Helper()
	if _, err := raw.Exec(ctx, `INSERT INTO locations (id, short, long) VALUES ($1, 'test', 'Test')`, id); err != nil {
		t.Fatal(err)
	}
}

func insertRegionRaw(t *testing.T, ctx context.Context, raw *pgxpool.Pool, id string) {
	t.Helper()
	if _, err := raw.Exec(ctx, `INSERT INTO regions (id, uuid, name, slug, enabled) VALUES ($1, $1, 'Test', 'test', true)`, id); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestCreatePlanIntegration(t *testing.T) {
	svc, raw := plannerTestSvc(t)
	ctx := context.Background()

	locID := uuid.NewString()
	insertLocationRaw(t, ctx, raw, locID)
	regID := uuid.NewString()
	insertRegionRaw(t, ctx, raw, regID)

	sourceNodeID := uuid.NewString()
	targetNodeID := uuid.NewString()
	insertNodeRaw(t, ctx, raw, sourceNodeID, locID, regID, 8, 65536, 512000)
	insertNodeRaw(t, ctx, raw, targetNodeID, locID, regID, 8, 65536, 512000)

	serverID := uuid.NewString()
	insertServerRaw(t, ctx, raw, serverID, sourceNodeID, 1024, 2048, 10240)

	result, err := svc.CreatePlan(ctx, sourceNodeID)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	if result.Preview {
		t.Fatal("expected Preview=false")
	}
	if result.Plan.ID == "" {
		t.Fatal("expected a plan ID")
	}
	if result.Plan.NodeID != sourceNodeID {
		t.Fatalf("NodeID = %q, want %q", result.Plan.NodeID, sourceNodeID)
	}
	if result.Plan.Status != store.EvacuationPlanStatusPending {
		t.Fatalf("Status = %q, want %q", result.Plan.Status, store.EvacuationPlanStatusPending)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].ServerID != serverID {
		t.Fatalf("item ServerID = %q, want %q", result.Items[0].ServerID, serverID)
	}
	if !result.Items[0].Eligible {
		t.Fatal("expected item to be eligible")
	}
	if result.Items[0].TargetNodeID != targetNodeID {
		t.Fatalf("TargetNodeID = %q, want %q", result.Items[0].TargetNodeID, targetNodeID)
	}
}

func TestCreatePlanIntegrationNoCandidates(t *testing.T) {
	svc, raw := plannerTestSvc(t)
	ctx := context.Background()

	locID := uuid.NewString()
	insertLocationRaw(t, ctx, raw, locID)
	regID := uuid.NewString()
	insertRegionRaw(t, ctx, raw, regID)

	sourceNodeID := uuid.NewString()
	insertNodeRaw(t, ctx, raw, sourceNodeID, locID, regID, 2, 4096, 51200)

	serverID := uuid.NewString()
	insertServerRaw(t, ctx, raw, serverID, sourceNodeID, 4096, 8192, 102400)

	result, err := svc.CreatePlan(ctx, sourceNodeID)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	if result.Plan.Status != store.EvacuationPlanStatusFailed {
		t.Fatalf("Status = %q, want %q", result.Plan.Status, store.EvacuationPlanStatusFailed)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].Eligible {
		t.Fatal("expected item to be ineligible")
	}
}

func TestGetPlanIntegration(t *testing.T) {
	svc, raw := plannerTestSvc(t)
	ctx := context.Background()

	locID := uuid.NewString()
	insertLocationRaw(t, ctx, raw, locID)
	regID := uuid.NewString()
	insertRegionRaw(t, ctx, raw, regID)

	sourceNodeID := uuid.NewString()
	targetNodeID := uuid.NewString()
	insertNodeRaw(t, ctx, raw, sourceNodeID, locID, regID, 8, 65536, 512000)
	insertNodeRaw(t, ctx, raw, targetNodeID, locID, regID, 8, 65536, 512000)

	serverID := uuid.NewString()
	insertServerRaw(t, ctx, raw, serverID, sourceNodeID, 1024, 2048, 10240)

	created, err := svc.CreatePlan(ctx, sourceNodeID)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	fetched, err := svc.GetPlan(ctx, created.Plan.ID)
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if fetched.ID != created.Plan.ID {
		t.Fatalf("plan ID mismatch: %q vs %q", fetched.ID, created.Plan.ID)
	}
}

func TestExecutePlanIntegration(t *testing.T) {
	svc, raw := plannerTestSvc(t)
	ctx := context.Background()

	locID := uuid.NewString()
	insertLocationRaw(t, ctx, raw, locID)
	regID := uuid.NewString()
	insertRegionRaw(t, ctx, raw, regID)

	sourceNodeID := uuid.NewString()
	targetNodeID := uuid.NewString()
	insertNodeRaw(t, ctx, raw, sourceNodeID, locID, regID, 8, 65536, 512000)
	insertNodeRaw(t, ctx, raw, targetNodeID, locID, regID, 8, 65536, 512000)

	serverID := uuid.NewString()
	insertServerRaw(t, ctx, raw, serverID, sourceNodeID, 1024, 2048, 10240)

	created, err := svc.CreatePlan(ctx, sourceNodeID)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	executor := &mockExecutor{}
	svc.SetMigrationExecutor(executor)

	plan, err := svc.ExecutePlan(ctx, created.Plan.ID)
	if err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}
	if plan.Status != store.EvacuationPlanStatusRunning {
		t.Fatalf("Status = %q, want %q", plan.Status, store.EvacuationPlanStatusRunning)
	}
}

func TestCancelPlanIntegration(t *testing.T) {
	svc, raw := plannerTestSvc(t)
	ctx := context.Background()

	locID := uuid.NewString()
	insertLocationRaw(t, ctx, raw, locID)
	regID := uuid.NewString()
	insertRegionRaw(t, ctx, raw, regID)

	sourceNodeID := uuid.NewString()
	targetNodeID := uuid.NewString()
	insertNodeRaw(t, ctx, raw, sourceNodeID, locID, regID, 8, 65536, 512000)
	insertNodeRaw(t, ctx, raw, targetNodeID, locID, regID, 8, 65536, 512000)

	serverID := uuid.NewString()
	insertServerRaw(t, ctx, raw, serverID, sourceNodeID, 1024, 2048, 10240)

	created, err := svc.CreatePlan(ctx, sourceNodeID)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}

	executor := &mockExecutor{}
	svc.SetMigrationExecutor(executor)

	_, err = svc.ExecutePlan(ctx, created.Plan.ID)
	if err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}

	plan, err := svc.CancelPlan(ctx, created.Plan.ID)
	if err != nil {
		t.Fatalf("CancelPlan: %v", err)
	}
	if plan.Status != store.EvacuationPlanStatusCancelled {
		t.Fatalf("Status = %q, want %q", plan.Status, store.EvacuationPlanStatusCancelled)
	}
}

func TestPreviewPlanIntegration(t *testing.T) {
	svc, raw := plannerTestSvc(t)
	ctx := context.Background()

	locID := uuid.NewString()
	insertLocationRaw(t, ctx, raw, locID)
	regID := uuid.NewString()
	insertRegionRaw(t, ctx, raw, regID)

	sourceNodeID := uuid.NewString()
	targetNodeID := uuid.NewString()
	insertNodeRaw(t, ctx, raw, sourceNodeID, locID, regID, 8, 65536, 512000)
	insertNodeRaw(t, ctx, raw, targetNodeID, locID, regID, 8, 65536, 512000)

	serverID := uuid.NewString()
	insertServerRaw(t, ctx, raw, serverID, sourceNodeID, 1024, 2048, 10240)

	result, err := svc.PreviewPlan(ctx, sourceNodeID)
	if err != nil {
		t.Fatalf("PreviewPlan: %v", err)
	}

	if !result.Preview {
		t.Fatal("expected Preview=true")
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
}

func TestFindCandidatesWithCapacityIntegration(t *testing.T) {
	svc, raw := plannerTestSvc(t)
	ctx := context.Background()

	locID := uuid.NewString()
	insertLocationRaw(t, ctx, raw, locID)
	regID := uuid.NewString()
	insertRegionRaw(t, ctx, raw, regID)

	sourceNodeID := uuid.NewString()
	targetNodeID := uuid.NewString()
	insertNodeRaw(t, ctx, raw, sourceNodeID, locID, regID, 4, 16384, 102400)
	insertNodeRaw(t, ctx, raw, targetNodeID, locID, regID, 8, 65536, 512000)

	allNodes, err := svc.store.ListNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}

	sourceNode := findNodeByID(t, allNodes, sourceNodeID)
	server := store.Server{ID: "test-server", CPUShares: 1024, MemoryMB: 2048, DiskMB: 10240}

	node, impact, reason := svc.FindCandidates(ctx, server, sourceNode, allNodes)
	if node.ID == "" {
		t.Fatalf("expected a candidate, got reason: %s", reason)
	}
	if node.ID == sourceNodeID {
		t.Fatal("should not select source node")
	}
	if impact == nil {
		t.Fatal("expected capacity impact")
	}
	if reason == "" {
		t.Fatal("expected a reason string")
	}
}

func findNodeByID(t *testing.T, nodes []store.Node, id string) store.Node {
	t.Helper()
	for _, n := range nodes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("node %s not found", id)
	return store.Node{}
}
