-- Uncloud-inspired service model: extends app_services with mode, update
-- strategy, health checks, resource limits, volume/secret references, and
-- links to the Agent 03 replica/placement model. Also introduces the
-- service_endpoints table for computed reachability information.

ALTER TABLE app_services
  ADD COLUMN IF NOT EXISTS mode TEXT NOT NULL DEFAULT 'replicated'
    CHECK (mode IN ('replicated', 'global'));

ALTER TABLE app_services
  ADD COLUMN IF NOT EXISTS update_config JSONB NOT NULL
    DEFAULT '{"strategy":"rolling","rollingOrder":"start-first","monitorPeriod":30}'::jsonb;

ALTER TABLE app_services
  ADD COLUMN IF NOT EXISTS health_check JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE app_services
  ADD COLUMN IF NOT EXISTS resources JSONB NOT NULL
    DEFAULT '{"cpu":1024,"memoryMb":2048,"diskMb":10240}'::jsonb;

ALTER TABLE app_services
  ADD COLUMN IF NOT EXISTS volumes JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE app_services
  ADD COLUMN IF NOT EXISTS secrets JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE app_services
  ADD COLUMN IF NOT EXISTS replica_app_id UUID
    REFERENCES replica_applications(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_app_services_replica_app
  ON app_services (replica_app_id);

-- Service endpoints: computed reachability for each service instance.
CREATE TABLE IF NOT EXISTS service_endpoints (
    id UUID PRIMARY KEY,
    service_id UUID NOT NULL REFERENCES app_services(id) ON DELETE CASCADE,
    host TEXT NOT NULL DEFAULT '',
    port INTEGER NOT NULL DEFAULT 0,
    protocol TEXT NOT NULL DEFAULT 'tcp',
    node_id UUID REFERENCES nodes(id),
    instance_id UUID REFERENCES instances(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_service_endpoints_service
  ON service_endpoints (service_id);
