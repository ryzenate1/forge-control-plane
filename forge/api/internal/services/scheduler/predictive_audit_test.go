package scheduler

import (
	"context"
	"testing"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/store"
)

type stubPredictiveStore struct {
	snapshot store.NodeCapacitySnapshot
}

func (s *stubPredictiveStore) NodeCapacitySnapshot(ctx context.Context, nodeID string) (store.NodeCapacitySnapshot, error) {
	return s.snapshot, nil
}

func (s *stubPredictiveStore) ListNodes(ctx context.Context) ([]store.Node, error) {
	return []store.Node{{ID: s.snapshot.NodeID}}, nil
}

func (s *stubPredictiveStore) ListServersByNode(ctx context.Context, nodeID string) ([]store.Server, error) {
	return nil, nil
}

func (s *stubPredictiveStore) ListAffinityRulesDB(ctx context.Context) ([]store.AffinityRuleRow, error) {
	return nil, nil
}

func (s *stubPredictiveStore) UpsertAffinityRuleDB(ctx context.Context, r store.AffinityRuleRow) (store.AffinityRuleRow, error) {
	return r, nil
}

func (s *stubPredictiveStore) DeleteAffinityRuleDB(ctx context.Context, id string) error { return nil }

func (s *stubPredictiveStore) ListAntiAffinityRulesDB(ctx context.Context) ([]store.AntiAffinityRuleRow, error) {
	return nil, nil
}

func (s *stubPredictiveStore) UpsertAntiAffinityRuleDB(ctx context.Context, r store.AntiAffinityRuleRow) (store.AntiAffinityRuleRow, error) {
	return r, nil
}

func (s *stubPredictiveStore) DeleteAntiAffinityRuleDB(ctx context.Context, id string) error {
	return nil
}

func TestPredictiveTotalScoreBounded(t *testing.T) {
	cases := []struct {
		name     string
		snapshot store.NodeCapacitySnapshot
	}{
		{"healthy node", store.NodeCapacitySnapshot{NodeID: "n-1", TotalCPU: 8000, AvailableCPU: 6000, TotalMemory: 16384, AvailableMemory: 8192, TotalDisk: 200000, AvailableDisk: 100000}},
		{"full node", store.NodeCapacitySnapshot{NodeID: "n-2", TotalCPU: 8000, AvailableCPU: 0, TotalMemory: 16384, AvailableMemory: 0, TotalDisk: 200000, AvailableDisk: 0}},
		{"unknown totals", store.NodeCapacitySnapshot{NodeID: "n-3", AvailableCPU: 6000, AvailableMemory: 8192, AvailableDisk: 100000}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scorer := NewPredictiveScorer(&stubPredictiveStore{snapshot: tc.snapshot})
			score, err := scorer.ScorePredictive(context.Background(), tc.snapshot.NodeID, domain.PlacementRequest{})
			if err != nil {
				t.Fatalf("ScorePredictive: %v", err)
			}
			if score.TotalScore < 0 || score.TotalScore > 1 {
				t.Fatalf("TotalScore = %f, want it normalized to [0,1]", score.TotalScore)
			}
		})
	}
}

func TestPredictiveTotalScoreBoundedWithHeavyRules(t *testing.T) {
	scorer := NewPredictiveScorer(&stubPredictiveStore{snapshot: store.NodeCapacitySnapshot{
		NodeID: "n-1", TotalCPU: 8000, AvailableCPU: 6000,
		TotalMemory: 16384, AvailableMemory: 8192, TotalDisk: 200000, AvailableDisk: 100000,
	}})
	ctx := context.Background()
	if err := scorer.AddAffinityRule(ctx, AffinityRule{ID: "a-1", Weight: 1e9}); err != nil {
		t.Fatal(err)
	}
	if err := scorer.AddAntiAffinityRule(ctx, AntiAffinityRule{ID: "b-1", Weight: 1e9}); err != nil {
		t.Fatal(err)
	}
	score, err := scorer.ScorePredictive(ctx, "n-1", domain.PlacementRequest{})
	if err != nil {
		t.Fatalf("ScorePredictive: %v", err)
	}
	if score.TotalScore < 0 || score.TotalScore > 1 {
		t.Fatalf("TotalScore = %f under heavy rules, want [0,1]", score.TotalScore)
	}
}
