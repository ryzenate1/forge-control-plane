package http

import (
	"gamepanel/forge/internal/services/trafficmanager"
	"github.com/gofiber/fiber/v2"
)

func registerTrafficManagerRoutes(protected fiber.Router, cfg Config, svc *trafficmanager.Service, adminIPAccess, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	tm := protected.Group("/admin/traffic", adminIPAccess)

	tm.Get("/rules", requireRole("admin"), requireAdminScope("traffic.read"), func(c *fiber.Ctx) error {
		rules, err := svc.ListRoutingRules(c.Context())
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": rules})
	})

	tm.Post("/rules", mutationLimiter, requireRole("admin"), requireAdminScope("traffic.write"), func(c *fiber.Ctx) error {
		var rule trafficmanager.RoutingRule
		if err := c.BodyParser(&rule); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if err := svc.CreateRoutingRule(c.Context(), &rule); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"data": rule})
	})

	tm.Get("/rules/:id", requireRole("admin"), requireAdminScope("traffic.read"), func(c *fiber.Ctx) error {
		rule, err := svc.GetRoutingRule(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": rule})
	})

	tm.Put("/rules/:id", mutationLimiter, requireRole("admin"), requireAdminScope("traffic.write"), func(c *fiber.Ctx) error {
		var rule trafficmanager.RoutingRule
		if err := c.BodyParser(&rule); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		rule.ID = c.Params("id")
		if err := svc.UpdateRoutingRule(c.Context(), &rule); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": rule})
	})

	tm.Delete("/rules/:id", mutationLimiter, requireRole("admin"), requireAdminScope("traffic.write"), func(c *fiber.Ctx) error {
		if err := svc.DeleteRoutingRule(c.Context(), c.Params("id")); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(204)
	})

	tm.Get("/rules/server/:serverId", requireRole("admin"), requireAdminScope("traffic.read"), func(c *fiber.Ctx) error {
		rules, err := svc.ListRoutingRulesByServer(c.Context(), c.Params("serverId"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": rules})
	})

	tm.Post("/policies", mutationLimiter, requireRole("admin"), requireAdminScope("traffic.write"), func(c *fiber.Ctx) error {
		var policy trafficmanager.TrafficPolicy
		if err := c.BodyParser(&policy); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if err := svc.CreateTrafficPolicy(c.Context(), &policy); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"data": policy})
	})

	tm.Get("/policies/:id", requireRole("admin"), requireAdminScope("traffic.read"), func(c *fiber.Ctx) error {
		policy, err := svc.GetTrafficPolicy(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": policy})
	})

	tm.Put("/policies/:id", mutationLimiter, requireRole("admin"), requireAdminScope("traffic.write"), func(c *fiber.Ctx) error {
		var policy trafficmanager.TrafficPolicy
		if err := c.BodyParser(&policy); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		policy.ID = c.Params("id")
		if err := svc.UpdateTrafficPolicy(c.Context(), &policy); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": policy})
	})

	tm.Delete("/policies/:id", mutationLimiter, requireRole("admin"), requireAdminScope("traffic.write"), func(c *fiber.Ctx) error {
		if err := svc.DeleteTrafficPolicy(c.Context(), c.Params("id")); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(204)
	})

	tm.Post("/sync", mutationLimiter, requireRole("admin"), requireAdminScope("traffic.write"), func(c *fiber.Ctx) error {
		if err := svc.SyncRoutes(c.Context()); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "routes synced"})
	})
}
