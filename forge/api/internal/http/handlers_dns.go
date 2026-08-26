package http

import (
	"github.com/gofiber/fiber/v2"

	"gamepanel/forge/internal/services/dns"
)

func registerDNSRoutes(protected fiber.Router, cfg Config, svc *dns.Service, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	dg := protected.Group("/dns", requireRole("admin"))

	dg.Get("/providers", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"data": svc.ListSupportedProviders()})
	})

	dg.Get("/providers/configured", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		providers, err := cfg.Store.ListDNSProviders(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"data": providers})
	})

	dg.Post("/providers", mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var body struct {
			Name         string `json:"name"`
			ProviderType string `json:"providerType"`
			Credentials  map[string]string `json:"credentials"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if body.Name == "" || body.ProviderType == "" {
			return fiber.NewError(fiber.StatusBadRequest, "name and providerType are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		provider, err := svc.ConfigureProvider(ctx, body.Name, body.ProviderType, c.Body())
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"data": provider})
	})

	dg.Delete("/providers/:id", mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteDNSProvider(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	dg.Post("/providers/:id/verify", mutationLimiter, func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.VerifyProvider(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "verified": true})
	})

	dg.Post("/providers/:id/set-default", mutationLimiter, func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.SetDefaultProvider(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})
}
