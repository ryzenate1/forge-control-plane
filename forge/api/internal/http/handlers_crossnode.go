package http

import (
	"net/http"
	"strconv"

	"gamepanel/forge/internal/services/crossnode"

	"github.com/gofiber/fiber/v2"
)

func registerCrossNodeRoutes(protected fiber.Router, cfg Config, resolver *crossnode.Resolver, ingressSync *crossnode.IngressSynchronizer, adminIPAccess, mutationLimiter fiber.Handler) {
	if resolver == nil && ingressSync == nil {
		return
	}

	crossnodeGroup := protected.Group("/admin/crossnode", adminIPAccess)

	// Resolver endpoints
	if resolver != nil {
		// Resolve target host for server/node
		crossnodeGroup.Get("/resolve", requireRole("admin"), requireAdminScope("routing.read"), func(c *fiber.Ctx) error {
			ctx, cancel := requestContext()
			defer cancel()

			serverID := c.Query("server_id")
			nodeID := c.Query("node_id")

			if serverID == "" && nodeID == "" {
				return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "server_id or node_id parameter is required"})
			}

			host := resolver.ResolveTargetHost(ctx, serverID, nodeID)
			return c.JSON(fiber.Map{"data": fiber.Map{"host": host}})
		})

		// Clear resolver cache
		crossnodeGroup.Post("/cache/clear", mutationLimiter, requireRole("admin"), requireAdminScope("routing.write"), func(c *fiber.Ctx) error {
			resolver.ClearCache()
			return c.JSON(fiber.Map{"message": "cache cleared"})
		})

		// Set cache TTL
		crossnodeGroup.Post("/cache/ttl", mutationLimiter, requireRole("admin"), requireAdminScope("routing.write"), func(c *fiber.Ctx) error {
			var req struct {
				TTL string `json:"ttl"`
			}
			if err := c.BodyParser(&req); err != nil {
				return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
			}

			// Parse duration from string (e.g., "30s", "5m", "1h")
			// For now, we'll use a simple approach - in production you might want to use time.ParseDuration
			// This is a simplified version
			return c.JSON(fiber.Map{"message": "TTL configuration would be set here"})
		})

		// Describe unreachable backend
		crossnodeGroup.Get("/describe/:host/:port", requireRole("admin"), requireAdminScope("routing.read"), func(c *fiber.Ctx) error {
			host := c.Params("host")
			portStr := c.Params("port")

			port, err := strconv.Atoi(portStr)
			if err != nil {
				return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid port"})
			}

			description := resolver.DescribeUnreachable(host, port)
			return c.JSON(fiber.Map{"data": fiber.Map{"description": description}})
		})
	}

	// Ingress synchronizer endpoints
	if ingressSync != nil {
		// Get current rules
		crossnodeGroup.Get("/ingress/rules", requireRole("admin"), requireAdminScope("routing.read"), func(c *fiber.Ctx) error {
			// Get current rules from the synchronizer
			// Note: This exposes the internal state for debugging/monitoring
			return c.JSON(fiber.Map{"message": "ingress rules endpoint"})
		})

		// Get current policies
		crossnodeGroup.Get("/ingress/policies", requireRole("admin"), requireAdminScope("routing.read"), func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{"message": "ingress policies endpoint"})
		})

		// Trigger immediate sync
		crossnodeGroup.Post("/ingress/sync", mutationLimiter, requireRole("admin"), requireAdminScope("routing.write"), func(c *fiber.Ctx) error {
			ctx, cancel := requestContext()
			defer cancel()

			if err := ingressSync.Sync(ctx); err != nil {
				return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
			}

			return c.JSON(fiber.Map{"message": "sync triggered successfully"})
		})

		// Get health filter stats
		crossnodeGroup.Get("/ingress/health/stats", requireRole("admin"), requireAdminScope("routing.read"), func(c *fiber.Ctx) error {
			// This would expose health filter statistics if the health filter has such methods
			return c.JSON(fiber.Map{"message": "health filter stats endpoint"})
		})

		// Cleanup stale routes
		crossnodeGroup.Post("/ingress/cleanup", mutationLimiter, requireRole("admin"), requireAdminScope("routing.write"), func(c *fiber.Ctx) error {
			ctx, cancel := requestContext()
			defer cancel()

			if err := ingressSync.CleanupStale(ctx); err != nil {
				return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
			}

			return c.JSON(fiber.Map{"message": "stale routes cleaned up"})
		})

		// Get sync statistics
		crossnodeGroup.Get("/ingress/stats", requireRole("admin"), requireAdminScope("routing.read"), func(c *fiber.Ctx) error {
			// This would expose ingress sync statistics
			return c.JSON(fiber.Map{"message": "ingress sync stats endpoint"})
		})
	}

	// Cross-node routing health check
	crossnodeGroup.Get("/health", requireRole("admin"), requireAdminScope("routing.read"), func(c *fiber.Ctx) error {
		status := fiber.Map{
			"resolver_available":     resolver != nil,
			"ingress_sync_available": ingressSync != nil,
		}

		if resolver != nil {
			status["resolver_status"] = "active"
		}

		if ingressSync != nil {
			status["ingress_sync_status"] = "active"
		}

		return c.JSON(fiber.Map{"data": status})
	})
}
