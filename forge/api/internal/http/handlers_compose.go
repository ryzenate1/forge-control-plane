package http

import (
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"time"

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

type GitOpsDeployRequest struct {
	Name            string            `json:"name"`
	NodeID          string            `json:"nodeId"`
	RepositoryURL   string            `json:"repositoryUrl"`
	RepositoryPath  string            `json:"repositoryPath,omitempty"`
	Branch          string            `json:"branch,omitempty"`
	EnvVars         map[string]string `json:"envVars,omitempty"`
	MemoryMB        int64             `json:"memoryMb"`
	CPUShares       int64             `json:"cpuShares"`
	DiskMB          int64             `json:"diskMb"`
	AutoUpdate      bool              `json:"autoUpdate"`
	PollIntervalSec int               `json:"pollIntervalSec,omitempty"`
	CredentialID    string            `json:"credentialId,omitempty"`
}

type WebhookRequest struct {
	Signature string `json:"signature"`
}

type CreateStackRequest struct {
	Name          string            `json:"name"`
	ComposeYAML   string            `json:"composeYaml"`
	NodeID        string            `json:"nodeId"`
	EnvVars       map[string]string `json:"envVars,omitempty"`
	MemoryMB      int64             `json:"memoryMb"`
	CPUShares     int64             `json:"cpuShares"`
	DiskMB        int64             `json:"diskMb"`
	ComposeType   string            `json:"composeType"`
	SourceType    string            `json:"sourceType"`
	EnvironmentID string            `json:"environmentId,omitempty"`
}

type UpdateStackRequest struct {
	ComposeYAML   string            `json:"composeYaml"`
	EnvVars       map[string]string `json:"envVars,omitempty"`
	MemoryMB      int64             `json:"memoryMb"`
	CPUShares     int64             `json:"cpuShares"`
	DiskMB        int64             `json:"diskMb"`
}

func registerComposeRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	composeSvc, err := compose.New(cfg.Store)
	if err != nil {
		slog.Error("failed to create compose service", "error", err)
		return
	}
	if cfg.ComposeService != nil {
		composeSvc = cfg.ComposeService
	}

	gitOpsSvc := compose.NewGitOpsService(cfg.Store, composeSvc, cfg.GitDeployService, nil, slog.Default())

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
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}

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

	// ---- GitOps Routes ----

	protected.Post("/compose/git/deploy", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		var req GitOpsDeployRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		userID := getUserID(c)
		if userID == "" {
			userID = "system"
		}
		if req.PollIntervalSec <= 0 {
			req.PollIntervalSec = 300
		}
		ctx, cancel := requestContext()
		defer cancel()
		result, err := gitOpsSvc.DeployFromGit(ctx, compose.GitDeployFromGitRequest{
			UserID:          userID,
			Name:            req.Name,
			NodeID:          req.NodeID,
			RepositoryURL:   req.RepositoryURL,
			RepositoryPath:  req.RepositoryPath,
			Branch:          req.Branch,
			EnvVars:         req.EnvVars,
			MemoryMB:        req.MemoryMB,
			CPUShares:       req.CPUShares,
			DiskMB:          req.DiskMB,
			AutoUpdate:      req.AutoUpdate,
			PollIntervalSec: req.PollIntervalSec,
			CredentialID:    req.CredentialID,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(result)
	})

	protected.Post("/compose/git/:id/redeploy", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		result, err := gitOpsSvc.RedeployFromGit(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(result)
	})

	protected.Get("/compose/git/:id/check-update", requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		preview, err := gitOpsSvc.CheckForUpdates(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(preview)
	})

	protected.Get("/compose/git/:id/preview", requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		preview, err := gitOpsSvc.GetUpdatePreview(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(preview)
	})

	protected.Post("/compose/git/:id/rollback", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		result, err := gitOpsSvc.RollbackToPrevious(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(result)
	})

	protected.Post("/compose/git/:id/pull-redeploy", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		result, err := gitOpsSvc.PullAndRedeploy(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(result)
	})

	protected.Put("/compose/git/:id/branch", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		var req struct {
			Branch string `json:"branch"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		stack, err := gitOpsSvc.SetBranch(ctx, c.Params("id"), req.Branch)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(stack)
	})

	protected.Put("/compose/git/:id/auto-update", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		var req struct {
			Enabled         bool `json:"enabled"`
			PollIntervalSec int  `json:"pollIntervalSec,omitempty"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		stack, err := gitOpsSvc.SetAutoUpdate(ctx, c.Params("id"), req.Enabled, req.PollIntervalSec)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(stack)
	})

	protected.Get("/compose/git/:id/drift", requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		result, err := gitOpsSvc.DetectDrift(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(result)
	})

	protected.Get("/compose/git/:id/status", requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		status, err := gitOpsSvc.GetGitStatus(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(status)
	})

	protected.Get("/compose/git/:id/last-webhook", requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		lastAt, err := gitOpsSvc.GetLastWebhookAt(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if lastAt == nil {
			return c.JSON(fiber.Map{"lastWebhookAt": nil})
		}
		return c.JSON(fiber.Map{"lastWebhookAt": lastAt.Format(time.RFC3339)})
	})

	// ---- Webhook Receiver (unauthenticated) ----

	protected.Post("/compose/webhook/:webhookId", func(c *fiber.Ctx) error {
		body := c.Body()
		signature := c.Get("X-Hub-Signature-256")
		if signature == "" {
			signature = c.Get("X-Git-Token")
		}
		if signature == "" {
			signature = c.Get("X-Gitea-Signature")
		}
		deliveryID := c.Get("X-GitHub-Delivery")
		ctx, cancel := requestContext()
		defer cancel()
		accepted, err := gitOpsSvc.HandleWebhook(ctx, c.Params("webhookId"), body, signature, deliveryID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"accepted": false,
				"error":    err.Error(),
			})
		}
		return c.JSON(fiber.Map{"accepted": accepted})
	})

	// ---- Compose Stack CRUD + Lifecycle Routes ----

	protected.Post("/compose", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req CreateStackRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.Name == "" || req.ComposeYAML == "" {
			return fiber.NewError(fiber.StatusBadRequest, "name and composeYaml are required")
		}
		userID := getUserID(c)
		ctx, cancel := requestContext()
		defer cancel()
		stack, err := composeSvc.DeployComposeStack(ctx, compose.DeployComposeRequest{
			UserID:        userID,
			Name:          req.Name,
			NodeID:        req.NodeID,
			ComposeYAML:   req.ComposeYAML,
			EnvVars:       req.EnvVars,
			MemoryMB:      req.MemoryMB,
			CPUShares:     req.CPUShares,
			DiskMB:        req.DiskMB,
			ComposeType:   req.ComposeType,
			SourceType:    req.SourceType,
			EnvironmentID: req.EnvironmentID,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(stack)
	})

	protected.Get("/compose", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		userID := getUserID(c)
		ctx, cancel := requestContext()
		defer cancel()
		stacks, err := composeSvc.ListStacks(ctx, userID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(stacks)
	})

	protected.Get("/compose/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		stack, err := composeSvc.GetStack(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "compose stack not found")
		}
		return c.JSON(stack)
	})

	protected.Patch("/compose/:id", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req UpdateStackRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		stack, err := composeSvc.UpdateComposeStack(ctx, c.Params("id"), compose.UpdateComposeRequest{
			ComposeYAML: req.ComposeYAML,
			EnvVars:     req.EnvVars,
			MemoryMB:    req.MemoryMB,
			CPUShares:   req.CPUShares,
			DiskMB:      req.DiskMB,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(stack)
	})

	protected.Delete("/compose/:id", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := composeSvc.DeleteComposeStack(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	protected.Post("/compose/:id/deploy", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		existing, err := composeSvc.GetStack(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "compose stack not found")
		}
		stack, err := composeSvc.DeployComposeStack(ctx, compose.DeployComposeRequest{
			UserID:        getUserID(c),
			Name:          existing.Name,
			NodeID:        existing.NodeID,
			ComposeYAML:   existing.ComposeYAML,
			EnvVars:       existing.EnvVars,
			MemoryMB:      existing.MemoryMB,
			CPUShares:     existing.CPUShares,
			DiskMB:        existing.DiskMB,
			ComposeType:   existing.ComposeType,
			SourceType:    existing.SourceType,
			EnvironmentID: existing.EnvironmentID,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(stack)
	})

	protected.Post("/compose/:id/stop", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		stack, err := composeSvc.StopStack(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(stack)
	})

	protected.Post("/compose/:id/start", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		stack, err := composeSvc.StartStack(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(stack)
	})

	protected.Get("/compose/:id/logs", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		service := c.Query("service")
		tail, _ := strconv.Atoi(c.Query("tail", "100"))
		ctx, cancel := requestContext()
		defer cancel()
		logs, err := composeSvc.GetStackLogs(ctx, c.Params("id"), service, tail)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(logs)
	})

	protected.Get("/compose/:id/status", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		status, err := composeSvc.GetStackStatus(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(status)
	})

	// ---- Legacy Compose Project Routes ----

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

func getUserID(c *fiber.Ctx) string {
	if val := c.Locals("userId"); val != nil {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
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
