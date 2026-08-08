-- Add public visibility flag to nodes for auto-deploy eligibility.
ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS public BOOLEAN NOT NULL DEFAULT true;
