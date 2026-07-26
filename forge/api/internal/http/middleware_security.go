package http

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

// SecurityHeaders implements comprehensive security headers as identified in the
// comprehensive technical audit. This prevents XSS, clickjacking, MIME sniffing,
// and other common web vulnerabilities.
var monacoPrefixes = []string{"/admin/nests", "/admin/servers", "/servers"}

func isMonacoRoute(path string) bool {
	for _, p := range monacoPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func SecurityHeaders(env string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		scriptSrc := "'self'"
		if isMonacoRoute(c.Path()) {
			scriptSrc = "'self' 'unsafe-eval'"
		}
		c.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src "+scriptSrc+"; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: https:; "+
				"font-src 'self' data:; "+
				"connect-src 'self' ws: wss:; "+
				"frame-ancestors 'none'; "+
				"base-uri 'self'; "+
				"form-action 'self'; "+
				"report-uri /api/v1/csp-report")

		c.Set("X-Frame-Options", "DENY")
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Set("Permissions-Policy",
			"geolocation=(), microphone=(), camera=(), payment=(), usb=(), magnetometer=(), gyroscope=()")

		hsts := "max-age=31536000; includeSubDomains"
		if env == "" || env == "production" {
			hsts += "; preload"
		}
		c.Set("Strict-Transport-Security", hsts)

		return c.Next()
	}
}
