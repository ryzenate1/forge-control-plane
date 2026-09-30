package http

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"testing"
	"time"

	"gamepanel/forge/internal/services/crossnode"
	"gamepanel/forge/internal/services/domains"
	"gamepanel/forge/internal/services/servicediscovery"
	"gamepanel/forge/internal/services/trafficmanager"
)

// These tests pin F-NET-01 in its final shape. crossnode.IngressSynchronizer was
// a second, unwired WRITER on the Caddy admin API that trafficmanager.Service
// also owns: UpdateRoutes is a full replace and CleanupStale(activeIDs) withdraws
// every gamepanel-* route missing from that set, and the synchronizer's own rule
// map was never populated in production. One click on /admin/crossnode/ingress/
// cleanup therefore handed the gateway an empty active set and pulled every live
// route down.
//
// The synchronizer is now a read-only OBSERVER: it reads rules through
// crossnode.RuleSource, counts healthy backends, and delegates every convergence
// step to crossnode.GatewayReconciler (in production both are
// trafficmanager.Service, which knows the real active set). Without a reconciler
// it refuses rather than writing. The gateway surface it may reach at all is the
// one-method reader interface below.

// gatewayReaderSurface restates crossnode's unexported gatewayReader: the only
// gateway capability an observer can be built on. crossnode.gatewayReader cannot
// be named from a package-http test, so this identical one-method interface is
// the closest portable form of "the constructor parameter admits no write".
type gatewayReaderSurface interface {
	Health(ctx context.Context) trafficmanager.AdapterHealth
}

// mockGateway is a writer-capable gateway: it implements the whole
// trafficmanager.GatewayAdapter surface the old synchronizer shared with
// trafficmanager.Service, and records every call by name. As an observer,
// crossnode may ask it exactly one question — Health — so any other entry in the
// log is the route-wipe regression, whether it comes as a full-replace
// UpdateRoutes or a CleanupStale with an empty active set.
type mockGateway struct {
	mu           sync.Mutex
	calls        []string
	lastRules    []*trafficmanager.RoutingRule
	lastPolicies map[string]*trafficmanager.TrafficPolicy
	cleanupIDs   []string
	shouldFail   error
}

// Compile-level proof that the spy really is the gateway: the object handed to
// NewIngressSynchronizer could withdraw every route if anything reached it.
var _ trafficmanager.GatewayAdapter = (*mockGateway)(nil)

func (m *mockGateway) record(method string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, method)
}

// Calls returns the recorded method names in call order.
func (m *mockGateway) Calls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.calls))
	copy(out, m.calls)
	return out
}

// NonHealthCalls is every gateway interaction except the read-only health probe.
// It must stay empty for the whole file.
func (m *mockGateway) NonHealthCalls() []string {
	var out []string
	for _, c := range m.Calls() {
		if c != "Health" {
			out = append(out, c)
		}
	}
	return out
}

func (m *mockGateway) NonHealthCount() int { return len(m.NonHealthCalls()) }

func (m *mockGateway) HealthCallCount() int {
	n := 0
	for _, c := range m.Calls() {
		if c == "Health" {
			n++
		}
	}
	return n
}

// LastPushedRules exposes what a full replace would have carried, so a violation
// can be reported instead of merely counted.
func (m *mockGateway) LastPushedRules() []*trafficmanager.RoutingRule {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*trafficmanager.RoutingRule, len(m.lastRules))
	copy(out, m.lastRules)
	return out
}

// WithdrawnIDs reports which route ids a CleanupStale pass tried to withdraw.
func (m *mockGateway) WithdrawnIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.cleanupIDs))
	copy(out, m.cleanupIDs)
	return out
}

func (m *mockGateway) Kind() trafficmanager.AdapterKind {
	m.record("Kind")
	return trafficmanager.AdapterCaddy
}

func (m *mockGateway) UpdateRoutes(ctx context.Context, rules []*trafficmanager.RoutingRule, policies map[string]*trafficmanager.TrafficPolicy) error {
	m.record("UpdateRoutes")
	m.mu.Lock()
	m.lastRules = append([]*trafficmanager.RoutingRule(nil), rules...)
	m.lastPolicies = policies
	m.mu.Unlock()
	return m.shouldFail
}

func (m *mockGateway) RemoveRoutes(ctx context.Context, ruleIDs []string) error {
	m.record("RemoveRoutes")
	m.mu.Lock()
	m.cleanupIDs = append(m.cleanupIDs, ruleIDs...)
	m.mu.Unlock()
	return m.shouldFail
}

func (m *mockGateway) GetActiveConnections() map[string]int {
	m.record("GetActiveConnections")
	return map[string]int{}
}

func (m *mockGateway) UpdateDomainRoutes(ctx context.Context, domainRoutes []domains.VerifiedDomainRoute) error {
	m.record("UpdateDomainRoutes")
	return m.shouldFail
}

func (m *mockGateway) SetCertificate(ctx context.Context, cert trafficmanager.CertConfig) error {
	m.record("SetCertificate")
	return m.shouldFail
}

func (m *mockGateway) RemoveCertificate(ctx context.Context, names []string) error {
	m.record("RemoveCertificate")
	return m.shouldFail
}

func (m *mockGateway) ValidateConfig(ctx context.Context) error {
	m.record("ValidateConfig")
	return nil
}

func (m *mockGateway) Reload(ctx context.Context) error {
	m.record("Reload")
	return m.shouldFail
}

func (m *mockGateway) Rollback(ctx context.Context) error {
	m.record("Rollback")
	return m.shouldFail
}

func (m *mockGateway) CleanupStale(ctx context.Context, activeRuleIDs map[string]bool) error {
	m.record("CleanupStale")
	m.mu.Lock()
	for id := range activeRuleIDs {
		m.cleanupIDs = append(m.cleanupIDs, id)
	}
	m.mu.Unlock()
	return m.shouldFail
}

func (m *mockGateway) Health(ctx context.Context) trafficmanager.AdapterHealth {
	m.record("Health")
	return trafficmanager.AdapterHealth{Status: trafficmanager.HealthHealthy}
}

func (m *mockGateway) SetUpstreamHealth(ctx context.Context, ruleID string, targetHost string, targetPort int, healthy bool) error {
	m.record("SetUpstreamHealth")
	return m.shouldFail
}

// mockRuleSource is a crossnode.RuleSource stub standing in for trafficmanager.Service:
// the component that owns the live rule and policy sets. Replacing the deleted
// SetRules/UpsertRule/RemoveRule mutators, it is the only way a test can change
// what the observer sees — which is exactly the point: the synchronizer holds no
// routing state of its own to be stale.
type mockRuleSource struct {
	mu          sync.Mutex
	rules       []*trafficmanager.RoutingRule
	policies    []*trafficmanager.TrafficPolicy
	rulesErr    error
	policiesErr error
	listCalls   int
}

var _ crossnode.RuleSource = (*mockRuleSource)(nil)

func (s *mockRuleSource) ListRoutingRules(ctx context.Context) ([]*trafficmanager.RoutingRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCalls++
	if s.rulesErr != nil {
		return nil, s.rulesErr
	}
	out := make([]*trafficmanager.RoutingRule, len(s.rules))
	copy(out, s.rules)
	return out, nil
}

func (s *mockRuleSource) ListTrafficPolicies(ctx context.Context) ([]*trafficmanager.TrafficPolicy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.policiesErr != nil {
		return nil, s.policiesErr
	}
	out := make([]*trafficmanager.TrafficPolicy, len(s.policies))
	copy(out, s.policies)
	return out, nil
}

func (s *mockRuleSource) setRules(rules ...*trafficmanager.RoutingRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules = append([]*trafficmanager.RoutingRule(nil), rules...)
}

func (s *mockRuleSource) addRule(rule *trafficmanager.RoutingRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules = append(s.rules, rule)
}

func (s *mockRuleSource) removeRule(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.rules[:0]
	for _, r := range s.rules {
		if r.ID != id {
			kept = append(kept, r)
		}
	}
	s.rules = kept
}

func (s *mockRuleSource) setPolicies(policies ...*trafficmanager.TrafficPolicy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policies = append([]*trafficmanager.TrafficPolicy(nil), policies...)
}

func (s *mockRuleSource) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.rules)
}

func (s *mockRuleSource) listCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listCalls
}

// mockReconciler is the crossnode.GatewayReconciler spy: it records the two
// convergence requests the synchronizer is allowed to make, and nothing else. In
// production this is trafficmanager.Service, which rebuilds the active route set
// from its own rule map instead of trusting crossnode's view.
type mockReconciler struct {
	mu           sync.Mutex
	syncCalls    int
	cleanupCalls int
	syncErr      error
	cleanupErr   error
}

var _ crossnode.GatewayReconciler = (*mockReconciler)(nil)

func (r *mockReconciler) SyncRoutes(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.syncCalls++
	return r.syncErr
}

func (r *mockReconciler) CleanupStaleRoutes(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupCalls++
	return r.cleanupErr
}

func (r *mockReconciler) SyncCallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.syncCalls
}

func (r *mockReconciler) CleanupCallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cleanupCalls
}

func (r *mockReconciler) TotalCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.syncCalls + r.cleanupCalls
}

// mockEndpointSource is a crossnode.EndpointSource stub for the service-discovery
// prober. Attaching it changes observation: every pass replaces all recorded
// health verdicts with the current endpoint set, so a backend whose endpoint
// disappeared cannot keep reporting healthy from a stale reading. That is why the
// fixture below leaves it unattached unless a test asks for it.
type mockEndpointSource struct {
	mu        sync.Mutex
	endpoints []servicediscovery.ServiceEndpoint
	listCalls int
}

var _ crossnode.EndpointSource = (*mockEndpointSource)(nil)

func (e *mockEndpointSource) ListEndpoints(ctx context.Context, filter servicediscovery.EndpointFilter) []servicediscovery.ServiceEndpoint {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.listCalls++
	out := make([]servicediscovery.ServiceEndpoint, len(e.endpoints))
	copy(out, e.endpoints)
	return out
}

func (e *mockEndpointSource) set(endpoints ...servicediscovery.ServiceEndpoint) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.endpoints = append([]servicediscovery.ServiceEndpoint(nil), endpoints...)
}

// ingressFixture wires the observer the way cmd/api does: a recording gateway
// reader, trafficmanager-shaped rule source and reconciler, and the health filter
// the prober writes into.
type ingressFixture struct {
	gw     *mockGateway
	rules  *mockRuleSource
	rec    *mockReconciler
	health *crossnode.HealthFilter
	syncer *crossnode.IngressSynchronizer
}

func newIngressFixture(t *testing.T, threshold int, rules ...*trafficmanager.RoutingRule) *ingressFixture {
	t.Helper()

	f := &ingressFixture{
		gw:     &mockGateway{},
		rules:  &mockRuleSource{},
		rec:    &mockReconciler{},
		health: crossnode.NewHealthFilter(threshold, 30*time.Second),
	}
	f.rules.setRules(rules...)
	f.syncer = crossnode.NewIngressSynchronizer(f.gw, f.health)
	f.syncer.SetReconciler(f.rules, f.rec)
	return f
}

// assertGatewayUntouched is the one assertion every test in this file shares.
func assertGatewayUntouched(t *testing.T, gw *mockGateway, stage string) {
	t.Helper()
	if touches := gw.NonHealthCalls(); len(touches) != 0 {
		t.Fatalf("%s: crossnode wrote to the gateway (%v); route-wipe regression (pushed %d rules, withdrew %d ids)",
			stage, touches, len(gw.LastPushedRules()), len(gw.WithdrawnIDs()))
	}
}

// TestIntegrationGateway_NoReconciler_ReconcileAndCleanupRefuse is the P0
// regression: with no gateway reconciler wired, neither convergence entry point
// may fall through to a gateway write. CleanupStale used to push the
// synchronizer's own (here: populated) view straight to the adapter; with an
// unpopulated view that was an empty active set and a full route withdrawal.
func TestIntegrationGateway_NoReconciler_ReconcileAndCleanupRefuse(t *testing.T) {
	rule := &trafficmanager.RoutingRule{
		ID: "no-rec-1", Domain: "svc.example.com", Path: "/",
		TargetHost: "10.0.0.9", TargetPort: 8080, Enabled: true,
	}
	gw := &mockGateway{}
	rules := &mockRuleSource{}
	rules.setRules(rule)
	// A reconciler exists but is deliberately never registered: it is the spy that
	// must record zero calls.
	rec := &mockReconciler{}

	health := crossnode.NewHealthFilter(2, 30*time.Second)
	health.RecordSuccess("10.0.0.9", 8080)

	syncer := crossnode.NewIngressSynchronizer(gw, health)
	// Rule source configured, converger absent — so a refusal here is specifically
	// ErrNoReconciler, not ErrNoRuleSource.
	syncer.SetReconciler(rules, nil)

	if syncer.ReconcilerConfigured() {
		t.Fatal("ReconcilerConfigured() must report false with no reconciler")
	}
	if stats := syncer.Stats(); stats.ReconcilerConfigured {
		t.Fatalf("Stats().ReconcilerConfigured must be false, got %+v", stats)
	}

	ctx := context.Background()
	res, err := syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("observation must still work without a reconciler: %v", err)
	}
	if res.Observed != 1 || res.Groups != 1 || res.Healthy != 1 || res.Skipped {
		t.Fatalf("expected a full observation (1 rule / 1 group / 1 healthy), got %+v", res)
	}

	res, err = syncer.Reconcile(ctx)
	if !errors.Is(err, crossnode.ErrNoReconciler) {
		t.Fatalf("Reconcile without a reconciler must return ErrNoReconciler, got %v", err)
	}
	if !res.Skipped || res.Reason != crossnode.ErrNoReconciler.Error() {
		t.Fatalf("refused Reconcile must report itself as skipped with the reason, got %+v", res)
	}

	if err := syncer.CleanupStale(ctx); !errors.Is(err, crossnode.ErrNoReconciler) {
		t.Fatalf("CleanupStale without a reconciler must return ErrNoReconciler, got %v", err)
	}

	if got := rec.TotalCalls(); got != 0 {
		t.Fatalf("unregistered reconciler recorded %d calls, want 0", got)
	}
	if got := rec.SyncCallCount(); got != 0 {
		t.Fatalf("SyncRoutes calls without a reconciler: %d, want 0", got)
	}
	if got := rec.CleanupCallCount(); got != 0 {
		t.Fatalf("CleanupStaleRoutes calls without a reconciler: %d, want 0", got)
	}
	assertGatewayUntouched(t, gw, "no-reconciler refusal")
	if gw.WithdrawnIDs() != nil && len(gw.WithdrawnIDs()) != 0 {
		t.Fatalf("no route may be withdrawn without a reconciler, got %v", gw.WithdrawnIDs())
	}

	// The refusal is an error, not a silent success: the operator sees it, and only
	// the refused CleanupStale counts as an error (Reconcile refuses before it
	// touches observation or the gateway).
	stats := syncer.Stats()
	if stats.ErrCount != 1 {
		t.Fatalf("ErrCount should record the refused cleanup, got %d", stats.ErrCount)
	}
	if stats.SyncCount != 1 {
		t.Fatalf("a refused Reconcile must not count as a completed sync, got %d", stats.SyncCount)
	}
	if stats.RuleCount != 1 {
		t.Fatalf("observation should still hold 1 rule, got %d", stats.RuleCount)
	}
}

// TestIntegrationGateway_ObserverNeverWritesGateway is the second P0 regression:
// whatever the synchronizer is asked to do, the only gateway interaction it can
// produce is the read-only health probe.
//
// Two halves. Compile-level: gw is a full trafficmanager.GatewayAdapter — it
// could withdraw every route — and it reaches NewIngressSynchronizer narrowed to
// gatewayReaderSurface, which names Health and nothing else, so no write call is
// even addressable from inside crossnode. Runtime: crossnode.gatewayReader is
// unexported and cannot be named from a package-http test, so the recording spy
// is the portable proof, and it is what a future refactor has to satisfy.
func TestIntegrationGateway_ObserverNeverWritesGateway(t *testing.T) {
	ctx := context.Background()
	gw := &mockGateway{}
	rules := &mockRuleSource{}
	rules.setRules(
		&trafficmanager.RoutingRule{ID: "obs-1", Domain: "app.example.com", Path: "/", TargetHost: "10.0.0.10", TargetPort: 8080, Enabled: true, Weight: 1},
		&trafficmanager.RoutingRule{ID: "obs-2", Domain: "app.example.com", Path: "/", TargetHost: "10.0.0.11", TargetPort: 8080, Enabled: true, Weight: 2},
	)
	rec := &mockReconciler{}

	health := crossnode.NewHealthFilter(2, 30*time.Second)
	health.RecordSuccess("10.0.0.10", 8080)
	health.RecordSuccess("10.0.0.11", 8080)

	// The writer-capable adapter, narrowed to the reader surface the constructor
	// demands. This assignment is the compile-level assertion.
	var adapter trafficmanager.GatewayAdapter = gw
	var reader gatewayReaderSurface = adapter

	syncer := crossnode.NewIngressSynchronizer(reader, health)
	syncer.SetReconciler(rules, rec)

	if _, err := syncer.Sync(ctx); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	assertGatewayUntouched(t, gw, "after Sync")

	if _, err := syncer.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	assertGatewayUntouched(t, gw, "after Reconcile")
	if got := rec.SyncCallCount(); got != 1 {
		t.Fatalf("Reconcile must delegate convergence exactly once, got %d", got)
	}

	if err := syncer.CleanupStale(ctx); err != nil {
		t.Fatalf("CleanupStale: %v", err)
	}
	assertGatewayUntouched(t, gw, "after CleanupStale")
	if got := rec.CleanupCallCount(); got != 1 {
		t.Fatalf("CleanupStale must delegate exactly once, got %d", got)
	}

	// Read-only surfaces, including the pass-through the admin health endpoints use.
	h := syncer.Health(ctx)
	if h.Status != trafficmanager.HealthHealthy {
		t.Fatalf("reader health should pass through, got %+v", h)
	}
	_ = syncer.Backends()
	_ = syncer.Stats()
	_ = syncer.CurrentRules()
	_ = syncer.CurrentPolicies()
	_ = syncer.RouteGenerationRecords()
	_ = syncer.DescribeBackend("10.0.0.10", 8080)

	assertGatewayUntouched(t, gw, "after every synchronizer entry point")
	// The gateway saw nothing but Health, so it was never asked to replace or
	// withdraw a route.
	if calls := gw.Calls(); len(calls) != 1 || calls[0] != "Health" {
		t.Fatalf("expected exactly one Health interaction, got %v", calls)
	}
	if n := gw.HealthCallCount(); n != 1 {
		t.Fatalf("Health probe count %d, want 1", n)
	}
	// Belt and braces on the wipe itself: no rule set was pushed and no id was
	// withdrawn, so there is no full replace and no empty active set.
	if got := len(gw.LastPushedRules()); got != 0 {
		t.Fatalf("%d rules pushed through the adapter", got)
	}
	if got := len(gw.WithdrawnIDs()); got != 0 {
		t.Fatalf("%d route ids withdrawn through the adapter", got)
	}
	if len(gw.NonHealthCalls()) != 0 {
		t.Fatalf("gateway write surface used: %v", gw.NonHealthCalls())
	}
}

// TestIntegrationGateway_NoRuleSource_SyncRefuses is the direct descendant of the
// empty-sync guard: an observer with no rule source does not know the live route
// set, so it must error rather than report an empty success — and, crucially, it
// must not converge anything.
func TestIntegrationGateway_NoRuleSource_SyncRefuses(t *testing.T) {
	gw := &mockGateway{}
	rec := &mockReconciler{}
	health := crossnode.NewHealthFilter(2, 30*time.Second)

	syncer := crossnode.NewIngressSynchronizer(gw, health)
	// A converger but no source: the refusal must be ErrNoRuleSource.
	syncer.SetReconciler(nil, rec)

	ctx := context.Background()
	res, err := syncer.Sync(ctx)
	if !errors.Is(err, crossnode.ErrNoRuleSource) {
		t.Fatalf("Sync without a rule source must return ErrNoRuleSource, got %v", err)
	}
	if !res.Skipped || res.Reason != crossnode.ErrNoRuleSource.Error() {
		t.Fatalf("refused Sync must report skipped with the reason, got %+v", res)
	}
	if res.Observed != 0 || res.Groups != 0 || res.Healthy != 0 {
		t.Fatalf("an observer that read nothing must report nothing, got %+v", res)
	}

	res, err = syncer.Reconcile(ctx)
	if !errors.Is(err, crossnode.ErrNoRuleSource) {
		t.Fatalf("Reconcile without a rule source must return ErrNoRuleSource, got %v", err)
	}

	stats := syncer.Stats()
	if stats.SyncCount != 0 {
		t.Fatalf("a refused Sync must not increment SyncCount, got %d", stats.SyncCount)
	}
	if stats.RuleCount != 0 || stats.TrackingCount != 0 {
		t.Fatalf("no source means no observed state, got %+v", stats)
	}
	if !stats.ReconcilerConfigured {
		t.Fatal("reconciler was registered, Stats must say so")
	}
	if got := rec.TotalCalls(); got != 0 {
		t.Fatalf("convergence ran %d times with no rule source, want 0", got)
	}
	assertGatewayUntouched(t, gw, "no-rule-source refusal")
}

// TestIntegrationGateway_RuleSourceError_ReconcileDoesNotConverge pins that a
// failed read is a failed pass: the observer reports the source error verbatim
// (wrapped, never swallowed) and refuses to converge on a half-known route set.
func TestIntegrationGateway_RuleSourceError_ReconcileDoesNotConverge(t *testing.T) {
	rule := &trafficmanager.RoutingRule{
		ID: "err-1", Domain: "err.example.com", Path: "/",
		TargetHost: "10.0.0.1", TargetPort: 8080, Enabled: true,
	}
	f := newIngressFixture(t, 2, rule)
	f.health.RecordSuccess("10.0.0.1", 8080)

	sentinel := errors.New("routing rule store unavailable")
	f.rules.rulesErr = sentinel

	ctx := context.Background()
	res, err := f.syncer.Sync(ctx)
	if !errors.Is(err, sentinel) {
		t.Fatalf("Sync must surface the source error, got %v", err)
	}
	if res != (crossnode.SyncResult{}) {
		t.Fatalf("a failed read must not report a partial observation, got %+v", res)
	}

	if _, err := f.syncer.Reconcile(ctx); !errors.Is(err, sentinel) {
		t.Fatalf("Reconcile must surface the source error, got %v", err)
	}
	if got := f.rec.SyncCallCount(); got != 0 {
		t.Fatalf("Reconcile converged %d times on a failed read, want 0", got)
	}

	// A policy read that fails after the rules read is a failure too, not a
	// rules-only observation.
	f.rules.rulesErr = nil
	f.rules.policiesErr = sentinel
	if _, err := f.syncer.Sync(ctx); !errors.Is(err, sentinel) {
		t.Fatalf("policy read failure must fail the pass, got %v", err)
	}

	stats := f.syncer.Stats()
	if stats.SyncCount != 0 {
		t.Fatalf("failed passes must not count as syncs, got %d", stats.SyncCount)
	}
	if stats.ErrCount != 3 {
		t.Fatalf("ErrCount should be 3 (two Sync, one Reconcile), got %d", stats.ErrCount)
	}
	assertGatewayUntouched(t, f.gw, "failed rule reads")
}

// TestIntegrationGateway_EmptyAndDisabledObservation_TouchesNothing keeps the
// original no-wipe property one layer up: an empty or fully disabled rule set is
// observed as empty, crossnode never writes, and the decision whether a push is
// safe belongs to the rule owner (trafficmanager.Service.SyncRoutes reads its own
// live map, not crossnode's view).
func TestIntegrationGateway_EmptyAndDisabledObservation_TouchesNothing(t *testing.T) {
	f := newIngressFixture(t, 2,
		&trafficmanager.RoutingRule{ID: "disabled-1", Domain: "old.example.com", Path: "/", TargetHost: "10.0.0.1", TargetPort: 8080, Enabled: false},
		&trafficmanager.RoutingRule{ID: "disabled-2", Domain: "old.example.com", Path: "/", TargetHost: "10.0.0.2", TargetPort: 8080, Enabled: false},
	)

	ctx := context.Background()
	res, err := f.syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("observing a disabled-only set should not error: %v", err)
	}
	if res.Observed != 0 || res.Groups != 0 || res.Healthy != 0 || res.Skipped {
		t.Fatalf("disabled rules must not be observed as live, got %+v", res)
	}
	if got := len(f.syncer.CurrentRules()); got != 0 {
		t.Fatalf("CurrentRules should hold only enabled rules, got %d", got)
	}
	if got := f.syncer.RouteGenerationRecords(); len(got) != 0 {
		t.Fatalf("a disabled set must produce no route groups, got %v", got)
	}
	if got := f.rec.TotalCalls(); got != 0 {
		t.Fatalf("Sync must never converge, got %d reconciler calls", got)
	}
	assertGatewayUntouched(t, f.gw, "disabled-only observation")

	// An empty rule source behaves the same as a disabled one: an empty observation
	// is a fact about the source, never a gateway write from here.
	f.rules.setRules()
	res, err = f.syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("observing an empty source should not error: %v", err)
	}
	if res.Observed != 0 || res.Groups != 0 {
		t.Fatalf("empty source must observe empty, got %+v", res)
	}
	assertGatewayUntouched(t, f.gw, "empty observation")

	stats := f.syncer.Stats()
	if stats.SyncCount != 2 {
		t.Fatalf("both empty passes still completed, SyncCount=%d", stats.SyncCount)
	}
	if stats.ErrCount != 0 {
		t.Fatalf("an empty but readable set is not an error, ErrCount=%d", stats.ErrCount)
	}
}

// TestIntegrationGateway_HealthyBackends_ObservedAndGrouped is the positive path:
// two recorded-healthy backends on one route are observed as one group with two
// backends, and the observation is what the reconciler is asked to converge.
func TestIntegrationGateway_HealthyBackends_ObservedAndGrouped(t *testing.T) {
	f := newIngressFixture(t, 3,
		&trafficmanager.RoutingRule{ID: "healthy-1", Domain: "app.example.com", Path: "/", TargetHost: "10.0.0.10", TargetPort: 8080, Enabled: true, Weight: 1},
		&trafficmanager.RoutingRule{ID: "healthy-2", Domain: "app.example.com", Path: "/", TargetHost: "10.0.0.11", TargetPort: 8080, Enabled: true, Weight: 2},
	)
	f.health.RecordSuccess("10.0.0.10", 8080)
	f.health.RecordSuccess("10.0.0.11", 8080)

	ctx := context.Background()
	res, err := f.syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("healthy Sync should succeed: %v", err)
	}
	if res.Observed != 2 {
		t.Fatalf("Observed should be 2 enabled rules, got %d", res.Observed)
	}
	if res.Groups != 1 {
		t.Fatalf("two backends on the same domain/path are one route group, got %d", res.Groups)
	}
	if res.Healthy != 2 {
		t.Fatalf("both backends were probed healthy, Healthy=%d", res.Healthy)
	}

	records := f.syncer.RouteGenerationRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 route group, got %d", len(records))
	}
	if records[0].BackendCount != 2 {
		t.Fatalf("the group should carry both backends (primary + replica), got %d", records[0].BackendCount)
	}
	if got := fmt.Sprint(records[0].RuleIDs); got != "[healthy-1 healthy-2]" {
		t.Fatalf("group rule ids should be sorted and complete, got %s", got)
	}

	stats := f.syncer.Stats()
	if stats.SyncCount != 1 {
		t.Fatalf("SyncCount should be 1 after a successful sync, got %d", stats.SyncCount)
	}
	if stats.RuleCount != 2 {
		t.Fatalf("RuleCount should reflect the 2 observed rules, got %d", stats.RuleCount)
	}
	if stats.BackendCount != 2 {
		t.Fatalf("BackendCount should be the 2 probed backends, got %d", stats.BackendCount)
	}
	if stats.LastSync.IsZero() {
		t.Fatal("a completed Sync must record when it ran")
	}
	assertGatewayUntouched(t, f.gw, "healthy observation")

	// Convergence is a separate, delegated step: Reconcile asks the rule owner and
	// does not pretend to have synced on its own.
	res, err = f.syncer.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile with healthy backends should succeed: %v", err)
	}
	if res.Healthy != 2 {
		t.Fatalf("Reconcile must still report the observation, got %+v", res)
	}
	if got := f.rec.SyncCallCount(); got != 1 {
		t.Fatalf("Reconcile should delegate exactly once, got %d", got)
	}
	if got := f.syncer.Stats().SyncCount; got != 1 {
		t.Fatalf("Reconcile must not inflate SyncCount, got %d", got)
	}
	assertGatewayUntouched(t, f.gw, "after Reconcile")
}

// TestIntegrationGateway_PartialHealthy_OnlyHealthyCounted preserves the
// healthy/unhealthy backend selection: only a recorded, in-window healthy verdict
// counts as routable. Degraded, down and never-probed backends are all excluded —
// an unchecked backend is not a healthy one.
func TestIntegrationGateway_PartialHealthy_OnlyHealthyCounted(t *testing.T) {
	f := newIngressFixture(t, 2,
		&trafficmanager.RoutingRule{ID: "r-healthy", Domain: "svc.example.com", Path: "/", TargetHost: "good.internal", TargetPort: 8080, Enabled: true},
		&trafficmanager.RoutingRule{ID: "r-down", Domain: "svc.example.com", Path: "/", TargetHost: "bad.internal", TargetPort: 8080, Enabled: true},
		&trafficmanager.RoutingRule{ID: "r-unknown", Domain: "svc.example.com", Path: "/", TargetHost: "never-probed.internal", TargetPort: 8080, Enabled: true},
	)
	f.health.RecordSuccess("good.internal", 8080)
	f.health.RecordFailure("bad.internal", 8080, "connection refused")
	f.health.RecordFailure("bad.internal", 8080, "connection refused")

	ctx := context.Background()
	res, err := f.syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("partial-healthy Sync failed: %v", err)
	}
	if res.Observed != 3 {
		t.Fatalf("all 3 enabled rules are live routing state, Observed=%d", res.Observed)
	}
	if res.Healthy != 1 {
		t.Fatalf("only the recorded-healthy backend may count as routable, Healthy=%d", res.Healthy)
	}
	if res.Groups != 1 {
		t.Fatalf("expected 1 group, got %d", res.Groups)
	}

	// Observation is not health-filtered: the down and unknown targets stay visible
	// so the operator can see what the gateway would be asked to serve.
	if got := len(f.syncer.CurrentRules()); got != 3 {
		t.Fatalf("CurrentRules should keep all 3 enabled rules, got %d", got)
	}
	// Only probed backends appear in the health view; an unknown host has no record.
	backends := f.syncer.Backends()
	if len(backends) != 2 {
		t.Fatalf("Backends should list the 2 probed hosts, got %v", backends)
	}
	for _, b := range backends {
		if b.Host == "never-probed.internal" {
			t.Fatal("a never-probed backend must not carry a recorded verdict")
		}
	}
	if got := f.syncer.Stats().BackendCount; got != 2 {
		t.Fatalf("BackendCount should be the probed backends, got %d", got)
	}
	// The group still names all 3 backends: health is a routing decision made by
	// FilterHealthy, not something that erases a configured target.
	if got := f.syncer.RouteGenerationRecords()[0].BackendCount; got != 3 {
		t.Fatalf("group should report all 3 configured backends, got %d", got)
	}
	assertGatewayUntouched(t, f.gw, "partial-healthy observation")
}

// TestIntegrationGateway_EmptyTargetHost_NotRoutable pins the routegroup change
// that removed the localhost substitution: a rule with no target host contributes
// no backend. A guessed address would look routable and send cross-node traffic
// to the control-plane machine.
func TestIntegrationGateway_EmptyTargetHost_NotRoutable(t *testing.T) {
	f := newIngressFixture(t, 2,
		&trafficmanager.RoutingRule{ID: "named", Domain: "host.example.com", Path: "/", TargetHost: "10.0.0.7", TargetPort: 8080, Enabled: true},
		&trafficmanager.RoutingRule{ID: "unnamed", Domain: "host.example.com", Path: "/", TargetHost: "", TargetPort: 8080, Enabled: true},
	)
	f.health.RecordSuccess("10.0.0.7", 8080)

	res, err := f.syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	if res.Observed != 2 {
		t.Fatalf("both enabled rules are observed, got %d", res.Observed)
	}
	if res.Healthy != 1 {
		t.Fatalf("only the named backend can be healthy, got %d", res.Healthy)
	}

	records := f.syncer.RouteGenerationRecords()
	if len(records) != 1 || records[0].BackendCount != 1 {
		t.Fatalf("an empty TargetHost must contribute no backend, got %+v", records)
	}
	for _, b := range f.syncer.Backends() {
		if b.Host == "localhost" || b.Host == "127.0.0.1" {
			t.Fatalf("no backend may be substituted for an unresolved target, got %+v", b)
		}
	}
	assertGatewayUntouched(t, f.gw, "empty target host")
}

// TestIntegrationGateway_DiscoveryVerdicts_ReplaceStaleHealth preserves backend
// health selection at its source: with an endpoint attached, discovery status
// drives the verdict, and a backend whose endpoint disappeared goes back to
// unknown instead of lingering as healthy.
func TestIntegrationGateway_DiscoveryVerdicts_ReplaceStaleHealth(t *testing.T) {
	f := newIngressFixture(t, 2,
		&trafficmanager.RoutingRule{ID: "disc-1", Domain: "disc.example.com", Path: "/", TargetHost: "10.0.0.5", TargetPort: 8080, Enabled: true},
	)
	endpoints := &mockEndpointSource{}
	f.syncer.SetEndpointSource(endpoints)

	ctx := context.Background()
	addr := netip.MustParseAddr("10.0.0.5")
	up := servicediscovery.ServiceEndpoint{ID: "ep-1", Address: addr, Port: 8080, Status: servicediscovery.EndpointStatusHealthy}

	endpoints.set(up)
	res, err := f.syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync with a healthy endpoint: %v", err)
	}
	if res.Healthy != 1 {
		t.Fatalf("a healthy endpoint should be routable, Healthy=%d", res.Healthy)
	}
	if got := len(f.syncer.Backends()); got != 1 {
		t.Fatalf("the probed backend should be the only record, got %d", got)
	}
	if !f.health.IsHealthy("10.0.0.5", 8080) {
		t.Fatal("discovery marked the healthy endpoint unroutable")
	}

	// The endpoint goes unhealthy: the stale healthy reading is replaced, not kept.
	endpoints.set(servicediscovery.ServiceEndpoint{ID: "ep-1", Address: addr, Port: 8080, Status: servicediscovery.EndpointStatusUnhealthy})
	res, err = f.syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync with an unhealthy endpoint: %v", err)
	}
	if res.Healthy != 0 {
		t.Fatalf("an unhealthy endpoint must not be counted routable, Healthy=%d", res.Healthy)
	}
	if f.health.IsHealthy("10.0.0.5", 8080) {
		t.Fatal("a stale healthy reading survived the failure")
	}
	// A second pass over the failure crosses the threshold and reads DOWN.
	if _, err := f.syncer.Sync(ctx); err != nil {
		t.Fatalf("repeat Sync: %v", err)
	}
	if desc := f.syncer.DescribeBackend("10.0.0.5", 8080); !containsSub(desc, "DOWN") {
		t.Fatalf("repeated discovery failures should mark the backend DOWN, got %q", desc)
	}

	// The endpoint disappears: unknown, not healthy and not a recorded backend.
	endpoints.set()
	res, err = f.syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync after the endpoint vanished: %v", err)
	}
	if res.Healthy != 0 {
		t.Fatalf("a removed endpoint must not stay routable, Healthy=%d", res.Healthy)
	}
	if got := f.syncer.Stats().BackendCount; got != 0 {
		t.Fatalf("a backend with no endpoint must stop being tracked, BackendCount=%d", got)
	}
	if desc := f.syncer.DescribeBackend("10.0.0.5", 8080); !containsSub(desc, "UNKNOWN") {
		t.Fatalf("a forgotten backend must read UNKNOWN, got %q", desc)
	}
	assertGatewayUntouched(t, f.gw, "discovery-driven health")
}

// TestIntegrationGateway_SourceChanges_ObservationFollowsWithoutWriting replaces
// the deleted UpsertRule/RemoveRule mutators: the synchronizer tracks the rule
// owner's set by re-reading it, and removal down to empty is still not a write.
func TestIntegrationGateway_SourceChanges_ObservationFollowsWithoutWriting(t *testing.T) {
	ruleA := &trafficmanager.RoutingRule{ID: "rule-1", Domain: "a.example.com", Path: "/", TargetHost: "10.0.0.1", TargetPort: 8080, Enabled: true}
	ruleB := &trafficmanager.RoutingRule{ID: "rule-2", Domain: "b.example.com", Path: "/", TargetHost: "10.0.0.2", TargetPort: 9090, Enabled: true}

	f := newIngressFixture(t, 2, ruleA)
	f.health.RecordSuccess("10.0.0.1", 8080)
	f.health.RecordSuccess("10.0.0.2", 9090)

	ctx := context.Background()
	res, err := f.syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	if res.Observed != 1 || res.Groups != 1 || res.Healthy != 1 {
		t.Fatalf("initial observation should be 1/1/1, got %+v", res)
	}

	// A second rule on a different domain adds a group.
	f.rules.addRule(ruleB)
	res, err = f.syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("sync after adding a rule: %v", err)
	}
	if res.Observed != 2 || res.Groups != 2 {
		t.Fatalf("expected 2 rules in 2 groups, got %+v", res)
	}
	if got := f.syncer.Stats().TrackingCount; got != 2 {
		t.Fatalf("TrackingCount should follow the source to 2, got %d", got)
	}

	// Removing a rule is reflected on the next read, and the surviving group is the
	// one that was left behind.
	f.rules.removeRule("rule-1")
	res, err = f.syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("sync after removing a rule: %v", err)
	}
	records := f.syncer.RouteGenerationRecords()
	if res.Observed != 1 || len(records) != 1 {
		t.Fatalf("expected 1 rule and 1 group after removal, got %+v / %+v", res, records)
	}
	if records[0].GroupID != "b.example.com///http" {
		t.Fatalf("the surviving route should be b.example.com, got %q", records[0].GroupID)
	}

	// Down to empty: the pass succeeds, the view goes empty, and the gateway is
	// still never asked to replace its config with nothing.
	f.rules.removeRule("rule-2")
	res, err = f.syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("sync after removing the last rule: %v", err)
	}
	if res.Observed != 0 || res.Groups != 0 || res.Healthy != 0 {
		t.Fatalf("empty set must observe empty, got %+v", res)
	}
	if got := len(f.syncer.RouteGenerationRecords()); got != 0 {
		t.Fatalf("route records should be cleared, got %d", got)
	}
	if got := f.rules.count(); got != 0 {
		t.Fatalf("source should be empty, got %d rules", got)
	}
	if got := f.rules.listCallCount(); got != 4 {
		t.Fatalf("each pass must re-read the live source, got %d reads", got)
	}
	if got := f.rec.TotalCalls(); got != 0 {
		t.Fatalf("Sync alone never converges, got %d reconciler calls", got)
	}
	assertGatewayUntouched(t, f.gw, "source changes")
}

// TestIntegrationGateway_CleanupStale_DelegatesAndPropagatesFailure is the button
// that caused the P0. It may only ask the rule owner to clean up, using the
// owner's live active set, and a failure must not be reported as success.
func TestIntegrationGateway_CleanupStale_DelegatesAndPropagatesFailure(t *testing.T) {
	f := newIngressFixture(t, 2,
		&trafficmanager.RoutingRule{ID: "keep-1", Domain: "keep.example.com", Path: "/", TargetHost: "10.0.0.1", TargetPort: 8080, Enabled: true},
	)
	f.health.RecordSuccess("10.0.0.1", 8080)

	ctx := context.Background()
	if err := f.syncer.CleanupStale(ctx); err != nil {
		t.Fatalf("delegated cleanup should succeed: %v", err)
	}
	if got := f.rec.CleanupCallCount(); got != 1 {
		t.Fatalf("CleanupStale should delegate exactly once, got %d", got)
	}
	// crossnode passes no active set at all, so it cannot withdraw what it never
	// read: the gateway is not touched directly, and nothing is withdrawn.
	if got := len(f.gw.WithdrawnIDs()); got != 0 {
		t.Fatalf("%d route ids withdrawn by the observer", got)
	}
	if got := f.syncer.Stats().ErrCount; got != 0 {
		t.Fatalf("a successful cleanup is not an error, ErrCount=%d", got)
	}
	assertGatewayUntouched(t, f.gw, "after delegated cleanup")

	sentinel := errors.New("gateway rejected the active set")
	f.rec.cleanupErr = sentinel
	err := f.syncer.CleanupStale(ctx)
	if !errors.Is(err, sentinel) {
		t.Fatalf("a failed cleanup must surface the reconciler error, got %v", err)
	}
	if f.rec.CleanupCallCount() != 2 {
		t.Fatalf("the second attempt should still reach the reconciler, got %d", f.rec.CleanupCallCount())
	}
	if got := f.syncer.Stats().ErrCount; got != 1 {
		t.Fatalf("ErrCount should record the failed cleanup, got %d", got)
	}
	assertGatewayUntouched(t, f.gw, "after failed delegated cleanup")

	// A converger failure in Reconcile propagates the same way.
	f.rec.syncErr = errors.New("caddy admin api unreachable")
	if _, err := f.syncer.Reconcile(ctx); err == nil {
		t.Fatal("Reconcile must report a failed gateway convergence")
	} else if !containsSub(err.Error(), "caddy admin api unreachable") {
		t.Fatalf("Reconcile should wrap the reconciler error, got %v", err)
	}
	assertGatewayUntouched(t, f.gw, "after failed Reconcile")
}

// TestIntegrationGateway_DescribeBackend_UnknownDegradedDownHealthy pins the
// description progression, including the change that a never-probed backend is
// UNKNOWN: it used to answer "is HEALTHY" for a backend nobody had checked.
func TestIntegrationGateway_DescribeBackend_UnknownDegradedDownHealthy(t *testing.T) {
	gw := &mockGateway{}
	health := crossnode.NewHealthFilter(2, 30*time.Second)
	syncer := crossnode.NewIngressSynchronizer(gw, health)

	desc := syncer.DescribeBackend("newhost.internal", 8080)
	if desc == "" {
		t.Fatal("DescribeBackend should return non-empty for an unknown host")
	}
	if !containsSub(desc, "UNKNOWN") {
		t.Fatalf("a never-probed backend must be UNKNOWN, got %q", desc)
	}
	if containsSub(desc, "HEALTHY") {
		t.Fatalf("a never-probed backend must not be described as healthy, got %q", desc)
	}

	// One failure below the threshold is a degraded reading, not a down one.
	health.RecordFailure("newhost.internal", 8080, "timeout")
	desc = syncer.DescribeBackend("newhost.internal", 8080)
	if !containsSub(desc, "DEGRADED") {
		t.Fatalf("first failure below threshold should be DEGRADED, got %q", desc)
	}
	if containsSub(desc, "HEALTHY") || containsSub(desc, "UNKNOWN") {
		t.Fatalf("a failing backend must not read healthy or unknown, got %q", desc)
	}

	// Crossing the threshold flips it to DOWN, with the failure count reported.
	health.RecordFailure("newhost.internal", 8080, "timeout")
	desc = syncer.DescribeBackend("newhost.internal", 8080)
	if !containsSub(desc, "DOWN") {
		t.Fatalf("expected DOWN after the threshold, got %q", desc)
	}
	if !containsSub(desc, "timeout") {
		t.Fatalf("the recorded reason should be reported, got %q", desc)
	}

	// A recovered probe replaces the failure verdict; describe never caches.
	health.RecordSuccess("newhost.internal", 8080)
	desc = syncer.DescribeBackend("newhost.internal", 8080)
	if !containsSub(desc, "HEALTHY") || containsSub(desc, "DEGRADED") {
		t.Fatalf("a recovered backend should read HEALTHY, got %q", desc)
	}
	if got := len(syncer.Backends()); got != 1 {
		t.Fatalf("the probed backend should be tracked exactly once, got %d", got)
	}
	assertGatewayUntouched(t, gw, "backend description")
}

// TestIntegrationGateway_RouteGenerationTracked checks that a successful pass
// publishes one record per route group, sorted and stable, with the group's
// members attributed. GetRouteGenerationRecord is gone: the sorted slice is the
// observation surface.
func TestIntegrationGateway_RouteGenerationTracked(t *testing.T) {
	f := newIngressFixture(t, 2,
		&trafficmanager.RoutingRule{ID: "track-1", Domain: "track.example.com", Path: "/", TargetHost: "10.0.0.1", TargetPort: 8080, ServerID: "srv-a", Enabled: true},
		&trafficmanager.RoutingRule{ID: "track-2", Domain: "track2.example.com", Path: "/api", TargetHost: "10.0.0.2", TargetPort: 8080, ServerID: "srv-b", Enabled: true, WebSocket: true, Strategy: "ip_hash"},
	)
	f.health.RecordSuccess("10.0.0.1", 8080)
	f.health.RecordSuccess("10.0.0.2", 8080)

	ctx := context.Background()
	res, err := f.syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Groups != 2 {
		t.Fatalf("2 distinct routes should give 2 groups, got %d", res.Groups)
	}

	records := f.syncer.RouteGenerationRecords()
	if len(records) != 2 {
		t.Fatalf("expected 2 route generation records (2 groups), got %d", len(records))
	}
	for i, rec := range records {
		if i > 0 && records[i-1].GroupID >= rec.GroupID {
			t.Fatalf("records must be sorted by group id, got %v", records)
		}
		if rec.GroupID == "" {
			t.Fatalf("record %d has an empty group id: %+v", i, rec)
		}
		if rec.BackendCount != 1 {
			t.Fatalf("each group has 1 configured backend, got %+v", rec)
		}
		if len(rec.RuleIDs) != 1 || len(rec.ServerIDs) != 1 {
			t.Fatalf("record should attribute its rule and server, got %+v", rec)
		}
	}

	byID := map[string]crossnode.RouteGenerationRecord{}
	for _, rec := range records {
		byID[rec.GroupID] = rec
	}
	if rec := byID["track2.example.com//api/http"]; !rec.HasWebSocket || rec.Strategy != "ip_hash" {
		t.Fatalf("the websocket/ip_hash route lost its attributes: %+v", rec)
	}
	// GroupID is domain/path/protocol with no path normalization, so a root
	// path keeps its slash: track.example.com + / + / + /http.
	if rec := byID["track.example.com///http"]; rec.HasWebSocket || rec.Strategy != "round_robin" {
		t.Fatalf("the plain route inherited attributes: %+v", rec)
	}

	// Re-reading replaces the view instead of accumulating generations of it.
	if _, err := f.syncer.Sync(ctx); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if got := len(f.syncer.RouteGenerationRecords()); got != 2 {
		t.Fatalf("records should be rebuilt, not appended to, got %d", got)
	}
	stats := f.syncer.Stats()
	if stats.TrackingCount != 2 {
		t.Fatalf("TrackingCount should be 2, got %d", stats.TrackingCount)
	}
	if stats.PolicyCount != 0 {
		t.Fatalf("no policies were in the source, PolicyCount=%d", stats.PolicyCount)
	}
	assertGatewayUntouched(t, f.gw, "route generation tracking")

	// Policies observed from the source are attributed too.
	f.rules.setPolicies(&trafficmanager.TrafficPolicy{ID: "pol-1", Name: "rate-limit", RateLimit: 100, TLSEnabled: true})
	if _, err := f.syncer.Sync(ctx); err != nil {
		t.Fatalf("sync with policies: %v", err)
	}
	if got := f.syncer.Stats().PolicyCount; got != 1 {
		t.Fatalf("PolicyCount should follow the source to 1, got %d", got)
	}
	if got := len(f.syncer.CurrentPolicies()); got != 1 {
		t.Fatalf("CurrentPolicies should expose the observed policy, got %d", got)
	}
	assertGatewayUntouched(t, f.gw, "observed policies")
}

// TestIntegrationGateway_ConcurrentSourceAndSync_NoWrites keeps the single-writer
// guarantee, which is now stronger than it was: concurrent source mutations,
// observations and delegated reconciles still produce no gateway write at all,
// and each Reconcile delegates exactly once (run under -race).
func TestIntegrationGateway_ConcurrentSourceAndSync_NoWrites(t *testing.T) {
	f := newIngressFixture(t, 2,
		&trafficmanager.RoutingRule{ID: "concurrent-base", Domain: "base.example.com", Path: "/", TargetHost: "10.0.0.1", TargetPort: 8080, Enabled: true},
	)
	f.health.RecordSuccess("10.0.0.1", 8080)

	ctx := context.Background()
	const passes = 5

	var wg sync.WaitGroup
	errs := make(chan error, passes*2)
	for i := 0; i < passes; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			f.rules.addRule(&trafficmanager.RoutingRule{
				ID:         fmt.Sprintf("concurrent-%d", idx),
				Domain:     fmt.Sprintf("c%d.example.com", idx),
				Path:       "/",
				TargetHost: "10.0.0.1",
				TargetPort: 8080,
				// One rule is added disabled: concurrent observers must never treat
				// it as live routing state.
				Enabled: idx != 2,
			})
			if _, err := f.syncer.Sync(ctx); err != nil {
				errs <- err
				return
			}
			_, err := f.syncer.Reconcile(ctx)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Sync/Reconcile failed: %v", err)
		}
	}

	if got := f.rules.count(); got != passes+1 {
		t.Fatalf("every concurrent add should land, source holds %d", got)
	}
	stats := f.syncer.Stats()
	if stats.SyncCount != passes {
		t.Fatalf("SyncCount should be %d, got %d", passes, stats.SyncCount)
	}
	if got := f.rec.SyncCallCount(); got != passes {
		t.Fatalf("each Reconcile must delegate exactly once, got %d of %d", got, passes)
	}
	if got := f.gw.NonHealthCount(); got != 0 {
		t.Fatalf("concurrent passes wrote to the gateway %d times", got)
	}
	// The published view is a snapshot of one pass, not a torn mix of several.
	if got := len(f.syncer.CurrentRules()); got != stats.RuleCount {
		t.Fatalf("CurrentRules (%d) disagrees with Stats().RuleCount (%d)", got, stats.RuleCount)
	}

	// Quiesced: the final pass must see the whole source, minus the disabled rule.
	res, err := f.syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("final sync: %v", err)
	}
	if res.Observed != passes {
		t.Fatalf("expected %d enabled rules observed, got %+v", passes, res)
	}
	if res.Groups != passes {
		t.Fatalf("each domain is its own route group, got %+v", res)
	}
	if res.Healthy != passes {
		t.Fatalf("the one probed backend serves every group, got %+v", res)
	}
	if got := len(f.syncer.RouteGenerationRecords()); got != passes {
		t.Fatalf("expected %d route records, got %d", passes, got)
	}
	assertGatewayUntouched(t, f.gw, "concurrent passes")
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}