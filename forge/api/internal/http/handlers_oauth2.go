package http

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// ---- /oauth2/token ----
// Implements RFC 6749 client_credentials grant for PufferPanel-style
// external integrations. The client authenticates with HTTP Basic auth
// (client_id:client_secret) and exchanges it for a short-lived JWT access
// token. The token is bound to:
//   - the client (aud: client.ClientID)
//   - the user that owns the client (sub: client.OwnerID)
//   - the requested scopes (less than or equal to client.AllowedScopes)
//   - if the client is server-scoped, the server (server_id claim)
//
// Tokens expire in 1 hour.

type oauthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

func IssueOAuth2Token(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		// Parse application/x-www-form-urlencoded
		grantType := strings.TrimSpace(c.FormValue("grant_type"))
		if grantType != "client_credentials" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":             "unsupported_grant_type",
				"error_description": "only client_credentials is supported",
			})
		}
		// Authenticate client via HTTP Basic. FastHTTP's Request doesn't expose
		// BasicAuth directly, so we parse the Authorization header ourselves.
		clientID, clientSecret, ok := parseBasicAuth(c.Get("Authorization"))
		if !ok {
			clientID = c.FormValue("client_id")
			clientSecret = c.FormValue("client_secret")
		}
		if clientID == "" || clientSecret == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":             "invalid_client",
				"error_description": "client_id and client_secret are required",
			})
		}
		ctx, cancel := requestContext()
		defer cancel()
		client, err := cfg.Store.GetOAuthClientByClientID(ctx, clientID)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":             "invalid_client",
				"error_description": "unknown client_id",
			})
		}
		if client.ExpiresAt != nil && time.Now().After(*client.ExpiresAt) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":             "invalid_client",
				"error_description": "client has expired",
			})
		}
		if err := bcrypt.CompareHashAndPassword([]byte(client.ClientSecretHash), []byte(clientSecret)); err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":             "invalid_client",
				"error_description": "client authentication failed",
			})
		}
		owner, err := cfg.Store.GetUserByID(ctx, client.OwnerID)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid_client"})
		}
		allowAdminScopes := client.Scope == store.OAuthClientScopeAccount && owner.Role == "admin"
		allowedScopes, err := store.ValidateApiKeyScopes(client.AllowedScopes, allowAdminScopes)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid_scope", "error_description": "client has invalid configured scopes",
			})
		}
		grantedScopes := allowedScopes
		if requested := strings.TrimSpace(c.FormValue("scope")); requested != "" {
			wanted := splitScopes(requested)
			if _, err := store.ValidateApiKeyScopes(wanted, allowAdminScopes); err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid_scope"})
			}
			for _, scope := range wanted {
				if !contains(allowedScopes, scope) {
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
						"error": "invalid_scope", "error_description": "a requested scope is not allowed for this client",
					})
				}
			}
			grantedScopes = wanted
		}
		// Mint the token. TTL 1 hour.
		ttl := time.Hour
		expiresAt := time.Now().Add(ttl)
		claims := jwt.MapClaims{
			"iss":       "forge-panel",
			"sub":       client.OwnerID,
			"aud":       client.ClientID,
			"iat":       time.Now().Unix(),
			"exp":       expiresAt.Unix(),
			"jti":       uuid.NewString(),
			"ver":       owner.SessionVersion,
			"scope":     strings.Join(grantedScopes, " "),
			"client_id": client.ClientID,
		}
		if client.Scope == store.OAuthClientScopeServer && client.ServerID != nil {
			claims["server_id"] = *client.ServerID
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := token.SignedString([]byte(cfg.AuthSecret))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to sign token")
		}
		// Persist an activity entry.
		_ = cfg.Store.LogActivity(ctx, "oauth.token.issued", &client.OwnerID, nil, ptrStr("oauth_client"), &client.ID, map[string]any{
			"clientId": client.ClientID,
			"scope":    client.Scope,
		})
		return c.JSON(oauthTokenResponse{
			AccessToken: signed,
			TokenType:   "Bearer",
			ExpiresIn:   int(ttl.Seconds()),
			Scope:       strings.Join(grantedScopes, " "),
		})
	}
}

func ptrStr(s string) *string { return &s }

// ---- /api/v1/account/oauth-clients ----
// Lets a user register and manage their own OAuth2 clients (PufferPanel-style
// "self.clients" scope). Admin can manage clients via /admin/oauth-clients.

func ListMyOAuthClients(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing user")
		}
		ctx, cancel := requestContext()
		defer cancel()
		clients, err := cfg.Store.ListOAuthClientsForUser(ctx, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(clients)
	}
}

func CreateMyOAuthClient(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing user")
		}
		var req createOAuthClientRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		res, err := cfg.Store.CreateOAuthClient(ctx, store.CreateOAuthClientRequest{
			Name:          strings.TrimSpace(req.Name),
			OwnerID:       claims.Sub,
			Scope:         store.OAuthClientScope(req.Scope),
			ServerID:      req.ServerID,
			AllowedScopes: req.AllowedScopes,
			Description:   req.Description,
			ExpiresAt:     req.ExpiresAt,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"client":       res.Client,
			"clientSecret": res.ClientSecret,
		})
	}
}

func DeleteMyOAuthClient(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing user")
		}
		ctx, cancel := requestContext()
		defer cancel()
		client, err := cfg.Store.GetOAuthClient(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "oauth client not found")
		}
		if client.OwnerID != claims.Sub {
			return fiber.NewError(fiber.StatusForbidden, "you do not own this oauth client")
		}
		if err := cfg.Store.DeleteOAuthClient(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	}
}

// ---- Admin: /api/v1/admin/oauth-clients ----
// Lets an admin manage any user's OAuth clients.

func AdminListOAuthClients(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		userID := c.Query("userId")
		if userID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "userId query param required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		clients, err := cfg.Store.ListOAuthClientsForUser(ctx, userID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(clients)
	}
}

func AdminCreateOAuthClient(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req createOAuthClientRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.OwnerID) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "ownerId is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		res, err := cfg.Store.CreateOAuthClient(ctx, store.CreateOAuthClientRequest{
			Name:          strings.TrimSpace(req.Name),
			OwnerID:       req.OwnerID,
			Scope:         store.OAuthClientScope(req.Scope),
			ServerID:      req.ServerID,
			AllowedScopes: req.AllowedScopes,
			Description:   req.Description,
			ExpiresAt:     req.ExpiresAt,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"client":       res.Client,
			"clientSecret": res.ClientSecret,
		})
	}
}

func AdminDeleteOAuthClient(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteOAuthClient(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	}
}

// VerifyOAuthToken parses + verifies a JWT issued by IssueOAuth2Token. It
// enforces signature, expiry, revocation list, and audience. On success it
// returns the parsed claims plus the resolved scopes.
//
// Store access is mandatory: revocation checks fail closed when persistence is
// unavailable or returns an error.
func VerifyOAuthToken(cfg Config, tokenString string) (jwt.MapClaims, []string, error) {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}))
	token, err := parser.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		return []byte(cfg.AuthSecret), nil
	})
	if err != nil {
		return nil, nil, err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, nil, errors.New("invalid token")
	}
	if claims["iss"] != "forge-panel" {
		return nil, nil, errors.New("invalid issuer")
	}
	jti, _ := claims["jti"].(string)
	if jti == "" {
		return nil, nil, errors.New("missing token id")
	}
	if cfg.Store == nil {
		return nil, nil, errors.New("token revocation store is unavailable")
	}
	ctx, cancel := requestContext()
	defer cancel()
	revoked, err := cfg.Store.IsJWTRevoked(ctx, jti)
	if err != nil {
		return nil, nil, errors.New("token revocation check failed")
	}
	if revoked {
		return nil, nil, errors.New("token has been revoked")
	}
	scopeStr, _ := claims["scope"].(string)
	return claims, splitScopes(scopeStr), nil
}

// ---- helpers ----

type createOAuthClientRequest struct {
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	Scope         string     `json:"scope"` // "server" or "account"
	ServerID      *string    `json:"serverId,omitempty"`
	AllowedScopes []string   `json:"allowedScopes"`
	ExpiresAt     *time.Time `json:"expiresAt"`
	OwnerID       string     `json:"ownerId"` // admin-only; ignored on self-create
}

func parseBasicAuth(header string) (string, string, bool) {
	const prefix = "Basic "
	if len(header) < len(prefix) || header[:len(prefix)] != prefix {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(header[len(prefix):])
	if err != nil {
		return "", "", false
	}
	idx := strings.IndexByte(string(decoded), ':')
	if idx < 0 {
		return "", "", false
	}
	return string(decoded[:idx]), string(decoded[idx+1:]), true
}

func splitScopes(s string) []string {
	out := []string{}
	for _, p := range strings.Fields(s) {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

// HashOAuthClientSecret is exposed for tests so they can seed a known
// client without going through CreateOAuthClient. It mirrors the panel's
// configurable bcrypt cost.
func HashOAuthClientSecret(secret string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(secret), store.BcryptCost())
	return string(h), err
}

// sha256Hex is a tiny helper used by tests to derive a stable JTI/secret
// fingerprint for assertions.
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// jsonString is used by tests; avoids pulling encoding/json directly into
// non-test code in this file.
func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
