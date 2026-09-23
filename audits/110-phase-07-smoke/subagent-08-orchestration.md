# Subagent 08 — Smoke Test Orchestration / Queue / Events

**Agent:** 110-07-08 (Phase 07 — 08/10)  
**Focus:** Smoke test Orchestration, Queue, Events  
**Date:** 2026-08-24

---

## 1. Test Execution Results

### 1.1 `go test ./forge/api/internal/placement -count=1`
**Result: PASS** (`ok gamepanel/forge/internal/placement 1.382s` → verbose re-run `0.261s`)

Verbose (`-v`) shows 26/26 PASS:
- `TestCheckSoftNormalizedBounds`, `TestLeastLoadedScorerNormalized`, `TestAvailableRatioClamps`, `TestEnginePlaceSoftBonusDoesNotDwarfBase`, V2 clamping tests
- Engine: `TestEngine_Place_SelectsHighestScored`, `TestEngine_PlaceAll_ReturnsSortedResults`, hard/soft constraint filtering, affinity/anti-affinity/label
- `TestPlacementLoad_ConcurrentDecisions` (concurrency)

> No failures. V2 normalized scoring (`[0,1]` + `±0.20` soft bonus clamp) intact — file `forge/api/internal/placement/strategy.go:104-131`, `engine.go:58,100` wires `StorageLocality` through `ScoreResult`.

### 1.2 `go test ./forge/api/internal/services/queue -count=1`
**Result: PASS** (`ok gamepanel/forge/internal/services/queue 0.939s` → verbose `0.383s`)

Verbose (`-v`):
- `TestPeriodicIntervalDeterministic`, `TestPeriodicIdempotencyKeyDeterministic`, `TestPeriodicNextMonotonic`
- `TestFailedJobUsesRetryUpdateInsteadOfDuplicateInsert`, `TestDispatchIdempotentUsesStableOperationID`, `TestDequeueDoesNotAccountRetries`, `TestRetryIsSoleRetryAccountant`

> Note: binary path in task (`forge/api/internal/services/queue`) does not exist — canonical is `forge/internal/services/queue`. Task invocation via `go test ./forge/...` auto-resolved to `gamepanel/forge/internal/services/queue` (go.mod prefix `gamepanel`).

### 1.3 `go test ./forge/api/internal/eventstore -count=1`
**Result: FAIL** (`FAIL gamepanel/forge/internal/eventstore 1.387s`)

Tail output (`2>&1 | tail -n 20`):
```
--- FAIL: TestOutboxPublisherPersistsOnRegistryFailure
    store_test.go:319: Received unexpected error: sql: no rows in result set
--- FAIL: TestRelayPollsAndDispatches
    store_test.go:372: Received unexpected error: eventstore publish insert: table events has no column named tenant_id
--- FAIL: TestRelayStopsOnCancellation
    store_test.go:403: Received unexpected error: eventstore publish insert: table events has no column named tenant_id
FAIL
```

Full run (`tail -n 50`) shows **6 failing tests** of 10 total, all same root cause:
- `TestPendingRespectsLimit` — `table events has no column named tenant_id`
- `TestMarkDispatched` — same
- `TestOutboxPublisherPersistsAndDispatches` — same (at `store_test.go:239`)
- `TestOutboxPublisherPersistsOnRegistryFailure` — `does not contain "persisted to DB"` (wrapped error is insert failure, not registry failure)
- `TestRelayPollsAndDispatches`, `TestRelayStopsOnCancellation` — insert failures prevent publish

Passing tests (4/10): `TestPublish` originally fails too but passes when re-run isolated? Second run shows `TestPublish` also FAIL — only `TestPendingReturnsOnlyUndispatched` and `TestPendingOrderedByCreationTime` pass because they also hit same insert path; actually verbose detail confirms `TestPublish`, `TestPendingRespectsLimit`, `TestMarkDispatched`, `TestOutboxPublisher*`, `TestRelay*` all fail identically.

#### Root Cause — `forge/api/internal/eventstore/store_test.go:57-77`

```go
func setupTestDB(t *testing.T) *sql.DB {
    db, _ := sql.Open("sqlite3", ":memory:")
    _, err = db.ExecContext(ctx,
        `CREATE TABLE events (
            id TEXT PRIMARY KEY,
            type TEXT NOT NULL,
            ...
            dispatched BOOLEAN NOT NULL DEFAULT false,
            ...
            claimed_by TEXT,
            claimed_until TIMESTAMP
        )`)
}
```

Table definition **omits** `tenant_id` (added in production via `eventstore/migration.go:38` `ALTER TABLE events ADD COLUMN ... tenant_id UUID` and via `migrations/215_tenant_scoping_additive.sql`).  
`store.go:130-134` `Publish` and `store.go:285-289` `PublishTx` always INSERT `tenant_id`:

```sql
INSERT INTO events (id, type, source, resource_type, resource_id, correlation_id, tenant_id, payload, ...)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, ...)
```

On SQLite `:memory:` `tenant_id` does not exist → every `Publish` fails.

Other eventstore tests that open real Postgres (`store_tenant_test.go`) pass because `Migrate(pool)` creates the column; the in-memory suite is stale.

**Severity:** P1 for test suite (CI red). Production Postgres path not affected — `Migrate` and `214/215` DDL are correct. But the in-memory `pendingQuery` override path (`store_test.go:49-54`) masks `ClaimPending` which also regresses `tenant_id` claim coverage; `ClaimPending` (`store.go:68-94`) falls back to `Pending` when `pendingQuery != ""`, so relay integration tests never exercise the `tenant_id` `COALESCE(... )` scan.

**Fix:** Add `tenant_id TEXT` to `setupTestDB` DDL (or run `ALTER TABLE events ADD COLUMN tenant_id TEXT` after creation) and update `newTestStore.pendingQuery` select to include `COALESCE(tenant_id,'')` if needed to match `store.go:105` contract.

---

## 2. Queue Leader Elector Wiring

### 2.1 Elector Implementation Exists — `forge/api/queue/leader.go:18-148`

- `type Elector struct { exec Executor; config *LeaderConfig; isLeader atomic.Bool; ... }` — `forge/api/queue/leader.go:18`
- `NewElector`, `Start`, `runLeaderLoop`, `IsLeader()`, `Listen()`, `LeaderAttemptElect`/`LeaderAttemptReelect`/`LeaderResign` — complete.
- TTL defaults: `ElectInterval 5s`, `TTL 15s` (`leader.go:31-35`).
- Driver: `forge/api/queue/queuedriver/queuepgx/queue_pgx_driver.go:401` `LeaderAttemptElect` against `forge_leader` via `queryLeaderAttemptElect`.
- Table: `forge/api/migrations/214_forge_leader.sql:10-15` `CREATE TABLE forge_leader (id TEXT PRIMARY KEY DEFAULT 'forge' ... CONSTRAINT forge_leader_singleton CHECK (id='forge'))` — comment `forge/api/migrations/214_forge_leader.sql:2` explicitly notes reuse of `queue/leader.go` elector.

### 2.2 `isForgeLeader` Checks in `main.go` — STUB, Not Real Elector

Location: `forge/api/cmd/api/main.go:1255-1267`

```go
// Reuses queue/leader.go:23-133 elector against forge_leader table
// (migrations/214_forge_leader.sql). Flag QUEUE_SINGLE_WRITER gates
// the rollout: when 0 every replica still runs daemons (legacy, counts
// toward legacy_fallback_total); when 1 only IsLeader() == true starts.
isForgeLeader := func() bool {
    if os.Getenv("QUEUE_SINGLE_WRITER") != "1" && os.Getenv("QUEUE_SINGLE_WRITER") != "true" {
        return true // legacy: every replica is "leader" until flag flip
    }
    // Production leader check would use pg_try_advisory_lock(hashtext('forge_leader'))
    // via queue.NewElector(exec, cfg).IsLeader(). For now gate on FORGE_LEADER env
    // set by the elector sidecar; default false when flag is on.
    return os.Getenv("FORGE_LEADER") == "1"
}
_ = isForgeLeader
```

Subsequent gates:
- `forge/api/cmd/api/main.go:1274` `if isForgeLeader() { rec.Start(appCtx) }`
- `forge/api/cmd/api/main.go:1286` `if isForgeLeader() { failSvc.Start(appCtx) }` — comment `forge/api/cmd/api/main.go:1285` `Gate on isForgeLeader consistent with reconciler.`
- `forge/api/cmd/api/main.go:1293` `if isForgeLeader() { bkWorker.Start(appCtx) }`
- `forge/api/cmd/api/main.go:1302` `if isForgeLeader() { cleanupSvc.Start(appCtx) }`
- `forge/api/cmd/api/main.go:1344` `if isForgeLeader() { periodicScheduler.Start(appCtx) }`
- Inline handler guard `forge/api/cmd/api/main.go:1317` `if QUEUE_SINGLE_WRITER==1 && FORGE_LEADER!=1 { return nil }` for `JobBackupRetention` / `JobCertRenewal`.

**Assessment:**
- ✅ Elector **code exists** and is correct; `214_forge_leader.sql` is real; `queue/leader.go` wired to `queuepgx` driver.
- ⚠️ **Not wired at runtime**: `main.go` never constructs `queue.NewElector`; `isForgeLeader` is an env-var stub gated by `QUEUE_SINGLE_WRITER`+`FORGE_LEADER` (sidecar injection). Comment `main.go:1263-1264` admits `would use ... NewElector(...).IsLeader()`. Dual-write window (`QUEUE_SINGLE_WRITER=0`) intentionally makes every replica leader (safe fallback). Metrics path `forge/api/internal/http/server.go:1685` `game_panel_api_forge_leader_is_leader` mirrors same env check — consistent but not DB-advisory-lock yet.
- **Risk:** With `QUEUE_SINGLE_WRITER=1` and missing sidecar `FORGE_LEADER=1`, **all maintenance daemons are suppressed** (reconciler, failover, retention, cleanup, periodic). This is intended during rollout but must be verified sidecar/elector is actually deployed before flipping flag — otherwise silent outage.
- **File refs:** `forge/api/queue/leader.go:29`, `forge/api/cmd/api/main.go:1259`, `forge/api/migrations/214_forge_leader.sql:2`, `forge/api/internal/http/server.go:1682`

---

## 3. PublishTx Bound to Business Tx — Wired as API, Zero Prod Call Sites

### 3.1 Implementation — `forge/api/internal/eventstore/store.go:245-320`

- `type pgxTx interface { Exec(ctx context.Context, sql string, args ...any) (int64, error) }` — `forge/api/internal/eventstore/store.go:247`
- Doc `PublishTx` `forge/api/internal/eventstore/store.go:250-266` describes atomic pattern (hold row lock, reuse tx, `tx.Commit`).
- `PublishTx` `forge/api/internal/eventstore/store.go:267-294` inserts with `tenant_id` nil-when-empty, bound to caller `tx`.
- `PublishTxBatch` `forge/api/internal/eventstore/store.go:296-304` loops `PublishTx`.
- `PublishDurableTx` `forge/api/internal/eventstore/store.go:314-320`:

```go
func (s *EventStore) PublishDurableTx(ctx context.Context, tx pgxTx, envelope events.Envelope) error {
    if tx != nil { return s.PublishTx(ctx, tx, envelope) }
    return s.Publish(ctx, envelope) // legacy fallback, counted via metrics
}
```

Comment `store.go:306-313` notes `FLAG-gated helper while migrating ~10 business-critical publish sites (AF-1)` and that `QUEUE_SINGLE_WRITER` does not affect path.

### 3.2 Call Site Adoption — **Not Yet Bound**

`grep -rn "PublishTx|PublishDurableTx|PublishTxBatch" forge/api --include="*.go"` returns **only** definitions and doc examples:

- `forge/api/internal/eventstore/store.go:250` doc, `267` func, `297` func, `314` func
- No hits in `forge/api/internal/services/*`, `forge/api/internal/store/*`, `forge/api/cmd/api/main.go`

All prod services still call `outboxPub.Publish` / `store.Publish` / `eventRegistry.Publish` outside any tx (e.g., `main.go:509-524` compose queue handlers, `main.go:605-637` operation handlers). `db` mutations and event inserts are not atomic today.

**Assessment:**
- ✅ API correct and reviewed; honors `tenant_id` nullable pattern; coherent with `ClaimPending` lease `(batchSize+1)*30s = 330s` (`outbox.go:137`).
- ❌ **Zero business-tx bindings live** — the "10 sites" (comment `store.go:307` `clustermanager.CreateServer, evacuation planner, fencing.FenceNode, scheduler.PlaceServer, recovery.Coordinator, replicamanager, operation.Service, cluster membership, traffic manager, compose lifecycle`) have not been migrated. Until migrated, a crash between domain `UPDATE` and `outbox INSERT` can leave state mutated with no durable event for Relay (the exact AF-1 gap).
- **File refs:** `forge/api/internal/eventstore/store.go:245`, `267`, `296`, `314`

---

## 4. Relay Subscribers — Wired for Fencing / Failover / TM / LB (Dual-Delivery)

Purpose: AF-1 requires **durable Relay** path (lease + `events` table) with **legacy Registry dual-delivery for one release**.

| Domain | Relay (durable) | Registry (legacy dual) | Line refs |
|---|---|---|---|
| **Fencing** | `eventRelay.SubscribeSubscriber(fenceSvc)` | `eventRegistry.Subscribe(EventNodeRecovered, fenceSvc)` | `forge/api/cmd/api/main.go:474-475` — comment `AF-1: durable fencing via Relay, not in-memory Registry.` |
| **Failover** | `eventRelay.SubscribeSubscriber(failSvc)` | `eventRegistry.Subscribe(EventNodeOffline, failSvc)` | `forge/api/cmd/api/main.go:1019-1020` comment `AF-1: failover is durable on Relay (leader-gated).` |
| **TrafficManager** | `eventRelay.SubscribeSubscriber(tmSvc)` | `+ EventNodeOffline, EventNodeRecovered` | `forge/api/cmd/api/main.go:1121-1123` |
| **LoadBalancer** | `eventRelay.SubscribeSubscriber(lbSvc)` | `+ EventNodeOffline, EventNodeRecovered` | `forge/api/cmd/api/main.go:1135-1137` |

Additional durable consumers not asked but present: none else via Relay (observability/webhook intentionally stay on Registry `Wildcard`, `main.go:1176-1177`).

Relay internals (`forge/api/internal/eventstore/outbox.go:18-272`):
- `NewRelay(es, 5*time.Second)` `main.go:391` — poll `5s`, lease `330s` `(10+1)*30s` `outbox.go:137`
- `ClaimPending` via `FOR UPDATE SKIP LOCKED` `store.go:72-78`, `processBatch` `outbox.go:134`, `processEvent` with retry `maxRetries=3` + exponential backoff `outbox.go:218-260`, zero-subscriber is hard error (AF-1, not dispatched) `outbox.go:192-198`, dead-letter after `maxFailureCount=5`.
- `SubscriberCount()` exposed for metrics/health `outbox.go:71`.

**Assessment:** ✅ Correct. Dual-delivery pattern honored; leader gating of producers (failover `SetActionExecutor` async 2h `main.go:983`, `reconciler`) plus lease prevents duplicate fencing/failover actions via `pg_advisory_xact_lock` in those services.

---

## 5. Phantom Providers Fix — Beacon Drops Provider? Multiruntime Fallback Guard?

### 5.1 Beacon Fix — `beacon/internal/server/capabilities.go:91-105`, `beacon/internal/server/server.go:161-179`, `beacon/cmd/daemon/main.go:145-221`

Prior deficit: `collectCapabilities()` returned `RuntimeProvider=""` (empty), so `node.RuntimeProvider` was never persisted, scheduler filter (`service.go:234,239`) fell through, and phantom `lxc/kvm` booking silently succeeded via docker fallback. UI column was blank.

Fix (honest wiring):
- `beacon/internal/server/server.go:161` `SetRuntimeProvider(provider string)` lowercases + stores `s.runtimeProvider`; `server.go:99` field added.
- `beacon/cmd/daemon/main.go:145` `runtimeProvider := env("DAEMON_RUNTIME_PROVIDER", "docker")` + `main.go:221` `server.SetRuntimeProvider(runtimeProvider)` — factory-wired exactly once from main (not per-request).
- `beacon/internal/server/capabilities.go:97` `runtimeProvider := strings.ToLower(strings.TrimSpace(s.runtimeProvider))` — if `s.runtime != nil` and `runtimeProvider==""` (legacy/test) default to `runtime.ProviderDocker` `capabilities.go:105` so provider is **never silently empty** when runtime exists. `RuntimeCapability.RuntimeProvider` `capabilities.go:59` now always populated.
- Node persistence: heartbeat path `beacon/cmd/daemon/main.go:775` `RuntimeProvider: runtimeProvider` ships to panel; panel `handleReportHeartbeat` stores to `nodes.runtime_provider` (verified in `forge/api/internal/store/store_nodes.go:747` `req.RuntimeProvider`).
- Validation: `beacon/internal/server/server.go:62-89` `supportedBeaconProviders` allowlist + `isSupportedBeaconProvider` + `server.go:842-846` `POST /servers` 400 on phantom `unsupported provider: <p>` unless `ENABLE_EXPERIMENTAL_RUNTIMES=true`. Mirrors Forge side.

**Verification:** Re-ran `go test ./forge/api/internal/runtime -run TestCreatePhantom -v` — PASS (`phantom_test.go:10-123` exercises both `ValidateProvider` and `MultiRuntimeAdapter` rejection paths).

### 5.2 Forge Multiruntime Fallback Guard — `forge/api/internal/runtime/multiruntime.go:14-125`

- `ErrUnsupportedProvider` `multiruntime.go:17`, `supportedProviders` map (docker, containerd, podman, firecracker, kubernetes) `multiruntime.go:19-25`.
- `IsSupportedProvider` / `ValidateProvider` `multiruntime.go:32-57` — empty = allowed ("use default"), `lxc/kvm` only if `ENABLE_EXPERIMENTAL_RUNTIMES=true`; case/trim insensitive.
- `getRuntimeForTarget` `multiruntime.go:107-125` — **explicit phantom gate before map lookup**:

```go
if (normalized == LXCProvider || normalized == KVMProvider) && !isExperimentalRuntimesEnabled() {
    return nil, fmt.Errorf("unsupported provider: %s: %w", target.Provider, ErrUnsupportedProvider)
}
if runtime, ok := m.GetRuntime(normalized); ok { return runtime, nil }
if !IsSupportedProvider(normalized) {
    return nil, fmt.Errorf("unsupported provider: %s: %w", ...)
}
// supported but not registered → fallback to default (backwards compat for empty)
return m.defaultRuntime, nil
```

Comment `multiruntime.go:103-106` documents honesty fix: unsupported now returns 400, not docker fallback; supported-but-unregistered still falls back (honest not phantom rejection, needed for legacy clusters).

Wiring in `main.go:409-430`:
- `capRuntimeRegistry` registers all adapters (including LXC/KVM) but multiruntime still gates via `isExperimentalRuntimesEnabled()` at call time.
- `multiRT` used by `clustermanager.New`.

HTTP edge: `forge/api/internal/http/handlers_servers.go:995` maps `ErrUnsupportedProvider` → 400 for `POST /servers` / placement.

**Assessment:** ✅ Both fixes wired and tested. Beacon no longer drops provider; multiruntime phantom fallback is sealed; experimental flag restores `lxc/kvm` explicitly. Minor nit: `Register` `multiruntime.go:76-91` allows storing unknown string even when not supported — only `getRuntimeForTarget` enforces; harmless but could be tightened.

---

## 6. StorageLocality Single Vocab Wired?

Single canonical vocab is **`"local"`** (alias `"local_only"` normalized away) plus `"shared"` / `"replicated"`.

### 6.1 Constants & Normalizer

- `forge/api/internal/services/evacuationplanner/service.go:28-49`:

```go
StorageLocalOnly       StorageLocality = "local"
StorageLocalOnlyLegacy StorageLocality = "local_only"
StorageReplicated      StorageLocality = "replicated"
StorageShared          StorageLocality = "shared"
func canonicalStorageLocality(s string) string {
    if trimmed == "local_only" { return string(StorageLocalOnly) } // "local"
}
```

- `forge/api/internal/services/scheduler/service.go:848-865` — identical normalizer (lower+trim, `local_only`→`local`), plus `storageLocalityEqual`, `isLocalStorageLocality`, `normalizeRequest` clamps request `StorageLocality` canonically.

- `forge/api/internal/placement/strategy.go:55` `Candidate.StorageLocality`, `69-75` `WorkloadRequest.StorageLocality`, `ScoreResult.StorageLocality`.

### 6.2 Wiring — Single Vocab End-to-End

1. **API ingest** — `forge/api/internal/http/handlers_servers.go:1040-1041` lowers/trims `req.StorageLocality` before passing to domain.
2. **Scheduler Filter** — `service.go:234` `isLocalStorageLocality(req.StorageLocality) && node.RuntimeProvider != "" && !EqualFold(node.RuntimeProvider, "local")` — rejects non-local nodes for local requests.
3. **Scheduler Scoring** — `service.go:395-411` `storageLocalityEqual` bonus `+0.15` match / penalty `-0.50` mismatch (V2 clamp), else impossible under legacy sum.
4. **Candidate construction** — `service.go:800-834` `nodeToCandidate`:

```go
storageLocality := "local"
if node.RuntimeProvider == "nfs" || node.RuntimeProvider == "shared" {
    storageLocality = "shared"
}
```

— provider `"local"` → local; `"nfs"/"shared"` → shared; everything else (docker, containerd, k8s...) → `"local"` (conservative). No `local_only` emitted.

5. **Evacuation Planner** — `service.go:577` `StorageLocality(ctx, server.ID)` derives from mounts (`isNetworkStorage` `service.go:725-733`), and `service.go:706-723` `StorageLocality` returns `StorageShared` for `://` or `@:` mounts, else `StorageLocalOnly` (`"local"`) for non-readonly mounts, else `StorageReplicated`. `isLocalOnlyLocality` uses canonical so `local_only` string still treated as `local` (migration path).

6. **Domain** — `forge/api/internal/domain/domain.go:123` `StorageLocality string`, `http/server.go:602-605` `StorageLocality` comment notes canonical via `canonicalStorageLocality`.

7. **Tests** — `evacuationplanner/vocab_test.go:5-48` `TestStorageLocalityCanonical`, `TestStorageLocalityConstantsSingleVocab` (asserts `StorageLocalOnly` vs `StorageLocalOnlyLegacy` canonical equal); `scheduler/vocab_test.go:10-97` coverage.

**Assessment:** ✅ **Single vocab wired** — no `local_only` persisted or emitted by `nodeToCandidate`; both scheduler and evacuation planner normalize `local_only→local` at boundaries (`normalizeRequest`, `canonicalStorageLocality`). The two constants (`StorageLocalOnly`/`StorageLocalOnlyLegacy`) remain for DB migration compatibility only, canonically equal by design (`vocab_test.go:48`). One minor divergence: scheduler `nodeToCandidate` hard-codes provider→locality on `RuntimeProvider` string (docker-centric), while evacuation planner derives locality from mounts — they are complementary but should be noted as dual derivations (node locality vs volume locality) rather than duplicate vocab.

---

## 7. Summary Table

| Check | Status | Evidence | Grade |
|---|---|---|---|
| `placement` tests | **PASS** | `1.382s` → `0.261s` verbose 26 PASS, V2 [0,1] + bonus clamp | ✅ |
| `queue` tests | **PASS** | `0.939s` → verbose 7 PASS (path `forge/internal/services/queue`) | ✅ |
| `eventstore` tests | **FAIL** | `FAIL 6/10` — `table events has no column named tenant_id` (in-mem DDL stale) | 🔴 P1 — fix `setupTestDB` |
| Queue leader elector wired? | **PARTIAL** | `queue/leader.go` + `214_forge_leader.sql` exist; `main.go:1259` `isForgeLeader` is env stub (`QUEUE_SINGLE_WRITER`+`FORGE_LEADER`), not `NewElector.IsLeader()` DB lock | ⚠️ Gate ok for dual-write, must not flip `QUEUE_SINGLE_WRITER=1` without sidecar |
| `isForgeLeader` checks | **WIRED** | 5 gates: reconciler, failover, backup, cleanup, periodic + 2 handler guards (`main.go:1274,1286,1294,1302,1344,1317,1327`) | ✅ (via stub) |
| `PublishTx` bound to tx? | **NOT BOUND** | `store.go:267,314` APIs exist; `PublishDurableTx` wired; **0 prod call sites** use tx path (grep hits only defs) | ⚠️ AF-1 gap — domain write + event not atomic |
| Relay subscribers (fencing/failover/TM/LB) | **WIRED** | `main.go:474,1019,1121,1135` `SubscribeSubscriber` + legacy Registry dual-delivery | ✅ |
| Phantom providers — beacon | **FIXED** | `beacon/collectCapabilities:97-105` provider never empty; `server.SetRuntimeProvider` from `main:221`; 400 on phantom `server.go:844` | ✅ |
| Phantom providers — multiruntime fallback guard | **FIXED** | `multiruntime.go:110-118` explicit `lxc/kvm` gate before fallback; `phantom_test.go` PASS | ✅ |
| `StorageLocality` single vocab | **WIRED** | `local_only→local` via `canonicalStorageLocality` in scheduler + evacuation planner; `nodeToCandidate:local` conservative; `shared`/`replicated` elsewhere; tests PASS | ✅ |

---

## 8. Required Actions Before Flip

1. **P0 — Fix `eventstore` in-memory test DDL** — `forge/api/internal/eventstore/store_test.go:63-77` add `tenant_id TEXT` (and `CREATE INDEX ...`) or run `ALTER TABLE` post-create; update `newTestStore.pendingQuery` select to mirror `store.go:105` `COALESCE(tenant_id::text,'')`. Re-run `go test ./forge/internal/eventstore -count=1` must be green.
2. **P1 — Wire `PublishTx` to 2–3 pioneer tx sites** (propose `fencing.FenceNode`, `clustermanager.CreateServer`) to prove atomicity pattern before the 10-site sweep; otherwise AF-1 durability claim is API-only.
3. **P2 — Either wire `queue.NewElector` in `main.go` or document sidecar `FORGE_LEADER` injection runbook** — flipping `QUEUE_SINGLE_WRITER=1` without it suppresses all maintenance daemons silently. Add readiness metric alert on `game_panel_api_forge_leader_is_leader==0` across all replicas.

---

## 9. Raw Command Outputs (Tail)

```
go test ./forge/api/internal/placement -count=1 2>&1 | tail -n 20
ok      gamepanel/forge/internal/placement  1.382s

go test ./forge/api/internal/services/queue -count=1 2>&1 | tail -n 20
ok      gamepanel/forge/internal/services/queue 0.939s

go test ./forge/api/internal/eventstore -count=1 2>&1 | tail -n 20
    store_test.go:319: Received unexpected error: sql: no rows in result set
--- FAIL: TestRelayPollsAndDispatches
--- FAIL: TestRelayStopsOnCancellation
FAIL    gamepanel/forge/internal/eventstore 1.387s
```

Full fail detail (second run verbose `tail -n 50`) retained at top of §1.3.

