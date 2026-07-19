package http

import (
	mailservice "gamepanel/forge/internal/services/mail"
	"gamepanel/forge/internal/store"
	"github.com/gofiber/fiber/v2"
)

func registerMailSettingsRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {
	admin := protected.Group("/admin", adminIPAccess)

	admin.Get("/mail/settings", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		settings, err := cfg.Store.GetPanelMailSettings(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "mail settings not found")
		}
		if settings.SMTPPassword != "" {
			settings.SMTPPassword = maskedSecret
		}
		return c.JSON(settings)
	})

	admin.Put("/mail/settings", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req store.PanelMailSettings
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if req.SMTPPassword == maskedSecret {
			existing, err := cfg.Store.GetPanelMailSettings(ctx)
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "could not preserve stored password")
			}
			req.SMTPPassword = existing.SMTPPassword
		}
		if err := cfg.Store.UpdatePanelMailSettings(ctx, req); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	admin.Post("/mail/test", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		var req struct {
			Recipient string `json:"recipient"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.Recipient == "" {
			return fiber.NewError(fiber.StatusBadRequest, "recipient is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		settings, err := cfg.Store.GetPanelMailSettings(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "mail settings not configured")
		}
		if err := mailservice.ValidateSettings(settings); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		subject := "Test Email from " + settings.MailFromName
		textBody := "This is a test email from your GamePanel. If you received this, your SMTP configuration is working correctly."
		htmlBody := "<p>This is a test email from your GamePanel.</p><p>If you received this, your SMTP configuration is working correctly.</p>"

		if cfg.MailTriggerService != nil {
			if err := cfg.MailTriggerService.Worker().Enqueue(ctx, req.Recipient, subject, textBody, htmlBody); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "failed to queue test email: "+err.Error())
			}
		} else {
			_, err := cfg.Store.EnqueueMail(ctx, req.Recipient, subject, textBody, htmlBody)
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "failed to queue test email: "+err.Error())
			}
		}
		return c.JSON(fiber.Map{"ok": true, "message": "Test email queued"})
	})

	admin.Get("/mail/triggers", requireRole("admin"), func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"triggers": []fiber.Map{
				{"event": "server.created", "template": string(mailservice.TemplateServerCreated), "label": "Server Created"},
				{"event": "server.suspended", "template": string(mailservice.TemplateServerSuspended), "label": "Server Suspended"},
				{"event": "server.unsuspended", "template": string(mailservice.TemplateServerUnsuspended), "label": "Server Unsuspended"},
				{"event": "server.install.complete", "template": string(mailservice.TemplateInstallComplete), "label": "Install Complete"},
				{"event": "backup.completed", "template": string(mailservice.TemplateBackupComplete), "label": "Backup Completed"},
				{"event": "backup.failed", "template": string(mailservice.TemplateBackupFailed), "label": "Backup Failed"},
				{"event": "account.welcome", "template": string(mailservice.TemplateWelcome), "label": "Account Welcome"},
				{"event": "subuser.invited", "template": string(mailservice.TemplateSubuserInvited), "label": "Subuser Invited"},
				{"event": "subuser.removed", "template": string(mailservice.TemplateSubuserRemoved), "label": "Subuser Removed"},
				{"event": "password.changed", "template": string(mailservice.TemplatePasswordChanged), "label": "Password Changed"},
				{"event": "2fa.enabled", "template": string(mailservice.Template2FAEnabled), "label": "2FA Enabled"},
				{"event": "2fa.disabled", "template": string(mailservice.Template2FADisabled), "label": "2FA Disabled"},
				{"event": "invitation", "template": string(mailservice.TemplateInvitation), "label": "Invitation"},
				{"event": "node.offline", "template": string(mailservice.TemplateNodeOffline), "label": "Node Offline Alert"},
			},
		})
	})
}
