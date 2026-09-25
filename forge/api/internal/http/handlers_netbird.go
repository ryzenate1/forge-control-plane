package http

import (
	"net/http"

	netbirdsvc "gamepanel/forge/internal/services/netbird"

	"github.com/gofiber/fiber/v2"
)

// registerNetBirdRoutes exposes the NetBird mesh-VPN control plane to admins.
// Every route is admin-only and additionally gated by the netbird.read /
// netbird.write API scopes. The service itself is nil-safe: when NETBIRD_API_URL
// / NETBIRD_API_TOKEN are unset, list endpoints return empty data and mutations
// fail with a "not configured" error.
func registerNetBirdRoutes(protected fiber.Router, cfg Config, adminIPAccess, mutationLimiter fiber.Handler) {
	svc := cfg.NetBirdService

	netbirdGroup := protected.Group("/admin/netbird", adminIPAccess)

	// --- Peers ---
	netbirdGroup.Get("/peers", requireRole("admin"), requireAdminScope("netbird.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		peers, err := svc.ListPeers(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": peers})
	})

	netbirdGroup.Get("/peers/:id", requireRole("admin"), requireAdminScope("netbird.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		peer, err := svc.GetPeer(ctx, c.Params("id"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": peer})
	})

	netbirdGroup.Post("/peers/:id/approve", mutationLimiter, requireRole("admin"), requireAdminScope("netbird.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		peer, err := svc.ApprovePeer(ctx, c.Params("id"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": peer})
	})

	netbirdGroup.Post("/peers/:id/deny", mutationLimiter, requireRole("admin"), requireAdminScope("netbird.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		peer, err := svc.DenyPeer(ctx, c.Params("id"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": peer})
	})

	netbirdGroup.Delete("/peers/:id", mutationLimiter, requireRole("admin"), requireAdminScope("netbird.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.DeletePeer(ctx, c.Params("id")); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"message": "peer deleted"})
	})

	// --- Networks ---
	netbirdGroup.Get("/networks", requireRole("admin"), requireAdminScope("netbird.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		networks, err := svc.ListNetworks(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": networks})
	})

	netbirdGroup.Post("/networks", mutationLimiter, requireRole("admin"), requireAdminScope("netbird.write"), func(c *fiber.Ctx) error {
		var req netbirdsvc.Network
		if err := c.BodyParser(&req); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}
		ctx, cancel := requestContext()
		defer cancel()
		network, err := svc.CreateNetwork(ctx, req)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": network})
	})

	netbirdGroup.Patch("/networks/:id", mutationLimiter, requireRole("admin"), requireAdminScope("netbird.write"), func(c *fiber.Ctx) error {
		var req netbirdsvc.Network
		if err := c.BodyParser(&req); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}
		ctx, cancel := requestContext()
		defer cancel()
		network, err := svc.UpdateNetwork(ctx, c.Params("id"), req)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": network})
	})

	netbirdGroup.Delete("/networks/:id", mutationLimiter, requireRole("admin"), requireAdminScope("netbird.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.DeleteNetwork(ctx, c.Params("id")); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"message": "network deleted"})
	})

	// --- Groups ---
	netbirdGroup.Get("/groups", requireRole("admin"), requireAdminScope("netbird.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		groups, err := svc.ListGroups(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": groups})
	})

	netbirdGroup.Post("/groups", mutationLimiter, requireRole("admin"), requireAdminScope("netbird.write"), func(c *fiber.Ctx) error {
		var req struct {
			Name  string   `json:"name"`
			Peers []string `json:"peers"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}
		ctx, cancel := requestContext()
		defer cancel()
		group, err := svc.CreateGroup(ctx, req.Name, req.Peers)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": group})
	})

	netbirdGroup.Delete("/groups/:id", mutationLimiter, requireRole("admin"), requireAdminScope("netbird.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.DeleteGroup(ctx, c.Params("id")); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"message": "group deleted"})
	})

	// --- Routes ---
	netbirdGroup.Get("/routes", requireRole("admin"), requireAdminScope("netbird.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		routes, err := svc.ListRoutes(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": routes})
	})

	netbirdGroup.Post("/routes", mutationLimiter, requireRole("admin"), requireAdminScope("netbird.write"), func(c *fiber.Ctx) error {
		var req netbirdsvc.Route
		if err := c.BodyParser(&req); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}
		ctx, cancel := requestContext()
		defer cancel()
		route, err := svc.CreateRoute(ctx, req)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": route})
	})

	netbirdGroup.Delete("/routes/:id", mutationLimiter, requireRole("admin"), requireAdminScope("netbird.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.DeleteRoute(ctx, c.Params("id")); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"message": "route deleted"})
	})

	// --- ACLs (policies) ---
	netbirdGroup.Get("/acls", requireRole("admin"), requireAdminScope("netbird.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		acls, err := svc.ListACLs(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": acls})
	})

	netbirdGroup.Post("/acls", mutationLimiter, requireRole("admin"), requireAdminScope("netbird.write"), func(c *fiber.Ctx) error {
		var req netbirdsvc.ACLRule
		if err := c.BodyParser(&req); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}
		ctx, cancel := requestContext()
		defer cancel()
		acl, err := svc.CreateACL(ctx, req)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": acl})
	})

	netbirdGroup.Delete("/acls/:id", mutationLimiter, requireRole("admin"), requireAdminScope("netbird.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.DeleteACL(ctx, c.Params("id")); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"message": "acl deleted"})
	})

	// --- DNS ---
	netbirdGroup.Get("/dns", requireRole("admin"), requireAdminScope("netbird.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		settings, err := svc.GetDNSSettings(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": settings})
	})

	netbirdGroup.Patch("/dns", mutationLimiter, requireRole("admin"), requireAdminScope("netbird.write"), func(c *fiber.Ctx) error {
		var req netbirdsvc.DNSConfig
		if err := c.BodyParser(&req); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}
		ctx, cancel := requestContext()
		defer cancel()
		settings, err := svc.UpdateDNSSettings(ctx, req)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": settings})
	})

	// --- Setup keys ---
	netbirdGroup.Post("/setup-keys", mutationLimiter, requireRole("admin"), requireAdminScope("netbird.write"), func(c *fiber.Ctx) error {
		var req netbirdsvc.SetupKeyRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}
		ctx, cancel := requestContext()
		defer cancel()
		key, err := svc.CreateSetupKey(ctx, req)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": key})
	})

	netbirdGroup.Post("/setup-keys/:id/revoke", mutationLimiter, requireRole("admin"), requireAdminScope("netbird.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.RevokeSetupKey(ctx, c.Params("id")); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"message": "setup key revoked"})
	})
}
