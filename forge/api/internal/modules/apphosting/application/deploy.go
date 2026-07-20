package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"gamepanel/forge/internal/modules/apphosting/ports"
	"gamepanel/forge/internal/platform/workloads"
)

// DeploymentService owns the Forge-side execution sequence. It records a
// node-specific instance before calling Beacon and always leaves an observed
// failure state if the runtime action cannot complete.
type DeploymentService struct {
	repository ports.DeploymentRepository
	runtime    ports.Runtime
}

func NewDeploymentService(repository ports.DeploymentRepository, runtime ports.Runtime) (*DeploymentService, error) {
	if repository == nil || runtime == nil {
		return nil, errors.New("deployment repository and runtime are required")
	}
	return &DeploymentService{repository: repository, runtime: runtime}, nil
}

func (s *DeploymentService) Deploy(ctx context.Context, workloadID string, input DeployInput) error {
	if input.DesiredGeneration < 1 || input.Application.NodeID == "" {
		return errors.New("application deployment requires a node and desired generation")
	}
	if err := input.Application.Validate(); err != nil {
		return err
	}
	if err := input.Application.DeployableNow(); err != nil {
		return err
	}
	revision, err := s.repository.CurrentWorkloadRevision(ctx, workloadID)
	if err != nil {
		return fmt.Errorf("load current workload revision: %w", err)
	}
	instance, err := s.repository.CreateWorkloadInstance(ctx, ports.CreateWorkloadInstanceInput{
		WorkloadID: workloadID, RevisionID: revision.ID, NodeID: input.Application.NodeID,
		DesiredState: workloads.DesiredState("running"), ObservedState: workloads.ObservedState("provisioning"),
	})
	if err != nil {
		return fmt.Errorf("record workload instance: %w", err)
	}
	request := ports.DeploymentRequest{WorkloadID: workloadID, NodeID: input.Application.NodeID, Image: input.Application.Image, Command: input.Application.Command, Environment: input.Application.Environment, MemoryMB: input.Application.MemoryMB, CPUPercent: input.Application.CPUPercent, DiskMB: input.Application.DiskMB, DesiredGeneration: input.DesiredGeneration}
	if err := s.runtime.Deploy(ctx, request); err != nil {
		_ = s.runtime.Delete(ctx, workloadID, input.Application.NodeID)
		_ = s.repository.SetWorkloadInstanceObservedState(ctx, instance.ID, workloads.ObservedState("failed"))
		_ = s.recordObservation(ctx, workloadID, input.DesiredGeneration, "failed", err.Error())
		return fmt.Errorf("deploy application runtime: %w", err)
	}
	if err := s.repository.SetWorkloadInstanceObservedState(ctx, instance.ID, workloads.ObservedState("running")); err != nil {
		return err
	}
	return s.recordObservation(ctx, workloadID, input.DesiredGeneration, "running", "")
}

func (s *DeploymentService) DeployPayload(ctx context.Context, workloadID string, payload []byte) error {
	var input DeployInput
	if err := json.Unmarshal(payload, &input); err != nil {
		return fmt.Errorf("decode application deployment payload: %w", err)
	}
	return s.Deploy(ctx, workloadID, input)
}

func (s *DeploymentService) recordObservation(ctx context.Context, workloadID string, generation int64, state, detail string) error {
	payload, err := json.Marshal(map[string]string{"source": "application.deploy", "detail": detail})
	if err != nil {
		return err
	}
	return s.repository.RecordWorkloadObservation(ctx, workloadID, generation, workloads.ObservedState(state), payload)
}
