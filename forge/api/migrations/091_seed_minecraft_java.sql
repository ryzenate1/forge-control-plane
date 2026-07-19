-- Ship one production-usable game definition so a fresh installation can
-- create its first real container without enabling the development demo seed.
INSERT INTO eggs (
    id, nest_id, name, description, docker_images, startup, config,
    default_memory_mb, install_script, install_container, install_entrypoint,
    file_denylist, author, features, startup_commands
)
SELECT
    '91ec0000-0000-4000-8000-000000000001',
    id,
    'Minecraft Java',
    'Minecraft Java server powered by itzg/minecraft-server.',
    '{"Java 21":"itzg/minecraft-server:java21"}'::jsonb,
    '',
    '{"stop":"stop","logs":{"custom":false},"startup":{"done":["Done ("]}}'::jsonb,
    2048,
    '#!/bin/sh
set -eu
mkdir -p /mnt/server
echo "Minecraft data directory prepared"',
    'alpine:3.21',
    'sh',
    '[]'::jsonb,
    'GamePanel',
    '["eula"]'::jsonb,
    '[]'::jsonb
FROM nests
WHERE name = 'Games'
ON CONFLICT (nest_id, name) DO NOTHING;

INSERT INTO egg_variables (
    id, egg_id, name, description, env_variable, default_value,
    user_viewable, user_editable, rules, sort
)
SELECT
    '91ec0000-0000-4000-8000-000000000002', id, 'Minecraft Version',
    'Minecraft version or channel supported by the container image.',
    'VERSION', 'LATEST', true, true, 'required|string|max:64', 10
FROM eggs
WHERE nest_id = (SELECT id FROM nests WHERE name = 'Games' LIMIT 1)
  AND name = 'Minecraft Java'
ON CONFLICT (egg_id, env_variable) DO NOTHING;

INSERT INTO egg_variables (
    id, egg_id, name, description, env_variable, default_value,
    user_viewable, user_editable, rules, sort
)
SELECT
    '91ec0000-0000-4000-8000-000000000003', id, 'Server Type',
    'Server implementation supported by itzg/minecraft-server.',
    'TYPE', 'PAPER', true, true, 'required|string|max:32', 20
FROM eggs
WHERE nest_id = (SELECT id FROM nests WHERE name = 'Games' LIMIT 1)
  AND name = 'Minecraft Java'
ON CONFLICT (egg_id, env_variable) DO NOTHING;
