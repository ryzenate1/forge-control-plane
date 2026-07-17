package cron

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewSchedulerInvalidTimezone(t *testing.T) {
	_, err := NewScheduler("Invalid/Timezone")
	if err == nil {
		t.Fatal("expected error for invalid timezone")
	}
}

func TestNewSchedulerValidTimezone(t *testing.T) {
	s, err := NewScheduler("UTC")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s == nil {
		t.Fatal("scheduler is nil")
	}
}

func TestSchedulerStartStop(t *testing.T) {
	s, err := NewScheduler("UTC")
	if err != nil {
		t.Fatal(err)
	}
	s.AddJob("test", time.Second, func(ctx context.Context) error {
		return nil
	})
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if !s.IsRunning() {
		t.Fatal("expected scheduler to be running")
	}
	s.Stop()
	if s.IsRunning() {
		t.Fatal("expected scheduler to be stopped")
	}
}

func TestSchedulerStartIdempotent(t *testing.T) {
	s, err := NewScheduler("UTC")
	if err != nil {
		t.Fatal(err)
	}
	s.AddJob("test", time.Minute, func(ctx context.Context) error { return nil })
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("second start should be idempotent, got: %v", err)
	}
}

func TestSchedulerStopIdempotent(t *testing.T) {
	s, err := NewScheduler("UTC")
	if err != nil {
		t.Fatal(err)
	}
	s.Stop()
	s.Stop()
}

func TestSchedulerJobExecution(t *testing.T) {
	s, err := NewScheduler("UTC")
	if err != nil {
		t.Fatal(err)
	}
	var count atomic.Int32
	s.AddJob("counter", 100*time.Millisecond, func(ctx context.Context) error {
		count.Add(1)
		return nil
	})
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(550 * time.Millisecond)
	s.Stop()
	c := count.Load()
	if c < 3 {
		t.Fatalf("expected at least 3 executions, got %d", c)
	}
}

func TestSchedulerTimezone(t *testing.T) {
	s, err := NewScheduler("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	loc := s.scheduler.Location()
	if loc.String() != "America/New_York" {
		t.Fatalf("expected America/New_York, got %s", loc.String())
	}
}

func TestSchedulerContextCancellation(t *testing.T) {
	s, err := NewScheduler("UTC")
	if err != nil {
		t.Fatal(err)
	}
	var cancelled atomic.Bool
	s.AddJob("ctx-check", 100*time.Millisecond, func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			cancelled.Store(true)
		default:
		}
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	cancel()
	time.Sleep(200 * time.Millisecond)
	s.Stop()
	if !cancelled.Load() {
		t.Fatal("expected job to observe context cancellation")
	}
}
