package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ---------- Admin-level API scopes ----------

// AdminScope defines a scope that can be attached to an API key.
var AdminScopes = map[string]string{
	"servers.read":       "View all servers",
	"servers.write":      "Create and update servers",
	"servers.delete":     "Delete servers",
	"nodes.read":         "View all nodes",
	"nodes.write":        "Create and update nodes",
	"nodes.delete":       "Delete nodes",
	"locations.read":     "View all locations",
	"locations.write":    "Create and update locations",
	"locations.delete":   "Delete locations",
	"regions.read":       "View cluster regions and capacity",
	"regions.write":      "Create and update cluster regions",
	"regions.delete":     "Delete cluster regions",
	"users.read":         "View all users",
	"users.write":        "Create and update users",
	"users.delete":       "Delete users",
	"allocations.read":   "View all allocations",
	"allocations.write":  "Create and update allocations",
	"allocations.delete": "Delete allocations",
	"databases.read":          "View server databases",
	"databases.write":         "Create server databases",
	"databases.delete":        "Delete server databases",
	"database_hosts.read":     "View database hosts",
	"database_hosts.write":    "Create and update database hosts",
	"database_hosts.delete":   "Delete database hosts",
	"nests.read":         "View nests and eggs",
	"nests.write":        "Create and update nests and eggs",
	"nests.delete":       "Delete nests and eggs",
	"eggs.read":          "View eggs",
	"eggs.write":         "Create and update eggs",
	"eggs.delete":        "Delete eggs",
	"mounts.read":        "View mounts",
	"mounts.write":       "Create mounts",
	"mounts.delete":      "Delete mounts",
	"settings.read":      "View panel settings",
	"settings.write":     "Update panel settings",
	"audit.read":         "View global audit log",
	"roles.read":         "View roles",
	"roles.write":        "Create and update roles",
	"roles.delete":       "Delete roles",
	"plugins.read":       "View plugins",
	"plugins.write":      "Create and update plugins",
	"plugins.delete":     "Delete plugins",
	"webhooks.read":      "View webhooks",
	"webhooks.write":     "Create and update webhooks",
	"webhooks.delete":    "Delete webhooks",
}

// ClientScopes are the scopes a non-admin may delegate to a personal API key.
// They intentionally exclude every panel-administration resource.
var ClientScopes = map[string]string{
	"servers.read":   "View accessible servers",
	"servers.write":  "Operate and update accessible servers",
	"servers.delete": "Delete accessible servers",
}

// ValidateApiKeyScopes normalizes, deduplicates, and validates delegated scopes.
func ValidateApiKeyScopes(input []string, admin bool) ([]string, error) {
	seen := make(map[string]struct{}, len(input))
	out := make([]string, 0, len(input))
	for _, raw := range input {
		scope := strings.TrimSpace(raw)
		if scope == "" {
			return nil, errors.New("scope must not be empty")
		}
		if _, duplicate := seen[scope]; duplicate {
			continue
		}
		if scope == "*" {
			if !admin {
				return nil, errors.New("admin scope is not allowed")
			}
		} else if _, ok := ClientScopes[scope]; !ok {
			if _, adminScope := AdminScopes[scope]; !admin || !adminScope {
				return nil, fmt.Errorf("invalid or unauthorized scope: %s", scope)
			}
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	sort.Strings(out)
	return out, nil
}

func normalizeAllowedIPs(input []string) ([]string, error) {
	out := make([]string, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, raw := range input {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			return nil, errors.New("allowed IP entry must not be empty")
		}
		var normalized string
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			normalized = prefix.Masked().String()
		} else if addr, err := netip.ParseAddr(entry); err == nil {
			normalized = addr.Unmap().String()
		} else {
			return nil, fmt.Errorf("invalid IP or CIDR: %s", raw)
		}
		if _, duplicate := seen[normalized]; duplicate {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out, nil
}

func apiKeyIPAllowed(configured []string, requestIP string) (bool, error) {
	if len(configured) == 0 {
		return true, nil
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(requestIP))
	if err != nil {
		return false, errors.New("invalid request IP")
	}
	addr = addr.Unmap()
	allowed := false
	for _, raw := range configured {
		entry := strings.TrimSpace(raw)
		if prefix, prefixErr := netip.ParsePrefix(entry); prefixErr == nil {
			if prefix.Contains(addr) {
				allowed = true
			}
			continue
		}
		configuredAddr, addrErr := netip.ParseAddr(entry)
		if addrErr != nil {
			return false, fmt.Errorf("malformed configured IP restriction: %s", raw)
		}
		if configuredAddr.Unmap() == addr {
			allowed = true
		}
	}
	return allowed, nil
}

// ---------- API Key types ----------

type ApiKey struct {
	ID          string     `json:"id"`
	UserID      string     `json:"userId"`
	Description string     `json:"description"`
	TokenPrefix string     `json:"tokenPrefix"`
	Scopes      []string   `json:"scopes"`
	AllowedIPs  []string   `json:"allowedIps,omitempty"`
	LastUsedAt  *time.Time `json:"lastUsedAt,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	Token       string     `json:"token,omitempty"` // only set on creation
}

type CreateApiKeyRequest struct {
	Description string
	Scopes      []string
	AllowedIPs  []string
}

// ApiKeyValidation is the result of ValidateApiKey — includes the user and their scopes.
type ApiKeyValidation struct {
	User   *User
	Scopes []string
}

// ---------- API Key CRUD ----------

// generateAPIToken creates a random 48-byte hex token and returns (fullToken, prefix, sha256Hash).
func generateAPIToken() (string, string, string, error) {
	raw := make([]byte, 48)
	if _, err := rand.Read(raw); err != nil {
		return "", "", "", err
	}
	token := hex.EncodeToString(raw)
	prefix := token[:16]
	hash := sha256.Sum256([]byte(token))
	return token, prefix, hex.EncodeToString(hash[:]), nil
}

func (s *Store) ListApiKeys(ctx context.Context, userID string) ([]ApiKey, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, user_id::text, description, token_prefix,
		       COALESCE(scopes, '[]'::jsonb), COALESCE(allowed_ips, '{}'),
		       last_used_at, expires_at, created_at
		FROM api_keys
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := []ApiKey{}
	for rows.Next() {
		var key ApiKey
		var scopesJSON []byte
		if err := rows.Scan(&key.ID, &key.UserID, &key.Description, &key.TokenPrefix,
			&scopesJSON, &key.AllowedIPs,
			&key.LastUsedAt, &key.ExpiresAt, &key.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(scopesJSON, &key.Scopes)
		if key.Scopes == nil {
			key.Scopes = []string{}
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *Store) CreateApiKey(ctx context.Context, userID string, req CreateApiKeyRequest) (ApiKey, error) {
	desc := strings.TrimSpace(req.Description)
	if desc == "" {
		desc = "API Key"
	}

	user, err := s.GetUserByID(ctx, userID)
	if err != nil {
		return ApiKey{}, errors.New("user not found or disabled")
	}
	scopes, err := ValidateApiKeyScopes(req.Scopes, user.Role == "admin")
	if err != nil {
		return ApiKey{}, err
	}
	allowedIPs, err := normalizeAllowedIPs(req.AllowedIPs)
	if err != nil {
		return ApiKey{}, err
	}

	token, prefix, tokenHash, err := generateAPIToken()
	if err != nil {
		return ApiKey{}, fmt.Errorf("generate token: %w", err)
	}

	scopesJSON, _ := json.Marshal(scopes)

	id := uuid.NewString()
	_, err = s.db.Exec(ctx, `
		INSERT INTO api_keys (id, user_id, description, token_hash, token_prefix, scopes, allowed_ips)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, id, userID, desc, tokenHash, prefix, scopesJSON, allowedIPs)
	if err != nil {
		return ApiKey{}, fmt.Errorf("create api key: %w", err)
	}

	_ = s.AppendAudit(ctx, &userID, "api key created", "api_key", &id, fmt.Sprintf(`{"description":"%s","prefix":"%s","scopes":%s}`, desc, prefix, string(scopesJSON)))

	return ApiKey{
		ID:          id,
		UserID:      userID,
		Description: desc,
		TokenPrefix: prefix,
		Scopes:      scopes,
		AllowedIPs:  allowedIPs,
		CreatedAt:   time.Now(),
		Token:       token, // Only visible once!
	}, nil
}

func (s *Store) DeleteApiKey(ctx context.Context, userID string, keyID string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM api_keys WHERE id = $1 AND user_id = $2`, keyID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("api key not found")
	}
	return s.AppendAudit(ctx, &userID, "api key deleted", "api_key", &keyID, `{"reason":"user delete"}`)
}

// ValidateApiKey checks a bearer token against stored hashes.
// Returns the owning User and token scopes if valid.
func (s *Store) ValidateApiKey(ctx context.Context, token, requestIP string) (*User, []string, error) {
	if len(token) < 16 {
		return nil, nil, errors.New("invalid token")
	}
	prefix := token[:16]
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	var userID, storedHash string
	var scopesJSON []byte
	var allowedIPs []string
	var expiresAt *time.Time
	err := s.db.QueryRow(ctx, `
		SELECT user_id::text, token_hash, COALESCE(scopes, '[]'::jsonb),
		       COALESCE(allowed_ips, '{}'), expires_at
		FROM api_keys
		WHERE token_prefix = $1
	`, prefix).Scan(&userID, &storedHash, &scopesJSON, &allowedIPs, &expiresAt)
	if err != nil {
		return nil, nil, errors.New("invalid token")
	}

	// Check expiration
	if expiresAt != nil && time.Now().After(*expiresAt) {
		return nil, nil, errors.New("token expired")
	}

	if subtle.ConstantTimeCompare([]byte(storedHash), []byte(tokenHash)) != 1 {
		return nil, nil, errors.New("invalid token")
	}
	allowed, err := apiKeyIPAllowed(allowedIPs, requestIP)
	if err != nil || !allowed {
		return nil, nil, errors.New("API key is not allowed from this IP")
	}

	var scopes []string
	if err := json.Unmarshal(scopesJSON, &scopes); err != nil {
		return nil, nil, errors.New("invalid configured API key scopes")
	}
	user, err := s.GetUserByID(ctx, userID)
	if err != nil {
		return nil, nil, errors.New("user not found or disabled")
	}
	scopes, err = ValidateApiKeyScopes(scopes, user.Role == "admin")
	if err != nil {
		return nil, nil, errors.New("invalid configured API key scopes")
	}

	// Record use only after all authentication and authorization checks pass.
	_, _ = s.db.Exec(ctx, `UPDATE api_keys SET last_used_at = now() WHERE token_prefix = $1`, prefix)
	return &user, scopes, nil
}

// HasAdminScope checks if a list of scopes contains a specific admin scope.
func HasAdminScope(scopes []string, required string) bool {
	if scopes == nil {
		return false
	}
	for _, s := range scopes {
		if s == "*" || s == required {
			return true
		}
	}
	return false
}
