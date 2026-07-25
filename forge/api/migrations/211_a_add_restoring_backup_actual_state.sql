-- 211_add_restoring_backup_actual_state: ensure server_actual_state enum supports restoring_backup lock
-- The initial 021 enum only had 7 values; store.go defines RestoringBackup, offline/terminating
-- but the enum was never extended. Without this value SetServerActualState('restoring_backup')
-- fails with "invalid input value for enum". This is unconditional security lock (GH-09 P1).

DO $$
BEGIN
    -- Add restoring_backup if missing
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum e
        JOIN pg_type t ON t.oid = e.enumtypid
        WHERE t.typname = 'server_actual_state' AND e.enumlabel = 'restoring_backup'
    ) THEN
        ALTER TYPE server_actual_state ADD VALUE 'restoring_backup';
    END IF;

    -- Also add other constants defined in store.go but missing from enum to avoid future failures
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum e
        JOIN pg_type t ON t.oid = e.enumtypid
        WHERE t.typname = 'server_actual_state' AND e.enumlabel = 'offline'
    ) THEN
        ALTER TYPE server_actual_state ADD VALUE 'offline';
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_enum e
        JOIN pg_type t ON t.oid = e.enumtypid
        WHERE t.typname = 'server_actual_state' AND e.enumlabel = 'terminating'
    ) THEN
        ALTER TYPE server_actual_state ADD VALUE 'terminating';
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_enum e
        JOIN pg_type t ON t.oid = e.enumtypid
        WHERE t.typname = 'server_actual_state' AND e.enumlabel = 'terminated'
    ) THEN
        ALTER TYPE server_actual_state ADD VALUE 'terminated';
    END IF;
END $$;
