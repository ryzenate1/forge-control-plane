package beacon

import (
	"context"
	"testing"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/modules/apphosting/ports"
	"gamepanel/forge/internal/store"
)

type nodesStub struct{}

func (nodesStub) GetNodeDaemonCredential(context.Context, string) (string, error) {
	return "credential", nil
}
func (nodesStub) GetNode(context.Context, string) (store.Node, error) {
	return store.Node{BaseURL: "https://beacon.example"}, nil
}

type unavailableNodesStub struct{ node store.Node }

func (s unavailableNodesStub) GetNodeDaemonCredential(context.Context, string) (string, error) {
	return "credential", nil
}
func (s unavailableNodesStub) GetNode(context.Context, string) (store.Node, error) {
	return s.node, nil
}

type clientStub struct{ create daemon.CreateRequest }

func (s *clientStub) CreateServer(_ context.Context, baseURL, credential string, request daemon.CreateRequest) (daemon.CreateResponse, error) {
	if baseURL != "https://beacon.example" || credential != "credential" {
		return daemon.CreateResponse{}, testingError("unexpected Beacon credentials")
	}
	s.create = request
	return daemon.CreateResponse{Accepted: true}, nil
}
func (s *clientStub) DeleteServer(context.Context, string, string, string) (daemon.PowerResponse, error) {
	return daemon.PowerResponse{}, nil
}

type testingError string

func (e testingError) Error() string { return string(e) }

func TestRuntimeDeploysRunningContainerWithStableEnvironment(t *testing.T) {
	client := &clientStub{}
	runtime, err := NewRuntime(nodesStub{}, client)
	if err != nil {
		t.Fatal(err)
	}
	err = runtime.Deploy(context.Background(), ports.DeploymentRequest{WorkloadID: "workload", NodeID: "node", Image: "busybox", Environment: map[string]string{"Z": "last", "A": "first"}})
	if err != nil {
		t.Fatal(err)
	}
	if !client.create.Start || client.create.NetworkName != "gamepanel" {
		t.Fatalf("expected started managed-network request, got %#v", client.create)
	}
	if got, want := client.create.Env, []string{"A=first", "Z=last"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected sorted environment %v, got %v", want, got)
	}
}

func TestRuntimeRejectsUnavailableNodeBeforeCallingBeacon(t *testing.T) {
	client := &clientStub{}
	runtime, err := NewRuntime(unavailableNodesStub{node: store.Node{BaseURL: "https://beacon.example", Maintenance: true}}, client)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Deploy(context.Background(), ports.DeploymentRequest{WorkloadID: "workload", NodeID: "node", Image: "busybox"}); err == nil {
		t.Fatal("expected unavailable node error")
	}
	if client.create.ServerID != "" {
		t.Fatal("Beacon must not receive a request for an unavailable node")
	}
}
