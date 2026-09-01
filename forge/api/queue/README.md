# Deprecated: River-style queue (forge/api/queue)

**Status:** Deprecated — not wired, not canonical. Retained per "deprecate not delete" policy.

**Canonical durable queue:** `forge/api/internal/services/queue` (see `queue/queue.go` and `store.go`)

- Table: `job_queue` (leased via `locked_until`, `locked_by`, `status`)
- Store: `queue.NewPostgresStore(pool)` wired in `forge/api/cmd/api/main.go:464-533`
- Handlers: `queue.Service.RegisterHandler(queue.JobCompose* …)` for durable compose lifecycle (`compose.deploy|update|delete|start|stop|restart`), backup (`backup.create|restore`) and periodic maintenance (`backup.retention` hourly, `cert.renewal` daily via `PeriodicJobScheduler`)
- Operations integration: `job_queue` rows mirror `operations`/`operation_steps`/`operation_attempts` for idempotency, retry and stale-reaper (`209_operation_stale_reaper.sql` index on `operations(status, updated_at) WHERE status='running'`)

**This package (`forge/api/queue`):** River-style driver (`river_job`, `river_queue`, `river_leader`) with `queuedriver/queuepgx` and template migrations (`queuedriver/queuepgx/migration/main/001_*`, `002_*`). It is **not imported** by `cmd/api/main.go` and `runRiverMigrations` in `cmd/api/river.go` is unused. The `river_queue` table created by migration `137_river_queue_features.sql` remains in the schema for backward-compatibility but receives no writes from the canonical path.

**Policy:** Do not add new jobs to this River implementation. Add durable jobs to `internal/services/queue` with an idempotency key (`DispatchIdempotent`) and register a handler on `queue.Service`. If River must be revived, coordinate a dedicated migration line and re-wire `runRiverMigrations` explicitly; until then this package is documentation-only and excluded from `go vet` critical path.

**References:**
- Canonical queue: `forge/api/internal/services/queue/queue.go:15`, `store.go:17`, `periodic.go:10`
- Wiring: `forge/api/cmd/api/main.go:464` (`queueSvc = queue.New`), `481` (`RegisterHandler JobCompose*`), `541` (`PeriodicJobScheduler`)
- River stub: `forge/api/cmd/api/river.go:13` (`runRiverMigrations` deprecated)
- Migration: `forge/api/migrations/137_river_queue_features.sql:20` (`river_queue` table, deprecated)
