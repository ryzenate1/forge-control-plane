package services

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/secrets"
	"gamepanel/forge/internal/store"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

const dbServicePingTimeout = 5 * time.Second

type adminDB interface {
	PingContext(context.Context) error
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) rowScanner
	Close() error
}

type rowScanner interface{ Scan(...any) error }

type sqlAdminDB struct{ *sql.DB }

func (d *sqlAdminDB) QueryRowContext(ctx context.Context, query string, args ...any) rowScanner {
	return d.DB.QueryRowContext(ctx, query, args...)
}

type DatabaseServiceProvisioner struct {
	store         *store.Store
	daemon        *daemon.Client
	beaconBaseURL string
	nodeToken     string
	dockerHost    string
	keyring       *secrets.Keyring
}

func NewDatabaseServiceProvisioner(s *store.Store, dc *daemon.Client, beaconBaseURL, nodeToken, dockerHost string, kr *secrets.Keyring) *DatabaseServiceProvisioner {
	return &DatabaseServiceProvisioner{
		store:         s,
		daemon:        dc,
		beaconBaseURL: strings.TrimRight(beaconBaseURL, "/"),
		nodeToken:     nodeToken,
		dockerHost:    dockerHost,
		keyring:       kr,
	}
}

func generatePassword(length int) string {
	b := make([]byte, length)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:length]
}

func generateDBName() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "db_" + hex.EncodeToString(b)[:8]
}

func generateUsername() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "u_" + hex.EncodeToString(b)[:8]
}

func defaultPortForEngine(engine string) int {
	ports := map[string]int{
		"postgresql": 5432,
		"mysql":      3306,
		"mariadb":    3306,
		"redis":      6379,
		"mongodb":    27017,
	}
	if p, ok := ports[strings.ToLower(engine)]; ok {
		return p
	}
	return 0
}

func imageForDB(engine, version string) string {
	images := map[string]string{
		"postgresql": "postgres",
		"mysql":      "mysql",
		"mariadb":    "mariadb",
		"redis":      "redis",
		"mongodb":    "mongo",
	}
	base, ok := images[strings.ToLower(engine)]
	if !ok {
		base = engine
	}
	return fmt.Sprintf("%s:%s", base, version)
}

func envVarsForDB(engine, dbName, username, password string) []string {
	switch strings.ToLower(engine) {
	case "postgresql":
		return []string{
			"POSTGRES_DB=" + dbName,
			"POSTGRES_USER=" + username,
			"POSTGRES_PASSWORD=" + password,
		}
	case "mysql", "mariadb":
		return []string{
			"MYSQL_DATABASE=" + dbName,
			"MYSQL_USER=" + username,
			"MYSQL_PASSWORD=" + password,
			"MYSQL_ROOT_PASSWORD=" + password,
		}
	case "mongodb":
		return []string{
			"MONGO_INITDB_DATABASE=" + dbName,
			"MONGO_INITDB_ROOT_USERNAME=" + username,
			"MONGO_INITDB_ROOT_PASSWORD=" + password,
		}
	case "redis":
		return []string{
			"REDIS_PASSWORD=" + password,
		}
	default:
		return nil
	}
}

func connectionString(engine, dbName, username, password, host string, port int) string {
	switch strings.ToLower(engine) {
	case "postgresql":
		return fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=disable", username, password, host, port, dbName)
	case "mysql":
		return fmt.Sprintf("mysql://%s:%s@%s:%d/%s", username, password, host, port, dbName)
	case "mariadb":
		return fmt.Sprintf("mariadb://%s:%s@%s:%d/%s", username, password, host, port, dbName)
	case "mongodb":
		return fmt.Sprintf("mongodb://%s:%s@%s:%d/%s", username, password, host, port, dbName)
	case "redis":
		return fmt.Sprintf("redis://:%s@%s:%d", password, host, port)
	default:
		return ""
	}
}

func credsJSON(engine, dbName, username, password string) json.RawMessage {
	creds := map[string]string{
		"username": username,
		"password": password,
	}
	switch strings.ToLower(engine) {
	case "postgresql", "mysql", "mariadb", "mongodb":
		creds["database"] = dbName
	}
	raw, _ := json.Marshal(creds)
	return raw
}

func (p *DatabaseServiceProvisioner) ProvisionService(ctx context.Context, name, engine, version string, memoryMB, cpuShares int) (store.DatabaseService, error) {
	engine = strings.ToLower(strings.TrimSpace(engine))
	version = strings.TrimSpace(version)
	if err := store.ValidateDBEngine(engine, version); err != nil {
		return store.DatabaseService{}, err
	}

	svc, err := p.store.CreateDatabaseService(ctx, store.CreateDatabaseServiceRequest{
		Name:      name,
		Type:      engine,
		Version:   version,
		MemoryMB:  memoryMB,
		CPUShares: cpuShares,
	})
	if err != nil {
		return store.DatabaseService{}, fmt.Errorf("create service record: %w", err)
	}

	dbName := generateDBName()
	username := generateUsername()
	password := generatePassword(32)
	volumeName := "mgp-dbsvc-" + svc.ID[:12]
	port := defaultPortForEngine(engine)

	if p.daemon != nil {
		daemonReq := daemon.DBContainerProvisionRequest{
			ServerID:   svc.ID,
			Engine:     engine,
			Version:    version,
			MemoryMB:   memoryMB,
			CPUShares:  cpuShares,
			DBName:     dbName,
			Username:   username,
			Password:   password,
			Port:       port,
			VolumeName: volumeName,
		}
		resp, err := p.daemon.ProvisionDatabase(ctx, p.beaconBaseURL, p.nodeToken, daemonReq)
		if err != nil {
			_ = p.store.UpdateDatabaseServiceStatus(ctx, svc.ID, "failed", "", 0, "", "", "", "", "", "", nil)
			return store.DatabaseService{}, fmt.Errorf("provision via beacon: %w", err)
		}

		encPass, _ := p.keyring.Encrypt([]byte(password), svc.ID)
		connStr := connectionString(engine, dbName, username, password, p.dockerHost, resp.Port)
		creds := credsJSON(engine, dbName, username, password)
		_ = p.store.UpdateDatabaseServiceStatus(ctx, svc.ID, "running", p.dockerHost, resp.Port, username, encPass, dbName, resp.ContainerID, resp.VolumeID, connStr, creds)
	} else {
		encPass, _ := p.keyring.Encrypt([]byte(password), svc.ID)
		connStr := connectionString(engine, dbName, username, password, "127.0.0.1", port)
		creds := credsJSON(engine, dbName, username, password)
		_ = p.store.UpdateDatabaseServiceStatus(ctx, svc.ID, "running", "127.0.0.1", port, username, encPass, dbName, "", volumeName, connStr, creds)
	}

	return p.store.GetDatabaseService(ctx, svc.ID)
}

func (p *DatabaseServiceProvisioner) StopService(ctx context.Context, id string) error {
	svc, err := p.store.GetDatabaseService(ctx, id)
	if err != nil {
		return err
	}
	if svc.ContainerID != "" && p.daemon != nil {
		_ = p.daemon.DeProvisionDatabase(ctx, p.beaconBaseURL, p.nodeToken, svc.ContainerID, svc.VolumeID)
	}
	return p.store.UpdateDatabaseServiceStatus(ctx, id, "stopped", "", 0, "", "", "", "", "", "", nil)
}

func (p *DatabaseServiceProvisioner) StartService(ctx context.Context, id string) error {
	svc, err := p.store.GetDatabaseService(ctx, id)
	if err != nil {
		return err
	}
	if svc.ContainerID == "" {
		return errors.New("container not yet provisioned; reprovision required")
	}
	return p.store.UpdateDatabaseServiceStatus(ctx, id, "running", "", 0, "", "", "", "", "", "", nil)
}

func (p *DatabaseServiceProvisioner) DeleteService(ctx context.Context, id string) error {
	svc, err := p.store.GetDatabaseService(ctx, id)
	if err != nil {
		return err
	}
	_ = p.store.UpdateDatabaseServiceStatus(ctx, id, "deleting", "", 0, "", "", "", "", "", "", nil)
	if svc.ContainerID != "" && p.daemon != nil {
		_ = p.daemon.DeProvisionDatabase(ctx, p.beaconBaseURL, p.nodeToken, svc.ContainerID, svc.VolumeID)
	}
	return p.store.DeleteDatabaseService(ctx, id)
}

func (p *DatabaseServiceProvisioner) CreateDatabase(ctx context.Context, serviceID, dbName string) error {
	conn, err := p.adminConn(ctx, serviceID)
	if err != nil {
		return err
	}
	defer conn.Close()
	svc, _ := p.store.GetDatabaseService(ctx, serviceID)
	switch strings.ToLower(svc.Type) {
	case "postgresql":
		_, err = conn.ExecContext(ctx, "CREATE DATABASE "+quoteIdent(svc.Type, dbName))
	case "mysql", "mariadb":
		_, err = conn.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS "+quoteIdent(svc.Type, dbName))
	default:
		return errors.New("database creation not supported for this engine")
	}
	return err
}

func (p *DatabaseServiceProvisioner) CreateUser(ctx context.Context, serviceID, username, password, permissions string) (store.DatabaseServiceCredential, error) {
	svc, err := p.store.GetDatabaseService(ctx, serviceID)
	if err != nil {
		return store.DatabaseServiceCredential{}, err
	}
	conn, err := p.adminConn(ctx, serviceID)
	if err != nil {
		return store.DatabaseServiceCredential{}, err
	}
	defer conn.Close()
	switch strings.ToLower(svc.Type) {
	case "postgresql":
		_, err = conn.ExecContext(ctx, "CREATE ROLE "+quoteIdent(svc.Type, username)+" LOGIN PASSWORD "+quoteSQLString(password))
	case "mysql", "mariadb":
		_, err = conn.ExecContext(ctx, "CREATE USER "+quoteIdent(svc.Type, username)+" IDENTIFIED BY "+quoteSQLString(password))
	default:
		return store.DatabaseServiceCredential{}, errors.New("user creation not supported for this engine")
	}
	if err != nil {
		return store.DatabaseServiceCredential{}, fmt.Errorf("create user: %w", err)
	}
	encPass, _ := p.keyring.Encrypt([]byte(password), serviceID)
	return p.store.CreateServiceCredential(ctx, store.CreateServiceCredentialRequest{
		ServiceID:     serviceID,
		Username:      username,
		EncryptedPass: encPass,
		DatabaseName:  svc.DatabaseName,
		Permissions:   permissions,
	})
}

func (p *DatabaseServiceProvisioner) GrantPermissions(ctx context.Context, serviceID, username, dbName, permissions string) error {
	conn, err := p.adminConn(ctx, serviceID)
	if err != nil {
		return err
	}
	defer conn.Close()
	svc, _ := p.store.GetDatabaseService(ctx, serviceID)
	grant := "ALL PRIVILEGES"
	if permissions == "read-only" {
		grant = "SELECT"
	}
	switch strings.ToLower(svc.Type) {
	case "postgresql":
		_, err = conn.ExecContext(ctx, "GRANT "+grant+" ON DATABASE "+quoteIdent(svc.Type, dbName)+" TO "+quoteIdent(svc.Type, username))
	case "mysql", "mariadb":
		_, err = conn.ExecContext(ctx, "GRANT "+grant+" ON "+quoteIdent(svc.Type, dbName)+".* TO "+quoteIdent(svc.Type, username))
	default:
		return errors.New("grant not supported for this engine")
	}
	return err
}

func (p *DatabaseServiceProvisioner) RotateCredentials(ctx context.Context, serviceID, oldPassword, newPassword string, actorID *string) (store.DatabaseService, error) {
	svc, err := p.store.GetDatabaseService(ctx, serviceID)
	if err != nil {
		return store.DatabaseService{}, err
	}
	conn, err := p.adminConn(ctx, serviceID)
	if err != nil {
		return store.DatabaseService{}, err
	}
	defer conn.Close()
	switch strings.ToLower(svc.Type) {
	case "postgresql":
		_, err = conn.ExecContext(ctx, "ALTER ROLE "+quoteIdent(svc.Type, svc.Username)+" PASSWORD "+quoteSQLString(newPassword))
	case "mysql", "mariadb":
		_, err = conn.ExecContext(ctx, "ALTER USER "+quoteIdent(svc.Type, svc.Username)+" IDENTIFIED BY "+quoteSQLString(newPassword))
	case "redis":
		_, err = conn.ExecContext(ctx, "CONFIG SET requirepass "+quoteSQLString(newPassword))
	default:
		return store.DatabaseService{}, errors.New("credential rotation not supported for this engine")
	}
	if err != nil {
		return store.DatabaseService{}, fmt.Errorf("rotate password: %w", err)
	}
	encPass, _ := p.keyring.Encrypt([]byte(newPassword), serviceID)
	_ = p.store.UpdateDatabaseServiceStatus(ctx, serviceID, svc.Status, "", 0, "", encPass, "", "", "", "", nil)
	return p.store.GetDatabaseService(ctx, serviceID)
}

func (p *DatabaseServiceProvisioner) CreateBackup(ctx context.Context, serviceID string) (store.DatabaseServiceBackup, error) {
	svc, err := p.store.GetDatabaseService(ctx, serviceID)
	if err != nil {
		return store.DatabaseServiceBackup{}, err
	}
	backup, err := p.store.CreateServiceBackup(ctx, store.CreateServiceBackupRequest{
		ServiceID: serviceID,
		Status:    "running",
	})
	if err != nil {
		return store.DatabaseServiceBackup{}, err
	}
	if svc.ContainerID != "" && p.daemon != nil {
		_, err = p.daemon.BackupDatabase(ctx, p.beaconBaseURL, p.nodeToken, svc.ContainerID, svc.Type)
	}
	status := "completed"
	filePath := fmt.Sprintf("/backups/%s/%s.sql", serviceID, backup.ID)
	if err != nil {
		status = "failed"
	}
	_ = p.store.UpdateServiceBackupStatus(ctx, backup.ID, status, filePath, 0)
	return p.store.GetServiceBackup(ctx, backup.ID)
}

func (p *DatabaseServiceProvisioner) ListBackups(ctx context.Context, serviceID string) ([]store.DatabaseServiceBackup, error) {
	return p.store.ListServiceBackups(ctx, serviceID)
}

func (p *DatabaseServiceProvisioner) RestoreBackup(ctx context.Context, serviceID, backupID string) error {
	if p.daemon == nil {
		return errors.New("daemon not available for restore")
	}
	backup, err := p.store.GetServiceBackup(ctx, backupID)
	if err != nil {
		return err
	}
	if backup.ServiceID != serviceID {
		return errors.New("backup does not belong to this service")
	}
	if backup.FilePath == "" {
		return errors.New("backup file path is empty")
	}
	_ = p.store.UpdateServiceBackupStatus(ctx, backupID, "running", "", 0)
	svc, err := p.store.GetDatabaseService(ctx, serviceID)
	if err != nil {
		_ = p.store.UpdateServiceBackupStatus(ctx, backupID, "failed", "", 0)
		return err
	}
	if err := p.daemon.RestoreDatabase(ctx, p.beaconBaseURL, p.nodeToken, svc.ContainerID, svc.Type); err != nil {
		_ = p.store.UpdateServiceBackupStatus(ctx, backupID, "failed", "", 0)
		return err
	}
	_ = p.store.UpdateServiceBackupStatus(ctx, backupID, "completed", "", 0)
	return nil
}

func (p *DatabaseServiceProvisioner) GetServiceLogs(ctx context.Context, serviceID string, tail int) ([]string, error) {
	if p.daemon == nil {
		return nil, errors.New("daemon not available for logs")
	}
	svc, err := p.store.GetDatabaseService(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	if tail <= 0 {
		tail = 50
	}
	if svc.ContainerID == "" {
		return nil, errors.New("container not yet provisioned")
	}
	logStr, err := p.daemon.AdminContainerLogs(ctx, p.beaconBaseURL, p.nodeToken, svc.ContainerID, fmt.Sprintf("%d", tail))
	if err != nil {
		return nil, err
	}
	if logStr == "" {
		return []string{}, nil
	}
	return strings.Split(strings.TrimSuffix(logStr, "\n"), "\n"), nil
}

func (p *DatabaseServiceProvisioner) TestConnection(ctx context.Context, host string, port int, engine, username, password, dbName string) error {
	var dsn string
	switch strings.ToLower(engine) {
	case "postgresql":
		u := &url.URL{
			Scheme: "postgres",
			User:   url.UserPassword(username, password),
			Host:   net.JoinHostPort(host, fmt.Sprintf("%d", port)),
			Path:   "/" + dbName,
		}
		q := u.Query()
		q.Set("sslmode", "disable")
		u.RawQuery = q.Encode()
		dsn = u.String()
	case "mysql", "mariadb":
		cfg := mysql.NewConfig()
		cfg.User = username
		cfg.Passwd = password
		cfg.Net = "tcp"
		cfg.Addr = net.JoinHostPort(host, fmt.Sprintf("%d", port))
		cfg.DBName = dbName
		dsn = cfg.FormatDSN()
	default:
		return errors.New("test connection not supported for this engine")
	}
	db, err := sql.Open(strings.ToLower(engine), dsn)
	if err != nil {
		return fmt.Errorf("open connection: %w", err)
	}
	defer db.Close()
	ctxPing, cancel := context.WithTimeout(ctx, dbServicePingTimeout)
	defer cancel()
	if err := db.PingContext(ctxPing); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}
	var result int
	if err := db.QueryRowContext(ctxPing, "SELECT 1").Scan(&result); err != nil {
		return fmt.Errorf("SELECT 1 failed: %w", err)
	}
	if result != 1 {
		return errors.New("SELECT 1 returned unexpected result")
	}
	return nil
}

func (p *DatabaseServiceProvisioner) adminConn(ctx context.Context, serviceID string) (*sqlAdminDB, error) {
	svc, err := p.store.GetDatabaseService(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	plainPass, err := p.keyring.Decrypt(svc.EncryptedPass, svc.ID)
	if err != nil {
		return nil, fmt.Errorf("decrypt password: %w", err)
	}
	password := string(plainPass)
	address := net.JoinHostPort(svc.Host, fmt.Sprintf("%d", svc.Port))
	switch strings.ToLower(svc.Type) {
	case "postgresql":
		cfg, err := pgx.ParseConfig(fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", svc.Username, password, address, svc.DatabaseName))
		if err != nil {
			return nil, err
		}
		cfg.TLSConfig = &tls.Config{InsecureSkipVerify: true}
		return &sqlAdminDB{sql.OpenDB(stdlib.GetConnector(*cfg))}, nil
	case "mysql", "mariadb":
		cfg := mysql.NewConfig()
		cfg.User, cfg.Passwd, cfg.Net = svc.Username, password, "tcp"
		cfg.Addr = address
		cfg.DBName = svc.DatabaseName
		connector, err := mysql.NewConnector(cfg)
		if err != nil {
			return nil, err
		}
		return &sqlAdminDB{sql.OpenDB(connector)}, nil
	default:
		return nil, errors.New("admin connection not supported for this engine")
	}
}

func quoteIdent(engine, value string) string {
	if strings.ToLower(engine) == "postgresql" {
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}

func quoteSQLString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
