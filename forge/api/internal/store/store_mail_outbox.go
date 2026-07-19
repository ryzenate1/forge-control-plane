package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type MailOutboxItem struct {
	ID        string
	Recipient string
	Subject   string
	TextBody  string
	HTMLBody  string
	Attempts  int
}

func (s *Store) EnqueueMail(ctx context.Context, recipient, subject, textBody, htmlBody string) (string, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO mail_outbox (id, recipient, subject, text_body, html_body)
		VALUES ($1, $2, $3, $4, $5)
	`, id, recipient, subject, textBody, htmlBody)
	return id, err
}

// EnqueuePasswordReset atomically creates the reset token and its email. A
// missing account is deliberately reported as not accepted without an error so
// the HTTP layer can return the same anti-enumeration response.
func (s *Store) EnqueuePasswordReset(ctx context.Context, email, tokenHash string, ttl time.Duration, ip, resetURL string) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var userID, storedEmail string
	if err := tx.QueryRow(ctx, `
		SELECT id::text, email FROM users
		WHERE lower(email) = lower($1) AND NOT disabled
		FOR UPDATE
	`, strings.TrimSpace(email)).Scan(&userID, &storedEmail); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at, requested_ip)
		VALUES ($1, $2, $3, now() + $4::interval, $5)
	`, uuid.NewString(), userID, tokenHash, ttl.String(), ip); err != nil {
		return false, err
	}
	text := "A password reset was requested for your account.\n\nReset your password: " + resetURL + "\n\nThis link expires in 30 minutes. If you did not request this, ignore this email."
	html := `<p>A password reset was requested for your account.</p><p><a href="` + resetURL + `">Reset your password</a></p><p>This link expires in 30 minutes. If you did not request this, ignore this email.</p>`
	if _, err := tx.Exec(ctx, `
		INSERT INTO mail_outbox (id, recipient, subject, text_body, html_body)
		VALUES ($1, $2, 'Reset your password', $3, $4)
	`, uuid.NewString(), storedEmail, text, html); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ClaimMail(ctx context.Context, workerID string, staleAfter time.Duration) (*MailOutboxItem, error) {
	if staleAfter <= 0 {
		staleAfter = time.Minute
	}
	var item MailOutboxItem
	err := s.db.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id FROM mail_outbox
			WHERE sent_at IS NULL
			  AND next_attempt_at <= now()
			  AND (locked_at IS NULL OR locked_at < now() - $2::interval)
			ORDER BY next_attempt_at, created_at
			FOR UPDATE SKIP LOCKED LIMIT 1
		)
		UPDATE mail_outbox m
		SET locked_at = now(), locked_by = $1, attempts = attempts + 1
		FROM candidate c WHERE m.id = c.id
		RETURNING m.id::text, m.recipient, m.subject, m.text_body, m.html_body, m.attempts
	`, workerID, staleAfter.String()).Scan(&item.ID, &item.Recipient, &item.Subject, &item.TextBody, &item.HTMLBody, &item.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &item, err
}

func (s *Store) CompleteMail(ctx context.Context, id, workerID string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE mail_outbox SET sent_at = now(), last_error = NULL, locked_at = NULL, locked_by = NULL
		WHERE id = $1 AND locked_by = $2
	`, id, workerID)
	return err
}

func (s *Store) RetryMail(ctx context.Context, id, workerID, lastError string, delay time.Duration) error {
	_, err := s.db.Exec(ctx, `
		UPDATE mail_outbox
		SET last_error = left($3, 4000), next_attempt_at = now() + $4::interval, locked_at = NULL, locked_by = NULL
		WHERE id = $1 AND locked_by = $2
	`, id, workerID, lastError, delay.String())
	return err
}
