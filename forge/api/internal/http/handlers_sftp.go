package http

import (
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerSFTPRoutes(protected fiber.Router, cfg Config) {
	admin := protected.Group("/admin", requireRole("admin"))

	admin.Get("/sftp/settings", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		settings, err := cfg.Store.GetSFTPGlobalConfig(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "SFTP settings not found")
		}
		return c.JSON(settings)
	})

	admin.Put("/sftp/settings", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req store.SFTPGlobalConfig
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.UpdateSFTPGlobalConfig(ctx, req); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	admin.Get("/nodes/:nodeId/sftp", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		config, err := cfg.Store.GetSFTPNodeConfig(ctx, c.Params("nodeId"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "node SFTP config not found")
		}
		return c.JSON(config)
	})

	admin.Put("/nodes/:nodeId/sftp", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req store.SFTPNodeConfig
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		req.NodeID = c.Params("nodeId")
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.UpdateSFTPNodeConfig(ctx, req); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	admin.Get("/sftp/nodes", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		configs, err := cfg.Store.ListSFTPNodeConfigs(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(configs)
	})
}
