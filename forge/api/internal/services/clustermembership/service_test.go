package clustermembership

import (
	"context"
	"errors"
	"testing"

	"gamepanel/forge/internal/services/evacuationplanner"
	"gamepanel/forge/internal/store"
)

type mockMembershipStore struct {
	getNodeResult            store.Node
	getNodeErr               error
	updateNodeCalled         bool
	updateNodeReq            store.UpdateNodeRequest
	updateNodeErr            error
	listServersForNodeResult []store.Server
	listServersForNodeErr    error
	createPlanCalled         bool
	executePlanCalled        bool
}

func (m *mockMembershipStore) GetNode(_ context.Context, _ string) (store.Node, error) {
	return m.getNodeResult, m.getNodeErr
}

func (m *mockMembershipStore) UpdateNode(_ context.Context, _ string, req store.UpdateNodeRequest, _ *string) (store.Node, error) {
	m.updateNodeCalled = true
	m.updateNodeReq = req
	return store.Node{}, m.updateNodeErr
}

func (m *mockMembershipStore) ListServersForNode(_ context.Context, _ string) ([]store.Server, error) {
	return m.listServersForNodeResult, m.listServersForNodeErr
}

func (m *mockMembershipStore) CreatePlan(_ context.Context, _ string) (evacuationplanner.PlanResult, error) {
	m.createPlanCalled = true
	return evacuationplanner.PlanResult{}, errors.New("noop")
}

func (m *mockMembershipStore) ExecutePlan(_ context.Context, _ string) (store.EvacuationPlan, error) {
	m.executePlanCalled = true
	return store.EvacuationPlan{}, errors.New("noop")
}

func TestJoin(t *testing.T) {
	t.Run("activates offline node", func(t *testing.T) {
		mock := &mockMembershipStore{
			getNodeResult: store.Node{ID: "node-1", HeartbeatState: string(store.NodeHeartbeatStateOffline)},
		}
		svc := &Service{store: mock, draining: make(map[string]chan struct{})}
		err := svc.Join(context.Background(), "node-1")
		if err != nil {
			t.Fatalf("Join returned error: %v", err)
		}
		if mock.updateNodeReq.DesiredState != store.NodeDesiredStateActive {
			t.Fatalf("expected desired state active, got %v", mock.updateNodeReq.DesiredState)
		}
	})

	t.Run("rejects already active node", func(t *testing.T) {
		mock := &mockMembershipStore{
			getNodeResult: store.Node{ID: "node-1", HeartbeatState: string(store.NodeHeartbeatStateHealthy), DesiredState: "active"},
		}
		svc := &Service{store: mock, draining: make(map[string]chan struct{})}
		err := svc.Join(context.Background(), "node-1")
		if err == nil {
			t.Fatal("expected error for already active node")
		}
	})

	t.Run("nil service returns error", func(t *testing.T) {
		var svc *Service
		err := svc.Join(context.Background(), "node-1")
		if err == nil {
			t.Fatal("expected error from nil service")
		}
	})
}

func TestLeave(t *testing.T) {
	t.Run("rejects node with active workloads", func(t *testing.T) {
		mock := &mockMembershipStore{
			getNodeResult:            store.Node{ID: "node-1", HeartbeatState: string(store.NodeHeartbeatStateHealthy)},
			listServersForNodeResult: []store.Server{{ID: "srv-1"}},
		}
		svc := &Service{store: mock, draining: make(map[string]chan struct{})}
		err := svc.Leave(context.Background(), "node-1")
		if err == nil {
			t.Fatal("expected error for node with active workloads")
		}
	})

	t.Run("deactivates empty node", func(t *testing.T) {
		mock := &mockMembershipStore{
			getNodeResult:            store.Node{ID: "node-1", HeartbeatState: string(store.NodeHeartbeatStateHealthy)},
			listServersForNodeResult: nil,
		}
		svc := &Service{store: mock, draining: make(map[string]chan struct{})}
		err := svc.Leave(context.Background(), "node-1")
		if err != nil {
			t.Fatalf("Leave returned error: %v", err)
		}
	})
}

func TestDrain(t *testing.T) {
	t.Run("starts drain on active node", func(t *testing.T) {
		mock := &mockMembershipStore{
			getNodeResult: store.Node{ID: "node-1", HeartbeatState: string(store.NodeHeartbeatStateHealthy)},
		}
		svc := &Service{store: mock, draining: make(map[string]chan struct{})}
		mockEvacuator := &mockMembershipStore{}
		svc.SetEvacuationPlanner(mockEvacuator)

		err := svc.StartDrain(context.Background(), "node-1")
		if err != nil {
			t.Fatalf("StartDrain returned error: %v", err)
		}
	})

	t.Run("rejects drain without evacuator", func(t *testing.T) {
		mock := &mockMembershipStore{
			getNodeResult: store.Node{ID: "node-1", HeartbeatState: string(store.NodeHeartbeatStateHealthy)},
		}
		svc := &Service{store: mock, draining: make(map[string]chan struct{})}
		err := svc.StartDrain(context.Background(), "node-1")
		if err == nil {
			t.Fatal("expected error without evacuator")
		}
	})
}

func TestMaintenance(t *testing.T) {
	t.Run("enables maintenance on idle node", func(t *testing.T) {
		mock := &mockMembershipStore{
			getNodeResult: store.Node{ID: "node-1", HeartbeatState: string(store.NodeHeartbeatStateHealthy)},
		}
		svc := &Service{store: mock, draining: make(map[string]chan struct{})}
		err := svc.EnableMaintenance(context.Background(), "node-1", "scheduled upgrade")
		if err != nil {
			t.Fatalf("EnableMaintenance returned error: %v", err)
		}
		if !mock.updateNodeReq.Maintenance {
			t.Fatal("expected maintenance=true")
		}
	})

	t.Run("disables maintenance", func(t *testing.T) {
		mock := &mockMembershipStore{
			getNodeResult: store.Node{ID: "node-1"},
		}
		svc := &Service{store: mock, draining: make(map[string]chan struct{})}
		err := svc.DisableMaintenance(context.Background(), "node-1")
		if err != nil {
			t.Fatalf("DisableMaintenance returned error: %v", err)
		}
	})

	t.Run("rejects maintenance during drain", func(t *testing.T) {
		mock := &mockMembershipStore{
			getNodeResult: store.Node{ID: "node-1", Draining: true},
		}
		svc := &Service{store: mock, draining: make(map[string]chan struct{})}
		err := svc.EnableMaintenance(context.Background(), "node-1", "")
		if err == nil {
			t.Fatal("expected error when node is draining")
		}
	})
}

func TestMetrics(t *testing.T) {
	var svc *Service
	m := svc.Metrics()
	if m.NodesJoinedTotal != 0 {
		t.Fatal("expected zero metrics")
	}
}
