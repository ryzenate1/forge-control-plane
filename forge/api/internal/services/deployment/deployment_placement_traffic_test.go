package deployment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"gamepanel/forge/internal/store"
)

// stubTraffic is a test double for TrafficExecutor.
type stubTraffic struct {
	shiftErr   error
	shiftCalls int
	verifyOK   bool
	verifyErr  error
}

func (s *stubTraffic) ShiftTraffic(_ context.Context, _, _, _, _ string, _ int) error {
	s.shiftCalls++
	return s.shiftErr
}

func (s *stubTraffic) VerifyTrafficShift(_ context.Context, _ string) (bool, error) {
	return s.verifyOK, s.verifyErr
}

// TestPromoteRequiresTrafficWhenDemanded proves that turning on
// FORGE_DEPLOY_REQUIRE_TRAFFIC makes an unwired gateway a hard failure rather
// than a promotion that quietly moves nothing.
func TestPromoteRequiresTrafficWhenDemanded(t *testing.T) {
	t.Setenv("FORGE_DEPLOY_REQUIRE_TRAFFIC", "true")
	if !isTrafficRequired() {
		t.Fatal("FORGE_DEPLOY_REQUIRE_TRAFFIC=true must require traffic")
	}

	s := New(nil)
	d := &Deployment{ID: "d-promote-strict", ServerID: "srv-1", ActiveTarget: "blue"}
	err := s.executePromoteStep(context.Background(), d)
	if !errors.Is(err, ErrNoTrafficExecutor) {
		t.Fatalf("promote must fail closed without a traffic executor, got %v", err)
	}
	if d.ActiveTarget != "blue" {
		t.Errorf("active target must not move when the shift never happened, got %q", d.ActiveTarget)
	}
}

// TestPromoteAbortsOnFailedShift proves the recorded active target does not
// move when the gateway rejects or cannot confirm the shift.
func TestPromoteAbortsOnFailedShift(t *testing.T) {
	for _, tc := range []struct {
		name    string
		traffic *stubTraffic
	}{
		{"shift rejected", &stubTraffic{shiftErr: errors.New("gateway refused")}},
		{"shift unconfirmed", &stubTraffic{verifyOK: false}},
		{"verify errored", &stubTraffic{verifyErr: errors.New("gateway unreachable")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(nil)
			s.SetTrafficExecutor(tc.traffic)
			d := &Deployment{ID: "d-promote", ServerID: "srv-1", ActiveTarget: "blue"}

			if err := s.executePromoteStep(context.Background(), d); err == nil {
				t.Fatal("promote must fail when traffic did not verifiably move")
			}
			if d.ActiveTarget != "blue" {
				t.Errorf("active target must not move, got %q", d.ActiveTarget)
			}
		})
	}
}

// TestHealthGateFailsWhenNodeUnresolved is the regression guard for a health
// gate that used to default to localhost, reporting the control plane's own
// machine as if it were the workload.
func TestHealthGateFailsWhenNodeUnresolved(t *testing.T) {
	s := New(nil)
	d := &Deployment{
		ID:              "hg-node-derived",
		ServerID:        "srv-missing",
		HealthCheckPath: "/healthz",
		HealthCheckPort: 8080,
	}

	result, err := s.CheckHealth(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed {
		t.Fatal("gate must fail when the target node cannot be resolved (was: silently probed localhost)")
	}
	if !strings.Contains(result.Error, "unresolved") {
		t.Errorf("expected an unresolved-target error, got %q", result.Error)
	}
}

// TestHealthGateProbesExplicitHost verifies an explicitly configured host is
// probed directly.
func TestHealthGateProbesExplicitHost(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("healthy"))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	hostPort := strings.TrimPrefix(ts.URL, "http://")
	host, portStr, found := strings.Cut(hostPort, ":")
	if !found {
		t.Fatalf("unexpected test server URL %s", ts.URL)
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		t.Fatalf("parse test server port: %v", err)
	}

	s := New(nil)
	d := &Deployment{
		ID:              "hg-direct",
		ServerID:        "srv-ignored",
		HealthCheckPath: "/healthz",
		HealthCheckPort: port,
		HealthCheckHost: host,
	}

	result, err := s.CheckHealth(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed {
		t.Fatalf("direct health probe should pass via an explicit host, got %+v", result)
	}
	if result.Status != http.StatusOK {
		t.Errorf("expected 200, got %d", result.Status)
	}
}

// TestConcurrentRollback409 verifies two concurrent rollbacks resolve to one
// success and one version conflict.
func TestConcurrentRollback409(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping concurrent rollback 409 test")
	}
	svc, cleanup := testService(t)
	defer cleanup()
	ctx := context.Background()

	dep := createTestPendingDeployment(t, svc, "test-server-rollback-race", StrategyRecreate)
	if _, err := svc.CreateRevision(ctx, dep.ID, &RevisionConfig{
		ImageRef:    "nginx:1.25@sha256:abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
		Description: "v1",
	}); err != nil {
		t.Fatal(err)
	}
	rev2, err := svc.CreateRevision(ctx, dep.ID, &RevisionConfig{
		ImageRef:    "nginx:1.26@sha256:def456abc123def456abc123def456abc123def456abc123def456abc123def456",
		Description: "v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.store.UpdateDeploymentCurrentRevision(ctx, dep.ID, &rev2.ID); err != nil {
		t.Fatal(err)
	}

	sd, _ := svc.store.GetDeployment(ctx, dep.ID)
	_ = svc.store.UpdateDeploymentStatusVersioned(ctx, dep.ID, sd.Version, string(StatusCompleted), "")

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = svc.RollbackToPrevious(ctx, dep.ID)
		}(i)
	}
	wg.Wait()

	success, conflict := 0, 0
	for _, e := range errs {
		switch {
		case e == nil:
			success++
		case errors.Is(e, store.ErrVersionConflict):
			conflict++
		default:
			t.Logf("rollback error (non-conflict): %v", e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("expected one success and one conflict, got success=%d conflict=%d errs=%v", success, conflict, errs)
	}
}

// TestProvisionFailurePropagates verifies a runtime failure aborts the DAG and
// leaves both the deployment and the provision step marked failed.
func TestProvisionFailurePropagates(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping provision failure propagation test")
	}
	svc, cleanup := testService(t)
	defer cleanup()

	dep := createTestPendingDeployment(t, svc, "test-server-provision-fail", StrategyRecreate)
	svc.SetRuntimeExecutor(&stubExecutor{applyErr: errors.New("node offline: refused")})

	err := svc.ExecuteDeployment(context.Background(), dep.ID)
	if err == nil || !strings.Contains(err.Error(), "node offline") {
		t.Fatalf("expected provision failure with node offline, got %v", err)
	}

	updated, _ := svc.GetDeployment(context.Background(), dep.ID)
	if updated.Status != StatusFailed {
		t.Fatalf("deployment should be failed, got %s", updated.Status)
	}
	steps, _ := svc.ListSteps(context.Background(), dep.ID)
	for _, st := range steps {
		if st.StepName != StepProvision {
			continue
		}
		if st.Status != StepStatusFailed {
			t.Errorf("provision step should be failed, got %s", st.Status)
		}
		if !strings.Contains(st.Error, "node offline") {
			t.Errorf("step error should mention node offline, got %q", st.Error)
		}
	}
}
