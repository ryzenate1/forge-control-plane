-- Phase 6: hot-path indexes for the durable drain ledger and the node
-- autoscaler audit table.
CREATE INDEX IF NOT EXISTS node_autoscale_events_policy_created_idx
    ON node_autoscale_events (policy_id, created_at DESC);
CREATE INDEX IF NOT EXISTS node_autoscale_events_state_idx
    ON node_autoscale_events (state);
CREATE INDEX IF NOT EXISTS node_autoscale_events_defict_idx
    ON node_autoscale_events (deficit DESC);
CREATE INDEX IF NOT EXISTS drain_states_status_idx
    ON drain_states (status);
CREATE INDEX IF NOT EXISTS drain_states_started_idx
    ON drain_states (started_at DESC);