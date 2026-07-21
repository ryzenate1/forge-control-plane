CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_active_deployment_per_server
ON deployments (server_id)
WHERE status IN ('pending', 'in_progress', 'provisioning', 'awaiting_health', 'promoting', 'rollback_pending', 'rolling_back');
