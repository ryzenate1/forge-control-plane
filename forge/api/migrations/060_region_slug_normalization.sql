-- Canonicalize regions created before the slug contract was enforced.
-- Stage values first so canonicalization can safely resolve case/punctuation collisions.
-- Fresh installs receive this generated name from the original table definition.
ALTER TABLE regions DROP CONSTRAINT IF EXISTS regions_slug_check;

UPDATE regions
SET slug = slug || '__region_slug_migration__' || REPLACE(id::text, '-', '');

WITH normalized AS (
    SELECT
        id,
        COALESCE(
            NULLIF(TRIM(BOTH '-' FROM REGEXP_REPLACE(
                LOWER(REGEXP_REPLACE(slug, '__region_slug_migration__[0-9a-f-]+$', '')),
                '[^a-z0-9]+', '-', 'g'
            )), ''),
            'region-' || REPLACE(id::text, '-', '')
        ) AS base_slug
    FROM regions
), ranked AS (
    SELECT
        id,
        base_slug || CASE
            WHEN COUNT(*) OVER (PARTITION BY base_slug) > 1
                THEN '-' || ROW_NUMBER() OVER (PARTITION BY base_slug ORDER BY id)
            ELSE ''
        END AS slug
    FROM normalized
)
UPDATE regions r
SET slug = ranked.slug,
    updated_at = now()
FROM ranked
WHERE r.id = ranked.id;

ALTER TABLE regions
    ADD CONSTRAINT regions_slug_format_check
    CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$');
