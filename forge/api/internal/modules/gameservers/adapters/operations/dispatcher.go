package operations

import (
	"context"
	"errors"
	"time"

	"gamepanel/forge/internal/platform/operations"
	"gamepanel/forge/internal/services/queue"
	"gamepanel/forge/internal/store"
)

type Queue interface {
	DispatchIdempotent(context.Context, string, queue.JobType, string, string, any, int) (*queue.Job, error)
}

type NodeResolver interface {
	GetServer(context.Context, string) (store.Server, error)
}

// Dispatcher translates the platform operation contract to the existing
// durable PostgreSQL queue. This adapter can be removed once the queue speaks
// the platform contract directly.
type Dispatcher struct {
	queue   Queue
	servers NodeResolver
}

func NewDispatcher(queue Queue, servers NodeResolver) *Dispatcher {
	return &Dispatcher{queue: queue, servers: servers}
}

func (dispatcher *Dispatcher) Dispatch(ctx context.Context, request operations.Request) (operations.Operation, error) {
	if dispatcher.queue == nil || dispatcher.servers == nil {
		return operations.Operation{}, errors.New("operation dispatcher is not configured")
	}
	server, err := dispatcher.servers.GetServer(ctx, request.ResourceID)
	if err != nil {
		return operations.Operation{}, err
	}
	job, err := dispatcher.queue.DispatchIdempotent(ctx, request.IdempotencyKey, queue.JobType(request.Kind), request.ResourceID, server.NodeID, request.Input, 0)
	if err != nil {
		return operations.Operation{}, err
	}
	return operations.Operation{
		ID: job.ID, Kind: string(job.Type), ResourceType: request.ResourceType,
		ResourceID: request.ResourceID, Status: operations.StatusQueued,
		DesiredGeneration: request.DesiredGeneration, CreatedAt: job.CreatedAt,
		UpdatedAt: time.Now().UTC(),
	}, nil
}

var _ operations.Dispatcher = (*Dispatcher)(nil)
