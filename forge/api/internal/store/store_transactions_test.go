package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestWithTransactionCommit(t *testing.T) {
	db := newTestSQLite(t)
	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `CREATE TABLE test_tx (id INTEGER PRIMARY KEY, val TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	err := WithTransaction(ctx, db, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO test_tx (id, val) VALUES (1, 'hello')`)
		return err
	})
	if err != nil {
		t.Fatalf("WithTransaction error: %v", err)
	}

	var val string
	if err := db.QueryRowContext(ctx, `SELECT val FROM test_tx WHERE id = 1`).Scan(&val); err != nil {
		t.Fatalf("query after commit: %v", err)
	}
	if val != "hello" {
		t.Errorf("val = %q, want hello", val)
	}
}

func TestWithTransactionRollback(t *testing.T) {
	db := newTestSQLite(t)
	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `CREATE TABLE test_tx (id INTEGER PRIMARY KEY, val TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	expectedErr := errors.New("intentional failure")
	err := WithTransaction(ctx, db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO test_tx (id, val) VALUES (1, 'hello')`); err != nil {
			return err
		}
		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM test_tx`).Scan(&count); err != nil {
		t.Fatalf("query after rollback: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 (rollback failed)", count)
	}
}

func TestWithTransactionIsolation(t *testing.T) {
	db := newTestSQLite(t)
	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `CREATE TABLE test_tx (id INTEGER PRIMARY KEY, val TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	err := WithTransactionIsolation(ctx, db, sql.LevelSerializable, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO test_tx (id, val) VALUES (1, 'isolated')`)
		return err
	})
	if err != nil {
		t.Fatalf("WithTransactionIsolation error: %v", err)
	}

	var val string
	if err := db.QueryRowContext(ctx, `SELECT val FROM test_tx WHERE id = 1`).Scan(&val); err != nil {
		t.Fatalf("query: %v", err)
	}
	if val != "isolated" {
		t.Errorf("val = %q, want isolated", val)
	}
}

func TestWithTransactionNilDB(t *testing.T) {
	err := WithTransaction(context.Background(), nil, func(tx *sql.Tx) error {
		return nil
	})
	if err == nil {
		t.Error("expected error for nil db")
	}
}

func TestWithTransactionNilFunc(t *testing.T) {
	db := newTestSQLite(t)
	err := WithTransaction(context.Background(), db, nil)
	if err == nil {
		t.Error("expected error for nil func")
	}
}
