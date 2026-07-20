package http

import (
	"encoding/json"
	"strings"

	"gamepanel/forge/internal/services/compose"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type ComposeValidateRequest struct {
	Content string `json:"content"`
}

type ComposeImportRequest struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

func registerComposeRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	composeSvc := compose.New(cfg.Store)

	protected.Post("/compose/validate", requireRole("admin"), func(c *fiber.Ctx) error {
		var req ComposeValidateRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.Content) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "content is required")
		}

		result := composeSvc.ValidateCompose([]byte(req.Content), "")
		return c.JSON(result)
	})

	protected.Post("/compose/import", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		var req ComposeImportRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}

		if strings.TrimSpace(req.Name) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "name is required")
		}
		if strings.TrimSpace(req.Content) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "content is required")
		}

		result := composeSvc.ValidateCompose([]byte(req.Content), "")
		if !result.Valid {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
				"error":   "validation failed",
				"details": result,
			})
		}

		ctx, cancel := requestContext()
		defer cancel()

		parsedJSON, err := json.Marshal(result.Summary)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to serialize parsed config")
		}

		project := &store.ProjectDocument{
			ID:             uuid.NewString(),
			Name:           req.Name,
			ComposeContent: req.Content,
			ParsedConfig:   parsedJSON,
			Status:         "imported",
			Revision:       1,
		}

		if err := cfg.Store.CreateComposeProject(ctx, project); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		return c.Status(fiber.StatusCreated).JSON(project)
	})

	protected.Get("/compose/projects", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		projects, err := cfg.Store.ListComposeProjects(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(projects)
	})

	protected.Get("/compose/projects/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		project, err := cfg.Store.GetComposeProject(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "compose project not found")
		}
		return c.JSON(project)
	})

	protected.Put("/compose/projects/:id", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req ComposeImportRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.Content) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "content is required")
		}

		ctx, cancel := requestContext()
		defer cancel()

		existing, err := cfg.Store.GetComposeProject(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "compose project not found")
		}

		result := composeSvc.ValidateCompose([]byte(req.Content), "")
		if !result.Valid {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
				"error":   "validation failed",
				"details": result,
			})
		}

		parsedJSON, err := json.Marshal(result.Summary)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to serialize parsed config")
		}

		existing.ComposeContent = req.Content
		existing.ParsedConfig = parsedJSON
		existing.Revision++
		if req.Name != "" {
			existing.Name = req.Name
		}

		if err := cfg.Store.UpdateComposeProject(ctx, existing); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		return c.JSON(existing)
	})

	protected.Delete("/compose/projects/:id", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteComposeProject(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	protected.Post("/compose/projects/:id/export", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		project, err := cfg.Store.GetComposeProject(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "compose project not found")
		}
		c.Set("Content-Type", "application/x-yaml")
		c.Set("Content-Disposition", "attachment; filename=\""+sanitizeFilename(project.Name)+".yml\"")
		return c.SendString(project.ComposeContent)
	})

	protected.Get("/compose/projects/:id/summary", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		project, err := cfg.Store.GetComposeProject(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "compose project not found")
		}

		var summary compose.ParsedCompose
		if err := json.Unmarshal(project.ParsedConfig, &summary); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to parse stored config")
		}

		return c.JSON(fiber.Map{
			"project": project,
			"summary": summary,
		})
	})
}

func sanitizeFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, name)
	if name == "" {
		return "compose"
	}
	return name
}
