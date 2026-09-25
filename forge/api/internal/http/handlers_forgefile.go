package http

import (
	"gamepanel/forge/internal/services/forgefile"

	"github.com/gofiber/fiber/v2"
)

// registerForgefileRoutes wires the environment-as-code endpoints that the web's
// lib/api/forgefile.ts calls. The service handles validation, listing, reading,
// and applying a Forgefile manifest.
func registerForgefileRoutes(protected fiber.Router, cfg Config, svc *forgefile.Service, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	ff := protected.Group("/forgefile", requireRole("admin"))

	// GET /forgefile — list all stored manifest slugs.
	ff.Get("", func(c *fiber.Ctx) error {
		slugs, err := svc.ListManifests(c.Context())
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"manifests": slugs})
	})

	// POST /forgefile/validate — parse and check without persisting.
	ff.Post("/validate", func(c *fiber.Ctx) error {
		var req struct {
			Content string `json:"content"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		manifest, warnings, err := forgefile.Validate([]byte(req.Content))
		if err != nil {
			return c.Status(422).JSON(fiber.Map{"valid": false, "error": err.Error(), "warnings": warnings})
		}
		return c.JSON(fiber.Map{"valid": true, "manifest": manifest, "warnings": warnings})
	})

	// POST /forgefile/apply — validate + materialize the declared resources.
	ff.Post("/apply", mutationLimiter, func(c *fiber.Ctx) error {
		var req struct {
			Content string `json:"content"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		userID := c.Locals("userId")
		role := c.Locals("userRole")
		orgID := c.Locals("organizationId")
		uid, _ := userID.(string)
		r, _ := role.(string)
		oid, _ := orgID.(string)
		result, err := svc.Apply(c.Context(), uid, r, []byte(req.Content), oid)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": result})
	})

	// GET /forgefile/:slug — read a stored manifest.
	ff.Get("/:slug", func(c *fiber.Ctx) error {
		slug := c.Params("slug")
		manifest, version, updatedAt, err := svc.GetManifest(c.Context(), slug)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.JSON(fiber.Map{
			"slug":      slug,
			"version":   version,
			"updatedAt": updatedAt,
			"manifest":  manifest,
		})
	})
}
