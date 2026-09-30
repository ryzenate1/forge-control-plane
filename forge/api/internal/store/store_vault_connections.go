package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// VaultConnection is a registered external secret store (a HashiCorp Vault
// instance) that environment-variable references can resolve against at deploy
// time.
//
// The Token and SecretID fields hold decrypted plaintext and are tagged
// json:"-" so they can never be serialized to a client by accident: they exist
// only so the vaultprovider service can talk to Vault. Every read-side method
// that backs an HTTP response (List/Create/Update) returns a *safe* view with
// those fields blanked and only a masked hint populated.
type VaultConnection struct {
	ID            string    `json:"id"`
	OrgID         *string   `json:"orgId,omitempty"`
	Name          string    `json:"name"`
	BaseURL       string    `json:"baseUrl"`
	MountPath     string    `json:"mountPath"`
	Namespace     *string   `json:"namespace,omitempty"`
	EngineVersion int       `json:"engineVersion"`
	AuthMethod    string    `json:"authMethod"`
	Enabled       bool      `json:"enabled"`
	RoleID        string    `json:"roleId,omitempty"`
	Token         string    `json:"-"`
	SecretID      string    `json:"-"`
	TokenHint     string    `json:"tokenHint,omitempty"`
	SecretIDHint  string    `json:"secretIdHint,omitempty"`
	CreatedBy     *string   `json:"createdBy,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type CreateVaultConnectionRequest struct {
	OrgID         *string
	Name          string
	BaseURL       string
	MountPath     string
	Namespace     *string
	EngineVersion int
	AuthMethod    string
	Token         string
	RoleID        string
	SecretID      string
	Enabled       *bool
	CreatedBy     *string
}

// UpdateVaultConnectionRequest uses pointers so a nil field means "leave the
// stored value untouched" (crucially, an omitted credential is not wiped).
type UpdateVaultConnectionRequest struct {
	Name          *string
	BaseURL       *string
	MountPath     *string
	Namespace     *string
	EngineVersion *int
	AuthMethod    *string
	Token         *string
	RoleID        *string
	SecretID      *string
	Enabled       *bool
}

const vaultConnectionsTable = "vault_connections"

// normalizeVaultAuthMethod maps caller input to the constrained enum, rejecting
// anything unknown rather than silently defaulting.
func normalizeVaultAuthMethod(method string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "token":
		return "token", nil
	case "approle", "app-role", "app_role":
		return "approle", nil
	default:
		return "", errors.New("auth method must be 'token' or 'approle'")
	}
}

func normalizeVaultEngineVersion(v int) int {
	if v == 1 {
		return 1
	}
	return 2
}

// maskCredentialHint reduces a secret to a non-reversible display hint
// ("****abcd"). An empty secret has no hint.
func maskCredentialHint(plaintext string) string {
	if plaintext == "" {
		return ""
	}
	runes := []rune(plaintext)
	if len(runes) <= 4 {
		return "****"
	}
	return "****" + string(runes[len(runes)-4:])
}

// encryptVaultCredential encrypts a required secret. Empty values store as
// empty envelopes; when a value is present but encryption is unavailable the
// write fails closed rather than persisting a raw credential.
func (s *Store) encryptVaultCredential(plaintext, aad string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	enc, err := s.encryptSecret(plaintext, aad)
	if err != nil {
		if errors.Is(err, ErrSecretEncryptionUnavailable) {
			return "", errors.New("secret encryption is not configured; cannot store Vault credentials")
		}
		return "", err
	}
	return enc, nil
}

// ListVaultConnections returns connections visible to the caller in a *safe*
// view: credentials are never returned in plaintext, only masked hints derived
// transiently for display. When orgID is non-nil, both that org's connections
// and global (org_id IS NULL) connections are returned so a global connection
// stays usable org-wide; when orgID is nil only globals are returned.
func (s *Store) ListVaultConnections(ctx context.Context, orgID *string) ([]VaultConnection, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, org_id::text, name, base_url, mount_path, namespace, engine_version, auth_method,
		       COALESCE(token_encrypted,''), COALESCE(token_plaintext,''), role_id,
		       COALESCE(secret_id_encrypted,''), COALESCE(secret_id_plaintext,''),
		       enabled, created_by::text, created_at, updated_at
		FROM vault_connections
		WHERE ($1::uuid IS NULL AND org_id IS NULL)
		   OR ($1::uuid IS NOT NULL AND (org_id IS NULL OR org_id = $1::uuid))
		ORDER BY created_at DESC
	`, orgParam(orgID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	connections := []VaultConnection{}
	for rows.Next() {
		var c VaultConnection
		var tokenEnv, tokenPlain, secretEnv, secretPlain string
		if err := rows.Scan(&c.ID, &c.OrgID, &c.Name, &c.BaseURL, &c.MountPath, &c.Namespace,
			&c.EngineVersion, &c.AuthMethod, &tokenEnv, &tokenPlain, &c.RoleID,
			&secretEnv, &secretPlain, &c.Enabled, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		// Decrypt transiently only to compute the masked hint, then discard the
		// plaintext so a list response can never carry a raw credential.
		token, err := s.decryptSecret(tokenEnv, tokenPlain, secretAAD(vaultConnectionsTable, c.ID, "token"))
		if err != nil {
			return nil, err
		}
		secretID, err := s.decryptSecret(secretEnv, secretPlain, secretAAD(vaultConnectionsTable, c.ID, "secret_id"))
		if err != nil {
			return nil, err
		}
		c.TokenHint = maskCredentialHint(token)
		c.SecretIDHint = maskCredentialHint(secretID)
		c.Token = ""
		c.SecretID = ""
		connections = append(connections, c)
	}
	return connections, rows.Err()
}

// GetVaultConnection returns the full connection including decrypted
// credentials, for server-side use only (Resolve / TestConnection). It must not
// be serialized to a client — Token/SecretID are json:"-" but callers should
// still prefer ListVaultConnections for anything that reaches the wire.
func (s *Store) GetVaultConnection(ctx context.Context, id string) (VaultConnection, error) {
	var c VaultConnection
	var tokenEnv, tokenPlain, secretEnv, secretPlain string
	err := s.db.QueryRow(ctx, `
		SELECT id::text, org_id::text, name, base_url, mount_path, namespace, engine_version, auth_method,
		       COALESCE(token_encrypted,''), COALESCE(token_plaintext,''), role_id,
		       COALESCE(secret_id_encrypted,''), COALESCE(secret_id_plaintext,''),
		       enabled, created_by::text, created_at, updated_at
		FROM vault_connections WHERE id::text = $1
	`, id).Scan(&c.ID, &c.OrgID, &c.Name, &c.BaseURL, &c.MountPath, &c.Namespace,
		&c.EngineVersion, &c.AuthMethod, &tokenEnv, &tokenPlain, &c.RoleID,
		&secretEnv, &secretPlain, &c.Enabled, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return VaultConnection{}, err
	}
	// c.ID here is the DB-normalized id::text, so the AAD recomputes byte-for-byte
	// identically to the encrypt calls (the canonical-AAD rule).
	token, err := s.decryptSecret(tokenEnv, tokenPlain, secretAAD(vaultConnectionsTable, c.ID, "token"))
	if err != nil {
		return VaultConnection{}, err
	}
	secretID, err := s.decryptSecret(secretEnv, secretPlain, secretAAD(vaultConnectionsTable, c.ID, "secret_id"))
	if err != nil {
		return VaultConnection{}, err
	}
	c.Token = token
	c.SecretID = secretID
	c.TokenHint = maskCredentialHint(token)
	c.SecretIDHint = maskCredentialHint(secretID)
	return c, nil
}

func (s *Store) CreateVaultConnection(ctx context.Context, req CreateVaultConnectionRequest) (VaultConnection, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return VaultConnection{}, errors.New("name is required")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if baseURL == "" {
		return VaultConnection{}, errors.New("base URL is required")
	}
	authMethod, err := normalizeVaultAuthMethod(req.AuthMethod)
	if err != nil {
		return VaultConnection{}, err
	}
	mountPath := strings.Trim(req.MountPath, "/")
	if mountPath == "" {
		mountPath = "secret"
	}
	engineVersion := normalizeVaultEngineVersion(req.EngineVersion)
	roleID := strings.TrimSpace(req.RoleID)
	// Validate the credential matches the auth method before persisting, so a
	// half-configured connection never reaches the resolver.
	switch authMethod {
	case "token":
		if strings.TrimSpace(req.Token) == "" {
			return VaultConnection{}, errors.New("a token is required for token auth")
		}
	case "approle":
		if roleID == "" || strings.TrimSpace(req.SecretID) == "" {
			return VaultConnection{}, errors.New("role id and secret id are required for AppRole auth")
		}
	}

	id := uuid.NewString()
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	// The freshly generated id is canonical lowercase (uuid.NewString), byte-equal
	// to the id::text the read path later returns, so the AAD is stable across
	// create and every subsequent read.
	tokenEnv, err := s.encryptVaultCredential(strings.TrimSpace(req.Token), secretAAD(vaultConnectionsTable, id, "token"))
	if err != nil {
		return VaultConnection{}, err
	}
	secretEnv, err := s.encryptVaultCredential(strings.TrimSpace(req.SecretID), secretAAD(vaultConnectionsTable, id, "secret_id"))
	if err != nil {
		return VaultConnection{}, err
	}

	now := time.Now().UTC()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO vault_connections (
			id, org_id, name, base_url, mount_path, namespace, engine_version, auth_method,
			token_encrypted, token_plaintext, role_id, secret_id_encrypted, secret_id_plaintext,
			enabled, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, '', $10, $11, '', $12, $13, $14, $14)
	`, id, req.OrgID, name, baseURL, mountPath, req.Namespace, engineVersion, authMethod,
		tokenEnv, roleID, secretEnv, enabled, req.CreatedBy, now); err != nil {
		if pgErr := isUniqueViolation(err); pgErr != nil {
			return VaultConnection{}, errors.New("a Vault connection with that name, or that endpoint/mount/namespace, already exists in this scope")
		}
		return VaultConnection{}, err
	}
	_ = s.AppendAudit(ctx, req.CreatedBy, "vault connection created", "vault_connection", &id, `{}`)
	return s.safeVaultConnection(ctx, id)
}

func (s *Store) UpdateVaultConnection(ctx context.Context, id string, req UpdateVaultConnectionRequest) (VaultConnection, error) {
	existing, err := s.GetVaultConnection(ctx, id)
	if err != nil {
		return VaultConnection{}, err
	}
	// Read the current credential envelopes verbatim. A nil credential pointer
	// means "leave the stored envelope untouched" (mirroring UpdateEnvironment-
	// Variable's empty-value-preserves-secret rule), so an edit that only flips a
	// name or the enabled flag can never wipe a working Vault credential.
	var tokenEnv, secretEnv string
	if err := s.db.QueryRow(ctx, `
		SELECT COALESCE(token_encrypted,''), COALESCE(secret_id_encrypted,'')
		FROM vault_connections WHERE id::text = $1
	`, existing.ID).Scan(&tokenEnv, &secretEnv); err != nil {
		return VaultConnection{}, err
	}

	name := existing.Name
	if req.Name != nil {
		if trimmed := strings.TrimSpace(*req.Name); trimmed != "" {
			name = trimmed
		}
	}
	baseURL := existing.BaseURL
	if req.BaseURL != nil {
		if trimmed := strings.TrimRight(strings.TrimSpace(*req.BaseURL), "/"); trimmed != "" {
			baseURL = trimmed
		}
	}
	mountPath := existing.MountPath
	if req.MountPath != nil {
		if trimmed := strings.Trim(*req.MountPath, "/"); trimmed != "" {
			mountPath = trimmed
		}
	}
	engineVersion := existing.EngineVersion
	if req.EngineVersion != nil {
		engineVersion = normalizeVaultEngineVersion(*req.EngineVersion)
	}
	authMethod := existing.AuthMethod
	if req.AuthMethod != nil {
		normalized, err := normalizeVaultAuthMethod(*req.AuthMethod)
		if err != nil {
			return VaultConnection{}, err
		}
		authMethod = normalized
	}
	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	namespace := existing.Namespace
	if req.Namespace != nil {
		ns := strings.TrimSpace(*req.Namespace)
		if ns == "" {
			namespace = nil
		} else {
			namespace = &ns
		}
	}

	// Credentials: a nil pointer keeps the existing envelope; a non-nil pointer
	// replaces it (empty string clears). Always encrypt with the canonical AAD
	// derived from existing.ID (the DB id::text), never the caller-supplied id.
	tokenAAD := secretAAD(vaultConnectionsTable, existing.ID, "token")
	secretIDAAD := secretAAD(vaultConnectionsTable, existing.ID, "secret_id")
	roleID := existing.RoleID
	if req.RoleID != nil {
		roleID = strings.TrimSpace(*req.RoleID)
	}
	if req.Token != nil {
		enc, err := s.encryptVaultCredential(strings.TrimSpace(*req.Token), tokenAAD)
		if err != nil {
			return VaultConnection{}, err
		}
		tokenEnv = enc
	}
	if req.SecretID != nil {
		enc, err := s.encryptVaultCredential(strings.TrimSpace(*req.SecretID), secretIDAAD)
		if err != nil {
			return VaultConnection{}, err
		}
		secretEnv = enc
	}

	// Consistency guard: the effective (post-merge) config must still satisfy the
	// auth method's credential requirement.
	switch authMethod {
	case "token":
		if tokenEnv == "" {
			return VaultConnection{}, errors.New("token auth requires a token")
		}
	case "approle":
		if roleID == "" || secretEnv == "" {
			return VaultConnection{}, errors.New("AppRole auth requires a role id and secret id")
		}
	}

	if _, err := s.db.Exec(ctx, `
		UPDATE vault_connections
		SET name=$1, base_url=$2, mount_path=$3, namespace=$4, engine_version=$5, auth_method=$6,
		    token_encrypted=$7, token_plaintext='', role_id=$8, secret_id_encrypted=$9, secret_id_plaintext='',
		    enabled=$10, updated_at=$11
		WHERE id::text=$12
	`, name, baseURL, mountPath, namespace, engineVersion, authMethod,
		tokenEnv, roleID, secretEnv, enabled, time.Now().UTC(), existing.ID); err != nil {
		if pgErr := isUniqueViolation(err); pgErr != nil {
			return VaultConnection{}, errors.New("a Vault connection with that name, or that endpoint/mount/namespace, already exists in this scope")
		}
		return VaultConnection{}, err
	}
	return s.safeVaultConnection(ctx, existing.ID)
}

// safeVaultConnection reads a connection and returns it with credentials
// blanked (hints retained) for anything that reaches a client response.
func (s *Store) safeVaultConnection(ctx context.Context, id string) (VaultConnection, error) {
	c, err := s.GetVaultConnection(ctx, id)
	if err != nil {
		return VaultConnection{}, err
	}
	c.Token = ""
	c.SecretID = ""
	return c, nil
}

func (s *Store) DeleteVaultConnection(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM vault_connections WHERE id::text = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("vault connection not found")
	}
	return nil
}

// orgParam adapts a nullable org id for the `WHERE $1::uuid IS NULL` query
// shape: a nil *string becomes an untyped NULL that the explicit ::uuid cast
// accepts, while a real id is passed as text and cast server-side.
func orgParam(orgID *string) any {
	if orgID == nil || strings.TrimSpace(*orgID) == "" {
		return nil
	}
	return *orgID
}
