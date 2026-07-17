package database

import (
	"context"
	"database/sql"
	"time"
)

type Database interface {
	Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	BeginTx(ctx context.Context) (*sql.Tx, error)
	Close() error
}

type SQLiteDatabase struct {
	db *sql.DB
}

func NewSQLiteDatabase(dsn string) (*SQLiteDatabase, error) {
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	// Set connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	return &SQLiteDatabase{db: db}, nil
}

func (s *SQLiteDatabase) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, query, args...)
}

func (s *SQLiteDatabase) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return s.db.ExecContext(ctx, query, args...)
}

func (s *SQLiteDatabase) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return s.db.BeginTx(ctx, nil)
}

func (s *SQLiteDatabase) Close() error {
	return s.db.Close()
}
