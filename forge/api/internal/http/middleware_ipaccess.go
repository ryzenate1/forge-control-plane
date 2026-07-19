package http

import (
	"net"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// IPAccessConfig defines IP-based access control configuration
type IPAccessConfig struct {
	// AllowedIPs is a list of allowed IP addresses or CIDR ranges
	AllowedIPs []string
	// DeniedIPs is a list of denied IP addresses or CIDR ranges
	DeniedIPs []string
	// TrustProxy determines if we should trust X-Forwarded-For headers
	TrustProxy bool
}

// IPAccessControl creates middleware for IP-based access control
func IPAccessControl(cfg IPAccessConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		clientIP := getClientIP(c, cfg.TrustProxy)

		// Check deny list first (if denied, reject immediately)
		if len(cfg.DeniedIPs) > 0 && isIPInList(clientIP, cfg.DeniedIPs) {
			return fiber.NewError(fiber.StatusForbidden, "access denied from this IP")
		}

		// Check allow list (if defined, only allow matching IPs)
		if len(cfg.AllowedIPs) > 0 && !isIPInList(clientIP, cfg.AllowedIPs) {
			return fiber.NewError(fiber.StatusForbidden, "access denied from this IP")
		}

		return c.Next()
	}
}

// getClientIP gets the client IP address, optionally trusting proxy headers
func getClientIP(c *fiber.Ctx, trustProxy bool) string {
	if trustProxy {
		// Check X-Forwarded-For header (may contain multiple IPs)
		xff := c.Get("X-Forwarded-For")
		if xff != "" {
			// Get the first IP in the chain (original client)
			ips := strings.Split(xff, ",")
			if len(ips) > 0 {
				return strings.TrimSpace(ips[0])
			}
		}

		// Check X-Real-IP header
		xri := c.Get("X-Real-IP")
		if xri != "" {
			return xri
		}
	}

	// Fall back to direct connection IP
	return c.IP()
}

// isIPInList checks if an IP is in a list of IPs or CIDR ranges
func isIPInList(ipStr string, ipList []string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	for _, item := range ipList {
		// Check if it's a CIDR range
		if strings.Contains(item, "/") {
			_, ipNet, err := net.ParseCIDR(item)
			if err != nil {
				continue
			}
			if ipNet.Contains(ip) {
				return true
			}
		} else {
			// Check exact IP match
			if item == ipStr {
				return true
			}
		}
	}

	return false
}

// splitCIDRs splits a comma-separated string of CIDR ranges into a slice.
func splitCIDRs(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// AdminIPAccessConfig returns IP access config for admin endpoints
func AdminIPAccessConfig(cfg Config) IPAccessConfig {
	allowRaw := os.Getenv("ADMIN_IP_ALLOW")
	denyRaw := os.Getenv("ADMIN_IP_DENY")

	ipcfg := IPAccessConfig{
		AllowedIPs: splitCIDRs(allowRaw),
		DeniedIPs:  splitCIDRs(denyRaw),
		TrustProxy: true,
	}

	if len(ipcfg.AllowedIPs) == 0 && len(ipcfg.DeniedIPs) == 0 {
		if cfg.Logger != nil {
			cfg.Logger.Warn("Admin IP access control not configured — set ADMIN_IP_ALLOW / ADMIN_IP_DENY environment variables")
		}
	}

	return ipcfg
}

// APIIPAccessConfig returns IP access config for API endpoints
func APIIPAccessConfig(cfg Config) IPAccessConfig {
	allowRaw := os.Getenv("API_IP_ALLOW")
	denyRaw := os.Getenv("API_IP_DENY")

	ipcfg := IPAccessConfig{
		AllowedIPs: splitCIDRs(allowRaw),
		DeniedIPs:  splitCIDRs(denyRaw),
		TrustProxy: true,
	}

	if len(ipcfg.AllowedIPs) == 0 && len(ipcfg.DeniedIPs) == 0 {
		if cfg.Logger != nil {
			cfg.Logger.Warn("API IP access control not configured — set API_IP_ALLOW / API_IP_DENY environment variables")
		}
	}

	return ipcfg
}
