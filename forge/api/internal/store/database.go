package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

type DatabaseType string

const (
	DatabasePostgres DatabaseType = "postgres"
	// DatabaseMySQL and DatabaseMariaDB are best-effort targets: the
	// deployed and CI-verified path is PostgreSQL (SQLite for local
	// dev/tests). The MigrationRunner applies mysqlCompatibleMigration for
	// the common PG-isms and per-file mysql/ dialect overrides take
	// precedence; migrations outside that coverage fail with the raw
	// driver error rather than silently diverging. Do not claim full
	// MySQL parity for migrations without an override.
	DatabaseMySQL   DatabaseType = "mysql"
	DatabaseMariaDB DatabaseType = "mariadb"
	DatabaseSQLite  DatabaseType = "sqlite"
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
			appEnv := os.Getenv("APP_ENV")
			if appEnv == "development" || appEnv == "" {
				sslmode = "disable"
			} else {
				sslmode = "require"
			}
		}
		// Credentials are percent-encoded so special characters (@, :, /, ?,
		// #) in usernames or passwords cannot corrupt the URL parse.
		// url.UserPassword applies RFC 3986 userinfo encoding (space becomes
		// %20, not the + that QueryEscape would emit).
		dsnURL := &url.URL{
			Scheme:   "postgres",
			User:     url.UserPassword(c.User, c.Password),
			Host:     fmt.Sprintf("%s:%d", c.Host, c.Port),
			Path:     "/" + c.Database,
			RawQuery: "sslmode=" + url.QueryEscape(sslmode),
		}
		return dsnURL.String()
	case DatabaseMySQL, DatabaseMariaDB:
		// TLS mirrors the Postgres branch above: an unset SSLMode must not mean
		// cleartext in production. "preferred" negotiates TLS when the server
		// offers it — the closest MySQL analogue to libpq's "prefer" — so
		// hardening the default does not break a server without TLS set up.
		tlsParam := "false"
		switch c.SSLMode {
		case "require", "enable":
			tlsParam = "true"
		case "skip-verify":
			tlsParam = "skip-verify"
		case "disable":
			tlsParam = "false"
		case "":
			if appEnv := os.Getenv("APP_ENV"); appEnv != "development" && appEnv != "" {
				tlsParam = "preferred"
			}
		}
		// FormatDSN escapes the credentials and database name, which the
		// hand-built format string it replaces could not: go-sql-driver's
		// parser splits on the last '/' and the last '@', so a password
		// containing '/' yielded a DSN that parsed into the wrong fields.
		// NewConfig (not a bare &mysql.Config{}) is required for its defaults,
		// notably AllowNativePasswords.
		myCfg := mysql.NewConfig()
		myCfg.User = c.User
		myCfg.Passwd = c.Password
		myCfg.Net = "tcp"
		myCfg.Addr = fmt.Sprintf("%s:%d", c.Host, c.Port)
		myCfg.DBName = c.Database
		myCfg.ParseTime = true
		myCfg.TLSConfig = tlsParam
		return myCfg.FormatDSN()
	case DatabaseSQLite:
		if c.SQLitePath == "" {
			c.SQLitePath = "file:gamepanel.db?cache=shared&_journal_mode=WAL"
		}
		// _foreign_keys=on applies per connection (mattn/go-sqlite3 honors it
		// for every pooled connection), unlike a one-shot PRAGMA that only
		// affects the connection it runs on.
		if !strings.Contains(c.SQLitePath, "_foreign_keys=") && !strings.HasPrefix(c.SQLitePath, ":memory:") {
			sep := "?"
			if strings.Contains(c.SQLitePath, "?") {
				sep = "&"
			}
			c.SQLitePath += sep + "_foreign_keys=on"
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
