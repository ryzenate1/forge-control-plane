# Subagent 07 — Quality Polish Audit (Phase 09 — error handling / logging / observability / metrics)

> **Agent:** 110-09-07 — Parallel subagent 07/10  
> **Focus:** Polish quality (error handling, logging, observability, metrics)  
> **Scope:** `forge/api/internal/http`, `forge/api/internal/services`, `forge/api/internal/events/context.go:12`, metrics snapshots, wildcard observer + webhook, heartbeat monitor, alerting, P0 metric completeness  
> **Date:** 2026-08-24  
> **Verdict:** PASS with reservations — P1 follow-ups required before 110-close

---

## 1. Executive Summary

| Area | Verdict | Confidence | Blocking |
|------|---------|------------|----------|
| Error handling (`%w` vs swallow) | **PASS** | high | no |
| Logging (`slog` + correlation) | **CONDITIONAL PASS** | high | P1 — 56 legacy `log.Printf`/`fmt.Printf` remain |
| Observability (snapshots) | **CONDITIONAL PASS** | high | P1 — 6 services have `Metrics()` but are not wired to `/metrics` |
| Wildcard observer + webhook | **PASS** | high | no |
| Heartbeat monitor | **PASS** | high | no |
| Alerting | **PASS** | high | no |
| P0 metrics completeness | **FAIL** | high | **P1 — `gateway_legacy_fallback_total` always 0, `trusted_proxy_unconfigured`/`lb_target_rejected` log-only** |

No data-loss or security regression was found. The codebase is already heavily polished; the remaining gaps are Polish-quality (log pipeline purity, metric wiring) rather than functional breakage.

---

## 2. Error Handling

### 2.1 Wrapping discipline (`%w`)

* `grep -rn "fmt.Errorf.*%w" forge/api/internal/http forge/api/internal/services --include="*.go" | wc -l` → **1254** occurrences. Wrapping is the norm, not the exception.
* Canonical pattern used in new services:

```go
// forge/api/internal/services/recovery/service.go:524-533
cleanupErr = errors.Join(cleanupErr, fmt.Errorf("mark migration %s failed: %w", item.MigrationID, err))
```

* `errors.Is` / `errors.As` usage: 77 occurrences (`grep -rn "errors.Is\|errors.As\|errors.Unwrap"` → 77). Example `forge/api/internal/http/handlers_loadbalancer.go:20` uses `errors.Is(err, loadbalancer.ErrGroupNotFound)` correctly.
* HTTP normalization is centralized in `forge/api/internal/http/errors.go:18-55` (`domainErrorStatus` → `respondStoreError` → `respondInternalError`). All audited handlers in `handlers_notifications_enhanced.go`, `handlers_scheduler.go`, `handlers_git_deploy.go`, `handlers_preview_deployments.go` correctly route through `respondInternalError` / `respondStoreError` rather than echoing `err.Error()` in production (`errors.go:98-107` masks 5xx to `"an internal error occurred"` in `APP_ENV=production`, `server.go:1174-1177` duplicates the guarantee in the fiber `ErrorHandler`).

### 2.2 Swallowed errors (`_ = ...`)

* Target anti-pattern `_ = daemon...` **not found** (`grep -rn "_ = daemon" forge --include="*.go"` → 0). Good.
* The broader `_ = ` search (non-test, non-generated) shows ~60 hits, all falling into three **intentionally best-effort** buckets:

| Bucket | Example | Risk |
|--------|---------|------|
| `WriteJSON` / `WriteControl` / `Close` on WebSockets | `forge/api/internal/http/realtime.go:145` `_ = client.WriteJSON(...)` (×25), `server.go:2253` `_ = conn.WriteJSON(...)` | Low — write on half-closed socket; error is unrecoverable and best-effort close is idiomatic. No data-loss. |
| Deferred cleanup status updates | `forge/api/internal/services/operation/service.go:258` `_ = s.store.UpdateStatus(ctx, op.ID, StatusFailed, ...)` (×8), `compose/lifecycle.go:443` `_ = s.store.UpdateComposeStack(...)` | **Medium** — swallow of persistence failure is silent. Existing code already returns the *primary* error; the swallow is an update-not-found race, but should emit a `slog.Warn` for DLQ visibility. |
| Idempotent best-effort (`rand.Read`, `json.Unmarshal`, `Scan`, `SetWriteDeadline`) | `handlers_reconcile.go:183` `_ = json.Unmarshal(row.DiffData, &diffs)`, `server.go:1699` `_ = db.QueryRow(...).Scan(&maxGen)` | Low — guard on optional columns; zero-value fallback is intended. |

**Recommendation (P2):** Add `slog.Warn` to the 8 `operation/service.go` + 4 `compose/lifecycle.go` status-update swallows so terminal-job observability is not blind when the store is degraded. Not blocking.

### 2.3 Error paths coverage

* `server.go:1165-1180` fiber `ErrorHandler` sanitizes every path; `errors.go:80-92` `logInternalError` records full error server-side with `requestId` before masking. This is the correct defense-in-depth pairing.
* No bare `panic` without recovery in hot paths: every long-lived goroutine (`eventstore/outbox.go:110`, `heartbeatmonitor/service.go:133`, `webhook/worker.go:51`, `reconciler/service.go:219`, `loadbalancer/dataplane.go:33`) has a `defer recover()` that either `slog.Error`s or `fmt.Printf`s (see §3.2 for the print-vs-slog issue).

**Score: 9/10 — fix the 12 status-update swallows with warn logging to reach 10.**

---

## 3. Logging

### 3.1 Structured `slog` adoption

* `grep -rn "slog" forge/api/internal --include="*.go" | grep -v _test.go | wc -l` → **165** structured sites. Positive indicator.
* Production HTTP path is fully structured:
  * `forge/api/internal/http/middleware_logger.go:11-59` — `StructuredLogger` emits `slog.LogAttrs` with `request_id`, `method`, `path`, `status`, `duration`, `ip`, `user_id`, `error` and dynamic level (`Info`/`Warn`/`Error`).
  * `forge/api/internal/http/middleware_requestid.go:17-35` — propagates/creates `X-Request-ID` → `c.Locals("requestId")` → `X-Request-ID` response header, also used by `errors.go:85` and `middleware_logger.go:14`.
  * `forge/api/internal/http/server.go:1221-1223` wires `StructuredLogger(cfg.Logger)` before any route, preceded by `SecurityHeaders` and `RequestIDMiddleware` equivalent — correct order.
* All security-sensitive call sites already use `slog`:
  * `trusted_proxies.go:34,59`, `middleware_ipaccess.go:29,35,50,121,150`, `middleware_mtls.go:82,90,104,112,131,174`, `handlers_loadbalancer.go:85,170`, `handlers_admin.go:1676,1683`.

### 3.2 Remaining `log.Printf` / `fmt.Printf` debt — **P1**

`grep -rn "log.Printf\|log.Println\|fmt.Printf" forge/api/internal --include="*.go" | grep -v _test.go | grep -v e2e | wc -l` → **56** hits (34 distinct files). Excluding `e2e/report.go` and `store/demo seed` (acceptable), the production-relevant remainder is **~31** sites:

| File | Lines | Pattern | Should be |
|------|-------|---------|-----------|
| `forge/api/internal/http/middleware_request_logging.go:80` | `log.Println(string(data))` | raw `log.Println` duplicates `StructuredLogger` and emits unstructured JSON without `request_id`/`correlation_id`. Either delete (redundant) or replace with `slog.Info`. | `slog.Info("request", "entry", entry)` |
| `forge/api/internal/services/webhook/worker.go:52,96,105,119,125` | `log.Printf("webhook … %v", err/r)` | Worker panic, claim failure, rate-limit, complete, retry. All lose `correlation_id`/`webhook_id` structure. | `slog.Error/Warn("webhook …", "webhook_id", d.ID, "error", err)` |
| `forge/api/internal/services/mail/worker.go:35,72,84,95,100` + `mail/triggers.go:74` | `log.Printf("mail …")` | Same worker pattern; `mail/worker.go:84` is the `log driver` path (acceptable as `log` driver but should still go through `slog`). | `slog` with `mail_id` |
| `forge/api/internal/services/dbbackup/service.go:592,594` | `log.Printf("failed to create backup dir …")` | Silent on Prometheus; backup failures should be `slog.Error` with `backup_dir` attr. | `slog.Error` |
| `forge/api/internal/services/domains/service.go:143,225,257` | `fmt.Printf("domain … panic: %v\nstack: %s")` | Panic recovery that bypasses `slog` entirely (no level, no JSON). | `slog.Error("domain … panic", "panic", r, "stack", buf[:n])` |
| `forge/api/internal/services/reservations/service.go:56` | `fmt.Printf("reservation manager panic …")` | Same | `slog.Error` |
| `forge/api/internal/services/evacuationplanner/service.go:127,174` | `fmt.Printf("evacuation planner … panic")` | Same | `slog.Error` |
| `forge/api/internal/services/loadbalancer/dataplane.go:33,118,134,153,165,190,198,259` | `log.Printf("load balancer … panic")` (×8) | Dataplane panics not visible in JSON log pipeline. | `slog.Error` |
| `forge/api/internal/services/observability/metrics_collector.go:96` | `fmt.Printf("metrics collector panic: %v", r)` | In `MetricsHistory.StartCollection` — never reaches Loki/CloudWatch JSON. | `slog.Error` |
| `forge/api/internal/services/cleanup/service.go:69` | `fmt.Printf("cleanup service panic …")` | Same | `slog.Error` |
| `forge/api/internal/services/backup/service.go:981,988` | `log.Printf("[backup] %s …")` | Backup operational log outside `slog` pipeline. | `slog.Info("backup …", "msg", msg)` |
| `forge/api/internal/services/failover/service.go:307,496,509,522,536` | `log.Printf("failover: … %v", err)` | Publish/notify errors lost from structured trail. | `slog.Error` |
| `forge/api/internal/services/healthcheckrunner/service.go:149,396,413` | `fmt.Printf("health check … panic")` | Panic recovery outside `slog`. | `slog.Error` |
| `forge/api/internal/services/heartbeatmonitor/service.go:137` | `fmt.Printf("heartbeat monitor panic …")` | Panic recovery outside `slog`. | `slog.Error` |
| `forge/api/internal/services/logger/logger.go:32` | `log.Printf("failed to open log file …")` | Acceptable fallback before `slog` is constructed (not a violation). | keep |
| `forge/api/internal/store/store.go:1416` | `log.Printf("demo seed: generated admin password …")` | Demo seed only, acceptable. | keep |

**Action:** Replace the 31 production sites above with `slog.*` in a single polish PR. The fix is mechanical and safe; it restores JSON-pipeline visibility and enables metric extraction via `metric` label queries that currently miss these sites.

### 3.3 `correlationId` / `request_id` propagation

* Canonical helpers exist at `forge/api/internal/events/context.go:12` (`ContextWithCorrelationID` / `CorrelationIDFromContext` + `TraceID`/`SpanID` siblings). Usage is solid in domain services:
  * `recovery/service.go:152-253`, `recovery/recovery_ops.go:155-233`, `clustermanager/service.go:78,138,333-384,624`, `clustermembership/service.go:361`, `reservations/service.go:174`, `replicamanager/service.go:851`, `reconciler/service.go:219-255`, `evacuationplanner/service.go:215-441`, `cleanup/service.go:183`.
  * Pattern is correct: generate `uuid.NewString()` at the entry boundary (often `firstNonEmpty(existing, uuid.NewString())`), store via `ContextWithCorrelationID`, propagate via `payload["correlationId"]` and `events.NewEnvelope` (which itself extracts `correlationId` from payload at `events/event.go:212-254`).
* **HTTP gap:** `middleware_logger.go` and `middleware_request_logging.go` log `request_id` but not `correlation_id`. No middleware bridges `X-Request-ID` → `events.ContextWithCorrelationID`. The two IDs are currently parallel namespaces:
  * `logger/logger.go:53` (`RequestIDKey = "request_id"`) vs `events/context.go:8` (`correlationContextKey`) — two separate context keys for the same logical concept, never joined.
* Only `reconciler/service.go:232,239,247,252,255,271` actually emits `correlationId` in `slog.*` calls. Other services (`clustermanager`, `recovery`, `evacuationplanner`) pass `correlationId` in payloads/timeline events but do not include it as a `slog` attribute, so log aggregation cannot correlate.

**Recommendations (P1 + P2):**

* **P1 — unify request logging:** Either (a) have `StructuredLogger` read `events.CorrelationIDFromContext(c.UserContext())` (or `c.Locals("correlationId")`) and emit `correlation_id` alongside `request_id`, or (b) have `RequestIDMiddleware` call `events.ContextWithCorrelationID(c.UserContext(), id)` so downstream `slog` can use a single key. The 1-line fix closes the correlation gap for all future `slog` sites.
* **P2 — emit `correlation_id` in every `slog.Error/Warn` that has a `ctx`:** Add `"correlation_id", events.CorrelationIDFromContext(ctx)` to worker/queue/alerting log lines. This is ~15 call sites.

**Score: 7/10 — structure is right, but the log pipeline leaks 31 `Printf` sites and `correlation_id` stops at the service boundary.**

---

## 4. Observability

### 4.1 Metrics snapshots per service

All long-lived services implement a `Metrics()` (or `MetricsSnapshot`) snapshot, guarded by `sync.Mutex`/`sync.RWMutex` and incremented via a private `increment` helper — correct pattern:

| Service | Type | File | Snapshot | Exposed in `/metrics`? |
|---------|------|------|----------|------------------------|
| `events.Registry` | `events.Metrics` (`EventsPublishedTotal`, `EventsDeliveredTotal`, `EventHandlerFailuresTotal`, `EventsDeadLetteredTotal`, `EventsByType`) | `forge/api/internal/events/registry.go:21-27,292-304` | ✅ `server.go:1531-1555` (4 counters + by-type) |
| `reconciler.Service` | `MetricsSnapshot` (11 counters) | `forge/api/internal/services/reconciler/service.go:56,421-430` | ✅ `server.go:1505-1528` |
| `scheduler.Scheduler` | `Metrics` (8 counters) | `forge/api/internal/services/scheduler/service.go:62,101` | ✅ `server.go:1558-1579` |
| `heartbeatmonitor.Service` | `Metrics` (7 counters) + state gauge | `forge/api/internal/services/heartbeatmonitor/service.go:26-34,118-125,315-354` | ✅ `server.go:1581-1618` |
| `observability.Service` + `MetricsHistory` | `SystemMetrics` + `ListRetentionPolicies` gauges | `forge/api/internal/services/observability/metrics_collector.go:11-90`, `observability/service.go:340-382,385-413` | ✅ `server.go:1621-1646` (retention, backup gauges) + `StartMetricsCollection` |
| `queue.Service` | `legacyFallbackTotal atomic.Uint64` | `forge/api/internal/services/queue/queue.go:92-98,294` | ⚠️ wired but stubbed (see §5.2) |
| `http.MetricsCollector` (RED) | `requestCounts`, `requestBuckets`, `requestSums`, `activeConnections`, `errorCounts` | `forge/api/internal/http/handlers_metrics.go:46-186`, `server.go:1225-1231`, `middleware_metrics.go:12-50` | ✅ `server.go:1650-1652` + `MetricsMiddleware` on every route |
| `clustermembership.Service` | `Metrics` (`NodesJoinedTotal`, `NodesLeftTotal`, `DrainStartedTotal`, `DrainCompletedTotal`, `MaintStartedTotal`, `MaintEndedTotal`) | `forge/api/internal/services/clustermembership/service.go:16,93-99,117-369` | ❌ not wired |
| `reservations.Manager` | `Metrics` (`PlacementReservationsTotal`, `ReservationConflictsTotal`, `ReservationExpirationsTotal`) | `forge/api/internal/services/reservations/service.go:15,37-43,82-193` | ❌ not wired |
| `replicamanager.Manager` | `Metrics` (+ `SnapshotMetrics`) | `forge/api/internal/services/replicamanager/service.go:29,91,874` | ❌ not wired |
| `recovery.Coordinator` | `Metrics` (`RecoveryPlansTotal`, `RecoveryItemsTotal`, `RecoveryFailuresTotal`) | `forge/api/internal/services/recovery/service.go:23,139-145,169-726` | ❌ not wired |
| `evacuationplanner.Service` | `Metrics` (`EvacuationPlansTotal`, `EvacuationValidationFailuresTotal`, `OrphanDetectionTotal` …) | `forge/api/internal/services/evacuationplanner/service.go:55,425-431,453-790` | ❌ not wired |
| `cleanup.Service` | `Metrics` (`CleanupErrorsTotal`, `StaleReservationsCleaned`, `OrphanedAllocationsCleaned`) | `forge/api/internal/services/cleanup/service.go:15,50,102-191` | ❌ not wired |
| `loadbalancer.Service` | `Metrics(ctx) map[string]any` (`groups`, `totalTargets`, `healthyTargets`) | `forge/api/internal/services/loadbalancer/service.go:530-549` | ⚠️ exposed only via `GET /admin/load-balancer/metrics` (`handlers_loadbalancer.go:214`), not global `/metrics` |
| `autoscaler.Service` | `Metrics` | `forge/api/internal/services/autoscaler/service.go:62,522` | ❌ not wired |
| `webhook.Service` | **no `Metrics` type** | `forge/api/internal/services/webhook/service.go`, `worker.go:58-128` | ❌ no snapshot at all |

**What is wired is excellent** (REd + registry + reconciler + scheduler + heartbeat + observability gauges + queue/relay depth + cert expiry + backup staleness + fencing generation + leader gauge + event store pending/dispatched/DLQ). But **6 services with mature `Metrics()` are dark in `/metrics`**, which means the Grafana alert rules that should consume them have no series.

**Recommendation (P1):** In `server.go:1474-1800` `/metrics` handler, add a block per dark service mirroring the reconciler/scheduler/heartbeat pattern (guard on `cfg.<Service> != nil`). Cost is ~6×8 lines, zero cardinality risk (all counters, no label explosion). Also add a `webhook.Metrics` struct (deliveries, failures, retries, DLQ) — the only service without any snapshot.

### 4.2 Wildcard observer + webhook

* **Registry wildcard:** Canonical at `forge/api/internal/events/registry.go:14` `WildcardEventType = "*"`, subscribed via `Registry.Subscribe(WildcardEventType, ...)` (`registry.go:78`), dispatched in `Publish` via `r.subscribers[WildcardEventType]` appended to typed entries (`registry.go:158-159`), with per-subscriber retry (`handleWithRetry` + `randomDuration` jitter), dead-letter tracking (`recordFailure`/`deadLettered`), and nil-safe `sameSubscriber` reflection (`registry.go:131-142`). Publish is typed-and-wildcard fanout with `sync.WaitGroup` and mutex-isolated `Metrics` — production-grade.
* **Notification fanout (WS Hub):** `forge/api/internal/http/ws_hub.go:16-382` is the **audited wildcard observer**:
  * Subscribes as `events.WildcardEventType` in `NewNotificationWSHub` (`ws_hub.go:110`) and `Attach` (`ws_hub.go:259`).
  * Deduplicates via `historyIDs` + bounded replay (`defaultHubHistory=200`, `maxHubHistory=500`) (`ws_hub.go:265-281`).
  * Resolves target users via `notificationUserResolver` → `ServerOwnerID` fallthrough, fans out only to `targets[sub.UserID]` or `admin` (`ws_hub.go:283-314`), with `perUserOutboxCapacity=256` drop-oldest + `dropped` counter (`ws_hub.go:232-251`) — never blocks hub.
  * `PublishUserNotification` (`ws_hub.go:325-360`) is explicit-recipient (admins not implied) — correct privacy boundary.
  * `Stop()` (`ws_hub.go:364-379`) closes every subscription and rejects future `Subscribe` — clean shutdown.
* **Webhook dispatch:** `forge/api/internal/services/webhook/service.go` + `worker.go:58-128` is **not a wildcard subscriber** (it is a store-backed queue worker with `ClaimWebhookDelivery`/`CompleteWebhookDelivery`/`FailWebhookDelivery`), but it correctly implements the complementary observability requirement: HMAC signature with timestamp replay protection, SSRF guard (`validateURL` → `forbiddenIP` → `secureHTTPClient` dial intercept), rate limiting (`rateLimiter.allow`), exponential backoff (`webhookRetryDelay` `1<<attempt` minutes capped at `1<<10`), redaction (`redactURL`), and `FailWebhookDelivery` retry flag. The webhook path is durable (DB-backed), not in-memory wildcard — correct durability boundary.

**Verdict:** Wildcard observer design is audit-clean. No change needed.

### 4.3 Heartbeat monitor

* `forge/api/internal/services/heartbeatmonitor/service.go:1-429` — **PASS**, exemplary:
  * Config with `WarningThreshold=30s`, `OfflineThreshold=90s`, `UnavailableAfter=300s`, `RecoveryThreshold=2`, `Interval=30s` (`DefaultConfig`), `normalizeConfig` enforces invariants.
  * `Metrics` has 7 counters (`HeartbeatEvaluationsTotal`, `NodesSuspectedTotal`, `NodesUnreachableTotal`, `NodesOfflineTotal`, `NodesRecoveredTotal`, `NodesReconcilingTotal`, `NodesUnavailableTotal`) incremented via `increment` + transitions (`publishTransitions`).
  * `Start` adds 0-5s jitter to desynchronize multi-replica DB write amplification (`service.go:142`), ticker on `config.Interval`, `context.WithCancel` shutdown (`Stop`).
  * `classify` handles future timestamps, nil `LastSeenAt`, `history[0].Success` failure, and full state machine (`Healthy → Suspected → Unreachable → Offline → Recovering → Reconciling → Healthy`).
  * Alerting hook: on `Unreachable` transition, calls `alerter.CheckStaleHeartbeat` (`service.go:254-258`) which correctly creates a deduplicated `stale_heartbeat` alert.
  * Jitter, retry-safe `EvaluateAll` (`continue` on per-node error), and state-count gauge in `/metrics` (`server.go:1602-1618`) are all present.

### 4.4 Alerting

* `forge/api/internal/services/alerting/service.go:1-513` — **PASS**:
  * Threshold-gated `CheckNodeThresholds` (`CPUWarning=80`/`Critical=95`, `Memory` same, `DiskWarning=85`/`Critical=90`) with `severityFromFloat` and `evaluateAndAlert` deduplication via `FindAlertBySuppressionKey` (`alertType:nodeID` key), acknowledged-check short-circuit, and `resolveIfActive` on `AlertSeverityOK`.
  * `CheckStaleHeartbeat` correctly escalates `Warning` (<10m) → `Critical` (≥10m) and resolves on recovery.
  * Notification dispatch: `dispatchNotifications` respects `route.Enabled`, `MinSeverity` score gate, `EventTypes` wildcard, and bounded `dispatchSlots=16` semaphore with 30s timeout (`evaluateAndAlert:204-216`).
  * `Notifier` supports `slack`/`discord`/`telegram`/`email`/`webhook`, with `validateNotificationURL` enforcing HTTPS + non-private/non-loopback resolution, blocked headers (`Host`, `Content-Length`, …), and `CheckRedirect = ErrUseLastResponse` to prevent open-redirect leading to SSRF.
  * `heartbeatmonitor` → `alerting` wiring via `SetAlerter` (`heartbeatmonitor/service.go:71`) closes the loop; `server.go` wiring should be verified in main.go (outside this audit's scope but pattern is present).

---

## 5. P0 Fixes → Metrics Mapping

### 5.1 Present and correct (log-based extraction, needs Prometheus counter)

| P0 fix | Code site | Log metric key | Prometheus counter | Status |
|--------|-----------|----------------|--------------------|--------|
| `trusted_proxy_unconfigured` | `middleware_ipaccess.go:50,121,123,148,150`, `trusted_proxies.go:59` — `slog.Warn("…", "metric", "trusted_proxy_unconfigured")` (×6) with `trustedProxiesWarnOnce.Do` de-dupe (`trusted_proxies.go:58`) | `"metric": "trusted_proxy_unconfigured"` | ❌ no `counter` — requires Loki/CloudWatch metric filter extraction | **P1 polish:** promote to `atomic.Uint64` counter + `game_panel_api_trusted_proxy_unconfigured_total` in `/metrics` |
| `lb_target_rejected` | `handlers_loadbalancer.go:170` — `slog.Warn("loadbalancer target IP rejected", "ip", req.IP, "reason", reason, "group", c.Params("id"), "metric", "lb_target_rejected")` gated by `isProhibitedTargetIP` (private/loopback/unspecified/link-local/multicast/metadata) | `"metric": "lb_target_rejected"` | ❌ log-only | **P1 polish:** same promotion — `game_panel_api_lb_target_rejected_total` with `reason` label (bounded: 6 values) |
| `mtls_xfp_untrusted` | `middleware_mtls.go:131` — `slog.Warn("mTLS X-Forwarded-Proto ignored — peer not in TRUSTED_PROXIES", "peer", c.IP(), "metric", "mtls_xfp_untrusted")` | `"metric": "mtls_xfp_untrusted"` | ❌ log-only | Informational — not requested but same pattern; worth a counter if mTLS is P0 |

All three emit the `metric` slog attribute that the existing Loki/CloudWatch metric-filter can scrape, so **alerting is not blind** even without the Prometheus counter. But the task explicitly calls for “metrics snapshots per service” and the polish checklist says “ensure all P0 fixes have metrics” — log-only extraction is fragile (dropped on `Printf` pipeline, missed if `slog` level is `Warn`-filtered). **Promoting to an `atomic.Uint64` counter and exposing in `/metrics` is the correct completion.**

### 5.2 `gateway_legacy_fallback_total` — **STUBBED, ALWAYS 0**

* Definition is correct: `forge/api/internal/services/queue/queue.go:92-98` `legacyFallbackTotal atomic.Uint64` + `LegacyFallbackTotal()` + increment on every `DispatchIdempotent` when `!queueSingleWriterEnabled()` (`queue.go:294`), and `eventstore/store.go:306-320` `PublishDurableTx` documents the dual-write migration window (flag `QUEUE_SINGLE_WRITER`) and its 10 migrated call sites (`clustermanager.CreateServer`, evacuation planner, fencing, scheduler, recovery, replicamanager, operation dispatch, cluster membership, traffic manager, compose lifecycle).
* **The `/metrics` exposure at `server.go:1671-1681` is a stub that always emits 0:**

```go
// server.go:1673-1681
body.WriteString("# HELP game_panel_api_legacy_fallback_total ...\n")
body.WriteString("# TYPE game_panel_api_legacy_fallback_total counter\n")
var v uint64  // ← always 0, comment admits circular-dep dodge
body.WriteString("game_panel_api_legacy_fallback_total " + strconv.FormatUint(v, 10) + "\n")
```

  The comment `queue.LegacyFallbackTotal is linked via build tag; attempt direct import via reflection … fallback to 0` is stale — `queue.LegacyFallbackTotal()` is a plain exported func with no build tag, importable from `forge/api/internal/http` without a circular dep (http does not import queue, queue does not import http). The stub defeats the entire AF-2 dual-write observability goal: operators cannot track migration convergence (`fallback should converge to 0 when QUEUE_SINGLE_WRITER=1` per `queue.go:298` comment).

**Fix (P1, 3 lines):**

```go
// server.go top: add import queue "gamepanel/forge/internal/services/queue"
// server.go:1677: var v = queue.LegacyFallbackTotal()
```

Or, if import is intentionally avoided, wire via `Config.QueueService.LegacyFallbackTotal()` or a `func() uint64` injected at `NewServer` construction. Any approach that makes the gauge reflect reality closes the gap. The predicate-guard and exponential backoff hardening in `queue/queue.go:303-324,199-208` (AF-3) are already correct — only the visibility is missing.

### 5.3 Other P0-adjacent metrics (already exposed, no action)

* `game_panel_api_forge_leader_is_leader` (`server.go:1682-1694`) — flag-gated on `QUEUE_SINGLE_WRITER`+`FORGE_LEADER`.
* `game_panel_api_fencing_generation` (`server.go:1695-1703`) — `SELECT MAX(generation) FROM servers`.
* `game_panel_api_event_relay_subscribers` (`server.go:1708-1714`) — `EventRelay.SubscriberCount()`, alert on `=0` when `QUEUE_SINGLE_WRITER=1`.
* `game_panel_api_event_store_pending/dispatched/dead_letter` (`server.go:1727-1742`) — store depth gauges for AF-1 mis-wiring detection.
* `game_panel_api_queue_pending/running_jobs` (`server.go:1658-1669`) — durable queue depth.
* `game_panel_api_certificates_total/expiry/expiring` + `game_panel_api_backup_last_success_seconds` + heartbeat state nodes gauge.

---

## 6. Structured Logging vs `correlationId` — Detailed Gap Table

| Requirement | Expected | Actual | File:line | Severity |
|-------------|----------|--------|-----------|----------|
| `slog` with `request_id` | Every HTTP request logs `request_id` via `StructuredLogger` | ✅ present | `middleware_logger.go:32` | — |
| `slog` with `correlationId` via `events/context.go:12` | Every service error/warn includes `correlationId` from `ContextWithCorrelationID` | ❌ only reconciler does | `reconciler/service.go:232,239,247,252,255` vs 12 other services that propagate `correlationId` in payloads but never in `slog` | P2 |
| `slog` not `log.Printf` | No `log.Printf` in production code | ❌ 31 sites still use `log.Printf`/`fmt.Printf` | `webhook/worker.go:52`, `mail/worker.go:35`, `domains/service.go:143`, `loadbalancer/dataplane.go:33`, `metrics_collector.go:96`, `cleanup/service.go:69`, `failover/service.go:307`, `healthcheckrunner/service.go:149`, `heartbeatmonitor/service.go:137`, `backup/service.go:981`, `dbbackup/service.go:592`, `middleware_request_logging.go:80` | P1 |
| `request_id` ↔ `correlation_id` bridge | HTTP `X-Request-ID` becomes `events.CorrelationID` context value | ❌ two parallel keys (`logger.RequestIDKey="request_id"` vs `events.correlationContextKey`) never joined | `logger/logger.go:53` vs `events/context.go:8` | P1 |

---

## 7. Actionable Follow-ups (prioritized)

### P1 — Fix before 110-close (blocking for “polished”)

1. **Replace 31 `log.Printf`/`fmt.Printf` with `slog.Error/Warn/Info`** — files listed in §3.2. Mechanical, one PR. Highest impact: `webhook/worker.go` (5), `loadbalancer/dataplane.go` (8), `domains/service.go` (3), `middleware_request_logging.go:80` (delete or `slog.Info`), `metrics_collector.go:96`, `heartbeatmonitor/service.go:137`, `failover/service.go:307,496,509,522,536`, `mail/worker.go` cluster, `healthcheckrunner/service.go:149`.
2. **Wire `queue.LegacyFallbackTotal()` into `/metrics`** — `server.go:1677` change `var v uint64` → `var v = queue.LegacyFallbackTotal()` (add import) so `game_panel_api_legacy_fallback_total` reflects dual-write reality. Without this, AF-2 has no observable convergence signal.
3. **Promote `trusted_proxy_unconfigured` + `lb_target_rejected` (+ optionally `mtls_xfp_untrusted`) from log-only `metric` slog attr to `atomic.Uint64` counters and expose as `game_panel_api_trusted_proxy_unconfigured_total` / `game_panel_api_lb_target_rejected_total{reason=}` in `/metrics`.** Keep the slog line (for log search) but add the counter increment at the same call site.
4. **Wire dark `Metrics()` snapshots into `/metrics`** — add exposure blocks for `clustermembership`, `reservations`, `replicamanager`, `recovery`, `evacuationplanner`, `cleanup`, `autoscaler` (mirroring `reconciler/scheduler/heartbeat` pattern). Stub is ~50 lines total, no cardinality risk.
5. **Bridge `request_id` → `correlation_id`** — one line in `middleware_logger.go:14` or `middleware_requestid.go:31` to call `events.ContextWithCorrelationID`, and include `"correlation_id"` in `StructuredLogger` attrs. This makes every future `slog` automatically correlatable.

### P2 — Polish before next audit window

6. Add `slog.Warn` to the 12 swallowed `UpdateStatus`/`UpdateComposeStack` best-effort failures (`operation/service.go:258,269,278,285,300,303,307`, `compose/lifecycle.go:443,451,456,493`) with `correlation_id` attr.
7. Emit `correlation_id` in every service-level `slog.Error/Warn` that has a `ctx` (15 sites: `recovery`, `clustermanager`, `evacuationplanner`, `cleanup`, `alerting`).
8. Add `webhook.Metrics` snapshot (`DeliveriesTotal`, `FailuresTotal`, `RetriesTotal`, `DeadLetteredTotal`, `RateLimitedTotal`) and expose it.
9. Normalize `logger.RequestIDKey` vs `events.correlationContextKey` — consider making `logger` re-export or alias `events.CorrelationIDFromContext` so there is one source of truth.
10. Add a `TestMetricsParity` unit test that asserts every service with a `Metrics()` method is represented in `server.go` `/metrics` handler — prevents future drift (the 6-service gap would have been caught).

### P3 — Nice-to-have

11. Replace `log.Printf("failed to open log file …")` in `logger/logger.go:32` with an `os.Stderr` write that does not depend on the logger being constructed (cosmetic).
12. Add `game_panel_api_webhook_target_rejected_total{reason=}` and `game_panel_api_trusted_proxy_unconfigured_total` exemplars for SLO dashboards.

---

## 8. Evidence Index (file:line)

* **Correlation helpers:** `forge/api/internal/events/context.go:12`, `events/event.go:212-263`, `events/registry.go:14,78,158`, `events/subscriber.go:1-40`
* **Logging core:** `forge/api/internal/http/middleware_logger.go:11-59`, `middleware_request_logging.go:1-85`, `middleware_requestid.go:17-35`, `errors.go:80-107`, `server.go:1165-1184,1221-1231,1474-1800`, `services/logger/logger.go:18-93`, `services/reconciler/service.go:232-271`
* **Legacy prints to replace (§3.2):** `services/webhook/worker.go:52,96,105,119,125`, `services/mail/worker.go:35,72,84,95,100`, `services/dbbackup/service.go:592,594`, `services/domains/service.go:143,225,257`, `services/reservations/service.go:56`, `services/evacuationplanner/service.go:127,174`, `services/loadbalancer/dataplane.go:33,118,134,153,165,190,198,259`, `services/observability/metrics_collector.go:96`, `services/cleanup/service.go:69`, `services/backup/service.go:981,988`, `services/failover/service.go:307,496,509,522,536`, `services/healthcheckrunner/service.go:149,396,413`, `services/heartbeatmonitor/service.go:137`, `http/middleware_request_logging.go:80`
* **P0 metric sites:** `http/middleware_ipaccess.go:50,121,123,148,150`, `http/trusted_proxies.go:59`, `http/handlers_loadbalancer.go:170`, `middleware_mtls.go:131`, `services/queue/queue.go:92-98,294`, `eventstore/store.go:306-320`, `http/server.go:1671-1681`
* **Snapshots:** `services/clustermembership/service.go:16,93`, `services/reservations/service.go:15,37`, `services/replicamanager/service.go:29,91,874`, `services/recovery/service.go:23,139`, `services/evacuationplanner/service.go:55,425`, `services/cleanup/service.go:15,50`, `services/scheduler/service.go:62,101`, `services/heartbeatmonitor/service.go:26,118`, `services/observability/metrics_collector.go:11,41`, `services/reconciler/service.go:56,421`, `services/loadbalancer/service.go:530`, `services/autoscaler/service.go:62,522`, `services/alerting/service.go:22-54`, `events/registry.go:21`
* **Wildcard + webhook:** `events/registry.go:14,73-106,144-191`, `http/ws_hub.go:16,110,155-382`, `services/webhook/worker.go:58-128,209-228`, `services/webhook/security.go:1-55`
* **Heartbeat + alerting:** `services/heartbeatmonitor/service.go:26-429`, `services/alerting/service.go:22-513`, `services/observability/service.go:1-463`, `http/handlers_metrics.go:1-199`, `http/middleware_metrics.go:12-50`

---

## 9. Scores

| Dimension | Score | Notes |
|-----------|-------|-------|
| Error handling | **9 / 10** | 1254 wrapped errors, correct `respondInternalError` masking; deduct 1 for 12 silent `UpdateStatus` swallows |
| Logging (structure) | **8 / 10** | `StructuredLogger` + `slog` everywhere on hot path; heavy adoption (165 sites) |
| Logging (purity) | **6 / 10** | 31 `Printf` sites bypass JSON pipeline; `correlation_id` not bridged |
| Observability | **8 / 10** | Snapshots per service all exist; wildcard observer and heartbeat are exemplary; 6 services dark in `/metrics`; webhook has no snapshot |
| P0 metric completeness | **6 / 10** | Keys emitted but `legacy_fallback` always 0 and two keys are log-only, not counters |
| **Overall quality polish** | **7.4 / 10** | Functionally solid; polish delta is ~100 lines of wiring + log replacement |

---

*Generated by 110-09-07 — no files modified, report-only.*
