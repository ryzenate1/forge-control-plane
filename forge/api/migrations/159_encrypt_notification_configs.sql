ALTER TABLE notification_channels
    ADD COLUMN IF NOT EXISTS config_encrypted TEXT NOT NULL DEFAULT '';
