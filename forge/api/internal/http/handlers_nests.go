package http

import (
	"strings"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// registerNestRoutes wires the nests-and-eggs CRUD endpoints that the web's
// AdminNestsEggs component calls. Nests are top-level groupings; eggs are
// service blueprints inside a nest.
//
// This group is the only live registration for /nests and /eggs CRUD.
// registerAdminRoutes used to register the same routes with requireAdminScope
// and adminIPAccess; because Fiber resolves overlapping paths in registration
// order and this function runs first, those copies were unreachable and their
// scope checks never ran. The gating has been folded in here and the duplicates
// removed, so a scoped admin API key is enforced on the route that actually
// serves the request.
//
// The scopes stay on the nests.* family for eggs as well, matching the
// (previously dead) admin registration: an existing key granted nests.write
// must keep working on eggs.
func registerNestRoutes(protected fiber.Router, cfg Config, mutationLimiter, adminIPAccess fiber.Handler) {
	if cfg.Store == nil {
		return
	}

	// --- Nests ---
	nests := protected.Group("/nests", adminIPAccess, requireRole("admin"))

	nests.Get("", requireAdminScope("nests.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		list, err := cfg.Store.ListNests(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(list)
	})

	nests.Get("/:id", requireAdminScope("nests.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		nest, err := cfg.Store.GetNest(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "nest not found")
		}
		return c.JSON(nest)
	})

	nests.Post("", mutationLimiter, requireAdminScope("nests.write"), func(c *fiber.Ctx) error {
		// Bind the http-package type rather than store.CreateNestRequest: only
		// this one carries validate tags, so a missing name is rejected as a
		// 422 naming the field. The store enforces the same requirement but
		// reports it as a plain error, which used to surface as a 500.
		var req CreateNestRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := Validate(&req); err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(err)
		}
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		nest, err := cfg.Store.CreateNest(ctx, store.CreateNestRequest{
			Name:        req.Name,
			Description: req.Description,
		}, actorID)
		if err != nil {
			return respondStoreError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(nest)
	})

	nests.Patch("/:id", mutationLimiter, requireAdminScope("nests.write"), func(c *fiber.Ctx) error {
		var req UpdateNestRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		nest, err := cfg.Store.UpdateNest(ctx, c.Params("id"), store.UpdateNestRequest{
			Name:        req.Name,
			Description: req.Description,
		}, actorID)
		if err != nil {
			return respondStoreError(c, err)
		}
		return c.JSON(nest)
	})

	nests.Delete("/:id", mutationLimiter, requireAdminScope("nests.delete"), func(c *fiber.Ctx) error {
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteNest(ctx, c.Params("id"), actorID); err != nil {
			return respondStoreError(c, err)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	// GET /nests/:nestId/eggs — eggs of one nest. Registered on the nests group
	// so the nested path resolves here rather than falling through to the dead
	// admin copy.
	nests.Get("/:nestId/eggs", requireAdminScope("nests.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		list, err := cfg.Store.ListEggs(ctx, c.Params("nestId"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(list)
	})

	// --- Eggs ---
	eggs := protected.Group("/eggs", adminIPAccess, requireRole("admin"))

	eggs.Get("", requireAdminScope("nests.read"), func(c *fiber.Ctx) error {
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

	eggs.Get("/:id", requireAdminScope("nests.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		egg, err := cfg.Store.GetEgg(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "egg not found")
		}
		return c.JSON(egg)
	})

	eggs.Post("", mutationLimiter, requireAdminScope("nests.write"), func(c *fiber.Ctx) error {
		var req CreateEggRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := Validate(&req); err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(err)
		}
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		egg, err := cfg.Store.CreateEgg(ctx, store.CreateEggRequest{
			NestID:            req.NestID,
			Name:              req.Name,
			Description:       req.Description,
			DockerImages:      req.DockerImages,
			Startup:           req.Startup,
			Config:            req.Config,
			DefaultMemoryMB:   req.DefaultMemoryMB,
			InstallScript:     req.InstallScript,
			InstallContainer:  req.InstallContainer,
			InstallEntrypoint: req.InstallEntrypoint,
			FileDenylist:      req.FileDenylist,
			ConfigFrom:        req.ConfigFrom,
			CopyScriptFrom:    req.CopyScriptFrom,
			UpdateURL:         req.UpdateURL,
			Author:            req.Author,
			Features:          req.Features,
			StartupCommands:   req.StartupCommands,
		}, actorID)
		if err != nil {
			return respondStoreError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(egg)
	})

	eggs.Patch("/:id", mutationLimiter, requireAdminScope("nests.write"), func(c *fiber.Ctx) error {
		var req UpdateEggRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		egg, err := cfg.Store.UpdateEgg(ctx, c.Params("id"), store.UpdateEggRequest{
			Name:              req.Name,
			Description:       req.Description,
			DockerImages:      req.DockerImages,
			Startup:           req.Startup,
			Config:            req.Config,
			DefaultMemoryMB:   req.DefaultMemoryMB,
			InstallScript:     req.InstallScript,
			InstallContainer:  req.InstallContainer,
			InstallEntrypoint: req.InstallEntrypoint,
			FileDenylist:      req.FileDenylist,
			ConfigFrom:        req.ConfigFrom,
			CopyScriptFrom:    req.CopyScriptFrom,
			UpdateURL:         req.UpdateURL,
			Author:            req.Author,
			Features:          req.Features,
			StartupCommands:   req.StartupCommands,
		}, actorID)
		if err != nil {
			return respondStoreError(c, err)
		}
		return c.JSON(egg)
	})

	eggs.Delete("/:id", mutationLimiter, requireAdminScope("nests.delete"), func(c *fiber.Ctx) error {
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteEgg(ctx, c.Params("id"), actorID); err != nil {
			return respondStoreError(c, err)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
}

// userIDFromCtx extracts the authenticated user ID from the Fiber context
// locals for audit attribution. authMiddleware stores the caller as a
// tokenClaims value under "user"; it never sets a "userId" local, so reading
// that key returned nil for every request and every region/location/nest/egg/
// template mutation was persisted with a NULL actor.
func userIDFromCtx(c *fiber.Ctx) *string {
	if claims, ok := c.Locals("user").(tokenClaims); ok && claims.Sub != "" {
		sub := claims.Sub
		return &sub
	}
	return nil
}
