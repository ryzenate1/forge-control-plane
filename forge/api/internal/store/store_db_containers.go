package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var SupportedDBEngines = map[string][]string{
	"postgresql": {"13", "14", "15", "16"},
	"mysql":      {"8.0", "8.1", "8.2", "8.3"},
	"mariadb":    {"10", "11"},
	"redis":      {"6", "7"},
	"mongodb":    {"6", "7"},
}

var DBEngineImages = map[string]string{
	"postgresql": "postgres",
	"mysql":      "mysql",
	"mariadb":    "mariadb",
	"redis":      "redis",
	"mongodb":    "mongo",
}

var DBEngineDefaultPorts = map[string]int{
	"postgresql": 5432,
	"mysql":      3306,
	"mariadb":    3306,
	"redis":      6379,
	"mongodb":    27017,
}

type DBContainer struct {
	ID               string          `json:"id"`
	ServerID         string          `json:"serverId"`
	Engine           string          `json:"engine"`
	Version          string          `json:"version"`
	ContainerID      string          `json:"containerId"`
	ConnectionString string          `json:"-"`
	Credentials      json.RawMessage `json:"-"`
	Status           string          `json:"status"`
	Port             int             `json:"port"`
	VolumeID         string          `json:"volumeId"`
	MemoryMB         int             `json:"memoryMb"`
	CPUShares        int             `json:"cpuShares"`
	CreatedAt        string          `json:"createdAt"`
	UpdatedAt        string          `json:"updatedAt"`
}

type CreateDBContainerRequest struct {
	ServerID  string
	Engine    string
	Version   string
	MemoryMB  int
	CPUShares int
}

func ValidateDBEngine(engine, version string) error {
	engine = canonicalDBEngine(engine)
	versions, ok := SupportedDBEngines[engine]
	if !ok {
		return fmt.Errorf("unsupported database engine %q; supported: %v", engine, engineNames())
	}
	for _, v := range versions {
		if v == strings.TrimSpace(version) {
			return nil
		}
	}
	return fmt.Errorf("unsupported version %q for engine %q; supported: %v", version, engine, versions)
}

func ValidateDBEngineOnly(engine string) error {
	engine = canonicalDBEngine(engine)
	if _, ok := SupportedDBEngines[engine]; !ok {
		return fmt.Errorf("unsupported database engine %q; supported: %v", engine, engineNames())
	}
	return nil
}

func engineNames() []string {
	names := make([]string, 0, len(SupportedDBEngines))
	for name := range SupportedDBEngines {
		names = append(names, name)
	}
	return names
}

func canonicalDBEngine(engine string) string {
	engine = strings.ToLower(strings.TrimSpace(engine))
	if engine == "postgres" {
		return "postgresql"
	}
	return engine
}

func (s *Store) CreateDBContainer(ctx context.Context, req CreateDBContainerRequest) (DBContainer, error) {
	req.Engine = canonicalDBEngine(req.Engine)
	req.Version = strings.TrimSpace(req.Version)
	if err := ValidateDBEngine(req.Engine, req.Version); err != nil {
		return DBContainer{}, err
	}
	if strings.TrimSpace(req.ServerID) == "" {
		return DBContainer{}, errors.New("server id is required")
	}
	if req.MemoryMB < 0 {
		return DBContainer{}, errors.New("memory must not be negative")
	}
	if req.MemoryMB == 0 {
		req.MemoryMB = 256
	}
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO db_containers
		    (id, server_id, engine, version, memory_mb, cpu_shares)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, req.ServerID, req.Engine, req.Version, req.MemoryMB, req.CPUShares)
	if err != nil {
		return DBContainer{}, err
	}
	return s.GetDBContainer(ctx, id)
}

func formatDBContainerTime(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case time.Time:
		return t.Format(time.RFC3339)
	case *time.Time:
		if t != nil {
			return t.Format(time.RFC3339)
		}
		return ""
	case string:
		return t
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (s *Store) GetDBContainer(ctx context.Context, id string) (DBContainer, error) {
	var db DBContainer
	var updatedAt, createdAt any
	var connectionEncrypted, credentialsEncrypted string
	err := s.db.QueryRow(ctx, `
		SELECT id, server_id, engine, version, container_id, connection_string,
		       credentials, status, port, volume_id, memory_mb, cpu_shares, created_at, updated_at,
		       COALESCE(connection_string_encrypted, ''), COALESCE(credentials_encrypted, '')
		FROM db_containers WHERE id = $1
	`, id).Scan(&db.ID, &db.ServerID, &db.Engine, &db.Version, &db.ContainerID,
		&db.ConnectionString, &db.Credentials, &db.Status, &db.Port, &db.VolumeID,
		&db.MemoryMB, &db.CPUShares, &createdAt, &updatedAt, &connectionEncrypted, &credentialsEncrypted)
	if err != nil {
		return DBContainer{}, err
	}
	if err := s.decryptDBContainerSecrets(&db, connectionEncrypted, credentialsEncrypted); err != nil {
		return DBContainer{}, err
	}
	db.CreatedAt = formatDBContainerTime(createdAt)
	db.UpdatedAt = formatDBContainerTime(updatedAt)
	return db, nil
}

func (s *Store) ListDBContainers(ctx context.Context, serverID string) ([]DBContainer, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, server_id, engine, version, container_id, connection_string,
		       credentials, status, port, volume_id, memory_mb, cpu_shares, created_at, updated_at
		FROM db_containers WHERE server_id = $1 ORDER BY created_at DESC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dbs []DBContainer
	for rows.Next() {
		var db DBContainer
		var updatedAt, createdAt any
		if err := rows.Scan(&db.ID, &db.ServerID, &db.Engine, &db.Version, &db.ContainerID,
			&db.ConnectionString, &db.Credentials, &db.Status, &db.Port, &db.VolumeID,
			&db.MemoryMB, &db.CPUShares, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		db.CreatedAt = formatDBContainerTime(createdAt)
		db.UpdatedAt = formatDBContainerTime(updatedAt)
		dbs = append(dbs, db)
	}
	return dbs, rows.Err()
}

func (s *Store) ListAllDBContainers(ctx context.Context, limit ...int) ([]DBContainer, error) {
	maxRows := 100
	if len(limit) > 0 && limit[0] > 0 {
		maxRows = limit[0]
		if maxRows > 1000 {
			maxRows = 1000
		}
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, server_id, engine, version, container_id, connection_string,
		       credentials, status, port, volume_id, memory_mb, cpu_shares, created_at, updated_at
		FROM db_containers ORDER BY created_at DESC LIMIT $1
	`, maxRows)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dbs []DBContainer
	for rows.Next() {
		var db DBContainer
		var updatedAt, createdAt any
		if err := rows.Scan(&db.ID, &db.ServerID, &db.Engine, &db.Version, &db.ContainerID,
			&db.ConnectionString, &db.Credentials, &db.Status, &db.Port, &db.VolumeID,
			&db.MemoryMB, &db.CPUShares, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		db.CreatedAt = formatDBContainerTime(createdAt)
		db.UpdatedAt = formatDBContainerTime(updatedAt)
		dbs = append(dbs, db)
	}
	return dbs, rows.Err()
}

func (s *Store) SetDBContainerStatus(ctx context.Context, id, containerID, status string, port int, volumeID, connectionString string, credentials json.RawMessage) error {
	if containerID == "" {
		connectionString = ""
	}
	connectionEncrypted, err := s.encryptSecret(connectionString, secretAAD("db_containers", id, "connection_string"))
	if err != nil {
		return err
	}
	var credentialsEncrypted string
	if credentials != nil {
		if !json.Valid(credentials) {
			return errors.New("database credentials must be valid JSON")
		}
		credentialsEncrypted, err = s.encryptSecret(string(credentials), secretAAD("db_containers", id, "credentials"))
		if err != nil {
			return err
		}
	}
	_, err = s.db.Exec(ctx, `
		UPDATE db_containers SET
		    container_id = COALESCE(NULLIF($2, ''), container_id),
		    status = $3,
		    port = $4,
		    volume_id = COALESCE(NULLIF($5, ''), volume_id),
		    connection_string = CASE WHEN $6 <> '' THEN '' ELSE connection_string END,
		    credentials = CASE WHEN $7 <> '' THEN '{}'::jsonb ELSE credentials END,
		    connection_string_encrypted = COALESCE(NULLIF($6, ''), connection_string_encrypted),
		    credentials_encrypted = COALESCE(NULLIF($7, ''), credentials_encrypted),
		    updated_at = NOW()
		WHERE id = $1
	`, id, containerID, status, port, volumeID, connectionEncrypted, credentialsEncrypted)
	if err != nil {
		return err
	}
	return nil
}

func (s *Store) DeleteDBContainer(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM db_containers WHERE id = $1`, id)
	return err
}

func (s *Store) GetDBContainerCredentials(ctx context.Context, id string) (json.RawMessage, string, error) {
	var credentials json.RawMessage
	var connectionString string
	var credentialsEncrypted, connectionEncrypted string
	err := s.db.QueryRow(ctx, `
		SELECT credentials, connection_string,
		       COALESCE(credentials_encrypted, ''), COALESCE(connection_string_encrypted, '')
		FROM db_containers WHERE id = $1
	`, id).Scan(&credentials, &connectionString, &credentialsEncrypted, &connectionEncrypted)
	if err != nil {
		return nil, "", err
	}
	db := DBContainer{ID: id, Credentials: credentials, ConnectionString: connectionString}
	if err := s.decryptDBContainerSecrets(&db, connectionEncrypted, credentialsEncrypted); err != nil {
		return nil, "", err
	}
	credentials, connectionString = db.Credentials, db.ConnectionString
	return credentials, connectionString, nil
}

func (s *Store) decryptDBContainerSecrets(db *DBContainer, connectionEncrypted, credentialsEncrypted string) error {
	connection, err := s.decryptSecret(connectionEncrypted, db.ConnectionString, secretAAD("db_containers", db.ID, "connection_string"))
	if err != nil {
		return err
	}
	credentials, err := s.decryptSecret(credentialsEncrypted, string(db.Credentials), secretAAD("db_containers", db.ID, "credentials"))
	if err != nil {
		return err
	}
	if credentials == "" {
		credentials = "{}"
	}
	if !json.Valid([]byte(credentials)) {
		return errors.New("stored database credentials are invalid")
	}
	db.ConnectionString = connection
	db.Credentials = json.RawMessage(credentials)
	return nil
}

// DBContainerBackupTarget contains the info needed to run a database backup
type DBContainerBackupTarget struct {
	ServerID    string
	ContainerID string
	Engine      string
}

// GetDBContainerBackupTarget retrieves container and server info for backup execution
func (s *Store) GetDBContainerBackupTarget(ctx context.Context, dbContainerID string) (DBContainerBackupTarget, error) {
	var t DBContainerBackupTarget
	err := s.db.QueryRow(ctx, `
		SELECT server_id::text, container_id, engine
		FROM db_containers WHERE id = $1
	`, dbContainerID).Scan(&t.ServerID, &t.ContainerID, &t.Engine)
	if err != nil {
		return DBContainerBackupTarget{}, err
	}
	return t, nil
}
