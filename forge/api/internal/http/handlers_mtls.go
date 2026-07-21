package http

import (
	"gamepanel/forge/internal/services"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerMTLSRoutes(protected fiber.Router, cfg Config, certSvc services.MTLSCertificateSvc, migrator *services.MTLSMigrator, adminIPAccess, mutationLimiter fiber.Handler) {
	mtls := protected.Group("/mtls", adminIPAccess)

	mtls.Post("/certificates/generate-ca", mutationLimiter, requireRole("admin"), requireAdminScope("certificates.write"), func(c *fiber.Ctx) error {
		var req struct {
			Organization string `json:"organization"`
			CommonName   string `json:"commonName"`
		}
		_ = c.BodyParser(&req)

		cert, err := certSvc.GenerateCA(c.Context(), req.Organization, req.CommonName)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"data": cert})
	})

	mtls.Post("/nodes/:id/certificates/generate", mutationLimiter, requireRole("admin"), requireAdminScope("certificates.write"), func(c *fiber.Ctx) error {
		cert, err := certSvc.GenerateNodeCert(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"data": cert})
	})

	mtls.Get("/certificates", requireRole("admin"), requireAdminScope("certificates.read"), func(c *fiber.Ctx) error {
		var filter store.MTLSCertificateFilter
		if t := c.Query("type"); t != "" {
			ct := store.MTLSCertType(t)
			filter.CertType = &ct
		}
		if n := c.Query("nodeId"); n != "" {
			filter.NodeID = &n
		}
		filter.Limit = c.QueryInt("limit", 50)
		filter.Offset = c.QueryInt("offset", 0)

		certs, err := certSvc.ListCerts(c.Context(), filter)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": certs})
	})

	mtls.Post("/certificates/:id/revoke", mutationLimiter, requireRole("admin"), requireAdminScope("certificates.write"), func(c *fiber.Ctx) error {
		if err := certSvc.RevokeCert(c.Context(), c.Params("id")); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	mtls.Get("/certificates/:id", requireRole("admin"), requireAdminScope("certificates.read"), func(c *fiber.Ctx) error {
		cert, err := certSvc.GetCert(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": cert})
	})

	mtls.Get("/status", requireRole("admin"), requireAdminScope("certificates.read"), func(c *fiber.Ctx) error {
		status, err := certSvc.GetStatus(c.Context())
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": status})
	})

	mtls.Get("/migration/status", requireRole("admin"), requireAdminScope("certificates.read"), func(c *fiber.Ctx) error {
		if migrator == nil {
			return c.JSON(fiber.Map{"data": fiber.Map{"migrationEnabled": false}})
		}
		status, err := migrator.Status(c.Context())
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": status})
	})

	mtls.Post("/migration/run", mutationLimiter, requireRole("admin"), requireAdminScope("certificates.write"), func(c *fiber.Ctx) error {
		if migrator == nil {
			return c.Status(400).JSON(fiber.Map{"error": "migration service not available"})
		}
		if err := migrator.Run(c.Context()); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true})
	})
}
