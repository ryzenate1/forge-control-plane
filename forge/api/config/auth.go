package config

import "time"

type Auth struct {
	Secret         string
	TokenTTL       time.Duration
	RefreshTTL     time.Duration
	SessionLimit   int
	PasswordPolicy PasswordPolicy
	TwoFactor      TwoFactorConfig
	RateLimit      RateLimitConfig
}

type PasswordPolicy struct {
	MinLength     int
	MaxLength     int
	RequireUpper  bool
	RequireLower  bool
	RequireDigit  bool
	RequireSymbol bool
}

type TwoFactorConfig struct {
	Policy        string
	Issuer        string
	Window        int
	Digits        int
	RecoveryCodes int
}

type RateLimitConfig struct {
	Enabled   bool
	Auth      int
	Mutation  int
	Read      int
	Window    time.Duration
	RedisAddr string
}

func AuthConfig() Auth {
	return Auth{
		Secret:       env("AUTH_SECRET", ""),
		TokenTTL:     time.Duration(envInt("AUTH_TOKEN_TTL", 24)) * time.Hour,
		RefreshTTL:   time.Duration(envInt("AUTH_REFRESH_TTL", 720)) * time.Hour,
		SessionLimit: envInt("AUTH_SESSION_LIMIT", 10),
		PasswordPolicy: PasswordPolicy{
			MinLength:     envInt("AUTH_PASSWORD_MIN_LENGTH", 8),
			MaxLength:     envInt("AUTH_PASSWORD_MAX_LENGTH", 64),
			RequireUpper:  envBool("AUTH_PASSWORD_REQUIRE_UPPER", false),
			RequireLower:  envBool("AUTH_PASSWORD_REQUIRE_LOWER", false),
			RequireDigit:  envBool("AUTH_PASSWORD_REQUIRE_DIGIT", true),
			RequireSymbol: envBool("AUTH_PASSWORD_REQUIRE_SYMBOL", false),
		},
		TwoFactor: TwoFactorConfig{
			Policy:        env("AUTH_2FA_POLICY", "none"),
			Issuer:        env("AUTH_2FA_ISSUER", "GamePanel"),
			Window:        envInt("AUTH_2FA_WINDOW", 1),
			Digits:        envInt("AUTH_2FA_DIGITS", 6),
			RecoveryCodes: envInt("AUTH_2FA_RECOVERY_CODES", 10),
		},
		RateLimit: RateLimitConfig{
			Enabled:  envBool("RATE_LIMIT_ENABLED", true),
			Auth:     envInt("RATE_LIMIT_AUTH", 5),
			Mutation: envInt("RATE_LIMIT_MUTATION", 30),
			Read:     envInt("RATE_LIMIT_READ", 120),
			Window:   time.Duration(envInt("RATE_LIMIT_WINDOW", 1)) * time.Minute,
		},
	}
}
