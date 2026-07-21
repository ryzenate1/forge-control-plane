package http

import (
	cleanupsvc "gamepanel/forge/internal/services/cleanup"
	"github.com/gofiber/fiber/v2"
)

func registerCleanupRoutes(protected fiber.Router, cleanup *cleanupsvc.Service) {
	if cleanup == nil {
		return
	}

	protected.Post("/cleanup/run", requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		info, err := cleanup.RunCleanup(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(info)
	})

	protected.Get("/cleanup/inspect", requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		info, err := cleanup.Inspect(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(info)
	})
}
