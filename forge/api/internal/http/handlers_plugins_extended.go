package http

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"
)

func registerPluginRoutes(protected fiber.Router, cfg Config) {
	pluginSvc := cfg.PluginService

	pluginGroup := protected.Group("/admin/plugins", requireRole("admin"))

	pluginGroup.Get("/marketplace", func(c *fiber.Ctx) error {
		if pluginSvc == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "plugin service not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		all, err := pluginSvc.List(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"marketplace": all})
	})

	pluginGroup.Get("/discover", func(c *fiber.Ctx) error {
		if pluginSvc == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "plugin service not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		discovered, err := pluginSvc.Discover(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"plugins": discovered})
	})

	pluginGroup.Put("/:id/settings", func(c *fiber.Ctx) error {
		if pluginSvc == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "plugin service not available")
		}
		var raw json.RawMessage
		if err := c.BodyParser(&raw); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid settings body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := pluginSvc.UpdateSettings(ctx, c.Params("id"), raw); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		plugin, err := pluginSvc.Get(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.JSON(plugin)
	})

	pluginGroup.Get("/:id/hooks", func(c *fiber.Ctx) error {
		if pluginSvc == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "plugin service not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		results, err := pluginSvc.ExecuteHook(ctx, "info", map[string]any{"plugin_id": c.Params("id")})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"hooks": results})
	})
}
