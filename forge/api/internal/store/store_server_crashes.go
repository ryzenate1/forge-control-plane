package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type CrashEvent struct {
	ID            string    `json:"id"`
	ServerID      string    `json:"server_id"`
	NodeID        string    `json:"node_id"`
	ExitCode      int       `json:"exit_code"`
	OOMKilled     bool      `json:"oom_killed"`
	CleanExit     bool      `json:"clean_exit"`
	AutoRestarted bool      `json:"auto_restarted"`
	CrashCount    int       `json:"crash_count"`
	NodeState     *string   `json:"node_state,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type CreateCrashEventRequest struct {
	ServerID      string
	NodeID        string
	ExitCode      int
	OOMKilled     bool
	CleanExit     bool
	AutoRestarted bool
	CrashCount    int
	NodeState     *string
}

func (s *Store) CreateCrashEvent(ctx context.Context, req CreateCrashEventRequest) (CrashEvent, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO server_crash_events (id, server_id, node_id, exit_code, oom_killed, clean_exit, auto_restarted, crash_count, node_state)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, id, req.ServerID, req.NodeID, req.ExitCode, req.OOMKilled, req.CleanExit, req.AutoRestarted, req.CrashCount, req.NodeState)
	if err != nil {
		return CrashEvent{}, err
	}
	return s.GetCrashEvent(ctx, id)
}

func (s *Store) ListCrashEvents(ctx context.Context, serverID string, limit int) ([]CrashEvent, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, node_id, exit_code, oom_killed, clean_exit, auto_restarted, crash_count, node_state, created_at
		FROM server_crash_events
		WHERE server_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, serverID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []CrashEvent
	for rows.Next() {
		var e CrashEvent
		if err := rows.Scan(&e.ID, &e.ServerID, &e.NodeID, &e.ExitCode, &e.OOMKilled, &e.CleanExit, &e.AutoRestarted, &e.CrashCount, &e.NodeState, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *Store) GetCrashEvent(ctx context.Context, id string) (CrashEvent, error) {
	var e CrashEvent
	err := s.db.QueryRow(ctx, `
		SELECT id::text, server_id::text, node_id, exit_code, oom_killed, clean_exit, auto_restarted, crash_count, node_state, created_at
		FROM server_crash_events
		WHERE id = $1
	`, id).Scan(&e.ID, &e.ServerID, &e.NodeID, &e.ExitCode, &e.OOMKilled, &e.CleanExit, &e.AutoRestarted, &e.CrashCount, &e.NodeState, &e.CreatedAt)
	if err != nil {
		return CrashEvent{}, err
	}
	return e, nil
}

func (s *Store) CountRecentCrashes(ctx context.Context, serverID string, window time.Duration) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM server_crash_events
		WHERE server_id = $1 AND created_at >= NOW() - $2::interval
	`, serverID, window.String()).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}
