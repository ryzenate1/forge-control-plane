package store

import "time"

// RetentionEngine implements the unified OR retention logic (BK-06).
// Keep if any active rule keeps (union), with safety rail handled by caller.
// Converges previous divergent engines in store_backups, artifact, and beacon.
//
// Rules:
//   - MaxBackups: keep newest N (0 = disabled)
//   - RetentionDays: keep backups younger than N days (0 = disabled)
//   - KeepDaily/Weekly/Monthly: keep up to N per period (for beacon)
//
// For forge store, only MaxBackups and RetentionDays are used.
type RetentionEngine struct {
	MaxBackups    int
	RetentionDays int
	KeepDaily     int
	KeepWeekly    int
	KeepMonthly   int
	MaxAge        time.Duration
}

// ShouldKeep reports whether a backup with given age and rank should be kept
// under OR semantics. rank is 0 for newest.
func (r RetentionEngine) ShouldKeep(age time.Duration, rank int, period string) bool {
	if r.MaxBackups > 0 && rank < r.MaxBackups {
		return true
	}
	if r.RetentionDays > 0 && age <= time.Duration(r.RetentionDays)*24*time.Hour {
		return true
	}
	if r.MaxAge > 0 && age < r.MaxAge {
		return true
	}
	// KeepDaily/Weekly/Monthly handled via period counting externally;
	// period argument indicates which bucket this backup falls into and whether
	// its count is within limit. If period is non-empty and within limit, keep.
	if period != "" {
		return true
	}
	return false
}

// HasActive reports whether any retention rule is active (non-zero).
func (r RetentionEngine) HasActive() bool {
	return r.MaxBackups > 0 || r.RetentionDays > 0 || r.MaxAge > 0 || r.KeepDaily > 0 || r.KeepWeekly > 0 || r.KeepMonthly > 0
}
