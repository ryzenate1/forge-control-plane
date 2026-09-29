# Scope 3 — migrations `098`–`114` (canonical `forge/api/migrations/*.sql`)

## SCOPE

27 canonical files with numeric prefix 098–114 inclusive:

`098_app_platform_foundations, 099_deployment_revisions, 100_team_tenancy,
100_z_app_platform_applications, 101_a_multi_node_replicas,
102_a_uncloud_service_model, 103_a_deployment_steps, 103_b_procedures,
104_a_backup_system*, 104_b_deployment_health_check_host, 105_deployment_version,
106_deployment_unique_active, 107_deployment_execution_lease,
108_compose_stack_indexes, 109_add_replica_columns, 110_node_fencing,
111_routing_rules_websocket, 112_cron_jobs, 113_app_store,
113_a_backup_encryption_compression, 114_database_service_plugins,
114_a_mtls_certificates*, 114_b_notifications, 114_c_procfile_processes,
114_d_source_deployments, 114_e_zero_downtime_deploy,
114_f_git_deployment_tracking`

`*` = `104_a` and `114_a` are ORCHESTRATOR-OWNED (per brief) — reported, never edited.

Dialect files in range (`sqlite/104_a_backup_system.sql`,
`sqlite/111_routing_rules_websocket.sql`, `mysql/111_routing_rules_websocket.sql`)
and `rollbacks/104*_backup_system.down.sql` are READ-ONLY here; findings are routed to
the dialect agents.

Scratch DB `mig_s3`. Harness: `.mig-audit/apply-scope3.sh` (mirrors the runner's real
apply order and wraps each file in one transaction, matching `store.go:1508-1524`).

_Written incrementally. Sections below are appended as verification completes._
