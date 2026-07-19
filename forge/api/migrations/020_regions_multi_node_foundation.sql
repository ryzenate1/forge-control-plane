CREATE TABLE IF NOT EXISTS regions (
    id UUID PRIMARY KEY,
    uuid UUID NOT NULL UNIQUE,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    description TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO regions (id, uuid, name, slug, description, enabled, created_at, updated_at)
SELECT l.id, l.id, l.long,
       COALESCE(
           NULLIF(TRIM(BOTH '-' FROM REGEXP_REPLACE(LOWER(l.short), '[^a-z0-9]+', '-', 'g')), ''),
           'region-' || REPLACE(l.id::text, '-', '')
       ),
       'Backfilled from existing location ' || l.short, TRUE, l.created_at, l.created_at
FROM locations l
ON CONFLICT (slug) DO NOTHING;

INSERT INTO regions (id, uuid, name, slug, description, enabled)
VALUES (
    'eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee',
    'eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee',
    'Local Lab',
    'local',
    'Default local region',
    TRUE
)
ON CONFLICT (slug) DO NOTHING;

ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS region_id UUID REFERENCES regions(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS draining BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE nodes n
SET region_id = COALESCE(
    n.region_id,
    (SELECT r.id FROM regions r
     WHERE r.slug = COALESCE(
         NULLIF(TRIM(BOTH '-' FROM REGEXP_REPLACE(LOWER(n.region), '[^a-z0-9]+', '-', 'g')), ''),
         'region-' || REPLACE(n.location_id::text, '-', '')
     )
     LIMIT 1),
    (SELECT r.id FROM regions r WHERE r.id = n.location_id LIMIT 1),
    (SELECT r.id FROM regions r WHERE r.slug = 'local' LIMIT 1)
)
WHERE n.region_id IS NULL;

CREATE INDEX IF NOT EXISTS nodes_region_id_idx ON nodes (region_id);
CREATE INDEX IF NOT EXISTS nodes_status_region_idx ON nodes (status, region_id);
