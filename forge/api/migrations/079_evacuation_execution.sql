ALTER TYPE evacuation_plan_status ADD VALUE IF NOT EXISTS 'cancelled';

ALTER TABLE evacuation_items
    ADD COLUMN IF NOT EXISTS migration_id UUID REFERENCES migrations(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS error TEXT;

CREATE INDEX IF NOT EXISTS evacuation_items_migration_idx ON evacuation_items (migration_id);
