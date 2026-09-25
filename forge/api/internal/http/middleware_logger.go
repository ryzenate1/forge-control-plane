package http

import (
	"log/slog"
	"time"

	"gamepanel/forge/internal/events"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func StructuredLogger(logger *slog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		requestID, _ := c.Locals("requestId").(string)
		if requestID == "" {
			requestID = uuid.NewString()
			c.Locals("requestId", requestID)
			c.Set("X-Request-ID", requestID)
		} else {
			c.Set("X-Request-ID", requestID)
		}

		// Bridge request_id -> correlation_id so downstream handlers and
		// services reading events.CorrelationIDFromContext observe the same id.
		// Only wrap when the request context carries no correlation id yet, so
		// an id already set upstream (e.g. RequestIDMiddleware) is preserved and
		// never double-emitted.
		if events.CorrelationIDFromContext(c.Context()) == "" {
			c.SetUserContext(events.ContextWithCorrelationID(c.Context(), requestID))
		}
		correlationID := events.CorrelationIDFromContext(c.Context())

		err := c.Next()

		duration := time.Since(start)
		status := c.Response().StatusCode()
		method := c.Method()
		path := c.Path()
		// Same resolver as every other IP decision, so the access log cannot be
		// made to record an address the caller invented. With no trusted proxy this
		// is the socket peer, exactly as before.
		ip := ExtractClientIP(c)

		attrs := []slog.Attr{
			slog.String("request_id", requestID),
			slog.String("method", method),
			slog.String("path", path),
			slog.Int("status", status),
			slog.Duration("duration", duration),
			slog.String("ip", ip),
		}
		if correlationID != "" {
			attrs = append(attrs, slog.String("correlation_id", correlationID))
		}

		if user, ok := c.Locals("user").(tokenClaims); ok {
			attrs = append(attrs, slog.String("user_id", user.Sub))
		}

		if err != nil {
			attrs = append(attrs, slog.String("error", err.Error()))
		}

		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}

		logger.LogAttrs(c.Context(), level, "request", attrs...)

		return err
	}
}
