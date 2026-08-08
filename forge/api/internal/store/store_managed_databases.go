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

type ManagedDatabase struct {
	ID                string          `json:"id"`
	ServerID          string          `json:"serverId,omitempty"`
	Name              string          `json:"name"`
	Engine            string          `json:"engine"`
	Version           string          `json:"version"`
	DockerImage       string          `json:"dockerImage,omitempty"`
	Status            string          `json:"status"`
	Host              string          `json:"host,omitempty"`
	Port              int             `json:"port"`
	Username          string          `json:"username,omitempty"`
	DatabaseName      string          `json:"databaseName,omitempty"`
	ConnectionString  string          `json:"connectionString,omitempty"`
	Credentials       json.RawMessage `json:"credentials,omitempty"`
	MemoryMB          int             `json:"memoryMb"`
	CPUShares         int             `json:"cpuShares"`
	VolumeID          string          `json:"volumeId,omitempty"`
	ContainerID       string          `json:"containerId,omitempty"`
	CreatedAt         string          `json:"createdAt"`
	UpdatedAt         string          `json:"updatedAt"`
}

type CreateManagedDatabaseRequest struct {
	ServerID  string
	Name      string
	Engine    string
	Version   string
	MemoryMB  int
	CPUShares int
}

type ManagedDatabaseBackup struct {
	ID                string     `json:"id"`
	ManagedDatabaseID string     `json:"managedDatabaseId"`
	Name              string     `json:"name"`
	Engine            string     `json:"engine"`
	Status            string     `json:"status"`
	Size              int64      `json:"size"`
	Checksum          string     `json:"checksum,omitempty"`
	StoragePath       string     `json:"storagePath,omitempty"`
	StorageAdapter    string     `json:"storageAdapter,omitempty"`
	Metadata          string     `json:"metadata,omitempty"`
	CreatedAt         string     `json:"createdAt"`
	CompletedAt       *string    `json:"completedAt,omitempty"`
	UpdatedAt         string     `json:"updatedAt"`
}

type ManagedDatabaseRestore struct {
	ID                string  `json:"id"`
	ManagedDatabaseID string  `json:"managedDatabaseId"`
	BackupID          string  `json:"backupId,omitempty"`
	Status            string  `json:"status"`
	ErrorMessage      string  `json:"errorMessage,omitempty"`
	CreatedAt         string  `json:"createdAt"`
	CompletedAt       *string `json:"completedAt,omitempty"`
	UpdatedAt         string  `json:"updatedAt"`
}

const (
	ManagedDBStatusCreating  = "creating"
	ManagedDBStatusRunning   = "running"
	ManagedDBStatusStopping  = "stopping"
	ManagedDBStatusStopped   = "stopped"
	ManagedDBStatusError     = "error"
	ManagedDBStatusDeleting  = "deleting"

	ManagedDBBackupPending   = "pending"
	ManagedDBBackupRunning   = "running"
	ManagedDBBackupCompleted = "completed"
	ManagedDBBackupFailed    = "failed"

	ManagedDBRestorePending   = "pending"
	ManagedDBRestoreRunning   = "running"
	ManagedDBRestoreCompleted = "completed"
	ManagedDBRestoreFailed    = "failed"
)

func (s *Store) CreateManagedDatabase(ctx context.Context, req CreateManagedDatabaseRequest) (ManagedDatabase, error) {
	engine := canonicalDBEngine(strings.TrimSpace(req.Engine))
	version := strings.TrimSpace(req.Version)
	if err := ValidateDBEngine(engine, version); err != nil {
		return ManagedDatabase{}, err
	}
	if strings.TrimSpace(req.Name) == "" {
		return ManagedDatabase{}, errors.New("name is required")
	}
	if req.MemoryMB < 0 {
		return ManagedDatabase{}, errors.New("memory must not be negative")
	}
	if req.MemoryMB == 0 {
		req.MemoryMB = 256
	}
	id := uuid.NewString()
	port := DBEngineDefaultPorts[engine]
	image := DBEngineImages[engine]
	dockerImage := fmt.Sprintf("%s:%s", image, version)
	dbName := "db_" + strings.ReplaceAll(id, "-", "")[:12]
	username := "u_" + strings.ReplaceAll(id, "-", "")[:8]

	_, err := s.db.Exec(ctx, `
		INSERT INTO managed_databases
		    (id, server_id, name, engine, version, docker_image, status, port, username, database_name, memory_mb, cpu_shares)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, id, req.ServerID, req.Name, engine, version, dockerImage, ManagedDBStatusCreating, port, username, dbName, req.MemoryMB, req.CPUShares)
	if err != nil {
		return ManagedDatabase{}, err
	}
	return s.GetManagedDatabase(ctx, id)
}

func (s *Store) GetManagedDatabase(ctx context.Context, id string) (ManagedDatabase, error) {
	var db ManagedDatabase
	var createdAt, updatedAt any
	var serverID, host, containerID, volumeID, connStr any
	var creds json.RawMessage

	err := s.db.QueryRow(ctx, `
		SELECT id, server_id, name, engine, version, docker_image, status, host, port,
		       username, database_name, connection_string, credentials, memory_mb, cpu_shares,
		       volume_id, container_id, created_at, updated_at
		FROM managed_databases WHERE id = $1
	`, id).Scan(&db.ID, &serverID, &db.Name, &db.Engine, &db.Version, &db.DockerImage,
		&db.Status, &host, &db.Port, &db.Username, &db.DatabaseName,
		&connStr, &creds, &db.MemoryMB, &db.CPUShares, &volumeID, &containerID,
		&createdAt, &updatedAt)
	if err != nil {
		return ManagedDatabase{}, err
	}
	if serverID != nil {
		db.ServerID = fmt.Sprintf("%v", serverID)
	}
	if host != nil {
		db.Host = fmt.Sprintf("%v", host)
	}
	if containerID != nil {
		db.ContainerID = fmt.Sprintf("%v", containerID)
	}
	if volumeID != nil {
		db.VolumeID = fmt.Sprintf("%v", volumeID)
	}
	if connStr != nil {
		db.ConnectionString = fmt.Sprintf("%v", connStr)
	}
	if creds != nil {
		db.Credentials = creds
	}
	db.CreatedAt = fmt.Sprintf("%v", createdAt)
	db.UpdatedAt = fmt.Sprintf("%v", updatedAt)
	return db, nil
}

func (s *Store) ListManagedDatabases(ctx context.Context) ([]ManagedDatabase, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, server_id, name, engine, version, docker_image, status, host, port,
		       username, database_name, connection_string, credentials, memory_mb, cpu_shares,
		       volume_id, container_id, created_at, updated_at
		FROM managed_databases ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dbs []ManagedDatabase
	for rows.Next() {
		var db ManagedDatabase
		var createdAt, updatedAt any
		var serverID, host, containerID, volumeID, connStr any
		var creds json.RawMessage
		if err := rows.Scan(&db.ID, &serverID, &db.Name, &db.Engine, &db.Version, &db.DockerImage,
			&db.Status, &host, &db.Port, &db.Username, &db.DatabaseName,
			&connStr, &creds, &db.MemoryMB, &db.CPUShares, &volumeID, &containerID,
			&createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if serverID != nil {
			db.ServerID = fmt.Sprintf("%v", serverID)
		}
		if host != nil {
			db.Host = fmt.Sprintf("%v", host)
		}
		if containerID != nil {
			db.ContainerID = fmt.Sprintf("%v", containerID)
		}
		if volumeID != nil {
			db.VolumeID = fmt.Sprintf("%v", volumeID)
		}
		if connStr != nil {
			db.ConnectionString = fmt.Sprintf("%v", connStr)
		}
		if creds != nil {
			db.Credentials = creds
		}
		db.CreatedAt = fmt.Sprintf("%v", createdAt)
		db.UpdatedAt = fmt.Sprintf("%v", updatedAt)
		dbs = append(dbs, db)
	}
	return dbs, rows.Err()
}

func (s *Store) UpdateManagedDatabaseStatus(ctx context.Context, id, status string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE managed_databases SET status = $2, updated_at = NOW() WHERE id = $1
	`, id, status)
	return err
}

func (s *Store) SetManagedDatabaseContainerInfo(ctx context.Context, id, containerID, volumeID, host, connStr string, port int, creds json.RawMessage, status string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE managed_databases SET
		    container_id = $2, volume_id = $3, host = $4, connection_string = $5,
		    port = $6, credentials = $7, status = $8, updated_at = NOW()
		WHERE id = $1
	`, id, containerID, volumeID, host, connStr, port, creds, status)
	return err
}

func (s *Store) UpdateManagedDatabasePort(ctx context.Context, id string, port int) error {
	_, err := s.db.Exec(ctx, `
		UPDATE managed_databases SET port = $2, updated_at = NOW() WHERE id = $1
	`, id, port)
	return err
}

func (s *Store) SetManagedDatabasePassword(ctx context.Context, id, encryptedPassword string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE managed_databases SET password_encrypted = $2, updated_at = NOW() WHERE id = $1
	`, id, encryptedPassword)
	return err
}

func (s *Store) UpdateManagedDatabase(ctx context.Context, id, name string, memoryMB, cpuShares int, version string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE managed_databases SET
		    name = COALESCE(NULLIF($2, ''), name),
		    memory_mb = CASE WHEN $3 > 0 THEN $3 ELSE memory_mb END,
		    cpu_shares = $4,
		    version = COALESCE(NULLIF($5, ''), version),
		    updated_at = NOW()
		WHERE id = $1
	`, id, name, memoryMB, cpuShares, version)
	return err
}

func (s *Store) DeleteManagedDatabase(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM managed_databases WHERE id = $1`, id)
	return err
}

func (s *Store) CreateManagedDatabaseBackup(ctx context.Context, dbID, name, engine string) (ManagedDatabaseBackup, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO managed_database_backups (id, managed_database_id, name, engine, status)
		VALUES ($1, $2, $3, $4, $5)
	`, id, dbID, name, engine, ManagedDBBackupPending)
	if err != nil {
		return ManagedDatabaseBackup{}, err
	}
	return s.GetManagedDatabaseBackup(ctx, id)
}

func (s *Store) GetManagedDatabaseBackup(ctx context.Context, id string) (ManagedDatabaseBackup, error) {
	var b ManagedDatabaseBackup
	var createdAt, updatedAt any
	var completedAt any
	err := s.db.QueryRow(ctx, `
		SELECT id, managed_database_id, name, engine, status, size, checksum,
		       storage_path, storage_adapter, metadata, created_at, completed_at, updated_at
		FROM managed_database_backups WHERE id = $1
	`, id).Scan(&b.ID, &b.ManagedDatabaseID, &b.Name, &b.Engine, &b.Status,
		&b.Size, &b.Checksum, &b.StoragePath, &b.StorageAdapter, &b.Metadata,
		&createdAt, &completedAt, &updatedAt)
	if err != nil {
		return ManagedDatabaseBackup{}, err
	}
	b.CreatedAt = fmt.Sprintf("%v", createdAt)
	b.UpdatedAt = fmt.Sprintf("%v", updatedAt)
	if completedAt != nil {
		s := fmt.Sprintf("%v", completedAt)
		b.CompletedAt = &s
	}
	return b, nil
}

func (s *Store) ListManagedDatabaseBackups(ctx context.Context, dbID string) ([]ManagedDatabaseBackup, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, managed_database_id, name, engine, status, size, checksum,
		       storage_path, storage_adapter, metadata, created_at, completed_at, updated_at
		FROM managed_database_backups WHERE managed_database_id = $1 ORDER BY created_at DESC
	`, dbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var backups []ManagedDatabaseBackup
	for rows.Next() {
		var b ManagedDatabaseBackup
		var createdAt, updatedAt any
		var completedAt any
		if err := rows.Scan(&b.ID, &b.ManagedDatabaseID, &b.Name, &b.Engine, &b.Status,
			&b.Size, &b.Checksum, &b.StoragePath, &b.StorageAdapter, &b.Metadata,
			&createdAt, &completedAt, &updatedAt); err != nil {
			return nil, err
		}
		b.CreatedAt = fmt.Sprintf("%v", createdAt)
		b.UpdatedAt = fmt.Sprintf("%v", updatedAt)
		if completedAt != nil {
			s := fmt.Sprintf("%v", completedAt)
			b.CompletedAt = &s
		}
		backups = append(backups, b)
	}
	return backups, rows.Err()
}

func (s *Store) UpdateManagedDatabaseBackupStatus(ctx context.Context, id, status string, size int64, checksum, storagePath string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE managed_database_backups SET
		    status = $2,
		    size = $3,
		    checksum = COALESCE(NULLIF($4, ''), checksum),
		    storage_path = COALESCE(NULLIF($5, ''), storage_path),
		    completed_at = CASE WHEN $2 IN ('completed','failed') THEN NOW() ELSE NULL END,
		    updated_at = NOW()
		WHERE id = $1
	`, id, status, size, checksum, storagePath)
	return err
}

func (s *Store) DeleteManagedDatabaseBackup(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM managed_database_backups WHERE id = $1`, id)
	return err
}

func (s *Store) CreateManagedDatabaseRestore(ctx context.Context, dbID, backupID string) (ManagedDatabaseRestore, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO managed_database_restores (id, managed_database_id, backup_id, status)
		VALUES ($1, $2, NULLIF($3, ''), $4)
	`, id, dbID, backupID, ManagedDBRestorePending)
	if err != nil {
		return ManagedDatabaseRestore{}, err
	}
	return s.GetManagedDatabaseRestore(ctx, id)
}

func (s *Store) GetManagedDatabaseRestore(ctx context.Context, id string) (ManagedDatabaseRestore, error) {
	var r ManagedDatabaseRestore
	var createdAt, updatedAt any
	var completedAt, backupID any
	err := s.db.QueryRow(ctx, `
		SELECT id, managed_database_id, backup_id, status, error_message, created_at, completed_at, updated_at
		FROM managed_database_restores WHERE id = $1
	`, id).Scan(&r.ID, &r.ManagedDatabaseID, &backupID, &r.Status, &r.ErrorMessage,
		&createdAt, &completedAt, &updatedAt)
	if err != nil {
		return ManagedDatabaseRestore{}, err
	}
	if backupID != nil {
		r.BackupID = fmt.Sprintf("%v", backupID)
	}
	r.CreatedAt = fmt.Sprintf("%v", createdAt)
	r.UpdatedAt = fmt.Sprintf("%v", updatedAt)
	if completedAt != nil {
		s := fmt.Sprintf("%v", completedAt)
		r.CompletedAt = &s
	}
	return r, nil
}

func (s *Store) ListManagedDatabaseRestores(ctx context.Context, dbID string) ([]ManagedDatabaseRestore, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, managed_database_id, backup_id, status, error_message, created_at, completed_at, updated_at
		FROM managed_database_restores WHERE managed_database_id = $1 ORDER BY created_at DESC
	`, dbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var restores []ManagedDatabaseRestore
	for rows.Next() {
		var r ManagedDatabaseRestore
		var createdAt, updatedAt any
		var completedAt, backupID any
		if err := rows.Scan(&r.ID, &r.ManagedDatabaseID, &backupID, &r.Status, &r.ErrorMessage,
			&createdAt, &completedAt, &updatedAt); err != nil {
			return nil, err
		}
		if backupID != nil {
			r.BackupID = fmt.Sprintf("%v", backupID)
		}
		r.CreatedAt = fmt.Sprintf("%v", createdAt)
		r.UpdatedAt = fmt.Sprintf("%v", updatedAt)
		if completedAt != nil {
			s := fmt.Sprintf("%v", completedAt)
			r.CompletedAt = &s
		}
		restores = append(restores, r)
	}
	return restores, rows.Err()
}

func (s *Store) UpdateManagedDatabaseRestoreStatus(ctx context.Context, id, status, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(ctx, `
		UPDATE managed_database_restores SET
		    status = $2, error_message = $3, completed_at = CASE WHEN $2 IN ('completed','failed') THEN $4::timestamptz ELSE NULL END,
		    updated_at = NOW()
		WHERE id = $1
	`, id, status, errMsg, now)
	return err
}
