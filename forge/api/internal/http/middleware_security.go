package http

import (
	"github.com/gofiber/fiber/v2"
)

// SecurityHeaders implements comprehensive security headers as identified in the
// comprehensive technical audit. This prevents XSS, clickjacking, MIME sniffing,
// and other common web vulnerabilities.
func SecurityHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Content Security Policy - prevents XSS attacks
		// This is a restrictive policy; adjust based on actual needs
		c.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline' 'unsafe-eval'; "+ // Monaco editor requires unsafe-eval
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: https:; "+
				"font-src 'self' data:; "+
				"connect-src 'self' ws: wss:; "+ // WebSocket connections
				"frame-ancestors 'none'") // Prevents framing

		// X-Frame-Options - prevents clickjacking
		c.Set("X-Frame-Options", "DENY")

		// X-Content-Type-Options - prevents MIME sniffing
		c.Set("X-Content-Type-Options", "nosniff")

		// X-XSS-Protection - enables browser XSS filtering (legacy but still useful)
		c.Set("X-XSS-Protection", "1; mode=block")

		// Referrer-Policy - controls referrer information
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Permissions-Policy - restricts browser features
		c.Set("Permissions-Policy",
			"geolocation=(), "+
				"microphone=(), "+
				"camera=(), "+
				"payment=(), "+
				"usb=(), "+
				"magnetometer=(), "+
				"gyroscope=()")

		// Strict-Transport-Security - enforces HTTPS (only if running on HTTPS)
		// This should be enabled in production behind reverse proxy
		if c.Secure() || c.Protocol() == "https" {
			c.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		return c.Next()
	}
}
