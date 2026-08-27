package main

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func runRiverMigrations(ctx context.Context, pool *pgxpool.Pool, migrationFS fs.FS, schema string) error {
	if schema == "" {
		schema = "public"
	}

	// Validate schema name to prevent SQL injection. Only allow alphanumeric
	// characters and underscores, which covers all valid PostgreSQL identifiers.
	for _, c := range schema {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return fmt.Errorf("invalid schema name %q: must contain only alphanumeric characters and underscores", schema)
		}
	}

	var schemaPrefix string
	if schema != "public" {
		schemaPrefix = schema + "."
	}

	entries, err := fs.ReadDir(migrationFS, ".")
	if err != nil {
		return fmt.Errorf("read migration dir: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	if len(names) == 0 {
		return nil
	}

	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS river_schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("create river_schema_migrations: %w", err)
	}

	for _, name := range names {
		var applied bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM river_schema_migrations WHERE version = $1)`, name).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if applied {
			continue
		}

		body, err := fs.ReadFile(migrationFS, name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		sql := string(body)
		sql = strings.ReplaceAll(sql, "/* TEMPLATE: schema */", schemaPrefix)

		if _, err := pool.Exec(ctx, sql); err != nil {
			return fmt.Errorf("execute migration %s: %w", name, err)
		}

		if _, err := pool.Exec(ctx, `INSERT INTO river_schema_migrations (version) VALUES ($1)`, name); err != nil {
			return fmt.Errorf("record migration %s: %w", name, err)
		}
	}

	return nil
}