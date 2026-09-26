package http

import (
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// registerLocationRoutes wires the location CRUD endpoints that the web's
// AdminLocations component calls. Locations group nodes by geography.
func registerLocationRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	if cfg.Store == nil {
		return
	}

	locs := protected.Group("/locations", requireRole("admin"))

	locs.Get("", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		list, err := cfg.Store.ListLocations(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(list)
	})

	locs.Get("/:id", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		loc, err := cfg.Store.GetLocation(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "location not found")
		}
		return c.JSON(loc)
	})

	locs.Post("", mutationLimiter, func(c *fiber.Ctx) error {
		var req store.CreateLocationRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		loc, err := cfg.Store.CreateLocation(ctx, req, actorID)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.Status(201).JSON(loc)
	})

	locs.Patch("/:id", mutationLimiter, func(c *fiber.Ctx) error {
		var req store.UpdateLocationRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		loc, err := cfg.Store.UpdateLocation(ctx, c.Params("id"), req, actorID)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(loc)
	})

	locs.Delete("/:id", mutationLimiter, func(c *fiber.Ctx) error {
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteLocation(ctx, c.Params("id"), actorID); err != nil {
			return respondInternalError(c, err)
		}
		return c.SendStatus(204)
	})
}
