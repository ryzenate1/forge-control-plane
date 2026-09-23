package failover

import "testing"

// NOTE: the locality-vocabulary shim (canonicalLocality / isLocalOnly) was
// deleted, so DetermineFailoverAction now compares the raw locality string:
// only the exact values "local_only", "shared" and "replicated" are
// recognised, and anything else — including the old "local" alias and any
// upper-case variant — falls through to the default branch. The truth table
// for the recognised values lives in service_test.go; this file pins the
// no-normalisation contract that replaced the alias helpers.

func TestLocalityIsMatchedExactly(t *testing.T) {
	// Unrecognised spellings no longer fold onto local_only: with a verified
	// backup they take the backup branch instead of the notify branch.
	for _, loc := range []string{"local", "LOCAL", "Local_Only", "local_only ", "LOCAL_ONLY"} {
		if got := DetermineFailoverAction(loc, true, false); got != FailoverActionEvacuate {
			t.Errorf("DetermineFailoverAction(%q, verifiedBackup=true) = %s, want evacuate (alias is not canonicalised)", loc, got)
		}
	}
	// The canonical spelling is required to reach the local-only branch.
	if got := DetermineFailoverAction("local_only", true, false); got != FailoverActionEvacuate {
		t.Errorf("verified backup still wins for local_only, got %s", got)
	}
	if got := DetermineFailoverAction("local_only", false, false); got != FailoverActionNotify {
		t.Errorf("DetermineFailoverAction(local_only) = %s, want notify", got)
	}
	if got := DetermineFailoverAction("LOCAL_ONLY", false, false); got != FailoverActionNotify {
		t.Errorf("unknown spellings still default to notify, got %s", got)
	}
}

func TestReplicationOnlyExcludesExactLocalOnly(t *testing.T) {
	// isReplicated evacuates unless locality is exactly "local_only".
	if got := DetermineFailoverAction("local_only", false, true); got != FailoverActionNotify {
		t.Errorf("local_only + replicated = %s, want notify", got)
	}
	for _, loc := range []string{"local", "LOCAL_ONLY", "shared", "replicated", ""} {
		if got := DetermineFailoverAction(loc, false, true); got != FailoverActionEvacuate {
			t.Errorf("replicated + %q = %s, want evacuate", loc, got)
		}
	}
}

func TestActionConstantsAreStable(t *testing.T) {
	if FailoverActionEvacuate != "evacuate" || FailoverActionRestart != "restart" || FailoverActionNotify != "notify" {
		t.Error("failover action wire values changed")
	}
}
