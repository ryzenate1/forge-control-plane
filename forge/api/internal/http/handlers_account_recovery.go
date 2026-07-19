package http

import (
	"gamepanel/forge/internal/services/recovery"

	"github.com/gofiber/fiber/v2"
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

func registerAccountRecoveryRoutes(v1 fiber.Router, cfg Config, authLimiter fiber.Handler) {
	v1.Post("/auth/recovery/initiate", authLimiter, func(c *fiber.Ctx) error {
		var req accountRecoveryRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}

		ctx, cancel := requestContext()
		defer cancel()

		user, err := cfg.Store.GetUserByEmail(ctx, req.Email)
		if err != nil {
			return c.JSON(accountRecoveryResponse{Status: "sent"})
		}

		token, err := cfg.RecoveryTokenService.GenerateToken(ctx, user.ID, recovery.TokenAccountRecovery, c.IP(), c.Get("User-Agent"), "")
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to generate token")
		}

		if cfg.MailTriggerService != nil {
			resetURL := cfg.PanelURL + "/account/recovery?token=" + token + "&email=" + req.Email
			cfg.MailTriggerService.SendPasswordReset(ctx, req.Email, resetURL, user.Email)
		}

		return c.JSON(accountRecoveryResponse{Status: "sent"})
	})

	v1.Post("/auth/recovery/verify", authLimiter, func(c *fiber.Ctx) error {
		var req accountRecoveryVerifyRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}

		if len(req.Password) < 8 {
			return fiber.NewError(fiber.StatusBadRequest, "password must be at least 8 characters")
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

		cfg.RecoveryTokenService.InvalidateUserTokens(ctx, userID, recovery.TokenPasswordReset)

		_ = cfg.Store.AppendAudit(ctx, &userID, "account.recovered", "user", &userID, safeAuditMeta(map[string]string{}))

		return c.JSON(accountRecoveryResponse{Status: "ok", Message: "Account recovered successfully"})
	})

	protected := v1.Group("", authMiddleware(cfg.AuthSecret, cfg.Store))
	protected.Post("/account/2fa/recovery-codes", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing user")
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

		return c.JSON(fiber.Map{"tokens": []any{}})
	})
}
