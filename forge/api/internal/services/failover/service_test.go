package failover

import (
	"context"
	"testing"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"
)

func TestDetermineFailoverAction_ReplicatedNonLocal(t *testing.T) {
	t.Parallel()
	action := DetermineFailoverAction("shared", false, true)
	if action != FailoverActionEvacuate {
		t.Fatalf("DetermineFailoverAction() = %s, want evacuate", action)
	}
}

func TestDetermineFailoverAction_SharedStorage(t *testing.T) {
	t.Parallel()
	action := DetermineFailoverAction("shared", false, false)
	if action != FailoverActionEvacuate {
		t.Fatalf("DetermineFailoverAction() = %s, want evacuate", action)
	}
}

func TestDetermineFailoverAction_Replicated(t *testing.T) {
	t.Parallel()
	action := DetermineFailoverAction("replicated", false, false)
	if action != FailoverActionEvacuate {
		t.Fatalf("DetermineFailoverAction() = %s, want evacuate", action)
	}
}

func TestDetermineFailoverAction_VerifiedBackup(t *testing.T) {
	t.Parallel()
	action := DetermineFailoverAction("local_only", true, false)
	if action != FailoverActionEvacuate {
		t.Fatalf("DetermineFailoverAction() = %s, want evacuate", action)
	}
}

func TestDetermineFailoverAction_LocalOnlyNoBackup(t *testing.T) {
	t.Parallel()
	action := DetermineFailoverAction("local_only", false, false)
	if action != FailoverActionNotify {
		t.Fatalf("DetermineFailoverAction() = %s, want notify", action)
	}
}

func TestDetermineFailoverAction_Unknown(t *testing.T) {
	t.Parallel()
	action := DetermineFailoverAction("", false, false)
	if action != FailoverActionNotify {
		t.Fatalf("DetermineFailoverAction() = %s, want notify", action)
	}
}

func TestDetermineFailoverAction_LocalOnlyReplicated(t *testing.T) {
	t.Parallel()
	action := DetermineFailoverAction("local_only", false, true)
	if action != FailoverActionNotify {
		t.Fatalf("DetermineFailoverAction() = %s, want notify for local_only even if replicated", action)
	}
}

func TestApplyPolicyDefaults_Nil(t *testing.T) {
	t.Parallel()
	applyPolicyDefaults(nil)
}

func TestApplyPolicyDefaults_Empty(t *testing.T) {
	t.Parallel()
	p := &Policy{}
	applyPolicyDefaults(p)
	if p.MaxFailures != 3 {
		t.Fatalf("MaxFailures = %d, want 3", p.MaxFailures)
	}
	if p.FailureWindowSec != 300 {
		t.Fatalf("FailureWindowSec = %d, want 300", p.FailureWindowSec)
	}
	if p.CooldownSec != 600 {
		t.Fatalf("CooldownSec = %d, want 600", p.CooldownSec)
	}
	if p.Action != FailoverActionEvacuate {
		t.Fatalf("Action = %s, want evacuate", p.Action)
	}
}

func TestApplyPolicyDefaults_Partial(t *testing.T) {
	t.Parallel()
	p := &Policy{MaxFailures: 5, CooldownSec: 100}
	applyPolicyDefaults(p)
	if p.MaxFailures != 5 {
		t.Fatalf("MaxFailures = %d, want 5", p.MaxFailures)
	}
	if p.FailureWindowSec != 300 {
		t.Fatalf("FailureWindowSec = %d, want 300", p.FailureWindowSec)
	}
	if p.CooldownSec != 100 {
		t.Fatalf("CooldownSec = %d, want 100", p.CooldownSec)
	}
}

func TestApplyPolicyDefaults_Full(t *testing.T) {
	t.Parallel()
	p := &Policy{MaxFailures: 1, FailureWindowSec: 60, CooldownSec: 30, Action: FailoverActionRestart}
	applyPolicyDefaults(p)
	if p.MaxFailures != 1 || p.FailureWindowSec != 60 || p.CooldownSec != 30 || p.Action != FailoverActionRestart {
		t.Fatalf("applyPolicyDefaults changed non-zero fields: %+v", p)
	}
}

func TestValidatePolicy_Nil(t *testing.T) {
	t.Parallel()
	err := validatePolicy(nil)
	if err == nil {
		t.Fatal("validatePolicy(nil) expected error")
	}
}

func TestValidatePolicy_EmptyID(t *testing.T) {
	t.Parallel()
	err := validatePolicy(&Policy{NodeID: "node-1"})
	if err == nil {
		t.Fatal("validatePolicy() expected error for empty ID")
	}
}

func TestValidatePolicy_EmptyNodeID(t *testing.T) {
	t.Parallel()
	err := validatePolicy(&Policy{ID: "policy-1"})
	if err == nil {
		t.Fatal("validatePolicy() expected error for empty nodeID")
	}
}

func TestValidatePolicy_Valid(t *testing.T) {
	t.Parallel()
	err := validatePolicy(&Policy{ID: "policy-1", NodeID: "node-1", Action: FailoverActionEvacuate})
	if err != nil {
		t.Fatalf("validatePolicy() unexpected error: %v", err)
	}
}

func TestValidatePolicy_ValidEmptyAction(t *testing.T) {
	t.Parallel()
	err := validatePolicy(&Policy{ID: "policy-1", NodeID: "node-1", Action: ""})
	if err != nil {
		t.Fatalf("validatePolicy() expected empty action to be valid: %v", err)
	}
}

func TestValidatePolicy_InvalidAction(t *testing.T) {
	t.Parallel()
	err := validatePolicy(&Policy{ID: "policy-1", NodeID: "node-1", Action: "invalid"})
	if err == nil {
		t.Fatal("validatePolicy() expected error for invalid action")
	}
}

func TestValidatePolicy_NegativeMaxFailures(t *testing.T) {
	t.Parallel()
	err := validatePolicy(&Policy{ID: "p1", NodeID: "n1", MaxFailures: -1})
	if err == nil {
		t.Fatal("validatePolicy() expected error for negative MaxFailures")
	}
}

func TestValidatePolicy_NegativeFailureWindow(t *testing.T) {
	t.Parallel()
	err := validatePolicy(&Policy{ID: "p1", NodeID: "n1", FailureWindowSec: -1})
	if err == nil {
		t.Fatal("validatePolicy() expected error for negative FailureWindowSec")
	}
}

func TestValidatePolicy_NegativeCooldown(t *testing.T) {
	t.Parallel()
	err := validatePolicy(&Policy{ID: "p1", NodeID: "n1", CooldownSec: -1})
	if err == nil {
		t.Fatal("validatePolicy() expected error for negative CooldownSec")
	}
}

func TestToStorePolicy(t *testing.T) {
	t.Parallel()
	p := &Policy{
		ID: "p1", Name: "test", NodeID: "n1", Enabled: true,
		MaxFailures: 3, FailureWindowSec: 300, CooldownSec: 600,
		Action: FailoverActionEvacuate, HealthCheckPath: "/health", HealthCheckPort: 8080,
	}
	sp := toStorePolicy(p)
	if sp.ID != p.ID || sp.Name != p.Name || sp.NodeID != p.NodeID || sp.Enabled != p.Enabled {
		t.Fatalf("toStorePolicy mismatch: %+v", sp)
	}
	if sp.Action != string(p.Action) {
		t.Fatalf("toStorePolicy action = %s, want %s", sp.Action, p.Action)
	}
}

func TestToStorePolicy_ZeroValues(t *testing.T) {
	t.Parallel()
	p := &Policy{}
	sp := toStorePolicy(p)
	if sp.Action != "" {
		t.Fatal("toStorePolicy should not change empty action")
	}
}

func TestFromStorePolicy(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	sp := store.FailoverPolicy{
		ID: "p1", Name: "test", NodeID: "n1", Enabled: true,
		MaxFailures: 3, FailureWindowSec: 300, CooldownSec: 600,
		Action: "evacuate", CreatedAt: now, UpdatedAt: now,
	}
	p := fromStorePolicy(sp)
	if p.ID != sp.ID || p.Name != sp.Name || p.Action != FailoverActionEvacuate {
		t.Fatalf("fromStorePolicy mismatch: %+v", p)
	}
	if p.CreatedAt != now || p.UpdatedAt != now {
		t.Fatal("fromStorePolicy timestamp mismatch")
	}
}

func TestToStoreEvent(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	e := &Event{
		PolicyID: "p1", NodeID: "n1", ServerID: "s1",
		EventType: EventNodeFailure, Action: FailoverActionEvacuate,
		Status: "detected", Message: "node failed", Timestamp: now,
	}
	se := toStoreEvent(e)
	if se.PolicyID != e.PolicyID || se.NodeID != e.NodeID || se.ServerID != e.ServerID {
		t.Fatalf("toStoreEvent mismatch: %+v", se)
	}
	if se.EventType != string(e.EventType) || se.Action != string(e.Action) {
		t.Fatalf("toStoreEvent action/event mismatch")
	}
}

func TestToStoreEvent_EmptyServerID(t *testing.T) {
	t.Parallel()
	e := &Event{PolicyID: "p1", NodeID: "n1"}
	se := toStoreEvent(e)
	if se.ServerID != "" {
		t.Fatal("toStoreEvent should preserve empty ServerID")
	}
}

func TestMetrics_InitiallyZero(t *testing.T) {
	t.Parallel()
	s := New(nil)
	m := s.Metrics()
	if m.FailuresDetected != 0 || m.EvacuationsTriggered != 0 || m.RestartsTriggered != 0 || m.NotificationsSent != 0 {
		t.Fatalf("Metrics() = %+v, want all zero", m)
	}
}

func TestHandle_IgnoredEventTypes(t *testing.T) {
	t.Parallel()
	s := New(nil)
	env := events.NewEnvelope(events.EventNodeOnline, "test", "node", "n1", nil)
	err := s.Handle(context.Background(), env)
	if err != nil {
		t.Fatalf("Handle() for non-offline event: %v", err)
	}
}

func TestHandleNodeOffline_EmptyNodeID(t *testing.T) {
	t.Parallel()
	s := New(nil)
	err := s.HandleNodeOffline(context.Background(), "", nil)
	if err == nil {
		t.Fatal("HandleNodeOffline() expected error for empty nodeID")
	}
}

func TestRecordFailure_EmptyNodeID(t *testing.T) {
	t.Parallel()
	s := New(nil)
	_, err := s.RecordFailure(context.Background(), "")
	if err == nil {
		t.Fatal("RecordFailure() expected error for empty nodeID")
	}
}

func TestStop_NotStarted(t *testing.T) {
	t.Parallel()
	s := New(nil)
	s.Stop()
}

func TestStart_ThenStop(t *testing.T) {
	t.Parallel()
	s := New(nil)
	ctx := context.Background()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start() unexpected error: %v", err)
	}
	s.Stop()
}

func TestStart_DoubleStart(t *testing.T) {
	t.Parallel()
	s := New(nil)
	ctx := context.Background()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(ctx); err != nil {
		t.Fatal("second Start() should return nil")
	}
	s.Stop()
}

func TestSetActionExecutor(t *testing.T) {
	t.Parallel()
	s := New(nil)
	s.SetActionExecutor(func(ctx context.Context, e *Event) error {
		return nil
	})
	if s.executor == nil {
		t.Fatal("SetActionExecutor() did not set executor")
	}
}

func TestSetWorkloadClassifier(t *testing.T) {
	t.Parallel()
	s := New(nil)
	s.SetWorkloadClassifier(func(ctx context.Context, nodeID string) (FailoverAction, error) {
		return FailoverActionEvacuate, nil
	})
	if s.workloadClassifier == nil {
		t.Fatal("SetWorkloadClassifier() did not set classifier")
	}
}
