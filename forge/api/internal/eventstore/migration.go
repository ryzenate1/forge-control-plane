package eventstore

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const migrationRetries = 3

func Migrate(pool *pgxpool.Pool) error {
	return MigrateWithContext(context.Background(), pool)
}

func MigrateWithContext(ctx context.Context, pool *pgxpool.Pool) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			source TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			correlation_id TEXT NOT NULL DEFAULT '',
			payload TEXT NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			dispatched BOOLEAN NOT NULL DEFAULT false,
			failure_count INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_events_dispatched ON events (dispatched)`,
		`CREATE INDEX IF NOT EXISTS idx_events_created_at ON events (created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_events_type ON events (type)`,
		`CREATE INDEX IF NOT EXISTS idx_events_failure_count ON events (failure_count) WHERE failure_count > 0`,
		`ALTER TABLE events ADD COLUMN IF NOT EXISTS claimed_by TEXT`,
		`ALTER TABLE events ADD COLUMN IF NOT EXISTS claimed_until TIMESTAMPTZ`,
		`CREATE INDEX IF NOT EXISTS idx_events_claimed_until ON events (claimed_until) WHERE dispatched = false`,
		`CREATE TABLE IF NOT EXISTS events_dead_letter (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			source TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			correlation_id TEXT NOT NULL DEFAULT '',
			payload TEXT NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			failed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			failure_count INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_events_dl_created_at ON events_dead_letter (created_at)`,
	}

	for _, stmt := range statements {
		if err := execWithRetry(ctx, pool, stmt); err != nil {
			return fmt.Errorf("eventstore migration: %w", err)
		}
	}
	return nil
}

func execWithRetry(ctx context.Context, pool *pgxpool.Pool, stmt string) error {
	var lastErr error
	for attempt := 0; attempt < migrationRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(100*(1<<attempt)) * time.Millisecond):
			}
		}
		_, lastErr = pool.Exec(ctx, stmt)
		if lastErr == nil {
			return nil
		}
	}
	return lastErr
}
