# Server lifecycle

Forge uses a synchronous panel/Beacon lifecycle:

1. `POST /servers` validates placement inputs and persists the server, allocations, build limits, image/startup overrides, and startup variables in one PostgreSQL transaction. The row is stored as `provisioning` and `installed = false`.
2. Forge loads the canonical provisioning target, including the selected node's current daemon credential, and synchronizes the canonical configuration to Beacon.
3. Forge calls Beacon `POST /servers` to create the workload. A successful response changes the panel status to `created`; it does **not** mark the server installed. Any failure releases panel allocations, cancels the placement reservation, and hard-deletes panel state. If a created workload cannot be deleted during compensation, Forge records a pending `server_orphan_remediations` row before deleting panel state.
4. `POST /servers/:id/install` explicitly calls Beacon's install endpoint and marks the server installed only after a successful zero-exit installer result.
5. `POST /servers/:id/reinstall` explicitly calls Beacon's reinstall endpoint. It is not an alias or redirect to a Forge handler.
6. `DELETE /servers/:id` calls Beacon first and hard-deletes panel state only after Beacon succeeds. `?force=true` always removes panel state; a Beacon failure is retained as an orphan-remediation audit record.

Server transfer/archive execution is intentionally unavailable and returns `501 Not Implemented`. Migration and transfer are outside this lifecycle.

## Current egg-model limitations

Beacon's create contract currently accepts image, command, environment, ports, mounts, memory, CPU shares, and disk. Forge persists the wider build model (CPU limit, swap, I/O weight, threads, and OOM behavior) and includes it in the synchronized canonical configuration, but Beacon's Docker create request does not yet apply those additional fields. Allocation ports are mapped to the same container port because the current egg model does not persist per-allocation container-port mappings. The `itzg/minecraft-server` compatibility behavior remains image-specific rather than egg-defined.
