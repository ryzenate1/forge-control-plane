package backup

import (
	"context"
	"time"
)

type RetentionPolicy struct {
	MaxBackups  int
	MaxAge      time.Duration
	KeepDaily   int
	KeepWeekly  int
	KeepMonthly int
}

func (p RetentionPolicy) Apply(ctx context.Context, store Store, serverID string) error {
	backups, err := store.List(ctx, serverID, 0)
	if err != nil {
		return err
	}
	if len(backups) == 0 {
		return nil
	}

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
