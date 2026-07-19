package configvalidator

import (
	"fmt"
	"gamepanel/forge/internal/config"
)

type Error struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Value   any    `json:"value,omitempty"`
}

func Validate(cfg *config.Config) []Error {
	var errs []Error

	if cfg.App.Env == "" {
		errs = append(errs, Error{Field: "app.env", Message: "env must be set"})
	}
	if cfg.App.Env == "production" && cfg.Auth.Secret == "" {
		errs = append(errs, Error{Field: "auth.secret", Message: "auth secret required in production"})
	}
	if cfg.App.Env == "production" && cfg.App.Key == "" {
		errs = append(errs, Error{Field: "app.key", Message: "app key required in production"})
	}
	if cfg.Server.Addr == "" {
		errs = append(errs, Error{Field: "server.addr", Message: "server address must be set"})
	}
	if cfg.DB.URL == "" {
		if cfg.App.Env == "production" {
			errs = append(errs, Error{Field: "db.url", Message: "DATABASE_URL must be set"})
		}
	}
	if cfg.Auth.TokenTTL <= 0 {
		errs = append(errs, Error{Field: "auth.token_ttl", Message: "must be positive", Value: cfg.Auth.TokenTTL})
	}
	if cfg.Server.ReadTimeout <= 0 {
		errs = append(errs, Error{Field: "server.read_timeout", Message: "must be positive", Value: cfg.Server.ReadTimeout})
	}
	if cfg.App.Env == "production" && cfg.Backup.RetentionDays < 1 {
		errs = append(errs, Error{Field: "backup.retention_days", Message: "must be at least 1 in production", Value: cfg.Backup.RetentionDays})
	}

	return errs
}

func ValidateOrFail(cfg *config.Config) {
	if errs := Validate(cfg); len(errs) > 0 {
		fmt.Println("Configuration errors:")
		for _, e := range errs {
			fmt.Printf("  - %s: %s", e.Field, e.Message)
			if e.Value != nil {
				fmt.Printf(" (got %v)", e.Value)
			}
			fmt.Println()
		}
	}
}
