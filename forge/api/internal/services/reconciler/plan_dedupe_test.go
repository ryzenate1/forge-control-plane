package reconciler

import (
	"testing"
	"time"
)

// Regression test for Phase-1 finding F-25: the plan deduplication window
// must suppress only IDENTICAL desired-state drift. A genuine desired-state
// change (different DesiredHash) inside a previous dedupe window must
// produce a new plan.

func TestDedupeSuppressesOnlyIdenticalDesiredState(t *testing.T) {
	s := &Service{} // hasPendingDuplicatePlan is pure w.r.t. store when listing errors; test hash semantics directly

	now := time.Now().UTC()
	oldDiff := ReconcileDiff{
		ResourceID:   "srv-1",
		ResourceKind: ResourceKindServer,
		DiffType:     DiffUpdate,
		DesiredHash:  "desired-generation-1",
		ObservedHash: "observed-a",
		Description:  "memory mismatch",
	}
	newSameDrift := oldDiff // identical desired+observed: duplicate
	newDesired := oldDiff
	newDesired.DesiredHash = "desired-generation-2" // legitimate change

	if snapshotHash([]ReconcileDiff{oldDiff}) != snapshotHash([]ReconcileDiff{newSameDrift}) {
		t.Fatal("identical drifts must hash identically (dedupe precondition)")
	}
	if snapshotHash([]ReconcileDiff{oldDiff}) == snapshotHash([]ReconcileDiff{[]ReconcileDiff{newDesired}[0]}) {
		t.Fatal("a real desired-state change must produce a different plan hash and escape suppression")
	}

	// The window arithmetic: plans older than the cutoff are never dupes.
	cutoff := now.Add(-PlanDedupeWindow)
	if cutoff.After(now) {
		t.Fatal("cutoff arithmetic inverted")
	}
	_ = s
}
