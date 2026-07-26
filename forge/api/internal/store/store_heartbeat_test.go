package store

import (
	"context"
	"testing"
)

func TestNodeHeartbeatStateConstants(t *testing.T) {
	states := []NodeHeartbeatState{
		NodeHeartbeatStateHealthy,
		NodeHeartbeatStateSuspected,
		NodeHeartbeatStateUnreachable,
		NodeHeartbeatStateOffline,
		NodeHeartbeatStateRecovering,
		NodeHeartbeatStateReconciling,
	}
	expected := []string{"healthy", "suspected", "unreachable", "offline", "recovering", "reconciling"}
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
		NodeActualStateReconciling,
	}
	expected := []string{"online", "degraded", "offline", "reconciling"}
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
	if v := NodeHeartbeatState("reconciling"); v != NodeHeartbeatStateReconciling {
		t.Fatalf("expected reconciling, got %s", v)
	}
}

func TestClassificationStateEnums(t *testing.T) {
	heartbeatCount := 6
	heartbeatStates := []NodeHeartbeatState{
		NodeHeartbeatStateHealthy,
		NodeHeartbeatStateSuspected,
		NodeHeartbeatStateUnreachable,
		NodeHeartbeatStateOffline,
		NodeHeartbeatStateRecovering,
		NodeHeartbeatStateReconciling,
	}
	if len(heartbeatStates) != heartbeatCount {
		t.Fatalf("expected %d heartbeat states, got %d", heartbeatCount, len(heartbeatStates))
	}

	actualCount := 4
	actualStates := []NodeActualState{
		NodeActualStateOnline,
		NodeActualStateDegraded,
		NodeActualStateOffline,
		NodeActualStateReconciling,
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

func TestHeartbeatEnumsSupportReconciling(t *testing.T) {
	s := migrationTestStore(t, false)
	var count int
	err := s.db.QueryRow(context.Background(), `
		SELECT COUNT(*)
			FROM pg_enum e
			JOIN pg_type t ON t.oid = e.enumtypid
			JOIN pg_namespace n ON n.oid = t.typnamespace
			WHERE t.typname IN ('node_heartbeat_state', 'node_actual_state')
			  AND e.enumlabel = 'reconciling'
			  AND n.nspname = current_schema()
		`).Scan(&count)
	if err != nil {
		t.Fatalf("query heartbeat enums: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected both heartbeat enums to support reconciling, got %d", count)
	}
}
