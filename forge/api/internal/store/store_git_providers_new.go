package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// ErrGitProviderNotFound distinguishes a missing git_providers row from a
// store failure.
var ErrGitProviderNotFound = errors.New("git provider not found")

func (s *Store) ListGitProviders(ctx context.Context) ([]GitProvider, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id::text, name, type,
		       (COALESCE(access_token_encrypted,'') <> ''),
		       COALESCE(token_type,'bearer'), expires_at, COALESCE(scope,''),
		       COALESCE(base_url,''), COALESCE(username,''), COALESCE(avatar_url,''),
		       COALESCE(metadata,'{}'::jsonb), created_at, updated_at
		FROM git_providers ORDER BY name LIMIT 500
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

// getGitProviderInternal decrypts the token pair. git_providers has no
// *_plaintext columns (unlike git_provider_tokens), so reading them used to
// fail the query outright.
func (s *Store) getGitProviderInternal(ctx context.Context, id string) (GitProvider, error) {
	var p GitProvider
	var accessEncrypted, refreshEncrypted string
	err := s.db.QueryRow(ctx, `
		SELECT id, user_id::text, name, type,
		       COALESCE(access_token_encrypted,''), COALESCE(refresh_token_encrypted,''),
		       COALESCE(token_type,'bearer'), expires_at, COALESCE(scope,''),
		       COALESCE(base_url,''), COALESCE(username,''), COALESCE(avatar_url,''),
		       COALESCE(metadata,'{}'::jsonb), created_at, updated_at
		FROM git_providers WHERE id = $1
	`, id).Scan(&p.ID, &p.UserID, &p.Name, &p.Type,
		&accessEncrypted, &refreshEncrypted,
		&p.TokenType, &p.ExpiresAt, &p.Scope,
		&p.BaseURL, &p.Username, &p.AvatarURL,
		&p.Metadata, &p.CreatedAt, &p.UpdatedAt)
	if isGitNoRows(err) {
		return GitProvider{}, ErrGitProviderNotFound
	}
	if err != nil {
		return GitProvider{}, fmt.Errorf("git provider lookup: %w", err)
	}
	p.AccessToken, err = s.decryptSecret(accessEncrypted, "", secretAAD("git_providers", p.ID, "access_token"))
	if err != nil {
		return GitProvider{}, fmt.Errorf("git provider %s: %w", p.ID, err)
	}
	p.RefreshToken, err = s.decryptSecret(refreshEncrypted, "", secretAAD("git_providers", p.ID, "refresh_token"))
	if err != nil {
		return GitProvider{}, fmt.Errorf("git provider %s: %w", p.ID, err)
	}
	return p, nil
}

// GetGitProvider is the read path for responses: both token columns come back
// masked, never decrypted, so a caller cannot harvest another integration's
// credentials through a list/get endpoint.
func (s *Store) GetGitProvider(ctx context.Context, id string) (GitProvider, error) {
	p, err := s.getGitProviderInternal(ctx, id)
	if err != nil {
		return GitProvider{}, err
	}
	if p.AccessToken != "" {
		p.AccessToken = maskedStoreSecret
	}
	if p.RefreshToken != "" {
		p.RefreshToken = maskedStoreSecret
	}
	return p, nil
}

// GetGitProviderUnmasked hands the decrypted token to provider API callers
// only; it must never be marshalled into an API response.
func (s *Store) GetGitProviderUnmasked(ctx context.Context, id string) (GitProvider, error) {
	return s.getGitProviderInternal(ctx, id)
}

func (s *Store) CreateGitProvider(ctx context.Context, req CreateGitProviderRequest) (GitProvider, error) {
	switch req.Type {
	case GitProviderGitHub, GitProviderGitLab, GitProviderBitbucket, GitProviderGitea, GitProviderGeneric:
	default:
		return GitProvider{}, fmt.Errorf("unsupported provider type %q", req.Type)
	}
	if strings.TrimSpace(req.UserID) == "" {
		return GitProvider{}, errors.New("userId is required")
	}
	if strings.TrimSpace(req.Name) == "" {
		return GitProvider{}, errors.New("name is required")
	}
	if strings.TrimSpace(req.AccessToken) == "" {
		return GitProvider{}, errors.New("accessToken is required")
	}
	if req.AccessToken == maskedStoreSecret || req.RefreshToken == maskedStoreSecret {
		return GitProvider{}, errors.New("accessToken is masked; supply the real token")
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
			access_token_encrypted,
			refresh_token_encrypted,
			token_type, expires_at, scope, base_url, username, avatar_url, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, id, req.UserID, req.Name, req.Type,
		accessEncrypted, refreshEncrypted,
		req.TokenType, req.ExpiresAt, req.Scope,
		req.BaseURL, req.Username, req.AvatarURL, req.Metadata, now, now)
	if err != nil {
		return GitProvider{}, err
	}
	return s.GetGitProvider(ctx, id)
}

func (s *Store) DeleteGitProvider(ctx context.Context, id string) error {
	cmd, err := s.db.Exec(ctx, `DELETE FROM git_providers WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrGitProviderNotFound
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
