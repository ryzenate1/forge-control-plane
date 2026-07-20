package http

import (
	"encoding/json"
	"strings"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

type ProvisionDBContainerRequest struct {
	Engine    string `json:"engine"`
	Version   string `json:"version"`
	MemoryMB  int    `json:"memoryMb"`
	CPUShares int    `json:"cpuShares"`
}

func registerDBContainerRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	dbProvisioner := cfg.DBContainerService

	protected.Get("/databases/engines", requireRole("admin"), func(c *fiber.Ctx) error {
		engines := map[string][]string{}
		if dbProvisioner != nil {
			engines = dbProvisioner.SupportedEngines()
		} else {
			engines = store.SupportedDBEngines
		}
		return c.JSON(engines)
	})

	protected.Post("/databases/provision", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		if dbProvisioner == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "container database provisioner is not available")
		}
		var req ProvisionDBContainerRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		req.Engine = strings.TrimSpace(req.Engine)
		req.Version = strings.TrimSpace(req.Version)
		if req.Engine == "" || req.Version == "" {
			return fiber.NewError(fiber.StatusBadRequest, "engine and version are required")
		}
		if req.MemoryMB < 0 {
			return fiber.NewError(fiber.StatusBadRequest, "memory must not be negative")
		}
		if req.MemoryMB == 0 {
			req.MemoryMB = 256
		}
		serverID := c.Query("serverId")
		if serverID == "" {
			serverID = "standalone"
		}
		ctx, cancel := requestContext()
		defer cancel()
		db, err := dbProvisioner.Provision(ctx, serverID, req.Engine, req.Version, req.MemoryMB, req.CPUShares)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(db)
	})

	protected.Get("/databases/containers", requireRole("admin"), requireAdminScope("databases.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		serverID := c.Query("serverId")
		var dbs []store.DBContainer
		var err error
		if serverID != "" {
			dbs, err = cfg.Store.ListDBContainers(ctx, serverID)
		} else {
			dbs, err = cfg.Store.ListAllDBContainers(ctx)
		}
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(dbs)
	})

	protected.Get("/databases/containers/:id", requireRole("admin"), requireAdminScope("databases.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		db, err := cfg.Store.GetDBContainer(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.JSON(db)
	})

	protected.Delete("/databases/containers/:id", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		if dbProvisioner == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "container database provisioner is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := dbProvisioner.Deprovision(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/databases/containers/:id/backup", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if dbProvisioner == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "container database provisioner is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := dbProvisioner.Backup(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/databases/containers/:id/restart", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if dbProvisioner == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "container database provisioner is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := dbProvisioner.Restart(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Get("/databases/containers/:id/credentials", requireRole("admin"), requireAdminScope("databases.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		creds, connStr, err := cfg.Store.GetDBContainerCredentials(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		var credsMap map[string]string
		if creds != nil {
			_ = json.Unmarshal(creds, &credsMap)
		}
		return c.JSON(fiber.Map{
			"credentials":      credsMap,
			"connectionString": connStr,
		})
	})
}
