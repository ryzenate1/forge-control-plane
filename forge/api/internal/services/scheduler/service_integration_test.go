//go:build integration

package scheduler

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/placement"
	"gamepanel/forge/internal/store"
)

func schedulerTestSvc(t *testing.T) (*Scheduler, *pgxpool.Pool) {
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
	schema := "scheduler_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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

	engine := placement.NewEngine(placement.NewScorer(placement.StrategyLeastLoaded), placement.NewConstraintChecker())
	return New(s, engine), raw
}

func insertLocation(t *testing.T, ctx context.Context, raw *pgxpool.Pool, id string) {
	t.Helper()
	if _, err := raw.Exec(ctx, `INSERT INTO locations (id, short, long) VALUES ($1, 'test', 'Test Location')`, id); err != nil {
		t.Fatal(err)
	}
}

func insertRegion(t *testing.T, ctx context.Context, raw *pgxpool.Pool, id, name, slug string, enabled bool) {
	t.Helper()
	if _, err := raw.Exec(ctx, `INSERT INTO regions (id, uuid, name, slug, enabled) VALUES ($1, $1, $2, $3, $4)`, id, name, slug, enabled); err != nil {
		t.Fatal(err)
	}
}

func insertNode(t *testing.T, ctx context.Context, raw *pgxpool.Pool, id, locationID, regionID string, overrides map[string]any) {
	t.Helper()
	name := id
	if n, ok := overrides["name"]; ok {
		name = n.(string)
	}
	region := "test"
	if r, ok := overrides["region"]; ok {
		region = r.(string)
	}
	status := "online"
	if s, ok := overrides["status"]; ok {
		status = s.(string)
	}
	desiredState := "active"
	if d, ok := overrides["desired_state"]; ok {
		desiredState = d.(string)
	}
	actualState := "online"
	if a, ok := overrides["actual_state"]; ok {
		actualState = a.(string)
	}
	maintenance := false
	if m, ok := overrides["maintenance_mode"]; ok {
		maintenance = m.(bool)
	}
	draining := false
	if d, ok := overrides["draining"]; ok {
		draining = d.(bool)
	}
	cpuThreads := 4
	if c, ok := overrides["cpu_threads"]; ok {
		cpuThreads = c.(int)
	}
	memoryMB := 16384
	if m, ok := overrides["memory_mb"]; ok {
		memoryMB = m.(int)
	}
	diskMB := 102400
	if d, ok := overrides["disk_mb"]; ok {
		diskMB = d.(int)
	}

	location := "NULL"
	if locationID != "" {
		location = "'" + locationID + "'"
	}
	rgn := "NULL"
	if regionID != "" {
		rgn = "'" + regionID + "'"
	}

	q := fmt.Sprintf(`
		INSERT INTO nodes (id, name, region, base_url, token_hash, location_id, region_id, status, desired_state, actual_state, maintenance_mode, draining, cpu_threads, memory_mb, disk_mb, heartbeat_state)
		VALUES ('%s', '%s', '%s', 'http://node.test:8080', 'hash', %s, %s, '%s', '%s', '%s', %t, %t, %d, %d, %d, 'offline')
	`, id, name, region, location, rgn, status, desiredState, actualState, maintenance, draining, cpuThreads, memoryMB, diskMB)

	if _, err := raw.Exec(ctx, q); err != nil {
		t.Fatalf("insert node %s: %v\nquery: %s", id, err, q)
	}
}

func TestFilterNodes(t *testing.T) {
	svc, raw := schedulerTestSvc(t)
	ctx := context.Background()

	locID := uuid.NewString()
	insertLocation(t, ctx, raw, locID)

	enabledRegion := uuid.NewString()
	disabledRegion := uuid.NewString()
	otherRegion := uuid.NewString()
	insertRegion(t, ctx, raw, enabledRegion, "Enabled", "enabled", true)
	insertRegion(t, ctx, raw, disabledRegion, "Disabled", "disabled", false)
	insertRegion(t, ctx, raw, otherRegion, "Other", "other", true)

	goodNode := uuid.NewString()
	insertNode(t, ctx, raw, goodNode, locID, enabledRegion, nil)

	maintNode := uuid.NewString()
	insertNode(t, ctx, raw, maintNode, locID, enabledRegion, map[string]any{"desired_state": "maintenance"})

	drainNode := uuid.NewString()
	insertNode(t, ctx, raw, drainNode, locID, enabledRegion, map[string]any{"desired_state": "draining"})

	maintFlagNode := uuid.NewString()
	insertNode(t, ctx, raw, maintFlagNode, locID, enabledRegion, map[string]any{"maintenance_mode": true})

	drainFlagNode := uuid.NewString()
	insertNode(t, ctx, raw, drainFlagNode, locID, enabledRegion, map[string]any{"draining": true})

	offlineNode := uuid.NewString()
	insertNode(t, ctx, raw, offlineNode, locID, enabledRegion, map[string]any{"actual_state": "offline"})

	wrongRegionNode := uuid.NewString()
	insertNode(t, ctx, raw, wrongRegionNode, locID, otherRegion, nil)

	disabledRegionNode := uuid.NewString()
	insertNode(t, ctx, raw, disabledRegionNode, locID, disabledRegion, nil)

	nodes, err := svc.store.ListNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) == 0 {
		t.Fatal("no nodes listed; schema or migrations may not be set up correctly")
	}

	nodeMap := make(map[string]store.Node, len(nodes))
	for _, n := range nodes {
		nodeMap[n.ID] = n
	}

	for _, id := range []string{goodNode, maintNode, drainNode, maintFlagNode, drainFlagNode, offlineNode, wrongRegionNode, disabledRegionNode} {
		if _, ok := nodeMap[id]; !ok {
			t.Fatalf("node %s not found in ListNodes result", id)
		}
	}

	t.Run("filters by state and region", func(t *testing.T) {
		req := domain.PlacementRequest{RegionID: enabledRegion, CPU: 1024, MemoryMB: 2048, DiskMB: 10240}
		filtered, err := svc.FilterNodes(ctx, req, nodes)
		if err != nil {
			t.Fatal(err)
		}

		filteredIDs := make(map[string]bool, len(filtered))
		for _, n := range filtered {
			filteredIDs[n.ID] = true
		}

		if !filteredIDs[goodNode] {
			t.Errorf("good node %s should have passed filtering", goodNode)
		}
		if filteredIDs[maintNode] {
			t.Errorf("maintenance desired_state node %s should have been filtered", maintNode)
		}
		if filteredIDs[drainNode] {
			t.Errorf("draining desired_state node %s should have been filtered", drainNode)
		}
		if filteredIDs[maintFlagNode] {
			t.Errorf("maintenance_mode node %s should have been filtered", maintFlagNode)
		}
		if filteredIDs[drainFlagNode] {
			t.Errorf("draining flag node %s should have been filtered", drainFlagNode)
		}
		if filteredIDs[offlineNode] {
			t.Errorf("offline actual_state node %s should have been filtered", offlineNode)
		}
		if filteredIDs[wrongRegionNode] {
			t.Errorf("wrong region node %s should have been filtered", wrongRegionNode)
		}
		if filteredIDs[disabledRegionNode] {
			t.Errorf("disabled region node %s should have been filtered", disabledRegionNode)
		}

		if len(filtered) != 1 {
			t.Errorf("expected exactly 1 node to pass filtering, got %d: %v", len(filtered), filteredIDs)
		}
	})

	t.Run("required node preserves only matching node", func(t *testing.T) {
		req := domain.PlacementRequest{RequiredNode: goodNode, CPU: 1024, MemoryMB: 2048, DiskMB: 10240}
		filtered, err := svc.FilterNodes(ctx, req, nodes)
		if err != nil {
			t.Fatal(err)
		}
		if len(filtered) != 1 || filtered[0].ID != goodNode {
			t.Errorf("expected only required node %s, got %d nodes", goodNode, len(filtered))
		}
	})

	t.Run("required node filters when none match", func(t *testing.T) {
		req := domain.PlacementRequest{RequiredNode: "nonexistent-node", CPU: 1024, MemoryMB: 2048, DiskMB: 10240}
		filtered, err := svc.FilterNodes(ctx, req, nodes)
		if err != nil {
			t.Fatal(err)
		}
		if len(filtered) != 0 {
			t.Errorf("expected 0 nodes, got %d", len(filtered))
		}
	})
}

func TestScoreNodes(t *testing.T) {
	svc, raw := schedulerTestSvc(t)
	ctx := context.Background()

	locID := uuid.NewString()
	insertLocation(t, ctx, raw, locID)

	regionID := uuid.NewString()
	insertRegion(t, ctx, raw, regionID, "Test", "test", true)

	smallNode := uuid.NewString()
	insertNode(t, ctx, raw, smallNode, locID, regionID, map[string]any{
		"cpu_threads": 2,
		"memory_mb":   8192,
		"disk_mb":     51200,
	})

	largeNode := uuid.NewString()
	insertNode(t, ctx, raw, largeNode, locID, regionID, map[string]any{
		"cpu_threads": 8,
		"memory_mb":   65536,
		"disk_mb":     512000,
	})

	nodes, err := svc.store.ListNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var smallFound, largeFound bool
	allNodes := make([]store.Node, 0, 2)
	for _, n := range nodes {
		if n.ID == smallNode {
			smallFound = true
			allNodes = append(allNodes, n)
		}
		if n.ID == largeNode {
			largeFound = true
			allNodes = append(allNodes, n)
		}
	}
	if !smallFound || !largeFound {
		t.Fatal("test nodes not found in ListNodes result")
	}

	t.Run("scores reflect available capacity", func(t *testing.T) {
		req := domain.PlacementRequest{RegionID: regionID, CPU: 1024, MemoryMB: 2048, DiskMB: 10240}
		scores, err := svc.ScoreNodes(ctx, req, allNodes)
		if err != nil {
			t.Fatal(err)
		}
		if len(scores) != 2 {
			t.Fatalf("expected 2 scores, got %d", len(scores))
		}

		var smallScore, largeScore float64
		for _, sc := range scores {
			if sc.Node.ID == smallNode {
				smallScore = sc.Score
			}
			if sc.Node.ID == largeNode {
				largeScore = sc.Score
			}
		}
		if largeScore <= smallScore {
			t.Errorf("large node score %f should exceed small node score %f", largeScore, smallScore)
		}
	})

	t.Run("preferred node gets bonus", func(t *testing.T) {
		req := domain.PlacementRequest{RegionID: regionID, PreferredNode: smallNode, CPU: 1024, MemoryMB: 2048, DiskMB: 10240}
		scores, err := svc.ScoreNodes(ctx, req, allNodes)
		if err != nil {
			t.Fatal(err)
		}

		var smallReason string
		preferredFound := false
		for _, sc := range scores {
			if sc.Node.ID == smallNode {
				preferredFound = true
				smallReason = sc.Reason
			}
		}
		if !preferredFound {
			t.Fatal("preferred node not found in scores")
		}
		if smallReason != "preferred node" {
			t.Errorf("preferred node reason = %q, want 'preferred node'", smallReason)
		}
	})

	t.Run("required node gets correct reason", func(t *testing.T) {
		req := domain.PlacementRequest{RequiredNode: largeNode, CPU: 1024, MemoryMB: 2048, DiskMB: 10240}
		scores, err := svc.ScoreNodes(ctx, req, allNodes)
		if err != nil {
			t.Fatal(err)
		}
		if len(scores) == 0 {
			t.Fatal("expected at least one score")
		}

		for _, sc := range scores {
			if sc.Node.ID == largeNode && !strings.Contains(sc.Reason, "available memory") {
				t.Errorf("required node reason = %q, want to contain 'available memory'", sc.Reason)
			}
			if sc.Node.ID == smallNode && !strings.Contains(sc.Reason, "available memory") {
				t.Errorf("non-required node reason = %q, want to contain 'available memory'", sc.Reason)
			}
		}
	})
}

func TestPlaceServer(t *testing.T) {
	svc, raw := schedulerTestSvc(t)
	ctx := context.Background()

	locID := uuid.NewString()
	insertLocation(t, ctx, raw, locID)

	regionID := uuid.NewString()
	insertRegion(t, ctx, raw, regionID, "Test", "test", true)

	smallNode := uuid.NewString()
	insertNode(t, ctx, raw, smallNode, locID, regionID, map[string]any{
		"cpu_threads": 2,
		"memory_mb":   8192,
		"disk_mb":     51200,
	})

	largeNode := uuid.NewString()
	insertNode(t, ctx, raw, largeNode, locID, regionID, map[string]any{
		"cpu_threads": 8,
		"memory_mb":   65536,
		"disk_mb":     512000,
	})

	t.Run("places on best available node", func(t *testing.T) {
		req := domain.PlacementRequest{
			RegionID:     regionID,
			CPU:          1024,
			MemoryMB:     2048,
			DiskMB:       10240,
			AllocationID: "test-alloc",
		}
		decision, err := svc.PlaceServer(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		if decision.NodeID != largeNode {
			t.Errorf("expected placement on large node %s, got %s", largeNode, decision.NodeID)
		}
		if decision.RegionID != regionID {
			t.Errorf("expected region %s, got %s", regionID, decision.RegionID)
		}
		if decision.AllocationID != "test-alloc" {
			t.Errorf("expected allocation test-alloc, got %s", decision.AllocationID)
		}
		if decision.Score <= 0 {
			t.Errorf("expected positive score, got %f", decision.Score)
		}
	})

	t.Run("places on preferred node", func(t *testing.T) {
		req := domain.PlacementRequest{
			RegionID:      regionID,
			PreferredNode: smallNode,
			CPU:           1024,
			MemoryMB:      2048,
			DiskMB:        10240,
			AllocationID:  "pref-alloc",
		}
		decision, err := svc.PlaceServer(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		if decision.NodeID != smallNode {
			t.Errorf("expected placement on preferred small node %s, got %s", smallNode, decision.NodeID)
		}
		if !decision.Manual {
			t.Errorf("decision.Manual should be false for preferred node")
		}
	})

	t.Run("places on required node", func(t *testing.T) {
		req := domain.PlacementRequest{
			RequiredNode:  smallNode,
			CPU:           1024,
			MemoryMB:      2048,
			DiskMB:        10240,
			AllocationID:  "req-alloc",
		}
		decision, err := svc.PlaceServer(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		if decision.NodeID != smallNode {
			t.Errorf("expected placement on required node %s, got %s", smallNode, decision.NodeID)
		}
		if !decision.Manual {
			t.Errorf("decision.Manual should be true for required node")
		}
	})

	t.Run("fails when no region or required node specified", func(t *testing.T) {
		req := domain.PlacementRequest{
			CPU:      1024,
			MemoryMB: 2048,
			DiskMB:   10240,
		}
		_, err := svc.PlaceServer(ctx, req)
		if err == nil {
			t.Fatal("expected error for missing region and required node")
		}
	})

	t.Run("fails when no nodes satisfy constraints", func(t *testing.T) {
		req := domain.PlacementRequest{
			RegionID:     regionID,
			RequiredNode: "nonexistent",
			CPU:          1024,
			MemoryMB:     2048,
			DiskMB:       10240,
		}
		_, err := svc.PlaceServer(ctx, req)
		if err == nil {
			t.Fatal("expected error for nonexistent required node")
		}
	})
}
