package http

import (
	"context"
	"crypto/subtle"
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
			if h := c.Get(maintenanceBypassHeader); subtle.ConstantTimeCompare([]byte(strings.TrimSpace(h)), []byte(bypassToken)) == 1 {
				return c.Next()
			}
		}

		whitelist := os.Getenv("FORGE_MAINTENANCE_WHITELIST")
		if whitelist != "" {
			ips := strings.Split(whitelist, ",")
			// Resolved the same way every other IP-based decision is: the socket peer
			// unless TRUSTED_PROXIES says otherwise. c.IP() would let anyone who can
			// set a proxy header claim a whitelisted address and watch maintenance
			// mode go by, and would hand the operator's own bypass to every visitor
			// the moment the panel sits behind a reverse proxy.
			clientIP := ExtractClientIP(c)
			for _, ip := range ips {
				if isIPInList(clientIP, []string{strings.TrimSpace(ip)}) {
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
