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

	sqlFiles := make([]string, 0, len(migrationPaths))
	for file := range migrationPaths {
		sqlFiles = append(sqlFiles, file)
	}
	sort.Strings(sqlFiles)

	if err := validateNoDuplicatePrefixes(sqlFiles); err != nil {
		return err
	}

	runMigrationIDs := mr.getRunMigrationIDs(ctx)

	for _, file := range sqlFiles {
		id := strings.TrimSuffix(file, ".sql")
		if _, exists := runMigrationIDs[id]; exists {
			continue
		}

		data, err := os.ReadFile(migrationPaths[file])
		if err != nil {
			return fmt.Errorf("read migration %s: %w", file, err)
		}

		sql := string(data)
		if mr.driver.Type() == DatabaseSQLite {
			sql = sqliteCompatibleMigration(sql)
		}
		statements := splitSQLStatements(sql)
		for _, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if mr.driver.Type() == DatabaseSQLite {
				if strings.Contains(strings.ToUpper(stmt), "DROP CONSTRAINT") || strings.Contains(strings.ToUpper(stmt), "ADD CONSTRAINT") {
					continue
				}
				if strings.Contains(strings.ToUpper(stmt), "ALTER COLUMN") {
					continue
				}
				if strings.HasPrefix(strings.TrimSpace(strings.ToUpper(stmt)), "DO $$") {
					continue
				}
				if strings.Contains(strings.ToLower(stmt), "regexp_replace") || strings.Contains(strings.ToLower(stmt), "substring(") {
					continue
				}
				for _, expanded := range splitSQLiteAlterAdd(stmt) {
					if _, err := mr.driver.Exec(ctx, strings.TrimSpace(expanded)); err != nil {
						if strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
							continue
						}
						return fmt.Errorf("run migration %s: %w (stmt: %s)", file, err, strings.TrimSpace(expanded))
					}
				}
				continue
			}
			if _, err := mr.driver.Exec(ctx, stmt); err != nil {
				return fmt.Errorf("run migration %s: %w (stmt: %s)", file, err, stmt)
			}
		}

		recordSQL := getRecordMigrationSQL(mr.driver.Type())
		if _, err := mr.driver.Exec(ctx, recordSQL, id); err != nil {
			return fmt.Errorf("record migration %s: %w", file, err)
		}
	}

	return nil
}

func splitSQLiteAlterAdd(stmt string) []string {
	upper := strings.ToUpper(stmt)
	if !strings.HasPrefix(upper, "ALTER TABLE ") || !strings.Contains(upper, "ADD COLUMN") {
		return []string{stmt}
	}
	parts := strings.SplitN(stmt, "\n", 2)
	if len(parts) != 2 {
		return []string{strings.Replace(stmt, "ADD COLUMN IF NOT EXISTS", "ADD COLUMN", 1)}
	}
	table := strings.TrimSpace(strings.TrimPrefix(parts[0], "ALTER TABLE"))
	columns := strings.Split(parts[1], ",\n")
	result := make([]string, 0, len(columns))
	for _, column := range columns {
		column = strings.Replace(column, "ADD COLUMN IF NOT EXISTS", "ADD COLUMN", 1)
		result = append(result, "ALTER TABLE "+table+"\n"+strings.TrimSpace(column))
	}
	return result
}

// sqliteCompatibleMigration adapts the small PostgreSQL-specific subset used by
// the canonical migrations. SQLite is supported for local development and tests;
// production PostgreSQL migrations are deliberately left byte-for-byte intact.
func sqliteCompatibleMigration(sql string) string {
	replacer := strings.NewReplacer(
		"TIMESTAMPTZ", "TIMESTAMP", "timestamptz", "timestamp",
		"::jsonb", "", "JSONB", "TEXT", "jsonb", "text",
		"TEXT[]", "TEXT", "text[]", "text",
		"UUID", "TEXT", "uuid", "text",
		"INET", "TEXT", "inet", "text",
		"now()", "CURRENT_TIMESTAMP", "NOW()", "CURRENT_TIMESTAMP",
		"gen_random_uuid()", "''",
		"split_part(email, '@', 1)", "substr(email, 1, instr(email, '@') - 1)",
		"CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$')", "CHECK (length(slug) > 0)",
		"::text", "", "::json", "",
		"::textb", "",
	)
	sql = replacer.Replace(sql)
	sql = strings.ReplaceAll(sql, "ALTER TABLE allocations\n    ALTER COLUMN ip TYPE text USING ip;", "")
	sql = strings.ReplaceAll(sql, "ALTER TABLE allocations\n    ALTER COLUMN ip TYPE text USING ip", "")
	sql = strings.ReplaceAll(sql, "ALTER TABLE allocations\n    DROP CONSTRAINT IF EXISTS allocations_port_range_check;", "")
	sql = strings.ReplaceAll(sql, "ALTER TABLE allocations\n    ADD CONSTRAINT allocations_port_range_check CHECK (port BETWEEN 1 AND 65535);", "")
	// SQLite permits only one ADD COLUMN per ALTER TABLE statement.
	// The canonical stream uses PostgreSQL's multi-column form for server
	// transfer fields; split that form while retaining the same schema.
	for _, table := range []string{"users", "servers", "allocations", "nodes", "backups", "deployments"} {
		marker := "ALTER TABLE " + table + "\n    ADD COLUMN"
		if strings.Contains(sql, marker) {
			// Restrict the split to the statement beginning at this table. The
			// canonical migrations use one table per multi-add statement.
			start := strings.Index(sql, marker)
			if end := strings.Index(sql[start:], ";"); end >= 0 {
				segment := sql[start : start+end]
				segment = strings.ReplaceAll(segment, ",\n    ADD COLUMN", ";\nALTER TABLE "+table+"\n    ADD COLUMN")
				sql = sql[:start] + segment + sql[start+end:]
			}
		}
	}
	// PostgreSQL casts can follow a quoted JSON default. Remove any remaining
	// casts so SQLite treats the default as ordinary text.
	for strings.Contains(sql, "::") {
		start := strings.Index(sql, "::")
		end := start + 2
		for end < len(sql) && ((sql[end] >= 'a' && sql[end] <= 'z') || (sql[end] >= 'A' && sql[end] <= 'Z') || sql[end] == '_') {
			end++
		}
		sql = sql[:start] + sql[end:]
	}
	return sql
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

// migrationPrefix extracts the numeric prefix (e.g., "015", "015_a", "111") from
// a migration filename. A "prefix" is defined as everything before the second underscore
// or the entire segment before the first underscore if there is no second underscore.
// This means "015_a_mounts.sql" has prefix "015_a" and "120_db_hosts_constraints.sql"
// has prefix "120", making them distinct after the rename strategy.
func migrationPrefix(name string) string {
	name = strings.TrimSuffix(name, ".sql")
	parts := strings.SplitN(name, "_", 3)
	if len(parts) >= 3 && len(parts[1]) == 1 && parts[1][0] >= 'a' && parts[1][0] <= 'z' {
		// Letter-suffixed: "015_a_mounts" -> prefix "015_a"
		return parts[0] + "_" + parts[1]
	}
	// Normal: "120_db_hosts_constraints" -> prefix "120"
	return parts[0]
}

func validateNoDuplicatePrefixes(files []string) error {
	seen := make(map[string]string)
	for _, f := range files {
		prefix := migrationPrefix(f)
		if existing, ok := seen[prefix]; ok {
			return fmt.Errorf("duplicate migration prefix %q: %q and %q conflict; rename one file with a letter suffix (e.g., %s_a_*) or bump to a new number",
				prefix, existing, f, prefix)
		}
		seen[prefix] = f
	}
	return nil
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
