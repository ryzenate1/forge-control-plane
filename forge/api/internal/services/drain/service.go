package drain

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"
)

// Step names rendered by the drain progress UI, in execution order.
const (
	StepTrafficWithdrawal = "traffic-withdrawal"
	StepEvacuationPlan    = "evacuation-plan"
	StepMigrateServers    = "migrate-servers"
	StepComplete          = "complete"
)

// Statuses persisted in drain_states.status.
const (
	StatusDraining  = "draining"
	StatusDrained   = "drained"
	StatusCancelled = "cancelled"
	StatusFailed    = "failed"
)

var stepNames = []string{StepTrafficWithdrawal, StepEvacuationPlan, StepMigrateServers, StepComplete}

// Service persists per-node drain lifecycle so progress survives restarts.
// It wraps (not replaces) the in-memory drain state owned by
// clustermembership.Service.
type Service struct {
	store  *store.Store
	logger *slog.Logger
}

func New(st *store.Store, logger *slog.Logger) *Service {
	return &Service{store: st, logger: logger}
}

// BeginDrain records the durable state for a drain that has just started.
// desiredFinal marks drains scheduled for a terminal state (leave / scale-in).
func (s *Service) BeginDrain(ctx context.Context, nodeID string, desiredFinal bool, planID string) (store.DrainState, error) {
	if s == nil || s.store == nil {
		return store.DrainState{}, errors.New("drain service unavailable")
	}
	progress := store.DrainProgress{
		State:     StatusDraining,
		Total:     0,
		Remaining: 0,
		Current:   StepTrafficWithdrawal,
		Steps:     newSteps(0, StepTrafficWithdrawal),
	}
	return s.store.UpsertDrainState(ctx, store.DrainState{
		NodeID:       nodeID,
		PlanID:       planID,
		Status:       StatusDraining,
		DesiredFinal: desiredFinal,
		StartedAt:    time.Now().UTC(),
		Progress:     progress,
	})
}

// ProgressDrain advances the persisted progress ledger. stage is one of the
// Step* constants; remaining is the number of workloads still on the node.
func (s *Service) ProgressDrain(ctx context.Context, nodeID, stage string, remaining int) (store.DrainState, error) {
	if s == nil || s.store == nil {
		return store.DrainState{}, errors.New("drain service unavailable")
	}
	if _, err := s.store.GetDrainState(ctx, nodeID); err != nil {
		return store.DrainState{}, err
	}
	progress := store.DrainProgress{
		State:     StatusDraining,
		Remaining: remaining,
		Current:   stage,
		Steps:     newSteps(remaining, stage),
	}
	return s.store.UpdateDrainProgress(ctx, nodeID, progress)
}

// EndDrain closes the durable record. ok=true records StatusDrained.
func (s *Service) EndDrain(ctx context.Context, nodeID string, ok bool) (store.DrainState, error) {
	if s == nil || s.store == nil {
		return store.DrainState{}, errors.New("drain service unavailable")
	}
	state, err := s.store.GetDrainState(ctx, nodeID)
	if err != nil {
		return store.DrainState{}, err
	}
	status := StatusDrained
	if !ok {
		status = StatusFailed
	}
	state.Status = status
	now := time.Now().UTC()
	state.CompletedAt = &now
	state.Progress.State = status
	state.Progress.Remaining = 0
	state.Progress.Current = StepComplete
	state.Progress.Steps = newSteps(0, StepComplete)
	if ok {
		state.Progress.Steps[len(state.Progress.Steps)-1].State = "done"
	}
	return s.store.UpsertDrainState(ctx, state)
}

// CancelDrainState records a cancelled drain and leaves the node drainable.
func (s *Service) CancelDrainState(ctx context.Context, nodeID string) (store.DrainState, error) {
	if s == nil || s.store == nil {
		return store.DrainState{}, errors.New("drain service unavailable")
	}
	state, err := s.store.GetDrainState(ctx, nodeID)
	if err != nil {
		return store.DrainState{}, err
	}
	state.Status = StatusCancelled
	now := time.Now().UTC()
	state.CompletedAt = &now
	state.Progress.State = StatusCancelled
	state.Progress.Remaining = 0
	state.Progress.Current = StepTrafficWithdrawal
	state.Progress.Steps = newSteps(0, StepTrafficWithdrawal)
	return s.store.UpsertDrainState(ctx, state)
}

// Status returns the durable drain state; a zero DrainState with nil error
// means no drain has ever been recorded for the node.
func (s *Service) Status(ctx context.Context, nodeID string) (store.DrainState, error) {
	if s == nil || s.store == nil {
		return store.DrainState{}, errors.New("drain service unavailable")
	}
	state, err := s.store.GetDrainState(ctx, nodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.DrainState{}, nil
	}
	return state, err
}

// List returns every recorded drain, newest first.
func (s *Service) List(ctx context.Context) ([]store.DrainState, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("drain service unavailable")
	}
	return s.store.ListDrainStates(ctx)
}

// Subscriber returns an events.Subscriber that mirrors membership/evacuation
// events into the durable ledger. Wire it into cfg.EventRegistry in main so
// drains started outside the phase 6 routes still record progress.
func (s *Service) Subscriber() events.Subscriber {
	return events.HandlerFunc(func(ctx context.Context, envelope events.Envelope) error {
		if s == nil || s.store == nil {
			return nil
		}
		nodeID := envelope.ResourceID
		if nodeID == "" {
			return nil
		}
		switch envelope.Type {
		case events.EventNodeDrainingStarted:
			if _, err := s.store.GetDrainState(ctx, nodeID); err != nil {
				planID, _ := envelope.Payload["planId"].(string)
				_, _ = s.BeginDrain(ctx, nodeID, false, planID)
			}
		case events.EventEvacuationPlanCreated:
			total, _ := envelope.Payload["total"].(int)
			if total == 0 {
				total, _ = envelope.Payload["items"].(int)
			}
			_, _ = s.ProgressDrain(ctx, nodeID, StepEvacuationPlan, total)
		case events.EventEvacuationPlanFailed:
			_, _ = s.ProgressDrain(ctx, nodeID, StepEvacuationPlan, 1)
			_, _ = s.EndDrain(ctx, nodeID, false)
		case events.EventNodeDrainingCompleted:
			_, _ = s.EndDrain(ctx, nodeID, true)
		}
		return nil
	})
}

func newSteps(remaining int, current string) []store.DrainProgressStep {
	steps := make([]store.DrainProgressStep, 0, len(stepNames))
	seen := false
	for _, name := range stepNames {
		state := "pending"
		switch {
		case name == current && remaining > 0:
			state = "active"
		case name == current:
			state = "done"
		}
		if !seen && name != current {
			state = "done"
		}
		if name == current {
			seen = true
		}
		if name == StepComplete && state == "pending" && remaining == 0 {
			state = "done"
		}
		steps = append(steps, store.DrainProgressStep{Name: name, State: state})
	}
	return steps
}
