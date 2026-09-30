package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DockerRegistry stores credentials for a private container registry, used to
// pull/push images on the Beacon nodes. The secret material is stored encrypted
// via the panel keyring; list/get return it masked and only the unmasked getter
// (used at deploy time) returns plaintext.
type DockerRegistry struct {
	ID            string    `json:"id"`
	UserID        string    `json:"userId,omitempty"`
	Name          string    `json:"name"`
	ServerAddress string    `json:"serverAddress"`
	Username      string    `json:"username"`
	Credential    string    `json:"credential,omitempty"`
	Email         string    `json:"email,omitempty"`
	IsGlobal      bool      `json:"isGlobal"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type CreateDockerRegistryRequest struct {
	UserID        string
	Name          string
	ServerAddress string
	Username      string
	Credential    string
	Email         string
	IsGlobal      bool
}

func normalizeServerAddress(addr string) string {
	addr = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(addr), "/"))
	for _, prefix := range []string{"https://", "http://"} {
		addr = strings.TrimPrefix(addr, prefix)
	}
	return addr
}

func scanDockerRegistry(scan func(dest ...any) error) (DockerRegistry, error) {
	var r DockerRegistry
	var userID *string
	err := scan(&r.ID, &userID, &r.Name, &r.ServerAddress, &r.Username, &r.Credential, &r.Email, &r.IsGlobal, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return DockerRegistry{}, err
	}
	if userID != nil {
		r.UserID = *userID
	}
	return r, nil
}

const dockerRegistryColumns = `id, user_id, name, server_address, username, credential_encrypted, email, is_global, created_at, updated_at`

func (s *Store) getDockerRegistryInternal(ctx context.Context, id string) (DockerRegistry, error) {
	var r DockerRegistry
	var userID *string
	var encrypted string
	err := s.db.QueryRow(ctx, `
		SELECT id, user_id, name, server_address, username, credential_encrypted, email, is_global, created_at, updated_at
		FROM docker_registries WHERE id = $1
	`, id).Scan(&r.ID, &userID, &r.Name, &r.ServerAddress, &r.Username, &encrypted, &r.Email, &r.IsGlobal, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return DockerRegistry{}, errors.New("docker registry not found")
	}
	if userID != nil {
		r.UserID = *userID
	}
	r.Credential, err = s.decryptSecret(encrypted, "", secretAAD("docker_registries", r.ID, "credential"))
	if err != nil {
		return DockerRegistry{}, err
	}
	return r, nil
}

// GetDockerRegistry returns the record with the credential masked.
func (s *Store) GetDockerRegistry(ctx context.Context, id string) (DockerRegistry, error) {
	r, err := s.getDockerRegistryInternal(ctx, id)
	if err != nil {
		return DockerRegistry{}, err
	}
	if r.Credential != "" {
		r.Credential = "********"
	}
	return r, nil
}

// GetDockerRegistryUnmasked returns the plaintext credential for deploy-time use.
func (s *Store) GetDockerRegistryUnmasked(ctx context.Context, id string) (DockerRegistry, error) {
	return s.getDockerRegistryInternal(ctx, id)
}

func (s *Store) ListDockerRegistries(ctx context.Context) ([]DockerRegistry, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, name, server_address, username, credential_encrypted, email, is_global, created_at, updated_at
		FROM docker_registries ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	registries := []DockerRegistry{}
	for rows.Next() {
		var r DockerRegistry
		var userID *string
		var encrypted string
		if err := rows.Scan(&r.ID, &userID, &r.Name, &r.ServerAddress, &r.Username, &encrypted, &r.Email, &r.IsGlobal, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		if userID != nil {
			r.UserID = *userID
		}
		if strings.TrimSpace(encrypted) != "" {
			r.Credential = "********"
		}
		registries = append(registries, r)
	}
	return registries, rows.Err()
}

func (s *Store) CreateDockerRegistry(ctx context.Context, req CreateDockerRegistryRequest) (DockerRegistry, error) {
	name := strings.TrimSpace(req.Name)
	serverAddress := normalizeServerAddress(req.ServerAddress)
	if name == "" {
		return DockerRegistry{}, errors.New("name is required")
	}
	if serverAddress == "" {
		return DockerRegistry{}, errors.New("serverAddress is required")
	}
	id := uuid.NewString()
	encrypted, err := s.encryptSecret(req.Credential, secretAAD("docker_registries", id, "credential"))
	if err != nil {
		return DockerRegistry{}, err
	}
	now := time.Now().UTC()
	var userIDArg any
	if strings.TrimSpace(req.UserID) != "" {
		userIDArg = req.UserID
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO docker_registries (id, user_id, name, server_address, username, credential_encrypted, email, is_global, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, id, userIDArg, name, serverAddress, strings.TrimSpace(req.Username), encrypted, strings.TrimSpace(req.Email), req.IsGlobal, now, now); err != nil {
		if isUniqueViolation(err) != nil {
			return DockerRegistry{}, errors.New("a registry with this name already exists")
		}
		return DockerRegistry{}, err
	}
	return s.GetDockerRegistry(ctx, id)
}

func (s *Store) UpdateDockerRegistry(ctx context.Context, id string, req CreateDockerRegistryRequest) (DockerRegistry, error) {
	existing, err := s.getDockerRegistryInternal(ctx, id)
	if err != nil {
		return DockerRegistry{}, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = existing.Name
	}
	serverAddress := normalizeServerAddress(req.ServerAddress)
	if serverAddress == "" {
		serverAddress = existing.ServerAddress
	}
	username := strings.TrimSpace(req.Username)
	email := strings.TrimSpace(req.Email)
	// Only re-encrypt when a new credential was supplied; otherwise keep stored one.
	newCredentialProvided := req.Credential != "" && req.Credential != "********"
	nextEncrypted := ""
	if newCredentialProvided {
		nextEncrypted, err = s.encryptSecret(req.Credential, secretAAD("docker_registries", id, "credential"))
		if err != nil {
			return DockerRegistry{}, err
		}
	} else {
		// Preserve existing ciphertext by re-reading it directly.
		if err := s.db.QueryRow(ctx, `SELECT credential_encrypted FROM docker_registries WHERE id = $1`, id).Scan(&nextEncrypted); err != nil {
			return DockerRegistry{}, err
		}
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE docker_registries SET name=$1, server_address=$2, username=$3, email=$4, is_global=$5, credential_encrypted=$6, updated_at=$7
		WHERE id=$8
	`, name, serverAddress, username, email, req.IsGlobal, nextEncrypted, time.Now().UTC(), id); err != nil {
		if isUniqueViolation(err) != nil {
			return DockerRegistry{}, errors.New("a registry with this name already exists")
		}
		return DockerRegistry{}, err
	}
	return s.GetDockerRegistry(ctx, id)
}

func (s *Store) DeleteDockerRegistry(ctx context.Context, id string) error {
	cmd, err := s.db.Exec(ctx, `DELETE FROM docker_registries WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("docker registry not found")
	}
	return nil
}
