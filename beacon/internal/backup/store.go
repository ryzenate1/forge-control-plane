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
	db *sql.DB
}
