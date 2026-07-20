package http

import (
	"gamepanel/forge/internal/services/acme"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerCertificateRoutes(protected fiber.Router, cfg Config, svc *acme.Service, adminIPAccess, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	certs := protected.Group("/certificates", adminIPAccess)

	certs.Post("/issue", mutationLimiter, requireRole("admin"), requireAdminScope("certificates.write"), func(c *fiber.Ctx) error {
		var req acme.IssueCertificateRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		cert, err := svc.IssueCertificate(c.Context(), req)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"data": cert})
	})

	certs.Get("/", requireRole("admin"), requireAdminScope("certificates.read"), func(c *fiber.Ctx) error {
		var filter store.CertificateFilter
		if p := c.Query("provider"); p != "" {
			filter.Provider = &p
		}
		if s := c.Query("status"); s != "" {
			filter.Status = &s
		}
		if w := c.Query("wildcard"); w == "true" {
			t := true
			filter.Wildcard = &t
		}
		if w := c.Query("wildcard"); w == "false" {
			f := false
			filter.Wildcard = &f
		}
		filter.Limit = c.QueryInt("limit", 50)
		filter.Offset = c.QueryInt("offset", 0)

		certs, err := svc.ListCertificates(c.Context(), filter)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": certs})
	})

	certs.Get("/:id", requireRole("admin"), requireAdminScope("certificates.read"), func(c *fiber.Ctx) error {
		cert, err := svc.GetCertificate(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": cert})
	})

	certs.Delete("/:id", mutationLimiter, requireRole("admin"), requireAdminScope("certificates.write"), func(c *fiber.Ctx) error {
		if err := svc.RevokeCertificate(c.Context(), c.Params("id")); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(204)
	})

	certs.Post("/:id/renew", mutationLimiter, requireRole("admin"), requireAdminScope("certificates.write"), func(c *fiber.Ctx) error {
		cert, err := svc.RenewCertificate(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": cert})
	})
}
