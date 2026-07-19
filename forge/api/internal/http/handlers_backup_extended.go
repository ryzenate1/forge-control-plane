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

	protected.Get("/servers/:id/backups/policies", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		policies, err := svc.ListPolicies(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"policies": policies})
	})

	protected.Post("/servers/:id/backups/policies", mutationLimiter, func(c *fiber.Ctx) error {
		var body struct {
			Interval      string `json:"interval"`
			MaxBackups    int    `json:"maxBackups"`
			RetentionDays int    `json:"retentionDays"`
			Storage       string `json:"storage"`
			Enabled       bool   `json:"enabled"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		policy := &store.BackupPolicy{
			ServerID:      c.Params("id"),
			Interval:      body.Interval,
			MaxBackups:    body.MaxBackups,
			RetentionDays: body.RetentionDays,
			Storage:       body.Storage,
			Enabled:       body.Enabled,
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.CreatePolicy(ctx, policy); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"policy": policy})
	})

	protected.Delete("/servers/:id/backups/policies/:policyId", mutationLimiter, func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.DeletePolicy(ctx, c.Params("policyId")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/backups/cleanup", mutationLimiter, func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		count, err := svc.CleanupExpiredBackups(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "cleaned": count})
	})
}
