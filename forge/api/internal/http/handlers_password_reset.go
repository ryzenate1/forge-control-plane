package http

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"

	mailservice "gamepanel/forge/internal/services/mail"
)

func passwordResetMailUnavailable(c *fiber.Ctx) error {
	return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
		"status":  "unavailable",
		"message": "password reset email delivery is unavailable",
	})
}

// registerPasswordResetRoutes wires the public, rate-limited password recovery
// endpoints onto the v1 API group. These do not require an authenticated
// session; they are throttled by authLimiter instead. Self-service password
// changes for logged-in users live under registerAuthRoutes.
func registerPasswordResetRoutes(v1 fiber.Router, cfg Config, authLimiter fiber.Handler) {
	// POST /auth/password/email
	v1.Post("/auth/password/email", authLimiter, func(c *fiber.Ctx) error {
		var req struct {
			Email string `json:"email"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		email := strings.ToLower(strings.TrimSpace(req.Email))

		ctx, cancel := requestContext()
		defer cancel()

		settings, err := cfg.Store.GetPanelMailSettings(ctx)
		if err != nil || mailservice.ValidateSettings(settings) != nil {
			return passwordResetMailUnavailable(c)
		}
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return passwordResetMailUnavailable(c)
		}
		plain := hex.EncodeToString(raw)
		sum := sha256.Sum256([]byte(plain))
		tokenHash := hex.EncodeToString(sum[:])
		resetURL := strings.TrimRight(cfg.PanelURL, "/") + "/reset-password?token=" + url.QueryEscape(plain) + "&email=" + url.QueryEscape(email)
		if _, err := cfg.Store.EnqueuePasswordReset(ctx, email, tokenHash, 30*time.Minute, c.IP(), resetURL); err != nil {
			return passwordResetMailUnavailable(c)
		}
		// Unknown accounts intentionally receive the same accepted response. A
		// success is returned only after mail configuration and queue storage are
		// available; known-account token and mail rows commit atomically.
		return c.JSON(fiber.Map{"status": "sent"})
	})

	// POST /auth/password/reset
	// Consumes a single-use reset token and updates the user's password.
	// The token is hashed (SHA-256) on the client-visible side exactly as it
	// was during request, so the plaintext never touches the database.
	v1.Post("/auth/password/reset", authLimiter, CaptchaMiddleware(cfg), func(c *fiber.Ctx) error {
		var req struct {
			Email    string `json:"email"`
			Token    string `json:"token"`
			Password string `json:"password"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		if err := store.ValidatePassword(req.Password); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}

		sum := sha256.Sum256([]byte(req.Token))
		tokenHash := hex.EncodeToString(sum[:])

		ctx, cancel := requestContext()
		defer cancel()

		userID, err := cfg.Store.ResetPasswordWithToken(ctx, tokenHash, req.Email, req.Password)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		_ = cfg.Store.AppendAudit(ctx, &userID, "password.reset.completed", "user", &userID, safeAuditMeta(map[string]string{}))
		return c.JSON(fiber.Map{"status": "ok"})
	})
}
