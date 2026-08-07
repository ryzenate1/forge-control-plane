package store

import (
	"context"
)

func (s *Store) ListAllBackups(ctx context.Context, limit ...int) ([]Backup, error) {
	maxRows := 100
	if len(limit) > 0 && limit[0] > 0 {
		maxRows = limit[0]
		if maxRows > 1000 {
			maxRows = 1000
		}
	}
	rows, err := s.db.Query(ctx, `
		SELECT uuid::text, server_id::text, name, checksum, size, status, upload_id, completed_at, created_at, updated_at, is_locked, status_message, status_callback, retry_count, last_retry_at,
		       COALESCE(source_type, ''), COALESCE(source_id, ''), COALESCE(database_type, ''), COALESCE(volume_name, ''), COALESCE(manifest, '{}'), COALESCE(storage_receipt, '{}'), checksum_verified, restore_count, last_restore_at,
		       compressed, encrypted, COALESCE(nonce, '')
		FROM backups
		ORDER BY created_at DESC
		LIMIT $1
	`, maxRows)
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

func (s *Store) GetBackupByUUIDFull(ctx context.Context, uuid string) (Backup, error) {
	var backup Backup
	err := s.db.QueryRow(ctx, `
		SELECT uuid::text, server_id::text, name, checksum, size, status, upload_id, completed_at, created_at, updated_at, is_locked, status_message, status_callback, retry_count, last_retry_at,
		       COALESCE(source_type, ''), COALESCE(source_id, ''), COALESCE(database_type, ''), COALESCE(volume_name, ''), COALESCE(manifest, '{}'), COALESCE(storage_receipt, '{}'), checksum_verified, restore_count, last_restore_at,
		       compressed, encrypted, COALESCE(nonce, '')
		FROM backups
		WHERE uuid = $1
	`, uuid).Scan(&backup.UUID, &backup.ServerID, &backup.Name, &backup.Checksum, &backup.Size, &backup.Status, &backup.UploadID, &backup.CompletedAt, &backup.CreatedAt, &backup.UpdatedAt, &backup.IsLocked, &backup.StatusMessage, &backup.StatusCallback, &backup.RetryCount, &backup.LastRetryAt, &backup.SourceType, &backup.SourceID, &backup.DatabaseType, &backup.VolumeName, &backup.Manifest, &backup.StorageReceipt, &backup.ChecksumVerified, &backup.RestoreCount, &backup.LastRestoreAt, &backup.Compressed, &backup.Encrypted, &backup.Nonce)
	return backup, err
}

func (s *Store) DeleteBackupByUUID(ctx context.Context, uuid string, actorID *string) error {
	var serverID, name string
	err := s.db.QueryRow(ctx, `SELECT server_id::text, name FROM backups WHERE uuid = $1`, uuid).Scan(&serverID, &name)
	if err != nil {
		return err
	}
	return s.DeleteBackup(ctx, serverID, name, actorID)
}

func (s *Store) LockBackupByUUID(ctx context.Context, uuid string, actorID *string) error {
	var serverID, name string
	err := s.db.QueryRow(ctx, `SELECT server_id::text, name FROM backups WHERE uuid = $1`, uuid).Scan(&serverID, &name)
	if err != nil {
		return err
	}
	return s.LockBackup(ctx, serverID, name, actorID)
}

func (s *Store) UnlockBackupByUUID(ctx context.Context, uuid string, actorID *string) error {
	var serverID, name string
	err := s.db.QueryRow(ctx, `SELECT server_id::text, name FROM backups WHERE uuid = $1`, uuid).Scan(&serverID, &name)
	if err != nil {
		return err
	}
	return s.UnlockBackup(ctx, serverID, name, actorID)
}

func (s *Store) GetBackupDownloadURL(ctx context.Context, uuid string) (string, error) {
	var b Backup
	b, err := s.GetBackupByUUIDFull(ctx, uuid)
	if err != nil {
		return "", err
	}
	if b.UploadID != nil && *b.UploadID != "" {
		return *b.UploadID, nil
	}
	return "", nil
}
