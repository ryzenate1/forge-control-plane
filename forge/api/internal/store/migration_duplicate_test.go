package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoDuplicatePrefixesInInternalMigrations(t *testing.T) {
	internalDir := filepath.Join("migrations")

	entries, err := os.ReadDir(internalDir)
	if err != nil {
		t.Fatalf("read internal migrations dir: %v", err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			files = append(files, entry.Name())
		}
	}

	if err := validateNoDuplicatePrefixes(files); err != nil {
		t.Errorf("duplicate prefixes in internal store migrations: %v", err)
	}
}

func TestValidateNoDuplicatePrefixes_NoDuplicates(t *testing.T) {
	files := []string{
		"001_init.sql",
		"002_add_primary_allocation.sql",
		"015_db_hosts_constraints.sql",
		"015_a_mounts.sql",
		"083_autoscaler.sql",
		"083_a_traffic_rules.sql",
		"110_node_fencing.sql",
		"111_routing_rules_websocket.sql",
	}
	if err := validateNoDuplicatePrefixes(files); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidateNoDuplicatePrefixes_DetectsDuplicates(t *testing.T) {
	files := []string{
		"001_init.sql",
		"015_db_hosts_constraints.sql",
		"015_mounts.sql",
		"083_autoscaler.sql",
		"083_traffic_rules.sql",
	}
	err := validateNoDuplicatePrefixes(files)
	if err == nil {
		t.Fatal("expected error for duplicate prefixes, got nil")
	}
	if !contains(err.Error(), "015") {
		t.Errorf("error should mention prefix 015, got: %v", err)
	}
}

func TestValidateNoDuplicatePrefixes_MultipleDuplicates(t *testing.T) {
	files := []string{
		"001_init.sql",
		"015_a.sql",
		"015_b.sql",
		"020_x.sql",
		"020_y.sql",
		"020_z.sql",
		"100_clean.sql",
	}
	err := validateNoDuplicatePrefixes(files)
	if err == nil {
		t.Fatal("expected error for multiple duplicate prefixes, got nil")
	}
}

func TestValidateNoDuplicatePrefixes_LetterSuffixNotMatch(t *testing.T) {
	files := []string{
		"001_init.sql",
		"015_db_hosts_constraints.sql",
		"015_a_mounts.sql",
		"015_b_other.sql",
	}
	// 015, 015_a, 015_b are three distinct prefixes
	if err := validateNoDuplicatePrefixes(files); err != nil {
		t.Errorf("letter suffixes should create distinct prefixes, got: %v", err)
	}
}

func TestValidateNoDuplicatePrefixes_OnlyBaseOrLetterNotBoth(t *testing.T) {
	files := []string{
		"015_db_hosts_constraints.sql",
		"015_a_mounts.sql",
	}
	// "015" and "015_a" are distinct
	if err := validateNoDuplicatePrefixes(files); err != nil {
		t.Errorf("expected 015 and 015_a to be distinct, got: %v", err)
	}
}

func TestMigrationPrefix(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"001_init.sql", "001"},
		{"015_db_hosts_constraints.sql", "015"},
		{"015_a_mounts.sql", "015_a"},
		{"057_backup_policies.sql", "057"},
		{"057_a_job_queue.sql", "057_a"},
		{"057_b_webauthn.sql", "057_b"},
		{"083_autoscaler.sql", "083"},
		{"083_a_traffic_rules.sql", "083_a"},
		{"110_node_fencing.sql", "110"},
		{"111_routing_rules_websocket.sql", "111"},
		{"101_app_platform_applications.sql", "101"},
		{"101_a_multi_node_replicas.sql", "101_a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := migrationPrefix(tt.name)
			if got != tt.expected {
				t.Errorf("migrationPrefix(%q) = %q, want %q", tt.name, got, tt.expected)
			}
		})
	}
}

func TestValidateNoDuplicatePrefixes_FreshInstallDeterminism(t *testing.T) {
	// After renaming, all prefixes should be unique across all migration files
	// This simulates a fresh install scan
	files := []string{
		"001_init.sql",
		"002_add_primary_allocation.sql",
		"003_server_manage_fields.sql",
		"004_server_transfer_state.sql",
		"005_server_transfer_lifecycle.sql",
		"006_transfer_run_token.sql",
		"007_postgres_core_foundation.sql",
		"008_server_schedules.sql",
		"009_schedule_run_history.sql",
		"010_node_heartbeat.sql",
		"011_wings_config_install.sql",
		"012_wings_node_parity.sql",
		"013_startup_variables.sql",
		"014_server_databases.sql",
		"015_db_hosts_constraints.sql",
		"015_a_mounts.sql",
		"016_subusers_panel_parity.sql",
		"017_api_keys.sql",
		"018_api_key_scopes.sql",
		"018_a_ssh_2fa_activity.sql",
		"019_backups.sql",
		"020_node_expansion.sql",
		"020_a_regions_multi_node_foundation.sql",
		"021_true_state_persistence.sql",
		"022_evacuation_planner.sql",
		"023_migration_engine.sql",
		"024_observability_foundation.sql",
		"025_heartbeat_expiry_engine.sql",
		"026_placement_reservations.sql",
		"027_recovery_coordinator.sql",
		"028_server_provisioning_parity.sql",
		"032_panel_settings.sql",
		"033_panel_settings_mail_and_advanced.sql",
		"034_user_resource_limits.sql",
		"035_oauth2_clients.sql",
		"036_plugins.sql",
		"037_panel_settings_expansion.sql",
		"038_password_reset_tokens.sql",
		"039_webhooks.sql",
		"040_truthful_server_lifecycle.sql",
		"041_auth_session_security.sql",
		"042_database_provisioning_security.sql",
		"043_unify_eggs_templates_mounts.sql",
		"044_async_delivery_foundation.sql",
		"044_a_cloud_node_links.sql",
		"045_real_server_transfer.sql",
		"046_encrypt_operational_secrets.sql",
		"047_2fa_enhancements.sql",
		"048_s3_backup_config.sql",
		"049_backup_locking.sql",
		"050_subuser_invitations.sql",
		"051_backup_status_tracking.sql",
		"052_schedule_timezone.sql",
		"053_account_sessions.sql",
		"054_activity_events.sql",
		"054_a_social_auth.sql",
		"055_plugin_metadata.sql",
		"056_mail_notification_triggers.sql",
		"057_backup_policies.sql",
		"057_a_job_queue.sql",
		"057_b_webauthn.sql",
		"058_rate_limit_settings.sql",
		"059_server_crash_events.sql",
		"060_region_slug_normalization.sql",
		"077_add_runtime_status_to_nodes.sql",
		"078_node_description.sql",
		"079_evacuation_execution.sql",
		"080_recovery_backup_execution.sql",
		"080_a_recovery_execution_statuses.sql",
		"081_social_provider_configuration.sql",
		"082_deployments.sql",
		"082_a_failover.sql",
		"082_b_target_groups.sql",
		"083_autoscaler.sql",
		"083_a_traffic_rules.sql",
		"084_create_missing_tables.sql",
		"085_node_public_flag.sql",
		"086_add_table_constraints.sql",
		"087_node_parity_fields.sql",
		"087_a_parity_schema.sql",
		"088_server_parity_fields.sql",
		"089_egg_parity_fields.sql",
		"090_allocation_transport.sql",
		"091_seed_minecraft_java.sql",
		"092_durable_operations.sql",
		"093_health_check_history.sql",
		"094_acme_certificates.sql",
		"096_dns_providers.sql",
		"097_git_credentials.sql",
		"098_app_platform_foundations.sql",
		"099_deployment_revisions.sql",
		"100_team_tenancy.sql",
		"101_a_multi_node_replicas.sql",
		"102_a_uncloud_service_model.sql",
		"103_a_deployment_steps.sql",
		"103_b_procedures.sql",
		"104_deployment_health_check_host.sql",
		"105_deployment_version.sql",
		"106_deployment_unique_active.sql",
		"107_deployment_execution_lease.sql",
		"108_compose_stack_indexes.sql",
		"109_add_replica_columns.sql",
		"110_node_fencing.sql",
		"111_routing_rules_websocket.sql",
		"112_cron_jobs.sql",
		"113_app_store.sql",
		"113_a_backup_encryption_compression.sql",
		"114_database_service_plugins.sql",
		"114_a_mtls_certificates.sql",
		"114_b_notifications.sql",
		"114_c_procfile_processes.sql",
		"114_d_source_deployments.sql",
		"114_e_zero_downtime_deploy.sql",
		"114_f_git_deployment_tracking.sql",
		"115_compose_stacks.sql",
		"116_managed_databases.sql",
		"117_domains_certificates.sql",
		"118_multi_tenancy.sql",
		"119_deployments_rollbacks.sql",
		"120_backup_policy_locking.sql",
		"120_a_db_hosts_constraints.sql",
		"121_api_key_scopes.sql",
		"122_node_expansion.sql",
		"123_async_delivery_foundation.sql",
		"124_activity_events.sql",
		"125_backup_policies.sql",
		"126_recovery_backup_execution.sql",
		"127_deployments.sql",
		"128_autoscaler.sql",
		"129_node_parity_fields.sql",
		"130_app_platform_applications.sql",
		"131_reconcile_plans.sql",
		"132_backup_schedules_orchestration.sql",
		"133_backup_encryption_compression_policy.sql",
		"133_a_routing_rules_persistence.sql",
		"134_certificate_dns_provider.sql",
	}
	if err := validateNoDuplicatePrefixes(files); err != nil {
		t.Errorf("fresh install should pass validation, got: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
