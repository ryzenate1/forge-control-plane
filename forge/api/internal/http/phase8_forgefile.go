package http

import (
	"os"
	"strings"

	"gamepanel/forge/internal/services/forgefile"

	"github.com/gofiber/fiber/v2"
)

// forgefileBaseDomain reads the FORGEFILE_BASE_DOMAIN env var used to compute
// app links at apply time (empty => placeholder internal URLs).
func forgefileBaseDomain() string {
	return strings.TrimSpace(os.Getenv(forgefile.BaseDomainEnv))
}

// extractManifestBody yields raw forge.yaml bytes from either a raw YAML
// body or {"content": "<yaml text>"} JSON wrapper.
func extractManifestBody(c *fiber.Ctx) ([]byte, error) {
	ct := c.Get("Content-Type")
	body := c.Body()
	if strings.Contains(ct, "application/json") {
		var wrapper struct {
			Content string `json:"content"`
		}
		body = []byte(strings.TrimSpace(body))
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

func validateForgefileHandler(svc *forgefile.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var body []byte
		if c.Method() == fiber.MethodGet {
			raw := c.Query("manifest", c.Query("yaml"))
			if raw == "" {
				return fiber.NewError(fiber.StatusBadRequest, "manifest query parameter is required")
			}
			raw = strings.ReplaceAll(raw, "\\n", "\n")
			body = []byte(raw)
		} else {
			b, err := extractManifestBody(c)
			if err != nil {
				return err
			}
			body = b
		}
		_, warnings, err := forgefile.Validate(body)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"valid": false, "error": err.Error(), "warnings": warnings})
		}
		return c.JSON(fiber.Map{"valid": true, "warnings": warnings})
	}
}

func applyForgefileHandler(svc *forgefile.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
		}
		body, err := extractManifestBody(c)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		result, err := svc.Apply(ctx, claims.Sub, claims.Role, body, "")
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(result)
	}
}

func listForgefileManifestsHandler(svc *forgefile.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		slugs, err := svc.ListManifests(ctx)
		if err != nil {
			if strings.Contains(err.Error(), "forge_manifests") && strings.Contains(err.Error(), "does not exist") {
				return c.JSON(fiber.Map{"manifests": []string{}, "migrationsPending": true})
			}
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if slugs == nil {
			slugs = []string{}
		}
		return c.JSON(fiber.Map{"manifests": slugs})
	}
}

func getForgefileHandler(svc *forgefile.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		slug := c.Params("slug")
		ctx, cancel := requestContext()
		defer cancel()
		m, version, updatedAt, err := svc.GetManifest(ctx, slug)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "manifest not found")
		}
		return c.JSON(fiber.Map{
			"slug":      slug,
			"version":   version,
			"updatedAt": updatedAt,
			"manifest":  m,
		})
	}
}

// publicForgefileHandler is the read-only manifest fetch the UI uses to
// render forge.yaml state without a session context beyond auth.
func publicForgefileHandler(svc *forgefile.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		slug := c.Params("slug")
		ctx, cancel := requestContext()
		defer cancel()
		m, version, updatedAt, err := svc.GetManifest(ctx, slug)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "manifest not found")
		}
		return c.JSON(fiber.Map{
			"slug":      slug,
			"version":   version,
			"updatedAt": updatedAt,
			"project":   m.Project,
			"deploy":    m.Deploy,
			"environments": m.Environments,
		})
	}
}