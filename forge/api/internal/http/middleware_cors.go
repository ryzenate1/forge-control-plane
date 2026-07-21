package http

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

type CORSConfig struct {
	AllowedOrigins   []string
	AllowMethods     string
	AllowHeaders     string
	AllowCredentials bool
	ExposeHeaders    string
	MaxAge           int
}

func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins: []string{
			"http://localhost:3000",
			"http://127.0.0.1:3000",
			"http://localhost:3002",
			"http://127.0.0.1:3002",
		},
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization,X-CSRF-Token,X-Forge-Session-Mode",
		AllowCredentials: true,
		MaxAge:           86400,
	}
}

func CORSMiddleware(cfg CORSConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		origin := c.Get("Origin")

		allowedOrigin := originAllowed(origin, cfg.AllowedOrigins)

		if allowedOrigin != "" {
			c.Set("Access-Control-Allow-Origin", allowedOrigin)
			c.Vary("Origin")
		}

		if cfg.AllowCredentials {
			c.Set("Access-Control-Allow-Credentials", "true")
		}

		if cfg.ExposeHeaders != "" {
			c.Set("Access-Control-Expose-Headers", cfg.ExposeHeaders)
		}

		if c.Method() == fiber.MethodOptions {
			c.Set("Access-Control-Allow-Methods", cfg.AllowMethods)
			c.Set("Access-Control-Allow-Headers", cfg.AllowHeaders)
			if cfg.MaxAge > 0 {
				c.Set("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
			}
			return c.SendStatus(fiber.StatusNoContent)
		}

		return c.Next()
	}
}

func originAllowed(origin string, allowedOrigins []string) string {
	if origin == "" {
		return ""
	}
	for _, allowed := range allowedOrigins {
		if allowed == "*" || allowed == origin {
			return origin
		}
	}
	return ""
}

func parseAllowedOrigins(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	origins := strings.Split(raw, ",")
	result := make([]string, 0, len(origins))
	for _, o := range origins {
		if trimmed := strings.TrimSpace(o); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
