-- 220_add_egg_install_steps.sql — additive typed install pipeline (puffer 24 ops -> shell fallback + typed steps)
-- Keeps shell blob (install_script/container/entrypoint) for simple games; adds optional typed pipeline.
-- Beacon: when egg.install_steps is present and non-empty array, typed interpreter runs; else fallback to shell script.
-- This migration is additive and idempotent — existing installs not affected.

ALTER TABLE eggs ADD COLUMN IF NOT EXISTS install_steps JSONB NOT NULL DEFAULT '[]'::jsonb;

COMMENT ON COLUMN eggs.install_steps IS 'Optional typed install pipeline (JSON array of {type, args, condition}) — when non-empty, beacon runs typed interpreter; else falls back to install_script shell blob. Additive; existing shell installs unaffected.';
