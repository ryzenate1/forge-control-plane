# SCOPE-1 migration audit — canonical `*.sql` 001–042 (46 files)

WORK IN PROGRESS — sections filled incrementally per the brief's Restart Protocol.

## SCOPE

Canonical `*.sql` files in `/Users/riyaz/forge-plane/forge-test/forge/api/migrations/` with
numeric prefix 001–042 inclusive — 46 files:

001_init, 002_add_primary_allocation, 003_server_manage_fields, 004_server_transfer_state,
005_server_transfer_lifecycle, 006_transfer_run_token, 007_postgres_core_foundation,
008_server_schedules, 009_schedule_run_history, 010_node_heartbeat, 011_wings_config_install,
012_wings_node_parity, 013_startup_variables, 014_server_databases,
015_a_mounts, 015_db_hosts_constraints(guard), 015_mounts(guard), 016_subusers_panel_parity,
017_api_keys, 018_a_ssh_2fa_activity, 018_api_key_scopes(guard), 018_ssh_2fa_activity(guard),
019_backups, 020_a_regions_multi_node_foundation, 020_node_expansion(guard),
020_regions_multi_node_foundation(guard), 021_true_state_persistence, 022_evacuation_planner,
023_migration_engine, 024_observability_foundation, 025_heartbeat_expiry_engine,
026_placement_reservations, 027_recovery_coordinator, 028_server_provisioning_parity,
032_panel_settings, 033_panel_settings_mail_and_advanced, 034_user_resource_limits,
035_oauth2_clients, 036_plugins, 037_panel_settings_expansion, 038_password_reset_tokens,
039_webhooks, 040_truthful_server_lifecycle, 041_a_placement_intents,
041_auth_session_security, 042_database_provisioning_security.

Retired slots 029/030/031 confirmed absent from the canonical dir.
Scratch DB: `mig_s1` (Postgres 16.15 @127.0.0.1:5432, role `gamepanel`, trust auth).
`mig_audit_schema` used read-only as the reference for whole-stream state.

## PER-FILE LEDGER

(pending)

## FIXES APPLIED

(pending)

## NEEDS NEW MIGRATION

(pending)

## NEEDS OTHER AGENTS' FILES

(pending)

## VERIFICATION

(pending)

## STATUS

STATUS: partial — remaining: full per-file sweep, double-apply verification, Go-side CHECK-set greps
