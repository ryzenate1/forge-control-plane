package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func newTestSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestDefaultPoolConfig(t *testing.T) {
	cfg := DefaultPoolConfig()
	if cfg.MaxOpenConns != 25 {
		t.Errorf("MaxOpenConns = %d, want 25", cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns != 10 {
		t.Errorf("MaxIdleConns = %d, want 10", cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime != 30*time.Minute {
		t.Errorf("ConnMaxLifetime = %v, want 30m", cfg.ConnMaxLifetime)
	}
	if cfg.ConnMaxIdleTime != 5*time.Minute {
		t.Errorf("ConnMaxIdleTime = %v, want 5m", cfg.ConnMaxIdleTime)
	}
}

func TestConfigurePool(t *testing.T) {
	db := newTestSQLite(t)
	cfg := ConnectionPoolConfig{
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
		ConnMaxIdleTime: 30 * time.Second,
	}
	if err := ConfigurePool(db, cfg); err != nil {
		t.Fatalf("ConfigurePool error: %v", err)
	}
	stats := db.Stats()
	if stats.MaxOpenConnections != 5 {
		t.Errorf("MaxOpenConnections = %d, want 5", stats.MaxOpenConnections)
	}
}

func TestConfigurePoolNilDB(t *testing.T) {
	err := ConfigurePool(nil, DefaultPoolConfig())
	if err == nil {
		t.Error("expected error for nil db")
	}
}

func TestGetPoolStats(t *testing.T) {
	db := newTestSQLite(t)
	_ = ConfigurePool(db, DefaultPoolConfig())

	stats := GetPoolStats(db)
	if stats.MaxOpenConnections != 25 {
		t.Errorf("MaxOpenConnections = %d, want 25", stats.MaxOpenConnections)
	}
}

func TestHealthCheck(t *testing.T) {
	db := newTestSQLite(t)
	ctx := context.Background()
	if err := HealthCheck(ctx, db); err != nil {
		t.Fatalf("HealthCheck error: %v", err)
	}
}

func TestHealthCheckNilDB(t *testing.T) {
	err := HealthCheck(context.Background(), nil)
	if err == nil {
		t.Error("expected error for nil db")
	}
}
