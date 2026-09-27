package http

import (
	"strings"

	"gamepanel/forge/internal/services/forgefile"

	"github.com/gofiber/fiber/v2"
)

// registerForgefileRoutes wires the environment-as-code endpoints that the web's
// lib/api/forgefile.ts calls. The service handles validation, listing, reading,
// and applying a Forgefile manifest.
//
// This group is the only live registration for /forgefile.
// RegisterPhase8 (phase8_registrar.go) registered the same paths on an
// ungated router; Fiber resolves overlapping paths in registration order and
// the phase registrars run after every register*Routes call, so those copies
// could never answer. Their two genuine improvements — accepting a raw
// forge.yaml body as well as {"content": "..."}, and the GET form of validate
// — have been folded in here, and the duplicates removed.
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
		if slugs == nil {
			// Encode an empty list rather than JSON null: the web client
			// branches on Array.isArray(manifests) and falls through to
			// returning null, which then blows up in .map().
			slugs = []string{}
		}
		return c.JSON(fiber.Map{"manifests": slugs})
	})

	// GET /forgefile/validate?manifest=... — validate without persisting, for
	// callers that cannot send a body. Registered ahead of GET /forgefile/:slug
	// because :slug would otherwise match "validate" and answer 404.
	ff.Get("/validate", func(c *fiber.Ctx) error {
		raw := c.Query("manifest", c.Query("yaml"))
		if raw == "" {
			return fiber.NewError(fiber.StatusBadRequest, "manifest query parameter is required")
		}
		// Query strings cannot carry real newlines, so accept the escaped form.
		raw = strings.ReplaceAll(raw, "\\n", "\n")
		manifest, warnings, err := forgefile.Validate([]byte(raw))
		if err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"valid": false, "error": err.Error(), "warnings": warnings})
		}
		return c.JSON(fiber.Map{"valid": true, "manifest": manifest, "warnings": warnings})
	})

	// POST /forgefile/validate — parse and check without persisting.
	ff.Post("/validate", func(c *fiber.Ctx) error {
		content, err := extractManifestBody(c)
		if err != nil {
			return err
		}
		manifest, warnings, err := forgefile.Validate(content)
		if err != nil {
			return c.Status(422).JSON(fiber.Map{"valid": false, "error": err.Error(), "warnings": warnings})
		}
		return c.JSON(fiber.Map{"valid": true, "manifest": manifest, "warnings": warnings})
	})

	// POST /forgefile/apply — validate + materialize the declared resources.
	ff.Post("/apply", mutationLimiter, func(c *fiber.Ctx) error {
		content, err := extractManifestBody(c)
		if err != nil {
			return err
		}
		// authMiddleware stores the caller as tokenClaims under "user". It does
		// not set "userId", "userRole" or "organizationId", so the previous
		// reads always yielded "" and every apply persisted its manifest with
		// an empty author while the service resolved the org from an empty
		// user. Take the identity from the claims instead.
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
		}
		// The org is left to the service: this route is admin-only and carries
		// no :orgId, so forgefile resolves the target org from the caller.
		result, err := svc.Apply(c.Context(), claims.Sub, claims.Role, content, "")
		if err != nil {
			return respondStoreError(c, err)
		}
		return c.JSON(fiber.Map{"data": result})
	})

	// GET /forgefile/:slug — read a stored manifest. Registered last so the
	// static /forgefile/validate path above resolves to its own handler.
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

// extractManifestBody yields raw forge.yaml bytes from either a raw YAML body
// or a {"content": "<yaml text>"} JSON wrapper. The dashboard's editor sends
// the JSON form; CI and curl callers send the file itself.
func extractManifestBody(c *fiber.Ctx) ([]byte, error) {
	ct := c.Get("Content-Type")
	body := c.Body()
	if strings.Contains(ct, "application/json") {
		var wrapper struct {
			Content string `json:"content"`
		}
		body = []byte(strings.TrimSpace(string(body)))
		if len(body) == 0 {
			return nil, fiber.NewError(fiber.StatusBadRequest, "forge.yaml content is required")
		}
		if err := c.BodyParser(&wrapper); err != nil || strings.TrimSpace(wrapper.Content) == "" {
			return nil, fiber.NewError(fiber.StatusBadRequest, `send {"content":"<forge.yaml text>"} or a raw forge.yaml body`)
		}
		return []byte(wrapper.Content), nil
	}
	if len(body) == 0 {
		return nil, fiber.NewError(fiber.StatusBadRequest, "forge.yaml content is required")
	}
	return body, nil
}
