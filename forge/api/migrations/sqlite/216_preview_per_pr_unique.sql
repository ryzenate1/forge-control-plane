-- 216_preview_per_pr_unique.sql (sqlite dialect)
-- Partial unique index enforcing one active preview per PR.
-- SQLite variant of the postgres migration: uses datetime() for backfill.

CREATE UNIQUE INDEX IF NOT EXISTS idx_preview_deployments_pr_unique
    ON preview_deployments (pr_number, lower(repo_owner), lower(repo_name))
    WHERE status IN ('deploying', 'running', 'stopped');

UPDATE preview_deployments
    SET expires_at = datetime(created_at, '+24 hours')
    WHERE expires_at IS NULL
      AND status IN ('deploying', 'running', 'stopped');
