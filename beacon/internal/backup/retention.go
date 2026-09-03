package backup

import (
	"context"
	"fmt"
	"sort"
	"time"
)

type RetentionPolicy struct {
	MaxBackups  int
	MaxAge      time.Duration
	KeepDaily   int
	KeepWeekly  int
	KeepMonthly int
}

// Apply enforces the retention policy for the given server's backups,
// deleting any backups that do not satisfy the configured rules.
//
// Bounds checking: MaxBackups, KeepDaily, KeepWeekly, KeepMonthly, and MaxAge
// are all expected to be non-negative. A negative value is a misconfiguration
// (e.g. a bad user input or a bug in a caller) rather than a meaningful
// "unlimited"/"disabled" signal — that is expressed with a zero value — so
// Apply rejects negative values outright by returning an error instead of
// silently clamping them, which could otherwise be misread as "keep
// everything" or "keep nothing" depending on the rule.
//
// Safety rail: even with valid, non-negative settings, a policy such as
// MaxBackups=0 or a very small MaxAge combined with KeepDaily/KeepWeekly/
// KeepMonthly all set to 0 would otherwise mark every single backup for
// deletion. To guard against a misconfiguration wiping out all backups and
// leaving no recovery point, Apply always keeps the single most recent
// backup regardless of what the other rules computed.
func (p RetentionPolicy) Apply(ctx context.Context, store Store, serverID string) error {
	if p.MaxBackups < 0 {
		return fmt.Errorf("retention policy: MaxBackups must not be negative, got %d", p.MaxBackups)
	}
	if p.KeepDaily < 0 || p.KeepWeekly < 0 || p.KeepMonthly < 0 {
		return fmt.Errorf("retention policy: KeepDaily/KeepWeekly/KeepMonthly must not be negative (got daily=%d weekly=%d monthly=%d)", p.KeepDaily, p.KeepWeekly, p.KeepMonthly)
	}
	if p.MaxAge < 0 {
		return fmt.Errorf("retention policy: MaxAge must not be negative, got %s", p.MaxAge)
	}

	backups, err := store.List(ctx, serverID, 0)
	if err != nil {
		return err
	}
	if len(backups) == 0 {
		return nil
	}
	sort.SliceStable(backups, func(i, j int) bool {
		return backups[i].CompletedAt.After(backups[j].CompletedAt)
	})

	now := time.Now()
	keep := make(map[string]bool, len(backups))

	// Intersection: a backup must satisfy ALL active rules to be kept.
	// Initialize with all backups, then apply each rule as a filter.

	// Rule 1: MaxAge
	if p.MaxAge > 0 {
		for _, b := range backups {
			if now.Sub(b.CompletedAt) < p.MaxAge {
				keep[b.ID] = true
			}
		}
	} else {
		for _, b := range backups {
			keep[b.ID] = true
		}
	}

	// Rule 2: KeepDaily / KeepWeekly / KeepMonthly
	// KeepX=N means: keep at most N backups from each period.
	// The periods are: daily (0-24h), weekly (24h-7d), monthly (7d-30d)
	for _, period := range []struct {
		loHours, hiHours int
		max              int
	}{
		{0, 24, p.KeepDaily},
		{24, 168, p.KeepWeekly},
		{168, 720, p.KeepMonthly},
	} {
		if period.max <= 0 {
			continue
		}
		count := 0
		for _, b := range backups {
			age := now.Sub(b.CompletedAt)
			ageHours := int(age.Hours())
			if ageHours >= period.loHours && ageHours < period.hiHours {
				keep[b.ID] = true
				count++
				if count >= period.max {
					break
				}
			}
		}
	}

	// Rule 3: MaxBackups (limit to newest N, intersect with previous rules)
	if p.MaxBackups > 0 {
		maxKeep := make(map[string]bool)
		for i, b := range backups {
			if i < p.MaxBackups {
				maxKeep[b.ID] = true
			}
		}
		// Intersect
		for id := range keep {
			if !maxKeep[id] {
				delete(keep, id)
			}
		}
	}

	// Safety rail: always keep the single most recent backup. Sorting above
	// makes this independent of the backing store's ordering guarantees.
	keep[backups[0].ID] = true

	// Delete anything not marked for keeping
	for _, b := range backups {
		if !keep[b.ID] {
			if err := store.Delete(ctx, b.ID); err != nil {
				return err
			}
		}
	}

	return nil
}
