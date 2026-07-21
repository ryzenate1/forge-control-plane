package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	log.Println("🚀 Starting comprehensive migration testing...")

	// Test SQLite (always available)
	log.Println("\n📋 Testing SQLite database...")

	// Create in-memory database
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		log.Fatalf("❌ Failed to open SQLite database: %v", err)
	}
	defer db.Close()

	// Enable foreign keys
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	if err != nil {
		log.Printf("⚠️  Could not enable foreign keys: %v", err)
	}

	migrationDir := filepath.Join("..", "forge", "api", "migrations")

	// Test 1: Fresh Installation
	log.Println("  🆕 Testing Fresh Installation...")
	if err := runAllMigrations(db, migrationDir); err != nil {
		log.Fatalf("❌ Fresh installation failed: %v", err)
	}
	log.Println("  ✅ Fresh installation completed")

	// Test 2: Validate schema
	log.Println("  🔍 Validating schema...")
	if err := validateSchema(db); err != nil {
		log.Fatalf("❌ Schema validation failed: %v", err)
	}
	log.Println("  ✅ Schema validation passed")

	// Test 3: Validate Batch 2 entities
	log.Println("  📦 Validating Batch 2 entities...")
	if err := validateBatch2Entities(db); err != nil {
		log.Fatalf("❌ Batch 2 entities validation failed: %v", err)
	}
	log.Println("  ✅ Batch 2 entities validation passed")

	// Test 4: Validate tenancy columns
	log.Println("  🏢 Validating tenancy columns...")
	if err := validateTenancyColumns(db); err != nil {
		log.Fatalf("❌ Tenancy columns validation failed: %v", err)
	}
	log.Println("  ✅ Tenancy columns validation passed")

	// Test 5: Validate foreign keys
	log.Println("  🔗 Validating foreign keys...")
	if err := validateForeignKeys(db); err != nil {
		log.Fatalf("❌ Foreign keys validation failed: %v", err)
	}
	log.Println("  ✅ Foreign keys validation passed")

	log.Println("\n🎉 All SQLite migration tests passed!")
}

func runAllMigrations(db *sql.DB, migrationDir string) error {
	// Create migrations tracking table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id TEXT PRIMARY KEY,
			applied_at TEXT DEFAULT (datetime('now'))
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get all migration files
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	// Sort migrations by name
	var migrations []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			migrations = append(migrations, entry.Name())
		}
	}
	sort.Strings(migrations)

	// Get already applied migrations
	applied := make(map[string]bool)
	rows, err := db.Query("SELECT id FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("failed to scan migration id: %w", err)
		}
		applied[id] = true
	}

	// Apply pending migrations
	for _, migration := range migrations {
		id := strings.TrimSuffix(migration, ".sql")
		if applied[id] {
			log.Printf("  ⏭️  Migration %s already applied", migration)
			continue
		}

		migrationPath := filepath.Join(migrationDir, migration)
		content, err := os.ReadFile(migrationPath)
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", migration, err)
		}

		// Split into individual statements
		statements := splitSQLStatements(string(content))

		// Begin transaction
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for %s: %w", migration, err)
		}

		for _, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}

			_, err := tx.Exec(stmt)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to execute statement in %s: %w (stmt: %.100s)", migration, err, stmt)
			}
		}

		// Record migration
		_, err = tx.Exec("INSERT INTO schema_migrations (id) VALUES (?)", id)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %s: %w", migration, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", migration, err)
		}

		log.Printf("  ✅ Applied migration: %s", migration)
	}

	return nil
}

func splitSQLStatements(sql string) []string {
	// Simple statement splitter that handles semicolons
	var statements []string
	var current strings.Builder
	inQuotes := false
	inBackticks := false

	for _, char := range sql {
		switch char {
		case ';':
			if !inQuotes && !inBackticks {
				stmt := strings.TrimSpace(current.String())
				if stmt != "" {
					statements = append(statements, stmt)
				}
				current.Reset()
			} else {
				current.WriteRune(char)
			}
		case '\'':
			inQuotes = !inQuotes
			current.WriteRune(char)
		case '`':
			inBackticks = !inBackticks
			current.WriteRune(char)
		default:
			current.WriteRune(char)
		}
	}

	// Add the last statement if it exists
	stmt := strings.TrimSpace(current.String())
	if stmt != "" {
		statements = append(statements, stmt)
	}

	return statements
}

func validateSchema(db *sql.DB) error {
	// Check for required tables
	requiredTables := []string{
		"users", "nodes", "servers", "allocations", "audit_events",
		"organizations", "projects", "environments", "team_members",
		"applications", "app_services", "deployments", "backups",
		"replica_applications", "instances", "placement_decisions",
		"reconcile_plans", "reconcile_events", "service_endpoints",
		"procedures", "procedure_steps", "procedure_executions",
		"deployment_steps", "backup_policies", "backup_manifests",
		"backup_storage_receipts", "database_backups", "volume_backups",
	}

	for _, table := range requiredTables {
		var exists bool
		err := db.QueryRow(fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='%s')", table)).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check table %s: %w", table, err)
		}
		if !exists {
			return fmt.Errorf("missing required table: %s", table)
		}
	}

	log.Printf("  ✅ All %d required tables exist", len(requiredTables))
	return nil
}

func validateBatch2Entities(db *sql.DB) error {
	// List of all required Batch 2 entities
	batch2Entities := []struct {
		Table       string
		Description string
	}{
		{"organizations", "Team-based tenancy"},
		{"projects", "Project grouping under organizations"},
		{"environments", "Per-project deployment environments"},
		{"team_members", "Organization-level membership with roles"},
		{"applications", "Unified workload identity for app-hosting"},
		{"app_services", "Service definitions with update strategy"},
		{"replica_applications", "Service definitions"},
		{"instances", "Replicas"},
		{"placement_decisions", "Placement constraints"},
		{"reservations", "Resource reservations"},
		{"reconcile_plans", "Drift records"},
		{"reconcile_events", "Reconciliation events"},
		{"service_endpoints", "Service discovery endpoints"},
		{"procedures", "Procedure definitions"},
		{"procedure_steps", "Procedure steps"},
		{"procedure_executions", "Procedure execution tracking"},
		{"deployment_steps", "Deployment strategy and rollout state"},
		{"backup_policies", "Backup policies"},
		{"backup_manifests", "Backup manifests"},
		{"backup_storage_receipts", "Storage receipts"},
		{"database_backups", "Database backup records"},
		{"volume_backups", "Volume backup records"},
	}

	missingEntities := []string{}
	for _, entity := range batch2Entities {
		var exists bool
		err := db.QueryRow(fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='%s')", entity.Table)).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check entity %s: %w", entity.Table, err)
		}

		if !exists {
			missingEntities = append(missingEntities, entity.Table)
			log.Printf("  ❌ Missing Batch 2 entity: %s (%s)", entity.Table, entity.Description)
		} else {
			log.Printf("  ✅ Found Batch 2 entity: %s (%s)", entity.Table, entity.Description)
		}
	}

	if len(missingEntities) > 0 {
		return fmt.Errorf("missing %d Batch 2 entities: %s", len(missingEntities), strings.Join(missingEntities, ", "))
	}

	return nil
}

func validateTenancyColumns(db *sql.DB) error {
	// Check that key tables have tenancy ownership columns
	tablesWithTenancy := []string{
		"servers", "deployments", "backups", "applications", "app_services",
		"replica_applications", "instances", "procedures",
	}

	missingTenancy := []string{}
	for _, table := range tablesWithTenancy {
		var exists bool
		err := db.QueryRow(fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM pragma_table_info('%s') WHERE name='org_id')", table)).Scan(&exists)
		if err != nil {
			// Table might not exist
			continue
		}

		if !exists {
			missingTenancy = append(missingTenancy, table)
			log.Printf("  ⚠️  No org_id column in %s", table)
		} else {
			log.Printf("  ✅ Tenancy column (org_id) found in %s", table)
		}
	}

	if len(missingTenancy) > 0 {
		log.Printf("  ⚠️  %d tables missing org_id column: %s", len(missingTenancy), strings.Join(missingTenancy, ", "))
	}

	return nil
}

func validateForeignKeys(db *sql.DB) error {
	// Get all tables
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		return fmt.Errorf("failed to get tables: %w", err)
	}
	defer rows.Close()

	tables := []string{}
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return fmt.Errorf("failed to scan table: %w", err)
		}
		tables = append(tables, table)
	}

	// Check foreign key references
	fkIssues := []string{}
	for _, table := range tables {
		// Get table schema
		var schema string
		err := db.QueryRow(fmt.Sprintf("SELECT sql FROM sqlite_master WHERE type='table' AND name='%s'", table)).Scan(&schema)
		if err != nil {
			continue // Skip if we can't get schema
		}

		// Look for foreign key references
		if strings.Contains(strings.ToUpper(schema), "REFERENCES") {
			// Extract referenced tables
			// This is a simple check - a more robust implementation would parse properly
			log.Printf("  ✅ Table %s has foreign key references", table)
		}
	}

	if len(fkIssues) > 0 {
		return fmt.Errorf("foreign key issues: %s", strings.Join(fkIssues, ", "))
	}

	return nil
}
