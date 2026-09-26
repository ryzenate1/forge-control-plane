package http

import (
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// registerApiKeyRoutes wires the API-key management endpoints that the web's
// AdminApiKeys component calls. Keys are scoped to the authenticated user.
func registerApiKeyRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	if cfg.Store == nil {
		return
	}

	keys := protected.Group("/api-keys", requireRole("admin"))

	keys.Get("", func(c *fiber.Ctx) error {
		userID := c.Locals("userId")
		uid, _ := userID.(string)
		list, err := cfg.Store.ListApiKeys(c.Context(), uid)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(list)
	})

	keys.Post("", mutationLimiter, func(c *fiber.Ctx) error {
		userID := c.Locals("userId")
		uid, _ := userID.(string)
		var req store.CreateApiKeyRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		key, err := cfg.Store.CreateApiKey(c.Context(), uid, req)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.Status(201).JSON(key)
	})

	keys.Delete("/:id", mutationLimiter, func(c *fiber.Ctx) error {
		userID := c.Locals("userId")
		uid, _ := userID.(string)
		if err := cfg.Store.DeleteApiKey(c.Context(), uid, c.Params("id")); err != nil {
			return respondInternalError(c, err)
		}
		return c.SendStatus(204)
	})
}
