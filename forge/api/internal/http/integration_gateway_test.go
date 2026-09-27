package http

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"gamepanel/forge/internal/services/crossnode"
	"gamepanel/forge/internal/services/trafficmanager"
)

// mockGateway records gateway calls to verify empty-sync guard (F-NET-01).
// It implements the unexported gatewayAdapter interface required by
// crossnode.NewIngressSynchronizer via structural typing (method set matches).
type mockGateway struct {
	mu           sync.Mutex
	updateCalls  int
	lastRules    []*trafficmanager.RoutingRule
	lastPolicies map[string]*trafficmanager.TrafficPolicy
	reloadCalls  int
	cleanupCalls int
	shouldFail   error
}

func (m *mockGateway) UpdateRoutes(ctx context.Context, rules []*trafficmanager.RoutingRule, policies map[string]*trafficmanager.TrafficPolicy) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCalls++
	// deep copy rules to avoid race with caller mutating slice
	copied := make([]*trafficmanager.RoutingRule, len(rules))
	for i, r := range rules {
		if r != nil {
			cp := *r
			copied[i] = &cp
		}
	}
	m.lastRules = copied
	m.lastPolicies = policies
	if m.shouldFail != nil {
		return m.shouldFail
	}
	return nil
}

func (m *mockGateway) RemoveRoutes(ctx context.Context, ruleIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupCalls++
	return nil
}

func (m *mockGateway) CleanupStale(ctx context.Context, activeRuleIDs map[string]bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupCalls++
	return nil
}

func (m *mockGateway) Reload(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reloadCalls++
	return nil
}

func (m *mockGateway) Health(ctx context.Context) trafficmanager.AdapterHealth {
	return trafficmanager.AdapterHealth{Status: trafficmanager.HealthHealthy}
}

func (m *mockGateway) UpdateCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updateCalls
}

func (m *mockGateway) LastRules() []*trafficmanager.RoutingRule {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*trafficmanager.RoutingRule, len(m.lastRules))
	copy(out, m.lastRules)
	return out
}

// TestIntegrationGateway_EmptySyncGuard_NoWipe verifies HOTFIX F-NET-01:
// Sync with zero rules must NOT call UpdateRoutes (empty push would wipe Caddy
// gamepanel server config including gamepanel-domains).
func TestIntegrationGateway_EmptySyncGuard_NoWipe(t *testing.T) {
	gw := &mockGateway{}
	resolver := crossnode.NewResolver(nil)
	health := crossnode.NewHealthFilter(2, 30*time.Second)
	syncer := crossnode.NewIngressSynchronizer(gw, resolver, health)

	// No SetRules called — internal map is empty
	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("Sync with empty rule set should not error: %v", err)
	}
	if gw.UpdateCallCount() != 0 {
		t.Fatalf("empty-sync guard violated: UpdateRoutes called %d times with empty rules (would wipe gateway)", gw.UpdateCallCount())
	}
	stats := syncer.Stats()
	if stats.SyncCount != 0 {
		t.Fatalf("empty sync should not increment SyncCount, got %d", stats.SyncCount)
	}
}

// TestIntegrationGateway_DisabledRules_NoWipe ensures rules that are all
// Enabled=false are treated as empty (filtered before grouping) and also skip
// gateway update.
func TestIntegrationGateway_DisabledRules_NoWipe(t *testing.T) {
	gw := &mockGateway{}
	resolver := crossnode.NewResolver(nil)
	health := crossnode.NewHealthFilter(2, 30*time.Second)
	syncer := crossnode.NewIngressSynchronizer(gw, resolver, health)

	syncer.SetRules([]*trafficmanager.RoutingRule{
		{ID: "disabled-1", Domain: "old.example.com", Path: "/", TargetHost: "10.0.0.1", TargetPort: 8080, Enabled: false},
		{ID: "disabled-2", Domain: "old.example.com", Path: "/", TargetHost: "10.0.0.2", TargetPort: 8080, Enabled: false},
	})
	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("Sync disabled-only should not error: %v", err)
	}
	if gw.UpdateCallCount() != 0 {
		t.Fatalf("disabled-only sync should not call UpdateRoutes, got %d calls", gw.UpdateCallCount())
	}
}

// TestIntegrationGateway_NoHealthyBackends_NoWipe verifies second guard:
// When all backends are unhealthy, mergedRules stays empty; Sync must skip
// UpdateRoutes to avoid wiping gateway with zero routes.
func TestIntegrationGateway_NoHealthyBackends_NoWipe(t *testing.T) {
	gw := &mockGateway{}
	resolver := crossnode.NewResolver(nil)
	health := crossnode.NewHealthFilter(1, 30*time.Second) // threshold 1 for quick down
	syncer := crossnode.NewIngressSynchronizer(gw, resolver, health)

	rules := []*trafficmanager.RoutingRule{
		{ID: "svc-a", Domain: "app.example.com", Path: "/", TargetHost: "node-1.internal", TargetPort: 8080, Enabled: true},
		{ID: "svc-b", Domain: "app.example.com", Path: "/", TargetHost: "node-2.internal", TargetPort: 8080, Enabled: true},
	}
	syncer.SetRules(rules)

	// Mark both backends DOWN
	health.RecordFailure("node-1.internal", 8080, "dial timeout")
	health.RecordFailure("node-2.internal", 8080, "dial timeout")

	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("no-healthy sync should not error: %v", err)
	}
	if gw.UpdateCallCount() != 0 {
		t.Fatalf("no-healthy guard violated: UpdateRoutes called %d times with zero healthy backends (would wipe gateway)", gw.UpdateCallCount())
	}
	stats := syncer.Stats()
	if stats.ErrCount != 0 {
		t.Fatalf("skipped sync due to no healthy should not count as error, ErrCount=%d", stats.ErrCount)
	}
}

// TestIntegrationGateway_SingleWriter_HealthySync_CallsUpdate verifies the
// positive path: with at least one healthy backend, Sync MUST call UpdateRoutes
// exactly once and propagate the merged rules.
func TestIntegrationGateway_SingleWriter_HealthySync_CallsUpdate(t *testing.T) {
	gw := &mockGateway{}
	resolver := crossnode.NewResolver(nil)
	health := crossnode.NewHealthFilter(3, 30*time.Second)
	syncer := crossnode.NewIngressSynchronizer(gw, resolver, health)

	// Only backends with a recorded healthy result are routed; unknown hosts
	// are excluded, never assumed.
	health.RecordSuccess("10.0.0.10", 8080)
	health.RecordSuccess("10.0.0.11", 8080)
	syncer.SetRules([]*trafficmanager.RoutingRule{
		{ID: "healthy-1", Domain: "app.example.com", Path: "/", TargetHost: "10.0.0.10", TargetPort: 8080, Enabled: true, Weight: 1},
		{ID: "healthy-2", Domain: "app.example.com", Path: "/", TargetHost: "10.0.0.11", TargetPort: 8080, Enabled: true, Weight: 1},
	})

	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("healthy Sync should succeed: %v", err)
	}
	if gw.UpdateCallCount() != 1 {
		t.Fatalf("expected exactly 1 UpdateRoutes call for healthy sync, got %d", gw.UpdateCallCount())
	}
	last := gw.LastRules()
	if len(last) == 0 {
		t.Fatal("expected non-empty rules pushed to gateway")
	}
	// Single route group with 2 backends should produce 2 merged rules (primary + replica)
	if len(last) != 2 {
		t.Fatalf("expected 2 merged rules (primary + replica) for 2 healthy backends, got %d", len(last))
	}
	stats := syncer.Stats()
	if stats.SyncCount != 1 {
		t.Fatalf("SyncCount should be 1 after successful sync, got %d", stats.SyncCount)
	}
	if stats.RuleCount != 2 {
		t.Fatalf("RuleCount should reflect SetRules size 2, got %d", stats.RuleCount)
	}
}

// TestIntegrationGateway_SingleWriter_PartialHealthy_OnlyHealthyPushed checks
// that when one backend is unhealthy, only the healthy backend is pushed.
func TestIntegrationGateway_SingleWriter_PartialHealthy_OnlyHealthyPushed(t *testing.T) {
	gw := &mockGateway{}
	resolver := crossnode.NewResolver(nil)
	health := crossnode.NewHealthFilter(1, 30*time.Second)
	syncer := crossnode.NewIngressSynchronizer(gw, resolver, health)

	syncer.SetRules([]*trafficmanager.RoutingRule{
		{ID: "r-healthy", Domain: "svc.example.com", Path: "/", TargetHost: "good.internal", TargetPort: 8080, Enabled: true},
		{ID: "r-unhealthy", Domain: "svc.example.com", Path: "/", TargetHost: "bad.internal", TargetPort: 8080, Enabled: true},
	})
	health.RecordSuccess("good.internal", 8080)
	health.RecordFailure("bad.internal", 8080, "connection refused")

	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("partial-healthy Sync failed: %v", err)
	}
	if gw.UpdateCallCount() != 1 {
		t.Fatalf("expected 1 UpdateRoutes, got %d", gw.UpdateCallCount())
	}
	last := gw.LastRules()
	if len(last) != 1 {
		t.Fatalf("expected 1 merged rule (only healthy backend), got %d: %+v", len(last), last)
	}
	if last[0].TargetHost != "good.internal" {
		t.Fatalf("expected healthy host good.internal, got %s", last[0].TargetHost)
	}
}

// TestIntegrationGateway_SingleWriter_UpsertAndRemove verifies UpsertRule and
// RemoveRule mutate the single-writer protected map correctly and Sync reflects it.
func TestIntegrationGateway_SingleWriter_UpsertAndRemove(t *testing.T) {
	gw := &mockGateway{}
	resolver := crossnode.NewResolver(nil)
	health := crossnode.NewHealthFilter(2, 30*time.Second)
	syncer := crossnode.NewIngressSynchronizer(gw, resolver, health)

	health.RecordSuccess("10.0.0.1", 8080)
	health.RecordSuccess("10.0.0.2", 9090)
	initial := &trafficmanager.RoutingRule{ID: "rule-1", Domain: "a.example.com", Path: "/", TargetHost: "10.0.0.1", TargetPort: 8080, Enabled: true}
	syncer.SetRules([]*trafficmanager.RoutingRule{initial})

	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	if gw.UpdateCallCount() != 1 {
		t.Fatalf("initial sync should have 1 call, got %d", gw.UpdateCallCount())
	}

	// Upsert second rule on different domain -> should increase groups
	upserted := &trafficmanager.RoutingRule{ID: "rule-2", Domain: "b.example.com", Path: "/", TargetHost: "10.0.0.2", TargetPort: 9090, Enabled: true}
	syncer.UpsertRule(upserted)
	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("upsert sync: %v", err)
	}
	if gw.UpdateCallCount() != 2 {
		t.Fatalf("after upsert, expected 2 total calls, got %d", gw.UpdateCallCount())
	}
	last := gw.LastRules()
	if len(last) != 2 {
		t.Fatalf("expected 2 routes after upsert (2 domains), got %d", len(last))
	}

	// Remove first rule
	syncer.RemoveRule("rule-1")
	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("remove sync: %v", err)
	}
	if gw.UpdateCallCount() != 3 {
		t.Fatalf("after remove, expected 3 total calls, got %d", gw.UpdateCallCount())
	}
	last = gw.LastRules()
	if len(last) != 1 {
		t.Fatalf("expected 1 route after removal, got %d", len(last))
	}
	if last[0].ID != "rule-2" && last[0].Domain != "b.example.com" {
		// ID may be suffixed in merged path but domain should match
		if last[0].Domain != "b.example.com" {
			t.Fatalf("remaining rule should be b.example.com, got %+v", last[0])
		}
	}

	// Remove last rule — next sync should hit empty guard and NOT call gateway
	syncer.RemoveRule("rule-2")
	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("empty-after-remove sync: %v", err)
	}
	if gw.UpdateCallCount() != 3 {
		t.Fatalf("empty after remove should not call gateway again, calls still %d", gw.UpdateCallCount())
	}
}

// TestIntegrationGateway_SingleWriter_ConcurrentUpdates ensures the
// IngressSynchronizer mutex protects against data races when rules are mutated
// concurrently while Sync runs (single-writer guarantee).
func TestIntegrationGateway_SingleWriter_ConcurrentUpdates(t *testing.T) {
	gw := &mockGateway{}
	resolver := crossnode.NewResolver(nil)
	health := crossnode.NewHealthFilter(2, 30*time.Second)
	syncer := crossnode.NewIngressSynchronizer(gw, resolver, health)

	for i := 0; i < 5; i++ {
		health.RecordSuccess("10.0.0.1", 8080+i)
	}
	syncer.SetRules([]*trafficmanager.RoutingRule{
		{ID: "concurrent-base", Domain: "base.example.com", Path: "/", TargetHost: "10.0.0.1", TargetPort: 8080, Enabled: true},
	})

	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			syncer.UpsertRule(&trafficmanager.RoutingRule{
				ID:         fmt.Sprintf("concurrent-%d", idx),
				Domain:     fmt.Sprintf("c%d.example.com", idx),
				Path:       "/",
				TargetHost: "10.0.0.1",
				TargetPort: 8080 + idx,
				Enabled:    true,
			})
			errs <- syncer.Sync(context.Background())
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Sync failed: %v", err)
		}
	}
	// At least the base + 5 upserts should have been counted via Stats; gateway may have been called up to 5 times
	if c := gw.UpdateCallCount(); c == 0 {
		t.Fatal("concurrent syncs should have resulted in at least one gateway update")
	}
}

// TestIntegrationGateway_HealthDescribe verifies the synchronizer exposes
// health description (used by /admin/crossnode/describe endpoint).
func TestIntegrationGateway_HealthDescribe(t *testing.T) {
	gw := &mockGateway{}
	resolver := crossnode.NewResolver(nil)
	health := crossnode.NewHealthFilter(1, 30*time.Second)
	syncer := crossnode.NewIngressSynchronizer(gw, resolver, health)

	// Unknown -> HEALTHY
	desc := syncer.DescribeBackend("newhost.internal", 8080)
	if desc == "" {
		t.Fatal("DescribeBackend should return non-empty for unknown host")
	}
	if !containsSub(desc, "HEALTHY") {
		t.Fatalf("unknown backend should be HEALTHY, got %q", desc)
	}
	// Mark degraded/down — first failure is Degraded (threshold logic creates Degraded on first record)
	health.RecordFailure("newhost.internal", 8080, "timeout")
	desc = syncer.DescribeBackend("newhost.internal", 8080)
	if len(desc) == 0 || (!containsSub(desc, "DOWN") && !containsSub(desc, "DEGRADED")) {
		t.Fatalf("expected DOWN or DEGRADED description after failure, got %q", desc)
	}
	// Second failure pushes over threshold to DOWN
	health.RecordFailure("newhost.internal", 8080, "timeout")
	desc = syncer.DescribeBackend("newhost.internal", 8080)
	if !containsSub(desc, "DOWN") {
		t.Fatalf("expected DOWN after threshold, got %q", desc)
	}
}

// TestIntegrationGateway_RouteGenerationTracked checks that successful sync
// populates tracking records visible via RouteGenerationRecords (single-writer
// state observation).
func TestIntegrationGateway_RouteGenerationTracked(t *testing.T) {
	gw := &mockGateway{}
	resolver := crossnode.NewResolver(nil)
	health := crossnode.NewHealthFilter(2, 30*time.Second)
	syncer := crossnode.NewIngressSynchronizer(gw, resolver, health)

	health.RecordSuccess("10.0.0.1", 8080)
	health.RecordSuccess("10.0.0.2", 8080)
	syncer.SetRules([]*trafficmanager.RoutingRule{
		{ID: "track-1", Domain: "track.example.com", Path: "/", TargetHost: "10.0.0.1", TargetPort: 8080, Enabled: true},
		{ID: "track-2", Domain: "track2.example.com", Path: "/api", TargetHost: "10.0.0.2", TargetPort: 8080, Enabled: true},
	})
	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	records := syncer.RouteGenerationRecords()
	if len(records) != 2 {
		t.Fatalf("expected 2 route generation records (2 groups), got %d", len(records))
	}
	for _, rec := range records {
		if _, ok := syncer.GetRouteGenerationRecord(rec.GroupID); !ok {
			t.Fatalf("GetRouteGenerationRecord missing for %s", rec.GroupID)
		}
	}
	stats := syncer.Stats()
	if stats.TrackingCount != 2 {
		t.Fatalf("TrackingCount should be 2, got %d", stats.TrackingCount)
	}
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
