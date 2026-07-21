package reconciler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/events"
	gpruntime "gamepanel/forge/internal/runtime"
	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/services/compose"
	"gamepanel/forge/internal/services/queue"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

const DefaultInterval = 30 * time.Second

type HealthChecker interface {
	ListUnhealthyTargets(ctx context.Context) []HealthCheckTarget
}

type HealthCheckTarget struct {
	TargetID     string
	ServerID     string
	Status       string
	FailureCount int
}

type MetricsSnapshot struct {
	ReconciliationCount       uint64 `json:"reconciliation_count"`
	ReconciliationFailures    uint64 `json:"reconciliation_failures"`
	NodeRefreshFailures       uint64 `json:"node_refresh_failures"`
	ServerSyncFailures        uint64 `json:"server_sync_failures"`
	ServerReconciliationTotal uint64 `json:"server_reconciliation_total"`
	NodeReconciliationTotal   uint64 `json:"node_reconciliation_total"`
	HealthRecoveryAttempts    uint64 `json:"health_recovery_attempts"`
	HealthRecoveryFailures    uint64 `json:"health_recovery_failures"`
	DriftsDetected            uint64 `json:"drifts_detected"`
	PlansGenerated            uint64 `json:"plans_generated"`
	AutoReconcileRuns         uint64 `json:"auto_reconcile_runs"`
}

type reconcilerStore interface {
	ListServers(ctx context.Context) ([]store.Server, error)
	ListServersForNode(ctx context.Context, nodeID string) ([]store.Server, error)
	GetServer(ctx context.Context, id string) (store.Server, error)
	GetNode(ctx context.Context, id string) (store.Node, error)
	ListNodes(ctx context.Context) ([]Node, error)
	SetNodeHeartbeatClassification(ctx context.Context, nodeID string, heartbeatState store.NodeHeartbeatState, actualState store.NodeActualState, recoveryCount int, reason string) (store.Node, store.Node, error)
	NodeCapacitySnapshot(ctx context.Context, id string) (store.NodeCapacitySnapshot, error)
	CreateReconcilePlan(ctx context.Context, plan *store.ReconcilePlanRow) error
	GetReconcilePlan(ctx context.Context, id string) (*store.ReconcilePlanRow, error)
	ListReconcilePlansByResource(ctx context.Context, resourceID, resourceKind string) ([]store.ReconcilePlanRow, error)
	ListReconcilePlans(ctx context.Context, offset, limit int) ([]store.ReconcilePlanRow, int, error)
	UpdateReconcilePlanState(ctx context.Context, id, state, execError string) error
	ConfirmReconcilePlan(ctx context.Context, id string) error
	RecordReconcileEvent(ctx context.Context, event *store.ReconcileEventRow) error
	ListReconcileEvents(ctx context.Context, resourceID string, limit int) ([]store.ReconcileEventRow, error)
	ReconcileSummary(ctx context.Context) (*store.ReconcileSummary, error)
	ListComposeStacks(ctx context.Context, userID string) ([]store.ComposeStack, error)
	ListComposeStacksForReconciliation(ctx context.Context) ([]store.ComposeStack, error)
	GetComposeStack(ctx context.Context, stackID string) (store.ComposeStack, error)
}

type reconcilerClusterManager interface {
	RefreshServerActualState(ctx context.Context, serverID string) (domain.ServerActualState, error)
	StartServer(ctx context.Context, serverID string) (gpruntime.PowerResponse, error)
	StopServer(ctx context.Context, serverID string) (gpruntime.PowerResponse, error)
	RestartServer(ctx context.Context, serverID string) (gpruntime.PowerResponse, error)
}

type GitOpsService interface {
	DetectRuntimeDrift(ctx context.Context, stackID string) (*compose.RuntimeDriftResult, error)
	PullAndRedeploy(ctx context.Context, stackID string) (*compose.GitDeployResult, error)
	RedeployFromGit(ctx context.Context, stackID string) (*compose.GitDeployResult, error)
}

type Service struct {
	store          reconcilerStore
	clusterManager reconcilerClusterManager
	publisher      events.Publisher
	interval       time.Duration
	healthChecker  HealthChecker
	mu             sync.Mutex
	metrics        MetricsSnapshot
	queueService   *queue.Service
	gitOpsService  GitOpsService
	autoReconcile  bool
	cancel         context.CancelFunc
}

func New(store *store.Store, clusterManager *clustermanager.Service, interval time.Duration, publishers ...events.Publisher) *Service {
	if interval <= 0 {
		interval = DefaultInterval
	}
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Service{store: store, clusterManager: clusterManager, publisher: publisher, interval: interval, autoReconcile: true}
}

func (s *Service) SetHealthChecker(hc HealthChecker) {
	s.healthChecker = hc
}

func (s *Service) SetQueueService(qs *queue.Service) {
	s.queueService = qs
}

func (s *Service) SetAutoReconcile(enabled bool) {
	s.autoReconcile = enabled
}

func (s *Service) SetGitOpsService(gs GitOpsService) {
	s.gitOpsService = gs
}

func (s *Service) Start(ctx context.Context) {
	if s == nil || s.store == nil || s.clusterManager == nil {
		return
	}
	ctx, s.cancel = context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.RunOnce(ctx)
			}
		}
	}()
}

func (s *Service) Stop() {
	if s != nil && s.cancel != nil {
		s.cancel()
	}
}

func (s *Service) RunOnce(ctx context.Context) error {
	if s == nil || s.store == nil || s.clusterManager == nil {
		return nil
	}
	correlationID := uuid.NewString()
	ctx = events.ContextWithCorrelationID(ctx, correlationID)
	s.increment(func(metrics *MetricsSnapshot) {
		metrics.ReconciliationCount++
	})

	slog.Debug("reconciler run started", "correlationId", correlationID)

	if err := s.refreshNodes(ctx, correlationID); err != nil {
		s.increment(func(metrics *MetricsSnapshot) {
			metrics.NodeRefreshFailures++
			metrics.ReconciliationFailures++
		})
		slog.Error("reconciler node refresh failed", "error", err, "correlationId", correlationID)
	}

	if err := s.refreshServers(ctx, correlationID); err != nil {
		s.increment(func(metrics *MetricsSnapshot) {
			metrics.ServerSyncFailures++
			metrics.ReconciliationFailures++
		})
		slog.Error("reconciler server refresh failed", "error", err, "correlationId", correlationID)
	}

	if s.autoReconcile {
		if err := s.autoReconcileDrifts(ctx, correlationID); err != nil {
			slog.Error("reconciler auto-reconcile drifts failed", "error", err, "correlationId", correlationID)
		}
		if err := s.autoReconcileComposeStacks(ctx, correlationID); err != nil {
			slog.Error("reconciler auto-reconcile compose stacks failed", "error", err, "correlationId", correlationID)
		}
	}

	s.recoverUnhealthyTargets(ctx, correlationID)

	if n, err := s.store.ListNodes(ctx); err == nil {
		for _, node := range n {
			if node.HeartbeatState == string(store.NodeHeartbeatStateReconciling) {
				if err := s.ReconcileReconnectingNode(ctx, node.ID); err != nil {
					slog.Error("reconciler: failed to reconcile reconnecting node", "nodeId", node.ID, "error", err)
				}
			}
		}
	}

	slog.Debug("reconciler run completed", "correlationId", correlationID,
		"driftsDetected", s.metrics.DriftsDetected,
		"plansGenerated", s.metrics.PlansGenerated)
	return nil
}

func (s *Service) autoReconcileDrifts(ctx context.Context, correlationID string) error {
	servers, err := s.store.ListServers(ctx)
	if err != nil {
		return err
	}

	for _, server := range servers {
		desired, observed, err := s.captureServerSnapshots(ctx, server.ID)
		if err != nil {
			slog.Warn("auto-reconcile: skip server", "serverId", server.ID, "error", err)
			continue
		}

		diffs := computeDiffs(desired, observed)

		hasActionableDiff := false
		for _, diff := range diffs {
			if diff.DiffType != DiffNoOp {
				hasActionableDiff = true
				break
			}
		}

		if !hasActionableDiff {
			continue
		}

		drifts := s.detectDrift(ctx, diffs)
		plan := generatePlan(server.ID, ResourceKindServer, diffs, drifts)
		sortDiffsByType(plan.Diffs)
		plan.ID = uuid.NewString()
		plan.CreatedAt = time.Now().UTC()

		row := s.planToRow(plan)
		if err := s.store.CreateReconcilePlan(ctx, row); err != nil {
			slog.Warn("auto-reconcile: persist plan", "serverId", server.ID, "error", err)
			continue
		}

		s.increment(func(metrics *MetricsSnapshot) {
			metrics.PlansGenerated++
			metrics.AutoReconcileRuns++
			metrics.DriftsDetected += uint64(len(drifts))
		})

		if !plan.Destructive {
			if _, err := s.ExecuteReconcilePlan(ctx, plan.ID); err != nil {
				slog.Warn("auto-reconcile: execute plan", "planId", plan.ID, "error", err)
			}
		} else {
			slog.Info("auto-reconcile: destructive plan pending confirmation",
				"planId", plan.ID, "serverId", server.ID)
		}

		s.publish(ctx, events.EventDesiredStateChanged, "server", server.ID, map[string]any{
			"reason":        "auto-reconcile",
			"diffCount":     len(diffs),
			"driftCount":    len(drifts),
			"destructive":   plan.Destructive,
			"correlationId": correlationID,
		})
	}

	return nil
}

func (s *Service) autoReconcileComposeStacks(ctx context.Context, correlationID string) error {
	if s.gitOpsService == nil {
		return nil
	}

	stacks, err := s.store.ListComposeStacksForReconciliation(ctx)
	if err != nil {
		return err
	}

	for _, stack := range stacks {
		desired, observed, snapErr := s.captureComposeSnapshots(ctx, stack.ID)
		if snapErr != nil {
			slog.Warn("auto-reconcile: skip compose stack", "stackId", stack.ID, "error", snapErr)
			continue
		}

		diffs := computeDiffs(desired, observed)

		hasActionableDiff := false
		for _, diff := range diffs {
			if diff.DiffType != DiffNoOp {
				hasActionableDiff = true
				break
			}
		}

		if !hasActionableDiff {
			continue
		}

		drifts := s.detectDrift(ctx, diffs)
		plan := generatePlan(stack.ID, ResourceKindComposeStack, diffs, drifts)
		sortDiffsByType(plan.Diffs)
		plan.ID = uuid.NewString()
		plan.CreatedAt = time.Now().UTC()

		row := s.planToRow(plan)
		if err := s.store.CreateReconcilePlan(ctx, row); err != nil {
			slog.Warn("auto-reconcile: persist compose plan", "stackId", stack.ID, "error", err)
			continue
		}

		s.increment(func(metrics *MetricsSnapshot) {
			metrics.PlansGenerated++
			metrics.AutoReconcileRuns++
			metrics.DriftsDetected += uint64(len(drifts))
		})

		if !plan.Destructive {
			if _, execErr := s.ExecuteReconcilePlan(ctx, plan.ID); execErr != nil {
				slog.Warn("auto-reconcile: execute compose plan", "planId", plan.ID, "error", execErr)
			}
		} else {
			slog.Info("auto-reconcile: destructive compose plan pending confirmation",
				"planId", plan.ID, "stackId", stack.ID)
		}

		s.publish(ctx, events.EventDesiredStateChanged, string(ResourceKindComposeStack), stack.ID, map[string]any{
			"reason":        "auto-reconcile-compose",
			"diffCount":     len(diffs),
			"driftCount":    len(drifts),
			"destructive":   plan.Destructive,
			"correlationId": correlationID,
		})
	}

	return nil
}

func (s *Service) Metrics() MetricsSnapshot {
	if s == nil {
		return MetricsSnapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metrics
}

func (s *Service) increment(update func(*MetricsSnapshot)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	update(&s.metrics)
}

func (s *Service) refreshNodes(ctx context.Context, correlationID string) error {
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if _, err := s.store.NodeCapacitySnapshot(ctx, node.ID); err != nil {
			s.increment(func(metrics *MetricsSnapshot) {
				metrics.NodeRefreshFailures++
			})
			continue
		}
		s.increment(func(metrics *MetricsSnapshot) {
			metrics.NodeReconciliationTotal++
		})
		s.reconcileNode(ctx, node, correlationID)
	}
	return nil
}

func (s *Service) refreshServers(ctx context.Context, correlationID string) error {
	servers, err := s.store.ListServers(ctx)
	if err != nil {
		return err
	}
	for _, server := range servers {
		s.increment(func(metrics *MetricsSnapshot) {
			metrics.ServerReconciliationTotal++
		})
		if err := s.reconcileServer(ctx, server, correlationID); err != nil {
			s.increment(func(metrics *MetricsSnapshot) {
				metrics.ServerSyncFailures++
				metrics.ReconciliationFailures++
			})
		}
	}
	return nil
}

func (s *Service) reconcileNode(ctx context.Context, node store.Node, correlationID string) {
	if node.DesiredState == store.NodeDesiredStateMaintenance || node.DesiredState == store.NodeDesiredStateDraining {
		s.publish(ctx, events.EventDesiredStateChanged, "node", node.ID, map[string]any{
			"desiredState":  node.DesiredState,
			"reason":        "reconciler placement guard",
			"correlationId": correlationID,
		})
		return
	}
}

func (s *Service) reconcileServer(ctx context.Context, server store.Server, correlationID string) error {
	s.publish(ctx, events.EventDesiredStateChanged, "server", server.ID, map[string]any{
		"desiredState":  server.DesiredState,
		"reason":        "reconciler comparison",
		"correlationId": correlationID,
	})
	domainState, err := s.clusterManager.RefreshServerActualState(ctx, server.ID)
	if err != nil {
		return err
	}
	storeActual := serverActualFromDomain(domainState)

	current, err := s.store.GetServer(ctx, server.ID)
	if err != nil {
		return err
	}
	s.publish(ctx, events.EventActualStateChanged, "server", server.ID, map[string]any{
		"actualState":   storeActual,
		"reason":        "reconciler comparison",
		"correlationId": correlationID,
	})

	if current.NodeID == "" {
		return nil // server not assigned to a node yet
	}
	if !s.isNodeOperable(ctx, current.NodeID) {
		return nil
	}

	switch {
	case current.DesiredState == store.ServerDesiredStateRunning && storeActual == store.ServerActualStateStopped:
		_, err := s.clusterManager.StartServer(ctx, server.ID)
		return err
	case current.DesiredState == store.ServerDesiredStateStopped && storeActual == store.ServerActualStateRunning:
		_, err := s.clusterManager.StopServer(ctx, server.ID)
		return err
	case current.DesiredState == store.ServerDesiredStateRunning && storeActual == store.ServerActualStateCrashed:
		_, err := s.clusterManager.RestartServer(ctx, server.ID)
		return err
	default:
		return nil
	}
}

func (s *Service) publish(ctx context.Context, eventType events.EventType, resourceType, resourceID string, payload map[string]any) {
	if s == nil || s.publisher == nil {
		return
	}
	_ = s.publisher.Publish(ctx, events.NewEnvelope(eventType, "reconciler", resourceType, resourceID, payload))
}

func (s *Service) ReconcileReconnectingNode(ctx context.Context, nodeID string) error {
	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		return err
	}
	if node.HeartbeatState != string(store.NodeHeartbeatStateReconciling) {
		return nil
	}

	servers, err := s.store.ListServersForNode(ctx, nodeID)
	if err != nil {
		return err
	}

	staleCount := 0
	for _, server := range servers {
		if server.WorkloadLeaseExpiry != nil && time.Now().UTC().After(*server.WorkloadLeaseExpiry) {
			staleCount++
			continue
		}
	}

	_, _, err = s.store.SetNodeHeartbeatClassification(ctx, nodeID,
		store.NodeHeartbeatStateHealthy,
		store.NodeActualStateOnline,
		0,
		fmt.Sprintf("reconciliation complete; %d stale workloads found", staleCount))

	if err != nil {
		slog.Error("reconciler: failed to transition node to healthy", "nodeId", nodeID, "error", err)
		return nil // will retry on next reconciliation cycle
	}

	return nil
}

func (s *Service) recoverUnhealthyTargets(ctx context.Context, correlationID string) {
	if s.healthChecker == nil || s.clusterManager == nil {
		return
	}
	unhealthy := s.healthChecker.ListUnhealthyTargets(ctx)
	if len(unhealthy) == 0 {
		return
	}
	seenServers := make(map[string]bool)
	for _, target := range unhealthy {
		if seenServers[target.ServerID] {
			continue
		}
		seenServers[target.ServerID] = true

		server, err := s.store.GetServer(ctx, target.ServerID)
		if err != nil {
			s.increment(func(metrics *MetricsSnapshot) {
				metrics.HealthRecoveryFailures++
			})
			continue
		}

		if server.DesiredState != store.ServerDesiredStateRunning {
			continue
		}
		if server.Suspended {
			continue
		}

		if !s.isNodeOperable(ctx, server.NodeID) {
			slog.Debug("reconciler: skipping health recovery restart, node not operable",
				"serverId", server.ID, "nodeId", server.NodeID,
				"targetId", target.TargetID, "correlationId", correlationID)
			continue
		}

		s.increment(func(metrics *MetricsSnapshot) {
			metrics.HealthRecoveryAttempts++
		})

		s.publish(ctx, events.EventTargetHealthChanged, "server", server.ID, map[string]any{
			"status":        "unhealthy",
			"reason":        "health check failure triggering recovery restart",
			"targetId":      target.TargetID,
			"failureCount":  target.FailureCount,
			"correlationId": correlationID,
		})

		if _, err := s.clusterManager.RestartServer(ctx, server.ID); err != nil {
			s.increment(func(metrics *MetricsSnapshot) {
				metrics.HealthRecoveryFailures++
			})
		}
	}
}

type Node = store.Node

func serverActualFromDomain(s domain.ServerActualState) store.ServerActualState {
	return store.ServerActualState(s)
}

func (s *Service) isNodeOperable(ctx context.Context, nodeID string) bool {
	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		return false
	}
	if node.ActualState == string(store.NodeActualStateReconciling) {
		return false
	}
	if node.ActualState != string(store.NodeActualStateOnline) &&
		node.ActualState != string(store.NodeActualStateDegraded) {
		return false
	}
	if node.HeartbeatState == string(store.NodeHeartbeatStateOffline) ||
		node.HeartbeatState == string(store.NodeHeartbeatStateUnreachable) {
		return false
	}
	return true
}

func (s *Service) ListReconcilePlans(ctx context.Context, offset, limit int) ([]store.ReconcilePlanRow, int, error) {
	return s.store.ListReconcilePlans(ctx, offset, limit)
}

func (s *Service) GetReconcilePlan(ctx context.Context, id string) (*store.ReconcilePlanRow, error) {
	return s.store.GetReconcilePlan(ctx, id)
}

func (s *Service) ReconcileSummary(ctx context.Context) (*store.ReconcileSummary, error) {
	return s.store.ReconcileSummary(ctx)
}

func (s *Service) ListReconcileEvents(ctx context.Context, resourceID string, limit int) ([]store.ReconcileEventRow, error) {
	return s.store.ListReconcileEvents(ctx, resourceID, limit)
}
