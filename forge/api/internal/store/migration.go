package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// MigrationRunner applies the file-based migration stream in dir through a
// DatabaseDriver. It shares the schema_migrations contract with the production
// runner in store.go (runMigrations): a single table keyed by a "version"
// column holding the full migration filename. Both runners can safely share
// the same table and history.
//
// CRITICAL RULE: never rename an already-applied migration file. The filename
// is the primary key in schema_migrations, so a rename re-applies as a brand
// new migration. Historical renames are handled through migrationAliases
// (alias-aware skip + backfill) instead of renames.
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

// sqliteRunnerMu serializes MigrationRunner runs against SQLite, which has no
// server-side advisory lock. The sqlite driver is already limited to a single
// pooled connection; this mutex additionally serializes separate driver
// instances pointing at the same file.
var sqliteRunnerMu sync.Mutex

// acquireRunnerLock serializes schema migrations across horizontally scaled
// API instances so two processes can never apply migrations concurrently.
// It is the portable counterpart of Store.acquireMigrationLock:
//   - postgres: session-level pg_advisory_lock (auto-released if the session dies)
//   - mysql/mariadb: GET_LOCK / RELEASE_LOCK (auto-released if the session dies)
//   - sqlite: in-process mutex (single-writer embedded database)
//
// Both session-scoped locks MUST be acquired and released on the same
// connection. A pooled Exec/Query may land on different sessions on every
// call, which would make the lock silently ineffective (lock taken on session
// A, DDL running on session B, unlock releasing nothing). The lock is
// therefore pinned to one dedicated *sql.Conn for the whole migration run —
// the same pattern as Store.acquireMigrationLock, which Acquires a pool
// connection instead of using the pool directly.
func (mr *MigrationRunner) acquireRunnerLock(ctx context.Context) (func(), error) {
	switch mr.driver.Type() {
	case DatabaseMySQL, DatabaseMariaDB:
		conn, err := mr.driver.DB().Conn(ctx)
		if err != nil {
			return nil, fmt.Errorf("acquire mysql migration lock connection: %w", err)
		}
		// GET_LOCK returns 1 on success, 0 on timeout, NULL on error.
		var result sql.NullInt64
		if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK('forge_migrations', 30)`).Scan(&result); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("acquire mysql migration lock: %w", err)
		}
		if !result.Valid || result.Int64 != 1 {
			_ = conn.Close()
			return nil, fmt.Errorf("acquire mysql migration lock: GET_LOCK did not grant the lock")
		}
		release := func() {
			// Best-effort unlock on the SAME session, then drop the
			// connection: closing the session releases the lock even if the
			// explicit unlock never runs, so a crashed process cannot wedge
			// future runs.
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), `SELECT RELEASE_LOCK('forge_migrations')`)
			_ = conn.Close()
		}
		return release, nil
	case DatabaseSQLite:
		sqliteRunnerMu.Lock()
		return func() { sqliteRunnerMu.Unlock() }, nil
	default:
		conn, err := mr.driver.DB().Conn(ctx)
		if err != nil {
			return nil, fmt.Errorf("acquire migration lock connection: %w", err)
		}
		if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationAdvisoryLockID); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("acquire migration advisory lock: %w", err)
		}
		release := func() {
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, migrationAdvisoryLockID)
			_ = conn.Close()
		}
		return release, nil
	}
}

func (mr *MigrationRunner) Run(ctx context.Context) error {
	releaseLock, err := mr.acquireRunnerLock(ctx)
	if err != nil {
		return err
	}
	defer releaseLock()

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
	sortMigrationFiles(sqlFiles)

	if err := validateNoDuplicatePrefixes(sqlFiles); err != nil {
		return err
	}
	if err := validateMigrationHashes(migrationPaths); err != nil {
		return err
	}

	runMigrationIDs, err := mr.getRunMigrationIDs(ctx)
	if err != nil {
		return fmt.Errorf("list run migrations: %w", err)
	}

	// Backfill guard rows for canonicals that already applied: a host that
	// ran the canonical DDL under its new name must also carry the guard
	// filename in history so the guard never executes needlessly. The reverse
	// is deliberately NOT done — a guard row proves nothing (guards are
	// no-ops), so a missing canonical is always executed, never backfilled.
	// INSERT ... ON CONFLICT / OR IGNORE keeps this safe to re-run.
	if err := mr.backfillMigrationAliases(ctx, runMigrationIDs); err != nil {
		return err
	}

	for _, file := range sqlFiles {
		if _, exists := runMigrationIDs[file]; exists {
			continue
		}
		// Guard skip: this file is a no-op whose canonical already applied,
		// so record it without executing. Canonicals NEVER skip via alias:
		// a guard row cannot prove the DDL ran (fresh hosts record the guard
		// as a no-op), and every canonical is idempotent, so re-applying on
		// an old host that already has the objects converges harmlessly.
		if isGuardFile(file) && canonicalApplied(file, runMigrationIDs) {
			if err := mr.recordMigration(ctx, nil, file); err != nil {
				return err
			}
			runMigrationIDs[file] = struct{}{}
			continue
		}

		data, err := os.ReadFile(migrationPaths[file])
		if err != nil {
			return fmt.Errorf("read migration %s: %w", file, err)
		}

		sql := string(data)
		switch mr.driver.Type() {
		case DatabaseSQLite:
			sql, err = sqliteCompatibleMigration(sql)
			if err != nil {
				return fmt.Errorf("translate migration %s for sqlite: %w", file, err)
			}
		case DatabaseMySQL, DatabaseMariaDB:
			sql = mysqlCompatibleMigration(sql)
		}
		statements := splitSQLStatements(sql)

		// MySQL/MariaDB have no transactional DDL: every CREATE/ALTER TABLE
		// commits implicitly, so the transaction below is best-effort only —
		// a failure partway through a file leaves the earlier DDL committed
		// with NO schema_migrations record. Migrations MUST therefore be
		// written idempotently (IF NOT EXISTS / dialect overrides) so a retry
		// converges, and a mid-file failure is reported loudly (see the error
		// wrap below) instead of looking like a clean rollback. Production
		// PostgreSQL is fully transactional here; MySQL is best-effort.
		tx, err := mr.driver.BeginTx(ctx)
		if err != nil {
			return fmt.Errorf("begin transaction for %s: %w", file, err)
		}

		// A migration that declares itself non-reversible applies normally;
		// the marker only affects Rollback, which must refuse loudly instead
		// of deleting history it cannot undo. Fail here only on genuinely
		// untranslatable DDL so a silent skip can never masquerade as success.
		for _, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if mr.driver.Type() == DatabaseSQLite {
				if untranslatable := sqliteUntranslatableDDL(stmt); untranslatable != "" {
					tx.Rollback()
					return fmt.Errorf("run migration %s: statement requires %s, which has no SQLite translation; add a sqlite/ dialect override for this file (stmt: %.120s)",
						file, untranslatable, stmt)
				}
				if sqliteSkippedDDL(stmt) {
					fmt.Fprintf(os.Stderr, "migration %s: skipping PostgreSQL-only statement on sqlite: %.120s\n", file, singleLine(stmt))
					continue
				}
				for _, expanded := range splitSQLiteAlterAdd(stmt) {
					if _, err := tx.ExecContext(ctx, strings.TrimSpace(expanded)); err != nil {
						if strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
							// The column already exists — but "exists" is not
							// "identical". A re-applied migration whose column
							// type drifted from the canonical definition would
							// otherwise converge SILENTLY on SQLite while
							// production PostgreSQL has the canonical type.
							// Verify compatibility and fail loudly on mismatch.
							if cerr := assertSQLiteColumnCompatible(ctx, tx, strings.TrimSpace(expanded)); cerr != nil {
								tx.Rollback()
								return fmt.Errorf("run migration %s: %w", file, cerr)
							}
							continue
						}
						tx.Rollback()
						return fmt.Errorf("run migration %s: %w (stmt: %s)", file, err, strings.TrimSpace(expanded))
					}
				}
				continue
			}
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				tx.Rollback()
				if mr.driver.Type() == DatabaseMySQL || mr.driver.Type() == DatabaseMariaDB {
					return fmt.Errorf("run migration %s: %w (stmt: %s); NOTE: MySQL commits DDL implicitly, so statements before this failure are already committed and will NOT roll back — fix the cause and re-run to converge (migrations are idempotent), or reconcile manually", file, err, stmt)
				}
				return fmt.Errorf("run migration %s: %w (stmt: %s)", file, err, stmt)
			}
		}

		if err := mr.recordMigration(ctx, tx, file); err != nil {
			tx.Rollback()
			if mr.driver.Type() == DatabaseMySQL || mr.driver.Type() == DatabaseMariaDB {
				return fmt.Errorf("record migration %s: %w; NOTE: MySQL commits DDL implicitly, so the schema changes above are already committed despite this failure — fix the cause and re-run to record", file, err)
			}
			return err
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", file, err)
		}
		runMigrationIDs[file] = struct{}{}
	}

	return nil
}

// sqlTx is the subset of *sql.Tx the migration recorder needs, so tests can
// substitute fakes and both runners share one record path.
type sqlTx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// recordMigration inserts the filename into schema_migrations inside tx, or in
// its own statement when tx is nil (alias backfill path).
func (mr *MigrationRunner) recordMigration(ctx context.Context, tx sqlTx, file string) error {
	recordSQL := getRecordMigrationSQL(mr.driver.Type())
	if mr.driver.Type() == DatabaseSQLite {
		// INSERT OR IGNORE keeps alias backfills idempotent on SQLite, which
		// has no ON CONFLICT DO NOTHING parity issue for a single-column PK
		// insert (both forms work; OR IGNORE also tolerates races).
		recordSQL = `INSERT OR IGNORE INTO schema_migrations (version) VALUES (?)`
	}
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, recordSQL, file)
	} else {
		_, err = mr.driver.Exec(ctx, recordSQL, file)
	}
	if err != nil {
		return fmt.Errorf("record migration %s: %w", file, err)
	}
	return nil
}

// backfillMigrationAliases records the guard side of every rename alias
// whose canonical side already applied. Directional by design: it never
// records a canonical from a guard row, because a guard row (a no-op SELECT)
// cannot prove the schema effect exists.
func (mr *MigrationRunner) backfillMigrationAliases(ctx context.Context, applied map[string]struct{}) error {
	for _, pair := range migrationAliases {
		guard, canonical := pair[0], pair[1]
		if _, ok := applied[canonical]; ok {
			if _, ok := applied[guard]; !ok {
				if err := mr.recordMigration(ctx, nil, guard); err != nil {
					return err
				}
				applied[guard] = struct{}{}
			}
		}
	}
	return nil
}

func containsKey(set map[string]struct{}, key string) bool {
	_, ok := set[key]
	return ok
}

// isGuardFile reports whether file is the retired (no-op guard) side of a
// rename alias. Guards carry no DDL; they exist only so hosts that recorded
// the old filename keep clean history.
func isGuardFile(file string) bool {
	for _, pair := range migrationAliases {
		if pair[0] == file {
			return true
		}
	}
	return false
}

// canonicalOf returns the canonical (DDL-bearing) counterpart of a guard
// file, or "" when file is not a guard.
func canonicalOf(file string) string {
	for _, pair := range migrationAliases {
		if pair[0] == file {
			return pair[1]
		}
	}
	return ""
}

// canonicalApplied reports whether the canonical counterpart of a guard file
// already applied.
func canonicalApplied(file string, applied map[string]struct{}) bool {
	canonical := canonicalOf(file)
	if canonical == "" {
		return false
	}
	return containsKey(applied, canonical)
}

// splitSQLiteAlterAdd expands a multi-column ALTER TABLE ... ADD COLUMN into
// one-column-per-ALTER statements: SQLite permits exactly one ADD COLUMN per
// ALTER TABLE. The parser is literal-aware (commas inside quoted defaults,
// CHECK(...) parens, or dollar-quoted bodies do not split) and applies to any
// table, not a hardcoded list. "--" comments are skipped while scanning:
// they routinely contain commas, parens, and apostrophes.
func splitSQLiteAlterAdd(stmt string) []string {
	upper := strings.ToUpper(stmt)
	if !strings.HasPrefix(upper, "ALTER TABLE ") || !strings.Contains(upper, "ADD COLUMN") {
		return []string{stmt}
	}
	// Normalize the idempotency guard first: the canonical stream uses
	// ADD COLUMN IF NOT EXISTS (Postgres 9.6+); SQLite needs it stripped.
	columns := splitAddColumns(stmt)
	if len(columns) <= 1 {
		out := strings.Replace(stmt, "ADD COLUMN IF NOT EXISTS", "ADD COLUMN", 1)
		out = strings.Replace(out, "add column if not exists", "ADD COLUMN", 1)
		return []string{out}
	}
	table := alterTableName(stmt)
	result := make([]string, 0, len(columns))
	for i, column := range columns {
		column = strings.TrimSpace(column)
		if i == 0 {
			// The first fragment carries the "ALTER TABLE <name>" head;
			// drop it so every part is rebuilt uniformly below.
			idx := strings.Index(strings.ToUpper(column), "ADD COLUMN")
			if idx < 0 {
				return []string{stmt}
			}
			column = strings.TrimSpace(column[idx:])
		}
		column = strings.Replace(column, "ADD COLUMN IF NOT EXISTS", "ADD COLUMN", 1)
		column = strings.Replace(column, "add column if not exists", "ADD COLUMN", 1)
		result = append(result, "ALTER TABLE "+table+" "+strings.TrimSpace(column))
	}
	return result
}

// assertSQLiteColumnCompatible verifies that a duplicate-column ADD COLUMN is
// a harmless re-application, not silent type divergence. SQLite reports
// "duplicate column name" whenever the column exists, even if the existing
// column has a DIFFERENT type than the canonical migration defines —
// swallowing that error would let a dev/test database drift from production
// PostgreSQL without a word. A type match continues silently; anything else
// (unparseable definition, missing column in PRAGMA, type mismatch) fails
// loudly so the divergence is fixed instead of hidden.
func assertSQLiteColumnCompatible(ctx context.Context, tx *sql.Tx, stmt string) error {
	upper := strings.ToUpper(stmt)
	idx := strings.Index(upper, "ADD COLUMN")
	if idx < 0 {
		return fmt.Errorf("cannot verify duplicate column: no ADD COLUMN in statement %.120s", stmt)
	}
	rest := strings.TrimSpace(stmt[idx+len("ADD COLUMN"):])
	// Strip the idempotency guard the translator normalizes around.
	if up := strings.ToUpper(rest); strings.HasPrefix(up, "IF NOT EXISTS") {
		rest = strings.TrimSpace(rest[len("IF NOT EXISTS"):])
	}
	var colName string
	if strings.HasPrefix(rest, "\"") {
		end := strings.Index(rest[1:], "\"")
		if end < 0 {
			return fmt.Errorf("cannot verify duplicate column: unterminated quoted name in %.120s", stmt)
		}
		colName = rest[1 : 1+end]
		rest = strings.TrimSpace(rest[1+end+1:])
	} else {
		fields := strings.Fields(rest)
		if len(fields) < 2 {
			return fmt.Errorf("cannot verify duplicate column: unparseable definition %.120s", stmt)
		}
		colName = fields[0]
		rest = strings.TrimSpace(rest[len(fields[0]):])
	}
	wantType := sqliteColumnTypeHead(rest)
	if wantType == "" {
		return fmt.Errorf("cannot verify duplicate column %q: missing type in %.120s", colName, stmt)
	}
	table := strings.Trim(alterTableName(stmt), "\"")
	pragma := `PRAGMA table_info("` + strings.ReplaceAll(table, `"`, `""`) + `")`
	rows, err := tx.QueryContext(ctx, pragma)
	if err != nil {
		return fmt.Errorf("cannot verify duplicate column %q on %q: %w", colName, table, err)
	}
	defer rows.Close()
	found := false
	var haveType string
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		// PRAGMA table_info: cid, name, type, notnull, dflt_value, pk.
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("cannot verify duplicate column %q on %q: %w", colName, table, err)
		}
		if strings.EqualFold(name, colName) {
			found = true
			haveType = ctype
			break
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot verify duplicate column %q on %q: %w", colName, table, err)
	}
	if !found {
		return fmt.Errorf("cannot verify duplicate column %q on %q: column missing from PRAGMA table_info despite duplicate-column error (stmt: %.120s)", colName, table, stmt)
	}
	if !strings.EqualFold(collapseSpaces(haveType), collapseSpaces(wantType)) {
		return fmt.Errorf("SQLite column %q.%q has type %q but migration declares %q: dev/test database diverges from production PostgreSQL; align the column instead of silently keeping it (stmt: %.120s)",
			table, colName, haveType, wantType, stmt)
	}
	return nil
}

// sqliteColumnTypeHead extracts the declared type head from the remainder of
// an ADD COLUMN definition (everything after the column name), stopping at
// the first constraint keyword. "TEXT NOT NULL DEFAULT 'x'" -> "TEXT";
// "DOUBLE PRECISION NOT NULL" -> "DOUBLE PRECISION"; "VARCHAR(36)" stays whole.
func sqliteColumnTypeHead(rest string) string {
	var parts []string
	for _, tok := range strings.Fields(rest) {
		switch strings.ToUpper(tok) {
		case "NOT", "NULL", "PRIMARY", "DEFAULT", "UNIQUE", "CHECK",
			"REFERENCES", "COLLATE", "GENERATED", "AS", "CONSTRAINT":
			return strings.Join(parts, " ")
		}
		parts = append(parts, tok)
		// A bare type is one token unless it is a multi-word type whose next
		// token continues it (e.g. "DOUBLE PRECISION", "CHARACTER VARYING").
		// Constraint keywords above terminate; anything else keeps consuming
		// only for the known multi-word heads.
		if len(parts) == 1 {
			switch strings.ToUpper(parts[0]) {
			case "DOUBLE", "CHARACTER":
				continue
			}
			// Single-word type, unless a "(n)" qualifier got split across a
			// space — Fields keeps "VARCHAR(36)" whole, so we are done.
			if !strings.HasSuffix(tok, "(") {
				return strings.Join(parts, " ")
			}
		} else {
			return strings.Join(parts, " ")
		}
	}
	return strings.Join(parts, " ")
}

// collapseSpaces folds runs of whitespace to a single space for type compare.
func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// alterTableName extracts the table name from an ALTER TABLE statement,
// handling optional ONLY / IF EXISTS and quoted identifiers.
func alterTableName(stmt string) string {
	rest := strings.TrimSpace(stmt[len("ALTER TABLE "):])
	rest = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), "ONLY"))
	rest = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), "IF EXISTS"))
	if strings.HasPrefix(rest, "\"") {
		if end := strings.Index(rest[1:], "\""); end >= 0 {
			return rest[:end+2]
		}
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return rest
	}
	return fields[0]
}

// splitAddColumns splits an ALTER TABLE statement on top-level ", ADD COLUMN"
// delimiters, ignoring commas inside parentheses, single-quoted literals,
// and dollar-quoted blocks. It returns the column definition fragments: the
// first still carries the "ALTER TABLE <name>" head, the rest start at their
// own ADD COLUMN keyword. Byte-oriented and ASCII-safe; migration files are
// ASCII by convention (identifiers, keywords, and quoted defaults).
func splitAddColumns(stmt string) []string {
	const keyword = "ADD COLUMN"
	var fragments []string
	start := 0
	depth := 0
	inStr := false
	i := 0
	armed := false // set once the first top-level ADD COLUMN is seen
	for i < len(stmt) {
		c := stmt[i]
		if inStr {
			if c == '\'' {
				if i+1 < len(stmt) && stmt[i+1] == '\'' {
					i += 2
					continue
				}
				inStr = false
			}
			i++
			continue
		}
		switch c {
		case '\'':
			inStr = true
			i++
		case '-':
			// Single-line comment: skip to end of line. Comments carry
			// commas, parens, and apostrophes ("don't") that must not
			// affect depth tracking or literal detection.
			if i+1 < len(stmt) && stmt[i+1] == '-' {
				for i < len(stmt) && stmt[i] != '\n' {
					i++
				}
				continue
			}
			i++
		case '$':
			if tag := scanDollarTagStr(stmt, i); tag != "" {
				end := strings.Index(stmt[i+len(tag):], tag)
				if end < 0 {
					i = len(stmt)
				} else {
					i += len(tag) + end + len(tag)
				}
			} else {
				i++
			}
		case '(':
			depth++
			i++
		case ')':
			if depth > 0 {
				depth--
			}
			i++
		case ',':
			if armed && depth == 0 && isAddColumnAt(stmt, i+1) {
				fragments = append(fragments, strings.TrimSpace(stmt[start:i]))
				i++
				start = i
				continue
			}
			i++
		default:
			if !armed && depth == 0 && isAddColumnAt(stmt, i) {
				armed = true
				i += len(keyword)
				continue
			}
			i++
		}
	}
	fragments = append(fragments, strings.TrimSpace(stmt[start:]))
	if len(fragments) <= 1 {
		return []string{stmt}
	}
	return fragments
}

// isAddColumnAt reports whether an ADD COLUMN keyword starts at stmt[pos],
// skipping whitespace. Case-insensitive, byte-safe.
func isAddColumnAt(stmt string, pos int) bool {
	for pos < len(stmt) && (stmt[pos] == ' ' || stmt[pos] == '\t' || stmt[pos] == '\n' || stmt[pos] == '\r') {
		pos++
	}
	const keyword = "ADD COLUMN"
	if pos+len(keyword) > len(stmt) {
		return false
	}
	return strings.EqualFold(stmt[pos:pos+len(keyword)], keyword)
}

func scanDollarTagStr(s string, pos int) string {
	if pos >= len(s) || s[pos] != '$' {
		return ""
	}
	i := pos + 1
	start := i
	for i < len(s) && (isDollarChar(s[i])) {
		i++
	}
	if i >= len(s) || s[i] != '$' {
		return ""
	}
	return "$" + s[start:i] + "$"
}

func isDollarChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// sqliteSkippedDDL reports whether a statement is a PostgreSQL-only maintenance
// form with no SQLite equivalent and no schema effect worth emulating:
// constraint guards are already idempotent by construction on a fresh SQLite
// build (CREATE TABLE IF NOT EXISTS carries the constraints), and COMMENT /
// procedural objects carry no relational schema.
//
// DOCUMENTED DIVERGENCE GAP: every skip below makes a SQLite dev/test database
// structurally weaker than production PostgreSQL, and the gap is silent after
// the one stderr warning the caller emits:
//   - ADD/DROP CONSTRAINT skips mean SQLite never enforces those CHECK / FK /
//     UNIQUE rules: constraint-violation bugs will not reproduce on SQLite.
//   - ALTER COLUMN skips mean SQLite keeps the OLD column type while
//     production moves on (the followup that changes a type is a no-op here).
//   - DROP COLUMN skips mean SQLite retains columns production dropped; code
//     that still reads them looks healthy on SQLite and breaks on PostgreSQL.
// PostgreSQL is authoritative. Anything that must hold on SQLite needs a
// sqlite/ dialect override; anything skipped here is accepted drift, not
// parity.
func sqliteSkippedDDL(stmt string) bool {
	upperStmt := strings.TrimSpace(strings.ToUpper(stmt))
	if strings.Contains(upperStmt, "DROP CONSTRAINT") || strings.Contains(upperStmt, "ADD CONSTRAINT") {
		return true
	}
	if strings.Contains(upperStmt, "ALTER COLUMN") || strings.Contains(upperStmt, "DROP COLUMN") {
		return true
	}
	for _, prefix := range []string{
		"DO $$", "COMMENT ON", "CREATE OR REPLACE FUNCTION", "CREATE FUNCTION",
		"CREATE OR REPLACE VIEW", "DROP VIEW", "CREATE EXTENSION",
		"CREATE TYPE", "ALTER TYPE",
	} {
		if strings.HasPrefix(upperStmt, prefix) {
			return true
		}
	}
	// Plain CREATE VIEW is valid SQLite and now executes (e.g. the backup
	// overview views); only the PG-only OR REPLACE form stays skipped.
	if strings.Contains(upperStmt, "LANGUAGE PLPGSQL") || strings.Contains(upperStmt, "EXECUTE FUNCTION") {
		return true
	}
	// Data backfills written with PostgreSQL-only functions have no SQLite
	// spelling. They are skipped with a loud stderr warning (emitted by the
	// caller), never silently: a sqlite dev/test database simply does not get
	// that backfill. Anything that changes stored shape (triggers, FTS,
	// jsonb aggregations) is NOT in this list — see sqliteUntranslatableDDL,
	// which fails loudly instead.
	lowerStmt := strings.ToLower(stmt)
	for _, marker := range []string{
		"regexp_replace", "string_to_array", "substring(",
		"cardinality(", "jsonb_typeof", "text_typeof", "text_object_agg",
	} {
		if strings.Contains(lowerStmt, marker) {
			return true
		}
	}
	return false
}

// sqliteUntranslatableDDL returns a short description of the SQLite-missing
// feature a statement depends on, or "" when the statement is translatable or
// covered by sqliteSkippedDDL. These change stored data shape, enforce
// invariants, or build indexes SQLite cannot build, so silently dropping them
// would corrupt a dev/test database while production PostgreSQL behaves
// differently. The runner fails loudly instead; add a sqlite/ dialect
// override to resolve it. Every migration currently hitting this tier ships
// such an override (043, 104_a, 140, 144, 212), so the error only fires for
// future migrations that forget one.
func sqliteUntranslatableDDL(stmt string) string {
	lower := strings.ToLower(stmt)
	// PostgreSQL trigger bindings have no inline SQLite spelling. (SQLite
	// native triggers in dialect overrides do not contain these markers and
	// pass through to execution.)
	if strings.Contains(lower, "execute function") || strings.Contains(lower, "execute procedure") {
		return "PostgreSQL trigger EXECUTE FUNCTION/PROCEDURE binding"
	}
	if isPostgresDropTrigger(stmt) {
		return "PostgreSQL DROP TRIGGER ... ON syntax"
	}
	// Full-text search and Postgres-only aggregations change stored data shape.
	for _, marker := range []string{
		"to_tsvector", "to_tsquery", "using gin", "using gist",
		"jsonb_object_agg", "jsonb_array_elements_text",
	} {
		if strings.Contains(lower, marker) {
			return marker
		}
	}
	return ""
}

// isPostgresDropTrigger matches "DROP TRIGGER [IF EXISTS] name ON table",
// which is mandatory in PostgreSQL and a syntax error in SQLite (whose DROP
// TRIGGER takes only the trigger name).
func isPostgresDropTrigger(stmt string) bool {
	upper := strings.ToUpper(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(stmt), ";")))
	if !strings.HasPrefix(upper, "DROP TRIGGER ") {
		return false
	}
	upper = strings.TrimPrefix(upper, "DROP TRIGGER ")
	upper = strings.TrimPrefix(upper, "IF EXISTS ")
	// Remaining form is "<name> ON <table>" in PostgreSQL.
	return strings.Contains(upper, " ON ")
}

// mysqlWordReplacements are whole-word, case-insensitive rewrites for the
// small PostgreSQL-specific subset used by the canonical migrations. They
// apply ONLY to code segments — never inside string literals, quoted
// identifiers, dollar-quoted blocks, or comments (see
// mysqlCompatibleMigration). A naive substring replacer corrupts identifiers
// that merely CONTAIN a type name: serial_number became
// "int auto_increment_number", cabinet would gain "varchar(45)", and so on
// (live case: 114_a_mtls_certificates.serial_number). \b is safe here because
// _ counts as a word character, so serial_number / uuid_col / cabinet never
// match SERIAL / UUID / INET.
//
// Order matters: plain type words are rewritten BEFORE the function defaults
// that introduce new UUID()/CURRENT_TIMESTAMP text, so generated text is
// never re-processed (gen_random_uuid() -> UUID() must survive; a later
// UUID -> CHAR(36) pass would corrupt it into CHAR(36)()).
var mysqlWordReplacements = []struct {
	pattern *regexp.Regexp
	repl    string
}{
	{regexp.MustCompile(`(?i)\bBIGSERIAL\b`), "BIGINT AUTO_INCREMENT"},
	{regexp.MustCompile(`(?i)\bSERIAL\b`), "INT AUTO_INCREMENT"},
	{regexp.MustCompile(`(?i)\bTIMESTAMPTZ\b`), "TIMESTAMP"},
	{regexp.MustCompile(`(?i)\bJSONB\b`), "JSON"},
	{regexp.MustCompile(`(?i)\bUUID\b`), "CHAR(36)"},
	{regexp.MustCompile(`(?i)\bINET\b`), "VARCHAR(45)"},
	{regexp.MustCompile(`(?i)\bTEXT\s*\[\]`), "TEXT"},
	{regexp.MustCompile(`(?i)\bgen_random_uuid\s*\(\s*\)`), "UUID()"},
	{regexp.MustCompile(`(?i)\bnow\s*\(\s*\)`), "CURRENT_TIMESTAMP"},
	{regexp.MustCompile(`(?i)::\s*(jsonb|json|text|uuid|inet)\b`), ""},
}

// mysqlConflictLookback bounds the upward search for the INSERT belonging to
// an ON CONFLICT clause. INSERT..SELECT bodies routinely span dozens of lines
// (043's egg backfill is ~20), so the old 6-line window silently missed them.
const mysqlConflictLookback = 100

// mysqlTranslateCodeSegment applies mysqlWordReplacements to one literal-free
// code segment.
func mysqlTranslateCodeSegment(seg string) string {
	for _, r := range mysqlWordReplacements {
		seg = r.pattern.ReplaceAllString(seg, r.repl)
	}
	return seg
}

// mysqlCompatibleMigration adapts the small PostgreSQL-specific subset used by
// the canonical migrations for MySQL/MariaDB. Coverage is intentionally
// narrow: gen_random_uuid() defaults, TIMESTAMPTZ, JSONB/UUID/INET types, and
// ON CONFLICT DO NOTHING upserts. Statements outside this subset must ship a
// mysql/ dialect override; see the mysql/ directory.
//
// MySQL is a best-effort target: the deployed and CI-verified path is
// PostgreSQL (with SQLite for local dev/tests). If this translator cannot
// cover a migration, add the override rather than hand-editing the canonical
// file into a cross-dialect compromise.
//
// The rewrite is literal-aware: single-quoted literals, double-quoted
// identifiers, dollar-quoted blocks, and both comment styles are copied
// verbatim so data (a 'serial' default, a "cabinet" identifier, a regex
// containing "now()") is never rewritten — only code is.
func mysqlCompatibleMigration(sql string) string {
	var out strings.Builder
	out.Grow(len(sql))
	start := 0 // start of the current code segment
	flush := func(end int) {
		if end > start {
			out.WriteString(mysqlTranslateCodeSegment(sql[start:end]))
		}
		start = end
	}
	i := 0
	for i < len(sql) {
		c := sql[i]
		switch {
		case c == '\'' || c == '"':
			// Literal or quoted identifier: copy verbatim, honouring the
			// doubled-quote escape.
			flush(i)
			j := i + 1
			for j < len(sql) {
				if sql[j] == c {
					if j+1 < len(sql) && sql[j+1] == c {
						j += 2
						continue
					}
					j++
					break
				}
				j++
			}
			out.WriteString(sql[i:j])
			i, start = j, j
		case c == '-' && i+1 < len(sql) && sql[i+1] == '-':
			flush(i)
			j := i
			for j < len(sql) && sql[j] != '\n' {
				j++
			}
			out.WriteString(sql[i:j])
			i, start = j, j
		case c == '/' && i+1 < len(sql) && sql[i+1] == '*':
			flush(i)
			j := len(sql)
			if end := strings.Index(sql[i+2:], "*/"); end >= 0 {
				j = i + 2 + end + 2
			}
			out.WriteString(sql[i:j])
			i, start = j, j
		case c == '$':
			if tag := scanDollarTagStr(sql, i); tag != "" {
				flush(i)
				j := len(sql)
				if end := strings.Index(sql[i+len(tag):], tag); end >= 0 {
					j = i + len(tag) + end + len(tag)
				}
				out.WriteString(sql[i:j])
				i, start = j, j
			} else {
				i++
			}
		default:
			i++
		}
	}
	flush(len(sql))
	return mysqlRewriteOnConflict(out.String())
}

// mysqlRewriteOnConflict converts every "ON CONFLICT ... DO NOTHING"
// idempotency guard (bare or with a column list) into INSERT IGNORE, which is
// the equivalent MySQL spelling for a pure guard. INSERT IGNORE is key-shape
// agnostic: the previous rewrite ("ON CONFLICT DO NOTHING" ->
// "ON DUPLICATE KEY UPDATE id=id") assumed an "id" column and produced
// invalid MySQL for composite-PK tables such as mount_server(mount_id,
// server_id) (043's backfill). "ON CONFLICT ... DO UPDATE" forms have no
// translation here and are left for the raw driver error / dialect override.
func mysqlRewriteOnConflict(sql string) string {
	lines := strings.Split(sql, "\n")
	for i, line := range lines {
		upper := strings.ToUpper(line)
		cIdx := strings.Index(upper, "ON CONFLICT")
		if cIdx < 0 {
			continue
		}
		tail := upper[cIdx:]
		if !strings.Contains(tail, "DO NOTHING") || strings.Contains(tail, "DO UPDATE") {
			continue
		}
		// Strip the clause, keeping anything before it (e.g. a SELECT list).
		lines[i] = strings.TrimRight(line[:cIdx], " \t")
		// Mark the enclosing INSERT so it becomes INSERT IGNORE.
		for j := i - 1; j >= 0 && j >= i-mysqlConflictLookback; j-- {
			trimmed := strings.TrimSpace(lines[j])
			if strings.HasPrefix(trimmed, "--") {
				continue
			}
			uj := strings.ToUpper(lines[j])
			if ins := strings.Index(uj, "INSERT INTO"); ins >= 0 {
				if strings.Contains(uj, "INSERT IGNORE") {
					break
				}
				lines[j] = lines[j][:ins] + "INSERT IGNORE INTO" + lines[j][ins+len("INSERT INTO"):]
				break
			}
			if strings.HasSuffix(strings.TrimSpace(uj), ";") {
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}

// sqliteCompatibleMigration adapts the small PostgreSQL-specific subset used by
// the canonical migrations. SQLite is supported for local development and tests;
// production PostgreSQL migrations are deliberately left byte-for-byte intact.
//
// It returns an error for statements that fail loudly (see
// sqliteUntranslatableDDL); the caller decides per statement. Cast stripping
// is literal-aware: a "::" inside a single-quoted literal or dollar-quoted
// block (e.g. a JSON default or a regex) is data, not a cast.
func sqliteCompatibleMigration(sql string) (string, error) {
	replacer := strings.NewReplacer(
		"TIMESTAMPTZ", "TIMESTAMP", "timestamptz", "timestamp",
		"jsonb_build_object(t.image, t.image)", "('{\"' || t.image || '\":\"' || t.image || '\"}')",
		"jsonb_build_object", "",
		"jsonb_object_agg", "",
		"jsonb_array_elements_text", "",
		"JSONB", "TEXT", "jsonb", "text",
		"TEXT[]", "TEXT", "text[]", "text",
		"UUID", "TEXT", "uuid", "text",
		"INET", "TEXT", "inet", "text",
		"now()", "CURRENT_TIMESTAMP", "NOW()", "CURRENT_TIMESTAMP",
		"DEFAULT gen_random_uuid()", "DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab', 1 + (abs(random()) % 4), 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))))",
		"DEFAULT gen_random_uuid ()", "DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab', 1 + (abs(random()) % 4), 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))))",
		"gen_random_uuid()", "(lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab', 1 + (abs(random()) % 4), 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))))",
		"split_part(email, '@', 1)", "substr(email, 1, instr(email, '@') - 1)",
		"CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$')", "CHECK (length(slug) > 0)",
		"CHECK (color ~ '^#[0-9a-fA-F]{6}$')", "CHECK (length(color) = 7)",
	)
	sql = replacer.Replace(sql)
	sql = strings.ReplaceAll(sql, "ALTER TABLE allocations\n    ALTER COLUMN ip TYPE text USING ip;", "")
	sql = strings.ReplaceAll(sql, "ALTER TABLE allocations\n    ALTER COLUMN ip TYPE text USING ip", "")
	sql = strings.ReplaceAll(sql, "ALTER TABLE allocations\n    DROP CONSTRAINT IF EXISTS allocations_port_range_check;", "")
	sql = strings.ReplaceAll(sql, "ALTER TABLE allocations\n    ADD CONSTRAINT allocations_port_range_check CHECK (port BETWEEN 1 AND 65535);", "")
	// Split every multi-column ADD COLUMN into one-column-per-ALTER (SQLite
	// permits only one ADD COLUMN per ALTER TABLE). Handled generically by
	// splitSQLiteAlterAdd at execution time; this pre-pass keeps the text
	// readable for the statement splitter.
	sql = stripCastsOutsideLiterals(sql)
	return sql, nil
}

// stripCastsOutsideLiterals removes PostgreSQL "::type" casts except inside
// single-quoted literals and dollar-quoted blocks, where "::" is data.
//
// Comment-awareness matters: migration comments contain apostrophes
// ("server's", "don't") that would otherwise open a phantom literal and
// desync the scanner, silently preserving casts (or swallowing code) for the
// rest of the file. "--" and "/* */" comments are copied through verbatim when
// the scanner is not inside a literal or dollar block.
func stripCastsOutsideLiterals(sql string) string {
	var out strings.Builder
	out.Grow(len(sql))
	i := 0
	for i < len(sql) {
		c := sql[i]
		if c == '\'' {
			// Copy the literal verbatim, honouring '' escapes.
			out.WriteByte(c)
			i++
			for i < len(sql) {
				out.WriteByte(sql[i])
				if sql[i] == '\'' {
					if i+1 < len(sql) && sql[i+1] == '\'' {
						out.WriteByte(sql[i+1])
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
			continue
		}
		if c == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			// Single-line comment: copy to end of line without scanning it.
			for i < len(sql) && sql[i] != '\n' {
				out.WriteByte(sql[i])
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			// Block comment: copy to the closing tag without scanning it, so
			// a "::" inside (e.g. documenting a cast) is data, not a cast.
			out.WriteString("/*")
			i += 2
			for i < len(sql) {
				out.WriteByte(sql[i])
				if sql[i] == '*' && i+1 < len(sql) && sql[i+1] == '/' {
					out.WriteByte('/')
					i += 2
					break
				}
				i++
			}
			continue
		}
		if c == '$' {
			if tag := scanDollarTagStr(sql, i); tag != "" {
				end := strings.Index(sql[i+len(tag):], tag)
				if end < 0 {
					out.WriteString(sql[i:])
					break
				}
				out.WriteString(sql[i : i+len(tag)+end+len(tag)])
				i += len(tag) + end + len(tag)
				continue
			}
			out.WriteByte(c)
			i++
			continue
		}
		if c == ':' && i+1 < len(sql) && sql[i+1] == ':' {
			// Skip the cast target: run of letters, digits, underscores,
			// spaces (e.g. "::double precision", "::character varying(36)").
			j := i + 2
			for j < len(sql) && (sql[j] == ' ' || sql[j] == '\t') {
				j++
			}
			k := j
			for k < len(sql) && ((sql[k] >= 'a' && sql[k] <= 'z') || (sql[k] >= 'A' && sql[k] <= 'Z') || (sql[k] >= '0' && sql[k] <= '9') || sql[k] == '_') {
				k++
			}
			if k == j {
				// Not a cast (e.g. slice syntax); keep it.
				out.WriteString("::")
				i += 2
				continue
			}
			// Skip an optional "(n)" length qualifier.
			if k < len(sql) && sql[k] == '(' {
				depth := 1
				k++
				for k < len(sql) && depth > 0 {
					if sql[k] == '(' {
						depth++
					} else if sql[k] == ')' {
						depth--
					}
					k++
				}
			}
			i = k
			continue
		}
		out.WriteByte(c)
		i++
	}
	return out.String()
}

func (mr *MigrationRunner) getRunMigrationIDs(ctx context.Context) (map[string]struct{}, error) {
	query := getListMigrationsSQL(mr.driver.Type())
	rows, err := mr.driver.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make(map[string]struct{})
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan migration ID: %w", err)
		}
		ids[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

// migrationAliases records historical renames as (guard, canonical) pairs.
// The guard is the retired filename, kept on disk as a no-op; the canonical
// is the DDL-bearing file that fresh installs execute. Both sides stay on
// disk because the filename is the PK in schema_migrations.
//
// Skip/backfill semantics are DIRECTIONAL (a guard row proves nothing):
//   - a guard is skipped (recorded without executing) when its canonical
//     already applied;
//   - a canonical is NEVER skipped via alias: every canonical is idempotent,
//     so executing it on an old host that already has the objects converges
//     harmlessly, while skipping it on a host that only recorded the guard
//     would silently drop schema (the 021 regression class).
//   - backfill records guards from recorded canonicals, never the reverse.
//
// NEVER add a rename by renaming a file. Add the pair here AND keep both
// files on disk.
var migrationAliases = [][2]string{
	{"015_mounts.sql", "015_a_mounts.sql"},
	{"015_db_hosts_constraints.sql", "120_a_db_hosts_constraints.sql"},
	{"018_api_key_scopes.sql", "121_api_key_scopes.sql"},
	{"018_ssh_2fa_activity.sql", "018_a_ssh_2fa_activity.sql"},
	{"020_node_expansion.sql", "122_node_expansion.sql"},
	{"020_regions_multi_node_foundation.sql", "020_a_regions_multi_node_foundation.sql"},
	{"044_cloud_node_links.sql", "044_a_cloud_node_links.sql"},
	{"054_activity_events.sql", "124_activity_events.sql"},
	{"054_social_auth.sql", "054_a_social_auth.sql"},
	{"057_job_queue.sql", "057_a_job_queue.sql"},
	{"057_webauthn.sql", "057_b_webauthn.sql"},
	{"057_backup_policies.sql", "119_z_backup_policies.sql"},
	{"080_recovery_execution_statuses.sql", "080_a_recovery_execution_statuses.sql"},
	{"087_parity_schema.sql", "087_a_parity_schema.sql"},
}

// fkFollowupMigrations tracks tables whose earliest migration created them
// WITHOUT foreign keys, and the later migration that adds the keys. The
// divergent early copies (bare 082/083 files) predate the FK-bearing
// canonicals, so a host that applied the early copy only converges through
// the followup. TestFKFollowupsExist pins every followup file to disk; a
// table with no followup yet is listed with an empty value as a tracked gap,
// not silently forgotten.
var fkFollowupMigrations = map[string]string{
	"failover_policies":  "086_add_table_constraints.sql",
	"deployments":        "127_deployments.sql",
	"traffic_rules":      "133_b_routing_rules_persistence.sql",
	"scaling_policies":   "128_autoscaler.sql",
	"scaling_events":     "128_autoscaler.sql",
	"backup_policies":    "125_backup_policies.sql",
	"recovery_items":     "126_recovery_backup_execution.sql",
	"webhook_deliveries": "123_async_delivery_foundation.sql",
	// Tracked gap: target_group_targets.server_id / node_id were created
	// without FKs in 082_target_groups.sql and no later migration adds them
	// yet. Recorded here so the gap stays visible until a followup ships.
	"target_group_targets": "",
}

// grandfatheredRetiredFiles are the exact legacy fragments retained as
// read-only history in internal/store/migrations. They predate the number
// retirement, no runner executes them, and they must never be renamed — so
// the retired-number rule exempts these filenames and these filenames only.
var grandfatheredRetiredFiles = map[string]bool{
	"029_auth_oauth.sql":    true,
	"030_auth_webauthn.sql": true,
	"031_auth_ssh_2fa.sql":  true,
}

// retiredMigrationPrefixes are numeric slots that once existed (or were
// reserved) and must never be reused: reusing a retired number reorders the
// stream for hosts that recorded history under the old name and silently
// changes fresh-install ordering. Allocate a new highest number instead.
var retiredMigrationPrefixes = map[string]string{
	"029": "retired auth fragment, superseded by 027-028 series",
	"030": "retired auth fragment, superseded by 027-028 series",
	"031": "retired auth fragment, superseded by 027-028 series",
	"061": "retired, never shipped; gap between 060 and 077 is intentional",
	"062": "retired, never shipped; gap between 060 and 077 is intentional",
	"063": "retired, never shipped; gap between 060 and 077 is intentional",
	"064": "retired, never shipped; gap between 060 and 077 is intentional",
	"065": "retired, never shipped; gap between 060 and 077 is intentional",
	"066": "retired, never shipped; gap between 060 and 077 is intentional",
	"067": "retired, never shipped; gap between 060 and 077 is intentional",
	"068": "retired, never shipped; gap between 060 and 077 is intentional",
	"069": "retired, never shipped; gap between 060 and 077 is intentional",
	"070": "retired, never shipped; gap between 060 and 077 is intentional",
	"071": "retired, never shipped; gap between 060 and 077 is intentional",
	"072": "retired, never shipped; gap between 060 and 077 is intentional",
	"073": "retired, never shipped; gap between 060 and 077 is intentional",
	"074": "retired, never shipped; gap between 060 and 077 is intentional",
	"075": "retired, never shipped; gap between 060 and 077 is intentional",
	"076": "retired, never shipped; gap between 060 and 077 is intentional",
	"166": "retired phase fragment, superseded by 165/170-172 series",
	"167": "retired phase fragment, superseded by 165/170-172 series",
	"168": "retired phase fragment, superseded by 165/170-172 series",
	"169": "retired phase fragment, superseded by 165/170-172 series",
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

// migrationSortKey orders files numerically by their leading number, then by
// suffix: bare "082" sorts before "082_a", and "082_a" before "082_b". Plain
// lexicographic sort already does this for zero-padded numbers, but an
// explicit numeric comparison keeps ordering correct if padding ever slips.
func migrationSortKey(name string) (int, string) {
	prefix := migrationPrefix(name)
	numStr := prefix
	suffix := ""
	if idx := strings.Index(prefix, "_"); idx >= 0 {
		numStr = prefix[:idx]
		suffix = prefix[idx:]
	}
	num, err := strconv.Atoi(numStr)
	if err != nil {
		// Non-numeric names sort last, lexicographically.
		return 1<<30 - 1, name
	}
	return num, suffix + "\x00" + name
}

// sortMigrationFiles sorts migration filenames in apply order:
// numeric-then-suffix.
func sortMigrationFiles(files []string) {
	sort.Slice(files, func(i, j int) bool {
		ni, si := migrationSortKey(files[i])
		nj, sj := migrationSortKey(files[j])
		if ni != nj {
			return ni < nj
		}
		return si < sj
	})
}

// validateNoDuplicatePrefixes rejects two migrations that would sort under the
// same numeric prefix. Known historical numbering exceptions in the shipped
// migration set are legal under this rule and must NOT be renamed, because the
// applied file names are already recorded in production schema_migrations:
//   - 024_a_sftp_config.sql coexists with 024_recovery_tokens.sql (prefixes
//     "024_a" and "024" are distinct)
//   - 035_a_*, 035_b_* coexist with 035_compose_gitops.sql
//   - 039_webhooks.sql is live (shipped since the initial commit): there is
//     no 039 gap and the number must never be added to retiredMigrationPrefixes,
//     which would reject the live file. Retired numbers are 029-031, 061-076
//     and 166-169 only.
//   - Historical duplicates (shipped before validator was strict) are allowlisted:
//     015, 018, 020, 044, 054, 057, 080, 082, 083, 087 each have 2-3 bare files
//     that share the same numeric prefix but are already idempotently applied
//     in production. They are grandfathered; new migrations MUST use unique
//     prefixes with letter suffix (e.g., 015_a_*, 015_b_*) or bump to new number.
//
// In addition, full filenames must be unique (exact match, not just prefix),
// and retired numbers (029-031, 061-076, 166-169, ...) must never be reused.
func validateNoDuplicatePrefixes(files []string) error {
	// Historical duplicate prefixes that are already shipped and must remain grandfathered.
	// New files must NOT reuse these bare prefixes without letter suffix.
	allowedHistoricalDuplicates := map[string]bool{
		"015": true, "018": true, "020": true, "044": true, "054": true,
		"057": true, "080": true, "082": true, "083": true, "087": true,
	}
	seen := make(map[string]string)
	names := make(map[string]struct{})
	counts := make(map[string]int)
	for _, f := range files {
		if _, dup := names[f]; dup {
			return fmt.Errorf("duplicate migration filename %q: full filenames must be unique", f)
		}
		names[f] = struct{}{}
		prefix := migrationPrefix(f)
		// Numeric part of the prefix must not be a retired number.
		numPart := prefix
		if idx := strings.Index(prefix, "_"); idx >= 0 {
			numPart = prefix[:idx]
		}
		// Bare (unsuffixed) reuse of a retired number is forbidden; suffixed
		// variants (e.g. a hypothetical 061_a_) are also forbidden because the
		// number itself is retired. The exact legacy fragments kept as
		// read-only history in internal/store/migrations are grandfathered by
		// filename — they predate the retirement, they never execute, and the
		// rule exists to stop NEW files reusing the numbers.
		if reason, retired := retiredMigrationPrefixes[numPart]; retired {
			if _, grandfathered := grandfatheredRetiredFiles[f]; !grandfathered {
				return fmt.Errorf("migration %q reuses retired number %q (%s); allocate a new highest number instead", f, numPart, reason)
			}
		}
		if existing, ok := seen[prefix]; ok {
			// If this prefix is a known historical duplicate, allow it but count for audit.
			// Still error if a NEW file would make a 4th duplicate beyond known shipped count.
			if allowedHistoricalDuplicates[prefix] {
				counts[prefix]++
				// Allow up to known shipped count: 015:2, 018:2, 020:2, 044:2, 054:2, 057:3, 080:2, 082:3, 083:2, 087:2
				// If a new file pushes beyond that, error.
				knownMax := map[string]int{"015": 2, "018": 2, "020": 2, "044": 2, "054": 2, "057": 3, "080": 2, "082": 3, "083": 2, "087": 2}
				if counts[prefix] > knownMax[prefix] {
					return fmt.Errorf("duplicate migration prefix %q: %q and %q conflict; historical prefix %q already has %d files, new files must use letter suffix (e.g., %s_a_*) or bump to new number",
						prefix, existing, f, prefix, knownMax[prefix], prefix)
				}
				// Allow historical duplicate — keep first seen as anchor, don't overwrite.
				continue
			}
			return fmt.Errorf("duplicate migration prefix %q: %q and %q conflict; rename one file with a letter suffix (e.g., %s_a_*) or bump to a new number",
				prefix, existing, f, prefix)
		}
		seen[prefix] = f
		counts[prefix] = 1
	}
	return nil
}

// validateMigrationHashes reads every migration file and fails on two
// different filenames with byte-identical content, unless the pair is a
// registered rename alias in migrationAliases. Identical copies outside the
// alias table mean someone duplicated a migration instead of referencing it:
// the copy either re-applies DDL (wasteful but safe when idempotent) or, worse,
// diverges later while looking authoritative. Register the rename or delete
// the copy; do not ship silent duplicates.
func validateMigrationHashes(paths map[string]string) error {
	byHash := make(map[string]string)
	for name, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("hash migration %s: %w", name, err)
		}
		sum := sha256.Sum256(normalizeMigrationBytes(data))
		digest := hex.EncodeToString(sum[:])
		if first, ok := byHash[digest]; ok {
			if !isRegisteredAlias(first, name) {
				return fmt.Errorf("migrations %q and %q are byte-identical but not a registered rename; register the pair in migrationAliases or remove the copy (never rename an applied file)",
					first, name)
			}
			continue
		}
		byHash[digest] = name
	}
	return nil
}

// normalizeMigrationBytes strips trailing whitespace per line so a copy that
// differs only in a trailing newline still counts as identical.
func normalizeMigrationBytes(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t\r")
	}
	return []byte(strings.Join(lines, "\n"))
}

func isRegisteredAlias(a, b string) bool {
	for _, pair := range migrationAliases {
		if (pair[0] == a && pair[1] == b) || (pair[0] == b && pair[1] == a) {
			return true
		}
	}
	return false
}

// singleLine collapses a statement to one line for log excerpts.
func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// The schema_migrations DDL below must stay column-compatible with the
// production runner in store.go (runMigrations), which keys the table on a
// "version" column holding the full migration filename. Both runners can then
// safely share the same table and history.
func getCreateMigrationTableSQL(dbType DatabaseType) string {
	switch dbType {
	case DatabaseMySQL, DatabaseMariaDB:
		return `CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`
	case DatabaseSQLite:
		return `CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`
	default:
		return `CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`
	}
}

func getRecordMigrationSQL(dbType DatabaseType) string {
	switch dbType {
	case DatabaseMySQL, DatabaseMariaDB:
		// INSERT IGNORE keeps alias backfills and guard re-records idempotent
		// on MySQL, where there is no ON CONFLICT clause.
		return `INSERT IGNORE INTO schema_migrations (version) VALUES (?)`
	default:
		// ON CONFLICT DO NOTHING keeps re-records (guard backfills racing a
		// concurrent runner, retried files after a MySQL-style partial apply)
		// idempotent instead of failing on the version primary key.
		return `INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT (version) DO NOTHING`
	}
}

func getListMigrationsSQL(dbType DatabaseType) string {
	return `SELECT version FROM schema_migrations ORDER BY version`
}
