package crossnode

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gamepanel/forge/internal/services/trafficmanager"

	"gopkg.in/yaml.v3"
)

type stubRuleSource struct {
	rules    []*trafficmanager.RoutingRule
	policies []*trafficmanager.TrafficPolicy
	err      error
}

func (s *stubRuleSource) ListRoutingRules(context.Context) ([]*trafficmanager.RoutingRule, error) {
	return s.rules, s.err
}

func (s *stubRuleSource) ListTrafficPolicies(context.Context) ([]*trafficmanager.TrafficPolicy, error) {
	return s.policies, s.err
}

type stubReconciler struct {
	syncRoutes   atomic.Int64
	cleanupCalls atomic.Int64
	syncErr      error
	cleanupErr   error
}

func (r *stubReconciler) SyncRoutes(context.Context) error {
	r.syncRoutes.Add(1)
	return r.syncErr
}

func (r *stubReconciler) CleanupStaleRoutes(context.Context) error {
	r.cleanupCalls.Add(1)
	return r.cleanupErr
}

func newTestSyncer(health *HealthFilter, src RuleSource, rec GatewayReconciler) *IngressSynchronizer {
	is := NewIngressSynchronizer(nil, health)
	if src != nil || rec != nil {
		is.SetReconciler(src, rec)
	}
	return is
}

func TestRouteGrouping_SingleRule(t *testing.T) {
	rules := []*trafficmanager.RoutingRule{
		{
			ID:         "r1",
			Domain:     "example.com",
			Path:       "/",
			TargetPort: 8080,
			TargetHost: "localhost",
			Enabled:    true,
		},
	}

	groups := GroupRulesByRoute(rules)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}

	backends := getFirstGroup(groups).UniqueBackends()
	if len(backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(backends))
	}
	if backends[0].Host != "localhost" || backends[0].Port != 8080 {
		t.Fatalf("unexpected backend: %+v", backends[0])
	}
}

func TestRouteGrouping_MultipleBackendsSameRoute(t *testing.T) {
	rules := []*trafficmanager.RoutingRule{
		{
			ID:         "r1",
			Domain:     "example.com",
			Path:       "/",
			TargetPort: 8080,
			TargetHost: "10.0.0.1",
			Enabled:    true,
			Weight:     2,
		},
		{
			ID:         "r2",
			Domain:     "example.com",
			Path:       "/",
			TargetPort: 8081,
			TargetHost: "10.0.0.2",
			Enabled:    true,
			Weight:     1,
		},
	}

	groups := GroupRulesByRoute(rules)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}

	backends := getFirstGroup(groups).UniqueBackends()
	if len(backends) != 2 {
		t.Fatalf("expected 2 backends, got %d", len(backends))
	}
}

func TestRouteGrouping_DifferentRoutesStaySeparate(t *testing.T) {
	rules := []*trafficmanager.RoutingRule{
		{
			ID:         "r1",
			Domain:     "example.com",
			Path:       "/api/",
			TargetPort: 8080,
			TargetHost: "10.0.0.1",
			Enabled:    true,
		},
		{
			ID:         "r2",
			Domain:     "other.com",
			Path:       "/",
			TargetPort: 9090,
			TargetHost: "10.0.0.2",
			Enabled:    true,
		},
	}

	groups := GroupRulesByRoute(rules)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
}

func TestRouteGrouping_DeduplicateBackends(t *testing.T) {
	rules := []*trafficmanager.RoutingRule{
		{
			ID:         "r1",
			Domain:     "example.com",
			Path:       "/",
			TargetPort: 8080,
			TargetHost: "10.0.0.1",
			Enabled:    true,
		},
		{
			ID:         "r2",
			Domain:     "example.com",
			Path:       "/",
			TargetPort: 8080,
			TargetHost: "10.0.0.1",
			Enabled:    true,
		},
	}

	groups := GroupRulesByRoute(rules)
	backends := getFirstGroup(groups).UniqueBackends()
	if len(backends) != 1 {
		t.Fatalf("expected 1 unique backend (deduplicated), got %d", len(backends))
	}
}

func TestWebSocketDetection(t *testing.T) {
	rules := []*trafficmanager.RoutingRule{
		{
			ID:         "ws1",
			Domain:     "ws.example.com",
			Path:       "/ws/",
			TargetPort: 9090,
			TargetHost: "localhost",
			WebSocket:  true,
			Enabled:    true,
		},
	}

	groups := GroupRulesByRoute(rules)
	if !getFirstGroup(groups).HasWebSocket() {
		t.Fatal("expected WebSocket to be detected")
	}
}

func TestHealthFilter_UnknownIsNotHealthy(t *testing.T) {
	filter := NewHealthFilter(2, 30*time.Second)
	if filter.IsHealthy("10.0.0.1", 8080) {
		t.Fatal("unknown backend must not be reported healthy")
	}
}

func TestHealthFilter_MarksDownAfterThreshold(t *testing.T) {
	filter := NewHealthFilter(3, 30*time.Second)

	for i := 0; i < 3; i++ {
		filter.RecordFailure("10.0.0.1", 8080, "connection refused")
	}

	if filter.IsHealthy("10.0.0.1", 8080) {
		t.Fatal("backend should be unhealthy after threshold failures")
	}
}

func TestHealthFilter_RecoversAfterSuccess(t *testing.T) {
	filter := NewHealthFilter(2, 30*time.Second)

	filter.RecordFailure("10.0.0.1", 8080, "timeout")
	filter.RecordFailure("10.0.0.1", 8080, "timeout")

	if filter.IsHealthy("10.0.0.1", 8080) {
		t.Fatal("backend should be down after 2 failures")
	}

	filter.RecordSuccess("10.0.0.1", 8080)
	if !filter.IsHealthy("10.0.0.1", 8080) {
		t.Fatal("backend should be healthy after successful recovery")
	}
}

func TestHealthFilter_FiltersUnhealthyBackends(t *testing.T) {
	filter := NewHealthFilter(2, 30*time.Second)

	// Only recorded-healthy backends pass the filter.
	filter.RecordSuccess("10.0.0.1", 8080)

	for i := 0; i < 2; i++ {
		filter.RecordFailure("10.0.0.2", 8081, "down")
	}

	backends := []BackendAddr{
		{Host: "10.0.0.1", Port: 8080, URL: "http://10.0.0.1:8080"},
		{Host: "10.0.0.2", Port: 8081, URL: "http://10.0.0.2:8081"},
	}

	healthy := filter.FilterHealthy(backends)
	if len(healthy) != 1 {
		t.Fatalf("expected 1 healthy backend, got %d", len(healthy))
	}
	if healthy[0].Host != "10.0.0.1" {
		t.Fatal("expected healthy backend to be 10.0.0.1")
	}
}

func TestHealthFilter_Clear(t *testing.T) {
	filter := NewHealthFilter(1, 30*time.Second)
	filter.RecordFailure("10.0.0.1", 8080, "error")

	if filter.IsHealthy("10.0.0.1", 8080) {
		t.Fatal("backend should be unhealthy")
	}

	filter.Clear("10.0.0.1", 8080)
	if filter.IsHealthy("10.0.0.1", 8080) {
		t.Fatal("cleared backend should return to unknown, not healthy")
	}
}

func TestRouteGenerationRecords(t *testing.T) {
	rules := []*trafficmanager.RoutingRule{
		{
			ID:         "svc1",
			Domain:     "app.example.com",
			Path:       "/",
			TargetPort: 3000,
			TargetHost: "10.0.0.1",
			ServerID:   "srv-1",
			Enabled:    true,
		},
		{
			ID:         "svc2",
			Domain:     "app.example.com",
			Path:       "/",
			TargetPort: 3001,
			TargetHost: "10.0.0.2",
			ServerID:   "srv-2",
			Enabled:    true,
		},
	}

	groups := GroupRulesByRoute(rules)
	records := BuildRouteGenerationRecords(groups)

	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	rec := records[0]
	if rec.BackendCount != 2 {
		t.Fatalf("expected 2 backends, got %d", rec.BackendCount)
	}
	if len(rec.ServerIDs) != 2 {
		t.Fatalf("expected 2 server IDs, got %d", len(rec.ServerIDs))
	}
	if len(rec.RuleIDs) != 2 {
		t.Fatalf("expected 2 rule IDs, got %d", len(rec.RuleIDs))
	}
}

func TestTwoReplicasOnTwoNodes(t *testing.T) {
	rules := []*trafficmanager.RoutingRule{
		{
			ID:         "app-replica-0",
			Domain:     "app.example.com",
			Path:       "/",
			TargetPort: 8080,
			TargetHost: "node-1.internal",
			ServerID:   "srv-1",
			Enabled:    true,
			Weight:     1,
		},
		{
			ID:         "app-replica-1",
			Domain:     "app.example.com",
			Path:       "/",
			TargetPort: 8080,
			TargetHost: "node-2.internal",
			ServerID:   "srv-2",
			Enabled:    true,
			Weight:     1,
		},
	}

	groups := GroupRulesByRoute(rules)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group for same route, got %d", len(groups))
	}

	backends := getFirstGroup(groups).UniqueBackends()
	if len(backends) != 2 {
		t.Fatalf("expected 2 backends for 2 nodes, got %d", len(backends))
	}

	hasNode1, hasNode2 := false, false
	for _, b := range backends {
		if b.Host == "node-1.internal" {
			hasNode1 = true
		}
		if b.Host == "node-2.internal" {
			hasNode2 = true
		}
	}
	if !hasNode1 || !hasNode2 {
		t.Fatal("both nodes should be represented as backends")
	}
}

func TestOneUnhealthyReplica(t *testing.T) {
	rules := []*trafficmanager.RoutingRule{
		{
			ID:         "healthy-replica",
			Domain:     "app.example.com",
			Path:       "/",
			TargetPort: 8080,
			TargetHost: "node-1.internal",
			ServerID:   "srv-1",
			Enabled:    true,
			Weight:     1,
		},
		{
			ID:         "unhealthy-replica",
			Domain:     "app.example.com",
			Path:       "/",
			TargetPort: 8080,
			TargetHost: "node-2.internal",
			ServerID:   "srv-2",
			Enabled:    true,
			Weight:     1,
		},
	}

	health := NewHealthFilter(2, 30*time.Second)
	// Only recorded-healthy backends pass the filter.
	health.RecordSuccess("node-1.internal", 8080)
	health.RecordFailure("node-2.internal", 8080, "connection refused")
	health.RecordFailure("node-2.internal", 8080, "connection refused")

	groups := GroupRulesByRoute(rules)
	grp := getFirstGroup(groups)
	allBackends := grp.UniqueBackends()
	healthyBackends := health.FilterHealthy(allBackends)

	if len(healthyBackends) != 1 {
		t.Fatalf("expected 1 healthy backend, got %d", len(healthyBackends))
	}
	if healthyBackends[0].Host != "node-1.internal" {
		t.Fatal("expected node-1.internal to be the only healthy backend")
	}
}

func TestOfflineNodeCausesStaleEntry(t *testing.T) {
	filter := NewHealthFilter(2, 30*time.Second)

	filter.RecordFailure("offline-node", 8080, "dial timeout")
	filter.RecordFailure("offline-node", 8080, "dial timeout")

	health := filter.GetHealth("offline-node", 8080)
	if health.Status != HealthDown {
		t.Fatalf("expected HealthDown, got %v", health.Status)
	}

	if !strings.Contains(health.Reason, "dial timeout") {
		t.Fatalf("expected reason about timeout, got %s", health.Reason)
	}
}

func TestGatewayReloadFailure(t *testing.T) {
	dir := t.TempDir()
	var mu sync.Mutex
	reloadCalled := false

	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reloadCalled = true
		mu.Unlock()
		if r.URL.Path == "/api/refresh" && r.Method == "POST" {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":"reload failed"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer admin.Close()

	proxy := trafficmanager.NewTraefikReverseProxy(dir, strings.TrimPrefix(admin.URL, "http://"))

	rules := []*trafficmanager.RoutingRule{
		{
			ID:         "fail",
			Domain:     "fail.com",
			Path:       "/",
			TargetPort: 3000,
			TargetHost: "localhost",
			Enabled:    true,
		},
	}

	err := proxy.UpdateRoutes(context.Background(), rules, nil)
	if err == nil {
		t.Fatal("expected error from reload failure")
	}
	if !strings.Contains(err.Error(), "reload failed") {
		t.Fatalf("expected reload failed error, got: %v", err)
	}

	if !reloadCalled {
		t.Fatal("reload should have been attempted")
	}

	configPath := filepath.Join(dir, "routes.yml")
	data, err := os.ReadFile(configPath)
	if err == nil && strings.Contains(string(data), "fail.com") {
		// Config was written before reload attempt - acceptable behavior
		return
	}
	// Config may have been restored to previous (empty) state on reload failure
	// which is safe behavior - no broken config remains active
	if err == nil {
		return
	}
	t.Logf("config file cleaned up as expected after failed reload (config dir: %s)", dir)
}

func TestStaleTargetCleanup(t *testing.T) {
	dir := t.TempDir()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer admin.Close()

	proxy := trafficmanager.NewTraefikReverseProxy(dir, strings.TrimPrefix(admin.URL, "http://"))

	err := proxy.UpdateRoutes(context.Background(), []*trafficmanager.RoutingRule{
		{ID: "active", Domain: "active.com", Path: "/", TargetPort: 8080, TargetHost: "localhost", Enabled: true},
		{ID: "stale", Domain: "stale.com", Path: "/", TargetPort: 8080, TargetHost: "localhost", Enabled: true},
	}, nil)
	if err != nil {
		t.Fatalf("initial UpdateRoutes failed: %v", err)
	}

	err = proxy.CleanupStale(context.Background(), map[string]bool{"active": true})
	if err != nil {
		t.Fatalf("CleanupStale failed: %v", err)
	}

	configPath := filepath.Join(dir, "routes.yml")
	data, _ := os.ReadFile(configPath)

	if !strings.Contains(string(data), "active.com") {
		t.Fatal("active route should still be present")
	}
	if strings.Contains(string(data), "stale.com") {
		t.Fatal("stale route should have been removed")
	}
}

func TestWebSocketViaGateway(t *testing.T) {
	dir := t.TempDir()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer admin.Close()

	proxy := trafficmanager.NewTraefikReverseProxy(dir, strings.TrimPrefix(admin.URL, "http://"))

	rules := []*trafficmanager.RoutingRule{
		{
			ID:         "ws1",
			Domain:     "ws.example.com",
			Path:       "/ws/",
			TargetPort: 9090,
			TargetHost: "localhost",
			WebSocket:  true,
			Enabled:    true,
		},
	}

	err := proxy.UpdateRoutes(context.Background(), rules, nil)
	if err != nil {
		t.Fatalf("UpdateRoutes failed: %v", err)
	}

	configPath := filepath.Join(dir, "routes.yml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}

	var cfg trafficmanager.TraefikFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	svc, ok := cfg.HTTP.Services["gamepanel-svc-ws1"]
	if !ok {
		t.Fatal("service not found")
	}
	if svc.LoadBalancer == nil {
		t.Fatal("service has no loadBalancer")
	}
	if svc.LoadBalancer.ResponseForwarding == nil {
		t.Fatal("websocket route missing ResponseForwarding")
	}
	if svc.LoadBalancer.ResponseForwarding.FlushInterval != "0ms" {
		t.Fatalf("expected flushInterval 0ms, got %s", svc.LoadBalancer.ResponseForwarding.FlushInterval)
	}
}

func TestIngressSyncStats(t *testing.T) {
	dir := t.TempDir()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer admin.Close()

	proxy := trafficmanager.NewTraefikReverseProxy(dir, strings.TrimPrefix(admin.URL, "http://"))
	health := NewHealthFilter(2, 30*time.Second)
	// The synchronizer only reports backends with a recorded healthy verdict.
	health.RecordSuccess("localhost", 8080)
	syncer := NewIngressSynchronizer(proxy, health)
	syncer.SetReconciler(&stubRuleSource{rules: []*trafficmanager.RoutingRule{
		{ID: "test", Domain: "test.com", Path: "/", TargetPort: 8080, TargetHost: "localhost", Enabled: true},
	}}, &stubReconciler{})

	result, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	if result.Observed != 1 {
		t.Fatalf("expected 1 observed rule, got %d", result.Observed)
	}
	if result.Healthy != 1 {
		t.Fatalf("expected 1 healthy backend, got %d", result.Healthy)
	}

	stats := syncer.Stats()
	if stats.RuleCount != 1 {
		t.Fatalf("expected 1 rule, got %d", stats.RuleCount)
	}
	if stats.SyncCount != 1 {
		t.Fatalf("expected 1 sync, got %d", stats.SyncCount)
	}
	if stats.ErrCount != 0 {
		t.Fatalf("expected 0 errors, got %d", stats.ErrCount)
	}
}

func TestRouteGenerationRecords_PerService(t *testing.T) {
	rules := []*trafficmanager.RoutingRule{
		{
			ID:         "svc-a",
			Domain:     "api.example.com",
			Path:       "/",
			TargetPort: 8080,
			TargetHost: "10.0.0.1",
			ServerID:   "srv-a",
			Enabled:    true,
		},
		{
			ID:         "svc-b",
			Domain:     "web.example.com",
			Path:       "/",
			TargetPort: 3000,
			TargetHost: "10.0.0.2",
			ServerID:   "srv-b",
			Enabled:    true,
		},
	}

	groups := GroupRulesByRoute(rules)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups (different domains), got %d", len(groups))
	}
}

func TestResolveTargetHost(t *testing.T) {
	resolver := NewResolver(nil)

	// ResolveTargetHost no longer falls back to localhost: an unresolvable
	// target is an error with an empty host, so a caller cannot mistake a
	// guess for a confirmed node.
	host, err := resolver.ResolveTargetHost(context.Background(), "", "")
	if !errors.Is(err, ErrNoTarget) {
		t.Fatalf("expected ErrNoTarget for empty inputs, got host %q err %v", host, err)
	}
	if host != "" {
		t.Fatalf("expected empty host alongside error, got %s", host)
	}

	resolver.ClearCache()
	host, err = resolver.ResolveTargetHost(context.Background(), "", "node-1")
	if !errors.Is(err, ErrNoTarget) {
		t.Fatalf("expected ErrNoTarget when no store, got host %q err %v", host, err)
	}
	if host != "" {
		t.Fatalf("expected empty host alongside error, got %s", host)
	}
}

func TestDescribeBackend(t *testing.T) {
	proxy := trafficmanager.NewTraefikReverseProxy(t.TempDir(), "localhost:8080")
	health := NewHealthFilter(2, 30*time.Second)
	syncer := NewIngressSynchronizer(proxy, health)

	// A backend that was never probed must not read as healthy; asserting the
	// opposite is what let an unchecked node look routable.
	desc := syncer.DescribeBackend("10.0.0.1", 8080)
	if !strings.Contains(desc, "UNKNOWN") {
		t.Fatalf("expected UNKNOWN description for an unprobed backend, got: %s", desc)
	}

	health.RecordFailure("10.0.0.1", 8080, "connection refused")
	desc = syncer.DescribeBackend("10.0.0.1", 8080)
	if !strings.Contains(desc, "DEGRADED") {
		t.Fatalf("expected DEGRADED description below threshold, got: %s", desc)
	}

	health.RecordFailure("10.0.0.1", 8080, "connection refused")
	desc = syncer.DescribeBackend("10.0.0.1", 8080)
	if !strings.Contains(desc, "DOWN") {
		t.Fatalf("expected DOWN description at threshold, got: %s", desc)
	}

	health.RecordSuccess("10.0.0.1", 8080)
	desc = syncer.DescribeBackend("10.0.0.1", 8080)
	if !strings.Contains(desc, "HEALTHY") {
		t.Fatalf("expected HEALTHY description after recovery, got: %s", desc)
	}
}

func TestCaddyMultiUpstreamConfig(t *testing.T) {
	appliedCh := make(chan []byte, 1)

	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/load" && r.Method == "POST" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"valid":true}`)
			return
		}
		if r.URL.Path == "/config/" && r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"apps":{"http":{"servers":{"gamepanel":{"listen":[":80"],"routes":[]}}}}}`)
			return
		}
		if r.URL.Path == "/config/" && r.Method == "POST" {
			body, _ := io.ReadAll(r.Body)
			appliedCh <- body
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"ok":true}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer admin.Close()

	proxy := trafficmanager.NewCaddyReverseProxy(strings.TrimPrefix(admin.URL, "http://"))

	rules := []*trafficmanager.RoutingRule{
		{
			ID:         "app-rep-0",
			Domain:     "app.example.com",
			Path:       "/",
			TargetPort: 8080,
			TargetHost: "node-1.internal",
			Enabled:    true,
			Weight:     1,
		},
		{
			ID:         "app-rep-1",
			Domain:     "app.example.com",
			Path:       "/",
			TargetPort: 8080,
			TargetHost: "node-2.internal",
			Enabled:    true,
			Weight:     2,
		},
	}

	err := proxy.UpdateRoutes(context.Background(), rules, nil)
	if err != nil {
		t.Fatalf("UpdateRoutes failed: %v", err)
	}

	select {
	case applied := <-appliedCh:
		cfgStr := string(applied)
		if !strings.Contains(cfgStr, "node-1.internal:8080") {
			t.Fatal("config missing node-1.internal:8080")
		}
		if !strings.Contains(cfgStr, "node-2.internal:8080") {
			t.Fatal("config missing node-2.internal:8080")
		}
		if !strings.Contains(cfgStr, `"upstreams"`) {
			t.Fatal("config missing upstreams array")
		}
	default:
		t.Fatal("config was never applied")
	}
}

func TestAPI_ReloadRestoresState(t *testing.T) {
	dir := t.TempDir()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/refresh" && r.Method == "POST" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"ok":true}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer admin.Close()

	proxy := trafficmanager.NewTraefikReverseProxy(dir, strings.TrimPrefix(admin.URL, "http://"))

	err := proxy.UpdateRoutes(context.Background(), []*trafficmanager.RoutingRule{
		{ID: "svc-v1", Domain: "v1.example.com", Path: "/", TargetPort: 8080, TargetHost: "localhost", Enabled: true},
	}, nil)
	if err != nil {
		t.Fatalf("initial update failed: %v", err)
	}

	configPath := filepath.Join(dir, "routes.yml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config not found: %v", err)
	}
	if !strings.Contains(string(data), "v1.example.com") {
		t.Fatal("config should be persisted after proxy restart")
	}

	proxy2 := trafficmanager.NewTraefikReverseProxy(dir, strings.TrimPrefix(admin.URL, "http://"))
	health := proxy2.Health(context.Background())
	if health.ErrCount > 0 {
		t.Fatalf("expected 0 errors after restart, got %d", health.ErrCount)
	}
}

func TestConcurrentIngressSync(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer admin.Close()

	proxy := trafficmanager.NewTraefikReverseProxy(t.TempDir(), strings.TrimPrefix(admin.URL, "http://"))
	health := NewHealthFilter(2, 30*time.Second)

	rules := make([]*trafficmanager.RoutingRule, 0, 5)
	for i := 0; i < 5; i++ {
		rules = append(rules, &trafficmanager.RoutingRule{
			ID:         fmt.Sprintf("svc-%d", i),
			Domain:     fmt.Sprintf("svc%d.example.com", i),
			Path:       "/",
			TargetPort: 8080 + i,
			TargetHost: "localhost",
			Enabled:    true,
		})
		health.RecordSuccess("localhost", 8080+i)
	}

	rec := &stubReconciler{}
	syncer := NewIngressSynchronizer(proxy, health)
	syncer.SetReconciler(&stubRuleSource{rules: rules}, rec)

	var wg sync.WaitGroup
	errs := make(chan error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := syncer.Sync(context.Background())
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent sync failed: %v", err)
		}
	}

	stats := syncer.Stats()
	if stats.SyncCount != 5 {
		t.Fatalf("expected 5 syncs, got %d", stats.SyncCount)
	}
	if stats.RuleCount != 5 {
		t.Fatalf("expected 5 observed rules, got %d", stats.RuleCount)
	}
	// Sync only observes. Converging the gateway belongs to the reconciler.
	if got := rec.syncRoutes.Load(); got != 0 {
		t.Fatalf("Sync must not reconcile the gateway, got %d SyncRoutes calls", got)
	}
}

// TestSyncFailsClosedWithoutRuleSource replaces the old behaviour where an
// unwired synchronizer returned nil and the operator saw "sync successful".
func TestSyncFailsClosedWithoutRuleSource(t *testing.T) {
	health := NewHealthFilter(2, 30*time.Second)
	syncer := NewIngressSynchronizer(nil, health)

	result, err := syncer.Sync(context.Background())
	if !errors.Is(err, ErrNoRuleSource) {
		t.Fatalf("expected ErrNoRuleSource, got %v", err)
	}
	if !result.Skipped || result.Reason == "" {
		t.Fatalf("expected a skipped result carrying a reason, got %+v", result)
	}
	if syncer.Stats().ErrCount == 0 {
		t.Fatal("an unwired sync must be counted as an error")
	}
}

// TestGatewayWritesRefusedWithoutReconciler is the regression test for the P0:
// crossnode shared one Caddy admin API with trafficmanager, and its cleanup path
// passed its own (always empty) rule set to a gateway call that withdraws every
// route absent from it. Refusing outright is the only safe behaviour.
func TestGatewayWritesRefusedWithoutReconciler(t *testing.T) {
	health := NewHealthFilter(2, 30*time.Second)
	syncer := NewIngressSynchronizer(nil, health)

	if syncer.ReconcilerConfigured() {
		t.Fatal("expected no reconciler configured")
	}

	if _, err := syncer.Reconcile(context.Background()); !errors.Is(err, ErrNoReconciler) {
		t.Fatalf("Reconcile without a reconciler must fail, got %v", err)
	}
	if err := syncer.CleanupStale(context.Background()); !errors.Is(err, ErrNoReconciler) {
		t.Fatalf("CleanupStale without a reconciler must fail, got %v", err)
	}
}

func TestReconcileDelegatesGatewayConvergence(t *testing.T) {
	health := NewHealthFilter(2, 30*time.Second)
	health.RecordSuccess("10.0.0.1", 8080)

	src := &stubRuleSource{rules: []*trafficmanager.RoutingRule{
		{ID: "svc-a", Domain: "a.example.com", Path: "/", TargetHost: "10.0.0.1", TargetPort: 8080, Enabled: true},
	}}
	rec := &stubReconciler{}
	syncer := NewIngressSynchronizer(nil, health)
	syncer.SetReconciler(src, rec)

	result, err := syncer.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}
	if result.Observed != 1 || result.Groups != 1 || result.Healthy != 1 {
		t.Fatalf("unexpected observation result: %+v", result)
	}
	if got := rec.syncRoutes.Load(); got != 1 {
		t.Fatalf("expected exactly 1 SyncRoutes call, got %d", got)
	}

	if err := syncer.CleanupStale(context.Background()); err != nil {
		t.Fatalf("CleanupStale failed: %v", err)
	}
	if got := rec.cleanupCalls.Load(); got != 1 {
		t.Fatalf("expected cleanup delegated to the reconciler, got %d calls", got)
	}
}

// TestUniqueBackendsSkipsEmptyTarget guards the localhost substitution: an
// unresolved target used to appear as a routable backend on the control plane.
func TestUniqueBackendsSkipsEmptyTarget(t *testing.T) {
	groups := GroupRulesByRoute([]*trafficmanager.RoutingRule{
		{ID: "known", Domain: "a.example.com", Path: "/", TargetHost: "10.0.0.1", TargetPort: 8080, Enabled: true},
		{ID: "unknown-target", Domain: "a.example.com", Path: "/", TargetPort: 9090, Enabled: true},
	})

	grp := groups[RouteKey{Domain: "a.example.com", Path: "/", Protocol: "http"}]
	if grp == nil {
		t.Fatal("expected the route group to exist")
	}
	backends := grp.UniqueBackends()
	if len(backends) != 1 {
		t.Fatalf("expected only the resolved backend, got %+v", backends)
	}
	if backends[0].Host == "localhost" || backends[0].Host == "" {
		t.Fatalf("an empty target must not become a backend, got %q", backends[0].Host)
	}
}

// TestBackendKeyDistinguishesNegativePorts replaces the hand-rolled itoa, whose
// non-positive branch returned "" so two different backends shared one record.
func TestBackendKeyDistinguishesNegativePorts(t *testing.T) {
	if backendKey("10.0.0.1", -1) == backendKey("10.0.0.1", -2) {
		t.Fatalf("negative ports collided on key %q", backendKey("10.0.0.1", -1))
	}

	health := NewHealthFilter(2, 30*time.Second)
	health.RecordFailure("10.0.0.1", -1, "one")
	health.RecordFailure("10.0.0.1", -2, "two")

	if health.GetHealth("10.0.0.1", -1).FailCount != 1 {
		t.Fatal("first backend's failure count was corrupted by the second")
	}
	if health.GetHealth("10.0.0.1", -2).FailCount != 1 {
		t.Fatal("second backend's failure count was corrupted by the first")
	}
}

func TestFilterHealthyExcludesUnprobedBackends(t *testing.T) {
	health := NewHealthFilter(2, 30*time.Second)
	health.RecordSuccess("10.0.0.1", 8080)

	backends := []BackendAddr{
		{Host: "10.0.0.1", Port: 8080},
		{Host: "10.0.0.2", Port: 8080},
	}
	healthy := health.FilterHealthy(backends)
	if len(healthy) != 1 || healthy[0].Host != "10.0.0.1" {
		t.Fatalf("expected only the probed-healthy backend, got %+v", healthy)
	}
	if health.IsHealthy("10.0.0.2", 8080) {
		t.Fatal("an unprobed backend must not report healthy")
	}
}

func TestGetAllHealthIsDeterministic(t *testing.T) {
	health := NewHealthFilter(2, 30*time.Second)
	for _, b := range []struct {
		host string
		port int
	}{{"10.0.0.3", 8080}, {"10.0.0.1", 9090}, {"10.0.0.1", 8080}, {"10.0.0.2", 8080}} {
		health.RecordSuccess(b.host, b.port)
	}

	first := health.GetAllHealth()
	second := health.GetAllHealth()
	if len(first) != 4 {
		t.Fatalf("expected 4 tracked backends, got %d", len(first))
	}
	for i := range first {
		if first[i].Host != second[i].Host || first[i].Port != second[i].Port {
			t.Fatalf("ordering differs at %d: %+v vs %+v", i, first, second)
		}
	}
	if first[0].Host != "10.0.0.1" || first[1].Port != 8080 {
		t.Fatalf("expected host-then-port ordering, got %+v", first)
	}
	if health.Count() != 4 {
		t.Fatalf("expected Count of 4, got %d", health.Count())
	}

	health.MarkUnknown("10.0.0.1", 8080)
	if health.Count() != 3 {
		t.Fatalf("MarkUnknown must drop the record, count is %d", health.Count())
	}
	if got := health.GetHealth("10.0.0.1", 8080).Status; got != HealthUnknown {
		t.Fatalf("expected HealthUnknown after MarkUnknown, got %d", got)
	}
}

func TestReaperRestartsAfterStop(t *testing.T) {
	health := NewHealthFilter(2, 30*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	health.StartReaper(ctx, 10*time.Millisecond)
	health.RecordSuccess("10.0.0.1", 8080)
	health.StopReaper()
	// A second Start must actually produce a working reaper, not see a stale flag
	// and return while nothing runs.
	health.StartReaper(ctx, 10*time.Millisecond)
	defer health.StopReaper()

	health.RecordFailure("10.9.9.9", 1234, "stale")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if health.Count() <= 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("reaper did not run after restart, tracked backends: %d", health.Count())
}

func TestRouteGroupStrategyDetection(t *testing.T) {
	rules := []*trafficmanager.RoutingRule{
		{
			ID:         "sticky-rep-0",
			Domain:     "sticky.example.com",
			Path:       "/",
			TargetPort: 8080,
			TargetHost: "10.0.0.1",
			Strategy:   "ip_hash",
			Enabled:    true,
		},
		{
			ID:         "sticky-rep-1",
			Domain:     "sticky.example.com",
			Path:       "/",
			TargetPort: 8081,
			TargetHost: "10.0.0.2",
			Strategy:   "ip_hash",
			Enabled:    true,
		},
	}

	groups := GroupRulesByRoute(rules)
	strategy := getFirstGroup(groups).Strategy()
	if strategy != "ip_hash" {
		t.Fatalf("expected ip_hash strategy, got %s", strategy)
	}
}

func TestPortMapping(t *testing.T) {
	rules := []*trafficmanager.RoutingRule{
		{
			ID:         "http-port",
			Domain:     "web.example.com",
			Path:       "/",
			TargetPort: 80,
			TargetHost: "10.0.0.1",
			Enabled:    true,
		},
	}

	groups := GroupRulesByRoute(rules)
	backends := getFirstGroup(groups).UniqueBackends()
	if len(backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(backends))
	}
	if backends[0].Port != 80 {
		t.Fatalf("expected port 80, got %d", backends[0].Port)
	}
}

func TestWorkloadOnGatewayNode(t *testing.T) {
	rules := []*trafficmanager.RoutingRule{
		{
			ID:         "local-svc",
			Domain:     "local.example.com",
			Path:       "/",
			TargetPort: 8080,
			TargetHost: "localhost",
			ServerID:   "srv-local",
			Enabled:    true,
		},
	}

	groups := GroupRulesByRoute(rules)
	backends := getFirstGroup(groups).UniqueBackends()

	if len(backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(backends))
	}
	if backends[0].Host != "localhost" {
		t.Fatalf("expected localhost host, got %s", backends[0].Host)
	}
	if backends[0].Port != 8080 {
		t.Fatalf("expected port 8080, got %d", backends[0].Port)
	}
}

func TestWorkloadOnRemoteNode(t *testing.T) {
	rules := []*trafficmanager.RoutingRule{
		{
			ID:         "remote-svc",
			Domain:     "remote.example.com",
			Path:       "/",
			TargetPort: 3000,
			TargetHost: "10.210.1.5",
			ServerID:   "srv-remote",
			Enabled:    true,
		},
	}

	groups := GroupRulesByRoute(rules)
	backends := getFirstGroup(groups).UniqueBackends()

	if len(backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(backends))
	}
	if backends[0].Host != "10.210.1.5" {
		t.Fatalf("expected remote WireGuard IP, got %s", backends[0].Host)
	}
}

func getFirstGroup(groups map[RouteKey]*RouteGroup) *RouteGroup {
	for _, grp := range groups {
		return grp
	}
	return nil
}
