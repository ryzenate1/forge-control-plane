package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// These tests pin the unified schema_migrations contract shared by the
// MigrationRunner in this package and the production runner in store.go
// (runMigrations): a single table keyed by a "version" column that stores the
// full migration file name.

func writeTestMigrations(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, sql := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(sql), 0o644); err != nil {
			t.Fatalf("write migration %s: %v", name, err)
		}
	}
}

func appliedVersions(t *testing.T, db DatabaseDriver) []string {
	t.Helper()
	rows, err := db.Query(context.Background(), getListMigrationsSQL(db.Type()))
	if err != nil {
		t.Fatalf("list applied migrations: %v", err)
	}
	defer rows.Close()
	var versions []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan version: %v", err)
		}
		versions = append(versions, v)
	}
	return versions
}

func TestMigrationRunner_RecordsFullFileNameInVersionColumn(t *testing.T) {
	db, cleanup := createDisposableDatabase(t, DatabaseSQLite)
	defer cleanup()

	dir := t.TempDir()
	writeTestMigrations(t, dir, map[string]string{
		"001_first.sql":  "CREATE TABLE t_one (id TEXT PRIMARY KEY);",
		"002_second.sql": "CREATE TABLE t_two (id TEXT PRIMARY KEY);",
	})

	runner := NewMigrationRunner(db, dir)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	versions := appliedVersions(t, db)
	want := []string{"001_first.sql", "002_second.sql"}
	if len(versions) != len(want) {
		t.Fatalf("applied versions = %v, want %v", versions, want)
	}
	for i, v := range want {
		if versions[i] != v {
			t.Errorf("versions[%d] = %q, want %q (must be full file name for store.go interop)", i, versions[i], v)
		}
	}
}

func TestMigrationRunner_IsIdempotent(t *testing.T) {
	db, cleanup := createDisposableDatabase(t, DatabaseSQLite)
	defer cleanup()

	dir := t.TempDir()
	writeTestMigrations(t, dir, map[string]string{
		"001_first.sql": "CREATE TABLE t_idem (id TEXT PRIMARY KEY);",
	})

	runner := NewMigrationRunner(db, dir)
	for i := 0; i < 3; i++ {
		if err := runner.Run(context.Background()); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}

	versions := appliedVersions(t, db)
	if len(versions) != 1 || versions[0] != "001_first.sql" {
		t.Fatalf("expected exactly one applied migration, got %v", versions)
	}
}

func TestMigrationRunner_SkipsMigrationsRecordedByProductionRunner(t *testing.T) {
	db, cleanup := createDisposableDatabase(t, DatabaseSQLite)
	defer cleanup()

	dir := t.TempDir()
	// Deliberately invalid SQL: if the runner does not honor the existing
	// record (as written by store.go's runMigrations), the run will fail.
	writeTestMigrations(t, dir, map[string]string{
		"001_first.sql": "THIS IS NOT VALID SQL;",
	})

	ctx := context.Background()
	if _, err := db.Exec(ctx, getCreateMigrationTableSQL(db.Type())); err != nil {
		t.Fatalf("create table: %v", err)
	}
	// Same insert shape as the production runner: full file name as version.
	if _, err := db.Exec(ctx, getRecordMigrationSQL(db.Type()), "001_first.sql"); err != nil {
		t.Fatalf("pre-record migration: %v", err)
	}

	runner := NewMigrationRunner(db, dir)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("run should skip pre-recorded migration, got: %v", err)
	}
}

func TestMigrationRunner_RejectsDuplicatePrefixes(t *testing.T) {
	db, cleanup := createDisposableDatabase(t, DatabaseSQLite)
	defer cleanup()

	dir := t.TempDir()
	writeTestMigrations(t, dir, map[string]string{
		"010_one.sql": "CREATE TABLE t_a (id TEXT PRIMARY KEY);",
		"010_two.sql": "CREATE TABLE t_b (id TEXT PRIMARY KEY);",
	})

	runner := NewMigrationRunner(db, dir)
	if err := runner.Run(context.Background()); err == nil {
		t.Fatal("expected duplicate prefix error, got nil")
	}
}
