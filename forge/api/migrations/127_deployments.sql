DO $$ BEGIN ALTER TABLE deployments ADD CONSTRAINT fk_deployments_server FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE; EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments(status);
CREATE INDEX IF NOT EXISTS idx_deployments_strategy ON deployments(strategy);
CREATE INDEX IF NOT EXISTS idx_deployments_created_at ON deployments(created_at DESC);
