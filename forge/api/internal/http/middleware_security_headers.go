package http

import (
	"crypto/rand"
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// generateCSPNonce returns a fresh cryptographically-random CSP nonce for a
// single response, encoded with base64.StdEncoding. It is generated per
// response (never static, never reused, never a fallback constant). On the
// (practically impossible) failure to read from crypto/rand it returns an empty
// string rather than a fake value, so the CSP degrades honestly to no nonce
// instead of pretending to be protected.
func generateCSPNonce() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(buf)
}

// applyCSPNonce weaves the per-response nonce source into a CSP's script-src
// directive. A {NONCE} placeholder, when present, is always substituted; else
// 'nonce-<value>' is injected into script-src so inline scripts can be
// authorized. No 'strict-dynamic' and no fallback tokens are ever emitted.
func applyCSPNonce(csp string, nonce string) string {
	nonceSrc := "'nonce-" + nonce + "'"
	if strings.Contains(csp, "{NONCE}") {
		return strings.ReplaceAll(csp, "{NONCE}", nonceSrc)
	}
	if idx := strings.Index(csp, "script-src"); idx >= 0 {
		end := idx + len("script-src")
		return csp[:end] + " " + nonceSrc + csp[end:]
	}
	if csp == "" || strings.HasSuffix(csp, ";") {
		return csp + "script-src " + nonceSrc + ";"
	}
	return csp + "; script-src " + nonceSrc
}

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
		// Strict-Transport-Security is emitted whenever the config enables it.
		// Production-only gating lives in the wired SecurityHeaders(env) handler
		// (the real server middleware chain), which reverification tests pin; this
		// config-driven helper honors its cfg.StrictTransport flag directly.
		if cfg.StrictTransport {
			c.Set("Strict-Transport-Security",
				"max-age="+strconv.Itoa(cfg.HSTSMaxAge)+"; includeSubDomains; preload")
		}
		if cfg.ContentSecurityPolicy {
			// Fresh per-response nonce; expose it to handlers/templates via
			// c.Locals("cspNonce", ...) and mirror it in a debug header.
			nonce := generateCSPNonce()
			c.Locals("cspNonce", nonce)
			c.Set("X-CSP-Nonce", nonce)
			c.Set("Content-Security-Policy", applyCSPNonce(cfg.CSPValue, nonce))
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
