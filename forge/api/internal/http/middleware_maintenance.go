package http

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

const (
	maintenanceModeEnvVar        = "FORGE_MAINTENANCE_MODE"
	maintenanceBypassHeader      = "X-Forge-Maintenance-Bypass"
	maintenanceBypassTokenEnvVar = "FORGE_MAINTENANCE_BYPASS_TOKEN"
)

func MaintenanceModeMiddleware(cfg Config) fiber.Handler {
	bypassToken := os.Getenv(maintenanceBypassTokenEnvVar)

	return func(c *fiber.Ctx) error {
		enabled := os.Getenv(maintenanceModeEnvVar) == "true"

		if !enabled && cfg.Store != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			ms, err := cfg.Store.GetMaintenanceSettings(ctx)
			cancel()
			if err == nil && ms.Enabled {
				enabled = true
			}
		}

		if !enabled {
			return c.Next()
		}

		if bypassToken != "" {
			if h := c.Get(maintenanceBypassHeader); strings.TrimSpace(h) == bypassToken {
				return c.Next()
			}
		}

		whitelist := os.Getenv("FORGE_MAINTENANCE_WHITELIST")
		if whitelist != "" {
			ips := strings.Split(whitelist, ",")
			clientIP := c.IP()
			for _, ip := range ips {
				if strings.TrimSpace(ip) == clientIP {
					return c.Next()
				}
			}
		}

		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"status":  "maintenance",
			"message": "the panel is currently under maintenance",
		})
	}
}
