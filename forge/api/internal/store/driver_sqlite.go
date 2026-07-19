package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

type sqliteDriver struct {
	db  *sql.DB
	cfg DBConfig
}

func newSQLiteDriver(ctx context.Context, cfg DBConfig) (*sqliteDriver, error) {
	dsn := cfg.DSN()
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	db.ExecContext(ctx, "PRAGMA journal_mode=WAL")
	db.ExecContext(ctx, "PRAGMA foreign_keys=ON")

	return &sqliteDriver{db: db, cfg: cfg}, nil
}

func (d *sqliteDriver) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *sqliteDriver) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return d.db.ExecContext(ctx, query, args...)
}

func (d *sqliteDriver) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return d.db.QueryContext(ctx, query, args...)
}

func (d *sqliteDriver) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return d.db.QueryRowContext(ctx, query, args...)
}

func (d *sqliteDriver) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return d.db.BeginTx(ctx, nil)
}

func (d *sqliteDriver) Close() error {
	return d.db.Close()
}

func (d *sqliteDriver) Type() DatabaseType {
	return DatabaseSQLite
}

func (d *sqliteDriver) Stats() sql.DBStats {
	return d.db.Stats()
}

func (d *sqliteDriver) DB() *sql.DB {
	return d.db
}
