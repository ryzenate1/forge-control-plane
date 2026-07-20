package http

import (
	"gamepanel/forge/internal/services/domains"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

type AddDomainRequest struct {
	Domain string `json:"domain"`
}

type CheckDNSRequest struct {
	Domain     string `json:"domain"`
	ExpectedIP string `json:"expectedIp"`
}

func registerDomainRoutes(protected fiber.Router, cfg Config, svc *domains.Service, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	protected.Post("/servers/:id/domains", mutationLimiter, requireServerPermission(cfg, store.PermServerSettings), func(c *fiber.Ctx) error {
		serverID := c.Params("id")
		var req AddDomainRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if req.Domain == "" {
			return c.Status(400).JSON(fiber.Map{"error": "domain is required"})
		}

		domain, err := svc.AddDomain(c.Context(), serverID, req.Domain)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"data": domain})
	})

	protected.Get("/servers/:id/domains", requireRole("admin"), requireAdminScope("domains.read"), func(c *fiber.Ctx) error {
		serverID := c.Params("id")
		domains, err := svc.ListDomains(c.Context(), serverID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": domains})
	})

	protected.Delete("/servers/:id/domains/:domainId", mutationLimiter, requireRole("admin"), requireAdminScope("domains.write"), func(c *fiber.Ctx) error {
		domainID := c.Params("domainId")
		if err := svc.RemoveDomain(c.Context(), domainID); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(204)
	})

	protected.Post("/domains/verify", mutationLimiter, requireRole("admin"), requireAdminScope("domains.write"), func(c *fiber.Ctx) error {
		var req struct {
			ID string `json:"id"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if req.ID == "" {
			return c.Status(400).JSON(fiber.Map{"error": "domain id is required"})
		}

		result, err := svc.VerifyOwnership(c.Context(), req.ID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": result})
	})

	protected.Post("/domains/check-dns", requireRole("admin"), requireAdminScope("domains.read"), func(c *fiber.Ctx) error {
		var req CheckDNSRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if req.Domain == "" {
			return c.Status(400).JSON(fiber.Map{"error": "domain is required"})
		}

		result, err := svc.CheckDNS(c.Context(), req.Domain, req.ExpectedIP)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": result})
	})
}

func registerWellKnownVerifyRoute(app *fiber.App, svc *domains.Service) {
	if svc == nil {
		return
	}

	app.Get("/.well-known/forge-verify", func(c *fiber.Ctx) error {
		host := c.Hostname()
		if host == "" {
			return c.Status(400).SendString("host header required")
		}

		record, err := svc.FindDomainByHost(c.Context(), host)
		if err != nil || record == nil {
			return c.Status(404).SendString("domain not registered")
		}
		if record.VerificationToken == nil || *record.VerificationToken == "" {
			return c.Status(404).SendString("no verification token")
		}

		c.Set("Content-Type", "text/plain")
		return c.SendString(*record.VerificationToken)
	})
}
