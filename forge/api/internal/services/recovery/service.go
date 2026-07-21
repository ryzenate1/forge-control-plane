package recovery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/events"
	reservationsvc "gamepanel/forge/internal/services/reservations"
	schedulersvc "gamepanel/forge/internal/services/scheduler"
	"gamepanel/forge/internal/store"
)

const DefaultRecoveryAcknowledgmentTimeout = 10 * time.Minute

type Metrics struct {
	RecoveryPlansTotal    uint64 `json:"recovery_plans_total"`
	RecoveryItemsTotal    uint64 `json:"recovery_items_total"`
	RecoveryFailuresTotal uint64 `json:"recovery_failures_total"`
}

type CreatePlanRequest struct {
	NodeID string `json:"nodeId"`
	Reason string `json:"reason"`
}

// MigrationExecutor is the migration lifecycle dependency used to run the
// migrations created by a recovery plan. *migration.Service satisfies it.
type MigrationExecutor interface {
	PrepareMigration(context.Context, string) (store.Migration, error)
	ExecuteMigration(context.Context, string) (store.Migration, error)
	GetMigration(context.Context, string) (store.Migration, error)
	CancelMigration(context.Context, string) (store.Migration, error)
	MarkFailed(context.Context, string, string) (store.Migration, error)
}

// BackupRestoreExecutor restores a previously verified backup that is directly
// accessible by the target daemon. It must not contact the offline source.
type BackupRestoreExecutor interface {
	VerifyAndRestore(context.Context, store.RecoveryItem) error
}

// coordinatorStore is the subset of *store.Store the coordinator needs,
// extracted as an interface so unit tests can substitute a mock.
type coordinatorStore interface {
	CreateRecoveryPlan(ctx context.Context, nodeID, reason string) (store.RecoveryPlan, error)
	UpdateRecoveryPlanStatus(ctx context.Context, planID string, status store.RecoveryPlanStatus, reason string) (store.RecoveryPlan, error)
	GetRecoveryPlan(ctx context.Context, planID string) (store.RecoveryPlan, error)
	ListRecoveryPlans(ctx context.Context) ([]store.RecoveryPlan, error)
	GetNode(ctx context.Context, nodeID string) (store.Node, error)
	ListServersForNode(ctx context.Context, nodeID string) ([]store.Server, error)
	CreateRecoveryItem(ctx context.Context, planID string, item store.RecoveryItem) (store.RecoveryItem, error)
	UpdateRecoveryItemStatus(ctx context.Context, itemID string, status store.RecoveryItemStatus, reason string) (store.RecoveryItem, error)
	UpdateRecoveryItemsForPlanStatus(ctx context.Context, planID string, status store.RecoveryItemStatus, reason string) error
	StartRecoveryPlan(ctx context.Context, planID string) (store.RecoveryPlan, error)
	LatestVerifiedRecoveryBackup(ctx context.Context, serverID string) (store.Backup, error)
	CreateMigration(ctx context.Context, req store.CreateMigrationRequest) (store.Migration, error)
	UpdateMigrationStatus(ctx context.Context, id string, status store.MigrationStatus, reason string) (store.Migration, error)
	UpdateServerGeneration(ctx context.Context, serverID string, generation int64, leaseExpiry *time.Time) error
	GetServer(ctx context.Context, serverID string) (store.Server, error)
	ListRecoveryItemsByStatus(ctx context.Context, statuses ...string) ([]store.RecoveryItem, error)
}

var _ coordinatorStore = (*store.Store)(nil)

type Coordinator struct {
	store           coordinatorStore
	scheduler       schedulersvc.Service
	reservations    *reservationsvc.Manager
	executor        MigrationExecutor
	restoreExecutor BackupRestoreExecutor
	publisher       events.Publisher
	mu              sync.Mutex
	metrics         Metrics
}

func New(store coordinatorStore, scheduler schedulersvc.Service, reservationManager *reservationsvc.Manager, publishers ...events.Publisher) *Coordinator {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Coordinator{store: store, scheduler: scheduler, reservations: reservationManager, publisher: publisher}
}

// NewWithMigrationExecutor creates a coordinator ready to execute recovery
// plans through the supplied migration lifecycle service.
func NewWithMigrationExecutor(store *store.Store, scheduler schedulersvc.Service, reservationManager *reservationsvc.Manager, executor MigrationExecutor, publishers ...events.Publisher) *Coordinator {
	coordinator := New(store, scheduler, reservationManager, publishers...)
	coordinator.SetMigrationExecutor(executor)
	return coordinator
}

// SetMigrationExecutor supplies the existing migration service that performs
// daemon transfer and restore work. It is intentionally injectable so recovery
// does not duplicate the migration runtime implementation.
func (c *Coordinator) SetMigrationExecutor(executor MigrationExecutor) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.executor = executor
}

func (c *Coordinator) SetBackupRestoreExecutor(executor BackupRestoreExecutor) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.restoreExecutor = executor
}

func (c *Coordinator) backupRestoreExecutor() BackupRestoreExecutor {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.restoreExecutor
}

func (c *Coordinator) migrationExecutor() MigrationExecutor {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.executor
}

func (c *Coordinator) Metrics() Metrics {
	if c == nil {
		return Metrics{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.metrics
}

func (c *Coordinator) CreatePlan(ctx context.Context, req CreatePlanRequest) (store.RecoveryPlan, error) {
	if c == nil || c.store == nil {
		return store.RecoveryPlan{}, errors.New("recovery coordinator unavailable")
	}
	correlationID := events.CorrelationIDFromContext(ctx)
	if correlationID == "" {
		correlationID = uuid.NewString()
		ctx = events.ContextWithCorrelationID(ctx, correlationID)
	}
	node, err := c.EvaluateNode(ctx, req.NodeID)
	if err != nil {
		return store.RecoveryPlan{}, err
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "node heartbeat classified offline"
	}
	plan, err := c.store.CreateRecoveryPlan(ctx, node.ID, reason)
	if err != nil {
		return store.RecoveryPlan{}, err
	}
	c.increment(func(metrics *Metrics) {
		metrics.RecoveryPlansTotal++
	})
	c.publish(ctx, events.EventRecoveryPlanCreated, "recovery_plan", plan.ID, map[string]any{
		"nodeId":        node.ID,
		"status":        plan.Status,
		"reason":        reason,
		"correlationId": correlationID,
	})
	plan, err = c.store.UpdateRecoveryPlanStatus(ctx, plan.ID, store.RecoveryPlanStatusPlanning, "planning recovery targets")
	if err != nil {
		return store.RecoveryPlan{}, err
	}
	servers, err := c.IdentifyAffectedServers(ctx, node.ID)
	if err != nil {
		return c.failPlan(ctx, plan.ID, "affected server lookup failed: "+err.Error())
	}
	if len(servers) == 0 {
		plan, err = c.store.UpdateRecoveryPlanStatus(ctx, plan.ID, store.RecoveryPlanStatusPlanned, "no affected servers")
		if err == nil {
			c.publish(ctx, events.EventRecoveryPlanPlanned, "recovery_plan", plan.ID, map[string]any{
				"nodeId":        node.ID,
				"status":        plan.Status,
				"itemCount":     0,
				"correlationId": correlationID,
			})
		}
		return plan, err
	}
	for _, server := range servers {
		item, err := c.planServer(ctx, plan.ID, node, server)
		if err != nil {
			item, _ = c.store.CreateRecoveryItem(ctx, plan.ID, store.RecoveryItem{
				ServerID:     server.ID,
				SourceNodeID: node.ID,
				Status:       string(store.RecoveryItemStatusFailed),
				Reason:       err.Error(),
			})
			if item.ID != "" {
				c.increment(func(metrics *Metrics) {
					metrics.RecoveryItemsTotal++
				})
				c.publish(ctx, events.EventRecoveryItemCreated, "recovery_item", item.ID, map[string]any{
					"planId":        plan.ID,
					"serverId":      item.ServerID,
					"sourceNodeId":  item.SourceNodeID,
					"status":        item.Status,
					"reason":        item.Reason,
					"correlationId": correlationID,
				})
			}
			continue
		}
		c.increment(func(metrics *Metrics) {
			metrics.RecoveryItemsTotal++
		})
		c.publish(ctx, events.EventRecoveryItemCreated, "recovery_item", item.ID, map[string]any{
			"planId":        plan.ID,
			"serverId":      item.ServerID,
			"sourceNodeId":  item.SourceNodeID,
			"targetNodeId":  item.TargetNodeID,
			"reservationId": item.ReservationID,
			"migrationId":   item.MigrationID,
			"status":        item.Status,
			"correlationId": correlationID,
		})
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
	plan, err = c.store.UpdateRecoveryPlanStatus(ctx, plan.ID, store.RecoveryPlanStatusPlanned, "recovery plan generated")
	if err != nil {
		return store.RecoveryPlan{}, err
	}
	c.publish(ctx, events.EventRecoveryPlanPlanned, "recovery_plan", plan.ID, map[string]any{
		"nodeId":        plan.NodeID,
		"status":        plan.Status,
		"itemCount":     len(plan.Items),
		"correlationId": correlationID,
	})
	return plan, nil
}

func (c *Coordinator) EvaluateNode(ctx context.Context, nodeID string) (store.Node, error) {
	if c == nil || c.store == nil {
		return store.Node{}, errors.New("recovery coordinator unavailable")
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return store.Node{}, errors.New("nodeId is required")
	}
	node, err := c.store.GetNode(ctx, nodeID)
	if err != nil {
		return store.Node{}, err
	}
	if node.HeartbeatState != string(store.NodeHeartbeatStateOffline) && node.ActualState != string(store.NodeActualStateOffline) {
		return store.Node{}, errors.New("node is not offline")
	}
	return node, nil
}

func (c *Coordinator) IdentifyAffectedServers(ctx context.Context, nodeID string) ([]store.Server, error) {
	if c == nil || c.store == nil {
		return nil, errors.New("recovery coordinator unavailable")
	}
	servers, err := c.store.ListServersForNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	affected := make([]store.Server, 0, len(servers))
	for _, server := range servers {
		if server.Status == "deleted" {
			continue
		}
		affected = append(affected, server)
	}
	return affected, nil
}

func (c *Coordinator) FindRecoveryTargets(ctx context.Context, source store.Node, server store.Server) (domain.PlacementDecision, error) {
	if c.scheduler == nil {
		return domain.PlacementDecision{}, errors.New("scheduler unavailable")
	}
	req := domain.PlacementRequest{
		MemoryMB:       server.MemoryMB,
		CPU:            server.CPUShares,
		DiskMB:         server.DiskMB,
		SkipReservation: true,
	}
	if source.RegionID != nil {
		req.RegionID = *source.RegionID
	}
	return c.scheduler.PlaceServer(ctx, req)
}

func (c *Coordinator) CreateReservations(ctx context.Context, server store.Server, targetNodeID, migrationID string) (store.PlacementReservation, error) {
	if c.reservations == nil {
		return store.PlacementReservation{}, errors.New("reservation manager unavailable")
	}
	return c.reservations.CreateReservation(ctx, store.CreatePlacementReservationRequest{
		NodeID:          targetNodeID,
		ServerID:        server.ID,
		MigrationID:     migrationID,
		ReservationType: store.PlacementReservationTypeRecovery,
		CPU:             server.CPUShares,
		Memory:          int64(server.MemoryMB),
		Disk:            int64(server.DiskMB),
		Status:          store.PlacementReservationStatusActive,
		ExpiresAt:       time.Now().UTC().Add(30 * time.Minute),
	})
}

func (c *Coordinator) CreateMigrationRecords(ctx context.Context, server store.Server, sourceNodeID, targetNodeID string) (store.Migration, error) {
	return c.store.CreateMigration(ctx, store.CreateMigrationRequest{
		ServerID:     server.ID,
		SourceNodeID: sourceNodeID,
		TargetNodeID: targetNodeID,
	})
}

// ExecutePlan starts every planned migration in a recovery plan. Migration
// execution itself is asynchronous; call ReconcilePlan to persist terminal
// migration outcomes after workers finish.
func (c *Coordinator) ExecutePlan(ctx context.Context, planID string) (store.RecoveryPlan, error) {
	if c == nil || c.store == nil {
		return store.RecoveryPlan{}, errors.New("recovery coordinator unavailable")
	}
	restoreExecutor := c.backupRestoreExecutor()
	if restoreExecutor == nil {
		return store.RecoveryPlan{}, errors.New("recovery backup restore executor unavailable")
	}
	plan, err := c.store.StartRecoveryPlan(ctx, planID)
	if err != nil {
		return store.RecoveryPlan{}, fmt.Errorf("start recovery plan: %w", err)
	}
	if len(plan.Items) == 0 {
		return c.store.UpdateRecoveryPlanStatus(ctx, plan.ID, store.RecoveryPlanStatusCompleted, "no recovery migrations to execute")
	}
	for _, item := range plan.Items {
		if item.Status != string(store.RecoveryItemStatusExecuting) {
			continue
		}
		if err := restoreExecutor.VerifyAndRestore(ctx, item); err != nil {
			_, _ = c.store.UpdateRecoveryItemStatus(ctx, item.ID, store.RecoveryItemStatusFailed, "backup restore failed: "+err.Error())
			continue
		}
		if _, err := c.store.UpdateRecoveryItemStatus(ctx, item.ID, store.RecoveryItemStatusAwaitingBeacon, "restore dispatched, awaiting Beacon workload acknowledgment"); err != nil {
			return store.RecoveryPlan{}, err
		}
	}
	return c.finishBackupRestorePlan(ctx, plan.ID)
}

// finishBackupRestorePlan checks every item in a backup-restore plan for a
// terminal status and transitions the plan accordingly. Items left in a
// non-terminal state (e.g. pending, planned, executing) prevent completion so
// the caller can retry. Failed items cause the plan itself to fail.
func (c *Coordinator) finishBackupRestorePlan(ctx context.Context, planID string) (store.RecoveryPlan, error) {
	plan, err := c.store.GetRecoveryPlan(ctx, planID)
	if err != nil {
		return store.RecoveryPlan{}, err
	}
	restored := false
	for _, item := range plan.Items {
		switch item.Status {
		case string(store.RecoveryItemStatusRestored), string(store.RecoveryItemStatusCompleted):
			restored = true
		case string(store.RecoveryItemStatusSkipped), string(store.RecoveryItemStatusCancelled):
		case string(store.RecoveryItemStatusFailed):
			return c.failPlan(ctx, plan.ID, "backup restore item has failed status: "+item.Reason)
		case string(store.RecoveryItemStatusAwaitingBeacon), string(store.RecoveryItemStatusHealthGating):
			return plan, nil
		default:
			return plan, nil
		}
	}
	if !restored {
		return c.store.UpdateRecoveryPlanStatus(ctx, plan.ID, store.RecoveryPlanStatusCompleted, "no verified backup restore sources were available")
	}
	return c.store.UpdateRecoveryPlanStatus(ctx, plan.ID, store.RecoveryPlanStatusRestored, "verified backup recovery restored and ownership committed")
}

func (c *Coordinator) ReconcileRecoveryAcknowledgment(ctx context.Context) error {
	if c == nil || c.store == nil {
		return errors.New("recovery coordinator unavailable")
	}
	items, err := c.store.ListRecoveryItemsByStatus(ctx,
		string(store.RecoveryItemStatusAwaitingBeacon),
		string(store.RecoveryItemStatusHealthGating),
	)
	if err != nil {
		return fmt.Errorf("list pending acknowledgment items: %w", err)
	}
	for _, item := range items {
		if time.Since(item.UpdatedAt) > DefaultRecoveryAcknowledgmentTimeout {
			if item.ReservationID != "" && c.reservations != nil {
				if _, err := c.reservations.CancelReservation(ctx, item.ReservationID); err != nil {
					slog.Error("recovery: failed to release reservation on timeout", "itemId", item.ID, "reservationId", item.ReservationID, "error", err)
				}
			}
			if _, err := c.store.UpdateRecoveryItemStatus(ctx, item.ID, store.RecoveryItemStatusFailed,
				"beacon acknowledgment timed out after "+DefaultRecoveryAcknowledgmentTimeout.String()); err != nil {
				return fmt.Errorf("timeout recovery item %s: %w", item.ID, err)
			}
			continue
		}
		server, err := c.store.GetServer(ctx, item.ServerID)
		if err != nil {
			continue
		}
		if server.ActualState != store.ServerActualStateRunning {
			continue
		}
		if _, err := c.store.UpdateRecoveryItemStatus(ctx, item.ID, store.RecoveryItemStatusRestored,
			"beacon acknowledged server running"); err != nil {
			return fmt.Errorf("acknowledge recovery item %s: %w", item.ID, err)
		}
		if item.ReservationID != "" && c.reservations != nil {
			if _, err := c.reservations.ConfirmReservation(ctx, item.ReservationID); err != nil {
				return fmt.Errorf("confirm reservation %s for item %s: %w", item.ReservationID, item.ID, err)
			}
		}
	}
	return nil
}

func (c *Coordinator) ReconcilePlan(ctx context.Context, planID string) (store.RecoveryPlan, error) {
	if c == nil || c.store == nil {
		return store.RecoveryPlan{}, errors.New("recovery coordinator unavailable")
	}
	executor := c.migrationExecutor()
	if executor == nil {
		return store.RecoveryPlan{}, errors.New("recovery migration executor unavailable")
	}
	plan, err := c.store.GetRecoveryPlan(ctx, planID)
	if err != nil {
		return store.RecoveryPlan{}, err
	}
	if plan.Status != store.RecoveryPlanStatusExecuting {
		return plan, nil
	}
	for _, item := range plan.Items {
		if item.Status != string(store.RecoveryItemStatusExecuting) {
			continue
		}
		if item.MigrationID == "" {
			return c.failRecoveryItem(ctx, executor, item, errors.New("recovery item has no migration"))
		}
		migration, err := executor.GetMigration(ctx, item.MigrationID)
		if err != nil {
			return store.RecoveryPlan{}, fmt.Errorf("get migration %s for recovery item %s: %w", item.MigrationID, item.ID, err)
		}
		switch migration.Status {
		case string(store.MigrationStatusCompleted):
			if _, err := c.store.UpdateRecoveryItemStatus(ctx, item.ID, store.RecoveryItemStatusCompleted, "migration completed"); err != nil {
				return store.RecoveryPlan{}, fmt.Errorf("mark recovery item %s completed: %w", item.ID, err)
			}
			if item.ReservationID != "" && c.reservations != nil {
				if _, err := c.reservations.ConfirmReservation(ctx, item.ReservationID); err != nil {
					return store.RecoveryPlan{}, fmt.Errorf("complete recovery reservation %s: %w", item.ReservationID, err)
				}
			}
		case string(store.MigrationStatusFailed):
			return c.failRecoveryItem(ctx, executor, item, fmt.Errorf("migration ended with status %s", migration.Status))
		case string(store.MigrationStatusCancelled):
			return c.cancelRecoveryItem(ctx, item)
		}
	}
	plan, err = c.store.GetRecoveryPlan(ctx, planID)
	if err != nil {
		return store.RecoveryPlan{}, err
	}
	allCompleted := len(plan.Items) > 0
	for _, item := range plan.Items {
		if item.Status != string(store.RecoveryItemStatusCompleted) && item.Status != string(store.RecoveryItemStatusSkipped) {
			allCompleted = false
			break
		}
	}
	if allCompleted {
		return c.store.UpdateRecoveryPlanStatus(ctx, plan.ID, store.RecoveryPlanStatusCompleted, "all recovery migrations completed")
	}
	return plan, nil
}

func (c *Coordinator) executeItem(ctx context.Context, executor MigrationExecutor, item store.RecoveryItem) error {
	if item.MigrationID == "" {
		return errors.New("recovery item has no migration")
	}
	if _, err := executor.PrepareMigration(ctx, item.MigrationID); err != nil {
		return fmt.Errorf("prepare migration %s for recovery item %s: %w", item.MigrationID, item.ID, err)
	}
	if _, err := executor.ExecuteMigration(ctx, item.MigrationID); err != nil {
		return fmt.Errorf("execute migration %s for recovery item %s: %w", item.MigrationID, item.ID, err)
	}
	return nil
}

func (c *Coordinator) failRecoveryItem(ctx context.Context, executor MigrationExecutor, item store.RecoveryItem, cause error) (store.RecoveryPlan, error) {
	reason := cause.Error()
	var cleanupErr error
	if item.MigrationID != "" {
		if _, err := executor.MarkFailed(ctx, item.MigrationID, reason); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("mark migration %s failed: %w", item.MigrationID, err))
		}
	}
	if item.ReservationID != "" && c.reservations != nil {
		if _, err := c.reservations.CancelReservation(ctx, item.ReservationID); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("cancel recovery reservation %s: %w", item.ReservationID, err))
		}
	}
	if _, err := c.store.UpdateRecoveryItemStatus(ctx, item.ID, store.RecoveryItemStatusFailed, reason); err != nil {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("mark recovery item %s failed: %w", item.ID, err))
	}
	plan, err := c.failPlan(ctx, item.PlanID, "recovery migration failed: "+reason)
	if err != nil {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("mark recovery plan failed: %w", err))
	}
	if cleanupErr != nil {
		return store.RecoveryPlan{}, errors.Join(cause, cleanupErr)
	}
	return plan, cause
}

func (c *Coordinator) cancelRecoveryItem(ctx context.Context, item store.RecoveryItem) (store.RecoveryPlan, error) {
	if item.ReservationID != "" && c.reservations != nil {
		if _, err := c.reservations.CancelReservation(ctx, item.ReservationID); err != nil {
			return store.RecoveryPlan{}, fmt.Errorf("cancel recovery reservation %s: %w", item.ReservationID, err)
		}
	}
	if _, err := c.store.UpdateRecoveryItemStatus(ctx, item.ID, store.RecoveryItemStatusCancelled, "migration cancelled"); err != nil {
		return store.RecoveryPlan{}, fmt.Errorf("mark recovery item %s cancelled: %w", item.ID, err)
	}
	return c.CancelPlan(ctx, item.PlanID)
}

func (c *Coordinator) CancelPlan(ctx context.Context, planID string) (store.RecoveryPlan, error) {
	existing, err := c.store.GetRecoveryPlan(ctx, planID)
	if err != nil {
		return store.RecoveryPlan{}, err
	}
	executor := c.migrationExecutor()
	var cleanupErr error
	for _, item := range existing.Items {
		if item.ReservationID != "" && c.reservations != nil {
			if _, err := c.reservations.CancelReservation(ctx, item.ReservationID); err != nil {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("cancel recovery reservation %s: %w", item.ReservationID, err))
			}
		}
		if item.MigrationID == "" {
			continue
		}
		if executor != nil {
			if _, err := executor.CancelMigration(ctx, item.MigrationID); err != nil {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("cancel migration %s: %w", item.MigrationID, err))
			}
			continue
		}
		if _, err := c.store.UpdateMigrationStatus(ctx, item.MigrationID, store.MigrationStatusCancelled, "recovery plan cancelled"); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("cancel migration %s: %w", item.MigrationID, err))
		}
	}
	if err := c.store.UpdateRecoveryItemsForPlanStatus(ctx, planID, store.RecoveryItemStatusCancelled, "recovery plan cancelled"); err != nil {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("mark recovery items cancelled: %w", err))
	}
	plan, err := c.store.UpdateRecoveryPlanStatus(ctx, planID, store.RecoveryPlanStatusCancelled, "recovery plan cancelled")
	if err != nil {
		return store.RecoveryPlan{}, errors.Join(cleanupErr, fmt.Errorf("mark recovery plan cancelled: %w", err))
	}
	c.publish(ctx, events.EventRecoveryPlanCancelled, "recovery_plan", plan.ID, map[string]any{
		"nodeId": plan.NodeID,
		"status": plan.Status,
	})
	return plan, cleanupErr
}

// ListPlans returns every recovery plan. As a side effect it reconciles any
// plans whose status is "executing" by reflecting terminal migration states
// onto recovery items. Callers that want to inspect a plan without mutation
// should use GetPlan instead.
func (c *Coordinator) ListPlans(ctx context.Context) ([]store.RecoveryPlan, error) {
	plans, err := c.store.ListRecoveryPlans(ctx)
	if err != nil {
		return nil, err
	}
	for i, plan := range plans {
		if plan.Status != store.RecoveryPlanStatusExecuting || c.migrationExecutor() == nil {
			continue
		}
		reconciled, err := c.ReconcilePlan(ctx, plan.ID)
		if err != nil {
			return nil, err
		}
		plans[i] = reconciled
	}
	return plans, nil
}

func (c *Coordinator) GetPlan(ctx context.Context, planID string) (store.RecoveryPlan, error) {
	if c == nil || c.store == nil {
		return store.RecoveryPlan{}, errors.New("recovery coordinator unavailable")
	}
	plan, err := c.store.GetRecoveryPlan(ctx, planID)
	if err != nil || plan.Status != store.RecoveryPlanStatusExecuting || c.migrationExecutor() == nil {
		return plan, err
	}
	return c.ReconcilePlan(ctx, planID)
}

func (c *Coordinator) planServer(ctx context.Context, planID string, source store.Node, server store.Server) (store.RecoveryItem, error) {
	backup, err := c.store.LatestVerifiedRecoveryBackup(ctx, server.ID)
	if err != nil {
		return c.store.CreateRecoveryItem(ctx, planID, store.RecoveryItem{
			ServerID: server.ID, SourceNodeID: source.ID, Status: string(store.RecoveryItemStatusSkipped),
			Reason: "no completed checksummed backup is available for offline recovery",
		})
	}
	decision, err := c.FindRecoveryTargets(ctx, source, server)
	if err != nil {
		return store.RecoveryItem{}, err
	}
	server.Generation++
	leaseExpiry := time.Now().UTC().Add(1 * time.Hour)
	server.WorkloadLeaseExpiry = &leaseExpiry
	fenceGeneration := server.Generation
	// WARNING: generation bump is not transactional with subsequent operations.
	// If CreateReservations or CreateRecoveryItem fail after this point, the
	// generation has already been bumped with no rollback. This is a known
	// limitation since the store does not support cross-table transactions.
	if err := c.store.UpdateServerGeneration(ctx, server.ID, server.Generation, &leaseExpiry); err != nil {
		return store.RecoveryItem{}, err
	}
	// Offline recovery does not create a migration record: migration execution
	// needs the source daemon and must never be attempted for this plan.
	reservation, err := c.CreateReservations(ctx, server, decision.NodeID, "")
	if err != nil {
		return store.RecoveryItem{}, err
	}
	return c.store.CreateRecoveryItem(ctx, planID, store.RecoveryItem{
		ServerID:             server.ID,
		SourceNodeID:         source.ID,
		TargetNodeID:         decision.NodeID,
		ReservationID:        reservation.ID,
		SourceBackupName:     backup.Name,
		SourceBackupChecksum: backup.Checksum,
		SourceBackupSize:     backup.Size,
		Status:               string(store.RecoveryItemStatusPlanned),
		Reason:               "planned verified backup recovery target",
		FenceGeneration:      fenceGeneration,
	})
}

func (c *Coordinator) failPlan(ctx context.Context, planID, reason string) (store.RecoveryPlan, error) {
	if existing, err := c.store.GetRecoveryPlan(ctx, planID); err == nil {
		for _, item := range existing.Items {
			if item.Status == string(store.RecoveryItemStatusFailed) {
				continue
			}
			if item.ReservationID != "" && c.reservations != nil {
				_, _ = c.reservations.CancelReservation(ctx, item.ReservationID)
			}
			if item.MigrationID != "" {
				_, _ = c.store.UpdateMigrationStatus(ctx, item.MigrationID, store.MigrationStatusFailed, reason)
			}
		}
		_ = c.store.UpdateRecoveryItemsForPlanStatus(ctx, planID, store.RecoveryItemStatusFailed, reason)
	}
	plan, err := c.store.UpdateRecoveryPlanStatus(ctx, planID, store.RecoveryPlanStatusFailed, reason)
	if err != nil {
		return store.RecoveryPlan{}, err
	}
	c.increment(func(metrics *Metrics) {
		metrics.RecoveryFailuresTotal++
	})
	c.publish(ctx, events.EventRecoveryPlanFailed, "recovery_plan", plan.ID, map[string]any{
		"nodeId": plan.NodeID,
		"status": plan.Status,
		"reason": reason,
	})
	return plan, nil
}

func (c *Coordinator) publish(ctx context.Context, eventType events.EventType, resourceType, resourceID string, payload map[string]any) {
	if c == nil || c.publisher == nil {
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
	_ = c.publisher.Publish(ctx, events.NewEnvelope(eventType, "recovery-coordinator", resourceType, resourceID, payload))
}

func (c *Coordinator) increment(update func(*Metrics)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	update(&c.metrics)
}
