package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

type mySQLDriver struct {
	db     *sql.DB
	cfg    DBConfig
	dbType DatabaseType
}

func newMySQLDriver(ctx context.Context, cfg DBConfig) (*mySQLDriver, error) {
	dsn := cfg.DSN()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	dbType := DatabaseMySQL
	if cfg.Type == DatabaseMariaDB {
		dbType = DatabaseMariaDB
	}

	return &mySQLDriver{db: db, cfg: cfg, dbType: dbType}, nil
}

func (d *mySQLDriver) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *mySQLDriver) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return d.db.ExecContext(ctx, query, args...)
}

func (d *mySQLDriver) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return d.db.QueryContext(ctx, query, args...)
}

func (d *mySQLDriver) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return d.db.QueryRowContext(ctx, query, args...)
}

func (d *mySQLDriver) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return d.db.BeginTx(ctx, nil)
}

func (d *mySQLDriver) Close() error {
	return d.db.Close()
}

func (d *mySQLDriver) Type() DatabaseType {
	return d.dbType
}

func (d *mySQLDriver) Stats() sql.DBStats {
	return d.db.Stats()
}

func (d *mySQLDriver) DB() *sql.DB {
	return d.db
}
