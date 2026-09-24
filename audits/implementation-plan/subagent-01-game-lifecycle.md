# Implementation Plan — Subagent 01/10: Game Server Lifecycle & State Machine

**Domain:** Game Hosting — Pterodactyl / Pelican / Wings parity → Forge (`forge/api`) + Beacon (`beacon`)  
**Author:** subagent-01 (plan-only, no product edits)  
**Date:** 2026-08-24  
**Master audit source:** `audits/FINAL_PARITY_AUDIT.md` GH-01..GH-19 + `audits/MASTER_FINDING_INDEX.md` REF-GAME-* + `audits/reverification/*` + `reference/wings` vs `beacon/internal/server/manager.go` + `forge/api/internal/store/store_state.go` + `forge/api/internal/store/store_servers*.go` + `forge/api/internal/services/clustermanager/service.go` + `forge/api/internal/http/handlers_servers.go` + `forge/api/internal/store/store_egg_variables.go`

> **Invariant for every change below:** Postgres is the durable control-plane model. No proposal drops columns, renames enums in place, or replaces Postgres-centred reconciliation with a new system. All DB changes are additive (`ADD COLUMN IF NOT EXISTS`, new enum values with `ADD VALUE IF NOT EXISTS`, new tables, new indexes). New behaviour is dual-read / dual-write with a feature flag and a rollback path. Beacon generation/lease enforcement is additive and fail-open when the column is absent.

---

## 0. Executive map

| GH | Title | Current status | Effort | Depends on |
|---|---|---|---|---|
| GH-05 | `kill` must pierce stuck power lock | MISSING / BROKEN | S | — |
| GH-07 | Reinstall gate rejects Docker (Reinstaller check) | BROKEN P0 | S | — |
| GH-09 | `restoring_backup` never wired as server-level lock | MISSING P1 | M | GH-05 fencing infra |
| GH-14 | Egg variable regex compiles delimiters literally | BROKEN P0 | S | — |
| GH-12 | Allocations kill/start race vs installing — API enqueues doomed ops | BROKEN | M | GH-09 guard set |
| GH-04/12 | Power handler only checks `transfer` not `installing`/`restoring_backup`/`suspended` | BROKEN | S | GH-09 |
| GH-01/02/06 | BeginInstall vs HandlePower same `RunningAction` slot, no generation on install | PARTIAL | M | Gen fencing |
| GEN | Beacon never enforces `generation`/`workload_lease_expiry` (fence fires on wrong edge + zero enforcement) | MISSING P1 | M | 110_node_fencing migration |
| SUSP | Suspend orthogonal boolean not fencing power/install at both edges | PARTIAL | S | — |
| ORPH | Orphan remediation only on delete/create-failure, not on install/reinstall failure | PARTIAL | S | lifecycle doc |
| TRANS | `ensureTransferIdle` is only transfer-aware | PARTIAL | S | — |

**Forge patterns reused (do not reinvent):**

- `forge/api/internal/store/store_state.go:10` — `desired_state` / `actual_state` + `desired_generation` / `observed_generation` + `state_transitions` audit row (`recordStateTransition:120`).
- `forge/api/migrations/092_durable_operations.sql:7` — generation columns + `operations` table with `desired_generation` / `observed_generation`.
- `forge/api/migrations/110_node_fencing.sql:2` — `servers.generation` + `servers.workload_lease_expiry` + `recovery_items.fence_generation` (central fence token).
- `forge/api/internal/store/store_evacuation.go:33` — fencing via `generation = generation+1, workload_lease_expiry = NOW()+1h` on evacuation eligibility.
- `forge/api/internal/store/store_servers_lifecycle.go:18` — `RecordOrphanAndHardDeleteServer(id, nodeURL, daemonError)` — remediation row has **no FK** to `servers` (survives hard-delete), pattern to clone.
- `forge/api/internal/services/operation/service.go:360` — durable operation queue with `202 Accepted` + `{operationId}` + `GET /operations/:id` poll, `Touch:87` heartbeat + `ReapStale:82` reaper + `dispatch:332` deterministic `forge-op:` idempotency.
- `beacon/internal/server/manager.go:271` `BeginInstall` / `manager.go:482` `HandlePower` — `TryLock` + `RunningAction` slot, `onBeforeStart:610` disk/chown checks, `persistPowerState:387`.
- `forge/api/docs/server-lifecycle.md:1` — canonical 6-step provisioning lifecycle (panel-first, beacon-second, orphan on compensation failure).

---

## 1. GH-05 — Kill must pierce a stuck power lock

### 1.1 Current broken code + why it breaks

**Forge Beacon:** `beacon/internal/server/manager.go:482-507`

```go
func (m *ServerManager) HandlePower(ctx context.Context, serverID, signal string) error {
    state := m.State(serverID)
    if !state.mu.TryLock() {
        return errors.New("another server action is already running")
    }
    if state.RunningAction != "" {  // same slot as BeginInstall
        state.mu.Unlock()
        return errors.New("another server action is already running")
    }
    // ...
    case "kill":
        // ... state.PowerState = Stopping, persist, then
        err = m.runtime.Kill(ctx, serverID)
```

`BeginInstall` at `manager.go:271` uses the **identical** slot (`TryLock` → check `RunningAction != ""` → set `RunningAction="install"`). A hung `stop`/`restart` that never returns from `runtime.WaitForStop` (network → beacon, docker hang, or a long `StopTimeout` default `30s` in `State:245` extended by `stopServer:447`'s `WaitForStop`) holds `RunningAction="stop"` for tens of minutes. An operator `kill` — whose entire purpose is to rescue the stuck server — is queued **behind** that same mutex and receives `409 "another server action is already running"`. The durable API path (`handlers_servers.go:888` `POST /servers/:id/power` → `operation.Service.DispatchPower` → worker → `clusterManager.KillServer` → `beacon power kill`) will then retry until `operation/service.go:284` MaxRetries, creating a retry storm that never pierces.

**Reference truth:** `wings/power.go:108` — `kill` intentionally uses `TryAcquire` **and ignores failure**: the emergency path `Kill` does not wait for `powerLock`. `wings/power.go:24` documents the power lock, `wings/power.go:57` blocks install while `IsInstalling`, but kill bypasses.

**Impact:** Stuck container (OOM loop, pid1 ignoring SIGTERM, Docker daemon stall) becomes unrecoverable without restarting the Beacon daemon itself.

### 1.2 Target design (exact code change, backward compatible)

**Do not change the `ServerState.mu` mutex type** (keep `sync.Mutex` for all other signals). Add a **piercing** path that is only reachable for `signal=="kill"`.

```go
// beacon/internal/server/manager.go — new helpers

// tryClaimPower returns (claimed bool, wasPierced bool, unlock func).
// Non-kill signals use TryLock strictly. Kill bypasses a held RunningAction
// slot: it overwrites RunningAction to "kill" so the in-flight holder's
// deferred cleanup (HandlePower:501-507) becomes a no-op for the stale value.
func (m *ServerManager) tryClaimPower(serverID, signal string) (bool, bool, func()) { ... }

// Kill-specific piercing logic (pseudocode for plan):
func (m *ServerManager) claimForKill(state *ServerState) (pierced bool) {
    // Fast path: normal claim.
    if state.mu.TryLock() {
        if state.RunningAction == "" {
            state.RunningAction = "kill"
            state.mu.Unlock()
            return false // not pierced
        }
        if state.RunningAction == "kill" {
            state.mu.Unlock()
            return false
        }
        // Slot held by install/stop/restart/start — pierce it.
        prev := state.RunningAction
        state.RunningAction = "kill" // steal the slot
        state.mu.Unlock()
        log.Printf("beacon: kill pierced stuck %q on %s", prev, serverID)
        return true
    }
    // Mutex itself held — still need to pierce. Use a short TryLock loop
    // with deadline rather than blocking forever, then overwrite under lock.
    // Implementation: hold a separate killMu or use atomic RunningAction CAS
    // via a dedicated sync.Mutex killMu that guards RunningAction alone.
    return false
}
```

**Pragmatic minimal diff (recommended for plan):** introduce `ServerState.powerMu sync.Mutex` + `ServerState.killSerial sync.Mutex` and **split** the guard:

- `HandlePower` normal path: `if !state.mu.TryLock() { return busy }` → check `RunningAction` → set → unlock → defer clear-if-still-mine. Unchanged for `start/stop/restart`.
- `HandlePower` when `signal=="kill"`: acquire `state.killMu` (always succeeds, serializes concurrent kills), then try `state.mu.TryLock()`. If `TryLock` fails **or** `RunningAction != ""`, overwrite `RunningAction="kill"` under `mu` (if `TryLock` failed, use `state.mu.Lock()` with 250ms timeout via `context` then overwrite). Then proceed directly to `runtime.Kill` without waiting for the previous `WaitForStop`.

Deferred cleanup must be `if state.RunningAction == signal { state.RunningAction = "" }` — already present at `manager.go:502` — so a pierced waiter that finally wakes will see `RunningAction=="kill"` and **not** clear it.

**API-side complement:** `forge/api/internal/http/handlers_servers.go:888` already dispatches kill as a durable operation (`DispatchPower` with `forge-op:power:<id>:kill:<idempotency>`). Add a dedicated **priority** for kill: when dispatching `signal=="kill"`, set `available_at = now()` and `priority = 100` so the operation worker (`operation/service.go:211` `Dequeue`) picks it before normal queued starts. Alternatively, keep kill on `operation` path but ensure the **beacon** pierce is sufficient — the retry storm disappears once beacon accepts kill immediately.

**Beacon API change:** `beacon/internal/server/server.go:1357` `power` handler already forwards to `manager.HandlePower`. No HTTP contract change needed — the same `POST /servers/{id}/power {"signal":"kill"}` now pierces. Advertise piercing in the `InstallResponse`/`PowerResponse` `Mode` field (keep `"docker"` for compatibility, add `"pierced":true` extra field).

### 1.3 Migration SQL (additive, no drop)

No DB migration required. Optional observability column for audit (additive):

```sql
-- 211_beacon_kill_pierce_audit.sql (optional, additive)
ALTER TABLE state_transitions ADD COLUMN IF NOT EXISTS pierced boolean NOT NULL DEFAULT false;
CREATE INDEX IF NOT EXISTS state_transitions_pierced_idx ON state_transitions (resource_id, pierced) WHERE pierced = true;
-- No enum changes. No column drops.
```

If you want kill bypass metrics without code deploy, use existing `state_transitions.reason` with `reason='kill pierced stuck <prev>'` — no migration at all. Recommended: **no migration for GH-05**; rely on beacon logs + `reason` text.

Feature flag (additive, no column):

```sql
-- 211a_beacon_feature_flags.sql
CREATE TABLE IF NOT EXISTS beacon_feature_flags (
    key text PRIMARY KEY,
    enabled boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO beacon_feature_flags(key, enabled) VALUES ('beacon.kill.pierce', true)
ON CONFLICT (key) DO NOTHING;
```

Beacon reads `beacon.kill.pierce` from panel `/servers/{id}/configuration` sync (already shipped in `store_servers_control.go:188` `ServerProvisionTarget` + `manager.go:689` `syncServerStateFromPanel`) — if absent, defaults to enabled.

### 1.4 Rollout steps

1. **DB (if audit flag chosen):** deploy `211_beacon_kill_pierce_audit.sql` (additive, `IF NOT EXISTS` — safe on replay). No app code reads it yet.
2. **Deploy Beacon** with piercing implementation behind `beacon.kill.pierce` flag default-on. Dual-read: if panel config omits the flag, treat as enabled. Kill path logs `kill pierced stuck <prev> on <server>`.
3. **Deploy API** with kill priority dispatch (optional, `available_at` bump on kill — no contract change, just queue ordering). API `DispatchPower` already idempotent (`forge-op:power:...`), so retrying kill before beacon upgrade is harmless.
4. **Verify** via integration test (see 1.6), then remove flag gate if desired — keep code path, drop flag read after one release.

Rollback: redeploy previous beacon binary; kill reverts to TryLock behaviour, no DB state to migrate back.

### 1.5 Frontend impact

None required for correctness. Optional UX improvement:

- On `POST /servers/:id/power` with `signal=kill` returning `202 {operationId}`, the poller (`web/app/api/proxy/operations/*` → `GET /operations/:id`) should **not** show `409 another server action is already running` as error — it should show `Kill dispatched (piercing stuck operation)` toast. Today `web/components/backup-manager.tsx:139` already polls `GET /operations/{id}` every 2s; that component is for restores, but the same pattern should be used for kill. No badge change.

### 1.6 Testing strategy

**Unit — beacon:** `beacon/internal/server/manager_test.go` (new file or append)

```go
func TestKillPiercesStuckStop(t *testing.T)
- Start a fake runtime where Stop blocks (WaitForStop hangs 5s).
- Call HandlePower("stop") in goroutine (holds RunningAction="stop").
- Immediately call HandlePower("kill") — must succeed (not 409), must call runtime.Kill.
- Assert RunningAction=="kill" after pierce, and that the stuck stop's deferred clear is no-op.
func TestConcurrentKillSerialized(t *testing.T)
- 10 concurrent kills on same server: exactly 1 Kill call, others either succeed or get already-killing dedup.
func TestNonKillStillFenced(t *testing.T)
- While install holds RunningAction, start must still be rejected 409.
```

**Unit — api:** `forge/api/internal/services/operation/service_test.go` — verify kill dispatch sets `available_at = now` not delayed.

**Integration:** `forge/api/internal/http/handlers_servers_integration_test.go` (or `testutil` harness):

- Provision server, trigger `POST /servers/:id/power {stop}` with a hanging daemon stub, then `POST ... {kill}` — assert second returns `202` with distinct `operationId`, and `GET /operations/:id` for kill reaches `succeeded` while stop is `cancelled` or remains `running` but does not block kill success.

**Contract test:** `beacon/internal/server/runtime_handlers_test.go:115` already has `{"power", "kill"}` table test — extend to piercing variant with a pre-seeded `RunningAction="stop"`.

### 1.7 Risk & mitigation

| Risk | Mitigation |
|---|---|
| Piercing corrupts daemon state (two concurrent `WaitForStop`/`Kill` racing on same container) | `docker kill` is idempotent; `RunningAction` CAS ensures only kill owns the slot; previous holder's stop is abandoned but container ends stopped — correct terminal state. |
| Lost accounting (kill success but previous stop's error swallowed) | Log pierced amount + previous action; record `state_transitions(reason='kill pierced stuck stop')` so audit survives hard-delete via orphan pattern if needed. |
| Flag read fails (panel unreachable) | Fail-open: `onBeforeStart:652` already logs but does not block startup on panel sync failure — same for kill pierce. |
| Retry storm before beacon upgrade | API kill priority + idempotency `forge-op:power:<id>:kill:<key>` already dedups; even without pierce, retries are bounded by `MaxRetries=3` + backoff at `operation/service.go:288`. |

### 1.8 Effort + dependencies

**S** (1–2 days). No DB dependency. Depends on nothing; precedes GH-12/GH-09 fencing hardening.

---

## 2. GH-07 — Reinstall fails on Docker runtime gate (Reinstaller iface check)

### 2.1 Current broken code + why it breaks

**Forge API:** `forge/api/internal/services/clustermanager/service.go:224-248`

```go
func (s *Service) runInstaller(ctx context.Context, serverID string, reinstall bool) (...) {
    // ...
    if reinstall {
        reinstaller, ok := s.runtime.(gpruntime.Reinstaller) // line 241
        if !ok {
            err = errors.New("runtime does not support reinstall") // line 243
        } else {
            response, err = reinstaller.ReinstallServer(ctx, ...)
        }
    } else {
        response, err = s.runtime.InstallServer(ctx, ...) // always works
    }
}
```

`gpruntime.Reinstaller` (`forge/api/internal/runtime/runtime.go:149`) is an **optional** capability:

```go
type Reinstaller interface {
    ReinstallServer(context.Context, Target, InstallRequest) (InstallResponse, error)
}
```

`DockerAdapter` (`forge/api/internal/runtime/docker.go:67`) **does** implement `ReinstallServer` (delegates to `client.ReinstallServer` → `daemon/client.go:693` `POST /servers/{id}/reinstall`). However `MultiRuntimeAdapter` (`multiruntime.go:106`) only delegates `ReinstallServer` when the underlying `rt.(Reinstaller)` succeeds; `Registry.instrumentedRuntime` (`registry.go:175`) does the same check. The bug is that the **concrete type stored in `clustermanager.Service.runtime`** at startup is often the **registry wrapper** whose static type is `gpruntime.Runtime` (not `*DockerAdapter`) — the type assertion `s.runtime.(Reinstaller)` on the **wrapper** fails because the wrapper's method set is checked, but the wrapper only implements `Reinstaller` via a delegation method that itself checks the inner runtime. In `registry.go:175` the wrapper **does** declare `ReinstallServer`:

```go
func (r *instrumentedRuntime) ReinstallServer(...) {
    reinstaller, ok := r.runtime.(Reinstaller)
    if !ok { return ErrNotImplemented }
}
```

So the outer assertion **succeeds**, but the inner one on `r.runtime` may fail if `r.runtime` is a plain `DockerAdapter` vs `MultiRuntimeAdapter` vs `*runtime.DockerAdapter` pointer mismatch — confirmed by `multiruntime_test.go:265` `TestMultiRuntimeAdapterReinstallServerNoReinstaller` returning `ErrNotImplemented`. In production, `r.runtime` is frequently a `*DockerAdapter` which **does** implement `Reinstaller`, but if the panel is configured with `runtime_provider='docker'` and the adapter is constructed via `factory.go:19` `NewRuntime(provider string)` returning a `DockerAdapter` value (non-pointer) whose method receiver is `(r *DockerAdapter) ReinstallServer`, the value type does **not** satisfy `Reinstaller`, so the assertion fails and the API returns `500 runtime does not support reinstall` for every Docker reinstall, even though **Beacon** `server.go:1331` `reinstall` handler works (it just checks `PowerState != Running` then calls `install` — no capability gate).

**Reference:** `wings/install.go:80` `WaitForStop 10s` then reinstall — no provider gate, just stop check. Forge added a provider capability gate that rejects the only provider that actually supports it.

### 2.2 Target design (exact code change, backward compatible)

**Fix the gate, not the contract.** Two additive fixes, both backward compatible:

**A. Make `Reinstaller` a required method on `gpruntime.Runtime` interface (non-breaking via adapter fallback).**

Change `clustermanager/service.go:240-246` to treat `ReinstallServer` as: *if Reinstaller is present, use it; otherwise, emulate reinstall as `ensure stopped` + `InstallServer`* — identical to Beacon's `server.go:1341` `reinstall` forwarding:

```go
func (s *Service) runInstaller(ctx context.Context, serverID string, reinstall bool) (...) {
    target, err := s.store.ServerProvisionTarget(ctx, serverID)
    if err != nil { return ..., err }
    if err := s.store.SetServerInstallState(ctx, serverID, "installing", ""); err != nil { return ..., err }
    if err := s.syncProvisionTarget(ctx, target); err != nil {
        _ = s.store.SetServerInstallState(ctx, serverID, "failed", err.Error())
        return ..., err
    }
    var response gpruntime.InstallResponse
    if reinstall {
        // Preserve existing Reinstaller delegation, but never hard-fail with
        // "does not support reinstall" for Docker — fall back to Install.
        if reinstaller, ok := s.runtime.(gpruntime.Reinstaller); ok {
            response, err = reinstaller.ReinstallServer(ctx, runtimeTargetFromProvision(target), runtimeInstallRequest(target))
            if errors.Is(err, gpruntime.ErrNotImplemented) {
                // Inner adapter (e.g., value-typed DockerAdapter) claims no Reinstaller — fall back.
                response, err = s.runtime.InstallServer(ctx, runtimeTargetFromProvision(target), runtimeInstallRequest(target))
            }
        } else {
            // No Reinstaller at all — emulate via Install (beacon handles reinstall as install when stopped).
            // Enforce stopped precondition here (same as beacon's server.go:1336).
            if s.runtime != nil {
                if err := s.requireServerStopped(ctx, serverID); err != nil {
                    _ = s.store.SetServerInstallState(ctx, serverID, "failed", err.Error())
                    return ..., err
                }
            }
            response, err = s.runtime.InstallServer(ctx, runtimeTargetFromProvision(target), runtimeInstallRequest(target))
        }
    } else {
        response, err = s.runtime.InstallServer(ctx, runtimeTargetFromProvision(target), runtimeInstallRequest(target))
    }
    // ... existing failed/succeeded handling unchanged
}
```

Add helper (or reuse `ServerControlTarget` + `GetServer` status check) — `requireServerStopped` mirrors beacon logic without a new RPC:

```go
func (s *Service) requireServerStopped(ctx context.Context, serverID string) error {
    srv, err := s.store.GetServer(ctx, serverID)
    if err != nil { return err }
    if srv.Status == "running" || srv.Status == "starting" {
        return errors.New("server must be stopped before reinstalling")
    }
    return nil
}
```

**B. Fix the value/pointer receiver mismatch (one-line).**

Ensure `forge/api/internal/runtime/factory.go` always returns `*DockerAdapter` (pointer) not `DockerAdapter` value, so `Reinstaller` assertion succeeds. Audit `factory.go:19-31` against every adapter (`docker.go`, `podmanadapter.go`, `containerd.go`, `lxc.go`, `kubernetesadapter.go`, `firecrackeradapter.go`) — all `ReinstallServer` have pointer receivers (`func (r *DockerAdapter) ReinstallServer` at `docker.go:67`). Returning a value will never satisfy `Reinstaller`. Change factory to return pointers.

**Beacon side:** no change required — `beacon/internal/server/server.go:1331` already enforces `PowerState != Running/Starting` then forwards to `install` (which uses `BeginInstall` fencing). Keep beacon reinstall as the source of truth for the stopped precondition; the panel fallback mirrors it for consistency.

### 2.3 Migration SQL (additive)

No DB migration. Optional observability:

```sql
-- 212_reinstall_fallback_audit.sql (optional, not required)
-- Use existing state_transitions.reason = 'reinstall via install fallback' — no column needed.
-- If you want a counter:
ALTER TABLE servers ADD COLUMN IF NOT EXISTS reinstall_via_install_fallback boolean NOT NULL DEFAULT false;
```

Recommendation: **no migration** — rely on existing `SetServerInstallState` audit + logs.

### 2.4 Rollout steps

1. **Deploy API** with dual-path fix (try `Reinstaller`, fallback to `InstallServer` on `ErrNotImplemented`). Dual-read: old path (`Reinstaller` direct) still attempted first, new fallback only when `ErrNotImplemented`. No DB change.
2. **Fix factory** pointer returns in same deploy (or immediately after) — re-assert `*DockerAdapter` satisfies `Reinstaller` via compile-time check (`var _ Reinstaller = (*DockerAdapter)(nil)` already present in `orchestrator/check_test.go:10` for `clustermanager.Service` — add same for each adapter).
3. **No beacon deploy needed** — beacon reinstall already works.
4. **Remove old hard-error path** (`errors.New("runtime does not support reinstall")`) after one release; keep fallback.

Rollback: previous API binary — reinstall returns to 500, no data loss.

### 2.5 Frontend impact

Current UI: `web/app/api/proxy/servers/[serverId]/reinstall/route.ts` (proxy) → `POST /servers/:id/reinstall`. On 500 `"runtime does not support reinstall"` the UI shows generic error; after fix, same call returns `202 {operationId}` (via `handlers_servers.go:1129` `DispatchReinstall`). Frontend poller should handle `202` identically to install (already does for `POST /servers/:id/install`). No new badge needed; just surface `operationId` and poll `GET /operations/:id`.

### 2.6 Testing strategy

**Unit — api:** `forge/api/internal/services/clustermanager/service_test.go` (extend)

```go
func TestReinstall_FallsBackToInstallWhenNoReinstaller(t *testing.T)
// mock runtime that implements only InstallServer, not Reinstaller
// assert ReinstallServer succeeds via fallback and calls InstallServer

func TestReinstall_UsesReinstallerWhenPresent(t *testing.T)
// mock runtime that implements Reinstaller — assert ReinstallServer called, not InstallServer

func TestReinstall_RequiresStopped(t *testing.T)
// server Status=running → Reinstall returns 409 even via fallback
```

**Contract — runtime:** `forge/api/internal/runtime/multiruntime_test.go:252` and `adapter_reinstall_nil_client` tests already cover delegation; add:

```go
func TestFactoryReturnsReinstallerCapableDocker(t *testing.T)
// rt := NewRuntime("docker"); _, ok := rt.(Reinstaller); assert ok
```

**Integration:** `handlers_servers_test.go` — `POST /servers/:id/reinstall` with a Docker-typed server in `stopped` state → expect `202` not `500`.

### 2.7 Risk & mitigation

| Risk | Mitigation |
|---|---|
| Fallback reinstall bypasses provider-specific reinstall logic (e.g., LXC volume handling) | Gate fallback: only for `provider=="docker"` or when inner error is `ErrNotImplemented`; for LXC/K8s that intentionally lack reinstall, keep 500. Document provider matrix. |
| Double install (concurrent reinstall + install) | Both paths go through `clustermanager/service.go:232` `SetServerInstallState("installing")` which is atomic (`UPDATE ... WHERE id=$1`), and beacon's `BeginInstall` serializes. |
| Regression for providers that correctly lack Reinstaller | Add allowlist check before fallback; do not silently promote `ErrNotImplemented` for non-Docker providers. |

### 2.8 Effort + dependencies

**S** (half-day). No DB. Depends on nothing; should land before GH-09 guard widens reinstall fencing (reinstall must not be blocked by `restoring_backup` semantics — reinstall during restore should be rejected, not fallen back).

---

## 3. GH-09 — `restoring_backup` never wired as server-level lock

### 3.1 Current broken code + why it breaks

**Forge API:** `forge/api/internal/http/handlers_servers.go:2104-2117` (restore handler) and `main.go:638-639`

```go
// handlers_servers.go fallback sync path (when OperationService unavailable):
_ = cfg.Store.MarkBackupStatus(ctx, target.ServerID, backup.Name, "restoring", actorID)
restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 15*time.Minute)
if err := cfg.Daemon.RestoreBackup(...); err != nil {
    _ = cfg.Store.MarkBackupStatus(ctx, target.ServerID, backup.Name, "restore_failed", ...)
}
// durable path:
op, err := cfg.OperationService.DispatchBackupRestore(ctx, target.ServerID, backup.Name, ...)
```

```go
// cmd/api/main.go:638
_ = db.MarkBackupStatus(ctx, serverID, backup.Name, "restoring", payload.ActorID)
// ...
if err := daemonClient.RestoreBackup(...); err != nil {
    _ = db.MarkBackupStatus(context.Background(), serverID, backup.Name, "restore_failed", payload.ActorID)
}
```

**Only `backups.status` transitions** (`pending → restoring → restored/failed`) are written. **No `servers.status` or `servers.actual_state`** change. The database already has the state for it:

- `store_state.go:137-144` maps `ServerActualStateRestoringBackup` ↔ `status="restoring_backup"` via `serverStatusFromActual` / `serverActualFromStatus`.
- `store_servers_control.go:51` `powerSignalPriorStates` **already lists** `restoring_backup` as a power-stop-allowed prior state, and `store_nodes.go:1034` counts `status IN ('installing','restoring_backup')` for node load.
- But **no writer** ever sets `servers.status='restoring_backup'` or `servers.actual_state='restoring_backup'` during a restore.

**Why it breaks:** Pterodactyl `Server.php:393` blocks `power` / `suspend` / `reinstall` when `status == restoring_backup`. Forge's equivalent guards are:

- `ensureTransferIdle` (`handlers_servers.go:145`) only checks `IsServerTransferBlocking` (`transfer_state IN ('queued','running')` from `store_servers.go:346`), not `status == restoring_backup`.
- `handlers_servers.go:888` power handler checks only transfer, not installing or restoring.
- `beacon/manager.go:482` `HandlePower` checks `InstallationState=="installing"` and `Suspended`, but **not** a restore lock.
- `clustermanager/service.go:389` `RequestServerPower` checks `Suspended` for start/restart, but not restore.

So concurrent `POST /servers/:id/power {start}` and `POST /servers/:id/reinstall` **race** with an in-flight `POST /servers/:id/backups/restore` (which is a 15-minute durable operation). The restore may be halfway through overwriting `server.properties` / world data when a start boots a half-restored container — data corruption, or the restart's `stopServer` kills the restore container.

### 3.2 Target design (exact code change, backward compatible)

Wire `restoring_backup` as a **first-class server lock** in three layers (DB is source of truth, API enforces, Beacon enforces).

**A. Store — add server-level restore lock (additive, no drop).**

Reuse existing `servers.actual_state` enum value `restoring_backup` (already `CREATE TYPE server_actual_state` includes it via `021_true_state_persistence.sql` plus later `ALTER TYPE ... ADD VALUE` pattern). Add a dedicated guard method (mirrors `IsServerTransferBlocking`):

```go
// forge/api/internal/store/store_state.go — new
func (s *Store) IsServerRestoreBlocking(ctx context.Context, serverID string) (bool, error) {
    var actual string
    err := s.db.QueryRow(ctx, `SELECT actual_state::text FROM servers WHERE id=$1`, serverID).Scan(&actual)
    if err != nil { return false, err }
    return actual == string(ServerActualStateRestoringBackup), nil
}

// forge/api/internal/store/store_servers_control.go — extend guard used by power handlers
func (s *Store) IsServerActionBlocked(ctx context.Context, serverID string) (blocked bool, reason string, err error) {
    var status, actual, desired string
    var suspended bool
    err = s.db.QueryRow(ctx,
        `SELECT status, actual_state::text, desired_state::text, suspended FROM servers WHERE id=$1`, serverID,
    ).Scan(&status, &actual, &desired, &suspended)
    if err != nil { return false, "", err }
    if status == "restoring_backup" || actual == "restoring_backup" { return true, "server restore in progress", nil }
    if status == "installing" { return true, "server is installing", nil }
    if suspended && (desired == "running") { return true, "server is suspended", nil }
    // also check transfer
    blocked, err = s.IsServerTransferBlocking(ctx, serverID)
    if blocked { return true, "server transfer in progress", nil }
    return false, "", err
}
```

Keep `IsServerTransferBlocking` for callers that only care about transfers; new `IsServerActionBlocked` is the unified power/reinstall guard.

**B. API — set and clear the server lock around durable restore.**

In `cmd/api/main.go:603` restore handler (the durable `OpBackupRestore` worker):

```go
// Before marking backup restoring, also lock the server:
if err := db.SetServerActualState(ctx, serverID, store.ServerActualStateRestoringBackup, "backup restore started"); err != nil {
    // log but continue — backup lock is still progress
}
// ... existing MarkBackupStatus(ctx, serverID, backup.Name, "restoring", ...)
if err := daemonClient.RestoreBackup(...); err != nil {
    _ = db.MarkBackupStatus(context.Background(), serverID, backup.Name, "restore_failed", payload.ActorID)
    // Unlock server on failure — restore_failed does not keep lock
    _ = db.SetServerActualState(context.Background(), serverID, store.ServerActualStateStopped, "backup restore failed")
    return err
}
if err := db.MarkBackupStatus(ctx, serverID, backup.Name, "restored", payload.ActorID); err != nil { return err }
// Unlock server on success:
_ = db.SetServerActualState(ctx, serverID, store.ServerActualStateStopped, "backup restore completed")
```

Mirror the same in the fallback sync paths `handlers_servers.go:2170` and `2336` (when `OperationService == nil`). Those already do `MarkBackupStatus("restoring")` — add the `SetServerActualState` call before and after.

**Important:** `SetServerActualState` (`store_state.go:31`) already writes `status` via `serverStatusFromActual` (so `status='restoring_backup'`) and `last_observation_at`, and conditionally promotes `observed_generation`. It also records `state_transitions` audit row (`recordStateTransition:120`). No new column needed — we reuse existing machinery.

**C. API — fence power / reinstall / suspend while restoring.**

Replace every `ensureTransferIdle` check on mutating server routes with the unified guard. At minimum, cover:

```go
// handlers_servers.go:888 power
if blocked, reason, _ := cfg.Store.IsServerActionBlocked(ctx, c.Params("id")); blocked {
    return fiber.NewError(fiber.StatusConflict, reason)
}
if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil { return err } // keep for legacy callers if desired, or fold into IsServerActionBlocked

// handlers_servers.go:1050 install, 1110 reinstall, 2685 decompress, file mutations, suspension
```

For `POST /servers/:id/reinstall` specifically, a restoring server must return `409 server restore in progress` — do not fall back to install.

**D. Beacon — enforce restore lock at the runtime edge (defence in depth).**

Beacon already knows `actual_state` via `syncServerStateFromPanel` (`manager.go:689`) which fetches `ServerControlTarget` + mounts + env. Extend the sync payload to include `actual_state`/`status` (add to `ServerControlTarget` or a new `ServerLifecycleState` struct). In `manager.go`, add a `RestoringBackup bool` field to `ServerState` (mirrors `Suspended`/`InstallationState`), set it from panel sync, and gate `HandlePower` and `BeginInstall`:

```go
type ServerState struct {
    // ...
    RestoringBackup bool // new, set from panel actual_state == restoring_backup
}

func (m *ServerManager) HandlePower(...) error {
    if state.RestoringBackup {
        return errors.New("server restore in progress")
    }
    // existing Installing / Suspended checks
}
func (m *ServerManager) BeginInstall(...) error {
    if state.RestoringBackup {
        return errors.New("server restore in progress")
    }
}
```

Clear `RestoringBackup` on the next successful panel sync after restore completes (when `actual_state != restoring_backup`).

**Generation integration:** `restoring_backup` does **not** bump `servers.generation` — it is not a fencing event, it is a mutual-exclusion lock. Fencing (`generation+lease`) is for node loss; restore lock is for operation serialization.

### 3.3 Migration SQL (additive, no drop)

No mandatory migration — the enum value and columns already exist. Two optional additive migrations for observability and for callers that query `backups.status` while also wanting a server-level index:

```sql
-- 213_restore_server_lock.sql (additive, all IF NOT EXISTS)
-- 1. Ensure server_actual_state has restoring_backup even on DBs created before 021
DO $$
BEGIN
    BEGIN
        ALTER TYPE server_actual_state ADD VALUE IF NOT EXISTS 'restoring_backup';
    EXCEPTION WHEN duplicate_object THEN NULL;
    END;
END $$;

-- 2. Index for fast "is restoring?" queries (used by IsServerRestoreBlocking / IsServerActionBlocked)
CREATE INDEX IF NOT EXISTS servers_restoring_actual_state_idx
    ON servers (actual_state) WHERE actual_state = 'restoring_backup';
CREATE INDEX IF NOT EXISTS servers_status_restoring_idx
    ON servers (status) WHERE status = 'restoring_backup';

-- 3. Optional: add a lightweight restores audit table if backup.status alone is insufficient for server-level polling
-- (Recommended: reuse existing backups.status + servers.actual_state, so this table is NOT created by default.
--  If created, it has no FK to servers, mirroring server_orphan_remediations pattern.)
CREATE TABLE IF NOT EXISTS server_restore_locks (
    id uuid PRIMARY KEY,
    server_id uuid NOT NULL,
    backup_name text NOT NULL,
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    status text NOT NULL DEFAULT 'restoring' CHECK (status IN ('restoring','restored','restore_failed')),
    actor_id uuid
);
CREATE INDEX IF NOT EXISTS server_restore_locks_server_idx ON server_restore_locks (server_id, started_at DESC);
```

**Feature flag variant (if you want gradual rollout):**

```sql
-- Additive, backward compatible
CREATE TABLE IF NOT EXISTS feature_flags (
    key text PRIMARY KEY,
    enabled boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO feature_flags(key, enabled) VALUES ('server.restore.lock', true)
ON CONFLICT (key) DO NOTHING;
```

When the flag is `false`, the API still writes `backups.status='restoring'` but skips `SetServerActualState(restoring_backup)`. When `true`, both are written and `IsServerActionBlocked` enforces. Default `true` after verification.

### 3.4 Rollout steps

1. **DB:** deploy `213_restore_server_lock.sql` (enum idempotent, indexes concurrently if on large table — use `CREATE INDEX CONCURRENTLY` via `migration_runner` additive path, or accept brief lock on small `servers` table). No column drops; existing `actual_state='restoring_backup'` rows (if any from prior manual edits) immediately benefit from the new index.
2. **Deploy API** with dual-write: `MarkBackupStatus("restoring")` **and** `SetServerActualState(restoring_backup)` in both durable (`main.go:638`) and fallback (`handlers_servers.go:2170/2336`) paths, and dual-read: `IsServerActionBlocked` checks both `backups.status` (fallback for old rows not yet migrated) and `servers.actual_state`. Power/reinstall handlers return `409` with distinct `reason` so the frontend can render a restoring badge.
   - Keep a `FIX_FORGE_RESTORE_LOCK` flag if you want dark-launch, but recommend shipping enabled — the change is fail-safe (adds a 409, never removes one).
3. **Deploy Beacon** with `RestoringBackup` gate on `HandlePower`/`BeginInstall` (fail-open if sync omits the field — treat missing as `false`).
4. **Backfill / cleanup:** no backfill needed; restoring is transient. After deploy, verify `SELECT count(*) FROM servers WHERE actual_state='restoring_backup'` correctly shows only genuinely restoring servers (not stale — see testing reaper).
5. **Remove flag** (if used) after one release; keep dual-read for at least one more release, then drop the `backups.status` fallback read.

Rollback: redeploy previous API binary — server-level `restoring_backup` rows remain but are harmless; power handlers revert to only checking `transfer`. To fully revert, run `UPDATE servers SET actual_state='stopped', status='stopped' WHERE actual_state='restoring_backup'` (advisory, not destructive — no column to drop).

### 3.5 Frontend impact

- **Badge:** `web/lib/api.ts:5` `ServerStatus = 'running'|'stopped'|'installing'` must add `'restoring_backup'` (and likely `'restoring'` alias for display). Update `web/components/*` that switches on `status` to render an amber "Restoring backup — controls disabled" badge. List of files to touch: `web/app/**/*server*.tsx`, `web/components/server-*` (power buttons, install progress, suspension toggle).
- **Controls disabled:** While `actualState=='restoring_backup'` (or `status=='restoring_backup'`), disable Start / Restart / Kill / Reinstall / Suspend buttons; show tooltip `Server restore in progress`. Kill piercing (GH-05) should **not** pierce a restore lock — kill during restore must remain `409` (restore is deliberately holding files).
- **Progress:** Existing `backup-manager.tsx:139` `pollOperation` already polls `GET /operations/:id` for restore; extend it to also poll `GET /servers/:id` and reflect `actual_state` transition `restoring_backup → stopped` in the timeline (reuse `DeploymentTimeline`-style component if present, or just the restore status card at `backup-manager.tsx:290`).
- **Empty/loading:** No change to `states-empty.tsx` / `states-error.tsx`.

### 3.6 Testing strategy

**Unit — store:** `forge/api/internal/store/store_state_test.go` (new)

```go
func TestIsServerRestoreBlocking(t *testing.T)
- Create server, SetServerActualState(restoring_backup) → IsServerRestoreBlocking == true
- SetServerActualState(stopped) → false
- IsServerActionBlocked returns true with reason "server restore in progress" for both power and reinstall paths
- Ensure state_transitions row recorded with reason "backup restore started"
```

**Unit — http:** `forge/api/internal/http/handlers_servers_test.go`

```go
func TestPowerBlockedDuringRestore(t *testing.T)
- Stub Store with server actual_state=restoring_backup
- POST /servers/:id/power {start} → 409 "server restore in progress"
- POST /servers/:id/reinstall → 409
- POST /servers/:id/backups/restore while already restoring → 409 (second restore rejected)
```

**Handler — operation worker:** `cmd/api/main_test.go` or new `backup_restore_lock_test.go`

- Dispatch `OpBackupRestore`, assert `servers.actual_state='restoring_backup'` is set before `daemonClient.RestoreBackup` is called, and cleared to `stopped` after success/failure. Simulate daemon failure → assert `status='stopped'` + `backup.status='restore_failed'`.

**Integration — beacon:** `beacon/internal/server/manager_test.go`

```go
func TestHandlePowerRejectedWhenRestoring(t *testing.T)
- state.RestoringBackup = true
- HandlePower(start) → error "server restore in progress"
- BeginInstall → same
- After clearing RestoringBackup, both succeed
```

**Reaper / stale unlock:** Add a `FailStaleRestores` analogue to `FailStaleBackups` (`store_backups.go:348`) for `servers.actual_state='restoring_backup'` held > 30m without an `operations` row in `running` — mark `actual_state='unknown'` with reason `reaper: restoring stuck`. Covered by existing `operation/service.go:169` reaper + `store.Touch` heartbeat; verify kill does **not** clear restore lock.

### 3.7 Risk & mitigation

| Risk | Mitigation |
|---|---|
| Stale `restoring_backup` lock after crash (daemon killed, panel never got `restored`) leaves server permanently 409 | Reaper that checks `servers.actual_state='restoring_backup' AND last_observation_at < now()-30m AND no running OpBackupRestore for this server` → clear to `stopped`. Also clear on next successful `RestoreBackup` callback. |
| Dual-write inconsistency (`backups.status='restoring'` but `servers.actual_state` not yet updated due to crash) | Dual-read `IsServerActionBlocked` checks **both**; API deploy writes both in same request (not transactionally, but second write failure is logged and re-attempted by the operation worker). Panel sync on next Beacon `syncServerStateFromPanel` reconciles `RestoringBackup` from either field. |
| Enums: `restoring_backup` value missing on old DBs | Migration `213` does `ADD VALUE IF NOT EXISTS` before any write, so first API write cannot fail with `invalid input value for enum`. |
| Kill pierce vs restore lock conflict | Explicit rule: kill **does not** pierce restore lock (different guarantee). Implement `HandlePower` check order: `RestoringBackup → Installing → Suspended → TryLock`. Kill pierce only applies to the `TryLock`/`RunningAction` gate, not the restore gate. |
| Frontend badge missing → operator confusion | Ship frontend badge in same release or one release behind; API 409 message is self-descriptive even before badge lands. |

### 3.8 Effort + dependencies

**M** (2–3 days: store method + handler wiring + operation worker + beacon gate + frontend badge). Depends on nothing except GH-05 ordering note (restore lock must be checked **before** kill pierce). Must land before addressing GH-12 fleet-wide fencing races.

---

## 4. GH-14 — Regex slash bug in `validateVariableValue` blocks Pterodactyl egg imports

### 4.1 Current broken code + why it breaks

**Forge API:** `forge/api/internal/store/store_egg_variables.go:176-182`

```go
case "regex":
    pattern, err := regexp.Compile(arg) // arg still contains /.../ delimiters
    if err != nil {
        return errors.New("invalid regex validation rule")
    }
    if !pattern.MatchString(value) {
        return errors.New("value does not match the required pattern")
    }
```

`validateVariableValue` splits `rules` by `|` (`strings.Split(rules, "|")`), then `strings.Cut(rule, ":")` into `name, arg`. For a Pterodactyl egg import, `rules` arrives as `regex:/^0\.0\.0\.0$/` or `regex:/^[a-z0-9_-]+$/` or the Paper example `minecraft-paper.json:58` referenced in `FINAL_PARITY_AUDIT GH-14` — specifically a rule whose pattern legitimately contains `|` (OR) **inside a character class** or group, e.g. `regex:/^(foo|bar)$/` or `regex:/^[A-Za-z0-9|_-]+$/`. Two bugs:

1. **Delimiter not stripped:** `arg` is `/^0\.0\.0\.0$/` (with leading/trailing `/`). `regexp.Compile("/^0\\.0\\.0\\.0$/")` compiles slashes as **literal** characters, so `0.0.0.0` fails `MatchString`. Every PTDL import with a regex rule fails validation, even though the egg is valid on Panel.
2. **`|` split breaks patterns containing `|`:** `strings.Split(rules, "|")` will split `regex:/^(foo|bar)$/|required` into `["regex:/^(foo", "bar)$/", "required"]`, corrupting the pattern and misclassifying `bar)$/` as an unsupported rule. This is the second half of the P0 — `required|regex:/.../` imports break on any pattern with alternation.

**Why it stays BROKEN:** Prior attempt likely fixed `|` splitting for `in:` but not `regex:`. The file still shows naive `strings.Split(rules, "|")` at line 143.

### 4.2 Target design (exact code change, backward compatible)

Fix both bugs in `store_egg_variables.go:142` without changing the rule language or stored values.

**A. Split rules without cutting inside `regex:`**

Add a helper that respects `regex:` as an atomic rule (the `|` inside a regex pattern belongs to the regex, not to the rule list). Strategy: scan `rules` left-to-right, accumulating `regex:` until a `|` that is **not** inside the regex delimiter pair, or simpler — split then re-stitch `regex:` fragments.

```go
// forge/api/internal/store/store_egg_variables.go — new helper

// splitRules splits a Panel rule string like "required|regex:/^(foo|bar)$/|max:32"
// into rule atoms without splitting the | that belongs inside a regex pattern.
// Panel's wire format is deterministic: every regex rule is "regex:/.../"
// (slashes are delimiters) and regex is always the last rule or followed by |.
// We therefore stitch: after seeing a token that starts with "regex:/" and does
// not end with "/" (modifiers may follow), concatenate subsequent | tokens
// until we have consumed the closing "/".
func splitRules(rules string) []string {
    if strings.TrimSpace(rules) == "" {
        return nil
    }
    raw := strings.Split(rules, "|")
    out := make([]string, 0, len(raw))
    for i := 0; i < len(raw); i++ {
        tok := strings.TrimSpace(raw[i])
        if strings.HasPrefix(tok, "regex:") {
            // Re-stitch if the regex pattern itself contained |
            // The closing slash is the delimiter before any modifiers (e.g. /.../i)
            // We detect it by the presence of a "/" that closes the pattern.
            // A robust heuristic: keep appending "|"+next while the concatenated
            // arg does not have a closing "/" (ignoring escaped \/).
            stitched := tok
            for !regexTokenClosed(stitched) && i+1 < len(raw) {
                i++
                stitched += "|" + raw[i]
            }
            out = append(out, stitched)
        } else {
            out = append(out, tok)
        }
    }
    return out
}

func regexTokenClosed(tok string) bool {
    // tok is like "regex:/.../" or "regex:/.../i" or "regex:/...|foo/"
    // Find the regex: prefix, then scan arg for unescaped closing /.
    _, arg, ok := strings.Cut(tok, ":")
    if !ok { return true }
    arg = strings.TrimSpace(arg)
    if arg == "" { return true }
    if arg[0] != '/' { return true } // not slash-delimited, treat as closed
    // scan for closing unescaped /
    escaped := false
    for j := 1; j < len(arg); j++ {
        c := arg[j]
        if escaped { escaped = false; continue }
        if c == '\\' { escaped = true; continue }
        if c == '/' { return true }
    }
    return false
}
```

**B. Strip delimiters and honour trailing flags before compiling.**

Inside `case "regex":` (currently `store_egg_variables.go:176`):

```go
case "regex":
    pat := strings.TrimSpace(arg)
    // Strip Pterodactyl /.../ delimiters if present, handling escaped slashes
    // and trailing flags (e.g. /pattern/i). Forge compiles with Go RE2 (no
    // PCRE lookaheads); flags we support: i (case-insensitive) -> (?i).
    cleaned, flags, err := stripRegexDelimiters(pat)
    if err != nil {
        return errors.New("invalid regex validation rule")
    }
    if cleaned == "" {
        return errors.New("invalid regex validation rule")
    }
    goPat := cleaned
    if strings.Contains(flags, "i") {
        goPat = "(?i)" + goPat
    }
    // Reject flags Forge does not support (m, s, x) with a clear error
    // rather than silently compiling the wrong pattern.
    if flags != "" && flags != "i" && flags != "i" { // only i allowed
        // stripRegexDelimiters already validates flags
    }
    re, err := regexp.Compile(goPat)
    if err != nil {
        return errors.New("invalid regex validation rule")
    }
    if !re.MatchString(value) {
        return errors.New("value does not match the required pattern")
    }

func stripRegexDelimiters(pat string) (pattern, flags string, err error) {
    if pat == "" { return "", "", errors.New("empty") }
    if pat[0] != '/' {
        // Not slash-delimited — treat as raw Go pattern (backward compat)
        return pat, "", nil
    }
    // Find the closing unescaped /
    escaped := false
    closeIdx := -1
    for i := 1; i < len(pat); i++ {
        c := pat[i]
        if escaped { escaped = false; continue }
        if c == '\\' { escaped = true; continue }
        if c == '/' { closeIdx = i; break }
    }
    if closeIdx == -1 {
        return "", "", errors.New("unterminated regex delimiter")
    }
    inner := pat[1:closeIdx] // between slashes
    flags = pat[closeIdx+1:] // after closing slash
    // Validate flags (only i supported for now; others rejected)
    for _, f := range flags {
        if f != 'i' {
            return "", "", fmt.Errorf("unsupported regex flag %q", string(f))
        }
    }
    // Unescape \/ → / inside inner (Pterodactyl escapes slashes)
    inner = strings.ReplaceAll(inner, `\/`, `/`)
    return inner, flags, nil
}
```

**Keep raw-pattern backward compatibility:** if `pat` does not start with `/`, treat it as already-clean Go pattern (no stripping). This ensures existing Forge-authored eggs that stored `regex:^[a-z]+$` without delimiters continue to work.

**Where to apply:** `validateVariableValue` is called from `validateEggVariableRequest:139`, `CreateServer:278` (startup variable validation), and `store_startup.go:73` — fixing it once covers all paths.

### 4.3 Migration SQL (additive)

No DB migration for the fix — `egg_variables.rules` is `text` storing the raw rule string with delimiters; we stop mis-compiling it, no column change.

Optional additive migration to **record** that the bug existed and which eggs were affected (for support):

```sql
-- 214_egg_variable_regex_fix_marker.sql (optional, additive)
ALTER TABLE egg_variables ADD COLUMN IF NOT EXISTS regex_fix_applied_at timestamptz;
CREATE INDEX IF NOT EXISTS egg_variables_regex_rules_idx
    ON egg_variables (egg_id) WHERE rules LIKE '%regex:%';
-- Backfill marker for eggs whose rules contain slash-delimited regex (for dashboard badge)
UPDATE egg_variables SET regex_fix_applied_at = now()
WHERE rules LIKE '%regex:/%' AND regex_fix_applied_at IS NULL;
```

Not required for correctness — the fix is pure Go. Include only if you want a “validated after fix” signal.

### 4.4 Rollout steps

1. **No DB step required** for correctness. If `214` marker desired, deploy it first (additive, `IF NOT EXISTS`, no downtime).
2. **Deploy API** with fixed `store_egg_variables.go` (helper + delimiter stripping + `|` stitching). Dual-read behaviour: old code accepted raw Go patterns without slashes; new code still accepts them. No flag needed — the fix is strictly more permissive (previously rejected patterns now accept), never more restrictive.
3. **Re-validate existing eggs:** run a one-off `SELECT id, rules FROM egg_variables WHERE rules LIKE '%regex:/%'` and call `validateVariableValue(default_value, rules)` in-process to surface any eggs that now fail due to genuinely invalid PCRE patterns not supported by RE2 (e.g., lookaheads). Those need manual egg edits, not code revert.
4. **Remove no old path** — this is a bugfix, not a feature dual. No flag removal needed.

Rollback: redeploy previous API binary; PTDL imports revert to failing, no data loss.

### 4.5 Frontend impact

Minimal. Template/egg admin UI that creates/edits egg variables (`web/app/admin/templates/*` or `packages/game-templates`) may have shown `value does not match the required pattern` for valid imports. After fix, those forms will succeed. No badge needed. If the admin egg editor shows a regex preview, ensure it strips delimiters similarly before calling `RegExp` in JS (mirror the Go fix in `web/lib/validation.ts` if such preview exists).

### 4.6 Testing strategy

**Unit — store:** `forge/api/internal/store/store_egg_variables_test.go` (new)

```go
func TestValidateVariableValue_RegexSlashDelimiters(t *testing.T)
    table:
    - {"host", "regex:/^0\\.0\\.0\\.0$/", "0.0.0.0"} → pass (previously failed)
    - {"name", "regex:/^[a-z0-9_-]+$/", "my-server_01"} → pass
    - {"choice", "regex:/^(foo|bar)$/", "foo"} → pass, "bar" → pass, "baz" → fail
    - {"flag", "regex:/^hello$/i", "HELLO"} → pass (case-insensitive)
    - {"raw", "regex:^[a-z]+$", "abc"} → pass (no slashes, still works)
    - {"escaped slash", `regex:/^a\/b$/`, "a/b"} → pass
    - {"broken delimiters", "regex:/unclosed", "anything"} → error "invalid regex validation rule"

func TestValidateVariableValue_PipeInsideRegex(t *testing.T)
    - "required|regex:/^(foo|bar)$/|max:10" with value "foo" → pass
    - same rule with value "baz" → fail
    - "regex:/^[a|b]+$/|required" with value "a|b" → pass ( | inside char class, not rule split)

func TestSplitRules(t *testing.T)
    - "required|regex:/^(foo|bar)$/|max:32" → ["required","regex:/^(foo|bar)$/","max:32"]
    - "regex:/^[a|b]+$/" → ["regex:/^[a|b]+$/"]
```

**Import parity test:** `forge/api/internal/store/store_egg_variables_import_test.go`

- Load `reference/pterodactyl-minecraft-paper.json` (or the repo's `minecraft-paper.json:58` fixture) and call `validateEggVariableRequest` for each variable with its `default_value` — all must pass.

**Integration:** `POST /eggs/:id/variables` with `rules: "regex:/^0\\.0\\.0\\.0$/"` and `defaultValue: "0.0.0.0"` → 201 not 422.

### 4.7 Risk & mitigation

| Risk | Mitigation |
|---|---|
| Stripping slashes breaks existing Forge-authored rules that intentionally include literal slashes | Flag: only strip when pattern **starts** with `/` and has a closing unescaped `/` — raw Go patterns without delimiters pass through unchanged. |
| `|` stitching incorrect for non-regex rules containing `|` (e.g., `in:a,b\|c`) | `splitRules` only stitches when token starts with `regex:` and its arg is slash-delimited and not yet closed — `in:` rules are never stitched. |
| PCRE vs RE2 incompatibility (lookahead `(?=...)`) after delimiters removed, pattern still invalid in Go | Fail honestly with `invalid regex validation rule` (422) — same as before, but now for the right reason. Document RE2 subset for egg authors. |
| Flags other than `i` silently ignored | Reject unknown flags with `unsupported regex flag` error rather than silent mismatch. |

### 4.8 Effort + dependencies

**S** (half-day). No DB. No dependencies; can land independently. Must land before any PTDL egg re-import campaign.

---

## 5. GH-12 + GH-04 — Allocation kill/start vs installing & API enqueues doomed ops

### 5.1 Current broken code + why it breaks

**Forge API:** `forge/api/internal/http/handlers_servers.go:888` `POST /servers/:id/power`

```go
protected.Post("/servers/:id/power", func(c *fiber.Ctx) error {
    if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil { // only transfer!
        return err
    }
    // ... checks perm, then directly DispatchPower / Queue dispatch
    // no check for installing, restoring_backup, suspended
})
```

`ensureTransferIdle:145` only queries `IsServerTransferBlocking` (`transfer_state IN ('queued','running')`). It does not check `status == installing` or `actual_state == installing` or `actual_state == restoring_backup` or `suspended == true`.

**Result:** While `status == 'installing'` (installer container 1000:1000 `mgp-*_installer` running via `beacon/runtime/docker.go:256` readonly+capDrop), the API enqueues `start`/`restart`/`kill` operations that the beacon will immediately reject (`manager.go:494-495` blocks power when `InstallationState == "installing"`). The operation worker then retries (`operation/service.go:284` backoff) creating a retry storm. Similarly, a `stop` during install is meaningless (install already holds the container namespace), and a `kill` during install should either be rejected (install is not a normal power state) or pierce to `EndInstall(failed:true)` — currently it is just retried.

**Reference truth:** `wings/power.go:57` blocks `HandlePower` when `IsInstalling` (except `kill` at `:108` pierces). Forge beacon **does** block (`manager.go:494` `if state.InstallationState == "installing" { return errors.New("server is installing") }`), but the **panel** does not pre-check, so the failure is deferred to the durable queue.

**GH-12 allocation dimension:** `handlers_servers.go:647` `POST /servers/:id/allocations` (create) and `681` `DELETE` also call `ensureTransferIdle`-only. Changing allocations while an install is remaking the container's network namespace can leave the new container with stale port maps or orphan the allocation.

### 5.2 Target design (exact code change, backward compatible)

**Additive guard helper (reuses GH-09 helper):**

```go
// forge/api/internal/store/store_state.go or store_servers_control.go

// IsServerPowerActionAllowed reports whether a given signal may be enqueued.
// installing → only kill is allowed (and even that is serialized via EndInstall).
// restoring_backup → no power signal is allowed (see GH-09).
// suspended → start/restart/kill rejected at panel (beacon also rejects).
func (s *Store) IsServerPowerActionAllowed(ctx context.Context, serverID, signal string) (bool, string, error) {
    var status, actual string
    var suspended bool
    if err := s.db.QueryRow(ctx,
        `SELECT status, actual_state::text, suspended FROM servers WHERE id=$1`, serverID,
    ).Scan(&status, &actual, &suspended); err != nil { return false, "", err }
    if status == "restoring_backup" || actual == "restoring_backup" {
        return false, "server restore in progress", nil
    }
    if status == "installing" || actual == "installing" {
        if signal == "kill" {
            // Kill during install is allowed but must route through install cancellation,
            // not normal power. For now, let it through to beacon's piercing EndInstall.
            return true, "", nil
        }
        return false, "server is installing", nil
    }
    if suspended && (signal == "start" || signal == "restart" || signal == "install" || signal == "reinstall") {
        return false, "server is suspended", nil
    }
    if blocked, _ := s.IsServerTransferBlocking(ctx, serverID); blocked {
        return false, "server transfer in progress", nil
    }
    return true, "", nil
}
```

**Wire in every mutating handler:**

```go
// handlers_servers.go:888 power — replace ensureTransferIdle with unified guard
if ok, reason, err := cfg.Store.IsServerPowerActionAllowed(ctx, c.Params("id"), req.Signal); err != nil {
    return fiber.NewError(fiber.StatusNotFound, "server not found")
} else if !ok {
    return fiber.NewError(fiber.StatusConflict, reason)
}
// keep ensureTransferIdle as legacy compat if other callers depend on its error shape, or fold it.

// handlers_servers.go:647 allocate, 681 unassign, 697 primary, 1050 install, 1110 reinstall:
if blocked, reason, _ := cfg.Store.IsServerActionBlocked(ctx, c.Params("id")); blocked {
    return fiber.NewError(fiber.StatusConflict, reason)
}
```

**Operation worker fast-fail:** In `cmd/api/main.go` `OpServer*` handlers, check `IsServerPowerActionAllowed` **again** before `daemonClient.SendPower` — if now blocked (state changed since enqueue), return a non-retriable error (`*NonRetriableError`) so `operation/service.go:284` marks `failed` not `retrying`. This prevents a legitimate race (power enqueued when idle, then install started) from spinning.

**Beacon complement:** `manager.go:494` already blocks `HandlePower` during `installing` (except kill piercing). Keep it — the panel pre-check merely avoids enqueuing doomed ops; beacon remains the enforcement edge.

### 5.3 Migration SQL (additive)

No migration. Optional index for the guard query:

```sql
-- 215_power_guard_index.sql (optional, additive)
CREATE INDEX IF NOT EXISTS servers_status_actual_suspended_idx
    ON servers (status, actual_state, suspended) ;
-- Partial indexes already added in 213 for restoring/installing; this composite is optional.
```

If `IsServerPowerActionAllowed` is latency-sensitive (power is hot path), the single-row `SELECT ... WHERE id=$1` is already PK-indexed; no new index is load-bearing. Include only if `explain analyze` shows seq scan.

### 5.4 Rollout steps

1. **No DB deploy** (or optional `215` concurrently — no downtime, `IF NOT EXISTS`).
2. **Deploy API** with new guard wired into `POST /power`, `POST /allocations`, `DELETE /allocations`, `POST /install`, `POST /reinstall`, file mutations that touch the container FS when installing. Keep `ensureTransferIdle` call alongside for one release (dual-guard) to avoid missing legacy transfer-only callers, then fold it into `IsServerActionBlocked`.
3. **No beacon change** (beacon already fences correctly — GH-05 kill pierce is the only beacon edit needed for this cluster).
4. **Remove legacy `ensureTransferIdle`** delegation after verification (keep function for other routes that genuinely only care about transfers, like `GET /transfers`).

Rollback: previous API binary — guards revert to transfer-only, doomed ops reappear but self-heal via beacon 409 + retry limit.

### 5.5 Frontend impact

- Power buttons (`Start`/`Stop`/`Restart`/`Kill`) should be **disabled with tooltip** when `status == installing` or `actual_state == installing` or `restoring_backup`, mirroring the backend 409. `web/lib/api.ts:5` `ServerStatus` already includes `installing`; ensure the fetch layer maps `status` and `actual_state` both to that UI value (today `ServerStatus` only lists three values — add `restoring_backup` per GH-09 and ensure `installing` is honoured).
- Allocation add/remove UI (`allocations-view.tsx` or similar) should disable while installing/restoring with message `Allocations cannot be changed while the server is installing`.
- Install progress: when power is rejected with `409 server is installing`, the UI should **not** poll power operation — instead show install log poll (`GET /servers/:id/install/logs` or `GET /operations/:id` for the install operation).

### 5.6 Testing strategy

**Unit — store:** `store_state_test.go`

```go
func TestIsServerPowerActionAllowed(t *testing.T)
  rows: (status=installing, signal=start→false), (installing,kill→true),
        (restoring_backup,start→false), (suspended,start→false), (stopped,start→true)
```

**Unit — http:** `handlers_servers_test.go`

```go
func TestPowerRejectedWhenInstalling(t *testing.T)
  stub server status=installing
  POST /servers/:id/power {start} → 409 "server is installing"
  POST /servers/:id/power {kill}  → 202 (allowed)
func TestAllocationRejectedWhenInstalling(t *testing.T)
  POST /servers/:id/allocations {allocationId: new} while installing → 409
```

**Integration:** concurrent `POST /install` + `POST /power {start}` — assert exactly one of them gets 409, and the install operation succeeds; the power operation either fails fast or is marked `failed` without retrying 3 times.

**Contract:** verify `ensureTransferIdle` path still 409s for `transfer_state=running` even when not installing.

### 5.7 Risk & mitigation

| Risk | Mitigation |
|---|---|
| Kill during install now allowed but should cancel install, not just kill container | Decide semantics: GH-05 piercing kill during install should call `EndInstall(failed:true)` so `status` moves to `install_failed`, not `stopped`. Today `kill` and `EndInstall` are separate slots — document that kill-during-install cancels the install. |
| Over-fencing legitimate concurrent file writes during install (e.g., user uploads an egg asset while install is running) | Allocation/file guards during install should be conservative — block them; document that file mutations during install are queued after install via operation queue. |
| Missing guard on a new route | Audit every `ensureTransferIdle` call site (grep list in 5.1) and replace; add a lint rule (`grep ensureTransferIdle` in CI) that fails if new code calls it without also checking install/restore. |

### 5.8 Effort + dependencies

**S** (half-day). No DB. Depends on GH-09's `IsServerActionBlocked` helper existing — otherwise duplicate logic. Should land in same PR as GH-09 or immediately after.

---

## 6. GH-06 / Generation Fencing — Install/Kill/Suspend fences + orphan handling + same-slot race and generation enforcement

### 6.1 Current broken code + why it breaks (four sub-findings)

**A. Same-slot `RunningAction` race.**

`beacon/internal/server/manager.go:271` `BeginInstall` and `manager.go:482` `HandlePower` share the **same** `ServerState.RunningAction` string and the same `mu TryLock`. A `BeginInstall` that wins the slot sets `RunningAction="install"`; a concurrent `HandlePower("start")` correctly gets `another server action is already running`. But the **reverse** is also true: a long `start` (e.g., `onBeforeStart` chown + panel sync `manager.go:643` + `runtime.Start`) blocks an `install` even when the operator has drained the server and wants to reinstall — the API's `POST /reinstall` enqueues a durable operation that will retry for minutes.

**B. Generation fencing exists in API but is unenforced on Beacon.**

- API side: `forge/api/migrations/110_node_fencing.sql:2` adds `servers.generation bigint` + `workload_lease_expiry`. `store_evacuation.go:33` increments `generation` on evacuation eligibility. `store.go:495` documents the contract: *"Beacon enforces this by rejecting operations whose generation is older than the control plane's current."*
- But **Beacon never checks** `generation`. `beacon/internal/server/manager.go` has **zero** occurrences of `generation`/`fence`/`lease` in power paths (`grep` in §B above shows none). `ServerProvisionTarget` (`store_servers_control.go:83`) does not even select `generation` or `workload_lease_expiry` — it selects `suspended, installed, status, runtime_provider` but not `generation`. The fence token is written on evacuation, never delivered to Beacon, never enforced. This is FINAL_PARITY_AUDIT §10.3 "Fencing twice, wrong edge, zero enforcement" and MASTER finding REF-ORCH-AF-3.

**C. Lease edge is wrong.**

`store_evacuation.go:33` sets `workload_lease_expiry = NOW()+1h` on eligible evacuation items. `manager.go:208` `crashAutoRestartWindow = 24h` and `persistedPowerState` treat the old instance as restartable for 24h. An evacuated server could be started on the new node **and** still be restartable on the old node after a daemon restart — split-brain.

**D. Orphan remediation only on delete/create-failure, not on install failure.**

`store_servers_lifecycle.go:18` `RecordOrphanAndHardDeleteServer` is only called from `clustermanager/service.go:292` `compensateCreateFailure` when `workloadCreated==true` but delete fails, and from `DeleteServer:349` when `force==true`. A failed `InstallServer` (`runInstaller:250` sets `status='failed'` but never records an orphan) that leaves a half-written `mgp-*_installer` container is not tracked — `server_orphan_remediations` has no row, so no reaper sweeps the installer container.

### 6.2 Target design (exact code change, backward compatible)

**A. Split the slot (Beacon).**

Change `ServerState` to have **two** independent concurrency tokens, not one string:

```go
type ServerState struct {
    mu sync.Mutex
    PowerState PowerState
    InstallationState string // "installing" / "installed" / "failed"
    // Replace single RunningAction string with two dedicated slots:
    PowerAction   string // "start"|"stop"|"restart"|"kill" or ""
    InstallAction string // "install" or ""
    // Alternative: keep RunningAction for log compatibility but also track
    // InstallAction separately; HandlePower checks PowerAction, BeginInstall checks InstallAction.
    Generation         int64      // new, synced from panel
    WorkloadLeaseExpiry *time.Time // new, synced from panel
}

func (m *ServerManager) BeginInstall(serverID string) error {
    state := m.State(serverID)
    if !state.mu.TryLock() { return busy }
    if state.PowerAction != "" { state.mu.Unlock(); return busy } // still fence against power
    if state.InstallAction != "" { state.mu.Unlock(); return busy }
    if state.RestoringBackup { state.mu.Unlock(); return restoreErr }
    state.InstallAction = "install"
    state.InstallationState = "installing"
    state.mu.Unlock()
    return nil
}
func (m *ServerManager) EndInstall(serverID string, failed bool) {
    state := m.State(serverID); state.mu.Lock(); defer state.mu.Unlock()
    state.InstallAction = ""
    if failed { state.InstallationState = "failed" } else { state.InstallationState = "installed" }
}
func (m *ServerManager) HandlePower(ctx context.Context, serverID, signal string) error {
    state := m.State(serverID)
    // Kill pierce (GH-05) still applies here — but now only needs to pierce PowerAction,
    // not InstallAction. Kill does NOT pierce an active install (install is not power).
    if signal == "kill" {
        // piercing logic touches PowerAction only
    } else {
        if !state.mu.TryLock() { return busy }
        if state.PowerAction != "" { state.mu.Unlock(); return busy }
        if state.InstallationState == "installing" { state.mu.Unlock(); return errors.New("server is installing") }
        if state.RestoringBackup { state.mu.Unlock(); return ... }
        state.PowerAction = signal
        state.mu.Unlock()
        defer func(){ state.mu.Lock(); if state.PowerAction==signal {state.PowerAction=""}; state.mu.Unlock()}()
    }
    // ... rest of HandlePower unchanged, but references PowerAction not RunningAction
}
```

Keep `RunningAction` as a computed view (`if InstallAction!="" { return InstallAction } return PowerAction`) for logs/metrics until all callers are migrated, then deprecate.

**B. Wire generation + lease through to Beacon and enforce.**

1. **API Store:** `store_servers_control.go:88` `ServerProvisionTarget` query — add `s.generation, s.workload_lease_expiry` to the SELECT and scan into `ServerProvisionTarget.Generation int64` + `WorkloadLeaseExpiry *time.Time`. Add `ServerControlTarget` similarly (it already joins `servers s` and `nodes n`).

   ```go
   type ServerProvisionTarget struct {
       // ... existing
       Generation         int64
       WorkloadLeaseExpiry *time.Time
   }
   ```

   Change `store_servers.go:502` `UpdateServerGeneration` to also be called from evacuation/recovery item creation (`store_evacuation.go:33` already does the SQL, keep it).

2. **Runtime wire:** `clustermanager/service.go:632` `runtimeCreateRequest` / `687` `runtimeInstallRequest` / `714` `runtimeConfiguration` / `730` `runtimeTargetFromProvision` — extend `gpruntime.Target` with `Generation int64` + `LeaseExpiry *time.Time` and `gpruntime.CreateServerRequest` similarly if needed. The daemon `POST /servers` and `POST /servers/{id}/install` JSON bodies must include `generation` and `leaseExpiry` (additive fields; old beacons ignore them, new beacons enforce).

3. **Beacon enforcement (additive, fail-open):**

   ```go
   // beacon/internal/server/manager.go
   func (m *ServerManager) CheckGeneration(serverID string, incomingGen int64, incomingLease *time.Time) error {
       state := m.State(serverID)
       state.mu.Lock()
       localGen := state.Generation
       localLease := state.WorkloadLeaseExpiry
       state.mu.Unlock()
       if incomingGen == 0 { return nil } // old panel, no generation — fail-open
       if localGen != 0 && incomingGen < localGen {
           return fmt.Errorf("stale generation %d < current %d: fenced", incomingGen, localGen)
       }
       if localLease != nil && time.Now().After(*localLease) && incomingGen == localGen {
           // Lease expired — workload must be stopped even if generation not yet bumped
           // (recovery path sets lease to 1h, new dest gets generation+1 and new lease)
       }
       return nil
   }

   // In Server.power, Server.install, Server.reinstall HTTP handlers (server.go:1052, 1331, 1357):
   // before calling runtime.Install or manager.HandlePower, read generation from request header
   // X-Forge-Generation (sent by API daemon client) and call CheckGeneration.
   // Also update state.Generation / state.WorkloadLeaseExpiry from the incoming request
   // if it is newer (monotonic).
   ```

   The API daemon client (`forge/api/internal/daemon/client.go`) must send `X-Forge-Generation: <target.Generation>` and `X-Forge-Lease-Expiry: <RFC3339>` on every `CreateServer`, `InstallServer`, `ReinstallServer`, `Power` call (additive header, old beacon ignores).

   Lease expiry enforcement: on each `HandlePower(start)` also check `if state.WorkloadLeaseExpiry != nil && time.Now().After(*state.WorkloadLeaseExpiry) { return errors.New("workload lease expired: server was fenced") }`. This closes split-brain after evacuation.

**C. Fix lease edge + split-brain window.**

- Change `store_evacuation.go:33` lease from `NOW()+1h` to align with `manager.go:208` `crashAutoRestartWindow` — either reduce beacon's window to `1h` or increase lease to `24h`. Recommendation: **set lease to 1h and reduce `crashAutoRestartWindow` to 1h** (evacuation fencing is the safety net, not crash restart). Add a comment in both places linking them.

**D. Extend orphan remediation to install/reinstall.**

In `clustermanager/service.go:224` `runInstaller`:

```go
response, err = s.runtime.InstallServer(ctx, ...)
if err != nil {
    // If installer container leaked (daemon reports container exists but install failed),
    // record an orphan remediation row so the beacon reaper can GC the mgp-*_installer volume.
    // Use the non-hard-delete variant: keep the server row, just record remediation.
    _ = s.store.RecordOrphanRemediation(ctx, serverID, target.NodeURL, "install failed: "+err.Error())
    _ = s.store.SetServerInstallState(ctx, serverID, "failed", err.Error())
    return ..., err
}
```

Add new store method `RecordOrphanRemediation` (insert into `server_orphan_remediations` without the `DELETE FROM servers`):

```go
// store_servers_lifecycle.go — new
func (s *Store) RecordOrphanRemediation(ctx context.Context, serverID, nodeURL, reason string) error {
    _, err := s.db.Exec(ctx,
        `INSERT INTO server_orphan_remediations (id, server_id, node_url, daemon_error)
         VALUES ($1,$2,$3,$4)`, uuid.NewString(), serverID, nodeURL, reason)
    return err
}
```

This reuses the table that `store_servers_lifecycle.go:36` already writes, so the existing reaper `handlers_orphan_remediations.go` surfaces it.

### 6.3 Migration SQL (additive, no drop)

```sql
-- 216_generation_fence_beacon.sql (additive, all IF NOT EXISTS)
-- 1. No new column on servers — generation + workload_lease_expiry already exist from 110.
--    Just ensure beacon state columns are indexed for fast fencing checks.
CREATE INDEX IF NOT EXISTS servers_generation_idx ON servers (generation);
CREATE INDEX IF NOT EXISTS servers_workload_lease_expiry_idx ON servers (workload_lease_expiry)
    WHERE workload_lease_expiry IS NOT NULL;

-- 2. Extend server_orphan_remediations to allow non-delete remediations (if not already).
--    The existing table has no FK to servers (correct — survives delete). Add a reason enum if desired.
ALTER TABLE server_orphan_remediations ADD COLUMN IF NOT EXISTS remediation_type text NOT NULL DEFAULT 'hard_delete'
    CHECK (remediation_type IN ('hard_delete','install_leak','fence_expired'));
-- Backfill
UPDATE server_orphan_remediations SET remediation_type='hard_delete' WHERE remediation_type IS NULL;

-- 3. Optional: track fencing deliveries to beacon (audit).
CREATE TABLE IF NOT EXISTS generation_fence_deliveries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id uuid NOT NULL,
    generation bigint NOT NULL,
    lease_expiry timestamptz,
    delivered_at timestamptz NOT NULL DEFAULT now(),
    beacon_accepted boolean NOT NULL DEFAULT true,
    error text
);
CREATE INDEX IF NOT EXISTS generation_fence_deliveries_server_idx ON generation_fence_deliveries (server_id, delivered_at DESC);

-- 4. Ensure daemon credential decrypt still works when generation is selected (no change, just guard).
--    No ALTER DROP. No column renames.
```

Feature flag (additive):

```sql
-- Additive flag to gate beacon generation enforcement (fail-open when false).
INSERT INTO beacon_feature_flags(key, enabled) VALUES ('beacon.generation.enforce', true)
ON CONFLICT (key) DO NOTHING;
```

### 6.4 Rollout steps

1. **DB:** deploy `216_generation_fence_beacon.sql` (indexes concurrently if needed; remediation_type column with default, no backfill lock on large table — column is tiny).
2. **Deploy API** with dual-write:
   - `ServerProvisionTarget` / `ServerControlTarget` now select and return `generation` + `leaseExpiry`.
   - Daemon client sends `X-Forge-Generation` + `X-Forge-Lease-Expiry` headers (additive, old beacon ignores).
   - `runInstaller` failure path writes `RecordOrphanRemediation` (non-destructive) in addition to `SetServerInstallState(failed)`.
   - Keep old code path that does not send headers for one release (dual-send), or send headers always — both are safe.
3. **Deploy Beacon** with split-slot + generation check (fail-open on `incomingGen==0` or `localGen==0`). Beacon logs `fenced generation 3 < 4 for <server>` and returns `412 Precondition Failed` with `generation stale` instead of `200`.
4. **Verify** via integration tests (see 6.6), then remove flag gate. Keep fail-open on zero generation for at least one major release to handle beacons that enroll with old API.

Rollback: redeploy previous API (stops sending headers) and previous beacon (ignores generation) — no data to revert; `remediation_type` rows with `install_leak` are harmless and remain queryable.

### 6.5 Frontend impact

- **Generation badge:** expose `generation` and `workloadLeaseExpiry` in `ServerDTO` (already present at `store.go:549`) — ensure `web/lib/api.ts:5` maps `generation` to a tooltip on the server list (e.g., `Fenced at generation 4, lease expires 2026-08-24T...`). Not required for correctness, but important for operators debugging split-brain.
- **Recovery/Evacuation UI:** `web/app/admin/recovery/*` and `web/app/admin/evacuation/*` should surface `generation` bump and `leaseExpiry` after plan creation (`store_evacuation.go:33`). Show "Old instance fenced until lease expiry" countdown.
- **Orphan badge:** `web/app/admin/orphans/page.tsx` (if exists) or `handlers_orphan_remediations.go` already lists `server_orphan_remediations`; ensure `remediation_type='install_leak'` rows appear there.

### 6.6 Testing strategy

**Unit — store:**

```go
// store_servers_control_test.go
func TestUpdateServerGeneration_Monotonic(t *testing.T)
func TestRecordOrphanRemediation_NoDelete(t *testing.T)
// assert server row still exists after RecordOrphanRemediation
// assert server_orphan_remediations count +1 with remediation_type install_leak
```

**Unit — beacon manager:**

```go
// beacon/internal/server/manager_test.go
func TestBeginInstall_AndHandlePower_IndependentSlots(t *testing.T)
// BeginInstall holds InstallAction, HandlePower(start) still rejected (fenced against install)
func TestHandlePower_KillDoesNotPierceInstall(t *testing.T)
// InstallAction="install", kill → still rejected (kill only pierces PowerAction)
func TestCheckGeneration_StaleRejected(t *testing.T)
// state.Generation=5, incoming 4 → error contains "fenced"
func TestCheckGeneration_ZeroFailOpen(t *testing.T)
// incomingGen 0 → no error
func TestWorkloadLeaseExpiry_BlocksStart(t *testing.T)
// lease in past → HandlePower(start) → "lease expired"
```

**Unit — api clustermanager:**

```go
// clustermanager/service_test.go
func TestRunInstaller_RecordsOrphanOnFailure(t *testing.T)
// mock runtime returns error, assert RecordOrphanRemediation called once with install_leak
```

**Integration — end-to-end fencing:**

- Provision server on node A, evacuate to node B (generation bump + lease). Attempt `POST /servers/:id/power {start}` against node A's beacon with old generation header → `412` fenced. Same against node B with new generation → `202`.

**Contract — daemon client headers:**

```go
// daemon/client_test.go
func TestCreateServer_SendsGenerationHeader(t *testing.T)
// capture outgoing HTTP, assert X-Forge-Generation header present when target.Generation>0
```

### 6.7 Risk & mitigation

| Risk | Mitigation |
|---|---|
| Splitting slot breaks code that reads `RunningAction` for metrics | Keep `RunningAction` as computed accessor for one release, deprecate after. |
| Beacon fail-open on `incomingGen==0` allows old panel to bypass new fence | Acceptable — the window is exactly one API deploy gap; panel upgrades are fast. After one release, make enforcement strict (reject `0` when `localGen>0` and `beacon.generation.enforce` flag true). |
| Lease reduction (24h→1h) causes legitimate crash restarts to be fenced too early | Tune `crashAutoRestartWindow` and lease together; document that crash restart is **not** a fence — lease is only for evacuation/recovery fencing, not for normal stops. |
| `generation` column NULL on old rows | Migration `110` set `DEFAULT 0`; treat `0` as "no fence" consistently (fail-open). |
| `RecordOrphanRemediation` inserts without FK could orphan forever | Existing `handlers_orphan_remediations.go` reaper already lists and allows manual GC; add a TTL sweep for `install_leak` rows older than 7d. |

### 6.8 Effort + dependencies

**M** (3–4 days: store query extension + runtime wire + beacon split-slot + generation check + orphan non-delete row). Depends on GH-05 (kill pierce semantics must not pierce `InstallAction`). Should land as a single PR after GH-05/GH-07/GH-09/GH-14 to avoid interleaving fence changes with regex fix.

---

## 7. GH-12 Sub-findings — Suspend, orphan, queue & transfer fencing

### 7.1 Current broken code + why it breaks

**Suspend:** `store_servers.go:374` `SetServerSuspension` is orthogonal boolean, but `store_servers_control.go:13` `SetServerPowerState` does not check `suspended` — `suspend` handler at `handlers_servers.go:1413` does `SendPower(stop)` best-effort then sets `suspended=true` without awaiting stop completion. A concurrent `start` between `SendPower` and `SetServerSuspended` slips through.

**Transfer vs power:** `ensureTransferIdle` only checks `transfer_state`; a restoring or installing server can still be queued for transfer, and a transferring server can still be power-cycled.

**Queue orphan:** `beacon/internal/server/queue.go:23` `OpKill` is a queued power op (`operations.EnqueueCommand` at `server.go:1376`), but `manager.go` `HandlePower` is also called synchronously from `applyPower:3406` and `completioncomplete:296` — two paths into the same `RunningAction` slot, with generation-unaware `EnqueueCommand`.

### 7.2 Target design

Consolidate all four signals into the unified helper `IsServerActionBlocked` / `IsServerPowerActionAllowed` added in §3.2/§5.2. Suspension check must be **CompareAndSet** where races matter:

```go
// handlers_servers.go:1413 suspension — make it atomic:
swapped, err := cfg.Store.CompareAndSetServerSuspension(ctx, id, expectedSuspended==false, true)
// expectedSuspended derived from current read; if swap fails, return 409 retry
```

### 7.3 Migration SQL

None (column `suspended` already boolean-indexed). Optional partial index for hot path:

```sql
CREATE INDEX IF NOT EXISTS servers_suspended_idx ON servers (id) WHERE suspended = true;
```

### 7.4 Rollout

1. API guard change only (see §5.4). No DB.
2. Beacon already enforces `Suspended` in `HandlePower` and `BeginInstall` — no beacon change.

### 7.5 Frontend

Disable Suspend/Unsuspend toggle while `installing`/`restoring_backup` to avoid 409.

### 7.6 Testing

Include suspend races in `TestIsServerPowerActionAllowed` and add `TestSuspend_CompareAndSetRace` in `store_servers_test.go`.

### 7.7 Risk

Suspend race: `CompareAndSet` prevents double-suspend; without it, two concurrent suspends idempotently succeed (acceptable). Document idempotency.

### 7.8 Effort

**S** (half-day, reuses GH-09/12 helpers).

---

## 8. Cross-cutting rollout plan (ordered)

| Phase | What ships | Flag | How to verify |
|---|---|---|---|
| **0. Migrate DB** | `211` (optional kill audit), `213` (restore lock indexes + enum guard), `214` (regex marker optional), `215` (power guard index optional), `216` (generation indexes + remediation_type) — all additive `IF NOT EXISTS`, run via `store/migration_runner` same as `092`/`110` | No flag | `SELECT * FROM state_transitions LIMIT 1` still works; `SELECT generation FROM servers LIMIT 1` returns 0 for old rows |
| **1. API — GH-14 regex** | `store_egg_variables.go:142` `splitRules` + `stripRegexDelimiters` | No flag (pure bugfix) | Re-import `minecraft-paper.json:58` PTDL fixture — variables create 201 not 422 |
| **2. API — GH-07 reinstall gate** | `clustermanager/service.go:224` fallback to `InstallServer` + factory pointer fix | No flag | `POST /servers/:id/reinstall` on Docker-stopped server → 202 not 500 |
| **3. Beacon — GH-05 kill pierce** | `manager.go:482` kill-pierce branch | `beacon.kill.pierce` default-on, fail-open | Integration `TestKillPiercesStuckStop` |
| **4. API — GH-09 restore lock** | `store_state.go:31` guard + `main.go:638` + `handlers_servers.go:2170/2336` dual-write + unified fence | `server.restore.lock` default-on (remove after 1 release) | `POST /power {start}` during `restoring_backup` → 409; beacon also rejects |
| **5. API — GH-12/04 power guard** | `IsServerPowerActionAllowed` wired into every mutating route | No flag | Concurrent `install` + `start` → exactly one 409 |
| **6. API+Beacon — Generation fencing** | `ServerProvisionTarget` selects `generation`/`lease`, daemon headers, `manager.go` split-slot + `CheckGeneration` + `RecordOrphanRemediation` non-delete | `beacon.generation.enforce` default-on, fail-open on `0` | Evacuation: old beacon 412 fenced, new beacon 202 |
| **7. Web — badges & disabled states** | `web/lib/api.ts:5` `ServerStatus` + status badge + disabled power/alloc controls + generation tooltip | No DB flag | Manual QA: restoring server shows amber badge, power disabled |

Each phase keeps the previous API/Beacon binary compatible (dual-read, fail-open on missing column/header, no `ALTER DROP`). Rollback is always: redeploy previous binary.

---

## 9. File:line index (every finding → every file)

| Finding | Forge API (control plane) | Beacon (node agent) | Migration | Web |
|---|---|---|---|---|
| GH-05 kill pierce | `services/operation/service.go:364` `DispatchPower` (priority) | `manager.go:482` `HandlePower`, `manager.go:387` `persistPowerState`, `server.go:3406` `applyPower` | — (optional `211`) | `api/proxy/operations/[operationId]/route.ts:13` poll render |
| GH-07 reinstall Docker gate | `services/clustermanager/service.go:241` `Reinstaller` check, `runtime/docker.go:67`, `runtime/multiruntime.go:106`, `runtime/registry.go:175`, `runtime/factory.go:19` pointer fix | `server.go:1331` `reinstall` (no change) | — | `api/proxy/servers/[serverId]/reinstall/route.ts` |
| GH-09 restoring_backup lock | `store/store_state.go:31` `SetServerActualState`, `store/store_state.go:137` `serverStatusFromActual`, `store_servers_control.go:51` `powerSignalPriorStates`, `http/handlers_servers.go:2170`/`2336` backup status, `cmd/api/main.go:638` operation handler, `store/store_backups.go:115` `MarkBackupStatus` | `manager.go:271` `BeginInstall` + `manager.go:482` `HandlePower` `RestoringBackup` gate, `manager.go:689` `syncServerStateFromPanel` | `213_restore_server_lock.sql` (`server_actual_state` guard + indexes) | `lib/api.ts:5` `ServerStatus`, install/restore badge |
| GH-14 regex slash | `store/store_egg_variables.go:142` `validateVariableValue` `case "regex":176`, `store/store_servers.go:278` startup var, `store/store_startup.go:73` | — | — (optional `214`) | `app/admin/templates/*` egg editor |
| GH-12/04 install/kill/alloc fence | `http/handlers_servers.go:145` `ensureTransferIdle`, `http/handlers_servers.go:888` `POST /power`, `:647` allocations, `:1050` install | `manager.go:494` install check | — (optional `215`) | power/alloc disabled states |
| GH-01/02/06 generation fence | `store/store_state.go:10` generations, `store/store.go:495` `Generation` doc, `store/store_evacuation.go:33` lease, `store/store_servers_control.go:83` `ServerProvisionTarget`, `services/clustermanager/service.go:632` runtime wire, `internal/daemon/client.go:693` headers | `manager.go:30` `ServerState`, `manager.go:271` split slot, `manager.go:689` sync, `server.go:1052` `install`/`reinstall` handlers | `110_node_fencing.sql:2` (already), `092_durable_operations.sql:7`, `216_generation_fence_beacon.sql` | `lib/api.ts` generation tooltip, evacuation UI |
| Orphan remediation | `store/store_servers_lifecycle.go:18` `RecordOrphanAndHardDeleteServer`, `services/clustermanager/service.go:285` `compensateCreateFailure`, `api/docs/server-lifecycle.md:1` | `server.go:1052` install failure path | `216` adds `remediation_type` | `admin/orphans` view |
| Suspend fencing | `store/store_servers.go:374` `SetServerSuspension`, `http/handlers_servers.go:1413` suspension | `manager.go:613` `onBeforeStart` suspended check | — | suspend toggle |

---

## 10. Backward-compatibility checklist (must be met by every PR)

- [ ] No `DROP COLUMN`, no `ALTER TYPE ... RENAME`, no `DROP INDEX` that the old binary still uses.
- [ ] All `ALTER TABLE ... ADD COLUMN` uses `IF NOT EXISTS` with `DEFAULT` so old rows remain readable and new rows are writable by old code (which ignores the column).
- [ ] All new enum values added with `ADD VALUE IF NOT EXISTS` — old code that does `SELECT ...::text` still works; old code that does `WHERE status='restoring_backup'` simply finds nothing.
- [ ] New DB reads are **dual-read**: API checks both `servers.actual_state` and `backups.status` for restoring; beacon treats `generation==0` and missing `RestoringBackup` as fail-open.
- [ ] New API writes are **dual-write** for one release (backup status + server actual_state), not cut-over.
- [ ] Beacon headers are additive (`X-Forge-Generation` ignored by old beacon).
- [ ] Every new fencing error uses `409 Conflict` with a distinct `reason` string so old web (which renders `err.response.data`) shows a useful message even before the badge lands.

---

## 11. Open questions for maintainers (not blockers)

1. Should `kill` during `installing` cancel the install (`EndInstall(failed:true)`) or be rejected? Recommendation: **cancel** — an operator who kills during install has decided the install is wedged; leaving it `installing` forever blocks later retries. GH-05 pierce + `EndInstall` achieves this.
2. Should `restoring_backup` be a `server_actual_state` enum value or also a `desired_state`? Recommendation: **actual only** — desired remains `stopped` during restore (like `installing`), so reconcile does not try to start the server.
3. RE2 lacks PCRE lookahead — do we vendor a PCRE engine for egg regex, or document RE2 subset? Recommendation: document + add a `regex_engine` column later if PCRE is truly needed (additive).
4. Lease window: is `1h` (evacuation) + `24h` (crash restart) correct? Recommendation: unify to `1h` as in §6.2 C.

---

## 12. References

- FINAL_PARITY_AUDIT GH-01..GH-19 `audits/FINAL_PARITY_AUDIT.md:40-64`
- MASTER_FINDING_INDEX REF-GAME-* `audits/MASTER_FINDING_INDEX.md:44-58`
- `reference/wings` — `wings/power.go:24` (powerLock), `:57` (IsInstalling), `:108` (kill pierce), `wings/install.go:33` (install_container), `:80` (WaitForStop 10s)
- `reference/pterodactyl-panel` — `Server.php:125` (suspended), `:138` (installing default), `:393` (restoring lock), `EggVariable.php:66` (regex delimiters)
- `audits/final-parity/*` + `audits/reverification/*` (10 parallel reverifiers post-Phase-6)
