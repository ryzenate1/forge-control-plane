package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/services/compose"
	"gamepanel/forge/internal/store"
	"gamepanel/forge/internal/services/queue"

	"github.com/google/uuid"
)

func (s *Service) TriggerReconcile(ctx context.Context, resourceID string, resourceKind ResourceKind) (*ReconcilePlan, error) {
	if !s.ensureNoDuplicateReconcile(ctx, resourceID, resourceKind) {
		return nil, fmt.Errorf("reconcile already in progress for %s/%s", resourceKind, resourceID)
	}

	desired, observed, err := s.captureSnapshots(ctx, resourceID, resourceKind)
	if err != nil {
		return nil, fmt.Errorf("capture snapshots: %w", err)
	}

	diffs := computeDiffs(desired, observed)
	drifts := s.detectDrift(ctx, diffs)
	plan := generatePlan(resourceID, resourceKind, diffs, drifts)
	sortDiffsByType(plan.Diffs)

	plan.ID = uuid.NewString()
	plan.CreatedAt = time.Now().UTC()

	s.publish(ctx, events.EventDesiredStateChanged, string(resourceKind), resourceID, map[string]any{
		"reason":         "reconcile triggered",
		"diffCount":      len(diffs),
		"driftCount":     len(drifts),
		"destructive":    plan.Destructive,
		"correlationId":  events.CorrelationIDFromContext(ctx),
	})

	row := s.planToRow(plan)
	if err := s.store.CreateReconcilePlan(ctx, row); err != nil {
		return nil, fmt.Errorf("persist plan: %w", err)
	}

	s.increment(func(metrics *MetricsSnapshot) {
		metrics.ReconciliationCount++
	})

	return plan, nil
}

func (s *Service) captureSnapshots(ctx context.Context, resourceID string, resourceKind ResourceKind) (DesiredStateSnapshot, ObservedStateSnapshot, error) {
	switch resourceKind {
	case ResourceKindServer:
		return s.captureServerSnapshots(ctx, resourceID)
	case ResourceKindNode:
		return s.captureNodeSnapshots(ctx, resourceID)
	case ResourceKindComposeStack:
		return s.captureComposeSnapshots(ctx, resourceID)
	default:
		return DesiredStateSnapshot{}, ObservedStateSnapshot{}, fmt.Errorf("unknown resource kind: %s", resourceKind)
	}
}

func (s *Service) captureServerSnapshots(ctx context.Context, serverID string) (DesiredStateSnapshot, ObservedStateSnapshot, error) {
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return DesiredStateSnapshot{}, ObservedStateSnapshot{}, fmt.Errorf("get server: %w", err)
	}

	desired := DesiredStateSnapshot{
		ResourceID:   server.ID,
		ResourceKind: ResourceKindServer,
		ServerState:  &server.DesiredState,
		ConfigHash:   collectServerConfigHash(server),
		TakenAt:      time.Now().UTC(),
	}

	var observed ObservedStateSnapshot
	domainState, err := s.clusterManager.RefreshServerActualState(ctx, serverID)
	if err != nil {
		observed = ObservedStateSnapshot{
			ResourceID:   serverID,
			ResourceKind: ResourceKindServer,
			TakenAt:      time.Now().UTC(),
		}
	} else {
		actual := serverActualFromDomain(domainState)
		observed = ObservedStateSnapshot{
			ResourceID:   serverID,
			ResourceKind: ResourceKindServer,
			ServerState:  &actual,
			ConfigHash:   collectServerConfigHash(server),
			TakenAt:      time.Now().UTC(),
		}
	}

	return desired, observed, nil
}

func (s *Service) captureNodeSnapshots(ctx context.Context, nodeID string) (DesiredStateSnapshot, ObservedStateSnapshot, error) {
	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		return DesiredStateSnapshot{}, ObservedStateSnapshot{}, fmt.Errorf("get node: %w", err)
	}

	desired := DesiredStateSnapshot{
		ResourceID:   node.ID,
		ResourceKind: ResourceKindNode,
		NodeState:    &node.DesiredState,
		TakenAt:      time.Now().UTC(),
	}

	actual := node.ActualState
	observed := ObservedStateSnapshot{
		ResourceID:   nodeID,
		ResourceKind: ResourceKindNode,
		NodeState:    &actual,
		TakenAt:      time.Now().UTC(),
	}

	return desired, observed, nil
}

func (s *Service) captureComposeSnapshots(ctx context.Context, stackID string) (DesiredStateSnapshot, ObservedStateSnapshot, error) {
	stack, err := s.store.GetComposeStack(ctx, stackID)
	if err != nil {
		return DesiredStateSnapshot{}, ObservedStateSnapshot{}, fmt.Errorf("get compose stack: %w", err)
	}

	composeState := &ComposeDesiredState{
		ComposeYAML: stack.ComposeYAML,
		ComposeHash: stack.ComposeHash,
		Status:      stack.Status,
	}

	configHash := collectComposeStackConfigHash(*composeState)

	desired := DesiredStateSnapshot{
		ResourceID:   stack.ID,
		ResourceKind: ResourceKindComposeStack,
		ComposeState: composeState,
		ConfigHash:   configHash,
		TakenAt:      time.Now().UTC(),
	}

	observed := ObservedStateSnapshot{
		ResourceID:   stackID,
		ResourceKind: ResourceKindComposeStack,
		ComposeState: &ComposeObservedState{
			Status: stack.Status,
		},
		ConfigHash: configHash,
		TakenAt:    time.Now().UTC(),
	}

	if s.gitOpsService != nil {
		drift, drErr := s.gitOpsService.DetectRuntimeDrift(ctx, stackID)
		if drErr != nil {
			slog.Warn("failed to detect compose runtime drift", "stackID", stackID, "error", drErr)
		} else if drift != nil && drift.HasDrift {
			observed.ConfigHash = stack.ComposeHash + ":drift"
			if observed.ComposeState != nil {
				observed.ComposeState.Status = "drift_detected"
			}
		}
	}

	return desired, observed, nil
}

func (s *Service) ExecuteReconcilePlan(ctx context.Context, planID string) (*ReconcileResult, error) {
	row, err := s.store.GetReconcilePlan(ctx, planID)
	if err != nil || row == nil {
		return nil, fmt.Errorf("plan not found: %w", err)
	}

	if row.State != string(PlanConfirmed) && row.State != string(PlanPending) {
		return nil, fmt.Errorf("plan %s is in state %s, cannot execute", planID, row.State)
	}

	if err := s.store.UpdateReconcilePlanState(ctx, planID, string(PlanExecuting), ""); err != nil {
		return nil, fmt.Errorf("update state: %w", err)
	}

	result := &ReconcileResult{
		PlanID:     planID,
		ResourceID: row.ResourceID,
		State:      PlanExecuting,
		StartedAt:  time.Now().UTC(),
	}

	var diffs []ReconcileDiff
	if len(row.DiffData) > 0 {
		_ = json.Unmarshal(row.DiffData, &diffs)
	}

	operationsCreated := 0
	var execErr error

	for _, diff := range diffs {
		if diff.DiffType == DiffNoOp {
			continue
		}

		if diff.DiffType == DiffDelete && !row.Confirmed {
			execErr = fmt.Errorf("destructive plan %s not confirmed", planID)
			break
		}

		if err := s.executeDiff(ctx, diff); err != nil {
			execErr = fmt.Errorf("execute diff %s/%s: %w", diff.ResourceKind, diff.ResourceID, err)
			s.store.RecordReconcileEvent(ctx, &store.ReconcileEventRow{
				PlanID:       planID,
				ResourceID:   row.ResourceID,
				ResourceKind: row.ResourceKind,
				EventType:    "execution_failed",
				Summary:      execErr.Error(),
			})
			break
		}
		operationsCreated++
	}

	completedAt := time.Now().UTC()
	if execErr != nil {
		_ = s.store.UpdateReconcilePlanState(ctx, planID, string(PlanFailed), execErr.Error())
		result.State = PlanFailed
		result.Error = execErr.Error()
	} else {
		_ = s.store.UpdateReconcilePlanState(ctx, planID, string(PlanSucceeded), "")
		result.State = PlanSucceeded
		result.CompletedAt = &completedAt
	}
	result.OperationCount = operationsCreated
	result.CompletedAt = &completedAt

	s.store.RecordReconcileEvent(ctx, &store.ReconcileEventRow{
		PlanID:       planID,
		ResourceID:   row.ResourceID,
		ResourceKind: row.ResourceKind,
		EventType:    string(result.State),
		Summary:      fmt.Sprintf("reconciliation %s: %d operations", result.State, operationsCreated),
	})

	s.increment(func(metrics *MetricsSnapshot) {
		if execErr != nil {
			metrics.ReconciliationFailures++
		}
	})

	return result, nil
}

func (s *Service) executeDiff(ctx context.Context, diff ReconcileDiff) error {
	switch diff.ResourceKind {
	case ResourceKindServer:
		return s.executeServerDiff(ctx, diff)
	case ResourceKindNode:
		return s.executeNodeDiff(ctx, diff)
	case ResourceKindComposeStack:
		return s.executeComposeDiff(ctx, diff)
	default:
		return fmt.Errorf("unsupported resource kind: %s", diff.ResourceKind)
	}
}

func (s *Service) executeServerDiff(ctx context.Context, diff ReconcileDiff) error {
	switch diff.DiffType {
	case DiffCreate:
		return fmt.Errorf("server creation not supported via reconcile")
	case DiffUpdate:
		if diff.Details != nil {
			desiredState, _ := diff.Details["desiredState"].(string)
			switch store.ServerDesiredState(desiredState) {
			case store.ServerDesiredStateRunning:
				if _, err := s.clusterManager.StartServer(ctx, diff.ResourceID); err != nil {
					if s.queueService != nil {
						s.queueService.Dispatch(ctx, queue.JobServerStart, diff.ResourceID, "", nil, 0)
					}
				}
			case store.ServerDesiredStateStopped:
				if _, err := s.clusterManager.StopServer(ctx, diff.ResourceID); err != nil {
					if s.queueService != nil {
						s.queueService.Dispatch(ctx, queue.JobServerStop, diff.ResourceID, "", nil, 0)
					}
				}
			}
		}
	case DiffDelete:
		return fmt.Errorf("server deletion not supported via reconcile")
	}
	return nil
}

func (s *Service) executeNodeDiff(ctx context.Context, diff ReconcileDiff) error {
	switch diff.DiffType {
	case DiffUpdate:
		slog.Warn("node update requested via reconcile, no automatic action", "nodeId", diff.ResourceID)
		return nil
	case DiffDelete:
		return fmt.Errorf("node deletion not supported via reconcile")
	default:
		return nil
	}
}

func (s *Service) executeComposeDiff(ctx context.Context, diff ReconcileDiff) error {
	if s.gitOpsService == nil {
		return fmt.Errorf("gitops service not available for compose reconciliation")
	}
	switch diff.DiffType {
	case DiffUpdate:
		if _, err := s.gitOpsService.PullAndRedeploy(ctx, diff.ResourceID); err != nil {
			if errors.Is(err, compose.ErrNoGitUpdateAvailable) {
				if _, redeployErr := s.gitOpsService.RedeployFromGit(ctx, diff.ResourceID); redeployErr != nil {
					slog.Warn("compose stack redeploy fallback failed", "stackId", diff.ResourceID, "error", redeployErr)
					return redeployErr
				}
				return nil
			}
			slog.Warn("compose stack redeploy failed", "stackId", diff.ResourceID, "error", err)
			return err
		}
		return nil
	case DiffDelete:
		return fmt.Errorf("compose stack deletion not supported via reconcile")
	case DiffCreate:
		return fmt.Errorf("compose stack creation not supported via reconcile")
	default:
		return nil
	}
}

func (s *Service) planToRow(plan *ReconcilePlan) *store.ReconcilePlanRow {
	diffJSON, _ := json.Marshal(plan.Diffs)
	driftJSON, _ := json.Marshal(plan.Drifts)

	state := string(PlanPending)
	if plan.Destructive {
		state = string(PlanPending)
	}

	return &store.ReconcilePlanRow{
		ID:           plan.ID,
		ResourceID:   plan.ResourceID,
		ResourceKind: string(plan.ResourceKind),
		State:        state,
		Destructive:  plan.Destructive,
		Confirmed:    plan.Confirmed,
		DiffCount:    len(plan.Diffs),
		DriftCount:   len(plan.Drifts),
		DiffData:     diffJSON,
		DriftData:    driftJSON,
		CreatedAt:    plan.CreatedAt,
	}
}

func (s *Service) ReconcileResource(ctx context.Context, resourceID string, resourceKind ResourceKind) (*ReconcileResult, error) {
	plan, err := s.TriggerReconcile(ctx, resourceID, resourceKind)
	if err != nil {
		return nil, err
	}

	if !plan.Destructive {
		return s.ExecuteReconcilePlan(ctx, plan.ID)
	}

	result := &ReconcileResult{
		PlanID:     plan.ID,
		ResourceID: resourceID,
		State:      PlanPending,
		StartedAt:  time.Now().UTC(),
	}
	return result, nil
}

func (s *Service) ConfirmAndExecute(ctx context.Context, planID string) (*ReconcileResult, error) {
	if err := s.store.ConfirmReconcilePlan(ctx, planID); err != nil {
		return nil, fmt.Errorf("confirm plan: %w", err)
	}
	return s.ExecuteReconcilePlan(ctx, planID)
}

func (s *Service) ReconcileAllResources(ctx context.Context) ([]*ReconcileResult, error) {
	var results []*ReconcileResult

	servers, err := s.store.ListServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}

	serverSet := make(map[string]bool)
	for _, server := range servers {
		if serverSet[server.ID] {
			continue
		}
		serverSet[server.ID] = true
		result, err := s.ReconcileResource(ctx, server.ID, ResourceKindServer)
		if err != nil {
			slog.Error("reconcile server", "serverId", server.ID, "error", err)
			continue
		}
		results = append(results, result)
	}

	return results, nil
}

func (s *Service) ReconcileAllComposeStacks(ctx context.Context) ([]*ReconcileResult, error) {
	var results []*ReconcileResult

	stacks, err := s.store.ListComposeStacksForReconciliation(ctx)
	if err != nil {
		return nil, fmt.Errorf("list compose stacks: %w", err)
	}

	for _, stack := range stacks {
		result, err := s.ReconcileResource(ctx, stack.ID, ResourceKindComposeStack)
		if err != nil {
			slog.Error("reconcile compose stack", "stackId", stack.ID, "error", err)
			continue
		}
		results = append(results, result)
	}

	return results, nil
}

func (s *Service) ReconcileNodes(ctx context.Context) ([]*ReconcileResult, error) {
	var results []*ReconcileResult

	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	for _, node := range nodes {
		result, err := s.ReconcileResource(ctx, node.ID, ResourceKindNode)
		if err != nil {
			slog.Error("reconcile node", "nodeId", node.ID, "error", err)
			continue
		}
		results = append(results, result)
	}

	return results, nil
}
