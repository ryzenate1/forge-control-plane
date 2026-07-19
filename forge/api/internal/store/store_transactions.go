package store

import (
	"context"
	"database/sql"
	"fmt"
)

func WithTransaction(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	return WithTransactionIsolation(ctx, db, sql.LevelDefault, fn)
}

func WithTransactionIsolation(ctx context.Context, db *sql.DB, level sql.IsolationLevel, fn func(tx *sql.Tx) error) error {
	if db == nil {
		return fmt.Errorf("database connection is nil")
	}
	if fn == nil {
		return fmt.Errorf("transaction function is nil")
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: level})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rollback failed: %v (original error: %w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
