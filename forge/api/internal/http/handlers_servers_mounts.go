package http

import (
	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerMountsServerRoutes(protected fiber.Router, cfg Config, runner *scheduleRunner, clusterManager *clustermanager.Service, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {

	protected.Get("/servers/:id/mounts", requireServerPermission(cfg, store.PermMountRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		mounts, err := cfg.Store.ServerMounts(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(mounts)
	})

	protected.Post("/servers/:id/mounts", requireRole("admin"), requireAdminScope("mounts.write"), func(c *fiber.Ctx) error {
		var req AssignMountRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
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
		if err := cfg.Store.AssignMountToServer(ctx, c.Params("id"), req.MountID, actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if clusterManager == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "mount assignment persisted but runtime synchronization is pending")
		}
		if err := clusterManager.SyncServerConfiguration(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "mount assignment persisted but runtime synchronization is pending: "+err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "runtimeSynchronized": true})
	})

	protected.Delete("/servers/:id/mounts/:mountId", requireRole("admin"), requireAdminScope("mounts.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.RemoveMountFromServer(ctx, c.Params("id"), c.Params("mountId"), actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if clusterManager == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "mount removal persisted but runtime synchronization is pending")
		}
		if err := clusterManager.SyncServerConfiguration(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "mount removal persisted but runtime synchronization is pending: "+err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "runtimeSynchronized": true})
	})
}
