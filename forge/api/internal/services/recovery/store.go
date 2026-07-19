package recovery

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) CreateToken(ctx context.Context, token *RecoveryToken) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO recovery_tokens (id, user_id, type, token_hash, expires_at, created_at, metadata, ip, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, token.ID, token.UserID, string(token.Type), token.TokenHash, token.ExpiresAt, token.CreatedAt, token.Metadata, token.IP, token.UserAgent)
	return err
}

func (s *Store) GetTokenByHash(ctx context.Context, hash string) (*RecoveryToken, error) {
	var t RecoveryToken
	err := s.db.QueryRow(ctx, `
		SELECT id::text, user_id::text, type, token_hash, expires_at, used_at, created_at, COALESCE(metadata, '{}'), COALESCE(ip::text, ''), COALESCE(user_agent, '')
		FROM recovery_tokens
		WHERE token_hash = $1
	`, hash).Scan(&t.ID, &t.UserID, &t.Type, &t.TokenHash, &t.ExpiresAt, &t.UsedAt, &t.CreatedAt, &t.Metadata, &t.IP, &t.UserAgent)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

func (s *Store) MarkTokenUsed(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE recovery_tokens SET used_at = now() WHERE id = $1 AND used_at IS NULL
	`, id)
	return err
}

func (s *Store) InvalidateUserTokens(ctx context.Context, userID string, tokenType TokenType) error {
	_, err := s.db.Exec(ctx, `
		UPDATE recovery_tokens SET used_at = now() WHERE user_id = $1 AND type = $2 AND used_at IS NULL
	`, userID, string(tokenType))
	return err
}

func (s *Store) ListUserTokens(ctx context.Context, userID string, tokenType TokenType, limit int) ([]RecoveryToken, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, user_id::text, type, token_hash, expires_at, used_at, created_at, COALESCE(metadata, '{}'), COALESCE(ip::text, ''), COALESCE(user_agent, '')
		FROM recovery_tokens
		WHERE user_id = $1 AND type = $2
		ORDER BY created_at DESC
		LIMIT $3
	`, userID, string(tokenType), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []RecoveryToken
	for rows.Next() {
		var t RecoveryToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.Type, &t.TokenHash, &t.ExpiresAt, &t.UsedAt, &t.CreatedAt, &t.Metadata, &t.IP, &t.UserAgent); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (s *Store) CleanupExpiredTokens(ctx context.Context) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		DELETE FROM recovery_tokens WHERE expires_at < now() AND used_at IS NULL
	`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

var _ TokenStore = (*Store)(nil)

// TokenTTL returns the configured TTL for a given token type.
// Used for testing and introspection.
func (s *TokenService) TokenTTL(tokenType TokenType) time.Duration {
	return s.tokenTTL[tokenType]
}
