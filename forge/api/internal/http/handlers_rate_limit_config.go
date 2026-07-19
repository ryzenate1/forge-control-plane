package http

import (
	"github.com/gofiber/fiber/v2"

	"gamepanel/forge/internal/store"
)

func registerRateLimitSettingsRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {
	protected.Get("/admin/settings/rate-limits", adminIPAccess, requireAdminScope("settings.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return c.JSON(store.DefaultRateLimitSettings())
		}
		ctx, cancel := requestContext()
		defer cancel()
		settings, err := cfg.Store.GetRateLimitSettings(ctx)
		if err != nil {
			return c.JSON(store.DefaultRateLimitSettings())
		}
		return c.JSON(settings)
	})

	protected.Put("/admin/settings/rate-limits", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("settings.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req store.RateLimitSettings
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid settings payload")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.UpdateRateLimitSettings(ctx, req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(req)
	})
}
