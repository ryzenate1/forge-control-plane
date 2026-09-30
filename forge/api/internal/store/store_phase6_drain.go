package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// Phase 6 durable drain states (migration 191). The drain driver writes a row
// per node so progress survives process restarts and can be surfaced in the
// drain progress UI.

// DrainProgressStep is one named step with its current state.
type DrainProgressStep struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

// DrainProgress is the JSONB progress ledger attached to every drain state.
type DrainProgress struct {
	State     string              `json:"state"`
	Total     int                 `json:"total"`
	Remaining int                 `json:"remaining"`
	Current   string              `json:"current"`
	Steps     []DrainProgressStep `json:"steps"`
}

func (p DrainProgress) asJSON() []byte {
	if p.Steps == nil {
		p.Steps = []DrainProgressStep{}
	}
	b, _ := json.Marshal(p)
	return b
}

type DrainState struct {
	NodeID       string        `json:"nodeId"`
	PlanID       string        `json:"planId,omitempty"`
	Status       string        `json:"status"`
	DesiredFinal bool          `json:"desiredFinal"`
	StartedAt    time.Time     `json:"startedAt"`
	CompletedAt  *time.Time    `json:"completedAt,omitempty"`
	Progress     DrainProgress `json:"progress"`
	UpdatedAt    time.Time     `json:"updatedAt"`
}

// UpsertDrainState inserts or replaces the durable drain state for a node.
func (s *Store) UpsertDrainState(ctx context.Context, state DrainState) (DrainState, error) {
	if _, err := s.db.Exec(ctx, `
		INSERT INTO drain_states (node_id, plan_id, status, desired_final, started_at, completed_at, progress, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, now())
		ON CONFLICT (node_id) DO UPDATE SET
			plan_id = EXCLUDED.plan_id,
			status = EXCLUDED.status,
			desired_final = EXCLUDED.desired_final,
			started_at = EXCLUDED.started_at,
			completed_at = EXCLUDED.completed_at,
			progress = EXCLUDED.progress,
			updated_at = now()
	`, state.NodeID, drainNullIfEmpty(state.PlanID), state.Status, state.DesiredFinal, state.StartedAt, state.CompletedAt, string(state.Progress.asJSON())); err != nil {
		return DrainState{}, err
	}
	return s.GetDrainState(ctx, state.NodeID)
}

// UpdateDrainProgress persists a progress ledger update for a node.
func (s *Store) UpdateDrainProgress(ctx context.Context, nodeID string, progress DrainProgress) (DrainState, error) {
	if _, err := s.db.Exec(ctx, `
		UPDATE drain_states SET progress = $2::jsonb, updated_at = now() WHERE node_id = $1
	`, nodeID, string(progress.asJSON())); err != nil {
		return DrainState{}, err
	}
	return s.GetDrainState(ctx, nodeID)
}

// GetDrainState returns the durable drain state for a node.
func (s *Store) GetDrainState(ctx context.Context, nodeID string) (DrainState, error) {
	var state DrainState
	var planID sql.NullString
	var progressRaw string
	err := s.db.QueryRow(ctx, `
		SELECT node_id::text, plan_id, status, desired_final, started_at, completed_at, progress::text, updated_at
		FROM drain_states WHERE node_id = $1
	`, nodeID).Scan(
		&state.NodeID, &planID, &state.Status, &state.DesiredFinal, &state.StartedAt, &state.CompletedAt, &progressRaw, &state.UpdatedAt,
	)
	if err != nil {
		return DrainState{}, err
	}
	if planID.Valid {
		state.PlanID = planID.String
	}
	if progressRaw != "" {
		_ = json.Unmarshal([]byte(progressRaw), &state.Progress)
	}
	if state.Progress.Steps == nil {
		state.Progress.Steps = []DrainProgressStep{}
	}
	return state, nil
}

// ListDrainStates returns every persisted drain state, newest first.
func (s *Store) ListDrainStates(ctx context.Context) ([]DrainState, error) {
	rows, err := s.db.Query(ctx, `
		SELECT node_id::text, plan_id, status, desired_final, started_at, completed_at, progress::text, updated_at
		FROM drain_states ORDER BY started_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	states := []DrainState{}
	for rows.Next() {
		var state DrainState
		var planID sql.NullString
		var progressRaw string
		if err := rows.Scan(&state.NodeID, &planID, &state.Status, &state.DesiredFinal, &state.StartedAt, &state.CompletedAt, &progressRaw, &state.UpdatedAt); err != nil {
			return nil, err
		}
		if planID.Valid {
			state.PlanID = planID.String
		}
		if progressRaw != "" {
			_ = json.Unmarshal([]byte(progressRaw), &state.Progress)
		}
		if state.Progress.Steps == nil {
			state.Progress.Steps = []DrainProgressStep{}
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

func drainNullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}