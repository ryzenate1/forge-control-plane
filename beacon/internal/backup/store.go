package backup

import (
	"context"
	"database/sql"
)

type Store interface {
	Create(ctx context.Context, b Backup) error
	Get(ctx context.Context, id string) (Backup, error)
	List(ctx context.Context, serverID string, limit int) ([]Backup, error)
	UpdateStatus(ctx context.Context, id string, status BackupStatus, errorMsg string) error
	Delete(ctx context.Context, id string) error
}

type SQLiteStore struct {
	DB *sql.DB
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{DB: db}
}

func (s *SQLiteStore) Create(ctx context.Context, b Backup) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO backups (id, server_id, started_at, completed_at, status, size_bytes, files, duration, adapter, path, error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ID, b.ServerID, b.StartedAt, b.CompletedAt, b.Status, b.SizeBytes, b.Files, b.Duration, b.Adapter, b.Path, b.Error)
	return err
}

func (s *SQLiteStore) Get(ctx context.Context, id string) (Backup, error) {
	var b Backup
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, server_id, started_at, completed_at, status, size_bytes, files, duration, adapter, path, error
		 FROM backups WHERE id = ?`, id).
		Scan(&b.ID, &b.ServerID, &b.StartedAt, &b.CompletedAt, &b.Status, &b.SizeBytes, &b.Files, &b.Duration, &b.Adapter, &b.Path, &b.Error)
	return b, err
}

func (s *SQLiteStore) List(ctx context.Context, serverID string, limit int) ([]Backup, error) {
	query := `SELECT id, server_id, started_at, completed_at, status, size_bytes, files, duration, adapter, path, error
		 FROM backups WHERE server_id = ? ORDER BY completed_at DESC`
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = s.DB.QueryContext(ctx, query+" LIMIT ?", serverID, limit)
	} else {
		rows, err = s.DB.QueryContext(ctx, query, serverID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backups []Backup
	for rows.Next() {
		var b Backup
		if err := rows.Scan(&b.ID, &b.ServerID, &b.StartedAt, &b.CompletedAt, &b.Status, &b.SizeBytes, &b.Files, &b.Duration, &b.Adapter, &b.Path, &b.Error); err != nil {
			return nil, err
		}
		backups = append(backups, b)
	}
	return backups, rows.Err()
}

func (s *SQLiteStore) UpdateStatus(ctx context.Context, id string, status BackupStatus, errorMsg string) error {
	_, err := s.DB.ExecContext(ctx,
		`UPDATE backups SET status = ?, error = ? WHERE id = ?`, status, errorMsg, id)
	return err
}

func (s *SQLiteStore) Delete(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM backups WHERE id = ?`, id)
	return err
}
