package http

import (
	"strings"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

type ProvisionDBServiceRequest struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Version   string `json:"version"`
	MemoryMB  int    `json:"memoryMb"`
	CPUShares int    `json:"cpuShares"`
}

type CreateServiceTemplateRequest struct {
	Type        string `json:"type"`
	Version     string `json:"version"`
	DockerImage string `json:"dockerImage"`
	DefaultPort int    `json:"defaultPort"`
	DefaultDB   string `json:"defaultDatabase"`
	MinMemoryMB int    `json:"minMemoryMb"`
}

type TestConnectionRequest struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Engine   string `json:"engine"`
	Username string `json:"username"`
	Password string `json:"password"`
	DBName   string `json:"databaseName"`
}

type CreateServiceCredentialRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	Database    string `json:"database"`
	Permissions string `json:"permissions"`
}

func registerDatabaseServiceRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	ds := cfg.DatabaseServiceProvisioner

	protected.Post("/admin/database-services", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		if ds == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "database service provisioner is not available")
		}
		var req ProvisionDBServiceRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		req.Type = strings.TrimSpace(req.Type)
		req.Version = strings.TrimSpace(req.Version)
		if req.Type == "" || req.Version == "" {
			return fiber.NewError(fiber.StatusBadRequest, "engine and version are required")
		}
		if req.MemoryMB < 0 {
			return fiber.NewError(fiber.StatusBadRequest, "memory must not be negative")
		}
		if req.MemoryMB == 0 {
			req.MemoryMB = 256
		}
		ctx, cancel := requestContext()
		defer cancel()
		svc, err := ds.ProvisionService(ctx, req.Name, req.Type, req.Version, req.MemoryMB, req.CPUShares)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(svc)
	})

	protected.Get("/admin/database-services", requireRole("admin"), requireAdminScope("databases.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		svcs, err := cfg.Store.ListDatabaseServices(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(svcs)
	})

	protected.Get("/admin/database-services/:id", requireRole("admin"), requireAdminScope("databases.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		svc, err := cfg.Store.GetDatabaseService(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.JSON(svc)
	})

	protected.Delete("/admin/database-services/:id", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if ds == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "database service provisioner is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := ds.DeleteService(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/admin/database-services/:id/restart", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if ds == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "database service provisioner is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := ds.StopService(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if err := ds.StartService(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/admin/database-services/:id/backups", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if ds == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "database service provisioner is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		backup, err := ds.CreateBackup(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(backup)
	})

	protected.Get("/admin/database-services/:id/backups", requireRole("admin"), requireAdminScope("databases.read"), func(c *fiber.Ctx) error {
		if ds == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "database service provisioner is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		backups, err := ds.ListBackups(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(backups)
	})

	protected.Post("/admin/database-services/:id/backups/:backupId/restore", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if ds == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "database service provisioner is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := ds.RestoreBackup(ctx, c.Params("id"), c.Params("backupId")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Get("/admin/database-services/:id/logs", requireRole("admin"), requireAdminScope("databases.read"), func(c *fiber.Ctx) error {
		if ds == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "database service provisioner is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		logs, err := ds.GetServiceLogs(ctx, c.Params("id"), 50)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"logs": logs})
	})

	protected.Post("/admin/database-services/:id/credentials", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil || ds == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and provisioner are required")
		}
		var req CreateServiceCredentialRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.Username == "" || req.Password == "" {
			return fiber.NewError(fiber.StatusBadRequest, "username and password are required")
		}
		perms := req.Permissions
		if perms == "" {
			perms = "read-write"
		}
		ctx, cancel := requestContext()
		defer cancel()
		cred, err := ds.CreateUser(ctx, c.Params("id"), req.Username, req.Password, perms)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if req.Database != "" {
			_ = ds.GrantPermissions(ctx, c.Params("id"), req.Username, req.Database, perms)
		}
		return c.Status(fiber.StatusCreated).JSON(cred)
	})

	protected.Get("/admin/database-services/:id/credentials", requireRole("admin"), requireAdminScope("databases.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		creds, err := cfg.Store.ListServiceCredentials(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(creds)
	})

	protected.Delete("/admin/database-services/:id/credentials/:credId", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.RevokeServiceCredential(ctx, c.Params("credId")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/admin/database-services/test-connection", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if ds == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "database service provisioner is not available")
		}
		var req TestConnectionRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.Host == "" || req.Engine == "" || req.Username == "" {
			return fiber.NewError(fiber.StatusBadRequest, "host, engine, and username are required")
		}
		if req.Port == 0 {
			req.Port = defaultPortForEngine(req.Engine)
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := ds.TestConnection(ctx, req.Host, req.Port, req.Engine, req.Username, req.Password, req.DBName); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "message": "Connection successful"})
	})

	protected.Get("/admin/database-service-templates", requireRole("admin"), requireAdminScope("databases.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		templates, err := cfg.Store.ListServiceTemplates(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(templates)
	})

	protected.Post("/admin/database-service-templates", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req CreateServiceTemplateRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.Type == "" || req.Version == "" || req.DockerImage == "" {
			return fiber.NewError(fiber.StatusBadRequest, "type, version, and dockerImage are required")
		}
		if req.DefaultPort == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "defaultPort is required")
		}
		if req.MinMemoryMB == 0 {
			req.MinMemoryMB = 256
		}
		ctx, cancel := requestContext()
		defer cancel()
		tpl, err := cfg.Store.CreateServiceTemplate(ctx, store.ServiceTemplate{
			Type:        req.Type,
			Version:     req.Version,
			DockerImage: req.DockerImage,
			DefaultPort: req.DefaultPort,
			DefaultDB:   req.DefaultDB,
			MinMemoryMB: req.MinMemoryMB,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(tpl)
	})

	protected.Put("/admin/database-service-templates", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req CreateServiceTemplateRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.Type == "" || req.Version == "" || req.DockerImage == "" {
			return fiber.NewError(fiber.StatusBadRequest, "type, version, and dockerImage are required")
		}
		if req.DefaultPort == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "defaultPort is required")
		}
		if req.MinMemoryMB == 0 {
			req.MinMemoryMB = 256
		}
		ctx, cancel := requestContext()
		defer cancel()
		tpl, err := cfg.Store.CreateServiceTemplate(ctx, store.ServiceTemplate{
			Type:        req.Type,
			Version:     req.Version,
			DockerImage: req.DockerImage,
			DefaultPort: req.DefaultPort,
			DefaultDB:   req.DefaultDB,
			MinMemoryMB: req.MinMemoryMB,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(tpl)
	})

	protected.Put("/servers/:id/database-service", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var body struct {
			ServiceID string `json:"serviceId"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if body.ServiceID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "serviceId is required")
		}
		serverID := c.Params("id")
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.UpdateDatabaseServiceServerID(ctx, body.ServiceID, &serverID); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Delete("/servers/:id/database-service", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var body struct {
			ServiceID string `json:"serviceId"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if body.ServiceID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "serviceId is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.UpdateDatabaseServiceServerID(ctx, body.ServiceID, nil); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Get("/servers/:id/database-services", requireServerPermission(cfg, store.PermDatabaseRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		svcs, err := cfg.Store.ListDatabaseServicesByServer(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(svcs)
	})
}

func defaultPortForEngine(engine string) int {
	ports := map[string]int{
		"postgresql": 5432,
		"mysql":      3306,
		"mariadb":    3306,
		"redis":      6379,
		"mongodb":    27017,
	}
	if p, ok := ports[strings.ToLower(engine)]; ok {
		return p
	}
	return 0
}
