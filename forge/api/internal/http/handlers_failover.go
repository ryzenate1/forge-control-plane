package http

import (
	"errors"

	"gamepanel/forge/internal/services/failover"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func registerFailoverRoutes(protected fiber.Router, cfg Config, svc *failover.Service, adminIPAccess, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	fo := protected.Group("/admin/failover", adminIPAccess)

	fo.Get("/policies", requireRole("admin"), requireAdminScope("failover.read"), func(c *fiber.Ctx) error {
		policies, err := svc.ListPolicies(c.Context())
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": policies})
	})

	fo.Post("/policies", mutationLimiter, requireRole("admin"), requireAdminScope("failover.write"), func(c *fiber.Ctx) error {
		var policy failover.Policy
		if err := c.BodyParser(&policy); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		policy.ID = uuid.NewString()
		if err := svc.CreatePolicy(c.Context(), &policy); err != nil {
			return c.Status(failoverErrorStatus(err)).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"data": policy})
	})

	fo.Get("/policies/:id", requireRole("admin"), requireAdminScope("failover.read"), func(c *fiber.Ctx) error {
		policy, err := svc.GetPolicy(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(failoverErrorStatus(err)).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": policy})
	})

	fo.Put("/policies/:id", mutationLimiter, requireRole("admin"), requireAdminScope("failover.write"), func(c *fiber.Ctx) error {
		var policy failover.Policy
		if err := c.BodyParser(&policy); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		policy.ID = c.Params("id")
		if err := svc.UpdatePolicy(c.Context(), &policy); err != nil {
			return c.Status(failoverErrorStatus(err)).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": policy})
	})

	fo.Delete("/policies/:id", mutationLimiter, requireRole("admin"), requireAdminScope("failover.write"), func(c *fiber.Ctx) error {
		if err := svc.DeletePolicy(c.Context(), c.Params("id")); err != nil {
			return c.Status(failoverErrorStatus(err)).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(204)
	})

	fo.Get("/policies/node/:nodeId", requireRole("admin"), requireAdminScope("failover.read"), func(c *fiber.Ctx) error {
		policies, err := svc.ListPoliciesByNode(c.Context(), c.Params("nodeId"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": policies})
	})

	fo.Post("/record-failure/:nodeId", mutationLimiter, requireRole("admin"), requireAdminScope("failover.write"), func(c *fiber.Ctx) error {
		event, err := svc.RecordFailure(c.Context(), c.Params("nodeId"))
		if err != nil {
			return c.Status(failoverErrorStatus(err)).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": event})
	})

	fo.Post("/crash/:serverId/:nodeId", mutationLimiter, requireRole("admin"), requireAdminScope("failover.write"), func(c *fiber.Ctx) error {
		event, err := svc.HandleServerCrash(c.Context(), c.Params("serverId"), c.Params("nodeId"))
		if err != nil {
			return c.Status(failoverErrorStatus(err)).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": event})
	})

	fo.Get("/metrics", requireRole("admin"), requireAdminScope("failover.read"), func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"data": svc.Metrics()})
	})
}

func failoverErrorStatus(err error) int {
	if errors.Is(err, failover.ErrPolicyNotFound) {
		return fiber.StatusNotFound
	}
	return fiber.StatusBadRequest
}
