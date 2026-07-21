package http

import (
	"encoding/json"
	"strings"

	"gamepanel/forge/internal/domain"
	schedulersvc "gamepanel/forge/internal/services/scheduler"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerSchedulerRoutes(protected fiber.Router, cfg Config, scorer *schedulersvc.PredictiveScorer, constraintScheduler *schedulersvc.ConstraintScheduler, adminIPAccess, mutationLimiter fiber.Handler) {
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
			var metric schedulersvc.ResourceMetric
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
			var rule schedulersvc.AffinityRule
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
			var rule schedulersvc.AntiAffinityRule
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
			var constraints []schedulersvc.Constraint
			if err := c.BodyParser(&constraints); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": err.Error()})
			}
			constraintScheduler.SetConstraints(constraints)
			return c.JSON(fiber.Map{"data": constraints})
		})
	}

	// ---- Scheduler backend management ----

	sc.Get("/backends", requireRole("admin"), requireAdminScope("scheduler.read"), func(c *fiber.Ctx) error {
		backends := []fiber.Map{
			{"type": "docker", "name": "Docker", "description": "Docker container runtime (default)"},
			{"type": "k3s", "name": "K3s", "description": "Lightweight Kubernetes via kubectl"},
			{"type": "nomad", "name": "Nomad", "description": "HashiCorp Nomad via nomad CLI"},
		}
		return c.JSON(fiber.Map{"data": backends})
	})

	// Node scheduler configuration update
	sc.Put("/nodes/:id/scheduler", mutationLimiter, requireRole("admin"), requireAdminScope("nodes.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			SchedulerType   string           `json:"schedulerType"`
			SchedulerConfig *json.RawMessage `json:"schedulerConfig,omitempty"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		sType := strings.TrimSpace(req.SchedulerType)
		if sType == "" {
			return fiber.NewError(fiber.StatusBadRequest, "schedulerType is required")
		}
		if sType != "docker" && sType != "k3s" && sType != "nomad" {
			return fiber.NewError(fiber.StatusBadRequest, "schedulerType must be one of: docker, k3s, nomad")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		node, err := cfg.Store.PatchNode(ctx, c.Params("id"), store.NodePatch{
			SchedulerType:   &sType,
			SchedulerConfig: req.SchedulerConfig,
		}, actorID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return fiber.NewError(fiber.StatusNotFound, err.Error())
			}
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(node)
	})
}
