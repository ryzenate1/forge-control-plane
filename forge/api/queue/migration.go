package queue

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Migrator struct {
	pool   *pgxpool.Pool
	schema string
	fsys   fs.FS
	line   string
}

func NewMigrator(pool *pgxpool.Pool, schema string, fsys fs.FS) *Migrator {
	return &Migrator{
		pool:   pool,
		schema: schema,
		fsys:   fsys,
		line:   "main",
	}
}

type Migration struct {
	Version   int
	CreatedAt time.Time
}

func (m *Migrator) Up(ctx context.Context) error {
	if err := m.ensureMigrationTable(ctx); err != nil {
		return fmt.Errorf("ensure migration table: %w", err)
	}

	applied, err := m.getAppliedVersions(ctx)
	if err != nil {
		return fmt.Errorf("get applied versions: %w", err)
	}

	migrations, err := m.readMigrationFiles()
	if err != nil {
		return fmt.Errorf("read migration files: %w", err)
	}

	appliedSet := make(map[int]bool)
	for _, v := range applied {
		appliedSet[v] = true
	}

	var pending []migrationFile
	for _, mf := range migrations {
		if !appliedSet[mf.version] {
			pending = append(pending, mf)
		}
	}

	sort.Slice(pending, func(i, j int) bool {
		return pending[i].version < pending[j].version
	})

	for _, mf := range pending {
		sql, err := fs.ReadFile(m.fsys, mf.path)
		if err != nil {
			return fmt.Errorf("read migration %d: %w", mf.version, err)
		}

		execSQL := replaceSchemaTemplate(string(sql), m.schema)

		_, err = m.pool.Exec(ctx, execSQL)
		if err != nil {
			return fmt.Errorf("execute migration %d: %w", mf.version, err)
		}

		err = m.recordMigration(ctx, mf.version)
		if err != nil {
			return fmt.Errorf("record migration %d: %w", mf.version, err)
		}
	}

	return nil
}

func (m *Migrator) Down(ctx context.Context, steps int) error {
	if err := m.ensureMigrationTable(ctx); err != nil {
		return fmt.Errorf("ensure migration table: %w", err)
	}

	applied, err := m.getAppliedVersions(ctx)
	if err != nil {
		return fmt.Errorf("get applied versions: %w", err)
	}

	if len(applied) == 0 {
		return nil
	}

	sort.Slice(applied, func(i, j int) bool {
		return applied[i] > applied[j]
	})

	if steps > 0 && steps < len(applied) {
		applied = applied[:steps]
	}

	migrations, err := m.readDownMigrationFiles()
	if err != nil {
		return fmt.Errorf("read down migration files: %w", err)
	}

	downByVersion := make(map[int]string)
	for _, mf := range migrations {
		downByVersion[mf.version] = mf.path
	}

	for _, version := range applied {
		downPath, ok := downByVersion[version]
		if !ok {
			return fmt.Errorf("no down migration found for version %d", version)
		}

		sql, err := fs.ReadFile(m.fsys, downPath)
		if err != nil {
			return fmt.Errorf("read down migration %d: %w", version, err)
		}

		execSQL := replaceSchemaTemplate(string(sql), m.schema)

		_, err = m.pool.Exec(ctx, execSQL)
		if err != nil {
			return fmt.Errorf("execute down migration %d: %w", version, err)
		}

		err = m.removeMigration(ctx, version)
		if err != nil {
			return fmt.Errorf("remove migration %d: %w", version, err)
		}
	}

	return nil
}

func (m *Migrator) ensureMigrationTable(ctx context.Context) error {
	sql := `CREATE TABLE IF NOT EXISTS ` + schemaTable(m.schema) + `river_migration(
    id bigserial PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    version bigint NOT NULL,
    CONSTRAINT version CHECK (version >= 1)
)`

	_, err := m.pool.Exec(ctx, sql)
	if err != nil {
		_, err2 := m.pool.Exec(ctx, sql)
		if err2 != nil {
			return fmt.Errorf("create migration table: %w", err2)
		}
	}

	indexSQL := `CREATE UNIQUE INDEX IF NOT EXISTS river_migration_version_idx ON ` + schemaTable(m.schema) + `river_migration USING btree(version)`
	_, _ = m.pool.Exec(ctx, indexSQL)

	return nil
}

func (m *Migrator) getAppliedVersions(ctx context.Context) ([]int, error) {
	sql := `SELECT version FROM ` + schemaTable(m.schema) + `river_migration ORDER BY version`

	rows, err := m.pool.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []int
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		versions = append(versions, int(v))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return versions, nil
}

func (m *Migrator) recordMigration(ctx context.Context, version int) error {
	sql := `INSERT INTO ` + schemaTable(m.schema) + `river_migration (version) VALUES ($1)`
	_, err := m.pool.Exec(ctx, sql, int64(version))
	return err
}

func (m *Migrator) removeMigration(ctx context.Context, version int) error {
	sql := `DELETE FROM ` + schemaTable(m.schema) + `river_migration WHERE version = $1`
	_, err := m.pool.Exec(ctx, sql, int64(version))
	return err
}

type migrationFile struct {
	version int
	path    string
	typ     string
}

func (m *Migrator) readMigrationFiles() ([]migrationFile, error) {
	entries, err := fs.ReadDir(m.fsys, path.Join("migration", m.line))
	if err != nil {
		return nil, err
	}

	var files []migrationFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}

		parts := strings.SplitN(name, "_", 2)
		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		files = append(files, migrationFile{
			version: version,
			path:    path.Join("migration", m.line, name),
			typ:     "up",
		})
	}
	return files, nil
}

func (m *Migrator) readDownMigrationFiles() ([]migrationFile, error) {
	entries, err := fs.ReadDir(m.fsys, path.Join("migration", m.line))
	if err != nil {
		return nil, err
	}

	var files []migrationFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".down.sql") {
			continue
		}

		parts := strings.SplitN(name, "_", 2)
		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		files = append(files, migrationFile{
			version: version,
			path:    path.Join("migration", m.line, name),
			typ:     "down",
		})
	}
	return files, nil
}

func schemaTable(schema string) string {
	if schema == "" {
		return ""
	}
	return `"` + strings.ReplaceAll(schema, `"`, `""`) + `".`
}

func replaceSchemaTemplate(sql, schema string) string {
	prefix := schemaTable(schema)
	return strings.ReplaceAll(sql, "/* TEMPLATE: schema */", prefix)
}
