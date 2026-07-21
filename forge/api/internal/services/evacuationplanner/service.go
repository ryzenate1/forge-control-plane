package evacuationplanner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/services/reservations"
	schedulersvc "gamepanel/forge/internal/services/scheduler"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

type ReplacementPolicy string

const (
	ReplacementPolicyAutoReplace ReplacementPolicy = "auto_replace"
	ReplacementPolicyProtect     ReplacementPolicy = "protect"
)

type StorageLocality string

const (
	StorageLocalOnly  StorageLocality = "local_only"
	StorageReplicated StorageLocality = "replicated"
	StorageShared     StorageLocality = "shared"
)

type ServerMountStore interface {
	ServerMounts(ctx context.Context, serverID string) ([]store.ServerMount, error)
}

type Metrics struct {
	EvacuationPlansTotal              uint64 `json:"evacuation_plans_total"`
	EvacuationCandidatesTotal         uint64 `json:"evacuation_candidates_total"`
	EvacuationValidationFailuresTotal uint64 `json:"evacuation_validation_failures_total"`
	StorageLocalSkippedTotal          uint64 `json:"storage_local_skipped_total"`
	OrphanDetectionTotal              uint64 `json:"orphan_detection_total"`
}

type CapacityImpact struct {
	AvailableCPUAfter    int `json:"availableCpuAfter"`
	AvailableMemoryAfter int `json:"availableMemoryAfter"`
	AvailableDiskAfter   int `json:"availableDiskAfter"`
}

type PlanItem struct {
	store.EvacuationItem
	CapacityImpact *CapacityImpact `json:"capacityImpact,omitempty"`
}

type PlanResult struct {
	Plan    store.EvacuationPlan `json:"plan"`
	Items   []PlanItem           `json:"items"`
	Preview bool                 `json:"preview"`
}

// MigrationExecutor is the migration lifecycle needed by an evacuation plan.
// It deliberately exposes only lifecycle operations, so the planner cannot
// move a workload itself or mark it complete before the migration does.
type MigrationExecutor interface {
	CreateEvacuationMigration(context.Context, string, string, string) (string, error)
	PrepareEvacuationMigration(context.Context, string) error
	ExecuteEvacuationMigration(context.Context, string) error
	CancelEvacuationMigration(context.Context, string) error
	EvacuationMigrationStatus(context.Context, string) (string, error)
}

type Service struct {
	store        *store.Store
	scheduler    schedulersvc.Service
	publisher    events.Publisher
	executor     MigrationExecutor
	mountStore   ServerMountStore
	reservations *reservations.Manager
	mu           sync.Mutex
	metrics      Metrics
	observers    map[string]struct{}
	startOnce    sync.Once
	cancel       context.CancelFunc
}

func New(store *store.Store, scheduler schedulersvc.Service, publishers ...events.Publisher) *Service {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Service{store: store, scheduler: scheduler, publisher: publisher, observers: make(map[string]struct{})}
}

// Start resumes observation of persisted running plans after a process restart.
// Migration runs themselves are durable and reconciled by MigrationService; this
// loop only resumes plan-level progression and terminal-status aggregation.
func (s *Service) Start(ctx context.Context) {
	if s == nil || s.store == nil {
		return
	}
	s.startOnce.Do(func() {
		ctx, s.cancel = context.WithCancel(ctx)
		go func() {
			s.resumeRunningPlans(ctx)
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					s.resumeRunningPlans(ctx)
				}
			}
		}()
	})
}

func (s *Service) Stop() {
	if s != nil && s.cancel != nil {
		s.cancel()
	}
}

func (s *Service) resumeRunningPlans(ctx context.Context) {
	plans, err := s.store.ListEvacuationPlansByStatus(ctx, store.EvacuationPlanStatusRunning)
	if err != nil {
		return
	}
	for _, plan := range plans {
		s.startObserver(ctx, plan.ID, "")
	}
}

func (s *Service) startObserver(parentCtx context.Context, planID, correlationID string) {
	s.mu.Lock()
	if _, active := s.observers[planID]; active {
		s.mu.Unlock()
		return
	}
	s.observers[planID] = struct{}{}
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.observers, planID)
			s.mu.Unlock()
		}()
		s.observePlan(parentCtx, planID, correlationID)
	}()
}

// SetMigrationExecutor binds the planner to the service that owns actual
// transfer execution. It is safe to call during application construction.
func (s *Service) SetMigrationExecutor(executor MigrationExecutor) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executor = executor
}

func (s *Service) migrationExecutor() MigrationExecutor {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.executor
}

const maxConcurrentEvacuationMigrations = 2

// ExecutePlan atomically claims a saved plan and starts a bounded number of
// migrations. The observer persists terminal outcomes and only starts more
// items as migration slots become available.
func (s *Service) ExecutePlan(ctx context.Context, planID string) (store.EvacuationPlan, error) {
	if s == nil || s.store == nil {
		return store.EvacuationPlan{}, errors.New("store unavailable")
	}
	if s.migrationExecutor() == nil {
		return store.EvacuationPlan{}, errors.New("migration executor unavailable")
	}
	ctx = events.ContextWithCorrelationID(ctx, firstNonEmpty(events.CorrelationIDFromContext(ctx), uuid.NewString()))
	plan, started, err := s.store.StartEvacuationPlan(ctx, planID)
	if err != nil {
		return store.EvacuationPlan{}, err
	}
	if !started {
		return plan, nil
	}
	if err := s.startAvailableItems(ctx, plan); err != nil {
		return s.failPlan(ctx, planID, err)
	}
	s.startObserver(ctx, planID, events.CorrelationIDFromContext(ctx))
	return s.store.GetEvacuationPlan(ctx, planID)
}

// CancelPlan prevents further items from starting and asks the migration
// service to compensate every active item. The plan becomes terminal only
// after cancellation has been requested for all active migrations.
func (s *Service) CancelPlan(ctx context.Context, planID string) (store.EvacuationPlan, error) {
	if s == nil || s.store == nil {
		return store.EvacuationPlan{}, errors.New("store unavailable")
	}
	executor := s.migrationExecutor()
	if executor == nil {
		return store.EvacuationPlan{}, errors.New("migration executor unavailable")
	}
	plan, err := s.store.GetEvacuationPlan(ctx, planID)
	if err != nil {
		return store.EvacuationPlan{}, err
	}
	if plan.Status != store.EvacuationPlanStatusRunning {
		return store.EvacuationPlan{}, errors.New("evacuation plan is not running")
	}
	for _, item := range plan.Items {
		if (item.Status == "preparing" || item.Status == "running") && item.MigrationID != "" {
			if err := executor.CancelEvacuationMigration(ctx, item.MigrationID); err != nil {
				return store.EvacuationPlan{}, fmt.Errorf("cancel migration %s: %w", item.MigrationID, err)
			}
		}
	}
	return s.store.CancelEvacuationPlan(ctx, planID)
}

// startAvailableItems fills the bounded number of execution slots.
// setup is recorded on its item; later items are still eligible to run.
func (s *Service) startAvailableItems(ctx context.Context, plan store.EvacuationPlan) error {
	executor := s.migrationExecutor()
	if executor == nil {
		return errors.New("migration executor unavailable")
	}
	active := 0
	for _, item := range plan.Items {
		if item.Status == "preparing" || item.Status == "running" {
			active++
		}
	}
	for _, item := range plan.Items {
		if active >= maxConcurrentEvacuationMigrations {
			break
		}
		if !item.Eligible || item.Status != "pending" {
			continue
		}
		if item.TargetNodeID == "" {
			if err := s.store.UpdateEvacuationItemExecution(ctx, item.ID, "", "failed", errors.New("eligible evacuation item has no target node")); err != nil {
				return err
			}
			continue
		}
		migrationID, err := executor.CreateEvacuationMigration(ctx, item.ServerID, item.SourceNodeID, item.TargetNodeID)
		if err != nil {
			if updateErr := s.store.UpdateEvacuationItemExecution(ctx, item.ID, "", "failed", fmt.Errorf("create migration: %w", err)); updateErr != nil {
				return updateErr
			}
			continue
		}
		if err := s.store.UpdateEvacuationItemExecution(ctx, item.ID, migrationID, "preparing", nil); err != nil {
			return err
		}
		if err := executor.PrepareEvacuationMigration(ctx, migrationID); err != nil {
			if updateErr := s.store.UpdateEvacuationItemExecution(ctx, item.ID, migrationID, "failed", fmt.Errorf("prepare migration: %w", err)); updateErr != nil {
				return updateErr
			}
			continue
		}
		if err := executor.ExecuteEvacuationMigration(ctx, migrationID); err != nil {
			if updateErr := s.store.UpdateEvacuationItemExecution(ctx, item.ID, migrationID, "failed", fmt.Errorf("execute migration: %w", err)); updateErr != nil {
				return updateErr
			}
			continue
		}
		if err := s.store.UpdateEvacuationItemExecution(ctx, item.ID, migrationID, "running", nil); err != nil {
			return err
		}
		active++
	}
	return nil
}

func (s *Service) failPlan(ctx context.Context, planID string, cause error) (store.EvacuationPlan, error) {
	_, updateErr := s.store.UpdateEvacuationPlanStatus(ctx, planID, store.EvacuationPlanStatusFailed)
	s.publish(ctx, events.EventEvacuationPlanFailed, "evacuation_plan", planID, map[string]any{"error": cause.Error()})
	if updateErr != nil {
		return store.EvacuationPlan{}, updateErr
	}
	return s.store.GetEvacuationPlan(ctx, planID)
}

func (s *Service) observePlan(parentCtx context.Context, planID, correlationID string) {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()
	if correlationID != "" {
		ctx = events.ContextWithCorrelationID(ctx, correlationID)
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		plan, err := s.store.GetEvacuationPlan(ctx, planID)
		if err != nil || plan.Status != store.EvacuationPlanStatusRunning {
			return
		}
		if err := s.reconcileItems(ctx, plan); err != nil {
			_, _ = s.failPlan(ctx, planID, err)
			return
		}
		plan, err = s.store.GetEvacuationPlan(ctx, planID)
		if err != nil {
			return
		}
		if evacuationPlanFinished(plan.Items) {
			s.completePlan(ctx, plan)
			return
		}
		if err := s.startAvailableItems(ctx, plan); err != nil {
			_, _ = s.failPlan(ctx, planID, err)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) reconcileItems(ctx context.Context, plan store.EvacuationPlan) error {
	executor := s.migrationExecutor()
	if executor == nil {
		return errors.New("migration executor unavailable")
	}
	for _, item := range plan.Items {
		if (item.Status != "preparing" && item.Status != "running") || item.MigrationID == "" {
			continue
		}
		status, err := executor.EvacuationMigrationStatus(ctx, item.MigrationID)
		if err != nil {
			continue
		}
		switch evacuationMigrationOutcome(status) {
		case "completed":
			if err := s.store.UpdateEvacuationItemExecution(ctx, item.ID, item.MigrationID, "completed", nil); err != nil {
				return err
			}
		case "failed":
			if err := s.store.UpdateEvacuationItemExecution(ctx, item.ID, item.MigrationID, "failed", fmt.Errorf("migration ended with status %s", status)); err != nil {
				return err
			}
		}
	}
	return nil
}

func evacuationPlanFinished(items []store.EvacuationItem) bool {
	for _, item := range items {
		if item.Status == "pending" || item.Status == "preparing" || item.Status == "running" {
			return false
		}
	}
	return true
}

func (s *Service) completePlan(ctx context.Context, plan store.EvacuationPlan) {
	status := store.EvacuationPlanStatusCompleted
	eventType := events.EventEvacuationPlanCompleted
	for _, item := range plan.Items {
		if item.Status == "failed" {
			status = store.EvacuationPlanStatusFailed
			eventType = events.EventEvacuationPlanFailed
			break
		}
	}
	if _, err := s.store.UpdateEvacuationPlanStatus(ctx, plan.ID, status); err == nil {
		s.publish(ctx, eventType, "evacuation_plan", plan.ID, map[string]any{"status": status})
	}
}

// evacuationMigrationOutcome deliberately treats every non-terminal migration
// state as pending. In particular, a plan cannot be completed merely because
// a migration was accepted by its asynchronous executor.
func evacuationMigrationOutcome(status string) string {
	switch status {
	case string(store.MigrationStatusCompleted):
		return "completed"
	case string(store.MigrationStatusFailed), string(store.MigrationStatusCancelled):
		return "failed"
	default:
		return "pending"
	}
}

func (s *Service) Metrics() Metrics {
	if s == nil {
		return Metrics{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metrics
}

func (s *Service) PreviewPlan(ctx context.Context, nodeID string) (PlanResult, error) {
	ctx = events.ContextWithCorrelationID(ctx, firstNonEmpty(events.CorrelationIDFromContext(ctx), uuid.NewString()))
	return s.evaluateNode(ctx, nodeID, true)
}

func (s *Service) CreatePlan(ctx context.Context, nodeID string) (PlanResult, error) {
	correlationID := firstNonEmpty(events.CorrelationIDFromContext(ctx), uuid.NewString())
	ctx = events.ContextWithCorrelationID(ctx, correlationID)
	result, err := s.evaluateNode(ctx, nodeID, false)
	if err != nil {
		return PlanResult{}, err
	}
	items := make([]store.EvacuationItem, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, item.EvacuationItem)
	}
	status := evacuationPlanStatus(result.Items)
	plan, err := s.store.CreateEvacuationPlan(ctx, nodeID, status, items)
	if err != nil {
		s.increment(func(metrics *Metrics) {
			metrics.EvacuationValidationFailuresTotal++
		})
		s.publish(ctx, events.EventEvacuationPlanFailed, "node", nodeID, map[string]any{"error": err.Error()})
		return PlanResult{}, err
	}
	s.increment(func(metrics *Metrics) {
		metrics.EvacuationPlansTotal++
	})
	s.publish(ctx, events.EventEvacuationPlanCreated, "evacuation_plan", plan.ID, map[string]any{"nodeId": nodeID, "status": plan.Status})
	if status == store.EvacuationPlanStatusFailed {
		s.publish(ctx, events.EventEvacuationPlanFailed, "evacuation_plan", plan.ID, map[string]any{"nodeId": nodeID})
	}
	result.Plan = plan
	result.Preview = false
	return result, nil
}

func (s *Service) GetPlan(ctx context.Context, planID string) (store.EvacuationPlan, error) {
	return s.store.GetEvacuationPlan(ctx, planID)
}

func (s *Service) FindCandidates(ctx context.Context, server store.Server, source store.Node, candidates []store.Node) (store.Node, *CapacityImpact, string) {
	return s.findCandidates(ctx, server, source, candidates, map[string]CapacityImpact{})
}

func (s *Service) findCandidates(ctx context.Context, server store.Server, source store.Node, candidates []store.Node, reserved map[string]CapacityImpact) (store.Node, *CapacityImpact, string) {
	req := domain.PlacementRequest{
		RegionID: "",
		MemoryMB: server.MemoryMB,
		CPU:      server.CPUShares,
		DiskMB:   server.DiskMB,
	}
	if source.RegionID != nil {
		req.RegionID = *source.RegionID
	}
	filtered, err := s.scheduler.FilterNodes(ctx, req, candidates)
	if err != nil {
		return store.Node{}, nil, err.Error()
	}
	available := make([]store.Node, 0, len(filtered))
	for _, candidate := range filtered {
		if candidate.ID == source.ID {
			continue
		}
		available = append(available, candidate)
	}
	if len(available) == 0 {
		return store.Node{}, nil, "no eligible candidate nodes"
	}
	scores, err := s.scheduler.ScoreNodes(ctx, req, available)
	if err != nil || len(scores) == 0 {
		return store.Node{}, nil, "no scored candidate nodes"
	}
	var lastReason string
	for len(scores) > 0 {
		bestIndex := 0
		for i := range scores {
			if scores[i].Score > scores[bestIndex].Score {
				bestIndex = i
			}
		}
		selected := scores[bestIndex].Node
		scores = append(scores[:bestIndex], scores[bestIndex+1:]...)
		impact, err := s.ValidateCapacity(ctx, selected.ID, server)
		if err != nil {
			lastReason = err.Error()
			continue
		}
		if existing, ok := reserved[selected.ID]; ok {
			impact.AvailableCPUAfter = existing.AvailableCPUAfter - server.CPUShares
			impact.AvailableMemoryAfter = existing.AvailableMemoryAfter - server.MemoryMB
			impact.AvailableDiskAfter = existing.AvailableDiskAfter - server.DiskMB
			if impact.AvailableCPUAfter < 0 || impact.AvailableMemoryAfter < 0 || impact.AvailableDiskAfter < 0 {
				lastReason = "candidate capacity exhausted by evacuation plan"
				continue
			}
		}
		return selected, &impact, "candidate selected"
	}
	if lastReason == "" {
		lastReason = "no candidate has enough capacity"
	}
	return store.Node{}, nil, lastReason
}

func (s *Service) ValidateCapacity(ctx context.Context, nodeID string, server store.Server) (CapacityImpact, error) {
	snapshot, err := s.store.NodeCapacitySnapshot(ctx, nodeID)
	if err != nil {
		return CapacityImpact{}, err
	}
	impact := CapacityImpact{
		AvailableCPUAfter:    snapshot.AvailableCPU - server.CPUShares,
		AvailableMemoryAfter: snapshot.AvailableMemory - server.MemoryMB,
		AvailableDiskAfter:   snapshot.AvailableDisk - server.DiskMB,
	}
	if snapshot.TotalCPU > 0 && impact.AvailableCPUAfter < 0 {
		return CapacityImpact{}, errors.New("cpu capacity exceeded")
	}
	if snapshot.TotalMemory > 0 && impact.AvailableMemoryAfter < 0 {
		return CapacityImpact{}, errors.New("memory capacity exceeded")
	}
	if snapshot.TotalDisk > 0 && impact.AvailableDiskAfter < 0 {
		return CapacityImpact{}, errors.New("disk capacity exceeded")
	}
	return impact, nil
}

func (s *Service) evaluateNode(ctx context.Context, nodeID string, preview bool) (PlanResult, error) {
	source, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		return PlanResult{}, err
	}
	servers, err := s.store.ListServersForNode(ctx, nodeID)
	if err != nil {
		return PlanResult{}, err
	}
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return PlanResult{}, err
	}
	items := []PlanItem{}
	reserved := map[string]CapacityImpact{}
	for _, server := range servers {
		locality, _ := s.StorageLocality(ctx, server.ID)
		if locality == StorageLocalOnly {
			s.increment(func(metrics *Metrics) {
				metrics.StorageLocalSkippedTotal++
			})
			items = append(items, PlanItem{
				EvacuationItem: store.EvacuationItem{
					ServerID:     server.ID,
					SourceNodeID: source.ID,
					Eligible:     false,
					Reason:       "storage locality is local-only; cannot migrate",
				},
			})
			continue
		}

		// Check for backup availability for stateful workloads that require protection
		// For now, we only protect StorageLocalOnly workloads (handled above)
		// Replicated and Shared storage workloads can be migrated without backup checks
		// as they don't have the same locality constraints
		policy := s.ReplacementPolicyForServer(ctx, server, locality)
		if policy == ReplacementPolicyProtect {
			s.increment(func(metrics *Metrics) {
				metrics.EvacuationValidationFailuresTotal++
			})
			items = append(items, PlanItem{
				EvacuationItem: store.EvacuationItem{
					ServerID:     server.ID,
					SourceNodeID: source.ID,
					Eligible:     false,
					Reason:       "replacement policy is protect; manual intervention required",
				},
			})
			continue
		}
		selected, impact, reason := s.findCandidates(ctx, server, source, nodes, reserved)
		if selected.ID != "" && !preview {
			server.Generation++
			leaseExpiry := time.Now().UTC().Add(1 * time.Hour)
			server.WorkloadLeaseExpiry = &leaseExpiry
			if err := s.store.UpdateServerGeneration(ctx, server.ID, server.Generation, &leaseExpiry); err != nil {
				return PlanResult{}, err
			}
		}
		item := PlanItem{
			EvacuationItem: store.EvacuationItem{
				ServerID:     server.ID,
				SourceNodeID: source.ID,
				Eligible:     selected.ID != "",
				Reason:       reason,
			},
			CapacityImpact: impact,
		}
		if selected.ID != "" {
			item.TargetNodeID = selected.ID
			if impact != nil {
				reserved[selected.ID] = *impact
			}
			s.increment(func(metrics *Metrics) {
				metrics.EvacuationCandidatesTotal++
			})
			s.publish(ctx, events.EventEvacuationCandidateSelected, "server", server.ID, map[string]any{
				"sourceNodeId": source.ID,
				"targetNodeId": selected.ID,
			})
		} else {
			s.increment(func(metrics *Metrics) {
				metrics.EvacuationValidationFailuresTotal++
			})
		}
		items = append(items, item)
	}
	status := evacuationPlanStatus(items)
	return PlanResult{
		Plan: store.EvacuationPlan{
			NodeID: nodeID,
			Status: status,
			Items:  planStoreItems(items),
		},
		Items:   items,
		Preview: preview,
	}, nil
}

func evacuationPlanStatus(items []PlanItem) store.EvacuationPlanStatus {
	if len(items) == 0 {
		return store.EvacuationPlanStatusCompleted
	}
	for _, item := range items {
		if !item.Eligible {
			return store.EvacuationPlanStatusFailed
		}
	}
	return store.EvacuationPlanStatusPending
}

func planStoreItems(items []PlanItem) []store.EvacuationItem {
	out := make([]store.EvacuationItem, 0, len(items))
	for _, item := range items {
		out = append(out, item.EvacuationItem)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (s *Service) SetServerMountStore(store ServerMountStore) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mountStore = store
}

func (s *Service) SetReservationsManager(mgr *reservations.Manager) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reservations = mgr
}

func (s *Service) reservationsManager() *reservations.Manager {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reservations
}

func (s *Service) StorageLocality(ctx context.Context, serverID string) (StorageLocality, error) {
	if s == nil || s.mountStore == nil {
		return StorageReplicated, nil
	}
	mounts, err := s.mountStore.ServerMounts(ctx, serverID)
	if err != nil {
		return StorageReplicated, err
	}
	for _, mount := range mounts {
		if mount.Source != "" && !mount.ReadOnly {
			if isNetworkStorage(mount.Source) {
				return StorageShared, nil
			}
			return StorageLocalOnly, nil
		}
	}
	return StorageReplicated, nil
}

func isNetworkStorage(source string) bool {
	if strings.Contains(source, "://") {
		return true
	}
	if strings.Contains(source, "@") && strings.Contains(source, ":") {
		return true
	}
	return false
}

func (s *Service) ReplacementPolicyForServer(_ context.Context, server store.Server, locality StorageLocality) ReplacementPolicy {
	if locality == StorageLocalOnly {
		return ReplacementPolicyProtect
	}
	if locality == StorageShared {
		return ReplacementPolicyAutoReplace
	}
	return ReplacementPolicyAutoReplace
}

func (s *Service) DetectOrphans(ctx context.Context) ([]OrphanedWorkload, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("evacuation planner unavailable")
	}
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	var orphans []OrphanedWorkload
	for _, node := range nodes {
		if node.HeartbeatState != string(store.NodeHeartbeatStateOffline) &&
			node.HeartbeatState != string(store.NodeHeartbeatStateUnreachable) {
			continue
		}
		servers, err := s.store.ListServersForNode(ctx, node.ID)
		if err != nil {
			continue
		}
		for _, server := range servers {
			if server.Status == "deleted" || server.Suspended {
				continue
			}
			locality, _ := s.StorageLocality(ctx, server.ID)
			policy := s.ReplacementPolicyForServer(ctx, server, locality)
			orphans = append(orphans, OrphanedWorkload{
				ServerID:          server.ID,
				NodeID:            node.ID,
				Status:            server.Status,
				StorageLocality:   locality,
				ReplacementPolicy: policy,
			})
			s.increment(func(m *Metrics) { m.OrphanDetectionTotal++ })
		}
	}
	return orphans, nil
}

type OrphanedWorkload struct {
	ServerID          string            `json:"serverId"`
	NodeID            string            `json:"nodeId"`
	Status            string            `json:"status"`
	StorageLocality   StorageLocality   `json:"storageLocality"`
	ReplacementPolicy ReplacementPolicy `json:"replacementPolicy"`
}

func (s *Service) increment(update func(*Metrics)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	update(&s.metrics)
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
	_ = s.publisher.Publish(ctx, events.NewEnvelope(eventType, "evacuation-planner", resourceType, resourceID, payload))
}
