package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSortMigrationFiles_NumericThenSuffix(t *testing.T) {
	files := []string{
		"100_team_tenancy.sql",
		"082_failover.sql",
		"082_b_target_groups.sql",
		"082_a_failover.sql",
		"082_deployments.sql",
		"009_schedule_run_history.sql",
	}
	sortMigrationFiles(files)
	want := []string{
		"009_schedule_run_history.sql",
		"082_deployments.sql",
		"082_failover.sql",
		"082_a_failover.sql",
		"082_b_target_groups.sql",
		"100_team_tenancy.sql",
	}
	for i, w := range want {
		if files[i] != w {
			t.Fatalf("sorted[%d] = %q, want %q (full order %v)", i, files[i], w, files)
		}
	}
}

func TestValidateNoDuplicatePrefixes_RetiredNumbers(t *testing.T) {
	for _, f := range []string{
		"029_new_thing.sql", "061_new_thing.sql", "076_new_thing.sql",
		"166_new_thing.sql", "169_new_thing.sql",
	} {
		if err := validateNoDuplicatePrefixes([]string{"001_init.sql", f}); err == nil {
			t.Errorf("expected retired-number error for %q, got nil", f)
		}
	}
	// Non-retired numbers still validate.
	if err := validateNoDuplicatePrefixes([]string{"001_init.sql", "234_new_thing.sql"}); err != nil {
		t.Errorf("expected no error for fresh number, got: %v", err)
	}
}

func TestValidateNoDuplicatePrefixes_FullFilenameUniqueness(t *testing.T) {
	err := validateNoDuplicatePrefixes([]string{"001_init.sql", "001_init.sql"})
	if err == nil {
		t.Fatal("expected error for duplicated full filename, got nil")
	}
}

func TestValidateMigrationHashes_AllowsRegisteredAliases(t *testing.T) {
	dir := t.TempDir()
	// Registered alias pair with identical content must pass.
	for _, name := range []string{"015_mounts.sql", "015_a_mounts.sql"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("SELECT 1;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	paths := map[string]string{
		"015_mounts.sql":   filepath.Join(dir, "015_mounts.sql"),
		"015_a_mounts.sql": filepath.Join(dir, "015_a_mounts.sql"),
	}
	if err := validateMigrationHashes(paths); err != nil {
		t.Fatalf("registered alias pair should pass hash check, got: %v", err)
	}
}

func TestValidateMigrationHashes_RejectsUnregisteredDuplicates(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"201_one.sql", "202_two.sql"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("SELECT 1;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	paths := map[string]string{
		"201_one.sql": filepath.Join(dir, "201_one.sql"),
		"202_two.sql": filepath.Join(dir, "202_two.sql"),
	}
	if err := validateMigrationHashes(paths); err == nil {
		t.Fatal("expected error for unregistered identical copies, got nil")
	}
}

func TestMigrationRunner_GuardSkippedWhenCanonicalApplied(t *testing.T) {
	db, cleanup := createDisposableDatabase(t, DatabaseSQLite)
	defer cleanup()
	ctx := context.Background()

	dir := t.TempDir()
	writeTestMigrations(t, dir, map[string]string{
		// Guard would fail if executed; canonical is pre-recorded.
		"015_mounts.sql": "THIS IS NOT VALID SQL;",
	})

	if _, err := db.Exec(ctx, getCreateMigrationTableSQL(db.Type())); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ('015_a_mounts.sql')`); err != nil {
		t.Fatalf("pre-record canonical: %v", err)
	}

	runner := NewMigrationRunner(db, dir)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("run should skip guard via canonical without executing it, got: %v", err)
	}

	versions := appliedVersions(t, db)
	want := map[string]bool{"015_a_mounts.sql": true, "015_mounts.sql": true}
	for _, v := range versions {
		delete(want, v)
	}
	if len(want) != 0 {
		t.Fatalf("expected both guard and canonical recorded, missing %v (got %v)", want, versions)
	}
}

// TestMigrationRunner_CanonicalExecutesDespiteGuardRow is the regression test
// for silently dropped schema: a guard row (no-op SELECT) must NEVER suppress
// its canonical. Only the canonical's own row suppresses it.
func TestMigrationRunner_CanonicalExecutesDespiteGuardRow(t *testing.T) {
	db, cleanup := createDisposableDatabase(t, DatabaseSQLite)
	defer cleanup()
	ctx := context.Background()

	dir := t.TempDir()
	writeTestMigrations(t, dir, map[string]string{
		"015_a_mounts.sql": "CREATE TABLE alias_canonical_probe (id TEXT PRIMARY KEY);",
		"015_mounts.sql":   "SELECT 1;",
	})

	if _, err := db.Exec(ctx, getCreateMigrationTableSQL(db.Type())); err != nil {
		t.Fatalf("create table: %v", err)
	}
	// Old-host style history: only the guard filename recorded.
	if _, err := db.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ('015_mounts.sql')`); err != nil {
		t.Fatalf("pre-record guard: %v", err)
	}

	runner := NewMigrationRunner(db, dir)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("run must execute canonical despite guard row, got: %v", err)
	}

	var name string
	err := db.QueryRow(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='alias_canonical_probe'`).Scan(&name)
	// QueryRow on DatabaseDriver returns *sql.Row; Scan errors when missing.
	if err != nil {
		t.Fatalf("canonical DDL was skipped despite guard-only history: %v", err)
	}
}

func TestMigrationRunner_GuardBackfilledWhenCanonicalRecorded(t *testing.T) {
	db, cleanup := createDisposableDatabase(t, DatabaseSQLite)
	defer cleanup()
	ctx := context.Background()

	dir := t.TempDir()
	writeTestMigrations(t, dir, map[string]string{
		"015_mounts.sql": "THIS IS NOT VALID SQL;",
	})

	if _, err := db.Exec(ctx, getCreateMigrationTableSQL(db.Type())); err != nil {
		t.Fatalf("create table: %v", err)
	}
	// Only the canonical side is on disk; the guard (old side) is missing
	// from this dir, so record the canonical and ensure the run succeeds.
	if _, err := db.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ('015_a_mounts.sql')`); err != nil {
		t.Fatalf("pre-record canonical: %v", err)
	}

	runner := NewMigrationRunner(db, dir)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("run should backfill guard via alias without executing it, got: %v", err)
	}
	versions := appliedVersions(t, db)
	found := false
	for _, v := range versions {
		if v == "015_mounts.sql" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected backfilled 015_mounts.sql, got %v", versions)
	}
}

func TestGetRunMigrationIDs_ReturnsError(t *testing.T) {
	db, cleanup := createDisposableDatabase(t, DatabaseSQLite)
	defer cleanup()
	ctx := context.Background()

	runner := NewMigrationRunner(db, t.TempDir())
	// schema_migrations does not exist yet: must return an error, never a
	// silent empty map (unknown is not zero).
	if _, err := runner.getRunMigrationIDs(ctx); err == nil {
		t.Fatal("expected error when migration table is missing, got nil")
	}

	if _, err := db.Exec(ctx, getCreateMigrationTableSQL(db.Type())); err != nil {
		t.Fatal(err)
	}
	ids, err := runner.getRunMigrationIDs(ctx)
	if err != nil {
		t.Fatalf("expected no error on empty table, got: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected empty set, got %v", ids)
	}
}

func TestMySQLCompatibleMigration_Basics(t *testing.T) {
	out := mysqlCompatibleMigration("CREATE TABLE t (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), created_at TIMESTAMPTZ NOT NULL DEFAULT now());")
	if strings.Contains(out, "gen_random_uuid()") {
		t.Errorf("expected gen_random_uuid rewritten, got: %s", out)
	}
	if !strings.Contains(out, "UUID()") {
		t.Errorf("expected UUID() default, got: %s", out)
	}
	if strings.Contains(out, "TIMESTAMPTZ") {
		t.Errorf("expected TIMESTAMPTZ rewritten, got: %s", out)
	}
}

func TestSQLiteCompatibleMigration_StripsCastsOutsideLiteralsOnly(t *testing.T) {
	// A :: cast inside a string literal is data and must survive; a real
	// cast outside must be stripped.
	sql := `SELECT '{}'::jsonb, '{}::notacast', col::text FROM t;`
	out, err := sqliteCompatibleMigration(sql)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "'{}::notacast'") {
		t.Errorf("cast inside literal must survive, got: %s", out)
	}
	if strings.Contains(out, "col::text") {
		t.Errorf("cast outside literal must be stripped, got: %s", out)
	}
}

func TestSQLiteUntranslatableDDL_FailsLoudly(t *testing.T) {
	for _, stmt := range []string{
		"CREATE TRIGGER x BEFORE INSERT ON t FOR EACH ROW EXECUTE FUNCTION f();",
		"DROP TRIGGER IF EXISTS x ON t;",
		"CREATE INDEX i ON t USING gin(to_tsvector('english', name));",
		"UPDATE t SET j = (SELECT jsonb_object_agg(k, v) FROM u);",
	} {
		if got := sqliteUntranslatableDDL(stmt); got == "" {
			t.Errorf("expected loud failure for %q, got empty", stmt)
		}
	}
	// SQLite-native triggers in dialect overrides must pass through.
	native := "CREATE TRIGGER t_upd AFTER UPDATE ON t BEGIN UPDATE t SET u = 1 WHERE id = NEW.id; END;"
	if got := sqliteUntranslatableDDL(native); got != "" {
		t.Errorf("native sqlite trigger must not fail, got %q", got)
	}
	if got := sqliteUntranslatableDDL("DROP TRIGGER IF EXISTS t_upd;"); got != "" {
		t.Errorf("native sqlite DROP TRIGGER must not fail, got %q", got)
	}
}

func TestSplitSQLStatements_TriggerBody(t *testing.T) {
	sql := "CREATE TRIGGER t_upd AFTER UPDATE ON t BEGIN UPDATE t SET u = 1 WHERE id = NEW.id; UPDATE t SET v = 2 WHERE id = OLD.id; END;\nCREATE INDEX i ON t (u);"
	stmts := splitSQLStatements(sql)
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements (trigger + index), got %d: %v", len(stmts), stmts)
	}
	if !strings.Contains(stmts[0], "END") {
		t.Errorf("first statement should be the whole trigger, got: %q", stmts[0])
	}
}

func TestSplitSQLiteAlterAdd_MultiColumnAnyTable(t *testing.T) {
	stmt := "ALTER TABLE whatever\n    ADD COLUMN IF NOT EXISTS a TEXT,\n    ADD COLUMN IF NOT EXISTS b TEXT DEFAULT 'x,y';"
	parts := splitSQLiteAlterAdd(stmt)
	if len(parts) != 2 {
		t.Fatalf("expected 2 one-column ALTERs, got %d: %v", len(parts), parts)
	}
	for _, p := range parts {
		if strings.Count(strings.ToUpper(p), "ADD COLUMN") != 1 {
			t.Errorf("each part must hold exactly one ADD COLUMN, got: %q", p)
		}
	}
	if !strings.Contains(parts[1], "'x,y'") {
		t.Errorf("quoted comma default must survive the split, got: %q", parts[1])
	}
}

func TestFKFollowupsExist(t *testing.T) {
	for table, file := range fkFollowupMigrations {
		if file == "" {
			t.Logf("tracked gap (no followup yet): %s", table)
			continue
		}
		path := filepath.Join("..", "..", "migrations", file)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("FK followup for %s missing on disk: %s: %v", table, file, err)
		}
	}
}

func TestIsNonReversibleRollback(t *testing.T) {
	if !isNonReversibleRollback([]byte("-- non-reversible: drops data\nALTER TABLE t DROP COLUMN c;\n")) {
		t.Error("expected marker detected")
	}
	if isNonReversibleRollback([]byte("ALTER TABLE t DROP COLUMN c;\n")) {
		t.Error("plain rollback must not be flagged")
	}
	// A bare mention inside a data statement is not the marker convention.
	if isNonReversibleRollback([]byte("SELECT '-- non-reversible';\n")) {
		t.Error("marker inside a statement must not count; only leading comment lines")
	}
}
