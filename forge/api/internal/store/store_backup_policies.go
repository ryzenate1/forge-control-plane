package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type BackupPolicy struct {
	ID                 string    `json:"id"`
	ServerID           string    `json:"serverId"`
	AppID              string    `json:"appId,omitempty"`
	ServiceID          string    `json:"serviceId,omitempty"`
	DatabaseID         string    `json:"databaseId,omitempty"`
	DatabaseType       string    `json:"databaseType,omitempty"`
	VolumeBackup       bool      `json:"volumeBackup"`
	Interval           string    `json:"interval"`
	MaxBackups         int       `json:"maxBackups"`
	RetentionDays      int       `json:"retentionDays"`
	Storage            string    `json:"storage"`
	Compress           bool      `json:"compress"`
	Encrypted          bool      `json:"encrypted"`
	EncryptionAlgorithm string   `json:"encryptionAlgorithm,omitempty"`
	EncryptionKey      string    `json:"encryptionKey,omitempty"`
	Enabled            bool      `json:"enabled"`
	IsLocked           bool      `json:"isLocked"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

func (s *Store) CreateBackupPolicy(ctx context.Context, p *BackupPolicy) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO backup_policies (id, server_id, app_id, service_id, database_id, database_type, volume_backup, interval, max_backups, retention_days, storage, compress, encrypted, encryption_algorithm, encryption_key, enabled, is_locked, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, now(), now())
	`, p.ID, p.ServerID, nullIfEmpty(p.AppID), nullIfEmpty(p.ServiceID), nullIfEmpty(p.DatabaseID), p.DatabaseType, p.VolumeBackup, p.Interval, p.MaxBackups, p.RetentionDays, p.Storage, p.Compress, p.Encrypted, p.EncryptionAlgorithm, p.EncryptionKey, p.Enabled, p.IsLocked)
	return err
}

func (s *Store) GetBackupPolicy(ctx context.Context, id string) (BackupPolicy, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id::text, server_id::text, COALESCE(app_id, ''), COALESCE(service_id, ''), COALESCE(database_id, ''), COALESCE(database_type, ''), volume_backup, interval, max_backups, retention_days, storage, compress, encrypted, encryption_algorithm, encryption_key, enabled, is_locked, created_at, updated_at
		FROM backup_policies
		WHERE id = $1
	`, id)
	var p BackupPolicy
	err := row.Scan(&p.ID, &p.ServerID, &p.AppID, &p.ServiceID, &p.DatabaseID, &p.DatabaseType, &p.VolumeBackup, &p.Interval, &p.MaxBackups, &p.RetentionDays, &p.Storage, &p.Compress, &p.Encrypted, &p.EncryptionAlgorithm, &p.EncryptionKey, &p.Enabled, &p.IsLocked, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (s *Store) ListBackupPolicies(ctx context.Context, serverID string) ([]BackupPolicy, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, COALESCE(app_id, ''), COALESCE(service_id, ''), COALESCE(database_id, ''), COALESCE(database_type, ''), volume_backup, interval, max_backups, retention_days, storage, compress, encrypted, encryption_algorithm, encryption_key, enabled, is_locked, created_at, updated_at
		FROM backup_policies
		WHERE server_id = $1
		ORDER BY created_at DESC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var policies []BackupPolicy
	for rows.Next() {
		var p BackupPolicy
		if err := rows.Scan(&p.ID, &p.ServerID, &p.AppID, &p.ServiceID, &p.DatabaseID, &p.DatabaseType, &p.VolumeBackup, &p.Interval, &p.MaxBackups, &p.RetentionDays, &p.Storage, &p.Compress, &p.Encrypted, &p.EncryptionAlgorithm, &p.EncryptionKey, &p.Enabled, &p.IsLocked, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *Store) UpdateBackupPolicy(ctx context.Context, p *BackupPolicy) error {
	_, err := s.db.Exec(ctx, `
		UPDATE backup_policies
		SET app_id = $2, service_id = $3, database_id = $4, database_type = $5, volume_backup = $6, interval = $7, max_backups = $8, retention_days = $9, storage = $10, compress = $11, encrypted = $12, encryption_algorithm = $13, encryption_key = $14, enabled = $15, is_locked = $16, updated_at = now()
		WHERE id = $1
	`, p.ID, nullIfEmpty(p.AppID), nullIfEmpty(p.ServiceID), nullIfEmpty(p.DatabaseID), p.DatabaseType, p.VolumeBackup, p.Interval, p.MaxBackups, p.RetentionDays, p.Storage, p.Compress, p.Encrypted, p.EncryptionAlgorithm, p.EncryptionKey, p.Enabled, p.IsLocked)
	return err
}

func (s *Store) DeleteBackupPolicy(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `
		DELETE FROM backup_policies WHERE id = $1
	`, id)
	return err
}

func (s *Store) ListAllEnabledBackupPolicies(ctx context.Context) ([]BackupPolicy, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, COALESCE(app_id, ''), COALESCE(service_id, ''), COALESCE(database_id, ''), COALESCE(database_type, ''), volume_backup, interval, max_backups, retention_days, storage, compress, encrypted, encryption_algorithm, encryption_key, enabled, is_locked, created_at, updated_at
		FROM backup_policies
		WHERE enabled = TRUE AND interval != ''
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var policies []BackupPolicy
	for rows.Next() {
		var p BackupPolicy
		if err := rows.Scan(&p.ID, &p.ServerID, &p.AppID, &p.ServiceID, &p.DatabaseID, &p.DatabaseType, &p.VolumeBackup, &p.Interval, &p.MaxBackups, &p.RetentionDays, &p.Storage, &p.Compress, &p.Encrypted, &p.EncryptionAlgorithm, &p.EncryptionKey, &p.Enabled, &p.IsLocked, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *Store) ListAllEnabledPolicies(ctx context.Context) ([]BackupPolicy, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, COALESCE(app_id, ''), COALESCE(service_id, ''), COALESCE(database_id, ''), COALESCE(database_type, ''), volume_backup, interval, max_backups, retention_days, storage, compress, encrypted, encryption_algorithm, encryption_key, enabled, is_locked, created_at, updated_at
		FROM backup_policies
		WHERE enabled = TRUE
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var policies []BackupPolicy
	for rows.Next() {
		var p BackupPolicy
		if err := rows.Scan(&p.ID, &p.ServerID, &p.AppID, &p.ServiceID, &p.DatabaseID, &p.DatabaseType, &p.VolumeBackup, &p.Interval, &p.MaxBackups, &p.RetentionDays, &p.Storage, &p.Compress, &p.Encrypted, &p.EncryptionAlgorithm, &p.EncryptionKey, &p.Enabled, &p.IsLocked, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *Store) ListBackupPoliciesByApp(ctx context.Context, appID string) ([]BackupPolicy, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, COALESCE(app_id, ''), COALESCE(service_id, ''), COALESCE(database_id, ''), COALESCE(database_type, ''), volume_backup, interval, max_backups, retention_days, storage, compress, encrypted, encryption_algorithm, encryption_key, enabled, is_locked, created_at, updated_at
		FROM backup_policies
		WHERE app_id = $1
		ORDER BY created_at DESC
	`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var policies []BackupPolicy
	for rows.Next() {
		var p BackupPolicy
		if err := rows.Scan(&p.ID, &p.ServerID, &p.AppID, &p.ServiceID, &p.DatabaseID, &p.DatabaseType, &p.VolumeBackup, &p.Interval, &p.MaxBackups, &p.RetentionDays, &p.Storage, &p.Compress, &p.Encrypted, &p.EncryptionAlgorithm, &p.EncryptionKey, &p.Enabled, &p.IsLocked, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *Store) ListBackupPoliciesByDatabase(ctx context.Context, databaseID string) ([]BackupPolicy, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, COALESCE(app_id, ''), COALESCE(service_id, ''), COALESCE(database_id, ''), COALESCE(database_type, ''), volume_backup, interval, max_backups, retention_days, storage, compress, encrypted, encryption_algorithm, encryption_key, enabled, is_locked, created_at, updated_at
		FROM backup_policies
		WHERE database_id = $1
		ORDER BY created_at DESC
	`, databaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var policies []BackupPolicy
	for rows.Next() {
		var p BackupPolicy
		if err := rows.Scan(&p.ID, &p.ServerID, &p.AppID, &p.ServiceID, &p.DatabaseID, &p.DatabaseType, &p.VolumeBackup, &p.Interval, &p.MaxBackups, &p.RetentionDays, &p.Storage, &p.Compress, &p.Encrypted, &p.EncryptionAlgorithm, &p.EncryptionKey, &p.Enabled, &p.IsLocked, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *Store) DeleteOrphanedBackupPolicies(ctx context.Context) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		DELETE FROM backup_policies
		WHERE (app_id IS NOT NULL AND app_id != ''
		       AND NOT EXISTS (SELECT 1 FROM applications WHERE id = app_id))
		   OR (database_id IS NOT NULL AND database_id != ''
		       AND NOT EXISTS (SELECT 1 FROM server_databases WHERE id = database_id))
	`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) ListAllBackupPolicies(ctx context.Context) ([]BackupPolicy, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, COALESCE(app_id, ''), COALESCE(service_id, ''), COALESCE(database_id, ''), COALESCE(database_type, ''), volume_backup, interval, max_backups, retention_days, storage, compress, encrypted, encryption_algorithm, encryption_key, enabled, is_locked, created_at, updated_at
		FROM backup_policies
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var policies []BackupPolicy
	for rows.Next() {
		var p BackupPolicy
		if err := rows.Scan(&p.ID, &p.ServerID, &p.AppID, &p.ServiceID, &p.DatabaseID, &p.DatabaseType, &p.VolumeBackup, &p.Interval, &p.MaxBackups, &p.RetentionDays, &p.Storage, &p.Compress, &p.Encrypted, &p.EncryptionAlgorithm, &p.EncryptionKey, &p.Enabled, &p.IsLocked, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *Store) CleanupBackupPoliciesByApp(ctx context.Context, appID string) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		DELETE FROM backup_policies WHERE app_id = $1
	`, appID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) CleanupBackupPoliciesByDatabase(ctx context.Context, databaseID string) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		DELETE FROM backup_policies WHERE database_id = $1
	`, databaseID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) LockBackupPolicy(ctx context.Context, id string, actorID *string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE backup_policies
		SET is_locked = TRUE, updated_at = now()
		WHERE id = $1
	`, id)
	return err
}

func (s *Store) UnlockBackupPolicy(ctx context.Context, id string, actorID *string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE backup_policies
		SET is_locked = FALSE, updated_at = now()
		WHERE id = $1
	`, id)
	return err
}

func (s *Store) ListExpiredBackups(ctx context.Context) ([]Backup, error) {
	rows, err := s.db.Query(ctx, `
		SELECT uuid::text, server_id::text, name, checksum, size, status, upload_id, completed_at, created_at, updated_at, is_locked, status_message, status_callback, retry_count, last_retry_at,
		       COALESCE(source_type, ''), COALESCE(source_id, ''), COALESCE(database_type, ''), COALESCE(volume_name, ''), COALESCE(manifest, '{}'), COALESCE(storage_receipt, '{}'), checksum_verified, restore_count, last_restore_at,
		       compressed, encrypted, COALESCE(nonce, '')
		FROM backups
		WHERE status = 'completed'
		AND is_locked = FALSE
		AND created_at < now() - interval '30 days'
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var backups []Backup
	for rows.Next() {
		var b Backup
		if err := rows.Scan(&b.UUID, &b.ServerID, &b.Name, &b.Checksum, &b.Size, &b.Status, &b.UploadID, &b.CompletedAt, &b.CreatedAt, &b.UpdatedAt, &b.IsLocked, &b.StatusMessage, &b.StatusCallback, &b.RetryCount, &b.LastRetryAt, &b.SourceType, &b.SourceID, &b.DatabaseType, &b.VolumeName, &b.Manifest, &b.StorageReceipt, &b.ChecksumVerified, &b.RestoreCount, &b.LastRestoreAt, &b.Compressed, &b.Encrypted, &b.Nonce); err != nil {
			return nil, err
		}
		backups = append(backups, b)
	}
	return backups, rows.Err()
}
