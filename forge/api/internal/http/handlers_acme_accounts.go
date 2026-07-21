package http

import (
	"encoding/json"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerAcmeAccountRoutes(protected fiber.Router, cfg Config, adminIPAccess, mutationLimiter fiber.Handler) {
	if cfg.Store == nil {
		return
	}

	acct := protected.Group("/acme", adminIPAccess)

	// ACME accounts
	acct.Get("/accounts", requireRole("admin"), requireAdminScope("certificates.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		accounts, err := cfg.Store.ListAcmeAccounts(ctx)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": accounts})
	})

	acct.Post("/accounts", mutationLimiter, requireRole("admin"), requireAdminScope("certificates.write"), func(c *fiber.Ctx) error {
		var body struct {
			Email      string `json:"email"`
			PrivateKey string `json:"privateKey"`
			CAURL      string `json:"caUrl"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		if body.Email == "" {
			return c.Status(400).JSON(fiber.Map{"error": "email is required"})
		}
		ctx, cancel := requestContext()
		defer cancel()
		account, err := cfg.Store.CreateAcmeAccount(ctx, store.CreateAcmeAccountRequest{
			Email:      body.Email,
			PrivateKey: body.PrivateKey,
			CAURL:      body.CAURL,
		})
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"data": account})
	})

	acct.Get("/accounts/:id", requireRole("admin"), requireAdminScope("certificates.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		account, err := cfg.Store.GetAcmeAccount(ctx, c.Params("id"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": account})
	})

	acct.Put("/accounts/:id", mutationLimiter, requireRole("admin"), requireAdminScope("certificates.write"), func(c *fiber.Ctx) error {
		var body struct {
			Email      *string `json:"email"`
			PrivateKey *string `json:"privateKey"`
			CAURL      *string `json:"caUrl"`
			IsDefault  *bool   `json:"isDefault"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		ctx, cancel := requestContext()
		defer cancel()
		account, err := cfg.Store.UpdateAcmeAccount(ctx, c.Params("id"), store.UpdateAcmeAccountRequest{
			Email:     body.Email,
			PrivateKey: body.PrivateKey,
			CAURL:     body.CAURL,
			IsDefault: body.IsDefault,
		})
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": account})
	})

	acct.Delete("/accounts/:id", mutationLimiter, requireRole("admin"), requireAdminScope("certificates.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteAcmeAccount(ctx, c.Params("id")); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(204)
	})

	// DNS provider accounts
	acct.Get("/dns-accounts", requireRole("admin"), requireAdminScope("certificates.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		provider := c.Query("provider")
		accounts, err := cfg.Store.ListDNSProviderAccounts(ctx, provider)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": accounts})
	})

	acct.Post("/dns-accounts", mutationLimiter, requireRole("admin"), requireAdminScope("certificates.write"), func(c *fiber.Ctx) error {
		var body struct {
			Name        string            `json:"name"`
			Provider    string            `json:"provider"`
			Credentials map[string]string `json:"credentials"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		if body.Name == "" || body.Provider == "" {
			return c.Status(400).JSON(fiber.Map{"error": "name and provider are required"})
		}
		raw, _ := json.Marshal(body.Credentials)
		ctx, cancel := requestContext()
		defer cancel()
		account, err := cfg.Store.CreateDNSProviderAccount(ctx, store.CreateDNSProviderAccountRequest{
			Name:        body.Name,
			Provider:    body.Provider,
			Credentials: raw,
		})
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"data": account})
	})

	acct.Get("/dns-accounts/:id", requireRole("admin"), requireAdminScope("certificates.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		account, err := cfg.Store.GetDNSProviderAccount(ctx, c.Params("id"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": account})
	})

	acct.Put("/dns-accounts/:id", mutationLimiter, requireRole("admin"), requireAdminScope("certificates.write"), func(c *fiber.Ctx) error {
		var body struct {
			Name        *string           `json:"name"`
			Provider    *string           `json:"provider"`
			Credentials *map[string]string `json:"credentials"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		var credsRaw *json.RawMessage
		if body.Credentials != nil {
			raw, _ := json.Marshal(*body.Credentials)
			rm := json.RawMessage(raw)
			credsRaw = &rm
		}
		ctx, cancel := requestContext()
		defer cancel()
		account, err := cfg.Store.UpdateDNSProviderAccount(ctx, c.Params("id"), store.UpdateDNSProviderAccountRequest{
			Name:        body.Name,
			Provider:    body.Provider,
			Credentials: credsRaw,
		})
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": account})
	})

	acct.Delete("/dns-accounts/:id", mutationLimiter, requireRole("admin"), requireAdminScope("certificates.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteDNSProviderAccount(ctx, c.Params("id")); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(204)
	})
}
