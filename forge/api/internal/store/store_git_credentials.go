package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type GitCredentialType string

const (
	GitCredentialSSHKey       GitCredentialType = "ssh_key"
	GitCredentialHTTPSPass    GitCredentialType = "https_password"
	GitCredentialHTTPSToken   GitCredentialType = "https_token"
	maskedGitSecret           string            = "********"
)

// ErrGitCredentialNotFound lets handlers answer 404 without echoing the
// driver's text; every other failure stays distinguishable from "missing".
var ErrGitCredentialNotFound = errors.New("git credential not found")

func isGitNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows)
}

type GitCredential struct {
	ID             string            `json:"id"`
	UserID         string            `json:"userId"`
	Name           string            `json:"name"`
	CredentialType GitCredentialType `json:"credentialType"`
	Credential     string            `json:"credential,omitempty"`
	PublicKey      string            `json:"publicKey"`
	Description    string            `json:"description"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
}

type CreateGitCredentialRequest struct {
	UserID         string
	Name           string
	CredentialType GitCredentialType
	Credential     string
	PublicKey      string
	Description    string
}

func (s *Store) ListGitCredentials(ctx context.Context, userID string) ([]GitCredential, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, name, credential_type,
		       (COALESCE(credential_plaintext,'') <> '' OR COALESCE(credential_encrypted,'') <> ''),
		       COALESCE(public_key,''), COALESCE(description,''), created_at, updated_at
		FROM git_credentials WHERE user_id = $1 ORDER BY name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	creds := []GitCredential{}
	for rows.Next() {
		var gc GitCredential
		var hasCredential bool
		if err := rows.Scan(&gc.ID, &gc.UserID, &gc.Name, &gc.CredentialType,
			&hasCredential, &gc.PublicKey, &gc.Description,
			&gc.CreatedAt, &gc.UpdatedAt); err != nil {
			return nil, err
		}
		if hasCredential {
			gc.Credential = maskedGitSecret
		}
		creds = append(creds, gc)
	}
	return creds, rows.Err()
}

func (s *Store) getGitCredentialInternal(ctx context.Context, id string) (GitCredential, error) {
	var gc GitCredential
	var plaintext, encrypted string
	err := s.db.QueryRow(ctx, `
		SELECT id, user_id, name, credential_type,
		       COALESCE(credential_plaintext,''), COALESCE(credential_encrypted,''),
		       COALESCE(public_key,''), COALESCE(description,''), created_at, updated_at
		FROM git_credentials WHERE id = $1
	`, id).Scan(&gc.ID, &gc.UserID, &gc.Name, &gc.CredentialType,
		&plaintext, &encrypted, &gc.PublicKey, &gc.Description,
		&gc.CreatedAt, &gc.UpdatedAt)
	if isGitNoRows(err) {
		return GitCredential{}, ErrGitCredentialNotFound
	}
	if err != nil {
		return GitCredential{}, fmt.Errorf("git credential lookup: %w", err)
	}
	gc.Credential, err = s.decryptSecret(encrypted, plaintext, secretAAD("git_credentials", gc.ID, "credential"))
	if err != nil {
		return GitCredential{}, fmt.Errorf("git credential %s: %w", gc.ID, err)
	}
	return gc, nil
}

func (s *Store) GetGitCredential(ctx context.Context, id string) (GitCredential, error) {
	gc, err := s.getGitCredentialInternal(ctx, id)
	if err != nil {
		return GitCredential{}, err
	}
	if gc.Credential != "" {
		gc.Credential = maskedGitSecret
	}
	return gc, nil
}

func (s *Store) GetGitCredentialUnmasked(ctx context.Context, id string) (GitCredential, error) {
	return s.getGitCredentialInternal(ctx, id)
}

func (s *Store) CreateGitCredential(ctx context.Context, req CreateGitCredentialRequest) (GitCredential, error) {
	if strings.TrimSpace(req.UserID) == "" {
		return GitCredential{}, errors.New("userId is required")
	}
	if strings.TrimSpace(req.Name) == "" {
		return GitCredential{}, errors.New("name is required")
	}
	if req.CredentialType != GitCredentialSSHKey && req.CredentialType != GitCredentialHTTPSPass && req.CredentialType != GitCredentialHTTPSToken {
		return GitCredential{}, errors.New("credentialType must be ssh_key, https_password, or https_token")
	}
	if req.CredentialType == GitCredentialSSHKey && strings.TrimSpace(req.Credential) == "" {
		return GitCredential{}, errors.New("credential (private key) is required for ssh_key type")
	}
	if (req.CredentialType == GitCredentialHTTPSPass || req.CredentialType == GitCredentialHTTPSToken) && strings.TrimSpace(req.Credential) == "" {
		return GitCredential{}, errors.New("credential is required")
	}
	// A client that round-trips a masked read back into create would store the
	// mask itself as the secret; every later clone then fails on authentication.
	if strings.TrimSpace(req.Credential) == maskedGitSecret {
		return GitCredential{}, errors.New("credential is masked; supply the real secret")
	}

	id := uuid.NewString()
	encrypted, err := s.encryptSecret(req.Credential, secretAAD("git_credentials", id, "credential"))
	if err != nil {
		return GitCredential{}, err
	}

	now := time.Now().UTC()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO git_credentials (id, user_id, name, credential_type, credential_encrypted, credential_plaintext, public_key, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, '', $6, $7, $8, $9)
	`, id, req.UserID, strings.TrimSpace(req.Name), req.CredentialType,
		encrypted, req.PublicKey, req.Description, now, now); err != nil {
		return GitCredential{}, err
	}
	return s.GetGitCredential(ctx, id)
}

// UpdateGitCredentialSecret rotates the secret of an existing credential in
// place. Callers must use this instead of delete+recreate: git_sources and
// server rows reference credentials by id, and a recreated row lands under a
// new id whose FK silently falls to NULL.
func (s *Store) UpdateGitCredentialSecret(ctx context.Context, id, credential, publicKey string) (GitCredential, error) {
	if strings.TrimSpace(credential) == "" {
		return GitCredential{}, errors.New("credential is required")
	}
	if strings.TrimSpace(credential) == maskedGitSecret {
		return GitCredential{}, errors.New("credential is masked; supply the real secret")
	}
	existing, err := s.getGitCredentialInternal(ctx, id)
	if err != nil {
		return GitCredential{}, err
	}
	encrypted, err := s.encryptSecret(credential, secretAAD("git_credentials", existing.ID, "credential"))
	if err != nil {
		return GitCredential{}, err
	}
	cmd, err := s.db.Exec(ctx, `
		UPDATE git_credentials
		SET credential_encrypted = $1, credential_plaintext = '', public_key = $2, updated_at = $3
		WHERE id = $4
	`, encrypted, publicKey, time.Now().UTC(), existing.ID)
	if err != nil {
		return GitCredential{}, err
	}
	if cmd.RowsAffected() == 0 {
		return GitCredential{}, ErrGitCredentialNotFound
	}
	return s.GetGitCredential(ctx, existing.ID)
}

func (s *Store) UpdateGitCredentialPublicKey(ctx context.Context, id, publicKey string) (GitCredential, error) {
	_, err := s.db.Exec(ctx, `
		UPDATE git_credentials SET public_key = $1, updated_at = $2 WHERE id = $3
	`, publicKey, time.Now().UTC(), id)
	if err != nil {
		return GitCredential{}, err
	}
	return s.GetGitCredential(ctx, id)
}

func (s *Store) DeleteGitCredential(ctx context.Context, id string) error {
	cmd, err := s.db.Exec(ctx, `DELETE FROM git_credentials WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrGitCredentialNotFound
	}
	return nil
}
