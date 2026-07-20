package reconciliation

import (
	"context"
	"errors"

	"gamepanel/forge/internal/platform/operations"
	"gamepanel/forge/internal/platform/workloads"
)

// Candidate is a workload whose observed state has not yet reached desired
// state. Placement is supplied by the module or scheduler adapter.
type Candidate struct {
	Workload workloads.Workload
	NodeID   string
}

type CandidateSource interface {
	Pending(context.Context, int) ([]Candidate, error)
}

type Planner interface {
	Plan(context.Context, Candidate) (operations.Request, error)
}

// Service is intentionally small: it compares state and records durable
// intent. It never executes a runtime action directly.
type Service struct {
	source     CandidateSource
	planner    Planner
	dispatcher operations.Dispatcher
}

func New(source CandidateSource, planner Planner, dispatcher operations.Dispatcher) (*Service, error) {
	if source == nil || planner == nil || dispatcher == nil {
		return nil, errors.New("reconciliation source, planner, and dispatcher are required")
	}
	return &Service{source: source, planner: planner, dispatcher: dispatcher}, nil
}

func (s *Service) Reconcile(ctx context.Context, limit int) ([]operations.Operation, error) {
	if limit < 1 || limit > 100 {
		limit = 100
	}
	candidates, err := s.source.Pending(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]operations.Operation, 0, len(candidates))
	for _, candidate := range candidates {
		request, err := s.planner.Plan(ctx, candidate)
		if err != nil {
			return result, err
		}
		if request.ResourceID == "" {
			request.ResourceID = candidate.Workload.ID
		}
		if request.ResourceType == "" {
			request.ResourceType = "workload"
		}
		if request.DesiredGeneration == 0 {
			request.DesiredGeneration = candidate.Workload.DesiredGeneration
		}
		operation, err := s.dispatcher.Dispatch(ctx, request)
		if err != nil {
			return result, err
		}
		result = append(result, operation)
	}
	return result, nil
}
