package http

import (
	"time"

	"gamepanel/forge/internal/services/activity"
	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerAccessServerRoutes(protected fiber.Router, cfg Config, runner *scheduleRunner, clusterManager *clustermanager.Service, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {

	protected.Get("/servers/:id/users", requireServerPermission(cfg, store.PermUserRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		subusers, err := cfg.Store.ListServerSubusers(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(subusers)
	})

	protected.Get("/servers/:id/activity", requireServerPermission(cfg, store.PermActivityRead), func(c *fiber.Ctx) error {
		if cfg.ActivityService == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "activity service is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		serverID := c.Params("id")
		subjectType := "server"
		filter := activity.ActivityFilter{
			SubjectType: &subjectType,
			SubjectID:   &serverID,
			Limit:       100,
		}
		events, err := cfg.ActivityService.Query(ctx, filter)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(events)
	})

	protected.Post("/servers/:id/users", mutationLimiter, requireServerPermission(cfg, store.PermUserCreate), func(c *fiber.Ctx) error {
		var req UpsertSubuserRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		// Enforce user-level subuser cap (for the owner of the server).
		if ownerID, ok := serverOwner(ctx, cfg, c.Params("id")); ok {
			if err := cfg.Store.CheckUserCanCreateSubuser(ctx, ownerID); err != nil {
				if store.IsUserLimitError(err) {
					return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
				}
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
		}
		var actorEmail string
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorEmail = claims.Email
			actorID = &claims.Sub
		}
		subuser, err := cfg.Store.UpsertServerSubuser(ctx, c.Params("id"), store.UpsertServerSubuserRequest{
			Email:       req.Email,
			Permissions: req.Permissions,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if cfg.MailTriggerService != nil && subuser.Email != "" {
			if srv, e := cfg.Store.GetServer(ctx, c.Params("id")); e == nil {
				cfg.MailTriggerService.SendSubuserInvited(ctx, subuser.Email, subuser.Email, actorEmail, srv.Name, srv.ID)
			}
		}
		return c.Status(fiber.StatusCreated).JSON(subuser)
	})

	protected.Patch("/servers/:id/users/:userId", mutationLimiter, requireServerPermission(cfg, store.PermUserUpdate), func(c *fiber.Ctx) error {
		var req UpsertSubuserRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		existing, err := cfg.Store.GetServerSubuser(ctx, c.Params("id"), c.Params("userId"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "subuser not found")
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		subuser, err := cfg.Store.UpsertServerSubuser(ctx, c.Params("id"), store.UpsertServerSubuserRequest{
			Email:       existing.Email,
			Permissions: req.Permissions,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(subuser)
	})

	protected.Delete("/servers/:id/users/:userId", mutationLimiter, requireServerPermission(cfg, store.PermUserDelete), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorEmail string
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorEmail = claims.Email
			actorID = &claims.Sub
		}
		if cfg.MailTriggerService != nil {
			if subuser, e := cfg.Store.GetServerSubuser(ctx, c.Params("id"), c.Params("userId")); e == nil {
				if srv, se := cfg.Store.GetServer(ctx, c.Params("id")); se == nil {
					cfg.MailTriggerService.SendSubuserRemoved(ctx, subuser.Email, subuser.Email, actorEmail, srv.Name)
				}
			}
		}
		if err := cfg.Store.DeleteServerSubuser(ctx, c.Params("id"), c.Params("userId"), actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

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
	protected.Get("/servers/:id/invitations", requireServerPermission(cfg, store.PermUserRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()

		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}

		invitations, err := cfg.Store.ListSubuserInvitations(ctx, target.ServerID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		return c.JSON(invitations)
	})

	protected.Post("/servers/:id/invitations", mutationLimiter, requireServerPermission(cfg, store.PermUserCreate), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req store.CreateSubuserInvitationRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}

		ctx, cancel := requestContext()
		defer cancel()

		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}

		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}

		// Create invitation with 7-day expiration
		invitation, err := cfg.Store.CreateSubuserInvitation(ctx, target.ServerID, req, actorID, 7*24*time.Hour)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		return c.Status(fiber.StatusCreated).JSON(invitation)
	})

	protected.Delete("/servers/:id/invitations/:invitationId", mutationLimiter, requireServerPermission(cfg, store.PermUserDelete), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()

		err := cfg.Store.DeleteSubuserInvitation(ctx, c.Params("invitationId"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/invitations/:invitationId/revoke", mutationLimiter, requireServerPermission(cfg, store.PermUserDelete), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()

		err := cfg.Store.RevokeSubuserInvitation(ctx, c.Params("invitationId"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		return c.JSON(fiber.Map{"ok": true})
	})
}
