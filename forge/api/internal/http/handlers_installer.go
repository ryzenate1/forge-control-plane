package http

import (
	"strings"

	installersvc "gamepanel/forge/internal/services/installer"

	"github.com/gofiber/fiber/v2"
)

// registerInstallerRoutes wires installer workflow visibility and optional
// execution into the protected API. Visibility (DB→UI) is always on;
// execution is gated by INSTALLER_WORKFLOW_ENABLED (default off).
//
//	forge/api/internal/http/handlers_installer.go:14
func registerInstallerRoutes(protected fiber.Router, cfg Config) {
	if cfg.InstallerService == nil || cfg.Store == nil {
		// Store-required handlers are not registered when DB is unavailable
		// (dev mode). The feature remains documented as intentionally deferred.
		return
	}
	svc := cfg.InstallerService

	// Server-scoped: list workflows for a single server.
	// GET /api/v1/servers/:id/install-workflows
	protected.Get("/servers/:id/install-workflows", requireServerAccess(cfg), func(c *fiber.Ctx) error {
		serverID := c.Params("id")
		ctx, cancel := requestContext()
		defer cancel()
		workflows, err := svc.ListWorkflows(ctx, serverID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to list workflows: "+err.Error())
		}
		if workflows == nil {
			workflows = []installersvc.Workflow{}
		}
		return c.JSON(fiber.Map{
			"data": workflows,
			"meta": fiber.Map{
				"executionEnabled": svc.CanExecute(),
				"count":            len(workflows),
			},
		})
	})

	// Create a new workflow for a server (admin).
	// POST /api/v1/servers/:id/install-workflows  {type:"install"|"uninstall"|"reinstall"}
	protected.Post("/servers/:id/install-workflows", requireRole("admin"), requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		serverID := c.Params("id")
		var body struct {
			Type string `json:"type"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		t := strings.ToLower(strings.TrimSpace(body.Type))
		if t == "" {
			t = string(installersvc.WorkflowInstall)
		}
		ctx, cancel := requestContext()
		defer cancel()
		var wf *installersvc.Workflow
		var err error
		switch t {
		case string(installersvc.WorkflowInstall):
			wf, err = svc.CreateInstallWorkflow(ctx, serverID)
		case string(installersvc.WorkflowUninstall):
			wf, err = svc.CreateUninstallWorkflow(ctx, serverID)
		case string(installersvc.WorkflowReinstall):
			wf, err = svc.CreateReinstallWorkflow(ctx, serverID)
		default:
			return fiber.NewError(fiber.StatusBadRequest, "unknown workflow type: "+t)
		}
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to create workflow: "+err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(wf)
	})

	// Get single workflow by ID (access-checked via server ownership).
	// GET /api/v1/install-workflows/:id
	protected.Get("/install-workflows/:id", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		wf, err := svc.GetWorkflow(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "workflow not found")
		}
		// Enforce server access if user is bound to a server.
		if cfg.Store != nil {
			if claims, ok := c.Locals("user").(tokenClaims); ok {
				allowed, _ := cfg.Store.UserCanAccessServer(ctx, wf.ServerID, claims.Sub, claims.Role, "")
				if !allowed && claims.Role != RoleAdmin {
					return fiber.NewError(fiber.StatusNotFound, "workflow not found")
				}
			}
		}
		return c.JSON(wf)
	})

	// Execute workflow (feature-flagged). When INSTALLER_WORKFLOW_ENABLED != 1
	// the handler returns 409 with a deferred-intent message so UI can render
	// the "intentionally deferred — manual execution" banner.
	// POST /api/v1/install-workflows/:id/execute
	protected.Post("/install-workflows/:id/execute", requireRole("admin"), requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		if !installersvc.IsEnabled() {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error":            "installer workflow execution is intentionally deferred (INSTALLER_WORKFLOW_ENABLED=0)",
				"executionEnabled": false,
				"hint":             "Workflows are visible for audit/history. Set INSTALLER_WORKFLOW_ENABLED=1 to enable beacon execution. Existing Beacon POST /servers/:id/install remains the canonical install path (see clustermanager/service.go:216).",
			})
		}
		// Opting in is not the same as being able. With the flag on but no
		// executor wired, this used to answer 202 and then do nothing, leaving
		// the workflow reported as running for good.
		if !svc.CanExecute() {
			return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
				"error":            installersvc.ErrNoWorkflowExecutor.Error(),
				"executionEnabled": true,
				"executorWired":    false,
				"hint":             "Install via the canonical path (POST /servers/:id/install) until a workflow executor is wired.",
			})
		}
		ctx, cancel := requestContext()
		defer cancel()
		wf, err := svc.GetWorkflow(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "workflow not found")
		}
		if err := svc.ExecuteWorkflow(ctx, wf.ID); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "workflow execution failed: "+err.Error())
		}
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
			"accepted":         true,
			"workflowId":       wf.ID,
			"executionEnabled": true,
		})
	})

	// Admin global visibility: recent workflows across all servers.
	// GET /api/v1/admin/install-workflows?limit=20
	protected.Get("/admin/install-workflows", requireRole("admin"), func(c *fiber.Ctx) error {
		limit := queryLimit(c)
		if limit > 100 {
			limit = 100
		}
		ctx, cancel := requestContext()
		defer cancel()
		workflows, err := svc.ListRecentWorkflows(ctx, limit)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to list workflows: "+err.Error())
		}
		if workflows == nil {
			workflows = []installersvc.Workflow{}
		}
		return c.JSON(fiber.Map{
			"data": workflows,
			"meta": fiber.Map{
				"executionEnabled": svc.CanExecute(),
				"count":            len(workflows),
			},
		})
	})
}
