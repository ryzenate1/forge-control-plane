package http

import (
	"gamepanel/forge/internal/services/auditlog"

	"github.com/gofiber/fiber/v2"
)

func registerAuditLogRoutes(protected fiber.Router, cfg Config) {
	if cfg.AuditLogService == nil {
		return
	}

	h := auditlog.NewAuditLogHandler(cfg.AuditLogService)

	admin := protected.Group("/admin", requireRole("admin"))
	admin.Get("/audit-logs", h.HandleQuery)
	admin.Get("/audit-logs/user/:userId", func(c *fiber.Ctx) error {
		c.Request().URI().QueryArgs().Add("user_id", c.Params("userId"))
		return h.HandleQuery(c)
	})
	admin.Get("/audit-logs/resource/:resourceType/:resourceId", func(c *fiber.Ctx) error {
		c.Request().URI().QueryArgs().Add("resource_type", c.Params("resourceType"))
		c.Request().URI().QueryArgs().Add("resource_id", c.Params("resourceId"))
		return h.HandleQuery(c)
	})
}
