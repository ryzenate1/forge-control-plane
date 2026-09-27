package http

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	"gamepanel/forge/internal/services/acme"

	"github.com/gofiber/fiber/v2"
)

// registerCertificateRoutesExt owns the operator-supplied certificate surface
// (/upload, /:id/download, /:id/export). Layering: handler -> acme.Service ->
// store. The guard matches registerCertificateRoutes (svc == nil): without the
// service neither route set registers, so /cert and /certificates never
// diverge into one live and one dead copy.
func registerCertificateRoutesExt(protected fiber.Router, cfg Config, svc *acme.Service, adminIPAccess, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}
	_ = cfg

	certs := protected.Group("/certificates", adminIPAccess)

	certs.Post("/upload", mutationLimiter, requireRole("admin"), requireAdminScope("certificates.write"), func(c *fiber.Ctx) error {
		var body struct {
			Certificate string `json:"certificate"`
			PrivateKey  string `json:"privateKey"`
			Chain       string `json:"chain,omitempty"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		if body.Certificate == "" || body.PrivateKey == "" {
			return c.Status(400).JSON(fiber.Map{"error": "certificate and privateKey are required"})
		}

		ctx, cancel := requestContext()
		defer cancel()
		cert, err := svc.ImportManualCertificate(ctx, body.Certificate, body.PrivateKey, body.Chain)
		if err != nil {
			// Validation failures are client errors; anything else is a store
			// failure. The service prefixes validation with invalid/key-pair
			// language, so match on that rather than echoing internals.
			msg := err.Error()
			if strings.Contains(msg, "invalid certificate") || strings.Contains(msg, "key pair mismatch") ||
				strings.Contains(msg, "no valid PEM") || strings.Contains(msg, "no private key") ||
				strings.Contains(msg, "do not match") || strings.Contains(msg, "expired") {
				return c.Status(400).JSON(fiber.Map{"error": msg})
			}
			return respondInternalError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"data": cert})
	})

	certs.Get("/:id/download", requireRole("admin"), requireAdminScope("certificates.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		cert, err := svc.GetCertificate(ctx, c.Params("id"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}

		filename := fmt.Sprintf("certificate-%s.pem", strings.Split(c.Params("id"), "-")[0])
		c.Set("Content-Type", "application/x-pem-file")
		c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		return c.SendString(cert.Certificate)
	})

	certs.Post("/:id/export", mutationLimiter, requireRole("admin"), requireAdminScope("certificates.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		cert, err := svc.ExportCertificate(ctx, c.Params("id"))
		if err != nil {
			if strings.Contains(err.Error(), "private key not available") {
				return c.Status(400).JSON(fiber.Map{"error": "private key not available for export"})
			}
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{
			"data": fiber.Map{
				"certificate": cert.Certificate,
				"privateKey":  cert.PrivateKey,
			},
		})
	})
}

// validateCertificatePEM and validateKeyPair remain as the shared PEM helpers
// for the proxy-domain import route (handlers_proxy_domains.go), which is
// store-backed by design: it binds an operator certificate to a proxy domain
// row rather than the ACME certificate lifecycle. They delegate to the same
// rules the acme service enforces so both imports agree.

func validateCertificatePEM(certPEM string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("no valid PEM certificate found")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}
	if time.Now().After(cert.NotAfter) {
		return nil, errors.New("certificate has expired")
	}
	return cert, nil
}

func validateKeyPair(certPEM, keyPEM string) error {
	certBlock, _ := pem.Decode([]byte(certPEM))
	if certBlock == nil {
		return errors.New("no certificate PEM data")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return err
	}

	keyBlock, _ := pem.Decode([]byte(keyPEM))
	if keyBlock == nil {
		return errors.New("no private key PEM data")
	}

	privKey, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		privKey, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
		if err != nil {
			privKey, err = x509.ParseECPrivateKey(keyBlock.Bytes)
			if err != nil {
				return fmt.Errorf("parse private key: %w", err)
			}
		}
	}

	certPubKey, ok := cert.PublicKey.(crypto.PublicKey)
	if !ok {
		return errors.New("invalid certificate public key type")
	}

	privPubKey, ok := privKey.(interface{ Public() crypto.PublicKey })
	if !ok {
		return errors.New("invalid private key type")
	}

	certPubKeyBytes, err := x509.MarshalPKIXPublicKey(certPubKey)
	if err != nil {
		return err
	}
	privPubKeyBytes, err := x509.MarshalPKIXPublicKey(privPubKey.Public())
	if err != nil {
		return err
	}

	if string(certPubKeyBytes) != string(privPubKeyBytes) {
		return errors.New("certificate and private key do not match")
	}
	return nil
}
