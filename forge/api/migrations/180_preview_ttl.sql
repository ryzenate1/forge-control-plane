-- Phase 4: preview environment lifecycle (TTL + row reaping).
-- Expires_at plans an expiry timestamp for each preview so the reaper can
-- auto-destroy both live (deploying/running/stopped) and retained
-- cleaned_up rows without touching webhook delivery.
ALTER TABLE preview_deployments
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_preview_deployments_expiry
    ON preview_deployments (expires_at, status);

CREATE INDEX IF NOT EXISTS idx_preview_deployments_cleaned
    ON preview_deployments (cleaned_at) WHERE status = 'cleaned_up';