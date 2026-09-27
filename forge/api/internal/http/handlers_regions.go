package http

import (
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// registerRegionRoutes wires the region CRUD endpoints that the web's
// AdminRegions component calls. Regions are placement zones that group nodes.
//
// This group is the only registration for /regions CRUD. registerAdminRoutes
// used to register the same five routes with requireAdminScope and
// adminIPAccess; because Fiber resolves overlapping paths in registration
// order and this function runs first, those copies were unreachable and the
// scope checks never ran. The gating has been folded in here and the duplicate
// removed, so a scoped admin API key is enforced on the live route.
func registerRegionRoutes(protected fiber.Router, cfg Config, mutationLimiter, adminIPAccess fiber.Handler) {
	if cfg.Store == nil {
		return
	}

	regions := protected.Group("/regions", adminIPAccess, requireRole("admin"))

	regions.Get("", requireAdminScope("regions.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		list, err := cfg.Store.ListRegions(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(list)
	})

	regions.Get("/:id", requireAdminScope("regions.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		region, err := cfg.Store.GetRegion(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "region not found")
		}
		return c.JSON(region)
	})

	regions.Post("", mutationLimiter, requireAdminScope("regions.write"), func(c *fiber.Ctx) error {
		// Bind the http-package type rather than store.CreateRegionRequest:
		// only this one carries validate tags, so a missing name or slug is
		// rejected as a 422 with the offending field named. The store enforces
		// the same requirement, but reports it as a plain error that the old
		// respondInternalError call surfaced to the client as a 500.
		var req CreateRegionRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := Validate(&req); err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(err)
		}
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		region, err := cfg.Store.CreateRegion(ctx, store.CreateRegionRequest{
			Name:        req.Name,
			Slug:        req.Slug,
			Description: req.Description,
			Enabled:     req.Enabled,
		}, actorID)
		if err != nil {
			return respondStoreError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(region)
	})

	regions.Patch("/:id", mutationLimiter, requireAdminScope("regions.write"), func(c *fiber.Ctx) error {
		var req store.UpdateRegionRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		region, err := cfg.Store.UpdateRegion(ctx, c.Params("id"), req, actorID)
		if err != nil {
			return respondStoreError(c, err)
		}
		return c.JSON(region)
	})

	regions.Delete("/:id", mutationLimiter, requireAdminScope("regions.delete"), func(c *fiber.Ctx) error {
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteRegion(ctx, c.Params("id"), actorID); err != nil {
			return respondStoreError(c, err)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
}
