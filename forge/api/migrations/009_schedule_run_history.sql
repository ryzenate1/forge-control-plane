-- Schedule execution history and task-level outcomes.

CREATE TABLE IF NOT EXISTS schedule_runs (
    id UUID PRIMARY KEY,
    schedule_id UUID NOT NULL REFERENCES server_schedules(id) ON DELETE CASCADE,
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'success', 'failed', 'partial', 'skipped')),
    trigger TEXT NOT NULL DEFAULT 'scheduler',
    error TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS schedule_runs_schedule_id_idx ON schedule_runs (schedule_id, started_at DESC);
CREATE INDEX IF NOT EXISTS schedule_runs_server_id_idx ON schedule_runs (server_id, started_at DESC);

CREATE TABLE IF NOT EXISTS schedule_task_runs (
    id UUID PRIMARY KEY,
    schedule_run_id UUID NOT NULL REFERENCES schedule_runs(id) ON DELETE CASCADE,
    schedule_task_id UUID NOT NULL REFERENCES schedule_tasks(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('success', 'failed', 'skipped')),
    error TEXT,
    executed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS schedule_task_runs_schedule_run_id_idx ON schedule_task_runs (schedule_run_id, executed_at ASC);
