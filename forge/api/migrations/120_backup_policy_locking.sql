-- Add lock support for backup policies
ALTER TABLE backup_policies
    ADD COLUMN IF NOT EXISTS is_locked BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN backup_policies.is_locked IS 'Prevents the policy from being deleted or modified when locked';
