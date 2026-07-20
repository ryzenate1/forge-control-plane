package application

import (
	"context"
	"errors"
	"testing"

	"gamepanel/forge/internal/modules/apphosting/domain"
	"gamepanel/forge/internal/modules/apphosting/ports"
	"gamepanel/forge/internal/platform/workloads"
)

type deploymentRepositoryStub struct {
	instanceStates []workloads.ObservedState
	observations   []workloads.ObservedState
}

func (s *deploymentRepositoryStub) CurrentWorkloadRevision(context.Context, string) (workloads.Revision, error) {
	return workloads.Revision{ID: "revision"}, nil
}

func (s *deploymentRepositoryStub) CreateWorkloadInstance(_ context.Context, input ports.CreateWorkloadInstanceInput) (workloads.Instance, error) {
	return workloads.Instance{ID: "instance", WorkloadID: input.WorkloadID, RevisionID: input.RevisionID, NodeID: input.NodeID, ObservedState: input.ObservedState}, nil
}

func (s *deploymentRepositoryStub) SetWorkloadInstanceObservedState(_ context.Context, _ string, state workloads.ObservedState) error {
	s.instanceStates = append(s.instanceStates, state)
	return nil
}

func (s *deploymentRepositoryStub) RecordWorkloadObservation(_ context.Context, _ string, _ int64, state workloads.ObservedState, _ []byte) error {
	s.observations = append(s.observations, state)
	return nil
}

type runtimeStub struct {
	deployRequest ports.DeploymentRequest
	deployErr     error
	deleted       bool
}

func (s *runtimeStub) Deploy(_ context.Context, request ports.DeploymentRequest) error {
	s.deployRequest = request
	return s.deployErr
}

func (s *runtimeStub) Delete(context.Context, string, string) error {
	s.deleted = true
	return nil
}

func deployInput() DeployInput {
	return DeployInput{DesiredGeneration: 1, Application: domain.Application{
		EnvironmentID: "environment", NodeID: "node", Name: "api", Source: domain.SourceImage,
		Image: "ghcr.io/example/api:1", Deployment: domain.StrategyRolling,
		Environment: map[string]string{"PORT": "8080"}, MemoryMB: 512, CPUPercent: 100,
	}}
}

func TestDeploymentServiceDeploysAndRecordsObservedState(t *testing.T) {
	repository := &deploymentRepositoryStub{}
	runtime := &runtimeStub{}
	service, err := NewDeploymentService(repository, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := service.Deploy(context.Background(), "workload", deployInput()); err != nil {
		t.Fatal(err)
	}
	if runtime.deployRequest.WorkloadID != "workload" || runtime.deployRequest.NodeID != "node" || runtime.deployRequest.Image == "" {
		t.Fatalf("unexpected runtime request: %#v", runtime.deployRequest)
	}
	if runtime.deleted || len(repository.instanceStates) != 1 || repository.instanceStates[0] != workloads.ObservedState("running") {
		t.Fatalf("expected running instance without cleanup, got states=%v deleted=%v", repository.instanceStates, runtime.deleted)
	}
	if len(repository.observations) != 1 || repository.observations[0] != workloads.ObservedState("running") {
		t.Fatalf("expected running workload observation, got %v", repository.observations)
	}
}

func TestDeploymentServiceCleansUpAndRecordsFailure(t *testing.T) {
	repository := &deploymentRepositoryStub{}
	runtime := &runtimeStub{deployErr: errors.New("Beacon unavailable")}
	service, err := NewDeploymentService(repository, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := service.Deploy(context.Background(), "workload", deployInput()); err == nil {
		t.Fatal("expected deployment failure")
	}
	if !runtime.deleted || len(repository.instanceStates) != 1 || repository.instanceStates[0] != workloads.ObservedState("failed") {
		t.Fatalf("expected failed cleanup state, got states=%v deleted=%v", repository.instanceStates, runtime.deleted)
	}
	if len(repository.observations) != 1 || repository.observations[0] != workloads.ObservedState("failed") {
		t.Fatalf("expected failed workload observation, got %v", repository.observations)
	}
}
