package http

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
)

type SecurityHeadersConfig struct {
	ContentTypeOptions    bool
	FrameOptions          bool
	XSSProtection         bool
	StrictTransport       bool
	ContentSecurityPolicy bool
	ReferrerPolicy        bool
	PermissionsPolicy     bool
	CSPValue              string
	HSTSMaxAge            int
	FrameOptionsValue     string
}

func DefaultSecurityHeadersConfig() SecurityHeadersConfig {
	return SecurityHeadersConfig{
		ContentTypeOptions:    true,
		FrameOptions:          true,
		XSSProtection:         true,
		StrictTransport:       true,
		ContentSecurityPolicy: true,
		ReferrerPolicy:        true,
		PermissionsPolicy:     true,
		CSPValue:              "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self' ws: wss:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'",
		HSTSMaxAge:            31536000,
		FrameOptionsValue:     "DENY",
	}
}

func SecurityHeadersMiddleware(cfg SecurityHeadersConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.ContentTypeOptions {
			c.Set("X-Content-Type-Options", "nosniff")
		}
		if cfg.FrameOptions {
			c.Set("X-Frame-Options", cfg.FrameOptionsValue)
		}
		if cfg.XSSProtection {
			c.Set("X-XSS-Protection", "1; mode=block")
		}
		if cfg.StrictTransport {
			c.Set("Strict-Transport-Security",
				"max-age="+strconv.Itoa(cfg.HSTSMaxAge)+"; includeSubDomains; preload")
		}
		if cfg.ContentSecurityPolicy {
			c.Set("Content-Security-Policy", cfg.CSPValue)
		}
		if cfg.ReferrerPolicy {
			c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		}
		if cfg.PermissionsPolicy {
			c.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		}
		return c.Next()
	}
}
