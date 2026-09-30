package placement

import (
	"context"
	"strings"
	"testing"
)

func TestCheckAntiAffinityUnknownServerErrors(t *testing.T) {
	checker := NewConstraintChecker()
	candidate := Candidate{NodeID: "node-1"}
	err := checker.CheckHard(candidate, []Constraint{
		{Type: ConstraintAntiAffinity, Values: []string{"ghost-server"}, Required: true},
	}, ConstraintContext{ServerNodeMap: map[string]string{}})
	if err == nil {
		t.Fatal("anti-affinity against an unknown server must error, not silently pass")
	}
}

func TestCheckAffinityUnknownServerErrors(t *testing.T) {
	checker := NewConstraintChecker()
	candidate := Candidate{NodeID: "node-1"}
	err := checker.CheckHard(candidate, []Constraint{
		{Type: ConstraintAffinity, Values: []string{"ghost-server"}, Required: true},
	}, ConstraintContext{ServerNodeMap: map[string]string{}})
	if err == nil {
		t.Fatal("affinity against an unknown server must error")
	}
}

func TestPlaceReplicasRequiredNodeUsesScoringPath(t *testing.T) {
	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	candidates := []Candidate{
		{NodeID: "node-1", TotalMemory: 16384, AvailableMemory: 8192, TotalCPU: 8192, AvailableCPU: 4096, TotalDisk: 102400, AvailableDisk: 51200, ServerCount: 4},
		{NodeID: "node-2", TotalMemory: 16384, AvailableMemory: 8192, TotalCPU: 4096, AvailableCPU: 2048, TotalDisk: 102400, AvailableDisk: 51200, ServerCount: 0},
	}
	req := ReplicaPlacementRequest{
		Replicas:        []ReplicaSpec{{Index: 0, CPU: 1024, MemoryMB: 2048, DiskMB: 10240, RuntimeProvider: "docker"}},
		RequiredNode:    "node-1",
		ExistingNodeMap: map[string]int{"node-1": 2},
	}
	result, err := engine.PlaceReplicas(context.Background(), candidates, req)
	if err != nil {
		t.Fatalf("PlaceReplicas: %v", err)
	}
	if len(result.Placements) != 1 || result.Placements[0].NodeID != "node-1" {
		t.Fatalf("required node must still win, got %+v", result.Placements)
	}
	found := false
	for _, reason := range result.Placements[0].Reasons {
		if strings.Contains(reason, "anti-affinity spread penalty") {
			found = true
		}
	}
	if !found {
		t.Fatalf("required node must carry the spread penalty like any candidate, reasons: %v", result.Placements[0].Reasons)
	}
}

func TestPlaceReplicasExistingUsageSums(t *testing.T) {
	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	candidates := []Candidate{
		{NodeID: "node-1", TotalMemory: 16384, AvailableMemory: 12288, TotalCPU: 4096, AvailableCPU: 3072, TotalDisk: 102400, AvailableDisk: 92160, ServerCount: 1},
	}
	// One existing instance consuming 4GB; the new replica needs 2GB. Exact
	// sums leave 12288-4096=8192 available, which fits.
	req := ReplicaPlacementRequest{
		Replicas:        []ReplicaSpec{{Index: 0, CPU: 1024, MemoryMB: 2048, DiskMB: 10240, RuntimeProvider: "docker"}},
		ExistingNodeMap: map[string]int{"node-1": 1},
		ExistingUsage:   map[string]ResourceUsage{"node-1": {CPU: 1024, MemoryMB: 4096, DiskMB: 10240}},
	}
	result, err := engine.PlaceReplicas(context.Background(), candidates, req)
	if err != nil {
		t.Fatalf("PlaceReplicas: %v", err)
	}
	if len(result.Placements) != 1 {
		t.Fatalf("expected 1 placement with exact usage accounting, failures: %v", result.Failures)
	}
}

func TestExplainPlacementCollectsScoreFailures(t *testing.T) {
	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	candidates := []Candidate{
		{NodeID: "node-full", TotalMemory: 2048, AvailableMemory: 0, TotalCPU: 1024, AvailableCPU: 0, TotalDisk: 10240, AvailableDisk: 0},
	}
	report, err := ExplainPlacement(context.Background(), engine, candidates, WorkloadRequest{CPU: 1024, MemoryMB: 2048, DiskMB: 10240})
	if err != nil {
		t.Fatalf("ExplainPlacement: %v", err)
	}
	if len(report.ScoredCandidates) != 0 {
		t.Fatalf("expected no scored candidates, got %d", len(report.ScoredCandidates))
	}
	if len(report.ScoreFailures) != 1 || report.ScoreFailures[0].NodeID != "node-full" {
		t.Fatalf("expected the unscorable node in ScoreFailures, got %+v", report.ScoreFailures)
	}
}
