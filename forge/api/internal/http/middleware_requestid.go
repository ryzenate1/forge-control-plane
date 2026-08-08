package http

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func isPrintableASCII(s string) bool {
	for _, c := range s {
		if c < 32 || c > 126 {
			return false
		}
	}
	return true
}

func generateRequestID(c *fiber.Ctx) string {
	id := c.Get("X-Request-ID")
	if id != "" && len(id) <= 128 && isPrintableASCII(id) {
		return id
	}
	return uuid.New().String()
}

// RequestIDMiddleware injects a unique request ID into every request.
// If the client supplies a valid X-Request-ID header (≤128 printable ASCII
// characters) it is forwarded; otherwise a new UUID v4 is generated. The ID
// is set on the response header and stored in fiber context locals under the
// "requestId" key.
func RequestIDMiddleware(c *fiber.Ctx) error {
	id := generateRequestID(c)
	c.Set("X-Request-ID", id)
	c.Locals("requestId", id)
	return c.Next()
}
