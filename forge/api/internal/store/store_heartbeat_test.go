package store

import (
	"testing"
)

func TestNodeHeartbeatStateConstants(t *testing.T) {
	states := []NodeHeartbeatState{
		NodeHeartbeatStateHealthy,
		NodeHeartbeatStateSuspected,
		NodeHeartbeatStateUnreachable,
		NodeHeartbeatStateOffline,
		NodeHeartbeatStateRecovering,
	}
	expected := []string{"healthy", "suspected", "unreachable", "offline", "recovering"}
	for i, state := range states {
		if string(state) != expected[i] {
			t.Fatalf("NodeHeartbeatState %d = %q, want %q", i, string(state), expected[i])
		}
	}
}

func TestNodeActualStateConstants(t *testing.T) {
	states := []NodeActualState{
		NodeActualStateOnline,
		NodeActualStateDegraded,
		NodeActualStateOffline,
	}
	expected := []string{"online", "degraded", "offline"}
	for i, state := range states {
		if string(state) != expected[i] {
			t.Fatalf("NodeActualState %d = %q, want %q", i, string(state), expected[i])
		}
	}
}

func TestNodeHeartbeatStateStringValues(t *testing.T) {
	if v := NodeHeartbeatState("healthy"); v != NodeHeartbeatStateHealthy {
		t.Fatalf("expected healthy, got %s", v)
	}
	if v := NodeHeartbeatState("suspected"); v != NodeHeartbeatStateSuspected {
		t.Fatalf("expected suspected, got %s", v)
	}
	if v := NodeHeartbeatState("unreachable"); v != NodeHeartbeatStateUnreachable {
		t.Fatalf("expected unreachable, got %s", v)
	}
	if v := NodeHeartbeatState("offline"); v != NodeHeartbeatStateOffline {
		t.Fatalf("expected offline, got %s", v)
	}
	if v := NodeHeartbeatState("recovering"); v != NodeHeartbeatStateRecovering {
		t.Fatalf("expected recovering, got %s", v)
	}
}

func TestClassificationStateEnums(t *testing.T) {
	heartbeatCount := 5
	heartbeatStates := []NodeHeartbeatState{
		NodeHeartbeatStateHealthy,
		NodeHeartbeatStateSuspected,
		NodeHeartbeatStateUnreachable,
		NodeHeartbeatStateOffline,
		NodeHeartbeatStateRecovering,
	}
	if len(heartbeatStates) != heartbeatCount {
		t.Fatalf("expected %d heartbeat states, got %d", heartbeatCount, len(heartbeatStates))
	}

	actualCount := 3
	actualStates := []NodeActualState{
		NodeActualStateOnline,
		NodeActualStateDegraded,
		NodeActualStateOffline,
	}
	if len(actualStates) != actualCount {
		t.Fatalf("expected %d actual states, got %d", actualCount, len(actualStates))
	}

	distinctHeartbeat := map[string]bool{}
	for _, s := range heartbeatStates {
		if distinctHeartbeat[string(s)] {
			t.Fatalf("duplicate heartbeat state: %s", s)
		}
		distinctHeartbeat[string(s)] = true
	}

	distinctActual := map[string]bool{}
	for _, s := range actualStates {
		if distinctActual[string(s)] {
			t.Fatalf("duplicate actual state: %s", s)
		}
		distinctActual[string(s)] = true
	}
}
