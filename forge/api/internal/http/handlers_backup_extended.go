package http

import (
	"github.com/gofiber/fiber/v2"

	"gamepanel/forge/internal/services/backup"
	"gamepanel/forge/internal/store"
)

func registerBackupRoutes(protected fiber.Router, cfg Config, svc *backup.Service, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	protected.Get("/servers/:id/backups/policies", requireRole("admin"), requireAdminScope("backups.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		policies, err := svc.ListPolicies(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"policies": policies})
	})

	protected.Post("/servers/:id/backups/policies", mutationLimiter, requireRole("admin"), requireAdminScope("backups.write"), func(c *fiber.Ctx) error {
		var body struct {
			Interval            string `json:"interval"`
			MaxBackups          int    `json:"maxBackups"`
			RetentionDays       int    `json:"retentionDays"`
			Storage             string `json:"storage"`
			Compress            bool   `json:"compress"`
			Encrypted           bool   `json:"encrypted"`
			EncryptionAlgorithm string `json:"encryptionAlgorithm"`
			EncryptionKey       string `json:"encryptionKey"`
			Enabled             bool   `json:"enabled"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		policy := &store.BackupPolicy{
			ServerID:            c.Params("id"),
			Interval:            body.Interval,
			MaxBackups:          body.MaxBackups,
			RetentionDays:       body.RetentionDays,
			Storage:             body.Storage,
			Compress:            body.Compress,
			Encrypted:           body.Encrypted,
			EncryptionAlgorithm: body.EncryptionAlgorithm,
			EncryptionKey:       body.EncryptionKey,
			Enabled:             body.Enabled,
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.CreatePolicy(ctx, policy); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"policy": policy})
	})

	protected.Delete("/servers/:id/backups/policies/:policyId", mutationLimiter, requireRole("admin"), requireAdminScope("backups.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.DeletePolicy(ctx, c.Params("policyId")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/backups/policies/:policyId/lock", mutationLimiter, requireRole("admin"), requireAdminScope("backups.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.LockPolicy(ctx, c.Params("policyId")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "locked": true})
	})

	protected.Post("/servers/:id/backups/policies/:policyId/unlock", mutationLimiter, requireRole("admin"), requireAdminScope("backups.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.UnlockPolicy(ctx, c.Params("policyId")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "locked": false})
	})

	protected.Post("/servers/:id/backups/cleanup", mutationLimiter, requireRole("admin"), requireAdminScope("backups.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		count, err := svc.CleanupExpiredBackups(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "cleaned": count})
	})

	protected.Get("/backup/providers", requireRole("admin"), func(c *fiber.Ctx) error {
		providers := backup.RegisteredProviders()
		return c.JSON(fiber.Map{"providers": providers})
	})

	// ----- Admin backup routes (stub) -----
	// The frontend admin backup page calls /api/v1/admin/backups/* but the
	// existing backup routes are under /servers/:id/backups/policies.
	// These stubs return meaningful errors until a full admin backup
	// controller is wired up.

	adminBackups := protected.Group("/admin/backups", requireRole("admin"))

	adminBackups.Get("/configs", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotImplemented, "admin backup configs not yet implemented; use /servers/:id/backups/policies instead")
	})
	adminBackups.Post("/configs", mutationLimiter, func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotImplemented, "admin backup configs not yet implemented; use /servers/:id/backups/policies instead")
	})
	adminBackups.Get("/configs/:id", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotImplemented, "admin backup configs not yet implemented; use /servers/:id/backups/policies instead")
	})
	adminBackups.Delete("/configs/:id", mutationLimiter, func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotImplemented, "admin backup configs not yet implemented; use /servers/:id/backups/policies instead")
	})
	adminBackups.Post("/configs/:id/execute", mutationLimiter, func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotImplemented, "admin backup configs not yet implemented")
	})

	adminBackups.Get("/jobs", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotImplemented, "admin backup jobs not yet implemented")
	})
	adminBackups.Post("/jobs", mutationLimiter, func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotImplemented, "admin backup jobs not yet implemented")
	})
	adminBackups.Post("/jobs/:id/cancel", mutationLimiter, func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotImplemented, "admin backup jobs not yet implemented")
	})

	adminBackups.Get("/artifacts", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotImplemented, "admin backup artifacts not yet implemented")
	})
	adminBackups.Delete("/artifacts/:id", mutationLimiter, func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotImplemented, "admin backup artifacts not yet implemented")
	})
	adminBackups.Post("/artifacts/:id/lock", mutationLimiter, func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotImplemented, "admin backup artifacts not yet implemented")
	})
	adminBackups.Post("/artifacts/:id/unlock", mutationLimiter, func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotImplemented, "admin backup artifacts not yet implemented")
	})
	adminBackups.Get("/artifacts/:id/download", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotImplemented, "admin backup artifacts not yet implemented")
	})

	adminBackups.Get("/restores", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotImplemented, "admin backup restores not yet implemented")
	})
	adminBackups.Post("/restore", mutationLimiter, func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotImplemented, "admin backup restores not yet implemented")
	})

	adminBackups.Get("/storage-providers", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotImplemented, "admin backup storage-providers not yet implemented")
	})

	adminBackups.Get("/status", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotImplemented, "admin backup status not yet implemented")
	})
}
