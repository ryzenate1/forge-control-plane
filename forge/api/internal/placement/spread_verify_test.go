package placement

import (
	"context"
	"testing"
)

// NOTE: the declarative SpreadConfig/SpreadTarget machinery (attribute+weight+
// percent targets, desiredCountsForSpread, NewSpreadScorer(cfg), and the
// Spread/SpreadCounts/SpreadTotal fields on WorkloadRequest) was removed by the
// placement refactor. Spreading now emerges from two simpler mechanisms that
// these tests pin:
//   1. PlaceReplicas decrements working-candidate capacity and applies an
//      anti-affinity penalty (0.1 per existing instance) per placement, so
//      replicas fan out across identical nodes without any Spread config.
//   2. SpreadScorer is a plain 1/(1+ServerCount) inverse-weight scorer.
// The removed region-percent targeting (SpreadTarget/implicit "*" bucket) has
// no replacement and its tests were dropped.

func TestSpreadEvenAcrossNodes(t *testing.T) {
	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	candidates := []Candidate{
		{NodeID: "node-1", RegionID: "us-east", TotalMemory: 16384, TotalCPU: 8000, TotalDisk: 200000, AvailableMemory: 16384, AvailableCPU: 8000, AvailableDisk: 200000, ServerCount: 0},
		{NodeID: "node-2", RegionID: "us-east", TotalMemory: 16384, TotalCPU: 8000, TotalDisk: 200000, AvailableMemory: 16384, AvailableCPU: 8000, AvailableDisk: 200000, ServerCount: 0},
		{NodeID: "node-3", RegionID: "us-east", TotalMemory: 16384, TotalCPU: 8000, TotalDisk: 200000, AvailableMemory: 16384, AvailableCPU: 8000, AvailableDisk: 200000, ServerCount: 0},
	}
	replicas := make([]ReplicaSpec, 6)
	for i := 0; i < 6; i++ {
		replicas[i] = ReplicaSpec{Index: i, CPU: 100, MemoryMB: 512, DiskMB: 1000}
	}
	req := ReplicaPlacementRequest{
		AppID:    "app-1",
		Replicas: replicas,
	}
	result, err := engine.PlaceReplicas(context.Background(), candidates, req)
	if err != nil {
		t.Fatalf("even spread err: %v", err)
	}
	if len(result.Placements) != 6 {
		t.Fatalf("expected 6 placements, got %d failures %d", len(result.Placements), len(result.Failures))
	}
	counts := map[string]int{}
	for _, p := range result.Placements {
		counts[p.NodeID]++
		t.Logf("idx %d -> %s score %.3f %v", p.Index, p.NodeID, p.Score, p.Reasons)
	}
	t.Logf("counts %v", counts)
	if len(counts) != 3 {
		t.Errorf("expected replicas to fan out over all 3 nodes, counts %v", counts)
	}
	max, min := 0, 100
	for _, c := range counts {
		if c > max {
			max = c
		}
		if c < min {
			min = c
		}
	}
	if max-min > 1 {
		t.Errorf("even spread uneven max %d min %d counts %v", max, min, counts)
	}
}

func TestSpreadFallback(t *testing.T) {
	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	candidates := []Candidate{
		{NodeID: "node-1", RegionID: "us-east", TotalMemory: 16384, TotalCPU: 8000, TotalDisk: 200000, AvailableMemory: 16384, AvailableCPU: 8000, AvailableDisk: 200000, ServerCount: 0},
		{NodeID: "node-2", RegionID: "us-east", TotalMemory: 16384, TotalCPU: 8000, TotalDisk: 200000, AvailableMemory: 16384, AvailableCPU: 8000, AvailableDisk: 200000, ServerCount: 0},
		{NodeID: "node-3", RegionID: "us-east", TotalMemory: 16384, TotalCPU: 8000, TotalDisk: 200000, AvailableMemory: 16384, AvailableCPU: 8000, AvailableDisk: 200000, ServerCount: 0},
	}
	replicas := make([]ReplicaSpec, 3)
	for i := 0; i < 3; i++ {
		replicas[i] = ReplicaSpec{Index: i, CPU: 100, MemoryMB: 512, DiskMB: 1000}
	}
	req := ReplicaPlacementRequest{AppID: "app-3", Replicas: replicas}
	result, err := engine.PlaceReplicas(context.Background(), candidates, req)
	if err != nil {
		t.Fatalf("fallback err %v", err)
	}
	if len(result.Placements) != 3 {
		t.Fatalf("expected 3 placements got %d", len(result.Placements))
	}
	for _, p := range result.Placements {
		t.Logf("%d -> %s score %.3f", p.Index, p.NodeID, p.Score)
	}
}

func TestSpreadScorerEven(t *testing.T) {
	// The config-free SpreadScorer is a pure inverse server-count weight:
	// the emptier node must always score higher.
	scorer := &SpreadScorer{}
	cands := []Candidate{
		{NodeID: "n1", RegionID: "dc1", ServerCount: 2, AvailableCPU: 8000, AvailableMemory: 16384, AvailableDisk: 200000, TotalCPU: 8000, TotalMemory: 16384, TotalDisk: 200000},
		{NodeID: "n2", RegionID: "dc2", ServerCount: 0, AvailableCPU: 8000, AvailableMemory: 16384, AvailableDisk: 200000, TotalCPU: 8000, TotalMemory: 16384, TotalDisk: 200000},
	}
	ctx := context.Background()
	work := WorkloadRequest{CPU: 100, MemoryMB: 512, DiskMB: 1000}
	s1, _, err := scorer.Score(ctx, cands[0], work)
	if err != nil {
		t.Fatalf("score n1: %v", err)
	}
	s2, _, err := scorer.Score(ctx, cands[1], work)
	if err != nil {
		t.Fatalf("score n2: %v", err)
	}
	t.Logf("dc1 (2 servers) score %.3f, dc2 (0 servers) score %.3f", s1, s2)
	if s2 <= s1 {
		t.Errorf("expected less-loaded dc2 (%.3f) higher than dc1 (%.3f)", s2, s1)
	}
	if want := 1.0 / 3.0; s1 != want {
		t.Errorf("dc1 score = %v, want 1/(1+2)=%v", s1, want)
	}
	if s2 != 1.0 {
		t.Errorf("dc2 score = %v, want 1.0", s2)
	}
}
