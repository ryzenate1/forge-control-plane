-- Secure database provisioning lifecycle and transport configuration.
ALTER TABLE database_hosts
    ADD COLUMN IF NOT EXISTS tls_mode TEXT,
    ADD COLUMN IF NOT EXISTS tls_ca TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS tls_server_name TEXT NOT NULL DEFAULT '';

-- Existing hosts historically connected without TLS. Preserve that behavior
-- explicitly; all newly-created hosts receive the secure verify-full default.
UPDATE database_hosts SET tls_mode = 'disable' WHERE tls_mode IS NULL;
ALTER TABLE database_hosts
    ALTER COLUMN tls_mode SET DEFAULT 'verify-full',
    ALTER COLUMN tls_mode SET NOT NULL;

UPDATE database_hosts SET engine = 'postgresql' WHERE engine = 'postgres';
UPDATE database_hosts SET engine = 'mysql' WHERE engine = 'mariadb';

ALTER TABLE database_hosts
    DROP CONSTRAINT IF EXISTS database_hosts_engine_check,
    ADD CONSTRAINT database_hosts_engine_check CHECK (engine IN ('postgresql', 'mysql')),
    DROP CONSTRAINT IF EXISTS database_hosts_tls_mode_check,
    ADD CONSTRAINT database_hosts_tls_mode_check CHECK (tls_mode IN ('disable', 'required', 'verify-ca', 'verify-full'));

ALTER TABLE server_databases
    ADD COLUMN IF NOT EXISTS provisioning_state TEXT NOT NULL DEFAULT 'ready',
    ADD COLUMN IF NOT EXISTS provisioning_error TEXT NOT NULL DEFAULT '';

ALTER TABLE server_databases
    DROP CONSTRAINT IF EXISTS server_databases_provisioning_state_check,
    ADD CONSTRAINT server_databases_provisioning_state_check
        CHECK (provisioning_state IN ('pending', 'ready', 'failed'));

CREATE TABLE IF NOT EXISTS database_orphan_remediations (
    id UUID PRIMARY KEY,
    server_database_id UUID NOT NULL,
    server_id UUID NOT NULL,
    database_host_id UUID NOT NULL,
    engine TEXT NOT NULL,
    host TEXT NOT NULL,
    port INTEGER NOT NULL,
    database_name TEXT NOT NULL,
    username TEXT NOT NULL,
    remote TEXT NOT NULL,
    reason TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'resolved')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS database_orphan_remediations_status_idx
    ON database_orphan_remediations (status, created_at);
