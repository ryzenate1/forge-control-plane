package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type GitProviderType string

const (
	GitProviderGitHub    GitProviderType = "github"
	GitProviderGitLab    GitProviderType = "gitlab"
	GitProviderBitbucket GitProviderType = "bitbucket"
	GitProviderGitea     GitProviderType = "gitea"
	GitProviderGeneric   GitProviderType = "generic"
)

type GitProviderToken struct {
	ID           string          `json:"id"`
	UserID       string          `json:"userId"`
	Provider     GitProviderType `json:"provider"`
	ProviderName string          `json:"providerName"`
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

type CreateGitProviderTokenRequest struct {
	UserID       string
	Provider     GitProviderType
	ProviderName string
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

func (s *Store) ListGitProviderTokens(ctx context.Context, userID string) ([]GitProviderToken, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, provider, COALESCE(provider_name,''),
		       (COALESCE(access_token_plaintext,'') <> '' OR COALESCE(access_token_encrypted,'') <> ''),
		       COALESCE(token_type,'bearer'), expires_at, COALESCE(scope,''),
		       COALESCE(base_url,''), COALESCE(username,''), COALESCE(avatar_url,''),
		       COALESCE(metadata,'{}'::jsonb), created_at, updated_at
		FROM git_provider_tokens WHERE user_id = $1 ORDER BY provider, provider_name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := []GitProviderToken{}
	for rows.Next() {
		var pt GitProviderToken
		var hasToken bool
		if err := rows.Scan(&pt.ID, &pt.UserID, &pt.Provider, &pt.ProviderName,
			&hasToken, &pt.TokenType, &pt.ExpiresAt, &pt.Scope,
			&pt.BaseURL, &pt.Username, &pt.AvatarURL,
			&pt.Metadata, &pt.CreatedAt, &pt.UpdatedAt); err != nil {
			return nil, err
		}
		if hasToken {
			pt.AccessToken = maskedStoreSecret
		}
		tokens = append(tokens, pt)
	}
	return tokens, rows.Err()
}

func (s *Store) getGitProviderTokenInternal(ctx context.Context, id string) (GitProviderToken, error) {
	var pt GitProviderToken
	var accessPlain, accessEncrypted, refreshPlain, refreshEncrypted string
	err := s.db.QueryRow(ctx, `
		SELECT id, user_id, provider, COALESCE(provider_name,''),
		       COALESCE(access_token_plaintext,''), COALESCE(access_token_encrypted,''),
		       COALESCE(refresh_token_plaintext,''), COALESCE(refresh_token_encrypted,''),
		       COALESCE(token_type,'bearer'), expires_at, COALESCE(scope,''),
		       COALESCE(base_url,''), COALESCE(username,''), COALESCE(avatar_url,''),
		       COALESCE(metadata,'{}'::jsonb), created_at, updated_at
		FROM git_provider_tokens WHERE id = $1
	`, id).Scan(&pt.ID, &pt.UserID, &pt.Provider, &pt.ProviderName,
		&accessPlain, &accessEncrypted, &refreshPlain, &refreshEncrypted,
		&pt.TokenType, &pt.ExpiresAt, &pt.Scope,
		&pt.BaseURL, &pt.Username, &pt.AvatarURL,
		&pt.Metadata, &pt.CreatedAt, &pt.UpdatedAt)
	if err != nil {
		return GitProviderToken{}, errors.New("git provider token not found")
	}
	pt.AccessToken, err = s.decryptSecret(accessEncrypted, accessPlain, secretAAD("git_provider_tokens", pt.ID, "access_token"))
	if err != nil {
		return GitProviderToken{}, err
	}
	pt.RefreshToken, err = s.decryptSecret(refreshEncrypted, refreshPlain, secretAAD("git_provider_tokens", pt.ID, "refresh_token"))
	if err != nil {
		return GitProviderToken{}, err
	}
	return pt, nil
}

func (s *Store) GetGitProviderToken(ctx context.Context, id string) (GitProviderToken, error) {
	pt, err := s.getGitProviderTokenInternal(ctx, id)
	if err != nil {
		return GitProviderToken{}, err
	}
	if pt.AccessToken != "" {
		pt.AccessToken = maskedStoreSecret
	}
	if pt.RefreshToken != "" {
		pt.RefreshToken = maskedStoreSecret
	}
	return pt, nil
}

func (s *Store) CreateGitProviderToken(ctx context.Context, req CreateGitProviderTokenRequest) (GitProviderToken, error) {
	if req.Provider == "" {
		return GitProviderToken{}, errors.New("provider is required")
	}
	if strings.TrimSpace(req.AccessToken) == "" {
		return GitProviderToken{}, errors.New("accessToken is required")
	}
	if req.TokenType == "" {
		req.TokenType = "bearer"
	}
	if len(req.Metadata) == 0 {
		req.Metadata = json.RawMessage("{}")
	}

	id := uuid.NewString()
	accessEncrypted, err := s.encryptSecret(req.AccessToken, secretAAD("git_provider_tokens", id, "access_token"))
	if err != nil {
		return GitProviderToken{}, err
	}
	refreshEncrypted, err := s.encryptSecret(req.RefreshToken, secretAAD("git_provider_tokens", id, "refresh_token"))
	if err != nil {
		return GitProviderToken{}, err
	}

	now := time.Now().UTC()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO git_provider_tokens (id, user_id, provider, provider_name,
			access_token_encrypted, access_token_plaintext,
			refresh_token_encrypted, refresh_token_plaintext,
			token_type, expires_at, scope, base_url, username, avatar_url, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, '', $6, '', $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, id, req.UserID, req.Provider, req.ProviderName,
		accessEncrypted, refreshEncrypted,
		req.TokenType, req.ExpiresAt, req.Scope,
		req.BaseURL, req.Username, req.AvatarURL, req.Metadata, now, now); err != nil {
		return GitProviderToken{}, err
	}
	return s.GetGitProviderToken(ctx, id)
}

func (s *Store) GetGitProviderTokenUnmasked(ctx context.Context, id string) (GitProviderToken, error) {
	return s.getGitProviderTokenInternal(ctx, id)
}

func (s *Store) DeleteGitProviderToken(ctx context.Context, id string) error {
	cmd, err := s.db.Exec(ctx, `DELETE FROM git_provider_tokens WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("git provider token not found")
	}
	return nil
}
