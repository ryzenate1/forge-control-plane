package http

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	mailservice "gamepanel/forge/internal/services/mail"
	"gamepanel/forge/internal/store"
)

const maskedSecret = "********"

func mailTestUnavailable(c *fiber.Ctx) error {
	return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
		"sent": false, "status": "unavailable", "message": "mail delivery is unavailable",
	})
}

// registerSettingsExtras adds the mail and advanced settings endpoints
// alongside the basic /admin/settings we already have.
func registerSettingsExtras(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {
	// Mail
	protected.Get("/admin/settings/mail", adminIPAccess, requireAdminScope("settings.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return c.JSON(store.PanelMailSettings{SMTPPort: 587, SMTPEncryption: "tls"})
		}
		ctx, cancel := requestContext()
		defer cancel()
		s, _ := cfg.Store.GetPanelMailSettings(ctx)
		if s.SMTPPassword != "" {
			s.SMTPPassword = maskedSecret
		}
		return c.JSON(s)
	})
	protected.Patch("/admin/settings/mail", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("settings.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var body store.PanelMailSettings
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid settings payload")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if body.SMTPPassword == maskedSecret {
			body.SMTPPassword = ""
		}
		if err := mailservice.ValidateSettings(body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if err := cfg.Store.UpdatePanelMailSettings(ctx, body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
	protected.Post("/admin/settings/mail/test", adminIPAccess, requireRole("admin"), requireAdminScope("settings.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return mailTestUnavailable(c)
		}
		var body struct {
			Email string `json:"email"`
		}
		if err := c.BodyParser(&body); err != nil || strings.TrimSpace(body.Email) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "email is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		settings, err := cfg.Store.GetPanelMailSettings(ctx)
		if err != nil {
			return mailTestUnavailable(c)
		}
		sender := mailservice.SMTPSender{Timeout: 15 * time.Second}
		if err := sender.Send(ctx, settings, strings.TrimSpace(body.Email), "Forge test email", "Your Forge SMTP configuration is working.", ""); err != nil {
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"sent": false, "status": "failed", "message": err.Error()})
		}
		return c.JSON(fiber.Map{"sent": true, "status": "sent"})
	})

	// Advanced
	protected.Get("/admin/settings/advanced", adminIPAccess, requireAdminScope("settings.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return c.JSON(store.PanelAdvancedSettings{
				GuzzleConnectTimeout: 30,
				GuzzleRequestTimeout: 30,
				AutoAllocStartPort:   25565,
				AutoAllocEndPort:     25600,
			})
		}
		ctx, cancel := requestContext()
		defer cancel()
		s, _ := cfg.Store.GetPanelAdvancedSettings(ctx)
		if s.RecaptchaSecretKey != "" {
			s.RecaptchaSecretKey = maskedSecret
		}
		return c.JSON(s)
	})
	protected.Patch("/admin/settings/advanced", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("settings.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var body store.PanelAdvancedSettings
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid settings payload")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if body.RecaptchaSecretKey == maskedSecret {
			existing, err := cfg.Store.GetPanelAdvancedSettings(ctx)
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "could not preserve stored secret")
			}
			body.RecaptchaSecretKey = existing.RecaptchaSecretKey
		}
		if err := cfg.Store.UpdatePanelAdvancedSettings(ctx, body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
}
