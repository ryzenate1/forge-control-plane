package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
)

type MTLSConfig struct {
	Enabled    bool
	CACertPath string
	CertPath   string
	KeyPath    string
	DevBypass  bool
}

type mtlsContextKey string

const (
	panelCNKey mtlsContextKey = "mtls-panel-cn"
)

func PanelCNFromContext(ctx context.Context) string {
	cn, _ := ctx.Value(panelCNKey).(string)
	return cn
}

func NewMTLSAuthMiddleware(cfg MTLSConfig) func(http.Handler) http.Handler {
	if !cfg.Enabled || cfg.DevBypass {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if cfg.DevBypass {
					ctx := context.WithValue(r.Context(), panelCNKey, "dev-bypass")
					r = r.WithContext(ctx)
				}
				next.ServeHTTP(w, r)
			})
		}
	}

	caPool, err := loadPool(cfg.CACertPath)
	if err != nil {
		panic(fmt.Sprintf("mTLS: failed to load CA cert: %v", err))
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" || r.URL.Path == "/metrics" || r.URL.Path == "/ready" {
				next.ServeHTTP(w, r)
				return
			}

			if r.TLS == nil {
				http.Error(w, "mTLS requires HTTPS", http.StatusBadRequest)
				return
			}

			if len(r.TLS.PeerCertificates) == 0 {
				http.Error(w, "no client certificate provided", http.StatusUnauthorized)
				return
			}

			clientCert := r.TLS.PeerCertificates[0]

			opts := x509.VerifyOptions{
				Roots:     caPool,
				KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			}

			if _, err := clientCert.Verify(opts); err != nil {
				http.Error(w, "client certificate verification failed", http.StatusUnauthorized)
				return
			}

			panelCN := clientCert.Subject.CommonName
			if panelCN == "" {
				http.Error(w, "client certificate missing common name", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), panelCNKey, panelCN)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func loadPool(caCertPath string) (*x509.CertPool, error) {
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

func NewMTLSServerTLSConfig(cfg MTLSConfig) (*tls.Config, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	caData, err := os.ReadFile(cfg.CACertPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caData) {
		return nil, errors.New("no CA certificates found in PEM data")
	}

	certData, err := os.ReadFile(cfg.CertPath)
	if err != nil {
		return nil, fmt.Errorf("read server cert: %w", err)
	}

	keyData, err := os.ReadFile(cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("read server key: %w", err)
	}

	cert, err := tls.X509KeyPair(certData, keyData)
	if err != nil {
		return nil, fmt.Errorf("parse server key pair: %w", err)
	}

	log.Printf("[mtls] mTLS server configured with CA: %s", cfg.CACertPath)

	return &tls.Config{
		RootCAs:      caPool,
		ClientCAs:    caPool,
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}, nil
}
