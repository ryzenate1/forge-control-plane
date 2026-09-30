package pipeline

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"
)

// scheduleDueInSlot reports whether sched fires within the one-minute slot
// beginning at slot. robfig's Next is strictly greater than its argument, so
// stepping back one second makes the slot's start boundary inclusive. This is
// the primitive the scheduler loop uses to decide, once per minute, which
// cron-triggered pipelines are due.
func scheduleDueInSlot(sched cron.Schedule, slot time.Time) bool {
	return sched.Next(slot.Add(-time.Second)).Before(slot.Add(time.Minute))
}

// scheduleLoop drives cron-triggered pipelines. Before this existed,
// Trigger.Cron was a phantom field: it was parsed, stored and round-tripped but
// never evaluated, so scheduled pipelines silently never ran. It fires at most
// one run per pipeline per matching minute slot and tolerates unparsable cron
// expressions by skipping them.
func (s *Service) scheduleLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	lastFired := map[string]time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.fireDueSchedules(ctx, now, lastFired)
		}
	}
}

func (s *Service) fireDueSchedules(ctx context.Context, now time.Time, lastFired map[string]time.Time) {
	defs, err := s.store.ListDefinitions(ctx)
	if err != nil {
		return
	}
	slot := now.Truncate(time.Minute)
	for _, def := range defs {
		if def.Trigger.Type != "schedule" || !def.Trigger.Enabled || def.Trigger.Cron == "" {
			continue
		}
		if last, ok := lastFired[def.ID]; ok && !last.Before(slot) {
			continue // already fired this slot
		}
		sched, err := cron.ParseStandard(def.Trigger.Cron)
		if err != nil {
			continue
		}
		if scheduleDueInSlot(sched, slot) {
			if _, err := s.TriggerRun(ctx, def.ID, "schedule", "scheduler"); err == nil {
				lastFired[def.ID] = slot
				s.logger.Info("fired scheduled pipeline", "pipeline", def.ID, "name", def.Name)
			}
		}
	}
}
