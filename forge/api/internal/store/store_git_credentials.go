package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type GitCredentialType string

const (
	GitCredentialSSHKey       GitCredentialType = "ssh_key"
	GitCredentialHTTPSPass    GitCredentialType = "https_password"
	GitCredentialHTTPSToken   GitCredentialType = "https_token"
	maskedGitSecret           string            = "********"
)

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
	if err != nil {
		return GitCredential{}, errors.New("git credential not found")
	}
	gc.Credential, err = s.decryptSecret(encrypted, plaintext, secretAAD("git_credentials", gc.ID, "credential"))
	if err != nil {
		return GitCredential{}, err
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
		return errors.New("git credential not found")
	}
	return nil
}
