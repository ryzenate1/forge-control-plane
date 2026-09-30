package store

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
)

// requirePostgresEnvVar, when set to "1", turns every "PostgreSQL is not
// reachable" skip in this package into a hard failure.
//
// It exists because the opposite was the status quo: CI ran the migration suite
// with no PostgreSQL service, every PostgreSQL subtest called t.Skipf, and the
// job reported green — so the production migration stream was never applied to
// a real PostgreSQL by any job that ran. A skip on a green build is the same
// class of problem AGENTS.md forbids in product code ("never report success for
// work not performed"), one level up.
//
// The .github/workflows/ci.yml forge-api-integration job sets it. A developer
// with no local PostgreSQL still gets skips.
const requirePostgresEnvVar = "FORGE_TEST_REQUIRE_POSTGRES"

func postgresRequired() bool {
	return os.Getenv(requirePostgresEnvVar) == "1"
}

// skipOrFailWithoutPostgres skips when PostgreSQL is merely absent from a
// developer machine, and fails when the environment has declared that
// PostgreSQL must be present. Callers must return immediately after calling it:
// on the skip path it does not stop the calling goroutine until t.Skipf runs,
// and both branches leave the caller with nothing usable.
func skipOrFailWithoutPostgres(t *testing.T, format string, args ...any) {
	t.Helper()
	if postgresRequired() {
		t.Fatalf("%s=1 but PostgreSQL is unusable: "+format,
			append([]any{requirePostgresEnvVar}, args...)...)
	}
	t.Skipf("Skipping PostgreSQL test (set %s=1 to make this a failure): "+format,
		append([]any{requirePostgresEnvVar}, args...)...)
}

// postgresTestConfig describes the PostgreSQL instance to test against. The
// coordinates come from the standard libpq environment variables so the same
// tests can run against a CI service container, a Homebrew instance, or the
// dev stack, instead of being pinned to localhost:5432/postgres:postgres as
// they were.
func postgresTestConfig(dbName string) DBConfig {
	return DBConfig{
		Type:     DatabasePostgres,
		Host:     envDefault("PGHOST", "localhost"),
		Port:     envDefaultInt("PGPORT", 5432),
		User:     envDefault("PGUSER", "postgres"),
		Password: envDefault("PGPASSWORD", "postgres"),
		Database: dbName,
		SSLMode:  envDefault("PGSSLMODE", "disable"),
	}
}

// postgresAdminDSN points at the maintenance database ("postgres"), which is
// where CREATE DATABASE / DROP DATABASE for a disposable test database must be
// issued from. Credentials are percent-encoded for the same reason DBConfig.DSN
// encodes them: a password containing @, :, / or ? otherwise corrupts the URL.
func postgresAdminDSN(cfg DBConfig) string {
	admin := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.User, cfg.Password),
		Host:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Path:     "/postgres",
		RawQuery: "sslmode=" + url.QueryEscape(cfg.SSLMode),
	}
	return admin.String()
}

func envDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envDefaultInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
