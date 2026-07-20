package http

import (
	"gamepanel/forge/internal/services/deployment"

	"github.com/gofiber/fiber/v2"
)

func registerRevisionRoutes(protected fiber.Router, cfg Config, svc *deployment.Service, adminIPAccess, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	rev := protected.Group("/admin/deployments", adminIPAccess)

	rev.Get("/:id/revisions", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		revisions, err := svc.ListRevisions(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": revisions})
	})

	rev.Get("/:id/revisions/:revId", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		r, err := svc.GetRevision(c.Context(), c.Params("revId"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": r})
	})

	rev.Post("/:id/revisions/:revId/rollback", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		d, err := svc.RollbackToRevision(c.Context(), c.Params("id"), c.Params("revId"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": d})
	})

	rev.Post("/:id/rollback-previous", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		d, err := svc.RollbackToPrevious(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": d})
	})

	rev.Post("/:id/rollout", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		var req deployment.RolloutRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		req.ServerID = c.Params("id")
		d, err := svc.StartRollout(c.Context(), &req)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"data": d})
	})

	rev.Get("/:id/compare", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		fromID := c.Query("from")
		toID := c.Query("to")
		if fromID == "" || toID == "" {
			return c.Status(400).JSON(fiber.Map{"error": "from and to query parameters are required"})
		}
		diff, err := svc.CompareRevisions(c.Context(), fromID, toID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": diff})
	})
}
