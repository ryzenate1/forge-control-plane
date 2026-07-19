ALTER TYPE recovery_plan_status ADD VALUE IF NOT EXISTS 'executing';
ALTER TYPE recovery_plan_status ADD VALUE IF NOT EXISTS 'completed';

ALTER TYPE recovery_item_status ADD VALUE IF NOT EXISTS 'executing';
ALTER TYPE recovery_item_status ADD VALUE IF NOT EXISTS 'completed';
ALTER TYPE recovery_item_status ADD VALUE IF NOT EXISTS 'skipped';
