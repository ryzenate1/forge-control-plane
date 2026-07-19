package trafficmanager

import (
	"context"
	"errors"
	"fmt"
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
	TargetPort int               `json:"targetPort"`
	Protocol   string            `json:"protocol"`
	Strategy   string            `json:"strategy"`
	Weight     int               `json:"weight"`
	Headers    map[string]string `json:"headers,omitempty"`
	Enabled    bool              `json:"enabled"`
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

type Service struct {
	store     trafficStore
	rules     map[string]*RoutingRule
	policies  map[string]*TrafficPolicy
	mu        sync.RWMutex
	publisher events.Publisher
	proxy     ReverseProxy
}

type trafficStore interface {
	GetServer(ctx context.Context, id string) (store.Server, error)
	GetNode(ctx context.Context, id string) (store.Node, error)
}

type ReverseProxy interface {
	UpdateRoutes(ctx context.Context, rules []*RoutingRule) error
	RemoveRoutes(ctx context.Context, ruleIDs []string) error
	GetActiveConnections() map[string]int
}

func New(store trafficStore, proxy ReverseProxy, publishers ...events.Publisher) *Service {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Service{
		store:     store,
		rules:     make(map[string]*RoutingRule),
		policies:  make(map[string]*TrafficPolicy),
		publisher: publisher,
		proxy:     proxy,
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

	s.mu.Lock()
	defer s.mu.Unlock()

	if rule.ID == "" {
		rule.ID = uuid.NewString()
	}
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now().UTC()
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
	s.rules[rule.ID] = rule
	return nil
}

func (s *Service) DeleteRoutingRule(ctx context.Context, ruleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.rules[ruleID]; !ok {
		return errors.New("rule not found")
	}
	delete(s.rules, ruleID)
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

	s.mu.Lock()
	defer s.mu.Unlock()

	if policy.ID == "" {
		policy.ID = uuid.NewString()
	}
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
	s.policies[policy.ID] = policy
	return nil
}

func (s *Service) DeleteTrafficPolicy(ctx context.Context, policyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.policies[policyID]; !ok {
		return errors.New("policy not found")
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

	return s.proxy.UpdateRoutes(ctx, rules)
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
	s.mu.RUnlock()

	if err := s.proxy.UpdateRoutes(ctx, rules); err != nil {
		return fmt.Errorf("sync routes: %w", err)
	}
	return nil
}
