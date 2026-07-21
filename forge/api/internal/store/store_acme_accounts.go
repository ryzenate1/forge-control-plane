package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

type AcmeAccount struct {
	ID         string    `json:"id"`
	Email      string    `json:"email"`
	PrivateKey string    `json:"-"`
	CAURL      string    `json:"caUrl"`
	IsDefault  bool      `json:"isDefault"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type CreateAcmeAccountRequest struct {
	Email      string
	PrivateKey string
	CAURL      string
}

type UpdateAcmeAccountRequest struct {
	Email      *string
	PrivateKey *string
	CAURL      *string
	IsDefault  *bool
}

func (s *Store) ListAcmeAccounts(ctx context.Context) ([]AcmeAccount, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, email, COALESCE(ca_url,'https://acme-v02.api.letsencrypt.org/directory'),
		       is_default, created_at, updated_at
		FROM acme_accounts ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accounts []AcmeAccount
	for rows.Next() {
		var a AcmeAccount
		if err := rows.Scan(&a.ID, &a.Email, &a.CAURL, &a.IsDefault, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (s *Store) GetAcmeAccount(ctx context.Context, id string) (AcmeAccount, error) {
	var a AcmeAccount
	err := s.db.QueryRow(ctx, `
		SELECT id::text, email, COALESCE(private_key,''), COALESCE(ca_url,'https://acme-v02.api.letsencrypt.org/directory'),
		       is_default, created_at, updated_at
		FROM acme_accounts WHERE id::text = $1
	`, id).Scan(&a.ID, &a.Email, &a.PrivateKey, &a.CAURL, &a.IsDefault, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return AcmeAccount{}, errors.New("acme account not found")
	}
	return a, nil
}

func (s *Store) CreateAcmeAccount(ctx context.Context, req CreateAcmeAccountRequest) (AcmeAccount, error) {
	if req.Email == "" {
		return AcmeAccount{}, errors.New("email is required")
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	if req.CAURL == "" {
		req.CAURL = "https://acme-v02.api.letsencrypt.org/directory"
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO acme_accounts (id, email, private_key, ca_url, is_default, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, id, req.Email, req.PrivateKey, req.CAURL, false, now, now)
	if err != nil {
		return AcmeAccount{}, err
	}
	return AcmeAccount{
		ID: id, Email: req.Email, PrivateKey: req.PrivateKey,
		CAURL: req.CAURL, IsDefault: false, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (s *Store) UpdateAcmeAccount(ctx context.Context, id string, req UpdateAcmeAccountRequest) (AcmeAccount, error) {
	updates := []string{}
	args := []any{}
	if req.Email != nil {
		updates = append(updates, "email = $"+itoa(len(args)+1))
		args = append(args, *req.Email)
	}
	if req.PrivateKey != nil {
		updates = append(updates, "private_key = $"+itoa(len(args)+1))
		args = append(args, *req.PrivateKey)
	}
	if req.CAURL != nil {
		updates = append(updates, "ca_url = $"+itoa(len(args)+1))
		args = append(args, *req.CAURL)
	}
	if req.IsDefault != nil {
		updates = append(updates, "is_default = $"+itoa(len(args)+1))
		args = append(args, *req.IsDefault)
	}
	updates = append(updates, "updated_at = now()")
	args = append(args, id)

	if len(updates) == 1 {
		return s.GetAcmeAccount(ctx, id)
	}

	q := "UPDATE acme_accounts SET " + updates[0]
	for i := 1; i < len(updates); i++ {
		q += ", " + updates[i]
	}
	q += " WHERE id::text = $" + itoa(len(args))

	if _, err := s.db.Exec(ctx, q, args...); err != nil {
		return AcmeAccount{}, err
	}
	return s.GetAcmeAccount(ctx, id)
}

func (s *Store) DeleteAcmeAccount(ctx context.Context, id string) error {
	cmd, err := s.db.Exec(ctx, `DELETE FROM acme_accounts WHERE id::text = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("acme account not found")
	}
	return nil
}

type DNSProviderAccount struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Provider    string          `json:"provider"`
	Credentials json.RawMessage `json:"credentials,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

type CreateDNSProviderAccountRequest struct {
	Name        string
	Provider    string
	Credentials json.RawMessage
}

type UpdateDNSProviderAccountRequest struct {
	Name        *string
	Provider    *string
	Credentials *json.RawMessage
}

func (s *Store) ListDNSProviderAccounts(ctx context.Context, provider string) ([]DNSProviderAccount, error) {
	query := `SELECT id::text, name, provider, credentials, created_at, updated_at FROM dns_provider_accounts`
	args := []any{}
	if provider != "" {
		query += " WHERE provider = $1"
		args = append(args, provider)
	}
	query += " ORDER BY created_at DESC"
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accounts []DNSProviderAccount
	for rows.Next() {
		var a DNSProviderAccount
		if err := rows.Scan(&a.ID, &a.Name, &a.Provider, &a.Credentials, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (s *Store) GetDNSProviderAccount(ctx context.Context, id string) (DNSProviderAccount, error) {
	var a DNSProviderAccount
	err := s.db.QueryRow(ctx, `
		SELECT id::text, name, provider, credentials, created_at, updated_at
		FROM dns_provider_accounts WHERE id::text = $1
	`, id).Scan(&a.ID, &a.Name, &a.Provider, &a.Credentials, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return DNSProviderAccount{}, errors.New("dns provider account not found")
	}
	return a, nil
}

func (s *Store) CreateDNSProviderAccount(ctx context.Context, req CreateDNSProviderAccountRequest) (DNSProviderAccount, error) {
	if req.Name == "" || req.Provider == "" {
		return DNSProviderAccount{}, errors.New("name and provider are required")
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	creds := req.Credentials
	if creds == nil {
		creds = json.RawMessage("{}")
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO dns_provider_accounts (id, name, provider, credentials, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, req.Name, req.Provider, creds, now, now)
	if err != nil {
		return DNSProviderAccount{}, err
	}
	return DNSProviderAccount{
		ID: id, Name: req.Name, Provider: req.Provider,
		Credentials: creds, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (s *Store) UpdateDNSProviderAccount(ctx context.Context, id string, req UpdateDNSProviderAccountRequest) (DNSProviderAccount, error) {
	updates := []string{}
	args := []any{}
	if req.Name != nil {
		updates = append(updates, "name = $"+itoa(len(args)+1))
		args = append(args, *req.Name)
	}
	if req.Provider != nil {
		updates = append(updates, "provider = $"+itoa(len(args)+1))
		args = append(args, *req.Provider)
	}
	if req.Credentials != nil {
		updates = append(updates, "credentials = $"+itoa(len(args)+1))
		args = append(args, *req.Credentials)
	}
	updates = append(updates, "updated_at = now()")
	args = append(args, id)

	if len(updates) == 1 {
		return s.GetDNSProviderAccount(ctx, id)
	}

	q := "UPDATE dns_provider_accounts SET " + updates[0]
	for i := 1; i < len(updates); i++ {
		q += ", " + updates[i]
	}
	q += " WHERE id::text = $" + itoa(len(args))

	if _, err := s.db.Exec(ctx, q, args...); err != nil {
		return DNSProviderAccount{}, err
	}
	return s.GetDNSProviderAccount(ctx, id)
}

func (s *Store) DeleteDNSProviderAccount(ctx context.Context, id string) error {
	cmd, err := s.db.Exec(ctx, `DELETE FROM dns_provider_accounts WHERE id::text = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("dns provider account not found")
	}
	return nil
}
