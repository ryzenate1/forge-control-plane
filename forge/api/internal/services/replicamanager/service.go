package replicamanager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/placement"
	"gamepanel/forge/internal/services/reservations"
	scheduler2 "gamepanel/forge/internal/services/scheduler"
	"gamepanel/forge/internal/store"
)

// BeaconClient abstracts sending commands to a Beacon node for provisioning.
type BeaconClient interface {
	// DispatchCommand sends a provisioning command to a Beacon node.
	// The commandID is an idempotent identifier so the node can deduplicate.
	DispatchCommand(ctx context.Context, nodeID string, commandID string, commandType InstanceCommandType, payload map[string]any) error
}

type Metrics struct {
	CreateAppTotal      uint64 `json:"create_app_total"`
	DeleteAppTotal      uint64 `json:"delete_app_total"`
	ScaleUpTotal        uint64 `json:"scale_up_total"`
	ScaleDownTotal      uint64 `json:"scale_down_total"`
	ReplacementTotal    uint64 `json:"replacement_total"`
	ReservationErrors   uint64 `json:"reservation_errors"`
	DispatchErrors      uint64 `json:"dispatch_errors"`
	ReconcileTotal      uint64 `json:"reconcile_total"`
	NoDoubleReservation uint64 `json:"no_double_reservation"`
	NoDuplicateAlloc    uint64 `json:"no_duplicate_alloc"`
}

type Manager struct {
	store        *store.Store
	engine       *placement.Engine
	scheduler    *scheduler2.Scheduler
	reservations *reservations.Manager
	dispatcher   InstanceCommandDispatcher
	beaconClient BeaconClient
	publisher    events.Publisher
	logger       *slog.Logger
	mu           sync.Mutex
	metrics      Metrics
	stopCh       chan struct{}
	started      atomic.Bool
}

type CreateAppRequest struct {
	Name            string
	Replicas        int
	CPU             int
	MemoryMB        int
	DiskMB          int
	RuntimeProvider string
	Image           string
}

func New(store *store.Store, engine *placement.Engine, scheduler *scheduler2.Scheduler, reservationMgr *reservations.Manager, dispatcher InstanceCommandDispatcher, beaconClient BeaconClient, logger *slog.Logger, publishers ...events.Publisher) *Manager {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	if dispatcher == nil {
		dispatcher = NoopCommandDispatcher{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		store:        store,
		engine:       engine,
		scheduler:    scheduler,
		reservations: reservationMgr,
		dispatcher:   dispatcher,
		beaconClient: beaconClient,
		publisher:    publisher,
		logger:       logger,
	}
}

func (m *Manager) Metrics() Metrics {
	if m == nil {
		return Metrics{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.metrics
}

func (m *Manager) CreateApp(ctx context.Context, req CreateAppRequest) (store.ReplicaApplication, error) {
	if req.Replicas < 1 {
		req.Replicas = 1
	}
	if req.Replicas > 100 {
		return store.ReplicaApplication{}, errors.New("replicas must not exceed 100")
	}
	if req.CPU <= 0 {
		req.CPU = 1024
	}
	if req.MemoryMB <= 0 {
		req.MemoryMB = 2048
	}
	if req.DiskMB <= 0 {
		req.DiskMB = 10240
	}
	if req.RuntimeProvider == "" {
		req.RuntimeProvider = "docker"
	}

	app, err := m.store.CreateReplicaApp(ctx, store.CreateReplicaAppRequest{
		Name:            req.Name,
		Replicas:        req.Replicas,
		CPU:             req.CPU,
		MemoryMB:        req.MemoryMB,
		DiskMB:          req.DiskMB,
		RuntimeProvider: req.RuntimeProvider,
		Image:           req.Image,
	})
	if err != nil {
		return store.ReplicaApplication{}, err
	}

	m.mu.Lock()
	m.metrics.CreateAppTotal++
	m.mu.Unlock()
	m.publish(ctx, events.EventAppCreated, app.ID, map[string]any{
		"name":     app.Name,
		"replicas": app.Replicas,
	})
	return app, nil
}

func (m *Manager) DeployApp(ctx context.Context, appID string) error {
	app, err := m.store.GetReplicaApp(ctx, appID)
	if err != nil {
		return err
	}

	_, _ = m.store.UpdateReplicaAppStatus(ctx, appID, string(AppDeploymentStatusDeploying))

	gen, err := m.store.IncrementReplicaAppGeneration(ctx, appID)
	if err != nil {
		return fmt.Errorf("increment generation: %w", err)
	}

	reasons, err := m.scheduler.PlaceReplicas(ctx, domain.PlaceReplicasRequest{
		AppID:        app.ID,
		ReplicaCount: app.Replicas,
		CPU:          app.CPU,
		MemoryMB:     app.MemoryMB,
		DiskMB:       app.DiskMB,
	})
	if err != nil {
		_, _ = m.store.UpdateReplicaAppStatus(ctx, appID, string(AppDeploymentStatusFailed))
		m.publish(ctx, events.EventAppFailed, appID, map[string]any{
			"appId": appID,
			"error": err.Error(),
		})
		return err
	}

	operationID := newOperationID()
	deployed, failed := m.deployReplicas(ctx, app, reasons, gen, operationID)

	m.updateAppStatus(ctx, appID, deployed, failed, app.Replicas)

	if failed > 0 && deployed == 0 {
		m.publish(ctx, events.EventAppFailed, appID, map[string]any{
			"appId":    appID,
			"replicas": app.Replicas,
		})
		return fmt.Errorf("all %d replicas failed to deploy", failed)
	}
	m.publish(ctx, events.EventAppDeploying, appID, map[string]any{
		"appId":    appID,
		"replicas": app.Replicas,
	})
	return nil
}

func (m *Manager) deployReplicas(ctx context.Context, app store.ReplicaApplication, reasons []domain.PlacementReason, generation int, operationID string) (deployed, failed int) {
	for _, reason := range reasons {
		if !reason.Accepted {
			failed++
			m.logger.WarnContext(ctx, "placement rejected",
				"appId", app.ID,
				"reasons", reason.Reasons,
			)
			continue
		}
		nodeID := reason.NodeID
		if nodeID == "" {
			failed++
			continue
		}

		switch app.Status {
		case string(AppDeploymentStatusRunning):
			m.mu.Lock()
			m.metrics.NoDoubleReservation++
			m.mu.Unlock()
			return deployed, failed
		}

		reservation, err := m.reservations.CreateReservation(ctx, store.CreatePlacementReservationRequest{
			NodeID:          nodeID,
			ServerID:        app.ID,
			ReservationType: store.PlacementReservationTypePlacement,
			CPU:             app.CPU,
			Memory:          int64(app.MemoryMB),
			Disk:            int64(app.DiskMB),
		})
		if err != nil {
			m.mu.Lock()
			m.metrics.ReservationErrors++
			m.mu.Unlock()
			m.logger.ErrorContext(ctx, "reservation failed",
				"appId", app.ID,
				"nodeId", nodeID,
				"error", err,
			)
			failed++
			continue
		}

		tx, err := m.store.DB().Begin(ctx)
		if err != nil {
			_, _ = m.reservations.CancelReservation(ctx, reservation.ID)
			failed++
			continue
		}

		inst, err := m.store.CreateInstance(ctx, tx, app.ID, nodeID, reason.Index, app.CPU, app.MemoryMB, app.DiskMB, app.RuntimeProvider)
		if err != nil {
			tx.Rollback(ctx)
			_, _ = m.reservations.CancelReservation(ctx, reservation.ID)
			m.logger.ErrorContext(ctx, "create instance failed",
				"appId", app.ID,
				"nodeId", nodeID,
				"error", err,
			)
			failed++
			continue
		}

		if err := tx.Commit(ctx); err != nil {
			_ = tx.Rollback(ctx)
			_, _ = m.reservations.CancelReservation(ctx, reservation.ID)
			m.logger.ErrorContext(ctx, "commit instance failed",
				"appId", app.ID,
				"instanceId", inst.ID,
				"error", err,
			)
			failed++
			continue
		}

		if _, err := m.store.AssignInstanceReservation(ctx, inst.ID, reservation.ID); err != nil {
			m.logger.ErrorContext(ctx, "assign reservation failed",
				"instanceId", inst.ID,
				"reservationId", reservation.ID,
				"error", err,
			)
		}

		commandID := instanceCommandID(inst.ID, StartInstanceCommand, generation)

		// Enqueue a versioned Beacon command for this instance.
		// This logs to store.BeaconCommandLog and dispatches via BeaconClient.
		payload := map[string]any{
			"appId":           app.ID,
			"instanceId":      inst.ID,
			"nodeId":          nodeID,
			"index":           reason.Index,
			"cpu":             app.CPU,
			"memoryMb":        app.MemoryMB,
			"diskMb":          app.DiskMB,
			"runtimeProvider": app.RuntimeProvider,
			"generation":      generation,
			"image":           app.Image,
		}
		cmdID, err := m.dispatchBeaconCommand(ctx, commandID, operationID, nodeID, inst.ID, StartInstanceCommand, payload)
		if err != nil {
			m.mu.Lock()
			m.metrics.DispatchErrors++
			m.mu.Unlock()
			_, _ = m.reservations.CancelReservation(ctx, reservation.ID)
			_, _ = m.store.UpdateInstanceStatus(ctx, inst.ID, "failed")
			m.logger.ErrorContext(ctx, "beacon command dispatch failed",
				"instanceId", inst.ID,
				"nodeId", nodeID,
				"error", err,
			)
			failed++
			continue
		}

		_, err = m.dispatcher.StartInstance(ctx, StartInstanceRequest{
			AppID:           app.ID,
			InstanceID:      inst.ID,
			NodeID:          nodeID,
			Index:           reason.Index,
			CPU:             app.CPU,
			MemoryMB:        app.MemoryMB,
			DiskMB:          app.DiskMB,
			RuntimeProvider: app.RuntimeProvider,
			CommandID:       cmdID,
			OperationID:     operationID,
			Generation:      generation,
			Image:           app.Image,
		})
		if err != nil {
			m.mu.Lock()
			m.metrics.DispatchErrors++
			m.mu.Unlock()
			_, _ = m.reservations.CancelReservation(ctx, reservation.ID)
			_, _ = m.store.UpdateInstanceStatus(ctx, inst.ID, "failed")
			m.logger.ErrorContext(ctx, "dispatch failed",
				"instanceId", inst.ID,
				"nodeId", nodeID,
				"error", err,
			)
			failed++
			continue
		}

		_, _ = m.store.UpdateInstanceStatus(ctx, inst.ID, "provisioning")
		_, _ = m.store.CreatePlacementDecision(ctx, inst.ID, nodeID, app.ID, reason.Index, reason.Score, true, reason.Reasons, app.RuntimeProvider)

		if _, err := m.reservations.ConfirmReservation(ctx, reservation.ID); err != nil {
			m.logger.ErrorContext(ctx, "confirm reservation failed",
				"reservationId", reservation.ID,
				"instanceId", inst.ID,
				"error", err,
			)
		}

		deployed++
	}
	return deployed, failed
}

func (m *Manager) ScaleApp(ctx context.Context, appID string, targetReplicas int) error {
	app, err := m.store.GetReplicaApp(ctx, appID)
	if err != nil {
		return err
	}
	current, err := m.store.ListInstancesByApp(ctx, appID)
	if err != nil {
		return err
	}

	activeCount := 0
	for _, inst := range current {
		if inst.Status != "removing" && inst.Status != "failed" {
			activeCount++
		}
	}

	if targetReplicas < 0 {
		return errors.New("target replicas must be non-negative")
	}

	if targetReplicas == activeCount {
		return nil
	}

	reasons, err := m.scheduler.ScaleReplicas(ctx, domain.ScaleRequest{
		AppID:        appID,
		ReplicaCount: targetReplicas,
	})
	if err != nil {
		return err
	}

	if targetReplicas > activeCount {
		m.mu.Lock()
		m.metrics.ScaleUpTotal++
		m.mu.Unlock()

		gen, err := m.store.IncrementReplicaAppGeneration(ctx, appID)
		if err != nil {
			return fmt.Errorf("increment generation: %w", err)
		}
		operationID := newOperationID()
		deployed, failed := m.deployReplicas(ctx, app, reasons, gen, operationID)
		m.updateAppStatus(ctx, appID, deployed, failed, targetReplicas)
		if failed > 0 && deployed == 0 {
			return fmt.Errorf("all %d scale-up replicas failed", failed)
		}
		m.publish(ctx, events.EventAppScaledUp, appID, map[string]any{
			"appId":  appID,
			"from":   activeCount,
			"to":     targetReplicas,
		})
		return nil
	}

	m.mu.Lock()
	m.metrics.ScaleDownTotal++
	m.mu.Unlock()

	operationID := newOperationID()
	stopped, stopFailed := m.safeStopReplicas(ctx, app, reasons, operationID)

	currentAfter, _ := m.store.ListInstancesByApp(ctx, appID)
	runningCount := 0
	for _, inst := range currentAfter {
		if inst.Status == "running" || inst.Status == "provisioning" {
			runningCount++
		}
	}
	status := m.computeAppStatus(ctx, appID, runningCount, targetReplicas)
	_, _ = m.store.UpdateReplicaAppStatus(ctx, appID, status)

	m.publish(ctx, events.EventAppScaledDown, appID, map[string]any{
		"appId":      appID,
		"from":       activeCount,
		"to":         targetReplicas,
		"stopped":    stopped,
		"stopFailed": stopFailed,
	})
	return nil
}

func (m *Manager) safeStopReplicas(ctx context.Context, app store.ReplicaApplication, reasons []domain.PlacementReason, operationID string) (stopped, failed int) {
	currentApp, err := m.store.GetReplicaApp(ctx, app.ID)
	if err != nil {
		currentApp = app
	}

	for _, reason := range reasons {
		if !reason.Accepted || reason.InstanceID == "" {
			continue
		}

		inst, err := m.store.GetInstance(ctx, reason.InstanceID)
		if err != nil {
			failed++
			continue
		}

		_, _ = m.store.UpdateInstanceStatus(ctx, inst.ID, "stopping")

		commandID := instanceCommandID(inst.ID, StopInstanceCommand, currentApp.Generation)
		m.logBeaconCommand(ctx, commandID, operationID, inst.NodeID, inst.ID, string(StopInstanceCommand), "dispatched", map[string]any{
			"instanceId": inst.ID,
			"nodeId":     inst.NodeID,
		})

		_, err = m.dispatcher.StopInstance(ctx, StopInstanceRequest{
			InstanceID:  inst.ID,
			NodeID:      inst.NodeID,
			CommandID:   commandID,
			OperationID: operationID,
		})
		if err != nil {
			m.mu.Lock()
			m.metrics.DispatchErrors++
			m.mu.Unlock()
			m.logger.ErrorContext(ctx, "stop dispatch failed",
				"instanceId", inst.ID,
				"nodeId", inst.NodeID,
				"error", err,
			)
			_, _ = m.store.UpdateInstanceStatus(ctx, inst.ID, "failed")
			failed++
			continue
		}

		if inst.ReservationID != nil {
			_, _ = m.reservations.CancelReservation(ctx, *inst.ReservationID)
		}

		_ = m.store.DeleteInstance(ctx, inst.ID)
		stopped++
	}
	return stopped, failed
}

func (m *Manager) DeleteApp(ctx context.Context, appID string) error {
	instances, err := m.store.ListInstancesByApp(ctx, appID)
	if err != nil {
		return err
	}

	app, err := m.store.GetReplicaApp(ctx, appID)
	if err != nil {
		return err
	}

	operationID := newOperationID()
	for _, inst := range instances {
		if inst.Status != "removing" && inst.Status != "failed" {
			_, _ = m.store.UpdateInstanceStatus(ctx, inst.ID, "stopping")

			commandID := instanceCommandID(inst.ID, StopInstanceCommand, app.Generation)
			m.logBeaconCommand(ctx, commandID, operationID, inst.NodeID, inst.ID, string(StopInstanceCommand), "dispatched", map[string]any{
				"instanceId": inst.ID,
				"nodeId":     inst.NodeID,
			})

			_, _ = m.dispatcher.StopInstance(ctx, StopInstanceRequest{
				InstanceID:  inst.ID,
				NodeID:      inst.NodeID,
				CommandID:   commandID,
				OperationID: operationID,
			})
		}

		if inst.ReservationID != nil {
			_, _ = m.reservations.CancelReservation(ctx, *inst.ReservationID)
		}

		_ = m.store.DeleteInstance(ctx, inst.ID)
	}

	if err := m.store.DeleteReplicaApp(ctx, appID); err != nil {
		return err
	}

	m.mu.Lock()
	m.metrics.DeleteAppTotal++
	m.mu.Unlock()
	m.publish(ctx, events.EventAppDeleted, appID, map[string]any{
		"appId": appID,
	})
	return nil
}

func (m *Manager) ReplaceInstance(ctx context.Context, instanceID string) (*domain.PlacementReason, error) {
	reason, err := m.scheduler.ReplaceFailedInstance(ctx, domain.ReplaceFailedInstanceRequest{
		InstanceID: instanceID,
	})
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.metrics.ReplacementTotal++
	m.mu.Unlock()

	if reason.Accepted {
		inst, err := m.store.GetInstance(ctx, instanceID)
		if err == nil {
			app, err := m.store.GetReplicaApp(ctx, inst.AppID)
			if err == nil {
				gen, err := m.store.IncrementReplicaAppGeneration(ctx, app.ID)
				if err == nil {
					operationID := newOperationID()
					commandID := instanceCommandID(inst.ID, StartInstanceCommand, gen)

					payload := map[string]any{
						"appId":           app.ID,
						"instanceId":      inst.ID,
						"nodeId":          reason.NodeID,
						"index":           inst.Idx,
						"cpu":             app.CPU,
						"memoryMb":        app.MemoryMB,
						"diskMb":          app.DiskMB,
						"runtimeProvider": app.RuntimeProvider,
						"generation":      gen,
						"image":           app.Image,
					}

					cmdID, err := m.dispatchBeaconCommand(ctx, commandID, operationID, reason.NodeID, inst.ID, StartInstanceCommand, payload)
					if err != nil {
						m.mu.Lock()
						m.metrics.DispatchErrors++
						m.mu.Unlock()
						_, _ = m.store.UpdateInstanceStatus(ctx, inst.ID, "failed")
						m.logger.ErrorContext(ctx, "beacon command dispatch failed during replacement",
							"instanceId", inst.ID,
							"nodeId", reason.NodeID,
							"error", err,
						)
					} else {
						_, err = m.dispatcher.StartInstance(ctx, StartInstanceRequest{
							AppID:           app.ID,
							InstanceID:      inst.ID,
							NodeID:          reason.NodeID,
							Index:           inst.Idx,
							CPU:             app.CPU,
							MemoryMB:        app.MemoryMB,
							DiskMB:          app.DiskMB,
							RuntimeProvider: app.RuntimeProvider,
							CommandID:       cmdID,
							OperationID:     operationID,
							Generation:      gen,
							Image:           app.Image,
						})
						if err != nil {
							m.mu.Lock()
							m.metrics.DispatchErrors++
							m.mu.Unlock()
							_, _ = m.store.UpdateInstanceStatus(ctx, inst.ID, "failed")
							m.logger.ErrorContext(ctx, "start dispatch failed during replacement",
								"instanceId", inst.ID,
								"nodeId", reason.NodeID,
								"error", err,
							)
						} else {
							_, _ = m.store.UpdateInstanceStatus(ctx, inst.ID, "provisioning")
						}
					}
				}
			}
		}

		m.publish(ctx, events.EventInstanceReplaced, instanceID, map[string]any{
			"instanceId": instanceID,
			"newNodeId":  reason.NodeID,
		})
	}
	return reason, nil
}

func (m *Manager) Status(ctx context.Context, appID string) (store.ReplicaApplication, []store.Instance, []store.PlacementDecision, error) {
	app, err := m.store.GetReplicaApp(ctx, appID)
	if err != nil {
		return store.ReplicaApplication{}, nil, nil, err
	}
	instances, err := m.store.ListInstancesByApp(ctx, appID)
	if err != nil {
		return store.ReplicaApplication{}, nil, nil, err
	}
	decisions, err := m.store.ListLatestPlacementPerInstance(ctx, appID)
	if err != nil {
		return store.ReplicaApplication{}, nil, nil, err
	}
	return app, instances, decisions, nil
}

func (m *Manager) VerifyNoDuplicates(ctx context.Context, appID string) error {
	instances, err := m.store.ListInstancesByApp(ctx, appID)
	if err != nil {
		return err
	}
	seen := make(map[int]string)
	for _, inst := range instances {
		if prevID, ok := seen[inst.Idx]; ok {
			m.mu.Lock()
			m.metrics.NoDuplicateAlloc++
			m.mu.Unlock()
			return fmt.Errorf("duplicate instance index %d: instances %s and %s", inst.Idx, prevID, inst.ID)
		}
		seen[inst.Idx] = inst.ID

		if inst.ReservationID != nil {
			res, err := m.reservations.GetReservation(ctx, *inst.ReservationID)
			if err == nil && res.Status == store.PlacementReservationStatusActive {
				m.mu.Lock()
				m.metrics.NoDoubleReservation++
				m.mu.Unlock()
			}
		}
	}
	return nil
}

func (m *Manager) Start(ctx context.Context) {
	if m == nil || m.store == nil {
		return
	}
	if !m.started.CompareAndSwap(false, true) {
		return
	}
	m.stopCh = make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-m.stopCh:
				return
			case <-ticker.C:
				m.reconcile(ctx)
			}
		}
	}()
}

func (m *Manager) Stop() {
	if m == nil {
		return
	}
	if m.started.CompareAndSwap(true, false) {
		if m.stopCh != nil {
			close(m.stopCh)
		}
	}
}

func (m *Manager) reconcile(ctx context.Context) {
	m.mu.Lock()
	m.metrics.ReconcileTotal++
	m.mu.Unlock()

	apps, err := m.store.ListReplicaApps(ctx)
	if err != nil {
		m.logger.ErrorContext(ctx, "reconcile list apps failed", "error", err)
		return
	}

	for _, app := range apps {
		instances, err := m.store.ListInstancesByApp(ctx, app.ID)
		if err != nil {
			continue
		}

		runningCount := 0
		hasFailed := 0
		hasProvisioning := false

		for _, inst := range instances {
			switch inst.Status {
			case "running":
				runningCount++
			case "failed":
				hasFailed++
			case "provisioning", "pending":
				hasProvisioning = true
			}
		}

		if hasFailed > 0 {
			_, _ = m.RetryFailedPlacements(ctx)
		}

		if !hasProvisioning && runningCount == app.Replicas && app.Status != string(AppDeploymentStatusRunning) {
			_, _ = m.store.UpdateReplicaAppStatus(ctx, app.ID, string(AppDeploymentStatusRunning))
			m.publish(ctx, events.EventAppRunning, app.ID, map[string]any{
				"appId":    app.ID,
				"replicas": app.Replicas,
				"running":  runningCount,
			})
		} else if runningCount > 0 && runningCount < app.Replicas && app.Status != string(AppDeploymentStatusDegraded) {
			_, _ = m.store.UpdateReplicaAppStatus(ctx, app.ID, string(AppDeploymentStatusDegraded))
			m.publish(ctx, events.EventAppDegraded, app.ID, map[string]any{
				"appId":    app.ID,
				"replicas": app.Replicas,
				"running":  runningCount,
			})
		} else if runningCount == 0 && hasFailed > 0 && !hasProvisioning && app.Status != string(AppDeploymentStatusFailed) {
			_, _ = m.store.UpdateReplicaAppStatus(ctx, app.ID, string(AppDeploymentStatusFailed))
			m.publish(ctx, events.EventAppFailed, app.ID, map[string]any{
				"appId":    app.ID,
				"replicas": app.Replicas,
				"failed":   hasFailed,
			})
		}
	}
}

func (m *Manager) updateAppStatus(ctx context.Context, appID string, deployed, _, desired int) {
	var status string
	if deployed == 0 {
		status = string(AppDeploymentStatusFailed)
	} else if deployed >= desired {
		status = string(AppDeploymentStatusRunning)
	} else {
		status = string(AppDeploymentStatusDegraded)
	}
	_, _ = m.store.UpdateReplicaAppStatus(ctx, appID, status)
}

func (m *Manager) computeAppStatus(ctx context.Context, appID string, running, desired int) string {
	if running == 0 {
		return string(AppDeploymentStatusFailed)
	}
	if running >= desired {
		return string(AppDeploymentStatusRunning)
	}
	return string(AppDeploymentStatusDegraded)
}

// dispatchBeaconCommand enqueues a command into Beacon's operation queue for the
// target node, logs it to store.BeaconCommandLog, and returns the command ID.
func (m *Manager) dispatchBeaconCommand(ctx context.Context, commandID, operationID, nodeID, serverID string, commandType InstanceCommandType, payload map[string]any) (string, error) {
	if m.beaconClient != nil {
		if err := m.beaconClient.DispatchCommand(ctx, nodeID, commandID, commandType, payload); err != nil {
			return commandID, err
		}
	}
	// Log the command to the store's beacon_command_logs table.
	m.logBeaconCommand(ctx, commandID, operationID, nodeID, serverID, string(commandType), "dispatched", payload)
	return commandID, nil
}

func (m *Manager) logBeaconCommand(ctx context.Context, commandID, operationID, nodeID, serverID, commandType, status string, payload map[string]any) {
	if m == nil || m.store == nil {
		return
	}
	correlationID := events.CorrelationIDFromContext(ctx)
	_, err := m.store.CreateBeaconCommandLog(ctx, store.CreateBeaconCommandLogRequest{
		CommandID:      commandID,
		OperationID:    operationID,
		CorrelationID:  correlationID,
		NodeID:         nodeID,
		ServerID:       serverID,
		CommandType:    commandType,
		Status:         status,
		RequestPayload: payload,
	})
	if err != nil {
		m.logger.ErrorContext(ctx, "log beacon command failed", "error", err, "commandId", commandID)
	}
}

func (m *Manager) publish(ctx context.Context, eventType events.EventType, resourceID string, payload map[string]any) {
	if m == nil || m.publisher == nil {
		return
	}
	_ = m.publisher.Publish(ctx, events.NewEnvelope(eventType, "replica-manager", "replica_app", resourceID, payload))
}

func (m *Manager) SnapshotMetrics(ctx context.Context) Metrics {
	return m.Metrics()
}

func (m *Manager) Health(ctx context.Context) error {
	_, err := m.store.ListReplicaApps(ctx)
	return err
}

func (m *Manager) RetryFailedPlacements(ctx context.Context) (int, error) {
	apps, err := m.store.ListReplicaApps(ctx)
	if err != nil {
		return 0, err
	}
	retried := 0
	for _, app := range apps {
		instances, err := m.store.ListInstancesByApp(ctx, app.ID)
		if err != nil {
			continue
		}
		for _, inst := range instances {
			if inst.Status == "failed" {
				_, _ = m.ReplaceInstance(ctx, inst.ID)
				retried++
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return retried, nil
}
