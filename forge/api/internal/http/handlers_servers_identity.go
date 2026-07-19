package http

import (
	"strings"

	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerIdentityServerRoutes(protected fiber.Router, cfg Config, runner *scheduleRunner, clusterManager *clustermanager.Service, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {
	protected.Get("/users", adminIPAccess, requireRole("admin"), requireAdminScope("users.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		users, err := cfg.Store.ListUsers(ctx)
		if err != nil {
			return err
		}
		return c.JSON(users)
	})

	protected.Post("/users", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("users.write"), func(c *fiber.Ctx) error {
		var req CreateUserRequest
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
		user, err := cfg.Store.CreateUser(ctx, store.CreateUserRequest{
			Email:           req.Email,
			Password:        req.Password,
			Role:            req.Role,
			CPULimit:        req.CPULimit,
			MemoryMBLimit:   req.MemoryMBLimit,
			DiskMBLimit:     req.DiskMBLimit,
			BackupLimit:     req.BackupLimit,
			DatabaseLimit:   req.DatabaseLimit,
			AllocationLimit: req.AllocationLimit,
			SubuserLimit:    req.SubuserLimit,
			ScheduleLimit:   req.ScheduleLimit,
			ServerLimit:     req.ServerLimit,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if cfg.MailTriggerService != nil {
			cfg.MailTriggerService.SendWelcome(ctx, user.Email, user.Email, req.Password)
		}
		return c.Status(fiber.StatusCreated).JSON(user)
	})

	protected.Patch("/users/:id", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("users.write"), func(c *fiber.Ctx) error {
		var req UpdateUserRequest
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
		user, err := cfg.Store.UpdateUser(ctx, c.Params("id"), store.UpdateUserRequest{
			Email:           req.Email,
			Password:        req.Password,
			Role:            req.Role,
			CPULimit:        req.CPULimit,
			MemoryMBLimit:   req.MemoryMBLimit,
			DiskMBLimit:     req.DiskMBLimit,
			BackupLimit:     req.BackupLimit,
			DatabaseLimit:   req.DatabaseLimit,
			AllocationLimit: req.AllocationLimit,
			SubuserLimit:    req.SubuserLimit,
			ScheduleLimit:   req.ScheduleLimit,
			ServerLimit:     req.ServerLimit,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(user)
	})

	protected.Delete("/users/:id", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("users.delete"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.DeleteUser(ctx, c.Params("id"), actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// Parse pagination parameters

	// Dedicated description endpoint.

	// Server reload: re-reads the server definition from disk on the daemon
	// (PufferPanel-style `server.reload`). Useful for picking up manually
	// edited config files without a full reinstall.

	// Enforce user-level subuser cap (for the owner of the server).

	// Enforce user-level allocation cap.

	// Zero is an explicit unlimited value for CPU, database, backup, and
	// allocation limits. Required build resources receive defaults only when omitted.

	// Enforce user-level schedule cap.

	// Best-effort: stop server processes when suspending.

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

	protected.Get("/users/search", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		query := strings.TrimSpace(c.Query("q"))
		if query == "" {
			return c.JSON([]store.User{})
		}
		ctx, cancel := requestContext()
		defer cancel()
		users, _, err := cfg.Store.SearchUsers(ctx, query, 1, 25)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(users)
	})
}
