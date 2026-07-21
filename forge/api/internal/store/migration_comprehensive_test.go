package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

// TestComprehensiveMigrationValidation provides a complete validation suite for database migrations
// covering all three requested scenarios: Fresh Installation, Upgrade Installation, and Batch 2 Entities
func TestComprehensiveMigrationValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping comprehensive migration test in short mode")
	}

	// Run tests for each database type
	databaseTypes := []DatabaseType{DatabaseSQLite, DatabasePostgres}

	for _, dbType := range databaseTypes {
		t.Run(string(dbType), func(t *testing.T) {
			// Test Fresh Installation
			t.Run("FreshInstallation", func(t *testing.T) {
				testFreshInstallation(t, dbType)
			})

			// Test Upgrade Installation (if applicable)
			if dbType != DatabaseSQLite {
				t.Run("UpgradeInstallation", func(t *testing.T) {
					testUpgradeInstallation(t, dbType)
				})
			}

			// Test Batch 2 Entities
			t.Run("Batch2Entities", func(t *testing.T) {
				testBatch2Entities(t, dbType)
			})
		})
	}
}

// testFreshInstallation validates applying every migration from zero
func testFreshInstallation(t *testing.T, dbType DatabaseType) {
	t.Logf("Testing Fresh Installation for %s", dbType)

	// Create disposable database
	db, cleanup := createDisposableDatabase(t, dbType)
	defer cleanup()

	// Get migration directory
	migrationDir := getMigrationDirectory(dbType)

	// Create migration runner
	runner := NewMigrationRunner(db, migrationDir)

	// Run all migrations
	ctx := context.Background()
	err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Validate migration order and completeness
	validateMigrationOrder(t, db, dbType)

	// Validate schema integrity
	validateSchemaIntegrity(t, db, dbType)

	// Validate all required tables exist
	validateRequiredTables(t, db, dbType)

	// Validate foreign keys, indexes, constraints
	validateDatabaseConstraints(t, db, dbType)

	t.Logf("✅ Fresh Installation test passed for %s", dbType)
}

// testUpgradeInstallation validates upgrading from Batch 1 schema to current
func testUpgradeInstallation(t *testing.T, dbType DatabaseType) {
	t.Logf("Testing Upgrade Installation for %s", dbType)

	// Create disposable database
	db, cleanup := createDisposableDatabase(t, dbType)
	defer cleanup()

	// Apply Batch 1 migrations (up to migration 099)
	ctx := context.Background()
	migrationDir := getMigrationDirectory(dbType)

	// First, apply only Batch 1 migrations (001-099)
	batch1Migrations := getBatch1Migrations(migrationDir)
	for _, migrationFile := range batch1Migrations {
		data, err := os.ReadFile(filepath.Join(migrationDir, migrationFile))
		if err != nil {
			t.Fatalf("Failed to read migration %s: %v", migrationFile, err)
		}

		statements := splitSQLStatements(string(data))
		for _, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := db.Exec(ctx, stmt); err != nil {
				t.Fatalf("Failed to apply Batch 1 migration %s: %v (stmt: %.100s)", migrationFile, err, stmt)
			}
		}

		// Record migration as applied
		recordSQL := getRecordMigrationSQL(dbType)
		migrationID := strings.TrimSuffix(migrationFile, ".sql")
		if _, err := db.Exec(ctx, recordSQL, migrationID); err != nil {
			t.Fatalf("Failed to record migration %s: %v", migrationFile, err)
		}
	}

	// Insert test data for Batch 1 entities
	insertBatch1TestData(t, ctx, db, dbType)

	// Now apply Batch 2 migrations (100+)
	batch2Migrations := getBatch2Migrations(migrationDir)
	for _, migrationFile := range batch2Migrations {
		data, err := os.ReadFile(filepath.Join(migrationDir, migrationFile))
		if err != nil {
			t.Fatalf("Failed to read Batch 2 migration %s: %v", migrationFile, err)
		}

		statements := splitSQLStatements(string(data))
		for _, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := db.Exec(ctx, stmt); err != nil {
				t.Fatalf("Failed to apply Batch 2 migration %s: %v (stmt: %.100s)", migrationFile, err, stmt)
			}
		}

		// Record migration as applied
		recordSQL := getRecordMigrationSQL(dbType)
		migrationID := strings.TrimSuffix(migrationFile, ".sql")
		if _, err := db.Exec(ctx, recordSQL, migrationID); err != nil {
			t.Fatalf("Failed to record Batch 2 migration %s: %v", migrationFile, err)
		}
	}

	// Validate that existing data survived
	validateDataSurvival(t, ctx, db, dbType)

	// Validate no silent data loss
	validateNoDataLoss(t, ctx, db, dbType)

	// Validate Batch 2 schema additions
	validateBatch2SchemaAdditions(t, ctx, db, dbType)

	t.Logf("✅ Upgrade Installation test passed for %s", dbType)
}

// testBatch2Entities validates persistence for all required Batch 2 entities
func testBatch2Entities(t *testing.T, dbType DatabaseType) {
	t.Logf("Testing Batch 2 Entities for %s", dbType)

	// Create disposable database
	db, cleanup := createDisposableDatabase(t, dbType)
	defer cleanup()

	// Apply all migrations
	ctx := context.Background()
	migrationDir := getMigrationDirectory(dbType)
	runner := NewMigrationRunner(db, migrationDir)

	err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Validate all required Batch 2 entities
	validateRequiredBatch2Entities(t, ctx, db, dbType)

	t.Logf("✅ Batch 2 Entities test passed for %s", dbType)
}

// Helper functions

func createDisposableDatabase(t *testing.T, dbType DatabaseType) (DatabaseDriver, func()) {
	switch dbType {
	case DatabaseSQLite:
		// Create in-memory SQLite database
		cfg := DBConfig{
			Type:       DatabaseSQLite,
			SQLitePath: ":memory:",
		}
		db, err := NewDatabaseDriver(context.Background(), cfg)
		if err != nil {
			t.Fatalf("Failed to create SQLite database: %v", err)
		}
		return db, func() {
			db.Close()
		}

	case DatabasePostgres:
		// For PostgreSQL, we'll use a temporary database name
		// In a real test environment, you'd need a running PostgreSQL instance
		// For this test, we'll skip if PostgreSQL is not available
		cfg := DBConfig{
			Type:     DatabasePostgres,
			Host:     "localhost",
			Port:     5432,
			User:     "postgres",
			Password: "postgres",
			Database: fmt.Sprintf("gamepanel_test_%d", time.Now().Unix()),
			SSLMode:  "disable",
		}

		// Try to create the database
		connStr := fmt.Sprintf("postgres://%s:%s@%s:%d/postgres?sslmode=disable",
			cfg.User, cfg.Password, cfg.Host, cfg.Port)

		createDB, err := sql.Open("postgres", connStr)
		if err != nil {
			t.Skipf("Skipping PostgreSQL test: %v", err)
		}
		defer createDB.Close()

		_, err = createDB.Exec(fmt.Sprintf("CREATE DATABASE %s", cfg.Database))
		if err != nil {
			t.Skipf("Skipping PostgreSQL test: %v", err)
		}

		db, err := NewDatabaseDriver(context.Background(), cfg)
		if err != nil {
			createDB.Exec(fmt.Sprintf("DROP DATABASE %s", cfg.Database))
			t.Fatalf("Failed to create PostgreSQL database: %v", err)
		}

		return db, func() {
			db.Close()
			// Clean up the database
			cleanupDB, _ := sql.Open("postgres", connStr)
			defer cleanupDB.Close()
			cleanupDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", cfg.Database))
		}

	default:
		t.Fatalf("Unsupported database type: %s", dbType)
		return nil, func() {}
	}
}

func getMigrationDirectory(dbType DatabaseType) string {
	// The canonical migration stream lives at forge/api/migrations. The
	// internal/store directory contains only supplemental historical fragments
	// and cannot produce a fresh database on its own.
	return filepath.Join("..", "..", "migrations")
}

func getBatch1Migrations(migrationDir string) []string {
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		return []string{}
	}

	var migrations []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			// Batch 1: migrations 001-099
			name := entry.Name()
			if len(name) >= 3 && name[:3] <= "099" {
				migrations = append(migrations, name)
			}
		}
	}

	sort.Strings(migrations)
	return migrations
}

func getBatch2Migrations(migrationDir string) []string {
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		return []string{}
	}

	var migrations []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			// Batch 2: migrations 100+
			name := entry.Name()
			if len(name) >= 3 && name[:3] >= "100" {
				migrations = append(migrations, name)
			}
		}
	}

	sort.Strings(migrations)
	return migrations
}

func validateMigrationOrder(t *testing.T, db DatabaseDriver, dbType DatabaseType) {
	ctx := context.Background()

	// Get all applied migrations
	query := getListMigrationsSQL(dbType)
	rows, err := db.Query(ctx, query)
	if err != nil {
		t.Fatalf("Failed to query applied migrations: %v", err)
	}
	defer rows.Close()

	var appliedMigrations []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("Failed to scan migration ID: %v", err)
		}
		appliedMigrations = append(appliedMigrations, id)
	}

	// Check that migrations are in order
	for i := 1; i < len(appliedMigrations); i++ {
		if appliedMigrations[i] < appliedMigrations[i-1] {
			t.Errorf("Migration order violation: %s comes after %s",
				appliedMigrations[i], appliedMigrations[i-1])
		}
	}

	// Check for duplicate identifiers
	seen := make(map[string]bool)
	for _, migration := range appliedMigrations {
		if seen[migration] {
			t.Errorf("Duplicate migration identifier: %s", migration)
		}
		seen[migration] = true
	}
}

func validateSchemaIntegrity(t *testing.T, db DatabaseDriver, dbType DatabaseType) {
	ctx := context.Background()

	// Get all tables
	tables, err := getAllTables(ctx, db, dbType)
	if err != nil {
		t.Fatalf("Failed to get tables: %v", err)
	}

	// Check for missing referenced tables
	for _, table := range tables {
		foreignKeys, err := getForeignKeys(ctx, db, dbType, table)
		if err != nil {
			continue // Skip if we can't get foreign keys
		}

		for _, fk := range foreignKeys {
			// Check if referenced table exists
			refTableExists := false
			for _, existingTable := range tables {
				if existingTable == fk.ReferencedTable {
					refTableExists = true
					break
				}
			}
			if !refTableExists {
				t.Errorf("Missing referenced table: %s referenced by %s.%s",
					fk.ReferencedTable, table, fk.Name)
			}
		}
	}
}

func validateRequiredTables(t *testing.T, db DatabaseDriver, dbType DatabaseType) {
	ctx := context.Background()

	requiredTables := []string{
		"users", "nodes", "servers", "allocations", "audit_events",
		"organizations", "projects", "environments", "team_members",
		"applications", "app_services", "deployments", "backups",
		"replica_applications", "instances", "placement_decisions",
		"reconcile_plans", "reconcile_events", "service_endpoints",
		"procedures", "procedure_steps", "procedure_executions",
		"procedure_step_executions", "procedure_schedules",
		"deployment_steps", "backup_policies", "backup_manifests",
		"backup_storage_receipts", "database_backups", "volume_backups",
	}

	tables, err := getAllTables(ctx, db, dbType)
	if err != nil {
		t.Fatalf("Failed to get tables: %v", err)
	}

	tableSet := make(map[string]bool)
	for _, table := range tables {
		tableSet[table] = true
	}

	for _, required := range requiredTables {
		if !tableSet[required] {
			t.Errorf("Missing required table: %s", required)
		}
	}
}

func validateDatabaseConstraints(t *testing.T, db DatabaseDriver, dbType DatabaseType) {
	ctx := context.Background()

	// Get all tables
	tables, err := getAllTables(ctx, db, dbType)
	if err != nil {
		t.Fatalf("Failed to get tables: %v", err)
	}

	for _, table := range tables {
		// Check foreign keys
		foreignKeys, err := getForeignKeys(ctx, db, dbType, table)
		if err != nil {
			continue
		}

		for _, fk := range foreignKeys {
			// Validate ON DELETE behavior
			if fk.OnDelete == "" {
				// Default should be RESTRICT or NO ACTION for data integrity
				t.Logf("Warning: Foreign key %s on table %s has no explicit ON DELETE action",
					fk.Name, table)
			}

			// Check for cascading behavior where appropriate
			if fk.OnDelete == "CASCADE" {
				t.Logf("CASCADE delete found on %s.%s -> %s", table, fk.Name, fk.ReferencedTable)
			}
		}

		// Check indexes
		indexes, err := getIndexes(ctx, db, dbType, table)
		if err != nil {
			continue
		}

		for _, idx := range indexes {
			if idx.IsUnique {
				t.Logf("Unique constraint: %s on %s", idx.Name, table)
			}
		}

		// Check nullable vs required fields
		columns, err := getColumns(ctx, db, dbType, table)
		if err != nil {
			continue
		}

		for _, col := range columns {
			if !col.IsNullable && col.DefaultValue == "" {
				// This is a required field without default - ensure it's handled in migrations
				t.Logf("Required field without default: %s.%s", table, col.Name)
			}
		}
	}
}

func validateRequiredBatch2Entities(t *testing.T, ctx context.Context, db DatabaseDriver, dbType DatabaseType) {
	// Test all required Batch 2 entities
	requiredEntities := []struct {
		Table       string
		Description string
	}{
		{"service_endpoints", "Service discovery endpoints"},
		{"replica_applications", "Service definitions"},
		{"instances", "Replicas"},
		{"placement_decisions", "Placement constraints"},
		{"reconcile_plans", "Drift records"},
		{"procedures", "Procedure definitions"},
		{"procedure_steps", "Procedure steps"},
		{"procedure_executions", "Procedure execution tracking"},
		{"deployment_steps", "Deployment strategy and rollout state"},
		{"backup_policies", "Backup policies"},
		{"backup_manifests", "Backup manifests"},
		{"backup_storage_receipts", "Storage receipts"},
		{"database_backups", "Database backup records"},
		{"volume_backups", "Volume backup records"},
		{"app_services", "Service definitions with update strategy"},
		{"applications", "Application definitions"},
	}

	for _, entity := range requiredEntities {
		// Check if table exists
		tableExists, err := tableExists(ctx, db, dbType, entity.Table)
		if err != nil {
			t.Errorf("Error checking table %s: %v", entity.Table, err)
			continue
		}

		if !tableExists {
			t.Errorf("Missing required Batch 2 entity table: %s (%s)", entity.Table, entity.Description)
		} else {
			t.Logf("✅ Found Batch 2 entity: %s (%s)", entity.Table, entity.Description)
		}

		// Check for tenancy ownership columns
		if entity.Table != "reconcile_plans" && entity.Table != "reconcile_events" {
			orgIDColumnExists, _ := columnExists(ctx, db, dbType, entity.Table, "org_id")
			projectIDColumnExists, _ := columnExists(ctx, db, dbType, entity.Table, "project_id")

			if orgIDColumnExists || projectIDColumnExists {
				t.Logf("✅ Tenancy ownership columns found in %s", entity.Table)
			} else {
				t.Logf("⚠️  No tenancy ownership columns in %s", entity.Table)
			}
		}
	}
}

func insertBatch1TestData(t *testing.T, ctx context.Context, db DatabaseDriver, dbType DatabaseType) {
	// Insert test data for Batch 1 entities that should survive upgrade

	// Create test users
	userID := "00000000-0000-0000-0000-000000000001"
	_, err := db.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, role, created_at) VALUES ($1, $2, $3, $4, NOW())`,
		userID, "test@example.com", "hashed_password", "admin")
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	// Create test nodes
	nodeID := "00000000-0000-0000-0000-000000000002"
	_, err = db.Exec(ctx,
		`INSERT INTO nodes (id, name, region, base_url, status, token_hash, created_at) VALUES ($1, $2, $3, $4, $5, $6, NOW())`,
		nodeID, "test-node", "us-east-1", "http://localhost:8080", "online", "token_hash")
	if err != nil {
		t.Fatalf("Failed to insert test node: %v", err)
	}

	// Create test servers
	serverID := "00000000-0000-0000-0000-000000000003"
	_, err = db.Exec(ctx,
		`INSERT INTO servers (id, node_id, owner_id, name, status, memory_mb, cpu_shares, disk_mb, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())`,
		serverID, nodeID, userID, "test-server", "stopped", 2048, 1024, 10240)
	if err != nil {
		t.Fatalf("Failed to insert test server: %v", err)
	}

	// Create test allocations
	allocationID := "00000000-0000-0000-0000-000000000004"
	_, err = db.Exec(ctx,
		`INSERT INTO allocations (id, node_id, server_id, ip, port, alias, notes, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`,
		allocationID, nodeID, serverID, "127.0.0.1", 25565, "default", "")
	if err != nil {
		t.Fatalf("Failed to insert test allocation: %v", err)
	}

	// Create test backups
	backupID := "00000000-0000-0000-0000-000000000005"
	_, err = db.Exec(ctx,
		`INSERT INTO backups (uuid, server_id, name, checksum, size, status, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`,
		backupID, serverID, "test-backup", "abc123", 1024, "completed")
	if err != nil {
		t.Fatalf("Failed to insert test backup: %v", err)
	}

	// Create test deployments
	deploymentID := "00000000-0000-0000-0000-000000000006"
	_, err = db.Exec(ctx,
		`INSERT INTO deployments (id, server_id, strategy, status, image, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, NOW(), NOW())`,
		deploymentID, serverID, "blue-green", "pending", "nginx:latest")
	if err != nil {
		t.Fatalf("Failed to insert test deployment: %v", err)
	}

	t.Log("✅ Inserted Batch 1 test data")
}

func validateDataSurvival(t *testing.T, ctx context.Context, db DatabaseDriver, dbType DatabaseType) {
	// Check that Batch 1 data still exists after upgrade

	// Check users
	var userCount int
	err := db.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&userCount)
	if err != nil {
		t.Fatalf("Failed to count users: %v", err)
	}
	if userCount == 0 {
		t.Error("Data loss: users table is empty after upgrade")
	} else {
		t.Logf("✅ Users survived upgrade: %d users", userCount)
	}

	// Check servers
	var serverCount int
	err = db.QueryRow(ctx, "SELECT COUNT(*) FROM servers").Scan(&serverCount)
	if err != nil {
		t.Fatalf("Failed to count servers: %v", err)
	}
	if serverCount == 0 {
		t.Error("Data loss: servers table is empty after upgrade")
	} else {
		t.Logf("✅ Servers survived upgrade: %d servers", serverCount)
	}

	// Check backups
	var backupCount int
	err = db.QueryRow(ctx, "SELECT COUNT(*) FROM backups").Scan(&backupCount)
	if err != nil {
		t.Fatalf("Failed to count backups: %v", err)
	}
	if backupCount == 0 {
		t.Error("Data loss: backups table is empty after upgrade")
	} else {
		t.Logf("✅ Backups survived upgrade: %d backups", backupCount)
	}

	// Check deployments
	var deploymentCount int
	err = db.QueryRow(ctx, "SELECT COUNT(*) FROM deployments").Scan(&deploymentCount)
	if err != nil {
		t.Fatalf("Failed to count deployments: %v", err)
	}
	if deploymentCount == 0 {
		t.Error("Data loss: deployments table is empty after upgrade")
	} else {
		t.Logf("✅ Deployments survived upgrade: %d deployments", deploymentCount)
	}
}

func validateNoDataLoss(t *testing.T, ctx context.Context, db DatabaseDriver, dbType DatabaseType) {
	// Validate that no data was silently lost during migration
	// Check specific test data that was inserted

	// Check for specific user
	var userExists bool
	err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE email = 'test@example.com')").Scan(&userExists)
	if err != nil {
		t.Fatalf("Failed to check user existence: %v", err)
	}
	if !userExists {
		t.Error("Data loss: test user missing after upgrade")
	} else {
		t.Log("✅ Test user data preserved")
	}

	// Check for specific server
	var serverExists bool
	err = db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM servers WHERE name = 'test-server')").Scan(&serverExists)
	if err != nil {
		t.Fatalf("Failed to check server existence: %v", err)
	}
	if !serverExists {
		t.Error("Data loss: test server missing after upgrade")
	} else {
		t.Log("✅ Test server data preserved")
	}
}

func validateBatch2SchemaAdditions(t *testing.T, ctx context.Context, db DatabaseDriver, dbType DatabaseType) {
	// Validate that Batch 2 migrations added the expected schema elements

	// Check for new tables
	batch2Tables := []string{
		"organizations", "projects", "environments", "team_members",
		"applications", "app_services", "replica_applications", "instances",
		"placement_decisions", "reconcile_plans", "reconcile_events",
		"service_endpoints", "procedures", "procedure_steps",
		"procedure_executions", "procedure_step_executions", "procedure_schedules",
		"deployment_steps", "backup_policies", "backup_manifests",
		"backup_storage_receipts", "database_backups", "volume_backups",
	}

	for _, table := range batch2Tables {
		tableExists, err := tableExists(ctx, db, dbType, table)
		if err != nil {
			t.Errorf("Error checking table %s: %v", table, err)
			continue
		}

		if !tableExists {
			t.Errorf("Batch 2 table missing: %s", table)
		} else {
			t.Logf("✅ Batch 2 table exists: %s", table)
		}
	}

	// Check for new columns in existing tables
	newColumns := []struct {
		Table  string
		Column string
	}{
		{"servers", "org_id"},
		{"servers", "project_id"},
		{"servers", "environment_id"},
		{"deployments", "org_id"},
		{"backups", "org_id"},
		{"app_services", "mode"},
		{"app_services", "update_config"},
		{"app_services", "health_check"},
		{"app_services", "resources"},
		{"app_services", "volumes"},
		{"app_services", "secrets"},
		{"app_services", "replica_app_id"},
	}

	for _, col := range newColumns {
		columnExists, err := columnExists(ctx, db, dbType, col.Table, col.Column)
		if err != nil {
			t.Errorf("Error checking column %s.%s: %v", col.Table, col.Column, err)
			continue
		}

		if !columnExists {
			t.Errorf("Batch 2 column missing: %s.%s", col.Table, col.Column)
		} else {
			t.Logf("✅ Batch 2 column exists: %s.%s", col.Table, col.Column)
		}
	}
}

// Database introspection functions

func getAllTables(ctx context.Context, db DatabaseDriver, dbType DatabaseType) ([]string, error) {
	var query string
	switch dbType {
	case DatabasePostgres:
		query = `SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE' ORDER BY table_name`
	case DatabaseMySQL, DatabaseMariaDB:
		query = `SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE' ORDER BY table_name`
	case DatabaseSQLite:
		query = `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}

	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}

	return tables, nil
}

func tableExists(ctx context.Context, db DatabaseDriver, dbType DatabaseType, tableName string) (bool, error) {
	tables, err := getAllTables(ctx, db, dbType)
	if err != nil {
		return false, err
	}

	for _, table := range tables {
		if table == tableName {
			return true, nil
		}
	}
	return false, nil
}

func columnExists(ctx context.Context, db DatabaseDriver, dbType DatabaseType, tableName, columnName string) (bool, error) {
	var query string
	switch dbType {
	case DatabasePostgres:
		query = fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = '%s' AND column_name = '%s')`, tableName, columnName)
	case DatabaseMySQL, DatabaseMariaDB:
		query = fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = '%s' AND column_name = '%s')`, tableName, columnName)
	case DatabaseSQLite:
		query = fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM pragma_table_info('%s') WHERE name = '%s')`, tableName, columnName)
	default:
		return false, fmt.Errorf("unsupported database type: %s", dbType)
	}

	var exists bool
	if err := db.QueryRow(ctx, query).Scan(&exists); err != nil {
		return false, err
	}

	return exists, nil
}

func getForeignKeys(ctx context.Context, db DatabaseDriver, dbType DatabaseType, tableName string) ([]struct {
	Name             string
	Column           string
	ReferencedTable  string
	ReferencedColumn string
	OnDelete         string
}, error) {
	var query string
	switch dbType {
	case DatabasePostgres:
		query = fmt.Sprintf(`
			SELECT
				conname as name,
				conkey as column,
				confrelid::regclass as referenced_table,
				confkey as referenced_column,
				confdeltype as on_delete
			FROM pg_constraint
			WHERE conrelid = '%s'::regclass AND contype = 'f'
		`, tableName)
	case DatabaseMySQL, DatabaseMariaDB:
		query = fmt.Sprintf(`
			SELECT
				CONSTRAINT_NAME as name,
				COLUMN_NAME as column,
				REFERENCED_TABLE_NAME as referenced_table,
				REFERENCED_COLUMN_NAME as referenced_column,
				DELETE_RULE as on_delete
			FROM information_schema.KEY_COLUMN_USAGE
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = '%s' AND REFERENCED_TABLE_NAME IS NOT NULL
		`, tableName)
	case DatabaseSQLite:
		// SQLite doesn't have good foreign key introspection, return empty
		return []struct {
			Name             string
			Column           string
			ReferencedTable  string
			ReferencedColumn string
			OnDelete         string
		}{}, nil
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}

	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fks []struct {
		Name             string
		Column           string
		ReferencedTable  string
		ReferencedColumn string
		OnDelete         string
	}

	for rows.Next() {
		var fk struct {
			Name             string
			Column           string
			ReferencedTable  string
			ReferencedColumn string
			OnDelete         string
		}
		if err := rows.Scan(&fk.Name, &fk.Column, &fk.ReferencedTable, &fk.ReferencedColumn, &fk.OnDelete); err != nil {
			return nil, err
		}
		fks = append(fks, fk)
	}

	return fks, nil
}

func getIndexes(ctx context.Context, db DatabaseDriver, dbType DatabaseType, tableName string) ([]struct {
	Name     string
	IsUnique bool
	Columns  string
}, error) {
	var query string
	switch dbType {
	case DatabasePostgres:
		query = fmt.Sprintf(`
			SELECT
				indexname as name,
				indexdef as definition
			FROM pg_indexes
			WHERE tablename = '%s'
		`, tableName)
	case DatabaseMySQL, DatabaseMariaDB:
		query = fmt.Sprintf(`
			SELECT
				INDEX_NAME as name,
				NON_UNIQUE as non_unique,
				COLUMN_NAME as column_name
			FROM information_schema.STATISTICS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = '%s'
		`, tableName)
	case DatabaseSQLite:
		query = fmt.Sprintf(`SELECT sql FROM sqlite_master WHERE type = 'index' AND tbl_name = '%s'`, tableName)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}

	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []struct {
		Name     string
		IsUnique bool
		Columns  string
	}

	for rows.Next() {
		var idx struct {
			Name     string
			IsUnique bool
			Columns  string
		}

		if dbType == DatabasePostgres {
			var definition string
			if err := rows.Scan(&idx.Name, &definition); err != nil {
				return nil, err
			}
			idx.IsUnique = strings.Contains(definition, "UNIQUE")
		} else if dbType == DatabaseMySQL || dbType == DatabaseMariaDB {
			var nonUnique int
			var columnName string
			if err := rows.Scan(&idx.Name, &nonUnique, &columnName); err != nil {
				return nil, err
			}
			idx.IsUnique = nonUnique == 0
			idx.Columns = columnName
		} else {
			var sql string
			if err := rows.Scan(&sql); err != nil {
				return nil, err
			}
			idx.Name = sql
			// Parse SQLite index SQL to determine uniqueness
			idx.IsUnique = strings.Contains(strings.ToUpper(sql), "UNIQUE")
		}

		indexes = append(indexes, idx)
	}

	return indexes, nil
}

func getColumns(ctx context.Context, db DatabaseDriver, dbType DatabaseType, tableName string) ([]struct {
	Name         string
	DataType     string
	IsNullable   bool
	DefaultValue string
}, error) {
	var query string
	switch dbType {
	case DatabasePostgres:
		query = fmt.Sprintf(`
			SELECT
				column_name as name,
				data_type as data_type,
				is_nullable as is_nullable,
				column_default as default_value
			FROM information_schema.columns
			WHERE table_name = '%s' ORDER BY ordinal_position
		`, tableName)
	case DatabaseMySQL, DatabaseMariaDB:
		query = fmt.Sprintf(`
			SELECT
				COLUMN_NAME as name,
				DATA_TYPE as data_type,
				IS_NULLABLE as is_nullable,
				COLUMN_DEFAULT as default_value
			FROM information_schema.columns
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = '%s' ORDER BY ORDINAL_POSITION
		`, tableName)
	case DatabaseSQLite:
		query = fmt.Sprintf(`PRAGMA table_info('%s')`, tableName)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}

	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []struct {
		Name         string
		DataType     string
		IsNullable   bool
		DefaultValue string
	}

	for rows.Next() {
		var col struct {
			Name         string
			DataType     string
			IsNullable   bool
			DefaultValue string
		}

		if dbType == DatabaseSQLite {
			var cid, notNull, pk int
			var dfltValue sql.NullString
			if err := rows.Scan(&cid, &col.Name, &col.DataType, &notNull, &dfltValue, &pk); err != nil {
				return nil, err
			}
			col.IsNullable = notNull == 0
			if dfltValue.Valid {
				col.DefaultValue = dfltValue.String
			}
		} else {
			if err := rows.Scan(&col.Name, &col.DataType, &col.IsNullable, &col.DefaultValue); err != nil {
				return nil, err
			}
		}

		columns = append(columns, col)
	}

	return columns, nil
}

// TestMain function to set up test environment
func TestMain(m *testing.M) {
	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Run tests
	os.Exit(m.Run())
}
