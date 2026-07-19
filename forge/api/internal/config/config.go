package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Manager struct {
	v *viper.Viper
}

type Config struct {
	App    AppConfig    `mapstructure:"app"`
	Server ServerConfig `mapstructure:"server"`
	DB     DBConfig     `mapstructure:"db"`
	Redis  RedisConfig  `mapstructure:"redis"`
	Auth   AuthConfig   `mapstructure:"auth"`
	Mail   MailConfig   `mapstructure:"mail"`
	Daemon DaemonConfig `mapstructure:"daemon"`
	Backup BackupConfig `mapstructure:"backup"`
	Log    LogConfig    `mapstructure:"log"`
}

type AppConfig struct {
	Env            string `mapstructure:"env"`
	Name           string `mapstructure:"name"`
	URL            string `mapstructure:"url"`
	Debug          bool   `mapstructure:"debug"`
	Version        string `mapstructure:"version"`
	Key            string `mapstructure:"key"`
	Cipher         string `mapstructure:"cipher"`
	Locale         string `mapstructure:"locale"`
	FallbackLocale string `mapstructure:"fallback_locale"`
	MigrationsDir  string `mapstructure:"migrations_dir"`
	PluginsDir     string `mapstructure:"plugins_dir"`
	LangsDir       string `mapstructure:"langs_dir"`
}

type ServerConfig struct {
	Addr        string        `mapstructure:"addr"`
	ReadTimeout time.Duration `mapstructure:"read_timeout"`
	PanelURL    string        `mapstructure:"panel_url"`
}

type DBConfig struct {
	Driver          string `mapstructure:"driver"`
	URL             string `mapstructure:"url"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	Enabled  bool   `mapstructure:"enabled"`
}

type AuthConfig struct {
	Secret          string        `mapstructure:"secret"`
	TokenTTL        time.Duration `mapstructure:"token_ttl"`
	SessionLimit    int           `mapstructure:"session_limit"`
	PasswordMinLen  int           `mapstructure:"password_min_length"`
	TwoFactorPolicy string        `mapstructure:"two_factor_policy"`
	SecretKey       string        `mapstructure:"secret_key"`
	PreviousKeys    []string      `mapstructure:"previous_keys"`
}

type MailConfig struct {
	Driver      string `mapstructure:"driver"`
	Host        string `mapstructure:"host"`
	Port        int    `mapstructure:"port"`
	Encryption  string `mapstructure:"encryption"`
	Username    string `mapstructure:"username"`
	Password    string `mapstructure:"password"`
	FromAddress string `mapstructure:"from_address"`
	FromName    string `mapstructure:"from_name"`
}

type DaemonConfig struct {
	NodeToken string `mapstructure:"node_token"`
}

type BackupConfig struct {
	Driver        string `mapstructure:"driver"`
	RetentionDays int    `mapstructure:"retention_days"`
	MaxBackups    int    `mapstructure:"max_backups"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

func defaults(v *viper.Viper) {
	v.SetDefault("app.env", "development")
	v.SetDefault("app.name", "GamePanel")
	v.SetDefault("app.url", "http://localhost:3000")
	v.SetDefault("app.debug", true)
	v.SetDefault("app.version", "0.1.0")
	v.SetDefault("app.locale", "en")
	v.SetDefault("app.fallback_locale", "en")
	v.SetDefault("app.migrations_dir", "migrations")

	v.SetDefault("server.addr", ":8080")
	v.SetDefault("server.read_timeout", "5s")
	v.SetDefault("server.panel_url", "http://localhost:3000")

	v.SetDefault("db.driver", "postgres")
	v.SetDefault("db.max_open_conns", 25)
	v.SetDefault("db.max_idle_conns", 5)
	v.SetDefault("db.conn_max_lifetime", 3600)

	v.SetDefault("redis.enabled", false)
	v.SetDefault("redis.addr", "127.0.0.1:6379")
	v.SetDefault("redis.db", 0)

	v.SetDefault("auth.token_ttl", "24h")
	v.SetDefault("auth.session_limit", 10)
	v.SetDefault("auth.password_min_length", 8)
	v.SetDefault("auth.two_factor_policy", "none")

	v.SetDefault("mail.driver", "log")
	v.SetDefault("mail.host", "127.0.0.1")
	v.SetDefault("mail.port", 587)
	v.SetDefault("mail.encryption", "tls")
	v.SetDefault("mail.from_address", "noreply@gamepanel.local")
	v.SetDefault("mail.from_name", "GamePanel")

	v.SetDefault("backup.driver", "s3")
	v.SetDefault("backup.retention_days", 30)
	v.SetDefault("backup.max_backups", 10)

	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "text")
	v.SetDefault("log.output", "stdout")
}

func NewManager(configPaths ...string) (*Manager, error) {
	v := viper.New()
	defaults(v)

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	for _, p := range configPaths {
		v.AddConfigPath(p)
	}
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.AddConfigPath("$HOME/.gamepanel")
	v.AddConfigPath("/etc/gamepanel")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	v.SetEnvPrefix("GAMEPANEL")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	return &Manager{v: v}, nil
}

func (m *Manager) All() Config {
	var cfg Config
	if err := m.v.Unmarshal(&cfg); err != nil {
		panic(fmt.Sprintf("unmarshal config: %v", err))
	}
	if cfg.DB.URL == "" {
		cfg.DB.URL = os.Getenv("DATABASE_URL")
	}
	if cfg.Auth.Secret == "" {
		cfg.Auth.Secret = os.Getenv("API_AUTH_SECRET")
	}
	return cfg
}

func (m *Manager) Viper() *viper.Viper { return m.v }

func (c *Config) Validate() []error {
	var errs []error
	if c.App.Env == "production" && c.Auth.Secret == "" {
		errs = append(errs, fmt.Errorf("auth.secret is required in production"))
	}
	if c.Server.Addr == "" {
		errs = append(errs, fmt.Errorf("server.addr is required"))
	}
	return errs
}
