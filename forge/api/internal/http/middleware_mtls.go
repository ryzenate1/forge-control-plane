package http

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"

	"os"

	"github.com/gofiber/fiber/v2"
)

type MTLSAuthConfig struct {
	Enabled          bool
	CACertPath       string
	CertPath         string
	KeyPath          string
	DevBypass        bool
	RevocationLookup func(context.Context, string) (bool, error)
}

var errMTLSCertificateRevoked = errors.New("client certificate has been revoked")

func MTLSAuthMiddleware(cfg MTLSAuthConfig) fiber.Handler {
	// Issue 26 (CRITICAL): DevBypass must never be used in production.
	if cfg.DevBypass {
		slog.Error("CRITICAL: mTLS DevBypass is active - all requests are treated as authenticated! This must NEVER be enabled in production.")
		if os.Getenv("FORGE_ENV") == "production" {
			panic("mTLS DevBypass must not be enabled in production (FORGE_ENV=production)")
		}
	}

	if !cfg.Enabled || cfg.DevBypass {
		return func(c *fiber.Ctx) error {
			if cfg.DevBypass {
				c.Locals("mtlsNodeID", "dev-bypass")
				c.Locals("mtlsAuthenticated", true)
			}
			return c.Next()
		}
	}
	if cfg.RevocationLookup == nil {
		slog.Error("mTLS: certificate revocation lookup is unavailable")
		return func(c *fiber.Ctx) error {
			return fiber.NewError(fiber.StatusServiceUnavailable, "mTLS revocation verification unavailable")
		}
	}

	caPool, err := loadCACertPool(cfg.CACertPath)
	if err != nil {
		slog.Error("mTLS: failed to load CA cert, disabling mTLS", "error", err)
		return func(c *fiber.Ctx) error {
			return fiber.NewError(fiber.StatusServiceUnavailable, "mTLS CA certificate unavailable")
		}
	}

	return func(c *fiber.Ctx) error {
		if c.Protocol() != "https" {
			return fiber.NewError(fiber.StatusBadRequest, "mTLS requires HTTPS")
		}

		conn := c.Context().Conn()
		if conn == nil {
			return fiber.NewError(fiber.StatusBadRequest, "no TLS connection")
		}

		tlsConn, ok := conn.(*tls.Conn)
		if !ok {
			return fiber.NewError(fiber.StatusBadRequest, "not a TLS connection")
		}

		state := tlsConn.ConnectionState()
		if len(state.PeerCertificates) == 0 {
			return fiber.NewError(fiber.StatusUnauthorized, "no client certificate provided")
		}

		clientCert := state.PeerCertificates[0]

		// Issue 27 (HIGH): Validate intermediate certificate chain.
		intermediates := x509.NewCertPool()
		for _, cert := range state.PeerCertificates[1:] {
			intermediates.AddCert(cert)
		}

		opts := x509.VerifyOptions{
			Roots:         caPool,
			Intermediates: intermediates,
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}

		if _, err := clientCert.Verify(opts); err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "client certificate verification failed")
		}
		if err := verifyMTLSRevocation(c.UserContext(), clientCert.SerialNumber.String(), cfg.RevocationLookup); err != nil {
			if errors.Is(err, errMTLSCertificateRevoked) {
				return fiber.NewError(fiber.StatusUnauthorized, err.Error())
			}
			slog.Error("mTLS certificate revocation lookup failed", "error", err)
			return fiber.NewError(fiber.StatusServiceUnavailable, "mTLS revocation verification failed")
		}

		// Issue 28 (MEDIUM): Prefer SANs over deprecated CommonName for identity.
		nodeID := ""
		for _, uri := range clientCert.URIs {
			if uri.Scheme == "forge-node" {
				nodeID = uri.Host
				break
			}
		}
		if nodeID == "" && len(clientCert.DNSNames) > 0 {
			nodeID = clientCert.DNSNames[0]
		}
		if nodeID == "" {
			nodeID = clientCert.Subject.CommonName
		}
		if nodeID == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "client certificate missing identity (no SAN or CN)")
		}

		c.Locals("mtlsNodeID", nodeID)
		c.Locals("mtlsAuthenticated", true)
		c.Locals("mtlsCertSerial", clientCert.SerialNumber.String())
		c.Locals("mtlsCertOrg", clientCert.Subject.Organization)

		return c.Next()
	}
}

func verifyMTLSRevocation(ctx context.Context, serial string, lookup func(context.Context, string) (bool, error)) error {
	if lookup == nil {
		return errors.New("mTLS revocation lookup is unavailable")
	}
	revoked, err := lookup(ctx, serial)
	if err != nil {
		return fmt.Errorf("mTLS revocation lookup: %w", err)
	}
	if revoked {
		return errMTLSCertificateRevoked
	}
	return nil
}

func loadCACertPool(caCertPath string) (*x509.CertPool, error) {
	caData, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caData) {
		return nil, errors.New("no CA certificates found in PEM data")
	}

	return pool, nil
}
