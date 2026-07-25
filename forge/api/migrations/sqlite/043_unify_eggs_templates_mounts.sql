-- SQLite dialect override for 043_unify_eggs_templates_mounts.sql

ALTER TABLE eggs ADD COLUMN default_memory_mb INTEGER NOT NULL DEFAULT 1024;
ALTER TABLE eggs ADD COLUMN install_script TEXT NOT NULL DEFAULT '';
ALTER TABLE eggs ADD COLUMN install_container TEXT NOT NULL DEFAULT 'alpine:3.21';
ALTER TABLE eggs ADD COLUMN install_entrypoint TEXT NOT NULL DEFAULT 'sh';
ALTER TABLE eggs ADD COLUMN file_denylist TEXT NOT NULL DEFAULT '[]';

INSERT OR IGNORE INTO nests (id, name, description)
VALUES (
    'eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee',
    'Legacy Templates',
    'Templates migrated from the legacy server_templates model'
);

INSERT OR IGNORE INTO eggs (
    id, nest_id, name, description, docker_images, startup, config,
    default_memory_mb, install_script, install_container, install_entrypoint,
    file_denylist, created_at
)
SELECT
    t.id,
    (SELECT id FROM nests WHERE name = 'Legacy Templates' LIMIT 1),
    t.name,
    '',
    ('{"' || t.image || '":"' || t.image || '"}'),
    t.startup_command,
    t.config_json,
    t.default_memory_mb,
    t.install_script,
    t.install_container,
    t.install_entrypoint,
    t.file_denylist,
    t.created_at
FROM server_templates t;

CREATE TABLE IF NOT EXISTS egg_variables_new (
    id TEXT PRIMARY KEY,
    egg_id TEXT NOT NULL REFERENCES eggs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    env_variable TEXT NOT NULL,
    default_value TEXT NOT NULL DEFAULT '',
    user_viewable BOOLEAN NOT NULL DEFAULT true,
    user_editable BOOLEAN NOT NULL DEFAULT true,
    rules TEXT NOT NULL DEFAULT 'nullable|string',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (egg_id, env_variable)
);
INSERT OR IGNORE INTO egg_variables_new SELECT * FROM egg_variables;
DROP TABLE egg_variables;
ALTER TABLE egg_variables_new RENAME TO egg_variables;

ALTER TABLE servers ADD COLUMN egg_id TEXT;
UPDATE servers SET egg_id = template_id WHERE egg_id IS NULL;
CREATE INDEX IF NOT EXISTS servers_egg_id_idx ON servers (egg_id);

-- SQLite encoding of the servers_sync_egg_identifiers trigger in the canonical
-- migration (which uses a plpgsql function + BEFORE trigger). SQLite cannot
-- assign NEW.* in a trigger body, so the NULL-fill half is an AFTER trigger
-- and the mismatch half is a BEFORE trigger with RAISE. Together they enforce
-- exactly what PostgreSQL enforces: fill the missing side, abort when both
-- are set but differ.
PRAGMA recursive_triggers=OFF;

DROP TRIGGER IF EXISTS servers_sync_egg_identifiers_fill_ins;
CREATE TRIGGER servers_sync_egg_identifiers_fill_ins
AFTER INSERT ON servers
FOR EACH ROW WHEN (NEW.template_id IS NULL OR NEW.egg_id IS NULL)
BEGIN
    UPDATE servers
    SET template_id = COALESCE(NEW.template_id, NEW.egg_id),
        egg_id = COALESCE(NEW.egg_id, NEW.template_id)
    WHERE id = NEW.id;
END;

DROP TRIGGER IF EXISTS servers_sync_egg_identifiers_fill_upd;
CREATE TRIGGER servers_sync_egg_identifiers_fill_upd
AFTER UPDATE OF egg_id, template_id ON servers
FOR EACH ROW WHEN (NEW.template_id IS NULL OR NEW.egg_id IS NULL)
BEGIN
    UPDATE servers
    SET template_id = COALESCE(NEW.template_id, NEW.egg_id),
        egg_id = COALESCE(NEW.egg_id, NEW.template_id)
    WHERE id = NEW.id;
END;

DROP TRIGGER IF EXISTS servers_sync_egg_identifiers_chk_ins;
CREATE TRIGGER servers_sync_egg_identifiers_chk_ins
BEFORE INSERT ON servers
FOR EACH ROW WHEN (NEW.template_id IS NOT NULL AND NEW.egg_id IS NOT NULL AND NEW.egg_id IS NOT NEW.template_id)
BEGIN
    SELECT RAISE(ABORT, 'server egg_id and template_id must identify the same canonical egg');
END;

DROP TRIGGER IF EXISTS servers_sync_egg_identifiers_chk_upd;
CREATE TRIGGER servers_sync_egg_identifiers_chk_upd
BEFORE UPDATE OF egg_id, template_id ON servers
FOR EACH ROW WHEN (NEW.template_id IS NOT NULL AND NEW.egg_id IS NOT NULL AND NEW.egg_id IS NOT NEW.template_id)
BEGIN
    SELECT RAISE(ABORT, 'server egg_id and template_id must identify the same canonical egg');
END;
