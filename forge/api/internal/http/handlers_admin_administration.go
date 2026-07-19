package http

import (
	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/services/evacuationplanner"
	migrationservice "gamepanel/forge/internal/services/migration"
	"gamepanel/forge/internal/services/noderegistry"
	recoverysvc "gamepanel/forge/internal/services/recovery"
	reservationsvc "gamepanel/forge/internal/services/reservations"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerAdministrationAdminRoutes(protected fiber.Router, cfg Config, nodeRegistry *noderegistry.Service, clusterManager *clustermanager.Service, evacuationPlanner *evacuationplanner.Service, migrationService *migrationservice.Service, reservationManager *reservationsvc.Manager, recoveryCoordinator *recoverysvc.Coordinator, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {

	protected.Get("/permissions", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"permissions": store.PermissionDescriptions(),
		})
	})

	// ---- Deployable node ----

	// Filter by location, capacity, and health.

	// Return the least-loaded node (lowest memory used/memory limit ratio).

	// ---- Regions CRUD (multi-node foundation) ----

	// ---- Locations CRUD ----

	// ---- Nests CRUD ----

	// ---- Eggs CRUD ----

	// ---- Egg Export/Import ----

	// Test a prospective host before it is saved. This uses the same normalization,
	// validation, TLS configuration, and ping flow as provisioning without persistence.

	// This is an administrator-only diagnostic for an external provisioning host.
	// Preserve the connector's sanitized cause (timeout, TLS, authentication, etc.)
	// instead of collapsing every failure into an indistinguishable message.

	// TLS CA values are write-only. Treat a blank value as omitted so clients can
	// safely round-trip an edit without overwriting a redacted certificate.

	// Server <-> Mount attachment. The mount must be eligible for the server's
	// node and egg, just as it is for the server-scoped assignment route.

	// Register the static path before /allocations/:id so it is not interpreted as id="bulk".

	// ---- Plugins ----
	protected.Get("/admin/plugins", requireRole("admin"), ListPlugins(cfg))
	protected.Get("/admin/plugins/:id", requireRole("admin"), GetPlugin(cfg))
	protected.Post("/admin/plugins/import/file", requireRole("admin"), ImportPluginFromFile(cfg))
	protected.Post("/admin/plugins/import/url", requireRole("admin"), ImportPluginFromURL(cfg))
	protected.Post("/admin/plugins/:id/install", requireRole("admin"), InstallPlugin(cfg, cfg.PluginsDir))
	protected.Post("/admin/plugins/:id/uninstall", requireRole("admin"), UninstallPlugin(cfg, cfg.PluginsDir))
	protected.Post("/admin/plugins/:id/enable", requireRole("admin"), EnablePlugin(cfg))
	protected.Post("/admin/plugins/:id/disable", requireRole("admin"), DisablePlugin(cfg))
	protected.Patch("/admin/plugins/:id", requireRole("admin"), UpdatePlugin(cfg))
	protected.Delete("/admin/plugins/:id", requireRole("admin"), DeletePlugin(cfg))

	// ---- Roles ----
	protected.Get("/admin/roles", requireRole("admin"), ListRoles(cfg))
	protected.Get("/admin/roles/:id", requireRole("admin"), GetRole(cfg))
	protected.Post("/admin/roles", requireRole("admin"), CreateRole(cfg))
	protected.Patch("/admin/roles/:id", requireRole("admin"), UpdateRole(cfg))
	protected.Delete("/admin/roles/:id", requireRole("admin"), DeleteRole(cfg))
	protected.Get("/admin/users/:id/roles", requireRole("admin"), ListUserRoles(cfg))
	protected.Patch("/admin/users/:id/roles/assign", requireRole("admin"), AssignRolesToUser(cfg))
	protected.Patch("/admin/users/:id/roles/remove", requireRole("admin"), RemoveRolesFromUser(cfg))

	// ---- OAuth2 admin routes ----
	protected.Get("/admin/oauth-clients", requireRole("admin"), AdminListOAuthClients(cfg))
	protected.Post("/admin/oauth-clients", requireRole("admin"), AdminCreateOAuthClient(cfg))
	protected.Delete("/admin/oauth-clients/:id", requireRole("admin"), AdminDeleteOAuthClient(cfg))

	// ---- Webhooks ----

	protected.Get("/webhooks", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		webhooks, err := cfg.Store.ListWebhooks(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		for i := range webhooks {
			if webhooks[i].Secret != "" {
				webhooks[i].Secret = maskedSecret
			}
		}
		return c.JSON(webhooks)
	})

	protected.Post("/webhooks", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req store.CreateWebhookRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		wh, err := cfg.Store.CreateWebhook(ctx, req)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if wh.Secret != "" {
			wh.Secret = maskedSecret
		}
		return c.Status(fiber.StatusCreated).JSON(wh)
	})

	protected.Patch("/webhooks/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req store.CreateWebhookRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if req.Secret == maskedSecret {
			req.Secret = ""
		}
		wh, err := cfg.Store.UpdateWebhook(ctx, c.Params("id"), req)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if wh.Secret != "" {
			wh.Secret = maskedSecret
		}
		return c.JSON(wh)
	})

	protected.Get("/webhooks/:id/deliveries", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		deliveries, err := cfg.Store.ListWebhookDeliveries(ctx, c.Params("id"), queryLimit(c))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(deliveries)
	})

	protected.Get("/admin/webhooks/:id/deliveries", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		deliveries, err := cfg.Store.ListWebhookDeliveries(ctx, c.Params("id"), queryLimit(c))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(deliveries)
	})

	protected.Delete("/webhooks/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteWebhook(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})
}
