package scheduledtasks

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"
)

const (
	// schedulerTickInterval is how often the leader renews its lease and
	// scans for due tasks. 15s keeps worst-case dispatch lateness well under
	// a minute (the smallest cron granularity) without hammering the DB.
	schedulerTickInterval = 15 * time.Second
	// leaseTTL bounds how long a wedged leader can block failover. The lease
	// is renewed every tick, so healthy leaders never lose it.
	leaseTTL = 45 * time.Second
	// maxDispatchConcurrency caps in-flight Beacon round-trips per tick so a
	// huge due-batch cannot swamp the API or the node fleet.
	maxDispatchConcurrency = 8
	// dueBatchSize limits one scan; leftovers are picked up next tick.
	dueBatchSize = 64
	// nextRunBackoff keeps a task whose stored schedule became unparseable
	// (e.g. written by a buggy release) from hot-looping the due scan.
	nextRunBackoff = 5 * time.Minute
)

// Start launches the background scheduler loop. Only the replica holding the
// store-based leader lease does work; followers wake on every tick to try to
// take over, so a crashed leader is replaced within leaseTTL. The loop exits
// with ctx and releases the lease for a fast graceful handover.
func (s *Service) Start(ctx context.Context) error {
	if s.store == nil {
		return errors.New("scheduledtasks: store required")
	}
	go s.loop(ctx)
	return nil
}

func (s *Service) loop(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			s.logger.Error("scheduled tasks scheduler panic recovered", "panic", r, "stack", string(buf[:n]))
		}
	}()

	leader := false
	ticker := time.NewTicker(schedulerTickInterval)
	defer ticker.Stop()

	// Try immediately at boot instead of waiting a full interval.
	s.tickPhase(ctx, &leader)
	for {
		select {
		case <-ctx.Done():
			s.releaseLease()
			return
		case <-ticker.C:
			s.tickPhase(ctx, &leader)
		}
	}
}

// tickPhase renewes/acquires leadership and, when in charge, runs one scan.
func (s *Service) tickPhase(ctx context.Context, leader *bool) {
	if ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	held, err := s.store.TryAcquireScheduledTasksLease(ctx, s.instanceID, leaseTTL)
	if err != nil {
		s.logger.Error("scheduled tasks lease attempt failed", "error", err)
		return
	}
	if !held {
		if *leader {
			s.logger.Info("scheduled tasks leadership moved to another instance", "instance", s.instanceID)
		}
		*leader = false
		return
	}
	wasLeader := *leader
	*leader = true

	if !wasLeader {
		s.logger.Info("acquired scheduled tasks scheduler leadership", "instance", s.instanceID)
		// Taking over: seed next_run_at for enabled tasks that lack one
		// (created while we were not leader, or legacy rows).
		s.backfillNextRuns(ctx)
	}
	s.runDueTasks(ctx)
}

// backfillNextRuns computes the first occurrence for enabled tasks whose
// next_run_at is null. Idempotent; safe to run on every leadership change.
func (s *Service) backfillNextRuns(ctx context.Context) {
	tasks, err := s.store.ListEnabledScheduledTasksWithoutNextRun(ctx)
	if err != nil {
		s.logger.Error("scheduled tasks next-run backfill scan failed", "error", err)
		return
	}
	now := time.Now().UTC()
	for i := range tasks {
		task := tasks[i]
		next, err := s.nextRun(task.Schedule, now)
		if err != nil {
			s.logger.Error("skipping unparseable schedule during backfill", "task_id", task.ID, "schedule", task.Schedule, "error", err)
			backoff := now.Add(nextRunBackoff)
			_ = s.store.UpdateScheduledTaskRunMeta(ctx, task.ID, nil, &backoff)
			continue
		}
		if err := s.store.UpdateScheduledTaskRunMeta(ctx, task.ID, nil, &next); err != nil {
			s.logger.Error("scheduled tasks backfill update failed", "task_id", task.ID, "error", err)
		}
	}
}

// runDueTasks scans for tasks whose occurrence came due and dispatches them.
// next_run_at is rolled forward BEFORE dispatch, so each occurrence is claimed
// exactly once by this leader even if the command is slow or fails.
func (s *Service) runDueTasks(ctx context.Context) {
	now := time.Now().UTC()
	due, err := s.store.ListDueScheduledTasks(ctx, now, dueBatchSize)
	if err != nil {
		s.logger.Error("scheduled tasks due scan failed", "error", err)
		return
	}
	if len(due) == 0 {
		return
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, maxDispatchConcurrency)
	for i := range due {
		task := due[i]
		if ctx.Err() != nil {
			break
		}

		next, err := s.nextRun(task.Schedule, now)
		if err != nil {
			s.logger.Error("due task has unparseable schedule; backing off", "task_id", task.ID, "schedule", task.Schedule, "error", err)
			backoff := now.Add(nextRunBackoff)
			_ = s.store.UpdateScheduledTaskRunMeta(ctx, task.ID, nil, &backoff)
			continue
		}
		lastRun := now
		if err := s.store.UpdateScheduledTaskRunMeta(ctx, task.ID, &lastRun, &next); err != nil {
			s.logger.Error("failed to claim due scheduled task", "task_id", task.ID, "error", err)
			continue
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(task ScheduledTask) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					buf := make([]byte, 4096)
					n := runtime.Stack(buf, false)
					s.logger.Error("scheduled task dispatch panic recovered", "task_id", task.ID, "panic", r, "stack", string(buf[:n]))
				}
			}()
			dispatchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
			defer cancel()
			if _, err := s.dispatch(dispatchCtx, task); err != nil && !errors.Is(err, ErrDispatch) {
				s.logger.Error("scheduled task dispatch error", "task_id", task.ID, "error", err)
			}
		}(task)
	}
	wg.Wait()
}

// releaseLease drops the leader lease on shutdown with a short grace so a
// dying request cannot wedge the DELETE behind an expired context.
func (s *Service) releaseLease() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.store.ReleaseScheduledTasksLease(ctx, s.instanceID); err != nil {
		s.logger.Warn("failed to release scheduled tasks lease on shutdown", "instance", s.instanceID, "error", fmt.Sprint(err))
	}
}
