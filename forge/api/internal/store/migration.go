package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type MigrationRunner struct {
	driver DatabaseDriver
	dir    string
}

func NewMigrationRunner(driver DatabaseDriver, dir string) *MigrationRunner {
	return &MigrationRunner{
		driver: driver,
		dir:    dir,
	}
}

func (mr *MigrationRunner) Run(ctx context.Context) error {
	createTable := getCreateMigrationTableSQL(mr.driver.Type())
	if _, err := mr.driver.Exec(ctx, createTable); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	dialectDir := mr.dir
	switch mr.driver.Type() {
	case DatabaseMySQL, DatabaseMariaDB:
		dialectDir = filepath.Join(mr.dir, "mysql")
	case DatabaseSQLite:
		dialectDir = filepath.Join(mr.dir, "sqlite")
	}

	baseEntries, err := os.ReadDir(mr.dir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	// Dialect-specific migrations override their base counterpart. Missing dialect
	// files deliberately fall back to the base set, so a partial dialect directory
	// cannot silently skip schema migrations.
	migrationPaths := make(map[string]string)
	for _, entry := range baseEntries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			migrationPaths[entry.Name()] = filepath.Join(mr.dir, entry.Name())
		}
	}
	if dialectDir != mr.dir {
		if entries, err := os.ReadDir(dialectDir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
					migrationPaths[entry.Name()] = filepath.Join(dialectDir, entry.Name())
				}
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("read dialect migrations dir: %w", err)
		}
	}

	runMigrationIDs := mr.getRunMigrationIDs(ctx)

	sqlFiles := make([]string, 0, len(migrationPaths))
	for file := range migrationPaths {
		sqlFiles = append(sqlFiles, file)
	}
	sort.Strings(sqlFiles)

	for _, file := range sqlFiles {
		id := strings.TrimSuffix(file, ".sql")
		if _, exists := runMigrationIDs[id]; exists {
			continue
		}

		data, err := os.ReadFile(migrationPaths[file])
		if err != nil {
			return fmt.Errorf("read migration %s: %w", file, err)
		}

		statements := splitSQLStatements(string(data))
		for _, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := mr.driver.Exec(ctx, stmt); err != nil {
				return fmt.Errorf("run migration %s: %w (stmt: %.100s)", file, err, stmt)
			}
		}

		recordSQL := getRecordMigrationSQL(mr.driver.Type())
		if mr.driver.Type() == DatabaseMySQL || mr.driver.Type() == DatabaseMariaDB {
			if _, err := mr.driver.Exec(ctx, recordSQL, id); err != nil {
				return fmt.Errorf("record migration %s: %w", file, err)
			}
		} else {
			if _, err := mr.driver.Exec(ctx, recordSQL, id); err != nil {
				return fmt.Errorf("record migration %s: %w", file, err)
			}
		}
	}

	return nil
}

func (mr *MigrationRunner) getRunMigrationIDs(ctx context.Context) map[string]struct{} {
	query := getListMigrationsSQL(mr.driver.Type())
	rows, err := mr.driver.Query(ctx, query)
	if err != nil {
		return map[string]struct{}{}
	}
	defer rows.Close()

	ids := make(map[string]struct{})
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids[id] = struct{}{}
		}
	}
	return ids
}

func getCreateMigrationTableSQL(dbType DatabaseType) string {
	switch dbType {
	case DatabaseMySQL, DatabaseMariaDB:
		return `CREATE TABLE IF NOT EXISTS schema_migrations (
			id VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`
	case DatabaseSQLite:
		return `CREATE TABLE IF NOT EXISTS schema_migrations (
			id TEXT PRIMARY KEY,
			applied_at TEXT DEFAULT (datetime('now'))
		)`
	default:
		return `CREATE TABLE IF NOT EXISTS schema_migrations (
			id TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ DEFAULT now()
		)`
	}
}

func getRecordMigrationSQL(dbType DatabaseType) string {
	switch dbType {
	case DatabaseMySQL, DatabaseMariaDB:
		return `INSERT INTO schema_migrations (id) VALUES (?)`
	default:
		return `INSERT INTO schema_migrations (id) VALUES ($1)`
	}
}

func getListMigrationsSQL(dbType DatabaseType) string {
	return `SELECT id FROM schema_migrations ORDER BY id`
}
