package operations

import (
	"context"
	"encoding/json"
	"time"
)

type Status string

const (
	StatusQueued      Status = "queued"
	StatusRunning     Status = "running"
	StatusWaiting     Status = "waiting"
	StatusRetrying    Status = "retrying"
	StatusCancelling  Status = "cancelling"
	StatusRollingBack Status = "rolling_back"
	StatusSucceeded   Status = "succeeded"
	StatusFailed      Status = "failed"
	StatusCancelled   Status = "cancelled"
)

type Request struct {
	Kind              string
	ResourceType      string
	ResourceID        string
	IdempotencyKey    string
	DesiredGeneration int64
	Input             json.RawMessage
}

type Operation struct {
	ID                 string    `json:"id"`
	Kind               string    `json:"kind"`
	ResourceType       string    `json:"resourceType"`
	ResourceID         string    `json:"resourceId"`
	Status             Status    `json:"status"`
	DesiredGeneration  int64     `json:"desiredGeneration"`
	ObservedGeneration int64     `json:"observedGeneration"`
	Error              string    `json:"error,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type Dispatcher interface {
	Dispatch(context.Context, Request) (Operation, error)
}
type Driver interface {
	Kind() string
	Execute(context.Context, Operation) error
}
type Repository interface {
	Create(context.Context, Request) (Operation, error)
	Get(context.Context, string) (Operation, error)
	UpdateStatus(context.Context, string, Status, string) error
}
