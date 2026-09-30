package http

import (
	"strconv"

	"gamepanel/forge/internal/services/nodeautoscale"

	"github.com/gofiber/fiber/v2"
)

// registerNodeAutoscaleRoutes exposes the node autoscaler (cluster scale-out /
// scale-in). The dashboard (forge/web/lib/api/nodeautoscale.ts) targets
// /autoscale/*; without these routes the entire feature returns 404 even though
// the service, worker and store were fully implemented. All routes are admin
// only and IP-gated like the vertical autoscaler. cfg is accepted for signature
// symmetry with sibling register helpers.
func registerNodeAutoscaleRoutes(protected fiber.Router, cfg Config, svc *nodeautoscale.Service, adminIPAccess, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}
	node := protected.Group("/autoscale", adminIPAccess, requireRole("admin"))

	node.Get("/policies", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		policies, err := svc.ListPolicies(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(policies)
	})

	node.Post("/policies", mutationLimiter, func(c *fiber.Ctx) error {
		var p nodeautoscale.Policy
		if err := c.BodyParser(&p); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		created, err := svc.CreatePolicy(ctx, &p)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(created)
	})

	node.Put("/policies/:id", mutationLimiter, func(c *fiber.Ctx) error {
		var p nodeautoscale.Policy
		if err := c.BodyParser(&p); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		updated, err := svc.UpdatePolicy(ctx, c.Params("id"), &p)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(updated)
	})

	node.Delete("/policies/:id", mutationLimiter, func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.DeletePolicy(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	node.Post("/evaluate", mutationLimiter, func(c *fiber.Ctx) error {
		var req struct {
			PolicyID string `json:"policyId"`
			DryRun   bool   `json:"dryRun"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		evaluation, err := svc.Evaluate(ctx, req.PolicyID, req.DryRun)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(evaluation)
	})

	node.Post("/scale-out", mutationLimiter, func(c *fiber.Ctx) error {
		var req struct {
			PolicyID string `json:"policyId"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		event, err := svc.ScaleOut(ctx, req.PolicyID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(event)
	})

	node.Post("/scale-in", mutationLimiter, func(c *fiber.Ctx) error {
		var req struct {
			NodeID      string `json:"nodeId"`
			Deprovision bool   `json:"deprovision"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		event, err := svc.ScaleIn(ctx, req.NodeID, req.Deprovision)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(event)
	})

	node.Get("/events", func(c *fiber.Ctx) error {
		limit := 50
		if raw := c.Query("limit"); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		ctx, cancel := requestContext()
		defer cancel()
		events, err := svc.ListEvents(ctx, c.Query("policyId"), limit)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(events)
	})
}
