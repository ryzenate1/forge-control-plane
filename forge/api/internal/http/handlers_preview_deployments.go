package http

import (
	"context"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// previewDeploymentService is the contract the preview-deployment admin API
// needs. It is satisfied by the real services/previewenv implementation (which
// derives per-PR URLs, provisions TLS/routing and reports commit status) so the
// endpoint is backed by a genuine lifecycle rather than a stub that only flips
// status and fabricates URLs.
type previewDeploymentService interface {
	ListAll(ctx context.Context) ([]store.PreviewDeployment, error)
	Get(ctx context.Context, id string) (*store.PreviewDeployment, error)
	Create(ctx context.Context, serverID string, req *store.PreviewDeployment) (*store.PreviewDeployment, error)
	Deploy(ctx context.Context, id string) error
	Cleanup(ctx context.Context, id string) error
	UpdateStatus(ctx context.Context, id string, status string) error
	List(ctx context.Context, serverID string) ([]store.PreviewDeployment, error)
}

func registerPreviewDeploymentRoutes(protected fiber.Router, cfg Config, svc previewDeploymentService, adminIPAccess, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	pd := protected.Group("/admin/preview-deployments", adminIPAccess)

	pd.Get("/", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		previews, err := svc.ListAll(c.Context())
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": previews})
	})

	pd.Get("/:id", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		p, err := svc.Get(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": p})
	})

	pd.Post("/", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		var req struct {
			ServerID  string `json:"serverId"`
			PRNumber  int    `json:"prNumber"`
			PRTitle   string `json:"prTitle"`
			PRURL     string `json:"prUrl"`
			Branch    string `json:"branch"`
			RepoOwner string `json:"repoOwner"`
			RepoName  string `json:"repoName"`
			CommitSHA string `json:"commitSha"`
			Source    string `json:"source"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if req.ServerID == "" {
			return c.Status(400).JSON(fiber.Map{"error": "serverId is required"})
		}
		if req.Source == "" {
			req.Source = "github"
		}
		p, err := svc.Create(c.Context(), req.ServerID, &store.PreviewDeployment{
			PRNumber:  req.PRNumber,
			PRTitle:   req.PRTitle,
			PRURL:     req.PRURL,
			Branch:    req.Branch,
			RepoOwner: req.RepoOwner,
			RepoName:  req.RepoName,
			CommitSHA: req.CommitSHA,
			Source:    req.Source,
		})
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"data": p})
	})

	pd.Post("/:id/deploy", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		if err := svc.Deploy(c.Context(), c.Params("id")); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": fiber.Map{"id": c.Params("id")}})
	})

	pd.Post("/:id/cleanup", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		if err := svc.Cleanup(c.Context(), c.Params("id")); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": fiber.Map{"id": c.Params("id")}})
	})

	pd.Post("/:id/status", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		var req struct {
			Status string `json:"status"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if err := svc.UpdateStatus(c.Context(), c.Params("id"), req.Status); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": fiber.Map{"id": c.Params("id"), "status": req.Status}})
	})

	pd.Get("/server/:serverId", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		previews, err := svc.List(c.Context(), c.Params("serverId"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": previews})
	})
}
