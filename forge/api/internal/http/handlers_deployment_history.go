package http

import (
	"strconv"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerDeploymentHistoryRoutes(protected fiber.Router, cfg Config, adminIPAccess, mutationLimiter fiber.Handler) {
	if cfg.Store == nil {
		return
	}

	dh := protected.Group("/admin/deployment-history", adminIPAccess)

	dh.Get("/", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		serverID := c.Query("serverId")
		var records []store.DeploymentRecord
		var err error
		if serverID != "" {
			records, err = cfg.Store.ListDeploymentRecords(c.Context(), serverID)
		} else {
			records, err = cfg.Store.ListAllDeploymentRecords(c.Context())
		}
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": records})
	})

	dh.Get("/:id", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		record, err := cfg.Store.GetDeploymentRecord(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "deployment record not found"})
		}
		return c.JSON(fiber.Map{"data": record})
	})

	dh.Get("/:id/logs", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		limit, _ := strconv.Atoi(c.Query("limit", "100"))
		entries, err := cfg.Store.ListDeploymentLogs(c.Context(), c.Params("id"), limit)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		type logEntry struct {
			ID        string `json:"id"`
			Content   string `json:"content"`
			CreatedAt string `json:"createdAt"`
		}
		result := make([]logEntry, 0, len(entries))
		for _, e := range entries {
			result = append(result, logEntry{
				ID:        e.ID,
				Content:   e.Message,
				CreatedAt: e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			})
		}
		return c.JSON(fiber.Map{"data": result})
	})

	dh.Get("/:id/rollbacks", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		rollbacks, err := cfg.Store.ListRollbacks(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": rollbacks})
	})
}
