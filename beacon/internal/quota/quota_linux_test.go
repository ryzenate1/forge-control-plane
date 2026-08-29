//go:build linux

package quota

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestLinuxTrackerGetUsage(t *testing.T) {
	tracker := &linuxTracker{}
	usage, err := tracker.GetUsage(context.Background(), "/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage.UsedBytes < 0 {
		t.Fatalf("used bytes should be non-negative, got %d", usage.UsedBytes)
	}
	if usage.LimitBytes <= 0 {
		t.Fatalf("limit bytes should be positive, got %d", usage.LimitBytes)
	}
	if usage.UsedPercent < 0 || usage.UsedPercent > 100 {
		t.Fatalf("used percent should be 0-100, got %f", usage.UsedPercent)
	}
}

func TestLinuxTrackerGetUsageOnTempDir(t *testing.T) {
	dir := t.TempDir()
	tracker := &linuxTracker{}
	usage, err := tracker.GetUsage(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage.UsedBytes < 0 {
		t.Fatalf("used bytes should be non-negative, got %d", usage.UsedBytes)
	}
}

func TestLinuxTrackerGetUsageNonExistentPath(t *testing.T) {
	tracker := &linuxTracker{}
	_, err := tracker.GetUsage(context.Background(), "/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestLinuxTrackerSetQuota(t *testing.T) {
	tracker := &linuxTracker{}
	err := tracker.SetQuota("/tmp", 1024)
	if !errors.Is(err, ErrNoQuota) {
		t.Fatalf("expected ErrNoQuota, got %v", err)
	}
}

func TestLinuxTrackerGetUsageOnFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "quota-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	tracker := &linuxTracker{}
	usage, err := tracker.GetUsage(context.Background(), tmpFile.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage.LimitBytes <= 0 {
		t.Fatalf("limit bytes should be positive, got %d", usage.LimitBytes)
	}
}

func TestEnforceReturnsErrorWhenLimitExceeded(t *testing.T) {
	tracker := &linuxTracker{}
	err := tracker.Enforce(context.Background(), "/tmp", 90, 20, 100)
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("expected ErrQuotaExceeded, got %v", err)
	}
}

func TestEnforceAllowsWhenWithinLimit(t *testing.T) {
	tracker := &linuxTracker{}
	err := tracker.Enforce(context.Background(), "/tmp", 50, 20, 100)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestEnforceAllowsWhenLimitIsZero(t *testing.T) {
	tracker := &linuxTracker{}
	err := tracker.Enforce(context.Background(), "/tmp", 90, 20, 0)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestEnforceAllowsWhenLimitIsNegative(t *testing.T) {
	tracker := &linuxTracker{}
	err := tracker.Enforce(context.Background(), "/tmp", 90, 20, -1)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestEnforceExactAtLimit(t *testing.T) {
	tracker := &linuxTracker{}
	err := tracker.Enforce(context.Background(), "/tmp", 80, 20, 100)
	if err != nil {
		t.Fatalf("expected nil when currentUsed+size == limit, got %v", err)
	}
}
