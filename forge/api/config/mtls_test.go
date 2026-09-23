package config

import (
	"testing"
)

// NOTE: MTLSConfig used to validate the environment and panic when
// MTLS_DEV_BYPASS/API_MTLS_DEV_BYPASS was enabled under APP_ENV=production,
// and exposed a package-level ValidateMTLS helper. That production guard was
// relocated to internal/http.MTLSAuthMiddleware (guarded via FORGE_ENV and
// covered in internal/http/middleware_mtls_test.go). config.MTLSConfig is now
// a pure env reader that must never panic, and the legacy API_MTLS_DEV_BYPASS
// alias is no longer honored.

func TestMTLSConfig_DevBypassFromEnv(t *testing.T) {
	t.Setenv("MTLS_DEV_BYPASS", "true")
	cfg := MTLSConfig()
	if !cfg.DevBypass {
		t.Fatalf("expected DevBypass true when MTLS_DEV_BYPASS=true")
	}
}

func TestMTLSConfig_DevBypassDefaultFalse(t *testing.T) {
	cfg := MTLSConfig()
	if cfg.DevBypass {
		t.Fatalf("expected DevBypass false by default")
	}
}

func TestMTLSConfig_NeverPanicsInProduction(t *testing.T) {
	// Validation/panic behavior moved to the http layer; config must stay a
	// pure env reader even with a "production" environment.
	t.Setenv("APP_ENV", "production")
	t.Setenv("MTLS_DEV_BYPASS", "true")
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("MTLSConfig must not panic (guard lives in internal/http): %v", r)
		}
	}()
	cfg := MTLSConfig()
	if !cfg.DevBypass {
		t.Fatalf("expected DevBypass to be read from env without validation")
	}
}

func TestMTLSConfig_ApiAliasIgnored(t *testing.T) {
	// The legacy API_MTLS_DEV_BYPASS alias was removed; only MTLS_DEV_BYPASS applies.
	t.Setenv("API_MTLS_DEV_BYPASS", "true")
	t.Setenv("MTLS_DEV_BYPASS", "false")
	cfg := MTLSConfig()
	if cfg.DevBypass {
		t.Fatalf("expected API_MTLS_DEV_BYPASS alias to be ignored")
	}
}

func TestMTLSConfig_FieldsFromEnv(t *testing.T) {
	t.Setenv("MTLS_ENABLED", "true")
	t.Setenv("MTLS_CA_CERT", "/etc/forge/ca.pem")
	t.Setenv("MTLS_CERT", "/etc/forge/cert.pem")
	t.Setenv("MTLS_KEY", "/etc/forge/key.pem")
	t.Setenv("MTLS_AUTO_MIGRATE", "true")
	cfg := MTLSConfig()
	if !cfg.Enabled {
		t.Fatalf("expected Enabled true")
	}
	if cfg.CACertPath != "/etc/forge/ca.pem" || cfg.CertPath != "/etc/forge/cert.pem" || cfg.KeyPath != "/etc/forge/key.pem" {
		t.Fatalf("unexpected cert paths: %+v", cfg)
	}
	if !cfg.AutoMigrate {
		t.Fatalf("expected AutoMigrate true")
	}
}
