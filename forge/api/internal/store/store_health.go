package store

import "context"

func (s *Store) PingDatabase(ctx context.Context) error {
	if s.db == nil {
		return nil
	}
	return s.db.Ping(ctx)
}

type DatabaseHealthDetails struct {
	Version           string `json:"version"`
	ActiveConnections int    `json:"activeConnections"`
	MigrationCount    int    `json:"migrationCount"`
}

func (s *Store) DatabaseHealthDetails(ctx context.Context) (DatabaseHealthDetails, error) {
	var details DatabaseHealthDetails
	if s.db == nil {
		return details, nil
	}
	if err := s.db.QueryRow(ctx, `SELECT version()`).Scan(&details.Version); err != nil {
		return details, err
	}
	_ = s.db.QueryRow(ctx, `SELECT count(*)::int FROM pg_stat_activity WHERE datname = current_database()`).Scan(&details.ActiveConnections)
	_ = s.db.QueryRow(ctx, `SELECT count(*)::int FROM schema_migrations`).Scan(&details.MigrationCount)
	return details, nil
}
