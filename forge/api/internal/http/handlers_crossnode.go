package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

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

			// A failed resolution is reported as a failure, never as a fallback
			// host: answering with a guess would point the caller at a node the
			// resolver never confirmed.
			host, err := resolver.ResolveTargetHost(ctx, serverID, nodeID)
			if err != nil {
				if errors.Is(err, crossnode.ErrNoTarget) {
					return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "no reachable target for the requested server or node"})
				}
				return c.Status(http.StatusBadGateway).JSON(fiber.Map{"error": "target resolution failed"})
			}
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
			ttl, err := time.ParseDuration(strings.TrimSpace(req.TTL))
			if err != nil || ttl <= 0 {
				return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid ttl duration (e.g. 30s, 5m, 1h)"})
			}
			resolver.SetCacheTTL(ttl)
			return c.JSON(fiber.Map{"message": "cache ttl set", "ttl": ttl.String()})
		})

		// Describe a backend using what the prober actually recorded. An unbounded
		// port is rejected here: the health key is host:port, so a nonsense port
		// would silently alias onto another backend's record.
		crossnodeGroup.Get("/describe/:host/:port", requireRole("admin"), requireAdminScope("routing.read"), func(c *fiber.Ctx) error {
			host := strings.TrimSpace(c.Params("host"))
			if host == "" {
				return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "host is required"})
			}

			port, err := strconv.Atoi(c.Params("port"))
			if err != nil || port < 1 || port > 65535 {
				return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "port must be an integer between 1 and 65535"})
			}

			description := resolver.DescribeUnreachable(host, port)
			if ingressSync != nil {
				description = ingressSync.DescribeBackend(host, port)
			}
			return c.JSON(fiber.Map{"data": fiber.Map{"description": description}})
		})
	}

	// Ingress synchronizer endpoints
	if ingressSync != nil {
		// Get current rules
		crossnodeGroup.Get("/ingress/rules", requireRole("admin"), requireAdminScope("routing.read"), func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{"data": ingressSync.CurrentRules()})
		})

		// Get current policies
		crossnodeGroup.Get("/ingress/policies", requireRole("admin"), requireAdminScope("routing.read"), func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{"data": ingressSync.CurrentPolicies()})
		})

		// Observed cross-node backend verdicts. These were previously unexposed, so
		// an operator had no way to see which node backends the prober believed up.
		crossnodeGroup.Get("/ingress/backends", requireRole("admin"), requireAdminScope("routing.read"), func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{"data": ingressSync.Backends()})
		})

		crossnodeGroup.Get("/ingress/route-groups", requireRole("admin"), requireAdminScope("routing.read"), func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{"data": ingressSync.RouteGenerationRecords()})
		})

		// Reconcile is the only route here that can change the gateway, and it
		// converges through trafficmanager, which owns the live rule set.
		crossnodeGroup.Post("/ingress/sync", mutationLimiter, requireRole("admin"), requireAdminScope("routing.write"), func(c *fiber.Ctx) error {
			ctx, cancel := requestContext()
			defer cancel()

			result, err := ingressSync.Reconcile(ctx)
			if errors.Is(err, crossnode.ErrNoReconciler) {
				return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
					"synced":  false,
					"skipped": true,
					"error":   err.Error(),
					"detail":  "cross-node ingress is observation-only until a gateway reconciler is wired",
				})
			}
			if err != nil {
				return respondInternalError(c, err)
			}
			return c.JSON(fiber.Map{"data": result, "synced": true})
		})

		// Get health filter stats
		crossnodeGroup.Get("/ingress/health/stats", requireRole("admin"), requireAdminScope("routing.read"), func(c *fiber.Ctx) error {
			ctx, cancel := requestContext()
			defer cancel()
			return c.JSON(fiber.Map{"data": ingressSync.Health(ctx)})
		})

		crossnodeGroup.Post("/ingress/cleanup", mutationLimiter, requireRole("admin"), requireAdminScope("routing.write"), func(c *fiber.Ctx) error {
			ctx, cancel := requestContext()
			defer cancel()

			if err := ingressSync.CleanupStale(ctx); err != nil {
				if errors.Is(err, crossnode.ErrNoReconciler) {
					return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
						"cleaned": false,
						"error":   err.Error(),
						"detail":  "refusing to clean routes without the reconciler's live rule set: an empty active set would withdraw every live route",
					})
				}
				return respondInternalError(c, err)
			}
			return c.JSON(fiber.Map{"cleaned": true, "message": "stale routes cleaned up"})
		})

		// Get sync statistics
		crossnodeGroup.Get("/ingress/stats", requireRole("admin"), requireAdminScope("routing.read"), func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{"data": ingressSync.Stats()})
		})
	}

	crossnodeGroup.Get("/health", requireRole("admin"), requireAdminScope("routing.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		status := fiber.Map{
			"resolver_available":     resolver != nil,
			"ingress_sync_available": ingressSync != nil,
		}
		var reasons []string
		if resolver == nil {
			reasons = append(reasons, "no cross-node resolver configured")
		}

		if ingressSync != nil {
			stats := ingressSync.Stats()
			status["ingress"] = stats
			status["gateway"] = ingressSync.Health(ctx)
			if !stats.ReconcilerConfigured {
				reasons = append(reasons, "no gateway reconciler wired: ingress is observation-only")
			}
			if stats.RuleCount == 0 {
				reasons = append(reasons, "no enabled routing rules observed")
			}
			if stats.BackendCount == 0 {
				reasons = append(reasons, "no backend has a probe result yet")
			}
		} else {
			reasons = append(reasons, "no ingress synchronizer configured")
		}

		// "active" used to be derived from a nil check, which reported a component
		// that provably did no work as healthy.
		status["status"] = "ok"
		if len(reasons) > 0 {
			status["status"] = "degraded"
			status["reasons"] = reasons
		}
		return c.JSON(fiber.Map{"data": status})
	})
}
