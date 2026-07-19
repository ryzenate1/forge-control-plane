package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type BackupPolicy struct {
	ID            string    `json:"id"`
	ServerID      string    `json:"serverId"`
	Interval      string    `json:"interval"`
	MaxBackups    int       `json:"maxBackups"`
	RetentionDays int       `json:"retentionDays"`
	Storage       string    `json:"storage"`
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func (s *Store) CreateBackupPolicy(ctx context.Context, p *BackupPolicy) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO backup_policies (id, server_id, interval, max_backups, retention_days, storage, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now(), now())
	`, p.ID, p.ServerID, p.Interval, p.MaxBackups, p.RetentionDays, p.Storage, p.Enabled)
	return err
}

func (s *Store) GetBackupPolicy(ctx context.Context, id string) (BackupPolicy, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id::text, server_id::text, interval, max_backups, retention_days, storage, enabled, created_at, updated_at
		FROM backup_policies
		WHERE id = $1
	`, id)
	var p BackupPolicy
	err := row.Scan(&p.ID, &p.ServerID, &p.Interval, &p.MaxBackups, &p.RetentionDays, &p.Storage, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (s *Store) ListBackupPolicies(ctx context.Context, serverID string) ([]BackupPolicy, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, interval, max_backups, retention_days, storage, enabled, created_at, updated_at
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
		if err := rows.Scan(&p.ID, &p.ServerID, &p.Interval, &p.MaxBackups, &p.RetentionDays, &p.Storage, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *Store) UpdateBackupPolicy(ctx context.Context, p *BackupPolicy) error {
	_, err := s.db.Exec(ctx, `
		UPDATE backup_policies
		SET interval = $2, max_backups = $3, retention_days = $4, storage = $5, enabled = $6, updated_at = now()
		WHERE id = $1
	`, p.ID, p.Interval, p.MaxBackups, p.RetentionDays, p.Storage, p.Enabled)
	return err
}

func (s *Store) DeleteBackupPolicy(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `
		DELETE FROM backup_policies WHERE id = $1
	`, id)
	return err
}

func (s *Store) ListExpiredBackups(ctx context.Context) ([]Backup, error) {
	rows, err := s.db.Query(ctx, `
		SELECT uuid::text, server_id::text, name, checksum, size, status, upload_id, completed_at, created_at, updated_at, is_locked, status_message, status_callback, retry_count, last_retry_at
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
		if err := rows.Scan(&b.UUID, &b.ServerID, &b.Name, &b.Checksum, &b.Size, &b.Status, &b.UploadID, &b.CompletedAt, &b.CreatedAt, &b.UpdatedAt, &b.IsLocked, &b.StatusMessage, &b.StatusCallback, &b.RetryCount, &b.LastRetryAt); err != nil {
			return nil, err
		}
		backups = append(backups, b)
	}
	return backups, rows.Err()
}
