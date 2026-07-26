package http

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/gofiber/fiber/v2"

	"gamepanel/forge/internal/services/backup"
	"gamepanel/forge/internal/store"
)

func hasServerAccess(cfg Config, c *fiber.Ctx) error {
	if cfg.Store == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
	}
	claims, ok := c.Locals("user").(tokenClaims)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "missing session")
	}
	ctx, cancel := requestContext()
	defer cancel()
	allowed, err := cfg.Store.UserCanAccessServer(ctx, c.Params("id"), claims.Sub, claims.Role, "")
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to check server access")
	}
	if !allowed {
		return fiber.NewError(fiber.StatusForbidden, "server access denied")
	}
	return nil
}

func registerBackupRoutes(protected fiber.Router, cfg Config, svc *backup.Service, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	protected.Get("/servers/:id/backups/policies", requireRole("admin"), requireAdminScope("backups.read"), func(c *fiber.Ctx) error {
		if err := hasServerAccess(cfg, c); err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		policies, err := svc.ListPolicies(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		// svc.ListPolicies does not currently support store-level pagination,
		// so limit/offset are applied here as a defense against unbounded
		// response sizes. A deeper fix would push limit/offset down into the
		// backup service/store layer (see other paginated endpoints such as
		// handlers_servers.go's backup list, which already does this).
		limit := c.QueryInt("limit", 50)
		if limit <= 0 {
			limit = 50
		}
		if limit > 200 {
			limit = 200
		}
		offset := c.QueryInt("offset", 0)
		if offset < 0 {
			offset = 0
		}
		total := len(policies)
		start := offset
		if start > total {
			start = total
		}
		end := start + limit
		if end > total {
			end = total
		}
		return c.JSON(fiber.Map{"policies": policies[start:end], "total": total, "limit": limit, "offset": offset})
	})

	protected.Post("/servers/:id/backups/policies", mutationLimiter, requireRole("admin"), requireAdminScope("backups.write"), func(c *fiber.Ctx) error {
		if err := hasServerAccess(cfg, c); err != nil {
			return err
		}
		var body struct {
			Interval            string `json:"interval"`
			MaxBackups          int    `json:"maxBackups"`
			RetentionDays       int    `json:"retentionDays"`
			Storage             string `json:"storage"`
			Compress            bool   `json:"compress"`
			Encrypted           bool   `json:"encrypted"`
			EncryptionAlgorithm string `json:"encryptionAlgorithm"`
			EncryptionKey       string `json:"encryptionKey"`
			Enabled             bool   `json:"enabled"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		policy := &store.BackupPolicy{
			ServerID:            c.Params("id"),
			Interval:            body.Interval,
			MaxBackups:          body.MaxBackups,
			RetentionDays:       body.RetentionDays,
			Storage:             body.Storage,
			Compress:            body.Compress,
			Encrypted:           body.Encrypted,
			EncryptionAlgorithm: body.EncryptionAlgorithm,
			EncryptionKey:       body.EncryptionKey,
			Enabled:             body.Enabled,
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.CreatePolicy(ctx, policy); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"policy": policy})
	})

	protected.Delete("/servers/:id/backups/policies/:policyId", mutationLimiter, requireRole("admin"), requireAdminScope("backups.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.DeletePolicy(ctx, c.Params("policyId")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/backups/policies/:policyId/lock", mutationLimiter, requireRole("admin"), requireAdminScope("backups.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.LockPolicy(ctx, c.Params("policyId")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "locked": true})
	})

	protected.Post("/servers/:id/backups/policies/:policyId/unlock", mutationLimiter, requireRole("admin"), requireAdminScope("backups.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.UnlockPolicy(ctx, c.Params("policyId")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "locked": false})
	})

	protected.Post("/servers/:id/backups/cleanup", mutationLimiter, requireRole("admin"), requireAdminScope("backups.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		count, err := svc.CleanupExpiredBackups(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "cleaned": count})
	})

	protected.Post("/servers/:id/backups", mutationLimiter, requireRole("admin"), requireAdminScope("backups.write"), func(c *fiber.Ctx) error {
		if err := hasServerAccess(cfg, c); err != nil {
			return err
		}
		var body struct {
			Name    string   `json:"name"`
			Storage string   `json:"storage"`
			Ignored []string `json:"ignored"`
		}
		_ = c.BodyParser(&body)
		if body.Name == "" {
			body.Name = "backup-" + time.Now().Format("20060102-150405")
		}
		ctx, cancel := requestContext()
		defer cancel()
		b, err := svc.CreateBackup(ctx, c.Params("id"), backup.CreateBackupRequest{
			Name:    body.Name,
			Storage: body.Storage,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"uuid":   b.ID,
			"name":   b.Name,
			"status": string(b.Status),
		})
	})

	protected.Get("/backup/providers", requireRole("admin"), func(c *fiber.Ctx) error {
		providers := backup.RegisteredProviders()
		return c.JSON(fiber.Map{"providers": providers})
	})

	adminBackups := protected.Group("/admin/backups", requireRole("admin"))
	adminSvc := backup.NewMainService(cfg.Store, backup.NewSlogLogger(nil))
	adminSvc.SetDaemonClient(cfg.Daemon)
	providerInitErr := adminSvc.LoadStorageProviders(context.Background())
	userID := func(c *fiber.Ctx) (string, error) {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok || claims.Sub == "" {
			return "", fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		return claims.Sub, nil
	}

	adminBackups.Get("/configs", func(c *fiber.Ctx) error {
		configs, _, err := adminSvc.ListBackupConfigs(c.UserContext(), backup.BackupConfigFilter{
			Page: c.QueryInt("page", 1), PerPage: c.QueryInt("perPage", 200),
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(configs)
	})
	adminBackups.Post("/configs", mutationLimiter, func(c *fiber.Ctx) error {
		var request backup.CreateBackupConfigRequest
		if err := c.BodyParser(&request); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		actor, err := userID(c)
		if err != nil {
			return err
		}
		config, err := adminSvc.CreateBackupConfig(c.UserContext(), request, actor)
		if err != nil {
			return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(config)
	})
	adminBackups.Get("/configs/:id", func(c *fiber.Ctx) error {
		config, err := adminSvc.GetBackupConfig(c.UserContext(), c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "backup configuration not found")
		}
		return c.JSON(config)
	})
	adminBackups.Patch("/configs/:id", mutationLimiter, func(c *fiber.Ctx) error {
		var request backup.UpdateBackupConfigRequest
		if err := c.BodyParser(&request); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		actor, err := userID(c)
		if err != nil {
			return err
		}
		config, err := adminSvc.UpdateBackupConfig(c.UserContext(), c.Params("id"), request, actor)
		if err != nil {
			return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
		}
		return c.JSON(config)
	})
	adminBackups.Delete("/configs/:id", mutationLimiter, func(c *fiber.Ctx) error {
		actor, err := userID(c)
		if err != nil {
			return err
		}
		if err := adminSvc.DeleteBackupConfig(c.UserContext(), c.Params("id"), actor); err != nil {
			return fiber.NewError(fiber.StatusConflict, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
	adminBackups.Post("/configs/:id/execute", mutationLimiter, func(c *fiber.Ctx) error {
		actor, err := userID(c)
		if err != nil {
			return err
		}
		job, err := adminSvc.ExecuteBackupConfig(c.UserContext(), c.Params("id"), actor)
		if err != nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
		}
		return c.Status(fiber.StatusAccepted).JSON(job)
	})

	adminBackups.Get("/jobs", func(c *fiber.Ctx) error {
		jobs, _, err := adminSvc.ListBackupJobs(c.UserContext(), backup.JobFilter{
			Page: c.QueryInt("page", 1), PerPage: c.QueryInt("perPage", 200),
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(jobs)
	})
	adminBackups.Post("/jobs", mutationLimiter, func(c *fiber.Ctx) error {
		var request backup.CreateBackupJobRequest
		if err := c.BodyParser(&request); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		actor, err := userID(c)
		if err != nil {
			return err
		}
		job, err := adminSvc.CreateBackupJob(c.UserContext(), request, actor)
		if err != nil {
			return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
		}
		if err := adminSvc.ExecuteBackupJob(c.UserContext(), job.ID, actor); err != nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, fmt.Sprintf("backup job %s failed: %v", job.ID, err))
		}
		completed, err := adminSvc.GetBackupJob(c.UserContext(), job.ID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(completed)
	})
	adminBackups.Post("/jobs/:id/cancel", mutationLimiter, func(c *fiber.Ctx) error {
		actor, err := userID(c)
		if err != nil {
			return err
		}
		if err := adminSvc.CancelBackupJob(c.UserContext(), c.Params("id"), actor); err != nil {
			return fiber.NewError(fiber.StatusConflict, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
	adminBackups.Delete("/jobs/:id", mutationLimiter, func(c *fiber.Ctx) error {
		actor, err := userID(c)
		if err != nil {
			return err
		}
		if err := adminSvc.DeleteBackupJob(c.UserContext(), c.Params("id"), actor); err != nil {
			return fiber.NewError(fiber.StatusConflict, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	adminBackups.Get("/artifacts", func(c *fiber.Ctx) error {
		artifacts, _, err := adminSvc.ListBackupArtifacts(c.UserContext(), backup.ArtifactFilter{
			Page: c.QueryInt("page", 1), PerPage: c.QueryInt("perPage", 200),
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(artifacts)
	})
	adminBackups.Delete("/artifacts/:id", mutationLimiter, func(c *fiber.Ctx) error {
		actor, err := userID(c)
		if err != nil {
			return err
		}
		if err := adminSvc.DeleteBackupArtifact(c.UserContext(), c.Params("id"), actor); err != nil {
			return fiber.NewError(fiber.StatusConflict, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
	adminBackups.Post("/artifacts/:id/lock", mutationLimiter, func(c *fiber.Ctx) error {
		var request struct {
			Reason string `json:"reason"`
		}
		if err := c.BodyParser(&request); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if request.Reason == "" {
			request.Reason = "manual lock"
		}
		actor, err := userID(c)
		if err != nil {
			return err
		}
		if err := adminSvc.LockBackupArtifact(c.UserContext(), c.Params("id"), actor, request.Reason); err != nil {
			return fiber.NewError(fiber.StatusConflict, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
	adminBackups.Post("/artifacts/:id/unlock", mutationLimiter, func(c *fiber.Ctx) error {
		actor, err := userID(c)
		if err != nil {
			return err
		}
		if err := adminSvc.UnlockBackupArtifact(c.UserContext(), c.Params("id"), actor); err != nil {
			return fiber.NewError(fiber.StatusConflict, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
	adminBackups.Get("/artifacts/:id/download", func(c *fiber.Ctx) error {
		artifact, err := adminSvc.GetBackupArtifact(c.UserContext(), c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "backup artifact not found")
		}
		reader, err := adminSvc.DownloadBackupArtifact(c.UserContext(), c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
		}
		c.Set(fiber.HeaderContentDisposition, fmt.Sprintf(`attachment; filename=%q`, filepath.Base(artifact.Name)))
		return c.SendStream(reader)
	})

	adminBackups.Get("/restores", func(c *fiber.Ctx) error {
		restores, _, err := adminSvc.ListRestores(c.UserContext(), backup.RestoreFilter{
			Page: c.QueryInt("page", 1), PerPage: c.QueryInt("perPage", 200),
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(restores)
	})
	adminBackups.Post("/restore", mutationLimiter, func(c *fiber.Ctx) error {
		var request backup.CreateRestoreRequest
		if err := c.BodyParser(&request); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		actor, err := userID(c)
		if err != nil {
			return err
		}
		restore, err := adminSvc.CreateRestore(c.UserContext(), request, actor)
		if err != nil {
			return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
		}
		if err := adminSvc.ExecuteRestore(c.UserContext(), restore.ID, actor); err != nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, fmt.Sprintf("restore %s failed: %v", restore.ID, err))
		}
		completed, err := adminSvc.GetRestore(c.UserContext(), restore.ID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(completed)
	})
	adminBackups.Delete("/restores/:id", mutationLimiter, func(c *fiber.Ctx) error {
		actor, err := userID(c)
		if err != nil {
			return err
		}
		if err := adminSvc.DeleteRestore(c.UserContext(), c.Params("id"), actor); err != nil {
			return fiber.NewError(fiber.StatusConflict, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	adminBackups.Get("/storage-providers", func(c *fiber.Ctx) error {
		if providerInitErr != nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, providerInitErr.Error())
		}
		providers, err := cfg.Store.ListBackupStorageProviders(c.UserContext())
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if len(providers) == 0 {
			for _, name := range backup.RegisteredProviders() {
				providers = append(providers, store.BackupStorageProviderRecord{
					ID: name, Name: name, Type: name, Enabled: true, IsDefault: name == "local",
				})
			}
		}
		return c.JSON(providers)
	})

	adminBackups.Get("/status", func(c *fiber.Ctx) error {
		configs, _, configErr := adminSvc.ListBackupConfigs(c.UserContext(), backup.BackupConfigFilter{PerPage: 200})
		jobs, _, jobErr := adminSvc.ListBackupJobs(c.UserContext(), backup.JobFilter{PerPage: 200})
		artifacts, _, artifactErr := adminSvc.ListBackupArtifacts(c.UserContext(), backup.ArtifactFilter{PerPage: 200})
		restores, _, restoreErr := adminSvc.ListRestores(c.UserContext(), backup.RestoreFilter{PerPage: 200})
		if err := firstBackupError(configErr, jobErr, artifactErr, restoreErr); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		status := backupStatusResponse(configs, jobs, artifacts, restores)
		return c.JSON(status)
	})
}

func firstBackupError(errors ...error) error {
	for _, err := range errors {
		if err != nil {
			return err
		}
	}
	return nil
}

func backupStatusResponse(configs []*backup.BackupConfig, jobs []*backup.BackupJob, artifacts []*backup.BackupArtifact, restores []*backup.BackupRestore) fiber.Map {
	status := fiber.Map{
		"backupConfigurations": fiber.Map{"total": len(configs), "scheduled": 0, "enabled": 0},
		"backupJobs":           fiber.Map{"total": len(jobs), "running": 0, "pending": 0, "failed": 0},
		"backupArtifacts":      fiber.Map{"total": len(artifacts), "verified": 0, "locked": 0, "expired": 0},
		"backupRestores":       fiber.Map{"total": len(restores), "completed": 0, "failed": 0},
	}
	for _, config := range configs {
		if config.IsScheduled {
			status["backupConfigurations"].(fiber.Map)["scheduled"] = status["backupConfigurations"].(fiber.Map)["scheduled"].(int) + 1
		}
		if config.Enabled {
			status["backupConfigurations"].(fiber.Map)["enabled"] = status["backupConfigurations"].(fiber.Map)["enabled"].(int) + 1
		}
	}
	for _, job := range jobs {
		key := string(job.Status)
		if _, ok := status["backupJobs"].(fiber.Map)[key]; ok {
			status["backupJobs"].(fiber.Map)[key] = status["backupJobs"].(fiber.Map)[key].(int) + 1
		}
	}
	now := time.Now()
	for _, artifact := range artifacts {
		if artifact.IsVerified {
			status["backupArtifacts"].(fiber.Map)["verified"] = status["backupArtifacts"].(fiber.Map)["verified"].(int) + 1
		}
		if artifact.IsLocked {
			status["backupArtifacts"].(fiber.Map)["locked"] = status["backupArtifacts"].(fiber.Map)["locked"].(int) + 1
		}
		if artifact.ExpiresAt != nil && artifact.ExpiresAt.Before(now) {
			status["backupArtifacts"].(fiber.Map)["expired"] = status["backupArtifacts"].(fiber.Map)["expired"].(int) + 1
		}
	}
	for _, restore := range restores {
		if _, ok := status["backupRestores"].(fiber.Map)[restore.Status]; ok {
			status["backupRestores"].(fiber.Map)[restore.Status] = status["backupRestores"].(fiber.Map)[restore.Status].(int) + 1
		}
	}
	return status
}
