package http

import (
	"gamepanel/forge/internal/services/appstore"

	"github.com/gofiber/fiber/v2"
)

func registerAppStoreRoutes(protected fiber.Router, cfg Config, svc *appstore.Service, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	store := protected.Group("/app-store", mutationLimiter)

	store.Get("/apps", func(c *fiber.Ctx) error {
		category := c.Query("category")
		search := c.Query("search")
		apps, err := svc.ListApps(c.Context(), category, search)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": apps})
	})

	store.Get("/apps/:key", func(c *fiber.Ctx) error {
		app, err := svc.GetApp(c.Context(), c.Params("key"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "app not found"})
		}
		return c.JSON(fiber.Map{"data": app})
	})

	store.Post("/install", func(c *fiber.Ctx) error {
		var req appstore.InstallRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request: " + err.Error()})
		}
		if req.AppKey == "" {
			return c.Status(400).JSON(fiber.Map{"error": "appKey is required"})
		}
		if req.UserID == "" {
			userID := getUserID(c)
			if userID == "" {
				userID = "system"
			}
			req.UserID = userID
		}
		if req.MemoryMB <= 0 {
			req.MemoryMB = 256
		}
		if req.CPUShares <= 0 {
			req.CPUShares = 512
		}
		if req.DiskMB <= 0 {
			req.DiskMB = 1024
		}

		inst, err := svc.InstallApp(c.Context(), &req)
		if err != nil {
			if inst != nil {
				return c.Status(500).JSON(fiber.Map{"data": inst, "error": err.Error()})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"data": inst})
	})

	store.Post("/:id/uninstall", func(c *fiber.Ctx) error {
		if err := svc.UninstallApp(c.Context(), c.Params("id")); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": "ok"})
	})

	store.Get("/installed", func(c *fiber.Ctx) error {
		installs, err := svc.ListInstalls(c.Context())
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": installs})
	})

	store.Post("/:id/upgrade", func(c *fiber.Ctx) error {
		inst, err := svc.UpgradeApp(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": inst})
	})

	store.Post("/sync", func(c *fiber.Ctx) error {
		var req struct {
			RegistryURL string `json:"registryUrl"`
		}
		_ = c.BodyParser(&req)
		if err := svc.SyncFromRemote(c.Context(), req.RegistryURL); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": "sync initiated"})
	})
}
