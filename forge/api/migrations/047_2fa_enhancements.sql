-- Enhance 2FA enforcement with configurable policy levels
-- This migration adds check constraint for require_2fa TEXT values

-- Add check constraint for valid values (if not exists)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'require_2fa_valid_values'
    ) THEN
        ALTER TABLE panel_settings
        ADD CONSTRAINT require_2fa_valid_values
        CHECK (require_2fa IN ('none', 'admin', 'all'));
    END IF;
END $$;

-- Add comment
COMMENT ON COLUMN panel_settings.require_2fa IS '2FA requirement policy: none (no requirement), admin (require for admin users only), all (require for all users)';
