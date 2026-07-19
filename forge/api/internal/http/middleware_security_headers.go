package http

import (
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
		CSPValue:              "default-src 'self'",
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
				"max-age="+intToStr(cfg.HSTSMaxAge)+"; includeSubDomains")
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

func intToStr(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
