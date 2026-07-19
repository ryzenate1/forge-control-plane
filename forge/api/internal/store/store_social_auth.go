package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type SocialIdentity struct {
	ID           string    `json:"id"`
	UserID       string    `json:"userId"`
	Provider     string    `json:"provider"`
	ProviderID   string    `json:"providerId"`
	ProviderName string    `json:"providerName"`
	AvatarURL    *string   `json:"avatarUrl,omitempty"`
	ProfileURL   *string   `json:"profileUrl,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type SocialProvider struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	DisplayName  string    `json:"displayName"`
	Enabled      bool      `json:"enabled"`
	ClientID     string    `json:"clientId"`
	ClientSecret string    `json:"clientSecret,omitempty"`
	IssuerURL    string    `json:"issuerUrl,omitempty"`
	Scopes       []string  `json:"scopes"`
	ButtonStyle  string    `json:"buttonStyle"`
	IconClass    string    `json:"iconClass"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func (s *Store) GetSocialIdentity(ctx context.Context, provider, providerID string) (*SocialIdentity, error) {
	row := s.db.QueryRow(ctx, `
		SELECT si.id::text, si.user_id::text, si.provider, si.provider_id, si.provider_name,
		       si.avatar_url, si.profile_url, si.created_at, si.updated_at
		FROM social_identities si
		WHERE si.provider = $1 AND si.provider_id = $2
	`, provider, providerID)

	var si SocialIdentity
	err := row.Scan(&si.ID, &si.UserID, &si.Provider, &si.ProviderID, &si.ProviderName,
		&si.AvatarURL, &si.ProfileURL, &si.CreatedAt, &si.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &si, nil
}

func (s *Store) LinkSocialIdentity(ctx context.Context, userID, provider, providerID, providerName string, avatarURL, profileURL *string) (*SocialIdentity, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO social_identities (id, user_id, provider, provider_id, provider_name, avatar_url, profile_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (provider, provider_id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			provider_name = EXCLUDED.provider_name,
			avatar_url = EXCLUDED.avatar_url,
			profile_url = EXCLUDED.profile_url,
			updated_at = EXCLUDED.updated_at
	`, id, userID, provider, providerID, providerName, avatarURL, profileURL, now, now)
	if err != nil {
		return nil, err
	}
	return s.GetSocialIdentity(ctx, provider, providerID)
}

func (s *Store) UnlinkSocialIdentity(ctx context.Context, userID, provider string) error {
	_, err := s.db.Exec(ctx, `
		DELETE FROM social_identities WHERE user_id = $1 AND provider = $2
	`, userID, provider)
	return err
}

func (s *Store) ListSocialIdentities(ctx context.Context, userID string) ([]SocialIdentity, error) {
	rows, err := s.db.Query(ctx, `
		SELECT si.id::text, si.user_id::text, si.provider, si.provider_id, si.provider_name,
		       si.avatar_url, si.profile_url, si.created_at, si.updated_at
		FROM social_identities si
		WHERE si.user_id = $1
		ORDER BY si.provider
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var identities []SocialIdentity
	for rows.Next() {
		var si SocialIdentity
		if err := rows.Scan(&si.ID, &si.UserID, &si.Provider, &si.ProviderID, &si.ProviderName,
			&si.AvatarURL, &si.ProfileURL, &si.CreatedAt, &si.UpdatedAt); err != nil {
			return nil, err
		}
		identities = append(identities, si)
	}
	return identities, rows.Err()
}

func (s *Store) GetSocialProviders(ctx context.Context) ([]SocialProvider, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, display_name, enabled, client_id, client_secret, issuer_url,
		       scopes, button_style, icon_class, created_at, updated_at
		FROM social_providers
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var providers []SocialProvider
	for rows.Next() {
		var sp SocialProvider
		if err := rows.Scan(&sp.ID, &sp.Name, &sp.DisplayName, &sp.Enabled,
			&sp.ClientID, &sp.ClientSecret, &sp.IssuerURL, &sp.Scopes, &sp.ButtonStyle, &sp.IconClass,
			&sp.CreatedAt, &sp.UpdatedAt); err != nil {
			return nil, err
		}
		providers = append(providers, sp)
	}
	return providers, rows.Err()
}

func (s *Store) GetEnabledSocialProviders(ctx context.Context) ([]SocialProvider, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, display_name, enabled, client_id, client_secret, issuer_url,
		       scopes, button_style, icon_class, created_at, updated_at
		FROM social_providers
		WHERE enabled = true
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var providers []SocialProvider
	for rows.Next() {
		var sp SocialProvider
		if err := rows.Scan(&sp.ID, &sp.Name, &sp.DisplayName, &sp.Enabled,
			&sp.ClientID, &sp.ClientSecret, &sp.IssuerURL, &sp.Scopes, &sp.ButtonStyle, &sp.IconClass,
			&sp.CreatedAt, &sp.UpdatedAt); err != nil {
			return nil, err
		}
		providers = append(providers, sp)
	}
	return providers, rows.Err()
}

func (s *Store) UpdateSocialProvider(ctx context.Context, id string, enabled *bool, clientID, clientSecret, issuerURL *string, scopes []string) (*SocialProvider, error) {
	query := "UPDATE social_providers SET updated_at = $2"
	args := []any{id, time.Now().UTC()}
	argIdx := 3

	if enabled != nil {
		query += fmt.Sprintf(", enabled = $%d", argIdx)
		args = append(args, *enabled)
		argIdx++
	}
	if clientID != nil {
		query += fmt.Sprintf(", client_id = $%d", argIdx)
		args = append(args, *clientID)
		argIdx++
	}
	if clientSecret != nil {
		query += fmt.Sprintf(", client_secret = $%d", argIdx)
		args = append(args, *clientSecret)
		argIdx++
	}
	if issuerURL != nil {
		query += fmt.Sprintf(", issuer_url = $%d", argIdx)
		args = append(args, *issuerURL)
		argIdx++
	}
	if scopes != nil {
		query += fmt.Sprintf(", scopes = $%d", argIdx)
		args = append(args, scopes)
		argIdx++
	}

	query += " WHERE id = $1"

	_, err := s.db.Exec(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	row := s.db.QueryRow(ctx, `
		SELECT id::text, name, display_name, enabled, client_id, client_secret, issuer_url,
		       scopes, button_style, icon_class, created_at, updated_at
		FROM social_providers WHERE id = $1
	`, id)

	var sp SocialProvider
	err = row.Scan(&sp.ID, &sp.Name, &sp.DisplayName, &sp.Enabled,
		&sp.ClientID, &sp.ClientSecret, &sp.IssuerURL, &sp.Scopes, &sp.ButtonStyle, &sp.IconClass,
		&sp.CreatedAt, &sp.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &sp, nil
}
