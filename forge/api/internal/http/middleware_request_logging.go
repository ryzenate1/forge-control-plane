package http

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

type RequestLogEntry struct {
	Method    string `json:"method"`
	Path      string `json:"path"`
	Status    int    `json:"status"`
	Duration  string `json:"duration"`
	ClientIP  string `json:"client_ip"`
	UserAgent string `json:"user_agent"`
	RequestID string `json:"request_id"`
	Timestamp string `json:"timestamp"`
}

type RequestLoggingConfig struct {
	LogLevel            string
	ExcludeHealthChecks bool
	HealthCheckPrefixes []string
	Logger              func(entry RequestLogEntry)
}

func DefaultRequestLoggingConfig() RequestLoggingConfig {
	return RequestLoggingConfig{
		LogLevel:            "info",
		ExcludeHealthChecks: true,
		HealthCheckPrefixes: []string{"/health", "/health/", "/api/v1/health"},
	}
}

type statusWriter struct {
	fiber.Response
	statusCode int
}

func RequestLoggingMiddleware(cfg RequestLoggingConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.ExcludeHealthChecks {
			path := c.Path()
			for _, prefix := range cfg.HealthCheckPrefixes {
				if strings.HasPrefix(path, prefix) {
					return c.Next()
				}
			}
		}

		start := time.Now()
		requestID := c.Get("X-Request-ID")
		if requestID == "" {
			requestID = c.GetRespHeader("X-Request-ID")
		}

		err := c.Next()

		duration := time.Since(start)
		status := c.Response().StatusCode()

		entry := RequestLogEntry{
			Method:    c.Method(),
			Path:      c.Path(),
			Status:    status,
			Duration:  duration.String(),
			ClientIP:  c.IP(),
			UserAgent: c.Get("User-Agent"),
			RequestID: requestID,
			Timestamp: start.UTC().Format(time.RFC3339),
		}

		if cfg.Logger != nil {
			cfg.Logger(entry)
		} else {
			data, _ := json.Marshal(entry)
			log.Println(string(data))
		}

		return err
	}
}
