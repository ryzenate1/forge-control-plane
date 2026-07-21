package http

import (
	"log/slog"

	"gamepanel/forge/internal/services/git"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// GitDeploymentHandlers contains handlers for Git deployment endpoints
type GitDeploymentHandlers struct {
	gitDeployService *git.DeployService
	store           *store.Store
	logger          *slog.Logger
}

// NewGitDeploymentHandlers creates a new Git deployment handlers instance
func NewGitDeploymentHandlers(
	gitDeployService *git.DeployService,
	store *store.Store,
	logger *slog.Logger,
) *GitDeploymentHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &GitDeploymentHandlers{
		gitDeployService: gitDeployService,
		store:           store,
		logger:          logger,
	}
}

// CreateGitDeploymentHandler creates the handler for triggering Git deployments
func (h *GitDeploymentHandlers) CreateGitDeploymentHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if h.store == nil || h.gitDeployService == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "git deployment service is not available")
		}

		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
		}

		var req git.DeployRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}

		if req.GitSourceID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "gitSourceId is required")
		}

		// Verify the Git source belongs to the user
		ctx, cancel := requestContext()
		defer cancel()

		gitSource, err := h.store.GetGitSource(ctx, req.GitSourceID)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "git source not found")
		}

		if gitSource.UserID != claims.Sub {
			return fiber.NewError(fiber.StatusForbidden, "you do not have permission to deploy this git source")
		}

		// Trigger the deployment
		deployResult, err := h.gitDeployService.TriggerDeployment(ctx, &req)
		if err != nil {
			h.logger.Error("Failed to trigger deployment", "error", err, "gitSourceId", req.GitSourceID)
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		return c.Status(fiber.StatusAccepted).JSON(deployResult)
	}
}

// GetGitDeploymentHandler creates the handler for getting Git deployment status
func (h *GitDeploymentHandlers) GetGitDeploymentHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if h.store == nil || h.gitDeployService == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "git deployment service is not available")
		}

		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
		}

		gitSourceID := c.Params("gitSourceId")
		if gitSourceID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "gitSourceId is required")
		}

		// Verify the Git source belongs to the user
		ctx, cancel := requestContext()
		defer cancel()

		gitSource, err := h.store.GetGitSource(ctx, gitSourceID)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "git source not found")
		}

		if gitSource.UserID != claims.Sub {
			return fiber.NewError(fiber.StatusForbidden, "you do not have permission to view deployments for this git source")
		}

		// Get the deployment status
		deployment, err := h.gitDeployService.GetDeploymentStatus(ctx, gitSourceID)
		if err != nil {
			h.logger.Error("Failed to get deployment status", "error", err, "gitSourceId", gitSourceID)
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		return c.JSON(deployment)
	}
}

// ListGitDeploymentsHandler creates the handler for listing Git deployments
func (h *GitDeploymentHandlers) ListGitDeploymentsHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if h.store == nil || h.gitDeployService == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "git deployment service is not available")
		}

		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
		}

		gitSourceID := c.Params("gitSourceId")
		if gitSourceID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "gitSourceId is required")
		}

		// Verify the Git source belongs to the user
		ctx, cancel := requestContext()
		defer cancel()

		gitSource, err := h.store.GetGitSource(ctx, gitSourceID)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "git source not found")
		}

		if gitSource.UserID != claims.Sub {
			return fiber.NewError(fiber.StatusForbidden, "you do not have permission to view deployments for this git source")
		}

		// Parse limit parameter
		limit := c.QueryInt("limit", 10)
		if limit <= 0 {
			limit = 10
		}
		if limit > 100 {
			limit = 100
		}

		// Get the deployment history
		deployments, err := h.gitDeployService.ListDeployments(ctx, gitSourceID, limit)
		if err != nil {
			h.logger.Error("Failed to list deployments", "error", err, "gitSourceId", gitSourceID)
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		return c.JSON(deployments)
	}
}

// CancelGitDeploymentHandler creates the handler for cancelling Git deployments
func (h *GitDeploymentHandlers) CancelGitDeploymentHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if h.store == nil || h.gitDeployService == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "git deployment service is not available")
		}

		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
		}

		deploymentID := c.Params("deploymentId")
		if deploymentID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "deploymentId is required")
		}

		// Get the deployment to verify ownership
		ctx, cancel := requestContext()
		defer cancel()

		deployment, err := h.store.GetGitDeployment(ctx, deploymentID)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "deployment not found")
		}

		// Verify the Git source belongs to the user
		gitSource, err := h.store.GetGitSource(ctx, deployment.GitSourceID)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "git source not found")
		}

		if gitSource.UserID != claims.Sub {
			return fiber.NewError(fiber.StatusForbidden, "you do not have permission to cancel this deployment")
		}

		// Cancel the deployment
		err = h.gitDeployService.CancelDeployment(ctx, deploymentID)
		if err != nil {
			h.logger.Error("Failed to cancel deployment", "error", err, "deploymentId", deploymentID)
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		return c.JSON(fiber.Map{
			"message": "deployment cancellation requested",
			"deploymentId": deploymentID,
		})
	}
}

// RegisterGitDeploymentRoutes registers the Git deployment routes
func RegisterGitDeploymentRoutes(router fiber.Router, cfg Config) {
	if cfg.GitDeployService == nil || cfg.Store == nil {
		return
	}

	handlers := NewGitDeploymentHandlers(cfg.GitDeployService, cfg.Store, cfg.Logger)

	// Git deployment routes
	router.Post("/git/sources/:gitSourceId/deploy", requireAuth(), handlers.CreateGitDeploymentHandler())
	router.Get("/git/sources/:gitSourceId/deployments", requireAuth(), handlers.ListGitDeploymentsHandler())
	router.Get("/git/sources/:gitSourceId/deployments/latest", requireAuth(), handlers.GetGitDeploymentHandler())
	router.Post("/git/deployments/:deploymentId/cancel", requireAuth(), handlers.CancelGitDeploymentHandler())
}

// requireAuth is a placeholder for the actual authentication middleware
func requireAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// This would be replaced with the actual authentication middleware
		return c.Next()
	}
}
