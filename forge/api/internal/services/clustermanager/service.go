package clustermanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/events"
	gpruntime "gamepanel/forge/internal/runtime"
	reservationsvc "gamepanel/forge/internal/services/reservations"
	schedulersvc "gamepanel/forge/internal/services/scheduler"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

type clusterStore interface {
	FindAvailableAllocation(context.Context, string) (store.Allocation, error)
	CreateServer(context.Context, store.CreateServerRequest) (store.Server, error)
	GetServer(context.Context, string) (store.Server, error)
	GetNode(context.Context, string) (store.Node, error)
	UpdateServer(context.Context, string, store.UpdateServerRequest, *string) (store.Server, error)
	ServerControlTarget(context.Context, string) (store.ServerControlTarget, error)
	ServerProvisionTarget(context.Context, string) (store.ServerProvisionTarget, error)
	SetServerProvisioned(context.Context, string) error
	SetServerInstallState(context.Context, string, string, string) error
	MarkServerConfigSynced(context.Context, string) error
	MarkServerConfigSyncFailed(context.Context, string, string) error
	HardDeleteServer(context.Context, string) error
	RecordOrphanAndHardDeleteServer(context.Context, string, string, string) error
	NodeCapacitySnapshot(context.Context, string) (store.NodeCapacitySnapshot, error)
	RegionCapacitySnapshots(context.Context, string) ([]store.NodeCapacitySnapshot, error)
	SetServerDesiredState(context.Context, string, store.ServerDesiredState, string) error
	SetServerActualState(context.Context, string, store.ServerActualState, string) error
	ResetNodeServerStates(context.Context, string) error
	ListServersForNode(context.Context, string) ([]store.Server, error)
}

type Service struct {
	store        clusterStore
	runtime      gpruntime.Runtime
	scheduler    schedulersvc.Service
	reservations *reservationsvc.Manager
	publisher    events.Publisher
}

type RegionCapacity struct {
	RegionID        string                       `json:"regionId"`
	Nodes           []store.NodeCapacitySnapshot `json:"nodes"`
	AllocatedCPU    int                          `json:"allocated_cpu"`
	AvailableCPU    int                          `json:"available_cpu"`
	AllocatedMemory int                          `json:"allocated_memory"`
	AvailableMemory int                          `json:"available_memory"`
	AllocatedDisk   int                          `json:"allocated_disk"`
	AvailableDisk   int                          `json:"available_disk"`
	ServerCount     int                          `json:"server_count"`
}

func New(store *store.Store, runtime gpruntime.Runtime, scheduler schedulersvc.Service, reservationManager *reservationsvc.Manager, publishers ...events.Publisher) *Service {
	return newService(store, runtime, scheduler, reservationManager, publishers...)
}

func newService(store clusterStore, runtime gpruntime.Runtime, scheduler schedulersvc.Service, reservationManager *reservationsvc.Manager, publishers ...events.Publisher) *Service {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Service{store: store, runtime: runtime, scheduler: scheduler, reservations: reservationManager, publisher: publisher}
}

func (s *Service) CreateServer(ctx context.Context, req store.CreateServerRequest, placementReq domain.PlacementRequest) (store.Server, domain.PlacementDecision, error) {
	correlationID := uuid.NewString()
	ctx = events.ContextWithCorrelationID(ctx, correlationID)
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.TemplateID) == "" {
		return store.Server{}, domain.PlacementDecision{}, errors.New("name and templateId are required")
	}
	if strings.TrimSpace(req.OwnerID) == "" {
		return store.Server{}, domain.PlacementDecision{}, errors.New("ownerId is required")
	}
	placementReq.NodeID = firstNonEmpty(placementReq.NodeID, req.NodeID)
	placementReq.RequiredNode = firstNonEmpty(placementReq.RequiredNode, req.NodeID)
	placementReq.AllocationID = firstNonEmpty(placementReq.AllocationID, req.AllocationID)
	placementReq.MemoryMB = firstNonZero(placementReq.MemoryMB, req.MemoryMB)
	placementReq.CPUShares = firstNonZero(placementReq.CPUShares, req.CPUShares)
	placementReq.DiskMB = firstNonZero(placementReq.DiskMB, req.DiskMB)
	decision, err := s.scheduler.PlaceServer(ctx, placementReq)
	if err != nil {
		return store.Server{}, domain.PlacementDecision{}, err
	}
	req.NodeID = decision.NodeID
	req.AllocationID = decision.AllocationID
	if req.AllocationID == "" {
		allocation, err := s.store.FindAvailableAllocation(ctx, req.NodeID)
		if err != nil {
			return store.Server{}, domain.PlacementDecision{}, err
		}
		req.AllocationID = allocation.ID
		decision.AllocationID = allocation.ID
		decision.Reasons = append(decision.Reasons, "selected first available allocation on placed node")
	}
	reservationID := decision.ReservationID
	var reservation store.PlacementReservation
	if reservationID == "" && s.reservations != nil {
		reservation, err = s.reservations.CreateReservation(ctx, store.CreatePlacementReservationRequest{
			NodeID:          req.NodeID,
			ReservationType: store.PlacementReservationTypePlacement,
			CPU:             firstNonZero(placementReq.CPU, placementReq.CPUShares, req.CPUShares),
			Memory:          int64(firstNonZero(placementReq.MemoryMB, req.MemoryMB)),
			Disk:            int64(firstNonZero(placementReq.DiskMB, req.DiskMB)),
			Status:          store.PlacementReservationStatusActive,
			ExpiresAt:       time.Now().UTC().Add(10 * time.Minute),
		})
		if err != nil {
			return store.Server{}, domain.PlacementDecision{}, err
		}
		reservationID = reservation.ID
		decision.Reasons = append(decision.Reasons, "reserved capacity on placed node")
		decision.ReservationID = reservationID
	} else if reservationID != "" {
		reservation, err = s.reservations.GetReservation(ctx, reservationID)
		if err != nil {
			return store.Server{}, domain.PlacementDecision{}, err
		}
	}
	s.publish(ctx, events.EventPlacementCreated, "placement", decision.NodeID, map[string]any{
		"regionId":      decision.RegionID,
		"nodeId":        decision.NodeID,
		"allocationId":  decision.AllocationID,
		"manual":        decision.Manual,
		"score":         decision.Score,
		"reasons":       decision.Reasons,
		"correlationId": correlationID,
	})
	server, err := s.store.CreateServer(ctx, req)
	if err != nil {
		s.cancelReservation(ctx, reservationID)
		return store.Server{}, domain.PlacementDecision{}, err
	}
	created := false
	if s.runtime == nil {
		err = gpruntime.ErrRuntimeUnavailable
	} else {
		created, err = s.provisionServer(ctx, server.ID)
	}
	if err != nil {
		s.cancelReservation(ctx, reservationID)
		return store.Server{}, domain.PlacementDecision{}, s.compensateCreateFailure(ctx, server.ID, created, err)
	}
	if s.reservations != nil && reservationID != "" {
		if _, err := s.reservations.ConfirmReservation(ctx, reservationID); err != nil {
			s.cancelReservation(ctx, reservationID)
			return store.Server{}, domain.PlacementDecision{}, s.compensateCreateFailure(ctx, server.ID, true, err)
		}
	}
	server, err = s.store.GetServer(ctx, server.ID)
	if err != nil {
		return store.Server{}, domain.PlacementDecision{}, err
	}
	s.publish(ctx, events.EventServerCreated, "server", server.ID, map[string]any{
		"name": server.Name, "node": server.Node, "desiredState": server.DesiredState,
		"actualState": server.ActualState, "correlationId": correlationID,
	})
	return server, decision, nil
}

func (s *Service) provisionServer(ctx context.Context, serverID string) (bool, error) {
	target, err := s.store.ServerProvisionTarget(ctx, serverID)
	if err != nil {
		return false, err
	}
	if err := s.syncProvisionTarget(ctx, target); err != nil {
		return false, err
	}
	response, err := s.runtime.CreateServer(ctx, runtimeTargetFromProvision(target), runtimeCreateRequest(target))
	if err != nil {
		return false, err
	}
	if !response.Accepted {
		return false, errors.New("runtime rejected server creation")
	}
	if err := s.store.SetServerProvisioned(ctx, serverID); err != nil {
		return true, err
	}
	return true, nil
}

// ProvisionRecoveredServer creates a workload from data already restored into
// the destination server directory. Installation is deliberately skipped.
func (s *Service) ProvisionRecoveredServer(ctx context.Context, serverID string) error {
	if s == nil || s.runtime == nil {
		return gpruntime.ErrRuntimeUnavailable
	}
	target, err := s.store.ServerProvisionTarget(ctx, serverID)
	if err != nil {
		return err
	}
	if err := s.syncProvisionTarget(ctx, target); err != nil {
		return err
	}
	response, err := s.runtime.CreateServer(ctx, runtimeTargetFromProvision(target), runtimeCreateRequest(target))
	if err != nil {
		return err
	}
	if !response.Accepted {
		return errors.New("runtime rejected recovered server creation")
	}
	return nil
}

func (s *Service) InstallServer(ctx context.Context, serverID string) (gpruntime.InstallResponse, error) {
	return s.runInstaller(ctx, serverID, false)
}

func (s *Service) ReinstallServer(ctx context.Context, serverID string) (gpruntime.InstallResponse, error) {
	return s.runInstaller(ctx, serverID, true)
}

func (s *Service) runInstaller(ctx context.Context, serverID string, reinstall bool) (gpruntime.InstallResponse, error) {
	if s.runtime == nil {
		return gpruntime.InstallResponse{}, gpruntime.ErrRuntimeUnavailable
	}
	target, err := s.store.ServerProvisionTarget(ctx, serverID)
	if err != nil {
		return gpruntime.InstallResponse{}, err
	}
	if err := s.store.SetServerInstallState(ctx, serverID, "installing", ""); err != nil {
		return gpruntime.InstallResponse{}, err
	}
	if err := s.syncProvisionTarget(ctx, target); err != nil {
		_ = s.store.SetServerInstallState(ctx, serverID, "failed", err.Error())
		return gpruntime.InstallResponse{}, err
	}
	var response gpruntime.InstallResponse
	if reinstall {
		reinstaller, ok := s.runtime.(gpruntime.Reinstaller)
		if !ok {
			err = errors.New("runtime does not support reinstall")
		} else {
			response, err = reinstaller.ReinstallServer(ctx, runtimeTargetFromProvision(target), runtimeInstallRequest(target))
		}
	} else {
		response, err = s.runtime.InstallServer(ctx, runtimeTargetFromProvision(target), runtimeInstallRequest(target))
	}
	if err != nil {
		_ = s.store.SetServerInstallState(ctx, serverID, "failed", err.Error())
		return gpruntime.InstallResponse{}, err
	}
	if !response.Accepted || response.ExitCode != 0 {
		err = fmt.Errorf("runtime install failed: accepted=%t exitCode=%d", response.Accepted, response.ExitCode)
		_ = s.store.SetServerInstallState(ctx, serverID, "failed", err.Error())
		return response, err
	}
	if err := s.store.SetServerInstallState(ctx, serverID, "installed", ""); err != nil {
		return response, err
	}
	return response, nil
}

func (s *Service) SyncServerConfiguration(ctx context.Context, serverID string) error {
	if s.runtime == nil {
		return gpruntime.ErrRuntimeUnavailable
	}
	target, err := s.store.ServerProvisionTarget(ctx, serverID)
	if err != nil {
		return err
	}
	return s.syncProvisionTarget(ctx, target)
}

func (s *Service) syncProvisionTarget(ctx context.Context, target store.ServerProvisionTarget) error {
	err := s.runtime.SyncServerConfiguration(ctx, runtimeTargetFromProvision(target), runtimeConfiguration(target))
	if err != nil {
		_ = s.store.MarkServerConfigSyncFailed(ctx, target.ServerID, err.Error())
		return err
	}
	return s.store.MarkServerConfigSynced(ctx, target.ServerID)
}

func (s *Service) compensateCreateFailure(ctx context.Context, serverID string, workloadCreated bool, cause error) error {
	target, targetErr := s.store.ServerControlTarget(ctx, serverID)
	if targetErr != nil {
		return errors.Join(cause, targetErr)
	}
	if workloadCreated {
		if _, deleteErr := s.runtime.DeleteServer(ctx, runtimeTarget(target)); deleteErr != nil {
			if cleanupErr := s.store.RecordOrphanAndHardDeleteServer(ctx, serverID, target.NodeURL, deleteErr.Error()); cleanupErr != nil {
				return errors.Join(cause, deleteErr, cleanupErr)
			}
			return errors.Join(cause, deleteErr)
		}
	}
	if cleanupErr := s.store.HardDeleteServer(ctx, serverID); cleanupErr != nil {
		return errors.Join(cause, cleanupErr)
	}
	return cause
}

func (s *Service) cancelReservation(ctx context.Context, reservationID string) {
	if s.reservations != nil && reservationID != "" {
		_, _ = s.reservations.CancelReservation(ctx, reservationID)
	}
}

func (s *Service) NodeCapacity(ctx context.Context, nodeID string) (store.NodeCapacitySnapshot, error) {
	return s.store.NodeCapacitySnapshot(ctx, nodeID)
}

func (s *Service) RegionCapacity(ctx context.Context, regionID string) (RegionCapacity, error) {
	snapshots, err := s.store.RegionCapacitySnapshots(ctx, regionID)
	if err != nil {
		return RegionCapacity{}, err
	}
	capacity := RegionCapacity{RegionID: regionID, Nodes: snapshots}
	for _, snapshot := range snapshots {
		capacity.AllocatedCPU += snapshot.AllocatedCPU
		capacity.AvailableCPU += snapshot.AvailableCPU
		capacity.AllocatedMemory += snapshot.AllocatedMemory
		capacity.AvailableMemory += snapshot.AvailableMemory
		capacity.AllocatedDisk += snapshot.AllocatedDisk
		capacity.AvailableDisk += snapshot.AvailableDisk
		capacity.ServerCount += snapshot.ServerCount
	}
	return capacity, nil
}

func (s *Service) DeleteServer(ctx context.Context, serverID string, force bool) (gpruntime.PowerResponse, error) {
	ctx = events.ContextWithCorrelationID(ctx, firstNonEmpty(events.CorrelationIDFromContext(ctx), uuid.NewString()))
	target, err := s.store.ServerControlTarget(ctx, serverID)
	if err != nil {
		return gpruntime.PowerResponse{}, err
	}
	var response gpruntime.PowerResponse
	var daemonErr error
	if s.runtime == nil {
		daemonErr = gpruntime.ErrRuntimeUnavailable
	} else {
		response, daemonErr = s.runtime.DeleteServer(ctx, runtimeTarget(target))
	}
	if daemonErr != nil && !force {
		return gpruntime.PowerResponse{}, daemonErr
	}
	if daemonErr != nil {
		if err := s.store.RecordOrphanAndHardDeleteServer(ctx, target.ServerID, target.NodeURL, daemonErr.Error()); err != nil {
			return gpruntime.PowerResponse{}, errors.Join(daemonErr, err)
		}
		s.publish(ctx, events.EventServerDeleted, "server", target.ServerID, map[string]any{"force": true, "orphaned": true})
		return gpruntime.PowerResponse{ServerID: target.ServerID, Accepted: true, Mode: "force"}, daemonErr
	}
	if err := s.store.HardDeleteServer(ctx, target.ServerID); err != nil {
		return gpruntime.PowerResponse{}, err
	}
	s.publish(ctx, events.EventServerDeleted, "server", target.ServerID, map[string]any{"force": force})
	return response, nil
}

func (s *Service) StartServer(ctx context.Context, serverID string) (gpruntime.PowerResponse, error) {
	ctx = events.ContextWithCorrelationID(ctx, firstNonEmpty(events.CorrelationIDFromContext(ctx), uuid.NewString()))
	return s.sendPower(ctx, serverID, "start")
}

func (s *Service) StopServer(ctx context.Context, serverID string) (gpruntime.PowerResponse, error) {
	ctx = events.ContextWithCorrelationID(ctx, firstNonEmpty(events.CorrelationIDFromContext(ctx), uuid.NewString()))
	return s.sendPower(ctx, serverID, "stop")
}

func (s *Service) RestartServer(ctx context.Context, serverID string) (gpruntime.PowerResponse, error) {
	ctx = events.ContextWithCorrelationID(ctx, firstNonEmpty(events.CorrelationIDFromContext(ctx), uuid.NewString()))
	return s.sendPower(ctx, serverID, "restart")
}

func (s *Service) KillServer(ctx context.Context, serverID string) (gpruntime.PowerResponse, error) {
	ctx = events.ContextWithCorrelationID(ctx, firstNonEmpty(events.CorrelationIDFromContext(ctx), uuid.NewString()))
	return s.sendPower(ctx, serverID, "kill")
}

func (s *Service) RequestServerPower(ctx context.Context, serverID, signal string) (gpruntime.PowerResponse, domain.ServerDesiredState, error) {
	correlationID := uuid.NewString()
	ctx = events.ContextWithCorrelationID(ctx, correlationID)
	desired := domain.ServerDesiredStateStopped
	if signal == "start" || signal == "restart" {
		desired = domain.ServerDesiredState(store.ServerDesiredStateRunning)
	}
	if signal == "start" || signal == "restart" {
		server, err := s.store.GetServer(ctx, serverID)
		if err != nil {
			return gpruntime.PowerResponse{}, desired, err
		}
		if server.Suspended {
			return gpruntime.PowerResponse{}, desired, errors.New("cannot start or restart a suspended server")
		}
	}
	if err := s.store.SetServerDesiredState(ctx, serverID, store.ServerDesiredState(desired), "cluster manager request "+signal); err != nil {
		return gpruntime.PowerResponse{}, desired, err
	}
	s.publish(ctx, events.EventDesiredStateChanged, "server", serverID, map[string]any{
		"desiredState":  desired,
		"signal":        signal,
		"correlationId": correlationID,
	})
	response, err := s.sendPower(ctx, serverID, signal)
	if err != nil {
		return gpruntime.PowerResponse{}, desired, err
	}
	return response, desired, nil
}

func (s *Service) RefreshServerActualState(ctx context.Context, serverID string) (domain.ServerActualState, error) {
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return domain.ServerActualStateUnknown, err
	}
	if s.runtime == nil || (server.DesiredState != store.ServerDesiredStateRunning && server.ActualState != store.ServerActualStateRunning) {
		return domain.ServerActualState(server.ActualState), nil
	}
	target, err := s.store.ServerControlTarget(ctx, serverID)
	if err != nil {
		return domain.ServerActualStateUnknown, err
	}
	if _, err := s.runtime.Stats(ctx, runtimeTarget(target)); err != nil {
		if server.ActualState == store.ServerActualStateRunning {
			actual := domain.ServerActualStateUnknown
			if setErr := s.store.SetServerActualState(ctx, serverID, store.ServerActualState(actual), "runtime stats refresh failed"); setErr != nil {
				return actual, setErr
			}
			s.publish(ctx, events.EventActualStateChanged, "server", serverID, map[string]any{
				"actualState": actual,
				"reason":      "runtime stats refresh failed",
			})
			return actual, nil
		}
		return domain.ServerActualState(server.ActualState), nil
	}
	if err := s.store.SetServerActualState(ctx, serverID, store.ServerActualStateRunning, "runtime stats refresh"); err != nil {
		return domain.ServerActualState(store.ServerActualStateRunning), err
	}
	s.publish(ctx, events.EventActualStateChanged, "server", serverID, map[string]any{
		"actualState": store.ServerActualStateRunning,
		"reason":      "runtime stats refresh",
	})
	return domain.ServerActualState(store.ServerActualStateRunning), nil
}

func (s *Service) ReconcileNode(ctx context.Context, nodeID string) error {
	if err := s.store.ResetNodeServerStates(ctx, nodeID); err != nil {
		return fmt.Errorf("reset node server states: %w", err)
	}

	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("get node: %w", err)
	}
	nodeHealthy := node.HeartbeatState == string(store.NodeHeartbeatStateHealthy)

	servers, err := s.store.ListServersForNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("list servers for node: %w", err)
	}

	for _, server := range servers {
		desiredRunning := string(store.ServerDesiredStateRunning)
		desiredStopped := string(store.ServerDesiredStateStopped)
		actualRunning := string(store.ServerActualStateRunning)

		if string(server.DesiredState) == desiredRunning && string(server.ActualState) != actualRunning {
			s.publish(ctx, events.EventActualStateChanged, "server", server.ID, map[string]any{
				"reason":           "reconciliation: desired state is running but actual state differs",
				"nodeId":           nodeID,
				"nodeHealthy":      nodeHealthy,
				"desiredState":     server.DesiredState,
				"actualState":      server.ActualState,
				"remediate":        true,
				"remediationStart": nodeHealthy,
			})
		}

		if string(server.DesiredState) == desiredStopped && string(server.ActualState) == actualRunning {
			s.publish(ctx, events.EventActualStateChanged, "server", server.ID, map[string]any{
				"reason":          "reconciliation: server should be stopped but is still running",
				"nodeId":          nodeID,
				"nodeHealthy":     nodeHealthy,
				"desiredState":    server.DesiredState,
				"actualState":     server.ActualState,
				"remediate":       true,
				"remediationStop": nodeHealthy,
			})
		}
	}

	return nil
}

func (s *Service) GetServerStats(ctx context.Context, serverID string) (*gpruntime.Stats, error) {
	if s.runtime == nil {
		return nil, gpruntime.ErrRuntimeUnavailable
	}
	target, err := s.store.ServerControlTarget(ctx, serverID)
	if err != nil {
		return nil, err
	}
	stats, err := s.runtime.Stats(ctx, runtimeTarget(target))
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

func (s *Service) ResizeServer(ctx context.Context, serverID string, memoryMB, cpu int64) error {
	mem := int(memoryMB)
	cpuShares := int(cpu)
	if _, err := s.store.UpdateServer(ctx, serverID, store.UpdateServerRequest{
		MemoryMB:  &mem,
		CPUShares: &cpuShares,
	}, nil); err != nil {
		return fmt.Errorf("update server resources: %w", err)
	}
	target, err := s.store.ServerControlTarget(ctx, serverID)
	if err != nil {
		return fmt.Errorf("get server target: %w", err)
	}
	if err := s.runtime.ResizeServer(ctx, runtimeTarget(target), memoryMB, cpu); err != nil {
		return fmt.Errorf("resize runtime server: %w", err)
	}
	return nil
}

func (s *Service) sendPower(ctx context.Context, serverID, signal string) (gpruntime.PowerResponse, error) {
	if s.runtime == nil {
		if err := s.store.SetServerActualState(ctx, serverID, store.ServerActualState(serverActualFromSignal(signal)), "cluster manager power "+signal); err != nil {
			return gpruntime.PowerResponse{}, err
		}
		s.publishServerPowerEvents(ctx, serverID, signal)
		return gpruntime.PowerResponse{ServerID: serverID, Signal: signal, Accepted: true}, nil
	}
	target, err := s.store.ServerControlTarget(ctx, serverID)
	if err != nil {
		return gpruntime.PowerResponse{}, err
	}
	response, err := s.sendRuntimePower(ctx, runtimeTarget(target), signal)
	if err != nil {
		return gpruntime.PowerResponse{}, err
	}
	if err := s.store.SetServerActualState(ctx, serverID, store.ServerActualState(serverActualFromSignal(signal)), "cluster manager power "+signal); err != nil {
		return gpruntime.PowerResponse{}, err
	}
	s.publishServerPowerEvents(ctx, serverID, signal)
	if response.ServerID == "" {
		response.ServerID = serverID
	}
	if response.Signal == "" {
		response.Signal = signal
	}
	response.Accepted = true
	return response, nil
}

func (s *Service) sendRuntimePower(ctx context.Context, target gpruntime.Target, signal string) (gpruntime.PowerResponse, error) {
	switch signal {
	case "start":
		return s.runtime.StartServer(ctx, target)
	case "stop":
		return s.runtime.StopServer(ctx, target)
	case "restart":
		return s.runtime.RestartServer(ctx, target)
	case "kill":
		return s.runtime.KillServer(ctx, target)
	default:
		return gpruntime.PowerResponse{}, errors.New("unsupported power signal")
	}
}

func (s *Service) publishServerPowerEvents(ctx context.Context, serverID, signal string) {
	actual := serverActualFromSignal(signal)
	s.publish(ctx, events.EventActualStateChanged, "server", serverID, map[string]any{
		"actualState": actual,
		"signal":      signal,
	})
	switch signal {
	case "start":
		s.publish(ctx, events.EventServerStarted, "server", serverID, map[string]any{"signal": signal})
	case "stop", "kill":
		s.publish(ctx, events.EventServerStopped, "server", serverID, map[string]any{"signal": signal})
	case "restart":
		s.publish(ctx, events.EventServerRestarted, "server", serverID, map[string]any{"signal": signal})
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
	_ = s.publisher.Publish(ctx, events.NewEnvelope(eventType, "cluster-manager", resourceType, resourceID, payload))
}

func serverActualFromSignal(signal string) domain.ServerActualState {
	if signal == "start" || signal == "restart" {
		return domain.ServerActualState(store.ServerActualStateRunning)
	}
	return domain.ServerActualStateStopped
}

func (s *Service) Ready() error {
	if s.store == nil {
		return errors.New("postgres is required")
	}
	return nil
}

func (s *Service) DaemonReady() error {
	if err := s.Ready(); err != nil {
		return err
	}
	if s.runtime == nil {
		return errors.New("runtime is required")
	}
	return nil
}

func runtimeCreateRequest(target store.ServerProvisionTarget) gpruntime.CreateServerRequest {
	envMap := map[string]string{
		"SERVER_MEMORY": fmt.Sprintf("%d", target.MemoryMB),
		"SERVER_IP":     target.AllocationIP,
		"SERVER_PORT":   fmt.Sprintf("%d", target.AllocationPort),
	}
	for key, value := range target.Environment {
		envMap[key] = value
	}
	keys := make([]string, 0, len(envMap))
	for key := range envMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+envMap[key])
	}
	command := []string{"/bin/sh", "-lc", target.StartupCommand}
	if strings.Contains(target.Image, "itzg/minecraft-server") {
		command = nil
		env = append(env, "EULA=TRUE")
	}
	ports := make([]gpruntime.Port, 0, len(target.Allocations))
	for _, allocation := range target.Allocations {
		containerPort := allocation.ContainerPort
		if containerPort == 0 {
			containerPort = allocation.Port
		}
		protocol := allocation.Protocol
		if protocol == "" {
			protocol = "tcp"
		}
		ports = append(ports, gpruntime.Port{HostIP: allocation.IP, HostPort: allocation.Port, ContainerPort: containerPort, Protocol: protocol})
	}
	mounts := make([]gpruntime.Mount, 0, len(target.Mounts))
	for _, mount := range target.Mounts {
		mounts = append(mounts, gpruntime.Mount{Source: mount.Source, Target: mount.Target, ReadOnly: mount.ReadOnly})
	}
	return gpruntime.CreateServerRequest{
		ServerID: target.ServerID, Name: target.Name, Image: target.Image, Command: command, Env: env,
		Ports: ports, Mounts: mounts, MemoryMB: target.MemoryMB, SwapMB: target.SwapMB,
		CPUShares: target.CPUShares, CPULimit: target.CPULimit, DiskMB: target.DiskMB,
		IOWeight: target.IOWeight, Threads: target.Threads, OOMDisabled: target.OOMDisabled,
	}
}

func runtimeInstallRequest(target store.ServerProvisionTarget) gpruntime.InstallRequest {
	env := map[string]string{"SERVER_MEMORY": fmt.Sprintf("%d", target.MemoryMB), "SERVER_IP": target.AllocationIP, "SERVER_PORT": fmt.Sprintf("%d", target.AllocationPort)}
	for key, value := range target.Environment {
		env[key] = value
	}
	return gpruntime.InstallRequest{ServerID: target.ServerID, Image: target.InstallContainer, Entrypoint: target.InstallEntrypoint, Script: target.InstallScript, Env: env}
}

func runtimeConfiguration(target store.ServerProvisionTarget) gpruntime.ServerConfiguration {
	config := map[string]any{}
	if strings.TrimSpace(target.ConfigJSON) != "" {
		_ = json.Unmarshal([]byte(target.ConfigJSON), &config)
	}
	denylist := []string{}
	if strings.TrimSpace(target.FileDenylist) != "" {
		_ = json.Unmarshal([]byte(target.FileDenylist), &denylist)
	}
	allocations := map[string]any{"default": map[string]any{"ip": target.AllocationIP, "port": target.AllocationPort}, "mappings": map[string][]int{}, "ports": target.Allocations}
	mappings := map[string][]int{}
	for _, allocation := range target.Allocations {
		mappings[allocation.IP] = append(mappings[allocation.IP], allocation.Port)
	}
	allocations["mappings"] = mappings
	mounts := make([]gpruntime.Mount, 0, len(target.Mounts))
	for _, mount := range target.Mounts {
		mounts = append(mounts, gpruntime.Mount{Source: mount.Source, Target: mount.Target, ReadOnly: mount.ReadOnly})
	}
	return gpruntime.ServerConfiguration{
		UUID: target.ServerID, Name: target.Name, Environment: target.Environment, Invocation: target.StartupCommand,
		DockerImage: target.Image, Egg: map[string]any{"id": target.EggID, "fileDenylist": denylist}, Config: config, Allocations: allocations, Mounts: mounts,
		Build: map[string]any{"memoryLimit": target.MemoryMB, "swapMb": target.SwapMB, "cpuShares": target.CPUShares, "cpuLimit": target.CPULimit, "diskSpace": target.DiskMB, "ioWeight": target.IOWeight, "threads": target.Threads, "oomDisabled": target.OOMDisabled},
	}
}

func runtimeTargetFromProvision(target store.ServerProvisionTarget) gpruntime.Target {
	return gpruntime.Target{NodeURL: target.NodeURL, NodeToken: target.NodeToken, ServerID: target.ServerID}
}

func runtimeTarget(target store.ServerControlTarget) gpruntime.Target {
	return gpruntime.Target{
		NodeURL:   target.NodeURL,
		NodeToken: target.NodeToken,
		ServerID:  target.ServerID,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
