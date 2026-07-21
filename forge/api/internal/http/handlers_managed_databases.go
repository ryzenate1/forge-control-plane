package http

import (
	"strings"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

type CreateManagedDatabaseRequest struct {
	Name      string `json:"name"`
	Engine    string `json:"engine"`
	Version   string `json:"version"`
	ServerID  string `json:"serverId,omitempty"`
	MemoryMB  int    `json:"memoryMb"`
	CPUShares int    `json:"cpuShares"`
}

type UpdateManagedDatabaseRequest struct {
	Name      *string `json:"name"`
	MemoryMB  *int    `json:"memoryMb"`
	CPUShares *int    `json:"cpuShares"`
	Version   *string `json:"version"`
}

func registerManagedDatabaseRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	dbBackupSvc := cfg.DBBackupService

	// engines static route MUST be registered before :id param route
	protected.Get("/managed-databases/engines", requireRole("admin"), func(c *fiber.Ctx) error {
		engines := store.SupportedDBEngines
		return c.JSON(engines)
	})

	protected.Get("/managed-databases", requireRole("admin"), requireAdminScope("databases.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		dbs, err := cfg.Store.ListManagedDatabases(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(dbs)
	})

	protected.Get("/managed-databases/:id", requireRole("admin"), requireAdminScope("databases.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		db, err := cfg.Store.GetManagedDatabase(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.JSON(db)
	})

	protected.Post("/managed-databases", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req CreateManagedDatabaseRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		req.Engine = strings.TrimSpace(req.Engine)
		req.Version = strings.TrimSpace(req.Version)
		req.Name = strings.TrimSpace(req.Name)
		if req.Engine == "" || req.Version == "" || req.Name == "" {
			return fiber.NewError(fiber.StatusBadRequest, "engine, version, and name are required")
		}
		if req.MemoryMB < 0 {
			return fiber.NewError(fiber.StatusBadRequest, "memory must not be negative")
		}
		if req.MemoryMB == 0 {
			req.MemoryMB = 256
		}
		storeReq := store.CreateManagedDatabaseRequest{
			ServerID:  req.ServerID,
			Name:      req.Name,
			Engine:    req.Engine,
			Version:   req.Version,
			MemoryMB:  req.MemoryMB,
			CPUShares: req.CPUShares,
		}
		ctx, cancel := requestContext()
		defer cancel()
		db, err := cfg.Store.CreateManagedDatabase(ctx, storeReq)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if cfg.DBContainerService != nil {
			go func() {
				pCtx, pCancel := requestContext()
				defer pCancel()
				container, pErr := cfg.DBContainerService.Provision(pCtx, req.ServerID, req.Engine, req.Version, req.MemoryMB, req.CPUShares)
				if pErr != nil {
					_ = cfg.Store.UpdateManagedDatabaseStatus(pCtx, db.ID, store.ManagedDBStatusError)
				} else {
					_ = cfg.Store.SetManagedDatabaseContainerInfo(pCtx, db.ID, container.ContainerID, container.VolumeID, "", container.ConnectionString, container.Port, container.Credentials, store.ManagedDBStatusRunning)
				}
			}()
		}
		return c.Status(fiber.StatusCreated).JSON(db)
	})

	protected.Patch("/managed-databases/:id", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req UpdateManagedDatabaseRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		id := c.Params("id")
		_, err := cfg.Store.GetManagedDatabase(ctx, id)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		name := ""
		memoryMB := 0
		cpuShares := 0
		version := ""
		if req.Name != nil {
			name = *req.Name
		}
		if req.MemoryMB != nil {
			memoryMB = *req.MemoryMB
		}
		if req.CPUShares != nil {
			cpuShares = *req.CPUShares
		}
		if req.Version != nil {
			version = *req.Version
		}
		if err := cfg.Store.UpdateManagedDatabase(ctx, id, name, memoryMB, cpuShares, version); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		db, err := cfg.Store.GetManagedDatabase(ctx, id)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.JSON(db)
	})

	protected.Delete("/managed-databases/:id", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		id := c.Params("id")
		ctx, cancel := requestContext()
		defer cancel()
		_ = cfg.Store.UpdateManagedDatabaseStatus(ctx, id, store.ManagedDBStatusDeleting)
		if err := cfg.Store.DeleteManagedDatabase(ctx, id); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/managed-databases/:id/backup", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if dbBackupSvc == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "database backup service is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		backup, err := dbBackupSvc.Backup(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(backup)
	})

	protected.Post("/managed-databases/:id/restore", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if dbBackupSvc == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "database backup service is not available")
		}
		var req struct {
			BackupID string `json:"backupId"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.BackupID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "backupId is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		restore, err := dbBackupSvc.Restore(ctx, c.Params("id"), req.BackupID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(restore)
	})

	protected.Post("/managed-databases/:id/rotate-password", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if dbBackupSvc == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "database backup service is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		db, err := dbBackupSvc.RotatePassword(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(db)
	})

	protected.Get("/managed-databases/:id/backups", requireRole("admin"), requireAdminScope("databases.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		backups, err := cfg.Store.ListManagedDatabaseBackups(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(backups)
	})

	protected.Get("/managed-databases/:id/restores", requireRole("admin"), requireAdminScope("databases.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		restores, err := cfg.Store.ListManagedDatabaseRestores(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(restores)
	})
}
