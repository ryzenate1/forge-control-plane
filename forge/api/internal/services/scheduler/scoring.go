package scheduler

import (
	"os"
	"strings"
)

// Normalized scoring terms for the scheduler layer. The engine already returns a
// base score in [0,1], so a term here that is larger than 1 would stop being a
// preference and become a directive: the legacy values below did exactly that,
// which is why an operator's "preferred" node always won regardless of how
// unsuitable it was, and why a storage-locality mismatch could bury every other
// signal.
const (
	// schedulerPreferredBonus rewards the request's preferred node. Preferred
	// means preferred: it beats comparable candidates but does not override a
	// node that is materially worse. RequiredNode is the directive, and it is
	// enforced by filtering instead.
	schedulerPreferredBonus = 0.30
	// schedulerStorageBonus is added when a node offers the requested storage
	// locality, and schedulerStoragePenalty is subtracted when it cannot. The
	// penalty is larger than the bonus because a locality the node cannot
	// provide is a functional mismatch, not a nicety.
	schedulerStorageBonus   = 0.15
	schedulerStoragePenalty = 0.50

	legacyPreferredNodeBonus     = 1e9
	legacyStorageLocalityBonus   = 1e8
	legacyStorageLocalityPenalty = 1e10
)

// schedulerPlacementV2 mirrors placement's gate so the scheduler layer and the
// engine layer cannot disagree about which maths produced a decision. Read per
// call for the same reason placement reads it per call.
func schedulerPlacementV2() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("FORGE_PLACEMENT_V2")))
	if value == "" {
		return true
	}
	return value != "false" && value != "0" && value != "no"
}

func preferredNodeBonus() float64 {
	if schedulerPlacementV2() {
		return schedulerPreferredBonus
	}
	return legacyPreferredNodeBonus
}

func storageLocalityBonus() float64 {
	if schedulerPlacementV2() {
		return schedulerStorageBonus
	}
	return legacyStorageLocalityBonus
}

func storageLocalityPenalty() float64 {
	if schedulerPlacementV2() {
		return schedulerStoragePenalty
	}
	return legacyStorageLocalityPenalty
}

// canonicalStorageLocality collapses the spellings the rest of the system uses
// for the same storage behaviour. "local_only" and "local" are the same
// requirement; comparing them as strings made every local-only workload look
// incompatible with every local node, and the mismatch carried the largest
// penalty in the score.
func canonicalStorageLocality(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return ""
	case "local", "local_only", "local-only", "localonly":
		return "local"
	case "shared", "shared_storage", "shared-storage":
		return "shared"
	case "replicated":
		return "replicated"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

// storageLocalityEqual compares two locality expressions by what they mean.
func storageLocalityEqual(left, right string) bool {
	return canonicalStorageLocality(left) == canonicalStorageLocality(right)
}

// isLocalStorageLocality reports whether an expression asks for storage that
// lives with the workload.
func isLocalStorageLocality(value string) bool {
	return canonicalStorageLocality(value) == "local"
}

// storageLocalityForProvider names the storage a runtime provider gives a
// workload, in the same vocabulary a request uses, so the two can be compared
// without either side guessing the other's spelling.
func storageLocalityForProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "nfs", "shared":
		return "shared"
	default:
		return "local"
	}
}
