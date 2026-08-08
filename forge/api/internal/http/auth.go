package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	defaultTokenTTL = 24 * time.Hour
	maxTokenTTL     = 7 * 24 * time.Hour
)

const (
	RoleAdmin         = "admin"
	RoleUser          = "user"
	sessionCookieName = "__Host-forge_session"
	csrfCookieName    = "__Host-forge_csrf"
)

type tokenClaims struct {
	Sub            string `json:"sub"`
	Email          string `json:"email"`
	Role           string `json:"role"`
	JTI            string `json:"jti"`
	SessionVersion int64  `json:"ver"`
	Iat            int64  `json:"iat,omitempty"`
	Exp            int64  `json:"exp"`
}

func issueToken(secret string, user store.User) (string, error) {
	return issueTokenWithTTL(secret, user, defaultTokenTTL)
}

func issueTokenWithTTL(secret string, user store.User, ttl time.Duration) (string, error) {
	if ttl <= 0 || ttl > maxTokenTTL {
		return "", errors.New("token TTL must be between 1ns and 7 days")
	}
	now := time.Now()
	claims := tokenClaims{
		Sub:            user.ID,
		Email:          user.Email,
		Role:           user.Role,
		JTI:            uuid.NewString(),
		SessionVersion: user.SessionVersion,
		Iat:            now.Unix(),
		Exp:            now.Add(ttl).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   claims.Sub,
		"email": claims.Email,
		"role":  claims.Role,
		"jti":   claims.JTI,
		"ver":   claims.SessionVersion,
		"iat":   claims.Iat,
		"exp":   claims.Exp,
		"iss":   "forge-panel",
		"aud":   "forge-api",
	})
	return token.SignedString([]byte(secret))
}

func parseToken(secret, token string) (tokenClaims, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithIssuer("forge-panel"),
		jwt.WithAudience("forge-api"),
		jwt.WithLeeway(30*time.Second),
	)
	parsed, err := parser.Parse(token, func(parsedToken *jwt.Token) (any, error) {
		return []byte(secret), nil
	})
	if err != nil || !parsed.Valid {
		return tokenClaims{}, errors.New("invalid token")
	}
	values, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return tokenClaims{}, errors.New("invalid token claims")
	}
	claims := tokenClaims{
		Sub:            stringFromClaim(values["sub"]),
		Email:          stringFromClaim(values["email"]),
		Role:           stringFromClaim(values["role"]),
		JTI:            stringFromClaim(values["jti"]),
		SessionVersion: int64FromClaim(values["ver"]),
		Iat:            int64FromClaim(values["iat"]),
		Exp:            int64FromClaim(values["exp"]),
	}
	if claims.Sub == "" || claims.JTI == "" || claims.Exp == 0 {
		return tokenClaims{}, errors.New("invalid token claims")
	}
	if claims.Iat > time.Now().Add(30*time.Second).Unix() {
		return tokenClaims{}, errors.New("token issued in the future")
	}
	if time.Unix(claims.Exp, 0).Sub(time.Unix(claims.Iat, 0)) > maxTokenTTL {
		return tokenClaims{}, errors.New("token TTL exceeds maximum")
	}
	if claims.Exp <= time.Now().Add(-30*time.Second).Unix() {
		return tokenClaims{}, errors.New("token expired")
	}
	return claims, nil
}

func configuredTokenTTL(cfg Config) time.Duration {
	ttl := cfg.TokenTTL
	if ttl <= 0 {
		return defaultTokenTTL
	}
	if ttl > maxTokenTTL {
		return maxTokenTTL
	}
	return ttl
}

func tokenExpiry(cfg Config) time.Time {
	return time.Now().Add(configuredTokenTTL(cfg))
}

func issueConfiguredToken(cfg Config, user store.User) (string, error) {
	if strings.TrimSpace(cfg.AuthSecret) == "" {
		return "", errors.New("auth secret is required")
	}
	token, err := issueTokenWithTTL(cfg.AuthSecret, user, configuredTokenTTL(cfg))
	if err != nil {
		return "", err
	}
	return token, nil
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

func int64FromClaim(v any) int64 {
	switch x := v.(type) {
	case int:
		return int64(x)
	case int64:
		return x
	case float64:
		return int64(x)
	case json.Number:
		i, _ := x.Int64()
		return i
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
	cfg := LoadSessionCookieConfig()
	cookie := c.Cookies(secureCookieName(sessionCookieName, cfg.Secure))
	if cookie == "" {
		return "", false
	}
	return cookie, true
}

func setSessionCookies(c *fiber.Ctx, sessionToken, csrfToken string, expires time.Time) {
	cfg := LoadSessionCookieConfig()
	sessionName := secureCookieName(sessionCookieName, cfg.Secure)
	csrfName := secureCookieName(csrfCookieName, cfg.Secure)
	sameSite := "Lax"
	if cfg.SameSite == http.SameSiteStrictMode {
		sameSite = "Strict"
	} else if cfg.SameSite == http.SameSiteNoneMode {
		sameSite = "None"
	}
	c.Cookie(&fiber.Cookie{
		Name:     sessionName,
		Value:    sessionToken,
		Path:     "/",
		HTTPOnly: true,
		Secure:   cfg.Secure,
		SameSite: sameSite,
		Expires:  expires,
	})
	c.Cookie(&fiber.Cookie{
		Name:     csrfName,
		Value:    csrfToken,
		Path:     "/",
		HTTPOnly: false,
		Secure:   cfg.Secure,
		SameSite: sameSite,
		Expires:  expires,
	})
}

func clearSessionCookies(c *fiber.Ctx) {
	cfg := LoadSessionCookieConfig()
	sessionName := secureCookieName(sessionCookieName, cfg.Secure)
	csrfName := secureCookieName(csrfCookieName, cfg.Secure)
	sameSite := "Lax"
	if cfg.SameSite == http.SameSiteStrictMode {
		sameSite = "Strict"
	} else if cfg.SameSite == http.SameSiteNoneMode {
		sameSite = "None"
	}
	c.Cookie(&fiber.Cookie{
		Name:     sessionName,
		Value:    "",
		Path:     "/",
		HTTPOnly: true,
		Secure:   cfg.Secure,
		SameSite: sameSite,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
	c.Cookie(&fiber.Cookie{
		Name:     csrfName,
		Value:    "",
		Path:     "/",
		HTTPOnly: false,
		Secure:   cfg.Secure,
		SameSite: sameSite,
		MaxAge:   -1,
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
		// Expose the concrete store to role-rule enforcement (requireRole).
		if concrete, ok := st.(*store.Store); ok {
			c.Locals("authStore", concrete)
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
			if current.Role == RoleAdmin {
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
			if current.Role == RoleAdmin {
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
				if err != nil || user.SessionVersion != int64FromClaim(oauthClaims["ver"]) {
					return fiber.NewError(fiber.StatusUnauthorized, "invalid or stale oauth session")
				}
				scopes, err = store.ValidateApiKeyScopes(scopes, user.Role == RoleAdmin && stringFromClaim(oauthClaims["server_id"]) == "")
				if err != nil {
					return fiber.NewError(fiber.StatusUnauthorized, "invalid oauth scopes")
				}
				c.Locals("user", claimsFromUser(user, stringFromClaim(oauthClaims["jti"]), int64FromClaim(oauthClaims["exp"])))
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
			c.Locals("user", claimsFromUser(*user, "", time.Now().Add(defaultTokenTTL).Unix()))
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
		Iat:            time.Now().Unix(),
		Exp:            exp,
	}
}

func validateCurrentSession(ctx context.Context, st authenticationStore, claims tokenClaims) (tokenClaims, error) {
	if claims.Sub == "" || claims.JTI == "" || claims.SessionVersion < 0 {
		return tokenClaims{}, errors.New("missing session identity")
	}
	revoked, err := st.IsJWTRevoked(ctx, claims.JTI)
	if err != nil {
		return tokenClaims{}, fmt.Errorf("check token revocation: %w", err)
	}
	if revoked {
		return tokenClaims{}, errors.New("token revoked")
	}
	// Sessions revoked from /account/sessions (or the revoke-all endpoint)
	// mark the user_sessions row for the jti-hash as revoked. A token whose
	// recorded row was revoked must not remain usable. Rows are only created
	// for login-issued tokens, so tokens without a row are unaffected.
	if concrete, ok := st.(*store.Store); ok && concrete != nil {
		rowRevoked, err := concrete.IsUserSessionRevoked(ctx, claims.Sub, sha256Hex(claims.JTI))
		if err != nil {
			return tokenClaims{}, fmt.Errorf("check session row revocation: %w", err)
		}
		if rowRevoked {
			return tokenClaims{}, errors.New("token revoked")
		}
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
		if claims.Role == RoleAdmin {
			// Scoped credentials (API keys, OAuth tokens) must carry at least
			// one panel-administration scope before they may touch admin-only
			// routes. Otherwise a key scoped purely to e.g. "servers.read"
			// would unlock every requireRole("admin") endpoint.
			if scoped, _ := c.Locals("scopedAuth").(bool); scoped {
				scopes, _ := c.Locals("apiScopes").([]string)
				if !hasAnyAdminScope(scopes) {
					return fiber.NewError(fiber.StatusForbidden, "admin scope is required")
				}
			}
		}
		// role_rules enforcement for custom roles: deny rules block matching
		// routes; when allow rules exist for the role, at least one must match
		// (allow-list semantics). Built-in admin/user roles are unaffected.
		if claims.Role != RoleAdmin && claims.Role != RoleUser {
			if st, ok := c.Locals("authStore").(*store.Store); ok && st != nil {
				if err := enforceRoleRulesForRequest(c, st, claims.Role); err != nil {
					return err
				}
			}
		}
		return c.Next()
	}
}

// enforceRoleRulesForRequest applies role_rules to the current request for a
// custom role. rule_key values are matched against the request path (with or
// without the /api/v1 prefix).
func enforceRoleRulesForRequest(c *fiber.Ctx, st *store.Store, roleKey string) error {
	ctx, cancel := requestContext()
	defer cancel()
	rules, err := st.ListRoleRulesByRoleKey(ctx, roleKey)
	if err != nil || len(rules) == 0 {
		return nil
	}
	path := c.Path()
	trimmed := strings.TrimPrefix(path, "/api/v1")
	matches := func(ruleKey string) bool {
		if ruleKey == "" {
			return false
		}
		if ruleKey == path || ruleKey == trimmed {
			return true
		}
		prefix := strings.TrimSuffix(ruleKey, "/")
		return strings.HasPrefix(trimmed, prefix+"/") || strings.HasPrefix(path, prefix+"/")
	}
	hasAllow := false
	for _, rule := range rules {
		switch rule.Effect {
		case "deny":
			if matches(rule.RuleKey) {
				return fiber.NewError(fiber.StatusForbidden, "role rule denies access to this route")
			}
		case "allow":
			hasAllow = true
			if matches(rule.RuleKey) {
				return nil
			}
		}
	}
	if hasAllow {
		return fiber.NewError(fiber.StatusForbidden, "role rules do not allow access to this route")
	}
	return nil
}

// hasAnyAdminScope reports whether the given scope list contains "*" or at
// least one scope registered in store.AdminScopes.
func hasAnyAdminScope(scopes []string) bool {
	for _, scope := range scopes {
		if scope == "*" {
			return true
		}
		if _, ok := store.AdminScopes[scope]; ok {
			return true
		}
	}
	return false
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
		return fiber.NewError(fiber.StatusInternalServerError, "failed to check server access: "+err.Error())
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
	JTI  string `json:"jti"`
	IP   string `json:"ip,omitempty"`
	UA   string `json:"ua,omitempty"`
	Iat  int64  `json:"iat"`
	Exp  int64  `json:"exp"`
}

// issue2FAConfirmationToken mints a short-lived, single-use 2FA checkpoint
// token bound to the requesting client's IP and user agent.
func issue2FAConfirmationToken(secret, userID, ip, userAgent string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  userID,
		"type": "2fa_confirmation",
		"jti":  uuid.NewString(),
		"ip":   ip,
		"ua":   truncateUserAgent(userAgent),
		"iat":  time.Now().Unix(),
		"exp":  time.Now().Add(5 * time.Minute).Unix(),
	})
	return token.SignedString([]byte(secret))
}

func truncateUserAgent(ua string) string {
	if len(ua) > 256 {
		return ua[:256]
	}
	return ua
}

// parse2FAConfirmationToken validates a confirmation token and returns its
// claims. The token is bound to an IP and user agent captured at issuance.
func parse2FAConfirmationToken(secret string, token string) (confirmationClaims, error) {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired(), jwt.WithLeeway(30*time.Second))
	parsed, err := parser.Parse(token, func(_ *jwt.Token) (any, error) { return []byte(secret), nil })
	if err != nil || !parsed.Valid {
		return confirmationClaims{}, errors.New("invalid confirmation token")
	}
	values, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || stringFromClaim(values["type"]) != "2fa_confirmation" {
		return confirmationClaims{}, errors.New("invalid token type")
	}
	claims := confirmationClaims{
		Sub:  stringFromClaim(values["sub"]),
		Type: stringFromClaim(values["type"]),
		JTI:  stringFromClaim(values["jti"]),
		IP:   stringFromClaim(values["ip"]),
		UA:   stringFromClaim(values["ua"]),
		Iat:  int64FromClaim(values["iat"]),
		Exp:  int64FromClaim(values["exp"]),
	}
	if claims.Sub == "" || claims.JTI == "" || claims.Exp == 0 {
		return confirmationClaims{}, errors.New("invalid confirmation token subject")
	}
	return claims, nil
}

// Routes excluded from 2FA enforcement so users can set up 2FA. Exact path
// matches only — prefix matching would over-exempt routes sharing a prefix.
var twoFactorExemptPaths = map[string]bool{
	"/account/two-factor":            true,
	"/auth/2fa/confirm":              true,
	"/auth/webauthn/register/begin":  true,
	"/auth/webauthn/register/finish": true,
}

func isTwoFactorExempt(c *fiber.Ctx) bool {
	path := strings.TrimPrefix(c.Path(), "/api/v1")
	return twoFactorExemptPaths[path]
}

func userHasTwoFactor(user store.User) bool {
	return user.UseTOTP
}

func userHasConfiguredTwoFactor(ctx context.Context, cfg Config, user store.User) bool {
	if user.UseTOTP {
		return true
	}
	if cfg.WebAuthnService == nil {
		return false
	}
	credentials, err := cfg.WebAuthnService.ListCredentials(ctx, user.ID)
	return err == nil && len(credentials) > 0
}

// 2FA enforcement middleware with configurable policy levels
// Policy levels: "none" (no requirement), "admin" (require for admin users only), "all" (require for all users)
func requireTwoFactorAuthentication(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if source, _ := c.Locals("authSource").(string); source != authSourceCookieSession {
			return c.Next()
		}
		st := cfg.Store
		if st == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}

		if isTwoFactorExempt(c) {
			return c.Next()
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
		has2FA := userHasConfiguredTwoFactor(ctx, cfg, user)

		// Apply policy logic
		switch settings.Require2FA {
		case "admin":
			if user.Role == RoleAdmin && !has2FA {
				return fiber.NewError(fiber.StatusForbidden, "two-factor authentication is required for admin accounts")
			}
		case "all":
			if !has2FA {
				return fiber.NewError(fiber.StatusForbidden, "two-factor authentication is required")
			}
		}

		return c.Next()
	}
}
