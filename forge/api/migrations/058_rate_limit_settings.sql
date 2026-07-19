CREATE TABLE IF NOT EXISTS panel_rate_limit_settings (
    id BOOLEAN PRIMARY KEY DEFAULT TRUE,
    settings JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT panel_rate_limit_settings_single CHECK (id)
);

INSERT INTO panel_rate_limit_settings (id) VALUES (TRUE)
ON CONFLICT (id) DO NOTHING;
