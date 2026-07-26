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

		csrfCookie := c.Cookies(secureCookieName(CSRFCookieName, cfg.Secure))
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
		panelOrigin := c.Locals("panelOrigin")
		if panelOriginStr, ok := panelOrigin.(string); ok && panelOriginStr != "" {
			if origin == "" {
				// Origin header required for cookie-session mutations
				fetchSite := c.Get("Sec-Fetch-Site")
				if fetchSite == "" {
					return fiber.NewError(fiber.StatusForbidden, "missing Origin header")
				}
			} else if origin != panelOriginStr {
				return fiber.NewError(fiber.StatusForbidden, "invalid Origin")
			}
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
	cfg := LoadSessionCookieConfig()
	c.Cookie(&fiber.Cookie{
		Name:     secureCookieName(CSRFCookieName, cfg.Secure),
		Value:    token,
		HTTPOnly: false,
		Secure:   cfg.Secure,
		SameSite: "Strict",
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
		panelOrigin := c.Locals("panelOrigin")
		if panelOriginStr, ok := panelOrigin.(string); ok && panelOriginStr != "" {
			if origin == "" {
				// Reject state-changing requests without Origin when panelOrigin is configured
				fetchSite := c.Get("Sec-Fetch-Site")
				if fetchSite != "same-origin" && fetchSite != "same-site" && fetchSite != "strict-same-origin" {
					return fiber.NewError(fiber.StatusForbidden, "missing Origin header on state-changing request")
				}
			} else if origin != panelOriginStr {
				return fiber.NewError(fiber.StatusForbidden, "invalid Origin")
			}
		}

		contentType := c.Get("Content-Type")
		if method == fiber.MethodPost || method == fiber.MethodPut || method == fiber.MethodPatch {
			if contentType == "" && c.Request().Header.ContentLength() > 0 {
				return fiber.NewError(fiber.StatusForbidden, "missing Content-Type header")
			}
			if contentType != "" && !strings.HasPrefix(contentType, "application/json") && !strings.HasPrefix(contentType, "multipart/form-data") {
				return fiber.NewError(fiber.StatusForbidden, "invalid Content-Type")
			}
		}

		return c.Next()
	}
}
