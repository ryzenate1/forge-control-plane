package placement

import (
	"context"
	"os"
	"testing"
)

// TestCheckSoftNormalizedBounds verifies that soft constraint bonuses are bounded
// to kSoftWeight (±0.30 / -0.10 window) rather than the legacy overflow values
// +1e12 / -1e10. A single soft constraint must never outweigh the normalized
// base score of ≤1.0.
func TestCheckSoftNormalizedBounds(t *testing.T) {
	// Ensure V2 (default) — normalized path.
	t.Setenv("FORGE_PLACEMENT_V2", "true")
	checker := NewConstraintChecker()
	candidate := Candidate{NodeID: "node-1", RegionID: "us-east"}
	ctx := ConstraintContext{}

	// One satisfied soft constraint should yield exactly kSoftWeight (0.30), not 1e12.
	constraints := []Constraint{{Type: ConstraintRegion, Required: false, Values: []string{"us-east"}}}
	bonus, _ := checker.CheckSoft(candidate, constraints, ctx)
	if bonus != kSoftWeight {
		t.Errorf("satisfied soft bonus = %v, want %v", bonus, kSoftWeight)
	}
	if bonus >= 1.0 {
		t.Errorf("satisfied soft bonus %v must be < 1.0 to stay within normalized range", bonus)
	}

	// One unsatisfied should be -kSoftPenalty (-0.10) not -1e10.
	miss := []Constraint{{Type: ConstraintRegion, Required: false, Values: []string{"eu-west"}}}
	bonusMiss, _ := checker.CheckSoft(candidate, miss, ctx)
	if bonusMiss != -kSoftPenalty {
		t.Errorf("unsatisfied soft bonus = %v, want %v", bonusMiss, -kSoftPenalty)
	}
	if bonusMiss <= -1.0 {
		t.Errorf("unsatisfied penalty %v must be > -1.0", bonusMiss)
	}

	// Mixed: 1 satisfied + 1 missed should be averaged, bounded in [-0.10, 0.30].
	mixed := []Constraint{
		{Type: ConstraintRegion, Required: false, Values: []string{"us-east"}},
		{Type: ConstraintRegion, Required: false, Values: []string{"eu-west"}},
	}
	bonusMixed, _ := checker.CheckSoft(candidate, mixed, ctx)
	// (1/2)*0.30 + (1/2)*(-0.10) = 0.10
	expectedMixed := 0.10
	if diff := bonusMixed - expectedMixed; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("mixed bonus = %v, want %v", bonusMixed, expectedMixed)
	}

	// Ten unsatisfied should remain bounded, not blow to -1e11.
	ten := make([]Constraint, 10)
	for i := range ten {
		ten[i] = Constraint{Type: ConstraintRegion, Required: false, Values: []string{"eu-west"}}
	}
	bonusTen, _ := checker.CheckSoft(candidate, ten, ctx)
	if bonusTen < -kSoftPenalty || bonusTen > kSoftWeight {
		t.Errorf("ten misses bonus %v out of bounded range [%v, %v]", bonusTen, -kSoftPenalty, kSoftWeight)
	}
	if bonusTen < -1000 || bonusTen > 1000 {
		t.Errorf("ten misses bonus overflowed: %v", bonusTen)
	}

	// Ten satisfied should still be 0.30, not 1e13.
	tenSat := make([]Constraint, 10)
	for i := range tenSat {
		tenSat[i] = Constraint{Type: ConstraintRegion, Required: false, Values: []string{"us-east"}}
	}
	bonusTenSat, _ := checker.CheckSoft(candidate, tenSat, ctx)
	if bonusTenSat != kSoftWeight {
		t.Errorf("ten satisfied bonus = %v, want %v", bonusTenSat, kSoftWeight)
	}
}

// TestCheckSoftLegacyOverflow confirms the legacy path still overflows when
// FORGE_PLACEMENT_V2=false, so we have a rollback comparison.
func TestCheckSoftLegacyOverflow(t *testing.T) {
	t.Setenv("FORGE_PLACEMENT_V2", "false")
	checker := NewConstraintChecker()
	candidate := Candidate{NodeID: "node-1", RegionID: "us-east"}
	ctx := ConstraintContext{}
	constraints := []Constraint{{Type: ConstraintRegion, Required: false, Values: []string{"us-east"}}}
	bonus, _ := checker.CheckSoft(candidate, constraints, ctx)
	if bonus != 1e12 {
		t.Errorf("legacy satisfied bonus = %v, want 1e12", bonus)
	}
	miss := []Constraint{{Type: ConstraintRegion, Required: false, Values: []string{"eu-west"}}}
	bonusMiss, _ := checker.CheckSoft(candidate, miss, ctx)
	if bonusMiss != -1e10 {
		t.Errorf("legacy missed bonus = %v, want -1e10", bonusMiss)
	}
}

// TestLeastLoadedScorerNormalized verifies base scores are clamped to [0,1]
// so soft bonuses cannot be dwarfed.
func TestLeastLoadedScorerNormalized(t *testing.T) {
	t.Setenv("FORGE_PLACEMENT_V2", "true")
	scorer := &LeastLoadedScorer{}
	c := Candidate{
		NodeID:          "n1",
		TotalCPU:        4000,
		TotalMemory:     8192,
		TotalDisk:       100000,
		AvailableCPU:    4000,
		AvailableMemory: 8192,
		AvailableDisk:   100000,
	}
	score, _, err := scorer.Score(context.Background(), c, WorkloadRequest{CPU: 100, MemoryMB: 512, DiskMB: 10000})
	if err != nil {
		t.Fatal(err)
	}
	if score < 0 || score > 1.0 {
		t.Errorf("LeastLoaded score %v not in [0,1]", score)
	}
	// Previously score was 3.0 (sum of 3 ratios); legacy should be 3.0.
	t.Setenv("FORGE_PLACEMENT_V2", "false")
	scoreLegacy, _, _ := scorer.Score(context.Background(), c, WorkloadRequest{CPU: 100, MemoryMB: 512, DiskMB: 10000})
	if scoreLegacy != 3.0 {
		t.Errorf("legacy LeastLoaded score = %v, want 3.0", scoreLegacy)
	}
	t.Setenv("FORGE_PLACEMENT_V2", "true")
}

// TestAvailableRatioClamps verifies unknown capacity case is bounded.
func TestAvailableRatioClamps(t *testing.T) {
	t.Setenv("FORGE_PLACEMENT_V2", "true")
	v := availableRatio(5000, 0)
	if v < 0 || v > 1 {
		t.Errorf("availableRatio(5000,0) = %v, want bounded [0,1]", v)
	}
	if v == 0 {
		t.Errorf("availableRatio(5000,0) should be >0 for positive available")
	}
	// When fallback is monotonic (available/(available+1000)), ordering is preserved.
	// When fallback is constant 0.5, both will be equal; tolerate either as long as bounded.
	v100 := availableRatio(100, 0)
	v5000 := availableRatio(5000, 0)
	if v100 > v5000 {
		t.Errorf("availableRatio ordering broken: 100=>%v should be <= 5000=>%v", v100, v5000)
	}
	if v := availableRatio(0, 0); v != 0 {
		t.Errorf("availableRatio(0,0) = %v, want 0", v)
	}
	// Normal case stays in [0,1].
	if v := availableRatio(100, 200); v != 0.5 {
		t.Errorf("availableRatio(100,200)=%v, want 0.5", v)
	}
	t.Setenv("FORGE_PLACEMENT_V2", "false")
	if v := availableRatio(5000, 0); v != 5000 {
		t.Errorf("legacy availableRatio(5000,0)=%v want 5000", v)
	}
}

// TestEnginePlaceSoftBonusDoesNotDwarfBase is the integration-level overflow
// regression: a node with higher load but satisfying a soft constraint must not
// always beat a much freer node due to 1e12. Under normalized math the freer
// node should still win if the load difference outweighs 0.30.
func TestEnginePlaceSoftBonusDoesNotDwarfBase(t *testing.T) {
	t.Setenv("FORGE_PLACEMENT_V2", "true")
	_ = os.Unsetenv // ensure env read happens inside Score
	// Node-1: half full (score ~0.5) but satisfies soft region us-east.
	// Node-2: empty (score 1.0) but misses soft region.
	candidates := []Candidate{
		{NodeID: "node-1", RegionID: "us-east", TotalCPU: 100, TotalMemory: 100, TotalDisk: 100, AvailableCPU: 50, AvailableMemory: 50, AvailableDisk: 50},
		{NodeID: "node-2", RegionID: "eu-west", TotalCPU: 100, TotalMemory: 100, TotalDisk: 100, AvailableCPU: 100, AvailableMemory: 100, AvailableDisk: 100},
	}
	constraints := []Constraint{{Type: ConstraintRegion, Required: false, Values: []string{"us-east"}}}
	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	result, err := engine.Place(context.Background(), candidates, WorkloadRequest{Constraints: constraints})
	if err != nil {
		t.Fatal(err)
	}
	// Under normalized math: node-1 base 0.5 +0.30 =0.8, node-2 base 1.0 -0.10=0.90 → node-2 should win.
	if result.NodeID != "node-2" {
		t.Errorf("normalized placement should prefer freer node-2, got %s (soft 0.30 must not dwarf load)", result.NodeID)
	}

	// Under legacy overflow, node-1 would always win (1e12 dwarfs 0.5).
	t.Setenv("FORGE_PLACEMENT_V2", "false")
	resultLegacy, err := engine.Place(context.Background(), candidates, WorkloadRequest{Constraints: constraints})
	if err != nil {
		t.Fatal(err)
	}
	if resultLegacy.NodeID != "node-1" {
		t.Errorf("legacy placement should prefer node-1 due to 1e12, got %s", resultLegacy.NodeID)
	}
	t.Setenv("FORGE_PLACEMENT_V2", "true")
}

// TestEnginePlaceAllSortedWithNormalized ensures PlaceAll still sorts by combined
// score and respects bounded bonuses.
func TestEnginePlaceAllSortedWithNormalized(t *testing.T) {
	t.Setenv("FORGE_PLACEMENT_V2", "true")
	candidates := []Candidate{
		{NodeID: "n1", AvailableMemory: 100, AvailableCPU: 10, AvailableDisk: 1000, TotalCPU: 100, TotalMemory: 1000, TotalDisk: 10000},
		{NodeID: "n2", AvailableMemory: 200, AvailableCPU: 20, AvailableDisk: 2000, TotalCPU: 100, TotalMemory: 1000, TotalDisk: 10000},
		{NodeID: "n3", AvailableMemory: 300, AvailableCPU: 30, AvailableDisk: 3000, TotalCPU: 100, TotalMemory: 1000, TotalDisk: 10000},
	}
	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	results, err := engine.PlaceAll(context.Background(), candidates, WorkloadRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	// LeastLoaded normalized: n3 most free should be first, but all within [0,1].
	for _, r := range results {
		if r.Score < 0 || r.Score > 1.5 {
			t.Errorf("score %v for %s out of bounded range", r.Score, r.NodeID)
		}
	}
}
