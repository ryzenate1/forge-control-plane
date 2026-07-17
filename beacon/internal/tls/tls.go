package tls

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
)

type TLSMode string

const (
	ModeNone    TLSMode = "none"
	ModeManual  TLSMode = "manual"
	ModeAutoTLS TLSMode = "autotls"
)

type Config struct {
	Mode      TLSMode
	CertFile  string
	KeyFile   string
	Hostname  string
	CacheDir  string
	ACMEEmail string
}

func DefaultTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
		},
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
		},
		NextProtos: []string{"h2", "http/1.1"},
	}
}

func (c *Config) Validate() error {
	switch c.Mode {
	case ModeNone:
		return nil
	case ModeManual:
		if c.CertFile == "" {
			return errors.New("cert_file is required for manual TLS mode")
		}
		if c.KeyFile == "" {
			return errors.New("key_file is required for manual TLS mode")
		}
		if _, err := os.Stat(c.CertFile); err != nil {
			return fmt.Errorf("cert_file not accessible: %w", err)
		}
		if _, err := os.Stat(c.KeyFile); err != nil {
			return fmt.Errorf("key_file not accessible: %w", err)
		}
		return nil
	case ModeAutoTLS:
		if c.Hostname == "" {
			return errors.New("hostname is required for autotls mode")
		}
		return nil
	default:
		return fmt.Errorf("unknown TLS mode: %s", c.Mode)
	}
}

func (c *Config) Apply(server *http.Server) error {
	if err := c.Validate(); err != nil {
		return err
	}

	switch c.Mode {
	case ModeNone:
		server.TLSConfig = nil
		return nil
	case ModeManual:
		cfg := DefaultTLSConfig()
		server.TLSConfig = cfg
		return nil
	case ModeAutoTLS:
		cacheDir := c.CacheDir
		if cacheDir == "" {
			cacheDir = ".tls-cache"
		}
		mgr := NewAutoTLSManager(c.Hostname, cacheDir, c.ACMEEmail)
		cfg := DefaultTLSConfig()
		cfg.GetCertificate = mgr.GetCertificate
		cfg.NextProtos = append(cfg.NextProtos, "acme-tls/1")
		server.TLSConfig = cfg
		return mgr.StartChallengeServer()
	default:
		return fmt.Errorf("unknown TLS mode: %s", c.Mode)
	}
}
