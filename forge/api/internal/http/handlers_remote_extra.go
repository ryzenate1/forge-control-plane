package http

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// Remote extras: additional /api/remote/* endpoints for daemon parity.
//
// These are endpoints the daemon calls that we either had under a different
// shape or were missing entirely. Each handler mirrors the contract
// documented in the upstream `api-remote.php`.

// appendAuditForNode appends a single audit event attributed to a remote node.
func appendAuditForNode(c *fiber.Ctx, cfg Config, node store.Node, action, targetType, targetID, metadata string) {
	serverID := targetID
	var targetPtr *string
	if strings.TrimSpace(targetID) != "" {
		targetPtr = &serverID
	}
	_ = cfg.Store.AppendAudit(c.Context(), &node.ID, action, targetType, targetPtr, metadata)
}

// registerRemoteExtras registers the additional /api/remote/*
// routes on the supplied `remote` group. The group already has
// `remoteNodeMiddleware` applied, so handlers can trust `c.Locals("remoteNode")`.
func registerRemoteExtras(remote fiber.Router, cfg Config) {
	// POST /api/remote/activity
	// Body: {"data": [{"server": "<uuid>", "action": "...", "metadata": "..."}, ...]}
	// Beacon streams a batch of activity entries from the daemon's activity_cron.
	remote.Post("/activity", func(c *fiber.Ctx) error {
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var body struct {
			Data []struct {
				Server   string `json:"server"`
				Action   string `json:"action"`
				Metadata string `json:"metadata"`
			} `json:"data"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		for _, entry := range body.Data {
			action := strings.TrimSpace(entry.Action)
			if action == "" {
				continue
			}
			metadata := entry.Metadata
			serverID := strings.TrimSpace(entry.Server)
			if serverID == "" {
				_ = cfg.Store.AppendAudit(ctx, &node.ID, action, "node", &node.ID, metadata)
			} else {
				_ = cfg.Store.AppendAudit(ctx, &node.ID, action, "server", &serverID, metadata)
			}
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	// GET /api/remote/backups/{backup}
	// Beacon uses this to ask the panel for a presigned S3 upload URL (or local
	// upload target) before streaming a backup. We now support both local and S3.
	remote.Get("/backups/:backup", func(c *fiber.Ctx) error {
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		backupUUID := strings.TrimSpace(c.Params("backup"))
		if backupUUID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "backup id required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		backup, err := cfg.Store.GetBackupByUUID(ctx, backupUUID)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "backup not found")
		}
		if backup.ServerID == "" {
			return fiber.NewError(fiber.StatusNotFound, "backup has no associated server")
		}
		belongs, err := cfg.Store.ServerBelongsToNode(ctx, backup.ServerID, node.ID)
		if err != nil || !belongs {
			return fiber.NewError(fiber.StatusForbidden, "backup does not belong to this node")
		}

		// Get panel settings to check if S3 is enabled
		settings, err := cfg.Store.GetPanelSettings(ctx)
		if err != nil {
			// Default to local if settings unavailable
			settings = store.DefaultPanelSettings()
		}

		uploadToken := generateUploadToken()
		response := fiber.Map{
			"object":     backupUUID,
			"token":      uploadToken,
			"expires_at": time.Now().Add(15 * time.Minute).UTC().Format(time.RFC3339),
		}

		if settings.S3BackupEnabled && settings.S3Bucket != "" {
			// Generate S3 presigned URL for upload
			// This would normally use AWS SDK to generate presigned URL
			// For now, return configuration for the daemon to use
			response["url"] = fmt.Sprintf("s3://%s/%s/%s", settings.S3Bucket, settings.S3Prefix, backupUUID)
			response["storage"] = "s3"
			response["s3_config"] = fiber.Map{
				"endpoint":   settings.S3Endpoint,
				"region":     settings.S3Region,
				"bucket":     settings.S3Bucket,
				"access_key": settings.S3AccessKeyID,
				"prefix":     settings.S3Prefix,
				"path_style": settings.S3UsePathStyle,
			}
		} else {
			// Local upload
			response["url"] = "/api/remote/backups/" + backupUUID + "/upload"
			response["storage"] = "local"
		}

		return c.JSON(response)
	})

	// POST /api/remote/backups/{backup}
	// The daemon reports the completed backup with checksum, size, and S3 parts.
	remote.Post("/backups/:backup", func(c *fiber.Ctx) error {
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var body struct {
			UUID         string `json:"uuid"`
			Checksum     string `json:"checksum"`
			ChecksumType string `json:"checksum_type"`
			Size         int64  `json:"size"`
			Successful   bool   `json:"successful"`
			Parts        []struct {
				PartNumber int    `json:"part_number"`
				ETag       string `json:"etag"`
				Size       int64  `json:"size"`
			} `json:"parts"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		backupUUID := strings.TrimSpace(c.Params("backup"))
		if backupUUID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "backup id required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		backup, err := cfg.Store.GetBackupByUUID(ctx, backupUUID)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "backup not found")
		}
		belongs, err := cfg.Store.ServerBelongsToNode(ctx, backup.ServerID, node.ID)
		if err != nil || !belongs {
			return fiber.NewError(fiber.StatusForbidden, "backup does not belong to this node")
		}
		completedAt := time.Now().UTC()
		actorID := node.ID
		status := "completed"
		if !body.Successful {
			status = "failed"
		}
		_, err = cfg.Store.UpsertBackup(ctx, backup.ServerID, store.UpsertBackupRequest{
			UUID:        backupUUID,
			Name:        backup.Name,
			Checksum:    body.Checksum,
			Size:        body.Size,
			Status:      status,
			CompletedAt: &completedAt,
		}, &actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if cfg.MailTriggerService != nil {
			if srv, e := cfg.Store.GetServer(ctx, backup.ServerID); e == nil {
				sizeStr := fmt.Sprintf("%d bytes", body.Size)
				if body.Successful {
					cfg.MailTriggerService.SendBackupComplete(ctx, srv.Owner, srv.Name, backup.Name, sizeStr)
				} else {
					cfg.MailTriggerService.SendBackupFailed(ctx, srv.Owner, srv.Name, backup.Name, "")
				}
			}
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	// POST /api/remote/backups/{backup}/restore
	// Beacon reports the result of a restore operation.
	remote.Post("/backups/:backup/restore", func(c *fiber.Ctx) error {
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var body struct {
			Successful bool   `json:"successful"`
			Error      string `json:"error"`
		}
		_ = c.BodyParser(&body)
		backupUUID := strings.TrimSpace(c.Params("backup"))
		if backupUUID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "backup id required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		backup, err := cfg.Store.GetBackupByUUID(ctx, backupUUID)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "backup not found")
		}
		belongs, err := cfg.Store.ServerBelongsToNode(ctx, backup.ServerID, node.ID)
		if err != nil || !belongs {
			return fiber.NewError(fiber.StatusForbidden, "backup does not belong to this node")
		}
		actorID := node.ID
		status := "restored"
		if !body.Successful {
			status = "restore_failed"
		}
		if err := cfg.Store.MarkBackupStatus(ctx, backup.ServerID, backup.Name, status, &actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		_ = cfg.Store.AppendAudit(ctx, &node.ID, "server.backup.restore", "server", &backup.ServerID, body.Error)
		return c.SendStatus(fiber.StatusNoContent)
	})

	// POST /api/remote/servers/:id/archive
	// Legacy archive/transfer endpoint.
	// Delegates to the active migration if one exists.
	remote.Post("/servers/:id/archive", func(c *fiber.Ctx) error {
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		belongs, err := cfg.Store.ServerBelongsToNode(ctx, c.Params("id"), node.ID)
		if err != nil || !belongs {
			return fiber.NewError(fiber.StatusForbidden, "requesting node cannot access this server")
		}
		migration, err := cfg.Store.GetActiveMigrationForServer(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "no active migration for server")
		}
		return c.JSON(fiber.Map{"migrationId": migration.ID, "status": migration.Status})
	})

	// GET /api/remote/servers/:id/transfer
	// Legacy transfer status endpoint. Returns the current transfer state.
	remote.Get("/servers/:id/transfer", func(c *fiber.Ctx) error {
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		belongs, err := cfg.Store.ServerBelongsToNode(ctx, c.Params("id"), node.ID)
		if err != nil || !belongs {
			return fiber.NewError(fiber.StatusForbidden, "requesting node cannot access this server")
		}
		state, err := cfg.Store.GetServerTransferState(ctx, c.Params("id"))
		if err != nil {
			return c.JSON(fiber.Map{"state": state, "transferring": false})
		}
		return c.JSON(fiber.Map{"state": state, "transferring": state == "queued" || state == "in_progress"})
	})
}

func generateUploadToken() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
