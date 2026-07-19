package http

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerBackupsServerRoutes(protected fiber.Router, cfg Config, runner *scheduleRunner, clusterManager *clustermanager.Service, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {

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
			IgnoredFiles []string `json:"ignored_files"`
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

	// Create invitation with 7-day expiration

	// Look for "file" or "files" or "content" fields

	// Check for text values in case they sent text in "content" form field

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
}
