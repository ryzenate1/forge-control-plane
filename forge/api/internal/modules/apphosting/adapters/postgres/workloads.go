package postgres

import (
	"context"
	"encoding/json"

	"gamepanel/forge/internal/modules/apphosting/ports"
	"gamepanel/forge/internal/platform/workloads"
	"gamepanel/forge/internal/store"
)

type Store interface {
	CreateWorkload(context.Context, store.CreateWorkloadRequest) (workloads.Workload, workloads.Revision, error)
	CurrentWorkloadRevision(context.Context, string) (workloads.Revision, error)
	CreateWorkloadInstance(context.Context, store.CreateWorkloadInstanceRequest) (workloads.Instance, error)
	SetWorkloadInstanceObservedState(context.Context, string, workloads.ObservedState) error
	RecordWorkloadObservation(context.Context, string, int64, workloads.ObservedState, json.RawMessage) error
}

type Workloads struct{ store Store }

func NewWorkloads(store Store) *Workloads { return &Workloads{store: store} }

func (w *Workloads) Create(ctx context.Context, input ports.CreateWorkloadInput) (workloads.Workload, error) {
	workload, _, err := w.store.CreateWorkload(ctx, store.CreateWorkloadRequest{EnvironmentID: input.EnvironmentID, Kind: input.Kind, Name: input.Name, DesiredState: input.DesiredState, Spec: input.Spec})
	return workload, err
}

func (w *Workloads) CurrentWorkloadRevision(ctx context.Context, workloadID string) (workloads.Revision, error) {
	return w.store.CurrentWorkloadRevision(ctx, workloadID)
}

func (w *Workloads) CreateWorkloadInstance(ctx context.Context, input ports.CreateWorkloadInstanceInput) (workloads.Instance, error) {
	return w.store.CreateWorkloadInstance(ctx, store.CreateWorkloadInstanceRequest{WorkloadID: input.WorkloadID, RevisionID: input.RevisionID, NodeID: input.NodeID, DesiredState: input.DesiredState, ObservedState: input.ObservedState})
}

func (w *Workloads) SetWorkloadInstanceObservedState(ctx context.Context, instanceID string, state workloads.ObservedState) error {
	return w.store.SetWorkloadInstanceObservedState(ctx, instanceID, state)
}

func (w *Workloads) RecordWorkloadObservation(ctx context.Context, workloadID string, generation int64, state workloads.ObservedState, details []byte) error {
	return w.store.RecordWorkloadObservation(ctx, workloadID, generation, state, json.RawMessage(details))
}

var _ ports.Workloads = (*Workloads)(nil)
var _ ports.DeploymentRepository = (*Workloads)(nil)
