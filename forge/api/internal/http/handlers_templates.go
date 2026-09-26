package http

import (
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// registerTemplateRoutes wires the legacy compatibility template endpoints that
// the web's AdminTemplates component calls. Templates are a thin view over eggs.
func registerTemplateRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	if cfg.Store == nil {
		return
	}

	tmpl := protected.Group("/templates", requireRole("admin"))

	tmpl.Get("", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		list, err := cfg.Store.ListTemplates(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(list)
	})

	tmpl.Get("/:id", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		t, err := cfg.Store.GetTemplate(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "template not found")
		}
		return c.JSON(t)
	})

	tmpl.Post("", mutationLimiter, func(c *fiber.Ctx) error {
		var req store.CreateTemplateRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		t, err := cfg.Store.CreateTemplate(ctx, req, actorID)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.Status(201).JSON(t)
	})

	tmpl.Delete("/:id", mutationLimiter, func(c *fiber.Ctx) error {
		actorID := userIDFromCtx(c)
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteEgg(ctx, c.Params("id"), actorID); err != nil {
			return respondInternalError(c, err)
		}
		return c.SendStatus(204)
	})
}
