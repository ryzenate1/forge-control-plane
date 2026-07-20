package ports

import (
	"context"

	"gamepanel/forge/internal/platform/operations"
	"gamepanel/forge/internal/platform/workloads"
)

type CreateWorkloadInput struct {
	EnvironmentID string
	Kind          workloads.Kind
	Name          string
	DesiredState  workloads.DesiredState
	Spec          []byte
}

type Workloads interface {
	Create(context.Context, CreateWorkloadInput) (workloads.Workload, error)
}

type Operations interface {
	Dispatch(context.Context, operations.Request) (operations.Operation, error)
}
