package http

import (
	"gamepanel/forge/internal/services/deployment"
	"github.com/gofiber/fiber/v2"
)

func registerDeploymentRoutes(protected fiber.Router, cfg Config, svc *deployment.Service, adminIPAccess, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	dep := protected.Group("/admin/deployments", adminIPAccess)

	dep.Post("/blue-green", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		var req struct {
			ServerID        string `json:"serverId"`
			Image           string `json:"image"`
			HealthCheckPath string `json:"healthCheckPath"`
			HealthCheckPort int    `json:"healthCheckPort"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		d, err := svc.StartBlueGreen(c.Context(), req.ServerID, req.Image, req.HealthCheckPath, req.HealthCheckPort)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"data": d})
	})

	dep.Post("/:id/rollback", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		d, err := svc.Rollback(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": d})
	})

	dep.Post("/:id/complete", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		d, err := svc.CompleteDeployment(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": d})
	})

	dep.Post("/:id/cancel", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		d, err := svc.CancelDeployment(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": d})
	})

	dep.Get("/:id", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		d, err := svc.GetDeployment(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": d})
	})

	dep.Get("/server/:serverId", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		deployments, err := svc.ListDeployments(c.Context(), c.Params("serverId"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": deployments})
	})
}
