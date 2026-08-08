package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ---------- SSH Key types ----------

type SSHKey struct {
	ID          string    `json:"id"`
	UserID      string    `json:"userId"`
	Name        string    `json:"name"`
	Fingerprint string    `json:"fingerprint"`
	PublicKey   string    `json:"publicKey"`
	CreatedAt   time.Time `json:"createdAt"`
}

type CreateSSHKeyRequest struct {
	Name      string
	PublicKey string
}

// ---------- SSH Key CRUD ----------

// computeFingerprint returns SHA256 fingerprint of a public key.
func computeFingerprint(publicKey string) string {
	hash := sha256.Sum256([]byte(strings.TrimSpace(publicKey)))
	return "SHA256:" + hex.EncodeToString(hash[:])
}

func (s *Store) ListSSHKeys(ctx context.Context, userID string) ([]SSHKey, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, user_id::text, name, fingerprint, public_key, created_at
		FROM user_ssh_keys
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := []SSHKey{}
	for rows.Next() {
		var key SSHKey
		if err := rows.Scan(&key.ID, &key.UserID, &key.Name, &key.Fingerprint, &key.PublicKey, &key.CreatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *Store) CreateSSHKey(ctx context.Context, userID string, req CreateSSHKeyRequest) (SSHKey, error) {
	name := strings.TrimSpace(req.Name)
	publicKey := strings.TrimSpace(req.PublicKey)
	if name == "" || publicKey == "" {
		return SSHKey{}, errors.New("name and publicKey are required")
	}

	fingerprint := computeFingerprint(publicKey)
	id := uuid.NewString()

	_, err := s.db.Exec(ctx, `
		INSERT INTO user_ssh_keys (id, user_id, name, fingerprint, public_key)
		VALUES ($1, $2, $3, $4, $5)
	`, id, userID, name, fingerprint, publicKey)
	if err != nil {
		return SSHKey{}, fmt.Errorf("create ssh key: %w", err)
	}

	_ = s.AppendAudit(ctx, &userID, "ssh key created", "ssh_key", &id, fmt.Sprintf(`{"fingerprint":"%s"}`, fingerprint))

	return SSHKey{
		ID:          id,
		UserID:      userID,
		Name:        name,
		Fingerprint: fingerprint,
		PublicKey:   publicKey,
		CreatedAt:   time.Now(),
	}, nil
}

func (s *Store) DeleteSSHKey(ctx context.Context, userID string, fingerprint string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM user_ssh_keys WHERE user_id = $1 AND fingerprint = $2`, userID, fingerprint)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("ssh key not found")
	}
	return s.AppendAudit(ctx, &userID, "ssh key deleted", "ssh_key", nil, fmt.Sprintf(`{"fingerprint":"%s"}`, fingerprint))
}
