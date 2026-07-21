package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type WebhookDelivery struct {
	ID                  string      `json:"id"`
	WebhookID           *string     `json:"webhookId,omitempty"`
	EventName           string      `json:"eventName"`
	TargetURL           string      `json:"targetUrl"`
	WebhookType         WebhookType `json:"webhookType"`
	Secret              string      `json:"-"`
	Payload             []byte      `json:"-"`
	RequestBody         []byte      `json:"-"`
	Attempts            int         `json:"attempts"`
	ResponseStatus      *int        `json:"responseStatus,omitempty"`
	ResponseBodyExcerpt string      `json:"responseBodyExcerpt,omitempty"`
	LastError           *string     `json:"lastError,omitempty"`
	NextAttemptAt       time.Time   `json:"nextAttemptAt"`
	State               string      `json:"state"`
	DeliveredAt         *time.Time  `json:"deliveredAt,omitempty"`
	CreatedAt           time.Time   `json:"createdAt"`
}

func (s *Store) CreateTestDelivery(ctx context.Context, wh Webhook, eventName string, payload []byte, requestBody []byte) (WebhookDelivery, error) {
	deliveryID := uuid.NewString()
	encryptedSnapshot, err := s.encryptSecret(wh.Secret, secretAAD("webhook_deliveries", deliveryID, "secret"))
	if err != nil {
		return WebhookDelivery{}, err
	}
	now := time.Now().UTC()
	_, err = s.db.Exec(ctx, `
		INSERT INTO webhook_deliveries (id, webhook_id, event_name, target_url, webhook_type, secret, secret_encrypted, payload, request_body, attempts, state, next_attempt_at, created_at)
		VALUES ($1,$2,$3,$4,$5,'',$6,$7::jsonb,$8::jsonb,0,'pending',$9,$10)
	`, deliveryID, wh.ID, eventName, wh.URL, wh.WebhookType, encryptedSnapshot, string(payload), string(requestBody), now, now)
	if err != nil {
		return WebhookDelivery{}, err
	}

	var d WebhookDelivery
	err = s.db.QueryRow(ctx, `
		SELECT id::text, webhook_id, event_name, target_url, webhook_type, attempts, response_status, response_body_excerpt, last_error, next_attempt_at, state, delivered_at, created_at
		FROM webhook_deliveries WHERE id = $1
	`, deliveryID).Scan(&d.ID, &d.WebhookID, &d.EventName, &d.TargetURL, &d.WebhookType, &d.Attempts, &d.ResponseStatus, &d.ResponseBodyExcerpt, &d.LastError, &d.NextAttemptAt, &d.State, &d.DeliveredAt, &d.CreatedAt)
	if err != nil {
		return WebhookDelivery{}, err
	}
	return d, nil
}

func (s *Store) UpdateTestDelivery(ctx context.Context, id string, status int, excerpt string, lastError string, success bool) error {
	now := time.Now().UTC()
	if !success {
		errLast := lastError
		if errLast == "" {
			errLast = "test failed"
		}
		_, err := s.db.Exec(ctx, `UPDATE webhook_deliveries SET state='failed', response_status=$1, response_body_excerpt=left($2,4000), last_error=left($3,4000), attempts=1, next_attempt_at=$4, locked_at=NULL, locked_by=NULL, updated_at=$5 WHERE id=$6`, status, excerpt, errLast, now.Add(24*time.Hour), now, id)
		return err
	}
	_, err := s.db.Exec(ctx, `UPDATE webhook_deliveries SET state='delivered', response_status=$1, response_body_excerpt=left($2,4000), last_error=NULL, attempts=1, delivered_at=$3, locked_at=NULL, locked_by=NULL, updated_at=$4 WHERE id=$5`, status, excerpt, now, now, id)
	return err
}

func (s *Store) RetryDelivery(ctx context.Context, deliveryID string) error {
	cmd, err := s.db.Exec(ctx, `UPDATE webhook_deliveries SET state='pending', locked_at=NULL, locked_by=NULL, next_attempt_at=now(), updated_at=now() WHERE id=$1 AND state='failed'`, deliveryID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("delivery not found or not in a retriable state")
	}
	return nil
}

func (s *Store) DeleteDelivery(ctx context.Context, deliveryID string) error {
	cmd, err := s.db.Exec(ctx, `DELETE FROM webhook_deliveries WHERE id=$1`, deliveryID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("delivery not found")
	}
	return nil
}

func (s *Store) EnqueueWebhookEvent(ctx context.Context, event string, payload map[string]any) error {
	payload["event"] = event
	if _, ok := payload["timestamp"]; !ok {
		payload["timestamp"] = time.Now().UTC().Format(time.RFC3339)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id, name, COALESCE(description,''), url, webhook_type, COALESCE(events,'{}'), enabled, COALESCE(secret,''), COALESCE(secret_encrypted,''), COALESCE(discord_username,''), COALESCE(discord_avatar_url,''), COALESCE(discord_content,''), created_at, updated_at FROM webhooks WHERE enabled = true FOR SHARE`)
	if err != nil {
		return err
	}
	var hooks []Webhook
	for rows.Next() {
		var wh Webhook
		var plaintext, encrypted string
		if err := rows.Scan(&wh.ID, &wh.Name, &wh.Description, &wh.URL, &wh.WebhookType, &wh.Events, &wh.Enabled, &plaintext, &encrypted, &wh.DiscordUsername, &wh.DiscordAvatarURL, &wh.DiscordContent, &wh.CreatedAt, &wh.UpdatedAt); err != nil {
			rows.Close()
			return err
		}
		wh.Secret, err = s.decryptSecret(encrypted, plaintext, secretAAD("webhooks", wh.ID, "secret"))
		if err != nil {
			rows.Close()
			return err
		}
		if eventMatchesSubscription(event, wh.Events) {
			hooks = append(hooks, wh)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, wh := range hooks {
		body := raw
		if wh.WebhookType == WebhookTypeDiscord {
			body = WrapDiscordEmbed(wh, event, raw)
		}
		deliveryID := uuid.NewString()
		encryptedSnapshot, err := s.encryptSecret(wh.Secret, secretAAD("webhook_deliveries", deliveryID, "secret"))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO webhook_deliveries (id, webhook_id, event_name, target_url, webhook_type, secret, secret_encrypted, payload, request_body) VALUES ($1,$2,$3,$4,$5,'',$6,$7::jsonb,$8::jsonb)`, deliveryID, wh.ID, event, wh.URL, wh.WebhookType, encryptedSnapshot, string(raw), string(body)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) ClaimWebhookDelivery(ctx context.Context, workerID string, staleAfter time.Duration) (*WebhookDelivery, error) {
	if staleAfter <= 0 {
		staleAfter = time.Minute
	}
	var d WebhookDelivery
	var plaintext, encrypted string
	err := s.db.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id FROM webhook_deliveries WHERE state IN ('pending','processing') AND next_attempt_at <= now() AND (locked_at IS NULL OR locked_at < now() - $2::interval)
			ORDER BY next_attempt_at, created_at FOR UPDATE SKIP LOCKED LIMIT 1
		)
		UPDATE webhook_deliveries d SET state='processing', locked_at=now(), locked_by=$1, attempts=attempts+1, updated_at=now()
		FROM candidate c WHERE d.id=c.id
		RETURNING d.id::text, d.webhook_id, d.event_name, d.target_url, d.webhook_type, COALESCE(d.secret,''), COALESCE(d.secret_encrypted,''), d.payload, d.request_body, d.attempts, d.next_attempt_at, d.state, d.created_at
	`, workerID, staleAfter.String()).Scan(&d.ID, &d.WebhookID, &d.EventName, &d.TargetURL, &d.WebhookType, &plaintext, &encrypted, &d.Payload, &d.RequestBody, &d.Attempts, &d.NextAttemptAt, &d.State, &d.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.Secret, err = s.decryptSecret(encrypted, plaintext, secretAAD("webhook_deliveries", d.ID, "secret"))
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) CompleteWebhookDelivery(ctx context.Context, id, workerID string, status int, excerpt string) error {
	_, err := s.db.Exec(ctx, `UPDATE webhook_deliveries SET state='delivered', response_status=$3, response_body_excerpt=$4, last_error=NULL, delivered_at=now(), locked_at=NULL, locked_by=NULL, updated_at=now() WHERE id=$1 AND locked_by=$2`, id, workerID, status, excerpt)
	return err
}

func (s *Store) FailWebhookDelivery(ctx context.Context, id, workerID string, status *int, excerpt, lastError string, retry bool, delay time.Duration) error {
	state := "failed"
	if retry {
		state = "pending"
	}
	// When retry is false (exhausted maxAttempts), the delivery transitions to
	// state='failed' permanently and becomes a dead-letter entry. It is never
	// deleted, preserving the full delivery record including last_error as the
	// dead_letter_reason. Dead-letter deliveries are queryable via
	// ListDeadLetterDeliveries and are excluded from ClaimWebhookDelivery.
	_, err := s.db.Exec(ctx, `UPDATE webhook_deliveries SET state=$3, response_status=$4, response_body_excerpt=left($5,4000), last_error=left($6,4000), next_attempt_at=now()+$7::interval, locked_at=NULL, locked_by=NULL, updated_at=now() WHERE id=$1 AND locked_by=$2`, id, workerID, state, status, excerpt, lastError, delay.String())
	return err
}

func (s *Store) ListWebhookDeliveries(ctx context.Context, webhookID string, limit int, offset int) ([]WebhookDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.Query(ctx, `SELECT id::text, webhook_id, event_name, target_url, webhook_type, attempts, response_status, response_body_excerpt, last_error, next_attempt_at, state, delivered_at, created_at FROM webhook_deliveries WHERE ($1='' OR webhook_id=$1) ORDER BY created_at DESC OFFSET $2 LIMIT $3`, webhookID, offset, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WebhookDelivery{}
	for rows.Next() {
		var d WebhookDelivery
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.EventName, &d.TargetURL, &d.WebhookType, &d.Attempts, &d.ResponseStatus, &d.ResponseBodyExcerpt, &d.LastError, &d.NextAttemptAt, &d.State, &d.DeliveredAt, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) RetryWebhookDelivery(ctx context.Context, deliveryID string) error {
	tag, err := s.db.Exec(ctx, `UPDATE webhook_deliveries SET state='pending', attempts=0, locked_at=NULL, locked_by=NULL, last_error=NULL, next_attempt_at=now(), updated_at=now() WHERE id=$1 AND state IN ('failed','delivered')`, deliveryID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("webhook delivery not found or not eligible for retry")
	}
	return nil
}

func (s *Store) DeleteWebhookDelivery(ctx context.Context, deliveryID string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM webhook_deliveries WHERE id = $1`, deliveryID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("webhook delivery not found")
	}
	return nil
}

func (s *Store) ListDeadLetterDeliveries(ctx context.Context, webhookID string, limit int, offset int) ([]WebhookDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}
	var rows interface {
		Close()
		Next() bool
		Scan(dest ...any) error
		Err() error
	}
	var err error
	if webhookID != "" {
		rows, err = s.db.Query(ctx, `SELECT id::text, webhook_id, event_name, target_url, webhook_type, attempts, response_status, response_body_excerpt, last_error, next_attempt_at, state, delivered_at, created_at FROM webhook_deliveries WHERE state='failed' AND webhook_id=$1 ORDER BY created_at DESC OFFSET $2 LIMIT $3`, webhookID, offset, limit)
	} else {
		rows, err = s.db.Query(ctx, `SELECT id::text, webhook_id, event_name, target_url, webhook_type, attempts, response_status, response_body_excerpt, last_error, next_attempt_at, state, delivered_at, created_at FROM webhook_deliveries WHERE state='failed' ORDER BY created_at DESC OFFSET $1 LIMIT $2`, offset, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WebhookDelivery{}
	for rows.Next() {
		var d WebhookDelivery
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.EventName, &d.TargetURL, &d.WebhookType, &d.Attempts, &d.ResponseStatus, &d.ResponseBodyExcerpt, &d.LastError, &d.NextAttemptAt, &d.State, &d.DeliveredAt, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
