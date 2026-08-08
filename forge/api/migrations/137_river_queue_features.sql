-- River queue features for job_queue table

ALTER TABLE job_queue ADD COLUMN IF NOT EXISTS errors jsonb DEFAULT '[]'::jsonb;
ALTER TABLE job_queue ADD COLUMN IF NOT EXISTS tags text[] DEFAULT '{}';
ALTER TABLE job_queue ADD COLUMN IF NOT EXISTS scheduled_at timestamptz;
ALTER TABLE job_queue ADD COLUMN IF NOT EXISTS attempt integer NOT NULL DEFAULT 0;
ALTER TABLE job_queue ADD COLUMN IF NOT EXISTS metadata jsonb DEFAULT '{}'::jsonb;
ALTER TABLE job_queue ADD COLUMN IF NOT EXISTS queue_name text NOT NULL DEFAULT 'default';

CREATE UNIQUE INDEX IF NOT EXISTS idx_job_queue_unique
    ON job_queue(queue_name, type, COALESCE(server_id::text, ''), COALESCE(node_id::text, ''))
    WHERE status IN ('pending', 'running');

CREATE INDEX IF NOT EXISTS idx_job_queue_scheduled_at
    ON job_queue(scheduled_at) WHERE scheduled_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_job_queue_queue_name
    ON job_queue(queue_name, status);

CREATE TABLE IF NOT EXISTS river_queue (
    id bigserial PRIMARY KEY,
    name text NOT NULL UNIQUE,
    metadata jsonb DEFAULT '{}'::jsonb,
    paused_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_river_queue_paused
    ON river_queue(paused_at) WHERE paused_at IS NOT NULL;
