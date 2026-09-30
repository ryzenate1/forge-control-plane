package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type BackupConfigurationRecord struct {
	ID                 string
	Name               string
	Description        string
	ServerID           *string
	AppID              *string
	DatabaseID         *string
	VolumeID           *string
	BackupType         string
	IsScheduled        bool
	CronExpression     *string
	NextRunAt          *time.Time
	LastRunAt          *time.Time
	StorageProvider    string
	StorageConfig      json.RawMessage
	MaxBackups         int
	RetentionDays      int
	CompressionEnabled bool
	EncryptionEnabled  bool
	EncryptionKeyID    *string
	Enabled            bool
	LastStatus         *string
	LastError          *string
	Data               json.RawMessage
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

const backupConfigurationColumns = `
	id, name, COALESCE(description, ''), server_id, app_id, database_id, volume_id,
	backup_type, is_scheduled, cron_expression, next_run_at, last_run_at,
	storage_provider, COALESCE(storage_config, '{}'::jsonb), max_backups, retention_days,
	compression_enabled, encryption_enabled, encryption_key_id, enabled,
	last_status, last_error, data, created_at, updated_at`

func scanBackupConfiguration(row pgx.Row) (BackupConfigurationRecord, error) {
	var record BackupConfigurationRecord
	err := row.Scan(
		&record.ID, &record.Name, &record.Description, &record.ServerID, &record.AppID,
		&record.DatabaseID, &record.VolumeID, &record.BackupType, &record.IsScheduled,
		&record.CronExpression, &record.NextRunAt, &record.LastRunAt, &record.StorageProvider,
		&record.StorageConfig, &record.MaxBackups, &record.RetentionDays,
		&record.CompressionEnabled, &record.EncryptionEnabled, &record.EncryptionKeyID,
		&record.Enabled, &record.LastStatus, &record.LastError, &record.Data,
		&record.CreatedAt, &record.UpdatedAt,
	)
	return record, err
}

func (s *Store) CreateBackupConfiguration(ctx context.Context, record *BackupConfigurationRecord) error {
	if record.ID == "" {
		record.ID = uuid.NewString()
	}
	if len(record.StorageConfig) == 0 {
		record.StorageConfig = json.RawMessage(`{}`)
	}
	if len(record.Data) == 0 {
		record.Data = json.RawMessage(`{}`)
	}
	return s.db.QueryRow(ctx, `
		INSERT INTO backup_configurations (
			id, name, description, server_id, app_id, database_id, volume_id, backup_type,
			is_scheduled, cron_expression, next_run_at, last_run_at, storage_provider,
			storage_config, max_backups, retention_days, compression_enabled,
			encryption_enabled, encryption_key_id, enabled, last_status, last_error, data
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23
		)
		RETURNING created_at, updated_at
	`, record.ID, record.Name, record.Description, record.ServerID, record.AppID,
		record.DatabaseID, record.VolumeID, record.BackupType, record.IsScheduled,
		record.CronExpression, record.NextRunAt, record.LastRunAt, record.StorageProvider,
		record.StorageConfig, record.MaxBackups, record.RetentionDays,
		record.CompressionEnabled, record.EncryptionEnabled, record.EncryptionKeyID,
		record.Enabled, record.LastStatus, record.LastError, record.Data,
	).Scan(&record.CreatedAt, &record.UpdatedAt)
}

func (s *Store) GetBackupConfiguration(ctx context.Context, id string) (BackupConfigurationRecord, error) {
	return scanBackupConfiguration(s.db.QueryRow(ctx,
		`SELECT `+backupConfigurationColumns+` FROM backup_configurations WHERE id=$1`, id))
}

func (s *Store) ListBackupConfigurations(ctx context.Context) ([]BackupConfigurationRecord, error) {
	rows, err := s.db.Query(ctx, `SELECT `+backupConfigurationColumns+` FROM backup_configurations ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]BackupConfigurationRecord, 0)
	for rows.Next() {
		record, err := scanBackupConfiguration(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) UpdateBackupConfiguration(ctx context.Context, record *BackupConfigurationRecord) error {
	if len(record.StorageConfig) == 0 {
		record.StorageConfig = json.RawMessage(`{}`)
	}
	if len(record.Data) == 0 {
		record.Data = json.RawMessage(`{}`)
	}
	command, err := s.db.Exec(ctx, `
		UPDATE backup_configurations SET
			name=$2, description=$3, is_scheduled=$4, cron_expression=$5, next_run_at=$6,
			last_run_at=$7, storage_provider=$8, storage_config=$9, max_backups=$10,
			retention_days=$11, compression_enabled=$12, encryption_enabled=$13,
			encryption_key_id=$14, enabled=$15, last_status=$16, last_error=$17, data=$18
		WHERE id=$1
	`, record.ID, record.Name, record.Description, record.IsScheduled, record.CronExpression,
		record.NextRunAt, record.LastRunAt, record.StorageProvider, record.StorageConfig,
		record.MaxBackups, record.RetentionDays, record.CompressionEnabled,
		record.EncryptionEnabled, record.EncryptionKeyID, record.Enabled, record.LastStatus,
		record.LastError, record.Data)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	updated, err := s.GetBackupConfiguration(ctx, record.ID)
	if err == nil {
		record.UpdatedAt = updated.UpdatedAt
	}
	return err
}

func (s *Store) DeleteBackupConfiguration(ctx context.Context, id string) error {
	command, err := s.db.Exec(ctx, `DELETE FROM backup_configurations WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

type BackupArtifactRecord struct {
	ID              string
	JobID           *string
	ConfigurationID *string
	ArtifactType    string
	Name            string
	DisplayName     string
	StorageProvider string
	StoragePath     string
	FileSize        int64
	FileHash        *string
	Status          string
	IsVerified      bool
	IsLocked        bool
	LockReason      *string
	Data            json.RawMessage
	CreatedAt       time.Time
	UpdatedAt       time.Time
	UploadedAt      *time.Time
}

const backupArtifactColumns = `
	id, job_id, configuration_id, artifact_type, name, COALESCE(display_name, ''),
	storage_provider, storage_path, file_size, file_hash, status, is_verified,
	is_locked, lock_reason, data, created_at, updated_at, uploaded_at`

func scanBackupArtifact(row pgx.Row) (BackupArtifactRecord, error) {
	var record BackupArtifactRecord
	err := row.Scan(&record.ID, &record.JobID, &record.ConfigurationID, &record.ArtifactType,
		&record.Name, &record.DisplayName, &record.StorageProvider, &record.StoragePath,
		&record.FileSize, &record.FileHash, &record.Status, &record.IsVerified,
		&record.IsLocked, &record.LockReason, &record.Data, &record.CreatedAt,
		&record.UpdatedAt, &record.UploadedAt)
	return record, err
}

func (s *Store) CreateBackupArtifactRecord(ctx context.Context, record *BackupArtifactRecord) error {
	if record.ID == "" {
		record.ID = uuid.NewString()
	}
	if len(record.Data) == 0 {
		record.Data = json.RawMessage(`{}`)
	}
	return s.db.QueryRow(ctx, `
		INSERT INTO backup_artifacts (
			id, job_id, configuration_id, artifact_type, name, display_name,
			storage_provider, storage_path, file_size, file_hash, status,
			is_verified, is_locked, lock_reason, data, uploaded_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		RETURNING created_at, updated_at, uploaded_at
	`, record.ID, record.JobID, record.ConfigurationID, record.ArtifactType, record.Name,
		record.DisplayName, record.StorageProvider, record.StoragePath, record.FileSize,
		record.FileHash, record.Status, record.IsVerified, record.IsLocked,
		record.LockReason, record.Data, record.UploadedAt,
	).Scan(&record.CreatedAt, &record.UpdatedAt, &record.UploadedAt)
}

func (s *Store) GetBackupArtifactRecord(ctx context.Context, id string) (BackupArtifactRecord, error) {
	return scanBackupArtifact(s.db.QueryRow(ctx,
		`SELECT `+backupArtifactColumns+` FROM backup_artifacts WHERE id=$1`, id))
}

func (s *Store) ListBackupArtifactRecords(ctx context.Context) ([]BackupArtifactRecord, error) {
	rows, err := s.db.Query(ctx, `SELECT `+backupArtifactColumns+` FROM backup_artifacts WHERE status <> 'deleted' ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]BackupArtifactRecord, 0)
	for rows.Next() {
		record, err := scanBackupArtifact(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) UpdateBackupArtifactRecord(ctx context.Context, record *BackupArtifactRecord) error {
	if len(record.Data) == 0 {
		record.Data = json.RawMessage(`{}`)
	}
	command, err := s.db.Exec(ctx, `
		UPDATE backup_artifacts SET display_name=$2, file_size=$3, file_hash=$4,
			status=$5, is_verified=$6, is_locked=$7, lock_reason=$8, data=$9,
			uploaded_at=$10
		WHERE id=$1
	`, record.ID, record.DisplayName, record.FileSize, record.FileHash, record.Status,
		record.IsVerified, record.IsLocked, record.LockReason, record.Data,
		record.UploadedAt)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteBackupArtifactRecord(ctx context.Context, id string) error {
	command, err := s.db.Exec(ctx, `UPDATE backup_artifacts SET status='deleted' WHERE id=$1 AND is_locked=FALSE`, id)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		record, lookupErr := s.GetBackupArtifactRecord(ctx, id)
		if lookupErr != nil {
			return lookupErr
		}
		if record.IsLocked {
			return errors.New("locked backup artifact cannot be deleted")
		}
		return pgx.ErrNoRows
	}
	return nil
}

type BackupStorageProviderRecord struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Type           string          `json:"type"`
	Config         json.RawMessage `json:"-"`
	Enabled        bool            `json:"enabled"`
	IsDefault      bool            `json:"isDefault"`
	LastTestAt     *time.Time      `json:"lastTestAt,omitempty"`
	LastTestStatus *string         `json:"lastTestStatus,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

func (s *Store) CreateBackupStorageProvider(ctx context.Context, record *BackupStorageProviderRecord) error {
	if record.ID == "" {
		record.ID = uuid.NewString()
	}
	encrypted, err := s.encryptSecret(string(record.Config), secretAAD("backup_storage_providers", record.ID, "config"))
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if record.IsDefault {
		if _, err := tx.Exec(ctx, `UPDATE backup_storage_providers SET is_default=FALSE WHERE is_default=TRUE`); err != nil {
			return err
		}
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO backup_storage_providers
			(id, name, provider_type, config, config_encrypted, enabled, is_default)
		VALUES ($1,$2,$3,'{}'::jsonb,$4,$5,$6)
		RETURNING created_at, updated_at
	`, record.ID, record.Name, record.Type, encrypted, record.Enabled, record.IsDefault).
		Scan(&record.CreatedAt, &record.UpdatedAt)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) GetBackupStorageProvider(ctx context.Context, name string) (BackupStorageProviderRecord, error) {
	var record BackupStorageProviderRecord
	var plaintext, encrypted string
	err := s.db.QueryRow(ctx, `
		SELECT id, name, provider_type, config::text, config_encrypted, enabled, is_default,
			last_test_at, last_test_status, created_at, updated_at
		FROM backup_storage_providers WHERE name=$1
	`, name).Scan(&record.ID, &record.Name, &record.Type, &plaintext, &encrypted,
		&record.Enabled, &record.IsDefault, &record.LastTestAt, &record.LastTestStatus,
		&record.CreatedAt, &record.UpdatedAt)
	if err != nil {
		return record, err
	}
	decrypted, err := s.decryptSecret(encrypted, plaintext, secretAAD("backup_storage_providers", record.ID, "config"))
	if err != nil {
		return record, err
	}
	record.Config = json.RawMessage(decrypted)
	return record, nil
}

func (s *Store) SetDefaultBackupStorageProvider(ctx context.Context, name string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM backup_storage_providers WHERE name=$1 AND enabled=TRUE)`, name).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return pgx.ErrNoRows
	}
	if _, err := tx.Exec(ctx, `UPDATE backup_storage_providers SET is_default=(name=$1)`, name); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ListBackupStorageProviders(ctx context.Context) ([]BackupStorageProviderRecord, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, provider_type, enabled, is_default,
			last_test_at, last_test_status, created_at, updated_at
		FROM backup_storage_providers
		ORDER BY is_default DESC, name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]BackupStorageProviderRecord, 0)
	for rows.Next() {
		var record BackupStorageProviderRecord
		if err := rows.Scan(&record.ID, &record.Name, &record.Type,
			&record.Enabled, &record.IsDefault, &record.LastTestAt,
			&record.LastTestStatus, &record.CreatedAt, &record.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

type BackupRetentionPolicyRecord struct {
	ID              string
	Name            string
	Description     string
	Scope           string
	ServerID        *string
	AppID           *string
	DatabaseID      *string
	VolumeID        *string
	MaxBackups      int
	RetentionDays   int
	RetentionWeeks  int
	RetentionMonths int
	CleanupSchedule *string
	LastCleanupAt   *time.Time
	NextCleanupAt   *time.Time
	Priority        int
	Enabled         bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

const backupRetentionColumns = `
	id, name, COALESCE(description,''), scope, server_id, app_id, database_id, volume_id,
	COALESCE(max_backups,0), COALESCE(retention_days,0), COALESCE(retention_weeks,0),
	COALESCE(retention_months,0), cleanup_schedule, last_cleanup_at, next_cleanup_at,
	priority, enabled, created_at, updated_at`

func scanBackupRetention(row pgx.Row) (BackupRetentionPolicyRecord, error) {
	var record BackupRetentionPolicyRecord
	err := row.Scan(&record.ID, &record.Name, &record.Description, &record.Scope,
		&record.ServerID, &record.AppID, &record.DatabaseID, &record.VolumeID,
		&record.MaxBackups, &record.RetentionDays, &record.RetentionWeeks,
		&record.RetentionMonths, &record.CleanupSchedule, &record.LastCleanupAt,
		&record.NextCleanupAt, &record.Priority, &record.Enabled, &record.CreatedAt,
		&record.UpdatedAt)
	return record, err
}

func (s *Store) CreateBackupRetentionPolicy(ctx context.Context, record *BackupRetentionPolicyRecord) error {
	if record.ID == "" {
		record.ID = uuid.NewString()
	}
	return s.db.QueryRow(ctx, `
		INSERT INTO backup_retention_policies (
			id,name,description,scope,server_id,app_id,database_id,volume_id,max_backups,
			retention_days,retention_weeks,retention_months,cleanup_schedule,last_cleanup_at,
			next_cleanup_at,priority,enabled
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING created_at,updated_at
	`, record.ID, record.Name, record.Description, record.Scope, record.ServerID,
		record.AppID, record.DatabaseID, record.VolumeID, nullablePositive(record.MaxBackups),
		nullablePositive(record.RetentionDays), nullablePositive(record.RetentionWeeks),
		nullablePositive(record.RetentionMonths), record.CleanupSchedule, record.LastCleanupAt,
		record.NextCleanupAt, record.Priority, record.Enabled).
		Scan(&record.CreatedAt, &record.UpdatedAt)
}

func nullablePositive(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

func (s *Store) GetBackupRetentionPolicy(ctx context.Context, id string) (BackupRetentionPolicyRecord, error) {
	return scanBackupRetention(s.db.QueryRow(ctx,
		`SELECT `+backupRetentionColumns+` FROM backup_retention_policies WHERE id=$1`, id))
}

func (s *Store) ListBackupRetentionPolicies(ctx context.Context) ([]BackupRetentionPolicyRecord, error) {
	rows, err := s.db.Query(ctx, `SELECT `+backupRetentionColumns+` FROM backup_retention_policies ORDER BY priority, created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]BackupRetentionPolicyRecord, 0)
	for rows.Next() {
		record, err := scanBackupRetention(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) UpdateBackupRetentionPolicy(ctx context.Context, record *BackupRetentionPolicyRecord) error {
	command, err := s.db.Exec(ctx, `
		UPDATE backup_retention_policies SET name=$2,description=$3,max_backups=$4,
			retention_days=$5,retention_weeks=$6,retention_months=$7,cleanup_schedule=$8,
			last_cleanup_at=$9,next_cleanup_at=$10,priority=$11,enabled=$12
		WHERE id=$1
	`, record.ID, record.Name, record.Description, nullablePositive(record.MaxBackups),
		nullablePositive(record.RetentionDays), nullablePositive(record.RetentionWeeks),
		nullablePositive(record.RetentionMonths), record.CleanupSchedule, record.LastCleanupAt,
		record.NextCleanupAt, record.Priority, record.Enabled)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteBackupRetentionPolicy(ctx context.Context, id string) error {
	command, err := s.db.Exec(ctx, `DELETE FROM backup_retention_policies WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
