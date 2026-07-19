-- Mail notification triggers configuration.
-- Defines which events trigger automated email notifications and which
-- template to use for each event.

CREATE TABLE IF NOT EXISTS mail_notification_triggers (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event           text NOT NULL UNIQUE,
    template        text NOT NULL,
    enabled         boolean NOT NULL DEFAULT true,
    subject_template text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- Seed default triggers
INSERT INTO mail_notification_triggers (event, template, enabled) VALUES
    ('server.created', 'server_created', true),
    ('server.suspended', 'server_suspended', true),
    ('server.unsuspended', 'server_unsuspended', true),
    ('backup.completed', 'backup_complete', true),
    ('password.changed', 'password_changed', true),
    ('2fa.enabled', '2fa_enabled', true),
    ('2fa.disabled', '2fa_disabled', false)
ON CONFLICT (event) DO NOTHING;
