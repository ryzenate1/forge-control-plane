package http

import (
	"context"
	"fmt"
	"net"
	"time"

	"gamepanel/forge/internal/services/acme"
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

		// Provision TLS before persisting the row: a domain that claims a
		// certificate must actually have one configured in the gateway, or the
		// request fails rather than recording a false completion.
		if cfg.CaddyTLS != nil {
			switch req.CertType {
			case "letsencrypt":
				if err := cfg.CaddyTLS.ProvisionLetsEncrypt(c.Context(), &domain, ""); err != nil {
					return fiber.NewError(fiber.StatusBadGateway, "tls provisioning failed: "+err.Error())
				}
			case "custom":
				if err := cfg.CaddyTLS.UploadCustomCert(c.Context(), &domain); err != nil {
					return fiber.NewError(fiber.StatusBadGateway, "certificate upload failed: "+err.Error())
				}
			}
		}

		result, err := cfg.Store.CreateProxyDomain(c.Context(), domain)
		if err != nil {
			return respondInternalError(c, err)
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
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": results})
	})

	domains.Get("/:id", requireRole("admin"), requireAdminScope("domains.read"), func(c *fiber.Ctx) error {
		d, err := cfg.Store.GetProxyDomain(c.Context(), c.Params("id"))
		if err != nil {
			return respondInternalError(c, err)
		}
		if d == nil {
			return c.Status(404).JSON(fiber.Map{"error": "domain not found"})
		}
		return c.JSON(fiber.Map{"data": d})
	})

	domains.Put("/:id", mutationLimiter, requireRole("admin"), requireAdminScope("domains.write"), func(c *fiber.Ctx) error {
		existing, err := cfg.Store.GetProxyDomain(c.Context(), c.Params("id"))
		if err != nil {
			return respondInternalError(c, err)
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

		if cfg.CaddyTLS != nil && req.CertType != nil {
			switch existing.CertType {
			case "letsencrypt":
				if err := cfg.CaddyTLS.ProvisionLetsEncrypt(c.Context(), existing, ""); err != nil {
					return fiber.NewError(fiber.StatusBadGateway, "tls provisioning failed: "+err.Error())
				}
			case "custom":
				if err := cfg.CaddyTLS.UploadCustomCert(c.Context(), existing); err != nil {
					return fiber.NewError(fiber.StatusBadGateway, "certificate upload failed: "+err.Error())
				}
			case "none":
				if err := cfg.CaddyTLS.RemoveCert(c.Context(), existing.Hostname); err != nil {
					return fiber.NewError(fiber.StatusBadGateway, "tls removal failed: "+err.Error())
				}
				existing.HTTPS = false
			}
		}

		if err := cfg.Store.UpdateProxyDomain(c.Context(), *existing); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": existing})
	})

	domains.Delete("/:id", mutationLimiter, requireRole("admin"), requireAdminScope("domains.write"), func(c *fiber.Ctx) error {
		if cfg.CaddyTLS != nil {
			if d, err := cfg.Store.GetProxyDomain(c.Context(), c.Params("id")); err == nil && d != nil && d.CertType == "letsencrypt" {
				if rmErr := cfg.CaddyTLS.RemoveCert(c.Context(), d.Hostname); rmErr != nil {
					return fiber.NewError(fiber.StatusBadGateway, "tls removal failed: "+rmErr.Error())
				}
			}
		}
		if err := cfg.Store.DeleteProxyDomain(c.Context(), c.Params("id")); err != nil {
			return respondInternalError(c, err)
		}
		return c.SendStatus(204)
	})

	domains.Post("/:id/verify", mutationLimiter, requireRole("admin"), requireAdminScope("domains.write"), func(c *fiber.Ctx) error {
		d, err := cfg.Store.GetProxyDomain(c.Context(), c.Params("id"))
		if err != nil {
			return respondInternalError(c, err)
		}
		if d == nil {
			return c.Status(404).JSON(fiber.Map{"error": "domain not found"})
		}
		// Real verification: the hostname must actually resolve in DNS. This used to
		// return verified:true unconditionally — a false completion.
		verified := false
		var resolved []string
		vctx, vcancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer vcancel()
		if addrs, lookupErr := net.DefaultResolver.LookupHost(vctx, d.Hostname); lookupErr == nil && len(addrs) > 0 {
			verified = true
			resolved = addrs
		}
		return c.JSON(fiber.Map{"data": fiber.Map{
			"id":        d.ID,
			"hostname":  d.Hostname,
			"verified":  verified,
			"addresses": resolved,
		}})
	})
}

// registerProxyCertificateRoutes owns one route: importing an operator-supplied
// certificate and binding it to a proxy domain.
//
// Guard note (unified with handlers_certificates.go / _ext.go): the ACME
// certificate routes guard on AcmeService == nil because they delegate to
// acme.Service. This import guards on Store == nil instead because it is
// store-backed by design — it binds PEM material to a proxy-domain row, not
// to the ACME lifecycle — and documents that boundary rather than taking a
// service it does not use.
//
// It used to also register GET /certificates, GET /certificates/:id,
// DELETE /certificates/:id and POST /certificates/:id/renew. All four were
// dead: registerCertificateRoutes (handlers_certificates.go, server.go:2562)
// claims the same paths and Fiber resolves overlapping routes in registration
// order, so this file — registered at server.go:2634 — never answered them.
// Its guard is cfg.AcmeService, which main.go:1149 constructs unconditionally,
// so there was no fallback case either.
//
// Removing them also removes a weaker implementation of DELETE: this copy
// called cfg.Store.DeleteCertificate directly, dropping the database row while
// leaving the certificate valid at the CA. The registration that actually
// serves calls svc.RevokeCertificate, which revokes first. Likewise its GET /
// filter understood only provider/limit/offset, where the live one also
// handles status and wildcard.
func registerProxyCertificateRoutes(protected fiber.Router, cfg Config, adminIPAccess, mutationLimiter fiber.Handler) {
	if cfg.Store == nil {
		return
	}

	certs := protected.Group("/certificates", adminIPAccess)

	// POST /certificates — import a certificate the operator already holds and
	// attach it to a proxy domain. This is the only certificate route in this
	// file that is reachable; POST /certificates/upload
	// (handlers_certificates_ext.go) is the equivalent for a certificate that
	// is not tied to a proxy domain.
	certs.Post("/", mutationLimiter, requireRole("admin"), requireAdminScope("certificates.write"), func(c *fiber.Ctx) error {
		var req struct {
			DomainID    string   `json:"domainId"`
			Domains     []string `json:"domains"`
			Certificate string   `json:"certificate"`
			PrivateKey  string   `json:"privateKey"`
			Issuer      string   `json:"issuer"`
			AutoRenew   bool     `json:"autoRenew"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		if req.DomainID == "" {
			return c.Status(400).JSON(fiber.Map{"error": "domainId is required"})
		}
		if req.Certificate == "" || req.PrivateKey == "" {
			return c.Status(400).JSON(fiber.Map{"error": "certificate and privateKey are required"})
		}

		// This route used to store whatever bytes it was handed, unparsed and
		// with a zero ExpiresAt. That is what expires_at in the certificates
		// table means, and both the "expiring soon" filter and
		// FindExpiringCertificates compare against it — so every certificate
		// imported here was permanently reported as long expired. Parse the PEM
		// and take the real NotAfter, the same way /certificates/upload does.
		certData, err := validateCertificatePEM(req.Certificate)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("invalid certificate: %v", err)})
		}
		if err := validateKeyPair(req.Certificate, req.PrivateKey); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("key pair mismatch: %v", err)})
		}

		ctx, cancel := requestContext()
		defer cancel()

		domains := req.Domains
		if len(domains) == 0 {
			d, domErr := cfg.Store.GetProxyDomain(ctx, req.DomainID)
			if domErr != nil || d == nil {
				return c.Status(400).JSON(fiber.Map{"error": "domainId does not resolve to a valid domain"})
			}
			domains = []string{d.Hostname}
		}

		issuer := req.Issuer
		if issuer == "" {
			issuer = certData.Issuer.String()
		}

		createReq := store.CreateCertificateRequest{
			Domains:     domains,
			Issuer:      issuer,
			Certificate: req.Certificate,
			PrivateKey:  req.PrivateKey,
			ExpiresAt:   certData.NotAfter,
			// An imported certificate cannot be reissued by Forge: there is no
			// ACME order behind it. Honouring autoRenew here would enrol it in
			// the renewal sweep, which would order a replacement from a public
			// CA and overwrite the operator's own material.
			// acme.IsACMEProvider gates that, and this provider is outside the
			// ACME set, so record the request and leave the flag off.
			AutoRenew: false,
			Provider:  acme.ProviderCustom,
		}

		cert, err := cfg.Store.CreateCertificate(ctx, createReq)
		if err != nil {
			return respondInternalError(c, err)
		}
		if req.AutoRenew {
			return c.Status(201).JSON(fiber.Map{
				"data":    cert,
				"warning": "autoRenew was ignored: an imported certificate has no ACME order behind it and must be replaced by uploading a new one",
			})
		}
		return c.Status(201).JSON(fiber.Map{"data": cert})
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
			return respondInternalError(c, err)
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
			return respondInternalError(c, err)
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
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": h})
	})

	headers.Delete("/:id", mutationLimiter, requireRole("admin"), requireAdminScope("domains.write"), func(c *fiber.Ctx) error {
		if err := cfg.Store.DeleteSecurityHeaders(c.Context(), c.Params("id")); err != nil {
			return respondInternalError(c, err)
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
			return respondInternalError(c, err)
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
			return respondInternalError(c, err)
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
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": rule})
	})

	redirects.Delete("/:id", mutationLimiter, requireRole("admin"), requireAdminScope("domains.write"), func(c *fiber.Ctx) error {
		if err := cfg.Store.DeleteRedirectRule(c.Context(), c.Params("id")); err != nil {
			return respondInternalError(c, err)
		}
		return c.SendStatus(204)
	})
}
