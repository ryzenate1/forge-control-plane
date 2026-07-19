package http

import (
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerMaintenanceRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	protected.Post("/admin/settings/maintenance", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}

		var req struct {
			Enabled      bool   `json:"enabled"`
			Message      string `json:"message"`
			BypassToken  string `json:"bypassToken"`
			WhitelistIPs string `json:"whitelistIps"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}

		ctx, cancel := requestContext()
		defer cancel()

		ms := store.DefaultMaintenanceSettings()
		ms.Enabled = req.Enabled
		if req.Message != "" {
			ms.Message = req.Message
		}
		if req.BypassToken != "" {
			ms.BypassToken = req.BypassToken
		}
		if req.WhitelistIPs != "" {
			ms.WhitelistIPs = req.WhitelistIPs
		}

		if err := cfg.Store.UpdateMaintenanceSettings(ctx, ms); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}

		return c.JSON(fiber.Map{
			"status":  "ok",
			"message": "maintenance mode updated",
		})
	})

	protected.Get("/admin/settings/maintenance", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return c.JSON(store.DefaultMaintenanceSettings())
		}
		ctx, cancel := requestContext()
		defer cancel()
		ms, err := cfg.Store.GetMaintenanceSettings(ctx)
		if err != nil {
			return c.JSON(store.DefaultMaintenanceSettings())
		}
		return c.JSON(ms)
	})
}
