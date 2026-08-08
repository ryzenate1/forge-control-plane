-- Per-user resource limits.
-- 0 means "unlimited". The frontend uses these in the user create/edit form
-- and the backend enforces them on server create / backup create / database
-- create / allocation create / subuser add / schedule create.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS cpu_limit INTEGER NOT NULL DEFAULT 0 CHECK (cpu_limit >= 0),
    ADD COLUMN IF NOT EXISTS memory_mb_limit INTEGER NOT NULL DEFAULT 0 CHECK (memory_mb_limit >= 0),
    ADD COLUMN IF NOT EXISTS disk_mb_limit INTEGER NOT NULL DEFAULT 0 CHECK (disk_mb_limit >= 0),
    ADD COLUMN IF NOT EXISTS backup_limit INTEGER NOT NULL DEFAULT 0 CHECK (backup_limit >= 0),
    ADD COLUMN IF NOT EXISTS database_limit INTEGER NOT NULL DEFAULT 0 CHECK (database_limit >= 0),
    ADD COLUMN IF NOT EXISTS allocation_limit INTEGER NOT NULL DEFAULT 0 CHECK (allocation_limit >= 0),
    ADD COLUMN IF NOT EXISTS subuser_limit INTEGER NOT NULL DEFAULT 0 CHECK (subuser_limit >= 0),
    ADD COLUMN IF NOT EXISTS schedule_limit INTEGER NOT NULL DEFAULT 0 CHECK (schedule_limit >= 0),
    ADD COLUMN IF NOT EXISTS server_limit INTEGER NOT NULL DEFAULT 0 CHECK (server_limit >= 0);