package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GetDB exposes the underlying pgx connection pool. Use sparingly — most
// code should go through a dedicated store method. It's intended for the
// plugin system and similar hot paths that need to issue arbitrary
// updates against the plugins table.
func (s *Store) GetDB() *pgxpool.Pool { return s.db }

// Exec proxies a raw SQL statement through the underlying pool. It's a
// thin convenience used by the plugin system.
func (s *Store) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	tag, err := s.db.Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
