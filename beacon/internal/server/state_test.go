package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"gamepanel/beacon/internal/runtime"
)

// fakeLifecycleRuntime answers Inspect with a fixed state. Stats mirrors the
// real runtimes: an observable container streams metrics, anything else fails,
// which is exactly why stats alone could never distinguish stopped from
// missing before the lifecycle fields existed.
type fakeLifecycleRuntime struct {
	runtime.Runtime
	state    runtime.ContainerState
	statsErr error
}

func (f *fakeLifecycleRuntime) Inspect(context.Context, string) (runtime.ContainerState, error) {
	return f.state, nil
}

func (f *fakeLifecycleRuntime) Stats(_ context.Context, _ string) (runtime.Stats, error) {
	if f.statsErr != nil {
		return runtime.Stats{}, f.statsErr
	}
	if f.state.Exists && f.state.Running {
		return runtime.Stats{CPUPercent: 1}, nil
	}
	return runtime.Stats{}, errors.New("container is not running")
}

func TestStateEndpointReportsLifecycle(t *testing.T) {
	rt := &fakeLifecycleRuntime{state: runtime.ContainerState{
		ServerID: "srv-1", ID: "abc", Exists: true, Running: false, Status: "exited",
	}}
	srv := &Server{runtime: rt}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/servers/srv-1/state", nil)
	req.SetPathValue("id", "srv-1")
	srv.state(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["exists"] != true || body["running"] != false || body["status"] != "exited" {
		t.Fatalf("unexpected body: %v", body)
	}
}

// The core truth this endpoint family exists for: a stopped container must be
// distinguishable from a missing one, and from a running one. Under stats-only
// observation all three collapsed into "stats call failed".
func TestStatsEndpointDistinguishesStoppedFromMissing(t *testing.T) {
	cases := []struct {
		name        string
		state       runtime.ContainerState
		wantExists  bool
		wantRunning bool
		wantStatus  string
	}{
		{"stopped", runtime.ContainerState{ServerID: "s", Exists: true, Status: "exited"}, true, false, "exited"},
		{"missing", runtime.ContainerState{ServerID: "s", Exists: false}, false, false, ""},
		{"running", runtime.ContainerState{ServerID: "s", Exists: true, Running: true, Status: "running"}, true, true, "running"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &fakeLifecycleRuntime{state: tc.state}
			srv := &Server{runtime: rt}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/servers/s/stats", nil)
			req.SetPathValue("id", "s")
			srv.stats(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["exists"] != tc.wantExists || body["running"] != tc.wantRunning || body["status"] != tc.wantStatus {
				t.Fatalf("lifecycle fields = %v/%v/%v, want %v/%v/%v",
					body["exists"], body["running"], body["status"], tc.wantExists, tc.wantRunning, tc.wantStatus)
			}
		})
	}
}

// A running workload whose metrics cannot be read is an error, not zero
// telemetry reported as health.
func TestStatsEndpointErrorsWhenRunningMetricsFail(t *testing.T) {
	rt := &fakeLifecycleRuntime{
		state:    runtime.ContainerState{ServerID: "s", Exists: true, Running: true, Status: "running"},
		statsErr: errors.New("metrics pipe broken"),
	}
	srv := &Server{runtime: rt}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/servers/s/stats", nil)
	req.SetPathValue("id", "s")
	srv.stats(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("running workload with failing stats must not answer 200: %s", rec.Body.String())
	}
}
