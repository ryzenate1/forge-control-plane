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
	// DatabaseMySQL and DatabaseMariaDB are best-effort targets scoped to
	// MigrationRunner use in tests and local dev: the MigrationRunner applies
	// mysqlCompatibleMigration for the common PG-isms and per-file mysql/
	// dialect overrides take precedence; migrations outside that coverage
	// fail with the raw driver error rather than silently diverging. Do not
	// claim full MySQL parity for migrations without an override.
	//
	// Store queries NEVER run on MySQL/MariaDB/SQLite: they use
	// PostgreSQL-only syntax (DISTINCT ON, FOR UPDATE/SHARE + SKIP LOCKED,
	// ::casts, ON CONFLICT, RETURNING, pg_advisory_lock). ConnectWithKeyring
	// fails fast on non-Postgres DSNs for this reason.
	DatabaseMySQL   DatabaseType = "mysql"
	DatabaseMariaDB DatabaseType = "mariadb"
	// DatabaseSQLite is scoped to MigrationRunner use in tests and local
	// dev (sqliteCompatibleMigration + sqlite/ overrides + documented
	// sqliteSkippedDDL gaps surfaced via MigrationIntegrity.SkippedDDL).
	// Never for Store queries.
	DatabaseSQLite DatabaseType = "sqlite"
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
		// FormatDSN writes the userinfo RAW (go-sql-driver/mysql v1.10.0
		// neither escapes it here nor decodes it in ParseDSN), so the
		// password must stay unencoded: percent-encoding it would make the
		// literal escapes part of the password and break authentication.
		// The driver's parser splits on the last '/' and the last '@', so
		// ordinary special characters (@ : / ? # %) in the password still
		// resolve to the right fields (pinned by TestMySQLDSN round-trip).
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

// RedactedDSN returns the DSN with password material removed for logging.
// It never contains the raw password or its percent-encoded form.
func (c DBConfig) RedactedDSN() string {
	switch c.Type {
	case DatabasePostgres:
		dsn := c.DSN()
		if at := strings.LastIndex(dsn, "@"); at >= 0 {
			if scheme := strings.Index(dsn, "://"); scheme >= 0 {
				dsn = dsn[:scheme+3] + dsn[at+1:]
			}
		}
		return dsn
	case DatabaseMySQL, DatabaseMariaDB:
		dsn := c.DSN()
		if at := strings.LastIndex(dsn, "@"); at >= 0 {
			if colon := strings.Index(dsn, ":"); colon >= 0 && colon < at {
				dsn = dsn[:colon+1] + "***" + dsn[at:]
			}
		}
		return dsn
	default:
		return c.DSN()
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
