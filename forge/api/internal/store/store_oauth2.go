package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type OAuthClientScope string

const (
	OAuthClientScopeServer  OAuthClientScope = "server"
	OAuthClientScopeAccount OAuthClientScope = "account"
)

type OAuthClient struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	OwnerID          string           `json:"ownerId"`
	ClientID         string           `json:"clientId"`
	ClientSecretHash string           `json:"-"`
	Scope            OAuthClientScope `json:"scope"`
	ServerID         *string          `json:"serverId,omitempty"`
	AllowedScopes    []string         `json:"allowedScopes"`
	Description      string           `json:"description"`
	ExpiresAt        *time.Time       `json:"expiresAt,omitempty"`
	CreatedAt        time.Time        `json:"createdAt"`
	UpdatedAt        time.Time        `json:"updatedAt"`
}

type CreateOAuthClientRequest struct {
	Name          string
	OwnerID       string
	Scope         OAuthClientScope
	ServerID      *string
	AllowedScopes []string
	Description   string
	ExpiresAt     *time.Time
}

type CreateOAuthClientResult struct {
	Client       OAuthClient
	ClientSecret string // plaintext, only returned on create
}

func (s *Store) CreateOAuthClient(ctx context.Context, req CreateOAuthClientRequest) (CreateOAuthClientResult, error) {
	if req.OwnerID == "" {
		return CreateOAuthClientResult{}, errors.New("owner_id is required")
	}
	if req.Scope != OAuthClientScopeServer && req.Scope != OAuthClientScopeAccount {
		return CreateOAuthClientResult{}, errors.New("scope must be 'server' or 'account'")
	}
	if req.Scope == OAuthClientScopeServer && (req.ServerID == nil || *req.ServerID == "") {
		return CreateOAuthClientResult{}, errors.New("server-scoped clients must have a server_id")
	}
	if req.Scope == OAuthClientScopeAccount && req.ServerID != nil {
		return CreateOAuthClientResult{}, errors.New("account-scoped clients must not have a server_id")
	}
	owner, err := s.GetUserByID(ctx, req.OwnerID)
	if err != nil {
		return CreateOAuthClientResult{}, errors.New("owner not found or disabled")
	}
	allowAdminScopes := req.Scope == OAuthClientScopeAccount && owner.Role == "admin"
	req.AllowedScopes, err = ValidateApiKeyScopes(req.AllowedScopes, allowAdminScopes)
	if err != nil {
		return CreateOAuthClientResult{}, err
	}
	clientID := newRandomToken(16)
	secret := newRandomToken(32)
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), BcryptCost())
	if err != nil {
		return CreateOAuthClientResult{}, err
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	allowedJSON := []byte("[]")
	if len(req.AllowedScopes) > 0 {
		// We're not using json.Marshal here to avoid the import cycle, but the
		// store package already uses json elsewhere so it's safe.
		allowedJSON = mustMarshalStringSlice(req.AllowedScopes)
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO oauth_clients (
			id, name, owner_id, client_id, client_secret_hash, scope,
			server_id, allowed_scopes, description, expires_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, id, req.Name, req.OwnerID, clientID, string(hash), req.Scope,
		req.ServerID, allowedJSON, req.Description, req.ExpiresAt, now, now); err != nil {
		return CreateOAuthClientResult{}, err
	}
	return CreateOAuthClientResult{
		Client: OAuthClient{
			ID:               id,
			Name:             req.Name,
			OwnerID:          req.OwnerID,
			ClientID:         clientID,
			ClientSecretHash: string(hash),
			Scope:            req.Scope,
			ServerID:         req.ServerID,
			AllowedScopes:    req.AllowedScopes,
			Description:      req.Description,
			ExpiresAt:        req.ExpiresAt,
			CreatedAt:        now,
			UpdatedAt:        now,
		},
		ClientSecret: secret,
	}, nil
}

func (s *Store) GetOAuthClientByClientID(ctx context.Context, clientID string) (OAuthClient, error) {
	var c OAuthClient
	var allowedJSON []byte
	err := s.db.QueryRow(ctx, `
		SELECT id, name, owner_id::text, client_id, client_secret_hash, scope,
		       server_id::text, allowed_scopes, COALESCE(description,''),
		       expires_at, created_at, updated_at
		FROM oauth_clients WHERE client_id = $1
	`, clientID).Scan(&c.ID, &c.Name, &c.OwnerID, &c.ClientID, &c.ClientSecretHash,
		&c.Scope, &c.ServerID, &allowedJSON, &c.Description, &c.ExpiresAt,
		&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return OAuthClient{}, errors.New("oauth client not found")
	}
	c.AllowedScopes = unmarshalStringSlice(allowedJSON)
	return c, nil
}

func (s *Store) GetOAuthClient(ctx context.Context, id string) (OAuthClient, error) {
	var c OAuthClient
	var allowedJSON []byte
	err := s.db.QueryRow(ctx, `
		SELECT id, name, owner_id::text, client_id, client_secret_hash, scope,
		       server_id::text, allowed_scopes, COALESCE(description,''),
		       expires_at, created_at, updated_at
		FROM oauth_clients WHERE id = $1
	`, id).Scan(&c.ID, &c.Name, &c.OwnerID, &c.ClientID, &c.ClientSecretHash,
		&c.Scope, &c.ServerID, &allowedJSON, &c.Description, &c.ExpiresAt,
		&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return OAuthClient{}, errors.New("oauth client not found")
	}
	c.AllowedScopes = unmarshalStringSlice(allowedJSON)
	return c, nil
}

func (s *Store) ListOAuthClientsForUser(ctx context.Context, userID string) ([]OAuthClient, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, owner_id::text, client_id, client_secret_hash, scope,
		       server_id::text, allowed_scopes, COALESCE(description,''),
		       expires_at, created_at, updated_at
		FROM oauth_clients WHERE owner_id = $1 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OAuthClient{}
	for rows.Next() {
		var c OAuthClient
		var allowedJSON []byte
		if err := rows.Scan(&c.ID, &c.Name, &c.OwnerID, &c.ClientID, &c.ClientSecretHash,
			&c.Scope, &c.ServerID, &allowedJSON, &c.Description, &c.ExpiresAt,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.AllowedScopes = unmarshalStringSlice(allowedJSON)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) DeleteOAuthClient(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM oauth_clients WHERE id = $1`, id)
	return err
}

func (s *Store) IsJWTRevoked(ctx context.Context, jti string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM oauth_revoked_tokens WHERE jti = $1)`, jti).Scan(&exists)
	return exists, err
}

func (s *Store) RevokeJWT(ctx context.Context, jti string, expiresAt time.Time) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO oauth_revoked_tokens (jti, expires_at) VALUES ($1, $2)
		ON CONFLICT (jti) DO NOTHING
	`, jti, expiresAt)
	return err
}

func newRandomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
