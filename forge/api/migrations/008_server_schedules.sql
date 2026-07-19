-- Server schedules and schedule tasks

CREATE TABLE IF NOT EXISTS server_schedules (
    id UUID PRIMARY KEY,
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    cron_minute TEXT NOT NULL DEFAULT '*',
    cron_hour TEXT NOT NULL DEFAULT '*',
    cron_day_of_month TEXT NOT NULL DEFAULT '*',
    cron_month TEXT NOT NULL DEFAULT '*',
    cron_day_of_week TEXT NOT NULL DEFAULT '*',
    only_when_online BOOLEAN NOT NULL DEFAULT FALSE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_run_at TIMESTAMPTZ,
    next_run_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS server_schedules_server_id_idx ON server_schedules (server_id, created_at DESC);

CREATE TABLE IF NOT EXISTS schedule_tasks (
    id UUID PRIMARY KEY,
    schedule_id UUID NOT NULL REFERENCES server_schedules(id) ON DELETE CASCADE,
    sequence INTEGER NOT NULL,
    action TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    time_offset_seconds INTEGER NOT NULL DEFAULT 0,
    continue_on_failure BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT schedule_tasks_sequence_positive CHECK (sequence > 0),
    CONSTRAINT schedule_tasks_time_offset_non_negative CHECK (time_offset_seconds >= 0),
    CONSTRAINT schedule_tasks_unique_sequence UNIQUE (schedule_id, sequence)
);

CREATE INDEX IF NOT EXISTS schedule_tasks_schedule_id_idx ON schedule_tasks (schedule_id, sequence ASC);
