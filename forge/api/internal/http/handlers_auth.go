package http

import (
	"strings"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerAuthRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	protected.Get("/auth/me", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		user, err := cfg.Store.GetUserByID(ctx, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "current user is unavailable")
		}
		return c.JSON(user)
	})

	protected.Post("/auth/logout", mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if ok && claims.JTI != "" {
			ctx, cancel := requestContext()
			defer cancel()
			if err := cfg.Store.RevokeJWT(ctx, claims.JTI, time.Unix(claims.Exp, 0)); err != nil {
				return fiber.NewError(fiber.StatusServiceUnavailable, "could not revoke session")
			}
			_ = cfg.Store.AppendAudit(ctx, &claims.Sub, "logout", "user", &claims.Sub, "{}")
		}
		clearSessionCookies(c)
		return c.SendStatus(fiber.StatusNoContent)
	})

	// Session migration: legacy localStorage JWT &rarr; HttpOnly cookies
	protected.Post("/auth/session/migrate", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		header := c.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			return fiber.NewError(fiber.StatusBadRequest, "migration requires bearer token")
		}
		rawToken := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		ctx, cancel := requestContext()
		defer cancel()
		claims, err := parseToken(cfg.AuthSecret, rawToken)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid session token")
		}
		current, err := validateCurrentSession(ctx, cfg.Store, claims)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid or revoked session")
		}
		// Issue new token with same JTI/Exp for continuity
		newToken, err := issueToken(cfg.AuthSecret, store.User{ID: current.Sub, Email: current.Email, Role: current.Role, SessionVersion: current.SessionVersion})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not issue session token")
		}
		csrfToken, _ := generateCSRFToken()
		expires := time.Unix(current.Exp, 0)
		setSessionCookies(c, newToken, csrfToken, expires)
		return c.SendStatus(fiber.StatusNoContent)
	})

	// ---- Subuser Invitations ----

	protected.Post("/invitations/accept", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}

		var req struct {
			Token string `json:"token"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}

		if req.Token == "" {
			return fiber.NewError(fiber.StatusBadRequest, "token is required")
		}

		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
		}

		ctx, cancel := requestContext()
		defer cancel()

		subuser, err := cfg.Store.AcceptSubuserInvitation(ctx, req.Token, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}

		return c.JSON(subuser)
	})

	// ---- API Keys ----

	protected.Get("/api-keys", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		keys, err := cfg.Store.ListApiKeys(ctx, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(keys)
	})

	protected.Post("/api-keys", mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		var req struct {
			Description string   `json:"description"`
			Scopes      []string `json:"scopes"`
			AllowedIPs  []string `json:"allowedIps"`
		}
		_ = c.BodyParser(&req)
		ctx, cancel := requestContext()
		defer cancel()
		key, err := cfg.Store.CreateApiKey(ctx, claims.Sub, store.CreateApiKeyRequest{
			Description: req.Description,
			Scopes:      req.Scopes,
			AllowedIPs:  req.AllowedIPs,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(key)
	})

	// List only scopes the current identity may delegate.
	protected.Get("/admin-scopes", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		if claims.Role == "admin" {
			return c.JSON(store.AdminScopes)
		}
		return c.JSON(store.ClientScopes)
	})

	protected.Delete("/api-keys/:id", mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteApiKey(ctx, claims.Sub, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// ---- SSH Keys ----

	protected.Get("/ssh-keys", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		keys, err := cfg.Store.ListSSHKeys(ctx, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(keys)
	})

	protected.Post("/ssh-keys", mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		var req struct {
			Name      string `json:"name"`
			PublicKey string `json:"publicKey"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		key, err := cfg.Store.CreateSSHKey(ctx, claims.Sub, store.CreateSSHKeyRequest{
			Name:      req.Name,
			PublicKey: req.PublicKey,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(key)
	})

	protected.Delete("/ssh-keys", mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		var req struct {
			Fingerprint string `json:"fingerprint"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteSSHKey(ctx, claims.Sub, req.Fingerprint); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	// ---- Two-Factor Auth ----

	protected.Get("/account/two-factor", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		setup, err := cfg.Store.SetupTwoFactor(ctx, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(setup)
	})

	protected.Post("/account/two-factor", mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		var req struct {
			Code     string `json:"code"`
			Password string `json:"password"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		tokens, err := cfg.Store.EnableTwoFactor(ctx, claims.Sub, req.Code, req.Password)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"tokens": tokens})
	})

	protected.Delete("/account/two-factor", mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		var req struct {
			Password string `json:"password"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DisableTwoFactor(ctx, claims.Sub, req.Password); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	// ---- Self-service password change ----

	protected.Put("/account/password", mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		var req struct {
			CurrentPassword string `json:"currentPassword"`
			NewPassword     string `json:"newPassword"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := store.ValidatePassword(req.NewPassword); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if req.NewPassword == req.CurrentPassword {
			return fiber.NewError(fiber.StatusBadRequest, "new password must differ from current password")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if _, err := cfg.Store.Authenticate(ctx, claims.Email, req.CurrentPassword); err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "current password is incorrect")
		}
		if err := cfg.Store.UpdateUserPassword(ctx, claims.Sub, req.NewPassword); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		_ = cfg.Store.AppendAudit(ctx, &claims.Sub, "account.password.changed", "user", &claims.Sub, "{}")
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// ---- Self-service email change ----

	protected.Patch("/account/email", mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		var req struct {
			NewEmail        string `json:"newEmail"`
			CurrentPassword string `json:"currentPassword"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.NewEmail == "" || req.CurrentPassword == "" {
			return fiber.NewError(fiber.StatusBadRequest, "newEmail and currentPassword are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.UpdateUserEmail(ctx, claims.Sub, req.NewEmail, req.CurrentPassword); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		_ = cfg.Store.AppendAudit(ctx, &claims.Sub, "account.email.changed", "user", &claims.Sub, "{}")
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// ---- Activity Logs ----

	protected.Get("/account/activity", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		logs, err := cfg.Store.ListUserActivityLogs(ctx, claims.Sub, 50)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(logs)
	})

	// ---- Account Sessions ----

	protected.Get("/account/sessions", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()

		sessions, err := cfg.Store.ListUserSessions(ctx, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		return c.JSON(sessions)
	})

	protected.Delete("/account/sessions/:id", mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}

		sessionID := c.Params("id")
		if sessionID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "session id is required")
		}

		ctx, cancel := requestContext()
		defer cancel()

		reason := c.Query("reason", "User requested revocation")
		if err := cfg.Store.RevokeUserSession(ctx, claims.Sub, sessionID, reason); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}

		return c.JSON(fiber.Map{"status": "ok"})
	})

	protected.Post("/account/sessions/revoke-all", mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}

		var req struct {
			ExceptSessionID string `json:"exceptSessionId"`
			Reason          string `json:"reason"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}

		if req.Reason == "" {
			req.Reason = "User requested bulk revocation"
		}

		ctx, cancel := requestContext()
		defer cancel()

		if req.ExceptSessionID != "" {
			if err := cfg.Store.RevokeAllUserSessionsExceptCurrent(ctx, claims.Sub, req.ExceptSessionID, req.Reason); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
		} else {
			// Revoke all sessions
			if err := cfg.Store.RevokeAllUserSessionsExceptCurrent(ctx, claims.Sub, "", req.Reason); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
		}

		return c.JSON(fiber.Map{"status": "ok"})
	})

	// ---- Legacy API path aliases for frontend compatibility ----

	protected.Post("/auth/password/change", mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		var req struct {
			CurrentPassword string `json:"currentPassword"`
			NewPassword     string `json:"newPassword"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := store.ValidatePassword(req.NewPassword); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if req.NewPassword == req.CurrentPassword {
			return fiber.NewError(fiber.StatusBadRequest, "new password must differ from current password")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if _, err := cfg.Store.Authenticate(ctx, claims.Email, req.CurrentPassword); err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "current password is incorrect")
		}
		if err := cfg.Store.UpdateUserPassword(ctx, claims.Sub, req.NewPassword); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		_ = cfg.Store.AppendAudit(ctx, &claims.Sub, "account.password.changed", "user", &claims.Sub, "{}")
		return c.JSON(fiber.Map{"status": "ok"})
	})

	protected.Post("/auth/email/change", mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		var req struct {
			CurrentPassword string `json:"currentPassword"`
			NewEmail        string `json:"newEmail"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.NewEmail == "" || req.CurrentPassword == "" {
			return fiber.NewError(fiber.StatusBadRequest, "newEmail and currentPassword are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.UpdateUserEmail(ctx, claims.Sub, req.NewEmail, req.CurrentPassword); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		_ = cfg.Store.AppendAudit(ctx, &claims.Sub, "account.email.changed", "user", &claims.Sub, "{}")
		return c.JSON(fiber.Map{"status": "ok"})
	})

	protected.Get("/auth/sessions", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		sessions, err := cfg.Store.ListUserSessions(ctx, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(sessions)
	})

	protected.Delete("/auth/sessions/:id", mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		sessionID := c.Params("id")
		if sessionID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "session id is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		reason := c.Query("reason", "User requested revocation")
		if err := cfg.Store.RevokeUserSession(ctx, claims.Sub, sessionID, reason); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"status": "ok"})
	})

	protected.Delete("/auth/sessions", mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.RevokeAllUserSessionsExceptCurrent(ctx, claims.Sub, "", "User requested bulk revocation"); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// ---- OAuth2 self-service (PufferPanel parity) ----
	protected.Get("/account/oauth-clients", ListMyOAuthClients(cfg))
	protected.Post("/account/oauth-clients", mutationLimiter, CreateMyOAuthClient(cfg))
	protected.Delete("/account/oauth-clients/:id", mutationLimiter, DeleteMyOAuthClient(cfg))
}
