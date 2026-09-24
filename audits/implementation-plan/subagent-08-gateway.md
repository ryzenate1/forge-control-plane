# Subagent 08 — Networking / Gateway — Single-Writer Gateway Inversion — Implementation Plan

**Scope:** P0 cluster F-NET-01..10 (FINAL_PARITY §8, phase-04 synthesis, reverification subagent-14 confirms all 10 still BROKEN 2026-08-23), plus oscillation / LB config ignored / weighting dead / compose verify global mu.
**Status:** DESIGN — no product code modified. All citations are file:line pinned to current snapshot.
**Owner:** Gateway/Reconciler guild — single DRI.
**Date:** 2026-08-24

> **One-line thesis:** Replace 5 concurrent writers on one Caddy JSON doc with **one reconciler** that converges a Traefik-shaped `Router → Middleware(named ref) → Service → Target` desired state to the gateway via true dry-run + sub-resource merges. Domains becomes a provider, not a writer.

---

## Table of Contents

1. [0. Executive summary & blast radius](#0-executive-summary--blast-radius)
2. [1. Current architecture autopsy (file:line)](#1-current-architecture-autopsy-fileline)
3. [2. Target model — Traefik-shaped gateway](#2-target-model--traefik-shaped-gateway)
4. [3. Schema migration — additive, dual-read](#3-schema-migration--additive-dual-read)
5. [4. Go interfaces — GatewayAdapter as sole abstraction](#4-go-interfaces--gatewayadapter-as-sole-abstraction)
6. [5. Single reconciler — desired-state convergence](#5-single-reconciler--desired-state-convergence)
7. [6. True dry-run & safe apply](#6-true-dry-run--safe-apply)
8. [7. Deterministic middleware order, group-aware withdrawal, protocol branch](#7-deterministic-middleware-order-group-aware-withdrawal-protocol-branch)
9. [8. Beacon / data-plane changes](#8-beacon--data-plane-changes)
10. [9. Frontend — ONE Gateways page](#9-frontend--one-gateways-page)
11. [10. Rollout Phases A–D with feature flags](#10-rollout-phases-ad-with-feature-flags)
12. [11. Backward compat — no wipe during migration](#11-backward-compat--no-wipe-during-migration)
13. [12. Testing strategy](#12-testing-strategy)
14. [13. Risks & mitigations](#13-risks--mitigations)
15. [14. Appendix A — P0 → fix traceability](#14-appendix-a--p0--fix-traceability)
16. [15. Appendix B — Verification checklist](#15-appendix-b--verification-checklist)

---

## 0. Executive summary & blast radius

This is the densest P0 cluster in the entire audit — **10 P0s** — and three amplify each other (F-NET-01 + F-NET-02 + F-NET-03 = wipe + last-writer-wins + fictitious rollback). No other phase has a five-writer single-resource race.

| # | Title | Current severity — what breaks today | After Phase A hotfix | After Phase C (inversion) |
|---|---|---|---|---|
| F-NET-01 | Empty-rule ingress sync wipes Caddy every 30 s `crossnode/ingress_sync.go:111` | Periodic total outage of `gamepanel-domains` + TLS every 30 s + on each node churn event | **Mitigated** (empty-skip + flag) | **Eliminated** (syncer removed) |
| F-NET-02 | Asymmetric merge: `UpdateDomainRoutes` merge vs `updateRoutesAtomic` full-replace `caddy_proxy.go:673-720,514-593` | Any route sync erases domain routes; TLS provision wipes both; `:80` SO_REUSEPORT nondeterminism | **Mitigated** (all → sub-resource) | **Eliminated** |
| F-NET-03 | `validate=apply+snapshot-after` `caddy_proxy.go:980-1002,691-695` | Rollback restores **bad** config; double-apply per update; no dry-run | **Mitigated** (snapshot before) | **Eliminated** (true dry-run) |
| F-NET-04 | Fictional `rate_limit`/`circuit_breaker` `caddy_proxy.go:795-875` | Enabling **any** policy bricks every future `UpdateRoutes` | **Mitigated** (feature gate) | **Eliminated** (real handlers) |
| F-NET-05 | All-policies-all-routes `traefik_proxy.go:960-972` | Cross-tenant ACL/rate-limit leakage, non-deterministic order | Partial | **Eliminated** (FK+order) |
| F-NET-06 | Grouped-route removal misses dead nodes `caddy_proxy.go:60,949` | Dead backends keep receiving traffic | **Fixed** | **Fixed** |
| F-NET-07 | TCP→HTTP mis-render `caddy_proxy.go:924-978` / `service.go:383-387` | L4 rules never match | **Mitigated** (reject-or-branch) | **Eliminated** |
| F-NET-08 | `SetCertificate` zero callers `gateway_adapter.go:49` | Certs never leave Postgres — handshake serves expired/self-signed | Wired | **Eliminated** + reconciler |
| F-NET-09 | `HTTPSolver` never mounted `acme/service.go:157-159` | HTTP-01 always times out | Wired via flag | **Eliminated** |
| F-NET-10 | Admin UX non-functional `traffic/page.tsx:14-22` vs `service.go:24` | Traffic page 100 % 405/400 | Shim + flag | **Eliminated** (ONE page) |
| + | Oscillation (probe removes, rebuild re-adds) `service.go:1001-1082` vs `caddy_proxy.go:503-512` | Flap during outage | Threshold respected | **Eliminated** (builder checks health) |
| + | LB `HealthCheckConfig` ignored `dataplane.go:296-326` vs `service.go:62-69` | Operator tuning dead | Flag | **Eliminated** |
| + | Weighting dead `caddy_proxy.go:934-939` + shared `__weighted` `loadbalancer/service.go:480-482` | WRR unweighted | Hotfix loses no weight | **Eliminated** |
| + | Compose verify global mu (shared note for orchestrator guild) | Global lock blocks verifies | Not in gateway — delegated | Delegated |

**Philosophy:** Phase A stops the bleeding in <1 day with guards and no schema change. Phase B adds new tables alongside old (dual-read). Phase C flips the single writer on behind a flag with old writers deleted. Phase D decides Traefik fate and cleans tech debt.

---

## 1. Current architecture autopsy (file:line)

### 1.1 Five writers, one JSON doc

| Writer | File:line | What it writes | Merge discipline | Flag |
|---|---|---|---|---|
| **1. TrafficManager** `tmSvc` | `cmd/api/main.go:1025` `trafficmanager.NewWithPersistence(db, db, db, db, caddyProxy)` | `apps.http.servers.gamepanel` via `caddy_proxy.go:673-720` `updateRoutesAtomic → buildServerConfig ({gamepanel:…}) → POST /config/` | **Full replace** — replaces **entire** running config (`caddy/caddy.go:115-142` semantics, verified `reference/networking/caddy/caddyconfig/load.go:68-72`) | Active every `POST /traffic/rules`, `SyncRoutes` (`service.go:590-618`), and probe path |
| **2. Domains service** `domainSvc` | `cmd/api/main.go:998` `domains.New(domainAdapter, caddyProxy)` → `domains/service.go:455-489` `syncCaddyRoutes` | `apps.http.servers.gamepanel-domains` via `caddy_proxy.go:514-593` `UpdateDomainRoutes` | **Merge** — `getRunningConfig → json.Unmarshal → servers["gamepanel-domains"]=routes → POST /config/` | Active on every domain mutation + startup sync `main.go:1030-1038` |
| **3. IngressSynchronizer** | `cmd/api/main.go:993-995` `crossnode.NewIngressSynchronizer(caddyProxy, resolver, healthFilter)` `Start(appCtx, 30*time.Second)` + 3 event handlers `main.go:1041-1067` | `caddyProxy.UpdateRoutes` `crossnode/ingress_sync.go:166` `mergedRules` | **Full replace** (same as #1) but **empty** | `SetRules/UpsertRule/RemoveRule` have **zero non-test callers** (`grep -rn SetRules forge/api` → only `crossnode/ingress_sync.go:200-228` + tests). `rules` map stays empty forever → `GroupRulesByRoute([])` → 0 groups → builds `routes:[]` and wipes config |
| **4. L4 LoadBalancer dataplane** | `loadbalancer/dataplane.go:20-49,115-270` direct TCP/UDP `net.Listen` bypassing any gateway adapter | Own sockets `:30000-30100` (`loadbalancer/service.go:104-108`) | N/A (bypasses Caddy) | Gated `LOAD_BALANCER_ENABLED` `:107` but still a gateway surface |
| **5. NPM-style rows** `proxy_domains / redirect_rules / security_headers` | `store_proxy_domains.go` + `store_redirect_rules.go` + `store_security_headers.go` | No renderer — `security_headers` has zero readers (`store_security_headers.go:45` write-only); `proxy_domains` only used when `domains/service.go:455-489` copies to `gamepanel-domains` | N/A | Orphan |

Shared `:80` on both servers `caddy_proxy.go:567` (`gamepanel-domains listen :80`) and `caddy_proxy.go:713` (`gamepanel listen :80,:443`) → `SO_REUSEPORT` nondeterminism (`reference/networking/caddy/listeners.go:115-121`). All writers pass the **same** `caddyProxy` instance (single `adminAddr` `main.go:982`), protected only by a `sync.Mutex` inside the adapter (`caddy_proxy.go:23`), not a distributed lock — last writer wins.

### 1.2 Four adapter abstractions that should be one

- `GatewayAdapter` `trafficmanager/gateway_adapter.go:38-64` — intended single abstraction (`UpdateRoutes`, `RemoveRoutes`, `SetCertificate`, `ValidateConfig`, `Health`, `SetUpstreamHealth`).
- Legacy `ReverseProxy` `trafficmanager/service.go:101-105` — older `UpdateRoutes/RemoveRoutes/GetActiveConnections` still used by `tmSvc.proxy` field (`service.go:83`), separate from `adapter`.
- `domains.caddyUpdater` `domains/service.go:74-76` — second interface `UpdateDomainRoutes` on same proxy.
- `servicediscovery.NetworkAdapter` `servicediscovery/adapter.go:8` — empty, unrelated.

Result: `tmSvc` holds **both** `proxy ReverseProxy` and `adapter GatewayAdapter` (`service.go:83-84`), with different codepaths using different fields (`ApplyRoutes` uses `proxy`, `ProbeTargets` uses `adapter.SetUpstreamHealth` `service.go:1048-1075`).

### 1.3 Additional confirmed breakage (re-verified 2026-08-23)

| Finding | File:line | Symptom |
|---|---|---|
| Fictive handlers | `caddy_proxy.go:795-875` `buildPolicyHandles` emits `{"handler":"rate_limit","rate":"N/s"}` / `{"handler":"circuit_breaker","max_failures":…}` — not in Caddy registry (`modules/caddyhttp/*` — CB is `reverse_proxy` namespace `reverseproxy.go:109`). Tests assert broken rendering `caddy_proxy_test.go:280-314,547-615`. | `validateConfig` via `/load` fails → every `UpdateRoutes` with any policy aborts |
| All-policies-all-routes | `caddy_proxy.go:722-793` `buildPolicyRoutes` concatenates every policy handle onto every route; `traefik_proxy.go:960-972` `collectApplicablePolicies` comment `For now, return all`; `routegroup.go:149-151` `extractPolicyID() = ""` always | Cross-tenant leakage; `traefik_proxy.go:769-787` map iteration non-deterministic order (Traefik order matters `middlewares.go:50-83`) |
| Grouped-route miss | `caddy_proxy.go:60-74` `RemoveRoutes` DELETEs `/id/gamepanel-<id>`; but `buildGroupedRoute` `caddy_proxy.go:949-951` creates `@id=gamepanel-<firstRuleID>-group`. Withdraw `service.go:753-798` enumerates ruleIDs per server — grouped route survives unless its first rule is the dead one. `CleanupStale` handles `-group` correctly (`:355-357`), but hot `RemoveRoutes` does not. | Dead backends keep receiving traffic |
| validate=apply+snapshot-after | `caddy_proxy.go:980-1002` `validateConfig` POSTs candidate to `/load` (`Cache-Control: must-revalidate` — still an apply, `caddyconfig/load.go:68-116`); then `updateRoutesAtomic:687-702` snapshots **after** mutation → `lastValidConfig` stores new/broken config → `restoreConfig`/`Rollback:279-293` replays bad gen. `caddy_tls.go:240-251` TLS validate is JSON-only. Traefik `validateYAMLConfig:974-1005` is YAML round-trip + dead `POST /api/refresh:1065-1084` (API is GET-only `pkg/api/handler.go:103-133`; file provider self-heats `provider/file/file.go:90,170-207`). | Every "validation" mutates; rollback fictitious; double-apply per update |
| TCP mis-render | `service.go:383-387` admits `tcp`, rejects `udp`; `caddy_proxy.go:924-978` has **zero** `if protocol=="tcp"` branch (always `host`+`path`+`reverse_proxy`); Traefik TCP `HostSNI` `traefik_proxy.go:854-905` so plain TCP never matches. Caddy weight `caddy_proxy.go:934-939` writes `upstreams[].weight` (not a Caddy `Upstream` field — `hosts.go:29`); recovery `caddy_proxy.go:503-507` appends bare `{"dial":…}` losing weight. | L4 rules never match; weights silently discarded |
| Cert non-delivery | `gateway_adapter.go:49` `SetCertificate/RemoveCertificate` **zero callers** (verified `grep -rn SetCertificate forge/` only interface+impls). Renewal `acme/service.go:322-366,420-438` + queue fallback `main.go:1178-1192` only `store.UpdateCertificate`. `caddy_tls.go:186-238` `buildTLSConfig` wipes routes; `NewCaddyTLSManager` never constructed (`grep NewCaddyTLSManager forge/` empty; `main.go:982` hardcodes `NewCaddyReverseProxy`). Traefik `traefik_proxy.go:380-385` writes PEM bodies into `certFile/keyFile` path fields. | ACME succeeds → no TLS served |
| HTTP-01 dead | `acme/service.go:52-92` `httpChallenger` + `HTTPSolver()->http.Handler` `service.go:157-159` → zero callers; defaults `ChallengeTypeHTTP01:203-205` always times out | Default challenge always fails |
| Admin UX non-functional | `forge/web/app/admin/traffic/page.tsx:14-22` posts `{path,targetGroup,priority,methods}` vs backend `RoutingRule{domain,path,targetPort,…} service.go:24-39`; `policy type:rate_limit` generic `config: {}` vs `TrafficPolicy{RateLimit,IPWhitelist,…} service.go:41-54`; `handlers_trafficmanager.go:80-86` only `GET /policies/:id` (no list) → 405; `PATCH:94` vs `PUT:42` → 405; cert upload `POST /certificates 405` (`certificates/page.tsx:43-50` vs `handlers_certificates.go:15-77` no `POST /`) | 100 % failure without curl workarounds |
| Oscillation | `service.go:1001-1082` `ProbeTargets` (2 s TCP dial, threshold 3) calls `adapter.SetUpstreamHealth:1048-1075` removing dead upstream; but full rebuild `caddy_proxy.go:673-720` / `service.go:562-618` ignores `healthStatus` and re-adds it. | Flap during outage |
| LB config ignored | `loadbalancer/dataplane.go:296-326` `runHealthChecks` fixed 2 s TCP dial ignoring `HealthCheckConfig{Path,Interval,Thresholds} service.go:62-69`, skipping UDP `:306-308`; shared `__weighted` counter `:480-482` across all groups | Operator tuning dead |
| Triplicated grouping | `caddy_proxy.go:894-922` + `traefik_proxy.go:725-753` duplicate `groupRules`; `crossnode/routegroup.go:32-68` `GroupRulesByRoute` (normalizes `""→"/"` `:44-49` where adapters don't) | Same rules group differently per path |

---

## 2. Target model — Traefik-shaped gateway

Adopt `reference/networking/traefik/pkg/config/dynamic/http_config.go:38-99` as the canonical shape (`reference/networking/caddy/modules/caddyhttp/routes.go:31` is the Caddy analogue but Traefik's named-middleware reference is the cleaner DB model). NPM `backend/schema/components/proxy-host-object.json:4-25` is the anti-pattern (denormalized) — we want normalized refs.

### 2.1 Conceptual model

```
Provider (DB tables)  →  Reconciler  →  GatewayAdapter  →  Caddy/Traefik

Providers emit     : GatewayRouter  (host+path+priority+protocol+tls+entrypoints)
                    GatewayMiddleware (typed, versioned, deterministic order)
                    GatewayService (LB strategy + sticky meta)
                    GatewayTarget (dial + weight + health)
                    GatewayCertificate (SNI → cert material)
                    GatewayRouterMiddleware (M:N, ordered)   ← replaces all-policies-all-routes
                    GatewayRouterTargetGroup (optional — when a router fronts >1 server)
```

```
┌─────────────────────────────────────────────────────────────────────────┐
│                Desired state (Postgres)                                 │
│  traffic_rules ──┐                                                      │
│  proxy_domains ──┼──► GatewayRouter  ──M:N──► GatewayMiddleware          │
│  (legacy)        │         │                       │  type ∈             │
│                  │         │                       │  { rate_limit,      │
│                  │         ├──► GatewayService ──*─┤    ip_allowlist,    │
│                  │         │         │              │    ip_denylist,    │
│                  │         │    GatewayTarget       │    headers,        │
│  target_groups ──┘         │                        │    redirect,       │
│                            │                        │    circuit_breaker}│
│  certificates ────────────► GatewayCertificate (SNI)└────────────────────┘
└─────────────────────────────────────────────────────────────────────────┘
                                 │
                        ┌────────▼────────┐
                        │  Gateway        │
                        │  Reconciler     │  single writer, owns diff & apply
                        │  (1 goroutine)  │
                        └────────┬────────┘
                                 │ GatewayAdapter
               ┌─────────────────┼─────────────────┐
               │                 │                 │
        CaddyAdapter      TraefikAdapter     (future: NginxAdapter)
     (sub-resource merge)  (file-watch)      — only ONE active via flag
```

### 2.2 Table ownership mapping (7 legacy tables → 6 normalized)

| Legacy table(s) | Rows | Target | Migration |
|---|---|---|---|
| `traffic_rules` (`038_traffic_routing.sql`, divergent `083_a_traffic_rules.sql`) | 1 row per upstream intent | `gateway_routers` (1 per unique Domain\|Path\|Protocol) + `gateway_services` + `gateway_targets` (1 per rule that shares router) | Phase B additive |
| `proxy_domains` (`118` + `store_proxy_domains.go`) | 1 per custom domain | Also becomes `gateway_router` with `source='proxy_domains'` — merged with `traffic_rules` routers | Synthetic router emission — no second writer |
| `traffic_policies` (`038`) + `security_headers` (`store_security_headers.go`) + `redirect_rules` (`store_redirect_rules.go`) | Policy as cross-cutting concern | `gateway_middlewares` — typed rows, versioned. Each router references via `gateway_router_middlewares` | FK join eliminates F-NET-05 |
| `target_groups` / `store_target_groups.go` (if present; otherwise `targetHost/Port` on rule) | Pool of backends | `gateway_services` + `gateway_targets` | Direct map |
| `certificates` (`094` + `store_certificates.go`) | TLS material | `gateway_certificates` (normalized from cert store) — consumed by adapter via `SetCertificate` reconciler | F-NET-08 |
| `proxy_domains` cert fields (`CertType/Data/Key`) | In-row TLS | Migrated to `gateway_certificates` — single source of truth | Deduplication |

**Invariant:** After Phase C, **no** row is rendered directly. Only `gateway_*` is the reconciler's input. Legacy tables stay for dual-read until Phase D cleanup.

### 2.3 Single writer invariant

- **Before:** 5 writers (`trafficmanager/service.go:1025`, `domains/service.go:455-489`, `crossnode/ingress_sync.go:30`, `loadbalancer/dataplane.go:20-49`, dead NPM rows) + 2 managers (`caddy_tls.go:18-32` never constructed, `gateway_adapter.go:10-64` dead `SetCertificate`).
- **After:** Exactly **one** goroutine holds "gateway writer" — `GatewayReconciler`. All mutation endpoints (traffic CRUD, domains CRUD, policy CRUD, cert issue/renew, health events) only **write desired state** to Postgres + emit an event; they never touch `adminAddr`.

This mirrors `reference/app-platforms/uncloud/internal/machine/caddyconfig/controller.go:92,148-200` — per-node Caddy watches a stream and fingerprint-caches (`:188-189` never writes invalid), never allowing two writers.

### 2.4 Why Traefik shape, even when rendering to Caddy

- Named middlewares referenced by router (`traefik/http_config.go:88` `Middlewares []string`) map cleanly to `gateway_router_middlewares` rows + ordered arrays.
- Services/targets separation lets Caddy emit correct `upstreams[]` **or** `weighted_round_robin { weights[] }` per-pool, not per-upstream.
- Priority/ordering (`traefik/http_config.go:49` `Priority`, Caddy `terminal/group` `routes.go:31`) becomes explicit `gateway_routers.priority` + deterministic middleware order column.

---

## 3. Schema migration — additive, dual-read

**Principle:** Every Phase B migration is **additive**; no `DROP`/`ALTER … DROP COLUMN` until Phase D after the flag is default-true for ≥1 release. Old code keeps reading legacy tables; new reconciler reads **both** and prefers `gateway_*` when present.

### 3.1 Migration index (new `internal/store/migrations/` + `forge/api/migrations/`)

| Seq | File | Purpose | Idempotency |
|---|---|---|---|
| 050 | `050_gateway_core.sql` | Core tables + enums | `CREATE TABLE IF NOT EXISTS` + `DO $$ … EXCEPTION WHEN duplicate_object` for types |
| 051 | `051_gateway_router_middlewares.sql` | M:N ordered join | Same |
| 052 | `052_gateway_backfill_routers.sql` | Data backfill from `traffic_rules` + `proxy_domains` (run once, idempotent `INSERT … ON CONFLICT DO NOTHING`) | Re-runnable |
| 053 | `053_gateway_certificates.sql` | Cert delivery normalized view | Same |
| 054 | `054_gateway_audit_triggers.sql` | Audit/timeline hooks (emit to `eventstore` or `audit_logs`; NOT Operations timeline — that stays pure) | Same |

Do **not** reuse numbers 038/083/117 etc. Use the next free sequence in `forge/api/migrations/` and `forge/api/internal/store/migrations/` — verify via `SELECT max(seq) FROM schema_migrations` at migration authoring time. Below uses the `050-054` placeholder range for doc clarity; actual numbers will be assigned by the SRE DBA at merge time.

### 3.2 DDL — `050_gateway_core.sql` (canonical — adapt to actual Postgres inheritance)

```sql
-- 050_gateway_core.sql — Gateway inversion core tables (additive).
-- Safe to run on existing clusters: IF NOT EXISTS + no destructive ALTER.
-- Backwards compat: legacy traffic_rules / proxy_domains / traffic_policies
-- keep getting written; this migration only ADDS new tables.

-- Enum for middleware type — deterministic ordering enforced by `kind_order`.
DO $$ BEGIN
  CREATE TYPE gateway_middleware_kind AS ENUM (
    'rate_limit',
    'ip_allowlist',
    'ip_denylist',
    'headers',
    'redirect',
    'circuit_breaker',
    'compress',
    'retry'
  );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
  CREATE TYPE gateway_router_source AS ENUM (
    'traffic_rules',
    'proxy_domains',
    'redirect_rules',
    'synthetic'
  );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- 1) Routers — collapse traffic_rules + proxy_domains into one router surface.
--    One row per unique (domain, path, protocol) = one logical match.
CREATE TABLE IF NOT EXISTS gateway_routers (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  -- Canonical match:
  domain          TEXT        NOT NULL,         -- normalized ASCII via idna.ToASCII, lowercased, no trailing dot
  path            TEXT        NOT NULL DEFAULT '/',  -- must start with /, url.ParseRequestURI valid
  protocol        TEXT        NOT NULL DEFAULT 'http' CHECK (protocol IN ('http','https','tcp','udp')),
  -- Behaviour:
  priority        INT         NOT NULL DEFAULT 0,     -- higher = earlier. Traefik Priority; Caddy sorted by specificity.
  enabled         BOOLEAN     NOT NULL DEFAULT true,
  tls_enabled     BOOLEAN     NOT NULL DEFAULT false, -- derived from attached redirect middleware; kept for fast query
  entrypoints     TEXT[]      NOT NULL DEFAULT '{web}', -- {'web','websecure','tcp','udp'} — allows L4 routes
  -- Provenance:
  source          gateway_router_source NOT NULL DEFAULT 'traffic_rules',
  legacy_group_key TEXT       NOT NULL DEFAULT '',     -- original domain|path|protocol key for backfill audit
  metadata        JSONB       NOT NULL DEFAULT '{}',  -- e.g. {"websocket":true}
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  -- Uniqueness: same logical route must not have two routers.
  UNIQUE (domain, path, protocol)
);
CREATE INDEX IF NOT EXISTS gateway_routers_domain_idx ON gateway_routers (domain);
CREATE INDEX IF NOT EXISTS gateway_routers_enabled_idx ON gateway_routers (enabled) WHERE enabled = true;
CREATE INDEX IF NOT EXISTS gateway_routers_protocol_idx ON gateway_routers (protocol);

-- 2) Middlewares — normalized, typed, versioned.
--    Each row is ONE Traefik-shaped middleware (one purpose), not a bag of unrelated fields.
CREATE TABLE IF NOT EXISTS gateway_middlewares (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name            TEXT        NOT NULL,      -- human label, unique for adapter reference fallback
  kind            gateway_middleware_kind NOT NULL,
  -- Rank inside a single router's chain. Lower = earlier.
  -- Enforced order: rate_limit(10) → ip_denylist(20) → ip_allowlist(30) → headers(40) → redirect(50) → cb(60) …
  kind_order      INT         NOT NULL DEFAULT 100,
  config          JSONB       NOT NULL DEFAULT '{}',
    -- Validated by kind:
    -- rate_limit:       {"average":int,"burst":int,"period":"1s"}  (no fictive handler)
    -- ip_allowlist:     {"sourceRange":["10.0.0.0/8"], "ipStrategy":{"depth":int}}
    -- ip_denylist:      same
    -- headers:          {"customRequestHeaders":{…},"customResponseHeaders":{…},"sslRedirect":bool,…}
    -- redirect:         {"scheme":"https","permanent":bool} or {"regex":…,"replacement":…}
    -- circuit_breaker:  {"expression":"NetworkErrorRatio() > 0.20"} OR {"maxFailures":int,"timeout":"30s"} (translated per adapter)
    -- compress/retry:   future
  enabled         BOOLEAN     NOT NULL DEFAULT true,
  -- Provenance: which legacy table row was collapsed into this middleware (for audit/backfill).
  source          TEXT        NOT NULL DEFAULT '',
  source_id       TEXT        NOT NULL DEFAULT '',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (name)
);
CREATE INDEX IF NOT EXISTS gateway_middlewares_kind_idx ON gateway_middlewares (kind);
CREATE INDEX IF NOT EXISTS gateway_middlewares_enabled_idx ON gateway_middlewares (enabled) WHERE enabled = true;

-- 3) Services — one per router (Traefik Service). Holds LB strategy, not per-target weight.
CREATE TABLE IF NOT EXISTS gateway_services (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  router_id       UUID        NOT NULL REFERENCES gateway_routers(id) ON DELETE CASCADE,
  name            TEXT        NOT NULL,      -- derived: 'gw-svc-' || router_id (or custom)
  strategy        TEXT        NOT NULL DEFAULT 'round_robin'
                  CHECK (strategy IN ('round_robin','least_conn','random','weighted_round_robin','ip_hash')),
  sticky          JSONB       NOT NULL DEFAULT '{}',   -- {"cookie":{"name":"_gp_…","secure":false,…}} when strategy needs it
  health_check    JSONB       NOT NULL DEFAULT '{}',   -- {"path":"/healthz","interval":"10s","timeout":"2s","unhealthyThreshold":3}
  pass_host_header BOOLEAN    NOT NULL DEFAULT true,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (router_id),
  UNIQUE (name)
);
CREATE INDEX IF NOT EXISTS gateway_services_router_idx ON gateway_services (router_id);

-- 4) Targets — upstreams per service (many). Weight lives HERE but is only emitted via
--    a weighted policy when needed (Caddy needs `weighted_round_robin { weights[] }` not per-upstream weight).
CREATE TABLE IF NOT EXISTS gateway_targets (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  service_id      UUID        NOT NULL REFERENCES gateway_services(id) ON DELETE CASCADE,
  server_id       UUID        REFERENCES servers(id) ON DELETE SET NULL, -- nullable for non-server targets
  host            TEXT        NOT NULL,         -- resolved targetHost or node PublicHostname/FQDN (no localhost fallback stored)
  port            INT         NOT NULL CHECK (port BETWEEN 1 AND 65535),
  weight          INT         NOT NULL DEFAULT 1 CHECK (weight >= 0),
  enabled         BOOLEAN     NOT NULL DEFAULT true,
  -- Health is NOT stored here long-term — read from HealthFilter at render time.
  -- But we keep a denormalized cache for the reconciler diff (fast path) + drift pre-check.
  health_status   TEXT        NOT NULL DEFAULT 'unknown' CHECK (health_status IN ('unknown','healthy','degraded','down')),
  last_checked_at TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (service_id, host, port)
);
CREATE INDEX IF NOT EXISTS gateway_targets_service_idx ON gateway_targets (service_id);
CREATE INDEX IF NOT EXISTS gateway_targets_server_idx ON gateway_targets (server_id);
CREATE INDEX IF NOT EXISTS gateway_targets_health_idx ON gateway_targets (health_status) WHERE health_status IN ('down','degraded');

-- 5) Certificates — normalized view so reconciler can diff desired vs reported.
--    This is NOT a copy of acme/certificates storage — it is the gateway-facing delivery record.
CREATE TABLE IF NOT EXISTS gateway_certificates (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  sni             TEXT        NOT NULL,       -- exact hostname or wildcard "*.example.com"
  cert_pem        TEXT        NOT NULL DEFAULT '',
  key_pem         TEXT        NOT NULL DEFAULT '',
  -- Paths when Traefik file provider is active (certFile/keyFile are paths, not bodies — fix F-NET-08 Traefik PEM-as-path).
  cert_path       TEXT        NOT NULL DEFAULT '',
  key_path        TEXT        NOT NULL DEFAULT '',
  issuer          TEXT        NOT NULL DEFAULT '',
  not_before      TIMESTAMPTZ,
  not_after       TIMESTAMPTZ,
  auto_renew      BOOLEAN     NOT NULL DEFAULT true,
  source_cert_id  UUID        REFERENCES certificates(id) ON DELETE SET NULL, -- legacy store.certificates FK
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (sni)
);
CREATE INDEX IF NOT EXISTS gateway_certificates_sni_idx ON gateway_certificates (sni);
CREATE INDEX IF NOT EXISTS gateway_certificates_expiry_idx ON gateway_certificates (not_after) WHERE not_after IS NOT NULL;

-- 6) Router ↔ Middleware join — ordered, per-router. THIS fixes F-NET-05.
CREATE TABLE IF NOT EXISTS gateway_router_middlewares (
  router_id       UUID        NOT NULL REFERENCES gateway_routers(id) ON DELETE CASCADE,
  middleware_id   UUID        NOT NULL REFERENCES gateway_middlewares(id) ON DELETE CASCADE,
  -- Rank inside this router's chain. Lower = earlier. Deterministic even when multiple mws share kind_order.
  priority        INT         NOT NULL DEFAULT 100,
  enabled         BOOLEAN     NOT NULL DEFAULT true,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (router_id, middleware_id)
);
CREATE INDEX IF NOT EXISTS gateway_router_middlewares_router_idx ON gateway_router_middlewares (router_id, priority);
CREATE INDEX IF NOT EXISTS gateway_router_middlewares_mw_idx ON gateway_router_middlewares (middleware_id);

-- 7) Reported state snapshot — so reconciler can diff desired vs reported without hitting gateway every tick.
--    This is NOT the source of truth — it mirrors `GET /config/` or file checksum.
CREATE TABLE IF NOT EXISTS gateway_reported_state (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  gateway_kind    TEXT        NOT NULL CHECK (gateway_kind IN ('caddy','traefik')),
  -- Fingerprint of last reported config (sha256 of canonical JSON/YAML). Fast diff before deep fetch.
  config_hash     TEXT        NOT NULL,
  config_json     JSONB       NOT NULL DEFAULT '{}',
  reported_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  -- Last reconciler attempt (for observability / audit).
  last_applied_at TIMESTAMPTZ,
  last_error      TEXT        NOT NULL DEFAULT '',
  UNIQUE (gateway_kind)
);

-- Updated-at triggers (idempotent pattern).
CREATE OR REPLACE FUNCTION gateway_touch_updated_at() RETURNS TRIGGER AS $$
BEGIN NEW.updated_at = NOW(); RETURN NEW; END; $$ LANGUAGE plpgsql;

DO $$ BEGIN CREATE TRIGGER gateway_routers_touch BEFORE UPDATE ON gateway_routers FOR EACH ROW EXECUTE FUNCTION gateway_touch_updated_at(); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TRIGGER gateway_middlewares_touch BEFORE UPDATE ON gateway_middlewares FOR EACH ROW EXECUTE FUNCTION gateway_touch_updated_at(); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TRIGGER gateway_services_touch BEFORE UPDATE ON gateway_services FOR EACH ROW EXECUTE FUNCTION gateway_touch_updated_at(); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TRIGGER gateway_targets_touch BEFORE UPDATE ON gateway_targets FOR EACH ROW EXECUTE FUNCTION gateway_touch_updated_at(); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TRIGGER gateway_certificates_touch BEFORE UPDATE ON gateway_certificates FOR EACH ROW EXECUTE FUNCTION gateway_touch_updated_at(); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
```

### 3.3 Join ordering DDL

```sql
-- 051_gateway_router_middlewares.sql — deterministic ordering + guard rails
-- Priority of a middleware instance = (middleware.kind_order * 1000 + router_middlewares.priority)
-- This lets global kind ordering (rate_limit before ip_denylist etc.) compose with per-router tweaks.

-- Enforce that priority is unique per router (stable sort needs total order).
-- Use a conditional unique index that tolerates disabled rows (they don't participate in rendering).
CREATE UNIQUE INDEX IF NOT EXISTS gateway_router_middlewares_priority_uniq
  ON gateway_router_middlewares (router_id, priority) WHERE enabled = true;

-- Ensure every router has at most one circuit_breaker — Caddy has no per-middleware CB, only reverse_proxy-level.
-- Traefik allows one CB per router; enforcing one keeps translation safe.
-- This is a soft guard via partial unique on the middleware kind (requires join check in app code; index is advisory).
-- App code validates: reject attaching second CB to same router at API layer.
```

### 3.4 Backfill — `052_gateway_backfill_routers.sql` (idempotent, re-runnable, zero-downtime)

```sql
-- 052_gateway_backfill_routers.sql — Populate gateway_* from legacy tables.
-- Idempotent: INSERT … ON CONFLICT DO NOTHING; UPDATE WHERE needed.
-- Safe to run while Phase A hotfix is live and before Phase B code is deployed;
-- reconciler will not read gateway_* until GATEWAY_SINGLE_WRITER=true.

-- Step 1: Routers from traffic_rules (collapse by domain|path|protocol).
-- Note: traffic_rules may have duplicate keys with different IDs → they become targets under one service.
INSERT INTO gateway_routers (domain, path, protocol, source, legacy_group_key, priority, enabled, metadata)
SELECT
  lower(trim(both '.' from r.domain)) AS domain,
  COALESCE(NULLIF(r.path,''), '/')   AS path,
  COALESCE(NULLIF(r.protocol,''), 'http') AS protocol,
  'traffic_rules'::gateway_router_source,
  (r.domain || '|' || COALESCE(r.path,'/') || '|' || COALESCE(r.protocol,'http')) AS legacy_group_key,
  0,
  true,
  jsonb_build_object('websocket', bool_or(r.web_socket))
FROM traffic_rules r
WHERE r.enabled = true
GROUP BY 1,2,3
ON CONFLICT (domain, path, protocol) DO NOTHING;

-- Routers from proxy_domains (custom domains). Use gateway_routers too — domains become a provider.
-- Only insert when not already present as a traffic_rules router (dedupe).
INSERT INTO gateway_routers (domain, path, protocol, source, priority, enabled, metadata)
SELECT
  lower(trim(both '.' from pd.hostname)),
  COALESCE(NULLIF(pd.path,'/'), '/'),
  'http',
  'proxy_domains'::gateway_router_source,
  0,
  true,
  jsonb_build_object('websocket', pd.websocket)
FROM proxy_domains pd
ON CONFLICT (domain, path, protocol) DO NOTHING;

-- Step 2: Services per router (one-to-one). Idempotent.
INSERT INTO gateway_services (router_id, name, strategy)
SELECT
  gr.id,
  'gw-svc-' || substring(gr.id::text, 1, 8) || '-' || regexp_replace(gr.domain, '[^a-z0-9]', '-', 'g'),
  'round_robin'
FROM gateway_routers gr
ON CONFLICT (router_id) DO NOTHING;

-- Step 3: Targets — one per enabled traffic_rules row that maps to its collapsed router.
-- This preserves per-row weight/Host/Port even though they previously were grouped only at render time.
INSERT INTO gateway_targets (service_id, server_id, host, port, weight, enabled)
SELECT
  gs.id,
  NULLIF(r.server_id, '')::uuid,
  COALESCE(NULLIF(r.target_host,''), ''),  -- empty means "resolve via node PublicHostname/FQDN" — reconciler resolves at render
  r.target_port,
  GREATEST(r.weight, 1),
  r.enabled
FROM traffic_rules r
JOIN gateway_routers gr ON gr.domain = lower(trim(both '.' from r.domain))
                       AND gr.path   = COALESCE(NULLIF(r.path,''), '/')
                       AND gr.protocol = COALESCE(NULLIF(r.protocol,''), 'http')
JOIN gateway_services gs ON gs.router_id = gr.id
WHERE r.enabled = true
  AND r.target_port BETWEEN 1 AND 65535
ON CONFLICT (service_id, host, port) DO NOTHING;

-- Targets for proxy_domains (single target per domain).
INSERT INTO gateway_targets (service_id, host, port, weight, enabled)
SELECT
  gs.id,
  '127.0.0.1',  -- proxy_domains currently hardcode localhost:8080 — preserve until reconciler fixes to TargetHost authoritative
  pd.port,
  1,
  true
FROM proxy_domains pd
JOIN gateway_routers gr ON gr.domain = lower(trim(both '.' from pd.hostname))
                       AND gr.path   = COALESCE(NULLIF(pd.path,'/'), '/')
JOIN gateway_services gs ON gs.router_id = gr.id
ON CONFLICT (service_id, host, port) DO NOTHING;

-- Step 4: Middlewares from traffic_policies + security_headers + redirect_rules.
-- Each policy row contributes 0..N typed middlewares (one per concern) — NOT one bag.
-- Idempotent: use source+source_id to avoid duplicates.

-- 4a) rate_limit
INSERT INTO gateway_middlewares (name, kind, kind_order, config, source, source_id)
SELECT
  'pol-' || tp.id || '-ratelimit',
  'rate_limit'::gateway_middleware_kind,
  10,
  jsonb_build_object('average', tp.rate_limit, 'burst', tp.rate_limit_burst, 'period','1s'),
  'traffic_policies', tp.id
FROM traffic_policies tp WHERE tp.rate_limit > 0
ON CONFLICT (name) DO NOTHING;

-- 4b) ip_allowlist
INSERT INTO gateway_middlewares (name, kind, kind_order, config, source, source_id)
SELECT
  'pol-' || tp.id || '-allowlist',
  'ip_allowlist'::gateway_middleware_kind,
  30,
  jsonb_build_object('sourceRange', tp.ip_whitelist),
  'traffic_policies', tp.id
FROM traffic_policies tp WHERE array_length(tp.ip_whitelist,1) IS NOT NULL
ON CONFLICT (name) DO NOTHING;

-- 4c) ip_denylist
INSERT INTO gateway_middlewares (name, kind, kind_order, config, source, source_id)
SELECT
  'pol-' || tp.id || '-denylist',
  'ip_denylist'::gateway_middleware_kind,
  20,
  jsonb_build_object('sourceRange', tp.ip_blacklist),
  'traffic_policies', tp.id
FROM traffic_policies tp WHERE array_length(tp.ip_blacklist,1) IS NOT NULL
ON CONFLICT (name) DO NOTHING;

-- 4d) circuit_breaker — guarded; only created but NOT attached until feature flagged
INSERT INTO gateway_middlewares (name, kind, kind_order, config, source, source_id)
SELECT
  'pol-' || tp.id || '-cb',
  'circuit_breaker'::gateway_middleware_kind,
  60,
  jsonb_build_object('maxFailures', COALESCE(tp.circuit_breaker_threshold,5), 'timeout', COALESCE(tp.circuit_breaker_timeout,30) || 's'),
  'traffic_policies', tp.id
FROM traffic_policies tp WHERE tp.circuit_breaker = true
ON CONFLICT (name) DO NOTHING;

-- 4e) https redirect (TLSEnabled)
INSERT INTO gateway_middlewares (name, kind, kind_order, config, source, source_id)
SELECT
  'pol-' || tp.id || '-redirect',
  'redirect'::gateway_middleware_kind,
  50,
  jsonb_build_object('scheme','https','permanent',true),
  'traffic_policies', tp.id
FROM traffic_policies tp WHERE tp.tls_enabled = true
ON CONFLICT (name) DO NOTHING;

-- Step 5: Attach middlewares to routers.
-- **Phase B backfill policy: NO auto-attach.** All-policies-all-routes is the bug (F-NET-05).
-- We intentionally leave gateway_router_middlewares EMPTY and let operators attach explicitly
-- via the new Gateways UI (Phase C). To preserve existing behaviour for clusters that DID rely
-- on the leak, the reconciler has a flag GATEWAY_LEGACY_ATTACH_ALL that, when true, renders
-- all middlewares on every router (compatibility mode) but logs a warning.
-- So no INSERT into gateway_router_middlewares in the backfill.
```

**Why no auto-attach in backfill:** F-NET-05 is cross-tenant leakage — automatically attaching every policy to every router would preserve the leak. The correct migration is to require explicit attachment; clusters that depended on the leak opt into `GATEWAY_LEGACY_ATTACH_ALL=true` for one release while they attach correctly.

### 3.5 Certificates — `053_gateway_certificates.sql`

```sql
-- 053_gateway_certificates.sql — gateway-visible cert delivery rows.
-- Populated by: (a) backfill from existing `certificates` + `proxy_domains` rows,
-- (b) the reconciler after every ACME issue/renew, and (c) custom-cert upload path.

-- Backfill: one gateway_certificates row per distinct SNI that has material.
INSERT INTO gateway_certificates (sni, cert_pem, key_pem, not_after, source_cert_id, auto_renew)
SELECT DISTINCT ON (lower(c.domains[1]))  -- assumes certificates.domains is TEXT[] ; adjust to schema
  lower(c.domains[1]) AS sni,
  c.certificate,
  c.private_key,
  c.expires_at,
  c.id,
  c.auto_renew
FROM certificates c
WHERE c.certificate <> '' AND c.private_key <> ''
ON CONFLICT (sni) DO NOTHING;

-- Backfill proxy_domains custom certs (CertData/CertKey) — these never reached Caddy before (F-NET-08).
INSERT INTO gateway_certificates (sni, cert_pem, key_pem, source_cert_id)
SELECT lower(pd.hostname), pd.cert_data, pd.cert_key, NULL
FROM proxy_domains pd
WHERE pd.cert_data <> '' AND pd.cert_key <> ''
ON CONFLICT (sni) DO UPDATE SET cert_pem = EXCLUDED.cert_pem, key_pem = EXCLUDED.key_pem;
```

For Traefik `certFile/keyFile` are **paths** (`traefik_proxy.go:1100-1108` `TraefikTLSCertificate{CertFile,KeyFile}`) — the adapter will materialize `gateway_certificates.cert_pem/key_pem` as `*.pem` files under `configDir/certs/<sni>.{crt,key}` and populate `certFile/keyFile` with **paths**. For Caddy `tls/certificates/<sni>` the adapter POSTs the PEM bodies directly (`caddy_proxy.go:83-121` pattern) — no file needed.

### 3.6 Dual-read strategy (Phase B, before single writer is flipped)

```
Legacy write path (still active in Phase A+B):
  POST /admin/traffic/rules → trafficmanager/service.go:291-326 CreateRoutingRule
      → s.ruleStore.CreateRoutingRule → traffic_rules
      → (if GATEWAY_DUAL_WRITE) also upsert gateway_routers/services/targets

New write path (Phase C, after flag flip):
  POST /gateways/routers     → gateway/service.go:CreateRouter
      → gateway_routers / services / targets / router_middlewares
      → event Envelope{Type:"gateway_desired_changed"} → reconciler trigger

Reconciler read path (Phase B — dual-read, no write to gateway yet):
  - If gateway_* has any rows for the domain → render from gateway_*.
  - Else fall back to legacy traffic_rules + proxy_domains (+ HealthFilter).
  - Count of "legacy fallback" routers emitted as metric `gateway_legacy_fallback_total`.
```

**Dual-write helper (Phase B only, removed in Phase C):**

```go
// trafficmanager/service.go — wrapped behind GATEWAY_DUAL_WRITE (default false until Phase B)
func (s *Service) createRoutingRuleDualWrite(ctx context.Context, rule *RoutingRule) error {
  if os.Getenv("GATEWAY_DUAL_WRITE") != "true" { return nil }
  // best-effort mirror to gateway_*: no hard error if it fails (log warn), legacy write already succeeded.
  // Uses the same INSERT … ON CONFLICT logic as backfill but per-row.
}
```

---

## 4. Go interfaces — GatewayAdapter as sole abstraction

### 4.1 Fold four abstractions into one

| Current | File:line | Fate |
|---|---|---|
| `GatewayAdapter` | `trafficmanager/gateway_adapter.go:38-64` | **Keep as THE interface**, extend with `Describe()` + `DiffDesired()` for reconciler |
| `ReverseProxy` (legacy) | `trafficmanager/service.go:101-105` | **Delete** — `tmSvc.proxy` field removed after Phase C (`service.go:83`) |
| `domains.caddyUpdater` | `domains/service.go:74-76` | **Delete** — domains no longer touches Caddy; it writes `gateway_*` and lets reconciler render |
| `NetworkAdapter` | `servicediscovery/adapter.go:8` | **Fold or delete** — it is unused by the gateway path; if discovery must affect targets, it emits `GatewayTarget.health_status` changes, not a direct adapter call |

### 4.2 Final `GatewayAdapter` (sole abstraction)

```go
// forge/api/internal/services/gateway/adapter.go  (new package; GatewayAdapter moves here)
// Package gateway is the ONLY package that holds the reconciler.
// Adapters (caddy/traefik) implement GatewayAdapter inside this package.
// Other services (trafficmanager, domains, acme, LB) become gateway-API consumers, not writers.

package gateway

type AdapterKind string
const (
  AdapterCaddy   AdapterKind = "caddy"
  AdapterTraefik AdapterKind = "traefik"
)

// GatewayAdapter is the sole gateway abstraction.
// Only GatewayReconciler calls Mutating methods. Everyone else writes to gateway_*.
type GatewayAdapter interface {
  // Identity
  Kind() AdapterKind
  Describe() string // e.g. "caddy:http://127.0.0.1:2019" for logs/metrics

  // Read — reported state
  GetConfig(ctx context.Context) (json.RawMessage, error)   // GET /config/ or file read
  Health(ctx context.Context) AdapterHealth
  // Stats — for admin debug (honest: fetch /config/ server counts, NOT GetActiveConnections fiction).
  RouteCount(ctx context.Context) (int, error)

  // Write — ONLY GatewayReconciler calls these.
  // Desired is the reconciler's computed DesiredSnapshot (see §5).
  ApplyDesired(ctx context.Context, desired DesiredSnapshot) error
  // DryRun returns the set of warnings/errors the adapter would produce without mutating.
  // Caddy:  POST /adapt  (or sub-resource GET+JSON diff if /adapt unavailable — see §6).
  // Traefik: YAML structural validation + file-check (no daemon hit).
  DryRun(ctx context.Context, desired DesiredSnapshot) error

  // Cert delivery — reconciler calls after reading gateway_certificates.
  // Our fix wires the zero-callers path (F-NET-08).
  SetCertificate(ctx context.Context, cert GatewayCertificate) error
  RemoveCertificate(ctx context.Context, sni string) error

  // Migration helper: when GatewaReconciler is not yet enabled, legacy proxy can still
  // delegate sub-resource merges via this escape hatch. Removed in Phase D.
  DeprecatedRawApply(ctx context.Context, cfg json.RawMessage) error
}

type DesiredSnapshot struct {
  Routers      []GatewayRouter
  Middlewares  []GatewayMiddleware
  Services     []GatewayService   // 1:1 with routers
  Targets      []GatewayTarget    // N:1 with services
  RouterMws    []RouterMiddleware // ordered joins
  Certificates []GatewayCertificate
  // Hash for cheap diff & audit
  Hash         string
  GeneratedAt  time.Time
}

// Wire errors are returned as `*GatewayError` with Code ∈ {Validation, Apply, Rollback, Health} for metrics.
```

**Deletion of `ReverseProxy`:**

```go
// Before (service.go:73-89): Service { proxy ReverseProxy; adapter GatewayAdapter; … }
// After  (gateway/service.go):
type Reconciler struct {
  adapter GatewayAdapter // sole gateway dependency
  store   GatewayStore   // gateway_* tables
  health  *crossnode.HealthFilter
  resolver *crossnode.Resolver
  // NOT a trafficmanager.Service nor domains.Service pointer
}
```

`trafficmanager.Service.mu sync.RWMutex :81` and per-adapter `p.mu sync.Mutex :23` remain inside each adapter for intra-adapter serialization, but the cross-adapter concurrency (5 writers) is gone — only the reconciler goroutine ever calls `ApplyDesired`.

### 4.3 Adapter implementations

**`CaddyAdapter`** (`gateway/caddy_adapter.go`, refactor of `trafficmanager/caddy_proxy.go` 1079 lines):

- Moves `lastValidConfig json.RawMessage` → **persisted** `gateway_reported_state.config_json` + `config_hash` (DB) so restart survives (currently memory-only `caddy_proxy.go:24` lost on restart).
- `DryRun` uses `POST /adapt --adapt` via the admin API's content-type negotiation or the CLI-equivalent HTTP contract `reference/networking/caddy/caddyconfig/load.go:137-175` — **never** `POST /load` again.
- `ApplyDesired` **never** `POST /config/` whole-doc. It uses **sub-resource merges** `POST /config/apps/http/servers/<serverName>` and `POST /config/apps/http/servers/<serverName>/routes` (the same read-modify-write pattern `UpdateDomainRoutes` already uses `caddy_proxy.go:523-593`). See §6.
- `SetCertificate` writes `POST /tls/certificates/<sni>` with `{"certificate":pem,"key":pem}` (`caddy_proxy.go:93-121` proven path) — but **now has callers** via reconciler (`F-NET-08`).
- `Health` stops hardcoding `HealthHealthy` (`caddy_proxy.go:375-377` fiction). It `GET /config/` and validates reachability + config checksum.

**`TraefikAdapter`** (`gateway/traefik_adapter.go`, refactor of `traefik_proxy.go` 1139 lines):

- Decision at Phase D: **either fix or delete** (`phase-04/synthesis §18` + `final-parity §5 LF-05`). This plan keeps it behind `GATEWAY_ADAPTER=traefik` for on-prem customers, but fixes its two fatal bugs:

  1. **Dead reload** `traefik_proxy.go:1065-1084` `POST /api/refresh` → removed. Traefik file provider self-heats via `fsnotify` (`provider/file/file.go:90,170-207`). Adapter just **writes** `configDir/routes.yml` (+ backup) + optional `configDir/certs/<sni>.pem` files and fsyncs; Traefik picks it up.
  2. **PEM-as-path** `traefik_proxy.go:380-385` → writes real `*.pem` files and sets `certFile/keyFile` to **paths** (`TraefikTLSCertificate :1100-1108`).

- If the team decides to delete, Phase D PR removes `traefik_proxy.go` entirely and `AdapterKind` shrinks to `caddy` only — the reconciler is unaffected because it depends on `GatewayAdapter`, not `TraefikReverseProxy`.

---

## 5. Single reconciler — desired-state convergence

Traefik's `provider→router→middleware→service` watch (`pkg/provider/file/file.go:90,170-207`) and NPM's `configure→test→reload` (`backend/internal/nginx.js:27-117`) are the reference. Forge has been push-on-mutation (3 event handlers + manual `POST /traffic/sync` `handlers_trafficmanager.go:107`). The reconciler **inverts** this: mutations write desired rows; the reconciler diffs `DesiredSnapshot` vs `ReportedSnapshot` and single-writes.

### 5.1 Trigger model

```
Every mutation enumerates exactly one event (via existing eventstore `eventRegistry`):
  trafficmanager.CreateRoutingRule  →  events.EventGatewayDesiredChanged{RouterIDs:[…]}
  domains.CreateProxyDomain         →  same
  gateway_middlewares Upsert        →  same
  acme.Issue/RenewCustomCert        →  same
  HealthFilter.RecordSuccess/Failure→  same (soft trigger — only when threshold flips)

Reconciler subscribes to EventGatewayDesiredChanged + a 30 s fallback ticker (drift catchup).
Debounce: 500 ms coalesce (like Uncloud controller.go:148-200 fingerprint-cache).
```

No polling of legacy tables — the reconciler reads `gateway_*` **only**. Legacy dual-write rows are surfaced via the backfill jobs or explicit Sync once.

### 5.2 Data flow

```
Desired:
  SELECT gateway_routers + services + targets + router_middlewares + middlewares + certificates
  → collapse targets per service → attach middlewares per router in deterministic order
  → DesiredSnapshot{Hash: sha256(canonicalJSON)}
Reported:
  adapter.GetConfig() → normalized RouterSnapshot + CertSnapshot
  (or gateway_reported_state.config_json if adapter is currently unreachable)

Diff:
  fingerprint = desired.Hash vs reported.Hash — fast skip if equal.
  else deep diff: routers added/removed/modified, middlewares order changed,
                  service strategy changed, target set changed (deduped via UniqueBackends()),
                  cert SNI set changed.

Apply:
  adapter.DryRun(desired) — on error → emit metric + audit row, DO NOT apply, keep desired row pending.
  delta = diff(desired, reported)
  if delta.IsNoop() → update gateway_reported_state.reported_at, return nil
  adapter.ApplyDesired(desired) — on error → update gateway_reported_state.last_error, emit metric, do NOT advance desired; next tick retries.
  on success → upsert gateway_reported_state{config_hash:desired.Hash, config_json:reportedCfg, last_applied_at:now() }
```

### 5.3 Reconciler skeleton

```go
// forge/api/internal/services/gateway/reconciler.go

type Reconciler struct {
  store       GatewayStore
  adapter     GatewayAdapter
  resolver    *crossnode.Resolver    // for TargetHost resolution (server→node→PublicHostname/FQDN)
  health      *crossnode.HealthFilter
  publisher   events.Publisher
  mu          sync.Mutex     // serializes one reconcile at a time (not a global mu — only inside reconciler)
  debounce    time.Duration  // 500ms
  interval    time.Duration  // 30s fallback
  kind        AdapterKind
}

type GatewayStore interface {
  ListRouters(ctx context.Context) ([]GatewayRouter, error)
  ListMiddlewares(ctx context.Context) ([]GatewayMiddleware, error)
  ListServices(ctx context.Context) ([]GatewayService, error)
  ListTargets(ctx context.Context) ([]GatewayTarget, error)
  ListRouterMiddlewares(ctx context.Context) ([]RouterMiddleware, error)
  ListCertificates(ctx context.Context) ([]GatewayCertificate, error)
  GetReportedState(ctx context.Context, kind AdapterKind) (*ReportedState, error)
  UpsertReportedState(ctx context.Context, s ReportedState) error
}

func (r *Reconciler) Start(ctx context.Context) {
  // Coalescing channel — mirrors Uncloud controller watch stream
  trigger := make(chan struct{}, 1)
  notify := func() { select { case trigger <- struct{}{}: default: } }
  events.Subscribe(eventRegistry, "gateway.desired_changed", func(_ context.Context, _ events.Envelope) error { notify(); return nil })
  go r.loop(ctx, trigger)
}

func (r *Reconciler) loop(ctx context.Context, trigger <-chan struct{}) {
  ticker := time.NewTicker(r.interval)
  defer ticker.Stop()
  for {
    select {
    case <-ctx.Done(): return
    case <-trigger:    r.debouncedReconcile(ctx)
    case <-ticker.C:   r.Reconcile(ctx) // drift catchup
    }
  }
}

func (r *Reconciler) Reconcile(ctx context.Context) error {
  r.mu.Lock()
  defer r.mu.Unlock()

  desired, err := r.buildDesired(ctx) // reads gateway_* + health + resolver, dedupes & orders
  if err != nil { metrics.GatewayReconcileFail.Inc(); return fmt.Errorf("build desired: %w", err) }

  if err := r.adapter.DryRun(ctx, desired); err != nil {
    // DO NOT snapshot — preserve prior reported state for rollback ops.
    metrics.GatewayDryRunFail.Inc()
    _ = r.store.UpsertReportedState(ctx, ReportedState{GatewayKind: r.kind, LastError: err.Error()})
    return fmt.Errorf("dry-run: %w", err)
  }

  reported, err := r.loadReported(ctx) // from adapter or cached gateway_reported_state
  if err != nil { reported = nil }

  if reported != nil && reported.ConfigHash == desired.Hash {
    // Cheap skip — mirrors Uncloud controller.go:188-189 fingerprint-cache
    metrics.GatewayReconcileSkip.Inc()
    _ = r.store.UpsertReportedState(ctx, ReportedState{GatewayKind: r.kind, ReportedAt: time.Now(), ConfigHash: reported.ConfigHash})
    return nil
  }

  if err := r.adapter.ApplyDesired(ctx, desired); err != nil {
    metrics.GatewayApplyFail.Inc()
    _ = r.store.UpsertReportedState(ctx, ReportedState{GatewayKind: r.kind, LastError: err.Error()})
    // Restore is handled inside adapter.ApplyDesired sub-resource rollback; we do not replay reported here.
    return fmt.Errorf("apply: %w", err)
  }

  cfg, _ := r.adapter.GetConfig(ctx)
  _ = r.store.UpsertReportedState(ctx, ReportedState{
    GatewayKind:   r.kind,
    ConfigHash:    desired.Hash,
    ConfigJSON:    cfg,
    ReportedAt:    time.Now(),
    LastAppliedAt: time.Now(),
    LastError:     "",
  })
  metrics.GatewayReconcileSuccess.Inc()
  if r.publisher != nil {
    _ = r.publisher.Publish(ctx, events.NewEnvelope("gateway_reconciled","gateway","reconcile","",map[string]any{"hash":desired.Hash,"routers":len(desired.Routers)}))
  }
  return nil
}
```

```go
func (r *Reconciler) buildDesired(ctx context.Context) (DesiredSnapshot, error) {
  routers, _ := r.store.ListRouters(ctx)
  services, _ := r.store.ListServices(ctx)
  targets, _ := r.store.ListTargets(ctx)
  rws, _     := r.store.ListRouterMiddlewares(ctx)
  mws, _     := r.store.ListMiddlewares(ctx)
  certs, _   := r.store.ListCertificates(ctx)

  // Group targets by service_id; dedupe via crossnode health + UniqueBackends()-like logic.
  // Resolve host="" via crossnode.Resolver (server→node→PublicHostname/FQDN) — NOT hardcoded localhost:8080 (F-NET-15).
  // Drop down/degraded targets where IsHealthy==false (respect Threshold=3), but keep at least 0 (covers oscillation — builder checks health).
  // Ensure per-group WRR counter not shared: each service has its own strategy+weights slice (fix F-NET-13).

  // Attach middlewares per router with deterministic order:
  //   mwByID := map[id]GatewayMiddleware
  //   perRouter := group(rws by router_id) sorted by (mw.kind_order * 1000 + priority, mw.name)
  //   routerMiddlewares := ordered slice of middleware names for this router.
  // Legacy fallback: if GATEWAY_LEGACY_ATTACH_ALL==true and router has 0 attached, attach ALL enabled middlewares (with warning metric).

  // Validate: reject backticks/control chars in router.path/domain, reject weight on wrong strategy, etc. §7.
}
```

### 5.4 How this eliminates the five-writer race

- `trafficmanager.Service` loses `proxy` + `adapter` fields entirely after Phase C; its CRUD only writes `gateway_*`.
- `domains.Service` loses `caddyProxy` arg; its `syncCaddyRoutes` (`domains/service.go:455-489`) is deleted — the reconciler renders `gamepanel-domains` routers just like any other `gateway_routers` with `source='proxy_domains'`.
- `crossnode.IngressSynchronizer` is **deleted** (`crossnode/ingress_sync.go:1-300` + `health_filter.go` reaper + `routegroup.go` duplicate grouper) — its functionality (health-aware backend selection) moves into `gateway.Reconciler.buildDesired` via a single shared `HealthFilter`. No empty-sync path remains.
- `loadbalancer.Service` direct listeners remain (they're not Caddy data-plane), but their **health** story is unified — they read the same `HealthFilter` threshold and never bypass gateway TLS.
- `caddy_tls.go:18-280` `CaddyTLSManager` is **deleted** after wiring cert delivery via `gateway_certificates` (or kept as a deprecated shim that delegates to `gateway.Store`). The decision is removal per §6.

### 5.5 Single `GroupRules`-like collapse

Only **one** grouping function survives, inside `gateway/build_desired.go` (replacing `caddy_proxy.go:894-922`, `traefik_proxy.go:725-753`, `crossnode/routegroup.go:32-68`):

```go
// gateway/grouping.go — single owner.
func canonicalRouterKey(r GatewayRouter) RouterKey {
  path := r.Path; if path == "" { path = "/" }
  proto := r.Protocol; if proto == "" { proto = "http" }
  return RouterKey{Domain: strings.ToLower(r.Domain), Path: path, Protocol: proto}
}
```

---

## 6. True dry-run & safe apply

### 6.1 Snapshot BEFORE, never snapshot-after-mutation

**Bug today (`F-NET-03`):** `caddy_proxy.go:687-702` `updateRoutesAtomic` does `validateConfig` (which itself **applies** via `POST /load :980-1002`) then snapshots — storing the *new* config as `lastValidConfig`, so `restoreConfig:697-702` / `Rollback:279-293` replays bad gen. TLS path `caddy_tls.go:240-251` skips validation entirely (JSON parse only).

**Fix:** `Reconciler.Reconcile` step order (above) — `DryRun` is **before** any mutation and is **non-mutating**. Snapshot is written **only after** a successful `ApplyDesired`.

### 6.2 What "true dry-run" means per gateway

| Gateway | Current (broken) | Correct |
|---|---|---|
| **Caddy** | `validateConfig` `caddy_proxy.go:980-1002` POSTs to `POST /load` with body + `Cache-Control: must-revalidate` — this **is** `Load` (`reference/networking/caddy/caddyconfig/load.go:68-116` — `Load(body, forceReload)` replaces config) | `POST /adapt` is the contract (`load.go:137-175` — adapt Caddyfile→JSON returning `{result,warnings}` without replacing running config). The HTTP equivalent is `POST /config/` sub-resource **GET + in-memory validation**, not `/load`. Next best: `caddy adapt --pretty --validate --adapter caddyfile` style. |
| **Caddy practical** | (TLS path `caddy_tls.go:240-251` skips) | Use **(a)** `POST http://admin:2019/config/` sub-resource **GET** then local JSON schema + `caddy adapt` byte-level validation (call the Caddy CLI or library `caddyconfig/adapt.go:137` via `os/exec`), or **(b)** the dedicated `/adapt` endpoint if the admin has it enabled. The existing `UpdateDomainRoutes` pattern (`caddy_proxy.go:523-593` `getRunningConfig → json.Unmarshal → merge → json.Marshal → validate → snapshot → apply`) is the template — but `validate` there is still `POST /load`. Replace that validate with a non-mutating check. |
| **Traefik** | `traefik_proxy.go:974-1005` `validateYAMLConfig` YAML round-trip only (`yaml.Marshal`+`Unmarshal`); `reloadTraefik:1065-1084` `POST /api/refresh` (API is GET-only `pkg/api/handler.go:103-133`; file provider watches `provider/file/file.go:90,170-207` `fsnotify`) | File provider self-heats — no reload POST at all. **Write + fsnotify** is the apply. Dry-run = structural YAML validation + Traefik config `Provider` validation (run `traefik config check` or `yaml.Validator` for the dynamic config) — no daemon hit. |

### 6.3 Apply without wiping — sub-resource merges (Caddy)

`UpdateDomainRoutes:523-593` already proves the safe pattern — it `GET /config/` → `json.Unmarshal` → `servers["gamepanel-domains"]=routes` → `POST /config/`. `updateRoutesAtomic:673-720` ignores it and POSTs `{apps:{http:{servers:{gamepanel:…}}}}` — destroys the sibling.

**Phase A+ immediate fix (before reconciler exists):** both paths switch to one shared helper:

```go
// trafficmanager/caddy_proxy.go — stop the bleeding (Phase A)
func (p *CaddyReverseProxy) safeApplyServers(ctx context.Context, updates map[string]any) error {
  // updates: e.g. {"gamepanel": <server>, "gamepanel-domains": <server>}  OR  only what changed
  raw, err := p.getRunningConfig(ctx, p.adminAddr)
  if err != nil { raw = json.RawMessage(`{}`) }
  var cfg map[string]any
  _ = json.Unmarshal(raw, &cfg)
  // ensure cfg.apps.http.servers map exists
  m := ensureServersMap(cfg)
  for k, v := range updates { m[k] = v }
  return p.applyViaSubResource(ctx, updates) // see below
}

// Preferred API: targeted sub-resource — never whole-doc unless bootstrapping empty Caddy.
func (p *CaddyReverseProxy) applyViaSubResource(ctx context.Context, servers map[string]any) error {
  for name, srv := range servers {
    body, _ := json.Marshal(srv)
    req, _ := newCaddyAdminRequest(ctx, "POST", p.adminAddr, "/config/apps/http/servers/"+name, bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    resp, err := p.client.Do(req); if err != nil { return err }
    // handle 3xx properly — Caddy admin returns 201/200 on success with Location on /id/<id>
    if resp.StatusCode >= 300 { return statusError(resp) }
  }
  return nil
}
```

**With reconciler (Phase C):** the adapter owns the full delta — it `GET /config/` once, computes `apps.http.servers` desired map (includes both `gamepanel` and `gamepanel-domains` plus future `gamepanel-tcp` etc.), then merges only the servers that changed via the same sub-resource calls. The reconciler **never** `POST /config/` whole document except when bootstrapping a brand-new empty Caddy (first run, `gateway_reported_state` empty).

Rollback: the prior `gateway_reported_state.config_json` is the source for `RestoreConfig`; the in-memory `p.lastValidConfig` (`caddy_proxy.go:24`) is retained only as a short-TTL LRU inside the adapter for crash-before-persist safety, not the reconciler's truth.

### 6.4 Traefik apply (when `GATEWAY_ADAPTER=traefik`)

- `DryRun` = `yaml.Marshal(cfg) → yaml.Unmarshal` + per-router/per-service validation (same `validateYAMLConfig :974-1005`) + optional `traefik check` via filesystem (no network).
- `ApplyDesired` = atomic file replace: `routes.yml.tmp → fsync → rename routes.yml` + `routes.backup.yml` kept (like `writeConfig :1045-1063` already). No `POST /api/refresh`. Watcher picks it up. If `certFile/keyFile` are needed, materialize PEM files first and reference **paths**.

---

## 7. Deterministic middleware order, group-aware withdrawal, protocol branch

### 7.1 Deterministic middleware order (fixes F-NET-04 + F-NET-05)

**Global kind order (enforced at attach time + sort-stable):**

| Priority band | `gateway_middleware_kind` | `kind_order` | Notes |
|---|---|---|---|
| 10 | `rate_limit` | 10 | Caddy uses real `request_rate` or `http.handlers.rate_limit` **only if** Caddy `http.handlers.rate_limit` module is present; otherwise map to a local `error` diagnostic — never emit fictional `rate_limit` handler |
| 20 | `ip_denylist` | 20 | `subroute { remote_ip ranges → 403 }` (like current `caddy_proxy.go:806-828` — correct) |
| 30 | `ip_allowlist` | 30 | `subroute { not remote_ip ranges → 403 }` (`caddy_proxy.go:830-856` — correct) |
| 40 | `headers` | 40 | `request_header` / `response_header` subroute |
| 50 | `redirect` | 50 | `static_response { headers Location 301 }` or Traefik `redirectScheme` |
| 60 | `circuit_breaker` | 60 | **Gated** — see F-NET-04 fix |
| 70 | `compress` / `retry` | 70+ | Future |

**Rule:** `gateway_router_middlewares.priority` resolves **ties** inside the same `kind_order` band. The conflated sort key `(mw.kind_order * 1000 + rm.priority)` is canonical and stable; if equal, tiebreak on `mw.name` alphabetically.

**F-NET-04 gate (fictive handlers):**

```go
// gateway/render_caddy.go
var fictiveGuard = os.Getenv("GATEWAY_ENABLE_FICTIVE_HANDLERS") == "true"

func renderCaddyMiddlewares(rtr GatewayRouter, chain []GatewayMiddleware) ([]map[string]any, error) {
  var out []map[string]any
  for _, mw := range chain {
    if !mw.Enabled { continue }
    switch mw.Kind {
    case "rate_limit":
      if !fictiveGuard && !caddyHasModule("http.handlers.rate_limit") {
        // Do NOT emit the fictive handler. Emit a diagnostic route that returns 503 with a message
        // OR silently skip if the policy is optional.
        // Decision: skip + metric gateway_fictive_handler_skipped_total{mw_id}++
        metrics.GatewayFictiveSkipped.Inc()
        continue
      }
      out = append(out, renderRateLimit(mw))
    case "circuit_breaker":
      if !fictiveGuard && !caddyHasModule("http.handlers.circuit_breaker") {
        metrics.GatewayFictiveSkipped.Inc()
        continue // CB is reverse_proxy-level (`reverseproxy.go:109`) — cannot be a top-level handler
      }
      // When real: wrap inside reverse_proxy.handle_response or pass as lb-level config, not a sibling handler
    default:
      out = append(out, renderMw(mw))
    }
  }
  return out, nil
}
```

Tests that assert fictional rendering (`caddy_proxy_test.go:280-314` `rate_limit`, `:547-615` `circuit_breaker`) are **updated** to assert skipped/real rendering, not the broken string.

### 7.2 Group-aware withdrawal (fixes F-NET-06 + F-NET-02 sibling)

**Root cause:** `caddy_proxy.go:60-74` `RemoveRoutes` DELETEs `/id/gamepanel-<id>`; but `buildGroupedRoute:949-951` creates `@id=gamepanel-<firstRuleID>-group`. A failure affecting a non-first rule can't remove the grouped route. `CleanupStale:350-362` handles `-group` suffix (`:355-357`) so the periodic cleanup eventually catches it, but hot withdraw doesn't.

**Fix — two layers:**

1. **Immediate (Phase A):** `RemoveRoutes` strips `-group` and handles both singular and grouped IDs. Pathological `strings.Contains :457` (which matches sibling IDs `abc` hits `abc-v2`) is replaced with exact `==` or `HasPrefix("gamepanel-"+id+"-group") || id=="gamepanel-"+id`.

```go
// caddy_proxy.go — hotfix, released Phase A
func (p *CaddyReverseProxy) RemoveRoutes(ctx context.Context, ruleIDs []string) error {
  for _, id := range ruleIDs {
    for _, cand := range []string{"gamepanel-" + id, "gamepanel-" + id + "-group"} {
      req, _ := newCaddyAdminRequest(ctx, "DELETE", p.adminAddr, "/id/"+cand, nil)
      resp, err := p.client.Do(req)
      if resp != nil && resp.StatusCode == 404 { continue } // grouped vs singular - one is expected 404
      // …
    }
  }
  // Also attempt grouped-route surgical upstream removal when group has >1 target —
  // drop just the dead target instead of whole route when the reconciler is not yet live.
}
```

2. **With reconciler (Phase C):** Withdrawal no longer calls Caddy at all. The reconciler's `buildDesired` is **group-aware**: it loads `gateway_routers.id` → `gateway_services.id` → `gateway_targets` list, filters via `HealthFilter.FilterHealthy` (only when threshold met) and the node-lookup `ListServersForNode` result, then renders the remaining backends. A grouped route that loses one backend is re-rendered as the same `@id` with fewer `upstreams` — no delete/create race.

**Also fix `SetUpstreamHealth` append-bug (`caddy_proxy.go:503-512`, `traefik_proxy.go:652-655`):**

```go
// Before (broken): if healthy && !modified { newUpstreams = append(newUpstreams, map{"dial": targetDial}); modified = true }
//  !modified means "already absent" was not detected, so it always appends — pool grows per flap.

// After:
func (p *CaddyReverseProxy) modifyUpstreamsInRoute(route map[string]any, targetDial string, healthy bool) bool {
  // ...
  alreadyPresent := false
  var kept []any
  for _, uRaw := range upstreamsRaw {
    u, _ := uRaw.(map[string]any)
    if u["dial"] == targetDial {
      alreadyPresent = true
      if !healthy { modified = true; continue } // drop unhealthy
    }
    kept = append(kept, u)
  }
  if healthy && !alreadyPresent {
    kept = append(kept, map[string]any{"dial": targetDial})
    modified = true
  }
  // …
}
```

Weight preservation: re-add uses the stored `gateway_targets.weight` for that dial, not `{"dial":…}` bare.

### 7.3 Protocol branch or reject (fixes F-NET-07)

| Protocol | Caddy | Traefik | Allowed? |
|---|---|---|---|
| `http` / `https` / `""` → normalized `http` | `match: {host, path}` + `reverse_proxy` | `Host(\`…\`) && PathPrefix(\`…\`)` | **Yes** |
| `tcp` | **Reject** until `layer4` app is explicitly wired; OR if `GATEWAY_ENABLE_L4=true` emit `apps.layer4.servers` with `tls` or `tcp` handlers | `TCPRouter HostSNI` only for TLS SNI; for plain TCP use `ClientIP`/`CatchAll`; dedicated `tcp:`/`udp:` config section | **Gated** |
| `udp` | Same as `tcp` | Same | **Gated** (admission currently rejects `udp`, keep rejection) |

**Phase A:** `service.go:383-387` `validateRoutingRule` should **reject** `"tcp"` unless `GATEWAY_ENABLE_L4=true` (keep current `udp` reject). Add protocol branch guard in `buildGroupedRoute:924-978` returning `error` rather than mis-rendering.

**Phase C:** When L4 is wanted, the reconciler owns it:

```go
// gateway/render_*.go — gated by env flag until infra for layer4/file UDP is ready
if router.Protocol == "tcp" {
  if os.Getenv("GATEWAY_ENABLE_L4") != "true" {
    return nil, fmt.Errorf("tcp router %q requires GATEWAY_ENABLE_L4=true", router.ID)
  }
  return renderCaddyLayer4(router, svc, targets), nil
}
if router.Protocol == "udp" { /* same, but Traefik udp: section */ }
```

Until then, `loadbalancer/dataplane.go:20-49` remains the **canonical L4 surface** (`phase-04/synthesis §18` recommendation).

### 7.4 Weighting (P1 hardening)

- Caddy **does not** support per-upstream `weight` (`caddy_proxy.go:934-939` silently discarded — `reference/networking/caddy/modules/caddyhttp/reverseproxy/selectionpolicies.go:41-98` `Weights []int` belongs on the **policy**, not upstream). When any `gateway_targets.weight != 1`, the Caddy render emits `{"handler":"reverse_proxy","load_balancing":{"selection_policy":{"policy":"weighted_round_robin","weights":[…]}}}` with an **ordered** `Weights` array matching `upstreams` order.
- Traefik per-server `Server.Weight` (`traefik/http_config.go:62-69`) is correct — no change.
- LB `roundRobin["__weighted"]` shared counter `loadbalancer/service.go:480-482` → per-service counter `roundRobin[serviceID]`.

---

## 8. Beacon / data-plane changes

**Summary: near-zero Beacon changes required by gateway inversion** — the inversion fixes the *panel* side. Beacon's dataplane (`beacon/…/runtime/docker.go:949-1101` WireGuard + beacon workload routing) is largely unaffected.

| Area | Change | File/crate | Effort |
|---|---|---|---|
| **ACME HTTP-01 solver mount** (F-NET-09) | Mount `acme.Service.HTTPSolver()` `acme/service.go:157-159` on `:80` or proxy `/.well-known/acme-challenge/*` onto Caddy. Prior: solver existed but never exposed (zero `ServeHTTP` mount). Fix must `strings.TrimSpace` host port (`net.SplitHostPort` or `strings.Cut`) before key lookup — `acme/service.go:81` currently uses raw `r.Host`. | `forge/api/internal/http/server.go` + `acme/service.go:75-92` | Small |
| **Caddy restart survival** | Today, Caddy is in-memory if run as a sidecar; on restart desired state is re-synced via `domainSvc.SyncCaddyRoutes` startup `main.go:1030-1038`. With reconciler, the reconciler's `interval` ticker + `gateway_reported_state` drive re-sync — ensure `Rec.Start` happens even if Caddy is temporarily down (adapter `Health` returns degraded but desired stays queued). | `gateway/reconciler.go` + `cmd/api/main.go:1030` | Config |
| **No Beacon changes** for TLS delivery, grouping, policies — those are panel-only. If Beacon runs a per-node Caddy (e.g., for `*:443` NodePort), ensure its `adminAddr` is distinct from panel Caddy's `CADDY_ADMIN_ADDR` (`:2019` hardcode `caddy_proxy.go:57-59` should become env-var). | `forge/api/internal/services/trafficmanager/caddy_admin.go:24-49` | Docs |

Cert delivery after `acme/service.go:322-366` `RenewCertificate` / `IssueCertificate` + custom upload now calls:

```go
// acme/service.go — after store.CreateCertificate / UpdateCertificate
if gatewayReconciler != nil {
  // Non-blocking: write to gateway_certificates, publisher will notify reconciler.
  // No direct adapter.SetCertificate call — reconciler does it from DB diff (single writer invariant).
  _ = gatewayStore.UpsertCertificate(ctx, GatewayCertificate{ SNI: domain, CertPEM: certPem, KeyPEM: keyPem, SourceCertID: certID })
  _ = publisher.Publish(ctx, events.NewEnvelope("gateway_desired_changed","acme","cert",certID,nil))
}
```

The `SetCertificate` adapter impls (`caddy_proxy.go:83-121` + `traefik_proxy.go:373-397`) are **no longer called by acme directly** — only by `GatewayReconciler.ApplyDesired` after diffing `gateway_certificates` vs `GET /tls/certificates` (or Traefik files). The legacy background `CaddyTLSManager ProvisionLetsEncrypt :34-56` path is removed (or guarded behind `GATEWAY_LEGACY_CADDY_TLS=true` for one release if any customer relied on embedded Caddy LE).

---

## 9. Frontend — ONE Gateways page

**Problem:** 7 scattered broken pages all fail differently, none matches backend:

| Current page | File | Status | Why not ONE page |
|---|---|---|---|
| `Traffic` (routes+policies tabs) | `forge/web/app/admin/traffic/page.tsx:14-22,67-70,84,94` | BROKEN — posts `{path,targetGroup,priority,methods}` vs `RoutingRule{domain,path,targetPort,…}`, `GET /policies` 405, `PATCH` vs `PUT` 405, `config: {}` mismatch | Core host/path editor but wrong schema |
| `Domains` | `forge/web/app/admin/domains/page.tsx:127-129` | BROKEN — Verify always `verified:true 199-212`, second Caddy writer | Duplicate router surface |
| `Load Balancer` | `forge/web/app/admin/load-balancer/page.tsx:142-146,158-159` → `handlers_loadbalancer.go:88-100` arbitrary IP, `handlers_loadbalancer.go:109-111` generic `network/gateway` | PARTIAL — correct L4 concepts but wrong gateway | Standalone L4 |
| `Certificates` | `forge/web/app/admin/certificates/page.tsx:43-50,76-79` `POST /certificates` 405 (`handlers_certificates.go:15-77` no `POST /`; working is `POST /custom-certificates`) | BROKEN | Separate |
| `Firewall` | `forge/web/app/admin/firewall/page.tsx` | MISSING — no persistence, no validation (`handlers_firewall.go:78-193`) | Separate |
| `Security` (`AdminSecurity.tsx:114-133`) | `forge/web/app/admin/security/*` hardcoded pills | DECORATIVE | Fragment |
| `Endpoints` | `forge/web/app/admin/endpoints/*` | PARTIAL — `servicediscovery` complete API, zero UI wants | Mesh |

### 9.1 Target IA — ` /admin/gateways ` (replaces all 7)

```
Sidebar:  Gateways  (icon: Globe/Route)  ← replaces Traffic/Domains/LB/Certs/Security/NetworkPolicy in nav
          (Firewall/SecurityHeader stay under Settings until persisted — Phase D)

Gateways page — one resource, four tabs (NPM "one-page host editor" + Traefik tree):

  /admin/gateways
    ├── Routers      (table: Domain | Path | Proto | Priority | Service | Middleware chain | Status)
    │     │         ─ domain+path match + entrypoints badge (web/websecure/tcp/udp) + health pills
    │     ├── NewRouterModal  (domain idle+punycode + path url.ParseRequestURI + proto enum + TLS toggle + weight)
    │     ├── RouterDrawer    (backends, attached middlewares ordered drag-drop, TLS cert SNI link)
    │     └── RouteDiffChip   (desired vs reported hash mismatch banner — red when drift)
    │
    ├── Services     (table: Service | Router | Strategy | Targets N | Health)
    │     └── ServiceDrawer (strategy dropdown + target list with weight + per-target health pill)
    │
    ├── Middlewares  (table: Name | Kind | Order | Enabled | Attached-to count)
    │     └── MiddlewareForm (kind-specific typed editors — NOT generic JSON {} )
    │           rate_limit {average,burst}  ip_allow/deny {CIDR textarea validated}
    │           headers {k:v editors}  redirect {scheme/regex}  circuit_breaker {expression}
    │
    └── Certificates (table: SNI | Issuer | NotAfter | AutoRenew | Gateway status)
          └── CertDrawer (PEM preview — redacted key, delivery status: pending|delivered|failed)
```

**Design system cues (from current admin pages):** reuse `AdminPageLayout`, `Card/CardHeader`, `AdminTabs`, `Btn`, `Pill`, `EmptyState`, `Modal`, `AdminLoadingState`, `Input` (`traffic/page.tsx:10`) — matching existing chrome. New components: `RouteMatchBadge`, `MiddlewareChainStack` (ordered chips), `GatewayDriftBanner`, `HealthPill`.

**Routing:** `forge/web/app/admin/gateways/page.tsx` (new). Keep `forge/web/app/admin/traffic/page.tsx` as a thin shim that redirects to `/admin/gateways` for 2 releases (or renders a deprecation banner) — avoids breaking bookmarks.

### 9.2 API surface — one resource, typed

```ts
// web/lib/api/gateways.ts — single client, replaces 5 lib/api/* clients
// All responses are envelope-typed (via /admin/gateways prefix).

GET    /admin/gateways/routers            // list GatewayRouter + service summary + middleware names
POST   /admin/gateways/routers            // {domain, path, protocol, priority, tlsEnabled, entrypoints, metadata}
GET    /admin/gateways/routers/:id        // + targets + ordered middlewares
PATCH  /admin/gateways/routers/:id        // partial update (domain/path/protocol locked after create — delete+recreate pattern)
DELETE /admin/gateways/routers/:id

POST   /admin/gateways/routers/:id/middlewares   // {middlewareId, priority} — attach
DELETE /admin/gateways/routers/:id/middlewares/:mwId
PUT    /admin/gateways/routers/:id/middlewares/reorder // [mwId ordered]

GET    /admin/gateways/middlewares
POST   /admin/gateways/middlewares        // {name, kind, kind_order, config: typed}
GET    /admin/gateways/middlewares/:id
PATCH  /admin/gateways/middlewares/:id
DELETE /admin/gateways/middlewares/:id

GET    /admin/gateways/services           // + targets health rollup
GET    /admin/gateways/services/:id
PATCH  /admin/gateways/services/:id       // {strategy, sticky, healthCheck}

GET    /admin/gateways/certificates
POST   /admin/gateways/certificates       // custom upload (replaces handlers_proxy_domains.go:227 /custom-certificates)
POST   /admin/gateways/certificates/:sni/reissue  // ACME DNS-01 via acme service
DELETE /admin/gateways/certificates/:sni

GET    /admin/gateways/reconcile/status   // {desiredHash, reportedHash, drift: bool, lastError, lastAppliedAt}
POST   /admin/gateways/reconcile/trigger  // manual tick (admin-only, mirrors Uncloud "force sync")
```

Keep `GET /admin/traffic/rules` + `POST /traffic/sync:107` as compat shims that delegate to `gateway.Store.ListRouters` + `publisher.Publish("gateway_desired_changed")` during Phase B (marked `@deprecated` header).

### 9.3 What is NOT here (per task brief)

- **Operations timeline** — **NOT** the Gateways page. That detail lives in a separate panel (ops/reconciler plan). Gateways shows only `gateway_reported_state` (hash/drift/last_error) + a link to the audit log. No `operation.service` coupling.

---

## 10. Rollout Phases A–D with feature flags

All phases behind env flags so a single canary env can trial before global rollout. Flags are additive — Phase D only flips defaults; no flag is ever the sole protection for a destructive path after stabilization.

### 10.1 Flag inventory

| Flag | Default A | Default B | Default C | Default D | What it guards |
|---|---|---|---|---|---|
| `GATEWAY_SINGLE_WRITER` | `false` | `false` | `true` | `true` (flag removed) | When true, reconciler is the only Caddy writer; legacy writers are deleted/unreachable |
| `GATEWAY_DUAL_WRITE` | `false` | `true` | `false` | removed | Phase B best-effort mirror from traffic/domains CRUD → `gateway_*` |
| `GATEWAY_SUBRESOURCE_MERGE` | `true` (A hotfix) | `true` | `true` | removed | Phase A hotfix: adapters use sub-resource merges, not `POST /config/` whole-doc |
| `GATEWAY_TRUE_DRYRUN` | `true` (A hotfix) | `true` | `true` | removed | Snapshot-before + `/adapt` or sub-resource validate, not `POST /load` |
| `GATEWAY_ENABLE_FICTIVE_HANDLERS` | `false` | `false` | `false` | removed or `true` only when module present | F-NET-04 gate |
| `GATEWAY_LEGACY_ATTACH_ALL` | `true` (compat) | `true` | `false` | removed | F-NET-05 compat — if true, empty `gateway_router_middlewares` falls back to all middlewares on every router (with warning) |
| `GATEWAY_ENABLE_L4` | `false` | `false` | `false` | gated | F-NET-07 — allow `tcp`/`udp` router rendering |
| `GATEWAY_ADAPTER` | `caddy` | `caddy` | `caddy` | `caddy` (or `traefik` for that fleet) | Adapter kind — decides which adapter is constructed at `main.go:982` |
| `GATEWAY_HEALTH_RECONCILER` | `false` | `true` | `true` | removed | Embed `HealthFilter` threshold respected at build-time |

### 10.2 Phase A — Stop the bleeding (1–3 days, no migration, no release gate beyond canary)

Goal: make the current cluster liveable without schema changes or new UI.

| Change | File:line | Implementation | Flag | How verified |
|---|---|---|---|---|
| **A1. Empty-sync guard** | `crossnode/ingress_sync.go:111-166` | Add early return `if len(is.rules)==0 && len(is.policies)==0 { slog.Debug("ingress sync skip — empty desired"); return nil }`. Also add the same guard for `healthyBackends` length already warns but still calls UpdateRoutes with nil. | Always on (no flag — safe) | Unit: call `Sync` with empty map → `UpdateRoutes` never called (mock adapter). E2E: check Caddy `gamepanel-domains` route survives 3 ticker cycles. |
| **A2. Sub-resource merge** | `caddy_proxy.go:673-720` + `caddy_tls.go:186-238` | Replace `buildServerConfig → POST /config/` with `safeApplyServers` pattern from §6.3 (both `updateRoutesAtomic` and `buildTLSConfig`). Both servers are rendered into a single `GET → merge → per-server POST` call. | `GATEWAY_SUBRESOURCE_MERGE=true` default; flag off reverts to old whole-doc (for rollback). | Manual: `GET /config/` before/after traffic sync — `gamepanel-domains` survives. `caddy_proxy_test.go:15-116` updated to assert `/config/apps/http/servers/<name>` called. |
| **A3. Snapshot-before + use non-mutating validate** | `caddy_proxy.go:673-705,980-1002` + `caddy_tls.go:240-251` | Snapshot `previousConfig=getRunningConfig` **before** `validateConfig`. Replace `validateConfig` body: try `POST /adapt` first; if Caddy returns 404/405 (admin doesn't expose it), fall back to local JSON schema validation (not an apply). Never call `/load` for validation again. TLS path gets same validator. Persist `lastValidConfig` to `gateway_reported_state` where possible; in-memory fallback still written. | `GATEWAY_TRUE_DRYRUN=true` | Test `TestAtomicReload_Success:15` updated to expect NO `/load` call (or 1 `/adapt` call); on validation error `POST /config/` is never called. |
| **A4. Fictive handler gate** | `caddy_proxy.go:795-875` | Guard `rate_limit`/`circuit_breaker` emission behind `GATEWAY_ENABLE_FICTIVE_HANDLERS != true` → skip + metric `gateway_fictive_handler_skipped_total`. Update tests `caddy_proxy_test.go:280-314,547-615` to assert skipped rendering. | `GATEWAY_ENABLE_FICTIVE_HANDLERS=false` | Enable a `TrafficPolicy{RateLimit:100}` then `UpdateRoutes` → no `rate_limit` key in serialized config; route update succeeds (currently fails). |
| **A5. Group-aware `RemoveRoutes`** | `caddy_proxy.go:51-74,457,949-951` | Handle both `gamepanel-<id>` and `gamepanel-<id>-group`; exact match; fix `Contains` → `==`/`HasPrefix`. | Always | Unit: create `gamepanel-abc-group` grouped route, call `RemoveRoutes(["abc"])` → `DELETE /id/gamepanel-abc-group` issued. |
| **A6. Health append-bug** | `caddy_proxy.go:503-512` | Check `alreadyPresent` before appending (see §7.2). Preserve weight from original entry on re-add. | Always | Unit: flap `SetUpstreamHealth(false)→true` 5 times → upstream count stays 1, not 5. |
| **A7. Protocol reject** | `service.go:383-387` | Reject `tcp` unless flag; never misrender. | `GATEWAY_ENABLE_L4=false` | Unit: `CreateRoutingRule{Protocol:"tcp"}` → error unless flag true. |
| **A8. Re-wire ACME solver mount + cert delivery** | `acme/service.go:75-92,157-159` + `cmd/api/main.go:981-995` | Mount `acmeSvc.HTTPSolver()` as `http.HandlerFunc` on `/.well-known/acme-challenge/*` (strip port from `r.Host` before key lookup — fix `r.Host` bug). Wire interim cert delivery: post-issue/renew also call `caddyProxy.SetCertificate` (until reconciler exists). | `GATEWAY_HEALTH_RECONCILER=false` (wire only) | E2E: `IssueCertificate{ChallengeType: http-01, Provider: letsencrypt-staging}` with Pebble → issuance succeeds. `openssl s_client -connect <sni>:443` serves the issued leaf. |
| **A9. Wire `TraefikProxy` out or leave — no change in A** | `traefik_proxy.go:380-385,1065-1084` | Minimal — gate writes so tests don't regress. No prod wiring change. | `GATEWAY_ADAPTER=caddy` | No-op |

**Ship criteria for A → B:** canary env runs 24 h with all 9 hotfixes; metrics `gateway_fictive_handler_skipped_total` and `caddy_subresource_merge_total` incrementing; no `gamepanel-domains` wipe in `gateway_reported_state.last_error == ""` drift logs. No migration yet.

### 10.3 Phase B — Schema migration (1 week, additive, dual-read)

Goal: introduce `gateway_*` tables alongside legacy; no destructive reads.

| Change | File | Implementation | Flag |
|---|---|---|---|
| **B1. Run migrations `050-054`** | `forge/api/migrations/050_gateway_core.sql` … | Additive DDL from §3. Backfill `052` runs automatically once (idempotent `ON CONFLICT DO NOTHING`). | Always |
| **B2. Introduce `gateway` package** | `forge/api/internal/services/gateway/{adapter,store,reconciler,render_caddy,render_traefik,grouping}.go` | Ports adapter logic from `trafficmanager/caddy_proxy.go:514-1079` + `traefik_proxy.go:1-1139` into a testable package with the new `GatewayAdapter` from §4. | No flag — code exists but is not wired as writer yet |
| **B3. Dual-write from legacy CRUD** | `trafficmanager/service.go:291-359`, `domains/service.go:74-76`, `store_traffic.go:11-263`, `store_routing.go:21-237`, `store_proxy_domains.go` | On each `CreateRoutingRule/UpdateRoutingRule/DeleteRoutingRule` and `CreateProxyDomain/DeleteProxyDomain`, also upsert `gateway_routers/services/targets` when `GATEWAY_DUAL_WRITE=true`. Errors from new path are `slog.Warn` — legacy remains truth. | `GATEWAY_DUAL_WRITE=true` (Phase B only) |
| **B4. Dual-read helper** | `gateway/reconciler.go:buildDesired` fallback path | When `gateway_routers` has rows for a domain, use them; else legacy fallback with metric `gateway_legacy_fallback_total`. | Always (read path) |
| **B5. Introduce `gateway_reported_state`** | `store/store_gateway.go` (new) | `GetReportedState/UpsertReportedState` for the reconciler's `config_hash` + audit. | Always |

**Ship criteria for B → C:** staging env's `gateway_*` row counts converge with legacy counts; manual spot-check: a new traffic rule created via legacy `POST /admin/traffic/rules` appears in both `traffic_rules` and `gateway_routers` within 1 s (dual-write). No customer-visible change yet.

### 10.4 Phase C — One reconciler (the inversion, 1–2 weeks)

Goal: flip the single writer on; delete 5-writer paths; expose ONE Gateways API + ONE page.

| Change | File | Implementation | Flag |
|---|---|---|---|
| **C1. Wire `GatewayReconciler` as the only writer** | `cmd/api/main.go:982,993-1028` | Replace `caddyProxy := trafficmanager.NewCaddyReverseProxy` + `crossnode.NewIngressSynchronizer` + `domains.New` caddy args with: <br> `gatewayStore := store.NewGatewayStore(db.GetDB())` <br> `gatewayAdapter := gateway.NewCaddyAdapter(env("CADDY_ADMIN_ADDR"), …)` (or Traefik) <br> `reconciler := gateway.NewReconciler(gatewayStore, gatewayAdapter, crossNodeResolver, healthFilter)` <br> `reconciler.Start(appCtx)` <br> Delete `ingressSync.Start`, delete `domainSvc.SetNodeResolver` caddy arg, delete `loadbalancer` direct Caddy touches. | `GATEWAY_SINGLE_WRITER=true` default; flag `false` reverts to single-legacy whole-doc behavior (emergency rollback). |
| **C2. Make services Gateway-API consumers** | `trafficmanager/service.go`, `domains/service.go`, `acme/service.go` | Each service's mutation now does: <br> (1) write legacy row if still in compat window (no dual flag anymore — legacy writes become best-effort until D), <br> (2) upsert `gateway_*` rows, <br> (3) `publisher.Publish("gateway_desired_changed")`. No `adapter`/`proxy` field remains on `tmSvc`. | Depends on `GATEWAY_SINGLE_WRITER` |
| **C3. Build desired now owns health + grouping + dedup + ordering** | `gateway/build_desired.go` + `gateway/grouping.go` | Single grouper (see §5.3). Reads `HealthFilter.FilterHealthy` threshold=3 (`health_filter.go:42-56`) at render time. Re-add path preserves weight. Per-router middleware chain deterministic `(kind_order*1000+priority, name)`. | Always when single writer |
| **C4. True dry-run + sub-resource apply** | `gateway/caddy_adapter.go:DryRun+ApplyDesired` | See §6. Persisted `lastValidConfig` → `gateway_reported_state`. Whole-doc `POST /config/` only for cold bootstrap. | Always when single writer |
| **C5. Cert delivery reconciler** | `gateway/reconciler.go:buildDesired` cert section + `gateway/caddy_adapter.go:SetCertificate` | `buildDesired` emits `DesiredSnapshot.Certificates` from `gateway_certificates`; `ApplyDesired` diffs vs `GET /tls/certificates/<sni>` presence and writes missing. `acme/service.go:322-366` post-renew only upserts `gateway_certificates` (no direct `SetCertificate` call — single writer). | Always when single writer |
| **C6. Mount `HTTPSolver`** | `internal/http/server.go` | `acme.HTTPSolver` on `:80` or proxied `/.well-known/acme-challenge/*` route. Strip port from `r.Host`. | Always when single writer |
| **C7. New Gateway API** | `internal/http/handlers_gateway.go` (new) + `server.go` route registration | Register `GET/POST/PATCH/DELETE /admin/gateways/routers/...` + `.../middlewares` + `.../certificates` + `.../reconcile/status+trigger` from §9.2. | Always when single writer |
| **C8. ONE page** | `forge/web/app/admin/gateways/page.tsx` | New page per §9.1 (Routers/Services/Middlewares/Certs tabs) + admin nav change (`forge/web/components/admin/navigation.tsx` — collapse 7 → 1). Keep `traffic/page.tsx:1-350` as redirect shim. | Always when single writer |
| **C9. Delete legacy hot writers** | `crossnode/ingress_sync.go:1-300`, `crossnode/routegroup.go:32-151` duplicate grouper, `caddy_tls.go:18-280` `CaddyTLSManager` (if not kept as shim) | Deleted or behind build tag. `crossnode.HealthFilter` + `crossnode.Resolver` are retained (health engine). | Always when single writer |
| **C10. Observability** | `infra/prometheus/alerts.yml` + `services/observability/service.go` | Add `gateway_reconcile_failures_total`, `gateway_drift_seconds`, `gateway_dry_run_fail_total`, `gateway_fictive_handler_skipped_total`, `gateway_legacy_fallback_total`, `gateway_reported_hash` gauge + alert on `rate(gateway_reconcile_failures_total[5m])>0`. | Always |

**Ship criteria for C → D:** canary env's `gateway_reported_state.last_error == ""` for 48 h; `GET /admin/gateways/reconcile/status` → `{drift:false}`; `openssl s_client` on auto-renewed wildcard serves the new leaf; `GET /api/v1/health/ready` unaffected; no `IngressSynchronizer` goroutine present (`grep -rn IngressSync* forge/api` only in `gateway/`). Legacy `traffic_rules` writes still succeed via shim.

### 10.5 Phase D — Traefik decision + cleanup (1 week, after C stable 1 release)

Goal: pay tech debt; make migration irreversible in a safe way.

| Change | Decision | Why |
|---|---|---|
| **Traefik adapter** | Either (a) fix: file-watch model, real PEM paths (`§6.4` + `§4.3` TraefikAdapter), wire behind `GATEWAY_ADAPTER=traefik` in at least one prod fleet, **or** (b) delete `traefik_proxy.go` 1,139 lines + `AdapterKind.Traefik` variant. | `traefik_proxy.go:1065-1084` dead reload + `380-385` PEM-as-path make it non-functional today. No halfway state. |
| **Delete `caddy_tls.go` or keep as shim** | Delete if no customer relied on embedded-Caddy LE (check `grep -rn ProvisionLetsEncrypt` — zero callers). Otherwise make it `gateway.Store.UpsertCertificate` behind a one-release `GATEWAY_LEGACY_CADDY_TLS=true`. | F-NET-02 wiper path. |
| **Drop dual-write** | Remove `GATEWAY_DUAL_WRITE` flag and the per-CRUD mirror helpers (see B3). Only `gateway_*` is now the source of truth. | Legacy `traffic_rules` become read-only compat view. |
| **Delete legacy tables (optional, deferred 2 releases)** | After proving 30 days `GATEWAY_LEGACY_ATTACH_ALL=false` with zero `gateway_legacy_fallback_total`, run `DROP TABLE traffic_rules, traffic_policies` etc. Or rename to `_legacy_*` first. | Minimizes surprise. Until then, legacy pages' shim keeps them as `SELECT * FROM gateway_routers` views — no second source. |
| **Remove `GATEWAY_SINGLE_WRITER` flag** | Make single-writer the only codepath (remove the `if flag==false` branch that allowed legacy whole-doc). | Lock-in the invariant. |
| **Firewall persistence fileprivate** | Not in this plan — stays in Settings until its own ADR (P1 `F-NET-19`). | Scope guard per task brief ("Operations timeline not here"). |

---

## 11. Backward compat — no wipe during migration

This is a **non-negotiable**: at **no** point between Phase A canary and Phase D lock-in does any code drop `traffic_rules` rows or call `POST /config/` whole-doc when there is existing reported state.

### 11.1 Guarantees

1. **No whole-doc wipe during migration:** From Phase A onward, the only allowed Caddy write is `safeApplyServers` (sub-resource per server). The whole-doc `POST /config/` path is guarded: it runs only when `gateway_reported_state` is empty (cold bootstrap) or when `GATEWAY_SUBRESOURCE_MERGE=false` (explicit opt-out for rollback). Reconciliation that wants to replace the whole doc still does it by merging every server via sub-resources.

2. **Empty `IngressSynchronizer` can never reach Caddy again:** Phase A's empty-sync guard is merged **before** any schema migration touches `gateway_*`. The guard stays even after `GatewayReconciler` exists — `IngressSynchronizer` is deleted in Phase C, but until then its `Sync` is a no-op for empty input. The same guard is added to `tmSvc.SyncRoutes/ReconcileRoutes:590-951` (they also rebuild from an empty filtered set when all targets are unhealthy).

3. **Dual-write atomicity:** Legacy CRUD transactions commit legacy rows **first**, then gateway rows in a second statement with `ON CONFLICT DO UPDATE`. If the second fails, the legacy row is still committed (the reconciler will backfill it on its next `ListRouters` fallback metric). No distributed transaction is required — the periodic backfill SQL (`052` logic) heals divergence within 1 reconciler interval (~30 s).

4. **Snapshot-durably:** `gateway_reported_state.config_json` is `json.RawMessage` of the last **reported** config read via `GET /config/` **after** a successful `ApplyDesired`, not a snapshot of desired. So a crash between desired write and gateway apply leaves `desiredHash != reportedHash` and the next reconciler tick retries — no silent loss.

5. **Rollout reversibility:** From Phase B through Phase C canary, `GATEWAY_SINGLE_WRITER=false` returns the binary to legacy wiring **without** data loss: `gateway_*` rows remain but are ignored; `gateway_reported_state` row is advisory. The canary→stable promotion flips this flag fleet-wide, not table by table.

### 11.2 Verifying "no wipe" in staging

Before promoting each phase to prod, run the **No-Wipe smoke suite** in a staging env that has real `gamepanel-domains` + custom certs + policies:

```bash
# 1. Seed 2 domains + 1 wildcard TLS cert + 2 traffic rules + 1 policy
GATEWAY_SINGLE_WRITER=false make seed:gateway-smoke

# 2. Phase A canary — wait 3 IngressSync cycles (90 s) and trigger 5 node churn events
while sleep 30; do curl -s http://admin:2019/config/ | jq '.apps.http.servers | keys'; done
# expect: keys == ["gamepanel","gamepanel-domains"] every iteration

# 3. Phase C flip — toggle flag and hit every new Gateways CRUD, then gate-wipe check
GATEWAY_SINGLE_WRITER=true systemctl restart forge-api
while sleep 30; do
  curl -s http://admin:2019/config/ | jq '.apps.http.servers["gamepanel"] | .routes | length'
  curl -s http://admin:2019/config/ | jq '.apps.http.servers["gamepanel-domains"] | .routes | length'
done
# expect: neither length drops to 0 at any point

# 4. Cert delivery smoke — renew a near-expiry test cert
curl -X POST /admin/gateways/certificates/<sni>/reissue
until openssl s_client -connect <sni>:443 </dev/null 2>/dev/null | openssl x509 -dates | grep notAfter | cut -d= -f2 | xargs -I{} date -d {} +%s | xargs -I{} test {} -gt $(date +%s); do sleep 5; done
```

---

## 12. Testing strategy

### 12.1 Unit — gateway package

| Suite | Files | What it asserts | Reference bug |
|---|---|---|---|
| `gateway/grouping_test.go` | `grouping.go` | One function `canonicalRouterKey` → `traffic_rules{domain:"EXAMPLE.COM", path:"", proto:""}` + `proxy_domains{hostname:"example.com", path:"/"}` → **same** `gateway_routers` row; `traffic_rules` half-divergence (traffic `domain` trimmed, domains not) collapsed | `caddy_proxy.go:894-922` vs `routegroup.go:32-68` triplication |
| `gateway/build_desired_test.go` | `build_desired.go` | Targets deduped (`UniqueBackends` sorted), degraded `health_status=down` excluded when threshold met but **kept** when no healthy alternative, weight preserved, per-router MW chain deterministic `(kind_order*1000+priority,name)` | F-NET-05, F-NET-06, F-NET-13, oscillation |
| `gateway/render_caddy_test.go` | `render_caddy.go` | `rate_limit` guarded → skipped + metric when `GATEWAY_ENABLE_FICTIVE_HANDLERS=false`; `ip_allowlist/denylist` emit `subroute{remote_ip…}` (as `caddy_proxy.go:806-856` proven); `circuit_breaker` wrapped as `reverse_proxy` param, not a sibling `handler`. Tests replace `caddy_proxy_test.go:280-314,547-615` assertions. | F-NET-04 |
| `gateway/protocol_branch_test.go` | `render_*.go` | `protocol==tcp` → error unless flag; `protocol==udp` → error always (admission already rejects); `protocol=="http"` → normal | F-NET-07 |
| `gateway/health_filter_embed_test.go` | `reconciler.go` | Builder respects `healthFilter.FilterHealthy` (threshold 3) — when a target is `HealthDown`, desired has one fewer server; when all targets down, builder emits no server for that router but keeps the router (320 empty pool vs wipe — covers `ProbeTargets:1001-1082` vs rebuild) | Oscillation |
| `crossnode/health_filter_test.go` | `crossnode/health_filter.go:76-124` (retain) | `RecordFailure` → `Degraded` until `FailCount>=threshold` → `Down`; `Reaper` `reapStaleEntries:268-281` deletes after `interval*2`; producers now **exist** (reconciler `RecordSuccess/Failure` per probe) | F-BRK-HEALTH-CONFIG-IGNORED |
| `gateway/dryrun_test.go` | `caddy_adapter.go` | `DryRun` never calls `POST /load`; snapshot **before** validate; `GET /config/` hash equals desired hash → skip apply | F-NET-03 |
| `gateway/weight_test.go` | `render_caddy.go` + `gateway/service.go:480-482` | Caddy weighted emit → `weighted_round_robin { weights: [3,1,5] }` ordered by `upstreams` order; LB per-service counter `roundRobin[serviceID]` not shared `__weighted` | Weighted routing |

### 12.2 Adapter contract tests (shared via `gateway/adapter_test.go`)

A **compliance harness** that runs against **both** `CaddyAdapter` (httptest admin) and `TraefikAdapter` (temp `configDir`):

```go
type adapterComplianceTC struct {
  name      string
  routers   []GatewayRouter
  targets   []GatewayTarget
  middlewares []GatewayMiddleware
  certs     []GatewayCertificate
  mutate    func(*gatewayStore) // optional: attach mw, add target, flip health
}

cases := []adapterComplianceTC{
  {"happy_one_router_one_target", …},
  {"fictive_skipped_dryrun_ok", …},              // F-NET-04
  {"grouped_route_withdraw_one_target", …},      // F-NET-06
  {"tcp_rejected_without_flag", …},              // F-NET-07
  {"weight_preserved_on_readd", …},              // weight dead
  {"empty_desired_no_wipe", …},                  // F-NET-01
  {"subresource_preserves_sibling_server", …},   // F-NET-02
}
for _, tc := range cases {
  runCompliance(t, caddyAdapter.New(httptest.NewServer(...).URL), tc)
  runCompliance(t, traefikAdapter.New(t.TempDir()), tc)
}
```

- **`subresource_preserves_sibling_server`**: seed caddy with `gamepanel-domains` route, then `ApplyDesired` only touching `gamepanel` → `GET /config/apps/http/servers/gamepanel-domains` still has its routes.
- **`weight_preserved_on_readd`**: `SetUpstreamHealth(false)` then `true` → target's `weight` equals original.
- **`fictive_skipped_dryrun_ok`**: `rate_limit` middleware on a router → `DryRun` succeeds, `GetConfig` after apply has no `rate_limit` handler, metric `gateway_fictive_handler_skipped_total==1`.

Extend existing suites rather than duplicating: `caddy_proxy_test.go:15-116` `TestAtomicReload_*` / `TestConcurrentReloadsAreSerialized` become `gateway/reconciler_concurrency_test.go` (single-reconciler `mu sync.Mutex` still serialized). `traefik_proxy_test.go` compliance case stays but driven by `GatewayAdapter` not raw YAML.

### 12.3 Integration / smoke

| Suite | How | Verifies |
|---|---|---|
| **Existing `crossnode/scenario7_test.go` + `crossnode_test.go`** | Already exercise `GroupRulesByRoute` / `UniqueBackends` / `HealthFilter.FilterHealthy` → adapt to `gateway/grouping.go` and add producer calls. | Health now has producers |
| **New `forge/api/internal/services/gateway/reconciler_integration_test.go`** | Use `testcontainers` Postgres + `httptest` Caddy admin fake (or real `caddy` binary via `exec`) + seeded `gateway_*` rows. Run `Reconcile → DryRun fail → desired unchanged → fix desired → success` sequence. | Single-writer invariant + true dry-run + snapshot-before |
| **Be-wire `acme/service_test.go:79` `TestHTTPSolver`** | Add a `TestHTTPSolverStripPort` — solver handles `r.Host == "example.com:8080"` correctly (`net.SplitHostPort` fix). | F-NET-09 |
| **Frontend Playwright** `web/e2e/gateways.spec.ts` | Create a router `example.com /api`, attach a `rate_limit` middleware, verify Routers tab shows ordered chip chain `rate_limit → redirect`; trigger `POST /admin/gateways/reconcile/trigger` and check drift banner goes green within 2 s. | ONE page + deterministic order |
| **No-wipe smoke** | Staging canary (§11.2) run on every nightly pipeline while flags are in transition. | Backward compat |

### 12.4 Load / chaos

- Reconcile every 200 ms with 500-route + 200-middleware large config — measure `gateway_reconcile_duration_seconds` P95; assert <1 s for typical tenant count (<100 routers).
- Chaos: kill Caddy admin, let reconciler retry 3 times, then restore — reported state should show `last_error` then recovery on next tick (not wipe).

### 12.5 Metrics & alerts to assert in tests

Every test that hits a mitigated F-NET bug should assert the corresponding `counter`/`gauge` moves:

```
gateway_reconcile_total{result="success|skip|error|dryrun_fail"}  counter
gateway_reconcile_duration_seconds                                histogram
gateway_drift                                                     gauge (0/1 via desiredHash != reportedHash)
gateway_reported_hash                                             gauge (or label on metric — hash is label in log)
gateway_fictive_handler_skipped_total                             counter   ← F-NET-04
gateway_legacy_fallback_total                                     counter   ← dual-read fallback
gateway_legacy_attach_all_fallback_total                          counter   ← empty join compat
caddy_subresource_apply_total                                     counter   ← F-NET-02
acme_http_solver_served_total                                     counter   ← F-NET-09
```

---

## 13. Risks & mitigations

| Risk | Severity | Mitigant |
|---|---|---|
| **Staging drift between legacy fallback and new tables** — operators assume `GET /admin/traffic/rules` shows truth while reconciler is rendering `gateway_*` | Medium | Keep `GET /admin/traffic/rules` shim that `SELECT` from `gateway_*` when `gateway_routers` has rows; emit `Warning: GatewayAdapter header` + `gateway_legacy_fallback_total`. Remove shim only after Phase D. |
| **Traefik adapter never verified in CI** — stock Traefik `pkg/api/handler.go:103-133` GET-only trick is validated off-box, not in Go tests | High | Keep compliance harness temp-dir path even when `GATEWAY_ADAPTER=caddy` in prod — the Traefik adapter is tested structurally (YAML marshal, file fsync, cert file path) without needing a running daemon. |
| **Caddy `/adapt` not exposed** — some Caddy builds leave admin `/adapt` disabled, making `DryRun` fall back to local JSON check only | Medium | Formalize fallback order: try `/adapt` (or `/config/` sub-resource `PUT` with `If-Match`); on 404 fall back to local schema + best-effort `caddy adapt --validate` via `exec` or bundled lib; record `gateway_dryrun_fallback_total` + emit warning audit row. |
| **Middleware order confusion** — operators attach middlewares in arbitrary order expecting per-policy semantics | Medium | UI enforces drag-drop ordering + server badges + "order matters" callout; API validates `priority` uniqueness per router (`CREATE UNIQUE INDEX … WHERE enabled=true`) — conflict returns `409` with suggestion. |
| **Weight translation bug on Caddy** — ordering of `Weights []int` must exactly match `Upstreams []Upstream` order | High | Adapter renders both slices in the **same** deterministic `sort.Slice` order (by `host:port`) and asserts `len(weights)==len(upstreams)` in `DryRun` — failure is a contract-test failure, not a runtime surprise. |
| **Flag sprawl** | Low | Phase D deletes all flags except `GATEWAY_ADAPTER` (and maybe `GATEWAY_ENABLE_L4`). CI lint `grep -rn GATEWAY_` ensures no flag lives >2 releases. |
| **Double source of truth during B** | Medium | The backfill SQL (`052`) is the heal; the canary monitor queries `SELECT COUNT(*) FROM traffic_rules WHERE id NOT IN (SELECT source_id FROM gateway_middlewares)` nightly and pages on >0 after B. |

---

## 14. Appendix A — P0 → fix traceability

| P0 | Location(s) | Phase A hotfix | Phase C inversion | Verifier |
|---|---|---|---|---|
| **F-NET-01** empty-rule ingress sync | `crossnode/ingress_sync.go:111-166` (always empty `rules` + no guard), wired `main.go:994` | `len==0→return nil` guard + `SetUpstreamHealth` append-bug fix | `IngressSynchronizer` **deleted**; reconciler never called with empty desired without being empty intentional (empty intentional = reconciler explicitly renders empty `gamepanel` server after confirming no routers — not wipe of sibling) | `ingress_sync_test.go` empty-guard; `reconciler empty_desired_no_wipe` compliance |
| **F-NET-02** asymmetric merge | `caddy_proxy.go:673-720` full-replace vs `514-593` merge; `caddy_tls.go:186-238` | Both → `safeApplyServers` sub-resource | `ApplyDesired` owns `apps.http.servers` map and merges only changed servers; `GET /config/` baseline always read first | `subresource_preserves_sibling_server` |
| **F-NET-03** validate=apply+snapshot-after | `caddy_proxy.go:980-1002,691-695` + `caddy_tls.go:240-251` + `traefik_proxy.go:974-1005,1065-1084` | Snapshot before; `/adapt` / local-validate, not `/load` | Same + `gateway_reported_state` persistence | `TestAtomicReload_*` updated to assert no `/load` |
| **F-NET-04** fictive `rate_limit`/`circuit_breaker` | `caddy_proxy.go:795-875`; `caddy_proxy_test.go:280-314,547-615` assert broken | `GATEWAY_ENABLE_FICTIVE_HANDLERS=false` → skip + metric | Real `rate_limit` only emitted when module present; CB translated to `reverse_proxy` param; old tests rewritten | `fictive_skipped_dryrun_ok` |
| **F-NET-05** all-policies-all-routes | `traefik_proxy.go:960-972`, `caddy_proxy.go:722-793`, `routegroup.go:149-151` | No hotfix (compat flag logs warning when all-policies used) | `gateway_router_middlewares` M:N + deterministic `(kind_order*1000+priority,name)` | Build-desired attach test |
| **F-NET-06** grouped-route miss | `caddy_proxy.go:60-74,949-951` vs `355-357`, `service.go:753-798` | `RemoveRoutes` handles `-group` suffix + exact match | Reconciler rebuild retains `@id` with filtered upstreams — no delete/create path | `grouped_route_withdraw_one_target` |
| **F-NET-07** TCP→HTTP mis-render | `service.go:383-387` admits `tcp`; `caddy_proxy.go:924-978` no branch | Reject `tcp` unless `GATEWAY_ENABLE_L4` | Gated L4 render (`layer4`/`udp:`) or reject; `loadbalancer/dataplane.go` stays canonical | `tcp_rejected_without_flag` |
| **F-NET-08** certs never leave Postgres | `gateway_adapter.go:49` zero callers; `acme/service.go:322-366` only DB; `caddy_tls.go:18-32` never constructed; `traefik_proxy.go:380-385` PEM-as-path | Interim: `acme` directly calls `SetCertificate` until reconciler ready | `acme → gateway_certificates` DB; reconciler diffs vs gateway + Traefik file paths | `openssl s_client` smoke |
| **F-NET-09** `HTTPSolver` dead | `acme/service.go:52-92,157-159` zero mount | Mount `HTTPSolver` on `:80` + fix `r.Host` port-strip | Same (reconciler ensures `/.well-known/acme-challenge/*` is proxied if challenge type is HTTP-01) | `TestHTTPSolverStripPort` |
| **F-NET-10** admin UX non-functional | `traffic/page.tsx:14-22,67-70,84,94` vs `service.go:24-39`; `handlers_trafficmanager.go:80-86` | Shim keeps `traffic/page.tsx` redirecting to `gateways` + fix `GET /policies` 405 / `PATCH`→`PUT` | **ONE** `Gateways` page (`/admin/gateways`) with Routers/Services/Middlewares/Certs tabs; old `traffic/page.tsx` becomes shim | Playwright `gateways.spec.ts` |

Cross-cutting:

| Issue | Location | Phase A | Phase C |
|---|---|---|---|
| Oscillation (probe removes, rebuild re-adds) | `service.go:1001-1082` vs `caddy_proxy.go:503-512` | `HealthFilter` threshold respected on build | `buildDesired` filters via `isHealthyLocked` per backend; weight preserved on re-add |
| LB `HealthCheckConfig` ignored | `dataplane.go:296-326` vs `service.go:62-69` | No change (warn metric) | `gateway_services.health_check` JSON honored as `TraefikHealthCheck` / Caddy `health_check` — same threshold engine |
| Weighting dead (Caddy per-upstream weight, shared `__weighted`) | `caddy_proxy.go:934-939` + `loadbalancer/service.go:480-482` | Preserve weight on `SetUpstreamHealth` re-add | `weighted_round_robin { weights: […] }` + per-service `roundRobin[serviceID]` |
| Compose verify global mu (delegated) | `compose/service.go` (outside gateway) | Not in scope — delegated to orchestrator guild | Compose guild owns fix — gateway never touches global mu |

---

## 15. Appendix B — Verification checklist

### 15.1 Pre-merge checklist (Phase A patch suite)

- [ ] `crossnode/ingress_sync.go:111` has `if len(is.rules)==0 && len(is.policies)==0 { return nil }` with log at `DEBUG`.
- [ ] `caddy_proxy.go:673-720` no longer calls `POST /config/` with a single-server body — uses `safeApplyServers` / `applyViaSubResource`.
- [ ] `caddy_proxy.go:980-1002` `validateConfig` never calls `POST /load` — try `/adapt`, else local JSON validate.
- [ ] Snapshot in `updateRoutesAtomic:687-702` is moved **before** `validateConfig` (or at least before any mutating call).
- [ ] `buildPolicyHandles:795-875` gated behind `GATEWAY_ENABLE_FICTIVE_HANDLERS != true`; tests `caddy_proxy_test.go:280-314,547-615` assert skip path.
- [ ] `RemoveRoutes:60-74` handles both `-group` and non-group + exact match; `Contains:457` fixed.
- [ ] `SetUpstreamHealth:503-512` checks `alreadyPresent` before appending; weight preserved.

### 15.2 Pre-promote checklist (Phase B → C canary)

- [ ] Migrations `050-054` applied on canary with no `DROP`/`ALTER DROP`; `gateway_routers` row count ≈ `SELECT COUNT(DISTINCT domain||'|'||path||'|'||protocol) FROM traffic_rules WHERE enabled=true` + `proxy_domains` dups.
- [ ] Dual-write parity: `make check:gridway-parity` (count equality) passes.
- [ ] No whole-doc `POST /config/` in Caddy admin access logs (grep admin logs for `POST /config/$` vs `POST /config/apps/http/servers`).
- [ ] Metrics `gateway_fictive_handler_skipped_total` > 0 suppressed fail-validation count drops to 0.
- [ ] No-wipe smoke (§11.2) passes for 24 h (no `gamepanel-domains` route loss).

### 15.3 Pre-GA checklist (Phase C fleet-wide)

- [ ] `main.go:982,994,1025` wire check: `caddyProxy` is constructed **only** inside `gateway` package (`grep -rn NewCaddyReverseProxy` outside `gateway/` fails in CI).
- [ ] Zero `IngressSynchronizer` refs remain (minus `gateway` shim): `grep -rn IngressSynchronizer forge/` empty outside tests.
- [ ] `gateway_adapter.go:49` `SetCertificate` has callers — `gateway/reconciler.go:ApplyDesired` + Traefik file path.
- [ ] Traefik compliance suite green (even though `GATEWAY_ADAPTER=caddy`) — dead reload removed `traefik_proxy.go:1065-1084` and PEM-as-path fixed.
- [ ] ONE Gateways page is the only entry in admin nav for networking; old `traffic/page.tsx:14-22` shims with redirect header.
- [ ] `GET /admin/gateways/reconcile/status` → `{drift:false, lastError:""}` for 48 h; `openssl s_client` on auto-renewed wildcard serves new NotAfter.
- [ ] Load test: 500 routers + 200 middlewares reconciles in <1 s P95.
- [ ] All 10 phase-04 P0s `F-NET-01..10` verified fixed by running `audits/reverification/subagent-14-networking-gateway.md` checklist again — expect each "Still broken?" = **NO**.

---

## 16. Open questions (decide before Phase C code freeze)

1. **Port allocator for LB (`30000-30100` `:104-108`) vs gateway `target_port` ledger.** Today they collide in `targetGroups`. The inversion keeps LB as separate dataplane; should the gateway ledger also allocate ports for LB targets? Default recommendation: keep LB port pool out-of-scope for gateway, but add a `port_allocations` table when we fix `F-NET-13`.

2. **TLS-ALPN-01** — `acme/service.go:40-41` only supports `HTTP01/DNS01`. Caddy supports `tlsalpn` (`modules/caddytls/acmeissuer.go`). Should we add `ChallengeTypeTLSALPN01` now or after DNS-01 is stable? Default: after — complexity with shared `:443`.

3. **SecurityHeaders/RedirectRules render on Caddy vs Traefik** — `security_headers` has write-only `store_security_headers.go:45` and `500` on `GET` (mirrored to `gateway_middlewares kind=headers`). Should the backfill synthesize one `headers` middleware per distinct header set now, or only on Phase D when `security_headers` is deleted? Default: synthesize during `052` backfill per distinct row.

4. **Firewall persistence** — intentionally NOT in Gateways per task brief ("Operations timeline not here"). Tracked in P1 backlog `F-NET-19`; no action in this plan beyond leaving firewall under Settings.

---

## 17. References (representative file:line — inspected this pass)

- Wiring: `forge/api/cmd/api/main.go:982` `NewCaddyReverseProxy`, `994-995` `IngressSynchronizer.Start(30s)`, `998` `domains.New`, `1025` `NewWithPersistence(...,caddyProxy)`, `1030-1038` startup domain reverify, `1041-1067` 7 subscriptions, `1178-1192` `JobCertRenewal`, `128-142` `APP_ENV`/`PANEL_URL` guards.
- Caddy adapter: `caddy_proxy.go:23-49` struct + `p.mu`, `51-74` `RemoveRoutes`, `83-121` `SetCertificate`, `149-204` `GetActiveConnections` fiction, `234-293,295-373` `ValidateConfig/CleanupStale`, `375-377` `Health()` hardcoded, `379-512` `SetUpstreamHealth` (append-bug `503-508`, `Contains:457`), `514-593` `UpdateDomainRoutes` merge, `595-655` wildcard apex+wildcard, `673-720` `updateRoutesAtomic` (validate-before-snapshot), `707-720` `buildServerConfig` (`gamepanel` only), `722-793` `buildPolicyRoutes`, `795-875` fictive handlers, `894-978` grouping+`buildGroupedRoute` (`-group:949-951`, `weight:937-939`, `lb_policy:973-975`), `980-1052` `validate/getRunning/apply/restore`.
- TLS second client: `caddy_tls.go:18-32,58-175,186-238,240-251` (never constructed).
- Traefik adapter 1139 lines: `traefik_proxy.go:223-237` ctor, `243-293` `UpdateRoutes` (round-trip validate), `273-293` `RemoveRoutes`, `373-397` `SetCertificate` (PEM-as-path), `471-559` `CleanupStale`, `561-604` `Health`, `606-697` `SetUpstreamHealth`, `725-753` duplicate grouping, `755-905` grouping+TCP branch, `907-972` `buildMiddlewares/collectApplicablePolicies` (`960-972` returns all), `974-1005` `validateYAMLConfig`, `1065-1084` `reloadTraefik POST /api/refresh` dead, `1100-1108` TLS paths, `1134-1139` `writeConfigWithBackup`.
- Gateway abstraction: `gateway_adapter.go:10-64` `GatewayAdapter` (+ legacy `service.go:101-105` `ReverseProxy`, `domains/service.go:74-76` `caddyUpdater`).
- Traffic service: `service.go:24-54` models, `81-89` fields `proxy+adapter+mu`, `131-141` `NewWithAdapter*`, `291-359` CRUD DB+persistence, `361-405` `validateRoutingRule` (protocol+weight), `562-618` `ApplyRoutes/SyncRoutes`, `675-720` `AdapterKind/Health`, `753-951` `Withdraw/Reinstate/ReconcileRoutes`, `1001-1082` `ProbeTargets` (2 s dial, threshold 3).
- Crossnode: `ingress_sync.go:23-37,111-198,200-228` empty-sync; `routegroup.go:32-97,149-151` grouping+`extractPolicyID=""`; `health_filter.go:42-56,76-124,203-282` threshold/reaper zero producers; `resolver.go:59-111,135-166` 30 s `localhost` cache.
- LB: `loadbalancer/service.go:22-69,480-482` `__weighted` shared counter, `552-639` node-mark; `dataplane.go:20-49,62-92,115-270,296-326` listeners + `runHealthChecks` fixed 2 s.
- Domains/DNS/ACME: `domains/service.go:74-76,87-89,455-489,504-534` + hardcode `localhost:8080:467-468`; `dns/service.go:311-487,489-498,555-591,639-661` ~36 providers + env mutation; `acme/service.go:52-92,157-159,193-295,315-317,368-370,389-438,492-546,612-638` solver dead + key reuse + revoke + 24 h ticker + recover outside loop.
- Handlers/UI: `handlers_trafficmanager.go:8-113`, `handlers_loadbalancer.go:24-138`, `handlers_domains.go:102-124`, `handlers_proxy_domains.go:9-413` (verify stub `199-212`, `/custom-certificates:227`), `handlers_certificates.go:15-77` collision, `handlers_servicediscovery.go:18-193`, `handlers_firewall.go:78-193`; web `traffic/page.tsx:14-22,67-70,84,94,108-121`, `load-balancer/page.tsx:142-146,158-159`, `certificates/page.tsx:43-50,76-79`, `domains/page.tsx:127-129`, `security (AdminSecurity.tsx:114-133,126)`.
- References read (contrasts): `reference/networking/caddy/caddyconfig/load.go:68-116,137-175` `/load` vs `/adapt`; `reference/networking/caddy/admin.go:1067-1069,1091-1099` `must-revalidate` + `/id/<id>`; `reference/networking/traefik/pkg/config/dynamic/http_config.go:38-99` router→middleware→service; `reference/networking/traefik/pkg/api/handler.go:103-133` GET-only; `reference/networking/traefik/pkg/provider/file/file.go:90,170-207` `fsnotify`; `reference/networking/nginx-proxy-manager/backend/internal/nginx.js:27-117` `configure→test→reload`; `reference/app-platforms/uncloud/internal/machine/caddyconfig/controller.go:92,148-200` fingerprint-cache single writer.

---

*Phase 04 established seven tables encoding three Traefik concepts and five writers on one Caddy; phase 06 showed Uncloud avoids the problem per-node. The networking cluster is therefore the load-bearing finding of the audit — fixing single-writer + true dry-run + cert delivery unblocks TLS, policy, and L4 simultaneously. Adopt the Traefik shape as the canonical Gateway model — this plan.*

