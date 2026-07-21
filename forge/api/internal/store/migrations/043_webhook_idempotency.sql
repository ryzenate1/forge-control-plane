ALTER TABLE webhook_deliveries ADD COLUMN IF NOT EXISTS idempotency_key TEXT DEFAULT '';
ALTER TABLE webhook_deliveries ADD COLUMN IF NOT EXISTS idempotency_key_processed_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_idempotency_key ON webhook_deliveries(idempotency_key) WHERE idempotency_key IS NOT NULL AND idempotency_key != '';
