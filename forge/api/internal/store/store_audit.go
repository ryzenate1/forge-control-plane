package store

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Store) ListAudit(ctx context.Context, limit, offset int) ([]AuditEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.Query(ctx, `
		SELECT a.id::text, a.action, a.target_type, a.target_id::text, a.metadata::text, a.created_at, u.email
		FROM audit_events a
		LEFT JOIN users u ON u.id = a.actor_id
		ORDER BY a.created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []AuditEvent{}
	for rows.Next() {
		var event AuditEvent
		if err := rows.Scan(&event.ID, &event.Action, &event.TargetType, &event.TargetID, &event.Metadata, &event.CreatedAt, &event.ActorEmail); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) ListServerAudit(ctx context.Context, serverID string) ([]AuditEvent, error) {
	rows, err := s.db.Query(ctx, `
		SELECT a.id::text, a.action, a.target_type, a.target_id::text, a.metadata::text, a.created_at, u.email
		FROM audit_events a
		LEFT JOIN users u ON u.id = a.actor_id
		WHERE a.target_type = 'server' AND a.target_id = $1
		ORDER BY a.created_at DESC
		LIMIT 100
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []AuditEvent{}
	for rows.Next() {
		var event AuditEvent
		if err := rows.Scan(&event.ID, &event.Action, &event.TargetType, &event.TargetID, &event.Metadata, &event.CreatedAt, &event.ActorEmail); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) AppendAudit(ctx context.Context, actorID *string, action, targetType string, targetID *string, metadata string) error {
	metadata = normalizeAuditMetadata(metadata)
	_, err := s.db.Exec(ctx, `
		INSERT INTO audit_events (id, actor_id, action, target_type, target_id, metadata)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb)
	`, uuid.NewString(), actorID, action, targetType, targetID, metadata)
	return err
}

func normalizeAuditMetadata(metadata string) string {
	if metadata == "" {
		return "{}"
	}
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, []byte(metadata)); err != nil {
		return "{}"
	}
	return compacted.String()
}

func mustAuditJSON(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(body)
}

// AuditEventsFilter narrows an admin audit query over the live audit_events
// table — the table every production writer appends to via AppendAudit.
type AuditEventsFilter struct {
	UserID       string
	Action       string
	ResourceType string
	ResourceID   string
	Since        time.Time
	Until        time.Time
	Limit        int
}

// AuditEventRow is one audit_events row with the actor resolved for display.
type AuditEventRow struct {
	ID         string
	ActorID    *string
	Action     string
	TargetType string
	TargetID   *string
	Metadata   string
	CreatedAt  time.Time
	ActorEmail *string
}

// QueryAuditEvents reads the live audit trail with optional filters. Empty
// filter fields match everything; limit defaults to 50 and caps at 500.
func (s *Store) QueryAuditEvents(ctx context.Context, f AuditEventsFilter) ([]AuditEventRow, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	conds := []string{}
	args := []any{}
	if f.UserID != "" {
		args = append(args, f.UserID)
		conds = append(conds, "a.actor_id = $"+strconv.Itoa(len(args)))
	}
	if f.Action != "" {
		args = append(args, f.Action)
		conds = append(conds, "a.action = $"+strconv.Itoa(len(args)))
	}
	if f.ResourceType != "" {
		args = append(args, f.ResourceType)
		conds = append(conds, "a.target_type = $"+strconv.Itoa(len(args)))
	}
	if f.ResourceID != "" {
		args = append(args, f.ResourceID)
		conds = append(conds, "a.target_id::text = $"+strconv.Itoa(len(args)))
	}
	if !f.Since.IsZero() {
		args = append(args, f.Since)
		conds = append(conds, "a.created_at >= $"+strconv.Itoa(len(args)))
	}
	if !f.Until.IsZero() {
		args = append(args, f.Until)
		conds = append(conds, "a.created_at <= $"+strconv.Itoa(len(args)))
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	args = append(args, limit)
	query := `
		SELECT a.id::text, a.actor_id::text, a.action, a.target_type, a.target_id::text,
		       a.metadata::text, a.created_at, u.email
		FROM audit_events a
		LEFT JOIN users u ON u.id = a.actor_id
		` + where + `
		ORDER BY a.created_at DESC
		LIMIT $` + strconv.Itoa(len(args))
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEventRow{}
	for rows.Next() {
		var r AuditEventRow
		if err := rows.Scan(&r.ID, &r.ActorID, &r.Action, &r.TargetType, &r.TargetID, &r.Metadata, &r.CreatedAt, &r.ActorEmail); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
