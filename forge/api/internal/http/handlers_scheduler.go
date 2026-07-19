package http

import (
	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/services/scheduler"
	"github.com/gofiber/fiber/v2"
)

func registerSchedulerRoutes(protected fiber.Router, cfg Config, scorer *scheduler.PredictiveScorer, constraintScheduler *scheduler.ConstraintScheduler, adminIPAccess, mutationLimiter fiber.Handler) {
	sc := protected.Group("/admin/scheduler", adminIPAccess)

	if scorer != nil {
		sc.Get("/predictive/nodes/:nodeId/score", requireRole("admin"), requireAdminScope("scheduler.read"), func(c *fiber.Ctx) error {
			score, err := scorer.ScorePredictive(c.Context(), c.Params("nodeId"), domain.PlacementRequest{})
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": err.Error()})
			}
			return c.JSON(fiber.Map{"data": score})
		})

		sc.Post("/predictive/metrics/:nodeId", mutationLimiter, requireRole("admin"), requireAdminScope("scheduler.write"), func(c *fiber.Ctx) error {
			var metric scheduler.ResourceMetric
			if err := c.BodyParser(&metric); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": err.Error()})
			}
			scorer.RecordMetric(c.Context(), c.Params("nodeId"), metric)
			return c.SendStatus(201)
		})

		sc.Get("/predictive/affinity-rules", requireRole("admin"), requireAdminScope("scheduler.read"), func(c *fiber.Ctx) error {
			rules, err := scorer.ListAffinityRules(c.Context())
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": err.Error()})
			}
			return c.JSON(fiber.Map{"data": rules})
		})

		sc.Post("/predictive/affinity-rules", mutationLimiter, requireRole("admin"), requireAdminScope("scheduler.write"), func(c *fiber.Ctx) error {
			var rule scheduler.AffinityRule
			if err := c.BodyParser(&rule); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": err.Error()})
			}
			if err := scorer.AddAffinityRule(c.Context(), rule); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(201).JSON(fiber.Map{"data": rule})
		})

		sc.Delete("/predictive/affinity-rules/:id", mutationLimiter, requireRole("admin"), requireAdminScope("scheduler.write"), func(c *fiber.Ctx) error {
			if err := scorer.RemoveAffinityRule(c.Context(), c.Params("id")); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": err.Error()})
			}
			return c.SendStatus(204)
		})

		sc.Get("/predictive/anti-affinity-rules", requireRole("admin"), requireAdminScope("scheduler.read"), func(c *fiber.Ctx) error {
			rules, err := scorer.ListAntiAffinityRules(c.Context())
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": err.Error()})
			}
			return c.JSON(fiber.Map{"data": rules})
		})

		sc.Post("/predictive/anti-affinity-rules", mutationLimiter, requireRole("admin"), requireAdminScope("scheduler.write"), func(c *fiber.Ctx) error {
			var rule scheduler.AntiAffinityRule
			if err := c.BodyParser(&rule); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": err.Error()})
			}
			if err := scorer.AddAntiAffinityRule(c.Context(), rule); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(201).JSON(fiber.Map{"data": rule})
		})

		sc.Delete("/predictive/anti-affinity-rules/:id", mutationLimiter, requireRole("admin"), requireAdminScope("scheduler.write"), func(c *fiber.Ctx) error {
			if err := scorer.RemoveAntiAffinityRule(c.Context(), c.Params("id")); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": err.Error()})
			}
			return c.SendStatus(204)
		})
	}

	if constraintScheduler != nil {
		sc.Get("/constraints", requireRole("admin"), requireAdminScope("scheduler.read"), func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{"data": constraintScheduler.GetConstraints()})
		})

		sc.Put("/constraints", mutationLimiter, requireRole("admin"), requireAdminScope("scheduler.write"), func(c *fiber.Ctx) error {
			var constraints []scheduler.Constraint
			if err := c.BodyParser(&constraints); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": err.Error()})
			}
			constraintScheduler.SetConstraints(constraints)
			return c.JSON(fiber.Map{"data": constraints})
		})
	}
}
