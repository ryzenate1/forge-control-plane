-- 221: store CPU load average and uptime from node heartbeats so the
-- observability collector can report real system metrics without querying
-- Beacon out-of-band.
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS load_average DOUBLE PRECISION;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS uptime_seconds BIGINT;
