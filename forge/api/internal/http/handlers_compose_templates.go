package http

import (
	"log/slog"
	"strings"

	"gamepanel/forge/internal/services/compose"
	composetemplatessvc "gamepanel/forge/internal/services/composetemplates"

	"github.com/gofiber/fiber/v2"
)

// CreateComposeTemplateRequest is the body for POST /admin/compose-templates.
type CreateComposeTemplateRequest struct {
	Name        string                                  `json:"name"`
	Description string                                  `json:"description"`
	Category    string                                  `json:"category"`
	LogoURL     string                                  `json:"logoUrl"`
	ComposeYAML string                                  `json:"composeYaml"`
	Parameters  []composetemplatessvc.TemplateParameter `json:"parameters"`
	Visibility  string                                  `json:"visibility"`
}

// UpdateComposeTemplateRequest is the body for PATCH /admin/compose-templates/:id.
// Every field is optional; empty values fall back to the stored template.
type UpdateComposeTemplateRequest struct {
	Name        string                                   `json:"name"`
	Description string                                   `json:"description"`
	Category    string                                   `json:"category"`
	LogoURL     string                                   `json:"logoUrl"`
	ComposeYAML string                                   `json:"composeYaml"`
	Parameters  *[]composetemplatessvc.TemplateParameter `json:"parameters"`
	Visibility  string                                   `json:"visibility"`
}

// InstantiateComposeTemplateRequest is the body for POST /admin/compose-templates/:id/instantiate.
type InstantiateComposeTemplateRequest struct {
	Name   string            `json:"name"`
	NodeID string            `json:"nodeId"`
	Values map[string]string `json:"values"`
}

// PreviewComposeTemplateRequest is the body for POST /admin/compose-templates/preview.
// It renders either a saved template (templateId) or an inline draft (composeYaml
// + parameters) without deploying anything.
type PreviewComposeTemplateRequest struct {
	TemplateID  string                                  `json:"templateId"`
	ComposeYAML string                                  `json:"composeYaml"`
	Parameters  []composetemplatessvc.TemplateParameter `json:"parameters"`
	Values      map[string]string                       `json:"values"`
}

func registerComposeTemplateRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	if cfg.Store == nil {
		return
	}

	svc := cfg.ComposeTemplateService
	if svc == nil {
		composeSvc := cfg.ComposeService
		if composeSvc == nil {
			built, err := compose.New(cfg.Store, cfg.Daemon)
			if err != nil {
				slog.Error("failed to create compose service for templates", "error", err)
				return
			}
			composeSvc = built
		}
		svc = composetemplatessvc.New(cfg.Store, composeSvc)
	}

	protected.Get("/admin/compose-templates", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		templates, err := svc.ListTemplates(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(templates)
	})

	protected.Post("/admin/compose-templates", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		var req CreateComposeTemplateRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		tpl := composetemplatessvc.Template{
			Name:        req.Name,
			Description: req.Description,
			Category:    req.Category,
			LogoURL:     req.LogoURL,
			ComposeYAML: req.ComposeYAML,
			Parameters:  req.Parameters,
			Visibility:  req.Visibility,
			CreatedBy:   getUserID(c),
		}
		ctx, cancel := requestContext()
		defer cancel()
		created, err := svc.CreateTemplate(ctx, tpl)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(created)
	})

	// Static route registered before the :id parameter routes so "preview" is
	// never treated as a template id.
	protected.Post("/admin/compose-templates/preview", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		var req PreviewComposeTemplateRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.TemplateID) == "" && strings.TrimSpace(req.ComposeYAML) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "templateId or composeYaml is required")
		}
		var inline *composetemplatessvc.Template
		if strings.TrimSpace(req.TemplateID) == "" {
			inline = &composetemplatessvc.Template{
				ComposeYAML: req.ComposeYAML,
				Parameters:  req.Parameters,
			}
		}
		ctx, cancel := requestContext()
		defer cancel()
		rendered, err := svc.RenderPreview(ctx, req.TemplateID, inline, req.Values)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"composeYaml": rendered})
	})

	protected.Get("/admin/compose-templates/:id", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		tpl, err := svc.GetTemplate(ctx, c.Params("id"))
		if err != nil {
			if isComposeTemplateNotFound(err) {
				return fiber.NewError(fiber.StatusNotFound, "compose template not found")
			}
			return respondInternalError(c, err)
		}
		return c.JSON(tpl)
	})

	protected.Patch("/admin/compose-templates/:id", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		var req UpdateComposeTemplateRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		patch := composetemplatessvc.Template{
			Name:        req.Name,
			Description: req.Description,
			Category:    req.Category,
			LogoURL:     req.LogoURL,
			ComposeYAML: req.ComposeYAML,
			Visibility:  req.Visibility,
		}
		if req.Parameters != nil {
			patch.Parameters = *req.Parameters
		}
		ctx, cancel := requestContext()
		defer cancel()
		updated, err := svc.UpdateTemplate(ctx, c.Params("id"), patch)
		if err != nil {
			if isComposeTemplateNotFound(err) {
				return fiber.NewError(fiber.StatusNotFound, "compose template not found")
			}
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(updated)
	})

	protected.Delete("/admin/compose-templates/:id", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.DeleteTemplate(ctx, c.Params("id")); err != nil {
			if isComposeTemplateNotFound(err) {
				return fiber.NewError(fiber.StatusNotFound, "compose template not found")
			}
			return respondInternalError(c, err)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	protected.Post("/admin/compose-templates/:id/instantiate", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		var req InstantiateComposeTemplateRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.Name) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "name is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		result, err := svc.InstantiateTemplate(ctx, c.Params("id"), req.Values, req.Name, req.NodeID, getUserID(c))
		if err != nil {
			if isComposeTemplateNotFound(err) {
				return fiber.NewError(fiber.StatusNotFound, "compose template not found")
			}
			return respondInternalError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(result)
	})
}

func isComposeTemplateNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), composetemplatessvc.ErrTemplateNotFound.Error())
}
