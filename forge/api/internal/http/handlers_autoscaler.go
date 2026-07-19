package http

import (
	"gamepanel/forge/internal/services/autoscaler"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func registerAutoScalerRoutes(protected fiber.Router, cfg Config, svc *autoscaler.Service, adminIPAccess, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	auto := protected.Group("/admin/autoscaler", adminIPAccess)

	auto.Get("/policies", requireRole("admin"), requireAdminScope("autoscaler.read"), func(c *fiber.Ctx) error {
		policies, err := svc.ListPolicies(c.Context())
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": policies})
	})

	auto.Get("/policies/:id", requireRole("admin"), requireAdminScope("autoscaler.read"), func(c *fiber.Ctx) error {
		policy, err := svc.GetPolicy(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": policy})
	})

	auto.Post("/policies", mutationLimiter, requireRole("admin"), requireAdminScope("autoscaler.write"), func(c *fiber.Ctx) error {
		var policy autoscaler.ScalingPolicy
		if err := c.BodyParser(&policy); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		policy.ID = uuid.NewString()
		if err := svc.CreatePolicy(c.Context(), &policy); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"data": policy})
	})

	auto.Put("/policies/:id", mutationLimiter, requireRole("admin"), requireAdminScope("autoscaler.write"), func(c *fiber.Ctx) error {
		var policy autoscaler.ScalingPolicy
		if err := c.BodyParser(&policy); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		policy.ID = c.Params("id")
		if err := svc.UpdatePolicy(c.Context(), &policy); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": policy})
	})

	auto.Delete("/policies/:id", mutationLimiter, requireRole("admin"), requireAdminScope("autoscaler.write"), func(c *fiber.Ctx) error {
		if err := svc.DeletePolicy(c.Context(), c.Params("id")); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(204)
	})

	auto.Get("/policies/server/:serverId", requireRole("admin"), requireAdminScope("autoscaler.read"), func(c *fiber.Ctx) error {
		policies, err := svc.ListPoliciesByServer(c.Context(), c.Params("serverId"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": policies})
	})

	auto.Post("/evaluate/:serverId", mutationLimiter, requireRole("admin"), requireAdminScope("autoscaler.write"), func(c *fiber.Ctx) error {
		event, err := svc.EvaluateServer(c.Context(), c.Params("serverId"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": event})
	})

	auto.Get("/metrics", requireRole("admin"), func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"data": svc.Metrics()})
	})
}
