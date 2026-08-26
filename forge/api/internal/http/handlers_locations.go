package http

import (
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// registerLocationRoutes wires the location CRUD endpoints that the web's
// AdminLocations component calls. Locations group nodes by geography.
//
// This group is the only live registration for /locations CRUD.
// registerAdminRoutes used to register the same five routes with
// requireAdminScope and adminIPAccess; because Fiber resolves overlapping
// paths in registration order and this function runs first, those copies were
// unreachable and their scope checks never ran. The gating has been folded in
// here and the duplicates removed.
func registerLocationRoutes(protected fiber.Router, cfg Config, mutationLimiter, adminIPAccess fiber.Handler) {
	if cfg.Store == nil {
		return
	}

	locs := protected.Group("/locations", adminIPAccess, requireRole("admin"))

	locs.Get("", requireAdminScope("locations.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		list, err := cfg.Store.ListLocations(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(list)
	})

	locs.Get("/:id", requireAdminScope("locations.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		loc, err := cfg.Store.GetLocation(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "location not found")
		}
		return c.JSON(loc)
	})

	locs.Post("", mutationLimiter, requireAdminScope("locations.write"), func(c *fiber.Ctx) error {
		// Bind the http-package type rather than store.CreateLocationRequest:
		// only this one carries validate tags, so a missing short or long name
		// is rejected as a 422 naming the field. The store enforces the same
		// requirement but reports it as a plain error, which used to surface as
		// a 500.
		var req CreateLocationRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := Validate(&req); err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(err)
		}
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		loc, err := cfg.Store.CreateLocation(ctx, store.CreateLocationRequest{
			Short: req.Short,
			Long:  req.Long,
		}, actorID)
		if err != nil {
			return respondStoreError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(loc)
	})

	locs.Patch("/:id", mutationLimiter, requireAdminScope("locations.write"), func(c *fiber.Ctx) error {
		var req UpdateLocationRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		loc, err := cfg.Store.UpdateLocation(ctx, c.Params("id"), store.UpdateLocationRequest{
			Short: req.Short,
			Long:  req.Long,
		}, actorID)
		if err != nil {
			return respondStoreError(c, err)
		}
		return c.JSON(loc)
	})

	locs.Delete("/:id", mutationLimiter, requireAdminScope("locations.delete"), func(c *fiber.Ctx) error {
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteLocation(ctx, c.Params("id"), actorID); err != nil {
			return respondStoreError(c, err)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
}
