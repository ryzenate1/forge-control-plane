package http

import (
	"gamepanel/forge/internal/services/clustermembership"
	"github.com/gofiber/fiber/v2"
)

func registerClusterMembershipRoutes(protected fiber.Router, membership *clustermembership.Service) {
	if membership == nil {
		return
	}

	protected.Post("/nodes/:id/join", requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := membership.Join(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"status": "joined"})
	})

	protected.Post("/nodes/:id/leave", requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := membership.Leave(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"status": "left"})
	})

	protected.Post("/nodes/:id/drain", requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := membership.StartDrain(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"status": "draining"})
	})

	protected.Post("/nodes/:id/drain/cancel", requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := membership.CancelDrain(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"status": "drain_cancelled"})
	})

	protected.Get("/nodes/:id/drain", requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		status, err := membership.DrainStatus(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"status": status})
	})

	protected.Post("/nodes/:id/maintenance", requireRole("admin"), func(c *fiber.Ctx) error {
		var req struct {
			Message string `json:"message"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := membership.EnableMaintenance(ctx, c.Params("id"), req.Message); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"status": "maintenance_enabled"})
	})

	protected.Delete("/nodes/:id/maintenance", requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := membership.DisableMaintenance(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"status": "maintenance_disabled"})
	})
}
