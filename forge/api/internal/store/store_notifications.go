package store

import (
	"context"
	"encoding/json"
	"time"

	"gamepanel/forge/internal/models"

	"github.com/google/uuid"
)

func (s *Store) CreateNotification(ctx context.Context, n *models.Notification) error {
	if n.ID == "" {
		n.ID = uuid.NewString()
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now()
	}
	metadata, _ := json.Marshal(n.Metadata)
	_, err := s.db.Exec(ctx, `
		INSERT INTO notifications (id, user_id, type, title, body, read, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8)
	`, n.ID, n.UserID, n.Type, n.Title, n.Body, n.Read, string(metadata), n.CreatedAt)
	return err
}

func (s *Store) ListNotificationsByUser(ctx context.Context, userID string, limit, offset int) ([]models.Notification, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, user_id::text, type, title, body, read, metadata::text, created_at, read_at
		FROM notifications
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notifications := make([]models.Notification, 0)
	for rows.Next() {
		var n models.Notification
		var metadataJSON []byte
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body, &n.Read, &metadataJSON, &n.CreatedAt, &n.ReadAt); err != nil {
			return nil, err
		}
		if len(metadataJSON) > 0 {
			_ = json.Unmarshal(metadataJSON, &n.Metadata)
		}
		notifications = append(notifications, n)
	}
	return notifications, rows.Err()
}

func (s *Store) MarkNotificationRead(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `UPDATE notifications SET read = true, read_at = now() WHERE id = $1`, id)
	return err
}

func (s *Store) MarkAllNotificationsRead(ctx context.Context, userID string) error {
	_, err := s.db.Exec(ctx, `UPDATE notifications SET read = true, read_at = now() WHERE user_id = $1 AND read = false`, userID)
	return err
}

func (s *Store) DeleteNotification(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM notifications WHERE id = $1`, id)
	return err
}

func (s *Store) CountUnreadNotifications(ctx context.Context, userID string) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND read = false`, userID).Scan(&count)
	return count, err
}

func (s *Store) GetNotificationPreference(ctx context.Context, userID, channel, eventType string) (*models.NotificationPreference, error) {
	var np models.NotificationPreference
	err := s.db.QueryRow(ctx, `
		SELECT id::text, user_id::text, channel, event_type, enabled
		FROM notification_preferences
		WHERE user_id = $1 AND channel = $2 AND event_type = $3
	`, userID, channel, eventType).Scan(&np.ID, &np.UserID, &np.Channel, &np.EventType, &np.Enabled)
	if err != nil {
		return nil, err
	}
	return &np, nil
}

func (s *Store) UpsertNotificationPreference(ctx context.Context, np *models.NotificationPreference) error {
	if np.ID == "" {
		np.ID = uuid.NewString()
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO notification_preferences (id, user_id, channel, event_type, enabled)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, channel, event_type) DO UPDATE SET enabled = EXCLUDED.enabled
	`, np.ID, np.UserID, np.Channel, np.EventType, np.Enabled)
	return err
}

func (s *Store) DeleteNotificationPreference(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM notification_preferences WHERE id = $1`, id)
	return err
}
