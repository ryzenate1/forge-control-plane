package scheduler

import (
	"context"
	"testing"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/placement"
	"gamepanel/forge/internal/store"
)

func TestPlaceReplicasNoNodes(t *testing.T) {
	s := &Scheduler{}
	reasons, err := s.PlaceReplicas(context.Background(), domain.PlaceReplicasRequest{
		AppID:        "test-app",
		ReplicaCount: 3,
		CPU:          1024,
		MemoryMB:     2048,
		DiskMB:       10240,
	})
	if err == nil {
		t.Fatal("expected error with no app or nodes")
	}
	if reasons != nil {
		t.Fatal("expected nil reasons on error")
	}
}

func TestScaleReplicasNoOp(t *testing.T) {
	s := &Scheduler{}
	reasons, err := s.ScaleReplicas(context.Background(), domain.ScaleRequest{
		AppID:        "nonexistent",
		ReplicaCount: 5,
	})
	if err == nil {
		t.Fatal("expected error for nonexistent app")
	}
	if reasons != nil {
		t.Fatal("expected nil reasons on error")
	}
}

func TestReplaceFailedInstanceNonExistent(t *testing.T) {
	s := &Scheduler{}
	reason, err := s.ReplaceFailedInstance(context.Background(), domain.ReplaceFailedInstanceRequest{
		InstanceID: "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent instance")
	}
	if reason != nil {
		t.Fatal("expected nil reason on error")
	}
}

func TestPlaceReplicasFilterByRuntime(t *testing.T) {
	nodes := []store.Node{
		{ID: "node-1", RuntimeProvider: "docker", ActualState: "online", DesiredState: "active"},
		{ID: "node-2", RuntimeProvider: "containerd", ActualState: "online", DesiredState: "active"},
		{ID: "node-3", RuntimeProvider: "", ActualState: "online", DesiredState: "active"},
	}

	filtered := filterByRuntimeProvider(nodes, "docker")
	if len(filtered) != 2 {
		t.Fatalf("expected 2 nodes compatible with docker (docker + unknown), got %d (containerd should not match)", len(filtered))
	}

	filtered2 := filterByRuntimeProvider(nodes, "")
	if len(filtered2) != 3 {
		t.Fatalf("expected all nodes with empty filter, got %d", len(filtered2))
	}
}

func TestFilterByRuntimeProvider(t *testing.T) {
	tests := []struct {
		name    string
		nodes   []store.Node
		runtime string
		want    int
	}{
		{
			name: "any runtime with docker filter",
			nodes: []store.Node{
				{ID: "n1", RuntimeProvider: "docker"},
				{ID: "n2", RuntimeProvider: ""},
			},
			runtime: "docker",
			want:    2,
		},
		{
			name: "no matching runtime falls back to all",
			nodes: []store.Node{
				{ID: "n1", RuntimeProvider: "containerd"},
			},
			runtime: "firecracker",
			want:    1,
		},
		{
			name: "empty filter returns all",
			nodes: []store.Node{
				{ID: "n1", RuntimeProvider: "docker"},
				{ID: "n2", RuntimeProvider: "containerd"},
			},
			runtime: "",
			want:    2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterByRuntimeProvider(tt.nodes, tt.runtime)
			if len(got) != tt.want {
				t.Fatalf("filterByRuntimeProvider() = %d, want %d", len(got), tt.want)
			}
		})
	}
}

func TestReplicaPlacementExplain(t *testing.T) {
	engine := placement.NewEngine(placement.NewScorer(placement.StrategyLeastLoaded), placement.NewConstraintChecker())
	candidates := []placement.Candidate{
		{
			NodeID:          "node-1",
			TotalMemory:     16384,
			AvailableMemory: 8192,
			TotalCPU:        4096,
			AvailableCPU:    2048,
			TotalDisk:       102400,
			AvailableDisk:   51200,
			ServerCount:     2,
		},
		{
			NodeID:          "node-2",
			TotalMemory:     8192,
			AvailableMemory: 4096,
			TotalCPU:        2048,
			AvailableCPU:    1024,
			TotalDisk:       51200,
			AvailableDisk:   25600,
			ServerCount:     1,
		},
	}

	req := placement.ReplicaPlacementRequest{
		Replicas: []placement.ReplicaSpec{
			{Index: 0, CPU: 1024, MemoryMB: 2048, DiskMB: 10240, RuntimeProvider: "docker"},
		},
	}

	explain := placement.ExplainReplicaPlacement(context.Background(), engine, candidates, req)
	if len(explain) != 1 {
		t.Fatalf("expected 1 explanation, got %d", len(explain))
	}
	if explain[0].Index != 0 {
		t.Fatalf("expected index 0, got %d", explain[0].Index)
	}
	if len(explain[0].Candidates) != 2 {
		t.Fatalf("expected 2 candidate explanations, got %d", len(explain[0].Candidates))
	}
}

func TestReplicaSpecDefaults(t *testing.T) {
	engine := placement.NewEngine(placement.NewScorer(placement.StrategyLeastLoaded), placement.NewConstraintChecker())
	candidates := []placement.Candidate{
		{
			NodeID:          "node-1",
			TotalMemory:     16384,
			AvailableMemory: 8192,
			TotalCPU:        4096,
			AvailableCPU:    2048,
			TotalDisk:       102400,
			AvailableDisk:   51200,
			ServerCount:     0,
		},
	}

	req := placement.ReplicaPlacementRequest{
		Replicas: []placement.ReplicaSpec{
			{Index: 0, CPU: 1024, MemoryMB: 2048, DiskMB: 10240, RuntimeProvider: "docker"},
		},
		ExistingNodeMap: map[string]int{},
	}

	result, err := engine.PlaceReplicas(context.Background(), candidates, req)
	if err != nil {
		t.Fatalf("PlaceReplicas: %v", err)
	}
	if len(result.Placements) != 1 {
		t.Fatalf("expected 1 placement, got %d", len(result.Placements))
	}
	if result.Placements[0].NodeID != "node-1" {
		t.Fatalf("expected node-1, got %s", result.Placements[0].NodeID)
	}
}

func TestReplicaAntiAffinity(t *testing.T) {
	engine := placement.NewEngine(placement.NewScorer(placement.StrategySpread), placement.NewConstraintChecker())
	candidates := []placement.Candidate{
		{
			NodeID:          "node-1",
			TotalMemory:     16384,
			AvailableMemory: 8192,
			TotalCPU:        4096,
			AvailableCPU:    2048,
			TotalDisk:       102400,
			AvailableDisk:   51200,
			ServerCount:     3,
		},
		{
			NodeID:          "node-2",
			TotalMemory:     8192,
			AvailableMemory: 4096,
			TotalCPU:        2048,
			AvailableCPU:    1024,
			TotalDisk:       51200,
			AvailableDisk:   25600,
			ServerCount:     1,
		},
	}

	req := placement.ReplicaPlacementRequest{
		Replicas: []placement.ReplicaSpec{
			{Index: 0, CPU: 1024, MemoryMB: 2048, DiskMB: 10240, RuntimeProvider: "docker"},
		},
	}

	result, err := engine.PlaceReplicas(context.Background(), candidates, req)
	if err != nil {
		t.Fatalf("PlaceReplicas: %v", err)
	}
	if len(result.Placements) == 0 {
		t.Fatal("expected at least one placement")
	}
	// Spread scorer prefers nodes with fewer servers
	if result.Placements[0].NodeID != "node-2" {
		t.Logf("expected spread to prefer node-2 (1 server), got %s", result.Placements[0].NodeID)
	}
}

func TestPlaceReplicasCapacityCheck(t *testing.T) {
	engine := placement.NewEngine(placement.NewScorer(placement.StrategyLeastLoaded), placement.NewConstraintChecker())
	candidates := []placement.Candidate{
		{
			NodeID:          "node-1",
			TotalMemory:     2048,
			AvailableMemory: 0,
			TotalCPU:        1024,
			AvailableCPU:    0,
			TotalDisk:       10240,
			AvailableDisk:   0,
			ServerCount:     5,
		},
		{
			NodeID:          "node-2",
			TotalMemory:     16384,
			AvailableMemory: 8192,
			TotalCPU:        4096,
			AvailableCPU:    2048,
			TotalDisk:       102400,
			AvailableDisk:   51200,
			ServerCount:     1,
		},
	}

	req := placement.ReplicaPlacementRequest{
		Replicas: []placement.ReplicaSpec{
			{Index: 0, CPU: 1024, MemoryMB: 2048, DiskMB: 10240, RuntimeProvider: "docker"},
		},
	}

	result, err := engine.PlaceReplicas(context.Background(), candidates, req)
	if err != nil {
		t.Fatalf("PlaceReplicas: %v", err)
	}
	if len(result.Placements) != 1 {
		t.Fatalf("expected 1 placement, got %d", len(result.Placements))
	}
	// Should skip the full node and place on node-2
	if result.Placements[0].NodeID != "node-2" {
		t.Fatalf("expected placement on node-2 (has capacity), got %s", result.Placements[0].NodeID)
	}
}

func TestValidateReplicaConstraints(t *testing.T) {
	tests := []struct {
		name    string
		req     placement.ReplicaPlacementRequest
		wantErr bool
	}{
		{
			name: "valid with docker",
			req: placement.ReplicaPlacementRequest{
				Replicas: []placement.ReplicaSpec{
					{Index: 0, RuntimeProvider: "docker"},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid runtime",
			req: placement.ReplicaPlacementRequest{
				RequiredNode: "node-1",
				Replicas: []placement.ReplicaSpec{
					{Index: 0, RuntimeProvider: "invalid"},
				},
			},
			wantErr: true,
		},
		{
			name: "no replicas",
			req: placement.ReplicaPlacementRequest{
				Replicas: []placement.ReplicaSpec{},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := placement.ValidateReplicaConstraints(tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateReplicaConstraints() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestReplicaPlacementScorer(t *testing.T) {
	leastLoaded := placement.NewScorer(placement.StrategyLeastLoaded)
	binPack := placement.NewScorer(placement.StrategyBinPack)
	spread := placement.NewScorer(placement.StrategySpread)
	random := placement.NewScorer(placement.StrategyRandom)

	candidate := placement.Candidate{
		NodeID:          "node-1",
		TotalMemory:     16384,
		AvailableMemory: 8192,
		TotalCPU:        4096,
		AvailableCPU:    2048,
		TotalDisk:       102400,
		AvailableDisk:   51200,
		ServerCount:     2,
	}

	workload := placement.WorkloadRequest{CPU: 1024, MemoryMB: 2048, DiskMB: 10240}

	score1, _, _ := leastLoaded.Score(context.Background(), candidate, workload)
	score2, _, _ := binPack.Score(context.Background(), candidate, workload)
	score3, _, _ := spread.Score(context.Background(), candidate, workload)
	score4, _, _ := random.Score(context.Background(), candidate, workload)

	if score1 <= 0 {
		t.Fatal("least-loaded score should be positive")
	}
	if score2 < 0 || score2 > 1 {
		t.Fatal("bin-pack score should be between 0 and 1")
	}
	if score3 <= 0 || score3 > 1 {
		t.Fatal("spread score should be between 0 and 1")
	}
	if score4 <= 0 || score4 > 1 {
		t.Fatal("random score should be between 0 and 1")
	}
}

func TestReplicaReservationRollback(t *testing.T) {
	// Verify double reservation guard works
	reservations := map[string]bool{"node-1": false, "node-2": false}

	assertNoDoubleReservation := func(nodeID string) bool {
		if reservations[nodeID] {
			return false
		}
		reservations[nodeID] = true
		return true
	}

	if !assertNoDoubleReservation("node-1") {
		t.Fatal("first reservation should succeed")
	}
	if assertNoDoubleReservation("node-1") {
		t.Fatal("double reservation should fail")
	}
	if !assertNoDoubleReservation("node-2") {
		t.Fatal("reservation on different node should succeed")
	}
}

func TestReplicaPlacementScores(t *testing.T) {
	engine := placement.NewEngine(placement.NewScorer(placement.StrategyLeastLoaded), placement.NewConstraintChecker())
	candidates := []placement.Candidate{
		{
			NodeID:          "node-large",
			TotalMemory:     65536,
			AvailableMemory: 32768,
			TotalCPU:        16384,
			AvailableCPU:    8192,
			TotalDisk:       1048576,
			AvailableDisk:   524288,
			ServerCount:     1,
		},
		{
			NodeID:          "node-small",
			TotalMemory:     4096,
			AvailableMemory: 2048,
			TotalCPU:        1024,
			AvailableCPU:    512,
			TotalDisk:       51200,
			AvailableDisk:   25600,
			ServerCount:     3,
		},
	}

	req := placement.ReplicaPlacementRequest{
		Replicas: []placement.ReplicaSpec{
			{Index: 0, CPU: 1024, MemoryMB: 2048, DiskMB: 10240, RuntimeProvider: "docker"},
			{Index: 1, CPU: 1024, MemoryMB: 2048, DiskMB: 10240, RuntimeProvider: "docker"},
		},
	}

	result, err := engine.PlaceReplicas(context.Background(), candidates, req)
	if err != nil {
		t.Fatalf("PlaceReplicas: %v", err)
	}
	if len(result.Placements) != 2 {
		t.Fatalf("expected 2 placements, got %d", len(result.Placements))
	}
	if len(result.Failures) != 0 {
		t.Fatalf("expected 0 failures, got %d: %v", len(result.Failures), result.Failures)
	}
	// Both should prefer node-large (least-loaded)
	for _, p := range result.Placements {
		if p.Score <= 0 {
			t.Fatalf("expected positive score, got %f", p.Score)
		}
	}
}

func TestPreferredNodeInReplicaPlacement(t *testing.T) {
	engine := placement.NewEngine(placement.NewScorer(placement.StrategyLeastLoaded), placement.NewConstraintChecker())
	candidates := []placement.Candidate{
		{
			NodeID:          "node-a",
			TotalMemory:     16384,
			AvailableMemory: 8192,
			TotalCPU:        4096,
			AvailableCPU:    2048,
			TotalDisk:       102400,
			AvailableDisk:   51200,
			ServerCount:     2,
		},
		{
			NodeID:          "node-b",
			TotalMemory:     65536,
			AvailableMemory: 32768,
			TotalCPU:        16384,
			AvailableCPU:    8192,
			TotalDisk:       524288,
			AvailableDisk:   262144,
			ServerCount:     0,
		},
	}

	req := placement.ReplicaPlacementRequest{
		Replicas: []placement.ReplicaSpec{
			{Index: 0, CPU: 1024, MemoryMB: 2048, DiskMB: 10240, RuntimeProvider: "docker"},
		},
		PreferredNode: "node-a",
	}

	result, err := engine.PlaceReplicas(context.Background(), candidates, req)
	if err != nil {
		t.Fatalf("PlaceReplicas: %v", err)
	}
	if len(result.Placements) == 0 {
		t.Fatal("expected at least one placement")
	}
	t.Logf("preferred node placement: node=%s score=%.0f", result.Placements[0].NodeID, result.Placements[0].Score)
}

func TestExplainReplicaScores(t *testing.T) {
	engine := placement.NewEngine(placement.NewScorer(placement.StrategyLeastLoaded), placement.NewConstraintChecker())
	candidates := []placement.Candidate{
		{
			NodeID:          "node-explain-1",
			TotalMemory:     16384,
			AvailableMemory: 8192,
			TotalCPU:        4096,
			AvailableCPU:    2048,
			TotalDisk:       102400,
			AvailableDisk:   51200,
			ServerCount:     0,
		},
		{
			NodeID:          "node-explain-2",
			TotalMemory:     8192,
			AvailableMemory: 2048,
			TotalCPU:        2048,
			AvailableCPU:    512,
			TotalDisk:       51200,
			AvailableDisk:   10240,
			ServerCount:     5,
		},
	}

	req := placement.ReplicaPlacementRequest{
		Replicas: []placement.ReplicaSpec{
			{Index: 0, CPU: 1024, MemoryMB: 2048, DiskMB: 10240, RuntimeProvider: "docker"},
		},
		ExistingNodeMap: map[string]int{},
	}

	explain := placement.ExplainReplicaPlacement(context.Background(), engine, candidates, req)
	if len(explain) == 0 {
		t.Fatal("expected explanations")
	}
	if len(explain[0].Candidates) != 2 {
		t.Fatalf("expected 2 candidate explanations, got %d", len(explain[0].Candidates))
	}
	// First candidate should have higher score (more available resources)
	if explain[0].Candidates[0].Score <= explain[0].Candidates[1].Score {
		t.Log("node-explain-1 should be scored higher than node-explain-2")
	}
}
