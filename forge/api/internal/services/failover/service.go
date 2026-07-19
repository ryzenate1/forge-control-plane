package failover

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"
)

type FailoverAction string

const (
	FailoverActionEvacuate FailoverAction = "evacuate"
	FailoverActionRestart  FailoverAction = "restart"
	FailoverActionNotify   FailoverAction = "notify"
)

type FailoverEventType string

const (
	EventNodeFailure        FailoverEventType = "node_failure"
	EventServerCrash        FailoverEventType = "server_crash"
	EventHealthCheckFailure FailoverEventType = "health_check_failure"
)

type Policy struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	NodeID           string         `json:"nodeId"`
	Enabled          bool           `json:"enabled"`
	MaxFailures      int            `json:"maxFailures"`
	FailureWindowSec int            `json:"failureWindowSec"`
	CooldownSec      int            `json:"cooldownSec"`
	Action           FailoverAction `json:"action"`
	HealthCheckPath  string         `json:"healthCheckPath,omitempty"`
	HealthCheckPort  int            `json:"healthCheckPort,omitempty"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}

type Event struct {
	ID        string            `json:"id"`
	PolicyID  string            `json:"policyId"`
	NodeID    string            `json:"nodeId"`
	ServerID  string            `json:"serverId,omitempty"`
	EventType FailoverEventType `json:"eventType"`
	Action    FailoverAction    `json:"action"`
	Status    string            `json:"status"`
	Message   string            `json:"message"`
	Timestamp time.Time         `json:"timestamp"`
}

type Metrics struct {
	FailuresDetected     uint64 `json:"failuresDetected"`
	EvacuationsTriggered uint64 `json:"evacuationsTriggered"`
	RestartsTriggered    uint64 `json:"restartsTriggered"`
	NotificationsSent    uint64 `json:"notificationsSent"`
}

type Service struct {
	db         *store.Store
	failures   map[string][]time.Time
	lastAction map[string]time.Time
	mu         sync.Mutex
	publisher  events.Publisher
	executor   func(context.Context, *Event) error
	metrics    Metrics
	stopCh     chan struct{}
	started    bool
}

var ErrPolicyNotFound = errors.New("failover policy not found")

func New(db *store.Store, publishers ...events.Publisher) *Service {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Service{
		db:         db,
		failures:   make(map[string][]time.Time),
		lastAction: make(map[string]time.Time),
		publisher:  publisher,
		stopCh:     make(chan struct{}),
	}
}

// SetActionExecutor wires policy decisions to the real evacuation/recovery
// control plane. Keeping it injectable makes the policy engine testable and
// prevents the failover package from depending on concrete orchestrators.
func (s *Service) SetActionExecutor(executor func(context.Context, *Event) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executor = executor
}

// Handle consumes heartbeat transitions. Suspected, unreachable, and offline
// transitions count as distinct failure observations for policy thresholds.
func (s *Service) Handle(ctx context.Context, envelope events.Envelope) error {
	switch envelope.Type {
	case events.EventNodeSuspected, events.EventNodeUnreachable, events.EventNodeOffline:
		_, err := s.RecordFailure(ctx, envelope.ResourceID)
		return err
	default:
		return nil
	}
}

func (s *Service) CreatePolicy(ctx context.Context, policy *Policy) error {
	applyPolicyDefaults(policy)
	if err := validatePolicy(policy); err != nil {
		return err
	}

	sp := toStorePolicy(policy)
	if err := s.db.CreateFailoverPolicy(ctx, &sp); err != nil {
		return err
	}
	policy.ID = sp.ID
	policy.CreatedAt = sp.CreatedAt
	policy.UpdatedAt = sp.UpdatedAt

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("failover_policy_created", "failover", "policy", policy.ID, map[string]any{
			"nodeId": policy.NodeID, "action": policy.Action,
		}))
	}
	return nil
}

func (s *Service) UpdatePolicy(ctx context.Context, policy *Policy) error {
	applyPolicyDefaults(policy)
	if err := validatePolicy(policy); err != nil {
		return err
	}

	sp := toStorePolicy(policy)
	if err := s.db.UpdateFailoverPolicy(ctx, &sp); err != nil {
		return err
	}
	policy.UpdatedAt = sp.UpdatedAt

	updated, err := s.db.GetFailoverPolicy(ctx, policy.ID)
	if err != nil {
		return err
	}
	policy.CreatedAt = updated.CreatedAt

	return nil
}

func applyPolicyDefaults(policy *Policy) {
	if policy == nil {
		return
	}
	if policy.MaxFailures == 0 {
		policy.MaxFailures = 3
	}
	if policy.FailureWindowSec == 0 {
		policy.FailureWindowSec = 300
	}
	if policy.CooldownSec == 0 {
		policy.CooldownSec = 600
	}
	if policy.Action == "" {
		policy.Action = FailoverActionNotify
	}
}

func (s *Service) DeletePolicy(ctx context.Context, policyID string) error {
	_, err := s.db.GetFailoverPolicy(ctx, policyID)
	if err != nil {
		return ErrPolicyNotFound
	}
	return s.db.DeleteFailoverPolicy(ctx, policyID)
}

func (s *Service) GetPolicy(ctx context.Context, policyID string) (*Policy, error) {
	sp, err := s.db.GetFailoverPolicy(ctx, policyID)
	if err != nil {
		return nil, ErrPolicyNotFound
	}
	p := fromStorePolicy(sp)
	return &p, nil
}

func (s *Service) ListPolicies(ctx context.Context) ([]*Policy, error) {
	policies, err := s.db.ListFailoverPolicies(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*Policy, 0, len(policies))
	for _, sp := range policies {
		p := fromStorePolicy(sp)
		result = append(result, &p)
	}
	return result, nil
}

func (s *Service) ListPoliciesByNode(ctx context.Context, nodeID string) ([]*Policy, error) {
	policies, err := s.db.ListFailoverPoliciesByNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	result := make([]*Policy, 0, len(policies))
	for _, sp := range policies {
		p := fromStorePolicy(sp)
		result = append(result, &p)
	}
	return result, nil
}

func (s *Service) RecordFailure(ctx context.Context, nodeID string) (*Event, error) {
	if nodeID == "" {
		return nil, errors.New("nodeId is required")
	}

	policies, err := s.db.ListFailoverPoliciesByNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}

	var matchingPolicy *Policy
	for _, sp := range policies {
		if sp.Enabled {
			p := fromStorePolicy(sp)
			matchingPolicy = &p
			break
		}
	}
	if matchingPolicy == nil {
		return nil, nil
	}

	s.mu.Lock()
	now := time.Now().UTC()
	s.failures[nodeID] = append(s.failures[nodeID], now)
	windowStart := now.Add(-time.Duration(matchingPolicy.FailureWindowSec) * time.Second)
	var recentFailures []time.Time
	for _, t := range s.failures[nodeID] {
		if t.After(windowStart) {
			recentFailures = append(recentFailures, t)
		}
	}
	s.failures[nodeID] = recentFailures
	failureCount := len(recentFailures)
	s.metrics.FailuresDetected++
	s.mu.Unlock()

	if failureCount < matchingPolicy.MaxFailures {
		return nil, nil
	}

	return s.executeAction(ctx, matchingPolicy, EventNodeFailure, nodeID, "", fmt.Sprintf("node %s failed %d times", nodeID, failureCount))
}

func (s *Service) HandleServerCrash(ctx context.Context, serverID, nodeID string) (*Event, error) {
	policies, err := s.db.ListFailoverPoliciesByNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}

	var matchingPolicy *Policy
	for _, sp := range policies {
		if sp.Enabled {
			p := fromStorePolicy(sp)
			matchingPolicy = &p
			break
		}
	}
	if matchingPolicy == nil {
		return nil, nil
	}

	return s.executeAction(ctx, matchingPolicy, EventServerCrash, nodeID, serverID, fmt.Sprintf("server %s crashed", serverID))
}

func (s *Service) executeAction(ctx context.Context, policy *Policy, eventType FailoverEventType, nodeID, serverID, message string) (*Event, error) {
	s.mu.Lock()
	now := time.Now().UTC()
	if last := s.lastAction[policy.ID]; !last.IsZero() && now.Sub(last) < time.Duration(policy.CooldownSec)*time.Second {
		s.mu.Unlock()
		return nil, nil
	}
	// Claim the cooldown before running the executor so event-delivery retries
	// cannot start the same failover twice.
	s.lastAction[policy.ID] = now
	s.failures[nodeID] = nil
	executor := s.executor
	s.mu.Unlock()

	event := &Event{
		PolicyID:  policy.ID,
		NodeID:    nodeID,
		ServerID:  serverID,
		EventType: eventType,
		Action:    policy.Action,
		Status:    "detected",
		Message:   message,
		Timestamp: time.Now().UTC(),
	}

	switch policy.Action {
	case FailoverActionEvacuate:
		event.Status = "evacuating"
		if s.publisher != nil {
			_ = s.publisher.Publish(ctx, events.NewEnvelope("node_evacuation_triggered", "failover", "node", nodeID, map[string]any{
				"policyId": policy.ID, "reason": message,
			}))
		}
		s.mu.Lock()
		s.metrics.EvacuationsTriggered++
		s.mu.Unlock()

	case FailoverActionRestart:
		event.Status = "restarting"
		if s.publisher != nil {
			_ = s.publisher.Publish(ctx, events.NewEnvelope("node_restart_triggered", "failover", "node", nodeID, map[string]any{
				"policyId": policy.ID, "reason": message,
			}))
		}
		s.mu.Lock()
		s.metrics.RestartsTriggered++
		s.mu.Unlock()

	case FailoverActionNotify:
		event.Status = "notified"
		if s.publisher != nil {
			_ = s.publisher.Publish(ctx, events.NewEnvelope("node_failure_notified", "failover", "node", nodeID, map[string]any{
				"policyId": policy.ID, "failures": message,
			}))
		}
		s.mu.Lock()
		s.metrics.NotificationsSent++
		s.mu.Unlock()
	}

	if executor != nil && policy.Action != FailoverActionNotify {
		if err := executor(ctx, event); err != nil {
			event.Status = "failed"
			event.Message = message + ": " + err.Error()
			se := toStoreEvent(event)
			_ = s.db.CreateFailoverEvent(ctx, &se)
			return event, err
		}
	}

	event.Status = "completed"
	se := toStoreEvent(event)
	if err := s.db.CreateFailoverEvent(ctx, &se); err != nil {
		return event, nil
	}
	event.ID = se.ID
	event.Timestamp = se.CreatedAt
	return event, nil
}

func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}
	s.started = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	go s.loop(ctx)
	return nil
}

func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		close(s.stopCh)
		s.started = false
	}
}

func (s *Service) loop(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now().UTC()
			for id, failures := range s.failures {
				windowStart := now.Add(-time.Duration(300) * time.Second)
				var recent []time.Time
				for _, t := range failures {
					if t.After(windowStart) {
						recent = append(recent, t)
					}
				}
				s.failures[id] = recent
			}
			s.mu.Unlock()
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func validatePolicy(policy *Policy) error {
	if policy == nil {
		return errors.New("policy is required")
	}
	if policy.ID == "" {
		return errors.New("policy id is required")
	}
	if policy.NodeID == "" {
		return errors.New("nodeId is required")
	}
	if policy.MaxFailures < 0 {
		return errors.New("maxFailures must not be negative")
	}
	if policy.FailureWindowSec < 0 {
		return errors.New("failureWindowSec must not be negative")
	}
	if policy.CooldownSec < 0 {
		return errors.New("cooldownSec must not be negative")
	}
	switch policy.Action {
	case "", FailoverActionEvacuate, FailoverActionRestart, FailoverActionNotify:
		return nil
	default:
		return errors.New("invalid failover action")
	}
}

func (s *Service) Metrics() Metrics {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metrics
}

func toStorePolicy(p *Policy) store.FailoverPolicy {
	return store.FailoverPolicy{
		ID:               p.ID,
		Name:             p.Name,
		NodeID:           p.NodeID,
		Enabled:          p.Enabled,
		MaxFailures:      p.MaxFailures,
		FailureWindowSec: p.FailureWindowSec,
		CooldownSec:      p.CooldownSec,
		Action:           string(p.Action),
		HealthCheckPath:  p.HealthCheckPath,
		HealthCheckPort:  p.HealthCheckPort,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
	}
}

func fromStorePolicy(sp store.FailoverPolicy) Policy {
	return Policy{
		ID:               sp.ID,
		Name:             sp.Name,
		NodeID:           sp.NodeID,
		Enabled:          sp.Enabled,
		MaxFailures:      sp.MaxFailures,
		FailureWindowSec: sp.FailureWindowSec,
		CooldownSec:      sp.CooldownSec,
		Action:           FailoverAction(sp.Action),
		HealthCheckPath:  sp.HealthCheckPath,
		HealthCheckPort:  sp.HealthCheckPort,
		CreatedAt:        sp.CreatedAt,
		UpdatedAt:        sp.UpdatedAt,
	}
}

func toStoreEvent(e *Event) store.FailoverEvent {
	return store.FailoverEvent{
		PolicyID:  e.PolicyID,
		NodeID:    e.NodeID,
		ServerID:  e.ServerID,
		EventType: string(e.EventType),
		Action:    string(e.Action),
		Status:    e.Status,
		Message:   e.Message,
	}
}
