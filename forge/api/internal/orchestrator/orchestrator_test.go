package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"gamepanel/forge/internal/events"
	gpruntime "gamepanel/forge/internal/runtime"
	"gamepanel/forge/internal/store"
)

var (
	_ SuspensionStore   = (*mockSuspensionStore)(nil)
	_ SuspensionRuntime = (*mockSuspensionRuntime)(nil)
	_ events.Publisher  = (*mockPublisher)(nil)
)

type mockSuspensionStore struct {
	mock.Mock
}

func (m *mockSuspensionStore) GetServer(ctx context.Context, serverID string) (store.Server, error) {
	args := m.Called(ctx, serverID)
	return args.Get(0).(store.Server), args.Error(1)
}

func (m *mockSuspensionStore) SetServerSuspension(ctx context.Context, serverID string, suspended bool) error {
	args := m.Called(ctx, serverID, suspended)
	return args.Error(0)
}

func (m *mockSuspensionStore) ServerControlTarget(ctx context.Context, serverID string) (store.ServerControlTarget, error) {
	args := m.Called(ctx, serverID)
	return args.Get(0).(store.ServerControlTarget), args.Error(1)
}

type mockSuspensionRuntime struct {
	mock.Mock
}

func (m *mockSuspensionRuntime) StopServer(ctx context.Context, target gpruntime.Target) (gpruntime.PowerResponse, error) {
	args := m.Called(ctx, target)
	return args.Get(0).(gpruntime.PowerResponse), args.Error(1)
}

type mockPublisher struct {
	mock.Mock
}

func (m *mockPublisher) Publish(ctx context.Context, envelope events.Envelope) error {
	args := m.Called(ctx, envelope)
	return args.Error(0)
}

func TestComputeRegionCapacity(t *testing.T) {
	snapshots := []store.NodeCapacitySnapshot{
		{NodeID: "node-1", AllocatedCPU: 100, AvailableCPU: 900, AllocatedMemory: 512, AvailableMemory: 1536, AllocatedDisk: 10000, AvailableDisk: 90000, ServerCount: 5},
		{NodeID: "node-2", AllocatedCPU: 200, AvailableCPU: 800, AllocatedMemory: 1024, AvailableMemory: 1024, AllocatedDisk: 20000, AvailableDisk: 80000, ServerCount: 3},
	}

	capacity := ComputeRegionCapacity("region-1", snapshots)

	assert.Equal(t, "region-1", capacity.RegionID)
	assert.Equal(t, 300, capacity.AllocatedCPU)
	assert.Equal(t, 1700, capacity.AvailableCPU)
	assert.Equal(t, 1536, capacity.AllocatedMemory)
	assert.Equal(t, 2560, capacity.AvailableMemory)
	assert.Equal(t, 30000, capacity.AllocatedDisk)
	assert.Equal(t, 170000, capacity.AvailableDisk)
	assert.Equal(t, 8, capacity.ServerCount)
	assert.Len(t, capacity.Nodes, 2)
}

func TestSuspendServer(t *testing.T) {
	mStore := new(mockSuspensionStore)
	mRuntime := new(mockSuspensionRuntime)
	mPub := new(mockPublisher)

	mStore.On("GetServer", mock.Anything, "server-1").Return(store.Server{ID: "server-1", Suspended: false}, nil)
	mStore.On("ServerControlTarget", mock.Anything, "server-1").Return(store.ServerControlTarget{
		ServerID: "server-1", NodeURL: "http://node:9090", NodeToken: "token",
	}, nil)
	mRuntime.On("StopServer", mock.Anything, mock.Anything).Return(gpruntime.PowerResponse{Accepted: true}, nil)
	mStore.On("SetServerSuspension", mock.Anything, "server-1", true).Return(nil)
	mPub.On("Publish", mock.Anything, mock.Anything).Return(nil)

	err := SuspendServer(context.Background(), mStore, mRuntime, mPub, "server-1")

	assert.NoError(t, err)
	mStore.AssertExpectations(t)
	mRuntime.AssertExpectations(t)
	mPub.AssertExpectations(t)
}

func TestSuspendServer_StoreRequired(t *testing.T) {
	err := SuspendServer(context.Background(), nil, nil, nil, "server-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "store is required")
}

func TestSuspendServer_RuntimeRequired(t *testing.T) {
	mStore := new(mockSuspensionStore)
	err := SuspendServer(context.Background(), mStore, nil, nil, "server-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "runtime is required")
}

func TestSuspendServer_GetServerError(t *testing.T) {
	mStore := new(mockSuspensionStore)
	mStore.On("GetServer", mock.Anything, "server-1").Return(store.Server{}, assert.AnError)

	err := SuspendServer(context.Background(), mStore, new(mockSuspensionRuntime), nil, "server-1")
	assert.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
	mStore.AssertExpectations(t)
}

func TestSuspendServer_AlreadySuspended(t *testing.T) {
	mStore := new(mockSuspensionStore)
	mStore.On("GetServer", mock.Anything, "server-1").Return(store.Server{ID: "server-1", Suspended: true}, nil)

	err := SuspendServer(context.Background(), mStore, new(mockSuspensionRuntime), nil, "server-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already suspended")
	mStore.AssertExpectations(t)
}

func TestUnsuspendServer(t *testing.T) {
	mStore := new(mockSuspensionStore)
	mPub := new(mockPublisher)

	mStore.On("GetServer", mock.Anything, "server-1").Return(store.Server{ID: "server-1", Suspended: true}, nil)
	mStore.On("SetServerSuspension", mock.Anything, "server-1", false).Return(nil)
	mPub.On("Publish", mock.Anything, mock.Anything).Return(nil)

	err := UnsuspendServer(context.Background(), mStore, mPub, "server-1")

	assert.NoError(t, err)
	mStore.AssertExpectations(t)
	mPub.AssertExpectations(t)
}

func TestUnsuspendServer_StoreRequired(t *testing.T) {
	err := UnsuspendServer(context.Background(), nil, nil, "server-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "store is required")
}

func TestUnsuspendServer_GetServerError(t *testing.T) {
	mStore := new(mockSuspensionStore)
	mStore.On("GetServer", mock.Anything, "server-1").Return(store.Server{}, assert.AnError)

	err := UnsuspendServer(context.Background(), mStore, nil, "server-1")
	assert.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
	mStore.AssertExpectations(t)
}

func TestUnsuspendServer_AlreadyUnsuspended(t *testing.T) {
	mStore := new(mockSuspensionStore)
	mStore.On("GetServer", mock.Anything, "server-1").Return(store.Server{ID: "server-1", Suspended: false}, nil)

	err := UnsuspendServer(context.Background(), mStore, nil, "server-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "server not suspended")
	mStore.AssertExpectations(t)
}

func TestUnsuspendServer_SetServerSuspensionError(t *testing.T) {
	mStore := new(mockSuspensionStore)
	mStore.On("GetServer", mock.Anything, "server-1").Return(store.Server{ID: "server-1", Suspended: true}, nil)
	mStore.On("SetServerSuspension", mock.Anything, "server-1", false).Return(assert.AnError)

	err := UnsuspendServer(context.Background(), mStore, nil, "server-1")
	assert.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
	mStore.AssertExpectations(t)
}
