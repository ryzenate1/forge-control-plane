-- Add web_socket support column to traffic_rules for routing persistence.
-- This column is needed by trafficmanager.RoutingRule.WebSocket.
ALTER TABLE traffic_rules
    ADD COLUMN IF NOT EXISTS web_socket BOOLEAN NOT NULL DEFAULT FALSE;