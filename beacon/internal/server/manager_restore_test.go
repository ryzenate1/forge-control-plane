package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gamepanel/beacon/internal/runtime"
)

// Test that a restore claim blocks concurrent power start and concurrent install,
// and that the second restore is also rejected with 409 semantics.

func TestManager_RestoreBlocksPowerAndInstall(t *testing.T) {
	rt := newBlockingRuntime()
	close(rt.release)
	m := NewServerManager(rt)
	root := t.TempDir()
	m.MarkCreated("s1", filepath.Join(root, "s1"), 10)
	m.MarkConfigurationSynced("s1", -1)
	// also set a real root so disk check passes
	if err := os.MkdirAll(filepath.Join(root, "s1"), 0o750); err != nil {
		t.Fatal(err)
	}
	m.State("s1").RootDir = filepath.Join(root, "s1")
	m.State("s1").ConfigurationSynced = true
	m.State("s1").DiskLimitBytes = 0 // disable disk check for simplicity

	if err := m.BeginRestore("s1"); err != nil {
		t.Fatalf("BeginRestore failed: %v", err)
	}
	// Power start must be rejected while restoring
	if err := m.HandlePower(context.Background(), "s1", "start"); err == nil {
		t.Fatalf("expected HandlePower to be blocked during restore, got nil")
	}
	// Install must be rejected while restoring
	if err := m.BeginInstall("s1"); err == nil {
		t.Fatalf("expected BeginInstall to be blocked during restore, got nil")
	}
	// Second restore must be rejected
	if err := m.BeginRestore("s1"); err == nil {
		t.Fatalf("expected second restore to be rejected")
	}
	if !m.IsRestoring("s1") {
		t.Fatal("IsRestoring should be true while restore is claimed")
	}
	m.EndRestore("s1", false)
	if m.IsRestoring("s1") {
		t.Fatal("IsRestoring should be false after EndRestore")
	}
	// After restore ends, power should succeed (or at least not be blocked by restoring)
	// It will try to start container; our blockingRuntime will succeed immediately if release is open.
	close(rt.started) // ensure not blocking
	// Reset for second start: need a new runtime that doesn't block
	rt2 := &blockingRuntime{started: make(chan struct{}, 1), release: make(chan struct{})}
	close(rt2.release)
	m2 := NewServerManager(rt2)
	m2.MarkCreated("s2", filepath.Join(root, "s2"), 10)
	m2.MarkConfigurationSynced("s2", -1)
	m2.State("s2").RootDir = filepath.Join(root, "s2")
	m2.State("s2").DiskLimitBytes = 0
	_ = os.MkdirAll(filepath.Join(root, "s2"), 0o750)
	if err := m2.BeginRestore("s2"); err != nil {
		t.Fatalf("BeginRestore s2: %v", err)
	}
	m2.EndRestore("s2", true) // failed restore
	if m2.IsRestoring("s2") {
		t.Fatal("IsRestoring should be false after failed restore")
	}
	// After failed restore, power should not be blocked by restoring state
	if err := m2.HandlePower(context.Background(), "s2", "start"); err != nil {
		t.Fatalf("HandlePower after failed restore should not be blocked by restore: %v", err)
	}
}

func TestManager_RestoreBlocksConcurrentPowerAction(t *testing.T) {
	rt := newBlockingRuntime()
	m := NewServerManager(rt)
	root := t.TempDir()
	m.MarkCreated("demo", root, 0)
	m.MarkConfigurationSynced("demo", -1)
	m.State("demo").DiskLimitBytes = 0
	m.State("demo").RootDir = root
	m.State("demo").ConfigurationSynced = true
	// Ensure underlying runtime Start would succeed if not blocked
	close(rt.release)

	// Start a restore in background that holds the RunningAction = "restore"
	if err := m.BeginRestore("demo"); err != nil {
		t.Fatalf("BeginRestore: %v", err)
	}
	// Simulate concurrent power operation racing with restore
	done := make(chan error, 1)
	go func() {
		done <- m.HandlePower(context.Background(), "demo", "start")
	}()
	err := <-done
	if err == nil {
		t.Fatal("expected concurrent HandlePower to be rejected during restore")
	}
	if err.Error() != "server is restoring backup" && err.Error() != "another server action is already running" {
		t.Fatalf("unexpected error %q", err.Error())
	}
	m.EndRestore("demo", false)
}

func TestManager_PowerBlocksRestore(t *testing.T) {
	rt := newBlockingRuntime()
	m := NewServerManager(rt)
	root := t.TempDir()
	m.MarkCreated("demo", root, 0)
	m.MarkConfigurationSynced("demo", -1)
	m.State("demo").DiskLimitBytes = 0
	m.State("demo").RootDir = root
	m.State("demo").ConfigurationSynced = true

	// Hold power action
	done := make(chan error, 1)
	go func() {
		done <- m.HandlePower(context.Background(), "demo", "start")
	}()
	<-rt.started // power has claimed RunningAction = "start"

	// Now restore should be blocked
	if err := m.BeginRestore("demo"); err == nil {
		t.Fatal("expected BeginRestore to be blocked while power action holds RunningAction")
	}
	close(rt.release)
	if err := <-done; err != nil {
		t.Fatalf("first power action failed: %v", err)
	}
	// After power completes, restore should be allowed
	if err := m.BeginRestore("demo"); err != nil {
		t.Fatalf("BeginRestore after power should succeed: %v", err)
	}
	m.EndRestore("demo", false)
}

// Ensure HandlePower checks IsRestoring similar to InstallationState==installing guard.
func TestManager_HandlePowerRejectsWhenRestoringFlagSet(t *testing.T) {
	rt := newBlockingRuntime()
	close(rt.release)
	m := NewServerManager(rt)
	root := t.TempDir()
	m.MarkCreated("demo", root, 0)
	m.MarkConfigurationSynced("demo", -1)
	m.State("demo").DiskLimitBytes = 0
	m.State("demo").RootDir = root
	state := m.State("demo")
	state.Restoring = true
	state.RestoreState = "restoring"

	if err := m.HandlePower(context.Background(), "demo", "start"); err == nil {
		t.Fatal("expected HandlePower to reject when Restoring=true")
	}
	state.Restoring = false
	state.RestoreState = ""
	if err := m.HandlePower(context.Background(), "demo", "start"); err != nil {
		t.Fatalf("HandlePower should succeed after clearing restoring flag: %v", err)
	}
}

// Verify state persistence helpers not needed for restore lock but Ensure cleanup
func TestManager_RestoreStateTransitions(t *testing.T) {
	m := NewServerManager(nil)
	s := m.State("t1")
	if s.RestoreState != "" || s.Restoring {
		t.Fatalf("initial restore state should be empty, got %+v", s)
	}
	if err := m.BeginRestore("t1"); err != nil {
		t.Fatalf("BeginRestore: %v", err)
	}
	if !m.IsRestoring("t1") {
		t.Fatal("IsRestoring after BeginRestore")
	}
	m.EndRestore("t1", false)
	if m.IsRestoring("t1") {
		t.Fatal("IsRestoring after EndRestore success should be false")
	}
	if m.State("t1").RestoreState != "restored" {
		t.Fatalf("RestoreState after success = %q, want restored", m.State("t1").RestoreState)
	}
	if err := m.BeginRestore("t1"); err != nil {
		t.Fatalf("second BeginRestore: %v", err)
	}
	m.EndRestore("t1", true)
	if m.State("t1").RestoreState != "failed" {
		t.Fatalf("RestoreState after fail = %q, want failed", m.State("t1").RestoreState)
	}
	// Ensure imports are used
	_ = runtime.ContainerState{}
}
