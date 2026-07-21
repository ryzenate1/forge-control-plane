package clustermembership

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/services/evacuationplanner"
	"gamepanel/forge/internal/store"
)

type Metrics struct {
	NodesJoinedTotal    uint64 `json:"nodes_joined_total"`
	NodesLeftTotal      uint64 `json:"nodes_left_total"`
	DrainStartedTotal   uint64 `json:"drain_started_total"`
	DrainCompletedTotal uint64 `json:"drain_completed_total"`
	MaintStartedTotal   uint64 `json:"maintenance_started_total"`
	MaintEndedTotal     uint64 `json:"maintenance_ended_total"`
}

type membershipStore interface {
	GetNode(ctx context.Context, nodeID string) (store.Node, error)
	UpdateNode(ctx context.Context, nodeID string, req store.UpdateNodeRequest, actorID *string) (store.Node, error)
	ListServersForNode(ctx context.Context, nodeID string) ([]store.Server, error)
}

type EvacuationPlanner interface {
	CreatePlan(ctx context.Context, nodeID string) (evacuationplanner.PlanResult, error)
	ExecutePlan(ctx context.Context, planID string) (store.EvacuationPlan, error)
}

type TrafficManager interface {
	WithdrawNodeTargets(ctx context.Context, nodeID string) error
	ReinstateNodeTargets(ctx context.Context, nodeID string) error
}

type Service struct {
	store      membershipStore
	evacuator  EvacuationPlanner
	publisher  events.Publisher
	trafficMgr TrafficManager
	mu         sync.Mutex
	metrics    Metrics
	draining   map[string]chan struct{}
}

func New(store *store.Store, publishers ...events.Publisher) *Service {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Service{
		store:     store,
		publisher: publisher,
		draining:  make(map[string]chan struct{}),
	}
}

func (s *Service) SetTrafficManager(tm TrafficManager) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trafficMgr = tm
}

func (s *Service) SetEvacuationPlanner(evacuator EvacuationPlanner) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evacuator = evacuator
}

func (s *Service) trafficManager() TrafficManager {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.trafficMgr
}

func (s *Service) evacuationPlanner() EvacuationPlanner {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.evacuator
}

func (s *Service) Metrics() Metrics {
	if s == nil {
		return Metrics{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metrics
}

func (s *Service) Join(ctx context.Context, nodeID string) error {
	if s == nil || s.store == nil {
		return errors.New("membership service unavailable")
	}
	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("get node: %w", err)
	}
	if node.HeartbeatState != string(store.NodeHeartbeatStateOffline) && node.DesiredState != "" {
		return errors.New("node is already active in the cluster")
	}
	req := store.UpdateNodeRequest{DesiredState: store.NodeDesiredStateActive, Status: "online"}
	if _, err := s.store.UpdateNode(ctx, nodeID, req, nil); err != nil {
		return fmt.Errorf("activate node: %w", err)
	}
	s.increment(func(m *Metrics) { m.NodesJoinedTotal++ })
	s.publish(ctx, events.EventNodeOnline, "node", nodeID, map[string]any{
		"previousState": node.HeartbeatState,
		"action":        "join",
	})
	return nil
}

func (s *Service) Leave(ctx context.Context, nodeID string) error {
	if s == nil || s.store == nil {
		return errors.New("membership service unavailable")
	}
	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("get node: %w", err)
	}
	servers, err := s.store.ListServersForNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("list servers: %w", err)
	}
	if len(servers) > 0 {
		return fmt.Errorf("node has %d active workloads; drain before leaving", len(servers))
	}
	req := store.UpdateNodeRequest{
		DesiredState: store.NodeDesiredStateActive,
		Status:       "offline",
		Draining:     false,
		Maintenance:  false,
	}
	if _, err := s.store.UpdateNode(ctx, nodeID, req, nil); err != nil {
		return fmt.Errorf("deactivate node: %w", err)
	}
	s.increment(func(m *Metrics) { m.NodesLeftTotal++ })
	s.publish(ctx, events.EventNodeOffline, "node", nodeID, map[string]any{
		"previousState": node.HeartbeatState,
		"action":        "leave",
	})
	return nil
}

func (s *Service) StartDrain(ctx context.Context, nodeID string) error {
	if s == nil || s.store == nil {
		return errors.New("membership service unavailable")
	}
	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("get node: %w", err)
	}
	if node.Draining {
		return errors.New("node is already draining")
	}
	if s.evacuationPlanner() == nil {
		return errors.New("evacuation planner unavailable")
	}
	req := store.UpdateNodeRequest{
		Draining:     true,
		DesiredState: store.NodeDesiredStateDraining,
	}
	if _, err := s.store.UpdateNode(ctx, nodeID, req, nil); err != nil {
		return fmt.Errorf("set draining: %w", err)
	}

	// Withdraw gateway targets for the draining node
	if tm := s.trafficManager(); tm != nil {
		if err := tm.WithdrawNodeTargets(ctx, nodeID); err != nil {
			slog.Warn("failed to withdraw node targets during drain", "nodeID", nodeID, "error", err)
		}
	}
	done := make(chan struct{})
	s.mu.Lock()
	s.draining[nodeID] = done
	s.mu.Unlock()

	s.increment(func(m *Metrics) { m.DrainStartedTotal++ })
	s.publish(ctx, events.EventNodeDrainingStarted, "node", nodeID, map[string]any{})

	go func() {
		defer close(done)
		result, err := s.evacuationPlanner().CreatePlan(ctx, nodeID)
		if err != nil {
			s.publish(ctx, events.EventEvacuationPlanFailed, "node", nodeID, map[string]any{"error": err.Error()})
			// Update node state back to active on failure
			req := store.UpdateNodeRequest{Draining: false, DesiredState: store.NodeDesiredStateActive}
			_, _ = s.store.UpdateNode(ctx, nodeID, req, nil)
			return
		}
		if result.Plan.ID == "" || result.Plan.Status == store.EvacuationPlanStatusFailed {
			s.publish(ctx, events.EventEvacuationPlanFailed, "node", nodeID, map[string]any{"error": "evacuation plan failed"})
			// Update node state back to active on failure
			req := store.UpdateNodeRequest{Draining: false, DesiredState: store.NodeDesiredStateActive}
			_, _ = s.store.UpdateNode(ctx, nodeID, req, nil)
			return
		}
		if result.Plan.Status == store.EvacuationPlanStatusCompleted {
			s.completeDrain(ctx, nodeID)
			return
		}
		if _, err := s.evacuationPlanner().ExecutePlan(ctx, result.Plan.ID); err != nil {
			s.publish(ctx, events.EventEvacuationPlanFailed, "node", nodeID, map[string]any{"error": err.Error()})
			// Update node state back to active on failure
			req := store.UpdateNodeRequest{Draining: false, DesiredState: store.NodeDesiredStateActive}
			_, _ = s.store.UpdateNode(ctx, nodeID, req, nil)
			return
		}
		s.completeDrain(ctx, nodeID)
	}()

	return nil
}

func (s *Service) completeDrain(ctx context.Context, nodeID string) {
	s.mu.Lock()
	if ch, ok := s.draining[nodeID]; ok {
		select {
		case <-ch:
		default:
			close(ch)
		}
		delete(s.draining, nodeID)
	}
	s.mu.Unlock()

	// Update node state to mark drain as complete
	req := store.UpdateNodeRequest{
		Draining:     false,
		DesiredState: store.NodeDesiredStateActive,
	}
	if _, err := s.store.UpdateNode(ctx, nodeID, req, nil); err != nil {
		slog.Error("failed to update node state after drain completion", "nodeID", nodeID, "error", err)
	} else {
		// Reinstate gateway targets for the node
		if tm := s.trafficManager(); tm != nil {
			if err := tm.ReinstateNodeTargets(ctx, nodeID); err != nil {
				slog.Warn("failed to reinstate node targets after drain", "nodeID", nodeID, "error", err)
			}
		}
	}

	s.increment(func(m *Metrics) { m.DrainCompletedTotal++ })
	s.publish(ctx, events.EventNodeDrainingCompleted, "node", nodeID, map[string]any{})
}

func (s *Service) CancelDrain(ctx context.Context, nodeID string) error {
	if s == nil || s.store == nil {
		return errors.New("membership service unavailable")
	}
	s.mu.Lock()
	if ch, ok := s.draining[nodeID]; ok {
		close(ch)
		delete(s.draining, nodeID)
	}
	s.mu.Unlock()

	// Reinstate gateway targets for the node
	if tm := s.trafficManager(); tm != nil {
		if err := tm.ReinstateNodeTargets(ctx, nodeID); err != nil {
			slog.Warn("failed to reinstate node targets during drain cancellation", "nodeID", nodeID, "error", err)
		}
	}

	req := store.UpdateNodeRequest{Draining: false, DesiredState: store.NodeDesiredStateActive}
	if _, err := s.store.UpdateNode(ctx, nodeID, req, nil); err != nil {
		return fmt.Errorf("cancel drain: %w", err)
	}
	s.publish(ctx, events.EventActualStateChanged, "node", nodeID, map[string]any{
		"draining": false, "desiredState": "active",
	})
	return nil
}

func (s *Service) DrainStatus(ctx context.Context, nodeID string) (string, error) {
	if s == nil || s.store == nil {
		return "", errors.New("membership service unavailable")
	}
	s.mu.Lock()
	_, draining := s.draining[nodeID]
	s.mu.Unlock()
	if draining {
		return "draining", nil
	}
	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		return "", fmt.Errorf("get node: %w", err)
	}
	if node.Draining {
		return "draining_idle", nil
	}
	return "idle", nil
}

func (s *Service) EnableMaintenance(ctx context.Context, nodeID, message string) error {
	if s == nil || s.store == nil {
		return errors.New("membership service unavailable")
	}
	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("get node: %w", err)
	}
	if node.Draining {
		return errors.New("node is draining; wait for drain to complete")
	}
	req := store.UpdateNodeRequest{
		Maintenance:        true,
		DesiredState:       store.NodeDesiredStateMaintenance,
		MaintenanceMessage: message,
	}
	if _, err := s.store.UpdateNode(ctx, nodeID, req, nil); err != nil {
		return fmt.Errorf("enable maintenance: %w", err)
	}
	s.increment(func(m *Metrics) { m.MaintStartedTotal++ })
	s.publish(ctx, events.EventNodeMaintenanceStarted, "node", nodeID, map[string]any{
		"message": message,
	})
	return nil
}

func (s *Service) DisableMaintenance(ctx context.Context, nodeID string) error {
	if s == nil || s.store == nil {
		return errors.New("membership service unavailable")
	}
	req := store.UpdateNodeRequest{
		Maintenance:        false,
		DesiredState:       store.NodeDesiredStateActive,
		MaintenanceMessage: "",
	}
	if _, err := s.store.UpdateNode(ctx, nodeID, req, nil); err != nil {
		return fmt.Errorf("disable maintenance: %w", err)
	}
	s.increment(func(m *Metrics) { m.MaintEndedTotal++ })
	s.publish(ctx, events.EventNodeMaintenanceEnded, "node", nodeID, map[string]any{})
	return nil
}

func (s *Service) publish(ctx context.Context, eventType events.EventType, resourceType, resourceID string, payload map[string]any) {
	if s == nil || s.publisher == nil {
		return
	}
	if correlationID := events.CorrelationIDFromContext(ctx); correlationID != "" {
		if _, exists := payload["correlationId"]; !exists {
			payload["correlationId"] = correlationID
		}
	}
	_ = s.publisher.Publish(ctx, events.NewEnvelope(eventType, "cluster-membership", resourceType, resourceID, payload))
}

func (s *Service) increment(update func(*Metrics)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	update(&s.metrics)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
