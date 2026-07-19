package http

import (
	"strings"

	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/services/evacuationplanner"
	migrationservice "gamepanel/forge/internal/services/migration"
	"gamepanel/forge/internal/services/noderegistry"
	recoverysvc "gamepanel/forge/internal/services/recovery"
	reservationsvc "gamepanel/forge/internal/services/reservations"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerMountsAdminRoutes(protected fiber.Router, cfg Config, nodeRegistry *noderegistry.Service, clusterManager *clustermanager.Service, evacuationPlanner *evacuationplanner.Service, migrationService *migrationservice.Service, reservationManager *reservationsvc.Manager, recoveryCoordinator *recoverysvc.Coordinator, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {

	protected.Get("/mounts", requireRole("admin"), requireAdminScope("mounts.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		mounts, err := cfg.Store.ListMounts(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(mounts)
	})

	protected.Get("/mounts/:id", requireRole("admin"), requireAdminScope("mounts.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		mount, err := cfg.Store.GetMount(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "mount not found")
		}
		return c.JSON(mount)
	})

	protected.Post("/mounts", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("mounts.write"), func(c *fiber.Ctx) error {
		var req CreateMountRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := Validate(&req); err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(err)
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		mount, err := cfg.Store.CreateMount(ctx, store.CreateMountRequest{
			Name:          req.Name,
			Description:   req.Description,
			Source:        req.Source,
			Target:        req.Target,
			ReadOnly:      req.ReadOnly,
			UserMountable: req.UserMountable,
			NodeIDs:       req.NodeIDs,
			TemplateIDs:   req.TemplateIDs,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(mount)
	})

	protected.Delete("/mounts/:id", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("mounts.delete"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.DeleteMount(ctx, c.Params("id"), actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Patch("/mounts/:id", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("mounts.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var req struct {
			Name          *string `json:"name"`
			Description   *string `json:"description"`
			Source        *string `json:"source"`
			Target        *string `json:"target"`
			ReadOnly      *bool   `json:"readOnly"`
			UserMountable *bool   `json:"userMountable"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		mount, err := cfg.Store.UpdateMount(ctx, c.Params("id"), store.UpdateMountRequest{
			Name:          req.Name,
			Description:   req.Description,
			Source:        req.Source,
			Target:        req.Target,
			ReadOnly:      req.ReadOnly,
			UserMountable: req.UserMountable,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(mount)
	})

	protected.Get("/mounts/:id/nodes", requireRole("admin"), requireAdminScope("mounts.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		nodes, err := cfg.Store.ListNodesForMount(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(nodes)
	})

	protected.Post("/mounts/:id/nodes", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("mounts.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var req struct {
			NodeIDs []string `json:"nodes"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		for _, nodeID := range req.NodeIDs {
			if err := cfg.Store.AttachNodeToMount(ctx, c.Params("id"), nodeID); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, err.Error())
			}
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Delete("/mounts/:id/nodes/:nodeId", adminIPAccess, requireRole("admin"), requireAdminScope("mounts.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DetachNodeFromMount(ctx, c.Params("id"), c.Params("nodeId")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// Server <-> Mount attachment. The mount must be eligible for the server's
	// node and egg, just as it is for the server-scoped assignment route.
	protected.Post("/mounts/:id/servers", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("mounts.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			ServerID string `json:"serverId"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.ServerID) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "serverId required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.AssignMountToServer(ctx, req.ServerID, c.Params("id"), actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if clusterManager == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "mount assignment persisted but runtime synchronization is pending")
		}
		if err := clusterManager.SyncServerConfiguration(ctx, req.ServerID); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "mount assignment persisted but runtime synchronization is pending: "+err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "runtimeSynchronized": true})
	})

	protected.Get("/mounts/:id/servers", requireRole("admin"), requireAdminScope("mounts.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		mounts, err := cfg.Store.ServerMountsForMount(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(mounts)
	})

	protected.Delete("/mounts/:id/servers/:serverId", adminIPAccess, requireRole("admin"), requireAdminScope("mounts.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DetachServerFromMount(ctx, c.Params("id"), c.Params("serverId")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if clusterManager == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "mount removal persisted but runtime synchronization is pending")
		}
		if err := clusterManager.SyncServerConfiguration(ctx, c.Params("serverId")); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "mount removal persisted but runtime synchronization is pending: "+err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "runtimeSynchronized": true})
	})
}
