package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Database interface {
	Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row
	Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	BeginTx(ctx context.Context) (*sql.Tx, error)
	Close() error
}

type SQLiteDatabase struct {
	db *sql.DB
}

func NewSQLiteDatabase(dsn string) (*SQLiteDatabase, error) {
	// SQLite permits only one writer. WAL plus a busy timeout avoids immediate
	// "database is locked" failures, while a single pooled connection keeps
	// transaction semantics predictable for the daemon's embedded workload.
	if !strings.Contains(dsn, "?") {
		dsn += "?"
	} else {
		dsn += "&"
	}
	dsn += "_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on&_synchronous=FULL&_txlock=immediate"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	return &SQLiteDatabase{db: db}, nil
}

func (s *SQLiteDatabase) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, query, args...)
}

func (s *SQLiteDatabase) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return s.db.QueryRowContext(ctx, query, args...)
}

func (s *SQLiteDatabase) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return s.db.ExecContext(ctx, query, args...)
}

func (s *SQLiteDatabase) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return s.db.BeginTx(ctx, &sql.TxOptions{})
}

func (s *SQLiteDatabase) Close() error {
	return s.db.Close()
}
