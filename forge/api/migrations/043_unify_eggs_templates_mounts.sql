-- Canonicalize eggs as the sole operational server template model.
-- server_templates and servers.template_id are retained for transition compatibility,
-- but all operational foreign keys and application reads target eggs.

ALTER TABLE eggs
    ADD COLUMN IF NOT EXISTS default_memory_mb INTEGER NOT NULL DEFAULT 1024 CHECK (default_memory_mb > 0),
    ADD COLUMN IF NOT EXISTS install_script TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS install_container TEXT NOT NULL DEFAULT 'alpine:3.21',
    ADD COLUMN IF NOT EXISTS install_entrypoint TEXT NOT NULL DEFAULT 'sh',
    ADD COLUMN IF NOT EXISTS file_denylist JSONB NOT NULL DEFAULT '[]'::jsonb;

-- Egg images are canonicalized to the label -> image map.
UPDATE eggs e
SET docker_images = COALESCE((
    SELECT jsonb_object_agg(image, image)
    FROM jsonb_array_elements_text(e.docker_images) AS images(image)
), '{}'::jsonb)
WHERE jsonb_typeof(e.docker_images) = 'array';

INSERT INTO nests (id, name, description)
VALUES (
    'eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee',
    'Legacy Templates',
    'Templates migrated from the legacy server_templates model'
)
ON CONFLICT (name) DO NOTHING;

-- Preserve IDs so servers, variables, mounts, and external API clients continue
-- to refer to the same identifier after the canonical model changes.
INSERT INTO eggs (
    id, nest_id, name, description, docker_images, startup, config,
    default_memory_mb, install_script, install_container, install_entrypoint,
    file_denylist, created_at
)
SELECT
    t.id,
    (SELECT id FROM nests WHERE name = 'Legacy Templates' LIMIT 1),
    t.name,
    '',
    jsonb_build_object(t.image, t.image),
    t.startup_command,
    t.config_json,
    t.default_memory_mb,
    t.install_script,
    t.install_container,
    t.install_entrypoint,
    t.file_denylist,
    t.created_at
FROM server_templates t
ON CONFLICT (nest_id, name) DO UPDATE SET
    docker_images = CASE
        WHEN eggs.docker_images = '{}'::jsonb THEN EXCLUDED.docker_images
        ELSE eggs.docker_images
    END,
    startup = CASE WHEN eggs.startup = '' THEN EXCLUDED.startup ELSE eggs.startup END,
    config = CASE WHEN eggs.config = '{}'::jsonb THEN EXCLUDED.config ELSE eggs.config END,
    default_memory_mb = EXCLUDED.default_memory_mb,
    install_script = CASE WHEN eggs.install_script = '' THEN EXCLUDED.install_script ELSE eggs.install_script END,
    install_container = CASE WHEN eggs.install_container = 'alpine:3.21' THEN EXCLUDED.install_container ELSE eggs.install_container END,
    install_entrypoint = CASE WHEN eggs.install_entrypoint = 'sh' THEN EXCLUDED.install_entrypoint ELSE eggs.install_entrypoint END,
    file_denylist = CASE WHEN eggs.file_denylist = '[]'::jsonb THEN EXCLUDED.file_denylist ELSE eggs.file_denylist END;

ALTER TABLE servers ADD COLUMN IF NOT EXISTS egg_id UUID;
UPDATE servers SET egg_id = template_id WHERE egg_id IS NULL;
ALTER TABLE servers ALTER COLUMN egg_id SET NOT NULL;

-- Replace the legacy template FK while retaining template_id as a compatibility
-- alias for clients that have not moved to eggId yet.
DO $$
DECLARE constraint_name TEXT;
BEGIN
    FOR constraint_name IN
        SELECT c.conname
        FROM pg_constraint c
        JOIN pg_class rel ON rel.oid = c.conrelid
        JOIN pg_class ref ON ref.oid = c.confrelid
        WHERE rel.relname = 'servers'
          AND ref.relname = 'server_templates'
          AND c.contype = 'f'
    LOOP
        EXECUTE format('ALTER TABLE servers DROP CONSTRAINT %I', constraint_name);
    END LOOP;
END $$;

ALTER TABLE servers DROP CONSTRAINT IF EXISTS servers_egg_id_fkey;
ALTER TABLE servers ADD CONSTRAINT servers_egg_id_fkey
    FOREIGN KEY (egg_id) REFERENCES eggs(id) ON DELETE RESTRICT;
ALTER TABLE servers DROP CONSTRAINT IF EXISTS servers_template_id_fkey;
ALTER TABLE servers ADD CONSTRAINT servers_template_id_fkey
    FOREIGN KEY (template_id) REFERENCES eggs(id) ON DELETE RESTRICT;
CREATE INDEX IF NOT EXISTS servers_egg_id_idx ON servers (egg_id);

-- Variables now belong to canonical eggs rather than legacy templates.
DO $$
DECLARE constraint_name TEXT;
BEGIN
    FOR constraint_name IN
        SELECT c.conname
        FROM pg_constraint c
        JOIN pg_class rel ON rel.oid = c.conrelid
        JOIN pg_class ref ON ref.oid = c.confrelid
        WHERE rel.relname = 'egg_variables'
          AND ref.relname = 'server_templates'
          AND c.contype = 'f'
    LOOP
        EXECUTE format('ALTER TABLE egg_variables DROP CONSTRAINT %I', constraint_name);
    END LOOP;
END $$;
ALTER TABLE egg_variables DROP CONSTRAINT IF EXISTS egg_variables_egg_id_fkey;
ALTER TABLE egg_variables ADD CONSTRAINT egg_variables_egg_id_fkey
    FOREIGN KEY (egg_id) REFERENCES eggs(id) ON DELETE CASCADE;

-- Egg mount eligibility uses canonical egg IDs.
DO $$
DECLARE constraint_name TEXT;
BEGIN
    FOR constraint_name IN
        SELECT c.conname
        FROM pg_constraint c
        JOIN pg_class rel ON rel.oid = c.conrelid
        JOIN pg_class ref ON ref.oid = c.confrelid
        WHERE rel.relname = 'egg_mount'
          AND ref.relname = 'server_templates'
          AND c.contype = 'f'
    LOOP
        EXECUTE format('ALTER TABLE egg_mount DROP CONSTRAINT %I', constraint_name);
    END LOOP;
END $$;
ALTER TABLE egg_mount DROP CONSTRAINT IF EXISTS egg_mount_egg_id_fkey;
ALTER TABLE egg_mount ADD CONSTRAINT egg_mount_egg_id_fkey
    FOREIGN KEY (egg_id) REFERENCES eggs(id) ON DELETE CASCADE;

-- Consolidate legacy pivot data without destructively dropping the old table.
DO $$
BEGIN
    IF to_regclass('server_mounts') IS NOT NULL THEN
        INSERT INTO mount_server (mount_id, server_id)
        SELECT mount_id, server_id FROM server_mounts
        ON CONFLICT DO NOTHING;
    END IF;
END $$;

-- Keep compatibility identifiers synchronized for all future writes.
CREATE OR REPLACE FUNCTION sync_server_egg_identifiers()
RETURNS trigger AS $$
BEGIN
    IF NEW.egg_id IS NULL THEN
        NEW.egg_id := NEW.template_id;
    END IF;
    IF NEW.template_id IS NULL THEN
        NEW.template_id := NEW.egg_id;
    END IF;
    IF NEW.egg_id IS DISTINCT FROM NEW.template_id THEN
        RAISE EXCEPTION 'server egg_id and template_id must identify the same canonical egg';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS servers_sync_egg_identifiers ON servers;
CREATE TRIGGER servers_sync_egg_identifiers
BEFORE INSERT OR UPDATE OF egg_id, template_id ON servers
FOR EACH ROW EXECUTE FUNCTION sync_server_egg_identifiers();
