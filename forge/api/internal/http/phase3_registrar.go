package http

import (
	"log/slog"
	"strings"

	"gamepanel/forge/internal/services/catalog"

	"github.com/gofiber/fiber/v2"
)

const phase3RegistrarName = "phase3-catalog"

func init() {
	RegisterPhaseRegistrar(phase3RegistrarName, 100, registerPhase3CatalogRoutes)
}

// registerPhase3CatalogRoutes wires the one-click service catalog API surface
// (Phase 3): the public catalog listing, admin provisioning, connection-string
// attach, and backup retention endpoints. It also starts the hourly retention
// worker when a background context is available. Store/service absence is
// tolerated: the routes respond 503 instead of crashing startup.
func registerPhase3CatalogRoutes(v1 fiber.Router, protected fiber.Router, cfg *Config) error {
	if cfg.Store == nil {
		if cfg.Logger != nil {
			cfg.Logger.Warn(phase3RegistrarName + ": store not configured, catalog routes skipped")
		}
		return nil
	}
	if cfg.DBContainerService == nil || cfg.ComposeService == nil {
		if cfg.Logger != nil {
			cfg.Logger.Warn(phase3RegistrarName + ": db container or compose service not configured, catalog routes skipped")
		}
		return nil
	}

	svc, err := catalog.New(catalog.Options{
		Store:        cfg.Store,
		DBProvider:   cfg.DBContainerService,
		ComposeStack: cfg.ComposeService,
		EnvSvc:       cfg.EnvVarService,
		Logger:       cfg.Logger,
	})
	if err != nil {
		return err
	}

	catalogRoutes(v1, protected, cfg, svc)

	if cfg.BackgroundContext != nil {
		svc.StartRetentionWorker(cfg.BackgroundContext)
	}
	return nil
}

func catalogRoutes(v1 fiber.Router, protected fiber.Router, cfg *Config, svc *catalog.Service) {
	// Public-ish listing of enabled entries (used by the one-click UI before
	// login completes and by the admin catalog page).
	v1.Get("/catalog", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		entries, err := svc.ListEntries(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": entries})
	})
	v1.Get("/catalog/:key", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		entry, err := svc.GetEntry(ctx, c.Params("key"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "catalog entry not found")
		}
		return c.JSON(fiber.Map{"data": entry})
	})

	mutationLimiter := RateLimiter(GetRateLimitForEndpoint("mutation", cfg.Redis, cfg.RedisEnabled && strings.EqualFold(strings.TrimSpace(cfg.AppEnv), "production")))

	// One-click provisioning (admin).
	// Body: {kind, version, envId, nodeId, resources: {memoryMb, cpuShares, diskMb}}
	protected.Post("/admin/catalog/provision", mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		var req struct {
			Kind      string `json:"kind"`
			Version   string `json:"version"`
			EnvID     string `json:"envId"`
			NodeID    string `json:"nodeId"`
			Resources struct {
				MemoryMB  int `json:"memoryMb"`
				CPUShares int `json:"cpuShares"`
				DiskMB    int `json:"diskMb"`
			} `json:"resources"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		req.Kind = strings.TrimSpace(req.Kind)
		if req.Kind == "" {
			return fiber.NewError(fiber.StatusBadRequest, "kind is required")
		}
		if strings.TrimSpace(req.NodeID) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "nodeId is required")
		}
		ctx, cancel := longRequestContext()
		defer cancel()
		inst, err := svc.Provision(ctx, catalog.ProvisionInput{
			Kind:          req.Kind,
			Version:       req.Version,
			EnvironmentID: req.EnvID,
			NodeID:        req.NodeID,
			MemoryMB:      req.Resources.MemoryMB,
			CPUShares:     req.Resources.CPUShares,
			DiskMB:        req.Resources.DiskMB,
		})
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": inst})
	})

	// Instances of a catalog entry, optionally scoped to an environment
	// (?envId=) — the "my instances" view.
	instances := protected.Group("/catalog/:key/instances", requireRole("admin"), requireAdminScope("databases.read"))
	instances.Get("", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		list, err := svc.ListInstances(ctx, c.Params("key"), c.Query("envId"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": list})
	})
	// POST variant mirrors the phase spec (the UI may POST with an envId).
	instances.Post("", mutationLimiter, func(c *fiber.Ctx) error {
		var req struct {
			EnvID string `json:"envId"`
		}
		_ = c.BodyParser(&req)
		ctx, cancel := requestContext()
		defer cancel()
		list, err := svc.ListInstances(ctx, c.Params("key"), req.EnvID)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": list})
	})

	// Attach one catalog instance's connection vars to an environment.
	instances.Post("/:id/attach", mutationLimiter, requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		var req struct {
			EnvID string `json:"envId"`
		}
		if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.EnvID) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "envId is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		link, err := svc.Attach(ctx, c.Params("id"), req.EnvID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"data": link})
	})

	// Backup retention policy + on-demand run.
	retention := protected.Group("/catalog/backups/retention", requireRole("admin"), requireAdminScope("databases.read"))
	retention.Get("", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		policies, err := svc.Retention(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": policies})
	})
	retention.Put("", mutationLimiter, requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		var req struct {
			Kind          string `json:"kind"`
			RetentionDays *int   `json:"retentionDays"`
			RetentionMax  *int   `json:"retentionMax"`
			Enabled       *bool  `json:"enabled"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.Kind) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "kind is required")
		}
		days, max, enabled := 30, 8, true
		if req.RetentionDays != nil {
			days = *req.RetentionDays
		}
		if req.RetentionMax != nil {
			max = *req.RetentionMax
		}
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		ctx, cancel := requestContext()
		defer cancel()
		policy, err := svc.SetRetention(ctx, req.Kind, days, max, enabled)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"data": policy})
	})
	// On-demand retention pass (normally run on the hourly ticker).
	retention.Post("/run", mutationLimiter, requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		ctx, cancel := longRequestContext()
		defer cancel()
		deleted, err := svc.RunRetentionNow(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"deleted": deleted})
	})
}
