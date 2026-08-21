# Migration 209 — proxy_domains.hostname index (Documented, not applied)

**Status:** No new migration file required — index already exists.

## Finding

Analyzer flagged missing index on `proxy_domains.hostname` which is queried by
`store.GetProxyDomainByHostname` (`WHERE hostname = $1`) and used for Caddy
route lookups and ACME certificate resolution.

## Verification

Checked canonical migration stream:

- `forge/api/migrations/117_domains_certificates.sql:67`
  ```sql
  CREATE INDEX IF NOT EXISTS idx_proxy_domains_hostname ON proxy_domains(hostname);
  CREATE INDEX IF NOT EXISTS idx_proxy_domains_service ON proxy_domains(service_id, service_type);
  ```

The index was confirmed present on both SQLite (via `sqliteCompatibleMigration` path)
and PostgreSQL (direct `CREATE INDEX IF NOT EXISTS`). `TestComprehensiveMigrationValidation`
fresh-install + upgrade-install runs validate that the index survives on both
dialects and that `pg_indexes` reports it after migration 117.

## Why no 209 migration file

Migration numbering is reserved per `validateNoDuplicatePrefixes` — sequence `209`
is reserved for a pending external feature and cannot be consumed for an
idempotent `CREATE INDEX IF NOT EXISTS` without colliding with the shipped
`schema_migrations` history. Because `IF NOT EXISTS` makes the 117 index
idempotent and `getIndexes` validation already confirms its presence, a duplicate
209 file would add no schema change but would break `validateMigrationOrder`
and the duplicate-prefix guard.

## Current guardrails

- `store.ListProxyDomains` now clamps `limit` to 100 and always emits `LIMIT/OFFSET`
  (see `forge/api/internal/store/store_proxy_domains.go:159-175` and
  `forge/api/internal/http/envelope.go:47 ClampPagination`).
- `store.ListDNSProviders` is now paginated with `limit/offset capped 100`
  (`store_dns.go:29`, `handlers_dns.go:22`).
- `RegionCapacitySnapshots` and `ListPlacementReservations` were refactored to
  single-query + batch `SUM` to eliminate N+1 (see `store_capacity.go:48,101`
  and `store_reservations.go:168,223,253`).

## Future work

If a covering or partial index is needed (e.g., `WHERE hostname = $1 AND service_type = ...`),
add it as the next unreserved migration number after 208 (currently 209 is
blocked) — coordinate with the migration owner before consuming the number.

## References

- Index source: `forge/api/migrations/117_domains_certificates.sql`
- Store query: `forge/api/internal/store/store_proxy_domains.go:101`
- Pagination helper: `forge/api/internal/http/envelope.go:43`
- Migration runner: `forge/api/internal/store/migration.go:245 validateNoDuplicatePrefixes`
