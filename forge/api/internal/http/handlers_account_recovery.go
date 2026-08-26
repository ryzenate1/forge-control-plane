package http

import (
	"errors"
	"net/url"
	"strings"

	"gamepanel/forge/internal/services/recovery"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

type accountRecoveryRequest struct {
	Email string `json:"email"`
}

type accountRecoveryVerifyRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type accountRecoveryResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// registerAccountRecoveryRoutes mounts the public recovery flow on v1 and the
// authenticated account routes on the canonical `protected` router (session
// auth + dual-session guard + 2FA enforcement + CSRF + per-method rate limit).
// It must NOT re-create a shadow protected group: doing so previously dropped
// requireTwoFactorAuthentication and the method limiter, letting a session that
// had not satisfied the panel's 2FA policy mint fresh recovery codes.
func registerAccountRecoveryRoutes(v1 fiber.Router, protected fiber.Router, cfg Config, authLimiter fiber.Handler) {
	v1.Post("/auth/recovery/initiate", authLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		if cfg.RecoveryTokenService == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "recovery token service is not configured")
		}

		var req accountRecoveryRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.Email) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "email is required")
		}

		ctx, cancel := requestContext()
		defer cancel()

		// A missing (or disabled) account must be indistinguishable from a
		// delivered request so this endpoint cannot enumerate users. Any other
		// failure is an infrastructure error and is reported as such rather
		// than answered with a "sent" the system did not achieve.
		user, err := cfg.Store.GetUserByEmail(ctx, req.Email)
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return respondInternalError(c, err)
			}
			return c.JSON(accountRecoveryResponse{Status: "sent"})
		}

		token, err := cfg.RecoveryTokenService.GenerateToken(ctx, user.ID, recovery.TokenAccountRecovery, ExtractClientIP(c), c.Get("User-Agent"), "")
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to generate token")
		}

		if cfg.MailTriggerService == nil || strings.TrimSpace(cfg.PanelURL) == "" {
			// The token exists but the link can never be delivered; reporting
			// "sent" would tell the user to wait for an email that will not
			// arrive. Fail loudly instead.
			return fiber.NewError(fiber.StatusServiceUnavailable, "recovery mail delivery is not configured")
		}

		resetURL := cfg.PanelURL + "/account/recovery#token=" + url.QueryEscape(token) + "&email=" + url.QueryEscape(req.Email)
		if err := cfg.MailTriggerService.SendPasswordReset(ctx, req.Email, resetURL, user.Email); err != nil {
			return respondInternalError(c, err)
		}

		return c.JSON(accountRecoveryResponse{Status: "sent"})
	})

	v1.Post("/auth/recovery/verify", authLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		if cfg.RecoveryTokenService == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "recovery token service is not configured")
		}

		var req accountRecoveryVerifyRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}

		if err := store.ValidatePassword(req.Password); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}

		ctx, cancel := requestContext()
		defer cancel()

		userID, err := cfg.RecoveryTokenService.ConsumeToken(ctx, req.Token, recovery.TokenAccountRecovery)
		if err != nil || userID == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired recovery token")
		}

		if err := cfg.Store.UpdateUserPassword(ctx, userID, req.Password); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to update password")
		}

		revokeFailed := false
		// Recovery grants a new password, so every outstanding credential of
		// this kind must die with the old one — an unused account-recovery
		// token minted before this request would otherwise still hand out a
		// password reset after the account has been recovered.
		for _, kind := range []recovery.TokenType{recovery.TokenAccountRecovery, recovery.TokenPasswordReset} {
			if err := cfg.RecoveryTokenService.InvalidateUserTokens(ctx, userID, kind); err != nil {
				revokeFailed = true
				if cfg.Logger != nil {
					cfg.Logger.Error("account recovery: failed to invalidate recovery tokens", "user", userID, "type", string(kind), "error", err.Error())
				}
			}
		}

		if err := cfg.Store.RevokeAllUserSessionsExceptCurrent(ctx, userID, "", "account recovered"); err != nil {
			revokeFailed = true
			if cfg.Logger != nil {
				cfg.Logger.Error("account recovery: failed to revoke active sessions", "user", userID, "error", err.Error())
			}
		}

		if err := cfg.Store.AppendAudit(ctx, &userID, "account.recovered", "user", &userID, safeAuditMeta(map[string]string{})); err != nil {
			// The audit trail is the record of who took over the account; a
			// write that failed must be visible in the logs even though it
			// does not change the outcome for the caller.
			if cfg.Logger != nil {
				cfg.Logger.Error("account recovery: failed to write audit entry", "user", userID, "error", err.Error())
			}
		}

		if revokeFailed {
			return fiber.NewError(fiber.StatusInternalServerError, "password was reset but recovery-token/session revocation was incomplete; please retry")
		}

		return c.JSON(accountRecoveryResponse{Status: "ok", Message: "Account recovered successfully"})
	})

	protected.Post("/account/2fa/recovery-codes", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing user")
		}
		if cfg.RecoveryTokenService == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "recovery token service is not configured")
		}

		ctx, cancel := requestContext()
		defer cancel()

		codes, err := cfg.RecoveryTokenService.GenerateRecoveryCodes(ctx, claims.Sub, 10)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to generate recovery codes")
		}

		return c.JSON(fiber.Map{"codes": codes})
	})

	protected.Get("/account/recovery/tokens", func(c *fiber.Ctx) error {
		if _, ok := c.Locals("user").(tokenClaims); !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing user")
		}
		// recovery.TokenService can only invalidate tokens, never list them, so
		// this endpoint has no source of truth to answer from. Returning an
		// empty list would claim the user holds no live recovery token when the
		// real answer is unknown, which is exactly what a client deciding
		// "nothing to clean up" must not be told.
		return fiber.NewError(fiber.StatusNotImplemented, "recovery token listing is not available")
	})
}
