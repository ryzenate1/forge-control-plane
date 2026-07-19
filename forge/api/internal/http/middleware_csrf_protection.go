package http

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

const (
	CSRFHeaderName  = "X-CSRF-Token"
	CSRFTokenLength = 32
	CSRFTokenExpiry = 2 * time.Hour
)

var (
	ErrMissingCSRFToken = errors.New("CSRF token is missing")
	ErrInvalidCSRFToken = errors.New("CSRF token is invalid")
)

// CSRFMiddleware provides CSRF protection using double-submit cookie pattern
func CSRFMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Skip CSRF for GET, HEAD, OPTIONS, TRACE requests
		if c.Method() == "GET" || c.Method() == "HEAD" || c.Method() == "OPTIONS" || c.Method() == "TRACE" {
			return c.Next()
		}

		// For state-changing requests, validate CSRF token
		token := c.Get(CSRFHeaderName)
		if token == "" {
			return fiber.NewError(fiber.StatusForbidden, ErrMissingCSRFToken.Error())
		}

		cookieToken := c.Cookies(CSRFCookieName)
		if cookieToken == "" {
			return fiber.NewError(fiber.StatusForbidden, ErrMissingCSRFToken.Error())
		}

		// Validate token matches cookie (double-submit pattern)
		if !constantTimeCompare(token, cookieToken) {
			return fiber.NewError(fiber.StatusForbidden, ErrInvalidCSRFToken.Error())
		}

		return c.Next()
	}
}

// GenerateCSRFToken generates a secure random CSRF token
func GenerateCSRFToken() (string, error) {
	bytes := make([]byte, CSRFTokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// SetCSRFCookie sets the CSRF cookie on the response
func SetCSRFCookie(c *fiber.Ctx, token string) {
	c.Cookie(&fiber.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Strict",
		Expires:  time.Now().Add(CSRFTokenExpiry),
		Path:     "/",
	})
}

// constantTimeCompare safely compares two strings in constant time
func constantTimeCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}

	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}

	return result == 0
}

// GetCSRFTokenHandler returns the CSRF token to the client
func GetCSRFTokenHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Generate new token
		token, err := GenerateCSRFToken()
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to generate CSRF token")
		}

		// Set CSRF cookie
		SetCSRFCookie(c, token)

		// Return token in response for non-cookie clients
		return c.JSON(fiber.Map{
			"token":   token,
			"expires": time.Now().Add(CSRFTokenExpiry).Format(time.RFC3339),
		})
	}
}

// IsCSRFExemptRoute checks if a route should be exempt from CSRF protection
func IsCSRFExemptRoute(path string) bool {
	exemptRoutes := []string{
		"/auth/login",
		"/auth/login/checkpoint",
		"/auth/logout",
		"/sanctum/csrf-cookie",
		"/csrf-token",
	}

	for _, route := range exemptRoutes {
		if strings.HasPrefix(path, route) {
			return true
		}
	}

	return false
}
