package autoscaler

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/runtime"
	"gamepanel/forge/internal/store"
)

type ScalingDirection string

const (
	ScalingDirectionUp   ScalingDirection = "up"
	ScalingDirectionDown ScalingDirection = "down"
)

type ScalingPolicy struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	ServerID            string    `json:"serverId"`
	Enabled             bool      `json:"enabled"`
	MinMemoryMB         int64     `json:"minMemoryMb"`
	MaxMemoryMB         int64     `json:"maxMemoryMb"`
	MinCPU              int64     `json:"minCpu"`
	MaxCPU              int64     `json:"maxCpu"`
	TargetCPUPercent    float64   `json:"targetCpuPercent"`
	TargetMemoryPercent float64   `json:"targetMemoryPercent"`
	ScaleUpThreshold    float64   `json:"scaleUpThreshold"`
	ScaleDownThreshold  float64   `json:"scaleDownThreshold"`
	CooldownSeconds     int       `json:"cooldownSeconds"`
	PollIntervalSeconds int       `json:"pollIntervalSeconds"`
	ScaleUpFactor       float64   `json:"scaleUpFactor"`
	ScaleDownFactor     float64   `json:"scaleDownFactor"`
	MaxScaleUpStepMB    int64     `json:"maxScaleUpStepMb"`
	MaxScaleDownStepMB  int64     `json:"maxScaleDownStepMb"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

type ScalingEvent struct {
	ID        string           `json:"id"`
	PolicyID  string           `json:"policyId"`
	ServerID  string           `json:"serverId"`
	Direction ScalingDirection `json:"direction"`
	OldMemory int64            `json:"oldMemory"`
	NewMemory int64            `json:"newMemory"`
	OldCPU    int64            `json:"oldCpu"`
	NewCPU    int64            `json:"newCpu"`
	CPUUsage  float64          `json:"cpuUsage"`
	MemUsage  float64          `json:"memUsage"`
	Reason    string           `json:"reason"`
	Success   bool             `json:"success"`
	Timestamp time.Time        `json:"timestamp"`
}

type Metrics struct {
	ScaleUpEventsTotal   uint64 `json:"scaleUpEventsTotal"`
	ScaleDownEventsTotal uint64 `json:"scaleDownEventsTotal"`
	ScalingErrorsTotal   uint64 `json:"scalingErrorsTotal"`
	ActivePolicies       int    `json:"activePolicies"`
}

type Service struct {
	st        *store.Store
	cluster   clusterInterface
	runtime   runtime.Runtime
	cooldowns map[string]time.Time
	mu        sync.RWMutex
	publisher events.Publisher
	metrics   Metrics
	stopCh    chan struct{}
	started   bool
}

type clusterInterface interface {
	GetServerStats(ctx context.Context, serverID string) (*runtime.Stats, error)
	ResizeServer(ctx context.Context, serverID string, memoryMB, cpu int64) error
}

var ErrNoPolicy = errors.New("scaling policy not found")

func New(store *store.Store, cluster clusterInterface, rt runtime.Runtime, publishers ...events.Publisher) *Service {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Service{
		st:        store,
		cluster:   cluster,
		runtime:   rt,
		cooldowns: make(map[string]time.Time),
		publisher: publisher,
		stopCh:    make(chan struct{}),
	}
}

func spFromStore(s store.ScalingPolicy) ScalingPolicy {
	return ScalingPolicy{
		ID:                  s.ID,
		Name:                s.Name,
		ServerID:            s.ServerID,
		Enabled:             s.Enabled,
		MinMemoryMB:         s.MinMemoryMB,
		MaxMemoryMB:         s.MaxMemoryMB,
		MinCPU:              s.MinCPU,
		MaxCPU:              s.MaxCPU,
		TargetCPUPercent:    s.TargetCPUPercent,
		TargetMemoryPercent: s.TargetMemoryPercent,
		ScaleUpThreshold:    s.ScaleUpThreshold,
		ScaleDownThreshold:  s.ScaleDownThreshold,
		CooldownSeconds:     s.CooldownSeconds,
		PollIntervalSeconds: s.PollIntervalSeconds,
		ScaleUpFactor:       s.ScaleUpFactor,
		ScaleDownFactor:     s.ScaleDownFactor,
		MaxScaleUpStepMB:    s.MaxScaleUpStepMB,
		MaxScaleDownStepMB:  s.MaxScaleDownStepMB,
		CreatedAt:           s.CreatedAt,
		UpdatedAt:           s.UpdatedAt,
	}
}

func spToStore(p *ScalingPolicy) store.ScalingPolicy {
	return store.ScalingPolicy{
		ID:                  p.ID,
		Name:                p.Name,
		ServerID:            p.ServerID,
		Enabled:             p.Enabled,
		MinMemoryMB:         p.MinMemoryMB,
		MaxMemoryMB:         p.MaxMemoryMB,
		MinCPU:              p.MinCPU,
		MaxCPU:              p.MaxCPU,
		TargetCPUPercent:    p.TargetCPUPercent,
		TargetMemoryPercent: p.TargetMemoryPercent,
		ScaleUpThreshold:    p.ScaleUpThreshold,
		ScaleDownThreshold:  p.ScaleDownThreshold,
		CooldownSeconds:     p.CooldownSeconds,
		PollIntervalSeconds: p.PollIntervalSeconds,
		ScaleUpFactor:       p.ScaleUpFactor,
		ScaleDownFactor:     p.ScaleDownFactor,
		MaxScaleUpStepMB:    p.MaxScaleUpStepMB,
		MaxScaleDownStepMB:  p.MaxScaleDownStepMB,
		CreatedAt:           p.CreatedAt,
		UpdatedAt:           p.UpdatedAt,
	}
}

func (s *Service) CreatePolicy(ctx context.Context, policy *ScalingPolicy) error {
	if policy == nil {
		return errors.New("policy is required")
	}
	if policy.ServerID == "" {
		return errors.New("serverId is required")
	}
	if policy.ID == "" {
		return errors.New("policy id is required")
	}
	if policy.PollIntervalSeconds <= 0 {
		policy.PollIntervalSeconds = 30
	}
	if policy.ScaleUpFactor <= 0 {
		policy.ScaleUpFactor = 1.25
	}
	if policy.ScaleDownFactor <= 0 {
		policy.ScaleDownFactor = 0.75
	}
	if policy.CooldownSeconds <= 0 {
		policy.CooldownSeconds = 120
	}
	if policy.ScaleUpThreshold <= 0 {
		policy.ScaleUpThreshold = 0.80
	}
	if policy.ScaleDownThreshold <= 0 {
		policy.ScaleDownThreshold = 0.30
	}
	if policy.TargetCPUPercent <= 0 {
		policy.TargetCPUPercent = 0.70
	}
	if policy.TargetMemoryPercent <= 0 {
		policy.TargetMemoryPercent = 0.70
	}
	storePolicy := spToStore(policy)
	if err := s.st.CreateScalingPolicy(ctx, &storePolicy); err != nil {
		return err
	}
	*policy = spFromStore(storePolicy)
	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("scaling_policy_created", "autoscaler", "policy", policy.ID, map[string]any{
			"serverId": policy.ServerID, "name": policy.Name,
		}))
	}
	return nil
}

func (s *Service) UpdatePolicy(ctx context.Context, policy *ScalingPolicy) error {
	storePolicy := spToStore(policy)
	if err := s.st.UpdateScalingPolicy(ctx, &storePolicy); err != nil {
		return err
	}
	policy.UpdatedAt = storePolicy.UpdatedAt
	return nil
}

func (s *Service) DeletePolicy(ctx context.Context, policyID string) error {
	s.mu.Lock()
	delete(s.cooldowns, policyID)
	s.mu.Unlock()
	return s.st.DeleteScalingPolicy(ctx, policyID)
}

func (s *Service) GetPolicy(ctx context.Context, policyID string) (*ScalingPolicy, error) {
	p, err := s.st.GetScalingPolicy(ctx, policyID)
	if err != nil {
		return nil, ErrNoPolicy
	}
	pol := spFromStore(p)
	return &pol, nil
}

func (s *Service) ListPolicies(ctx context.Context) ([]*ScalingPolicy, error) {
	policies, err := s.st.ListScalingPolicies(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*ScalingPolicy, len(policies))
	for i := range policies {
		p := spFromStore(policies[i])
		result[i] = &p
	}
	return result, nil
}

func (s *Service) ListPoliciesByServer(ctx context.Context, serverID string) ([]*ScalingPolicy, error) {
	policies, err := s.st.ListScalingPoliciesByServer(ctx, serverID)
	if err != nil {
		return nil, err
	}
	result := make([]*ScalingPolicy, len(policies))
	for i := range policies {
		p := spFromStore(policies[i])
		result[i] = &p
	}
	return result, nil
}

func (s *Service) EvaluateServer(ctx context.Context, serverID string) (*ScalingEvent, error) {
	storePolicies, err := s.st.ListScalingPoliciesByServer(ctx, serverID)
	if err != nil {
		return nil, err
	}

	var targetPolicy *ScalingPolicy
	for _, p := range storePolicies {
		if p.Enabled {
			pol := spFromStore(p)
			targetPolicy = &pol
			break
		}
	}

	if targetPolicy == nil {
		return nil, nil
	}

	s.mu.RLock()
	if cooldownUntil, ok := s.cooldowns[targetPolicy.ID]; ok {
		if time.Now().UTC().Before(cooldownUntil) {
			s.mu.RUnlock()
			return nil, nil
		}
	}
	s.mu.RUnlock()

	stats, err := s.cluster.GetServerStats(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("get server stats: %w", err)
	}

	cpuUsage := stats.CPUPercent / 100.0
	memUsage := float64(stats.MemoryBytes) / float64(max(stats.MemoryLimit, 1))

	event := &ScalingEvent{
		PolicyID:  targetPolicy.ID,
		ServerID:  serverID,
		CPUUsage:  cpuUsage,
		MemUsage:  memUsage,
		Timestamp: time.Now().UTC(),
	}

	if cpuUsage > targetPolicy.ScaleUpThreshold || memUsage > targetPolicy.ScaleUpThreshold {
		return s.executeScaleUp(ctx, targetPolicy, event)
	}

	if cpuUsage < targetPolicy.ScaleDownThreshold && memUsage < targetPolicy.ScaleDownThreshold {
		return s.executeScaleDown(ctx, targetPolicy, event)
	}

	return nil, nil
}

func (s *Service) executeScaleUp(ctx context.Context, policy *ScalingPolicy, event *ScalingEvent) (*ScalingEvent, error) {
	event.Direction = ScalingDirectionUp
	newMemory := int64(math.Ceil(float64(policy.MinMemoryMB) * policy.ScaleUpFactor))
	newCPU := int64(math.Ceil(float64(policy.MinCPU) * policy.ScaleUpFactor))

	if policy.MaxMemoryMB > 0 && newMemory > policy.MaxMemoryMB {
		newMemory = policy.MaxMemoryMB
	}
	if policy.MaxCPU > 0 && newCPU > policy.MaxCPU {
		newCPU = policy.MaxCPU
	}
	if policy.MaxScaleUpStepMB > 0 && (newMemory-policy.MinMemoryMB) > policy.MaxScaleUpStepMB {
		newMemory = policy.MinMemoryMB + policy.MaxScaleUpStepMB
	}

	event.OldMemory = policy.MinMemoryMB
	event.NewMemory = newMemory
	event.OldCPU = policy.MinCPU
	event.NewCPU = newCPU
	event.Reason = fmt.Sprintf("scaling up: cpu=%.1f%%, mem=%.1f%% exceeded threshold", event.CPUUsage*100, event.MemUsage*100)

	policy.MinMemoryMB = newMemory
	policy.MinCPU = newCPU

	s.mu.Lock()
	s.cooldowns[policy.ID] = time.Now().UTC().Add(time.Duration(policy.CooldownSeconds) * time.Second)
	s.metrics.ScaleUpEventsTotal++
	s.mu.Unlock()

	storePolicy := spToStore(policy)
	if err := s.st.UpdateScalingPolicy(ctx, &storePolicy); err != nil {
		return nil, fmt.Errorf("persist policy after scale up: %w", err)
	}
	policy.UpdatedAt = storePolicy.UpdatedAt

	if err := s.cluster.ResizeServer(ctx, policy.ServerID, newMemory, newCPU); err != nil {
		s.mu.Lock()
		s.metrics.ScalingErrorsTotal++
		s.mu.Unlock()
		event.Reason = fmt.Sprintf("%s; resize failed: %v", event.Reason, err)
		event.Success = false
	} else {
		event.Success = true
	}
	storeEvent := store.ScalingEvent{
		PolicyID:  event.PolicyID,
		ServerID:  event.ServerID,
		Direction: string(event.Direction),
		OldMemory: event.OldMemory,
		NewMemory: event.NewMemory,
		OldCPU:    event.OldCPU,
		NewCPU:    event.NewCPU,
		CPUUsage:  event.CPUUsage,
		MemUsage:  event.MemUsage,
		Reason:    event.Reason,
		Success:   event.Success,
	}
	if err := s.st.CreateScalingEvent(ctx, &storeEvent); err != nil {
		return nil, fmt.Errorf("persist scaling event: %w", err)
	}
	event.ID = storeEvent.ID
	event.Timestamp = storeEvent.CreatedAt

	if s.publisher != nil {
		_ = s.publisher.Publish(context.Background(), events.NewEnvelope("server_scaled_up", "autoscaler", "server", event.ServerID, map[string]any{
			"policyId": event.PolicyID, "oldMemory": event.OldMemory, "newMemory": event.NewMemory,
			"oldCpu": event.OldCPU, "newCpu": event.NewCPU, "reason": event.Reason,
		}))
	}

	return event, nil
}

func (s *Service) executeScaleDown(ctx context.Context, policy *ScalingPolicy, event *ScalingEvent) (*ScalingEvent, error) {
	event.Direction = ScalingDirectionDown
	newMemory := int64(math.Ceil(float64(policy.MinMemoryMB) * policy.ScaleDownFactor))
	newCPU := int64(math.Ceil(float64(policy.MinCPU) * policy.ScaleDownFactor))

	if policy.MaxScaleDownStepMB > 0 && (policy.MinMemoryMB-newMemory) > policy.MaxScaleDownStepMB {
		newMemory = policy.MinMemoryMB - policy.MaxScaleDownStepMB
	}
	if newMemory < 256 {
		newMemory = 256
	}
	if newCPU < 1 {
		newCPU = 1
	}

	event.OldMemory = policy.MinMemoryMB
	event.NewMemory = newMemory
	event.OldCPU = policy.MinCPU
	event.NewCPU = newCPU
	event.Reason = fmt.Sprintf("scaling down: cpu=%.1f%%, mem=%.1f%% below threshold", event.CPUUsage*100, event.MemUsage*100)

	policy.MinMemoryMB = newMemory
	policy.MinCPU = newCPU

	s.mu.Lock()
	s.cooldowns[policy.ID] = time.Now().UTC().Add(time.Duration(policy.CooldownSeconds) * time.Second)
	s.metrics.ScaleDownEventsTotal++
	s.mu.Unlock()

	storePolicy := spToStore(policy)
	if err := s.st.UpdateScalingPolicy(ctx, &storePolicy); err != nil {
		return nil, fmt.Errorf("persist policy after scale down: %w", err)
	}
	policy.UpdatedAt = storePolicy.UpdatedAt

	if err := s.cluster.ResizeServer(ctx, policy.ServerID, newMemory, newCPU); err != nil {
		s.mu.Lock()
		s.metrics.ScalingErrorsTotal++
		s.mu.Unlock()
		event.Reason = fmt.Sprintf("%s; resize failed: %v", event.Reason, err)
		event.Success = false
	} else {
		event.Success = true
	}

	storeEvent := store.ScalingEvent{
		PolicyID:  event.PolicyID,
		ServerID:  event.ServerID,
		Direction: string(event.Direction),
		OldMemory: event.OldMemory,
		NewMemory: event.NewMemory,
		OldCPU:    event.OldCPU,
		NewCPU:    event.NewCPU,
		CPUUsage:  event.CPUUsage,
		MemUsage:  event.MemUsage,
		Reason:    event.Reason,
		Success:   event.Success,
	}
	if err := s.st.CreateScalingEvent(ctx, &storeEvent); err != nil {
		return nil, fmt.Errorf("persist scaling event: %w", err)
	}
	event.ID = storeEvent.ID
	event.Timestamp = storeEvent.CreatedAt

	if s.publisher != nil {
		_ = s.publisher.Publish(context.Background(), events.NewEnvelope("server_scaled_down", "autoscaler", "server", event.ServerID, map[string]any{
			"policyId": event.PolicyID, "oldMemory": event.OldMemory, "newMemory": event.NewMemory,
			"oldCpu": event.OldCPU, "newCpu": event.NewCPU, "reason": event.Reason,
		}))
	}

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

	cooldowns := make(map[string]time.Time)
	policies, err := s.st.ListScalingPolicies(ctx)
	if err == nil {
		for _, p := range policies {
			lastEvent, err := s.st.GetLastScalingEvent(ctx, p.ID)
			if err == nil && lastEvent.Success {
				cooldownEnd := lastEvent.CreatedAt.Add(time.Duration(p.CooldownSeconds) * time.Second)
				if time.Now().UTC().Before(cooldownEnd) {
					cooldowns[p.ID] = cooldownEnd
				}
			}
		}
	}
	s.mu.Lock()
	s.cooldowns = cooldowns
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
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			policies, err := s.st.ListScalingPolicies(ctx)
			if err != nil {
				continue
			}
			for _, policy := range policies {
				if !policy.Enabled {
					continue
				}
				if _, err := s.EvaluateServer(ctx, policy.ServerID); err != nil {
					s.mu.Lock()
					s.metrics.ScalingErrorsTotal++
					s.mu.Unlock()
				}
			}
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) Metrics() Metrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.metrics
	if s.st != nil {
		policies, err := s.st.ListScalingPolicies(context.Background())
		if err == nil {
			m.ActivePolicies = len(policies)
		}
	}
	return m
}

func max(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}
