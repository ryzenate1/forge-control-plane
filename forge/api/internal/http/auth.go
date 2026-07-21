package http

import (
	"context"
	"crypto/hmac"

	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const (
	tokenTTL          = 24 * time.Hour
	sessionCookieName = "__Host-forge_session"
	csrfCookieName    = "__Host-forge_csrf"
)

type tokenClaims struct {
	Sub            string `json:"sub"`
	Email          string `json:"email"`
	Role           string `json:"role"`
	JTI            string `json:"jti"`
	SessionVersion int64  `json:"ver"`
	Exp            int64  `json:"exp"`
}

func issueToken(secret string, user store.User) (string, error) {
	claims := tokenClaims{
		Sub:            user.ID,
		Email:          user.Email,
		Role:           user.Role,
		JTI:            uuid.NewString(),
		SessionVersion: user.SessionVersion,
		Exp:            time.Now().Add(tokenTTL).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := signToken(secret, encodedPayload)
	return encodedPayload + "." + signature, nil
}

func parseToken(secret, token string) (tokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return tokenClaims{}, errors.New("invalid token")
	}
	expected := signToken(secret, parts[0])
	if !hmac.Equal([]byte(parts[1]), []byte(expected)) {
		return tokenClaims{}, errors.New("invalid token signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return tokenClaims{}, err
	}
	var claims tokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return tokenClaims{}, err
	}
	if claims.Exp <= time.Now().Unix() {
		return tokenClaims{}, errors.New("token expired")
	}
	return claims, nil
}

func signToken(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func stringFromClaim(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func intFromClaim(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case json.Number:
		i, _ := x.Int64()
		return int(i)
	}
	return 0
}

type authenticationStore interface {
	GetUserByID(context.Context, string) (store.User, error)
	IsJWTRevoked(context.Context, string) (bool, error)
	ValidateApiKey(context.Context, string, string) (*store.User, []string, error)
}

type oauthTokenVerifier func(string) (map[string]any, []string, error)

func getSessionCookie(c *fiber.Ctx) (string, bool) {
	cookie := c.Cookies(sessionCookieName)
	if cookie == "" {
		return "", false
	}
	return cookie, true
}

func setSessionCookies(c *fiber.Ctx, sessionToken, csrfToken string, expires time.Time) {
	c.Cookie(&fiber.Cookie{
		Name:     sessionCookieName,
		Value:    sessionToken,
		Path:     "/",
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
		Expires:  expires,
	})
	c.Cookie(&fiber.Cookie{
		Name:     csrfCookieName,
		Value:    csrfToken,
		Path:     "/",
		HTTPOnly: false,
		Secure:   true,
		SameSite: "Lax",
		Expires:  expires,
	})
}

func clearSessionCookies(c *fiber.Ctx) {
	c.Cookie(&fiber.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
		Expires:  time.Unix(0, 0),
	})
	c.Cookie(&fiber.Cookie{
		Name:     csrfCookieName,
		Value:    "",
		Path:     "/",
		HTTPOnly: false,
		Secure:   true,
		SameSite: "Lax",
		Expires:  time.Unix(0, 0),
	})
}

func authMiddleware(secret string, st *store.Store) fiber.Handler {
	var authStore authenticationStore
	var verifyOAuth oauthTokenVerifier
	if st != nil {
		authStore = st
		verifyOAuth = func(rawToken string) (map[string]any, []string, error) {
			claims, scopes, err := VerifyOAuthToken(Config{AuthSecret: secret, Store: st}, rawToken)
			return map[string]any(claims), scopes, err
		}
	}
	return authMiddlewareWithStore(secret, authStore, verifyOAuth)
}

func authMiddlewareWithStore(secret string, st authenticationStore, verifyOAuth oauthTokenVerifier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if secret == "" {
			return fiber.NewError(fiber.StatusInternalServerError, "auth secret is not configured")
		}
		// Try session cookie first (browser clients)
		if sessionToken, ok := getSessionCookie(c); ok {
			if st == nil {
				return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
			}
			ctx, cancel := requestContext()
			defer cancel()

			claims, err := parseToken(secret, sessionToken)
			if err != nil {
				return fiber.NewError(fiber.StatusUnauthorized, "invalid session cookie")
			}

			current, err := validateCurrentSession(ctx, st, claims)
			if err != nil {
				return fiber.NewError(fiber.StatusUnauthorized, "invalid or revoked session")
			}

			c.Locals("user", current)
			if current.Role == "admin" {
				c.Locals("apiScopes", []string{"*"})
			} else {
				c.Locals("apiScopes", []string{})
			}
			c.Locals("scopedAuth", false)
			c.Locals("authSource", authSourceCookieSession)
			return c.Next()
		}

		// Fall back to Bearer token for API keys and OAuth tokens
		header := c.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			return fiber.NewError(fiber.StatusUnauthorized, "missing authentication")
		}
		if st == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}

		rawToken := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		ctx, cancel := requestContext()
		defer cancel()

		// Browser sessions may also be sent as Bearer tokens by API clients. They
		// must be validated against the current account and revocation state, just
		// like the cookie form, so role changes take effect immediately.
		if claims, err := parseToken(secret, rawToken); err == nil {
			current, err := validateCurrentSession(ctx, st, claims)
			if err != nil {
				return fiber.NewError(fiber.StatusUnauthorized, "invalid or revoked session")
			}
			c.Locals("user", current)
			if current.Role == "admin" {
				c.Locals("apiScopes", []string{"*"})
			} else {
				c.Locals("apiScopes", []string{})
			}
			c.Locals("scopedAuth", false)
			c.Locals("authSource", authSourceCookieSession)
			return c.Next()
		}

		if verifyOAuth != nil {
			if oauthClaims, scopes, oauthErr := verifyOAuth(rawToken); oauthErr == nil {
				user, err := st.GetUserByID(ctx, stringFromClaim(oauthClaims["sub"]))
				if err != nil || user.SessionVersion != int64(intFromClaim(oauthClaims["ver"])) {
					return fiber.NewError(fiber.StatusUnauthorized, "invalid or stale oauth session")
				}
				scopes, err = store.ValidateApiKeyScopes(scopes, user.Role == "admin" && stringFromClaim(oauthClaims["server_id"]) == "")
				if err != nil {
					return fiber.NewError(fiber.StatusUnauthorized, "invalid oauth scopes")
				}
				c.Locals("user", claimsFromUser(user, stringFromClaim(oauthClaims["jti"]), int64(intFromClaim(oauthClaims["exp"]))))
				c.Locals("apiScopes", scopes)
				c.Locals("scopedAuth", true)
				c.Locals("oauthClientId", stringFromClaim(oauthClaims["client_id"]))
				if serverID := stringFromClaim(oauthClaims["server_id"]); serverID != "" {
					c.Locals("oauthServerId", serverID)
				}
				c.Locals("authSource", authSourceOAuth)
				return c.Next()
			}
		}

		user, scopes, keyErr := st.ValidateApiKey(ctx, rawToken, c.IP())
		if keyErr == nil && user != nil {
			c.Locals("user", claimsFromUser(*user, "", time.Now().Add(tokenTTL).Unix()))
			c.Locals("apiScopes", scopes)
			c.Locals("scopedAuth", true)
			c.Locals("authSource", authSourceAPIKey)
			return c.Next()
		}

		return fiber.NewError(fiber.StatusUnauthorized, "invalid bearer token")
	}
}

func claimsFromUser(user store.User, jti string, exp int64) tokenClaims {
	return tokenClaims{
		Sub:            user.ID,
		Email:          user.Email,
		Role:           user.Role,
		JTI:            jti,
		SessionVersion: user.SessionVersion,
		Exp:            exp,
	}
}

func validateCurrentSession(ctx context.Context, st authenticationStore, claims tokenClaims) (tokenClaims, error) {
	if claims.Sub == "" || claims.JTI == "" || claims.SessionVersion < 1 {
		return tokenClaims{}, errors.New("missing session identity")
	}
	revoked, err := st.IsJWTRevoked(ctx, claims.JTI)
	if err != nil {
		return tokenClaims{}, fmt.Errorf("check token revocation: %w", err)
	}
	if revoked {
		return tokenClaims{}, errors.New("token revoked")
	}
	user, err := st.GetUserByID(ctx, claims.Sub)
	if err != nil || user.Disabled {
		return tokenClaims{}, errors.New("user not found or disabled")
	}
	if user.SessionVersion != claims.SessionVersion {
		return tokenClaims{}, errors.New("stale session")
	}
	return claimsFromUser(user, claims.JTI, claims.Exp), nil
}

func requireRole(roles ...string) fiber.Handler {
	allowed := map[string]struct{}{}
	for _, role := range roles {
		allowed[role] = struct{}{}
	}
	return func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		if _, ok := allowed[claims.Role]; !ok {
			return fiber.NewError(fiber.StatusForbidden, "insufficient role")
		}
		return c.Next()
	}
}

func requireAdminScope(scope string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		scopes, ok := c.Locals("apiScopes").([]string)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing api scope context")
		}
		if !store.HasAdminScope(scopes, scope) {
			return fiber.NewError(fiber.StatusForbidden, "missing api scope: "+scope)
		}
		return c.Next()
	}
}

func requireServerAccess(cfg Config) fiber.Handler {
	return requireServerPermission(cfg, "")
}

func requireServerPermission(cfg Config, permission string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if err := checkServerPermission(c, cfg, permission); err != nil {
			return err
		}
		return c.Next()
	}
}

func checkServerPermission(c *fiber.Ctx, cfg Config, permission string) error {
	if oauthServerID, ok := c.Locals("oauthServerId").(string); ok && oauthServerID != "" && oauthServerID != c.Params("id") {
		return fiber.NewError(fiber.StatusForbidden, "oauth token is bound to a different server")
	}
	if cfg.Store == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
	}
	if scoped, _ := c.Locals("scopedAuth").(bool); scoped {
		scopes, ok := c.Locals("apiScopes").([]string)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing api scope context")
		}
		requiredScope := "servers.write"
		switch c.Method() {
		case fiber.MethodGet:
			requiredScope = "servers.read"
		case fiber.MethodDelete:
			requiredScope = "servers.delete"
		}
		if !store.HasAdminScope(scopes, requiredScope) {
			return fiber.NewError(fiber.StatusForbidden, "missing api scope: "+requiredScope)
		}
	}
	claims, ok := c.Locals("user").(tokenClaims)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "missing session")
	}
	ctx, cancel := requestContext()
	defer cancel()
	allowed, err := cfg.Store.UserCanAccessServer(ctx, c.Params("id"), claims.Sub, claims.Role, permission)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "server not found")
	}
	if !allowed {
		if permission == "" {
			return fiber.NewError(fiber.StatusForbidden, "server access is not assigned to this user")
		}
		return fiber.NewError(fiber.StatusForbidden, "missing server permission: "+permission)
	}
	return nil
}

func tokenFromRequest(ctx context.Context, secret string, st *store.Store, header, queryToken string) (tokenClaims, error) {
	if secret == "" || st == nil {
		return tokenClaims{}, errors.New("authentication service is not configured")
	}
	var raw string
	if strings.HasPrefix(header, "Bearer ") {
		raw = strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	} else if queryToken != "" {
		raw = strings.TrimSpace(queryToken)
	} else {
		return tokenClaims{}, errors.New("missing bearer token")
	}
	claims, err := parseToken(secret, raw)
	if err != nil {
		return tokenClaims{}, err
	}
	return validateCurrentSession(ctx, st, claims)
}

type confirmationClaims struct {
	Sub  string `json:"sub"`
	Type string `json:"type"`
	Exp  int64  `json:"exp"`
}

func issue2FAConfirmationToken(secret string, userID string) (string, error) {
	claims := confirmationClaims{
		Sub:  userID,
		Type: "2fa_confirmation",
		Exp:  time.Now().Add(5 * time.Minute).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := signToken(secret, encodedPayload)
	return encodedPayload + "." + signature, nil
}

func parse2FAConfirmationToken(secret string, token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", errors.New("invalid confirmation token")
	}
	expected := signToken(secret, parts[0])
	if !hmac.Equal([]byte(parts[1]), []byte(expected)) {
		return "", errors.New("invalid confirmation token signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", err
	}
	var claims confirmationClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", err
	}
	if claims.Type != "2fa_confirmation" {
		return "", errors.New("invalid token type")
	}
	if claims.Exp <= time.Now().Unix() {
		return "", errors.New("confirmation token expired")
	}
	return claims.Sub, nil
}

// 2FA enforcement middleware with configurable policy levels
// Policy levels: "none" (no requirement), "admin" (require for admin users only), "all" (require for all users)
func requireTwoFactorAuthentication(st *store.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if st == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}

		// Get current 2FA policy from panel settings
		ctx, cancel := requestContext()
		defer cancel()

		settings, err := st.GetPanelSettings(ctx)
		if err != nil {
			// If settings cannot be retrieved, default to the most restrictive
			// policy so we fail closed instead of silently bypassing 2FA checks.
			settings = store.PanelSettings{Require2FA: "all"}
		}

		// If policy is "none", skip 2FA check
		if settings.Require2FA == "" || settings.Require2FA == "none" {
			return c.Next()
		}

		// Get user from context
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}

		// Get user details to check 2FA status
		user, err := st.GetUserByID(ctx, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "user not found")
		}

		// Check if 2FA is enabled for the user
		has2FA := user.UseTOTP && user.TOTPSecret != nil && *user.TOTPSecret != ""

		// Apply policy logic
		switch settings.Require2FA {
		case "admin":
			// Only require 2FA for admin users
			if user.Role == "admin" && !has2FA {
				return fiber.NewError(fiber.StatusForbidden, "two-factor authentication is required for admin accounts")
			}
		case "all":
			// Require 2FA for all users
			if !has2FA {
				return fiber.NewError(fiber.StatusForbidden, "two-factor authentication is required")
			}
		}

		return c.Next()
	}
}
