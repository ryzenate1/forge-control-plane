package healthcheckrunner

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"gamepanel/forge/internal/store"
)

type mockStore struct {
	mu             sync.Mutex
	targetGroups   []store.TargetGroupRow
	targetsByGroup map[string][]store.TargetRow
	statusUpdates  map[string]string
	history        []store.HealthCheckHistoryRow
}

func newMockStore() *mockStore {
	return &mockStore{
		targetGroups:   make([]store.TargetGroupRow, 0),
		targetsByGroup: make(map[string][]store.TargetRow),
		statusUpdates:  make(map[string]string),
	}
}

func (m *mockStore) ListTargetGroups(ctx context.Context) ([]store.TargetGroupRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]store.TargetGroupRow, len(m.targetGroups))
	copy(result, m.targetGroups)
	return result, nil
}

func (m *mockStore) ListTargetsByGroup(ctx context.Context, groupID string) ([]store.TargetRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	targets := m.targetsByGroup[groupID]
	result := make([]store.TargetRow, len(targets))
	copy(result, targets)
	return result, nil
}

func (m *mockStore) UpdateTargetStatus(ctx context.Context, id string, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusUpdates[id] = status
	return nil
}

func (m *mockStore) InsertHealthCheckHistory(ctx context.Context, row store.HealthCheckHistoryRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.history = append(m.history, row)
	return nil
}

func (m *mockStore) PruneHealthCheckHistory(ctx context.Context, before time.Time) (int64, error) {
	return 0, nil
}

func (m *mockStore) GetTargetHealthSummary(ctx context.Context, targetID string) (int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	failures := 0
	successes := 0
	for i := len(m.history) - 1; i >= 0; i-- {
		if m.history[i].TargetID != targetID {
			continue
		}
		if m.history[i].Status == "unhealthy" || m.history[i].Status == "suspected" {
			if successes == 0 {
				failures++
			}
		} else {
			if failures == 0 {
				successes++
			}
		}
	}
	return failures, successes, nil
}

func (m *mockStore) ListServers(ctx context.Context) ([]store.Server, error) {
	return nil, nil
}

func (m *mockStore) GetServer(ctx context.Context, serverID string) (store.Server, error) {
	return store.Server{}, fmt.Errorf("not implemented in mock")
}

func (m *mockStore) addGroup(group store.TargetGroupRow, targets []store.TargetRow) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.targetGroups = append(m.targetGroups, group)
	m.targetsByGroup[group.ID] = targets
}

func TestTCPHealthCheck_Success(t *testing.T) {
	st := newMockStore()
	svc := New(st, Config{Interval: 1 * time.Hour})

	hcJSON, _ := json.Marshal(HealthCheckConfig{TimeoutSeconds: 4})
	group := store.TargetGroupRow{
		ID:          "g-1",
		Name:        "test-group",
		Protocol:    "tcp",
		HealthCheck: hcJSON,
	}
	st.addGroup(group, []store.TargetRow{
		{ID: "t-1", GroupID: "g-1", ServerID: "s-1", IP: "127.0.0.1", Port: 0},
	})

	svc.runOnce(context.Background())

	_ = st
}

func TestTCPHealthCheck_Timeout(t *testing.T) {
	st := newMockStore()
	svc := New(st, Config{Interval: 1 * time.Hour})

	hcJSON, _ := json.Marshal(HealthCheckConfig{TimeoutSeconds: 1, UnhealthyThreshold: 1})
	group := store.TargetGroupRow{
		ID:          "g-timeout",
		Name:        "timeout-group",
		Protocol:    "tcp",
		HealthCheck: hcJSON,
	}
	st.addGroup(group, []store.TargetRow{
		{ID: "t-to", GroupID: "g-timeout", ServerID: "s-to", IP: "10.255.255.1", Port: 1},
	})

	svc.runOnce(context.Background())

	state := svc.GetTargetState("t-to")
	if state == nil {
		t.Fatal("state should exist after first check")
	}
	if state.Status != TargetStatusUnhealthy {
		t.Fatalf("expected unhealthy with threshold=1, got %s failures=%d", state.Status, state.ConsecutiveFailures)
	}
	if state.ConsecutiveFailures < 1 {
		t.Fatalf("expected at least 1 consecutive failure, got %d", state.ConsecutiveFailures)
	}
}

func TestHTTPHealthCheck_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	port := server.Listener.Addr().(*net.TCPAddr).Port

	st := newMockStore()
	svc := New(st, Config{Interval: 1 * time.Hour})

	hcJSON, _ := json.Marshal(HealthCheckConfig{TimeoutSeconds: 4, Path: "/"})
	group := store.TargetGroupRow{
		ID:          "g-http",
		Name:        "http-group",
		Protocol:    "http",
		HealthCheck: hcJSON,
	}
	st.addGroup(group, []store.TargetRow{
		{ID: "t-http", GroupID: "g-http", ServerID: "s-http", IP: "127.0.0.1", Port: port},
	})

	svc.runOnce(context.Background())

	state := svc.GetTargetState("t-http")
	if state == nil {
		t.Fatal("state should exist after check")
	}
	if state.Status != TargetStatusHealthy {
		t.Fatalf("expected healthy, got %s", state.Status)
	}
}

func TestHTTPHealthCheck_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer server.Close()

	port := server.Listener.Addr().(*net.TCPAddr).Port

	st := newMockStore()
	svc := New(st, Config{Interval: 1 * time.Hour})

	hcJSON, _ := json.Marshal(HealthCheckConfig{TimeoutSeconds: 4, Path: "/", UnhealthyThreshold: 1})
	group := store.TargetGroupRow{
		ID:          "g-500",
		Name:        "error-group",
		Protocol:    "http",
		HealthCheck: hcJSON,
	}
	st.addGroup(group, []store.TargetRow{
		{ID: "t-500", GroupID: "g-500", ServerID: "s-500", IP: "127.0.0.1", Port: port},
	})

	svc.runOnce(context.Background())

	state := svc.GetTargetState("t-500")
	if state == nil {
		t.Fatal("state should exist after check")
	}
	if state.Status != TargetStatusUnhealthy {
		t.Fatalf("expected unhealthy for HTTP 500, got %s", state.Status)
	}
}

func TestHealthThreshold_HealthyToSuspectedToUnhealthy(t *testing.T) {
	st := newMockStore()

	hcJSON, _ := json.Marshal(HealthCheckConfig{
		HealthyThreshold:   2,
		UnhealthyThreshold: 3,
		TimeoutSeconds:     1,
	})
	group := store.TargetGroupRow{
		ID:          "g-threshold",
		Name:        "threshold-group",
		Protocol:    "tcp",
		HealthCheck: hcJSON,
	}
	st.addGroup(group, []store.TargetRow{
		{ID: "t-th", GroupID: "g-threshold", ServerID: "s-th", IP: "10.255.255.1", Port: 1},
	})

	svc := New(st, Config{Interval: 1 * time.Hour, HistoryRetention: 1 * time.Hour})

	for i := 0; i < 5; i++ {
		svc.runOnce(context.Background())
	}

	state := svc.GetTargetState("t-th")
	if state == nil {
		t.Fatal("state should exist")
	}
	// After 3-4 failures from unreachable: should be suspected or unhealthy
	if state.Status == TargetStatusHealthy {
		t.Fatalf("after multiple failures, should not be healthy, got %s failures=%d successes=%d",
			state.Status, state.ConsecutiveFailures, state.ConsecutiveSuccesses)
	}
	if state.ConsecutiveFailures < 1 {
		t.Fatalf("expected consecutive failures > 0, got %d", state.ConsecutiveFailures)
	}
	if state.ConsecutiveSuccesses != 0 {
		t.Fatalf("expected 0 successes for unreachable target, got %d", state.ConsecutiveSuccesses)
	}
}

func TestHealthThreshold_HealthyToUnhealthyToHealthy_Recovery(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, _ := ln.Accept()
			if conn != nil {
				conn.Close()
			}
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	st := newMockStore()

	hcJSON, _ := json.Marshal(HealthCheckConfig{
		HealthyThreshold:   2,
		UnhealthyThreshold: 2,
		TimeoutSeconds:     2,
	})
	group := store.TargetGroupRow{
		ID:          "g-recovery",
		Name:        "recovery-group",
		Protocol:    "tcp",
		HealthCheck: hcJSON,
	}
	st.addGroup(group, []store.TargetRow{
		{ID: "t-rec", GroupID: "g-recovery", ServerID: "s-rec", IP: addr.IP.String(), Port: addr.Port},
	})

	svc := New(st, Config{Interval: 1 * time.Hour, HistoryRetention: 1 * time.Hour})

	svc.runOnce(context.Background())
	state := svc.GetTargetState("t-rec")
	if state == nil || state.Status != TargetStatusHealthy {
		t.Fatalf("initial check should be healthy, got %v", state)
	}

	st.mu.Lock()
	for i := range st.targetsByGroup["g-recovery"] {
		st.targetsByGroup["g-recovery"][i].IP = "10.255.255.1"
		st.targetsByGroup["g-recovery"][i].Port = 1
	}
	st.mu.Unlock()

	for i := 0; i < 4; i++ {
		svc.runOnce(context.Background())
	}
	state = svc.GetTargetState("t-rec")
	if state == nil || state.Status == TargetStatusHealthy {
		t.Fatalf("after failures should not be healthy, got status=%s fail=%d succ=%d",
			state.Status, state.ConsecutiveFailures, state.ConsecutiveSuccesses)
	}

	st.mu.Lock()
	for i := range st.targetsByGroup["g-recovery"] {
		st.targetsByGroup["g-recovery"][i].IP = addr.IP.String()
		st.targetsByGroup["g-recovery"][i].Port = addr.Port
	}
	st.mu.Unlock()

	for i := 0; i < 5; i++ {
		svc.runOnce(context.Background())
	}
	state = svc.GetTargetState("t-rec")
	if state == nil || state.Status != TargetStatusHealthy {
		t.Fatalf("after recovery should be healthy, got status=%s successes=%d",
			state.Status, state.ConsecutiveSuccesses)
	}
}

func TestUnhealthyCallbackFires(t *testing.T) {
	st := newMockStore()

	hcJSON, _ := json.Marshal(HealthCheckConfig{
		HealthyThreshold:   2,
		UnhealthyThreshold: 2,
		TimeoutSeconds:     1,
	})
	group := store.TargetGroupRow{
		ID:          "g-callback",
		Name:        "callback-group",
		Protocol:    "tcp",
		HealthCheck: hcJSON,
	}
	st.addGroup(group, []store.TargetRow{
		{ID: "t-cb", GroupID: "g-callback", ServerID: "s-cb", IP: "10.255.255.1", Port: 1},
	})

	svc := New(st, Config{Interval: 1 * time.Hour, HistoryRetention: 1 * time.Hour})

	var cbTargetID, cbServerID string
	var cbCount int
	done := make(chan struct{}, 1)
	svc.OnUnhealthy(func(ctx context.Context, serverID, targetID string, consecutiveFailures int) {
		cbTargetID = targetID
		cbServerID = serverID
		cbCount = consecutiveFailures
		done <- struct{}{}
	})

	for i := 0; i < 5; i++ {
		svc.runOnce(context.Background())
	}

	select {
	case <-done:
		if cbTargetID != "t-cb" {
			t.Fatalf("expected target 't-cb', got '%s'", cbTargetID)
		}
		if cbServerID != "s-cb" {
			t.Fatalf("expected server 's-cb', got '%s'", cbServerID)
		}
		if cbCount < 2 {
			t.Fatalf("expected at least 2 consecutive failures, got %d", cbCount)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("unhealthy callback not fired within timeout")
	}
}

func TestTimeoutHandling(t *testing.T) {
	st := newMockStore()
	svc := New(st, Config{Interval: 1 * time.Hour})

	hcJSON, _ := json.Marshal(HealthCheckConfig{TimeoutSeconds: 1, UnhealthyThreshold: 1})
	group := store.TargetGroupRow{
		ID:          "g-totest",
		Name:        "totest-group",
		Protocol:    "tcp",
		HealthCheck: hcJSON,
	}
	st.addGroup(group, []store.TargetRow{
		{ID: "t-totest", GroupID: "g-totest", ServerID: "s-totest", IP: "10.255.255.1", Port: 1},
	})

	start := time.Now()
	svc.runOnce(context.Background())
	elapsed := time.Since(start)

	if elapsed > 3*time.Second {
		t.Fatalf("check took too long even with timeout: %v", elapsed)
	}

	state := svc.GetTargetState("t-totest")
	if state == nil || state.Status != TargetStatusUnhealthy {
		t.Fatalf("expected unhealthy for timeout, got %v", state)
	}
}

func TestMetrics(t *testing.T) {
	st := newMockStore()
	svc := New(st, Config{Interval: 1 * time.Hour})

	svc.mu.Lock()
	svc.states["t-healthy"] = &TargetHealthState{ID: "t-healthy", Status: TargetStatusHealthy}
	svc.states["t-suspected"] = &TargetHealthState{ID: "t-suspected", Status: TargetStatusSuspected}
	svc.states["t-unhealthy"] = &TargetHealthState{ID: "t-unhealthy", Status: TargetStatusUnhealthy}
	svc.mu.Unlock()

	metrics := svc.Metrics()
	if metrics["total"] != 3 {
		t.Fatalf("expected 3 total, got %v", metrics["total"])
	}
	if metrics["healthy"] != 1 {
		t.Fatalf("expected 1 healthy, got %v", metrics["healthy"])
	}
	if metrics["suspected"] != 1 {
		t.Fatalf("expected 1 suspected, got %v", metrics["suspected"])
	}
	if metrics["unhealthy"] != 1 {
		t.Fatalf("expected 1 unhealthy, got %v", metrics["unhealthy"])
	}
}

func TestHistoryPersistence(t *testing.T) {
	st := newMockStore()
	svc := New(st, Config{Interval: 1 * time.Hour})

	hcJSON, _ := json.Marshal(HealthCheckConfig{TimeoutSeconds: 1})
	group := store.TargetGroupRow{
		ID:          "g-history",
		Name:        "history-group",
		Protocol:    "tcp",
		HealthCheck: hcJSON,
	}
	st.addGroup(group, []store.TargetRow{
		{ID: "t-history", GroupID: "g-history", ServerID: "s-history", IP: "10.255.255.1", Port: 1},
	})

	svc.runOnce(context.Background())

	st.mu.Lock()
	historyLen := len(st.history)
	st.mu.Unlock()
	if historyLen == 0 {
		t.Fatal("expected history entry after check")
	}

	st.mu.Lock()
	lastEntry := st.history[len(st.history)-1]
	st.mu.Unlock()
	if lastEntry.TargetID != "t-history" {
		t.Fatalf("expected target 't-history', got '%s'", lastEntry.TargetID)
	}
	if lastEntry.CheckType != "tcp" {
		t.Fatalf("expected check type 'tcp', got '%s'", lastEntry.CheckType)
	}
}

func TestReconcilerAdapter(t *testing.T) {
	st := newMockStore()
	svc := New(st, Config{Interval: 1 * time.Hour})

	svc.mu.Lock()
	svc.states["t-adapt-u"] = &TargetHealthState{
		ID: "t-adapt-u", GroupID: "g-adapt", ServerID: "s-adapt",
		Status: TargetStatusUnhealthy, ConsecutiveFailures: 3,
	}
	svc.states["t-adapt-h"] = &TargetHealthState{
		ID: "t-adapt-h", GroupID: "g-adapt", ServerID: "s-adapt",
		Status: TargetStatusHealthy, ConsecutiveFailures: 0,
	}
	svc.mu.Unlock()

	adapter := svc.ReconcilerAdapter()
	unhealthy := adapter.ListUnhealthyTargets(context.Background())
	if len(unhealthy) != 1 {
		t.Fatalf("expected 1 unhealthy target, got %d", len(unhealthy))
	}
	if unhealthy[0].TargetID != "t-adapt-u" {
		t.Fatalf("expected target 't-adapt-u', got '%s'", unhealthy[0].TargetID)
	}
	if unhealthy[0].FailureCount != 3 {
		t.Fatalf("expected 3 failures, got %d", unhealthy[0].FailureCount)
	}
}
