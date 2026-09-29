package http

import (
	"errors"
	"strings"

	"gamepanel/forge/internal/services/loadbalancer"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func loadBalancerError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, loadbalancer.ErrGroupNotFound), errors.Is(err, loadbalancer.ErrTargetNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, loadbalancer.ErrNoHealthyTarget):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	default:
		return respondInternalError(c, err)
	}
}

func registerLoadBalancerRoutes(protected fiber.Router, cfg Config, svc *loadbalancer.Service, adminIPAccess, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	lb := protected.Group("/admin/load-balancer", adminIPAccess)

	lb.Get("/groups", requireRole("admin"), requireAdminScope("loadbalancer.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		groups, err := svc.ListGroups(ctx)
		if err != nil {
			return loadBalancerError(c, err)
		}
		return c.JSON(fiber.Map{"data": groups})
	})

	lb.Post("/groups", mutationLimiter, requireRole("admin"), requireAdminScope("loadbalancer.write"), func(c *fiber.Ctx) error {
		var group loadbalancer.TargetGroup
		if err := c.BodyParser(&group); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid target group payload"})
		}
		group.ID = uuid.NewString()
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.CreateTargetGroup(ctx, &group); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": group})
	})

	lb.Get("/groups/:id", requireRole("admin"), requireAdminScope("loadbalancer.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		group, err := svc.GetTargetGroup(ctx, c.Params("id"))
		if err != nil {
			return loadBalancerError(c, err)
		}
		return c.JSON(fiber.Map{"data": group})
	})

	lb.Put("/groups/:id", mutationLimiter, requireRole("admin"), requireAdminScope("loadbalancer.write"), func(c *fiber.Ctx) error {
		var group loadbalancer.TargetGroup
		if err := c.BodyParser(&group); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid target group payload"})
		}
		ctx, cancel := requestContext()
		defer cancel()
		existing, err := svc.GetTargetGroup(ctx, c.Params("id"))
		if err != nil {
			return loadBalancerError(c, err)
		}
		group.ID = existing.ID
		group.Targets = existing.Targets
		group.CreatedAt = existing.CreatedAt
		if err := svc.UpdateTargetGroup(ctx, &group); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": group})
	})

	lb.Delete("/groups/:id", mutationLimiter, requireRole("admin"), requireAdminScope("loadbalancer.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.DeleteTargetGroup(ctx, c.Params("id")); err != nil {
			return loadBalancerError(c, err)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	lb.Post("/groups/:id/targets", mutationLimiter, requireRole("admin"), requireAdminScope("loadbalancer.write"), func(c *fiber.Ctx) error {
		var req struct {
			ServerID string `json:"serverId"`
			NodeID   string `json:"nodeId"`
			IP       string `json:"ip"`
			Port     int    `json:"port"`
			Weight   int    `json:"weight"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid target payload"})
		}
		req.ServerID = strings.TrimSpace(req.ServerID)
		req.IP = strings.TrimSpace(req.IP)
		if req.ServerID == "" || req.IP == "" || req.Port < 1 || req.Port > 65535 || req.Weight < 1 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "serverId, ip, a valid port, and a positive weight are required"})
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := svc.AddTarget(ctx, c.Params("id"), req.ServerID, strings.TrimSpace(req.NodeID), req.IP, req.Port, req.Weight)
		if err != nil {
			return loadBalancerError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": target})
	})

	lb.Delete("/groups/:groupId/targets/:targetId", mutationLimiter, requireRole("admin"), requireAdminScope("loadbalancer.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.RemoveTarget(ctx, c.Params("groupId"), c.Params("targetId")); err != nil {
			return loadBalancerError(c, err)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	lb.Patch("/groups/:groupId/targets/:targetId", mutationLimiter, requireRole("admin"), requireAdminScope("loadbalancer.write"), func(c *fiber.Ctx) error {
		var req struct {
			Status loadbalancer.TargetStatus `json:"status"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid target payload"})
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.SetTargetStatus(ctx, c.Params("groupId"), c.Params("targetId"), req.Status); err != nil {
			return loadBalancerError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	lb.Get("/groups/:id/next", requireRole("admin"), requireAdminScope("loadbalancer.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		target, err := svc.NextTarget(ctx, c.Params("id"), ExtractClientIP(c))
		if err != nil {
			return loadBalancerError(c, err)
		}
		return c.JSON(fiber.Map{"data": target})
	})

	lb.Get("/metrics", requireRole("admin"), requireAdminScope("loadbalancer.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		return c.JSON(fiber.Map{"data": svc.Metrics(ctx)})
	})
}
