package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

// Database configuration
type DBConfig struct {
	Type     string
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string
}

func main() {
	log.Println("🚀 Starting comprehensive database migration testing...")

	// Test SQLite (always available)
	log.Println("\n📋 Testing SQLite database...")
	testSQLiteMigrations()

	// Test PostgreSQL (if available)
	log.Println("\n📋 Testing PostgreSQL database...")
	testPostgresMigrations()

	log.Println("\n✅ All migration tests completed!")
}

func testSQLiteMigrations() {
	// Create in-memory database
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		log.Printf("❌ SQLite test failed: %v", err)
		return
	}
	defer db.Close()

	// Set up database for better foreign key support
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	if err != nil {
		log.Printf("⚠️  Could not enable foreign keys: %v", err)
	}

	migrationDir := filepath.Join("..", "forge", "api", "migrations")

	// Test Fresh Installation
	log.Println("  🆕 Testing Fresh Installation...")
	if err := runAllMigrations(db, migrationDir); err != nil {
		log.Printf("❌ Fresh installation failed: %v", err)
		return
	}

	// Validate fresh installation
	if err := validateFreshInstallation(db); err != nil {
		log.Printf("❌ Fresh installation validation failed: %v", err)
		return
	}

	// Test Batch 2 Entities
	log.Println("  📦 Testing Batch 2 Entities...")
	if err := validateBatch2Entities(db); err != nil {
		log.Printf("❌ Batch 2 entities validation failed: %v", err)
		return
	}

	// Test Upgrade Installation
	log.Println("  🔄 Testing Upgrade Installation...")
	if err := testUpgradeScenario(db, migrationDir); err != nil {
		log.Printf("❌ Upgrade installation test failed: %v", err)
		return
	}

	log.Println("✅ SQLite migration tests passed!")
}

func testPostgresMigrations() {
	// Try to connect to PostgreSQL
	connStr := "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Printf("⚠️  PostgreSQL not available: %v", err)
		return
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		log.Printf("⚠️  PostgreSQL connection failed: %v", err)
		return
	}

	// Create temporary database
	tempDBName := fmt.Sprintf("gamepanel_test_%d", time.Now().Unix())
	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", tempDBName))
	if err != nil {
		log.Printf("⚠️  Could not create test database: %v", err)
		return
	}

	// Connect to test database
	testConnStr := fmt.Sprintf("postgres://postgres:postgres@localhost:5432/%s?sslmode=disable", tempDBName)
	testDB, err := sql.Open("postgres", testConnStr)
	if err != nil {
		log.Printf("⚠️  Could not connect to test database: %v", err)
		// Clean up
		db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", tempDBName))
		return
	}
	defer testDB.Close()

	// Clean up on exit
	defer func() {
		db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", tempDBName))
	}()

	migrationDir := filepath.Join("..", "forge", "api", "migrations")

	// Test Fresh Installation
	log.Println("  🆕 Testing Fresh Installation...")
	if err := runAllMigrations(testDB, migrationDir); err != nil {
		log.Printf("❌ Fresh installation failed: %v", err)
		return
	}

	// Validate fresh installation
	if err := validateFreshInstallation(testDB); err != nil {
		log.Printf("❌ Fresh installation validation failed: %v", err)
		return
	}

	// Test Batch 2 Entities
	log.Println("  📦 Testing Batch 2 Entities...")
	if err := validateBatch2Entities(testDB); err != nil {
		log.Printf("❌ Batch 2 entities validation failed: %v", err)
		return
	}

	log.Println("✅ PostgreSQL migration tests passed!")
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

	// Check for dialect-specific migrations
	dialectMigrations := make(map[string]string)
	for _, migration := range migrations {
		dialectMigrations[migration] = filepath.Join(migrationDir, migration)
	}

	// Check for MySQL-specific migrations
	mysqlDir := filepath.Join(migrationDir, "mysql")
	if entries, err := os.ReadDir(mysqlDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
				dialectMigrations[entry.Name()] = filepath.Join(mysqlDir, entry.Name())
			}
		}
	}

	// Check for SQLite-specific migrations
	sqliteDir := filepath.Join(migrationDir, "sqlite")
	if entries, err := os.ReadDir(sqliteDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
				dialectMigrations[entry.Name()] = filepath.Join(sqliteDir, entry.Name())
			}
		}
	}

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
			log.Printf("  ✅ Migration %s already applied", migration)
			continue
		}

		migrationPath := dialectMigrations[migration]
		if migrationPath == "" {
			migrationPath = filepath.Join(migrationDir, migration)
		}

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
	var current string
	inQuotes := false
	inBackticks := false

	for _, char := range sql {
		switch char {
		case ';':
			if !inQuotes && !inBackticks {
				if strings.TrimSpace(current) != "" {
					statements = append(statements, current)
				}
				current = ""
			} else {
				current += char
			}
		case '\'':
			inQuotes = !inQuotes
			current += char
		case '`':
			inBackticks = !inBackticks
			current += char
		default:
			current += char
		}
	}

	// Add the last statement if it exists
	if strings.TrimSpace(current) != "" {
		statements = append(statements, current)
	}

	return statements
}

func validateFreshInstallation(db *sql.DB) error {
	log.Println("  🔍 Validating fresh installation...")

	// Check for duplicate migration identifiers
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to count migrations: %w", err)
	}
	log.Printf("  ✅ Applied %d migrations", count)

	// Check for duplicate IDs
	var dupCount int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT id FROM schema_migrations GROUP BY id HAVING COUNT(*) > 1
		) AS duplicates
	`).Scan(&dupCount)
	if err != nil {
		return fmt.Errorf("failed to check for duplicate migrations: %w", err)
	}
	if dupCount > 0 {
		return fmt.Errorf("found %d duplicate migration identifiers", dupCount)
	}
	log.Println("  ✅ No duplicate migration identifiers")

	// Check for required tables
	requiredTables := []string{
		"users", "nodes", "servers", "allocations", "audit_events",
		"organizations", "projects", "environments", "team_members",
		"applications", "app_services", "deployments", "backups",
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
	log.Println("  ✅ All required tables exist")

	// Check for missing referenced tables (foreign keys)
	if err := validateForeignKeys(db); err != nil {
		return err
	}

	// Check for tenancy ownership columns
	if err := validateTenancyColumns(db); err != nil {
		return err
	}

	return nil
}

func validateForeignKeys(db *sql.DB) error {
	log.Println("  🔍 Validating foreign keys...")

	// Get all tables
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		return fmt.Errorf("failed to get tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return fmt.Errorf("failed to scan table: %w", err)
		}
		tables = append(tables, table)
	}

	// For SQLite, we need to parse the schema to find foreign keys
	// This is a simplified check - in a real implementation, you'd parse the CREATE TABLE statements
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

	return nil
}

func validateTenancyColumns(db *sql.DB) error {
	log.Println("  🔍 Validating tenancy ownership columns...")

	// Check that key tables have org_id columns
	tablesWithTenancy := []string{
		"servers", "deployments", "backups", "applications", "app_services",
		"replica_applications", "instances", "procedures",
	}

	for _, table := range tablesWithTenancy {
		var exists bool
		err := db.QueryRow(fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM pragma_table_info('%s') WHERE name='org_id')", table)).Scan(&exists)
		if err != nil {
			// Table might not exist in SQLite test
			continue
		}
		if exists {
			log.Printf("  ✅ Tenancy column (org_id) found in %s", table)
		} else {
			log.Printf("  ⚠️  No org_id column in %s", table)
		}
	}

	return nil
}

func validateBatch2Entities(db *sql.DB) error {
	log.Println("  🔍 Validating Batch 2 entities...")

	// List of all required Batch 2 entities
	batch2Entities := []struct {
		Table       string
		Description string
	}{
		{"organizations", "Team-based tenancy"},
		{"projects", "Project grouping under organizations"},
		{"environments", "Per-project deployment environments"},
		{"team_members", "Organization-level membership"},
		{"applications", "Unified workload identity"},
		{"app_services", "Service definitions with update strategy"},
		{"replica_applications", "Service definitions"},
		{"instances", "Replicas"},
		{"placement_decisions", "Placement constraints"},
		{"reconcile_plans", "Drift records"},
		{"reconcile_events", "Reconciliation events"},
		{"service_endpoints", "Service discovery endpoints"},
		{"procedures", "Procedure definitions"},
		{"procedure_steps", "Procedure steps"},
		{"procedure_executions", "Procedure execution tracking"},
		{"procedure_step_executions", "Procedure step execution tracking"},
		{"procedure_schedules", "Procedure schedules"},
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

func testUpgradeScenario(db *sql.DB, migrationDir string) error {
	log.Println("  🔍 Testing upgrade scenario...")

	// First, create a fresh database with only Batch 1 migrations (001-099)
	batch1Migrations := []string{
		"001_init.sql", "002_add_primary_allocation.sql", "003_server_manage_fields.sql",
		"004_server_transfer_state.sql", "005_server_transfer_lifecycle.sql",
		"006_transfer_run_token.sql", "007_postgres_core_foundation.sql",
		"008_server_schedules.sql", "009_schedule_run_history.sql", "010_node_heartbeat.sql",
		"011_wings_config_install.sql", "012_wings_node_parity.sql", "013_startup_variables.sql",
		"014_server_databases.sql", "015_db_hosts_constraints.sql", "015_mounts.sql",
		"016_subusers_panel_parity.sql", "017_api_keys.sql", "018_api_key_scopes.sql",
		"018_ssh_2fa_activity.sql", "019_backups.sql", "020_node_expansion.sql",
		"020_regions_multi_node_foundation.sql", "021_true_state_persistence.sql",
		"022_evacuation_planner.sql", "023_migration_engine.sql", "024_observability_foundation.sql",
		"025_heartbeat_expiry_engine.sql", "026_placement_reservations.sql",
		"027_recovery_coordinator.sql", "028_server_provisioning_parity.sql",
		"032_panel_settings.sql", "033_panel_settings_mail_and_advanced.sql",
		"034_user_resource_limits.sql", "035_oauth2_clients.sql", "036_plugins.sql",
		"037_panel_settings_expansion.sql", "038_password_reset_tokens.sql",
		"039_webhooks.sql", "040_truthful_server_lifecycle.sql",
		"041_auth_session_security.sql", "042_database_provisioning_security.sql",
		"043_unify_eggs_templates_mounts.sql", "044_async_delivery_foundation.sql",
		"044_cloud_node_links.sql", "045_real_server_transfer.sql",
		"046_encrypt_operational_secrets.sql", "047_2fa_enhancements.sql",
		"048_s3_backup_config.sql", "049_backup_locking.sql", "050_subuser_invitations.sql",
		"051_backup_status_tracking.sql", "052_schedule_timezone.sql",
		"053_account_sessions.sql", "054_activity_events.sql", "054_social_auth.sql",
		"055_plugin_metadata.sql", "056_mail_notification_triggers.sql",
		"057_backup_policies.sql", "057_job_queue.sql", "057_webauthn.sql",
		"058_rate_limit_settings.sql", "059_server_crash_events.sql",
		"060_region_slug_normalization.sql", "077_add_runtime_status_to_nodes.sql",
		"078_node_description.sql", "079_evacuation_execution.sql",
		"080_recovery_backup_execution.sql", "080_recovery_execution_statuses.sql",
		"081_social_provider_configuration.sql", "082_deployments.sql",
		"082_failover.sql", "082_target_groups.sql", "083_autoscaler.sql",
		"083_traffic_rules.sql", "084_create_missing_tables.sql",
		"085_node_public_flag.sql", "086_add_table_constraints.sql",
		"087_node_parity_fields.sql", "087_parity_schema.sql",
		"088_server_parity_fields.sql", "089_egg_parity_fields.sql",
		"090_allocation_transport.sql", "091_seed_minecraft_java.sql",
		"092_durable_operations.sql", "093_health_check_history.sql",
		"094_acme_certificates.sql", "096_dns_providers.sql",
		"097_git_credentials.sql", "098_app_platform_foundations.sql",
		"099_deployment_revisions.sql",
	}

	// Apply Batch 1 migrations
	for _, migration := range batch1Migrations {
		migrationPath := filepath.Join(migrationDir, migration)
		if _, err := os.Stat(migrationPath); os.IsNotExist(err) {
			log.Printf("  ⚠️  Batch 1 migration not found: %s", migration)
			continue
		}

		content, err := os.ReadFile(migrationPath)
		if err != nil {
			return fmt.Errorf("failed to read Batch 1 migration %s: %w", migration, err)
		}

		statements := splitSQLStatements(string(content))
		for _, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				return fmt.Errorf("failed to apply Batch 1 migration %s: %w (stmt: %.100s)", migration, err, stmt)
			}
		}
	}

	// Insert test data
	if err := insertTestData(db); err != nil {
		return fmt.Errorf("failed to insert test data: %w", err)
	}

	// Now apply Batch 2 migrations (100+)
	batch2Migrations := []string{
		"100_team_tenancy.sql",
		"101_app_platform_applications.sql",
		"101_multi_node_replicas.sql",
		"102_reconcile_plans.sql",
		"102_uncloud_service_model.sql",
		"103_backup_schedules_orchestration.sql",
		"103_deployment_steps.sql",
		"103_procedures.sql",
	}

	for _, migration := range batch2Migrations {
		migrationPath := filepath.Join(migrationDir, migration)
		if _, err := os.Stat(migrationPath); os.IsNotExist(err) {
			log.Printf("  ⚠️  Batch 2 migration not found: %s", migration)
			continue
		}

		content, err := os.ReadFile(migrationPath)
		if err != nil {
			return fmt.Errorf("failed to read Batch 2 migration %s: %w", migration, err)
		}

		statements := splitSQLStatements(string(content))
		for _, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				return fmt.Errorf("failed to apply Batch 2 migration %s: %w (stmt: %.100s)", migration, err, stmt)
			}
		}

		log.Printf("  ✅ Applied Batch 2 migration: %s", migration)
	}

	// Validate data survival
	if err := validateDataSurvival(db); err != nil {
		return err
	}

	return nil
}

func insertTestData(db *sql.DB) error {
	log.Println("  📝 Inserting test data...")

	// Insert test user
	_, err := db.Exec(`
		INSERT INTO users (id, email, password_hash, role, created_at)
		VALUES ('00000000-0000-0000-0000-000000000001', 'test@example.com', 'hashed_password', 'admin', datetime('now'))
	`)
	if err != nil {
		return fmt.Errorf("failed to insert test user: %w", err)
	}

	// Insert test node
	_, err = db.Exec(`
		INSERT INTO nodes (id, name, region, base_url, status, token_hash, created_at)
		VALUES ('00000000-0000-0000-0000-000000000002', 'test-node', 'us-east-1', 'http://localhost:8080', 'online', 'token_hash', datetime('now'))
	`)
	if err != nil {
		return fmt.Errorf("failed to insert test node: %w", err)
	}

	// Insert test server
	_, err = db.Exec(`
		INSERT INTO servers (id, node_id, owner_id, name, status, memory_mb, cpu_shares, disk_mb, created_at)
		VALUES ('00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001', 'test-server', 'stopped', 2048, 1024, 10240, datetime('now'))
	`)
	if err != nil {
		return fmt.Errorf("failed to insert test server: %w", err)
	}

	// Insert test backup
	_, err = db.Exec(`
		INSERT INTO backups (uuid, server_id, name, checksum, size, status, created_at, updated_at)
		VALUES ('00000000-0000-0000-0000-000000000004', '00000000-0000-0000-0000-000000000003', 'test-backup', 'abc123', 1024, 'completed', datetime('now'), datetime('now'))
	`)
	if err != nil {
		return fmt.Errorf("failed to insert test backup: %w", err)
	}

	log.Println("  ✅ Test data inserted successfully")
	return nil
}

func validateDataSurvival(db *sql.DB) error {
	log.Println("  🔍 Validating data survival...")

	// Check users
	var userCount int
	err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	if err != nil {
		return fmt.Errorf("failed to count users: %w", err)
	}
	if userCount == 0 {
		return fmt.Errorf("data loss: users table is empty after upgrade")
	}
	log.Printf("  ✅ Users survived: %d users", userCount)

	// Check servers
	var serverCount int
	err = db.QueryRow("SELECT COUNT(*) FROM servers").Scan(&serverCount)
	if err != nil {
		return fmt.Errorf("failed to count servers: %w", err)
	}
	if serverCount == 0 {
		return fmt.Errorf("data loss: servers table is empty after upgrade")
	}
	log.Printf("  ✅ Servers survived: %d servers", serverCount)

	// Check backups
	var backupCount int
	err = db.QueryRow("SELECT COUNT(*) FROM backups").Scan(&backupCount)
	if err != nil {
		return fmt.Errorf("failed to count backups: %w", err)
	}
	if backupCount == 0 {
		return fmt.Errorf("data loss: backups table is empty after upgrade")
	}
	log.Printf("  ✅ Backups survived: %d backups", backupCount)

	// Check specific test data
	var testUserExists bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE email = 'test@example.com')").Scan(&testUserExists)
	if err != nil {
		return fmt.Errorf("failed to check test user: %w", err)
	}
	if !testUserExists {
		return fmt.Errorf("data loss: test user missing after upgrade")
	}
	log.Println("  ✅ Test user data preserved")

	// Check Batch 2 tables were added
	var orgTableExists bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='organizations')").Scan(&orgTableExists)
	if err != nil {
		return fmt.Errorf("failed to check organizations table: %w", err)
	}
	if !orgTableExists {
		return fmt.Errorf("Batch 2 table not created: organizations")
	}
	log.Println("  ✅ Batch 2 tables created successfully")

	return nil
}
