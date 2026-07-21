package http

import (
	"gamepanel/forge/internal/store"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func registerProxyDomainRoutes(protected fiber.Router, cfg Config, adminIPAccess, mutationLimiter fiber.Handler) {
	if cfg.Store == nil {
		return
	}

	domains := protected.Group("/domains", adminIPAccess)

	domains.Post("/", mutationLimiter, requireRole("admin"), requireAdminScope("domains.write"), func(c *fiber.Ctx) error {
		var req struct {
			Hostname       string `json:"hostname"`
			ServiceID      string `json:"serviceId"`
			ServiceType    string `json:"serviceType"`
			Port           int    `json:"port"`
			Path           string `json:"path"`
			StripPath      bool   `json:"stripPath"`
			CertType       string `json:"certType"`
			CertData       string `json:"certData"`
			CertKey        string `json:"certKey"`
			AutoRenew      bool   `json:"autoRenew"`
			ForwardAuthURL string `json:"forwardAuthUrl"`
			WebSocket      bool   `json:"websocket"`
			RateLimit      int    `json:"rateLimit"`
			RateLimitBurst int    `json:"rateLimitBurst"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		if req.Hostname == "" {
			return c.Status(400).JSON(fiber.Map{"error": "hostname is required"})
		}
		if req.Port == 0 {
			req.Port = 8080
		}
		if req.Path == "" {
			req.Path = "/"
		}
		if req.ServiceType == "" {
			req.ServiceType = "server"
		}
		if req.CertType == "" {
			req.CertType = "none"
		}

		domain := store.ProxyDomain{
			ID:               uuid.NewString(),
			Hostname:         req.Hostname,
			ServiceID:        req.ServiceID,
			ServiceType:      req.ServiceType,
			HTTPS:            req.CertType != "none",
			Port:             req.Port,
			CertType:         req.CertType,
			CertData:         req.CertData,
			CertKey:          req.CertKey,
			AutoRenew:        req.AutoRenew,
			Path:             req.Path,
			StripPath:        req.StripPath,
			ForwardAuthURL:   req.ForwardAuthURL,
			WebSocket:        req.WebSocket,
			RateLimit:        req.RateLimit,
			RateLimitBurst:   req.RateLimitBurst,
		}

		result, err := cfg.Store.CreateProxyDomain(c.Context(), domain)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"data": result})
	})

	domains.Get("/", requireRole("admin"), requireAdminScope("domains.read"), func(c *fiber.Ctx) error {
		var filter store.ProxyDomainFilter
		if s := c.Query("serviceId"); s != "" {
			filter.ServiceID = &s
		}
		if s := c.Query("serviceType"); s != "" {
			filter.ServiceType = &s
		}
		if s := c.Query("certType"); s != "" {
			filter.CertType = &s
		}
		filter.Limit = c.QueryInt("limit", 100)
		filter.Offset = c.QueryInt("offset", 0)

		results, err := cfg.Store.ListProxyDomains(c.Context(), filter)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": results})
	})

	domains.Get("/:id", requireRole("admin"), requireAdminScope("domains.read"), func(c *fiber.Ctx) error {
		d, err := cfg.Store.GetProxyDomain(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if d == nil {
			return c.Status(404).JSON(fiber.Map{"error": "domain not found"})
		}
		return c.JSON(fiber.Map{"data": d})
	})

	domains.Put("/:id", mutationLimiter, requireRole("admin"), requireAdminScope("domains.write"), func(c *fiber.Ctx) error {
		existing, err := cfg.Store.GetProxyDomain(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if existing == nil {
			return c.Status(404).JSON(fiber.Map{"error": "domain not found"})
		}

		var req struct {
			Hostname       *string `json:"hostname"`
			ServiceID      *string `json:"serviceId"`
			ServiceType    *string `json:"serviceType"`
			Port           *int    `json:"port"`
			Path           *string `json:"path"`
			StripPath      *bool   `json:"stripPath"`
			CertType       *string `json:"certType"`
			CertData       *string `json:"certData"`
			CertKey        *string `json:"certKey"`
			AutoRenew      *bool   `json:"autoRenew"`
			HTTPS          *bool   `json:"https"`
			ForwardAuthURL *string `json:"forwardAuthUrl"`
			WebSocket      *bool   `json:"websocket"`
			RateLimit      *int    `json:"rateLimit"`
			RateLimitBurst *int    `json:"rateLimitBurst"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}

		if req.Hostname != nil {
			existing.Hostname = *req.Hostname
		}
		if req.ServiceID != nil {
			existing.ServiceID = *req.ServiceID
		}
		if req.ServiceType != nil {
			existing.ServiceType = *req.ServiceType
		}
		if req.Port != nil {
			existing.Port = *req.Port
		}
		if req.Path != nil {
			existing.Path = *req.Path
		}
		if req.StripPath != nil {
			existing.StripPath = *req.StripPath
		}
		if req.CertType != nil {
			existing.CertType = *req.CertType
		}
		if req.CertData != nil {
			existing.CertData = *req.CertData
		}
		if req.CertKey != nil {
			existing.CertKey = *req.CertKey
		}
		if req.AutoRenew != nil {
			existing.AutoRenew = *req.AutoRenew
		}
		if req.HTTPS != nil {
			existing.HTTPS = *req.HTTPS
		}
		if req.ForwardAuthURL != nil {
			existing.ForwardAuthURL = *req.ForwardAuthURL
		}
		if req.WebSocket != nil {
			existing.WebSocket = *req.WebSocket
		}
		if req.RateLimit != nil {
			existing.RateLimit = *req.RateLimit
		}
		if req.RateLimitBurst != nil {
			existing.RateLimitBurst = *req.RateLimitBurst
		}

		if err := cfg.Store.UpdateProxyDomain(c.Context(), *existing); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": existing})
	})

	domains.Delete("/:id", mutationLimiter, requireRole("admin"), requireAdminScope("domains.write"), func(c *fiber.Ctx) error {
		if err := cfg.Store.DeleteProxyDomain(c.Context(), c.Params("id")); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(204)
	})

	domains.Post("/:id/verify", mutationLimiter, requireRole("admin"), requireAdminScope("domains.write"), func(c *fiber.Ctx) error {
		d, err := cfg.Store.GetProxyDomain(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if d == nil {
			return c.Status(404).JSON(fiber.Map{"error": "domain not found"})
		}
		return c.JSON(fiber.Map{"data": fiber.Map{
			"id":       d.ID,
			"hostname": d.Hostname,
			"verified": true,
		}})
	})
}

func registerProxyCertificateRoutes(protected fiber.Router, cfg Config, adminIPAccess, mutationLimiter fiber.Handler) {
	if cfg.Store == nil {
		return
	}

	certs := protected.Group("/certificates", adminIPAccess)

	certs.Post("/", mutationLimiter, requireRole("admin"), requireAdminScope("certificates.write"), func(c *fiber.Ctx) error {
		var req struct {
			DomainID    string `json:"domainId"`
			Domains     []string `json:"domains"`
			Certificate string `json:"certificate"`
			PrivateKey  string `json:"privateKey"`
			Issuer      string `json:"issuer"`
			AutoRenew   bool   `json:"autoRenew"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		if req.DomainID == "" {
			return c.Status(400).JSON(fiber.Map{"error": "domainId is required"})
		}

		domains := req.Domains
		if len(domains) == 0 {
			if d, err := cfg.Store.GetProxyDomain(c.Context(), req.DomainID); err == nil && d != nil {
				domains = []string{d.Hostname}
			} else {
				return c.Status(400).JSON(fiber.Map{"error": "domainId does not resolve to a valid domain"})
			}
		}

		createReq := store.CreateCertificateRequest{
			Domains:     domains,
			Issuer:      req.Issuer,
			Certificate: req.Certificate,
			PrivateKey:  req.PrivateKey,
			AutoRenew:   req.AutoRenew,
			Provider:    "custom",
		}

		cert, err := cfg.Store.CreateCertificate(c.Context(), createReq)
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
		filter.Limit = c.QueryInt("limit", 50)
		filter.Offset = c.QueryInt("offset", 0)

		results, err := cfg.Store.ListCertificates(c.Context(), filter)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": results})
	})

	certs.Get("/:id", requireRole("admin"), requireAdminScope("certificates.read"), func(c *fiber.Ctx) error {
		cert, err := cfg.Store.GetCertificate(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": cert})
	})

	certs.Delete("/:id", mutationLimiter, requireRole("admin"), requireAdminScope("certificates.write"), func(c *fiber.Ctx) error {
		if err := cfg.Store.DeleteCertificate(c.Context(), c.Params("id")); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(204)
	})

	certs.Post("/:id/renew", mutationLimiter, requireRole("admin"), requireAdminScope("certificates.write"), func(c *fiber.Ctx) error {
		if cfg.AcmeService == nil {
			return c.Status(503).JSON(fiber.Map{"error": "ACME service not available"})
		}
		cert, err := cfg.AcmeService.RenewCertificate(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": cert})
	})
}

func registerSecurityHeadersRoutes(protected fiber.Router, cfg Config, adminIPAccess, mutationLimiter fiber.Handler) {
	if cfg.Store == nil {
		return
	}

	headers := protected.Group("/domains/:domainId/security-headers", adminIPAccess)

	headers.Get("/", requireRole("admin"), requireAdminScope("domains.read"), func(c *fiber.Ctx) error {
		h, err := cfg.Store.GetSecurityHeadersByDomain(c.Context(), c.Params("domainId"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": h})
	})

	headers.Post("/", mutationLimiter, requireRole("admin"), requireAdminScope("domains.write"), func(c *fiber.Ctx) error {
		var h store.SecurityHeaderConfig
		if err := c.BodyParser(&h); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		h.DomainID = c.Params("domainId")

		result, err := cfg.Store.CreateSecurityHeaders(c.Context(), h)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"data": result})
	})

	headers.Put("/:id", mutationLimiter, requireRole("admin"), requireAdminScope("domains.write"), func(c *fiber.Ctx) error {
		var h store.SecurityHeaderConfig
		if err := c.BodyParser(&h); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		h.ID = c.Params("id")

		if err := cfg.Store.UpdateSecurityHeaders(c.Context(), h); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": h})
	})

	headers.Delete("/:id", mutationLimiter, requireRole("admin"), requireAdminScope("domains.write"), func(c *fiber.Ctx) error {
		if err := cfg.Store.DeleteSecurityHeaders(c.Context(), c.Params("id")); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(204)
	})
}

func registerRedirectRulesRoutes(protected fiber.Router, cfg Config, adminIPAccess, mutationLimiter fiber.Handler) {
	if cfg.Store == nil {
		return
	}

	redirects := protected.Group("/domains/:domainId/redirects", adminIPAccess)

	redirects.Get("/", requireRole("admin"), requireAdminScope("domains.read"), func(c *fiber.Ctx) error {
		domainID := c.Params("domainId")
		filter := store.RedirectRuleFilter{DomainID: &domainID}
		rules, err := cfg.Store.ListRedirectRules(c.Context(), filter)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": rules})
	})

	redirects.Post("/", mutationLimiter, requireRole("admin"), requireAdminScope("domains.write"), func(c *fiber.Ctx) error {
		var rule store.RedirectRule
		if err := c.BodyParser(&rule); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		rule.DomainID = c.Params("domainId")

		result, err := cfg.Store.CreateRedirectRule(c.Context(), rule)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"data": result})
	})

	redirects.Put("/:id", mutationLimiter, requireRole("admin"), requireAdminScope("domains.write"), func(c *fiber.Ctx) error {
		var rule store.RedirectRule
		if err := c.BodyParser(&rule); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		rule.ID = c.Params("id")

		if err := cfg.Store.UpdateRedirectRule(c.Context(), rule); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": rule})
	})

	redirects.Delete("/:id", mutationLimiter, requireRole("admin"), requireAdminScope("domains.write"), func(c *fiber.Ctx) error {
		if err := cfg.Store.DeleteRedirectRule(c.Context(), c.Params("id")); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(204)
	})
}
