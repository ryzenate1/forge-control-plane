package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

func FromEnv() Config {
	return Config{
		App: AppConfig{
			Env:            env("APP_ENV", "development"),
			Name:           "GamePanel",
			URL:            env("PANEL_URL", "http://localhost:3000"),
			Debug:          env("APP_ENV", "development") != "production" && env("APP_DEBUG", "false") == "true",
			Version:        "0.1.0",
			Key:            env("APP_KEY", ""),
			Cipher:         env("APP_CIPHER", "aes-256-gcm"),
			Locale:         env("APP_LOCALE", "en"),
			FallbackLocale: env("APP_FALLBACK_LOCALE", "en"),
			MigrationsDir:  env("MIGRATIONS_DIR", "migrations"),
			PluginsDir:     env("PLUGINS_DIR", ""),
			LangsDir:       env("LANGS_DIR", "langs"),
		},
		Server: ServerConfig{
			Addr:        env("API_ADDR", ":8080"),
			ReadTimeout: envDuration("API_READ_TIMEOUT", 5*time.Second),
			PanelURL:    env("PANEL_URL", "http://localhost:3000"),
		},
		DB: DBConfig{
			Driver:          env("DB_DRIVER", "postgres"),
			URL:             env("DATABASE_URL", ""),
			MaxOpenConns:    envInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    envInt("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: envInt("DB_CONN_MAX_LIFETIME", 3600),
		},
		Redis: RedisConfig{
			Addr:     env("REDIS_ADDR", "127.0.0.1:6379"),
			Password: env("REDIS_PASSWORD", ""),
			DB:       envInt("REDIS_DB", 0),
			Enabled:  envBool("REDIS_ENABLED", false),
		},
		Auth: AuthConfig{
			Secret:          env("API_AUTH_SECRET", ""),
			TokenTTL:        envDuration("AUTH_TOKEN_TTL", 24*time.Hour),
			SessionLimit:    envInt("AUTH_SESSION_LIMIT", 10),
			PasswordMinLen:  envInt("AUTH_PASSWORD_MIN_LENGTH", 8),
			TwoFactorPolicy: env("AUTH_TWO_FACTOR_POLICY", "none"),
			SecretKey:       env("FORGE_MASTER_KEY", ""),
			PreviousKeys:    splitEnv("FORGE_PREVIOUS_MASTER_KEYS"),
		},
		Log: LogConfig{
			Level:  env("LOG_LEVEL", "info"),
			Format: env("LOG_FORMAT", "text"),
			Output: env("LOG_OUTPUT", "stdout"),
		},
		Mail: MailConfig{
			Driver:      env("MAIL_DRIVER", "log"),
			Host:        env("MAIL_HOST", "127.0.0.1"),
			Port:        envInt("MAIL_PORT", 587),
			Encryption:  env("MAIL_ENCRYPTION", "tls"),
			Username:    env("MAIL_USERNAME", ""),
			Password:    env("MAIL_PASSWORD", ""),
			FromAddress: env("MAIL_FROM_ADDRESS", "noreply@gamepanel.local"),
			FromName:    env("MAIL_FROM_NAME", "GamePanel"),
		},
		Daemon: DaemonConfig{NodeToken: env("DAEMON_NODE_TOKEN", "")},
		Backup: BackupConfig{
			Driver:        env("BACKUP_DRIVER", "s3"),
			RetentionDays: envInt("BACKUP_RETENTION_DAYS", 30),
			MaxBackups:    envInt("BACKUP_MAX_BACKUPS", 10),
		},
	}
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitEnv(key string) []string {
	var values []string
	for _, value := range strings.Split(os.Getenv(key), ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}
