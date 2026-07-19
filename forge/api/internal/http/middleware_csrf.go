package http

import (
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v2"
)

const (
	authSourceCookieSession = "cookie-session"
	authSourceBearerSession = "bearer-session"
	authSourceOAuth         = "oauth"
	authSourceAPIKey        = "api-key"
)

func csrfMiddleware(cfg SessionCookieConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		method := c.Method()
		if method == fiber.MethodGet || method == fiber.MethodHead || method == fiber.MethodOptions {
			return c.Next()
		}

		authSource, _ := c.Locals("authSource").(string)
		if authSource != authSourceCookieSession {
			return c.Next()
		}

		csrfCookie := c.Cookies(CSRFCookieName)
		if csrfCookie == "" {
			return fiber.NewError(fiber.StatusForbidden, "missing CSRF cookie")
		}

		csrfHeader := c.Get("X-CSRF-Token")
		if csrfHeader == "" {
			return fiber.NewError(fiber.StatusForbidden, "missing X-CSRF-Token header")
		}

		if subtle.ConstantTimeCompare([]byte(csrfCookie), []byte(csrfHeader)) != 1 {
			return fiber.NewError(fiber.StatusForbidden, "invalid CSRF token")
		}

		origin := c.Get("Origin")
		if origin != "" {
			panelOrigin := c.Locals("panelOrigin")
			if panelOriginStr, ok := panelOrigin.(string); ok && panelOriginStr != "" {
				if origin != panelOriginStr {
					return fiber.NewError(fiber.StatusForbidden, "invalid Origin")
				}
			}
		}

		fetchSite := c.Get("Sec-Fetch-Site")
		if fetchSite == "cross-site" {
			return fiber.NewError(fiber.StatusForbidden, "cross-site request forbidden")
		}

		return c.Next()
	}
}

func publicMutationOriginCheck(cfg SessionCookieConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		method := c.Method()
		if method != fiber.MethodPost && method != fiber.MethodPut && method != fiber.MethodPatch && method != fiber.MethodDelete {
			return c.Next()
		}

		origin := c.Get("Origin")
		if origin != "" {
			panelOrigin := c.Locals("panelOrigin")
			if panelOriginStr, ok := panelOrigin.(string); ok && panelOriginStr != "" {
				if origin != panelOriginStr {
					return fiber.NewError(fiber.StatusForbidden, "invalid Origin")
				}
			}
		}

		fetchSite := c.Get("Sec-Fetch-Site")
		if fetchSite == "cross-site" {
			return fiber.NewError(fiber.StatusForbidden, "cross-site request forbidden")
		}

		contentType := c.Get("Content-Type")
		if contentType != "" && !strings.HasPrefix(contentType, "application/json") && !strings.HasPrefix(contentType, "multipart/form-data") {
			return fiber.NewError(fiber.StatusForbidden, "invalid Content-Type")
		}

		return c.Next()
	}
}
