package tls

import (
	"crypto/tls"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultTLSConfig(t *testing.T) {
	cfg := DefaultTLSConfig()

	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("expected MinVersion TLS 1.2, got %d", cfg.MinVersion)
	}
	if cfg.MaxVersion != tls.VersionTLS13 {
		t.Errorf("expected MaxVersion TLS 1.3, got %d", cfg.MaxVersion)
	}
	if len(cfg.CipherSuites) != 6 {
		t.Errorf("expected 6 cipher suites, got %d", len(cfg.CipherSuites))
	}
	if len(cfg.CurvePreferences) != 2 {
		t.Errorf("expected 2 curves, got %d", len(cfg.CurvePreferences))
	}
	if cfg.CurvePreferences[0] != tls.X25519 {
		t.Error("expected X25519 as first curve")
	}
	if cfg.CurvePreferences[1] != tls.CurveP256 {
		t.Error("expected P256 as second curve")
	}
	if len(cfg.NextProtos) != 2 || cfg.NextProtos[0] != "h2" || cfg.NextProtos[1] != "http/1.1" {
		t.Errorf("unexpected ALPN protos: %v", cfg.NextProtos)
	}
}

func TestConfigValidate_None(t *testing.T) {
	c := &Config{Mode: ModeNone}
	if err := c.Validate(); err != nil {
		t.Errorf("ModeNone should be valid, got: %v", err)
	}
}

func TestConfigValidate_Manual_MissingCert(t *testing.T) {
	c := &Config{Mode: ModeManual, KeyFile: "key.pem"}
	if err := c.Validate(); err == nil {
		t.Error("expected error for missing cert_file")
	}
}

func TestConfigValidate_Manual_MissingKey(t *testing.T) {
	c := &Config{Mode: ModeManual, CertFile: "cert.pem"}
	if err := c.Validate(); err == nil {
		t.Error("expected error for missing key_file")
	}
}

func TestConfigValidate_Manual_NonExistentFiles(t *testing.T) {
	c := &Config{Mode: ModeManual, CertFile: "/nonexistent/cert.pem", KeyFile: "/nonexistent/key.pem"}
	if err := c.Validate(); err == nil {
		t.Error("expected error for non-existent files")
	}
}

func TestConfigValidate_Manual_ValidFiles(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	os.WriteFile(certFile, []byte("cert"), 0644)
	os.WriteFile(keyFile, []byte("key"), 0644)

	c := &Config{Mode: ModeManual, CertFile: certFile, KeyFile: keyFile}
	if err := c.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestConfigValidate_AutoTLS_MissingHostname(t *testing.T) {
	c := &Config{Mode: ModeAutoTLS}
	if err := c.Validate(); err == nil {
		t.Error("expected error for missing hostname")
	}
}

func TestConfigValidate_AutoTLS_Valid(t *testing.T) {
	c := &Config{Mode: ModeAutoTLS, Hostname: "example.com"}
	if err := c.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestConfigValidate_UnknownMode(t *testing.T) {
	c := &Config{Mode: "invalid"}
	if err := c.Validate(); err == nil {
		t.Error("expected error for unknown mode")
	}
}

func TestConfigApply_None(t *testing.T) {
	c := &Config{Mode: ModeNone}
	srv := &http.Server{}
	if err := c.Apply(srv); err != nil {
		t.Errorf("Apply failed: %v", err)
	}
	if srv.TLSConfig != nil {
		t.Error("expected nil TLSConfig for ModeNone")
	}
}

func TestConfigApply_Manual(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	os.WriteFile(certFile, []byte("cert"), 0644)
	os.WriteFile(keyFile, []byte("key"), 0644)

	c := &Config{Mode: ModeManual, CertFile: certFile, KeyFile: keyFile}
	srv := &http.Server{}
	if err := c.Apply(srv); err != nil {
		t.Errorf("Apply failed: %v", err)
	}
	if srv.TLSConfig == nil {
		t.Error("expected non-nil TLSConfig for ModeManual")
	}
	if srv.TLSConfig.MinVersion != tls.VersionTLS12 {
		t.Error("expected TLS 1.2 minimum")
	}
}
