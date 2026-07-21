package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type NotificationChannelType string

const (
	NotificationChannelSlack    NotificationChannelType = "slack"
	NotificationChannelDiscord  NotificationChannelType = "discord"
	NotificationChannelTelegram NotificationChannelType = "telegram"
	NotificationChannelEmail    NotificationChannelType = "email"
	NotificationChannelWebhook  NotificationChannelType = "webhook"
)

type NotificationChannel struct {
	ID        string                  `json:"id"`
	Type      NotificationChannelType `json:"type"`
	Name      string                  `json:"name"`
	Config    map[string]any          `json:"config"`
	Enabled   bool                    `json:"enabled"`
	CreatedAt time.Time               `json:"createdAt"`
	UpdatedAt time.Time               `json:"updatedAt"`
}

type CreateNotificationChannelRequest struct {
	Type    NotificationChannelType
	Name    string
	Config  map[string]any
	Enabled bool
}

type UpdateNotificationChannelRequest struct {
	Name    *string
	Config  *map[string]any
	Enabled *bool
}

type NotificationEventSubscription struct {
	ID             string     `json:"id"`
	ChannelID      string     `json:"channelId"`
	EventType      string     `json:"eventType"`
	Template       string     `json:"template"`
	LastSentAt     *time.Time `json:"lastSentAt,omitempty"`
	DeliveryStatus string     `json:"deliveryStatus"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type NotificationLog struct {
	ID        string    `json:"id"`
	ChannelID string    `json:"channelId"`
	EventType string    `json:"eventType"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	SentAt    time.Time `json:"sentAt"`
}

func (s *Store) CreateNotificationChannel(ctx context.Context, req CreateNotificationChannelRequest) (NotificationChannel, error) {
	id := uuid.NewString()
	config := req.Config
	if config == nil {
		config = map[string]any{}
	}
	configBytes, _ := json.Marshal(config)
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO notification_channels (id, type, name, config, enabled, created_at, updated_at)
		VALUES ($1,$2,$3,$4::jsonb,$5,$6,$7)
	`, id, string(req.Type), req.Name, string(configBytes), req.Enabled, now, now)
	if err != nil {
		return NotificationChannel{}, err
	}
	return s.GetNotificationChannel(ctx, id)
}

func (s *Store) GetNotificationChannel(ctx context.Context, id string) (NotificationChannel, error) {
	var ch NotificationChannel
	var configBytes []byte
	err := s.db.QueryRow(ctx, `
		SELECT id::text, type, name, config, enabled, created_at, updated_at
		FROM notification_channels WHERE id = $1
	`, id).Scan(&ch.ID, &ch.Type, &ch.Name, &configBytes, &ch.Enabled, &ch.CreatedAt, &ch.UpdatedAt)
	if err != nil {
		return NotificationChannel{}, err
	}
	if len(configBytes) > 0 {
		json.Unmarshal(configBytes, &ch.Config)
	}
	if ch.Config == nil {
		ch.Config = map[string]any{}
	}
	return ch, nil
}

func (s *Store) ListNotificationChannels(ctx context.Context) ([]NotificationChannel, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, type, name, config, enabled, created_at, updated_at
		FROM notification_channels ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []NotificationChannel{}
	for rows.Next() {
		var ch NotificationChannel
		var configBytes []byte
		if err := rows.Scan(&ch.ID, &ch.Type, &ch.Name, &configBytes, &ch.Enabled, &ch.CreatedAt, &ch.UpdatedAt); err != nil {
			return nil, err
		}
		if len(configBytes) > 0 {
			json.Unmarshal(configBytes, &ch.Config)
		}
		if ch.Config == nil {
			ch.Config = map[string]any{}
		}
		result = append(result, ch)
	}
	return result, rows.Err()
}

func (s *Store) UpdateNotificationChannel(ctx context.Context, id string, req UpdateNotificationChannelRequest) (NotificationChannel, error) {
	existing, err := s.GetNotificationChannel(ctx, id)
	if err != nil {
		return NotificationChannel{}, err
	}
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Config != nil {
		existing.Config = *req.Config
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	configBytes, _ := json.Marshal(existing.Config)
	now := time.Now().UTC()
	_, err = s.db.Exec(ctx, `
		UPDATE notification_channels SET name=$1, config=$2::jsonb, enabled=$3, updated_at=$4 WHERE id=$5
	`, existing.Name, string(configBytes), existing.Enabled, now, id)
	if err != nil {
		return NotificationChannel{}, err
	}
	return s.GetNotificationChannel(ctx, id)
}

func (s *Store) DeleteNotificationChannel(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM notification_channels WHERE id = $1`, id)
	return err
}

func (s *Store) CreateNotificationEventSubscription(ctx context.Context, channelID, eventType, template string) (NotificationEventSubscription, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO notification_event_subscriptions (id, channel_id, event_type, template, delivery_status, created_at, updated_at)
		VALUES ($1,$2,$3,$4,'pending',$5,$6)
		ON CONFLICT (channel_id, event_type) DO UPDATE SET template = EXCLUDED.template, updated_at = EXCLUDED.updated_at
	`, id, channelID, eventType, template, now, now)
	if err != nil {
		return NotificationEventSubscription{}, err
	}
	return s.GetNotificationEventSubscription(ctx, channelID, eventType)
}

func (s *Store) GetNotificationEventSubscription(ctx context.Context, channelID, eventType string) (NotificationEventSubscription, error) {
	var sub NotificationEventSubscription
	var lastSentAt sql.NullTime
	var template sql.NullString
	err := s.db.QueryRow(ctx, `
		SELECT id::text, channel_id::text, event_type, COALESCE(template,''), last_sent_at, delivery_status, created_at, updated_at
		FROM notification_event_subscriptions
		WHERE channel_id = $1 AND event_type = $2
	`, channelID, eventType).Scan(&sub.ID, &sub.ChannelID, &sub.EventType, &template, &lastSentAt, &sub.DeliveryStatus, &sub.CreatedAt, &sub.UpdatedAt)
	if err != nil {
		return NotificationEventSubscription{}, err
	}
	sub.Template = template.String
	if lastSentAt.Valid {
		sub.LastSentAt = &lastSentAt.Time
	}
	return sub, nil
}

func (s *Store) ListNotificationEventSubscriptions(ctx context.Context, channelID string) ([]NotificationEventSubscription, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, channel_id::text, event_type, COALESCE(template,''), last_sent_at, delivery_status, created_at, updated_at
		FROM notification_event_subscriptions
		WHERE ($1 = '' OR channel_id = $1)
		ORDER BY event_type ASC
	`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []NotificationEventSubscription{}
	for rows.Next() {
		var sub NotificationEventSubscription
		var lastSentAt sql.NullTime
		var template sql.NullString
		if err := rows.Scan(&sub.ID, &sub.ChannelID, &sub.EventType, &template, &lastSentAt, &sub.DeliveryStatus, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, err
		}
		sub.Template = template.String
		if lastSentAt.Valid {
			sub.LastSentAt = &lastSentAt.Time
		}
		result = append(result, sub)
	}
	return result, rows.Err()
}

func (s *Store) DeleteNotificationEventSubscription(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM notification_event_subscriptions WHERE id = $1`, id)
	return err
}

func (s *Store) CreateNotificationLog(ctx context.Context, channelID, eventType, status, errorMsg string) (NotificationLog, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO notification_logs (id, channel_id, event_type, status, error, sent_at)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, id, channelID, eventType, status, errorMsg, now)
	if err != nil {
		return NotificationLog{}, err
	}
	return NotificationLog{ID: id, ChannelID: channelID, EventType: eventType, Status: status, Error: errorMsg, SentAt: now}, nil
}

func (s *Store) ListNotificationLogs(ctx context.Context, channelID string, limit, offset int) ([]NotificationLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, channel_id::text, event_type, status, COALESCE(error,''), sent_at
		FROM notification_logs
		WHERE ($1 = '' OR channel_id = $1)
		ORDER BY sent_at DESC
		LIMIT $2 OFFSET $3
	`, channelID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []NotificationLog{}
	for rows.Next() {
		var log NotificationLog
		if err := rows.Scan(&log.ID, &log.ChannelID, &log.EventType, &log.Status, &log.Error, &log.SentAt); err != nil {
			return nil, err
		}
		result = append(result, log)
	}
	return result, rows.Err()
}

func (s *Store) UpdateNotificationSubscriptionDelivery(ctx context.Context, channelID, eventType, status string) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		UPDATE notification_event_subscriptions
		SET last_sent_at = $1, delivery_status = $2, updated_at = $1
		WHERE channel_id = $3 AND event_type = $4
	`, now, status, channelID, eventType)
	return err
}
