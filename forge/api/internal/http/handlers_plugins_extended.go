package http

import (
	"github.com/gofiber/fiber/v2"
)

func registerPluginRoutes(protected fiber.Router, cfg Config) {
	pluginSvc := cfg.PluginService

	pluginGroup := protected.Group("/admin/plugins", requireRole("admin"))

	pluginGroup.Get("/marketplace", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"marketplace": []any{}})
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

	// These endpoints belong to the planned runtime API. Keep their response
	// explicit rather than allowing metadata registry state to imply execution.
	pluginGroup.Post("/install", pluginRuntimeUnavailable("install"))
	pluginGroup.Post("/:id/enable", pluginRuntimeUnavailable("enable"))
	pluginGroup.Post("/:id/disable", pluginRuntimeUnavailable("disable"))
	pluginGroup.Put("/:id/settings", pluginRuntimeUnavailable("settings"))
	pluginGroup.Get("/:id/hooks", pluginRuntimeUnavailable("hooks"))
}
