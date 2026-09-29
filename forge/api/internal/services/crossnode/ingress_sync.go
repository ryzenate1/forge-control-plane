package crossnode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sort"
	"sync"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/services/servicediscovery"
	"gamepanel/forge/internal/services/trafficmanager"
)

var (
	ErrNoRuleSource = errors.New("crossnode: no routing rule source configured")
	ErrNoReconciler = errors.New("crossnode: no gateway reconciler configured")
)

// gatewayReader is the only gateway surface crossnode may touch. trafficmanager
// owns the sole full-replace writer on the shared Caddy admin API, so a second
// writer here would withdraw live routes on every pass.
type gatewayReader interface {
	Health(ctx context.Context) trafficmanager.AdapterHealth
}

type RuleSource interface {
	ListRoutingRules(ctx context.Context) ([]*trafficmanager.RoutingRule, error)
	ListTrafficPolicies(ctx context.Context) ([]*trafficmanager.TrafficPolicy, error)
}

type GatewayReconciler interface {
	SyncRoutes(ctx context.Context) error
	CleanupStaleRoutes(ctx context.Context) error
}

type EndpointSource interface {
	ListEndpoints(ctx context.Context, filter servicediscovery.EndpointFilter) []servicediscovery.ServiceEndpoint
}

type IngressSynchronizer struct {
	reader    gatewayReader
	health    *HealthFilter
	publisher events.Publisher

	ruleSource RuleSource
	reconciler GatewayReconciler
	endpoints  EndpointSource

	mu        sync.RWMutex
	rules     []*trafficmanager.RoutingRule
	policies  map[string]*trafficmanager.TrafficPolicy
	tracking  map[string]RouteGenerationRecord
	lastSync  time.Time
	syncCount int
	errCount  int
	running   bool
	cancel    context.CancelFunc
}

func NewIngressSynchronizer(reader gatewayReader, health *HealthFilter, publishers ...events.Publisher) *IngressSynchronizer {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &IngressSynchronizer{
		reader:    reader,
		health:    health,
		publisher: publisher,
		policies:  make(map[string]*trafficmanager.TrafficPolicy),
		tracking:  make(map[string]RouteGenerationRecord),
	}
}

// SetReconciler supplies the live routing state and the component allowed to
// converge the gateway. In production both are trafficmanager.Service.
func (is *IngressSynchronizer) SetReconciler(src RuleSource, rec GatewayReconciler) {
	is.mu.Lock()
	defer is.mu.Unlock()
	is.ruleSource = src
	is.reconciler = rec
}

func (is *IngressSynchronizer) SetEndpointSource(src EndpointSource) {
	is.mu.Lock()
	defer is.mu.Unlock()
	is.endpoints = src
}

func (is *IngressSynchronizer) ReconcilerConfigured() bool {
	is.mu.RLock()
	defer is.mu.RUnlock()
	return is.reconciler != nil
}

func (is *IngressSynchronizer) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		slog.Error("ingress synchronizer not started: interval must be positive", "interval", interval)
		return
	}

	is.mu.Lock()
	if is.running {
		is.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	is.cancel = cancel
	is.running = true
	is.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 4096)
				n := runtime.Stack(buf, false)
				slog.Error("ingress sync panic recovered", "panic", r, "stack", string(buf[:n]))
			}
		}()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		var lastErr error

		for {
			select {
			case <-ctx.Done():
				is.mu.Lock()
				is.running = false
				is.mu.Unlock()
				return
			case <-ticker.C:
				_, err := is.Sync(ctx)
				if err != nil {
					if lastErr == nil {
						slog.Error("ingress observe failed", "error", err)
					} else {
						slog.Debug("ingress observe still failing", "error", err)
					}
					lastErr = err
				} else {
					lastErr = nil
				}
			}
		}
	}()
	slog.Info("ingress synchronizer started", "interval", interval)
}

func (is *IngressSynchronizer) Stop() {
	is.mu.Lock()
	defer is.mu.Unlock()
	if is.cancel != nil {
		is.cancel()
		is.cancel = nil
	}
	is.running = false
}

// Sync refreshes the observation view only. It never writes to a gateway.
func (is *IngressSynchronizer) Sync(ctx context.Context) (SyncResult, error) {
	is.mu.RLock()
	src := is.ruleSource
	is.mu.RUnlock()

	result, err := is.observe(ctx, src)
	if err != nil {
		is.mu.Lock()
		is.errCount++
		is.mu.Unlock()
		return result, err
	}

	is.mu.Lock()
	is.lastSync = time.Now()
	is.syncCount++
	syncCount := is.syncCount
	is.mu.Unlock()

	if is.publisher != nil {
		if perr := is.publisher.Publish(ctx, events.NewEnvelope(
			"ingress_synced",
			"crossnode",
			"ingress",
			"",
			map[string]any{
				"routes":          result.Observed,
				"groups":          result.Groups,
				"healthyBackends": result.Healthy,
				"syncCount":       syncCount,
			},
		)); perr != nil {
			slog.Warn("ingress observe event publish failed", "error", perr)
		}
	}
	return result, nil
}

// Reconcile observes, then asks the rule owner to converge the gateway.
// crossnode deliberately cannot perform that step itself.
func (is *IngressSynchronizer) Reconcile(ctx context.Context) (SyncResult, error) {
	is.mu.RLock()
	src := is.ruleSource
	rec := is.reconciler
	is.mu.RUnlock()

	if rec == nil {
		return SyncResult{Skipped: true, Reason: ErrNoReconciler.Error()}, ErrNoReconciler
	}

	result, err := is.observe(ctx, src)
	if err != nil {
		is.mu.Lock()
		is.errCount++
		is.mu.Unlock()
		return result, err
	}

	if err := rec.SyncRoutes(ctx); err != nil {
		is.mu.Lock()
		is.errCount++
		is.mu.Unlock()
		return result, fmt.Errorf("crossnode: gateway reconcile: %w", err)
	}
	return result, nil
}

func (is *IngressSynchronizer) observe(ctx context.Context, src RuleSource) (SyncResult, error) {
	if src == nil {
		return SyncResult{Skipped: true, Reason: ErrNoRuleSource.Error()}, ErrNoRuleSource
	}

	rules, err := src.ListRoutingRules(ctx)
	if err != nil {
		return SyncResult{}, fmt.Errorf("crossnode: list routing rules: %w", err)
	}
	policies, err := src.ListTrafficPolicies(ctx)
	if err != nil {
		return SyncResult{}, fmt.Errorf("crossnode: list traffic policies: %w", err)
	}

	enabled := make([]*trafficmanager.RoutingRule, 0, len(rules))
	for _, rule := range rules {
		if rule == nil || !rule.Enabled {
			continue
		}
		enabled = append(enabled, rule)
	}
	sort.Slice(enabled, func(i, j int) bool { return enabled[i].ID < enabled[j].ID })

	is.refreshHealthFromDiscovery(ctx)

	groups := GroupRulesByRoute(enabled)
	healthyBackends := 0
	for _, grp := range groups {
		healthyBackends += len(is.health.FilterHealthy(grp.UniqueBackends()))
	}

	policyByID := make(map[string]*trafficmanager.TrafficPolicy, len(policies))
	for _, p := range policies {
		if p != nil {
			policyByID[p.ID] = p
		}
	}

	records := BuildRouteGenerationRecords(groups)
	tracking := make(map[string]RouteGenerationRecord, len(records))
	for _, rec := range records {
		tracking[rec.GroupID] = rec
	}

	is.mu.Lock()
	is.rules = enabled
	is.policies = policyByID
	is.tracking = tracking
	is.mu.Unlock()

	return SyncResult{
		Observed: len(enabled),
		Groups:   len(groups),
		Healthy:  healthyBackends,
	}, nil
}

// refreshHealthFromDiscovery replaces every recorded verdict with what the prober
// currently believes, so a backend whose endpoint disappeared cannot keep
// reporting healthy from a stale reading.
func (is *IngressSynchronizer) refreshHealthFromDiscovery(ctx context.Context) {
	is.mu.RLock()
	src := is.endpoints
	is.mu.RUnlock()
	if src == nil {
		return
	}

	endpoints := src.ListEndpoints(ctx, servicediscovery.EndpointFilter{})
	seen := make(map[string]bool, len(endpoints))
	for _, ep := range endpoints {
		if !ep.Address.IsValid() || ep.Port <= 0 {
			continue
		}
		host := ep.Address.String()
		seen[backendKey(host, ep.Port)] = true

		switch ep.Status {
		case servicediscovery.EndpointStatusHealthy:
			is.health.RecordSuccess(host, ep.Port)
		case servicediscovery.EndpointStatusUnhealthy, servicediscovery.EndpointStatusDraining:
			is.health.RecordFailure(host, ep.Port, "discovery status "+string(ep.Status))
		default:
			is.health.MarkUnknown(host, ep.Port)
		}
	}

	for _, tracked := range is.health.GetAllHealth() {
		if !seen[backendKey(tracked.Host, tracked.Port)] {
			is.health.MarkUnknown(tracked.Host, tracked.Port)
		}
	}
}

// CleanupStale delegates to the rule owner, which knows the real active set.
// Passing crossnode's own (often empty) set straight to the gateway would
// withdraw every live route.
func (is *IngressSynchronizer) CleanupStale(ctx context.Context) error {
	is.mu.RLock()
	rec := is.reconciler
	is.mu.RUnlock()

	if rec == nil {
		is.mu.Lock()
		is.errCount++
		is.mu.Unlock()
		return ErrNoReconciler
	}
	if err := rec.CleanupStaleRoutes(ctx); err != nil {
		is.mu.Lock()
		is.errCount++
		is.mu.Unlock()
		return fmt.Errorf("crossnode: cleanup stale routes: %w", err)
	}
	return nil
}

func (is *IngressSynchronizer) Health(ctx context.Context) trafficmanager.AdapterHealth {
	if is.reader == nil {
		return trafficmanager.AdapterHealth{Message: "gateway reader not configured"}
	}
	return is.reader.Health(ctx)
}

func (is *IngressSynchronizer) RouteGenerationRecords() []RouteGenerationRecord {
	is.mu.RLock()
	defer is.mu.RUnlock()
	records := make([]RouteGenerationRecord, 0, len(is.tracking))
	for _, rec := range is.tracking {
		records = append(records, rec)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].GroupID < records[j].GroupID })
	return records
}

func (is *IngressSynchronizer) Stats() IngressSyncStats {
	is.mu.RLock()
	defer is.mu.RUnlock()
	return IngressSyncStats{
		RuleCount:            len(is.rules),
		PolicyCount:          len(is.policies),
		TrackingCount:        len(is.tracking),
		BackendCount:         is.health.Count(),
		LastSync:             is.lastSync,
		SyncCount:            is.syncCount,
		ErrCount:             is.errCount,
		Running:              is.running,
		ReconcilerConfigured: is.reconciler != nil,
	}
}

// CurrentRules returns the enabled routing rules most recently observed.
func (is *IngressSynchronizer) CurrentRules() []*trafficmanager.RoutingRule {
	is.mu.RLock()
	defer is.mu.RUnlock()
	out := make([]*trafficmanager.RoutingRule, len(is.rules))
	copy(out, is.rules)
	return out
}

// CurrentPolicies returns a snapshot of the traffic policies keyed by policy id.
func (is *IngressSynchronizer) CurrentPolicies() map[string]*trafficmanager.TrafficPolicy {
	is.mu.RLock()
	defer is.mu.RUnlock()
	out := make(map[string]*trafficmanager.TrafficPolicy, len(is.policies))
	for k, v := range is.policies {
		out[k] = v
	}
	return out
}

// Backends exposes the tracked cross-node backends and their recorded verdicts.
func (is *IngressSynchronizer) Backends() []BackendHealth {
	return is.health.GetAllHealth()
}

type SyncResult struct {
	Observed int    `json:"observedRules"`
	Groups   int    `json:"groups"`
	Healthy  int    `json:"healthyBackends"`
	Skipped  bool   `json:"skipped"`
	Reason   string `json:"reason,omitempty"`
}

type IngressSyncStats struct {
	RuleCount            int       `json:"ruleCount"`
	PolicyCount          int       `json:"policyCount"`
	TrackingCount        int       `json:"trackingCount"`
	BackendCount         int       `json:"backendCount"`
	LastSync             time.Time `json:"lastSync"`
	SyncCount            int       `json:"syncCount"`
	ErrCount             int       `json:"errCount"`
	Running              bool      `json:"running"`
	ReconcilerConfigured bool      `json:"reconcilerConfigured"`
}

func (is *IngressSynchronizer) DescribeBackend(host string, port int) string {
	health := is.health.GetHealth(host, port)
	switch health.Status {
	case HealthDown:
		return fmt.Sprintf("backend %s:%d is DOWN (failures: %d, reason: %s)", host, port, health.FailCount, health.Reason)
	case HealthDegraded:
		return fmt.Sprintf("backend %s:%d is DEGRADED (failures: %d, reason: %s)", host, port, health.FailCount, health.Reason)
	case HealthHealthy:
		return fmt.Sprintf("backend %s:%d is HEALTHY", host, port)
	default:
		return fmt.Sprintf("backend %s:%d is UNKNOWN (never probed)", host, port)
	}
}
