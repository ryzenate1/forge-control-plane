package dbprovisioner

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"gamepanel/forge/internal/store"
)

const pingTimeout = 5 * time.Second

type provisioningStore interface {
	GetDatabaseHostForServerDatabase(context.Context, string) (store.DatabaseHost, string, error)
	GetServerDatabaseForProvisioning(context.Context, string, string) (store.ServerDatabase, error)
	SetServerDatabaseProvisioningState(context.Context, string, string, string, string) error
	NewServerDatabasePasswordCandidate() string
	CommitServerDatabasePassword(context.Context, string, string, string, string, *string) error
}

type adminDB interface {
	PingContext(context.Context) error
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) rowScanner
	BeginTx(context.Context, *sql.TxOptions) (adminTx, error)
	Close() error
}

type adminTx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) rowScanner
	Commit() error
	Rollback() error
}

type rowScanner interface{ Scan(...any) error }

type sqlAdmin struct{ *sql.DB }
type sqlAdminTx struct{ *sql.Tx }

func (d sqlAdmin) QueryRowContext(ctx context.Context, query string, args ...any) rowScanner {
	return d.DB.QueryRowContext(ctx, query, args...)
}
func (d sqlAdmin) BeginTx(ctx context.Context, opts *sql.TxOptions) (adminTx, error) {
	tx, err := d.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return sqlAdminTx{tx}, nil
}
func (t sqlAdminTx) QueryRowContext(ctx context.Context, query string, args ...any) rowScanner {
	return t.Tx.QueryRowContext(ctx, query, args...)
}

type openAdminFunc func(store.DatabaseHost, string) (adminDB, error)

type Service struct {
	store provisioningStore
	open  openAdminFunc
}

func NewService(s *store.Store) *Service {
	service := &Service{store: s}
	service.open = service.connect
	return service
}

func newServiceForTest(s provisioningStore, open openAdminFunc) *Service {
	return &Service{store: s, open: open}
}

// Provision creates the remote resources and only then marks the panel record ready.
// Any failure remains visible as a failed panel record and triggers best-effort cleanup.
func (s *Service) Provision(ctx context.Context, serverID, databaseID string) error {
	host, hostPassword, target, err := s.provisionTarget(ctx, serverID, databaseID)
	if err != nil {
		return err
	}
	if target.ProvisioningState != store.DatabaseStatePending {
		return fmt.Errorf("database is not pending (state %q)", target.ProvisioningState)
	}
	password, err := databasePassword(target)
	if err != nil {
		return s.fail(ctx, serverID, databaseID, err, nil)
	}
	db, err := s.open(host, hostPassword)
	if err != nil {
		return s.fail(ctx, serverID, databaseID, fmt.Errorf("connect to database host: %w", err), nil)
	}
	defer db.Close()
	if err := ping(ctx, db); err != nil {
		return s.fail(ctx, serverID, databaseID, err, nil)
	}
	if err := provisionRemote(ctx, db, host.Engine, target.DatabaseName, target.Username, password, target.Remote); err != nil {
		return s.fail(ctx, serverID, databaseID, fmt.Errorf("remote provisioning failed: %w", err), nil)
	}
	if err := s.store.SetServerDatabaseProvisioningState(ctx, serverID, databaseID, store.DatabaseStateReady, ""); err != nil {
		cleanupErr := deprovisionRemote(ctx, db, host.Engine, target.DatabaseName, target.Username, target.Remote)
		return s.fail(ctx, serverID, databaseID, fmt.Errorf("remote resources created but panel ready-state commit failed: %w", err), cleanupErr)
	}
	return nil
}

func (s *Service) fail(ctx context.Context, serverID, databaseID string, cause, cleanupErr error) error {
	detail := cause.Error()
	if cleanupErr != nil {
		detail += "; remote cleanup also failed: " + cleanupErr.Error()
	}
	if stateErr := s.store.SetServerDatabaseProvisioningState(ctx, serverID, databaseID, store.DatabaseStateFailed, detail); stateErr != nil {
		detail += "; failed to persist failed state: " + stateErr.Error()
	}
	return errors.New(detail)
}

// TestConnection verifies that the host accepts its saved administrator credentials.
// It deliberately reuses the provisioning connector so TLS behavior is identical.
func (s *Service) TestConnection(ctx context.Context, host store.DatabaseHost, password string) error {
	db, err := s.open(host, password)
	if err != nil {
		return fmt.Errorf("connect to database host: %w", err)
	}
	defer db.Close()
	return ping(ctx, db)
}

func (s *Service) Deprovision(ctx context.Context, serverID, databaseID string) error {
	host, hostPassword, target, err := s.provisionTarget(ctx, serverID, databaseID)
	if err != nil {
		return err
	}
	db, err := s.open(host, hostPassword)
	if err != nil {
		return fmt.Errorf("connect to database host: %w", err)
	}
	defer db.Close()
	if err := ping(ctx, db); err != nil {
		return err
	}
	return deprovisionRemote(ctx, db, host.Engine, target.DatabaseName, target.Username, target.Remote)
}

// RotatePassword changes the remote password first. The panel credential is
// committed only after host acceptance. A failed panel commit rolls the host back.
func (s *Service) RotatePassword(ctx context.Context, serverID, databaseID string, actorID *string) (store.ServerDatabase, error) {
	host, hostPassword, target, err := s.provisionTarget(ctx, serverID, databaseID)
	if err != nil {
		return store.ServerDatabase{}, err
	}
	if target.ProvisioningState != store.DatabaseStateReady {
		return store.ServerDatabase{}, fmt.Errorf("database is not ready (state %q)", target.ProvisioningState)
	}
	oldPassword, err := databasePassword(target)
	if err != nil {
		return store.ServerDatabase{}, err
	}
	candidate := s.store.NewServerDatabasePasswordCandidate()
	if candidate == "" || candidate == oldPassword {
		return store.ServerDatabase{}, errors.New("failed to generate a distinct password candidate")
	}
	db, err := s.open(host, hostPassword)
	if err != nil {
		return store.ServerDatabase{}, fmt.Errorf("connect to database host: %w", err)
	}
	defer db.Close()
	if err := ping(ctx, db); err != nil {
		return store.ServerDatabase{}, err
	}
	if err := rotateRemote(ctx, db, host.Engine, target.Username, candidate, target.Remote); err != nil {
		return store.ServerDatabase{}, fmt.Errorf("remote password rotation failed: %w", err)
	}
	commitErr := s.store.CommitServerDatabasePassword(ctx, serverID, databaseID, oldPassword, candidate, actorID)
	if commitErr == nil {
		target.Password = &candidate
		target.ProvisioningState = store.DatabaseStateReady
		target.ProvisioningError = ""
		return target, nil
	}
	rollbackErr := rotateRemote(ctx, db, host.Engine, target.Username, oldPassword, target.Remote)
	if rollbackErr != nil {
		detail := fmt.Sprintf("panel credential commit failed: %v; remote password rollback failed: %v", commitErr, rollbackErr)
		_ = s.store.SetServerDatabaseProvisioningState(ctx, serverID, databaseID, store.DatabaseStateFailed, detail)
		return store.ServerDatabase{}, errors.New(detail)
	}
	return store.ServerDatabase{}, fmt.Errorf("panel credential commit failed; remote password was rolled back: %w", commitErr)
}

func (s *Service) provisionTarget(ctx context.Context, serverID, databaseID string) (store.DatabaseHost, string, store.ServerDatabase, error) {
	host, hostPassword, err := s.store.GetDatabaseHostForServerDatabase(ctx, databaseID)
	if err != nil {
		return store.DatabaseHost{}, "", store.ServerDatabase{}, fmt.Errorf("retrieve database host: %w", err)
	}
	target, err := s.store.GetServerDatabaseForProvisioning(ctx, serverID, databaseID)
	if err != nil {
		return store.DatabaseHost{}, "", store.ServerDatabase{}, fmt.Errorf("retrieve server database: %w", err)
	}
	if err := validateTarget(host, target); err != nil {
		return store.DatabaseHost{}, "", store.ServerDatabase{}, err
	}
	return host, hostPassword, target, nil
}

func validateTarget(host store.DatabaseHost, target store.ServerDatabase) error {
	if host.Engine != "postgresql" && host.Engine != "mysql" {
		return errors.New("unsupported database engine")
	}
	if err := store.ValidateDatabaseIdentifier(target.DatabaseName); err != nil {
		return fmt.Errorf("database name: %w", err)
	}
	if err := store.ValidateDatabaseIdentifier(target.Username); err != nil {
		return fmt.Errorf("database username: %w", err)
	}
	if _, err := store.ValidateDatabaseRemote(target.Remote); err != nil {
		return err
	}
	return nil
}

func databasePassword(target store.ServerDatabase) (string, error) {
	if target.Password == nil || *target.Password == "" {
		return "", errors.New("database credential is unavailable")
	}
	return *target.Password, nil
}

func ping(ctx context.Context, db adminDB) error {
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return fmt.Errorf("database host ping failed: %w", err)
	}
	return nil
}

func (s *Service) connect(host store.DatabaseHost, password string) (adminDB, error) {
	connector, err := connectorForHost(host, password)
	if err != nil {
		return nil, err
	}
	return sqlAdmin{sql.OpenDB(connector)}, nil
}

func connectorForHost(host store.DatabaseHost, password string) (driver.Connector, error) {
	tlsConfig, err := hostTLSConfig(host)
	if err != nil {
		return nil, err
	}
	address := net.JoinHostPort(host.Host, fmt.Sprintf("%d", host.Port))
	switch host.Engine {
	case "mysql":
		cfg := mysql.NewConfig()
		cfg.User, cfg.Passwd, cfg.Net, cfg.Addr = host.Username, password, "tcp", address
		if host.TLSMode != "disable" {
			name := mysqlTLSConfigName(host)
			if err := mysql.RegisterTLSConfig(name, tlsConfig); err != nil && !strings.Contains(err.Error(), "already registered") {
				return nil, err
			}
			cfg.TLSConfig = name
		}
		return mysql.NewConnector(cfg)
	case "postgresql":
		cfg, err := pgx.ParseConfig(connectionDSN(host, password))
		if err != nil {
			return nil, err
		}
		cfg.TLSConfig = tlsConfig
		return stdlib.GetConnector(*cfg), nil
	default:
		return nil, errors.New("unsupported database engine")
	}
}

func connectionDSN(host store.DatabaseHost, password string) string {
	if host.Engine == "mysql" {
		cfg := mysql.NewConfig()
		cfg.User, cfg.Passwd, cfg.Net = host.Username, password, "tcp"
		cfg.Addr = net.JoinHostPort(host.Host, fmt.Sprintf("%d", host.Port))
		if host.TLSMode != "disable" {
			cfg.TLSConfig = mysqlTLSConfigName(host)
		}
		return cfg.FormatDSN()
	}
	u := &url.URL{Scheme: "postgres", User: url.UserPassword(host.Username, password), Host: net.JoinHostPort(host.Host, fmt.Sprintf("%d", host.Port)), Path: "/postgres"}
	query := u.Query()
	sslMode := host.TLSMode
	if sslMode == "required" {
		sslMode = "require"
	}
	query.Set("sslmode", sslMode)
	u.RawQuery = query.Encode()
	return u.String()
}

func mysqlTLSConfigName(host store.DatabaseHost) string {
	sum := sha256.Sum256([]byte(host.ID + "\x00" + host.Host + "\x00" + host.TLSMode + "\x00" + host.TLSServerName + "\x00" + host.TLSCA))
	return fmt.Sprintf("gamepanel-%x", sum[:12])
}

func hostTLSConfig(host store.DatabaseHost) (*tls.Config, error) {
	if host.TLSMode == "disable" {
		return nil, nil
	}
	serverName := host.TLSServerName
	if serverName == "" {
		serverName = host.Host
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName}
	if host.TLSCA != "" {
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM([]byte(host.TLSCA)) {
			return nil, errors.New("tlsCa does not contain a valid PEM certificate")
		}
		cfg.RootCAs = roots
	}
	switch host.TLSMode {
	case "required":
		cfg.InsecureSkipVerify = true // Encryption is required; identity verification was explicitly not requested.
	case "verify-ca":
		cfg.InsecureSkipVerify = true
		cfg.VerifyConnection = func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return errors.New("database host supplied no TLS certificate")
			}
			_, err := state.PeerCertificates[0].Verify(x509.VerifyOptions{Roots: cfg.RootCAs, Intermediates: intermediates(state.PeerCertificates[1:])})
			return err
		}
	case "verify-full":
	case "":
		return nil, errors.New("database host TLS mode is missing")
	default:
		return nil, errors.New("unsupported database host TLS mode")
	}
	return cfg, nil
}

func intermediates(certificates []*x509.Certificate) *x509.CertPool {
	pool := x509.NewCertPool()
	for _, certificate := range certificates {
		pool.AddCert(certificate)
	}
	return pool
}

func provisionRemote(ctx context.Context, db adminDB, engine, dbName, username, password, remote string) error {
	if engine == "mysql" {
		return provisionMySQL(ctx, db, dbName, username, password, remote)
	}
	return provisionPostgreSQL(ctx, db, dbName, username, password)
}

func provisionMySQL(ctx context.Context, db adminDB, dbName, username, password, remote string) error {
	if _, err := db.ExecContext(ctx, "CREATE DATABASE "+quoteMySQLIdentifier(dbName)); err != nil {
		return fmt.Errorf("create MySQL database: %w", err)
	}
	account := quoteSQLString(username) + "@" + quoteSQLString(remote)
	if _, err := db.ExecContext(ctx, "CREATE USER "+account+" IDENTIFIED BY "+quoteSQLString(password)); err != nil {
		cleanupErr := dropCreatedMySQL(ctx, db, dbName, account, false)
		return provisioningError("create MySQL user", err, cleanupErr)
	}
	if _, err := db.ExecContext(ctx, "GRANT ALL PRIVILEGES ON "+quoteMySQLIdentifier(dbName)+".* TO "+account); err != nil {
		cleanupErr := dropCreatedMySQL(ctx, db, dbName, account, true)
		return provisioningError("grant MySQL privileges", err, cleanupErr)
	}
	return nil
}

func dropCreatedMySQL(ctx context.Context, db adminDB, dbName, account string, userCreated bool) error {
	var cleanupErr error
	if userCreated {
		if _, err := db.ExecContext(ctx, "DROP USER IF EXISTS "+account); err != nil {
			cleanupErr = fmt.Errorf("drop newly-created MySQL user: %w", err)
		}
	}
	if _, err := db.ExecContext(ctx, "DROP DATABASE IF EXISTS "+quoteMySQLIdentifier(dbName)); err != nil && cleanupErr == nil {
		cleanupErr = fmt.Errorf("drop newly-created MySQL database: %w", err)
	}
	return cleanupErr
}

func provisionPostgreSQL(ctx context.Context, db adminDB, dbName, username, password string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin PostgreSQL role transaction: %w", err)
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)", username).Scan(&exists); err != nil {
		return fmt.Errorf("check PostgreSQL role: %w", err)
	}
	if exists {
		return errors.New("PostgreSQL role already exists")
	}
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", dbName).Scan(&exists); err != nil {
		return fmt.Errorf("check PostgreSQL database: %w", err)
	}
	if exists {
		return errors.New("PostgreSQL database already exists")
	}
	if _, err := tx.ExecContext(ctx, "CREATE ROLE "+quotePostgresIdentifier(username)+" LOGIN PASSWORD "+quoteSQLString(password)); err != nil {
		return fmt.Errorf("create PostgreSQL role: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit PostgreSQL role: %w", err)
	}
	// PostgreSQL CREATE DATABASE is intentionally outside a transaction.
	if _, err := db.ExecContext(ctx, "CREATE DATABASE "+quotePostgresIdentifier(dbName)+" WITH OWNER "+quotePostgresIdentifier(username)); err != nil {
		_, cleanupErr := db.ExecContext(ctx, "DROP ROLE IF EXISTS "+quotePostgresIdentifier(username))
		return provisioningError("create PostgreSQL database", err, cleanupErr)
	}
	if _, err := db.ExecContext(ctx, "GRANT ALL PRIVILEGES ON DATABASE "+quotePostgresIdentifier(dbName)+" TO "+quotePostgresIdentifier(username)); err != nil {
		cleanupErr := deprovisionRemote(ctx, db, "postgresql", dbName, username, "")
		return provisioningError("grant PostgreSQL privileges", err, cleanupErr)
	}
	return nil
}

func provisioningError(operation string, cause, cleanupErr error) error {
	if cleanupErr != nil {
		return fmt.Errorf("%s: %w; cleanup failed: %v", operation, cause, cleanupErr)
	}
	return fmt.Errorf("%s: %w", operation, cause)
}

func deprovisionRemote(ctx context.Context, db adminDB, engine, dbName, username, remote string) error {
	if engine == "mysql" {
		account := quoteSQLString(username) + "@" + quoteSQLString(remote)
		if _, err := db.ExecContext(ctx, "DROP DATABASE IF EXISTS "+quoteMySQLIdentifier(dbName)); err != nil {
			return fmt.Errorf("drop MySQL database: %w", err)
		}
		if _, err := db.ExecContext(ctx, "DROP USER IF EXISTS "+account); err != nil {
			return fmt.Errorf("drop MySQL user: %w", err)
		}
		return nil
	}
	if _, err := db.ExecContext(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()`, dbName); err != nil {
		return fmt.Errorf("terminate PostgreSQL database connections: %w", err)
	}
	if _, err := db.ExecContext(ctx, "DROP DATABASE IF EXISTS "+quotePostgresIdentifier(dbName)); err != nil {
		return fmt.Errorf("drop PostgreSQL database: %w", err)
	}
	if _, err := db.ExecContext(ctx, "DROP ROLE IF EXISTS "+quotePostgresIdentifier(username)); err != nil {
		return fmt.Errorf("drop PostgreSQL role: %w", err)
	}
	return nil
}

func rotateRemote(ctx context.Context, db adminDB, engine, username, password, remote string) error {
	if engine == "mysql" {
		account := quoteSQLString(username) + "@" + quoteSQLString(remote)
		_, err := db.ExecContext(ctx, "ALTER USER "+account+" IDENTIFIED BY "+quoteSQLString(password))
		if err != nil {
			return fmt.Errorf("alter MySQL user password: %w", err)
		}
		return nil
	}
	_, err := db.ExecContext(ctx, "ALTER ROLE "+quotePostgresIdentifier(username)+" PASSWORD "+quoteSQLString(password))
	if err != nil {
		return fmt.Errorf("alter PostgreSQL role password: %w", err)
	}
	return nil
}

func quotePostgresIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
func quoteMySQLIdentifier(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}
func quoteSQLString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
