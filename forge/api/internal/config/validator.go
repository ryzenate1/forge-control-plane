package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (ve ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", ve.Field, ve.Message)
}

func (c *Config) ValidateAll() []ValidationError {
	var errs []ValidationError

	if c.App.Env == "production" && c.Auth.Secret == "" {
		errs = append(errs, ValidationError{"auth.secret", "required in production"})
	}
	if c.App.Env == "production" && c.App.Key == "" {
		errs = append(errs, ValidationError{"app.key", "required in production"})
	}
	if c.App.Env == "production" && c.Auth.Secret != "" && len(c.Auth.Secret) < 32 {
		errs = append(errs, ValidationError{"auth.secret", "must be at least 32 characters in production"})
	}
	if c.App.Env == "production" && c.App.Key != "" && len(c.App.Key) != 32 {
		errs = append(errs, ValidationError{"app.key", "must be exactly 32 characters in production"})
	}
	if c.App.Env == "production" && c.Redis.Enabled && strings.TrimSpace(c.Redis.Password) == "" {
		errs = append(errs, ValidationError{"redis.password", "required when Redis is enabled in production"})
	}
	if c.App.Env == "production" && c.DB.URL != "" && !isTLSDatabaseURL(c.DB.URL) {
		errs = append(errs, ValidationError{"db.url", "must enforce TLS (sslmode=require, verify-ca, or verify-full) in production"})
	}
	if c.App.Env == "production" && os.Getenv("DAEMON_ALLOW_INSECURE_NO_AUTH") == "true" {
		errs = append(errs, ValidationError{"daemon.allow_insecure_no_auth", "must be false in production"})
	}
	if c.App.Env == "production" && os.Getenv("DAEMON_ALLOW_MOCK_RUNTIME") == "true" {
		errs = append(errs, ValidationError{"daemon.allow_mock_runtime", "must be false in production"})
	}
	if c.App.Env == "production" && os.Getenv("SESSION_COOKIE_SECURE") == "false" {
		errs = append(errs, ValidationError{"session.cookie_secure", "must be true in production"})
	}
	if c.Server.Addr == "" {
		errs = append(errs, ValidationError{"server.addr", "must not be empty"})
	}
	if c.DB.URL == "" {
		errs = append(errs, ValidationError{"db.url", "DATABASE_URL must be set"})
	}
	if c.Auth.TokenTTL <= 0 {
		errs = append(errs, ValidationError{"auth.token_ttl", "must be positive"})
	}
	if c.Server.ReadTimeout <= 0 {
		errs = append(errs, ValidationError{"server.read_timeout", "must be positive"})
	}

	return errs
}

func isTLSDatabaseURL(databaseURL string) bool {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return false
	}
	values, present := parsed.Query()["sslmode"]
	if !present || len(values) != 1 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(values[0])) {
	case "require", "verify-ca", "verify-full":
		return true
	default:
		return false
	}
}

// Validate checks cfg for common configuration errors.
// Deprecated: use Config.ValidateAll instead.
func Validate(cfg *Config) []ValidationError {
	if cfg == nil {
		return nil
	}
	return cfg.ValidateAll()
}
