package clustermanager

import (
	"context"
	"errors"
	"testing"

	"gamepanel/forge/internal/store"

	gpruntime "gamepanel/forge/internal/runtime"
)

type mockClusterStore struct {
	serverControlTarget       store.ServerControlTarget
	serverControlTargetErr    error
	hardDeleteServerErr       error
	recordOrphanAndHardDelete func(ctx context.Context, serverID, nodeURL, deleteErr string) error
	serverProvisionTarget     store.ServerProvisionTarget
	serverProvisionTargetErr  error
	createServerRequest       store.CreateServerRequest
	createServerErr           error
	getServerResult           store.Server
	getServerErr              error
	getNodeResult             store.Node
	getNodeErr                error
	listServersForNodeResult  []store.Server
	listServersForNodeErr     error
	markServerConfigSyncedErr error
	markServerConfigSyncFail  func(ctx context.Context, serverID, errMsg string) error
}

func (m *mockClusterStore) FindAvailableAllocation(ctx context.Context, nodeID string) (store.Allocation, error) {
	return store.Allocation{}, nil
}

func (m *mockClusterStore) CreateServer(ctx context.Context, req store.CreateServerRequest) (store.Server, error) {
	m.createServerRequest = req
	return store.Server{ID: "server-1", Node: req.NodeID}, m.createServerErr
}

func (m *mockClusterStore) GetServer(ctx context.Context, id string) (store.Server, error) {
	return m.getServerResult, m.getServerErr
}

func (m *mockClusterStore) GetNode(ctx context.Context, id string) (store.Node, error) {
	return m.getNodeResult, m.getNodeErr
}

func (m *mockClusterStore) ServerControlTarget(ctx context.Context, serverID string) (store.ServerControlTarget, error) {
	return m.serverControlTarget, m.serverControlTargetErr
}

func (m *mockClusterStore) ServerProvisionTarget(ctx context.Context, serverID string) (store.ServerProvisionTarget, error) {
	return m.serverProvisionTarget, m.serverProvisionTargetErr
}

func (m *mockClusterStore) SetServerProvisioned(ctx context.Context, serverID string) error {
	return nil
}

func (m *mockClusterStore) SetServerInstallState(ctx context.Context, serverID, state, errMsg string) error {
	return nil
}

func (m *mockClusterStore) MarkServerConfigSynced(ctx context.Context, serverID string) error {
	return m.markServerConfigSyncedErr
}

func (m *mockClusterStore) MarkServerConfigSyncFailed(ctx context.Context, serverID, errMsg string) error {
	if m.markServerConfigSyncFail != nil {
		return m.markServerConfigSyncFail(ctx, serverID, errMsg)
	}
	return nil
}

func (m *mockClusterStore) HardDeleteServer(ctx context.Context, serverID string) error {
	return m.hardDeleteServerErr
}

func (m *mockClusterStore) RecordOrphanAndHardDeleteServer(ctx context.Context, serverID, nodeURL, deleteErr string) error {
	if m.recordOrphanAndHardDelete != nil {
		return m.recordOrphanAndHardDelete(ctx, serverID, nodeURL, deleteErr)
	}
	return nil
}

func (m *mockClusterStore) NodeCapacitySnapshot(ctx context.Context, nodeID string) (store.NodeCapacitySnapshot, error) {
	return store.NodeCapacitySnapshot{}, nil
}

func (m *mockClusterStore) RegionCapacitySnapshots(ctx context.Context, regionID string) ([]store.NodeCapacitySnapshot, error) {
	return nil, nil
}

func (m *mockClusterStore) SetServerDesiredState(ctx context.Context, serverID string, state store.ServerDesiredState, reason string) error {
	return nil
}

func (m *mockClusterStore) SetServerActualState(ctx context.Context, serverID string, state store.ServerActualState, reason string) error {
	return nil
}

func (m *mockClusterStore) UpdateServer(ctx context.Context, id string, req store.UpdateServerRequest, actorID *string) (store.Server, error) {
	return store.Server{}, nil
}

func (m *mockClusterStore) ListServersForNode(ctx context.Context, nodeID string) ([]store.Server, error) {
	return m.listServersForNodeResult, m.listServersForNodeErr
}

func (m *mockClusterStore) ResetNodeServerStates(ctx context.Context, nodeID string) error {
	return nil
}

type mockRuntime struct {
	deleteServerResponse gpruntime.PowerResponse
	deleteServerErr      error
}

func (m *mockRuntime) Name() string { return "mock" }

func (m *mockRuntime) Capabilities() gpruntime.Capabilities { return gpruntime.Capabilities{} }

func (m *mockRuntime) SupportsMigration() bool { return false }

func (m *mockRuntime) CreateServer(ctx context.Context, t gpruntime.Target, req gpruntime.CreateServerRequest) (gpruntime.CreateResponse, error) {
	return gpruntime.CreateResponse{Accepted: true}, nil
}

func (m *mockRuntime) InstallServer(ctx context.Context, t gpruntime.Target, req gpruntime.InstallRequest) (gpruntime.InstallResponse, error) {
	return gpruntime.InstallResponse{Accepted: true, ExitCode: 0}, nil
}

func (m *mockRuntime) SyncServerConfiguration(ctx context.Context, t gpruntime.Target, cfg gpruntime.ServerConfiguration) error {
	return nil
}

func (m *mockRuntime) ResizeServer(ctx context.Context, t gpruntime.Target, memoryMB, cpu int64) error {
	return nil
}

func (m *mockRuntime) DeleteServer(ctx context.Context, t gpruntime.Target) (gpruntime.PowerResponse, error) {
	return m.deleteServerResponse, m.deleteServerErr
}

func (m *mockRuntime) StartServer(ctx context.Context, t gpruntime.Target) (gpruntime.PowerResponse, error) {
	return gpruntime.PowerResponse{}, nil
}

func (m *mockRuntime) StopServer(ctx context.Context, t gpruntime.Target) (gpruntime.PowerResponse, error) {
	return gpruntime.PowerResponse{}, nil
}

func (m *mockRuntime) RestartServer(ctx context.Context, t gpruntime.Target) (gpruntime.PowerResponse, error) {
	return gpruntime.PowerResponse{}, nil
}

func (m *mockRuntime) KillServer(ctx context.Context, t gpruntime.Target) (gpruntime.PowerResponse, error) {
	return gpruntime.PowerResponse{}, nil
}

func (m *mockRuntime) Stats(ctx context.Context, t gpruntime.Target) (gpruntime.Stats, error) {
	return gpruntime.Stats{}, nil
}

func (m *mockRuntime) Exists(ctx context.Context, t gpruntime.Target) (bool, error) {
	return true, nil
}

func (m *mockRuntime) Inspect(ctx context.Context, t gpruntime.Target) (gpruntime.Inspection, error) {
	return gpruntime.Inspection{}, nil
}

func (m *mockRuntime) PrepareMigration(ctx context.Context, req gpruntime.MigrationRequest) (gpruntime.MigrationResponse, error) {
	return gpruntime.MigrationResponse{}, nil
}

func (m *mockRuntime) ExecuteMigration(ctx context.Context, req gpruntime.MigrationRequest) (gpruntime.MigrationResponse, error) {
	return gpruntime.MigrationResponse{}, nil
}

func (m *mockRuntime) CancelMigration(ctx context.Context, req gpruntime.MigrationRequest) (gpruntime.MigrationResponse, error) {
	return gpruntime.MigrationResponse{}, nil
}

func TestCompensateCreateFailure_WorkloadNotCreated(t *testing.T) {
	store := &mockClusterStore{
		serverControlTarget: store.ServerControlTarget{
			ServerID: "server-1",
			NodeURL:  "http://node:8080",
		},
		hardDeleteServerErr: nil,
	}
	runtime := &mockRuntime{}
	svc := newService(store, runtime, nil, nil)
	cause := errors.New("provisioning failed")

	err := svc.compensateCreateFailure(context.Background(), "server-1", false, cause)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("expected error to contain cause, got %v", err)
	}
}

func TestCompensateCreateFailure_WorkloadCreatedThenRuntimeDeletesSuccessfully(t *testing.T) {
	store := &mockClusterStore{
		serverControlTarget: store.ServerControlTarget{
			ServerID: "server-1",
			NodeURL:  "http://node:8080",
		},
		hardDeleteServerErr: nil,
	}
	runtime := &mockRuntime{
		deleteServerResponse: gpruntime.PowerResponse{Accepted: true},
		deleteServerErr:      nil,
	}
	svc := newService(store, runtime, nil, nil)
	cause := errors.New("reservation confirmation failed")

	err := svc.compensateCreateFailure(context.Background(), "server-1", true, cause)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCompensateCreateFailure_RuntimeDeleteFailsRecordsOrphan(t *testing.T) {
	orphanRecorded := false
	store := &mockClusterStore{
		serverControlTarget: store.ServerControlTarget{
			ServerID: "server-1",
			NodeURL:  "http://node:8080",
		},
		hardDeleteServerErr: nil,
		recordOrphanAndHardDelete: func(ctx context.Context, serverID, nodeURL, deleteErr string) error {
			orphanRecorded = true
			if serverID != "server-1" {
				t.Fatalf("expected server-1, got %s", serverID)
			}
			if nodeURL != "http://node:8080" {
				t.Fatalf("expected http://node:8080, got %s", nodeURL)
			}
			return nil
		},
	}
	runtime := &mockRuntime{
		deleteServerErr: errors.New("daemon unreachable"),
	}
	svc := newService(store, runtime, nil, nil)
	cause := errors.New("provisioning failed")

	err := svc.compensateCreateFailure(context.Background(), "server-1", true, cause)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !orphanRecorded {
		t.Fatal("expected orphan to be recorded when runtime delete fails")
	}
}

func TestCompensateCreateFailure_ServerControlTargetFails(t *testing.T) {
	store := &mockClusterStore{
		serverControlTargetErr: errors.New("target not found"),
	}
	svc := newService(store, nil, nil, nil)
	cause := errors.New("provisioning failed")

	err := svc.compensateCreateFailure(context.Background(), "server-1", true, cause)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCompensateCreateFailure_HardDeleteFails(t *testing.T) {
	store := &mockClusterStore{
		serverControlTarget: store.ServerControlTarget{
			ServerID: "server-1",
			NodeURL:  "http://node:8080",
		},
		hardDeleteServerErr: errors.New("delete failed"),
	}
	runtime := &mockRuntime{}
	svc := newService(store, runtime, nil, nil)
	cause := errors.New("provisioning failed")

	err := svc.compensateCreateFailure(context.Background(), "server-1", false, cause)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestReconcileNode_ServersMatch(t *testing.T) {
	store := &mockClusterStore{
		getNodeResult: store.Node{
			HeartbeatState: string(store.NodeHeartbeatStateHealthy),
		},
		listServersForNodeResult: []store.Server{
			{ID: "srv-1", DesiredState: store.ServerDesiredStateRunning, ActualState: store.ServerActualStateRunning},
			{ID: "srv-2", DesiredState: store.ServerDesiredStateStopped, ActualState: store.ServerActualStateStopped},
		},
	}
	svc := newService(store, nil, nil, nil)
	err := svc.ReconcileNode(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReconcileNode_DesiredRunning_ActualNotRunning(t *testing.T) {
	store := &mockClusterStore{
		getNodeResult: store.Node{
			HeartbeatState: string(store.NodeHeartbeatStateHealthy),
		},
		listServersForNodeResult: []store.Server{
			{ID: "srv-1", DesiredState: store.ServerDesiredStateRunning, ActualState: store.ServerActualStateStopped},
		},
	}
	svc := newService(store, nil, nil, nil)
	err := svc.ReconcileNode(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReconcileNode_DesiredStopped_ActualRunning(t *testing.T) {
	store := &mockClusterStore{
		getNodeResult: store.Node{
			HeartbeatState: string(store.NodeHeartbeatStateHealthy),
		},
		listServersForNodeResult: []store.Server{
			{ID: "srv-1", DesiredState: store.ServerDesiredStateStopped, ActualState: store.ServerActualStateRunning},
		},
	}
	svc := newService(store, nil, nil, nil)
	err := svc.ReconcileNode(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReconcileNode_NodeUnhealthy(t *testing.T) {
	store := &mockClusterStore{
		getNodeResult: store.Node{
			HeartbeatState: string(store.NodeHeartbeatStateOffline),
		},
		listServersForNodeResult: []store.Server{
			{ID: "srv-1", DesiredState: store.ServerDesiredStateRunning, ActualState: store.ServerActualStateStopped},
		},
	}
	svc := newService(store, nil, nil, nil)
	err := svc.ReconcileNode(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReconcileNode_GetNodeError(t *testing.T) {
	store := &mockClusterStore{
		getNodeErr: errors.New("node not found"),
	}
	svc := newService(store, nil, nil, nil)
	err := svc.ReconcileNode(context.Background(), "node-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestReady(t *testing.T) {
	t.Run("store is nil", func(t *testing.T) {
		svc := newService(nil, nil, nil, nil)
		err := svc.Ready()
		if err == nil {
			t.Fatal("expected error when store is nil")
		}
		if err.Error() != "postgres is required" {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("store is present", func(t *testing.T) {
		svc := newService(&mockClusterStore{}, nil, nil, nil)
		err := svc.Ready()
		if err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})
}

func TestDaemonReady(t *testing.T) {
	t.Run("store is nil", func(t *testing.T) {
		svc := newService(nil, nil, nil, nil)
		err := svc.DaemonReady()
		if err == nil {
			t.Fatal("expected error when store is nil")
		}
	})

	t.Run("runtime is nil", func(t *testing.T) {
		svc := newService(&mockClusterStore{}, nil, nil, nil)
		err := svc.DaemonReady()
		if err == nil {
			t.Fatal("expected error when runtime is nil")
		}
		if err.Error() != "runtime is required" {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("both present", func(t *testing.T) {
		svc := newService(&mockClusterStore{}, &mockRuntime{}, nil, nil)
		err := svc.DaemonReady()
		if err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})
}

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{"all empty", []string{"", "", ""}, ""},
		{"first non-empty", []string{"", "b", "c"}, "b"},
		{"first value used", []string{"a", "b", "c"}, "a"},
		{"with spaces", []string{"", "  ", "d"}, "d"},
		{"single value", []string{"x"}, "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstNonEmpty(tt.values...)
			if got != tt.want {
				t.Fatalf("firstNonEmpty(%v) = %q, want %q", tt.values, got, tt.want)
			}
		})
	}
}

func TestFirstNonZero(t *testing.T) {
	tests := []struct {
		name   string
		values []int
		want   int
	}{
		{"all zero", []int{0, 0, 0}, 0},
		{"first non-zero", []int{0, 5, 3}, 5},
		{"first value used", []int{1, 2, 3}, 1},
		{"single zero", []int{0}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstNonZero(tt.values...)
			if got != tt.want {
				t.Fatalf("firstNonZero(%v) = %d, want %d", tt.values, got, tt.want)
			}
		})
	}
}

func TestServerActualFromSignal(t *testing.T) {
	tests := []struct {
		signal string
		want   string
	}{
		{"start", "running"},
		{"restart", "running"},
		{"stop", "stopped"},
		{"kill", "stopped"},
		{"unknown", "stopped"},
	}
	for _, tt := range tests {
		t.Run(tt.signal, func(t *testing.T) {
			got := serverActualFromSignal(tt.signal)
			if string(got) != tt.want {
				t.Fatalf("serverActualFromSignal(%q) = %q, want %q", tt.signal, got, tt.want)
			}
		})
	}
}
