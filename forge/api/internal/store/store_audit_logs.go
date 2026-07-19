package store

import (
	"context"
	"encoding/json"
	"time"

	"gamepanel/forge/internal/models"

	"github.com/google/uuid"
)

func (s *Store) CreateAuditLog(ctx context.Context, log *models.AuditLog) error {
	if log.ID == "" {
		log.ID = uuid.NewString()
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}
	details, _ := json.Marshal(log.Details)
	_, err := s.db.Exec(ctx, `
		INSERT INTO audit_logs (id, user_id, action, resource_type, resource_id, details, ip_address, user_agent, created_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9)
	`, log.ID, log.UserID, log.Action, log.ResourceType, log.ResourceID, string(details), log.IPAddress, log.UserAgent, log.CreatedAt)
	return err
}

func (s *Store) ListAuditLogsByUser(ctx context.Context, userID string, limit, offset int) ([]models.AuditLog, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, user_id::text, action, resource_type, resource_id, details::text, ip_address, user_agent, created_at
		FROM audit_logs
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAuditLogs(rows)
}

func (s *Store) ListAuditLogsByResource(ctx context.Context, resourceType, resourceID string, limit, offset int) ([]models.AuditLog, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, user_id::text, action, resource_type, resource_id, details::text, ip_address, user_agent, created_at
		FROM audit_logs
		WHERE resource_type = $1 AND resource_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, resourceType, resourceID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAuditLogs(rows)
}

func (s *Store) ListRecentAuditLogs(ctx context.Context, limit int) ([]models.AuditLog, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, user_id::text, action, resource_type, resource_id, details::text, ip_address, user_agent, created_at
		FROM audit_logs
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAuditLogs(rows)
}

func (s *Store) DeleteAuditLog(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM audit_logs WHERE id = $1`, id)
	return err
}

type pgxRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}

func scanAuditLogs(rows pgxRows) ([]models.AuditLog, error) {
	logs := make([]models.AuditLog, 0)
	for rows.Next() {
		var log models.AuditLog
		var detailsJSON []byte
		if err := rows.Scan(&log.ID, &log.UserID, &log.Action, &log.ResourceType, &log.ResourceID, &detailsJSON, &log.IPAddress, &log.UserAgent, &log.CreatedAt); err != nil {
			return nil, err
		}
		if len(detailsJSON) > 0 {
			_ = json.Unmarshal(detailsJSON, &log.Details)
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}
