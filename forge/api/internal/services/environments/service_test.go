package environments

import (
	"context"
	"errors"
	"testing"
	"time"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/store"
)

type mockEnvStore struct {
	endpoints           []store.InfraEndpoint
	endpoint            store.InfraEndpoint
	getEndpointErr      error
	listEndpointErr     error
	createEndpointErr   error
	updateEndpointErr   error
	deleteEndpointErr   error
	nodes               []store.InfraEndpointNode
	nodesErr            error
	addNodeErr          error
	removeNodeErr       error
	policies            []store.InfraEndpointAccessPolicy
	policiesErr         error
	setPolicyResult     store.InfraEndpointAccessPolicy
	setPolicyErr        error
	removePolicyErr     error
	recordHealthErr     error
	healthRecords       []store.InfraEndpointHealthHistory
	healthRecordsErr    error
	getNodeResult       store.Node
	getNodeErr          error
	listServersResult   []store.Server
	listServersErr      error
	listNodesResult     []store.Node
	listNodesErr        error
}

func (m *mockEnvStore) ListInfraEndpoints(ctx context.Context) ([]store.InfraEndpoint, error) {
	return m.endpoints, m.listEndpointErr
}

func (m *mockEnvStore) GetInfraEndpoint(ctx context.Context, id string) (store.InfraEndpoint, error) {
	if m.getEndpointErr != nil {
		return store.InfraEndpoint{}, m.getEndpointErr
	}
	return m.endpoint, nil
}

func (m *mockEnvStore) CreateInfraEndpoint(ctx context.Context, req domain.CreateEndpointRequest, actorID *string) (store.InfraEndpoint, error) {
	if m.createEndpointErr != nil {
		return store.InfraEndpoint{}, m.createEndpointErr
	}
	return m.endpoint, nil
}

func (m *mockEnvStore) UpdateInfraEndpoint(ctx context.Context, id string, req domain.UpdateEndpointRequest, actorID *string) (store.InfraEndpoint, error) {
	if m.updateEndpointErr != nil {
		return store.InfraEndpoint{}, m.updateEndpointErr
	}
	return m.endpoint, nil
}

func (m *mockEnvStore) DeleteInfraEndpoint(ctx context.Context, id string, actorID *string) error {
	return m.deleteEndpointErr
}

func (m *mockEnvStore) ListInfraEndpointNodes(ctx context.Context, endpointID string) ([]store.InfraEndpointNode, error) {
	return m.nodes, m.nodesErr
}

func (m *mockEnvStore) AddNodeToInfraEndpoint(ctx context.Context, endpointID, nodeID string) error {
	return m.addNodeErr
}

func (m *mockEnvStore) RemoveNodeFromInfraEndpoint(ctx context.Context, endpointID, nodeID string) error {
	return m.removeNodeErr
}

func (m *mockEnvStore) ListInfraEndpointAccessPolicies(ctx context.Context, endpointID string) ([]store.InfraEndpointAccessPolicy, error) {
	return m.policies, m.policiesErr
}

func (m *mockEnvStore) SetInfraEndpointAccessPolicy(ctx context.Context, endpointID, principalType, principalID, role string) (store.InfraEndpointAccessPolicy, error) {
	return m.setPolicyResult, m.setPolicyErr
}

func (m *mockEnvStore) RemoveInfraEndpointAccessPolicy(ctx context.Context, endpointID, principalType, principalID string) error {
	return m.removePolicyErr
}

func (m *mockEnvStore) RecordEndpointHealth(ctx context.Context, endpointID string, status string, reachable bool, score float64, version string, containers, images, volumes int, errMsg string) error {
	return m.recordHealthErr
}

func (m *mockEnvStore) ListEndpointHealthHistory(ctx context.Context, endpointID string, limit int) ([]store.InfraEndpointHealthHistory, error) {
	return m.healthRecords, m.healthRecordsErr
}

func (m *mockEnvStore) GetNode(ctx context.Context, nodeID string) (store.Node, error) {
	return m.getNodeResult, m.getNodeErr
}

func (m *mockEnvStore) ListServersForNode(ctx context.Context, nodeID string) ([]store.Server, error) {
	return m.listServersResult, m.listServersErr
}

func (m *mockEnvStore) ListNodes(ctx context.Context) ([]store.Node, error) {
	return m.listNodesResult, m.listNodesErr
}

func TestList(t *testing.T) {
	mock := &mockEnvStore{
		endpoints: []store.InfraEndpoint{
			{ID: "ep-1", Name: "Production", EndpointType: "docker", Status: "online"},
			{ID: "ep-2", Name: "Staging", EndpointType: "kubernetes", Status: "degraded"},
		},
	}
	svc := New(mock)
	endpoints, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(endpoints))
	}
	if endpoints[0].Name != "Production" {
		t.Fatalf("expected Production, got %s", endpoints[0].Name)
	}
	if endpoints[1].EndpointType != domain.EndpointTypeK8s {
		t.Fatalf("expected kubernetes type, got %s", endpoints[1].EndpointType)
	}
}

func TestGet(t *testing.T) {
	mock := &mockEnvStore{
		endpoint: store.InfraEndpoint{ID: "ep-1", Name: "Test", EndpointType: "swarm", Status: "online"},
	}
	svc := New(mock)
	ep, err := svc.Get(context.Background(), "ep-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Name != "Test" || ep.EndpointType != domain.EndpointTypeSwarm {
		t.Fatalf("unexpected endpoint: %+v", ep)
	}
}

func TestGet_NotFound(t *testing.T) {
	mock := &mockEnvStore{getEndpointErr: errors.New("not found")}
	svc := New(mock)
	_, err := svc.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCreate_EmptyName(t *testing.T) {
	svc := New(&mockEnvStore{})
	_, err := svc.Create(context.Background(), domain.CreateEndpointRequest{Name: ""}, nil)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestCreate_Success(t *testing.T) {
	mock := &mockEnvStore{
		endpoint: store.InfraEndpoint{ID: "ep-new", Name: "NewEP", EndpointType: "docker", Status: "unknown"},
	}
	svc := New(mock)
	ep, err := svc.Create(context.Background(), domain.CreateEndpointRequest{
		Name:         "NewEP",
		EndpointType: domain.EndpointTypeDocker,
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.Name != "NewEP" {
		t.Fatalf("expected NewEP, got %s", ep.Name)
	}
}

func TestDiagnostics(t *testing.T) {
	mock := &mockEnvStore{
		endpoint: store.InfraEndpoint{ID: "ep-1", Name: "DiagEP", Reachable: true, Version: "1.0"},
		nodes: []store.InfraEndpointNode{
			{ID: "en-1", EndpointID: "ep-1", NodeID: "node-1"},
		},
		getNodeResult: store.Node{
			ID: "node-1", Name: "node-a", Status: "online",
			NodeMemoryMB: intPtr(16000), NodeDiskMB: intPtr(500000),
		},
		listServersResult: []store.Server{
			{ID: "srv-1", MemoryMB: 2048, CPUShares: 100, DiskMB: 50000},
			{ID: "srv-2", MemoryMB: 4096, CPUShares: 200, DiskMB: 100000},
		},
	}
	svc := New(mock)
	diag, err := svc.Diagnostics(context.Background(), "ep-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !diag.Reachable {
		t.Fatal("expected reachable")
	}
	if len(diag.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(diag.Nodes))
	}
	if diag.Nodes[0].ServerCount != 2 {
		t.Fatalf("expected 2 servers, got %d", diag.Nodes[0].ServerCount)
	}
	if diag.Nodes[0].AllocatedMem != 6144 {
		t.Fatalf("expected 6144 allocated mem, got %d", diag.Nodes[0].AllocatedMem)
	}
}

func TestInventory(t *testing.T) {
	mock := &mockEnvStore{
		endpoint: store.InfraEndpoint{ID: "ep-1", Name: "InvEP"},
		nodes: []store.InfraEndpointNode{
			{ID: "en-1", EndpointID: "ep-1", NodeID: "node-1"},
		},
		getNodeResult: store.Node{ID: "node-1", Name: "node-a", Status: "online"},
		listServersResult: []store.Server{
			{ID: "srv-1", MemoryMB: 1024},
			{ID: "srv-2", MemoryMB: 2048},
		},
	}
	svc := New(mock)
	inv, err := svc.Inventory(context.Background(), "ep-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.TotalServers != 2 {
		t.Fatalf("expected 2 servers, got %d", inv.TotalServers)
	}
}

func TestHealthHistory(t *testing.T) {
	records := []store.InfraEndpointHealthHistory{
		{ID: "r-1", EndpointID: "ep-1", Status: "online", HealthScore: 1.0, Reachable: true, ObservedAt: time.Now()},
		{ID: "r-2", EndpointID: "ep-1", Status: "degraded", HealthScore: 0.5, Reachable: true, ObservedAt: time.Now().Add(-time.Minute)},
	}
	mock := &mockEnvStore{healthRecords: records}
	svc := New(mock)
	history, err := svc.HealthHistory(context.Background(), "ep-1", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 records, got %d", len(history))
	}
	if history[0].Status != domain.EndpointStatusOnline {
		t.Fatalf("expected online, got %s", history[0].Status)
	}
}

func TestAccessPolicies(t *testing.T) {
	mock := &mockEnvStore{
		policies: []store.InfraEndpointAccessPolicy{
			{ID: "p-1", EndpointID: "ep-1", PrincipalType: "org", PrincipalID: "org-1", Role: "admin"},
		},
		setPolicyResult: store.InfraEndpointAccessPolicy{
			ID: "p-2", EndpointID: "ep-1", PrincipalType: "user", PrincipalID: "user-1", Role: "viewer",
		},
	}
	svc := New(mock)

	policies, err := svc.ListAccessPolicies(context.Background(), "ep-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(policies))
	}

	policy, err := svc.SetAccessPolicy(context.Background(), "ep-1", "user", "user-1", "viewer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if policy.PrincipalID != "user-1" {
		t.Fatalf("expected user-1, got %s", policy.PrincipalID)
	}

	if err := svc.RemoveAccessPolicy(context.Background(), "ep-1", "user", "user-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckEndpointAccess_AdminBypass(t *testing.T) {
	svc := New(&mockEnvStore{})
	err := svc.CheckEndpointAccess(context.Background(), "ep-1", "admin-user", "admin", nil)
	if err != nil {
		t.Fatalf("admin should always pass: %v", err)
	}
}

func TestCheckEndpointAccess_Denied(t *testing.T) {
	mock := &mockEnvStore{
		policies: []store.InfraEndpointAccessPolicy{
			{EndpointID: "ep-1", PrincipalType: "org", PrincipalID: "org-other", Role: "member"},
		},
	}
	svc := New(mock)
	err := svc.CheckEndpointAccess(context.Background(), "ep-1", "user-1", "user", []string{})
	if err == nil {
		t.Fatal("expected access denied error")
	}
}

func TestCheckEndpointAccess_AllowedByUserPolicy(t *testing.T) {
	mock := &mockEnvStore{
		policies: []store.InfraEndpointAccessPolicy{
			{EndpointID: "ep-1", PrincipalType: "user", PrincipalID: "user-1", Role: "viewer"},
		},
	}
	svc := New(mock)
	err := svc.CheckEndpointAccess(context.Background(), "ep-1", "user-1", "user", nil)
	if err != nil {
		t.Fatalf("expected access allowed: %v", err)
	}
}

func TestCheckEndpointAccess_AllowedByOrgPolicy(t *testing.T) {
	mock := &mockEnvStore{
		policies: []store.InfraEndpointAccessPolicy{
			{EndpointID: "ep-1", PrincipalType: "org", PrincipalID: "org-a", Role: "admin"},
		},
	}
	svc := New(mock)
	err := svc.CheckEndpointAccess(context.Background(), "ep-1", "user-1", "user", []string{"org-a"})
	if err != nil {
		t.Fatalf("expected access allowed via org: %v", err)
	}
}

func TestRecordHealth(t *testing.T) {
	mock := &mockEnvStore{}
	svc := New(mock)
	err := svc.RecordHealth(context.Background(), "ep-1", "online", true, 1.0, "1.0", 10, 5, 3, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDelete(t *testing.T) {
	mock := &mockEnvStore{}
	svc := New(mock)
	err := svc.Delete(context.Background(), "ep-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddRemoveNode(t *testing.T) {
	mock := &mockEnvStore{}
	svc := New(mock)
	if err := svc.AddNode(context.Background(), "ep-1", "node-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := svc.RemoveNode(context.Background(), "ep-1", "node-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func intPtr(v int) *int { return &v }
