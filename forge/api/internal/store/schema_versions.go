package store

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type SchemaVersion struct {
	Version   int       `json:"version"`
	Name      string    `json:"name"`
	AppliedAt time.Time `json:"appliedAt"`
	Checksum  string    `json:"checksum"`
}

type SchemaMigrationTx interface {
	Exec(ctx context.Context, sql string, args ...any) error
}

type SchemaMigration interface {
	Version() int
	Name() string
	Up(ctx context.Context, tx SchemaMigrationTx) error
	Down(ctx context.Context, tx SchemaMigrationTx) error
}

type schemaMigrationEntry struct {
	version int
	name    string
	up      func(ctx context.Context, tx SchemaMigrationTx) error
	down    func(ctx context.Context, tx SchemaMigrationTx) error
}

func (e schemaMigrationEntry) Version() int { return e.version }
func (e schemaMigrationEntry) Name() string  { return e.name }
func (e schemaMigrationEntry) Up(ctx context.Context, tx SchemaMigrationTx) error {
	return e.up(ctx, tx)
}
func (e schemaMigrationEntry) Down(ctx context.Context, tx SchemaMigrationTx) error {
	return e.down(ctx, tx)
}

type SchemaMigrator struct {
	store      *Store
	migrations []schemaMigrationEntry
}

func NewSchemaMigrator(store *Store) *SchemaMigrator {
	return &SchemaMigrator{
		store:      store,
		migrations: make([]schemaMigrationEntry, 0),
	}
}

func (m *SchemaMigrator) Register(version int, name string, up, down func(ctx context.Context, tx SchemaMigrationTx) error) {
	m.migrations = append(m.migrations, schemaMigrationEntry{
		version: version,
		name:    name,
		up:      up,
		down:    down,
	})
}

func (m *SchemaMigrator) ensureTable(ctx context.Context) error {
	_, err := m.store.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_versions (
			version    INTEGER PRIMARY KEY,
			name       TEXT NOT NULL,
			checksum   TEXT NOT NULL DEFAULT '',
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	return err
}

func (m *SchemaMigrator) appliedVersions(ctx context.Context) (map[int]SchemaVersion, error) {
	rows, err := m.store.db.Query(ctx, `SELECT version, name, checksum, applied_at FROM schema_versions ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	versions := make(map[int]SchemaVersion)
	for rows.Next() {
		var v SchemaVersion
		if err := rows.Scan(&v.Version, &v.Name, &v.Checksum, &v.AppliedAt); err != nil {
			return nil, err
		}
		versions[v.Version] = v
	}
	return versions, rows.Err()
}

func (m *SchemaMigrator) Run(ctx context.Context) error {
	if err := m.ensureTable(ctx); err != nil {
		return fmt.Errorf("ensure schema_versions: %w", err)
	}

	sort.Slice(m.migrations, func(i, j int) bool {
		return m.migrations[i].version < m.migrations[j].version
	})

	applied, err := m.appliedVersions(ctx)
	if err != nil {
		return fmt.Errorf("list applied versions: %w", err)
	}

	for _, mig := range m.migrations {
		if _, exists := applied[mig.version]; exists {
			continue
		}

		tx, err := m.store.db.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration v%d: %w", mig.version, err)
		}

		execWrapper := &pgxTxAdapter{tx: tx}
		if err := mig.up(ctx, execWrapper); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("run migration v%d (%s): %w", mig.version, mig.name, err)
		}

		checksum := fmt.Sprintf("v%d-%s-%s", mig.version, mig.name, uuid.NewString()[:8])
		if _, err := tx.Exec(ctx, `INSERT INTO schema_versions (version, name, checksum) VALUES ($1, $2, $3)`,
			mig.version, mig.name, checksum); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration v%d: %w", mig.version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration v%d: %w", mig.version, err)
		}
	}

	return nil
}

func (m *SchemaMigrator) Rollback(ctx context.Context, targetVersion int) error {
	if err := m.ensureTable(ctx); err != nil {
		return fmt.Errorf("ensure schema_versions: %w", err)
	}

	applied, err := m.appliedVersions(ctx)
	if err != nil {
		return fmt.Errorf("list applied versions: %w", err)
	}

	migrationMap := make(map[int]schemaMigrationEntry)
	for _, mig := range m.migrations {
		migrationMap[mig.version] = mig
	}

	versionsToRollback := make([]int, 0)
	for v := range applied {
		if v > targetVersion {
			versionsToRollback = append(versionsToRollback, v)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(versionsToRollback)))

	if len(versionsToRollback) == 0 {
		return nil
	}

	for _, v := range versionsToRollback {
		mig, exists := migrationMap[v]
		if !exists {
			return fmt.Errorf("migration v%d not found in registry", v)
		}

		tx, err := m.store.db.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin rollback v%d: %w", v, err)
		}

		execWrapper := &pgxTxAdapter{tx: tx}
		if err := mig.down(ctx, execWrapper); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("rollback v%d (%s): %w", v, mig.name, err)
		}

		if _, err := tx.Exec(ctx, `DELETE FROM schema_versions WHERE version = $1`, v); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("remove schema_versions v%d: %w", v, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit rollback v%d: %w", v, err)
		}
	}

	return nil
}

func (m *SchemaMigrator) AppliedVersions(ctx context.Context) ([]SchemaVersion, error) {
	if err := m.ensureTable(ctx); err != nil {
		return nil, err
	}
	applied, err := m.appliedVersions(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]SchemaVersion, 0, len(applied))
	for _, v := range applied {
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Version < result[j].Version
	})
	return result, nil
}

type pgxTxAdapter struct {
	tx pgx.Tx
}

func (a *pgxTxAdapter) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := a.tx.Exec(ctx, sql, args...)
	return err
}
