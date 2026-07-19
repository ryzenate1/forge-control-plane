package http

import (
	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerTransfersServerRoutes(protected fiber.Router, cfg Config, runner *scheduleRunner, clusterManager *clustermanager.Service, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {

	protected.Post("/servers/:id/transfer", mutationLimiter, requireRole("admin"), requireAdminScope("servers.write"), legacyServerTransferUnavailable)

	protected.Get("/servers/:id/transfer", requireServerPermission(cfg, store.PermSettingsRename), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		server, err := cfg.Store.GetServer(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		return c.JSON(fiber.Map{
			"state":        server.TransferState,
			"transferring": server.Transferring,
			"targetNodeId": server.TransferTargetNodeID,
			"error":        server.TransferError,
		})
	})

	protected.Post("/servers/:id/transfer/cancel", mutationLimiter, requireRole("admin"), requireAdminScope("servers.write"), legacyServerTransferUnavailable)
}
