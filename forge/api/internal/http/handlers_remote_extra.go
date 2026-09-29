package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gofiber/fiber/v2"
)

// requireNodeOwnsServer is the shared ownership gate for the node-facing
// /api/remote handlers: it reports whether the authenticated node owns
// serverID, and returns a ready-to-return fiber error when it does not.
//
// It fails closed either way, but it distinguishes the two ways it can fail.
// 403 means the server definitely is not this node's. 500 means we could not
// establish ownership at all. Every /api/remote handler used to open with its
// own copy of this check: most collapsed both cases into 403 — which during a
// database outage reads as "this node's credentials are wrong" and sends the
// operator to the wrong place — and a few echoed the raw database error back to
// the node, bypassing respondInternalError's production redaction.
//
// denyMsg is the 403 body because some callers reached the server through a
// backup and phrase the denial in those terms.
func requireNodeOwnsServer(ctx context.Context, c *fiber.Ctx, st *store.Store, serverID, nodeID, denyMsg string) error {
	if st == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
	}
	belongs, err := st.ServerBelongsToNode(ctx, serverID, nodeID)
	switch {
	case err != nil:
		return respondInternalError(c, fmt.Errorf("verify node %s owns server %s: %w", nodeID, serverID, err))
	case !belongs:
		return fiber.NewError(fiber.StatusForbidden, denyMsg)
	}
	return nil
}

// nodeCannotAccessServer is the denial message shared by the handlers that
// address a server directly.
const nodeCannotAccessServer = "requesting node cannot access this server"

// nodeDoesNotOwnBackup is the denial message for handlers that reach a server
// through one of its backups.
const nodeDoesNotOwnBackup = "backup does not belong to this node"

// Remote extras: additional /api/remote/* endpoints for daemon parity.
//
// These are endpoints the daemon calls that we either had under a different
// shape or were missing entirely. Each handler mirrors the contract
// documented in the upstream `api-remote.php`.

// appendAuditForNode appends a single audit event attributed to a remote node.
//
// The audit write is best-effort — a node's report is not rejected because the
// audit trail could not be extended — but the failure is logged rather than
// discarded, so a silently empty audit trail is not mistaken for an absence of
// node activity.
func appendAuditForNode(c *fiber.Ctx, cfg Config, node store.Node, action, targetType, targetID, metadata string) {
	serverID := targetID
	var targetPtr *string
	if strings.TrimSpace(targetID) != "" {
		targetPtr = &serverID
	}
	if err := cfg.Store.AppendAudit(c.Context(), &node.ID, action, targetType, targetPtr, metadata); err != nil {
		logger := cfg.Logger
		if logger == nil {
			logger = slog.Default()
		}
		logger.Error("audit append failed",
			"action", action, "targetType", targetType, "targetId", targetID, "nodeId", node.ID, "error", err)
	}
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
				// Fail closed: a node must only forge audit entries for
				// servers it owns. An unknown server or a DB error denies
				// rather than auditing a forged entry.
				belongs, err := cfg.Store.ServerBelongsToNode(ctx, serverID, node.ID)
				if err != nil || !belongs {
					return fiber.NewError(fiber.StatusForbidden, "requesting node cannot access this server")
				}
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
		if err := requireNodeOwnsServer(ctx, c, cfg.Store, backup.ServerID, node.ID, nodeDoesNotOwnBackup); err != nil {
			return err
		}

		// Get panel settings to check if S3 is enabled
		settings, err := cfg.Store.GetPanelSettings(ctx)
		if err != nil {
			// Default to local if settings unavailable
			settings = store.DefaultPanelSettings()
		}

		expiresAt := time.Now().Add(15 * time.Minute)
		response := fiber.Map{
			"object":     backupUUID,
			"expires_at": expiresAt.UTC().Format(time.RFC3339),
		}

		if settings.S3BackupEnabled && settings.S3Bucket != "" {
			// S3 credentials must never leave the panel: Beacon receives a
			// short-lived, single-backup presigned PUT URL, never raw access
			// keys. The panel signs on behalf of the node with the
			// panel-held S3 credentials; Beacon PUTs the backup bytes to
			// the returned URL. Fail closed when the URL cannot be signed:
			// a missing URL must never degrade into key disclosure or a
			// dead upload target that Beacon treats as success.
			s3Object := strings.Trim(strings.Trim(settings.S3Prefix, "/")+"/"+backupUUID, "/")
			presignedURL, err := presignBackupUploadURL(ctx, settings, settings.S3Bucket, s3Object, time.Until(expiresAt))
			if err != nil {
				return respondInternalError(c, fmt.Errorf("sign backup upload URL for backup %s: %w", backupUUID, err))
			}
			response["url"] = presignedURL
			response["storage"] = "s3"
			response["s3_object"] = s3Object
			response["s3_bucket"] = settings.S3Bucket
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
		if err := requireNodeOwnsServer(ctx, c, cfg.Store, backup.ServerID, node.ID, nodeDoesNotOwnBackup); err != nil {
			return err
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
			// Not necessarily the caller's fault: respondStoreError keeps the
			// recognisable client-side conditions as 4xx and reports the rest
			// as a redacted 500 instead of blaming the node for an outage.
			return respondStoreError(c, err)
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
		if err := requireNodeOwnsServer(ctx, c, cfg.Store, backup.ServerID, node.ID, nodeDoesNotOwnBackup); err != nil {
			return err
		}
		actorID := node.ID
		status := "restored"
		if !body.Successful {
			status = "restore_failed"
		}
		if err := cfg.Store.MarkBackupStatus(ctx, backup.ServerID, backup.Name, status, &actorID); err != nil {
			// Not necessarily the caller's fault: respondStoreError keeps the
			// recognisable client-side conditions as 4xx and reports the rest
			// as a redacted 500 instead of blaming the node for an outage.
			return respondStoreError(c, err)
		}
		_ = cfg.Store.AppendAudit(ctx, &node.ID, "server.backup.restore", "server", &backup.ServerID, body.Error)
		return c.SendStatus(fiber.StatusNoContent)
	})

	// POST /api/remote/servers/:id/backups/restore-status
	// Beacon reports the result of a restore operation against the server
	// scoped route (contract: beacon/internal/remote/client.go SendRestoreStatus).
	remote.Post("/servers/:id/backups/restore-status", func(c *fiber.Ctx) error {
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var body struct {
			BackupUUID string `json:"backup_uuid"`
			ServerUUID string `json:"server_uuid"`
			Successful bool   `json:"successful"`
			Error      string `json:"error"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		serverID := strings.TrimSpace(c.Params("id"))
		if serverID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "server id required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := requireNodeOwnsServer(ctx, c, cfg.Store, serverID, node.ID, nodeDoesNotOwnBackup); err != nil {
			return err
		}
		backup, err := cfg.Store.GetBackupByUUID(ctx, strings.TrimSpace(body.BackupUUID))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "backup not found")
		}
		if backup.ServerID != serverID {
			return fiber.NewError(fiber.StatusForbidden, "backup does not belong to this server")
		}
		actorID := node.ID
		status := "restored"
		if !body.Successful {
			status = "restore_failed"
		}
		if err := cfg.Store.MarkBackupStatus(ctx, serverID, backup.Name, status, &actorID); err != nil {
			// Not necessarily the caller's fault: respondStoreError keeps the
			// recognisable client-side conditions as 4xx and reports the rest
			// as a redacted 500 instead of blaming the node for an outage.
			return respondStoreError(c, err)
		}
		_ = cfg.Store.AppendAudit(ctx, &node.ID, "server.backup.restore", "server", &serverID, body.Error)
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
		if err := requireNodeOwnsServer(ctx, c, cfg.Store, c.Params("id"), node.ID, nodeCannotAccessServer); err != nil {
			return err
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
		if err := requireNodeOwnsServer(ctx, c, cfg.Store, c.Params("id"), node.ID, nodeCannotAccessServer); err != nil {
			return err
		}
		state, err := cfg.Store.GetServerTransferState(ctx, c.Params("id"))
		switch {
		case errors.Is(err, store.ErrServerNotFound):
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		case err != nil:
			// Previously this returned 200 with transferring:false. A node
			// that asks whether a server is transferring and is told "no"
			// may act on it; answering "no" when the real answer is unknown
			// is reporting success for work not performed.
			return fiber.NewError(fiber.StatusInternalServerError, "could not determine transfer state")
		}
		return c.JSON(fiber.Map{"state": state, "transferring": state == "queued" || state == "in_progress"})
	})
}

// presignBackupUploadURL signs a short-lived S3 PUT URL for a single backup
// object using the panel-held S3 credentials. The credentials never leave the
// panel: only the resulting URL is returned to Beacon. A signing failure is
// an error, never a fallback to raw keys or to an unsigned URL.
func presignBackupUploadURL(ctx context.Context, settings store.PanelSettings, bucket, key string, ttl time.Duration) (string, error) {
	if strings.TrimSpace(bucket) == "" || strings.TrimSpace(key) == "" {
		return "", errors.New("s3 bucket and object key are required")
	}
	if strings.TrimSpace(settings.S3AccessKeyID) == "" || strings.TrimSpace(settings.S3SecretAccessKey) == "" {
		return "", errors.New("s3 credentials are not configured")
	}
	if ttl <= 0 || ttl > 15*time.Minute {
		ttl = 15 * time.Minute
	}
	region := strings.TrimSpace(settings.S3Region)
	if region == "" {
		region = "us-east-1"
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			settings.S3AccessKeyID, settings.S3SecretAccessKey, "",
		)),
	)
	if err != nil {
		return "", fmt.Errorf("load aws config: %w", err)
	}
	s3Opts := []func(*s3.Options){
		func(o *s3.Options) {
			o.UsePathStyle = settings.S3UsePathStyle
		},
	}
	if endpoint := strings.TrimSpace(settings.S3Endpoint); endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	}
	presigner := s3.NewPresignClient(s3.NewFromConfig(awsCfg, s3Opts...))
	out, err := presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, func(o *s3.PresignOptions) {
		o.Expires = ttl
	})
	if err != nil {
		return "", fmt.Errorf("presign put object: %w", err)
	}
	if strings.TrimSpace(out.URL) == "" {
		return "", errors.New("presigned upload URL is empty")
	}
	return out.URL, nil
}
