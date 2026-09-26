package http

import (
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// registerRegionRoutes wires the region CRUD endpoints that the web's
// AdminRegions component calls. Regions are placement zones that group nodes.
func registerRegionRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	if cfg.Store == nil {
		return
	}

	regions := protected.Group("/regions", requireRole("admin"))

	regions.Get("", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		list, err := cfg.Store.ListRegions(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(list)
	})

	regions.Get("/:id", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		region, err := cfg.Store.GetRegion(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "region not found")
		}
		return c.JSON(region)
	})

	regions.Post("", mutationLimiter, func(c *fiber.Ctx) error {
		var req store.CreateRegionRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		region, err := cfg.Store.CreateRegion(ctx, req, actorID)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.Status(201).JSON(region)
	})

	regions.Patch("/:id", mutationLimiter, func(c *fiber.Ctx) error {
		var req store.UpdateRegionRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		region, err := cfg.Store.UpdateRegion(ctx, c.Params("id"), req, actorID)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(region)
	})

	regions.Delete("/:id", mutationLimiter, func(c *fiber.Ctx) error {
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteRegion(ctx, c.Params("id"), actorID); err != nil {
			return respondInternalError(c, err)
		}
		return c.SendStatus(204)
	})
}
