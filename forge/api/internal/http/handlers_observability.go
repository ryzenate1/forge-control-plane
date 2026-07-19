package http

import (
	"strconv"

	heartbeatmonitor "gamepanel/forge/internal/services/heartbeatmonitor"
	observabilitysvc "gamepanel/forge/internal/services/observability"

	"github.com/gofiber/fiber/v2"
)

func registerObservabilityRoutes(protected fiber.Router, cfg Config, observability *observabilitysvc.Service, heartbeatMonitor *heartbeatmonitor.Service) {
	protected.Get("/timeline", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		events, err := observability.Timeline(ctx, queryLimit(c))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(events)
	})

	protected.Get("/timeline/:resourceType/:resourceId", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		events, err := observability.ResourceTimeline(ctx, c.Params("resourceType"), c.Params("resourceId"), queryLimit(c))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(events)
	})

	protected.Get("/correlations/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		events, err := observability.Correlation(ctx, c.Params("id"), queryLimit(c))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(events)
	})

	protected.Get("/nodes/:id/heartbeats", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		history, err := observability.NodeHeartbeatHistory(ctx, c.Params("id"), queryLimit(c))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(history)
	})

	protected.Get("/nodes/:id/heartbeat", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		evaluation, err := heartbeatMonitor.InspectNode(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		history, err := observability.NodeHeartbeatHistory(ctx, c.Params("id"), queryLimit(c))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{
			"evaluation": evaluation,
			"history":    history,
		})
	})

	protected.Get("/nodes/:id/health-history", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		history, err := observability.NodeHealthHistory(ctx, c.Params("id"), queryLimit(c))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(history)
	})
}

func queryLimit(c *fiber.Ctx) int {
	limit, err := strconv.Atoi(c.Query("limit", "100"))
	if err != nil {
		return 100
	}
	if limit <= 0 {
		return 100
	}
	if limit > 500 {
		return 500
	}
	return limit
}
