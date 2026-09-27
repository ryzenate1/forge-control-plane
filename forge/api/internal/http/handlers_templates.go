package http

import (
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// registerTemplateRoutes wires the legacy compatibility template endpoints.
// A template is a thin projection of an egg, so the nests.* scope family gates
// it: there is no templates.* scope in store.AdminScopes, and minting one would
// silently reject every API key that already carries nests.write.
//
// This group is the only live registration for /templates.
// registerAdminRoutes registered the same three routes; because Fiber resolves
// overlapping paths in registration order and this function runs first, those
// copies were unreachable. Two of them (GET "" and GET "/:id") also lacked
// requireRole("admin") entirely, so folding the gating in here and deleting
// them closes that gap rather than widening it.
//
// Only POST /templates has a frontend consumer (lib/api.ts createTemplate,
// used by AdminTemplates); the reads and the delete are API-only. The
// dashboard lists templates through GET /eggs.
func registerTemplateRoutes(protected fiber.Router, cfg Config, mutationLimiter, adminIPAccess fiber.Handler) {
	if cfg.Store == nil {
		return
	}

	tmpl := protected.Group("/templates", adminIPAccess, requireRole("admin"))

	tmpl.Get("", requireAdminScope("nests.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		list, err := cfg.Store.ListTemplates(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(list)
	})

	tmpl.Get("/:id", requireAdminScope("nests.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		t, err := cfg.Store.GetTemplate(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "template not found")
		}
		return c.JSON(t)
	})

	tmpl.Post("", mutationLimiter, requireAdminScope("nests.write"), func(c *fiber.Ctx) error {
		// Bind the http-package type rather than store.CreateTemplateRequest:
		// only this one carries validate tags, so a missing name or image is
		// rejected as a 422 naming the field. The store enforces the same
		// requirement but reports it as a plain error, which used to surface as
		// a 500.
		var req CreateTemplateRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := Validate(&req); err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(err)
		}
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		t, err := cfg.Store.CreateTemplate(ctx, store.CreateTemplateRequest{
			Name:            req.Name,
			Image:           req.Image,
			StartupCommand:  req.StartupCommand,
			DefaultMemoryMB: req.DefaultMemoryMB,
		}, actorID)
		if err != nil {
			return respondStoreError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(t)
	})

	// A template is an egg, so deleting one deletes the backing egg.
	tmpl.Delete("/:id", mutationLimiter, requireAdminScope("nests.delete"), func(c *fiber.Ctx) error {
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteEgg(ctx, c.Params("id"), actorID); err != nil {
			return respondStoreError(c, err)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
}
