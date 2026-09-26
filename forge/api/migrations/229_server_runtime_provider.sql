-- 229: Persist the workload runtime provider on servers.
-- Previously the runtime was implicit (always docker). This column allows
-- placement to filter nodes by their reported runtime_provider and lets the
-- control plane dispatch operations through MultiRuntimeAdapter correctly.

ALTER TABLE servers
  ADD COLUMN IF NOT EXISTS runtime_provider TEXT NOT NULL DEFAULT 'docker';

-- Constrain to the set of providers Forge knows about.
ALTER TABLE servers
  DROP CONSTRAINT IF EXISTS servers_runtime_provider_check;

ALTER TABLE servers
  ADD CONSTRAINT servers_runtime_provider_check
  CHECK (runtime_provider IN (
    'docker', 'containerd', 'podman',
    'firecracker', 'kubernetes', 'kvm', 'lxc'
  ));

-- Index for placement queries filtering by provider.
CREATE INDEX IF NOT EXISTS idx_servers_runtime_provider
  ON servers (runtime_provider);
