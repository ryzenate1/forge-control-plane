package config

import (
	"os"
	"time"
)

func FromEnv() Config {
	return Config{
		App: AppConfig{
			Env:           env("APP_ENV", "development"),
			Name:          "GamePanel",
			URL:           env("PANEL_URL", "http://localhost:3000"),
			Debug:         env("APP_ENV", "development") != "production",
			Version:       "0.1.0",
			MigrationsDir: env("MIGRATIONS_DIR", "migrations"),
			PluginsDir:    env("PLUGINS_DIR", ""),
		},
		Server: ServerConfig{
			Addr:        env("API_ADDR", ":8080"),
			ReadTimeout: 5 * time.Second,
			PanelURL:    env("PANEL_URL", "http://localhost:3000"),
		},
		DB: DBConfig{
			URL:             env("DATABASE_URL", ""),
			MaxOpenConns:    8,
			MaxIdleConns:    1,
			ConnMaxLifetime: 3600,
		},
		Redis: RedisConfig{
			Addr: env("REDIS_ADDR", ""),
		},
		Auth: AuthConfig{
			Secret:   env("API_AUTH_SECRET", ""),
			TokenTTL: 24 * time.Hour,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
			Output: "stdout",
		},
		Mail: MailConfig{
			Driver:      "log",
			FromAddress: "noreply@gamepanel.local",
			FromName:    "GamePanel",
		},
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
