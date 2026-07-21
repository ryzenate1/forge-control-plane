package http

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

const (
	authSourceCookieSession = "cookie-session"
	authSourceOAuth         = "oauth"
	authSourceAPIKey        = "api-key"

	CSRFTokenLength = 32
	CSRFTokenExpiry = 2 * time.Hour
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

func GenerateCSRFToken() (string, error) {
	bytes := make([]byte, CSRFTokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func SetCSRFCookie(c *fiber.Ctx, token string) {
	c.Cookie(&fiber.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		HTTPOnly: false,
		Secure:   true,
		SameSite: "Lax",
		Expires:  time.Now().Add(CSRFTokenExpiry),
		Path:     "/",
	})
}

func GetCSRFTokenHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		token, err := GenerateCSRFToken()
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to generate CSRF token")
		}

		SetCSRFCookie(c, token)

		return c.JSON(fiber.Map{
			"token":   token,
			"expires": time.Now().Add(CSRFTokenExpiry).Format(time.RFC3339),
		})
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
