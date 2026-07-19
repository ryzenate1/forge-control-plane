package http

import (
	"github.com/gofiber/fiber/v2"

	"gamepanel/forge/internal/store"
)

type PanelSettings = store.PanelSettings

func defaultPanelSettings() PanelSettings {
	return store.PanelSettings{
		CompanyName:        "Forge Control Plane",
		Require2FA:         "none",
		DefaultLocale:      "en",
		SMTPPort:           587,
		SMTPEncryption:     "tls",
		ConnectionTimeout:  30,
		RequestTimeout:     30,
		AutoAllocStartPort: 25565,
		AutoAllocEndPort:   25600,
	}
}

func registerSettingsRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {
	protected.Get("/admin/settings", adminIPAccess, requireAdminScope("settings.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return c.JSON(defaultPanelSettings())
		}
		ctx, cancel := requestContext()
		defer cancel()
		settings, err := cfg.Store.GetPanelSettings(ctx)
		if err != nil {
			return c.JSON(defaultPanelSettings())
		}
		maskPanelSettingsSecrets(&settings)
		return c.JSON(settings)
	})

	protected.Put("/admin/settings", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("settings.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req PanelSettings
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid settings payload")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if req.SMTPPassword == maskedSecret || req.RecaptchaSecretKey == maskedSecret || req.DiscordWebhookURL == maskedSecret || req.SlackWebhookURL == maskedSecret || req.TelegramBotToken == maskedSecret {
			existing, err := cfg.Store.GetPanelSettings(ctx)
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "could not preserve stored secrets")
			}
			if req.SMTPPassword == maskedSecret {
				req.SMTPPassword = existing.SMTPPassword
			}
			if req.RecaptchaSecretKey == maskedSecret {
				req.RecaptchaSecretKey = existing.RecaptchaSecretKey
			}
			if req.DiscordWebhookURL == maskedSecret {
				req.DiscordWebhookURL = existing.DiscordWebhookURL
			}
			if req.SlackWebhookURL == maskedSecret {
				req.SlackWebhookURL = existing.SlackWebhookURL
			}
			if req.TelegramBotToken == maskedSecret {
				req.TelegramBotToken = existing.TelegramBotToken
			}
		}
		if err := cfg.Store.UpdatePanelSettings(ctx, req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		maskPanelSettingsSecrets(&req)
		return c.JSON(req)
	})
}

func maskPanelSettingsSecrets(settings *PanelSettings) {
	if settings == nil {
		return
	}
	if settings.SMTPPassword != "" {
		settings.SMTPPassword = maskedSecret
	}
	if settings.RecaptchaSecretKey != "" {
		settings.RecaptchaSecretKey = maskedSecret
	}
	if settings.DiscordWebhookURL != "" {
		settings.DiscordWebhookURL = maskedSecret
	}
	if settings.SlackWebhookURL != "" {
		settings.SlackWebhookURL = maskedSecret
	}
	if settings.TelegramBotToken != "" {
		settings.TelegramBotToken = maskedSecret
	}
}
