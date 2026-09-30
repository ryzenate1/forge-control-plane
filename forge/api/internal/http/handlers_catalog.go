package http

import (
	"gamepanel/forge/internal/services/catalog"

	"github.com/gofiber/fiber/v2"
)

// registerCatalogRoutes wires the one-click service catalog endpoints that the
// web's lib/api/catalog.ts calls. The public group is /catalog; provisioning is
// admin-only under /admin/catalog.
func registerCatalogRoutes(protected fiber.Router, cfg Config, svc *catalog.Service, adminIPAccess, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	// --- Public catalog browsing (admin-authenticated) ---
	cat := protected.Group("/catalog", adminIPAccess, requireRole("admin"))

	cat.Get("", func(c *fiber.Ctx) error {
		entries, err := svc.ListEntries(c.Context())
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": entries})
	})

	cat.Get("/backups/retention", func(c *fiber.Ctx) error {
		policies, err := svc.Retention(c.Context())
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": policies})
	})

	cat.Put("/backups/retention", mutationLimiter, requireAdminScope("catalog.write"), func(c *fiber.Ctx) error {
		var req struct {
			Kind          string `json:"kind"`
			RetentionDays int    `json:"retentionDays"`
			RetentionMax  int    `json:"retentionMax"`
			Enabled       *bool  `json:"enabled"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if req.Kind == "" {
			return c.Status(400).JSON(fiber.Map{"error": "kind is required"})
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		policy, err := svc.SetRetention(c.Context(), req.Kind, req.RetentionDays, req.RetentionMax, enabled)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": policy})
	})

	cat.Post("/backups/retention/run", mutationLimiter, requireAdminScope("catalog.write"), func(c *fiber.Ctx) error {
		deleted, err := svc.RunRetentionNow(c.Context())
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"deleted": deleted})
	})

	cat.Get("/:key", func(c *fiber.Ctx) error {
		entry, err := svc.GetEntry(c.Context(), c.Params("key"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.JSON(fiber.Map{"data": entry})
	})

	cat.Get("/:key/instances", func(c *fiber.Ctx) error {
		envID := c.Query("envId")
		instances, err := svc.ListInstances(c.Context(), c.Params("key"), envID)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": instances})
	})

	// attach links a provisioned instance's connection variables to an environment.
	cat.Post("/:key/instances/:id/attach", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		var req struct {
			EnvID string `json:"envId"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if req.EnvID == "" {
			return c.Status(400).JSON(fiber.Map{"error": "envId is required"})
		}
		link, err := svc.Attach(c.Context(), c.Params("id"), req.EnvID)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"data": link})
	})

	// --- Admin provisioning ---
	adminCat := protected.Group("/admin/catalog", adminIPAccess, requireRole("admin"))

	adminCat.Post("/provision", mutationLimiter, requireAdminScope("catalog.write"), func(c *fiber.Ctx) error {
		var req struct {
			Kind      string `json:"kind"`
			Version   string `json:"version"`
			EnvID     string `json:"envId"`
			NodeID    string `json:"nodeId"`
			UserID    string `json:"userId"`
			Resources *struct {
				MemoryMB  int `json:"memoryMb"`
				CPUShares int `json:"cpuShares"`
				DiskMB    int `json:"diskMb"`
			} `json:"resources"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if req.Kind == "" {
			return c.Status(400).JSON(fiber.Map{"error": "kind is required"})
		}
		if req.NodeID == "" {
			return c.Status(400).JSON(fiber.Map{"error": "nodeId is required"})
		}
		in := catalog.ProvisionInput{
			Kind:          req.Kind,
			UserID:        req.UserID,
			Version:       req.Version,
			EnvironmentID: req.EnvID,
			NodeID:        req.NodeID,
		}
		if req.Resources != nil {
			in.MemoryMB = req.Resources.MemoryMB
			in.CPUShares = req.Resources.CPUShares
			in.DiskMB = req.Resources.DiskMB
		}
		instance, err := svc.Provision(c.Context(), in)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"data": instance})
	})
}
