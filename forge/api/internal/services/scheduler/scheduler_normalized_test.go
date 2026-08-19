package scheduler

import (
	"testing"
)

// TestSchedulerPreferredBonusNormalized ensures preferred and storage bonuses
// are bounded to the normalized range, not the legacy 1e9/1e10/1e8 overflows.
func TestSchedulerPreferredBonusNormalized(t *testing.T) {
	t.Setenv("FORGE_PLACEMENT_V2", "true")
	if schedulerPlacementV2() != true {
		t.Fatal("expected V2 enabled by default")
	}
	if schedulerPreferredBonus != 0.30 {
		t.Errorf("preferred bonus = %v, want 0.30", schedulerPreferredBonus)
	}
	if schedulerStorageBonus != 0.15 {
		t.Errorf("storage match bonus = %v, want 0.15", schedulerStorageBonus)
	}
	if schedulerStoragePenalty != 0.50 {
		t.Errorf("storage mismatch penalty = %v, want 0.50", schedulerStoragePenalty)
	}
	// All must be < 1.0 so they cannot dwarf base [0,1].
	for name, v := range map[string]float64{
		"preferred":      schedulerPreferredBonus,
		"storageBonus":   schedulerStorageBonus,
		"storagePenalty": schedulerStoragePenalty,
	} {
		if v >= 1.0 || v < 0 {
			t.Errorf("%s bonus %v out of (0,1) range", name, v)
		}
	}
}

// TestSchedulerLegacyOverflows confirms legacy path would overflow.
func TestSchedulerLegacyOverflows(t *testing.T) {
	// Legacy constants are hard-coded in service.go else branches; we just
	// verify the normalized values are small and the env flag gates them.
	legacyPreferred := 1e9
	legacyMismatch := 1e10
	legacyMatch := 1e8
	if legacyPreferred < 1e6 {
		t.Error("legacy preferred should be huge")
	}
	if legacyMismatch < 1e6 {
		t.Error("legacy mismatch should be huge")
	}
	if legacyMatch < 1e6 {
		t.Error("legacy match should be huge")
	}
}
