package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrCrashEventUnreadable reports that a crash event was durably inserted but
// could not be read back afterwards. It is deliberately distinct from an insert
// failure: the row exists, so a caller must not retry the write, but the
// returned CrashEvent carries only the fields known without reading (ID and
// ServerID) and must not be treated as the full stored row.
var ErrCrashEventUnreadable = errors.New("crash event recorded but could not be read back")

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

// CreateCrashEvent is the single writer for server_crash_events. Both the
// background crash detector and the node-facing /api/remote crash report go
// through it.
//
// It is a plain VALUES insert, and server_crash_events.server_id and node_id
// both carry foreign keys, so a crash reported against a server or node that
// no longer exists fails loudly. The /api/remote handler previously ran its own
// inline copy of this write shaped as
// `INSERT ... SELECT $1, s.id, s.node_id, ... FROM servers s WHERE s.id = $2`,
// which matched no rows for an unknown server while Exec returned no error —
// so the handler answered {"ok": true, "id": ...} for an event it had not
// recorded. Keep the write here and keep it a VALUES insert.
func (s *Store) CreateCrashEvent(ctx context.Context, req CreateCrashEventRequest) (CrashEvent, error) {
	if s.db == nil {
		return CrashEvent{}, errors.New("no database connection")
	}
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO server_crash_events (id, server_id, node_id, exit_code, oom_killed, clean_exit, auto_restarted, crash_count, node_state)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, id, req.ServerID, req.NodeID, req.ExitCode, req.OOMKilled, req.CleanExit, req.AutoRestarted, req.CrashCount, req.NodeState)
	if err != nil {
		return CrashEvent{}, fmt.Errorf("insert crash event for server %s: %w", req.ServerID, err)
	}
	event, err := s.GetCrashEvent(ctx, id)
	if err != nil {
		// The insert already committed. Reporting a plain failure here would
		// invite the caller to retry and record the same crash twice, so the
		// durable part of the outcome is returned alongside a distinguishable
		// error rather than being thrown away.
		return CrashEvent{ID: id, ServerID: req.ServerID}, fmt.Errorf("%w: %s: %v", ErrCrashEventUnreadable, id, err)
	}
	return event, nil
}

func (s *Store) ListCrashEvents(ctx context.Context, serverID string, limit int) ([]CrashEvent, error) {
	if s.db == nil {
		return nil, errors.New("no database connection")
	}
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
	if s.db == nil {
		return CrashEvent{}, errors.New("no database connection")
	}
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

// CountRecentCrashes returns the number of crash events recorded for a server
// inside the window. A query failure returns an error rather than 0: callers
// use this to decide whether a server is crash-looping, and "I could not count"
// must not read as "it has not crashed".
func (s *Store) CountRecentCrashes(ctx context.Context, serverID string, window time.Duration) (int, error) {
	if s.db == nil {
		return 0, errors.New("no database connection")
	}
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
