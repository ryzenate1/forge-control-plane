package http

import (
	"strconv"
	"time"

	"gamepanel/forge/internal/services/alerting"
	observabilitysvc "gamepanel/forge/internal/services/observability"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerAlertRoutes(protected fiber.Router, alertService *alerting.Service, observability *observabilitysvc.Service) {
	alerts := protected.Group("/alerts")
	alerts.Get("/", requireRole("admin"), handleListAlerts(alertService))
	alerts.Get("/:id", requireRole("admin"), handleGetAlert(alertService))
	alerts.Post("/:id/acknowledge", requireRole("admin"), handleAcknowledgeAlert(alertService))
	alerts.Post("/:id/resolve", requireRole("admin"), handleResolveAlert(alertService))

	protected.Get("/monitoring/summary", requireRole("admin"), handleMonitoringSummary(observability))
	protected.Get("/monitoring/nodes/metrics", requireRole("admin"), handleNodeMetrics(observability))
}

func handleListAlerts(svc *alerting.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		filter := parseAlertFilter(c)
		alerts, err := svc.ListAlerts(ctx, filter)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{
			"alerts": alerts,
		})
	}
}

func handleGetAlert(svc *alerting.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		alert, err := svc.GetAlert(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "alert not found")
		}
		return c.JSON(alert)
	}
}

func handleAcknowledgeAlert(svc *alerting.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		acknowledgedBy := c.Query("by", "admin")
		if err := svc.AcknowledgeAlert(ctx, c.Params("id"), acknowledgedBy); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"status": "acknowledged"})
	}
}

func handleResolveAlert(svc *alerting.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		if err := svc.ResolveAlert(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"status": "resolved"})
	}
}

func handleMonitoringSummary(svc *observabilitysvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		summary, err := svc.MonitoringSummary(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(summary)
	}
}

func handleNodeMetrics(svc *observabilitysvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		nodeID := c.Query("nodeId")
		limit, _ := strconv.Atoi(c.Query("limit", "100"))
		var since *time.Time
		if v := c.Query("since"); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				since = &t
			}
		}

		if nodeID != "" {
			metrics, err := svc.ListNodeMetrics(ctx, nodeID, limit, since)
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
			return c.JSON(fiber.Map{"metrics": metrics})
		}

		all, err := svc.ListAllNodeMetricsLatest(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"metrics": all})
	}
}

func parseAlertFilter(c *fiber.Ctx) store.AlertFilter {
	filter := store.AlertFilter{}
	if v := c.Query("nodeId"); v != "" {
		filter.NodeID = &v
	}
	if v := c.Query("serverId"); v != "" {
		filter.ServerID = &v
	}
	if v := c.Query("severity"); v != "" {
		s := store.AlertSeverity(v)
		filter.Severity = &s
	}
	if v := c.Query("acknowledged"); v != "" {
		b := v == "true" || v == "1"
		filter.Acknowledged = &b
	}
	if v := c.Query("alertType"); v != "" {
		filter.AlertType = &v
	}
	if v := c.Query("source"); v != "" {
		filter.Source = &v
	}
	if v := c.Query("tenantId"); v != "" {
		filter.TenantID = &v
	}
	if v := c.Query("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.From = &t
		}
	}
	if v := c.Query("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.To = &t
		}
	}
	if v := c.Query("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil {
			filter.Limit = l
		}
	}
	if v := c.Query("offset"); v != "" {
		if o, err := strconv.Atoi(v); err == nil {
			filter.Offset = o
		}
	}
	return filter
}
