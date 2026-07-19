package http

import (
	"github.com/gofiber/fiber/v2"
)

func registerExternalLookupRoutes(protected fiber.Router, cfg Config) {
	protected.Get("/users/external/:externalId", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		externalID := c.Params("externalId")
		if externalID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "externalId is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		user, err := cfg.Store.GetUserByExternalID(ctx, externalID)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "user not found")
		}
		return c.JSON(user)
	})

	protected.Get("/servers/external/:externalId", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		externalID := c.Params("externalId")
		if externalID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "externalId is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		server, err := cfg.Store.GetServerByExternalID(ctx, externalID)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		return c.JSON(server)
	})
}
