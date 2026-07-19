package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type DatabaseType string

const (
	DatabasePostgres DatabaseType = "postgres"
	DatabaseMySQL    DatabaseType = "mysql"
	DatabaseMariaDB  DatabaseType = "mariadb"
	DatabaseSQLite   DatabaseType = "sqlite"
)

type DatabaseDriver interface {
	Ping(ctx context.Context) error
	Exec(ctx context.Context, query string, args ...any) (sql.Result, error)
	Query(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRow(ctx context.Context, query string, args ...any) *sql.Row
	BeginTx(ctx context.Context) (*sql.Tx, error)
	Close() error
	Type() DatabaseType
	Stats() sql.DBStats
	DB() *sql.DB
}

type DBConfig struct {
	Type            DatabaseType
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	SQLitePath      string
}

func (c DBConfig) DSN() string {
	switch c.Type {
	case DatabasePostgres:
		sslmode := c.SSLMode
		if sslmode == "" {
			sslmode = "disable"
		}
		return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			c.User, c.Password, c.Host, c.Port, c.Database, sslmode)
	case DatabaseMySQL, DatabaseMariaDB:
		tls := "false"
		if c.SSLMode == "require" || c.SSLMode == "enable" {
			tls = "true"
		}
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?tls=%s&parseTime=true&multiStatements=true",
			c.User, c.Password, c.Host, c.Port, c.Database, tls)
	case DatabaseSQLite:
		if c.SQLitePath == "" {
			c.SQLitePath = "file:gamepanel.db?cache=shared&_journal_mode=WAL"
		}
		return c.SQLitePath
	default:
		return ""
	}
}

func NewDatabaseDriver(ctx context.Context, cfg DBConfig) (DatabaseDriver, error) {
	switch cfg.Type {
	case DatabasePostgres:
		return newPostgresDriver(ctx, cfg)
	case DatabaseMySQL, DatabaseMariaDB:
		return newMySQLDriver(ctx, cfg)
	case DatabaseSQLite:
		return newSQLiteDriver(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}
}
