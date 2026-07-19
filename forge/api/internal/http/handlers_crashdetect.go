package http

import (
	"github.com/gofiber/fiber/v2"

	"gamepanel/forge/internal/services/crashdetector"
)

func registerCrashDetectionRoutes(protected fiber.Router, cfg Config, detector *crashdetector.Detector, mutationLimiter fiber.Handler) {
	if detector == nil {
		return
	}

	admin := protected.Group("/admin")

	// GET /admin/crash-detection/servers/:id - get crash history for a server
	admin.Get("/crash-detection/servers/:id", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return c.Status(503).JSON(fiber.Map{"error": "database not available"})
		}
		events, err := cfg.Store.ListCrashEvents(c.Context(), c.Params("id"), 50)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"events": events})
	})

	// POST /admin/crash-detection/servers/:id/reset - reset crash state for a server
	admin.Post("/crash-detection/servers/:id/reset", mutationLimiter, func(c *fiber.Ctx) error {
		detector.Reset(c.Params("id"))
		return c.JSON(fiber.Map{"ok": true})
	})

	// GET /admin/crash-detection/config - get current crash detection config
	admin.Get("/crash-detection/config", func(c *fiber.Ctx) error {
		dc := crashdetector.DefaultConfig()
		return c.JSON(fiber.Map{
			"threshold":   dc.Threshold,
			"window":      dc.Window.String(),
			"cooldown":    dc.Cooldown.String(),
			"autoRestart": dc.AutoRestart,
			"maxRestarts": dc.MaxRestarts,
			"notifyAdmin": dc.NotifyAdmin,
		})
	})
}
