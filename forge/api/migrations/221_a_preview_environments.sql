-- 221_a_preview_environments.sql
-- Project-scoped ephemeral preview deployments (Dokploy preview-deployment /
-- CapRover webhook-triggered auto-deploy style).
--
-- WHY A NEW TABLE AND NOT preview_deployments
-- -------------------------------------------
-- preview_deployments (migration 119, extended by 180 and 216) is already live
-- and already carries a *different*, incompatible model: it is server-scoped
-- (server_id NOT NULL), always PR-numbered, its status CHECK enumerates
-- (deploying, running, stopped, failed, cleaned_up) and it has a partial unique
-- index enforcing one live row per (pr_number, repo_owner, repo_name). The
-- feature below is project-scoped, addresses branches as well as PRs, keeps the
-- deployed compose document, and uses the pending/deploying/active/expired/
-- failed/teardown vocabulary. Reusing that name would mean either breaking a
-- shipped CHECK/unique index or mutating rows the legacy previewenv lifecycle
-- and the /admin/preview-deployments API already own. preview_environments is
-- therefore a separate, additive table with its own service surface
-- (internal/services/previewenv previews.go / preview_*.go).

CREATE TABLE IF NOT EXISTS preview_environments (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    environment_id UUID REFERENCES environments(id) ON DELETE SET NULL,
    server_id UUID REFERENCES servers(id) ON DELETE SET NULL,

    -- Exactly one of branch/pr_number identifies the source unit; branch is
    -- always present because the subdomain slug is derived from it.
    branch VARCHAR NOT NULL,
    pr_number INT,
    commit_sha VARCHAR NOT NULL DEFAULT '',

    -- Subdomain label under the preview base domain, globally unique so two
    -- projects can never silently share one preview hostname.
    slug VARCHAR UNIQUE NOT NULL,

    status VARCHAR NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'deploying', 'active', 'expired', 'failed', 'teardown')),

    url TEXT NOT NULL DEFAULT '',
    compose_content TEXT NOT NULL DEFAULT '',

    -- The compose stack that actually runs the preview. Teardown is impossible
    -- without it, so it is part of the schema even though the brief lists only
    -- the columns above.
    stack_id TEXT NOT NULL DEFAULT '',
    source VARCHAR NOT NULL DEFAULT 'manual',
    title TEXT NOT NULL DEFAULT '',
    pr_url TEXT NOT NULL DEFAULT '',
    close_reason TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',

    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS preview_environments_project_idx ON preview_environments (project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS preview_environments_status_idx ON preview_environments (status);
CREATE INDEX IF NOT EXISTS preview_environments_expiry_idx ON preview_environments (expires_at, status);
-- One live preview per branch within a project; a closed/failed preview frees
-- the slot so the same branch can be re-opened later.
CREATE UNIQUE INDEX IF NOT EXISTS preview_environments_branch_unique
    ON preview_environments (project_id, lower(branch))
    WHERE status IN ('pending', 'deploying', 'active');

-- Per-project webhook signing secret. The public preview webhook must know
-- which project an event belongs to before it trusts anything, and the HMAC
-- key has to be scoped to that project; projects is the only sane home.
-- Empty means "not provisioned yet" — the route 403s instead of accepting
-- unsigned or wrongly-signed calls.
ALTER TABLE projects ADD COLUMN IF NOT EXISTS preview_webhook_secret TEXT NOT NULL DEFAULT '';
