package store

import (
	"context"
	"fmt"
	"time"
)

type MailNotificationTrigger struct {
	ID              string    `json:"id"`
	Event           string    `json:"event"`
	Template        string    `json:"template"`
	Enabled         bool      `json:"enabled"`
	SubjectTemplate string    `json:"subjectTemplate"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type UpdateMailNotificationTriggerRequest struct {
	Enabled *bool   `json:"enabled"`
	Subject *string `json:"subjectTemplate"`
}

func (s *Store) GetMailNotificationTriggers(ctx context.Context) ([]MailNotificationTrigger, error) {
	if s.db == nil {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, event, template, enabled, subject_template, created_at, updated_at
		FROM mail_notification_triggers
		ORDER BY event
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var triggers []MailNotificationTrigger
	for rows.Next() {
		var t MailNotificationTrigger
		if err := rows.Scan(&t.ID, &t.Event, &t.Template, &t.Enabled, &t.SubjectTemplate, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		triggers = append(triggers, t)
	}
	return triggers, rows.Err()
}

func (s *Store) UpdateMailNotificationTrigger(ctx context.Context, event string, req UpdateMailNotificationTriggerRequest) error {
	if s.db == nil {
		return nil
	}
	if req.Enabled == nil && req.Subject == nil {
		return nil
	}
	sets := "updated_at = now()"
	args := []any{}
	argIdx := 1

	if req.Enabled != nil {
		sets = fmt.Sprintf("enabled = $%d, %s", argIdx, sets)
		args = append(args, *req.Enabled)
		argIdx++
	}
	if req.Subject != nil {
		sets = fmt.Sprintf("subject_template = $%d, %s", argIdx, sets)
		args = append(args, *req.Subject)
		argIdx++
	}

	args = append(args, event)
	sql := fmt.Sprintf(`UPDATE mail_notification_triggers SET %s WHERE event = $%d`, sets, argIdx)
	_, err := s.db.Exec(ctx, sql, args...)
	return err
}
