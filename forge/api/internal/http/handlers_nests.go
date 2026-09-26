package http

import (
	"strings"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// registerNestRoutes wires the nests-and-eggs CRUD endpoints that the web's
// AdminNestsEggs component calls. Nests are top-level groupings; eggs are
// service blueprints inside a nest.
func registerNestRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	if cfg.Store == nil {
		return
	}

	// --- Nests ---
	nests := protected.Group("/nests", requireRole("admin"))

	nests.Get("", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		list, err := cfg.Store.ListNests(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(list)
	})

	nests.Get("/:id", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		nest, err := cfg.Store.GetNest(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "nest not found")
		}
		return c.JSON(nest)
	})

	nests.Post("", mutationLimiter, func(c *fiber.Ctx) error {
		var req store.CreateNestRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		nest, err := cfg.Store.CreateNest(ctx, req, actorID)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.Status(201).JSON(nest)
	})

	nests.Patch("/:id", mutationLimiter, func(c *fiber.Ctx) error {
		var req store.UpdateNestRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		nest, err := cfg.Store.UpdateNest(ctx, c.Params("id"), req, actorID)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(nest)
	})

	nests.Delete("/:id", mutationLimiter, func(c *fiber.Ctx) error {
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteNest(ctx, c.Params("id"), actorID); err != nil {
			return respondInternalError(c, err)
		}
		return c.SendStatus(204)
	})

	// --- Eggs ---
	eggs := protected.Group("/eggs", requireRole("admin"))

	eggs.Get("", func(c *fiber.Ctx) error {
		// ListEggs treats an empty nestID as "every nest". "*" is not that
		// sentinel: it is bound to a UUID column and Postgres rejects the cast,
		// so the default listing failed with a 500 on every request.
		nestID := strings.TrimSpace(c.Query("nestId", ""))
		ctx, cancel := requestContext()
		defer cancel()
		list, err := cfg.Store.ListEggs(ctx, nestID)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(list)
	})

	eggs.Get("/:id", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		egg, err := cfg.Store.GetEgg(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "egg not found")
		}
		return c.JSON(egg)
	})

	eggs.Post("", mutationLimiter, func(c *fiber.Ctx) error {
		var req store.CreateEggRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		egg, err := cfg.Store.CreateEgg(ctx, req, actorID)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.Status(201).JSON(egg)
	})

	eggs.Patch("/:id", mutationLimiter, func(c *fiber.Ctx) error {
		var req store.UpdateEggRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		egg, err := cfg.Store.UpdateEgg(ctx, c.Params("id"), req, actorID)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(egg)
	})

	eggs.Delete("/:id", mutationLimiter, func(c *fiber.Ctx) error {
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteEgg(ctx, c.Params("id"), actorID); err != nil {
			return respondInternalError(c, err)
		}
		return c.SendStatus(204)
	})
}

// userIDFromCtx extracts the authenticated user ID from the Fiber context locals.
func userIDFromCtx(c *fiber.Ctx) *string {
	v := c.Locals("userId")
	if s, ok := v.(string); ok && s != "" {
		return &s
	}
	return nil
}
