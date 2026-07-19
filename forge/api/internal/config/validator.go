package config

import "fmt"

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

// Validate checks cfg for common configuration errors.
// Deprecated: use Config.ValidateAll instead.
func Validate(cfg *Config) []ValidationError {
	return cfg.ValidateAll()
}
