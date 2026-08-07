package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ---------- Activity Log types ----------

type ActivityLog struct {
	ID          string          `json:"id"`
	Event       string          `json:"event"`
	IP          *string         `json:"ip,omitempty"`
	Description *string         `json:"description,omitempty"`
	Properties  json.RawMessage `json:"properties"`
	ActorID     *string         `json:"actorId,omitempty"`
	ActorEmail  *string         `json:"actorEmail,omitempty"`
	SubjectType *string         `json:"subjectType,omitempty"`
	SubjectID   *string         `json:"subjectId,omitempty"`
	Timestamp   time.Time       `json:"timestamp"`
}

// ---------- Activity Log methods ----------

func (s *Store) LogActivity(ctx context.Context, event string, actorID *string, ip *string, subjectType *string, subjectID *string, properties map[string]any) error {
	propsJSON, _ := json.Marshal(properties)
	if propsJSON == nil {
		propsJSON = []byte("{}")
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO activity_logs (id, event, ip, actor_id, subject_type, subject_id, properties)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, uuid.NewString(), event, ip, actorID, subjectType, subjectID, propsJSON)
	return err
}

func (s *Store) ListActivityLogs(ctx context.Context, subjectType *string, subjectID *string, limit int) ([]ActivityLog, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	query := `
		SELECT a.id::text, a.event, a.ip, a.description, a.properties,
		       a.actor_id::text, u.email, a.subject_type, a.subject_id::text, a.timestamp
		FROM activity_logs a
		LEFT JOIN users u ON u.id = a.actor_id
		WHERE ($1::text IS NULL OR a.subject_type = $1)
		  AND ($2::text IS NULL OR a.subject_id = $2::uuid)
		ORDER BY a.timestamp DESC
		LIMIT $3
	`

	rows, err := s.db.Query(ctx, query, subjectType, subjectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := []ActivityLog{}
	for rows.Next() {
		var log ActivityLog
		if err := rows.Scan(&log.ID, &log.Event, &log.IP, &log.Description, &log.Properties,
			&log.ActorID, &log.ActorEmail, &log.SubjectType, &log.SubjectID, &log.Timestamp); err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

// ---------- Activity Event (activity_events table) ----------

type ActivityEvent struct {
	ID          string          `json:"id"`
	Event       string          `json:"event"`
	Description *string         `json:"description,omitempty"`
	ActorID     *string         `json:"actorId,omitempty"`
	ActorEmail  *string         `json:"actorEmail,omitempty"`
	ActorType   string          `json:"actorType"`
	IP          *string         `json:"ip,omitempty"`
	UserAgent   *string         `json:"userAgent,omitempty"`
	SubjectType *string         `json:"subjectType,omitempty"`
	SubjectID   *string         `json:"subjectId,omitempty"`
	SubjectName *string         `json:"subjectName,omitempty"`
	Properties  json.RawMessage `json:"properties"`
	Level       string          `json:"level"`
	Source      string          `json:"source"`
	Timestamp   time.Time       `json:"timestamp"`
	ExpiresAt   *time.Time      `json:"expiresAt,omitempty"`
}

func (s *Store) CreateActivityEvent(ctx context.Context, event ActivityEvent) error {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.Properties == nil {
		event.Properties = json.RawMessage("{}")
	}
	if event.Level == "" {
		event.Level = "info"
	}
	if event.Source == "" {
		event.Source = "api"
	}
	if event.ActorType == "" {
		event.ActorType = "user"
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO activity_events (id, event, description, actor_id, actor_email, actor_type, ip, user_agent, subject_type, subject_id, subject_name, properties, level, source, timestamp, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`, event.ID, event.Event, event.Description, event.ActorID, event.ActorEmail, event.ActorType, event.IP, event.UserAgent, event.SubjectType, event.SubjectID, event.SubjectName, event.Properties, event.Level, event.Source, event.Timestamp, event.ExpiresAt)
	return err
}

func (s *Store) ListUserActivityLogs(ctx context.Context, userID string, limit int) ([]ActivityLog, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := s.db.Query(ctx, `
		SELECT a.id::text, a.event, a.ip, a.description, a.properties,
		       a.actor_id::text, u.email, a.subject_type, a.subject_id::text, a.timestamp
		FROM activity_logs a
		LEFT JOIN users u ON u.id = a.actor_id
		WHERE a.actor_id = $1
		ORDER BY a.timestamp DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := []ActivityLog{}
	for rows.Next() {
		var log ActivityLog
		if err := rows.Scan(&log.ID, &log.Event, &log.IP, &log.Description, &log.Properties,
			&log.ActorID, &log.ActorEmail, &log.SubjectType, &log.SubjectID, &log.Timestamp); err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}
