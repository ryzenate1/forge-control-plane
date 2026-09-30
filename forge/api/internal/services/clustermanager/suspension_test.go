package clustermanager

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gamepanel/forge/internal/store"
)

func suspensionFixture(t *testing.T) (*mockClusterStore, *mockRuntime, *Service) {
	t.Helper()
	st := &mockClusterStore{
		serverControlTarget: store.ServerControlTarget{ServerID: "server-1", NodeURL: "http://node:8080"},
	}
	rt := &mockRuntime{}
	return st, rt, newService(st, rt, nil, nil)
}

// A suspend that cannot stop the workload must not leave the server flagged as
// suspended. The flag is rolled back and the error surfaced.
func TestSuspendServer_StopFailsRollsBackFlag(t *testing.T) {
	st, rt, svc := suspensionFixture(t)
	stopErr := errors.New("beacon unreachable")
	rt.stopServerErr = stopErr

	err := svc.SuspendServer(context.Background(), "server-1", nil)
	if err == nil {
		t.Fatal("expected error when runtime stop fails")
	}
	if !errors.Is(err, stopErr) {
		t.Fatalf("error %v must wrap the stop failure %v", err, stopErr)
	}
	if len(st.suspendedWrites) != 2 || st.suspendedWrites[0] != true || st.suspendedWrites[1] != false {
		t.Fatalf("expected suspend write then rollback write, got %v", st.suspendedWrites)
	}
}

// A successful suspend writes the flag once and stops the container once.
func TestSuspendServer_SuccessStopsWorkload(t *testing.T) {
	st, rt, svc := suspensionFixture(t)

	if err := svc.SuspendServer(context.Background(), "server-1", nil); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if len(st.suspendedWrites) != 1 || st.suspendedWrites[0] != true {
		t.Fatalf("expected exactly one suspended=true write, got %v", st.suspendedWrites)
	}
	if rt.stopServerCalls != 1 {
		t.Fatalf("expected one runtime stop, got %d", rt.stopServerCalls)
	}
}

// Unsuspending clears the flag but does not start anything: starting is an
// explicit power decision made after un-suspension.
func TestUnsuspendServer_ClearsFlagWithoutStarting(t *testing.T) {
	st, _, svc := suspensionFixture(t)
	st.getServerResult = store.Server{ID: "server-1", Suspended: true}

	if err := svc.UnsuspendServer(context.Background(), "server-1", nil); err != nil {
		t.Fatalf("unsuspend: %v", err)
	}
	if len(st.suspendedWrites) != 1 || st.suspendedWrites[0] != false {
		t.Fatalf("expected one suspended=false write, got %v", st.suspendedWrites)
	}
}

// Suspending an already-suspended server is a no-op, not a second stop that
// would fail on an already-stopped workload and roll the flag back.
func TestSuspendServer_AlreadySuspendedIsNoOp(t *testing.T) {
	st, rt, svc := suspensionFixture(t)
	st.getServerResult = store.Server{ID: "server-1", Suspended: true}

	if err := svc.SuspendServer(context.Background(), "server-1", nil); err != nil {
		t.Fatalf("suspending a suspended server: %v", err)
	}
	if len(st.suspendedWrites) != 0 {
		t.Fatalf("expected no flag writes, got %v", st.suspendedWrites)
	}
	if rt.stopServerCalls != 0 {
		t.Fatalf("expected no runtime stop, got %d", rt.stopServerCalls)
	}
}

// Changing suspension while a transfer owns the workload would race with the
// migration's own stop/start sequence; both directions refuse.
func TestSetSuspension_TransferInProgressRejected(t *testing.T) {
	for name, run := range map[string]func(svc *Service) error{
		"suspend":   func(svc *Service) error { return svc.SuspendServer(context.Background(), "server-1", nil) },
		"unsuspend": func(svc *Service) error { return svc.UnsuspendServer(context.Background(), "server-1", nil) },
	} {
		t.Run(name, func(t *testing.T) {
			st, rt, svc := suspensionFixture(t)
			st.getServerResult = store.Server{ID: "server-1", TransferState: "running"}

			err := run(svc)
			if err == nil || !strings.Contains(err.Error(), "transfer") {
				t.Fatalf("expected transfer conflict error, got %v", err)
			}
			if len(st.suspendedWrites) != 0 || rt.stopServerCalls != 0 {
				t.Fatalf("rejected %s must not touch state or runtime (writes=%v stops=%d)", name, st.suspendedWrites, rt.stopServerCalls)
			}
		})
	}
}

// The power path enforces the suspension guard before recording desired state,
// so a rejected start leaves the control-plane state untouched.
func TestRequestServerPower_SuspendedStartRejectedBeforeDesiredState(t *testing.T) {
	st, _, svc := suspensionFixture(t)
	st.getServerResult = store.Server{ID: "server-1", Suspended: true}

	_, _, err := svc.RequestServerPower(context.Background(), "server-1", "start")
	if err == nil || !strings.Contains(err.Error(), "suspended") {
		t.Fatalf("expected suspended-server rejection, got %v", err)
	}
}
