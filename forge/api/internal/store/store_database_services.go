package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

type DatabaseService struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Type             string          `json:"type"`
	Version          string          `json:"version"`
	Status           string          `json:"status"`
	Host             string          `json:"host"`
	Port             int             `json:"port"`
	Username         string          `json:"username"`
	EncryptedPass    string          `json:"-"`
	DatabaseName     string          `json:"databaseName"`
	ContainerID      string          `json:"containerId"`
	VolumeID         string          `json:"volumeId"`
	MemoryMB         int             `json:"memoryMb"`
	CPUShares        int             `json:"cpuShares"`
	ServerID         *string         `json:"serverId,omitempty"`
	ConnectionString string          `json:"connectionString"`
	Credentials      json.RawMessage `json:"credentials,omitempty"`
	TemplateID       *string         `json:"templateId,omitempty"`
	CreatedAt        string          `json:"createdAt"`
	UpdatedAt        string          `json:"updatedAt"`
}

type DatabaseServiceBackup struct {
	ID        string `json:"id"`
	ServiceID string `json:"serviceId"`
	Status    string `json:"status"`
	FilePath  string `json:"filePath"`
	SizeBytes int64  `json:"sizeBytes"`
	CreatedAt string `json:"createdAt"`
}

type DatabaseServiceCredential struct {
	ID              string     `json:"id"`
	ServiceID       string     `json:"serviceId"`
	Username        string     `json:"username"`
	EncryptedPass   string     `json:"-"`
	DatabaseName    string     `json:"databaseName"`
	Permissions     string     `json:"permissions"`
	CreatedAt       string     `json:"createdAt"`
	RevokedAt       *time.Time `json:"revokedAt,omitempty"`
}

type ServiceTemplate struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Version       string `json:"version"`
	DockerImage   string `json:"dockerImage"`
	DefaultPort   int    `json:"defaultPort"`
	DefaultDB     string `json:"defaultDatabase"`
	MinMemoryMB   int    `json:"minMemoryMb"`
	CreatedAt     string `json:"createdAt"`
}

type CreateDatabaseServiceRequest struct {
	Name       string
	Type       string
	Version    string
	MemoryMB   int
	CPUShares  int
	ServerID   *string
	TemplateID *string
}

type CreateServiceBackupRequest struct {
	ServiceID string
	Status    string
	FilePath  string
	SizeBytes int64
}

type CreateServiceCredentialRequest struct {
	ServiceID     string
	Username      string
	EncryptedPass string
	DatabaseName  string
	Permissions   string
}

func (s *Store) CreateDatabaseService(ctx context.Context, req CreateDatabaseServiceRequest) (DatabaseService, error) {
	req.Type = canonicalDBEngine(req.Type)
	if err := ValidateDBEngineOnly(req.Type); err != nil {
		return DatabaseService{}, err
	}
	if req.MemoryMB < 0 {
		return DatabaseService{}, errors.New("memory must not be negative")
	}
	if req.MemoryMB == 0 {
		req.MemoryMB = 256
	}
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO database_services
		    (id, name, type, version, memory_mb, cpu_shares, server_id, template_id, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'provisioning')
	`, id, req.Name, req.Type, req.Version, req.MemoryMB, req.CPUShares, req.ServerID, req.TemplateID)
	if err != nil {
		return DatabaseService{}, err
	}
	return s.GetDatabaseService(ctx, id)
}

func (s *Store) GetDatabaseService(ctx context.Context, id string) (DatabaseService, error) {
	var svc DatabaseService
	var createdAt, updatedAt any
	err := s.db.QueryRow(ctx, `
		SELECT id, name, type, version, status, host, port, username,
		       encrypted_password, database_name, container_id, volume_id,
		       memory_mb, cpu_shares, server_id, connection_string, credentials,
		       template_id, created_at, updated_at
		FROM database_services WHERE id = $1
	`, id).Scan(&svc.ID, &svc.Name, &svc.Type, &svc.Version, &svc.Status,
		&svc.Host, &svc.Port, &svc.Username, &svc.EncryptedPass, &svc.DatabaseName,
		&svc.ContainerID, &svc.VolumeID, &svc.MemoryMB, &svc.CPUShares,
		&svc.ServerID, &svc.ConnectionString, &svc.Credentials,
		&svc.TemplateID, &createdAt, &updatedAt)
	if err != nil {
		return DatabaseService{}, err
	}
	svc.CreatedAt = formatTimestamp(createdAt)
	svc.UpdatedAt = formatTimestamp(updatedAt)
	return svc, nil
}

func (s *Store) ListDatabaseServices(ctx context.Context) ([]DatabaseService, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, type, version, status, host, port, username,
		       encrypted_password, database_name, container_id, volume_id,
		       memory_mb, cpu_shares, server_id, connection_string, credentials,
		       template_id, created_at, updated_at
		FROM database_services ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var svcs []DatabaseService
	for rows.Next() {
		var svc DatabaseService
		var createdAt, updatedAt any
		if err := rows.Scan(&svc.ID, &svc.Name, &svc.Type, &svc.Version, &svc.Status,
			&svc.Host, &svc.Port, &svc.Username, &svc.EncryptedPass, &svc.DatabaseName,
			&svc.ContainerID, &svc.VolumeID, &svc.MemoryMB, &svc.CPUShares,
			&svc.ServerID, &svc.ConnectionString, &svc.Credentials,
			&svc.TemplateID, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		svc.CreatedAt = formatTimestamp(createdAt)
		svc.UpdatedAt = formatTimestamp(updatedAt)
		svcs = append(svcs, svc)
	}
	return svcs, rows.Err()
}

func (s *Store) ListDatabaseServicesByServer(ctx context.Context, serverID string) ([]DatabaseService, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, type, version, status, host, port, username,
		       encrypted_password, database_name, container_id, volume_id,
		       memory_mb, cpu_shares, server_id, connection_string, credentials,
		       template_id, created_at, updated_at
		FROM database_services WHERE server_id = $1 ORDER BY created_at DESC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var svcs []DatabaseService
	for rows.Next() {
		var svc DatabaseService
		var createdAt, updatedAt any
		if err := rows.Scan(&svc.ID, &svc.Name, &svc.Type, &svc.Version, &svc.Status,
			&svc.Host, &svc.Port, &svc.Username, &svc.EncryptedPass, &svc.DatabaseName,
			&svc.ContainerID, &svc.VolumeID, &svc.MemoryMB, &svc.CPUShares,
			&svc.ServerID, &svc.ConnectionString, &svc.Credentials,
			&svc.TemplateID, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		svc.CreatedAt = formatTimestamp(createdAt)
		svc.UpdatedAt = formatTimestamp(updatedAt)
		svcs = append(svcs, svc)
	}
	return svcs, rows.Err()
}

func (s *Store) UpdateDatabaseServiceStatus(ctx context.Context, id, status, host string, port int, username, encryptedPass, dbName, containerID, volumeID, connStr string, credentials json.RawMessage) error {
	_, err := s.db.Exec(ctx, `
		UPDATE database_services SET
		    status = $2,
		    host = COALESCE(NULLIF($3, ''), host),
		    port = CASE WHEN $4 > 0 THEN $4 ELSE port END,
		    username = COALESCE(NULLIF($5, ''), username),
		    encrypted_password = COALESCE(NULLIF($6, ''), encrypted_password),
		    database_name = COALESCE(NULLIF($7, ''), database_name),
		    container_id = COALESCE(NULLIF($8, ''), container_id),
		    volume_id = COALESCE(NULLIF($9, ''), volume_id),
		    connection_string = COALESCE(NULLIF($10, ''), connection_string),
		    credentials = CASE WHEN $11::jsonb IS NOT NULL THEN $11 ELSE credentials END,
		    updated_at = NOW()
		WHERE id = $1
	`, id, status, host, port, username, encryptedPass, dbName, containerID, volumeID, connStr, credentials)
	return err
}

func (s *Store) UpdateDatabaseServiceServerID(ctx context.Context, id string, serverID *string) error {
	_, err := s.db.Exec(ctx, `UPDATE database_services SET server_id = $2, updated_at = NOW() WHERE id = $1`, id, serverID)
	return err
}

func (s *Store) DeleteDatabaseService(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM database_services WHERE id = $1`, id)
	return err
}

func (s *Store) CreateServiceBackup(ctx context.Context, req CreateServiceBackupRequest) (DatabaseServiceBackup, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO database_service_backups (id, service_id, status, file_path, size_bytes)
		VALUES ($1, $2, $3, $4, $5)
	`, id, req.ServiceID, req.Status, req.FilePath, req.SizeBytes)
	if err != nil {
		return DatabaseServiceBackup{}, err
	}
	return s.GetServiceBackup(ctx, id)
}

func (s *Store) GetServiceBackup(ctx context.Context, id string) (DatabaseServiceBackup, error) {
	var b DatabaseServiceBackup
	var createdAt any
	err := s.db.QueryRow(ctx, `
		SELECT id, service_id, status, file_path, size_bytes, created_at
		FROM database_service_backups WHERE id = $1
	`, id).Scan(&b.ID, &b.ServiceID, &b.Status, &b.FilePath, &b.SizeBytes, &createdAt)
	if err != nil {
		return DatabaseServiceBackup{}, err
	}
	b.CreatedAt = formatTimestamp(createdAt)
	return b, nil
}

func (s *Store) ListServiceBackups(ctx context.Context, serviceID string) ([]DatabaseServiceBackup, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, service_id, status, file_path, size_bytes, created_at
		FROM database_service_backups WHERE service_id = $1 ORDER BY created_at DESC
	`, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var backups []DatabaseServiceBackup
	for rows.Next() {
		var b DatabaseServiceBackup
		var createdAt any
		if err := rows.Scan(&b.ID, &b.ServiceID, &b.Status, &b.FilePath, &b.SizeBytes, &createdAt); err != nil {
			return nil, err
		}
		b.CreatedAt = formatTimestamp(createdAt)
		backups = append(backups, b)
	}
	return backups, rows.Err()
}

func (s *Store) UpdateServiceBackupStatus(ctx context.Context, id, status, filePath string, sizeBytes int64) error {
	_, err := s.db.Exec(ctx, `
		UPDATE database_service_backups SET
		    status = $2,
		    file_path = COALESCE(NULLIF($3, ''), file_path),
		    size_bytes = CASE WHEN $4 > 0 THEN $4 ELSE size_bytes END
		WHERE id = $1
	`, id, status, filePath, sizeBytes)
	return err
}

func (s *Store) CreateServiceCredential(ctx context.Context, req CreateServiceCredentialRequest) (DatabaseServiceCredential, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO database_service_credentials
		    (id, service_id, username, encrypted_password, database_name, permissions)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, req.ServiceID, req.Username, req.EncryptedPass, req.DatabaseName, req.Permissions)
	if err != nil {
		return DatabaseServiceCredential{}, err
	}
	return s.GetServiceCredential(ctx, id)
}

func (s *Store) GetServiceCredential(ctx context.Context, id string) (DatabaseServiceCredential, error) {
	var c DatabaseServiceCredential
	var createdAt any
	err := s.db.QueryRow(ctx, `
		SELECT id, service_id, username, encrypted_password, database_name, permissions, created_at, revoked_at
		FROM database_service_credentials WHERE id = $1
	`, id).Scan(&c.ID, &c.ServiceID, &c.Username, &c.EncryptedPass, &c.DatabaseName, &c.Permissions, &createdAt, &c.RevokedAt)
	if err != nil {
		return DatabaseServiceCredential{}, err
	}
	c.CreatedAt = formatTimestamp(createdAt)
	return c, nil
}

func (s *Store) ListServiceCredentials(ctx context.Context, serviceID string) ([]DatabaseServiceCredential, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, service_id, username, encrypted_password, database_name, permissions, created_at, revoked_at
		FROM database_service_credentials WHERE service_id = $1 ORDER BY created_at DESC
	`, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var creds []DatabaseServiceCredential
	for rows.Next() {
		var c DatabaseServiceCredential
		var createdAt any
		if err := rows.Scan(&c.ID, &c.ServiceID, &c.Username, &c.EncryptedPass, &c.DatabaseName, &c.Permissions, &createdAt, &c.RevokedAt); err != nil {
			return nil, err
		}
		c.CreatedAt = formatTimestamp(createdAt)
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

func (s *Store) RevokeServiceCredential(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `UPDATE database_service_credentials SET revoked_at = NOW() WHERE id = $1`, id)
	return err
}

func (s *Store) ListServiceTemplates(ctx context.Context) ([]ServiceTemplate, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, type, version, docker_image, default_port, default_database, min_memory_mb, created_at
		FROM service_templates ORDER BY type, version
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var templates []ServiceTemplate
	for rows.Next() {
		var t ServiceTemplate
		var createdAt any
		if err := rows.Scan(&t.ID, &t.Type, &t.Version, &t.DockerImage, &t.DefaultPort, &t.DefaultDB, &t.MinMemoryMB, &createdAt); err != nil {
			return nil, err
		}
		t.CreatedAt = formatTimestamp(createdAt)
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

func (s *Store) GetServiceTemplate(ctx context.Context, id string) (ServiceTemplate, error) {
	var t ServiceTemplate
	var createdAt any
	err := s.db.QueryRow(ctx, `
		SELECT id, type, version, docker_image, default_port, default_database, min_memory_mb, created_at
		FROM service_templates WHERE id = $1
	`, id).Scan(&t.ID, &t.Type, &t.Version, &t.DockerImage, &t.DefaultPort, &t.DefaultDB, &t.MinMemoryMB, &createdAt)
	if err != nil {
		return ServiceTemplate{}, err
	}
	t.CreatedAt = formatTimestamp(createdAt)
	return t, nil
}

func (s *Store) CreateServiceTemplate(ctx context.Context, t ServiceTemplate) (ServiceTemplate, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO service_templates (id, type, version, docker_image, default_port, default_database, min_memory_mb)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (type, version) DO UPDATE SET
		    docker_image = EXCLUDED.docker_image,
		    default_port = EXCLUDED.default_port,
		    default_database = EXCLUDED.default_database,
		    min_memory_mb = EXCLUDED.min_memory_mb
	`, id, t.Type, t.Version, t.DockerImage, t.DefaultPort, t.DefaultDB, t.MinMemoryMB)
	if err != nil {
		return ServiceTemplate{}, err
	}
	return s.GetServiceTemplate(ctx, id)
}

func formatTimestamp(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case time.Time:
		return t.Format(time.RFC3339)
	default:
		return ""
	}
}
