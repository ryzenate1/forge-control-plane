-- Plugin metadata columns for the enhanced plugin system.
-- Adds structured metadata columns alongside the existing manifest JSONB.

ALTER TABLE plugins ADD COLUMN IF NOT EXISTS author text DEFAULT '';
ALTER TABLE plugins ADD COLUMN IF NOT EXISTS license text DEFAULT '';
ALTER TABLE plugins ADD COLUMN IF NOT EXISTS homepage text DEFAULT '';
ALTER TABLE plugins ADD COLUMN IF NOT EXISTS dependencies jsonb DEFAULT '{}';
ALTER TABLE plugins ADD COLUMN IF NOT EXISTS min_version text DEFAULT '';
ALTER TABLE plugins ADD COLUMN IF NOT EXISTS max_version text DEFAULT '';
ALTER TABLE plugins ADD COLUMN IF NOT EXISTS hooks jsonb DEFAULT '{}';
ALTER TABLE plugins ADD COLUMN IF NOT EXISTS state text DEFAULT 'installed';
ALTER TABLE plugins ADD COLUMN IF NOT EXISTS error_message text DEFAULT '';
ALTER TABLE plugins ADD COLUMN IF NOT EXISTS settings jsonb DEFAULT 'null'::jsonb;
