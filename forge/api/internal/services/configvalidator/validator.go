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
	if cfg == nil {
		return []Error{{Field: "config", Message: "configuration is required"}}
	}
	for _, validationError := range cfg.ValidateAll() {
		errs = append(errs, Error{Field: validationError.Field, Message: validationError.Message})
	}
	if cfg.App.Env == "production" && cfg.Backup.RetentionDays < 1 {
		errs = append(errs, Error{Field: "backup.retention_days", Message: "must be at least 1 in production", Value: cfg.Backup.RetentionDays})
	}

	return errs
}

func ValidateOrFail(cfg *config.Config) error {
	if errs := Validate(cfg); len(errs) > 0 {
		return fmt.Errorf("invalid configuration: %v", errs)
	}
	return nil
}
