package http

import (
	"strings"

	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerStartupServerRoutes(protected fiber.Router, cfg Config, runner *scheduleRunner, clusterManager *clustermanager.Service, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {

	protected.Get("/servers/:id/startup", requireServerPermission(cfg, store.PermStartupRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		startup, err := cfg.Store.GetServerStartup(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		return c.JSON(startup)
	})

	protected.Put("/servers/:id/startup/variable", requireServerPermission(cfg, store.PermStartupUpdate), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req UpdateStartupVariableRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.Key) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "key is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		startup, err := cfg.Store.UpdateServerStartupVariable(ctx, c.Params("id"), strings.TrimSpace(req.Key), req.Value, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if clusterManager == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "startup variable persisted but runtime synchronization is pending")
		}
		if err := clusterManager.SyncServerConfiguration(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "startup variable persisted but runtime synchronization is pending: "+err.Error())
		}
		return c.JSON(startup)
	})

	protected.Post("/servers/:id/startup/variable", requireServerPermission(cfg, store.PermStartupUpdate), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			VariableID string `json:"variableId"`
			Value      string `json:"value"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		startup, err := cfg.Store.UpdateServerStartupVariable(ctx, c.Params("id"), req.VariableID, req.Value, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if clusterManager != nil {
			_ = clusterManager.SyncServerConfiguration(ctx, c.Params("id"))
		}
		return c.JSON(startup)
	})

	protected.Patch("/servers/:id/startup/command", requireServerPermission(cfg, store.PermStartupUpdate), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Command string `json:"command"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		server, err := cfg.Store.UpdateServer(ctx, c.Params("id"), store.UpdateServerRequest{StartupCommand: &req.Command}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if clusterManager != nil {
			_ = clusterManager.SyncServerConfiguration(ctx, c.Params("id"))
		}
		return c.JSON(server)
	})

	protected.Patch("/servers/:id/startup/image", requireServerPermission(cfg, store.PermStartupDockerImage), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Image string `json:"image"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		server, err := cfg.Store.UpdateServer(ctx, c.Params("id"), store.UpdateServerRequest{DockerImage: &req.Image}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if clusterManager != nil {
			_ = clusterManager.SyncServerConfiguration(ctx, c.Params("id"))
		}
		return c.JSON(server)
	})

	// Enforce user-level database cap.

	// Route aliases:
	//   DELETE /admin/servers/view/{serverId}/database/{databaseId}/delete
	//   PATCH  /admin/servers/view/{serverId}/database  body: { database: <id> }
	// (note: no id in URL for the reset-password endpoint). We register both
	// shapes so admin UIs that use either path work.

	// Parse pagination parameters

	// Enforce user-level backup cap (in addition to per-server cap below).

	// Enforce backup rate limit per server

	// Store a pending record immediately so the client can track progress
	// even if the daemon operation is asynchronous.

	// Initiate the backup on the daemon. The daemon will call back via
	// /api/remote/backups/:backup when the operation completes.

	// Mark the pending record as failed so it does not hang indefinitely.

	// Update the pending record with the daemon result.

	// The backup was created on the daemon but we failed to persist.
	// The daemon callback will reconcile this on retry.

	// Backup lock/unlock endpoints

	// Backup rename endpoint

	// Backup cleanup endpoint

	// Get panel settings for retention policy

	// Get server-specific backup limit

	// Perform cleanup

	// Subuser invitation endpoints

	// Create invitation with 7-day expiration

	// Look for "file" or "files" or "content" fields

	// Check for text values in case they sent text in "content" form field

	// ---- PufferPanel-inspired operations pipeline ----

	// ---- API parity endpoints ----

	protected.Patch("/servers/:id/startup", mutationLimiter, requireServerPermission(cfg, store.PermStartupUpdate), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			DockerImage    *string `json:"dockerImage"`
			StartupCommand *string `json:"startupCommand"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		server, err := cfg.Store.UpdateServer(ctx, c.Params("id"), store.UpdateServerRequest{
			DockerImage: req.DockerImage, StartupCommand: req.StartupCommand,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if clusterManager != nil && (req.DockerImage != nil || req.StartupCommand != nil) {
			_ = clusterManager.SyncServerConfiguration(ctx, c.Params("id"))
		}
		return c.JSON(server)
	})
}
