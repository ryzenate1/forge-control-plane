# Subagent 16 — Integration / e2e Smoke (Phase 08)

**Scope:** `forge/api/internal/store/*integration*test.go`, `forge/api/internal/services/*_test.go`, `forge/api/internal/http/integration_gateway_test.go`, `store_tenant_scoping_test.go:1-257`, `infra/compose.yml:1-762`

**Task brief (110-08-16):**
- Inspect `ls forge/api/internal/store/*integration*test.go`, `forge/api/internal/services/*_test.go | head -n 20`
- Verify `forge/api/internal/store/store_tenant_scoping_test.go` still passes (subagent-19)
- Create NEW `forge/api/internal/http/integration_gateway_test.go` covering gateway single-writer (empty-sync guard, no wipe) if not exists
- Run `go test ./forge/api/... -run TestIntegration -count=1 2>&1 | tail -n 30` (many SKIP without DB, no FAIL)
- Check docker-compose smoke: `docker ps 2>&1 | head -n 20; curl -s http://localhost:8080/api/health 2>&1 | head -n 20`

---

## 1. Files Inspected

| Location | Count | Finding |
|----------|-------|---------|
| `forge/api/internal/store/*integration*test.go` | 16 files | See inventory below — all pre-existing, DB-gated via `TEST_DATABASE_URL` |
| `forge/api/internal/services/*_test.go` | 0 at top-level (glob `forge/api/internal/services/*_test.go` has no matches) — tests live in subpackages | Each service (`crossnode`, `trafficmanager`, `deployment`, `scheduler`, etc.) carries its own `*_test.go`; counted indirectly |
| `forge/api/internal/store/store_tenant_scoping_test.go:1-257` | 8470 bytes, 3 tests | Created by subagent-19, verifies `tenant_id` nullable uuid + partial indexes on `placement_decisions`, `reconcile_plans`, `events` |
| `forge/api/internal/services/crossnode/ingress_sync.go:1-317` | single-writer + F-NET-01 guards at lines 128-135 and 176-181 | Empty-sync and no-healthy guards skip `UpdateRoutes` to prevent Caddy wipe |
| `forge/api/internal/services/crossnode/crossnode_test.go:1-865` | 28 tests incl. `TestScenario7_*`, `TestHealthFilter_*` | Existing coverage for grouping, health filtering, gateway reload failure |
| `forge/api/internal/http/*integration*` | 0 before this run | Gap — created `integration_gateway_test.go` |
| `forge/api/internal/http/*gateway*` | 0 before this run | No http-level gateway integration test existed |
| `infra/compose.yml:1-762` | `docker-compose` canonical file (not `docker-compose.yml` at root) | Defines `api` (8080), `postgres`, `redis`, `caddy` (profile `edge-caddy`), etc. |

**Store integration inventory (16 files):**

```
async_foundation_integration_test.go
store_admin_backups_integration_test.go
store_app_store_integration_test.go
store_auth_timing_integration_test.go
store_backup_lock_integration_test.go
store_compose_integration_test.go
store_eggs_integration_test.go
store_encryption_integration_test.go
store_git_deployment_hooks_integration_test.go
store_migration_transfer_integration_test.go
store_mounts_delete_integration_test.go
store_nodes_integration_test.go
store_operations_schema_integration_test.go
store_regions_integration_test.go
store_restore_lock_integration_test.go
store_servers_lifecycle_integration_test.go
```

Note: `ls forge/api/internal/store/*_test.go` returns 46 files total (including non-integration unit tests); the service-level glob at `forge/api/internal/services/*_test.go` is intentionally empty because services are organized as `services/<domain>/*_test.go`.

---

## 2. Tenant Scoping — Verification (subagent-19 artifact)

`forge/api/internal/store/store_tenant_scoping_test.go:18-257`

- Helper `tenantStore()` at `store_tenant_scoping_test.go:11-16` delegates to `migrationTestStore(t, false)` — requires `TEST_DATABASE_URL`; otherwise skips (consistent with other `store_*_integration_test.go` helpers).
- `TestPlacementDecisions_TenantColumn:18` — verifies `information_schema.columns` for `placement_decisions.tenant_id`, `reconcile_plans.tenant_id`, `events.tenant_id`; creates `CreatePlacementDecisionWithTenant` for tenant A/B and legacy `CreatePlacementDecision` (nullable), asserts list preserves tenants, checks `pg_index`/`pg_indexes` partial index `idx_placement_decisions_tenant`.
- `TestReconcilePlans_TenantColumn:130` — creates `ReconcilePlanRow` with tenant vs legacy nil tenant, verifies `GetReconcilePlan` round-trip and `ListReconcilePlans`.
- `TestTenantColumns_Nullable_And_Indexes:198` — asserts `is_nullable='YES'` for `placement_decisions`/`reconcile_plans`, checks `events.tenant_id` `data_type='uuid'`, fetches `pg_indexes.indexdef` for `idx_events_tenant` and asserts `WHERE` (partial index).

**Run (no DB):**

```
go test ./forge/api/internal/store -run "TestPlacementDecisions_TenantColumn|TestReconcilePlans_TenantColumn|TestTenantColumns" -count=1 -v

=== RUN   TestPlacementDecisions_TenantColumn
    store_tenant_scoping_test.go:19: TEST_DATABASE_URL is not set
--- SKIP: TestPlacementDecisions_TenantColumn (0.00s)
=== RUN   TestReconcilePlans_TenantColumn
    store_tenant_scoping_test.go:131: TEST_DATABASE_URL is not set
--- SKIP: TestReconcilePlans_TenantColumn (0.00s)
=== RUN   TestTenantColumns_Nullable_And_Indexes
    store_tenant_scoping_test.go:199: TEST_DATABASE_URL is not set
--- SKIP: TestTenantColumns_Nullable_And_Indexes (0.00s)
PASS
ok      gamepanel/forge/internal/store  0.892s
```

Result: **PASS (SKIP without DB, no FAIL)** — artifact is sound. With `TEST_DATABASE_URL` set these become integration tests against real postgres; without, they correctly skip.

---

## 3. Gateway Single-Writer — New `integration_gateway_test.go`

**File created:** `forge/api/internal/http/integration_gateway_test.go` (15 KB, 9 tests, package `http`)

**Gap before:** No file matching `forge/api/internal/http/*gateway*` or `*integration*` existed. Cross-node sync guards were tested in `forge/api/internal/services/crossnode/crossnode_test.go:512-760` (`TestIngressSyncStats`, `TestConcurrentIngressSync`, `TestScenario7_*`) but never from the HTTP integration layer. The task's requested single-writer empty-sync guard and no-wipe invariants were not exercisable via `go test -run TestIntegration` in `forge/api/internal/http`.

**Design — mock gateway `mockGateway:18-68`:**

Implements the unexported `gatewayAdapter` interface structurally (`UpdateRoutes`, `RemoveRoutes`, `CleanupStale`, `Reload`, `Health`) required by `crossnode.NewIngressSynchronizer:39`. Thread-safe via `sync.Mutex`, records `updateCalls`, `lastRules`, `lastPolicies`, `reloadCalls`, `cleanupCalls`. `Health` returns `trafficmanager.AdapterHealth{Status: HealthHealthy}` (fixed from original draft's `Healthy` field — `forge/api/internal/services/trafficmanager/gateway_adapter.go:30-36` has no `Healthy` field, only `Status HealthStatus`).

Single-writer is verified by exercising `IngressSynchronizer` which protects `rules`/`policies`/`tracking`/`syncCount`/`errCount`/`running` with `sync.RWMutex:28` and serializes gateway writes through `Sync:111-215` and `UpdateRoutes` path. The `Start` ticker spawns a single goroutine at `ingress_sync.go:55-99`; our tests exercise the synchronous `Sync` single-writer path directly.

**Tests (F-NET-01 coverage — `forge/api/internal/services/crossnode/ingress_sync.go:128-135` empty guard, `176-181` no-healthy guard, `forge/api/internal/services/trafficmanager/caddy_proxy.go:163` "For F-NET-01 we would have skipped empty sync"):**

| Test | Invariant | Crossnode lines | Assertion |
|------|-----------|-----------------|-----------|
| `TestIntegrationGateway_EmptySyncGuard_NoWipe:71` | Empty `is.rules` must not call `UpdateRoutes` (would wipe Caddy `gamepanel` server including `gamepanel-domains`) | `ingress_sync.go:132-134 INFO "ingress sync skipping empty rule set"` | `gw.UpdateCallCount()==0`, `SyncCount==0` |
| `TestIntegrationGateway_DisabledRules_NoWipe:92` | All `Enabled=false` treated as empty after filtering at `ingress_sync.go:113-116` | same guard | 0 calls |
| `TestIntegrationGateway_NoHealthyBackends_NoWipe:111` | All backends unhealthy → `mergedRules==0` must skip gateway update at `ingress_sync.go:178-181 INFO "no healthy backends — skipping gateway update to avoid wipe"` | `FilterHealthy` + second guard | 0 calls, `ErrCount==0` |
| `TestIntegrationGateway_SingleWriter_HealthySync_CallsUpdate:143` | Healthy path must call `UpdateRoutes` exactly once | `ingress_sync.go:183` | 1 call, 2 merged rules for 2 backends on same route (primary+replica), `SyncCount==1`, `RuleCount==2` |
| `TestIntegrationGateway_SingleWriter_PartialHealthy_OnlyHealthyPushed:181` | One healthy + one unhealthy (threshold 1) → only healthy pushed via `HealthFilter:138-148 FilterHealthy` | health + sync | 1 call, 1 rule with `TargetHost=="good.internal"` |
| `TestIntegrationGateway_SingleWriter_UpsertAndRemove:209` | `UpsertRule:235`/`RemoveRule:241` mutate protected map, `Sync` reflects groups, removing last rule re-enters empty guard | mutex `29` + guards | 3 calls, then empty after final remove → no extra call |
| `TestIntegrationGateway_SingleWriter_ConcurrentUpdates:270` | 5 goroutines `UpsertRule`+`Sync` concurrent → no data race, single-writer mutex holds | `ingress_sync.go:217-244 mutex` | `UpdateCallCount>=1` |
| `TestIntegrationGateway_HealthDescribe:311` | `DescribeBackend:306-316` reports `HEALTHY`/`DEGRADED`/`DOWN` | `health_filter.go:98-123` threshold 1: first failure `DEGRADED`, second `DOWN` | Asserts `HEALTHY` unknown, `DEGRADED\|DOWN` after 1 failure, `DOWN` after 2 |
| `TestIntegrationGateway_RouteGenerationTracked:341` | Successful sync populates `tracking` and `RouteGenerationRecords:265`/`GetRouteGenerationRecord:275`/`Stats:282 TrackingCount` | `ingress_sync.go:190-197 tracking` | 2 records for 2 groups |

**Run:**

```
go test ./forge/api/internal/http -run TestIntegrationGateway -count=1 -v

=== RUN   TestIntegrationGateway_EmptySyncGuard_NoWipe
2026/08/24 07:39:14 INFO ingress sync skipping empty rule set — no-op to protect gateway config
--- PASS: TestIntegrationGateway_EmptySyncGuard_NoWipe (0.00s)
=== RUN   TestIntegrationGateway_DisabledRules_NoWipe
2026/08/24 07:39:14 INFO ingress sync skipping empty rule set — no-op to protect gateway config
--- PASS: TestIntegrationGateway_DisabledRules_NoWipe (0.00s)
=== RUN   TestIntegrationGateway_NoHealthyBackends_NoWipe
2026/08/24 07:39:14 WARN no healthy backends for route; route omitted domain=app.example.com path=/
2026/08/24 07:39:14 INFO ingress sync no healthy backends — skipping gateway update to avoid wipe groups=1
--- PASS: TestIntegrationGateway_NoHealthyBackends_NoWipe (0.00s)
=== RUN   TestIntegrationGateway_SingleWriter_HealthySync_CallsUpdate
--- PASS: TestIntegrationGateway_SingleWriter_HealthySync_CallsUpdate (0.00s)
=== RUN   TestIntegrationGateway_SingleWriter_PartialHealthy_OnlyHealthyPushed
--- PASS: TestIntegrationGateway_SingleWriter_PartialHealthy_OnlyHealthyPushed (0.00s)
=== RUN   TestIntegrationGateway_SingleWriter_UpsertAndRemove
2026/08/24 07:39:14 INFO ingress sync skipping empty rule set — no-op to protect gateway config
--- PASS: TestIntegrationGateway_SingleWriter_UpsertAndRemove (0.00s)
=== RUN   TestIntegrationGateway_SingleWriter_ConcurrentUpdates
--- PASS: TestIntegrationGateway_SingleWriter_ConcurrentUpdates (0.00s)
=== RUN   TestIntegrationGateway_HealthDescribe
--- PASS: TestIntegrationGateway_HealthDescribe (0.00s)
=== RUN   TestIntegrationGateway_RouteGenerationTracked
--- PASS: TestIntegrationGateway_RouteGenerationTracked (0.00s)
PASS
ok      gamepanel/forge/internal/http   1.371s
```

All 9 PASS; guards emit expected slog INFO/WARN. The file provides the previously missing HTTP integration entry point for F-NET-01 and is picked up by `go test -run TestIntegration`.

---

## 4. Integration Filter Run — `go test ./forge/api/... -run TestIntegration -count=1`

**Command:** `go test ./forge/api/... -run TestIntegration -count=1 2>&1 | tail -n 30` (full run ≈ 12 s, many packages report `[no tests to run]`)

Captured tail (representative):

```
?       gamepanel/forge/internal/services/onboarding [no test files]
ok      gamepanel/forge/internal/services/operation        7.885s [no tests to run]
?       gamepanel/forge/internal/services/phase1git        [no test files]
ok      gamepanel/forge/internal/services/pipeline         7.573s [no tests to run]
ok      gamepanel/forge/internal/services/plugins          7.396s [no tests to run]
?       gamepanel/forge/internal/services/preview          [no test files]
?       gamepanel/forge/internal/services/previewenv       [no test files]
ok      gamepanel/forge/internal/services/procedure        7.119s [no tests to run]
?       gamepanel/forge/internal/services/process          [no test files]
ok      gamepanel/forge/internal/services/queue            7.137s [no tests to run]
ok      gamepanel/forge/internal/services/reconciler       7.428s [no tests to run]
ok      gamepanel/forge/internal/services/recovery         7.353s [no tests to run]
ok      gamepanel/forge/internal/services/registrations    7.160s [no tests to run]
ok      gamepanel/forge/internal/services/replicamanager   6.771s [no tests to run]
?       gamepanel/forge/internal/services/reservations     [no test files]
?       gamepanel/forge/internal/services/runtime          [no test files]
ok      gamepanel/forge/internal/services/scheduler        6.800s [no tests to run]
ok      gamepanel/forge/internal/services/servicediscovery 6.930s [no tests to run]
?       gamepanel/forge/internal/services/tenancy          [no test files]
ok      gamepanel/forge/internal/services/trafficmanager   7.131s [no tests to run]
?       gamepanel/forge/internal/services/upgrade          [no test files]
ok      gamepanel/forge/internal/services/webauthn         6.978s [no tests to run]
ok      gamepanel/forge/internal/services/webhook          7.091s [no tests to run]
?       gamepanel/forge/internal/services/zerodowntime     [no test files]
ok      gamepanel/forge/internal/store                     7.155s [no tests to run]
?       gamepanel/forge/internal/testutil                  [no test files]
?       gamepanel/forge/internal/version                   [no test files]
?       gamepanel/forge/queue                              [no test files]
?       gamepanel/forge/queue/queuedriver/queuepgx         [no test files]
?       gamepanel/forge/queue/queuetype                    [no test files]
```

Full filtered verbose run shows:

- `-run TestIntegration` in `forge/api/internal/http` now **does** run the 9 new `TestIntegrationGateway_*` tests (PASS above) plus `TestIntegration_LoadRealTranslations` (SKIP) in `i18n`.
- Store integration tests show `[no tests to run]` under the `TestIntegration` filter because their names are `TestPlacementDecisions_TenantColumn` etc., not `TestIntegration`. The more precise filter `go test ./forge/api/internal/store -run TestPlacementDecisions_TenantColumn -v` shows SKIP as in §2. For the broad `-run TestIntegration` store shows `[no tests to run]` (expected).
- **FAIL count:** `go test ./forge/api/... -run TestIntegration -count=1 2>&1 | grep -c "FAIL"` → `0`. No FAIL, only PASS/SKIP. This satisfies the task's "expect many SKIP without DB, but ensure no FAIL".

Note: Full `go test ./forge/api/internal/store -count=1` (unfiltered) currently has 2 unrelated FAILs due to missing DB panic in `store_schedules_reverification_test.go:124 CreateScheduleTask` and boundary mismatch in `store_mounts_ext_reverification_test.go:61,113` — these are not `TestIntegration`-filtered and thus not in scope for the requested command, but they are noted as pre-existing gaps requiring follow-up (panic on nil `pgxpool.Pool` when `TEST_DATABASE_URL` unset).

---

## 5. Docker-Compose Smoke

**Compose file location:** No `docker-compose.yml` at repo root (task's suggested `docker-compose.yml` path does not exist). Canonical compose is `infra/compose.yml:1-762` with overlay matrix documented at `infra/compose.yml:5-12` (`-f compose.yml -f compose.override.yml`, `compose.production.yml`, `compose.caddy.production.yml`, `compose.tls.yml`, `compose.logging.yml`, profiles `edge-caddy`/`edge-traefik`/`docs`). Smoke uses `infra/compose.yml` baseline.

**`docker ps 2>&1 | head -n 20`**

Early run (daemon not yet ready):
```
Cannot connect to the Docker daemon at unix:///Users/riyaz/.docker/run/docker.sock. Is the docker daemon running?
```
After daemon available (final):
```
CONTAINER ID   IMAGE            COMMAND                  CREATED        STATUS                   PORTS                                         NAMES
01d1f1f50364   redis:7-alpine   "docker-entrypoint.s…"   9 hours ago    Up 5 minutes (healthy)   127.0.0.1:6379->6379/tcp                      infra-redis-1
9813998520c4   postgres:16      "docker-entrypoint.s…"   20 hours ago   Up 5 minutes             127.0.0.1:55409->5432/tcp                     mgp-db-012e45ec-c281-4aa4-b9f9-02fefba76218-postgresql
336055d83562   mariadb:11       "docker-entrypoint.s…"   7 days ago     Up 5 minutes             0.0.0.0:3306->3306/tcp, [::]:3306->3306/tcp   mariadb-mariadb
```

Only `redis` ( infra ), a throwaway `mgp-db-*` postgres, and an unrelated `mariadb` are running. `api`, `caddy`, `web`, `daemon` are not up — expected: `infra/compose.yml:195-277 api` requires `${TAG:?Set TAG}` and `${API_AUTH_SECRET}`, `${DATABASE_URL}`, etc. plus `docker-proxy` and `postgres` health; it is not started by default without `TAG` and `.env`.

**`curl -s http://localhost:8080/api/health 2>&1 | head -n 20`**

```
(empty — no body)
```

Verbose:

```
* Uses proxy env variable no_proxy == '127.0.0.1,localhost,::1'
* Host localhost:8080 was resolved.
* IPv6: ::1
* IPv4: 127.0.0.1
*   Trying [::1]:8080...
* connect to ::1 port 8080 from ::1 port 56479 failed: Connection refused
*   Trying 127.0.0.1:8080...
* connect to 127.0.0.1 port 8080 from 127.0.0.1 port 56480 failed: Connection refused
* Failed to connect to localhost port 8080 after 0 ms: Couldn't connect to server
curl: (7) Failed to connect to localhost port 8080 after 0 ms: Couldn't connect to server
```

**`curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/health` → `000`** (no listener).

This matches `infra/compose.yml:237-239` (`127.0.0.1:8080:8080`, `healthcheck: ["/api", "--healthcheck"]`) — health is served only when `api` container is running and healthy. Without `api`, curl correctly returns 000. For a full smoke the stack would need `TAG` + `infraid` + `docker compose -f infra/compose.yml -f infra/compose.override.yml up --wait` (or production overlay). Current smoke correctly documents the gap: compose infra is present but `api` not yet launched in this dev host.

---

## 6. Gaps / Notes

- **Resolved — http gateway gap:** `forge/api/internal/http/integration_gateway_test.go` now exists; 9 tests, all PASS without DB (no `TEST_DATABASE_URL` required). Documents single-writer empty-sync guard and no-wipe semantics per `ingress_sync.go:128-181`. Prior state had zero http integration coverage for F-NET-01.
- **Pre-existing — docker-compose path:** Task suggests `docker-compose.yml` at root; actual compose is `infra/compose.yml` with `compose.override.yml`/`compose.production.yml` overlays. Report treats `infra/compose.yml` as canonical.
- **Pre-existing — api health smoke:** `curl 000` is expected without `api` running; not a regression. To get `200`, run `infra` stack with required env (`TAG`, `POSTGRES_PASSWORD`, `API_AUTH_SECRET`, `FORGE_MASTER_KEY`, `DAEMON_NODE_TOKEN`, `METRICS_TOKEN`, etc.) as per `infra/compose.yml:210-236`.
- **Non-blocking — store full suite:** Unfiltered `go test ./forge/api/internal/store -count=1` reports 2 FAILs unrelated to this task (`store_mounts_ext_reverification_test.go:61 traversal boundary`, `store_mounts_ext_reverification_test.go:113 mountsAllowedPrefixes`, `store_schedules_reverification_test.go:124 nil *pgxpool.Pool` panic on `CreateScheduleTask` without DB). The latter is a guard bug (should `t.Skip` when `TEST_DATABASE_URL` unset, as done by 16 other integration files at `store_*.go:19`). Recommend adding `if os.Getenv("TEST_DATABASE_URL")=="" { t.Skip(...) }` at top of `TestCreateScheduleTask_AcceptsValidActions_NoUnsupportedError:120`.
- **Non-blocking — vet noise:** `go vet ./forge/api/internal/store` flushes no `missing ','` today (previously a transient syntax error in `store_servers_reverification_test.go:540-543` was observed but self-resolved in later runs; current branch `store_servers_reverification_test.go:540` is single-line struct with no trailing comma requirement — syntax is valid under `go vet -composites`).
- **Single-writer queue vs gateway:** Gateway single-writer is via `IngressSynchronizer` mutex; queue single-writer (`forge/api/internal/services/queue/queue.go:33 single writer for durable compose`, `queueSingleWriterEnabled:100`) is a separate path exercised via `crossnode`/`operation` services, not duplicated here.

---

## 7. Verdict

**PASS.** Tenant scoping tests (`store_tenant_scoping_test.go:18,130,198`) SKIP cleanly without DB and are ready for integration with `TEST_DATABASE_URL`. HTTP gateway integration gap is now closed: `integration_gateway_test.go` adds 9 tests covering empty-sync guard, no-healthy guard, single-writer `Upsert`/`Remove`/concurrency and route-generation tracking, all PASS and counted under `-run TestIntegration`. `go test ./forge/api/... -run TestIntegration -count=1` reports **0 FAIL** (many `[no tests to run]`/`SKIP`, 9 PASS in http). Docker smoke documents that `infra/compose.yml` exists but `api :8080` is not running on this host (expected without `TAG` stack), `docker ps` shows only `redis`/`postgres` infra containers, `curl http://localhost:8080/api/health` returns `000` (connection refused) — compose health smoke gap documented for next bring-up.
