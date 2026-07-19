package webauthn

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type PostgresCredentialStore struct {
	pool *pgxpool.Pool
}

func NewPostgresCredentialStore(pool *pgxpool.Pool) *PostgresCredentialStore {
	return &PostgresCredentialStore{pool: pool}
}

func (s *PostgresCredentialStore) GetCredentials(ctx context.Context, userID string) ([]WebAuthnCredential, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, user_id::text, credential_id, public_key,
		       attestation_type, aaguid, sign_count, clone_warning,
		       name, created_at, last_used_at
		FROM webauthn_credentials
		WHERE user_id = $1
		ORDER BY created_at ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []WebAuthnCredential
	for rows.Next() {
		var c WebAuthnCredential
		if err := rows.Scan(
			&c.ID, &c.UserID, &c.CredentialID, &c.PublicKey,
			&c.AttestationType, &c.AAGUID, &c.SignCount, &c.CloneWarning,
			&c.Name, &c.CreatedAt, &c.LastUsedAt,
		); err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

func (s *PostgresCredentialStore) SaveCredential(ctx context.Context, userID string, cred WebAuthnCredential) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO webauthn_credentials (id, user_id, credential_id, public_key,
		                                  attestation_type, aaguid, sign_count, clone_warning,
		                                  name, created_at, last_used_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, cred.ID, userID, cred.CredentialID, cred.PublicKey,
		cred.AttestationType, cred.AAGUID, cred.SignCount, cred.CloneWarning,
		cred.Name, cred.CreatedAt, cred.LastUsedAt)
	return err
}

func (s *PostgresCredentialStore) RemoveCredential(ctx context.Context, userID, credentialID string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM webauthn_credentials
		WHERE id = $1 AND user_id = $2
	`, credentialID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("credential not found")
	}
	return nil
}

type RedisSessionStore struct {
	client *redis.Client
}

func NewRedisSessionStore(client *redis.Client) *RedisSessionStore {
	return &RedisSessionStore{client: client}
}

func (s *RedisSessionStore) Save(ctx context.Context, key string, data []byte, expiry time.Duration) error {
	return s.client.Set(ctx, key, data, expiry).Err()
}

func (s *RedisSessionStore) Get(ctx context.Context, key string) ([]byte, error) {
	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, errors.New("session not found or expired")
		}
		return nil, err
	}
	return data, nil
}

func (s *RedisSessionStore) Delete(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}

var _ CredentialStore = (*PostgresCredentialStore)(nil)
var _ SessionStore = (*RedisSessionStore)(nil)
