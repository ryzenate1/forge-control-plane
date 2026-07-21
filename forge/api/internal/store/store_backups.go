package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

func (s *Store) ListBackups(ctx context.Context, serverID string, page, perPage int) ([]Backup, error) {
	offset := (page - 1) * perPage
	rows, err := s.db.Query(ctx, `
		SELECT uuid::text, server_id::text, name, checksum, size, status, upload_id, completed_at, created_at, updated_at, is_locked, status_message, status_callback, retry_count, last_retry_at,
		       COALESCE(source_type, ''), COALESCE(source_id, ''), COALESCE(database_type, ''), COALESCE(volume_name, ''), COALESCE(manifest, '{}'), COALESCE(storage_receipt, '{}'), checksum_verified, restore_count, last_restore_at,
		       compressed, encrypted, COALESCE(nonce, '')
		FROM backups
		WHERE server_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, serverID, perPage, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	backups := []Backup{}
	for rows.Next() {
		var backup Backup
		if err := rows.Scan(&backup.UUID, &backup.ServerID, &backup.Name, &backup.Checksum, &backup.Size, &backup.Status, &backup.UploadID, &backup.CompletedAt, &backup.CreatedAt, &backup.UpdatedAt, &backup.IsLocked, &backup.StatusMessage, &backup.StatusCallback, &backup.RetryCount, &backup.LastRetryAt, &backup.SourceType, &backup.SourceID, &backup.DatabaseType, &backup.VolumeName, &backup.Manifest, &backup.StorageReceipt, &backup.ChecksumVerified, &backup.RestoreCount, &backup.LastRestoreAt, &backup.Compressed, &backup.Encrypted, &backup.Nonce); err != nil {
			return nil, err
		}
		backups = append(backups, backup)
	}
	return backups, rows.Err()
}

func (s *Store) CountBackups(ctx context.Context, serverID string) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM backups WHERE server_id = $1`, serverID).Scan(&count)
	return count, err
}

func (s *Store) CountCompletedBackups(ctx context.Context, serverID string) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM backups WHERE server_id = $1 AND status = 'completed'`, serverID).Scan(&count)
	return count, err
}

func (s *Store) BackupLimit(ctx context.Context, serverID string) (int, error) {
	var limit int
	err := s.db.QueryRow(ctx, `SELECT COALESCE(backup_limit, 0) FROM servers WHERE id = $1`, serverID).Scan(&limit)
	return limit, err
}

func (s *Store) UpsertBackup(ctx context.Context, serverID string, req UpsertBackupRequest, actorID *string) (Backup, error) {
	if req.UUID == "" {
		req.UUID = uuid.NewString()
	}
	if req.Name == "" {
		return Backup{}, errors.New("backup name is required")
	}
	if req.Status == "" {
		req.Status = "completed"
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO backups (uuid, server_id, name, checksum, size, status, upload_id, completed_at, updated_at, is_locked, status_message, status_callback, retry_count,
		                     source_type, source_id, database_type, volume_name, manifest, storage_receipt, checksum_verified, restore_count, last_restore_at,
		                     compressed, encrypted, nonce)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), FALSE, $9, $10, COALESCE($11, 0),
		        $12, $13, $14, $15, $16, $17, $18, $19, $20,
		        $21, $22, $23)
		ON CONFLICT (server_id, name) DO UPDATE SET
			checksum = EXCLUDED.checksum,
			size = EXCLUDED.size,
			status = EXCLUDED.status,
			upload_id = EXCLUDED.upload_id,
			completed_at = EXCLUDED.completed_at,
			updated_at = now(),
			status_message = EXCLUDED.status_message,
			status_callback = EXCLUDED.status_callback,
			retry_count = EXCLUDED.retry_count,
			source_type = EXCLUDED.source_type,
			source_id = EXCLUDED.source_id,
			database_type = EXCLUDED.database_type,
			volume_name = EXCLUDED.volume_name,
			manifest = EXCLUDED.manifest,
			storage_receipt = EXCLUDED.storage_receipt,
			checksum_verified = EXCLUDED.checksum_verified,
			restore_count = EXCLUDED.restore_count,
			last_restore_at = EXCLUDED.last_restore_at,
			compressed = EXCLUDED.compressed,
			encrypted = EXCLUDED.encrypted,
			nonce = EXCLUDED.nonce
	`, req.UUID, serverID, req.Name, req.Checksum, req.Size, req.Status, req.UploadID, req.CompletedAt, req.StatusMessage, req.StatusCallback, req.RetryCount,
		req.SourceType, req.SourceID, req.DatabaseType, req.VolumeName, jsonOrNull(req.Manifest), jsonOrNull(req.StorageReceipt), req.ChecksumVerified, req.RestoreCount, req.LastRestoreAt,
		req.Compressed, req.Encrypted, req.Nonce)
	if err != nil {
		return Backup{}, err
	}
	_ = s.AppendAudit(ctx, actorID, "backup "+req.Status, "server", &serverID, mustAuditJSON(map[string]any{"name": req.Name, "size": req.Size, "checksum": req.Checksum}))
	return s.GetBackupByName(ctx, serverID, req.Name)
}

func (s *Store) GetBackupByName(ctx context.Context, serverID, name string) (Backup, error) {
	var backup Backup
	err := s.db.QueryRow(ctx, `
		SELECT uuid::text, server_id::text, name, checksum, size, status, upload_id, completed_at, created_at, updated_at, is_locked, status_message, status_callback, retry_count, last_retry_at,
		       COALESCE(source_type, ''), COALESCE(source_id, ''), COALESCE(database_type, ''), COALESCE(volume_name, ''), COALESCE(manifest, '{}'), COALESCE(storage_receipt, '{}'), checksum_verified, restore_count, last_restore_at,
		       compressed, encrypted, COALESCE(nonce, '')
		FROM backups
		WHERE server_id = $1 AND name = $2
	`, serverID, name).Scan(&backup.UUID, &backup.ServerID, &backup.Name, &backup.Checksum, &backup.Size, &backup.Status, &backup.UploadID, &backup.CompletedAt, &backup.CreatedAt, &backup.UpdatedAt, &backup.IsLocked, &backup.StatusMessage, &backup.StatusCallback, &backup.RetryCount, &backup.LastRetryAt, &backup.SourceType, &backup.SourceID, &backup.DatabaseType, &backup.VolumeName, &backup.Manifest, &backup.StorageReceipt, &backup.ChecksumVerified, &backup.RestoreCount, &backup.LastRestoreAt, &backup.Compressed, &backup.Encrypted, &backup.Nonce)
	return backup, err
}

func (s *Store) MarkBackupStatus(ctx context.Context, serverID, name, status string, actorID *string) error {
	commandTag, err := s.db.Exec(ctx, `
		UPDATE backups
		SET status = $3, updated_at = now()
		WHERE server_id = $1 AND name = $2
	`, serverID, name, status)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("backup not found")
	}
	return s.AppendAudit(ctx, actorID, "backup "+status, "server", &serverID, mustAuditJSON(map[string]any{"name": name}))
}

func (s *Store) DeleteBackup(ctx context.Context, serverID, name string, actorID *string) error {
	// Check if backup is locked before deletion
	var isLocked bool
	err := s.db.QueryRow(ctx, `
		SELECT is_locked FROM backups WHERE server_id = $1 AND name = $2
	`, serverID, name).Scan(&isLocked)
	if err != nil {
		return errors.New("backup not found")
	}
	if isLocked {
		return errors.New("backup is locked and cannot be deleted")
	}

	commandTag, err := s.db.Exec(ctx, `
		DELETE FROM backups
		WHERE server_id = $1 AND name = $2
	`, serverID, name)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("backup not found")
	}
	return s.AppendAudit(ctx, actorID, "backup deleted", "server", &serverID, mustAuditJSON(map[string]any{"name": name}))
}

func (s *Store) LockBackup(ctx context.Context, serverID, name string, actorID *string) error {
	commandTag, err := s.db.Exec(ctx, `
		UPDATE backups
		SET is_locked = TRUE, updated_at = now()
		WHERE server_id = $1 AND name = $2
	`, serverID, name)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("backup not found")
	}
	return s.AppendAudit(ctx, actorID, "backup locked", "server", &serverID, mustAuditJSON(map[string]any{"name": name}))
}

func (s *Store) UnlockBackup(ctx context.Context, serverID, name string, actorID *string) error {
	commandTag, err := s.db.Exec(ctx, `
		UPDATE backups
		SET is_locked = FALSE, updated_at = now()
		WHERE server_id = $1 AND name = $2
	`, serverID, name)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("backup not found")
	}
	return s.AppendAudit(ctx, actorID, "backup unlocked", "server", &serverID, mustAuditJSON(map[string]any{"name": name}))
}

func (s *Store) RenameBackup(ctx context.Context, serverID, oldName, newName string, actorID *string) error {
	if oldName == "" || newName == "" {
		return errors.New("backup name is required")
	}

	var isLocked bool
	err := s.db.QueryRow(ctx, `SELECT is_locked FROM backups WHERE server_id = $1 AND name = $2`, serverID, oldName).Scan(&isLocked)
	if err != nil {
		return errors.New("backup not found")
	}
	if isLocked {
		return errors.New("backup is locked and cannot be renamed")
	}

	var exists bool
	err = s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM backups WHERE server_id = $1 AND name = $2)`, serverID, newName).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("a backup with the new name already exists")
	}

	commandTag, err := s.db.Exec(ctx, `
		UPDATE backups
		SET name = $3, updated_at = now()
		WHERE server_id = $1 AND name = $2
	`, serverID, oldName, newName)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("backup not found")
	}
	return s.AppendAudit(ctx, actorID, "backup renamed", "server", &serverID, mustAuditJSON(map[string]any{"from": oldName, "to": newName}))
}

// CleanupOldBackups removes backups that exceed the retention policy
// This function respects backup locking and server-specific limits
func (s *Store) CleanupOldBackups(ctx context.Context, retentionDays int, autoCleanup bool) (int, error) {
	if !autoCleanup || retentionDays <= 0 {
		return 0, nil
	}

	// Delete old backups that are not locked and exceed retention period
	commandTag, err := s.db.Exec(ctx, `
		DELETE FROM backups
		WHERE is_locked = FALSE
		AND created_at < now() - interval '1 day' * $1
		AND status = 'completed'
	`, retentionDays)
	if err != nil {
		return 0, err
	}

	return int(commandTag.RowsAffected()), nil
}

// CleanupOldBackupsForServer removes old backups for a specific server
// This function respects backup locking and server-specific limits
func (s *Store) CleanupOldBackupsForServer(ctx context.Context, serverID string, retentionDays int, backupLimit int) (int, error) {
	// First, get count of completed backups
	count, err := s.CountCompletedBackups(ctx, serverID)
	if err != nil {
		return 0, err
	}

	// If under limit, no cleanup needed
	if backupLimit > 0 && count <= backupLimit {
		return 0, nil
	}

	// Delete oldest unlocked backups that exceed retention or limit
	// Prioritize deleting oldest non-locked backups
	commandTag, err := s.db.Exec(ctx, `
		DELETE FROM backups
		WHERE server_id = $1
		AND is_locked = FALSE
		AND status = 'completed'
		AND (
			-- Delete if older than retention days
			created_at < now() - interval '1 day' * $2
			OR
			-- Or if we're over the limit, delete oldest (except locked ones)
			uuid IN (
				SELECT uuid FROM backups
				WHERE server_id = $1
				AND is_locked = FALSE
				AND status = 'completed'
				ORDER BY created_at ASC
				LIMIT CASE WHEN $3 > 0 THEN GREATEST(0, (SELECT COUNT(*) FROM backups WHERE server_id = $1 AND status = 'completed') - $3) ELSE 0 END
			)
		)
	`, serverID, retentionDays, backupLimit)
	if err != nil {
		return 0, err
	}

	return int(commandTag.RowsAffected()), nil
}

// UpdateBackupStatusWithCallback updates backup status with callback URL and message
func (s *Store) UpdateBackupStatusWithCallback(ctx context.Context, serverID, name, status, message, callback string) error {
	commandTag, err := s.db.Exec(ctx, `
		UPDATE backups
		SET status = $4, status_message = $5, status_callback = $6, updated_at = now()
		WHERE server_id = $1 AND name = $2
	`, serverID, name, status, message, callback)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("backup not found")
	}
	return nil
}

// IncrementBackupRetry increments retry count and updates last retry time
func (s *Store) IncrementBackupRetry(ctx context.Context, serverID, name string) error {
	commandTag, err := s.db.Exec(ctx, `
		UPDATE backups
		SET retry_count = retry_count + 1, last_retry_at = now(), updated_at = now()
		WHERE server_id = $1 AND name = $2
	`, serverID, name)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("backup not found")
	}
	return nil
}

// CountRecentBackups counts backups created for a server within a time window
func (s *Store) CountRecentBackups(ctx context.Context, serverID string, windowMinutes int) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `
		SELECT count(*) FROM backups
		WHERE server_id = $1
		AND created_at > now() - interval '1 minute' * $2
	`, serverID, windowMinutes).Scan(&count)
	return count, err
}

// FailStaleBackups marks backups stuck in pending/running state past the threshold as failed
func (s *Store) FailStaleBackups(ctx context.Context, pruneAgeMinutes int) (int64, error) {
	if pruneAgeMinutes <= 0 {
		pruneAgeMinutes = 360
	}
	commandTag, err := s.db.Exec(ctx, `
		UPDATE backups
		SET status = 'failed', status_message = 'backup timed out', updated_at = now()
		WHERE status IN ('pending', 'running')
		AND created_at < now() - interval '1 minute' * $1
	`, pruneAgeMinutes)
	if err != nil {
		return 0, err
	}
	return commandTag.RowsAffected(), nil
}

// GetBackupsByStatus retrieves backups with specific status (for callback processing)
func (s *Store) GetBackupsByStatus(ctx context.Context, status string) ([]Backup, error) {
	rows, err := s.db.Query(ctx, `
		SELECT uuid::text, server_id::text, name, checksum, size, status, upload_id, completed_at, created_at, updated_at, is_locked, status_message, status_callback, retry_count, last_retry_at,
		       COALESCE(source_type, ''), COALESCE(source_id, ''), COALESCE(database_type, ''), COALESCE(volume_name, ''), COALESCE(manifest, '{}'), COALESCE(storage_receipt, '{}'), checksum_verified, restore_count, last_restore_at,
		       compressed, encrypted, COALESCE(nonce, '')
		FROM backups
		WHERE status = $1 AND status_callback IS NOT NULL
		ORDER BY created_at ASC
	`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	backups := []Backup{}
	for rows.Next() {
		var backup Backup
		if err := rows.Scan(&backup.UUID, &backup.ServerID, &backup.Name, &backup.Checksum, &backup.Size, &backup.Status, &backup.UploadID, &backup.CompletedAt, &backup.CreatedAt, &backup.UpdatedAt, &backup.IsLocked, &backup.StatusMessage, &backup.StatusCallback, &backup.RetryCount, &backup.LastRetryAt, &backup.SourceType, &backup.SourceID, &backup.DatabaseType, &backup.VolumeName, &backup.Manifest, &backup.StorageReceipt, &backup.ChecksumVerified, &backup.RestoreCount, &backup.LastRestoreAt, &backup.Compressed, &backup.Encrypted, &backup.Nonce); err != nil {
			return nil, err
		}
		backups = append(backups, backup)
	}
	return backups, rows.Err()
}

// jsonOrNull marshals data to JSON bytes, returning nil (SQL NULL) for empty/nil input
func jsonOrNull(data []byte) any {
	if len(data) == 0 {
		return nil
	}
	return data
}
