package config

import (
	"testing"
	"time"
)

func TestValidateAll_ValidConfig(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		App: AppConfig{
			Env: "development",
			Key: "01234567890123456789012345678901",
		},
		Server: ServerConfig{Addr: ":8080", ReadTimeout: 5 * time.Second},
		DB:     DBConfig{URL: "postgres://localhost/gamepanel"},
		Auth:   AuthConfig{Secret: "supersecret", TokenTTL: 24 * time.Hour},
	}
	errs := cfg.ValidateAll()
	if len(errs) != 0 {
		t.Fatalf("ValidateAll() = %v, want no errors", errs)
	}
}

func TestValidateAll_ProductionRequiresSecret(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		App:    AppConfig{Env: "production"},
		Server: ServerConfig{Addr: ":8080", ReadTimeout: 5 * time.Second},
		DB:     DBConfig{URL: "postgres://localhost/gamepanel"},
		Auth:   AuthConfig{TokenTTL: 24 * time.Hour},
	}
	errs := cfg.ValidateAll()
	if len(errs) == 0 {
		t.Fatal("ValidateAll() expected errors for production without auth.secret")
	}
	found := false
	for _, e := range errs {
		if e.Field == "auth.secret" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ValidateAll() missing auth.secret error, got: %v", errs)
	}
}

func TestValidateAll_ProductionRequiresAppKey(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		App:    AppConfig{Env: "production"},
		Server: ServerConfig{Addr: ":8080", ReadTimeout: 5 * time.Second},
		DB:     DBConfig{URL: "postgres://localhost/gamepanel"},
		Auth:   AuthConfig{Secret: "supersecret", TokenTTL: 24 * time.Hour},
	}
	errs := cfg.ValidateAll()
	found := false
	for _, e := range errs {
		if e.Field == "app.key" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ValidateAll() missing app.key error, got: %v", errs)
	}
}

func TestValidateAll_EmptyServerAddr(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		App:    AppConfig{Env: "development"},
		Server: ServerConfig{ReadTimeout: 5 * time.Second},
		DB:     DBConfig{URL: "postgres://localhost/gamepanel"},
		Auth:   AuthConfig{Secret: "s", TokenTTL: 24 * time.Hour},
	}
	errs := cfg.ValidateAll()
	found := false
	for _, e := range errs {
		if e.Field == "server.addr" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ValidateAll() missing server.addr error, got: %v", errs)
	}
}

func TestValidateAll_EmptyDBURL(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		App:    AppConfig{Env: "development"},
		Server: ServerConfig{Addr: ":8080", ReadTimeout: 5 * time.Second},
		Auth:   AuthConfig{Secret: "s", TokenTTL: 24 * time.Hour},
	}
	errs := cfg.ValidateAll()
	found := false
	for _, e := range errs {
		if e.Field == "db.url" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ValidateAll() missing db.url error, got: %v", errs)
	}
}

func TestValidateAll_NegativeTokenTTL(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		App:    AppConfig{Env: "development"},
		Server: ServerConfig{Addr: ":8080", ReadTimeout: 5 * time.Second},
		DB:     DBConfig{URL: "postgres://localhost/gamepanel"},
		Auth:   AuthConfig{Secret: "s", TokenTTL: -1},
	}
	errs := cfg.ValidateAll()
	found := false
	for _, e := range errs {
		if e.Field == "auth.token_ttl" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ValidateAll() missing auth.token_ttl error, got: %v", errs)
	}
}

func TestValidateAll_NegativeReadTimeout(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		App:    AppConfig{Env: "development"},
		Server: ServerConfig{Addr: ":8080", ReadTimeout: -1},
		DB:     DBConfig{URL: "postgres://localhost/gamepanel"},
		Auth:   AuthConfig{Secret: "s", TokenTTL: 24 * time.Hour},
	}
	errs := cfg.ValidateAll()
	found := false
	for _, e := range errs {
		if e.Field == "server.read_timeout" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ValidateAll() missing server.read_timeout error, got: %v", errs)
	}
}

func TestValidateAll_ProductionShortSecret(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		App:    AppConfig{Env: "production", Key: "01234567890123456789012345678901"},
		Server: ServerConfig{Addr: ":8080", ReadTimeout: 5 * time.Second},
		DB:     DBConfig{URL: "postgres://localhost/gamepanel"},
		Auth:   AuthConfig{Secret: "short", TokenTTL: 24 * time.Hour},
	}
	errs := cfg.ValidateAll()
	found := false
	for _, e := range errs {
		if e.Field == "auth.secret" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ValidateAll() missing auth.secret length error, got: %v", errs)
	}
}

func TestValidateAll_RedisPasswordInProduction(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		App:    AppConfig{Env: "production", Key: "01234567890123456789012345678901"},
		Server: ServerConfig{Addr: ":8080", ReadTimeout: 5 * time.Second},
		DB:     DBConfig{URL: "postgres://localhost/gamepanel"},
		Auth:   AuthConfig{Secret: "supersecret-long-enough-for-production", TokenTTL: 24 * time.Hour},
		Redis:  RedisConfig{Enabled: true},
	}
	errs := cfg.ValidateAll()
	found := false
	for _, e := range errs {
		if e.Field == "redis.password" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ValidateAll() missing redis.password error, got: %v", errs)
	}
}

func TestValidateAll_AppKeyInvalidLength(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		App:    AppConfig{Env: "production", Key: "wrong-length-key"},
		Server: ServerConfig{Addr: ":8080", ReadTimeout: 5 * time.Second},
		DB:     DBConfig{URL: "postgres://localhost/gamepanel"},
		Auth:   AuthConfig{Secret: "supersecret-long-enough-for-production", TokenTTL: 24 * time.Hour},
	}
	errs := cfg.ValidateAll()
	found := false
	for _, e := range errs {
		if e.Field == "app.key" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ValidateAll() missing app.key length error, got: %v", errs)
	}
}

func TestValidateAll_ProductionRejectsOpportunisticOrSmuggledDatabaseTLS(t *testing.T) {
	t.Parallel()
	for _, databaseURL := range []string{
		"postgres://localhost/gamepanel?sslmode=prefer",
		"postgres://localhost/gamepanel?note=sslmode=require",
		"postgres://localhost/gamepanel?sslmode=disable&note=sslmode=require",
	} {
		cfg := &Config{
			App:    AppConfig{Env: "production", Key: "01234567890123456789012345678901"},
			Server: ServerConfig{Addr: ":8080", ReadTimeout: 5 * time.Second},
			DB:     DBConfig{URL: databaseURL},
			Auth:   AuthConfig{Secret: "supersecret-long-enough-for-production", TokenTTL: 24 * time.Hour},
		}
		errs := cfg.ValidateAll()
		found := false
		for _, validationErr := range errs {
			if validationErr.Field == "db.url" {
				found = true
			}
		}
		if !found {
			t.Errorf("ValidateAll() accepted insecure database URL %q", databaseURL)
		}
	}
}

func TestValidate_Deprecated(t *testing.T) {
	t.Parallel()
	if errs := Validate(nil); errs != nil {
		t.Fatalf("Validate(nil) = %v, want nil", errs)
	}
	if errs := Validate(&Config{}); len(errs) == 0 {
		t.Fatal("Validate(&Config{}) expected errors")
	}
}

