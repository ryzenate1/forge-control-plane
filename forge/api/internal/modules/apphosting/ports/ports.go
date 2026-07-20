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

type DeploymentRequest struct {
	WorkloadID        string
	NodeID            string
	Image             string
	Command           []string
	Environment       map[string]string
	MemoryMB          int64
	CPUPercent        int64
	DiskMB            int64
	DesiredGeneration int64
}

type BuildRequest struct {
	WorkloadID        string
	NodeID            string
	RepositoryURL     string
	Branch            string
	BaseDirectory     string
	DockerfilePath    string
	BuildArgs         map[string]string
	DesiredGeneration int64
}

type BuildResult struct {
	Image  string
	Commit string
}

type DeploymentRepository interface {
	CurrentWorkloadRevision(context.Context, string) (workloads.Revision, error)
	CreateWorkloadInstance(context.Context, CreateWorkloadInstanceInput) (workloads.Instance, error)
	SetWorkloadInstanceObservedState(context.Context, string, workloads.ObservedState) error
	RecordWorkloadObservation(context.Context, string, int64, workloads.ObservedState, []byte) error
}

type CreateWorkloadInstanceInput struct {
	WorkloadID    string
	RevisionID    string
	NodeID        string
	DesiredState  workloads.DesiredState
	ObservedState workloads.ObservedState
}

type Runtime interface {
	Build(context.Context, BuildRequest) (BuildResult, error)
	Deploy(context.Context, DeploymentRequest) error
	Delete(context.Context, string, string) error
}
