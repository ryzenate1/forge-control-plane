package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/services/activity"
	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/services/migration"
	"gamepanel/forge/internal/services/queue"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// safeAuditMeta serializes audit metadata to JSON safely, preventing
// injection via user-controlled values like file paths or backup names.
func safeAuditMeta(kv map[string]string) string {
	b, err := json.Marshal(kv)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func validFileMode(mode string) bool {
	if len(mode) != 3 && len(mode) != 4 {
		return false
	}
	for _, character := range mode {
		if character < '0' || character > '7' {
			return false
		}
	}
	return true
}

func legacyServerTransferUnavailable(c *fiber.Ctx) error {
	return fiber.NewError(fiber.StatusNotImplemented, "legacy server transfer endpoints are not implemented")
}

func legacyServerTransferCallbackUnavailable(c *fiber.Ctx) error {
	return fiber.NewError(fiber.StatusGone, "legacy server transfer callbacks have been retired")
}

func createResourceValue(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func ensureTransferIdle(c *fiber.Ctx, cfg Config, serverID string) error {
	if cfg.Store == nil {
		return nil
	}
	ctx, cancel := requestContext()
	defer cancel()
	blocked, err := cfg.Store.IsServerTransferBlocking(ctx, serverID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "server not found")
	}
	if blocked {
		return fiber.NewError(fiber.StatusConflict, "server transfer in progress")
	}
	return nil
}

func registerServerRoutes(protected fiber.Router, cfg Config, runner *scheduleRunner, clusterManager *clustermanager.Service, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {
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

	protected.Get("/servers", requireAdminScope("servers.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}

		// Parse pagination parameters
		page := 1
		if p := c.Query("page"); p != "" {
			if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
				page = parsed
			}
		}

		perPage := 15
		if pp := c.Query("per_page"); pp != "" {
			if parsed, err := strconv.Atoi(pp); err == nil && parsed > 0 && parsed <= 100 {
				perPage = parsed
			}
		}

		search := c.Query("search", "")

		ctx, cancel := requestContext()
		defer cancel()
		servers, total, err := cfg.Store.ListServersForUser(ctx, claims.Sub, claims.Role, page, perPage, search)
		if err != nil {
			return err
		}

		totalPages := (total + perPage - 1) / perPage
		if totalPages == 0 {
			totalPages = 1
		}

		return c.JSON(fiber.Map{
			"data": servers,
			"meta": fiber.Map{
				"pagination": fiber.Map{
					"current":       page,
					"total":         totalPages,
					"count":         len(servers),
					"per_page":      perPage,
					"total_records": total,
				},
			},
		})
	})

	protected.Get("/servers/:id", requireServerAccess(cfg), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		server, err := cfg.Store.GetServer(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		if claims.Role == "admin" || claims.Sub == server.OwnerID {
			server.Permissions = []string{"*"}
		} else {
			subuser, err := cfg.Store.GetServerSubuser(ctx, server.ID, claims.Sub)
			if err != nil {
				return fiber.NewError(fiber.StatusForbidden, "server access is not assigned to this user")
			}
			server.Permissions = subuser.Permissions
		}
		return c.JSON(server)
	})

	protected.Patch("/servers/:id", mutationLimiter, func(c *fiber.Ctx) error {
		var req UpdateServerRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		detailsChanged := req.Name != nil || req.Description != nil
		startupChanged := req.StartupCommand != nil
		imageChanged := req.DockerImage != nil
		allocationChanged := req.PrimaryAlloc != nil
		adminChanged := req.OwnerID != nil || req.MemoryMB != nil || req.CPUShares != nil || req.CPULimit != nil || req.DiskMB != nil || req.DatabaseLimit != nil || req.BackupLimit != nil || req.AllocationLimit != nil || req.IOWeight != nil || req.SwapMB != nil || req.Threads != nil || req.OOMDisabled != nil
		if !detailsChanged && !startupChanged && !imageChanged && !allocationChanged && !adminChanged {
			return fiber.NewError(fiber.StatusBadRequest, "at least one supported field is required")
		}
		if detailsChanged {
			if err := checkServerPermission(c, cfg, store.PermSettingsRename); err != nil {
				return err
			}
		}
		if startupChanged {
			if err := checkServerPermission(c, cfg, store.PermStartupUpdate); err != nil {
				return err
			}
		}
		if imageChanged {
			if err := checkServerPermission(c, cfg, store.PermStartupDockerImage); err != nil {
				return err
			}
		}
		if allocationChanged {
			if err := checkServerPermission(c, cfg, store.PermAllocationUpdate); err != nil {
				return err
			}
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if adminChanged && (!ok || claims.Role != "admin") {
			return fiber.NewError(fiber.StatusForbidden, "admin role is required to update owner or build limits")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if ok {
			actorID = &claims.Sub
		}
		server, err := cfg.Store.UpdateServer(ctx, c.Params("id"), store.UpdateServerRequest{
			Name: req.Name, Description: req.Description, OwnerID: req.OwnerID,
			MemoryMB: req.MemoryMB, CPUShares: req.CPUShares, CPULimit: req.CPULimit, DiskMB: req.DiskMB,
			DatabaseLimit: req.DatabaseLimit, BackupLimit: req.BackupLimit, AllocationLimit: req.AllocationLimit,
			IOWeight: req.IOWeight, SwapMB: req.SwapMB, Threads: req.Threads, OOMDisabled: req.OOMDisabled,
			DockerImage: req.DockerImage, StartupCommand: req.StartupCommand, PrimaryAllocationID: req.PrimaryAlloc,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if server.ConfigSyncPending {
			if clusterManager == nil {
				return fiber.NewError(fiber.StatusServiceUnavailable, "runtime synchronization is unavailable; update is pending sync")
			}
			if err := clusterManager.SyncServerConfiguration(ctx, server.ID); err != nil {
				return fiber.NewError(fiber.StatusBadGateway, "update persisted but runtime synchronization is pending: "+err.Error())
			}
			server, err = cfg.Store.GetServer(ctx, server.ID)
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
		}
		return c.JSON(server)
	})

	// Dedicated description endpoint.
	protected.Post("/servers/:id/description", mutationLimiter, requireServerPermission(cfg, store.PermSettingsRename), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Description string `json:"description"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		server, err := cfg.Store.UpdateServer(ctx, c.Params("id"), store.UpdateServerRequest{
			Description: &req.Description,
		}, nil)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(server)
	})

	// Server reload: re-reads the server definition from disk on the daemon
	// (PufferPanel-style `server.reload`). Useful for picking up manually
	// edited config files without a full reinstall.
	protected.Post("/servers/:id/reload", mutationLimiter, requireServerPermission(cfg, store.PermSettingsReinstall), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerProvisionTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if err := cfg.Daemon.SyncServerConfiguration(ctx, target.NodeURL, target.NodeToken, target.ServerID, buildDaemonServerConfiguration(target.ToDTO())); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Get("/servers/:id/allocations", requireServerPermission(cfg, store.PermAllocationRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		allocations, err := cfg.Store.ListServerAllocations(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(allocations)
	})

	protected.Patch("/servers/:id/allocations/:allocationId", mutationLimiter, requireServerPermission(cfg, store.PermAllocationUpdate), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req UpdateAllocationRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		allocation, err := cfg.Store.UpdateServerAllocation(ctx, c.Params("id"), c.Params("allocationId"), store.UpdateAllocationRequest{Alias: req.Alias, Notes: req.Notes}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(allocation)
	})

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

	protected.Post("/servers/:id/allocations", mutationLimiter, requireServerPermission(cfg, store.PermAllocationCreate), func(c *fiber.Ctx) error {
		var body struct {
			AllocationID string `json:"allocationId"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if body.AllocationID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "allocationId is required")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		// Enforce user-level allocation cap.
		ctx, cancel := requestContext()
		defer cancel()
		if ownerID, ok := serverOwner(ctx, cfg, c.Params("id")); ok {
			if err := cfg.Store.CheckUserCanCreateAllocation(ctx, ownerID); err != nil {
				if store.IsUserLimitError(err) {
					return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
				}
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.AssignAllocationToServer(ctx, c.Params("id"), body.AllocationID, actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true})
	})

	protected.Delete("/servers/:id/allocations/:allocationId", mutationLimiter, requireServerPermission(cfg, store.PermAllocationDelete), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.UnassignAllocationFromServer(ctx, c.Params("id"), c.Params("allocationId"), actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/allocations/:allocationId/primary", mutationLimiter, requireServerPermission(cfg, store.PermAllocationUpdate), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.SetPrimaryAllocation(ctx, c.Params("id"), c.Params("allocationId"), actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/transfer", mutationLimiter, requireRole("admin"), requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		if cfg.MigrationService == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "migration service is not available")
		}
		var req TransferServerRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.TargetNodeID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "targetNodeId is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		m, err := cfg.MigrationService.CreateMigration(ctx, migration.CreateMigrationRequest{
			ServerID:     c.Params("id"),
			TargetNodeID: req.TargetNodeID,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		executed, err := cfg.MigrationService.ExecuteMigration(ctx, m.ID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.Status(fiber.StatusAccepted).JSON(executed)
	})

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

	protected.Post("/servers/:id/transfer/cancel", mutationLimiter, requireRole("admin"), requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		if cfg.MigrationService == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "migration service is not available")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		migrations, err := cfg.MigrationService.ListMigrations(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		var active *store.Migration
		for _, m := range migrations {
			if m.ServerID == c.Params("id") && m.Status != string(store.MigrationStatusCompleted) && m.Status != string(store.MigrationStatusFailed) && m.Status != string(store.MigrationStatusCancelled) {
				cp := m
				active = &cp
				break
			}
		}
		if active == nil {
			return fiber.NewError(fiber.StatusNotFound, "no active transfer found for this server")
		}
		cancelled, err := cfg.MigrationService.CancelMigration(ctx, active.ID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(cancelled)
	})

	protected.Post("/servers", mutationLimiter, requireRole("admin"), requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		var req CreateServerRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := Validate(&req); err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(err)
		}
		if req.TemplateID == "" || (req.RegionID == "" && req.Region == "" && req.NodeID == "" && req.RequiredNode == "") {
			return fiber.NewError(fiber.StatusBadRequest, "templateId, and regionId or nodeId are required")
		}
		if req.OwnerID == "" {
			if claims, ok := c.Locals("user").(tokenClaims); ok {
				req.OwnerID = claims.Sub
			}
		}
		if cfg.Store == nil || clusterManager == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and runtime lifecycle service are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		defaultMemoryMB := 2048
		if req.MemoryMB == nil {
			egg, err := cfg.Store.GetEgg(ctx, req.TemplateID)
			if err != nil {
				return fiber.NewError(fiber.StatusBadRequest, "egg not found")
			}
			defaultMemoryMB = egg.DefaultMemoryMB
		}
		memoryMB := createResourceValue(req.MemoryMB, defaultMemoryMB)
		cpuShares := createResourceValue(req.CPUShares, 1024)
		cpuLimit := createResourceValue(req.CPU, 0)
		diskMB := createResourceValue(req.DiskMB, 10240)
		databaseLimit := createResourceValue(req.DatabaseLimit, 0)
		backupLimit := createResourceValue(req.BackupLimit, 0)
		allocationLimit := createResourceValue(req.AllocationLimit, 0)
		ioWeight := createResourceValue(req.IOWeight, 500)
		swapMB := createResourceValue(req.SwapMB, 0)
		if memoryMB <= 0 || cpuShares <= 0 || diskMB <= 0 || cpuLimit < 0 || databaseLimit < 0 || backupLimit < 0 || allocationLimit < 0 || swapMB < -1 || ioWeight < 10 || ioWeight > 1000 {
			return fiber.NewError(fiber.StatusBadRequest, "invalid server resource limits")
		}
		// Zero is an explicit unlimited value for CPU, database, backup, and
		// allocation limits. Required build resources receive defaults only when omitted.
		if err := cfg.Store.CheckUserCanCreateServer(ctx, req.OwnerID, memoryMB, diskMB, cpuLimit); err != nil {
			if store.IsUserLimitError(err) {
				return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
			}
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		server, _, err := clusterManager.CreateServer(ctx, store.CreateServerRequest{
			Name:                    req.Name,
			NodeID:                  req.NodeID,
			OwnerID:                 req.OwnerID,
			TemplateID:              req.TemplateID,
			AllocationID:            req.AllocationID,
			AdditionalAllocationIDs: req.AdditionalAllocationIDs,
			MemoryMB:                memoryMB,
			CPUShares:               cpuShares,
			CPULimit:                cpuLimit,
			DiskMB:                  diskMB,
			DatabaseLimit:           databaseLimit,
			BackupLimit:             backupLimit,
			AllocationLimit:         allocationLimit,
			IOWeight:                ioWeight,
			SwapMB:                  swapMB,
			Threads:                 req.Threads,
			OOMDisabled:             req.OOMDisabled,
			DockerImage:             req.DockerImage,
			StartupCommand:          req.StartupCommand,
			StartupVariables:        req.StartupVariables,
		}, domain.PlacementRequest{
			RegionID:      req.RegionID,
			Region:        req.Region,
			NodeID:        req.NodeID,
			PreferredNode: req.PreferredNode,
			RequiredNode:  req.RequiredNode,
			AllocationID:  req.AllocationID,
			MemoryMB:      memoryMB,
			CPUShares:     cpuShares,
			CPU:           cpuLimit,
			DiskMB:        diskMB,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		if cfg.Store != nil {
			cfg.Store.DispatchWebhookEvent("server:created", map[string]any{
				"subject_type": "server",
				"subject_id":   server.ID,
				"name":         server.Name,
				"owner_id":     server.Owner,
				"node_id":      server.Node,
			})
		}
		return c.Status(fiber.StatusCreated).JSON(server)
	})

	protected.Post("/servers/:id/power", mutationLimiter, func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		var req PowerRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		switch req.Signal {
		case "start", "stop", "restart", "kill":
			requiredPermission := store.PermControlStart
			switch req.Signal {
			case "stop", "kill":
				requiredPermission = store.PermControlStop
			case "restart":
				requiredPermission = store.PermControlRestart
			}
			if err := checkServerPermission(c, cfg, requiredPermission); err != nil {
				return err
			}
			idempotencyKey := strings.TrimSpace(c.Get("Idempotency-Key"))
			if idempotencyKey != "" {
				idempotencyKey = "power:" + c.Params("id") + ":" + req.Signal + ":" + idempotencyKey
			}
			ctx, cancel := requestContext()
			defer cancel()
			if cfg.OperationService != nil {
				op, err := cfg.OperationService.DispatchPower(ctx, c.Params("id"), req.Signal, idempotencyKey)
				if err != nil {
					return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
				}
				return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
					"serverId":    c.Params("id"),
					"signal":      req.Signal,
					"accepted":    true,
					"mode":        "durable",
					"operationId": op.ID,
				})
			}
			if cfg.QueueService == nil {
				return fiber.NewError(fiber.StatusServiceUnavailable, "durable operation service is required")
			}
			jobType := queue.JobServerStart
			switch req.Signal {
			case "stop":
				jobType = queue.JobServerStop
			case "restart":
				jobType = queue.JobServerRestart
			case "kill":
				jobType = queue.JobServerKill
			}
			job, err := cfg.QueueService.DispatchIdempotent(ctx, idempotencyKey, jobType, c.Params("id"), "", map[string]any{"signal": req.Signal}, 100)
			if err != nil {
				return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
			}
			return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
				"serverId":    c.Params("id"),
				"signal":      req.Signal,
				"accepted":    true,
				"mode":        "queued",
				"operationId": job.ID,
			})
		default:
			return fiber.NewError(fiber.StatusBadRequest, "invalid power signal")
		}
	})

	protected.Get("/operations/:id", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if cfg.OperationService != nil {
			op, err := cfg.OperationService.Get(ctx, c.Params("id"))
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
			if op == nil {
				return fiber.NewError(fiber.StatusNotFound, "operation not found")
			}
			return c.JSON(op)
		}
		if cfg.QueueService == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "durable operation service is required")
		}
		job, err := cfg.QueueService.Get(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if job == nil {
			return fiber.NewError(fiber.StatusNotFound, "operation not found")
		}
		return c.JSON(job)
	})

	protected.Post("/servers/:id/install", mutationLimiter, requireRole("admin"), requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if clusterManager == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "runtime lifecycle service is required")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		response, err := clusterManager.InstallServer(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		if cfg.Store != nil {
			cfg.Store.DispatchWebhookEvent("server:installed", map[string]any{"subject_type": "server", "subject_id": c.Params("id")})
		}
		return c.Status(fiber.StatusAccepted).JSON(response)
	})

	protected.Get("/servers/:id/configuration", requireRole("admin"), requireAdminScope("servers.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerProvisionTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		return c.JSON(buildDaemonServerConfiguration(target.ToDTO()))
	})

	protected.Post("/servers/:id/reinstall", mutationLimiter, requireServerPermission(cfg, store.PermSettingsReinstall), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if clusterManager == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "runtime lifecycle service is required")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		response, err := clusterManager.ReinstallServer(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.Status(fiber.StatusAccepted).JSON(response)
	})

	protected.Get("/servers/:id/schedules", requireServerPermission(cfg, store.PermScheduleRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		schedules, err := cfg.Store.ListSchedules(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(schedules)
	})

	protected.Get("/servers/:id/schedules/:scheduleId", requireServerPermission(cfg, store.PermScheduleRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		schedule, err := cfg.Store.GetSchedule(ctx, c.Params("id"), c.Params("scheduleId"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "schedule not found")
		}
		return c.JSON(schedule)
	})

	protected.Post("/servers/:id/schedules", requireServerPermission(cfg, store.PermScheduleCreate), func(c *fiber.Ctx) error {
		var req CreateScheduleRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		// Enforce user-level schedule cap.
		if ownerID, ok := serverOwner(ctx, cfg, c.Params("id")); ok {
			if err := cfg.Store.CheckUserCanCreateSchedule(ctx, ownerID); err != nil {
				if store.IsUserLimitError(err) {
					return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
				}
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		schedule, err := cfg.Store.CreateSchedule(ctx, c.Params("id"), store.CreateScheduleRequest{
			Name:           req.Name,
			CronMinute:     req.CronMinute,
			CronHour:       req.CronHour,
			CronDayOfMonth: req.CronDayOfMonth,
			CronMonth:      req.CronMonth,
			CronDayOfWeek:  req.CronDayOfWeek,
			OnlyWhenOnline: req.OnlyWhenOnline,
			Enabled:        req.Enabled,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(schedule)
	})

	protected.Patch("/servers/:id/schedules/:scheduleId", requireServerPermission(cfg, store.PermScheduleUpdate), func(c *fiber.Ctx) error {
		var req PatchScheduleRequest
		if err := c.BodyParser(&req); err != nil && err != io.EOF {
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
		schedule, err := cfg.Store.PatchSchedule(ctx, c.Params("id"), c.Params("scheduleId"), store.PatchScheduleRequest{
			Name:           req.Name,
			CronMinute:     req.CronMinute,
			CronHour:       req.CronHour,
			CronDayOfMonth: req.CronDayOfMonth,
			CronMonth:      req.CronMonth,
			CronDayOfWeek:  req.CronDayOfWeek,
			OnlyWhenOnline: req.OnlyWhenOnline,
			Enabled:        req.Enabled,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(schedule)
	})

	protected.Delete("/servers/:id/schedules/:scheduleId", requireServerPermission(cfg, store.PermScheduleDelete), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.DeleteSchedule(ctx, c.Params("id"), c.Params("scheduleId"), actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/schedules/:scheduleId/tasks", requireServerPermission(cfg, store.PermScheduleUpdate), func(c *fiber.Ctx) error {
		var req CreateScheduleTaskRequest
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
		task, err := cfg.Store.CreateScheduleTask(ctx, c.Params("id"), c.Params("scheduleId"), store.CreateScheduleTaskRequest{
			Sequence:          req.Sequence,
			Action:            req.Action,
			Payload:           req.Payload,
			TimeOffsetSeconds: req.TimeOffsetSeconds,
			ContinueOnFailure: req.ContinueOnFailure,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(task)
	})

	protected.Post("/servers/:id/schedules/:scheduleId/run", requireServerPermission(cfg, store.PermScheduleUpdate), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := runner.RunNow(ctx, c.Params("id"), c.Params("scheduleId")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"ok": true})
	})

	protected.Get("/servers/:id/schedules/:scheduleId/runs", requireServerPermission(cfg, store.PermScheduleRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		runs, err := cfg.Store.ListScheduleRuns(ctx, c.Params("id"), c.Params("scheduleId"), 20)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(runs)
	})

	protected.Patch("/servers/:id/schedules/:scheduleId/tasks/:taskId", requireServerPermission(cfg, store.PermScheduleUpdate), func(c *fiber.Ctx) error {
		var req PatchScheduleTaskRequest
		if err := c.BodyParser(&req); err != nil && err != io.EOF {
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
		task, err := cfg.Store.PatchScheduleTask(ctx, c.Params("id"), c.Params("scheduleId"), c.Params("taskId"), store.PatchScheduleTaskRequest{
			Sequence:          req.Sequence,
			Action:            req.Action,
			Payload:           req.Payload,
			TimeOffsetSeconds: req.TimeOffsetSeconds,
			ContinueOnFailure: req.ContinueOnFailure,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(task)
	})

	protected.Delete("/servers/:id/schedules/:scheduleId/tasks/:taskId", requireServerPermission(cfg, store.PermScheduleDelete), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.DeleteScheduleTask(ctx, c.Params("id"), c.Params("scheduleId"), c.Params("taskId"), actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Get("/servers/:id/schedules/:scheduleId/tasks", requireServerPermission(cfg, store.PermScheduleRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		tasks, err := cfg.Store.ListScheduleTasks(ctx, c.Params("id"), c.Params("scheduleId"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(tasks)
	})

	protected.Post("/servers/:id/toggle-install", requireRole("admin"), requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		next, err := cfg.Store.ToggleServerInstallStatus(ctx, c.Params("id"), actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"status": next})
	})

	protected.Post("/servers/:id/suspension", requireRole("admin"), requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		var body struct {
			Action string `json:"action"`
		}
		_ = c.BodyParser(&body)
		switch body.Action {
		case "suspend", "unsuspend":
		default:
			return fiber.NewError(fiber.StatusBadRequest, "action must be suspend or unsuspend")
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
		suspended := body.Action == "suspend"
		if suspended && cfg.Daemon != nil {
			// Best-effort: stop server processes when suspending.
			if target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id")); err == nil {
				_, _ = cfg.Daemon.SendPower(ctx, target.NodeURL, target.NodeToken, target.ServerID, "stop")
				_ = cfg.Store.SetServerPowerState(ctx, target.ServerID, "stop")
			}
		}
		if err := cfg.Store.SetServerSuspended(ctx, c.Params("id"), suspended, actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "suspended": suspended})
	})

	protected.Post("/servers/:id/suspend", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.SetServerSuspension(ctx, c.Params("id"), true); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/unsuspend", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.SetServerSuspension(ctx, c.Params("id"), false); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Delete("/servers/:id", requireRole("admin"), requireAdminScope("servers.delete"), func(c *fiber.Ctx) error {
		forceDelete := c.Query("force") == "1" || c.Query("force") == "true"
		if cfg.Store == nil || clusterManager == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and runtime lifecycle service are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		response, err := clusterManager.DeleteServer(ctx, c.Params("id"), forceDelete)
		if err != nil && !forceDelete {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		if err != nil && forceDelete {
			return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
				"serverId": c.Params("id"),
				"accepted": true,
				"mode":     "force",
				"warning":  err.Error(),
			})
		}
		return c.Status(fiber.StatusAccepted).JSON(response)
	})

	protected.Get("/servers/:id/stats", requireServerPermission(cfg, store.PermWebsocketConnect), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		stats, err := cfg.Daemon.Stats(ctx, target.NodeURL, target.NodeToken, target.ServerID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(stats)
	})

	protected.Get("/servers/:id/logs", requireServerPermission(cfg, store.PermWebsocketConnect), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		logs, err := cfg.Daemon.Logs(ctx, target.NodeURL, target.NodeToken, target.ServerID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		c.Set("Content-Type", "text/plain; charset=utf-8")
		return c.SendString(logs)
	})

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

	protected.Get("/servers/:id/databases", requireServerPermission(cfg, store.PermDatabaseRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		databases, err := cfg.Store.ListServerDatabases(ctx, c.Params("id"), false)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(databases)
	})

	protected.Get("/servers/:id/databases/:databaseId", requireServerPermission(cfg, store.PermDatabaseRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		database, err := cfg.Store.GetServerDatabase(ctx, c.Params("id"), c.Params("databaseId"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "database not found")
		}
		return c.JSON(database)
	})

	protected.Post("/servers/:id/databases", requireServerPermission(cfg, store.PermDatabaseCreate), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req CreateServerDatabaseRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		// Enforce user-level database cap.
		if ownerID, ok := serverOwner(ctx, cfg, c.Params("id")); ok {
			if err := cfg.Store.CheckUserCanCreateDatabase(ctx, ownerID); err != nil {
				if store.IsUserLimitError(err) {
					return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
				}
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		database, err := cfg.Store.CreateServerDatabase(ctx, c.Params("id"), store.CreateServerDatabaseRequest{
			Database:       req.Database,
			Remote:         req.Remote,
			MaxConnections: req.MaxConnections,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if cfg.DBProvisioner == nil {
			_ = cfg.Store.SetServerDatabaseProvisioningState(ctx, c.Params("id"), database.ID, store.DatabaseStateFailed, "database provisioner is unavailable")
			return fiber.NewError(fiber.StatusServiceUnavailable, "database record created in failed state: database provisioner is unavailable")
		}
		if err := cfg.DBProvisioner.Provision(ctx, c.Params("id"), database.ID); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "database provisioning failed; record retained in failed state: "+err.Error())
		}
		database, err = cfg.Store.GetServerDatabaseForProvisioning(ctx, c.Params("id"), database.ID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "database is ready but its creation response could not be loaded")
		}
		if database.ProvisioningState != store.DatabaseStateReady {
			database.Password = nil
			return fiber.NewError(fiber.StatusInternalServerError, "database did not reach ready state")
		}
		return c.Status(fiber.StatusCreated).JSON(database)
	})

	protected.Post("/servers/:id/databases/:databaseId/rotate-password", requireServerPermission(cfg, store.PermDatabaseUpdate), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if cfg.DBProvisioner == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "database provisioner is unavailable")
		}
		database, err := cfg.DBProvisioner.RotatePassword(ctx, c.Params("id"), c.Params("databaseId"), actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "failed to rotate database password: "+err.Error())
		}
		return c.JSON(database)
	})

	protected.Delete("/servers/:id/databases/:databaseId", requireServerPermission(cfg, store.PermDatabaseDelete), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		force := strings.EqualFold(c.Query("force"), "true")
		var deprovisionErr error
		if cfg.DBProvisioner == nil {
			deprovisionErr = errors.New("database provisioner is unavailable")
		} else {
			deprovisionErr = cfg.DBProvisioner.Deprovision(ctx, c.Params("id"), c.Params("databaseId"))
		}
		if deprovisionErr != nil {
			if !force {
				return fiber.NewError(fiber.StatusBadGateway, "failed to deprovision database; panel record retained: "+deprovisionErr.Error())
			}
			if err := cfg.Store.ForceDeleteServerDatabase(ctx, c.Params("id"), c.Params("databaseId"), deprovisionErr.Error(), actorID); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "failed to record orphan remediation: "+err.Error())
			}
			return c.JSON(fiber.Map{"ok": true, "orphanRemediation": true})
		}
		if err := cfg.Store.DeleteServerDatabase(ctx, c.Params("id"), c.Params("databaseId"), actorID); err != nil {
			detail := "remote database resources were removed, but panel record deletion failed: " + err.Error()
			_ = cfg.Store.SetServerDatabaseProvisioningState(ctx, c.Params("id"), c.Params("databaseId"), store.DatabaseStateFailed, detail)
			return fiber.NewError(fiber.StatusInternalServerError, detail)
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// Route aliases:
	//   DELETE /admin/servers/view/{serverId}/database/{databaseId}/delete
	//   PATCH  /admin/servers/view/{serverId}/database  body: { database: <id> }
	// (note: no id in URL for the reset-password endpoint). We register both
	// shapes so admin UIs that use either path work.
	protected.Delete("/servers/:id/databases/:databaseId/delete", requireServerPermission(cfg, store.PermDatabaseDelete), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		force := strings.EqualFold(c.Query("force"), "true")
		var deprovisionErr error
		if cfg.DBProvisioner == nil {
			deprovisionErr = errors.New("database provisioner is unavailable")
		} else {
			deprovisionErr = cfg.DBProvisioner.Deprovision(ctx, c.Params("id"), c.Params("databaseId"))
		}
		if deprovisionErr != nil {
			if !force {
				return fiber.NewError(fiber.StatusBadGateway, "failed to deprovision database; panel record retained: "+deprovisionErr.Error())
			}
			if err := cfg.Store.ForceDeleteServerDatabase(ctx, c.Params("id"), c.Params("databaseId"), deprovisionErr.Error(), actorID); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "failed to record orphan remediation: "+err.Error())
			}
			return c.SendStatus(fiber.StatusNoContent)
		}
		if err := cfg.Store.DeleteServerDatabase(ctx, c.Params("id"), c.Params("databaseId"), actorID); err != nil {
			detail := "remote database resources were removed, but panel record deletion failed: " + err.Error()
			_ = cfg.Store.SetServerDatabaseProvisioningState(ctx, c.Params("id"), c.Params("databaseId"), store.DatabaseStateFailed, detail)
			return fiber.NewError(fiber.StatusInternalServerError, detail)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
	protected.Patch("/servers/:id/databases/reset-password", requireServerPermission(cfg, store.PermDatabaseUpdate), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var body struct {
			Database string `json:"database"`
		}
		if err := c.BodyParser(&body); err != nil || body.Database == "" {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body; expected { database: <id> }")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if cfg.DBProvisioner == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "database provisioner is unavailable")
		}
		if _, err := cfg.DBProvisioner.RotatePassword(ctx, c.Params("id"), body.Database, actorID); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "failed to rotate database password: "+err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

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

	protected.Get("/servers/:id/backups", requireServerPermission(cfg, store.PermBackupRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()

		// Parse pagination parameters
		page := 1
		if pageStr := c.Query("page"); pageStr != "" {
			if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
				page = p
			}
		}
		perPage := 20
		if perPageStr := c.Query("per_page"); perPageStr != "" {
			if pp, err := strconv.Atoi(perPageStr); err == nil && pp > 0 && pp <= 100 {
				perPage = pp
			}
		}

		backups, err := cfg.Store.ListBackups(ctx, c.Params("id"), page, perPage)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		total, err := cfg.Store.CountBackups(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{
			"data": backups,
			"pagination": fiber.Map{
				"page":        page,
				"per_page":    perPage,
				"total":       total,
				"total_pages": (total + perPage - 1) / perPage,
			},
		})
	})

	protected.Get("/servers/:id/backups/:backupName", requireServerPermission(cfg, store.PermBackupRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		backup, err := cfg.Store.GetBackupByName(ctx, target.ServerID, c.Params("backupName"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "backup not found")
		}
		return c.JSON(backup)
	})

	protected.Post("/servers/:id/backups", requireServerPermission(cfg, store.PermBackupCreate), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		// Enforce user-level backup cap (in addition to per-server cap below).
		if ownerID, ok := serverOwner(ctx, cfg, c.Params("id")); ok {
			if err := cfg.Store.CheckUserCanCreateBackup(ctx, ownerID); err != nil {
				if store.IsUserLimitError(err) {
					return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
				}
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
		}
		// Enforce backup rate limit per server
		if settings, err := cfg.Store.GetPanelSettings(ctx); err == nil && settings.BackupRateLimitEnabled && settings.BackupRateLimitCount > 0 {
			recentCount, err := cfg.Store.CountRecentBackups(ctx, c.Params("id"), settings.BackupRateLimitWindowMinutes)
			if err == nil && recentCount >= settings.BackupRateLimitCount {
				return fiber.NewError(fiber.StatusTooManyRequests, "backup rate limit exceeded")
			}
		}
		limit, err := cfg.Store.BackupLimit(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if limit > 0 {
			count, err := cfg.Store.CountCompletedBackups(ctx, c.Params("id"))
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
			if count >= limit {
				return fiber.NewError(fiber.StatusConflict, "backup limit reached")
			}
		}
		var req struct {
			IgnoredFiles []string `json:"ignored"`
		}
		_ = c.BodyParser(&req)

		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		// Store a pending record immediately so the client can track progress
		// even if the daemon operation is asynchronous.
		pending := store.UpsertBackupRequest{
			Name:   fmt.Sprintf("backup-%s", time.Now().UTC().Format("20060102T150405Z")),
			Status: "pending",
		}
		stored, storeErr := cfg.Store.UpsertBackup(ctx, target.ServerID, pending, actorID)
		if storeErr != nil {
			return fiber.NewError(fiber.StatusInternalServerError, storeErr.Error())
		}

		// Initiate the backup on the daemon. The daemon will call back via
		// /api/remote/backups/:backup when the operation completes.
		backupCtx, backupCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer backupCancel()
		backup, daemonErr := cfg.Daemon.CreateBackup(backupCtx, target.NodeURL, target.NodeToken, target.ServerID, req.IgnoredFiles)
		if daemonErr != nil {
			// Mark the pending record as failed so it does not hang indefinitely.
			now := time.Now().UTC()
			_, _ = cfg.Store.UpsertBackup(ctx, target.ServerID, store.UpsertBackupRequest{
				UUID:        stored.UUID,
				Name:        stored.Name,
				Status:      "failed",
				CompletedAt: &now,
			}, actorID)
			return fiber.NewError(fiber.StatusBadGateway, daemonErr.Error())
		}
		completedAt := time.Now().UTC()
		if backup.Completed != "" {
			if parsed, parseErr := time.Parse(time.RFC3339, backup.Completed); parseErr == nil {
				completedAt = parsed
			}
		}
		// Update the pending record with the daemon result.
		updated, updateErr := cfg.Store.UpsertBackup(ctx, target.ServerID, store.UpsertBackupRequest{
			UUID:        backup.UUID,
			Name:        backup.Name,
			Checksum:    backup.Checksum,
			Size:        backup.Size,
			Status:      "completed",
			CompletedAt: &completedAt,
		}, actorID)
		if updateErr != nil {
			// The backup was created on the daemon but we failed to persist.
			// The daemon callback will reconcile this on retry.
			return c.Status(fiber.StatusCreated).JSON(stored)
		}
		return c.Status(fiber.StatusCreated).JSON(updated)
	})

	protected.Get("/servers/:id/backups/download", requireServerPermission(cfg, store.PermBackupDownload), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusNotFound, "backup not found")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if _, err := cfg.Store.GetBackupByName(ctx, target.ServerID, c.Query("name")); err != nil {
			return fiber.NewError(fiber.StatusNotFound, "backup not found")
		}
		downloadCtx, downloadCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer downloadCancel()
		body, err := cfg.Daemon.DownloadBackup(downloadCtx, target.NodeURL, target.NodeToken, target.ServerID, c.Query("name"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		defer body.Close()
		c.Set("Content-Type", "application/zip")
		c.Set("Content-Disposition", `attachment; filename="`+c.Query("name")+`"`)
		return c.SendStream(body)
	})

	protected.Post("/servers/:id/backups/restore", requireServerPermission(cfg, store.PermBackupRestore), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		var body struct {
			Name     string `json:"name"`
			Truncate bool   `json:"truncate"`
		}
		if err := c.BodyParser(&body); err != nil {
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
		if _, err := cfg.Store.GetBackupByName(ctx, target.ServerID, body.Name); err != nil {
			return fiber.NewError(fiber.StatusNotFound, "backup not found")
		}
		_ = cfg.Store.MarkBackupStatus(ctx, target.ServerID, body.Name, "restoring", actorID)
		restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer restoreCancel()
		if err := cfg.Daemon.RestoreBackup(restoreCtx, target.NodeURL, target.NodeToken, target.ServerID, body.Name, body.Truncate); err != nil {
			_ = cfg.Store.MarkBackupStatus(ctx, target.ServerID, body.Name, "restore_failed", actorID)
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		if err := cfg.Store.MarkBackupStatus(ctx, target.ServerID, body.Name, "restored", actorID); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"ok": true, "name": body.Name, "status": "restored"})
	})

	protected.Get("/servers/:id/backups/verify", requireServerPermission(cfg, store.PermBackupRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		name := c.Query("name")
		if name == "" {
			return fiber.NewError(fiber.StatusBadRequest, "backup name is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		b, err := cfg.Store.GetBackupByName(ctx, target.ServerID, name)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "backup not found")
		}
		if b.Status != "completed" {
			return fiber.NewError(fiber.StatusPreconditionFailed, "backup is not in completed state")
		}
		verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer verifyCancel()
		backups, err := cfg.Daemon.ListBackups(verifyCtx, target.NodeURL, target.NodeToken, target.ServerID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, fmt.Sprintf("failed to list backups on daemon: %v", err))
		}
		var found *daemon.BackupEntry
		for i := range backups {
			if backups[i].Name == name {
				found = &backups[i]
				break
			}
		}
		if found == nil {
			return fiber.NewError(fiber.StatusNotFound, "backup not found on daemon")
		}
		verified := found.Status == "completed" && found.Size > 0 && found.Checksum != ""
		checksumMatch := false
		if b.Checksum != "" && found.Checksum != "" {
			checksumMatch = strings.EqualFold(b.Checksum, found.Checksum)
			verified = verified && checksumMatch
		}
		return c.JSON(fiber.Map{
			"ok":               true,
			"name":             name,
			"verified":         verified,
			"checksumMatch":    checksumMatch,
			"dbChecksum":       b.Checksum,
			"daemonChecksum":   found.Checksum,
			"daemonSize":       found.Size,
			"dbSize":           b.Size,
			"daemonStatus":     found.Status,
		})
	})

	protected.Get("/servers/:id/backups/storage/download", requireServerPermission(cfg, store.PermBackupDownload), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.BackupSvc == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and backup service are required")
		}
		name := c.Query("name")
		if name == "" {
			return fiber.NewError(fiber.StatusBadRequest, "backup name is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		b, err := cfg.Store.GetBackupByName(ctx, c.Params("id"), name)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "backup not found")
		}
		if b.Status != "completed" {
			return fiber.NewError(fiber.StatusPreconditionFailed, "backup is not in completed state")
		}
		downloadCtx, downloadCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer downloadCancel()
		storagePath := "backups/" + c.Params("id") + "/" + name
		data, err := cfg.BackupSvc.DownloadFromStorage(downloadCtx, "", storagePath)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		c.Set("Content-Type", "application/octet-stream")
		c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
		c.Set("Content-Length", strconv.Itoa(len(data)))
		return c.Send(data)
	})

	protected.Post("/servers/:id/backups/storage/restore", requireServerPermission(cfg, store.PermBackupRestore), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.BackupSvc == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres, backup service, and daemon are required")
		}
		var body struct {
			Name     string `json:"name"`
			Truncate bool   `json:"truncate"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if body.Name == "" {
			return fiber.NewError(fiber.StatusBadRequest, "backup name is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		serverID := c.Params("id")
		b, err := cfg.Store.GetBackupByName(ctx, serverID, body.Name)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "backup not found")
		}
		if b.Status != "completed" {
			return fiber.NewError(fiber.StatusPreconditionFailed, "backup status is %s, cannot restore", b.Status)
		}
		target, err := cfg.Store.ServerControlTarget(ctx, serverID)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		downloadCtx, downloadCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer downloadCancel()
		storagePath := "backups/" + serverID + "/" + body.Name
		data, err := cfg.BackupSvc.DownloadFromStorage(downloadCtx, "", storagePath)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, fmt.Sprintf("download from storage: %v", err))
		}
		if err := cfg.BackupSvc.VerifyChecksum(data, b.Checksum); err != nil {
			return fiber.NewError(fiber.StatusPreconditionFailed, fmt.Sprintf("checksum verification failed: %v", err))
		}
		_ = cfg.Store.MarkBackupStatus(ctx, serverID, body.Name, "restoring", actorID)
		restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer restoreCancel()
		if err := cfg.Daemon.RestoreBackup(restoreCtx, target.NodeURL, target.NodeToken, target.ServerID, body.Name, body.Truncate); err != nil {
			_ = cfg.Store.MarkBackupStatus(ctx, serverID, body.Name, "restore_failed", actorID)
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		if err := cfg.Store.MarkBackupStatus(ctx, serverID, body.Name, "restored", actorID); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
			"ok":       true,
			"name":     body.Name,
			"status":   "restored",
			"verified": true,
			"size":     len(data),
		})
	})

	protected.Delete("/servers/:id/backups", requireServerPermission(cfg, store.PermBackupDelete), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		name := c.Query("name")
		if name == "" {
			return fiber.NewError(fiber.StatusBadRequest, "backup name is required")
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
		if _, err := cfg.Store.GetBackupByName(ctx, target.ServerID, name); err != nil {
			return fiber.NewError(fiber.StatusNotFound, "backup not found")
		}
		if err := cfg.Daemon.DeleteBackup(ctx, target.NodeURL, target.NodeToken, target.ServerID, name); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		if err := cfg.Store.DeleteBackup(ctx, target.ServerID, name, actorID); err != nil {
			if err.Error() == "backup is locked and cannot be deleted" {
				return fiber.NewError(fiber.StatusForbidden, err.Error())
			}
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "name": name})
	})

	// Backup lock/unlock endpoints
	protected.Post("/servers/:id/backups/:name/lock", mutationLimiter, requireServerPermission(cfg, store.PermBackupDelete), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		name := c.Params("name")
		if name == "" {
			return fiber.NewError(fiber.StatusBadRequest, "backup name is required")
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
		if err := cfg.Store.LockBackup(ctx, target.ServerID, name, actorID); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "name": name, "locked": true})
	})

	protected.Post("/servers/:id/backups/:name/unlock", mutationLimiter, requireServerPermission(cfg, store.PermBackupDelete), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		name := c.Params("name")
		if name == "" {
			return fiber.NewError(fiber.StatusBadRequest, "backup name is required")
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
		if err := cfg.Store.UnlockBackup(ctx, target.ServerID, name, actorID); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "name": name, "locked": false})
	})

	// Backup rename endpoint
	protected.Post("/servers/:id/backups/:name/rename", mutationLimiter, requireServerPermission(cfg, store.PermBackupDelete), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		name := c.Params("name")
		if name == "" {
			return fiber.NewError(fiber.StatusBadRequest, "backup name is required")
		}
		var req struct {
			Name string `json:"name"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.Name == "" {
			return fiber.NewError(fiber.StatusBadRequest, "new name is required")
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
		if err := cfg.Store.RenameBackup(ctx, target.ServerID, name, req.Name, actorID); err != nil {
			if err.Error() == "backup is locked and cannot be renamed" {
				return fiber.NewError(fiber.StatusForbidden, err.Error())
			}
			if err.Error() == "a backup with the new name already exists" {
				return fiber.NewError(fiber.StatusConflict, err.Error())
			}
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "name": req.Name, "previousName": name})
	})

	// Backup cleanup endpoint
	protected.Post("/servers/:id/backups/cleanup", mutationLimiter, requireServerPermission(cfg, store.PermBackupDelete), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()

		// Get panel settings for retention policy
		settings, err := cfg.Store.GetPanelSettings(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to get settings")
		}

		// Get server-specific backup limit
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}

		backupLimit, err := cfg.Store.BackupLimit(ctx, target.ServerID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to get backup limit")
		}

		// Perform cleanup
		deleted, err := cfg.Store.CleanupOldBackupsForServer(ctx, target.ServerID, settings.BackupRetentionDays, backupLimit)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		return c.JSON(fiber.Map{"ok": true, "deleted": deleted})
	})

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

	protected.Get("/servers/:id/files", requireServerPermission(cfg, store.PermFileRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		files, err := cfg.Daemon.ListFiles(ctx, target.NodeURL, target.NodeToken, target.ServerID, c.Query("path"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(files)
	})

	protected.Post("/servers/:id/files/archive", requireServerPermission(cfg, store.PermFileArchive), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		archivePath := c.Query("path")
		if archivePath == "" {
			var req struct {
				Path string `json:"path"`
			}
			if err := c.BodyParser(&req); err == nil && req.Path != "" {
				archivePath = req.Path
			}
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		archiveCtx, archiveCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer archiveCancel()
		body, err := cfg.Daemon.ArchiveFiles(archiveCtx, target.NodeURL, target.NodeToken, target.ServerID, archivePath)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		defer body.Close()
		c.Set("Content-Type", "application/gzip")
		c.Set("Content-Disposition", `attachment; filename="archive.tar.gz"`)
		return c.SendStream(body)
	})

	protected.Post("/servers/:id/files/decompress", requireServerPermission(cfg, store.PermFileArchive), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		var body struct {
			Path string `json:"path"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		decompressCtx, decompressCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer decompressCancel()
		if err := cfg.Daemon.DecompressFile(decompressCtx, target.NodeURL, target.NodeToken, target.ServerID, body.Path); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"ok": true})
	})

	protected.Get("/servers/:id/files/content", requireServerPermission(cfg, store.PermFileReadContent), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		body, err := cfg.Daemon.ReadFile(ctx, target.NodeURL, target.NodeToken, target.ServerID, c.Query("path"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.read", "server", &target.ServerID, safeAuditMeta(map[string]string{"file": c.Query("path")}))
		c.Set("Content-Type", "text/plain; charset=utf-8")
		return c.SendString(body)
	})

	protected.Put("/servers/:id/files/content", requireServerPermission(cfg, store.PermFileUpdate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}

		var fileBytes []byte
		contentType := c.Get("Content-Type")
		if strings.HasPrefix(contentType, "multipart/form-data") {
			form, err := c.MultipartForm()
			if err != nil {
				return fiber.NewError(fiber.StatusBadRequest, "failed to parse multipart form")
			}
			// Look for "file" or "files" or "content" fields
			files := form.File["file"]
			if len(files) == 0 {
				files = form.File["content"]
			}
			if len(files) > 0 {
				fileHeader := files[0]
				file, err := fileHeader.Open()
				if err != nil {
					return fiber.NewError(fiber.StatusBadRequest, "failed to open form file")
				}
				defer file.Close()
				fileBytes, err = io.ReadAll(file)
				if err != nil {
					return fiber.NewError(fiber.StatusInternalServerError, "failed to read form file")
				}
			} else {
				// Check for text values in case they sent text in "content" form field
				if vals := form.Value["content"]; len(vals) > 0 {
					fileBytes = []byte(vals[0])
				} else {
					return fiber.NewError(fiber.StatusBadRequest, "no file or content field found in multipart form")
				}
			}
		} else {
			fileBytes = c.Body()
		}

		if err := cfg.Daemon.WriteFile(ctx, target.NodeURL, target.NodeToken, target.ServerID, c.Query("path"), fileBytes); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.write", "server", &target.ServerID, safeAuditMeta(map[string]string{"file": c.Query("path")}))
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Put("/servers/:id/files/upload", requireServerPermission(cfg, store.PermFileCreate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		offset, err := strconv.ParseInt(c.Query("offset", "0"), 10, 64)
		if err != nil || offset < 0 {
			return fiber.NewError(fiber.StatusBadRequest, "invalid offset")
		}
		uploadID := c.Query("uploadId")
		if uploadID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "uploadId is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		body := c.Context().RequestBodyStream()
		if body == nil {
			body = bytes.NewReader(c.Body())
		} else {
			defer c.Context().Request.CloseBodyStream()
		}
		uploadCtx, uploadCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer uploadCancel()
		if err := cfg.Daemon.UploadFileChunk(uploadCtx, target.NodeURL, target.NodeToken, target.ServerID, c.Query("path"), uploadID, offset, c.Query("final") == "true", body); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		if c.Query("final") == "true" {
			var actorID *string
			if claims, ok := c.Locals("user").(tokenClaims); ok {
				actorID = &claims.Sub
			}
			_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.upload", "server", &target.ServerID, safeAuditMeta(map[string]string{"file": c.Query("path")}))
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Delete("/servers/:id/files", requireServerPermission(cfg, store.PermFileDelete), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if err := cfg.Daemon.DeleteFile(ctx, target.NodeURL, target.NodeToken, target.ServerID, c.Query("path")); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.delete", "server", &target.ServerID, safeAuditMeta(map[string]string{"file": c.Query("path")}))
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/files/mkdir", requireServerPermission(cfg, store.PermFileCreate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		mkdirPath := c.Query("path")
		if mkdirPath == "" {
			var req struct {
				Path string `json:"path"`
			}
			if err := c.BodyParser(&req); err == nil && req.Path != "" {
				mkdirPath = req.Path
			}
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if err := cfg.Daemon.MakeDir(ctx, target.NodeURL, target.NodeToken, target.ServerID, mkdirPath); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.create-directory", "server", &target.ServerID, safeAuditMeta(map[string]string{"directory": mkdirPath}))
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true})
	})

	protected.Patch("/servers/:id/files/rename", requireServerPermission(cfg, store.PermFileUpdate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		var req RenameFileRequest
		if err := c.BodyParser(&req); err != nil && err != io.EOF {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if err := cfg.Daemon.RenameFile(ctx, target.NodeURL, target.NodeToken, target.ServerID, req.From, req.To); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.rename", "server", &target.ServerID, safeAuditMeta(map[string]string{"from": req.From, "to": req.To}))
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/files/delete-batch", mutationLimiter, requireServerPermission(cfg, store.PermFileDelete), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		var req struct {
			Paths []string `json:"paths"`
		}
		if err := c.BodyParser(&req); err != nil || len(req.Paths) == 0 || len(req.Paths) > 100 {
			return fiber.NewError(fiber.StatusBadRequest, "between 1 and 100 paths are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		opCtx, opCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer opCancel()
		if err := cfg.Daemon.DeleteFiles(opCtx, target.NodeURL, target.NodeToken, target.ServerID, req.Paths); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.delete-batch", "server", &target.ServerID, safeAuditMeta(map[string]string{"paths": strings.Join(req.Paths, ",")}))
		return c.JSON(fiber.Map{"ok": true, "deleted": len(req.Paths)})
	})

	protected.Post("/servers/:id/files/rename-batch", mutationLimiter, requireServerPermission(cfg, store.PermFileUpdate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		var req struct {
			Files []struct {
				From string `json:"from"`
				To   string `json:"to"`
			} `json:"files"`
		}
		if err := c.BodyParser(&req); err != nil || len(req.Files) == 0 || len(req.Files) > 100 {
			return fiber.NewError(fiber.StatusBadRequest, "between 1 and 100 files are required")
		}
		files := make([]map[string]string, len(req.Files))
		for index, file := range req.Files {
			if strings.TrimSpace(file.From) == "" || strings.TrimSpace(file.To) == "" {
				return fiber.NewError(fiber.StatusBadRequest, "every rename requires from and to")
			}
			files[index] = map[string]string{"from": file.From, "to": file.To}
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		opCtx, opCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer opCancel()
		if err := cfg.Daemon.RenameFiles(opCtx, target.NodeURL, target.NodeToken, target.ServerID, files); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.rename-batch", "server", &target.ServerID, safeAuditMeta(map[string]string{"count": strconv.Itoa(len(files))}))
		return c.JSON(fiber.Map{"ok": true, "renamed": len(files)})
	})

	protected.Post("/servers/:id/files/copy", mutationLimiter, requireServerPermission(cfg, store.PermFileCreate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		var req struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.From) == "" || strings.TrimSpace(req.To) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "from and to are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		copyCtx, copyCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer copyCancel()
		if err := cfg.Daemon.CopyFile(copyCtx, target.NodeURL, target.NodeToken, target.ServerID, req.From, req.To); err != nil {
			status := fiber.StatusBadGateway
			var daemonErr *daemon.ResponseError
			if errors.As(err, &daemonErr) && daemonErr.StatusCode == fiber.StatusConflict {
				status = fiber.StatusConflict
			}
			return fiber.NewError(status, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.copy", "server", &target.ServerID, safeAuditMeta(map[string]string{"from": req.From, "to": req.To}))
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/files/chmod", mutationLimiter, requireServerPermission(cfg, store.PermFileUpdate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		var req struct {
			Path string `json:"path"`
			Mode string `json:"mode"`
		}
		if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.Path) == "" || !validFileMode(req.Mode) {
			return fiber.NewError(fiber.StatusBadRequest, "path and a three or four digit octal mode are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if err := cfg.Daemon.ChmodFile(ctx, target.NodeURL, target.NodeToken, target.ServerID, req.Path, req.Mode); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.chmod", "server", &target.ServerID, safeAuditMeta(map[string]string{"path": req.Path, "mode": req.Mode}))
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Delete("/servers/:id/files/delete", requireServerPermission(cfg, store.PermFileDelete), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if err := cfg.Daemon.DeleteFile(ctx, target.NodeURL, target.NodeToken, target.ServerID, c.Query("path")); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.delete", "server", &target.ServerID, safeAuditMeta(map[string]string{"file": c.Query("path")}))
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/files/rename", requireServerPermission(cfg, store.PermFileUpdate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		var req RenameFileRequest
		if err := c.BodyParser(&req); err != nil && err != io.EOF {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if err := cfg.Daemon.RenameFile(ctx, target.NodeURL, target.NodeToken, target.ServerID, req.From, req.To); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.rename", "server", &target.ServerID, safeAuditMeta(map[string]string{"from": req.From, "to": req.To}))
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/files/chmod-batch", mutationLimiter, requireServerPermission(cfg, store.PermFileUpdate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		var req struct {
			Files []struct {
				Path string `json:"path"`
				Mode string `json:"mode"`
			} `json:"files"`
		}
		if err := c.BodyParser(&req); err != nil || len(req.Files) == 0 || len(req.Files) > 100 {
			return fiber.NewError(fiber.StatusBadRequest, "between 1 and 100 files are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		for _, f := range req.Files {
			if !validFileMode(f.Mode) {
				return fiber.NewError(fiber.StatusBadRequest, "invalid mode: "+f.Mode)
			}
			if err := cfg.Daemon.ChmodFile(ctx, target.NodeURL, target.NodeToken, target.ServerID, f.Path, f.Mode); err != nil {
				return fiber.NewError(fiber.StatusBadGateway, err.Error())
			}
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.chmod-batch", "server", &target.ServerID, safeAuditMeta(map[string]string{"count": strconv.Itoa(len(req.Files))}))
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/files/create-directory", requireServerPermission(cfg, store.PermFileCreate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		var req struct {
			Path string `json:"path"`
		}
		if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.Path) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "path is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if err := cfg.Daemon.MakeDir(ctx, target.NodeURL, target.NodeToken, target.ServerID, req.Path); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.create-directory", "server", &target.ServerID, safeAuditMeta(map[string]string{"directory": req.Path}))
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/files/pull", mutationLimiter, requireServerPermission(cfg, store.PermFileCreate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		var req struct {
			URL  string `json:"url"`
			Path string `json:"path"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		req.URL = strings.TrimSpace(req.URL)
		if req.URL == "" {
			return fiber.NewError(fiber.StatusBadRequest, "url is required")
		}
		destination := strings.TrimSpace(req.Path)
		targetDirectory, fileName := "", ""
		if destination != "" {
			cleaned := path.Clean(destination)
			if cleaned == "." || cleaned == "/" || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "\\") {
				return fiber.NewError(fiber.StatusBadRequest, "invalid destination path")
			}
			targetDirectory = path.Dir(cleaned)
			if targetDirectory == "." {
				targetDirectory = ""
			}
			fileName = path.Base(cleaned)
		}
		lookupCtx, lookupCancel := requestContext()
		defer lookupCancel()
		target, err := cfg.Store.ServerControlTarget(lookupCtx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		pullCtx, pullCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer pullCancel()
		if err := cfg.Daemon.PullRemoteFile(pullCtx, target.NodeURL, target.NodeToken, target.ServerID, req.URL, targetDirectory, fileName); err != nil {
			status := fiber.StatusBadGateway
			var daemonErr *daemon.ResponseError
			if errors.As(err, &daemonErr) {
				switch daemonErr.StatusCode {
				case fiber.StatusBadRequest, fiber.StatusForbidden, fiber.StatusRequestEntityTooLarge, fiber.StatusInsufficientStorage:
					status = daemonErr.StatusCode
				}
			}
			return fiber.NewError(status, "remote file pull failed: "+err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(lookupCtx, actorID, "server:file.pull", "server", &target.ServerID, safeAuditMeta(map[string]string{"url": req.URL, "path": destination}))
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"ok": true, "path": destination})
	})

	protected.Delete("/servers/:id/backups/:backupName", requireServerPermission(cfg, store.PermBackupDelete), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		name := c.Params("backupName")
		if name == "" {
			return fiber.NewError(fiber.StatusBadRequest, "backup name is required")
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
		if _, err := cfg.Store.GetBackupByName(ctx, target.ServerID, name); err != nil {
			return fiber.NewError(fiber.StatusNotFound, "backup not found")
		}
		if err := cfg.Daemon.DeleteBackup(ctx, target.NodeURL, target.NodeToken, target.ServerID, name); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		if err := cfg.Store.DeleteBackup(ctx, target.ServerID, name, actorID); err != nil {
			if err.Error() == "backup is locked and cannot be deleted" {
				return fiber.NewError(fiber.StatusForbidden, err.Error())
			}
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "name": name})
	})

	protected.Get("/admin/audit", requireRole("admin"), requireAdminScope("audit.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		events, err := cfg.Store.ListAudit(ctx)
		if err != nil {
			return err
		}
		return c.JSON(events)
	})

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

	protected.Get("/audit", requireRole("admin"), requireAdminScope("audit.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		events, err := cfg.Store.ListAudit(ctx)
		if err != nil {
			return err
		}
		return c.JSON(events)
	})

	// ---- PufferPanel-inspired operations pipeline ----

	protected.Post("/servers/:id/files/download", mutationLimiter, requireServerPermission(cfg, store.PermFileCreate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		var req struct {
			URL  string `json:"url"`
			Path string `json:"path"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		req.URL = strings.TrimSpace(req.URL)
		if req.URL == "" {
			return fiber.NewError(fiber.StatusBadRequest, "url is required")
		}
		destination := strings.TrimSpace(req.Path)
		targetDirectory, fileName := "", ""
		if destination != "" {
			cleaned := path.Clean(destination)
			if cleaned == "." || cleaned == "/" || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "\\") {
				return fiber.NewError(fiber.StatusBadRequest, "invalid destination path")
			}
			targetDirectory = path.Dir(cleaned)
			if targetDirectory == "." {
				targetDirectory = ""
			}
			fileName = path.Base(cleaned)
		}
		lookupCtx, lookupCancel := requestContext()
		defer lookupCancel()
		target, err := cfg.Store.ServerControlTarget(lookupCtx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		pullCtx, pullCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer pullCancel()
		if err := cfg.Daemon.PullRemoteFile(pullCtx, target.NodeURL, target.NodeToken, target.ServerID, req.URL, targetDirectory, fileName); err != nil {
			status := fiber.StatusBadGateway
			var daemonErr *daemon.ResponseError
			if errors.As(err, &daemonErr) {
				switch daemonErr.StatusCode {
				case fiber.StatusBadRequest, fiber.StatusForbidden, fiber.StatusRequestEntityTooLarge, fiber.StatusInsufficientStorage:
					status = daemonErr.StatusCode
				}
			}
			return fiber.NewError(status, "remote file pull failed: "+err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(lookupCtx, actorID, "server:file.pull", "server", &target.ServerID, safeAuditMeta(map[string]string{"url": req.URL, "path": destination}))
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"ok": true, "path": destination})
	})

	// ---- API parity endpoints ----

	protected.Get("/servers/:id/status", requireServerAccess(cfg), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		server, err := cfg.Store.GetServer(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		running := server.Status == "running" || server.ActualState == store.ServerActualStateRunning
		installing := server.Status == "installing" || server.Status == "install_failed"
		return c.JSON(fiber.Map{
			"running":    running,
			"installing": installing,
		})
	})

	protected.Patch("/servers/:id/details", mutationLimiter, requireServerPermission(cfg, store.PermSettingsRename), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
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
			Name: req.Name, Description: req.Description,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(server)
	})

	protected.Patch("/servers/:id/build", mutationLimiter, requireRole("admin"), requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			MemoryMB      *int    `json:"memoryMb"`
			CPUShares     *int    `json:"cpuShares"`
			CPULimit      *int    `json:"cpuLimit"`
			DiskMB        *int    `json:"diskMb"`
			DatabaseLimit *int    `json:"databaseLimit"`
			BackupLimit   *int    `json:"backupLimit"`
			IOWeight      *int    `json:"ioWeight"`
			SwapMB        *int    `json:"swapMb"`
			Threads       *string `json:"threads"`
			OOMDisabled   *bool   `json:"oomDisabled"`
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
			MemoryMB: req.MemoryMB, CPUShares: req.CPUShares, CPULimit: req.CPULimit,
			DiskMB: req.DiskMB, DatabaseLimit: req.DatabaseLimit, BackupLimit: req.BackupLimit,
			IOWeight: req.IOWeight, SwapMB: req.SwapMB, Threads: req.Threads, OOMDisabled: req.OOMDisabled,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(server)
	})

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

	protected.Post("/servers/:id/settings/rename", mutationLimiter, requireServerPermission(cfg, store.PermSettingsRename), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Name string `json:"name"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.Name) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "name is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		server, err := cfg.Store.UpdateServer(ctx, c.Params("id"), store.UpdateServerRequest{
			Name: &req.Name,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(server)
	})

	protected.Get("/servers/:id/flags", requireServerAccess(cfg), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		flags, err := cfg.Store.GetServerFlags(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		return c.JSON(flags)
	})

	protected.Post("/servers/:id/flags", mutationLimiter, requireServerPermission(cfg, store.PermSettingsRename), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			AutoStart   *bool `json:"autoStart"`
			AutoRestart *bool `json:"autoRestart"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		current, err := cfg.Store.GetServerFlags(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if req.AutoStart != nil {
			current.AutoStart = *req.AutoStart
		}
		if req.AutoRestart != nil {
			current.AutoRestart = *req.AutoRestart
		}
		if err := cfg.Store.SetServerFlags(ctx, c.Params("id"), current); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(current)
	})

	protected.Post("/servers/:id/operations/run", mutationLimiter, requireServerPermission(cfg, store.PermSettingsReinstall), func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotImplemented, "operation pipelines are disabled until durable execution, per-step authorization, and failure reporting are implemented")
	})
}
