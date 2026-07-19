package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresSessionStore struct {
	pool *pgxpool.Pool
}

func NewPostgresSessionStore(pool *pgxpool.Pool) *PostgresSessionStore {
	return &PostgresSessionStore{pool: pool}
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

const (
	insertSessionSQL = `INSERT INTO user_sessions
		(id, user_id, session_token_hash, ip_address, user_agent, created_at, expires_at, last_activity)
		VALUES ($1, $2, $3, $4, $5, NOW(), $6, NOW())`

	getSessionSQL = `SELECT id::text, user_id::text, ip_address, user_agent, created_at, expires_at, last_activity
		FROM user_sessions
		WHERE id = $1 AND NOT is_revoked`

	getSessionByTokenSQL = `SELECT id::text, user_id::text, ip_address, user_agent, created_at, expires_at, last_activity
		FROM user_sessions
		WHERE session_token_hash = $1 AND NOT is_revoked`

	updateSessionSQL = `UPDATE user_sessions
		SET ip_address = $2, user_agent = $3, expires_at = $4, last_activity = $5
		WHERE id = $1 AND NOT is_revoked`

	deleteSessionSQL    = `DELETE FROM user_sessions WHERE id = $1`
	deleteSessionsBySQL = `DELETE FROM user_sessions WHERE user_id = $1`

	listSessionsByUserSQL = `SELECT id::text, user_id::text, ip_address, user_agent, created_at, expires_at, last_activity
		FROM user_sessions
		WHERE user_id = $1 AND NOT is_revoked
		ORDER BY last_activity DESC`

	cleanupSQL = `DELETE FROM user_sessions WHERE expires_at < NOW() OR (is_revoked = TRUE AND revoked_at < NOW() - INTERVAL '30 days')`
)

func scanSession(row pgx.Row) (*Session, error) {
	var s Session
	err := row.Scan(&s.ID, &s.UserID, &s.IPAddress, &s.UserAgent, &s.CreatedAt, &s.ExpiresAt, &s.LastActiveAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("session: not found")
		}
		return nil, err
	}
	if time.Now().After(s.ExpiresAt) {
		return nil, errors.New("session: expired")
	}
	return &s, nil
}

func scanSessions(rows pgx.Rows) ([]*Session, error) {
	defer rows.Close()
	var result []*Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.UserID, &s.IPAddress, &s.UserAgent, &s.CreatedAt, &s.ExpiresAt, &s.LastActiveAt); err != nil {
			return nil, err
		}
		result = append(result, &s)
	}
	return result, rows.Err()
}

func (p *PostgresSessionStore) Create(ctx context.Context, session *Session) error {
	if session == nil {
		return errors.New("session: nil session")
	}
	if session.ID == "" || session.Token == "" || session.UserID == "" {
		return errors.New("session: id, token, and userId are required")
	}

	tokenHash := hashToken(session.Token)
	_, err := p.pool.Exec(ctx, insertSessionSQL,
		session.ID,
		session.UserID,
		tokenHash,
		session.IPAddress,
		session.UserAgent,
		session.ExpiresAt,
	)
	return err
}

func (p *PostgresSessionStore) Get(ctx context.Context, id string) (*Session, error) {
	row := p.pool.QueryRow(ctx, getSessionSQL, id)
	return scanSession(row)
}

func (p *PostgresSessionStore) GetByToken(ctx context.Context, token string) (*Session, error) {
	tokenHash := hashToken(token)
	row := p.pool.QueryRow(ctx, getSessionByTokenSQL, tokenHash)
	return scanSession(row)
}

func (p *PostgresSessionStore) Update(ctx context.Context, session *Session) error {
	if session == nil {
		return errors.New("session: nil session")
	}
	result, err := p.pool.Exec(ctx, updateSessionSQL,
		session.ID,
		session.IPAddress,
		session.UserAgent,
		session.ExpiresAt,
		session.LastActiveAt,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errors.New("session: not found")
	}
	return nil
}

func (p *PostgresSessionStore) Delete(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, deleteSessionSQL, id)
	return err
}

func (p *PostgresSessionStore) DeleteByUser(ctx context.Context, userID string) error {
	_, err := p.pool.Exec(ctx, deleteSessionsBySQL, userID)
	return err
}

func (p *PostgresSessionStore) ListByUser(ctx context.Context, userID string) ([]*Session, error) {
	rows, err := p.pool.Query(ctx, listSessionsByUserSQL, userID)
	if err != nil {
		return nil, err
	}
	return scanSessions(rows)
}

func (p *PostgresSessionStore) Cleanup(ctx context.Context) error {
	_, err := p.pool.Exec(ctx, cleanupSQL)
	return err
}
