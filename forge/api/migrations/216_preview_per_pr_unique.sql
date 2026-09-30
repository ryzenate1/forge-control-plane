-- 216_preview_per_pr_unique.sql
-- Partial unique index enforcing one active preview per PR.
-- Closes the race in previewenv/service.go:144 where ListActivePreviewDeployments scan
-- is non-atomic; two concurrent inserts with same pr_number+repo_owner+repo_name can
-- both pass the scan and create duplicate active rows. The DB index makes this
-- impossible and the service maps the unique violation to ErrAlreadyExists.
-- cleaned_up and failed are excluded so a new preview can be created after cleanup.

CREATE UNIQUE INDEX IF NOT EXISTS idx_preview_deployments_pr_unique
    ON preview_deployments (pr_number, lower(repo_owner), lower(repo_name))
    WHERE status IN ('deploying', 'running', 'stopped');

-- Backfill TTL for legacy rows that were created via the old preview/service.go:42
-- path (no expires_at). Without this the reaper (reaper.go:60 ListExpired) would
-- never find them because expires_at IS NULL. Use the default 24h TTL so every
-- live preview eventually expires and the partial index above can be exercised
-- without manual intervention.
UPDATE preview_deployments
    SET expires_at = created_at + INTERVAL '24 hours'
    WHERE expires_at IS NULL
      AND status IN ('deploying', 'running', 'stopped');
