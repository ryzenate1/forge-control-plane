-- Explicit rollback audit trail.
--
-- A `deployments` row records what one deploy attempted. It does not record
-- that an operator chose to move a workload back to an older version, which
-- version was live when they chose it, or why. Those are different facts with a
-- different lifetime, so they get their own table instead of more nullable
-- columns on `deployments` (Dokploy's `rollback` table plays the same role: it
-- carries the context a rollback was executed with).
--
--   deployment_id                the version being rolled back TO
--   triggered_by_deployment_id   the version that was current/failed when the
--                                rollback was requested
--   rollback_deployment_id       the strategy='rollback' deployment row created
--                                to mirror the rollback in the browsable
--                                history. Nullable on purpose: the rollback can
--                                succeed while the mirror row is refused by
--                                idx_unique_active_deployment_per_server, and a
--                                lost history entry must never be recorded as a
--                                failed rollback.
--   initiated_by                 the user who asked for it (NULL for the
--                                automated health-gate rollback path)
--   snapshot                     compose/env context captured at request time,
--                                so a later diff does not depend on state that
--                                keeps mutating afterwards
--
-- Additive only: no existing table is altered and no existing writer changes.
-- The pre-existing `rollbacks` table (119) keys off `deployment_history`, not
-- `deployments`, so it cannot serve this surface.

CREATE TABLE IF NOT EXISTS deployment_rollbacks (
    id UUID PRIMARY KEY,
    deployment_id UUID NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    triggered_by_deployment_id UUID REFERENCES deployments(id) ON DELETE SET NULL,
    rollback_deployment_id UUID REFERENCES deployments(id) ON DELETE SET NULL,
    application_id UUID REFERENCES applications(id) ON DELETE SET NULL,
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    initiated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'in_progress', 'completed', 'failed', 'cancelled')),
    reason TEXT NOT NULL DEFAULT '',
    snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

-- History is read per workload, newest first.
CREATE INDEX IF NOT EXISTS idx_deployment_rollbacks_server_created
    ON deployment_rollbacks (server_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_deployment_rollbacks_app_created
    ON deployment_rollbacks (application_id, created_at DESC)
    WHERE application_id IS NOT NULL;
-- "What was this version rolled back from?" and "is a rollback still running?"
CREATE INDEX IF NOT EXISTS idx_deployment_rollbacks_deployment
    ON deployment_rollbacks (deployment_id);
CREATE INDEX IF NOT EXISTS idx_deployment_rollbacks_status
    ON deployment_rollbacks (status)
    WHERE status IN ('pending', 'in_progress');
