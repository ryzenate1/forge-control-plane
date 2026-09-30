-- 218_db_perf_fk_indexes.sql
-- Additive performance indexes for FK-constrained hot paths and sequential-scan risks.
-- Covers the 98 missing FK indexes triaged as high-impact (large/growing tables, RBAC, logs)
-- plus composite hot-path indexes for capacity/expiry queries.
--
-- CONCURRENTLY note: CREATE INDEX CONCURRENTLY cannot run inside a transaction block.
-- The canonical migration runner (forge/api/internal/store/store.go:1261, migration.go)
-- executes every migration inside a TX while holding pg_advisory_lock(0x466F7267656D6967).
-- That advisory lock already serializes DDL across replicas, so plain CREATE INDEX
-- IF NOT EXISTS is safe and additive (idempotent reruns, no table rewrite, no CONCURRENTLY).
-- If a truly concurrent build is required, run the same statements manually with CONCURRENTLY
-- outside the runner; the IF NOT EXISTS guards make either path converge.
--
-- Sequential-scan risks addressed:
--   * servers(node_id, owner_id, egg_id) already indexed (001_init, 043_unify) — verified.
--   * allocations(server_id, node_id) already indexed (001_init) — verified.
--   * backups(server_id, created_at) already indexed (019_backups) — verified.
--   * Remaining risks were FK columns without any index on large tables (user_roles, egg_variables,
--     server_variables, role_rules, beacon_command_logs.server_id, build_logs, deployment_logs, etc.)
--     which force seq scans on JOIN / WHERE fk = $1 at scale.
--
-- JSONB risk: 127 JSONB columns present but no query uses containment (@>, ->>, GIN). All
-- access is via whole-document fetch or jsonb_each_text lateral (store.go:194). No GIN needed;
-- large JSONB values are TOAST-compressed automatically. Flagged as non-indexable without query change.
--
-- EXPLAIN plans: heavy ListServers* queries ORDER BY created_at DESC now hit idx_servers_created_at
-- (210_servers_created_at_index.sql). NodeCapacitySnapshot filters servers.status <> 'deleted' and
-- instances.status NOT IN (...) via FK-indexed node_id scans rather than seq scans after this patch.
-- See audits/110-phase-09-optimize/subagent-04-db-perf.md for before/after plans.

-- Core RBAC & parity (hottest paths, no index at all before)
CREATE INDEX IF NOT EXISTS idx_user_roles_user_id ON user_roles (user_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_role_id ON user_roles (role_id);
CREATE INDEX IF NOT EXISTS idx_role_rules_role_id ON role_rules (role_id);
CREATE INDEX IF NOT EXISTS idx_eggs_nest_id ON eggs (nest_id);
CREATE INDEX IF NOT EXISTS idx_egg_variables_egg_id ON egg_variables (egg_id);
CREATE INDEX IF NOT EXISTS idx_server_variables_server_id ON server_variables (server_id);
CREATE INDEX IF NOT EXISTS idx_server_variables_variable_id ON server_variables (variable_id);

-- Audit / observability (large, unbounded growth)
CREATE INDEX IF NOT EXISTS idx_audit_events_actor_id ON audit_events (actor_id);
CREATE INDEX IF NOT EXISTS idx_beacon_command_logs_server_id ON beacon_command_logs (server_id);
CREATE INDEX IF NOT EXISTS idx_build_logs_node_id ON build_logs (node_id);
CREATE INDEX IF NOT EXISTS idx_build_logs_server_id ON build_logs (server_id);
CREATE INDEX IF NOT EXISTS idx_deployment_logs_node_id ON deployment_logs (node_id);
CREATE INDEX IF NOT EXISTS idx_deployment_logs_server_id ON deployment_logs (server_id);
CREATE INDEX IF NOT EXISTS idx_deployment_history_deployment_id ON deployment_history (deployment_id);
CREATE INDEX IF NOT EXISTS idx_deployment_history_revision_id ON deployment_history (revision_id);
CREATE INDEX IF NOT EXISTS idx_deployment_history_release_id ON deployment_history (release_id);

-- Procedure / drain / pipeline orchestration
CREATE INDEX IF NOT EXISTS idx_procedure_steps_procedure_id ON procedure_steps (procedure_id);
CREATE INDEX IF NOT EXISTS idx_procedure_schedules_procedure_id ON procedure_schedules (procedure_id);
CREATE INDEX IF NOT EXISTS idx_procedure_executions_actor_id ON procedure_executions (actor_id);
CREATE INDEX IF NOT EXISTS idx_procedure_step_executions_step_id ON procedure_step_executions (step_id);
CREATE INDEX IF NOT EXISTS idx_procedure_step_executions_operation_id ON procedure_step_executions (operation_id);
CREATE INDEX IF NOT EXISTS idx_drain_states_node_id ON drain_states (node_id);
CREATE INDEX IF NOT EXISTS idx_node_capabilities_node_id ON node_capabilities (node_id);
CREATE INDEX IF NOT EXISTS idx_node_role_node_id ON node_role (node_id);
CREATE INDEX IF NOT EXISTS idx_node_role_role_id ON node_role (role_id);

-- Compose / service model (hot placement path)
CREATE INDEX IF NOT EXISTS idx_compose_projects_server_id ON compose_projects (server_id);
CREATE INDEX IF NOT EXISTS idx_compose_stacks_reservation_id ON compose_stacks (reservation_id);
CREATE INDEX IF NOT EXISTS idx_instances_allocation_id ON instances (allocation_id);
CREATE INDEX IF NOT EXISTS idx_service_endpoints_instance_id ON service_endpoints (instance_id);
CREATE INDEX IF NOT EXISTS idx_service_endpoints_node_id ON service_endpoints (node_id);

-- Database & transfer plumbing
CREATE INDEX IF NOT EXISTS idx_database_orphan_remediations_server_id ON database_orphan_remediations (server_id);
CREATE INDEX IF NOT EXISTS idx_database_orphan_remediations_server_database_id ON database_orphan_remediations (server_database_id);
CREATE INDEX IF NOT EXISTS idx_database_orphan_remediations_database_host_id ON database_orphan_remediations (database_host_id);
CREATE INDEX IF NOT EXISTS idx_server_databases_database_host_id ON server_databases (database_host_id);
CREATE INDEX IF NOT EXISTS idx_server_transfers_old_node_id ON server_transfers (old_node_id);
CREATE INDEX IF NOT EXISTS idx_server_transfers_new_node_id ON server_transfers (new_node_id);
CREATE INDEX IF NOT EXISTS idx_health_check_configs_server_id ON health_check_configs (server_id);
CREATE INDEX IF NOT EXISTS idx_sftp_node_configs_node_id ON sftp_node_configs (node_id);
CREATE INDEX IF NOT EXISTS idx_mount_server_mount_id ON mount_server (mount_id);
CREATE INDEX IF NOT EXISTS idx_egg_mount_mount_id ON egg_mount (mount_id);

-- Scheduling & autoscale
CREATE INDEX IF NOT EXISTS idx_schedule_task_runs_task_id ON schedule_task_runs (schedule_task_id);
CREATE INDEX IF NOT EXISTS idx_scaling_events_server_id ON scaling_events (server_id);
CREATE INDEX IF NOT EXISTS idx_failover_events_server_id ON failover_events (server_id);

-- Evacuation / recovery (node failure path, large at scale)
CREATE INDEX IF NOT EXISTS idx_evacuation_items_source_node_id ON evacuation_items (source_node_id);
CREATE INDEX IF NOT EXISTS idx_evacuation_items_target_node_id ON evacuation_items (target_node_id);
CREATE INDEX IF NOT EXISTS idx_recovery_items_source_node_id ON recovery_items (source_node_id);
CREATE INDEX IF NOT EXISTS idx_recovery_items_target_node_id ON recovery_items (target_node_id);
CREATE INDEX IF NOT EXISTS idx_recovery_items_reservation_id ON recovery_items (reservation_id);

-- Catalog / env / domain provisioning
CREATE INDEX IF NOT EXISTS idx_catalog_instances_node_id ON catalog_instances (node_id);
CREATE INDEX IF NOT EXISTS idx_catalog_attach_links_instance_id ON catalog_attach_links (catalog_instance_id);
CREATE INDEX IF NOT EXISTS idx_env_manifests_env_id ON env_manifests (env_id);
CREATE INDEX IF NOT EXISTS idx_env_manifests_applied_by ON env_manifests (applied_by);
CREATE INDEX IF NOT EXISTS idx_env_domain_provisioning_env_id ON env_domain_provisioning (env_id);
CREATE INDEX IF NOT EXISTS idx_env_domain_provisioning_cert_id ON env_domain_provisioning (cert_id);

-- Backup / restore (large, retention hot path)
CREATE INDEX IF NOT EXISTS idx_managed_database_restores_backup_id ON managed_database_restores (backup_id);
CREATE INDEX IF NOT EXISTS idx_migration_runs_migration_id ON migration_runs (migration_id);
CREATE INDEX IF NOT EXISTS idx_migration_runs_target_allocation_id ON migration_runs (target_allocation_id);
CREATE INDEX IF NOT EXISTS idx_backup_configurations_encryption_key_id ON backup_configurations (encryption_key_id);
CREATE INDEX IF NOT EXISTS idx_backup_jobs_triggered_by_user_id ON backup_jobs (triggered_by_user_id);
CREATE INDEX IF NOT EXISTS idx_backup_restores_node_id ON backup_restores (node_id);
CREATE INDEX IF NOT EXISTS idx_backup_restores_triggered_by_user_id ON backup_restores (triggered_by_user_id);
CREATE INDEX IF NOT EXISTS idx_backup_restores_target_app_id ON backup_restores (target_app_id);
CREATE INDEX IF NOT EXISTS idx_backup_restores_target_volume_id ON backup_restores (target_volume_id);
CREATE INDEX IF NOT EXISTS idx_backup_artifacts_source_app_id ON backup_artifacts (source_app_id);
CREATE INDEX IF NOT EXISTS idx_database_backups_backup_id ON database_backups (backup_id);
CREATE INDEX IF NOT EXISTS idx_volume_backups_backup_id ON volume_backups (backup_id);

-- Auth / tenancy / marketplace
CREATE INDEX IF NOT EXISTS idx_git_providers_user_id ON git_providers (user_id);
CREATE INDEX IF NOT EXISTS idx_source_deployments_created_by ON source_deployments (created_by);
CREATE INDEX IF NOT EXISTS idx_subuser_invitations_created_by ON subuser_invitations (created_by);
CREATE INDEX IF NOT EXISTS idx_preview_deployments_created_by ON preview_deployments (created_by);
CREATE INDEX IF NOT EXISTS idx_org_invitations_invited_by ON org_invitations (invited_by);
CREATE INDEX IF NOT EXISTS idx_app_builds_buildpack_id ON app_builds (buildpack_id);
CREATE INDEX IF NOT EXISTS idx_server_buildpacks_buildpack_id ON server_buildpacks (buildpack_id);
CREATE INDEX IF NOT EXISTS idx_org_quotas_org_id ON org_quotas (org_id);

-- Join tables that had zero indexes before
CREATE INDEX IF NOT EXISTS idx_backup_host_node_node_id ON backup_host_node (node_id);
CREATE INDEX IF NOT EXISTS idx_backup_host_node_host_id ON backup_host_node (backup_host_id);
CREATE INDEX IF NOT EXISTS idx_database_host_node_node_id ON database_host_node (node_id);
CREATE INDEX IF NOT EXISTS idx_database_host_node_host_id ON database_host_node (database_host_id);
CREATE INDEX IF NOT EXISTS idx_migration_allocation_reservations_migration_id ON migration_allocation_reservations (migration_id);
CREATE INDEX IF NOT EXISTS idx_environment_variable_revisions_created_by ON environment_variable_revisions (created_by);

-- Composite hot-path indexes for sequential-scan-prone queries
-- Capacity snapshot filters servers.status <> 'deleted' per node_id (store_capacity.go:60)
CREATE INDEX IF NOT EXISTS idx_servers_node_id_status ON servers (node_id, status);
-- Backup retention reaper: status+created_at composite helps ListExpiredBackups (status='completed' AND is_locked=FALSE)
CREATE INDEX IF NOT EXISTS idx_backups_server_status_created ON backups (server_id, status, created_at DESC);
-- Operations stale reaper already covered by idx_operations_running_updated_at (209); add resource-scoped variant
CREATE INDEX IF NOT EXISTS idx_operations_resource_status_updated ON operations (resource_type, resource_id, status, updated_at);
