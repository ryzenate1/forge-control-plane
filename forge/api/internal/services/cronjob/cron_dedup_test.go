package cronjob

import (
	"context"
	"testing"
	"time"

	"gamepanel/forge/internal/store"
)

// NOTE: the in-process duplicate-execution guard that this file used to test
// is gone. Service no longer carries an inFlight map/inFlightMu mutex, and
// tryAcquireInFlight/releaseInFlight were deleted, so nothing suppresses a
// second firing of the same job while an execution is running — dedup is now
// entirely the store's problem (CreateCronJobExecution), and the
// store.ErrCronDuplicate sentinel the old integration test looked for no
// longer exists either. The non-blocking retry helper scheduleRetry
// (time.AfterFunc) is gone too: executeJob retries inline with a blocking
// time.Sleep, and the registered cron callback invokes executeJob directly
// instead of handing it to a goroutine. The scheduling behaviour that still
// exists is pinned below.

func newDedupService(t *testing.T) *Service {
	t.Helper()
	svc, err := New(&store.Store{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestInFlightDedupAndAfterFuncRetryWereRemoved(t *testing.T) {
	t.Skip("tryAcquireInFlight/releaseInFlight/inFlight and scheduleRetry were deleted from service.go; there is no in-process dedup or AfterFunc retry left to exercise")
}

func TestScheduleJobRegistersEntry(t *testing.T) {
	ctx := context.Background()
	svc := newDedupService(t)
	svc.cron.Start()
	defer svc.Stop()

	job := store.CronJob{
		ID:       "schedule-test",
		Schedule: "*/5 * * * *",
		Command:  "echo hello",
		Type:     "shell",
		Enabled:  true,
	}
	if err := svc.scheduleJob(ctx, job); err != nil {
		t.Fatalf("scheduleJob: %v", err)
	}

	next := svc.NextRun(job)
	if next == nil {
		t.Fatal("expected non-nil next run after scheduling")
	}
	if !next.After(time.Now().Add(-time.Second)) {
		t.Fatalf("next run should be in the future, got %v", next)
	}

	svc.mu.Lock()
	entryID, ok := svc.entries[job.ID]
	svc.mu.Unlock()
	if !ok {
		t.Fatal("expected entry to be tracked in entries map")
	}
	if entryID < 0 {
		t.Fatalf("unexpected entry id %d", entryID)
	}

	// Re-scheduling replaces rather than duplicates the entry.
	if err := svc.scheduleJob(ctx, store.CronJob{ID: job.ID, Schedule: "*/10 * * * *", Command: job.Command, Enabled: true}); err != nil {
		t.Fatalf("re-scheduleJob: %v", err)
	}
	if second := svc.NextRun(store.CronJob{ID: job.ID}); second == nil {
		t.Fatal("expected next run after re-scheduling")
	}
}

func TestRescheduleJobRemovesDisabledEntry(t *testing.T) {
	ctx := context.Background()
	svc := newDedupService(t)
	svc.cron.Start()
	defer svc.Stop()

	job := store.CronJob{ID: "disable-me", Schedule: "*/5 * * * *", Command: "true", Enabled: true}
	if err := svc.RescheduleJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if svc.NextRun(job) == nil {
		t.Fatal("expected job to be scheduled while enabled")
	}

	job.Enabled = false
	if err := svc.RescheduleJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if svc.NextRun(job) != nil {
		t.Error("expected disabled job to be unscheduled")
	}
}
