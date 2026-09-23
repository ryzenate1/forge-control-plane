package placement

import (
	"os"
	"strings"
)

// Soft-constraint terms are bounded so that satisfying a preference can never
// outweigh the base score of a candidate by orders of magnitude. The legacy
// terms are kept behind FORGE_PLACEMENT_V2=false so a decision made before
// normalization can still be reproduced while investigating an incident.
const (
	// kSoftWeight is the full bonus when every soft constraint is satisfied, and
	// kSoftPenalty the full deduction when none is. Partial satisfaction is
	// interpolated between the two, so the result always lands inside
	// [-kSoftPenalty, kSoftWeight].
	kSoftWeight  = 0.30
	kSoftPenalty = 0.10

	// legacySoftWeight and legacySoftPenalty are the pre-normalization terms.
	// They are only reachable with FORGE_PLACEMENT_V2=false, kept so a decision
	// made before normalization can still be reproduced while investigating it.
	legacySoftWeight  = 1e12
	legacySoftPenalty = 1e10
)

// placementV2 reports whether normalized scoring is active. It is read per call
// rather than resolved once at start-up so that the maths can be compared inside
// a single process, and so an operator can flip it during a rollout without a
// restart. An unset variable means normalized, because that is the intended
// behaviour; only an explicit negative turns the legacy maths back on.
func placementV2() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("FORGE_PLACEMENT_V2")))
	if value == "" {
		return true
	}
	return value != "false" && value != "0" && value != "no"
}

// clampUnit constrains a derived score to the [0,1] band the normalized terms
// assume. Scores outside it would make a bounded bonus meaningless, since the
// bonus is sized relative to a base of at most 1.
func clampUnit(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
