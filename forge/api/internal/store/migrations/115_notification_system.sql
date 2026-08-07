-- Notification System Tables
-- This migration creates the core tables for the enhanced notification system

-- Notification channels table (if not exists)
CREATE TABLE IF NOT EXISTS notification_channels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id VARCHAR(255) DEFAULT 'global',
    user_id VARCHAR(255) DEFAULT NULL,
    type VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,
    config JSONB DEFAULT '{}',
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Indexes for notification_channels
CREATE INDEX IF NOT EXISTS idx_notification_channels_tenant ON notification_channels(tenant_id);
CREATE INDEX IF NOT EXISTS idx_notification_channels_user ON notification_channels(user_id);
CREATE INDEX IF NOT EXISTS idx_notification_channels_type ON notification_channels(type);
CREATE INDEX IF NOT EXISTS idx_notification_channels_active ON notification_channels(is_active);

-- Alert rules table
CREATE TABLE IF NOT EXISTS alert_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id VARCHAR(255) DEFAULT 'global',
    user_id VARCHAR(255) DEFAULT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT DEFAULT NULL,
    rule_type VARCHAR(50) NOT NULL, -- 'threshold' or 'state'
    entity_type VARCHAR(50) NOT NULL, -- 'server', 'node', 'backup', 'deployment', etc.
    metric_name VARCHAR(100) DEFAULT NULL, -- For threshold-based alerts
    threshold_value DECIMAL(10,2) DEFAULT NULL, -- For threshold-based alerts
    comparison_operator VARCHAR(10) DEFAULT '>', -- >, <, >=, <=, ==, !=
    state_value VARCHAR(100) DEFAULT NULL, -- For state-based alerts
    duration_minutes INTEGER DEFAULT 5, -- Minimum duration before triggering
    cooldown_minutes INTEGER DEFAULT 30, -- Cooldown period after triggering
    severity VARCHAR(20) DEFAULT 'info', -- 'info', 'warning', 'critical'
    is_enabled BOOLEAN DEFAULT true,
    notification_channel_ids UUID[] DEFAULT '{}', -- Array of channel IDs to notify
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_triggered_at TIMESTAMPTZ DEFAULT NULL
);

-- Indexes for alert_rules
CREATE INDEX IF NOT EXISTS idx_alert_rules_tenant ON alert_rules(tenant_id);
CREATE INDEX IF NOT EXISTS idx_alert_rules_user ON alert_rules(user_id);
CREATE INDEX IF NOT EXISTS idx_alert_rules_entity_type ON alert_rules(entity_type);
CREATE INDEX IF NOT EXISTS idx_alert_rules_enabled ON alert_rules(is_enabled);
CREATE INDEX IF NOT EXISTS idx_alert_rules_severity ON alert_rules(severity);

-- Notification logs table
CREATE TABLE IF NOT EXISTS notification_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id VARCHAR(255) DEFAULT 'global',
    channel_id UUID NOT NULL,
    alert_rule_id UUID DEFAULT NULL,
    event_type VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL, -- 'pending', 'delivered', 'failed'
    error_message TEXT DEFAULT NULL,
    payload JSONB DEFAULT '{}',
    sent_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Indexes for notification_logs
CREATE INDEX IF NOT EXISTS idx_notification_logs_tenant ON notification_logs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_notification_logs_channel ON notification_logs(channel_id);
CREATE INDEX IF NOT EXISTS idx_notification_logs_alert ON notification_logs(alert_rule_id);
CREATE INDEX IF NOT EXISTS idx_notification_logs_event_type ON notification_logs(event_type);
CREATE INDEX IF NOT EXISTS idx_notification_logs_status ON notification_logs(status);
CREATE INDEX IF NOT EXISTS idx_notification_logs_created ON notification_logs(created_at);

-- Alert state tracking table
CREATE TABLE IF NOT EXISTS alert_states (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_rule_id UUID NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    entity_id VARCHAR(255) NOT NULL, -- server ID, node ID, etc.
    entity_type VARCHAR(50) NOT NULL,
    current_value DECIMAL(10,2) DEFAULT NULL, -- For threshold alerts
    current_state VARCHAR(100) DEFAULT NULL, -- For state alerts
    triggered_at TIMESTAMPTZ DEFAULT NULL,
    resolved_at TIMESTAMPTZ DEFAULT NULL,
    is_active BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(alert_rule_id, entity_id, entity_type)
);

-- Indexes for alert_states
CREATE INDEX IF NOT EXISTS idx_alert_states_rule ON alert_states(alert_rule_id);
CREATE INDEX IF NOT EXISTS idx_alert_states_entity ON alert_states(entity_id);
CREATE INDEX IF NOT EXISTS idx_alert_states_active ON alert_states(is_active);

-- Notification preferences table (user-level)
CREATE TABLE IF NOT EXISTS notification_preferences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id VARCHAR(255) NOT NULL,
    tenant_id VARCHAR(255) DEFAULT 'global',
    channel_type VARCHAR(50) NOT NULL, -- 'email', 'slack', 'discord', etc.
    event_types TEXT[] DEFAULT '{}', -- Array of event types user wants to receive
    is_enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, channel_type, tenant_id)
);

-- Indexes for notification_preferences
CREATE INDEX IF NOT EXISTS idx_notification_preferences_user ON notification_preferences(user_id);
CREATE INDEX IF NOT EXISTS idx_notification_preferences_tenant ON notification_preferences(tenant_id);

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Triggers for updated_at
DROP TRIGGER IF EXISTS update_notification_channels_updated_at ON notification_channels;
CREATE TRIGGER update_notification_channels_updated_at
    BEFORE UPDATE ON notification_channels
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_alert_rules_updated_at ON alert_rules;
CREATE TRIGGER update_alert_rules_updated_at
    BEFORE UPDATE ON alert_rules
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_alert_states_updated_at ON alert_states;
CREATE TRIGGER update_alert_states_updated_at
    BEFORE UPDATE ON alert_states
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_notification_preferences_updated_at ON notification_preferences;
CREATE TRIGGER update_notification_preferences_updated_at
    BEFORE UPDATE ON notification_preferences
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Add comments for documentation
COMMENT ON TABLE notification_channels IS 'Stores notification channel configurations for different delivery methods (email, slack, discord, webhook, etc.)';
COMMENT ON TABLE alert_rules IS 'Defines alert rules that trigger notifications based on thresholds or state changes';
COMMENT ON TABLE notification_logs IS 'Logs all notification delivery attempts for auditing and debugging';
COMMENT ON TABLE alert_states IS 'Tracks the current state of alerts for each entity';
COMMENT ON TABLE notification_preferences IS 'User-level notification preferences and subscriptions';

-- Ensure the tables are compatible with existing data by adding columns if they exist
DO $$
BEGIN
    -- Add tenant_id column to existing notification_channels if it exists but doesn't have tenant_id
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'notification_channels') THEN
        IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'notification_channels' AND column_name = 'tenant_id') THEN
            ALTER TABLE notification_channels ADD COLUMN tenant_id VARCHAR(255) DEFAULT 'global';
            ALTER TABLE notification_channels ADD COLUMN user_id VARCHAR(255) DEFAULT NULL;
            ALTER TABLE notification_channels ADD COLUMN is_active BOOLEAN DEFAULT true;
            CREATE INDEX IF NOT EXISTS idx_notification_channels_tenant ON notification_channels(tenant_id);
            CREATE INDEX IF NOT EXISTS idx_notification_channels_user ON notification_channels(user_id);
        END IF;
    END IF;
END $$;