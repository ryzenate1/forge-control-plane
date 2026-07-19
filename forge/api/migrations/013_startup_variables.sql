CREATE TABLE IF NOT EXISTS egg_variables (
    id UUID PRIMARY KEY,
    egg_id UUID NOT NULL REFERENCES server_templates(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    env_variable TEXT NOT NULL,
    default_value TEXT NOT NULL DEFAULT '',
    user_viewable BOOLEAN NOT NULL DEFAULT true,
    user_editable BOOLEAN NOT NULL DEFAULT true,
    rules TEXT NOT NULL DEFAULT 'nullable|string',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (egg_id, env_variable)
);

CREATE TABLE IF NOT EXISTS server_variables (
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    variable_id UUID NOT NULL REFERENCES egg_variables(id) ON DELETE CASCADE,
    variable_value TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (server_id, variable_id)
);

INSERT INTO egg_variables (id, egg_id, name, description, env_variable, default_value, user_viewable, user_editable, rules)
SELECT '77777777-7777-7777-7777-777777777777', id, 'Server Jar File',
       'The server jar file to execute when the container starts.',
       'SERVER_JARFILE', 'server.jar', true, true, 'required|string|max:64'
FROM server_templates
WHERE name = 'Minecraft Java'
ON CONFLICT (egg_id, env_variable) DO NOTHING;

INSERT INTO egg_variables (id, egg_id, name, description, env_variable, default_value, user_viewable, user_editable, rules)
SELECT '88888888-8888-8888-8888-888888888888', id, 'Minecraft Version',
       'Minecraft version passed to the container image.',
       'VERSION', 'LATEST', true, true, 'required|string|max:32'
FROM server_templates
WHERE name = 'Minecraft Java'
ON CONFLICT (egg_id, env_variable) DO NOTHING;
