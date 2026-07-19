package http

import (
	"context"
	"testing"
	"time"

	"gamepanel/forge/internal/store"
)

func TestScheduleWakeDelay(t *testing.T) {
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	fallback := time.Minute
	if got := scheduleWakeDelay(now, nil, fallback); got != fallback {
		t.Fatalf("nil next run uses fallback: got %s", got)
	}
	next := now.Add(5 * time.Second)
	if got := scheduleWakeDelay(now, &next, fallback); got != 5*time.Second {
		t.Fatalf("future next run uses exact delay: got %s", got)
	}
	next = now.Add(-time.Second)
	if got := scheduleWakeDelay(now, &next, fallback); got != 0 {
		t.Fatalf("past next run wakes immediately: got %s", got)
	}
	next = now.Add(5 * time.Minute)
	if got := scheduleWakeDelay(now, &next, fallback); got != fallback {
		t.Fatalf("far next run is capped by fallback: got %s", got)
	}
}

func TestScheduleOffsetRespectsDelayAndCancellation(t *testing.T) {
	runner := &scheduleRunner{}
	started := time.Now()
	if err := runner.waitOffset(context.Background(), store.ClaimedSchedule{}, 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) < 15*time.Millisecond {
		t.Fatal("task offset was not respected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runner.waitOffset(ctx, store.ClaimedSchedule{}, time.Hour); err == nil {
		t.Fatal("canceled offset wait must fail")
	}
}

func TestScheduleFailurePolicy(t *testing.T) {
	if continueAfterTaskFailure(store.ScheduleTask{ContinueOnFailure: false}) {
		t.Fatal("failure should halt when continueOnFailure is false")
	}
	if !continueAfterTaskFailure(store.ScheduleTask{ContinueOnFailure: true}) {
		t.Fatal("failure should continue when continueOnFailure is true")
	}
	status, _ := scheduleRunOutcome(true, true)
	if status != store.ScheduleRunPartial {
		t.Fatalf("mixed outcome = %s", status)
	}
	status, _ = scheduleRunOutcome(false, true)
	if status != store.ScheduleRunFailed {
		t.Fatalf("failed outcome = %s", status)
	}
	status, errMessage := scheduleRunOutcome(true, false)
	if status != store.ScheduleRunSuccess || errMessage != nil {
		t.Fatalf("success outcome = %s, %v", status, errMessage)
	}
}
