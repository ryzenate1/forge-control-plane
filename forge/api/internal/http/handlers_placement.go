package http

import (
	"strings"

	envaffinitysvc "gamepanel/forge/internal/services/envaffinity"
	"gamepanel/forge/internal/domain"

	"github.com/gofiber/fiber/v2"
)

// registerPlacementRoutes exposes the env-affinity service's operator-facing
// surface: /placement/explain (why a node won or lost), /placement/enrich
// (preview the constraints a request would carry), and
// /placement/patch-constraints (re-sync affinity rules from servers.env_affinity
// and nodes.env_groups). The frontend (forge/web/lib/api/envaffinity.ts) and
// /admin/env-affinity page call these to power the placement debugger view.
func registerPlacementRoutes(protected fiber.Router, cfg Config, svc *envaffinitysvc.EnvAffinity, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}
	placement := protected.Group("/placement", requireRole("admin"))

	placement.Post("/explain", func(c *fiber.Ctx) error {
		var req struct {
			NodeID string                 `json:"nodeId"`
			Place  domain.PlacementRequest `json:"place"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		req.NodeID = strings.TrimSpace(req.NodeID)
		if req.NodeID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "nodeId is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		result, err := svc.ExplainPlacement(ctx, req.NodeID, req.Place)
		if err != nil {
			return mapPlacementErr(err)
		}
		return c.JSON(fiber.Map{"data": result})
	})

	placement.Post("/enrich", func(c *fiber.Ctx) error {
		var req domain.PlacementRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		enriched, err := svc.EnrichPlacementRequest(ctx, req)
		if err != nil {
			return mapPlacementErr(err)
		}
		return c.JSON(fiber.Map{"data": enriched})
	})

	placement.Post("/patch-constraints", mutationLimiter, func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		result, err := svc.PatchPlacementConstraints(ctx)
		if err != nil {
			return mapPlacementErr(err)
		}
		return c.JSON(fiber.Map{"data": result})
	})
}

func mapPlacementErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "not found"):
		return fiber.NewError(fiber.StatusNotFound, msg)
	case strings.Contains(low, "requires"), strings.Contains(low, "invalid"), strings.Contains(low, "required"):
		return fiber.NewError(fiber.StatusBadRequest, msg)
	}
	return fiber.NewError(fiber.StatusInternalServerError, msg)
}
