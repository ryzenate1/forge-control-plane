ALTER TABLE subusers
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS subusers_server_id_idx ON subusers (server_id);

CREATE TABLE IF NOT EXISTS subuser_permission_catalog (
    key TEXT PRIMARY KEY,
    group_name TEXT NOT NULL,
    description TEXT NOT NULL
);

INSERT INTO subuser_permission_catalog (key, group_name, description) VALUES
    ('websocket.connect', 'websocket', 'Connect to the server websocket.'),
    ('control.console', 'control', 'Send console commands.'),
    ('control.start', 'control', 'Start the server.'),
    ('control.stop', 'control', 'Stop the server.'),
    ('control.restart', 'control', 'Restart the server.'),
    ('user.read', 'user', 'View server subusers.'),
    ('user.create', 'user', 'Create server subusers.'),
    ('user.update', 'user', 'Update server subusers.'),
    ('user.delete', 'user', 'Delete server subusers.'),
    ('file.read', 'file', 'Read files and directories.'),
    ('file.create', 'file', 'Create files and directories.'),
    ('file.update', 'file', 'Update files.'),
    ('file.delete', 'file', 'Delete files.'),
    ('file.archive', 'file', 'Create and extract archives.'),
    ('file.sftp', 'file', 'Access server files over SFTP.'),
    ('backup.read', 'backup', 'View backups.'),
    ('backup.create', 'backup', 'Create backups.'),
    ('backup.delete', 'backup', 'Delete backups.'),
    ('allocation.read', 'allocation', 'View allocations.'),
    ('allocation.update', 'allocation', 'Update primary allocation.'),
    ('startup.read', 'startup', 'View startup configuration.'),
    ('startup.update', 'startup', 'Update startup variables.'),
    ('database.read', 'database', 'View databases.'),
    ('database.create', 'database', 'Create databases.'),
    ('database.update', 'database', 'Rotate database passwords.'),
    ('database.delete', 'database', 'Delete databases.'),
    ('schedule.read', 'schedule', 'View schedules.'),
    ('schedule.create', 'schedule', 'Create schedules.'),
    ('schedule.update', 'schedule', 'Update schedules.'),
    ('schedule.delete', 'schedule', 'Delete schedules.'),
    ('settings.read', 'settings', 'View server settings.'),
    ('settings.reinstall', 'settings', 'Reinstall the server.')
ON CONFLICT (key) DO UPDATE SET
    group_name = EXCLUDED.group_name,
    description = EXCLUDED.description;
