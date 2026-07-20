// Package operations translates app-hosting intent into Forge's durable
// PostgreSQL worker. It is a compatibility adapter while the queue evolves
// into the canonical operation engine.
package operations

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	apphostingapplication "gamepanel/forge/internal/modules/apphosting/application"
	"gamepanel/forge/internal/platform/operations"
	"gamepanel/forge/internal/services/queue"
)

type Queue interface {
	DispatchResourceIdempotent(context.Context, string, queue.JobType, string, string, string, any, int) (*queue.Job, error)
}

type Dispatcher struct{ queue Queue }

func NewDispatcher(queue Queue) *Dispatcher { return &Dispatcher{queue: queue} }

func (d *Dispatcher) Dispatch(ctx context.Context, request operations.Request) (operations.Operation, error) {
	if d == nil || d.queue == nil {
		return operations.Operation{}, errors.New("application operation dispatcher is not configured")
	}
	if request.Kind != string(queue.JobApplicationDeploy) || request.ResourceType != "workload" {
		return operations.Operation{}, errors.New("unsupported application operation")
	}
	var payload apphostingapplication.DeployInput
	if err := json.Unmarshal(request.Input, &payload); err != nil {
		return operations.Operation{}, err
	}
	if payload.Application.NodeID == "" {
		return operations.Operation{}, errors.New("application deployment node is required")
	}
	job, err := d.queue.DispatchResourceIdempotent(ctx, request.IdempotencyKey, queue.JobApplicationDeploy, request.ResourceType, request.ResourceID, payload.Application.NodeID, payload, 0)
	if err != nil {
		return operations.Operation{}, err
	}
	return operations.Operation{ID: job.ID, Kind: string(job.Type), ResourceType: job.ResourceType, ResourceID: request.ResourceID, Status: operations.StatusQueued, DesiredGeneration: request.DesiredGeneration, CreatedAt: job.CreatedAt, UpdatedAt: time.Now().UTC()}, nil
}

var _ operations.Dispatcher = (*Dispatcher)(nil)
