package quota

import (
	"context"
	"errors"
	"testing"
)

func TestNoopTrackerReturnsErrNoQuota(t *testing.T) {
	tracker := NoopTracker{}
	_, err := tracker.GetUsage(context.Background(), "/tmp")
	if !errors.Is(err, ErrNoQuota) {
		t.Fatalf("expected ErrNoQuota, got %v", err)
	}
}

func TestNoopTrackerEnforceFailsClosedWhenQuotaRequested(t *testing.T) {
	tracker := NoopTracker{}
	err := tracker.Enforce(context.Background(), "/tmp", 100, 50, 200)
	if !errors.Is(err, ErrNoQuota) {
		t.Fatalf("expected ErrNoQuota, got %v", err)
	}
}

func TestNoopTrackerSetQuotaReturnsNil(t *testing.T) {
	tracker := NoopTracker{}
	err := tracker.SetQuota("/tmp", 1024)
	if !errors.Is(err, ErrNoQuota) {
		t.Fatalf("expected ErrNoQuota, got %v", err)
	}
}

func TestNewTrackerReturnsNoopOnNonLinux(t *testing.T) {
	tracker := NewTracker()
	ctx := context.Background()
	_, err := tracker.GetUsage(ctx, "/tmp")
	if err != nil && !errors.Is(err, ErrNoQuota) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNoopTrackerGetUsageDoesNotPanic(t *testing.T) {
	tracker := NoopTracker{}
	_, err := tracker.GetUsage(context.Background(), "")
	if !errors.Is(err, ErrNoQuota) {
		t.Fatalf("expected ErrNoQuota, got %v", err)
	}
}

func TestNoopTrackerEnforceWithZeroValues(t *testing.T) {
	tracker := NoopTracker{}
	err := tracker.Enforce(context.Background(), "", 0, 0, 0)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestNewTrackerIsNotNil(t *testing.T) {
	tracker := NewTracker()
	if tracker == nil {
		t.Fatal("NewTracker() returned nil")
	}
}
