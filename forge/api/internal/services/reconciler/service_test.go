package reconciler

import (
	"context"
	"testing"
	"time"

	"gamepanel/forge/internal/domain"
	gpruntime "gamepanel/forge/internal/runtime"
	"gamepanel/forge/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockStore struct {
	mock.Mock
}

func (m *mockStore) ListServers(ctx context.Context) ([]store.Server, error) {
	args := m.Called(ctx)
	return args.Get(0).([]store.Server), args.Error(1)
}

func (m *mockStore) ListServersForNode(ctx context.Context, nodeID string) ([]store.Server, error) {
	args := m.Called(ctx, nodeID)
	return args.Get(0).([]store.Server), args.Error(1)
}

func (m *mockStore) GetServer(ctx context.Context, id string) (store.Server, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(store.Server), args.Error(1)
}

func (m *mockStore) GetNode(ctx context.Context, id string) (store.Node, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(store.Node), args.Error(1)
}

func (m *mockStore) ListNodes(ctx context.Context) ([]store.Node, error) {
	args := m.Called(ctx)
	return args.Get(0).([]store.Node), args.Error(1)
}

func (m *mockStore) SetNodeHeartbeatClassification(ctx context.Context, nodeID string, heartbeatState store.NodeHeartbeatState, actualState store.NodeActualState, recoveryCount int, reason string) (store.Node, store.Node, error) {
	args := m.Called(ctx, nodeID, heartbeatState, actualState, recoveryCount, reason)
	return args.Get(0).(store.Node), args.Get(1).(store.Node), args.Error(2)
}

func (m *mockStore) NodeCapacitySnapshot(ctx context.Context, id string) (store.NodeCapacitySnapshot, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(store.NodeCapacitySnapshot), args.Error(1)
}

func (m *mockStore) CreateReconcilePlan(ctx context.Context, plan *store.ReconcilePlanRow) error {
	args := m.Called(ctx, plan)
	return args.Error(0)
}

func (m *mockStore) GetReconcilePlan(ctx context.Context, id string) (*store.ReconcilePlanRow, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*store.ReconcilePlanRow), args.Error(1)
}

func (m *mockStore) ListReconcilePlansByResource(ctx context.Context, resourceID, resourceKind string) ([]store.ReconcilePlanRow, error) {
	args := m.Called(ctx, resourceID, resourceKind)
	return args.Get(0).([]store.ReconcilePlanRow), args.Error(1)
}

func (m *mockStore) ListReconcilePlans(ctx context.Context, offset, limit int) ([]store.ReconcilePlanRow, int, error) {
	args := m.Called(ctx, offset, limit)
	return args.Get(0).([]store.ReconcilePlanRow), args.Int(1), args.Error(2)
}

func (m *mockStore) UpdateReconcilePlanState(ctx context.Context, id, state, execError string) error {
	args := m.Called(ctx, id, state, execError)
	return args.Error(0)
}

func (m *mockStore) ConfirmReconcilePlan(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockStore) RecordReconcileEvent(ctx context.Context, event *store.ReconcileEventRow) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *mockStore) ListReconcileEvents(ctx context.Context, resourceID string, limit int) ([]store.ReconcileEventRow, error) {
	args := m.Called(ctx, resourceID, limit)
	return args.Get(0).([]store.ReconcileEventRow), args.Error(1)
}

func (m *mockStore) ReconcileSummary(ctx context.Context) (*store.ReconcileSummary, error) {
	args := m.Called(ctx)
	return args.Get(0).(*store.ReconcileSummary), args.Error(1)
}

func (m *mockStore) ListComposeStacks(ctx context.Context, userID string) ([]store.ComposeStack, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]store.ComposeStack), args.Error(1)
}

func (m *mockStore) ListComposeStacksForReconciliation(ctx context.Context) ([]store.ComposeStack, error) {
	args := m.Called(ctx)
	return args.Get(0).([]store.ComposeStack), args.Error(1)
}

func (m *mockStore) GetComposeStack(ctx context.Context, stackID string) (store.ComposeStack, error) {
	args := m.Called(ctx, stackID)
	return args.Get(0).(store.ComposeStack), args.Error(1)
}

type mockClusterManager struct {
	mock.Mock
}

func (m *mockClusterManager) RefreshServerActualState(ctx context.Context, serverID string) (domain.ServerActualState, error) {
	args := m.Called(ctx, serverID)
	return args.Get(0).(domain.ServerActualState), args.Error(1)
}

func (m *mockClusterManager) StartServer(ctx context.Context, serverID string) (gpruntime.PowerResponse, error) {
	args := m.Called(ctx, serverID)
	return args.Get(0).(gpruntime.PowerResponse), args.Error(1)
}

func (m *mockClusterManager) StopServer(ctx context.Context, serverID string) (gpruntime.PowerResponse, error) {
	args := m.Called(ctx, serverID)
	return args.Get(0).(gpruntime.PowerResponse), args.Error(1)
}

func (m *mockClusterManager) RestartServer(ctx context.Context, serverID string) (gpruntime.PowerResponse, error) {
	args := m.Called(ctx, serverID)
	return args.Get(0).(gpruntime.PowerResponse), args.Error(1)
}

func setupTestService() (*Service, *mockStore, *mockClusterManager) {
	mockSt := new(mockStore)
	mockCm := new(mockClusterManager)
	svc := &Service{
		store:          mockSt,
		clusterManager: mockCm,
		interval:       time.Hour, // prevent auto-ticker
		autoReconcile:  false,
	}
	return svc, mockSt, mockCm
}

// --- Tests ---

func TestNoDrift(t *testing.T) {
	svc, mockSt, mockCm := setupTestService()

	srv := store.Server{
		ID:           "srv-1",
		DesiredState: store.ServerDesiredStateRunning,
		MemoryMB:     1024,
		CPUShares:    100,
		DiskMB:       10000,
	}

	mockSt.On("GetServer", mock.Anything, "srv-1").Return(srv, nil)
	mockCm.On("RefreshServerActualState", mock.Anything, "srv-1").Return(domain.ServerActualStateRunning, nil)
	mockSt.On("ListReconcilePlansByResource", mock.Anything, "srv-1", "server").Return([]store.ReconcilePlanRow{}, nil)
	mockSt.On("CreateReconcilePlan", mock.Anything, mock.Anything).Return(nil)

	desired, observed, err := svc.captureServerSnapshots(context.Background(), "srv-1")
	assert.NoError(t, err)

	diffs := computeDiffs(desired, observed)
	assert.Len(t, diffs, 1)
	assert.Equal(t, DiffNoOp, diffs[0].DiffType)

	drifts := svc.detectDrift(context.Background(), diffs)
	assert.Len(t, drifts, 0)
}

func TestConfigurationDrift(t *testing.T) {
	srv := store.Server{
		ID:           "srv-2",
		DesiredState: store.ServerDesiredStateRunning,
		MemoryMB:     2048,
		CPUShares:    200,
		DiskMB:       50000,
	}

	desired := DesiredStateSnapshot{
		ResourceID:   "srv-2",
		ResourceKind: ResourceKindServer,
		ServerState:  &srv.DesiredState,
		ConfigHash:   snapshotHash(map[string]any{"memoryMb": 2048, "cpuShares": 200, "diskMb": 50000}),
	}

	observed := ObservedStateSnapshot{
		ResourceID:   "srv-2",
		ResourceKind: ResourceKindServer,
		ServerState:  statePtr(store.ServerActualStateRunning),
		ConfigHash:   snapshotHash(map[string]any{"memoryMb": 1024, "cpuShares": 100, "diskMb": 25000}),
	}

	diffs := computeDiffs(desired, observed)
	assert.Len(t, diffs, 1)
	assert.Equal(t, DiffUpdate, diffs[0].DiffType)
	assert.True(t, diffs[0].Details["configChanged"].(bool))
}

func TestMissingRuntimeResource(t *testing.T) {
	desired := DesiredStateSnapshot{
		ResourceID:   "srv-3",
		ResourceKind: ResourceKindServer,
		ServerState:  statePtr(store.ServerDesiredStateRunning),
		ConfigHash:   "abc",
	}

	observed := ObservedStateSnapshot{
		ResourceID:   "srv-3",
		ResourceKind: ResourceKindServer,
		ServerState:  nil,
	}

	diffs := computeDiffs(desired, observed)
	assert.Len(t, diffs, 1)
	assert.Equal(t, DiffCreate, diffs[0].DiffType)
}

func TestOrphanedRuntimeResource(t *testing.T) {
	desired := DesiredStateSnapshot{
		ResourceID:   "srv-4",
		ResourceKind: ResourceKindServer,
		ServerState:  nil,
	}

	observed := ObservedStateSnapshot{
		ResourceID:   "srv-4",
		ResourceKind: ResourceKindServer,
		ServerState:  statePtr(store.ServerActualStateRunning),
		ConfigHash:   "xyz",
	}

	diffs := computeDiffs(desired, observed)
	assert.Len(t, diffs, 1)
	assert.Equal(t, DiffDelete, diffs[0].DiffType)
}

func TestUnsafeDestructiveDeleteDetection(t *testing.T) {
	diffs := []ReconcileDiff{
		{ResourceID: "srv-5", ResourceKind: ResourceKindServer, DiffType: DiffDelete},
	}
	drifts := []DriftRecord{}

	plan := generatePlan("srv-5", ResourceKindServer, diffs, drifts)
	assert.True(t, plan.Destructive)

	assert.True(t, requiresConfirmation(diffs))
}

func TestConcurrentEditRejected(t *testing.T) {
	svc, mockSt, _ := setupTestService()

	existing := []store.ReconcilePlanRow{
		{ID: "plan-existing", State: string(PlanExecuting)},
	}
	mockSt.On("ListReconcilePlansByResource", mock.Anything, "srv-6", "server").Return(existing, nil)

	canReconcile := svc.ensureNoDuplicateReconcile(context.Background(), "srv-6", ResourceKindServer)
	assert.False(t, canReconcile)
}

func TestAPIStartupRestorePlans(t *testing.T) {
	svc, mockSt, _ := setupTestService()

	emptyPlans := []store.ReconcilePlanRow{}

	mockSt.On("ListReconcilePlansByResource", mock.Anything, "srv-10", "server").Return(emptyPlans, nil)
	mockSt.On("ListReconcilePlansByResource", mock.Anything, "srv-11", "server").Return(emptyPlans, nil)

	canReconcile := svc.ensureNoDuplicateReconcile(context.Background(), "srv-10", ResourceKindServer)
	assert.True(t, canReconcile)

	_ = svc.ensureNoDuplicateReconcile(context.Background(), "srv-11", ResourceKindServer)
	// No crash = restart safe
}

func TestDuplicateReconcileBlocked(t *testing.T) {
	svc, mockSt, _ := setupTestService()

	pending := []store.ReconcilePlanRow{
		{ID: "pending-plan", State: string(PlanPending)},
	}
	mockSt.On("ListReconcilePlansByResource", mock.Anything, "srv-7", "server").Return(pending, nil)

	canReconcile := svc.ensureNoDuplicateReconcile(context.Background(), "srv-7", ResourceKindServer)
	assert.False(t, canReconcile)
}

func TestFailedReconciliation(t *testing.T) {
	svc, mockSt, mockCm := setupTestService()

	srv := store.Server{
		ID:           "srv-8",
		DesiredState: store.ServerDesiredStateRunning,
	}

	mockSt.On("GetServer", mock.Anything, "srv-8").Return(srv, nil)
	mockCm.On("RefreshServerActualState", mock.Anything, "srv-8").Return(domain.ServerActualState(""), assert.AnError)
	mockSt.On("ListReconcilePlansByResource", mock.Anything, "srv-8", "server").Return([]store.ReconcilePlanRow{}, nil)
	mockSt.On("CreateReconcilePlan", mock.Anything, mock.Anything).Return(nil)

	plan, err := svc.TriggerReconcile(context.Background(), "srv-8", ResourceKindServer)
	assert.NoError(t, err)
	assert.NotNil(t, plan)
}

func TestBeaconReconnectSafety(t *testing.T) {
	srv := store.Server{
		ID:           "srv-9",
		DesiredState: store.ServerDesiredStateRunning,
	}

	desired := DesiredStateSnapshot{
		ResourceID:   "srv-9",
		ResourceKind: ResourceKindServer,
		ServerState:  &srv.DesiredState,
		ConfigHash:   snapshotHash(srv),
	}

	observed := ObservedStateSnapshot{
		ResourceID:   "srv-9",
		ResourceKind: ResourceKindServer,
		ServerState:  statePtr(store.ServerActualStateRunning),
		ConfigHash:   snapshotHash(srv),
	}

	diffs := computeDiffs(desired, observed)
	assert.Len(t, diffs, 1)
	assert.Equal(t, DiffNoOp, diffs[0].DiffType)
}

func TestGeneratePlan(t *testing.T) {
	diffs := []ReconcileDiff{
		{ResourceID: "srv-a", DiffType: DiffNoOp},
		{ResourceID: "srv-b", DiffType: DiffUpdate},
	}
	drifts := []DriftRecord{
		{ResourceID: "srv-b", DriftKind: DriftConfigMismatch},
	}

	plan := generatePlan("test-server", ResourceKindServer, diffs, drifts)
	assert.Equal(t, "test-server", plan.ResourceID)
	assert.Equal(t, ResourceKindServer, plan.ResourceKind)
	assert.Len(t, plan.Diffs, 2)
	assert.Len(t, plan.Drifts, 1)
	assert.False(t, plan.Destructive)
}

func TestDiffSummaries(t *testing.T) {
	diffs := []ReconcileDiff{
		{DiffType: DiffCreate},
		{DiffType: DiffUpdate},
		{DiffType: DiffDelete},
		{DiffType: DiffNoOp},
		{DiffType: DiffUpdate},
	}

	s := diffSummaries(diffs)
	assert.Equal(t, 5, s.TotalDiffs)
	assert.Equal(t, 1, s.Creates)
	assert.Equal(t, 2, s.Updates)
	assert.Equal(t, 1, s.Deletes)
	assert.Equal(t, 1, s.NoOps)
}

func TestDriftClassification(t *testing.T) {
	assert.Equal(t, "configuration drift", classifyDrift(DriftRecord{DriftKind: DriftConfigMismatch}))
	assert.Equal(t, "missing resource", classifyDrift(DriftRecord{DriftKind: DriftMissingResource}))
	assert.Equal(t, "orphaned resource", classifyDrift(DriftRecord{DriftKind: DriftOrphanedResource}))
	assert.Equal(t, "state mismatch", classifyDrift(DriftRecord{DriftKind: DriftStateMismatch}))
	assert.Equal(t, "unknown", classifyDrift(DriftRecord{DriftKind: "undefined"}))
}

func TestDestructiveConfirmation(t *testing.T) {
	tests := []struct {
		name            string
		diffs           []ReconcileDiff
		expectNeedsConf bool
	}{
		{"no destructive", []ReconcileDiff{{DiffType: DiffCreate}, {DiffType: DiffUpdate}}, false},
		{"has delete", []ReconcileDiff{{DiffType: DiffCreate}, {DiffType: DiffDelete}}, true},
		{"all noop", []ReconcileDiff{{DiffType: DiffNoOp}}, false},
		{"only delete", []ReconcileDiff{{DiffType: DiffDelete}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectNeedsConf, requiresConfirmation(tt.diffs))
		})
	}
}

func TestSnapshotHash(t *testing.T) {
	h1 := snapshotHash(map[string]any{"a": 1, "b": "hello"})
	h2 := snapshotHash(map[string]any{"a": 1, "b": "hello"})
	h3 := snapshotHash(map[string]any{"a": 2, "b": "world"})

	assert.Equal(t, h1, h2)
	assert.NotEqual(t, h1, h3)
	assert.NotEmpty(t, h1)
	assert.Len(t, h1, 16) // 8 bytes hex = 16 chars
}

func TestComputeServerDiffs(t *testing.T) {
	tests := []struct {
		name           string
		desired        *store.ServerDesiredState
		observed       *store.ServerActualState
		expectedType   ReconcileDiffType
		expectEmpty    bool
	}{
		{
			name:         "both nil",
			expectedType: "",
			expectEmpty:  true,
		},
		{
			name:         "create",
			desired:      statePtr(store.ServerDesiredStateRunning),
			observed:     nil,
			expectedType: DiffCreate,
		},
		{
			name:         "delete",
			desired:      nil,
			observed:     statePtr(store.ServerActualStateStopped),
			expectedType: DiffDelete,
		},
		{
			name:         "state mismatch",
			desired:      statePtr(store.ServerDesiredStateRunning),
			observed:     statePtr(store.ServerActualStateStopped),
			expectedType: DiffUpdate,
		},
		{
			name:         "in sync",
			desired:      statePtr(store.ServerDesiredStateRunning),
			observed:     statePtr(store.ServerActualStateRunning),
			expectedType: DiffNoOp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := "fixed-hash"
			desired := DesiredStateSnapshot{
				ResourceID:   "test",
				ResourceKind: ResourceKindServer,
				ServerState:  tt.desired,
				ConfigHash:   hash,
			}
			observed := ObservedStateSnapshot{
				ResourceID:   "test",
				ResourceKind: ResourceKindServer,
				ServerState:  tt.observed,
				ConfigHash:   hash,
			}
			diffs := computeDiffs(desired, observed)
			if tt.expectEmpty {
				assert.Empty(t, diffs)
			} else if tt.expectedType == DiffNoOp {
				assert.GreaterOrEqual(t, len(diffs), 1)
				found := false
				for _, d := range diffs {
					if d.DiffType == DiffNoOp {
						found = true
					}
				}
				assert.True(t, found, "expected a no-op diff")
			} else {
				assert.Len(t, diffs, 1)
				assert.Equal(t, tt.expectedType, diffs[0].DiffType)
			}
		})
	}
}

func TestCollectServerConfigHash(t *testing.T) {
	srv1 := store.Server{ID: "a", MemoryMB: 1024, CPUShares: 100, DiskMB: 10000, DockerImage: "nginx", StartupCommand: "nginx -g"}
	srv2 := store.Server{ID: "b", MemoryMB: 2048, CPUShares: 200, DiskMB: 20000, DockerImage: "nginx", StartupCommand: "nginx -g"}

	h1 := collectServerConfigHash(srv1)
	h2 := collectServerConfigHash(srv2)

	assert.NotEqual(t, h1, h2)
	assert.NotEmpty(t, h1)
}

func statePtr[T ~string](s T) *T {
	return &s
}

func TestComputeComposeDiffs_NoDiff(t *testing.T) {
	desired := DesiredStateSnapshot{
		ResourceID:   "stack-1",
		ResourceKind: ResourceKindComposeStack,
		ComposeState: &ComposeDesiredState{Status: "running", ComposeYAML: "yaml", ComposeHash: "abc"},
		ConfigHash:   "hash1",
	}
	observed := ObservedStateSnapshot{
		ResourceID:   "stack-1",
		ResourceKind: ResourceKindComposeStack,
		ComposeState: &ComposeObservedState{Status: "running"},
		ConfigHash:   "hash1",
	}
	diffs := computeComposeDiffs(desired, observed)
	assert.Len(t, diffs, 1)
	assert.Equal(t, DiffNoOp, diffs[0].DiffType)
}

func TestComputeComposeDiffs_StatusDrift(t *testing.T) {
	desired := DesiredStateSnapshot{
		ResourceID:   "stack-2",
		ResourceKind: ResourceKindComposeStack,
		ComposeState: &ComposeDesiredState{Status: "running", ComposeYAML: "yaml", ComposeHash: "abc"},
		ConfigHash:   "hash1",
	}
	observed := ObservedStateSnapshot{
		ResourceID:   "stack-2",
		ResourceKind: ResourceKindComposeStack,
		ComposeState: &ComposeObservedState{Status: "degraded"},
		ConfigHash:   "hash1",
	}
	diffs := computeComposeDiffs(desired, observed)
	assert.Len(t, diffs, 1)
	assert.Equal(t, DiffUpdate, diffs[0].DiffType)
}

func TestComputeComposeDiffs_MissingObserved(t *testing.T) {
	desired := DesiredStateSnapshot{
		ResourceID:   "stack-3",
		ResourceKind: ResourceKindComposeStack,
		ComposeState: &ComposeDesiredState{Status: "running", ComposeYAML: "yaml", ComposeHash: "abc"},
		ConfigHash:   "hash1",
	}
	observed := ObservedStateSnapshot{
		ResourceID:   "stack-3",
		ResourceKind: ResourceKindComposeStack,
		ComposeState: nil,
	}
	diffs := computeComposeDiffs(desired, observed)
	assert.Len(t, diffs, 1)
	assert.Equal(t, DiffCreate, diffs[0].DiffType)
}
