package application

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"gamepanel/forge/internal/modules/apphosting/domain"
	"gamepanel/forge/internal/modules/apphosting/ports"
	"gamepanel/forge/internal/platform/operations"
	"gamepanel/forge/internal/platform/workloads"
)

type Service struct {
	workloads  ports.Workloads
	operations ports.Operations
}

func New(workloads ports.Workloads, operations ports.Operations) (*Service, error) {
	if workloads == nil || operations == nil {
		return nil, errors.New("workload repository and operation dispatcher are required")
	}
	return &Service{workloads: workloads, operations: operations}, nil
}

func (s *Service) Create(ctx context.Context, app domain.Application) (workloads.Workload, operations.Operation, error) {
	if err := app.Validate(); err != nil {
		return workloads.Workload{}, operations.Operation{}, err
	}
	if app.Deployment == "" {
		app.Deployment = domain.StrategyRolling
	}
	spec, err := json.Marshal(app)
	if err != nil {
		return workloads.Workload{}, operations.Operation{}, err
	}
	workload, err := s.workloads.Create(ctx, ports.CreateWorkloadInput{EnvironmentID: app.EnvironmentID, Kind: workloads.KindApplication, Name: app.Name, DesiredState: "running", Spec: spec})
	if err != nil {
		return workloads.Workload{}, operations.Operation{}, err
	}
	operation, err := s.operations.Dispatch(ctx, operations.Request{Kind: "application.deploy", ResourceType: "workload", ResourceID: workload.ID, IdempotencyKey: workload.ID + ":" + strconv.FormatInt(workload.DesiredGeneration, 10), DesiredGeneration: workload.DesiredGeneration, Input: spec})
	if err != nil {
		return workloads.Workload{}, operations.Operation{}, err
	}
	return workload, operation, nil
}
