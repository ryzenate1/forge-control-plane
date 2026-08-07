package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// TestProductionRunnerAgainstPostgres applies the real migration stream with
// Store.RunMigrations — the runner that actually ships, entered from
// app.Container.InitDB.
//
// Every other migration test in this package drives MigrationRunner
// (internal/store/migration.go), which is referenced only from _test.go files.
// So the runner that was covered was not the one deployed, and the one deployed
// was not covered. This test closes that gap; it needs PostgreSQL because
// Store is pgxpool-only.
func TestProductionRunnerAgainstPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping PostgreSQL migration test in short mode")
	}

	dsn, migrationsDir, cleanup := newDisposablePostgres(t)
	defer cleanup()

	ctx := context.Background()
	st, err := Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer st.Close()

	onDisk := countMigrationFiles(t, migrationsDir)

	// Fresh install.
	if err := st.RunMigrations(ctx, migrationsDir); err != nil {
		t.Fatalf("RunMigrations (fresh): %v", err)
	}

	recorded := countAppliedMigrations(ctx, t, st)
	if recorded != onDisk {
		t.Errorf("schema_migrations has %d rows, want %d (one per .sql file)", recorded, onDisk)
	}
	if integrity := st.MigrationIntegrity(); len(integrity.Drift) != 0 {
		t.Errorf("fresh install reported %d drifted migrations, want 0: %+v",
			len(integrity.Drift), integrity.Drift)
	}

	// Re-running must be a no-op, not an error and not a re-apply.
	if err := st.RunMigrations(ctx, migrationsDir); err != nil {
		t.Fatalf("RunMigrations (second run): %v", err)
	}
	if again := countAppliedMigrations(ctx, t, st); again != recorded {
		t.Errorf("second run changed schema_migrations from %d to %d rows", recorded, again)
	}

	// Every row applied by this run carries a checksum, so the second run has
	// something to verify. Rows predating the checksum column are the only
	// legitimate source of Unverified, and a fresh database has none.
	if integrity := st.MigrationIntegrity(); integrity.Unverified != 0 {
		t.Errorf("second run found %d unverifiable migrations on a database it populated itself, want 0",
			integrity.Unverified)
	}
}

// TestProductionRunnerDetectsDrift proves the checksum column does what it was
// added for: an already-applied migration whose file changed is reported rather
// than silently skipped forever. Filenames are primary keys in
// schema_migrations, so before this there was no mechanism that could notice.
func TestProductionRunnerDetectsDrift(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping PostgreSQL migration test in short mode")
	}

	dsn, migrationsDir, cleanup := newDisposablePostgres(t)
	defer cleanup()

	ctx := context.Background()
	st, err := Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer st.Close()

	if err := st.RunMigrations(ctx, migrationsDir); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Simulate an edit to an applied migration by corrupting its recorded
	// checksum. Editing the repository's own migration file from a test would
	// be the more faithful simulation and a far worse idea.
	var victim string
	if err := st.DB().QueryRow(ctx,
		`SELECT version FROM schema_migrations WHERE checksum IS NOT NULL ORDER BY version LIMIT 1`,
	).Scan(&victim); err != nil {
		t.Fatalf("pick a migration to tamper with: %v", err)
	}
	if _, err := st.DB().Exec(ctx,
		`UPDATE schema_migrations SET checksum = $1 WHERE version = $2`,
		strings.Repeat("0", 64), victim,
	); err != nil {
		t.Fatalf("tamper with checksum: %v", err)
	}

	if err := st.RunMigrations(ctx, migrationsDir); err != nil {
		t.Fatalf("RunMigrations after tamper: %v", err)
	}

	integrity := st.MigrationIntegrity()
	if len(integrity.Drift) != 1 {
		t.Fatalf("got %d drifted migrations, want exactly 1: %+v", len(integrity.Drift), integrity.Drift)
	}
	if integrity.Drift[0].Version != victim {
		t.Errorf("drift reported for %q, want %q", integrity.Drift[0].Version, victim)
	}
	if integrity.Drift[0].OnDisk == integrity.Drift[0].Applied {
		t.Errorf("drift record has matching checksums %q; nothing would be reported",
			integrity.Drift[0].OnDisk)
	}
}

// newDisposablePostgres creates an empty database and returns a DSN for it, the
// migrations directory, and a cleanup that drops it.
func newDisposablePostgres(t *testing.T) (dsn string, migrationsDir string, cleanup func()) {
	t.Helper()

	cfg := postgresTestConfig(fmt.Sprintf("forge_prodrunner_%d", time.Now().UnixNano()))
	adminDSN := postgresAdminDSN(cfg)

	admin, err := sql.Open("postgres", adminDSN)
	if err != nil {
		skipOrFailWithoutPostgres(t, "open admin connection: %v", err)
		return "", "", func() {}
	}
	if err := admin.PingContext(context.Background()); err != nil {
		admin.Close()
		skipOrFailWithoutPostgres(t, "ping %s:%d: %v", cfg.Host, cfg.Port, err)
		return "", "", func() {}
	}
	if _, err := admin.Exec(fmt.Sprintf("CREATE DATABASE %s", cfg.Database)); err != nil {
		admin.Close()
		skipOrFailWithoutPostgres(t, "create database %s: %v", cfg.Database, err)
		return "", "", func() {}
	}

	return cfg.DSN(), getMigrationDirectory(DatabasePostgres), func() {
		defer admin.Close()
		if _, err := admin.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", cfg.Database)); err != nil {
			t.Logf("could not drop test database %s: %v", cfg.Database, err)
		}
	}
}

func countMigrationFiles(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations dir %s: %v", dir, err)
	}
	n := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			n++
		}
	}
	if n == 0 {
		t.Fatalf("no .sql files in %s; the test would prove nothing", filepath.Clean(dir))
	}
	return n
}

func countAppliedMigrations(ctx context.Context, t *testing.T, st *Store) int {
	t.Helper()
	var n int
	if err := st.DB().QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	return n
}
