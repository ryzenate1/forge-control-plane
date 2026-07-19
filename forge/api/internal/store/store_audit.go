package store

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

func (s *Store) ListAudit(ctx context.Context) ([]AuditEvent, error) {
	rows, err := s.db.Query(ctx, `
		SELECT a.id::text, a.action, a.target_type, a.target_id::text, a.metadata::text, a.created_at, u.email
		FROM audit_events a
		LEFT JOIN users u ON u.id = a.actor_id
		ORDER BY a.created_at DESC
		LIMIT 50
	`)
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
