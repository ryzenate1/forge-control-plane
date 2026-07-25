-- 223_notification_channels.sql
-- Notifications Engine (Dokploy-style notification router): extend the
-- existing notification_channels table (created in 114_b_notifications.sql,
-- config encrypted in 159_encrypt_notification_configs.sql) with user/org
-- ownership so channels can be managed by regular users, not only admins.
--
-- Design notes:
-- * Additive only. Pre-existing rows keep user_id/org_id NULL, which the
--   engine interprets as "global" channels managed by admins — exactly the
--   behavior the legacy /notification-channels routes already have, so no
--   existing dispatch changes semantics.
-- * The subscription schema the router needs (channel_id FK + event_type
--   with UNIQUE(channel_id, event_type) + created_at) already exists in
--   notification_event_subscriptions; rather than duplicating the table we
--   expose it under the canonical notification_subscriptions name as a view.
-- * The migration file numbering follows the repo convention of sharing a
--   numeric prefix across parallel files (see 211_a/211_b); the schema
--   migration history keys on the full filename.

ALTER TABLE notification_channels
    ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS org_id  UUID REFERENCES organizations(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS notification_channels_user_idx
    ON notification_channels (user_id) WHERE user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS notification_channels_org_idx
    ON notification_channels (org_id) WHERE org_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS notification_channels_enabled_idx
    ON notification_channels (enabled) WHERE enabled;

-- Canonical subscription surface used by the notifications engine.
CREATE OR REPLACE VIEW notification_subscriptions AS
    SELECT id, channel_id, event_type, template, created_at
    FROM notification_event_subscriptions;

-- Backfill: make the (channel_id, event_type) uniqueness explicit for the
-- view contract on fresh databases where 114_b already created it, and
-- ensure the FK index exists for unsubscribe-by-channel hot paths.
CREATE INDEX IF NOT EXISTS notification_event_subscriptions_channel_event_idx
    ON notification_event_subscriptions (channel_id, event_type);
