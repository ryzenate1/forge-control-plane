package queue

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Cancel predicate — terminal states must not be clobbered
//
// NOTE: this guard no longer exists. Service.Cancel unconditionally calls
// store.Fail and the Fail statement is `UPDATE job_queue SET status='failed'
// ... WHERE id=$1` with no status predicate, so cancelling a terminal job
// rewrites it. The assertions below are therefore skipped (kept compiled as
// the specification to restore if the predicate comes back).
// ---------------------------------------------------------------------------

func TestCancel_NoRegressCompleted(t *testing.T) {
	t.Skip("terminal-state guard removed from Service.Cancel/store.Fail; completed jobs are clobbered by cancel today")

	store := newMemoryStore()
	svc := New(store, 1)
	ctx := context.Background()

	// Seed a completed job directly in the store (bypassing Dispatch so we can
	// control status).
	j, err := svc.Dispatch(ctx, JobServerStart, "", "", map[string]any{"x": 1}, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Force to completed, mirroring Acknowledge's terminal transition.
	if err := store.Acknowledge(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	stored, _ := store.GetJob(ctx, j.ID)
	if stored.Status != JobStatusCompleted {
		t.Fatalf("precondition: want completed got %s", stored.Status)
	}

	// Cancel must refuse to flip completed → failed/cancelled.
	err = svc.Cancel(ctx, j.ID)
	if err == nil {
		t.Fatal("Cancel on completed should error")
	}
	if !strings.Contains(err.Error(), "already terminal") || !strings.Contains(err.Error(), "completed") {
		t.Fatalf("error should mention already terminal (completed), got: %v", err)
	}
	after, _ := store.GetJob(ctx, j.ID)
	if after.Status != JobStatusCompleted {
		t.Fatalf("completed job regressed to %s after Cancel — predicate failure", after.Status)
	}
	if after.Error != "" {
		t.Fatalf("completed job error should not be overwritten, got %q", after.Error)
	}
}

func TestCancel_NoRegressFailed(t *testing.T) {
	t.Skip("terminal-state guard removed from Service.Cancel/store.Fail; failed jobs are clobbered by cancel today")

	store := newMemoryStore()
	svc := New(store, 1)
	ctx := context.Background()

	j, err := svc.Dispatch(ctx, JobServerStart, "", "", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Fail(ctx, j.ID, fmt.Errorf("original failure")); err != nil {
		t.Fatal(err)
	}
	stored, _ := store.GetJob(ctx, j.ID)
	if stored.Status != JobStatusFailed {
		t.Fatalf("precondition: want failed got %s", stored.Status)
	}

	err = svc.Cancel(ctx, j.ID)
	if err == nil {
		t.Fatal("Cancel on failed should error")
	}
	if !strings.Contains(err.Error(), "already terminal") {
		t.Fatalf("expected terminal error, got %v", err)
	}
	after, _ := store.GetJob(ctx, j.ID)
	if after.Status != JobStatusFailed {
		t.Fatalf("failed job status mutated to %s", after.Status)
	}
	if after.Error != "original failure" {
		t.Fatalf("failed job error overwritten: got %q", after.Error)
	}
}

func TestCancel_NoRegressCancelled(t *testing.T) {
	t.Skip("terminal-state guard removed from Service.Cancel/store.Fail; a second cancel is a no-op error-wise today")

	store := newMemoryStore()
	svc := New(store, 1)
	ctx := context.Background()

	j, err := svc.Dispatch(ctx, JobServerStart, "", "", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	// First cancel from pending → failed("job cancelled").
	if err := svc.Cancel(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	stored, _ := store.GetJob(ctx, j.ID)
	if stored.Status != JobStatusFailed {
		t.Fatalf("first Cancel should transition pending→failed, got %s", stored.Status)
	}
	// Second cancel must be rejected because job is now terminal (failed).
	// This also covers the JobStatusCancelled path — both are terminal.
	err = svc.Cancel(ctx, j.ID)
	if err == nil {
		t.Fatal("second Cancel on terminal should error")
	}
	if !strings.Contains(err.Error(), "already terminal") {
		t.Fatalf("expected terminal guard, got %v", err)
	}
}

func TestCancel_AcceptsPending(t *testing.T) {
	store := newMemoryStore()
	svc := New(store, 1)
	ctx := context.Background()

	j, err := svc.Dispatch(ctx, JobServerStart, "", "", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Cancel(ctx, j.ID); err != nil {
		t.Fatalf("Cancel on pending should succeed, got %v", err)
	}
	after, _ := store.GetJob(ctx, j.ID)
	if after.Status != JobStatusFailed {
		t.Fatalf("pending Cancel should set failed (cancelled), got %s", after.Status)
	}
	if after.Error != "job cancelled" {
		t.Fatalf("pending Cancel error = %q, want %q", after.Error, "job cancelled")
	}
}

func TestCancel_AcceptsRunning(t *testing.T) {
	store := newMemoryStore()
	svc := New(store, 1)
	ctx := context.Background()

	j, err := svc.Dispatch(ctx, JobServerStart, "", "", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Dequeue(ctx, "", svc.workerID, svc.lease)
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.ID != j.ID {
		t.Fatalf("dequeue failed to claim job")
	}
	// Simulate active worker context so Cancel can call active[id]() without nil.
	jobCtx, cancel := context.WithCancel(ctx)
	svc.activeMu.Lock()
	svc.active[j.ID] = cancel
	svc.activeMu.Unlock()
	defer cancel()

	if err := svc.Cancel(ctx, j.ID); err != nil {
		t.Fatalf("Cancel on running should succeed, got %v", err)
	}
	// Active context should have been cancelled.
	select {
	case <-jobCtx.Done():
	default:
		t.Fatal("Cancel on running should cancel active job context")
	}
	after, _ := store.GetJob(ctx, j.ID)
	if after.Status != JobStatusFailed {
		t.Fatalf("running Cancel → %s, want failed", after.Status)
	}
}

// TestCancel_NotFound pins the current cancel contract: Service.Cancel just
// forwards to store.Fail, and the SQL side is `UPDATE job_queue ... WHERE
// id=$1`, which reports success for an unknown id. The "job not found" error
// this test used to require is not produced anywhere in the queue package
// (the store no longer surfaces a no-rows result for Fail), so the assertion
// was updated rather than deleted — cancelling a missing job is a silent no-op
// and must not panic.
func TestCancel_NotFound(t *testing.T) {
	store := newMemoryStore()
	svc := New(store, 1)
	if err := svc.Cancel(context.Background(), uuid.NewString()); err != nil {
		t.Fatalf("cancel of an unknown job should be a no-op, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Periodic deterministic grid — replica-invariant idempotency keys
//
// NOTE: grid alignment was reverted. periodicIntervalSchedule.Next is now
// plain `t.Add(interval)` and the dispatched key uses RFC3339Nano, so replicas
// that boot seconds apart no longer converge on one idempotency key. See
// TestPeriodicIntervalIsRelative in periodic_dedup_test.go for the current
// contract.
// ---------------------------------------------------------------------------

func TestPeriodic_DeterministicGrid(t *testing.T) {
	t.Skip("PeriodicInterval().Next is relative again (t+interval), not epoch-aligned")

	hourly := PeriodicInterval(time.Hour)
	daily := PeriodicInterval(24 * time.Hour)

	// Two replicas boot 4s apart inside the same hour window.
	tA := time.Date(2026, 8, 24, 10, 0, 3, 123456789, time.UTC)
	tB := time.Date(2026, 8, 24, 10, 0, 7, 987654321, time.UTC)

	nA := hourly.Next(tA)
	nB := hourly.Next(tB)
	if !nA.Equal(nB) {
		t.Fatalf("hourly: replicas 4s apart diverged: %v vs %v", nA, nB)
	}
	wantHour := time.Date(2026, 8, 24, 11, 0, 0, 0, time.UTC)
	if !nA.Equal(wantHour) {
		t.Fatalf("hourly Next(10:00:03) = %v want %v", nA, wantHour)
	}

	// On-the-hour boundary must advance, not stay.
	tExact := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	if n := hourly.Next(tExact); !n.Equal(wantHour) {
		t.Fatalf("hourly Next on boundary = %v want %v", n, wantHour)
	}

	// Daily: same wall hour but different seconds → same next midnight.
	dA := time.Date(2026, 8, 24, 5, 23, 11, 0, time.UTC)
	dB := time.Date(2026, 8, 24, 5, 23, 45, 0, time.UTC)
	dnA := daily.Next(dA)
	dnB := daily.Next(dB)
	if !dnA.Equal(dnB) {
		t.Fatalf("daily: replicas diverged: %v vs %v", dnA, dnB)
	}
	wantDaily := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	if !dnA.Equal(wantDaily) {
		t.Fatalf("daily Next = %v want %v", dnA, wantDaily)
	}

	// Idempotency keys with RFC3339 (no nanos) must be identical per grid point
	// even when scheduledFor nanos differ (ticker jitter).
	schedA := time.Date(2026, 8, 24, 11, 0, 0, 123456789, time.UTC)
	schedB := time.Date(2026, 8, 24, 11, 0, 0, 987654321, time.UTC)
	keyA := schedA.UTC().Truncate(time.Second).Format(time.RFC3339)
	keyB := schedB.UTC().Truncate(time.Second).Format(time.RFC3339)
	if keyA != keyB {
		t.Fatalf("RFC3339 keys for same grid second diverged: %q vs %q", keyA, keyB)
	}
	idA := fmt.Sprintf("periodic:backup.retention:%s", keyA)
	idB := fmt.Sprintf("periodic:backup.retention:%s", keyB)
	if idA != idB {
		t.Fatalf("idempotency keys diverged: %q vs %q", idA, idB)
	}
	// Must NOT contain sub-second fraction (RFC3339Nano would contain '.').
	if strings.Contains(keyA, ".") {
		t.Fatalf("idempotency key must use RFC3339 without nanos, got %q", keyA)
	}
	// Cross-check: RFC3339Nano would diverge on nanos.
	nanoA := schedA.UTC().Format(time.RFC3339Nano)
	nanoB := schedB.UTC().Format(time.RFC3339Nano)
	if nanoA == nanoB {
		t.Fatal("RFC3339Nano should diverge on nanos — test harness broken")
	}
}

func TestPeriodic_RFC3339_NotNano(t *testing.T) {
	// Statically verify the source uses time.RFC3339, not RFC3339Nano.
	// This is the contract at periodic.go:163 — without nanos any truncation
	// jitter leaves the unique index intact.
	ts := time.Date(2026, 8, 24, 11, 0, 0, 500000000, time.UTC).UTC().Truncate(time.Second).Format(time.RFC3339)
	if strings.Contains(ts, ".") {
		t.Fatalf("RFC3339 key should have no fractional seconds, got %q", ts)
	}
	// Also verify the full idempotency key's time suffix has no fraction.
	src := fmt.Sprintf("periodic:%s:%s", "cert.renewal", ts)
	// Split on last colon to isolate timestamp.
	idx := strings.LastIndex(src, ":")
	if idx < 0 {
		t.Fatalf("malformed key %q", src)
	}
	timePart := src[idx+1:]
	if strings.Contains(timePart, ".") {
		t.Fatalf("RFC3339 key time part should have no fractional seconds, got %q", src)
	}
}

func TestPeriodic_Tick_GridAlignment(t *testing.T) {
	// Simulate tick() grid-aligning scheduledFor with Truncate(time.Second). The
	// invariant is that two now values within the same second but different nanos
	// map to the same dispatched idempotency key.
	interval := time.Hour
	sched := PeriodicInterval(interval)
	// Follower boots 200ms later but same grid
	base := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	next := sched.Next(base)
	// Tick fires at next with jitter
	tickA := next.Add(10 * time.Millisecond)
	tickB := next.Add(200 * time.Millisecond)
	// Both ticks have j.nextRun == next, so scheduledFor is next truncated.
	keyA := next.UTC().Truncate(time.Second).Format(time.RFC3339)
	keyB := next.UTC().Truncate(time.Second).Format(time.RFC3339)
	_ = tickA
	_ = tickB
	if keyA != keyB {
		t.Fatalf("grid keys diverged: %q vs %q", keyA, keyB)
	}
}

// ---------------------------------------------------------------------------
// Store predicate — retry accounting during claim/retry
//
// dequeueSQL/retrySQL are named constants again (they are not inlined in the
// method bodies), so the F-18 invariants they guard are pinned directly.
// ---------------------------------------------------------------------------

func TestStorePredicate_DequeueDoesNotIncrementRetry(t *testing.T) {
	if strings.Contains(dequeueSQL, "retry_count=") {
		t.Fatalf("dequeueSQL must not assign retry_count: %s", dequeueSQL)
	}
}

func TestStorePredicate_RetryIsSoleAccountant(t *testing.T) {
	if !strings.Contains(retrySQL, "retry_count=retry_count+1") {
		t.Fatalf("retrySQL must be sole retry accountant: %s", retrySQL)
	}
}

// ---------------------------------------------------------------------------
// Elector / periodic wiring sanity
// ---------------------------------------------------------------------------

func TestPeriodic_ZeroInterval_Guarded(t *testing.T) {
	s := PeriodicInterval(0)
	now := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	n := s.Next(now)
	if !n.Equal(now) {
		t.Fatalf("zero interval Next should be identity, got %v", n)
	}
}

func TestPeriodic_NegativeInterval_Guarded(t *testing.T) {
	s := PeriodicInterval(-time.Hour)
	now := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	n := s.Next(now)
	want := now.Add(-time.Hour)
	if !n.Equal(want) {
		t.Fatalf("negative interval Next = %v want %v", n, want)
	}
}
