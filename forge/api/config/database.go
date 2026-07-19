package config

import "time"

type Database struct {
	Default     string
	Connections map[string]DatabaseConnection
	Migrations  string
}

type DatabaseConnection struct {
	Driver          string
	Host            string
	Port            int
	Database        string
	Username        string
	Password        string
	Charset         string
	Collation       string
	Prefix          string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	SQLitePath      string
}

func DatabaseConfig() Database {
	return Database{
		Default: env("DB_CONNECTION", "postgres"),
		Connections: map[string]DatabaseConnection{
			"postgres": {
				Driver:          "postgres",
				Host:            env("DB_HOST", "127.0.0.1"),
				Port:            envInt("DB_PORT", 5432),
				Database:        env("DB_DATABASE", "gamepanel"),
				Username:        env("DB_USERNAME", "gamepanel"),
				Password:        env("DB_PASSWORD", ""),
				SSLMode:         env("DB_SSLMODE", "disable"),
				MaxOpenConns:    envInt("DB_MAX_OPEN_CONNS", 25),
				MaxIdleConns:    envInt("DB_MAX_IDLE_CONNS", 5),
				ConnMaxLifetime: time.Duration(envInt("DB_CONN_MAX_LIFETIME", 3600)) * time.Second,
				ConnMaxIdleTime: time.Duration(envInt("DB_CONN_MAX_IDLE_TIME", 300)) * time.Second,
			},
			"mysql": {
				Driver:          "mysql",
				Host:            env("DB_HOST", "127.0.0.1"),
				Port:            envInt("DB_PORT", 3306),
				Database:        env("DB_DATABASE", "gamepanel"),
				Username:        env("DB_USERNAME", "gamepanel"),
				Password:        env("DB_PASSWORD", ""),
				Charset:         "utf8mb4",
				Collation:       "utf8mb4_unicode_ci",
				MaxOpenConns:    envInt("DB_MAX_OPEN_CONNS", 25),
				MaxIdleConns:    envInt("DB_MAX_IDLE_CONNS", 5),
				ConnMaxLifetime: time.Duration(envInt("DB_CONN_MAX_LIFETIME", 3600)) * time.Second,
			},
			"sqlite": {
				Driver:     "sqlite",
				SQLitePath: env("DB_DATABASE", "storage/gamepanel.sqlite"),
			},
		},
		Migrations: env("DB_MIGRATIONS", "migrations"),
	}
}
