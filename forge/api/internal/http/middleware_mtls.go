package http

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"github.com/gofiber/fiber/v2"
)

type MTLSAuthConfig struct {
	Enabled    bool
	CACertPath string
	CertPath   string
	KeyPath    string
	DevBypass  bool
}

func MTLSAuthMiddleware(cfg MTLSAuthConfig) fiber.Handler {
	if !cfg.Enabled || cfg.DevBypass {
		return func(c *fiber.Ctx) error {
			if cfg.DevBypass {
				c.Locals("mtlsNodeID", "dev-bypass")
				c.Locals("mtlsAuthenticated", true)
			}
			return c.Next()
		}
	}

	caPool, err := loadCACertPool(cfg.CACertPath)
	if err != nil {
		panic(fmt.Sprintf("mTLS: failed to load CA cert: %v", err))
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

		opts := x509.VerifyOptions{
			Roots:     caPool,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}

		if _, err := clientCert.Verify(opts); err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "client certificate verification failed")
		}

		nodeID := clientCert.Subject.CommonName
		if nodeID == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "client certificate missing common name")
		}

		c.Locals("mtlsNodeID", nodeID)
		c.Locals("mtlsAuthenticated", true)
		c.Locals("mtlsCertSerial", clientCert.SerialNumber.String())
		c.Locals("mtlsCertOrg", clientCert.Subject.Organization)

		return c.Next()
	}
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
