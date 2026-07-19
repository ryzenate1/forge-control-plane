-- Add timezone column to server_schedules table
ALTER TABLE server_schedules
ADD COLUMN IF NOT EXISTS timezone VARCHAR(64) DEFAULT 'UTC';

-- Update existing server_schedules to have UTC timezone
UPDATE server_schedules SET timezone = 'UTC' WHERE timezone IS NULL;
