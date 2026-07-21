package http

import (
	"net/http"

	"gamepanel/forge/internal/services/servicediscovery"
	"github.com/gofiber/fiber/v2"
)

func registerServiceDiscoveryRoutes(protected fiber.Router, cfg Config, svc *servicediscovery.Service, adminIPAccess, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	discovery := protected.Group("/admin/service-discovery", adminIPAccess)

	// List all services
	discovery.Get("/services", requireRole("admin"), requireAdminScope("services.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		services := svc.ListServices(ctx)
		return c.JSON(fiber.Map{"data": services})
	})

	// List endpoints with optional filters
	discovery.Get("/endpoints", requireRole("admin"), requireAdminScope("services.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		filter := servicediscovery.EndpointFilter{
			ServiceName: c.Query("service"),
			NodeID:      c.Query("node_id"),
			TenantID:    c.Query("tenant_id"),
			HealthyOnly: c.Query("healthy_only") == "true",
		}

		endpoints := svc.ListEndpoints(ctx, filter)
		return c.JSON(fiber.Map{"data": endpoints})
	})

	// Get specific endpoint
	discovery.Get("/endpoints/:id", requireRole("admin"), requireAdminScope("services.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		endpoint, err := svc.GetEndpoint(ctx, c.Params("id"))
		if err != nil {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": endpoint})
	})

	// Register endpoint
	discovery.Post("/endpoints", mutationLimiter, requireRole("admin"), requireAdminScope("services.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		var req servicediscovery.ServiceEndpoint
		if err := c.BodyParser(&req); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}

		endpoint, err := svc.RegisterEndpoint(ctx, req)
		if err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}

		return c.Status(http.StatusCreated).JSON(fiber.Map{"data": endpoint})
	})

	// Remove endpoint
	discovery.Delete("/endpoints/:id", mutationLimiter, requireRole("admin"), requireAdminScope("services.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		if err := svc.RemoveEndpoint(ctx, c.Params("id")); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.SendStatus(http.StatusNoContent)
	})

	// Update endpoint status
	discovery.Patch("/endpoints/:id/status", mutationLimiter, requireRole("admin"), requireAdminScope("services.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		var req struct {
			Status string `json:"status"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}

		status := servicediscovery.EndpointStatus(req.Status)
		if status != servicediscovery.EndpointStatusHealthy &&
			status != servicediscovery.EndpointStatusUnhealthy &&
			status != servicediscovery.EndpointStatusUnknown {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid status"})
		}

		if err := svc.UpdateEndpointStatus(ctx, c.Params("id"), status); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.SendStatus(http.StatusNoContent)
	})

	// Resolve service endpoints
	discovery.Get("/resolve", requireRole("admin"), requireAdminScope("services.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		serviceName := c.Query("service")
		tenantID := c.Query("tenant_id")

		if serviceName == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "service parameter is required"})
		}

		endpoints, err := svc.Resolve(ctx, serviceName, tenantID)
		if err != nil {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{"data": endpoints})
	})

	// Network visibility
	discovery.Get("/network/visibility", requireRole("admin"), requireAdminScope("services.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		visibility := svc.NetworkVisibility(ctx)
		return c.JSON(fiber.Map{"data": visibility})
	})

	// Node network view
	discovery.Get("/network/nodes/:nodeId", requireRole("admin"), requireAdminScope("services.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		view := svc.NodeNetworkView(ctx, c.Params("nodeId"))
		return c.JSON(fiber.Map{"data": view})
	})

	// Verify cross-node reachability
	discovery.Post("/reachability/verify", mutationLimiter, requireRole("admin"), requireAdminScope("services.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		var req struct {
			SourceNodeID string `json:"sourceNodeId"`
			TargetNodeID string `json:"targetNodeId"`
			ServiceName  string `json:"serviceName"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}

		if req.SourceNodeID == "" || req.TargetNodeID == "" || req.ServiceName == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "sourceNodeId, targetNodeId, and serviceName are required"})
		}

		result, err := svc.VerifyCrossNodeReachability(ctx, req.SourceNodeID, req.TargetNodeID, req.ServiceName)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{"data": result})
	})

	// Sweep reachability check
	discovery.Post("/reachability/sweep", mutationLimiter, requireRole("admin"), requireAdminScope("services.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		results := svc.SweepReachability(ctx)
		return c.JSON(fiber.Map{"data": results})
	})

	// Reaper stats
	discovery.Get("/reaper/stats", requireRole("admin"), requireAdminScope("services.read"), func(c *fiber.Ctx) error {
		lastRun, count, interval := svc.ReaperStats()
		return c.JSON(fiber.Map{
			"data": fiber.Map{
				"lastRun":  lastRun,
				"count":    count,
				"interval": interval,
			},
		})
	})
}
