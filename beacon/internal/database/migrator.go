package database

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"sort"
	"strings"
)

type Migrator struct {
	db     Database
	dir    string
	logger *log.Logger
}

func NewMigrator(db Database, dir string, logger *log.Logger) *Migrator {
	return &Migrator{
		db:     db,
		dir:    dir,
		logger: logger,
	}
}

func (m *Migrator) Migrate(ctx context.Context) error {
	// Create migrations table if it doesn't exist
	_, err := m.db.Exec(ctx,
		`CREATE TABLE IF NOT EXISTS migrations (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL UNIQUE,
            applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
        );`)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get list of migration files
	files, err := ioutil.ReadDir(m.dir)
	if err != nil {
		return fmt.Errorf("failed to read migration directory: %w", err)
	}

	// Sort files by name
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})

	// Get already applied migrations
	rows, err := m.db.Query(ctx, "SELECT name FROM migrations")
	if err != nil {
		return fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("failed to scan applied migration: %w", err)
		}
		applied[name] = true
	}

	// Apply pending migrations
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".sql") {
			continue
		}

		if applied[file.Name()] {
			m.logger.Printf("Migration %s already applied, skipping", file.Name())
			continue
		}

		m.logger.Printf("Applying migration %s", file.Name())

		// Read migration file
		content, err := ioutil.ReadFile(filepath.Join(m.dir, file.Name()))
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", file.Name(), err)
		}

		// Execute migration
		tx, err := m.db.BeginTx(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}

		_, err = tx.ExecContext(ctx, string(content))
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("failed to execute migration %s and rollback failed: %v, original error: %w", file.Name(), rbErr, err)
			}
			return fmt.Errorf("failed to execute migration %s: %w", file.Name(), err)
		}

		// Record migration as applied
		_, err = tx.ExecContext(ctx, "INSERT INTO migrations (name) VALUES (?)", file.Name())
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("failed to record migration %s and rollback failed: %v, original error: %w", file.Name(), rbErr, err)
			}
			return fmt.Errorf("failed to record migration %s: %w", file.Name(), err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", file.Name(), err)
		}
	}

	m.logger.Println("Migrations applied successfully")
	return nil
}
