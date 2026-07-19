ALTER TABLE server_templates
    ADD COLUMN IF NOT EXISTS install_script TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS install_container TEXT NOT NULL DEFAULT 'alpine:3.21',
    ADD COLUMN IF NOT EXISTS install_entrypoint TEXT NOT NULL DEFAULT 'sh',
    ADD COLUMN IF NOT EXISTS config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS file_denylist JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE servers
    ADD COLUMN IF NOT EXISTS installed BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS install_started_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS install_completed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS install_failed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS install_error TEXT,
    ADD COLUMN IF NOT EXISTS last_config_sync_at TIMESTAMPTZ;

UPDATE server_templates
SET
    install_script = CASE
        WHEN install_script = '' AND image LIKE '%itzg/minecraft-server%' THEN '#!/bin/sh
set -eu
echo "Preparing Minecraft Java server data directory"
mkdir -p /mnt/server
touch /mnt/server/eula.txt
printf "eula=true\n" > /mnt/server/eula.txt
'
        ELSE install_script
    END,
    install_container = CASE
        WHEN install_container = 'alpine:3.21' AND image LIKE '%itzg/minecraft-server%' THEN 'alpine:3.21'
        ELSE install_container
    END,
    install_entrypoint = CASE
        WHEN install_entrypoint = 'sh' AND image LIKE '%itzg/minecraft-server%' THEN 'sh'
        ELSE install_entrypoint
    END,
    config_json = CASE
        WHEN config_json = '{}'::jsonb AND image LIKE '%itzg/minecraft-server%' THEN '{"stop":"stop","logs":{"custom":false},"startup":{"done":["Done ("]}}'::jsonb
        ELSE config_json
    END,
    file_denylist = CASE
        WHEN file_denylist = '[]'::jsonb THEN '["/proc","/sys","/dev","/.dockerenv"]'::jsonb
        ELSE file_denylist
    END;
