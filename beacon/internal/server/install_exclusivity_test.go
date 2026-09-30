package server

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// Regression tests for the install authorization/mutual-exclusion boundary
// (SEC-5.5): installs must hold the server's RunningAction slot exclusively,
// refuse to run alongside power operations, and be refused for suspended
// servers.

func newInstallTestManager(t *testing.T) *ServerManager {
	t.Helper()
	return NewServerManager(newBlockingRuntime())
}

func TestBeginInstallClaimsExclusively(t *testing.T) {
	m := newInstallTestManager(t)

	if err := m.BeginInstall("srv-1"); err != nil {
		t.Fatalf("first BeginInstall should succeed: %v", err)
	}
	if err := m.BeginInstall("srv-1"); err == nil {
		t.Fatal("second concurrent BeginInstall must be rejected")
	}
	if err := m.HandlePower(context.Background(), "srv-1", "start"); err == nil {
		t.Fatal("power op during install must be rejected")
	}
	state := m.State("srv-1")
	state.mu.Lock()
	got := state.RunningAction
	state.mu.Unlock()
	if got != "install" {
		t.Errorf("RunningAction = %q, want install", got)
	}
}

func TestEndInstallReleasesClaim(t *testing.T) {
	m := newInstallTestManager(t)

	if err := m.BeginInstall("srv-2"); err != nil {
		t.Fatalf("BeginInstall: %v", err)
	}
	m.EndInstall("srv-2", false)
	state := m.State("srv-2")
	state.mu.Lock()
	action, inst := state.RunningAction, state.InstallationState
	state.mu.Unlock()
	if action != "" {
		t.Errorf("RunningAction = %q, want empty after EndInstall", action)
	}
	if inst != "installed" {
		t.Errorf("InstallationState = %q, want installed", inst)
	}

	// Claim is reusable after release.
	if err := m.BeginInstall("srv-2"); err != nil {
		t.Fatalf("BeginInstall after EndInstall: %v", err)
	}
}

func TestEndInstallFailedMarksState(t *testing.T) {
	m := newInstallTestManager(t)
	if err := m.BeginInstall("srv-3"); err != nil {
		t.Fatalf("BeginInstall: %v", err)
	}
	m.EndInstall("srv-3", true)
	state := m.State("srv-3")
	state.mu.Lock()
	inst := state.InstallationState
	state.mu.Unlock()
	if inst != "failed" {
		t.Errorf("InstallationState = %q, want failed", inst)
	}
}

func TestBeginInstallRejectsSuspendedServer(t *testing.T) {
	m := newInstallTestManager(t)
	state := m.State("srv-susp")
	state.mu.Lock()
	state.Suspended = true
	state.mu.Unlock()

	err := m.BeginInstall("srv-susp")
	if err == nil || !strings.Contains(err.Error(), "suspended") {
		t.Fatalf("expected suspended rejection, got %v", err)
	}
	state.mu.Lock()
	action := state.RunningAction
	state.mu.Unlock()
	if action != "" {
		t.Errorf("rejected install leaked RunningAction %q", action)
	}
}

func TestConcurrentInstallExactlyOneWins(t *testing.T) {
	m := newInstallTestManager(t)
	const contenders = 16

	var wg sync.WaitGroup
	wins := make(chan struct{}, contenders)
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := m.BeginInstall("srv-race"); err == nil {
				wins <- struct{}{}
			}
		}()
	}
	wg.Wait()
	close(wins)
	if len(wins) != 1 {
		t.Fatalf("expected exactly 1 winner among %d contenders, got %d", contenders, len(wins))
	}
	m.EndInstall("srv-race", false)
}
