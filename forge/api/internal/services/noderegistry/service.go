package noderegistry

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"
)

type Service struct {
	store     *store.Store
	publisher events.Publisher
}

type NodeHealthScore struct {
	CPU       int `json:"cpu"`
	Memory    int `json:"memory"`
	Disk      int `json:"disk"`
	Heartbeat int `json:"heartbeat"`
	Status    int `json:"status"`
	Total     int `json:"total"`
}

type NodeLifecycleView struct {
	Node                   store.Node                 `json:"node"`
	Health                 domain.NodeHealth          `json:"health"`
	HealthScore            NodeHealthScore            `json:"healthScore"`
	Capacity               store.NodeCapacitySnapshot `json:"capacity"`
	Draining               bool                       `json:"draining"`
	Maintenance            bool                       `json:"maintenance"`
	PlacementEligible      bool                       `json:"placementEligible"`
	PlacementBlockedReason string                     `json:"placementBlockedReason,omitempty"`
}

func New(store *store.Store, publishers ...events.Publisher) *Service {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Service{store: store, publisher: publisher}
}

func (s *Service) RegisterNode(ctx context.Context, req store.CreateNodeRequest, actorID *string) (store.Node, string, error) {
	return s.store.CreateNode(ctx, req, actorID)
}

func (s *Service) UpdateNode(ctx context.Context, nodeID string, req store.UpdateNodeRequest, actorID *string) (store.Node, error) {
	before, beforeErr := s.store.GetNode(ctx, nodeID)
	if beforeErr != nil {
		slog.Warn("failed to fetch node state before update, skipping events", "nodeID", nodeID, "error", beforeErr)
	}
	node, err := s.store.UpdateNode(ctx, nodeID, req, actorID)
	if err != nil {
		return store.Node{}, err
	}
	if beforeErr == nil && before.DesiredState != node.DesiredState {
		s.publish(ctx, events.EventDesiredStateChanged, "node", node.ID, map[string]any{
			"from": before.DesiredState,
			"to":   node.DesiredState,
		})
		s.publishNodeLifecycleEvents(ctx, before, node)
	}
	return node, nil
}

func (s *Service) PatchNode(ctx context.Context, nodeID string, patch store.NodePatch, actorID *string) (store.Node, error) {
	before, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		return store.Node{}, err
	}
	node, err := s.store.PatchNode(ctx, nodeID, patch, actorID)
	if err != nil {
		return store.Node{}, err
	}
	if before.DesiredState != node.DesiredState {
		s.publish(ctx, events.EventDesiredStateChanged, "node", node.ID, map[string]any{"from": before.DesiredState, "to": node.DesiredState})
		s.publishNodeLifecycleEvents(ctx, before, node)
	}
	return node, nil
}

func (s *Service) GetNode(ctx context.Context, nodeID string) (store.Node, error) {
	return s.store.GetNode(ctx, nodeID)
}

func (s *Service) ListNodes(ctx context.Context) ([]store.Node, error) {
	return s.store.ListNodes(ctx)
}

func (s *Service) DeleteNode(ctx context.Context, nodeID string, actorID *string) error {
	return s.store.DeleteNode(ctx, nodeID, actorID)
}

func (s *Service) RotateNodeToken(ctx context.Context, nodeID string, actorID *string) (string, error) {
	return s.store.RotateNodeToken(ctx, nodeID, actorID)
}

func (s *Service) ListRegions(ctx context.Context) ([]store.Region, error) {
	return s.store.ListRegions(ctx)
}

func (s *Service) GetRegion(ctx context.Context, regionID string) (store.Region, error) {
	return s.store.GetRegion(ctx, regionID)
}

func (s *Service) CreateRegion(ctx context.Context, req store.CreateRegionRequest, actorID *string) (store.Region, error) {
	return s.store.CreateRegion(ctx, req, actorID)
}

func (s *Service) UpdateRegion(ctx context.Context, regionID string, req store.UpdateRegionRequest, actorID *string) (store.Region, error) {
	return s.store.UpdateRegion(ctx, regionID, req, actorID)
}

func (s *Service) DeleteRegion(ctx context.Context, regionID string, actorID *string) error {
	return s.store.DeleteRegion(ctx, regionID, actorID)
}

func (s *Service) VerifyNodeToken(ctx context.Context, nodeID, token string) (bool, error) {
	return s.store.VerifyNodeToken(ctx, nodeID, token)
}

func (s *Service) AuthenticateRemoteNode(ctx context.Context, bearer string) (store.Node, error) {
	return s.store.AuthenticateRemoteNode(ctx, bearer)
}

func (s *Service) RecordHeartbeat(ctx context.Context, nodeID string, req store.NodeHeartbeatRequest) (store.Node, error) {
	before, beforeErr := s.store.GetNode(ctx, nodeID)
	if beforeErr != nil {
		slog.Warn("failed to fetch node state before heartbeat, skipping events", "nodeID", nodeID, "error", beforeErr)
	}
	node, err := s.store.UpdateNodeHeartbeat(ctx, nodeID, req)
	if err != nil {
		return store.Node{}, err
	}
	if beforeErr == nil && before.ActualState != node.ActualState {
		s.publishActualStateEvents(ctx, before.ActualState, node)
	}
	return node, nil
}

func (s *Service) publishActualStateEvents(ctx context.Context, beforeState string, node store.Node) {
	s.publish(ctx, events.EventActualStateChanged, "node", node.ID, map[string]any{
		"from": beforeState,
		"to":   node.ActualState,
	})
	switch node.ActualState {
	case string(domain.NodeActualStateOnline):
		s.publish(ctx, events.EventNodeOnline, "node", node.ID, map[string]any{"status": node.ActualState})
	case string(domain.NodeActualStateOffline):
		s.publish(ctx, events.EventNodeOffline, "node", node.ID, map[string]any{"status": node.ActualState})
	case string(domain.NodeActualStateDegraded):
		s.publish(ctx, events.EventNodeDegraded, "node", node.ID, map[string]any{"status": node.ActualState})
	}
}

func (s *Service) publish(ctx context.Context, eventType events.EventType, resourceType, resourceID string, payload map[string]any) {
	if s == nil || s.publisher == nil {
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	if correlationID := events.CorrelationIDFromContext(ctx); correlationID != "" {
		if _, exists := payload["correlationId"]; !exists {
			payload["correlationId"] = correlationID
		}
	}
	_ = s.publisher.Publish(ctx, events.NewEnvelope(eventType, "node-registry", resourceType, resourceID, payload))
}

func (s *Service) LifecycleView(ctx context.Context, nodeID string) (NodeLifecycleView, error) {
	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		return NodeLifecycleView{}, err
	}
	capacity, err := s.store.NodeCapacitySnapshot(ctx, node.ID)
	if err != nil {
		return NodeLifecycleView{}, err
	}
	view := NodeLifecycleView{
		Node:        node,
		Health:      s.Health(node),
		HealthScore: s.HealthScore(node, capacity),
		Capacity:    capacity,
		Draining:    node.DesiredState == store.NodeDesiredStateDraining || node.Draining,
		Maintenance: node.DesiredState == store.NodeDesiredStateMaintenance || node.Maintenance,
	}
	view.PlacementEligible, view.PlacementBlockedReason = PlacementEligibility(node, capacity)
	return view, nil
}

func (s *Service) HealthScore(node store.Node, capacity store.NodeCapacitySnapshot) NodeHealthScore {
	score := NodeHealthScore{
		CPU:       resourceScore(capacity.TotalCPU, capacity.AvailableCPU),
		Memory:    resourceScore(capacity.TotalMemory, capacity.AvailableMemory),
		Disk:      resourceScore(capacity.TotalDisk, capacity.AvailableDisk),
		Heartbeat: heartbeatScore(node.LastSeenAt),
		Status:    statusScore(node.ActualState),
	}
	score.Total = (score.CPU + score.Memory + score.Disk + score.Heartbeat + score.Status) / 5
	return score
}

func PlacementEligibility(node store.Node, capacity store.NodeCapacitySnapshot) (bool, string) {
	if node.DesiredState == store.NodeDesiredStateMaintenance || node.Maintenance {
		return false, "maintenance"
	}
	if node.DesiredState == store.NodeDesiredStateDraining || node.Draining {
		return false, "draining"
	}
	if node.ActualState != string(domain.NodeActualStateOnline) {
		return false, "node is not online"
	}
	if capacity.TotalCPU > 0 && capacity.AvailableCPU <= 0 {
		return false, "cpu exhausted"
	}
	if capacity.TotalMemory > 0 && capacity.AvailableMemory <= 0 {
		return false, "memory exhausted"
	}
	if capacity.TotalDisk > 0 && capacity.AvailableDisk <= 0 {
		return false, "disk exhausted"
	}
	return true, ""
}

func (s *Service) publishNodeLifecycleEvents(ctx context.Context, before, after store.Node) {
	if before.DesiredState != store.NodeDesiredStateDraining && after.DesiredState == store.NodeDesiredStateDraining {
		s.publish(ctx, events.EventNodeDrainingStarted, "node", after.ID, map[string]any{"from": before.DesiredState, "to": after.DesiredState})
	}
	if before.DesiredState == store.NodeDesiredStateDraining && after.DesiredState != store.NodeDesiredStateDraining {
		s.publish(ctx, events.EventNodeDrainingCompleted, "node", after.ID, map[string]any{"from": before.DesiredState, "to": after.DesiredState})
	}
	if before.DesiredState != store.NodeDesiredStateMaintenance && after.DesiredState == store.NodeDesiredStateMaintenance {
		s.publish(ctx, events.EventNodeMaintenanceStarted, "node", after.ID, map[string]any{"from": before.DesiredState, "to": after.DesiredState})
	}
	if before.DesiredState == store.NodeDesiredStateMaintenance && after.DesiredState != store.NodeDesiredStateMaintenance {
		s.publish(ctx, events.EventNodeMaintenanceEnded, "node", after.ID, map[string]any{"from": before.DesiredState, "to": after.DesiredState})
	}
}

// Health returns a NodeHealth derived entirely from stored node data.
// It does NOT actively probe the Docker daemon or any other runtime endpoint;
// the result reflects the last known state reported via heartbeat.
func (s *Service) Health(node store.Node) domain.NodeHealth {
	runtime := "unknown"
	if node.DockerStatus != nil && strings.TrimSpace(*node.DockerStatus) != "" {
		runtime = strings.TrimSpace(*node.DockerStatus)
	}
	if node.HeartbeatErr != nil && strings.TrimSpace(*node.HeartbeatErr) != "" {
		runtime = "error"
	}
	return domain.NodeHealth{
		CPU:     "unknown",
		Memory:  healthSignal(node.NodeMemoryMB),
		Disk:    healthSignal(node.NodeDiskMB),
		Network: healthFromStatus(node.Status),
		Runtime: runtime,
	}
}

func (s *Service) Capacity(node store.Node) domain.NodeCapacity {
	capacity := domain.NodeCapacity{
		MemoryMB: node.MemoryMB,
		DiskMB:   node.DiskMB,
	}
	if node.CPUThreads != nil {
		capacity.CPUThreads = *node.CPUThreads
	}
	if node.NodeMemoryMB != nil && *node.NodeMemoryMB > 0 {
		capacity.MemoryMB = *node.NodeMemoryMB
	}
	if node.NodeDiskMB != nil && *node.NodeDiskMB > 0 {
		capacity.DiskMB = *node.NodeDiskMB
	}
	return capacity
}

func healthSignal(value *int) string {
	if value != nil && *value > 0 {
		return "ok"
	}
	return "unknown"
}

func healthFromStatus(status string) string {
	switch domain.NodeStatus(status) {
	case domain.NodeStatusOnline:
		return "ok"
	case domain.NodeStatusDegraded:
		return "degraded"
	case domain.NodeStatusMaintenance, domain.NodeStatusDraining:
		return "maintenance"
	default:
		return "unknown"
	}
}

// resourceScore returns 0–100 based on the fraction of capacity in use.
// When available > total (overcommitted), used becomes negative, is clamped
// to 0, and the node scores 100 — i.e. overcommit is treated as fully healthy.
func resourceScore(total, available int) int {
	if total <= 0 {
		return 50
	}
	used := total - available
	if used < 0 {
		used = 0
	}
	score := 100 - ((used * 100) / total)
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func heartbeatScore(lastSeen *time.Time) int {
	if lastSeen == nil {
		return 0
	}
	return heartbeatScoreAge(time.Since(*lastSeen))
}

// heartbeatScoreAge returns a deterministic score based on elapsed time
// since the last heartbeat. Useful in tests where time.Since would be
// non-deterministic.
func heartbeatScoreAge(age time.Duration) int {
	switch {
	case age <= 2*time.Minute:
		return 100
	case age <= 5*time.Minute:
		return 75
	case age <= 15*time.Minute:
		return 40
	default:
		return 0
	}
}

func statusScore(status string) int {
	switch domain.NodeActualState(status) {
	case domain.NodeActualStateOnline:
		return 100
	case domain.NodeActualStateDegraded:
		return 40
	default:
		return 0
	}
}
