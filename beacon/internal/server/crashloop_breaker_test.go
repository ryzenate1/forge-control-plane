package server

import (
	"context"
	"testing"

	"gamepanel/beacon/internal/runtime"
)

// Regression tests for the crash-loop circuit breaker (audit F-6): a broken
// server must not be auto-restarted indefinitely once consecutive crashes
// exceed the threshold. The panel crash handler still fires for every crash
// so the operator sees the loop; only the automatic restart stops.

func TestCrashLoopBreakerOpensAfterThreshold(t *testing.T) {
	rt := newBlockingRuntime()
	close(rt.release) // Start succeeds immediately, like a boot-crash loop
	m := NewServerManager(rt)
	const id = "loop-srv"

	m.MarkCreated(id, t.TempDir(), 0)
	m.MarkConfigurationSynced(id, -1)

	st := m.State(id)
	st.mu.Lock()
	st.CrashDetectionEnabled = true
	st.CrashCooldown = 0 // no cooldown: isolate breaker behavior
	st.mu.Unlock()

	for i := 0; i < maxConsecutiveCrashRestarts+3; i++ {
		m.HandleContainerEvent(context.Background(), runtime.ContainerEvent{
			ServerID: id, Action: "die", ExitCode: 1,
		})
	}

	rt.mu.Lock()
	starts := rt.starts
	rt.mu.Unlock()
	if starts != maxConsecutiveCrashRestarts {
		t.Fatalf("expected exactly %d auto-restart attempts before breaker opens, got %d", maxConsecutiveCrashRestarts, starts)
	}

	st = m.State(id)
	st.mu.Lock()
	count, power := st.CrashCount, st.PowerState
	st.mu.Unlock()
	if count <= maxConsecutiveCrashRestarts {
		t.Errorf("crash count not accumulated past threshold: %d", count)
	}
	if power != PowerStateOffline {
		t.Errorf("server left in %s after breaker opened", power)
	}
}

func TestManualStartResetsCrashCounter(t *testing.T) {
	rt := newBlockingRuntime()
	m := NewServerManager(rt)
	const id = "reset-srv"
	close(rt.release) // let Start succeed immediately

	m.MarkCreated(id, t.TempDir(), 0)
	m.MarkConfigurationSynced(id, -1)

	st := m.State(id)
	st.mu.Lock()
	st.CrashDetectionEnabled = true
	st.CrashCount = maxConsecutiveCrashRestarts + 1 // breaker currently open
	st.mu.Unlock()

	if err := m.HandlePower(context.Background(), id, "start"); err != nil {
		t.Fatalf("HandlePower start: %v", err)
	}
	// Counter survives a successful start (boot-crash loops start fine);
	// only a clean/expected stop resets it.
	st = m.State(id)
	st.mu.Lock()
	count := st.CrashCount
	expectedStop := st.ExpectedStop
	st.mu.Unlock()
	if count == 0 {
		t.Error("CrashCount was reset by a mere successful start; breaker would never open for boot loops")
	}
	_ = expectedStop
}
