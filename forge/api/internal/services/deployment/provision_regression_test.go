package deployment

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Regression tests for Phase-1 P0 findings:
//   - FORGE-LOGIC-001 / REF-APP-C02: ExecuteDeployment stub steps reported
//     `completed` with no runtime work.
//   - Provision must fail closed when no runtime executor is wired, and must
//     propagate executor failures instead of advancing the DAG.

func TestProvisionFailsClosedWithoutExecutor(t *testing.T) {
	s := New(nil) // no store needed for this step; no runtime wired
	d := &Deployment{ID: "d1", ServerID: "srv-1", Image: "nginx:1.27"}

	err := s.executeProvisionStep(context.Background(), d)
	if err == nil {
		t.Fatal("provision must refuse to succeed when no runtime executor is wired")
	}
	if !strings.Contains(err.Error(), "refusing to report success") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProvisionPropagatesExecutorFailure(t *testing.T) {
	s := New(nil)
	s.SetRuntimeExecutor(&stubExecutor{
		applyErr: errors.New("node offline"),
	})
	d := &Deployment{ID: "d2", ServerID: "srv-2", Image: "nginx:1.27"}

	err := s.executeProvisionStep(context.Background(), d)
	if err == nil || !strings.Contains(err.Error(), "node offline") {
		t.Fatalf("executor failure must abort the step, got %v", err)
	}
}

func TestProvisionSucceedsOnlyOnExecutorSuccess(t *testing.T) {
	s := New(nil)
	applied := false
	s.SetRuntimeExecutor(&stubExecutor{
		applyFn: func(ctx context.Context, serverID, image string) error {
			if serverID != "srv-3" || image != "app:v2" {
				t.Errorf("executor received wrong target: %s %s", serverID, image)
			}
			applied = true
			return nil
		},
	})
	d := &Deployment{ID: "d3", ServerID: "srv-3", Image: "app:v2"}

	if err := s.executeProvisionStep(context.Background(), d); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !applied {
		t.Error("executor was never invoked")
	}
}

func TestVerifyStepsFailWhenWorkloadNotRunning(t *testing.T) {
	s := New(nil)
	s.SetRuntimeExecutor(&stubExecutor{running: false})
	d := &Deployment{ID: "d4", ServerID: "srv-4"}

	for _, step := range []struct {
		name string
		fn   func(ctx context.Context, d *Deployment) error
	}{
		{"scale_up", s.executeScaleUpStep},
		{"scale_down", s.executeScaleDownStep},
		{"drain_old", s.executeDrainOldStep},
		{"drain_canary", s.executeDrainCanaryStep},
	} {
		if err := step.fn(context.Background(), d); err == nil {
			t.Errorf("%s must fail when workload is not verifiably running", step.name)
		}
	}
}

func TestVerifyStepsPassWhenRunning(t *testing.T) {
	s := New(nil)
	s.SetRuntimeExecutor(&stubExecutor{running: true})
	d := &Deployment{ID: "d5", ServerID: "srv-5"}

	if err := s.executeScaleUpStep(context.Background(), d); err != nil {
		t.Errorf("scale_up should pass with running workload: %v", err)
	}
}

type stubExecutor struct {
	applyErr  error
	applyFn   func(ctx context.Context, serverID, image string) error
	running   bool
	verifyErr error
}

func (e *stubExecutor) ApplyDeployment(ctx context.Context, serverID, image string) error {
	if e.applyFn != nil {
		return e.applyFn(ctx, serverID, image)
	}
	return e.applyErr
}

func (e *stubExecutor) VerifyRunning(ctx context.Context, serverID string) (bool, error) {
	if e.verifyErr != nil {
		return false, e.verifyErr
	}
	return e.running, nil
}
