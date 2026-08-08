-- Persist operator documentation entered during node registration.
ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';
