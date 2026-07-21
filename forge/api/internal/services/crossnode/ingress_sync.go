package crossnode

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/services/trafficmanager"
)

type gatewayAdapter interface {
	UpdateRoutes(ctx context.Context, rules []*trafficmanager.RoutingRule, policies map[string]*trafficmanager.TrafficPolicy) error
	RemoveRoutes(ctx context.Context, ruleIDs []string) error
	CleanupStale(ctx context.Context, activeRuleIDs map[string]bool) error
	Reload(ctx context.Context) error
	Health(ctx context.Context) trafficmanager.AdapterHealth
}

type IngressSynchronizer struct {
	adapter     gatewayAdapter
	resolver    *Resolver
	health      *HealthFilter
	publisher   events.Publisher
	mu          sync.RWMutex
	rules       map[string]*trafficmanager.RoutingRule
	policies    map[string]*trafficmanager.TrafficPolicy
	tracking    map[string]RouteGenerationRecord
	lastSync    time.Time
	syncCount   int
	errCount    int
	running     bool
	cancel      context.CancelFunc
}

func NewIngressSynchronizer(adapter gatewayAdapter, resolver *Resolver, health *HealthFilter, publishers ...events.Publisher) *IngressSynchronizer {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &IngressSynchronizer{
		adapter:   adapter,
		resolver:  resolver,
		health:    health,
		publisher: publisher,
		rules:     make(map[string]*trafficmanager.RoutingRule),
		policies:  make(map[string]*trafficmanager.TrafficPolicy),
		tracking:  make(map[string]RouteGenerationRecord),
	}
}

func (is *IngressSynchronizer) Start(ctx context.Context, interval time.Duration) {
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
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				is.mu.Lock()
				is.running = false
				is.mu.Unlock()
				return
			case <-ticker.C:
				if err := is.Sync(ctx); err != nil {
					slog.Error("ingress sync failed", "error", err)
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
	}
	is.running = false
}

func (is *IngressSynchronizer) Sync(ctx context.Context) error {
	is.mu.RLock()
	rules := make([]*trafficmanager.RoutingRule, 0, len(is.rules))
	for _, rule := range is.rules {
		if rule.Enabled {
			rules = append(rules, rule)
		}
	}
	policies := make(map[string]*trafficmanager.TrafficPolicy, len(is.policies))
	for k, v := range is.policies {
		policies[k] = v
	}
	is.mu.RUnlock()

	groups := GroupRulesByRoute(rules)
	var mergedRules []*trafficmanager.RoutingRule

	for key, grp := range groups {
		backends := grp.UniqueBackends()
		healthyBackends := is.health.FilterHealthy(backends)

		if len(healthyBackends) == 0 {
			slog.Warn("no healthy backends for route, using all backends",
				"domain", key.Domain, "path", key.Path)
			healthyBackends = backends
		}

		primary := grp.Rules[0]
		primary.TargetHost = healthyBackends[0].Host
		primary.TargetPort = healthyBackends[0].Port

		mergedRules = append(mergedRules, primary)

		for i := 1; i < len(healthyBackends); i++ {
			replicaRule := &trafficmanager.RoutingRule{
				ID:         primary.ID + "-replica-" + itoa(i),
				Name:       primary.Name + "-replica-" + itoa(i),
				ServerID:   primary.ServerID,
				Domain:     primary.Domain,
				Path:       primary.Path,
				TargetHost: healthyBackends[i].Host,
				TargetPort: healthyBackends[i].Port,
				Protocol:   primary.Protocol,
				Strategy:   grp.Strategy(),
				Weight:     healthyBackends[i].Weight,
				Enabled:    true,
				WebSocket:  primary.WebSocket,
			}
			if replicaRule.Weight <= 0 {
				replicaRule.Weight = 1
			}
			mergedRules = append(mergedRules, replicaRule)
		}
	}

	if err := is.adapter.UpdateRoutes(ctx, mergedRules, policies); err != nil {
		is.mu.Lock()
		is.errCount++
		is.mu.Unlock()
		return fmt.Errorf("ingress sync update routes: %w", err)
	}

	records := BuildRouteGenerationRecords(groups)
	is.mu.Lock()
	is.tracking = make(map[string]RouteGenerationRecord)
	for _, rec := range records {
		is.tracking[rec.GroupID] = rec
	}
	is.lastSync = time.Now()
	is.syncCount++
	is.mu.Unlock()

	if is.publisher != nil {
		_ = is.publisher.Publish(ctx, events.NewEnvelope(
			"ingress_synced",
			"crossnode",
			"ingress",
			"",
			map[string]any{
				"routes":     len(mergedRules),
				"groups":     len(groups),
				"syncCount":  is.syncCount,
			},
		))
	}

	return nil
}

func (is *IngressSynchronizer) SetRules(rules []*trafficmanager.RoutingRule) {
	is.mu.Lock()
	defer is.mu.Unlock()
	is.rules = make(map[string]*trafficmanager.RoutingRule, len(rules))
	for _, rule := range rules {
		is.rules[rule.ID] = rule
	}
}

func (is *IngressSynchronizer) SetPolicies(policies map[string]*trafficmanager.TrafficPolicy) {
	is.mu.Lock()
	defer is.mu.Unlock()
	is.policies = make(map[string]*trafficmanager.TrafficPolicy, len(policies))
	for k, v := range policies {
		is.policies[k] = v
	}
}

func (is *IngressSynchronizer) UpsertRule(rule *trafficmanager.RoutingRule) {
	is.mu.Lock()
	defer is.mu.Unlock()
	is.rules[rule.ID] = rule
}

func (is *IngressSynchronizer) RemoveRule(ruleID string) {
	is.mu.Lock()
	defer is.mu.Unlock()
	delete(is.rules, ruleID)
}

func (is *IngressSynchronizer) CleanupStale(ctx context.Context) error {
	is.mu.RLock()
	activeIDs := make(map[string]bool, len(is.rules))
	for id := range is.rules {
		activeIDs[id] = true
	}
	is.mu.RUnlock()
	return is.adapter.CleanupStale(ctx, activeIDs)
}

func (is *IngressSynchronizer) ReloadGateway(ctx context.Context) error {
	return is.adapter.Reload(ctx)
}

func (is *IngressSynchronizer) Health(ctx context.Context) trafficmanager.AdapterHealth {
	return is.adapter.Health(ctx)
}

func (is *IngressSynchronizer) RouteGenerationRecords() []RouteGenerationRecord {
	is.mu.RLock()
	defer is.mu.RUnlock()
	records := make([]RouteGenerationRecord, 0, len(is.tracking))
	for _, rec := range is.tracking {
		records = append(records, rec)
	}
	return records
}

func (is *IngressSynchronizer) GetRouteGenerationRecord(groupID string) (RouteGenerationRecord, bool) {
	is.mu.RLock()
	defer is.mu.RUnlock()
	rec, ok := is.tracking[groupID]
	return rec, ok
}

func (is *IngressSynchronizer) Stats() IngressSyncStats {
	is.mu.RLock()
	defer is.mu.RUnlock()
	return IngressSyncStats{
		RuleCount:     len(is.rules),
		PolicyCount:   len(is.policies),
		TrackingCount: len(is.tracking),
		LastSync:      is.lastSync,
		SyncCount:     is.syncCount,
		ErrCount:      is.errCount,
		Running:       is.running,
	}
}

type IngressSyncStats struct {
	RuleCount     int       `json:"ruleCount"`
	PolicyCount   int       `json:"policyCount"`
	TrackingCount int       `json:"trackingCount"`
	LastSync      time.Time `json:"lastSync"`
	SyncCount     int       `json:"syncCount"`
	ErrCount      int       `json:"errCount"`
	Running       bool      `json:"running"`
}

func (is *IngressSynchronizer) DescribeBackend(host string, port int) string {
	health := is.health.GetHealth(host, port)
	if health.Status == HealthDown {
		return fmt.Sprintf("backend %s:%d is DOWN (failures: %d, reason: %s)",
			host, port, health.FailCount, health.Reason)
	}
	if health.Status == HealthDegraded {
		return fmt.Sprintf("backend %s:%d is DEGRADED (failures: %d, reason: %s)",
			host, port, health.FailCount, health.Reason)
	}
	return fmt.Sprintf("backend %s:%d is HEALTHY", host, port)
}
