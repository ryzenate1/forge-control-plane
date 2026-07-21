package trafficmanager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

type RoutingRule struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	ServerID   string            `json:"serverId,omitempty"`
	Domain     string            `json:"domain"`
	Path       string            `json:"path"`
	TargetHost string            `json:"targetHost,omitempty"`
	TargetPort int               `json:"targetPort"`
	Protocol   string            `json:"protocol"`
	Strategy   string            `json:"strategy"`
	Weight     int               `json:"weight"`
	Headers    map[string]string `json:"headers,omitempty"`
	Enabled    bool              `json:"enabled"`
	WebSocket  bool              `json:"webSocketSupport"`
	CreatedAt  time.Time         `json:"createdAt"`
}

type TrafficPolicy struct {
	ID                      string   `json:"id"`
	Name                    string   `json:"name"`
	RateLimit               int      `json:"rateLimit"`
	RateLimitBurst          int      `json:"rateLimitBurst"`
	IPWhitelist             []string `json:"ipWhitelist,omitempty"`
	IPBlacklist             []string `json:"ipBlacklist,omitempty"`
	TLSEnabled              bool     `json:"tlsEnabled"`
	TLSCertFile             string   `json:"tlsCertFile,omitempty"`
	TLSKeyFile              string   `json:"tlsKeyFile,omitempty"`
	CircuitBreaker          bool     `json:"circuitBreaker"`
	CircuitBreakerThreshold int      `json:"circuitBreakerThreshold"`
	CircuitBreakerTimeout   int      `json:"circuitBreakerTimeout"`
}

type RoutingPersistence interface {
	CreateRoutingRule(ctx context.Context, rule store.RoutingRuleRow) error
	UpdateRoutingRule(ctx context.Context, rule store.RoutingRuleRow) error
	DeleteRoutingRule(ctx context.Context, id string) error
	GetRoutingRule(ctx context.Context, id string) (*store.RoutingRuleRow, error)
	ListRoutingRules(ctx context.Context) ([]store.RoutingRuleRow, error)
	ListRoutingRulesForServer(ctx context.Context, serverID string) ([]store.RoutingRuleRow, error)
}

type PolicyPersistence interface {
	CreatePolicy(ctx context.Context, policy store.TrafficPolicyRow) error
	UpdatePolicy(ctx context.Context, policy store.TrafficPolicyRow) error
	DeletePolicy(ctx context.Context, id string) error
	GetPolicy(ctx context.Context, id string) (*store.TrafficPolicyRow, error)
	ListPolicies(ctx context.Context) ([]store.TrafficPolicyRow, error)
}

type Service struct {
	store             trafficStore
	nodeStore         NodeStore
	ruleStore         RoutingPersistence
	policyStore       PolicyPersistence
	rules             map[string]*RoutingRule
	withdrawnRules    map[string]*RoutingRule
	policies          map[string]*TrafficPolicy
	mu                sync.RWMutex
	publisher         events.Publisher
	proxy             ReverseProxy
	adapter           GatewayAdapter
	reconcileInterval time.Duration
	healthFailures    map[string]int
	healthMu          sync.Mutex
	cancel            context.CancelFunc
}

type trafficStore interface {
	GetServer(ctx context.Context, id string) (store.Server, error)
	GetNode(ctx context.Context, id string) (store.Node, error)
}

type NodeStore interface {
	ListServersForNode(ctx context.Context, nodeID string) ([]store.Server, error)
	GetNode(ctx context.Context, nodeID string) (store.Node, error)
}

type ReverseProxy interface {
	UpdateRoutes(ctx context.Context, rules []*RoutingRule, policies map[string]*TrafficPolicy) error
	RemoveRoutes(ctx context.Context, ruleIDs []string) error
	GetActiveConnections() map[string]int
}

func New(store trafficStore, nodeStore NodeStore, proxy ReverseProxy, publishers ...events.Publisher) *Service {
	return NewWithPersistence(store, nodeStore, nil, nil, proxy, publishers...)
}

func NewWithPersistence(store trafficStore, nodeStore NodeStore, ruleStore RoutingPersistence, policyStore PolicyPersistence, proxy ReverseProxy, publishers ...events.Publisher) *Service {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Service{
		store:             store,
		nodeStore:         nodeStore,
		ruleStore:         ruleStore,
		policyStore:       policyStore,
		rules:             make(map[string]*RoutingRule),
		withdrawnRules:    make(map[string]*RoutingRule),
		policies:          make(map[string]*TrafficPolicy),
		publisher:         publisher,
		proxy:             proxy,
		reconcileInterval: 2 * time.Minute,
		healthFailures:    make(map[string]int),
	}
}

func NewWithAdapter(store trafficStore, nodeStore NodeStore, proxy ReverseProxy, adapter GatewayAdapter, publishers ...events.Publisher) *Service {
	svc := New(store, nodeStore, proxy, publishers...)
	svc.adapter = adapter
	return svc
}

func NewWithAdapterAndPersistence(store trafficStore, nodeStore NodeStore, ruleStore RoutingPersistence, policyStore PolicyPersistence, proxy ReverseProxy, adapter GatewayAdapter, publishers ...events.Publisher) *Service {
	svc := NewWithPersistence(store, nodeStore, ruleStore, policyStore, proxy, publishers...)
	svc.adapter = adapter
	return svc
}

func (s *Service) SetAdapter(a GatewayAdapter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.adapter = a
}

func (s *Service) Adapter() GatewayAdapter {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.adapter
}

// InitFromDB loads all routing rules from the database into the in-memory map.
// Safe to call even when the traffic_rules table does not exist yet (migration not run).
func (s *Service) InitFromDB(ctx context.Context) error {
	if s.ruleStore == nil {
		return nil
	}
	dbRules, err := s.ruleStore.ListRoutingRules(ctx)
	if err != nil {
		// If the table doesn't exist yet, this is not a fatal error;
		// the table will be created when migrations run.
		if isTableNotFoundError(err) {
			slog.Warn("traffic manager: traffic_rules table not found, skipping DB load")
			return nil
		}
		return fmt.Errorf("load routing rules from db: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, dbRule := range dbRules {
		rule := routingRowToRule(dbRule)
		s.rules[rule.ID] = rule
	}
	slog.Info("traffic manager loaded routing rules from database", "count", len(dbRules))
	return nil
}

func isTableNotFoundError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "relation") && strings.Contains(msg, "does not exist")
}

func routingRowToRule(r store.RoutingRuleRow) *RoutingRule {
	headers := r.Headers
	if headers == nil {
		headers = make(map[string]string)
	}
	return &RoutingRule{
		ID:         r.ID,
		Name:       r.Name,
		ServerID:   r.ServerID,
		Domain:     r.Domain,
		Path:       r.Path,
		TargetHost: r.TargetHost,
		TargetPort: r.TargetPort,
		Protocol:   r.Protocol,
		Strategy:   r.Strategy,
		Weight:     r.Weight,
		Headers:    headers,
		Enabled:    r.Enabled,
		WebSocket:  r.WebSocket,
		CreatedAt:  r.CreatedAt,
	}
}

func policyRowToPolicy(r store.TrafficPolicyRow) *TrafficPolicy {
	return &TrafficPolicy{
		ID:                      r.ID,
		Name:                    r.Name,
		RateLimit:               r.RateLimit,
		RateLimitBurst:          r.RateLimitBurst,
		IPWhitelist:             r.IPWhitelist,
		IPBlacklist:             r.IPBlacklist,
		TLSEnabled:              r.TLSEnabled,
		TLSCertFile:             r.TLSCertFile,
		TLSKeyFile:              r.TLSKeyFile,
		CircuitBreaker:          r.CircuitBreaker,
		CircuitBreakerThreshold: r.CircuitBreakerThreshold,
		CircuitBreakerTimeout:   r.CircuitBreakerTimeout,
	}
}

func trafficPolicyToRow(policy *TrafficPolicy, createdAt time.Time) store.TrafficPolicyRow {
	return store.TrafficPolicyRow{
		ID:                      policy.ID,
		Name:                    policy.Name,
		RateLimit:               policy.RateLimit,
		RateLimitBurst:          policy.RateLimitBurst,
		IPWhitelist:             policy.IPWhitelist,
		IPBlacklist:             policy.IPBlacklist,
		TLSEnabled:              policy.TLSEnabled,
		TLSCertFile:             policy.TLSCertFile,
		TLSKeyFile:              policy.TLSKeyFile,
		CircuitBreaker:          policy.CircuitBreaker,
		CircuitBreakerThreshold: policy.CircuitBreakerThreshold,
		CircuitBreakerTimeout:   policy.CircuitBreakerTimeout,
		CreatedAt:               createdAt,
		UpdatedAt:               time.Now().UTC(),
	}
}

func (s *Service) InitPoliciesFromDB(ctx context.Context) error {
	if s.policyStore == nil {
		return nil
	}
	dbPolicies, err := s.policyStore.ListPolicies(ctx)
	if err != nil {
		if isTableNotFoundError(err) {
			slog.Warn("traffic manager: traffic_policies table not found, skipping DB load")
			return nil
		}
		return fmt.Errorf("load traffic policies from db: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, dbPolicy := range dbPolicies {
		policy := policyRowToPolicy(dbPolicy)
		s.policies[policy.ID] = policy
	}
	slog.Info("traffic manager loaded traffic policies from database", "count", len(dbPolicies))
	return nil
}

func routingRuleToRow(rule *RoutingRule) store.RoutingRuleRow {
	return store.RoutingRuleRow{
		ID:         rule.ID,
		Name:       rule.Name,
		ServerID:   rule.ServerID,
		Domain:     rule.Domain,
		Path:       rule.Path,
		TargetHost: rule.TargetHost,
		TargetPort: rule.TargetPort,
		Protocol:   rule.Protocol,
		Strategy:   rule.Strategy,
		Weight:     rule.Weight,
		Headers:    rule.Headers,
		Enabled:    rule.Enabled,
		WebSocket:  rule.WebSocket,
		CreatedAt:  rule.CreatedAt,
		UpdatedAt:  time.Now().UTC(),
	}
}

func (s *Service) CreateRoutingRule(ctx context.Context, rule *RoutingRule) error {
	if rule == nil {
		return errors.New("rule is required")
	}
	if rule.Domain == "" || rule.TargetPort == 0 {
		return errors.New("domain and targetPort are required")
	}
	if rule.Strategy == "" {
		rule.Strategy = "round_robin"
	}
	if rule.Path == "" {
		rule.Path = "/"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if rule.ID == "" {
		rule.ID = uuid.NewString()
	}
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now().UTC()
	}

	// Persist to DB first; on failure, don't add to local cache.
	if s.ruleStore != nil {
		row := routingRuleToRow(rule)
		row.UpdatedAt = row.CreatedAt
		if err := s.ruleStore.CreateRoutingRule(ctx, row); err != nil {
			return fmt.Errorf("persist routing rule: %w", err)
		}
	}

	s.rules[rule.ID] = rule
	return nil
}

func (s *Service) UpdateRoutingRule(ctx context.Context, rule *RoutingRule) error {
	if rule == nil {
		return errors.New("rule is required")
	}
	if rule.ID == "" {
		return errors.New("rule id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.rules[rule.ID]
	if !ok {
		return errors.New("rule not found")
	}

	rule.CreatedAt = existing.CreatedAt

	// Persist to DB first; on failure, don't update local cache.
	if s.ruleStore != nil {
		row := routingRuleToRow(rule)
		if err := s.ruleStore.UpdateRoutingRule(ctx, row); err != nil {
			return fmt.Errorf("persist routing rule update: %w", err)
		}
	}

	s.rules[rule.ID] = rule
	return nil
}

func (s *Service) DeleteRoutingRule(ctx context.Context, ruleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, inRules := s.rules[ruleID]
	_, inWithdrawn := s.withdrawnRules[ruleID]
	if !inRules && !inWithdrawn {
		return errors.New("rule not found")
	}

	// Delete from DB first; on failure, don't remove from local cache.
	if s.ruleStore != nil {
		if err := s.ruleStore.DeleteRoutingRule(ctx, ruleID); err != nil {
			return fmt.Errorf("persist routing rule delete: %w", err)
		}
	}

	delete(s.rules, ruleID)
	delete(s.withdrawnRules, ruleID)
	return nil
}

func (s *Service) GetRoutingRule(ctx context.Context, ruleID string) (*RoutingRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rule, ok := s.rules[ruleID]
	if !ok {
		return nil, errors.New("rule not found")
	}
	return rule, nil
}

func (s *Service) ListRoutingRules(ctx context.Context) ([]*RoutingRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rules := make([]*RoutingRule, 0, len(s.rules))
	for _, rule := range s.rules {
		rules = append(rules, rule)
	}
	return rules, nil
}

func (s *Service) ListRoutingRulesByServer(ctx context.Context, serverID string) ([]*RoutingRule, error) {
	if serverID == "" {
		return nil, errors.New("serverId is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*RoutingRule
	for _, rule := range s.rules {
		if rule.ServerID == serverID {
			result = append(result, rule)
		}
	}
	return result, nil
}

func (s *Service) CreateTrafficPolicy(ctx context.Context, policy *TrafficPolicy) error {
	if policy == nil {
		return errors.New("policy is required")
	}
	if policy.Name == "" {
		return errors.New("policy name is required")
	}

	if policy.ID == "" {
		policy.ID = uuid.NewString()
	}

	now := time.Now().UTC()

	if s.policyStore != nil {
		row := trafficPolicyToRow(policy, now)
		if err := s.policyStore.CreatePolicy(ctx, row); err != nil {
			return fmt.Errorf("persist traffic policy: %w", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.policies[policy.ID] = policy
	return nil
}

func (s *Service) UpdateTrafficPolicy(ctx context.Context, policy *TrafficPolicy) error {
	if policy == nil {
		return errors.New("policy is required")
	}
	if policy.ID == "" {
		return errors.New("policy id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.policies[policy.ID]; !ok {
		return errors.New("policy not found")
	}

	if s.policyStore != nil {
		row := trafficPolicyToRow(policy, time.Now().UTC())
		if err := s.policyStore.UpdatePolicy(ctx, row); err != nil {
			return fmt.Errorf("persist traffic policy update: %w", err)
		}
	}

	s.policies[policy.ID] = policy
	return nil
}

func (s *Service) DeleteTrafficPolicy(ctx context.Context, policyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.policies[policyID]; !ok {
		return errors.New("policy not found")
	}

	if s.policyStore != nil {
		if err := s.policyStore.DeletePolicy(ctx, policyID); err != nil {
			return fmt.Errorf("persist traffic policy delete: %w", err)
		}
	}

	delete(s.policies, policyID)
	return nil
}

func (s *Service) GetTrafficPolicy(ctx context.Context, policyID string) (*TrafficPolicy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	policy, ok := s.policies[policyID]
	if !ok {
		return nil, errors.New("policy not found")
	}
	return policy, nil
}

func (s *Service) ListTrafficPolicies(ctx context.Context) ([]*TrafficPolicy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	policies := make([]*TrafficPolicy, 0, len(s.policies))
	for _, p := range s.policies {
		policies = append(policies, p)
	}
	return policies, nil
}

func (s *Service) ApplyRoutes(ctx context.Context) error {
	if s.proxy == nil {
		return errors.New("reverse proxy is not configured")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	rules := make([]*RoutingRule, 0, len(s.rules))
	for _, rule := range s.rules {
		if rule.Enabled {
			rules = append(rules, rule)
		}
	}

	resolved, err := s.resolveTargets(ctx, rules)
	if err != nil {
		return fmt.Errorf("resolve targets: %w", err)
	}

	policies := make(map[string]*TrafficPolicy, len(s.policies))
	for k, v := range s.policies {
		policies[k] = v
	}

	return s.proxy.UpdateRoutes(ctx, resolved, policies)
}

func (s *Service) SyncRoutes(ctx context.Context) error {
	if s.proxy == nil {
		return errors.New("reverse proxy is not configured")
	}

	s.mu.RLock()
	rules := make([]*RoutingRule, 0, len(s.rules))
	for _, rule := range s.rules {
		if rule.Enabled {
			rules = append(rules, rule)
		}
	}

	policies := make(map[string]*TrafficPolicy, len(s.policies))
	for k, v := range s.policies {
		policies[k] = v
	}
	s.mu.RUnlock()

	resolved, err := s.resolveTargets(ctx, rules)
	if err != nil {
		return fmt.Errorf("resolve targets: %w", err)
	}

	if err := s.proxy.UpdateRoutes(ctx, resolved, policies); err != nil {
		return fmt.Errorf("sync routes: %w", err)
	}
	return nil
}

func (s *Service) resolveTargets(ctx context.Context, rules []*RoutingRule) ([]*RoutingRule, error) {
	if s.store == nil {
		return rules, nil
	}

	resolved := make([]*RoutingRule, 0, len(rules))
	for _, rule := range rules {
		clone := *rule
		host, healthy := s.resolveTargetHost(ctx, rule)
		if clone.TargetHost == "" {
			clone.TargetHost = host
		}
		if !healthy {
			slog.Warn("traffic manager skipping unhealthy node target",
				"ruleID", rule.ID,
				"serverID", rule.ServerID,
				"targetHost", clone.TargetHost,
			)
			continue
		}
		resolved = append(resolved, &clone)
	}
	return resolved, nil
}

func (s *Service) resolveTargetHost(ctx context.Context, rule *RoutingRule) (string, bool) {
	if rule.ServerID == "" {
		return "localhost", true
	}

	server, err := s.store.GetServer(ctx, rule.ServerID)
	if err != nil || server.NodeID == "" {
		return "localhost", true
	}

	node, err := s.store.GetNode(ctx, server.NodeID)
	if err != nil {
		return "localhost", true
	}

	// Check whether the resolved node is online/healthy
	// A node is considered healthy if both ActualState is Online AND HeartbeatState is Healthy
	healthy := node.ActualState == string(store.NodeActualStateOnline) &&
		node.HeartbeatState == string(store.NodeHeartbeatStateHealthy)

	host := strings.TrimSpace(node.PublicHostname)
	if host == "" {
		host = strings.TrimSpace(node.FQDN)
	}
	if host == "" {
		return "localhost", healthy
	}
	return host, healthy
}

func (s *Service) AdapterKind(ctx context.Context) AdapterKind {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.adapter != nil {
		return s.adapter.Kind()
	}
	return AdapterCaddy
}

func (s *Service) AdapterHealth(ctx context.Context) AdapterHealth {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.adapter != nil {
		return s.adapter.Health(ctx)
	}
	return AdapterHealth{Status: HealthUnknown, Message: "no adapter configured"}
}

func (s *Service) ValidateGatewayConfig(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.adapter == nil {
		return nil
	}
	return s.adapter.ValidateConfig(ctx)
}

func (s *Service) ReloadGateway(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.adapter == nil {
		return nil
	}
	return s.adapter.Reload(ctx)
}

func (s *Service) RollbackGateway(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.adapter == nil {
		return errors.New("no gateway adapter configured for rollback")
	}
	return s.adapter.Rollback(ctx)
}

func (s *Service) CleanupStaleRoutes(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.adapter == nil {
		return nil
	}

	activeRuleIDs := make(map[string]bool, len(s.rules)+len(s.withdrawnRules))
	for id := range s.rules {
		activeRuleIDs[id] = true
	}
	for id := range s.withdrawnRules {
		activeRuleIDs[id] = true
	}

	return s.adapter.CleanupStale(ctx, activeRuleIDs)
}

func (s *Service) Handle(ctx context.Context, envelope events.Envelope) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	switch envelope.Type {
	case events.EventNodeOffline, events.EventNodeDegraded, events.EventNodeDrainingStarted:
		return s.WithdrawNodeTargets(ctx, envelope.ResourceID)
	case events.EventNodeRecovered, events.EventNodeDrainingCompleted:
		return s.ReinstateNodeTargets(ctx, envelope.ResourceID)
	default:
		return nil
	}
}

func (s *Service) WithdrawNodeTargets(ctx context.Context, nodeID string) error {
	if s.nodeStore == nil {
		return errors.New("node store is not configured")
	}

	servers, err := s.nodeStore.ListServersForNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("list servers for node %s: %w", nodeID, err)
	}

	s.mu.Lock()

	var ruleIDs []string
	for _, server := range servers {
		for id, rule := range s.rules {
			if rule.ServerID == server.ID {
				ruleIDs = append(ruleIDs, id)
			}
		}
	}

	if len(ruleIDs) == 0 {
		s.mu.Unlock()
		return nil
	}

	for _, id := range ruleIDs {
		s.withdrawnRules[id] = s.rules[id]
		delete(s.rules, id)
	}

	s.healthMu.Lock()
	for key := range s.healthFailures {
		for _, id := range ruleIDs {
			if strings.HasPrefix(key, id+"|") {
				delete(s.healthFailures, key)
				break
			}
		}
	}
	s.healthMu.Unlock()

	s.mu.Unlock()

	if s.proxy != nil {
		if err := s.proxy.RemoveRoutes(ctx, ruleIDs); err != nil {
			return fmt.Errorf("remove routes for node %s: %w", nodeID, err)
		}
	}

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope(
			events.EventTargetRemoved,
			"trafficmanager",
			"node",
			nodeID,
			map[string]any{
				"nodeID":           nodeID,
				"withdrawnRuleIDs": ruleIDs,
				"count":            len(ruleIDs),
			},
		))
	}

	slog.Info("traffic manager withdrew node targets", "nodeID", nodeID, "count", len(ruleIDs))
	return nil
}

func (s *Service) ReinstateNodeTargets(ctx context.Context, nodeID string) error {
	if s.nodeStore == nil {
		return errors.New("node store is not configured")
	}

	node, err := s.nodeStore.GetNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("get node %s: %w", nodeID, err)
	}
	if node.HeartbeatState != string(store.NodeHeartbeatStateHealthy) {
		return fmt.Errorf("node %s is not healthy (state: %s)", nodeID, node.HeartbeatState)
	}

	servers, err := s.nodeStore.ListServersForNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("list servers for node %s: %w", nodeID, err)
	}

	serverIDs := make(map[string]bool, len(servers))
	for _, server := range servers {
		serverIDs[server.ID] = true
	}

	s.mu.Lock()
	var reinstated []*RoutingRule
	reinstatedIDs := make([]string, 0)
	for id, rule := range s.withdrawnRules {
		if serverIDs[rule.ServerID] {
			s.rules[id] = rule
			delete(s.withdrawnRules, id)
			reinstated = append(reinstated, rule)
			reinstatedIDs = append(reinstatedIDs, id)
		}
	}

	if len(reinstated) == 0 {
		s.mu.Unlock()
		return nil
	}

	policies := make(map[string]*TrafficPolicy, len(s.policies))
	for k, v := range s.policies {
		policies[k] = v
	}

	enabled := make([]*RoutingRule, 0, len(s.rules))
	for _, rule := range s.rules {
		if rule.Enabled {
			enabled = append(enabled, rule)
		}
	}
	s.mu.Unlock()

	resolved, err := s.resolveTargets(ctx, enabled)
	if err != nil {
		return fmt.Errorf("resolve targets: %w", err)
	}

	if s.proxy != nil {
		if err := s.proxy.UpdateRoutes(ctx, resolved, policies); err != nil {
			return fmt.Errorf("reinstate routes: %w", err)
		}
	}

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope(
			events.EventTargetAdded,
			"trafficmanager",
			"node",
			nodeID,
			map[string]any{
				"nodeID":            nodeID,
				"reinstatedRuleIDs": reinstatedIDs,
				"count":             len(reinstated),
			},
		))
	}

	slog.Info("traffic manager reinstated node targets", "nodeID", nodeID, "count", len(reinstated))
	return nil
}

func (s *Service) ReconcileRoutes(ctx context.Context) error {
	if s.nodeStore == nil || s.proxy == nil {
		return nil
	}

	s.mu.RLock()
	rules := make([]*RoutingRule, 0, len(s.rules))
	for _, rule := range s.rules {
		rules = append(rules, rule)
	}
	s.mu.RUnlock()

	var unhealthyRuleIDs []string
	for _, rule := range rules {
		if rule.ServerID == "" {
			continue
		}
		server, err := s.store.GetServer(ctx, rule.ServerID)
		if err != nil {
			continue
		}
		if server.NodeID == "" {
			continue
		}
		node, err := s.nodeStore.GetNode(ctx, server.NodeID)
		if err != nil {
			continue
		}
		if node.ActualState != string(store.NodeActualStateOnline) ||
			node.HeartbeatState != string(store.NodeHeartbeatStateHealthy) {
			unhealthyRuleIDs = append(unhealthyRuleIDs, rule.ID)
		}
	}

	if len(unhealthyRuleIDs) > 0 {
		if err := s.proxy.RemoveRoutes(ctx, unhealthyRuleIDs); err != nil {
			return fmt.Errorf("reconcile remove routes: %w", err)
		}

		s.mu.Lock()
		for _, id := range unhealthyRuleIDs {
			if rule, ok := s.rules[id]; ok {
				s.withdrawnRules[id] = rule
				delete(s.rules, id)
			}
		}
		s.mu.Unlock()

		slog.Info("traffic manager reconciled unhealthy node routes", "count", len(unhealthyRuleIDs))
	}
	return nil
}

func (s *Service) Start(ctx context.Context) {
	slog.Info("traffic manager starting")
	ctx, s.cancel = context.WithCancel(ctx)
	if err := s.InitFromDB(ctx); err != nil {
		slog.Error("traffic manager: failed to load routing rules from DB", "error", err)
	}
	if err := s.InitPoliciesFromDB(ctx); err != nil {
		slog.Error("traffic manager: failed to load policies from DB", "error", err)
	}

	go func() {
		ticker := time.NewTicker(s.reconcileInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.ReconcileRoutes(ctx); err != nil {
					slog.Error("traffic manager reconcile failed", "error", err)
				}
				if err := s.ProbeTargets(ctx); err != nil {
					slog.Error("traffic manager health probe failed", "error", err)
				}
				s.healthMu.Lock()
				for key := range s.healthFailures {
					parts := strings.SplitN(key, "|", 2)
					if len(parts) > 0 && parts[0] != "" {
						s.mu.RLock()
						_, exists := s.rules[parts[0]]
						s.mu.RUnlock()
						if !exists {
							delete(s.healthFailures, key)
						}
					}
				}
				s.healthMu.Unlock()
			}
		}
	}()
}

func (s *Service) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *Service) ProbeTargets(ctx context.Context) error {
	if s.adapter == nil {
		return nil
	}

	s.mu.RLock()
	rules := make([]*RoutingRule, 0, len(s.rules))
	for _, rule := range s.rules {
		if rule.Enabled {
			rules = append(rules, rule)
		}
	}
	s.mu.RUnlock()

	resolved, err := s.resolveTargets(ctx, rules)
	if err != nil {
		return fmt.Errorf("resolve targets for probe: %w", err)
	}

	for _, rule := range resolved {
		if rule.TargetHost == "" || rule.TargetPort == 0 {
			continue
		}
		key := rule.ID + "|" + rule.TargetHost + fmt.Sprintf(":%d", rule.TargetPort)
		addr := net.JoinHostPort(rule.TargetHost, fmt.Sprintf("%d", rule.TargetPort))
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			s.healthMu.Lock()
			s.healthFailures[key]++
			failures := s.healthFailures[key]
			s.healthMu.Unlock()
			if failures >= 3 {
				if s.publisher != nil {
					_ = s.publisher.Publish(ctx, events.NewEnvelope(
						events.EventTargetHealthChanged,
						"trafficmanager",
						"server",
						rule.ServerID,
						map[string]any{
							"ruleID":     rule.ID,
							"targetHost": rule.TargetHost,
							"targetPort": rule.TargetPort,
							"healthy":    false,
							"failures":   failures,
						},
					))
				}
				if markErr := s.adapter.SetUpstreamHealth(ctx, rule.ID, rule.TargetHost, rule.TargetPort, false); markErr != nil {
					slog.Warn("failed to mark upstream unhealthy", "ruleID", rule.ID, "target", addr, "error", markErr)
				}
			}
		} else {
			conn.Close()
			s.healthMu.Lock()
			wasUnhealthy := s.healthFailures[key] > 0
			if wasUnhealthy {
				s.healthFailures[key] = 0
			}
			s.healthMu.Unlock()
			if wasUnhealthy {
				if s.publisher != nil {
					_ = s.publisher.Publish(ctx, events.NewEnvelope(
						events.EventTargetHealthChanged,
						"trafficmanager",
						"server",
						rule.ServerID,
						map[string]any{
							"ruleID":     rule.ID,
							"targetHost": rule.TargetHost,
							"targetPort": rule.TargetPort,
							"healthy":    true,
						},
					))
				}
				if markErr := s.adapter.SetUpstreamHealth(ctx, rule.ID, rule.TargetHost, rule.TargetPort, true); markErr != nil {
					slog.Warn("failed to mark upstream healthy", "ruleID", rule.ID, "target", addr, "error", markErr)
				}
			}
		}
	}
	return nil
}
