ALTER TYPE migration_status ADD VALUE IF NOT EXISTS 'pending';
ALTER TYPE migration_status ADD VALUE IF NOT EXISTS 'in_progress';
ALTER TYPE migration_status ADD VALUE IF NOT EXISTS 'cancelling';
