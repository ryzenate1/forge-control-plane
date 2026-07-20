package postgres

import (
	"context"

	"gamepanel/forge/internal/modules/apphosting/ports"
	"gamepanel/forge/internal/platform/workloads"
	"gamepanel/forge/internal/store"
)

type Store interface {
	CreateWorkload(context.Context, store.CreateWorkloadRequest) (workloads.Workload, workloads.Revision, error)
}

type Workloads struct{ store Store }

func NewWorkloads(store Store) *Workloads { return &Workloads{store: store} }

func (w *Workloads) Create(ctx context.Context, input ports.CreateWorkloadInput) (workloads.Workload, error) {
	workload, _, err := w.store.CreateWorkload(ctx, store.CreateWorkloadRequest{EnvironmentID: input.EnvironmentID, Kind: input.Kind, Name: input.Name, DesiredState: input.DesiredState, Spec: input.Spec})
	return workload, err
}

var _ ports.Workloads = (*Workloads)(nil)
