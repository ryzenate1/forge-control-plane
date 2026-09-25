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
	var privateKeyEncrypted string
	err := s.db.QueryRow(ctx, `
		SELECT id::text, email, COALESCE(private_key,''), COALESCE(private_key_encrypted, ''),
		       COALESCE(ca_url,'https://acme-v02.api.letsencrypt.org/directory'),
		       is_default, created_at, updated_at
		FROM acme_accounts WHERE id::text = $1
	`, id).Scan(&a.ID, &a.Email, &a.PrivateKey, &privateKeyEncrypted, &a.CAURL, &a.IsDefault, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return AcmeAccount{}, errors.New("acme account not found")
	}
	a.PrivateKey, err = s.decryptSecret(privateKeyEncrypted, a.PrivateKey, secretAAD("acme_accounts", a.ID, "private_key"))
	if err != nil {
		return AcmeAccount{}, err
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
	privateKeyEncrypted, err := s.encryptSecret(req.PrivateKey, secretAAD("acme_accounts", id, "private_key"))
	if err != nil {
		return AcmeAccount{}, err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO acme_accounts (id, email, private_key, private_key_encrypted, ca_url, is_default, created_at, updated_at)
		VALUES ($1, $2, '', $3, $4, $5, $6, $7)
	`, id, req.Email, privateKeyEncrypted, req.CAURL, false, now, now)
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
		privateKeyEncrypted, err := s.encryptSecret(*req.PrivateKey, secretAAD("acme_accounts", id, "private_key"))
		if err != nil {
			return AcmeAccount{}, err
		}
		updates = append(updates, "private_key = '', private_key_encrypted = $"+itoa(len(args)+1))
		args = append(args, privateKeyEncrypted)
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

	var allowedAcmeAccountColumns = map[string]bool{
		"email": true, "private_key": true, "ca_url": true, "is_default": true, "updated_at": true,
	}
	for _, u := range updates {
		col := strings.SplitN(u, " =", 2)[0]
		if !allowedAcmeAccountColumns[col] {
			return AcmeAccount{}, fmt.Errorf("disallowed column: %s", col)
		}
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
	query := `SELECT id::text, name, provider, COALESCE(credentials_encrypted,''), COALESCE(credentials::text,'{}'), created_at, updated_at FROM dns_provider_accounts`
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
		var credentialsEncrypted, plainForDecrypt string
		if err := rows.Scan(&a.ID, &a.Name, &a.Provider, &credentialsEncrypted, &plainForDecrypt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		credentials, err := s.dnsProviderCredentials(a.ID, credentialsEncrypted, plainForDecrypt)
		if err != nil {
			return nil, err
		}
		a.Credentials = credentials
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

// dnsProviderCredentials dual-reads a DNS provider account's credentials: the
// secret-AAD envelope first, the legacy plaintext jsonb column as fallback.
// An envelope that cannot be opened is an error, never a silent empty set.
func (s *Store) dnsProviderCredentials(id string, envelope, plaintext string) (json.RawMessage, error) {
	credentialsJSON, err := s.decryptSecret(envelope, plaintext, secretAAD("dns_provider_accounts", id, "credentials"))
	if err != nil {
		return nil, err
	}
	if credentialsJSON == "" {
		credentialsJSON = "{}"
	}
	if !json.Valid([]byte(credentialsJSON)) {
		return nil, fmt.Errorf("dns provider account %s contains invalid credentials JSON", id)
	}
	return json.RawMessage(credentialsJSON), nil
}

func (s *Store) GetDNSProviderAccount(ctx context.Context, id string) (DNSProviderAccount, error) {
	var a DNSProviderAccount
	var credentialsEncrypted, plainForDecrypt string
	err := s.db.QueryRow(ctx, `
		SELECT id::text, name, provider, COALESCE(credentials_encrypted,''), COALESCE(credentials::text,'{}'), created_at, updated_at
		FROM dns_provider_accounts WHERE id::text = $1
	`, id).Scan(&a.ID, &a.Name, &a.Provider, &credentialsEncrypted, &plainForDecrypt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return DNSProviderAccount{}, errors.New("dns provider account not found")
	}
	credentials, err := s.dnsProviderCredentials(a.ID, credentialsEncrypted, plainForDecrypt)
	if err != nil {
		return DNSProviderAccount{}, err
	}
	a.Credentials = credentials
	return a, nil
}

func (s *Store) CreateDNSProviderAccount(ctx context.Context, req CreateDNSProviderAccountRequest) (DNSProviderAccount, error) {
	if req.Name == "" || req.Provider == "" {
		return DNSProviderAccount{}, errors.New("name and provider are required")
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	creds := req.Credentials
	if len(creds) == 0 {
		creds = json.RawMessage("{}")
	}

	// Credentials are secrets: encrypt into the envelope column and keep the
	// plaintext jsonb column cleared (dual-write, migration 213).
	var credentialsEncrypted string
	if string(creds) != "{}" {
		var err error
		credentialsEncrypted, err = s.encryptSecret(string(creds), secretAAD("dns_provider_accounts", id, "credentials"))
		if err != nil {
			return DNSProviderAccount{}, err
		}
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO dns_provider_accounts (id, name, provider, credentials, credentials_encrypted, created_at, updated_at)
		VALUES ($1, $2, $3, '{}'::jsonb, $4, $5, $6)
	`, id, req.Name, req.Provider, credentialsEncrypted, now, now)
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
		creds := string(*req.Credentials)
		if creds == "" {
			creds = "{}"
		}
		credentialsEncrypted := ""
		if creds != "{}" {
			encrypted, err := s.encryptSecret(creds, secretAAD("dns_provider_accounts", id, "credentials"))
			if err != nil {
				return DNSProviderAccount{}, err
			}
			credentialsEncrypted = encrypted
		}
		updates = append(updates, "credentials = '{}'::jsonb, credentials_encrypted = $"+itoa(len(args)+1))
		args = append(args, credentialsEncrypted)
	}
	updates = append(updates, "updated_at = now()")
	args = append(args, id)

	if len(updates) == 1 {
		return s.GetDNSProviderAccount(ctx, id)
	}

	var allowedDNSProviderColumns = map[string]bool{
		"name": true, "provider": true, "credentials": true,
		"credentials_encrypted": true, "updated_at": true,
	}
	for _, u := range updates {
		col := strings.SplitN(u, " =", 2)[0]
		if !allowedDNSProviderColumns[col] {
			return DNSProviderAccount{}, fmt.Errorf("disallowed column: %s", col)
		}
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
