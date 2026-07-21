package recovery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

type RecoveryPlanStatusView struct {
	ID           string                   `json:"id"`
	NodeID       string                   `json:"nodeId"`
	Status       store.RecoveryPlanStatus `json:"status"`
	Reason       string                   `json:"reason"`
	ItemCount    int                      `json:"itemCount"`
	Items        []RecoveryItemStatusView `json:"items"`
	NeedsConfirm bool                     `json:"needsConfirm"`
	CreatedAt    time.Time                `json:"createdAt"`
	UpdatedAt    time.Time                `json:"updatedAt"`
}

type RecoveryItemStatusView struct {
	ID               string `json:"id"`
	ServerID         string `json:"serverId"`
	SourceNodeID     string `json:"sourceNodeId"`
	TargetNodeID     string `json:"targetNodeId,omitempty"`
	Status           string `json:"status"`
	Reason           string `json:"reason,omitempty"`
	HasBackup        bool   `json:"hasBackup"`
	BackupName       string `json:"backupName,omitempty"`
	StorageLocalOnly bool   `json:"storageLocalOnly"`
	Destructive      bool   `json:"destructive"`
}

func (c *Coordinator) PlanStatus(ctx context.Context, planID string) (RecoveryPlanStatusView, error) {
	if c == nil || c.store == nil {
		return RecoveryPlanStatusView{}, errors.New("recovery coordinator unavailable")
	}
	plan, err := c.store.GetRecoveryPlan(ctx, planID)
	if err != nil {
		return RecoveryPlanStatusView{}, err
	}
	return c.toStatusView(ctx, plan), nil
}

func (c *Coordinator) ListPlanStatuses(ctx context.Context) ([]RecoveryPlanStatusView, error) {
	if c == nil || c.store == nil {
		return nil, errors.New("recovery coordinator unavailable")
	}
	plans, err := c.store.ListRecoveryPlans(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]RecoveryPlanStatusView, 0, len(plans))
	for _, plan := range plans {
		views = append(views, c.toStatusView(ctx, plan))
	}
	return views, nil
}

func (c *Coordinator) toStatusView(ctx context.Context, plan store.RecoveryPlan) RecoveryPlanStatusView {
	view := RecoveryPlanStatusView{
		ID:        plan.ID,
		NodeID:    plan.NodeID,
		Status:    plan.Status,
		Reason:    plan.Reason,
		ItemCount: len(plan.Items),
		CreatedAt: plan.CreatedAt,
		UpdatedAt: plan.UpdatedAt,
	}
	needsConfirm := false
	for _, item := range plan.Items {
		itemView := RecoveryItemStatusView{
			ID:           item.ID,
			ServerID:     item.ServerID,
			SourceNodeID: item.SourceNodeID,
			TargetNodeID: item.TargetNodeID,
			Status:       item.Status,
			Reason:       item.Reason,
			HasBackup:    item.SourceBackupName != "",
			BackupName:   item.SourceBackupName,
		}
		if itemView.Status == string(store.RecoveryItemStatusPlanned) && item.TargetNodeID != "" {
			itemView.Destructive = true
			needsConfirm = true
		}
		view.Items = append(view.Items, itemView)
	}
	// Keep destructive impact visible after a plan leaves the planned state.
	// Lifecycle state must not make a destructive recovery look safe.
	view.NeedsConfirm = needsConfirm
	return view
}

type ConfirmRecoveryRequest struct {
	PlanID string `json:"planId"`
	Force  bool   `json:"force"`
}

func (c *Coordinator) ConfirmDestructiveRecovery(ctx context.Context, req ConfirmRecoveryRequest) (store.RecoveryPlan, error) {
	if c == nil || c.store == nil {
		return store.RecoveryPlan{}, errors.New("recovery coordinator unavailable")
	}
	plan, err := c.store.GetRecoveryPlan(ctx, req.PlanID)
	if err != nil {
		return store.RecoveryPlan{}, err
	}
	if plan.Status != store.RecoveryPlanStatusPlanned {
		return store.RecoveryPlan{}, fmt.Errorf("recovery plan %s is not in planned status", req.PlanID)
	}
	hasDestructive := false
	for _, item := range plan.Items {
		if item.Status == string(store.RecoveryItemStatusPlanned) && item.TargetNodeID != "" {
			hasDestructive = true
			break
		}
	}
	if hasDestructive && !req.Force {
		return store.RecoveryPlan{}, errors.New("destructive recovery requires operator confirmation; set force=true to proceed")
	}
	plan, err = c.store.UpdateRecoveryPlanStatus(ctx, req.PlanID, store.RecoveryPlanStatusPlanned, "operator confirmed destructive recovery")
	if err != nil {
		return store.RecoveryPlan{}, err
	}
	c.publish(ctx, events.EventRecoveryPlanPlanned, "recovery_plan", plan.ID, map[string]any{
		"nodeId":    plan.NodeID,
		"status":    plan.Status,
		"confirmed": true,
		"force":     req.Force,
	})
	return plan, nil
}

func (c *Coordinator) RecoverFromUnavailable(ctx context.Context, nodeID string) (store.RecoveryPlan, error) {
	if c == nil || c.store == nil {
		return store.RecoveryPlan{}, errors.New("recovery coordinator unavailable")
	}
	node, err := c.store.GetNode(ctx, nodeID)
	if err != nil {
		return store.RecoveryPlan{}, err
	}
	if node.HeartbeatState == string(store.NodeHeartbeatStateHealthy) {
		return store.RecoveryPlan{}, errors.New("node is healthy; no recovery needed")
	}
	backOnline := node.HeartbeatState == string(store.NodeHeartbeatStateRecovering) ||
		node.HeartbeatState == string(store.NodeHeartbeatStateHealthy)
	if !backOnline && node.HeartbeatState != string(store.NodeHeartbeatStateOffline) {
		return store.RecoveryPlan{}, fmt.Errorf("node state %s does not support recovery", node.HeartbeatState)
	}
	correlationID := events.CorrelationIDFromContext(ctx)
	if correlationID == "" {
		correlationID = uuid.NewString()
		ctx = events.ContextWithCorrelationID(ctx, correlationID)
	}
	plan, err := c.store.CreateRecoveryPlan(ctx, nodeID, "recovery from unavailable state")
	if err != nil {
		return store.RecoveryPlan{}, err
	}
	c.increment(func(m *Metrics) { m.RecoveryPlansTotal++ })
	if _, err := c.store.UpdateRecoveryPlanStatus(ctx, plan.ID, store.RecoveryPlanStatusPlanning, "evaluating workloads for recovery"); err != nil {
		return store.RecoveryPlan{}, err
	}
	servers, err := c.IdentifyAffectedServers(ctx, nodeID)
	if err != nil {
		return c.failPlan(ctx, plan.ID, "affected server lookup failed: "+err.Error())
	}
	if len(servers) == 0 {
		plan, err = c.store.UpdateRecoveryPlanStatus(ctx, plan.ID, store.RecoveryPlanStatusCompleted, "no affected workloads")
		return plan, err
	}
	for _, server := range servers {
		backup, backupErr := c.store.LatestVerifiedRecoveryBackup(ctx, server.ID)
		if backupErr != nil {
			item, _ := c.store.CreateRecoveryItem(ctx, plan.ID, store.RecoveryItem{
				ServerID: server.ID, SourceNodeID: node.ID,
				Status: string(store.RecoveryItemStatusSkipped),
				Reason: "no verified backup available for recovery",
			})
			if item.ID != "" {
				c.increment(func(m *Metrics) { m.RecoveryItemsTotal++ })
			}
			continue
		}
		decision, err := c.FindRecoveryTargets(ctx, node, server)
		if err != nil {
			item, _ := c.store.CreateRecoveryItem(ctx, plan.ID, store.RecoveryItem{
				ServerID: server.ID, SourceNodeID: node.ID,
				Status: string(store.RecoveryItemStatusFailed),
				Reason: err.Error(),
			})
			if item.ID != "" {
				c.increment(func(m *Metrics) { m.RecoveryItemsTotal++ })
			}
			continue
		}
		server.Generation++
		leaseExpiry := time.Now().UTC().Add(1 * time.Hour)
		server.WorkloadLeaseExpiry = &leaseExpiry
		fenceGeneration := server.Generation
		if err := c.store.UpdateServerGeneration(ctx, server.ID, server.Generation, &leaseExpiry); err != nil {
			item, _ := c.store.CreateRecoveryItem(ctx, plan.ID, store.RecoveryItem{
				ServerID: server.ID, SourceNodeID: node.ID,
				Status:   string(store.RecoveryItemStatusFailed),
				Reason:   "generation bump failed: " + err.Error(),
			})
			if item.ID != "" {
				c.increment(func(m *Metrics) { m.RecoveryItemsTotal++ })
			}
			continue
		}
		reservation, err := c.CreateReservations(ctx, server, decision.NodeID, "")
		if err != nil {
			continue
		}
		item, _ := c.store.CreateRecoveryItem(ctx, plan.ID, store.RecoveryItem{
			ServerID:             server.ID,
			SourceNodeID:         node.ID,
			TargetNodeID:         decision.NodeID,
			ReservationID:        reservation.ID,
			SourceBackupName:     backup.Name,
			SourceBackupChecksum: backup.Checksum,
			SourceBackupSize:     backup.Size,
			Status:               string(store.RecoveryItemStatusPlanned),
			Reason:               "planned backup restore for unavailable node recovery",
			FenceGeneration:      fenceGeneration,
		})
		if item.ID != "" {
			c.increment(func(m *Metrics) { m.RecoveryItemsTotal++ })
		}
	}
	plan, err = c.store.GetRecoveryPlan(ctx, plan.ID)
	if err != nil {
		return store.RecoveryPlan{}, err
	}
	for _, item := range plan.Items {
		if item.Status == string(store.RecoveryItemStatusFailed) {
			return c.failPlan(ctx, plan.ID, "one or more recovery items failed to plan")
		}
	}
	plan, err = c.store.UpdateRecoveryPlanStatus(ctx, plan.ID, store.RecoveryPlanStatusPlanned, "unavailable node recovery plan generated")
	if err != nil {
		return store.RecoveryPlan{}, err
	}
	c.publish(ctx, events.EventRecoveryPlanPlanned, "recovery_plan", plan.ID, map[string]any{
		"nodeId":    plan.NodeID,
		"status":    plan.Status,
		"itemCount": len(plan.Items),
	})
	return plan, nil
}

type PostRecoveryAction string

const (
	PostRecoveryActionNone           PostRecoveryAction = "none"
	PostRecoveryActionStartWorkloads PostRecoveryAction = "start_workloads"
	PostRecoveryActionMigrateBack    PostRecoveryAction = "migrate_back"
)

func (c *Coordinator) PostRecoveryAction(ctx context.Context, planID string, action PostRecoveryAction) (store.RecoveryPlan, error) {
	if c == nil || c.store == nil {
		return store.RecoveryPlan{}, errors.New("recovery coordinator unavailable")
	}
	plan, err := c.store.GetRecoveryPlan(ctx, planID)
	if err != nil {
		return store.RecoveryPlan{}, err
	}
	if plan.Status != store.RecoveryPlanStatusRestored && plan.Status != store.RecoveryPlanStatusCompleted {
		return store.RecoveryPlan{}, fmt.Errorf("recovery plan %s is not in a terminal restored state", planID)
	}
	result, err := c.store.UpdateRecoveryPlanStatus(ctx, planID, plan.Status, fmt.Sprintf("post-recovery action: %s", action))
	if err != nil {
		return store.RecoveryPlan{}, err
	}
	c.publish(ctx, events.EventRecoveryPlanCompleted, "recovery_plan", plan.ID, map[string]any{
		"postAction": string(action),
		"status":     result.Status,
	})
	return result, nil
}
