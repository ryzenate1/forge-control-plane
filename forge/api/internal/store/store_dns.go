package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

type DNSProvider struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	ProviderType  string          `json:"providerType"`
	Credentials   json.RawMessage `json:"credentials,omitempty"`
	IsDefault     bool            `json:"isDefault"`
	Verified      bool            `json:"verified"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

type UpsertDNSProviderRequest struct {
	Name         string
	ProviderType string
	Credentials  json.RawMessage
}

func (s *Store) ListDNSProviders(ctx context.Context) ([]DNSProvider, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, provider_type,
		       CASE WHEN credentials_encrypted <> '' THEN '{"encrypted":true}'::jsonb ELSE credentials::jsonb END,
		       is_default, verified, created_at, updated_at
		FROM dns_providers
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	providers := []DNSProvider{}
	for rows.Next() {
		var p DNSProvider
		if err := rows.Scan(&p.ID, &p.Name, &p.ProviderType, &p.Credentials, &p.IsDefault, &p.Verified, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		providers = append(providers, p)
	}
	return providers, rows.Err()
}

func (s *Store) GetDNSProvider(ctx context.Context, id string) (DNSProvider, error) {
	var p DNSProvider
	var plaintext, encrypted string
	err := s.db.QueryRow(ctx, `
		SELECT id, name, provider_type,
		       COALESCE(credentials::text, ''),
		       COALESCE(credentials_encrypted, ''),
		       is_default, verified, created_at, updated_at
		FROM dns_providers WHERE id = $1
	`, id).Scan(&p.ID, &p.Name, &p.ProviderType, &plaintext, &encrypted, &p.IsDefault, &p.Verified, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return DNSProvider{}, err
	}
	decrypted, err := s.decryptSecret(encrypted, plaintext, secretAAD("dns_providers", p.ID, "credentials"))
	if err != nil {
		return DNSProvider{}, err
	}
	if decrypted != "" {
		p.Credentials = json.RawMessage(decrypted)
	}
	return p, nil
}

func (s *Store) UpsertDNSProvider(ctx context.Context, req UpsertDNSProviderRequest) (DNSProvider, error) {
	if req.Name == "" || req.ProviderType == "" {
		return DNSProvider{}, errors.New("name and provider type are required")
	}
	id := uuid.NewString()
	raw, err := json.Marshal(req.Credentials)
	if err != nil {
		return DNSProvider{}, err
	}
	encrypted, err := s.encryptSecret(string(raw), secretAAD("dns_providers", id, "credentials"))
	if err != nil {
		return DNSProvider{}, err
	}
	now := time.Now().UTC()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO dns_providers (id, name, provider_type, credentials, credentials_encrypted, is_default, verified, created_at, updated_at)
		VALUES ($1, $2, $3, '', $4, FALSE, FALSE, $5, $5)
	`, id, req.Name, req.ProviderType, encrypted, now); err != nil {
		return DNSProvider{}, err
	}
	return s.GetDNSProvider(ctx, id)
}

func (s *Store) UpdateDNSProvider(ctx context.Context, id string, req UpsertDNSProviderRequest) (DNSProvider, error) {
	existing, err := s.GetDNSProvider(ctx, id)
	if err != nil {
		return DNSProvider{}, err
	}
	if req.Name == "" {
		req.Name = existing.Name
	}
	if req.ProviderType == "" {
		req.ProviderType = existing.ProviderType
	}
	if req.Credentials == nil {
		req.Credentials = existing.Credentials
	}
	raw, err := json.Marshal(req.Credentials)
	if err != nil {
		return DNSProvider{}, err
	}
	encrypted, err := s.encryptSecret(string(raw), secretAAD("dns_providers", id, "credentials"))
	if err != nil {
		return DNSProvider{}, err
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE dns_providers SET name=$1, provider_type=$2, credentials='', credentials_encrypted=$3, updated_at=$4
		WHERE id=$5
	`, req.Name, req.ProviderType, encrypted, time.Now().UTC(), id); err != nil {
		return DNSProvider{}, err
	}
	return s.GetDNSProvider(ctx, id)
}

func (s *Store) DeleteDNSProvider(ctx context.Context, id string) error {
	cmd, err := s.db.Exec(ctx, `DELETE FROM dns_providers WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("dns provider not found")
	}
	return nil
}

func (s *Store) SetDefaultDNSProvider(ctx context.Context, id string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE dns_providers SET is_default = FALSE, updated_at = NOW()`); err != nil {
		return err
	}
	cmd, err := tx.Exec(ctx, `UPDATE dns_providers SET is_default = TRUE, updated_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("dns provider not found")
	}
	return tx.Commit(ctx)
}

func (s *Store) GetDefaultDNSProvider(ctx context.Context) (DNSProvider, error) {
	var p DNSProvider
	var plaintext, encrypted string
	err := s.db.QueryRow(ctx, `
		SELECT id, name, provider_type,
		       COALESCE(credentials::text, ''),
		       COALESCE(credentials_encrypted, ''),
		       is_default, verified, created_at, updated_at
		FROM dns_providers WHERE is_default = TRUE LIMIT 1
	`).Scan(&p.ID, &p.Name, &p.ProviderType, &plaintext, &encrypted, &p.IsDefault, &p.Verified, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return DNSProvider{}, err
	}
	decrypted, err := s.decryptSecret(encrypted, plaintext, secretAAD("dns_providers", p.ID, "credentials"))
	if err != nil {
		return DNSProvider{}, err
	}
	if decrypted != "" {
		p.Credentials = json.RawMessage(decrypted)
	}
	return p, nil
}

func (s *Store) MarkDNSProviderVerified(ctx context.Context, id string, verified bool) error {
	cmd, err := s.db.Exec(ctx, `UPDATE dns_providers SET verified = $1, updated_at = NOW() WHERE id = $2`, verified, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("dns provider not found")
	}
	return nil
}
