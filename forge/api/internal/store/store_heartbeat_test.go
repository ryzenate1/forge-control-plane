package store

import (
	"context"
	"testing"
)

func TestSetNodeHeartbeatClassification_StateTransition(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t, ctx)
	nodeID := createTestNode(t, ctx, s, "hb")

	previous, updated, err := s.SetNodeHeartbeatClassification(ctx, nodeID, NodeHeartbeatStateUnreachable, NodeActualStateDegraded, 0, "heartbeat exceeded offline threshold")
	if err != nil {
		t.Fatal(err)
	}
	if updated.HeartbeatState != string(NodeHeartbeatStateUnreachable) {
		t.Fatalf("heartbeat_state = %q, want %q", updated.HeartbeatState, NodeHeartbeatStateUnreachable)
	}
	if updated.ActualState != string(NodeActualStateDegraded) {
		t.Fatalf("actual_state = %q, want %q", updated.ActualState, NodeActualStateDegraded)
	}
	if updated.HeartbeatRecoveryCount != 0 {
		t.Fatalf("recovery count = %d, want 0", updated.HeartbeatRecoveryCount)
	}
	if previous.HeartbeatState == updated.HeartbeatState {
		t.Fatal("previous and updated heartbeat states must differ for a transition")
	}

	// Re-classifying with the same state must not record a duplicate transition.
	if _, _, err := s.SetNodeHeartbeatClassification(ctx, nodeID, NodeHeartbeatStateUnreachable, NodeActualStateDegraded, 1, "same state again"); err != nil {
		t.Fatal(err)
	}
	var transitions int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM state_transitions WHERE resource_id = $1 AND resource_type = 'node'`, nodeID).Scan(&transitions); err != nil {
		t.Fatal(err)
	}
	if transitions != 2 {
		t.Fatalf("expected 2 state_transitions rows (heartbeat + actual), got %d", transitions)
	}

	// Classifying an unknown node must fail.
	if _, _, err := s.SetNodeHeartbeatClassification(ctx, "no-such-node", NodeHeartbeatStateHealthy, NodeActualStateOnline, 0, "test"); err == nil {
		t.Fatal("expected error for unknown node")
	}
}

func TestSetNodeHeartbeatClassification_RecoveryCount(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t, ctx)
	nodeID := createTestNode(t, ctx, s, "hb-rec")

	_, updated, err := s.SetNodeHeartbeatClassification(ctx, nodeID, NodeHeartbeatStateRecovering, NodeActualStateReconciling, 3, "recovery in progress")
	if err != nil {
		t.Fatal(err)
	}
	if updated.HeartbeatRecoveryCount != 3 {
		t.Fatalf("recovery count = %d, want 3", updated.HeartbeatRecoveryCount)
	}
	if updated.HeartbeatState != string(NodeHeartbeatStateRecovering) {
		t.Fatalf("heartbeat_state = %q, want %q", updated.HeartbeatState, NodeHeartbeatStateRecovering)
	}
	if updated.ActualState != string(NodeActualStateReconciling) {
		t.Fatalf("actual_state = %q, want %q", updated.ActualState, NodeActualStateReconciling)
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
