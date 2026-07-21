package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type GitProvider struct {
	ID           string          `json:"id"`
	UserID       string          `json:"userId"`
	Name         string          `json:"name"`
	Type         GitProviderType `json:"type"`
	AccessToken  string          `json:"accessToken,omitempty"`
	RefreshToken string          `json:"refreshToken,omitempty"`
	TokenType    string          `json:"tokenType"`
	ExpiresAt    *time.Time      `json:"expiresAt,omitempty"`
	Scope        string          `json:"scope"`
	BaseURL      string          `json:"baseUrl"`
	Username     string          `json:"username"`
	AvatarURL    string          `json:"avatarUrl"`
	Metadata     json.RawMessage `json:"metadata"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
}

func (s *Store) ListGitProviders(ctx context.Context) ([]GitProvider, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id::text, name, type,
		       (COALESCE(access_token_plaintext,'') <> '' OR COALESCE(access_token_encrypted,'') <> ''),
		       COALESCE(token_type,'bearer'), expires_at, COALESCE(scope,''),
		       COALESCE(base_url,''), COALESCE(username,''), COALESCE(avatar_url,''),
		       COALESCE(metadata,'{}'::jsonb), created_at, updated_at
		FROM git_providers ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var providers []GitProvider
	for rows.Next() {
		var p GitProvider
		var hasToken bool
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Type,
			&hasToken, &p.TokenType, &p.ExpiresAt, &p.Scope,
			&p.BaseURL, &p.Username, &p.AvatarURL,
			&p.Metadata, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if hasToken {
			p.AccessToken = maskedStoreSecret
		}
		providers = append(providers, p)
	}
	return providers, rows.Err()
}

func (s *Store) GetGitProvider(ctx context.Context, id string) (GitProvider, error) {
	var p GitProvider
	var accessPlain, accessEncrypted, refreshPlain, refreshEncrypted string
	err := s.db.QueryRow(ctx, `
		SELECT id, user_id::text, name, type,
		       COALESCE(access_token_plaintext,''), COALESCE(access_token_encrypted,''),
		       COALESCE(refresh_token_plaintext,''), COALESCE(refresh_token_encrypted,''),
		       COALESCE(token_type,'bearer'), expires_at, COALESCE(scope,''),
		       COALESCE(base_url,''), COALESCE(username,''), COALESCE(avatar_url,''),
		       COALESCE(metadata,'{}'::jsonb), created_at, updated_at
		FROM git_providers WHERE id = $1
	`, id).Scan(&p.ID, &p.UserID, &p.Name, &p.Type,
		&accessPlain, &accessEncrypted, &refreshPlain, &refreshEncrypted,
		&p.TokenType, &p.ExpiresAt, &p.Scope,
		&p.BaseURL, &p.Username, &p.AvatarURL,
		&p.Metadata, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return GitProvider{}, errors.New("git provider not found")
	}
	plainToken, err := s.decryptSecret(accessEncrypted, accessPlain, secretAAD("git_providers", p.ID, "access_token"))
	if err != nil {
		return GitProvider{}, err
	}
	p.AccessToken = plainToken
	refreshToken, err := s.decryptSecret(refreshEncrypted, refreshPlain, secretAAD("git_providers", p.ID, "refresh_token"))
	if err == nil {
		p.RefreshToken = refreshToken
	}
	return p, nil
}

func (s *Store) CreateGitProvider(ctx context.Context, req CreateGitProviderRequest) (GitProvider, error) {
	if strings.TrimSpace(req.Name) == "" {
		return GitProvider{}, errors.New("name is required")
	}
	if req.Type == "" {
		return GitProvider{}, errors.New("type is required")
	}
	if strings.TrimSpace(req.AccessToken) == "" {
		return GitProvider{}, errors.New("accessToken is required")
	}
	if req.TokenType == "" {
		req.TokenType = "bearer"
	}
	if len(req.Metadata) == 0 {
		req.Metadata = json.RawMessage("{}")
	}

	id := uuid.NewString()
	accessEncrypted, err := s.encryptSecret(req.AccessToken, secretAAD("git_providers", id, "access_token"))
	if err != nil {
		return GitProvider{}, err
	}
	refreshEncrypted, err := s.encryptSecret(req.RefreshToken, secretAAD("git_providers", id, "refresh_token"))
	if err != nil {
		return GitProvider{}, err
	}

	now := time.Now().UTC()
	_, err = s.db.Exec(ctx, `
		INSERT INTO git_providers (id, user_id, name, type,
			access_token_encrypted, access_token_plaintext,
			refresh_token_encrypted, refresh_token_plaintext,
			token_type, expires_at, scope, base_url, username, avatar_url, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, '', $6, '', $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, id, req.UserID, req.Name, req.Type,
		accessEncrypted, refreshEncrypted,
		req.TokenType, req.ExpiresAt, req.Scope,
		req.BaseURL, req.Username, req.AvatarURL, req.Metadata, now, now)
	if err != nil {
		return GitProvider{}, err
	}
	p, err := s.GetGitProvider(ctx, id)
	if err != nil {
		return GitProvider{}, err
	}
	p.AccessToken = maskedStoreSecret
	return p, nil
}

func (s *Store) DeleteGitProvider(ctx context.Context, id string) error {
	cmd, err := s.db.Exec(ctx, `DELETE FROM git_providers WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("git provider not found")
	}
	return nil
}

type CreateGitProviderRequest struct {
	UserID       string
	Name         string
	Type         GitProviderType
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresAt    *time.Time
	Scope        string
	BaseURL      string
	Username     string
	AvatarURL    string
	Metadata     json.RawMessage
}
