# Subagent 10 — Phase 01 Audit: Observability & UX (Health, Monitoring, Alerting, IA, Design Tokens)

> **Agent:** 110-01-10 of 110 — Phase 01 Agent 10/10 (Observability & UX)
> **Date:** 2026-08-24
> **Mode:** preparation audit — inventory + gaps + files to touch — **no code modified**
> **Scope:** `forge/web` IA/design tokens/component inventory + `forge/api/internal/services/{cronjob,heartbeatmonitor,observability}` + `forge/web/lib/api` tone maps + `forge/web/components/{server,admin,shared,charts}` health/monitoring/console/backups/transfer
> **Evidence rule:** every claim is `path:line` verifiable on the current checkout under `/Users/riyaz/project/gamepanel`.

---

## 0. Executive Summary

Forge observability/UX is **inventory-complete but consistency-broken**. The highest-value design philosophy (`forge/web/app/globals.css:5` semantic tokens + `AdminOverview.tsx:62`/`monitoring/page.tsx:62`/`AdminHealth.tsx:105`/`host/page.tsx:213` “universalize” examples) is intentionally not propagated to the majority of the admin surface. Four pages now demonstrate the target language (observability = **monitoring**, diagnostics = **health**, live probe = **host**, snapshot = **overview**); ~40 remaining admin routes still carry hard-coded palette values and duplicated state/presentation primitives. The densest P0-grade UX risk remains the **gateway scattering + deployment dual-poller + triple env-editor + Docker node-blind** cluster already flagged in `FINAL_PARITY_AUDIT.md §16` and `phase-01/subagent-04` C01–C17. Activation work should codify the existing good philosophy as lint-able tokens/primitives first, then converge the duplicated components under single writers.

**Single sentence verdict:** the design philosophy is correct and exemplified in `monitoring`/`health`/`host`/`overview`; it must now be mechanically universalized via tokens + single `ServerStatus`/`DeploymentTimeline`/`LogViewer` primitives, not re-designed.

---

## 1. Methodology

* Read inspected (via `read` tool) every file named in the task plus their direct dependencies (shell, registry, overview, monitoring, health, host, console-view, backups-view, transfer-view, deployment-progress, DeploymentTimeline, env editors, docker/containers-view, admin-ui, states-* , stores, lib/api/*, heartbeatmonitor, cronjob, observability, metrics).
* Cross-checked against `audits/FINAL_PARITY_AUDIT.md:326-341 §12.7 + §16` (14 UX rows) and `audits/phase-01/subagent-04-ux-control-plane.md:164-386` (17 capability comparisons C01–C17, 6 logic findings FL-UX-01–06).
* Searched for hard-coded color literals vs token usage (`var(--*)` vs `bg-[#1e2536]` etc.), duplicated `queryKey` prefixes, and per-file `statusConfig` maps.
* No code written — `git diff` null expected.

---

## 2. Inventory — Files Inspected (with line-cited purpose)

### 2.1 Navigation / IA

| File | Line(s) | Role |
|---|---|---|
| `forge/web/components/admin/admin-registry.ts:22` | `adminPageRegistry: AdminNavGroup[]` | Source registry — 5 groups, 62 entries total (`Operations 10 :23`, `Infrastructure 8 :35`, `Management 8 :45`, `Services 9 :55`, `Advanced 27 :65`) |
| `forge/web/components/admin/admin-shell.tsx:42` | `navGroups` memo + `plan:48` | Goal-grouped re-mapping — 8 groups in current `plan` (`Command 4`, `Operations 4`, `Runtime 2`, `Workloads 8`, `People & Access 6`, `Infrastructure & Data 10`, `Networking & Security 12`, `Platform Configuration 10` → 54 hrefs mapped of ~62) |
| `forge/web/components/admin/admin-shell.tsx:58` | `entries = adminPagesForRole(...).flatMap` | Filtering by role (`admin`) — `capability: metadata-only` (`admin-registry.ts:60` Plugins) excluded from shell until wired |
| `forge/web/components/admin/admin-shell.tsx:119` | layout `h-screen overflow-hidden` | Two-pane scroll split: sidebar independent `overflow-y-auto` + content `overflow-y-auto` with `bg-[radial-gradient(...)]` hero |
| `forge/web/components/admin/AdminOverview.tsx:30` | `export function AdminOverview()` | Snapshot dashboard (5 queries: `nodesQ:33`, `serversQ:34`, `usersQ:35`, `healthQ:36`, `activityQ:37`) |

### 2.2 Observability Pages (the 4 “good philosophy” exemplars)

| File | Line(s) | Role / why cited |
|---|---|---|
| `forge/web/app/admin/monitoring/page.tsx:1` | entire 189 lines | **Monitoring = live series** — `Period 5m/1h/6h/24h :14`, `Metric cpu/memory/disk/networkRx :20`, synthetic-axis guard `isSynthetic:51`, chart `AreaChart :112`, node comparison `table :132`, `metricsQ refetch 10s/30s :46` |
| `forge/web/components/admin/AdminHealth.tsx:70` | `export function AdminHealth` | **Health = diagnostics** — 6 health queries `healthQ:71`/`nodesQ:72`/`serversQ:73`, overall `tone :99`, Failures ` :120`, Warnings ` :149`, sections `Infrastructure :165`, `Database & cache :197`, `Control plane :209`, `Workloads :220`, `Runtime :234` |
| `forge/web/app/admin/host/page.tsx:1` | `AdminHost` | **Host = per-node live probe** — tabs `info/disk/memory/network/processes :14`, `useHostQuery :23` with `AbortSignal.timeout 15s :27`, `NodeSelect :209`, per-tab daemons `InfoTab:62`, `DiskTab:96`, `MemoryTab:123`, `NetworkTab:144`, `ProcessesTab:168` |
| `forge/web/components/admin/AdminOverview.tsx:62` | `border-b border-[var(--line)]` etc | **Overview = inventory + capacity + attention** — same token language as the other three; `attentionCount:49`, `nodeMem:53`, `StatusBreakdown implicit at :98-138` |

### 2.3 App Detail / Deployment Experience

| File | Line(s) | Evidence |
|---|---|---|
| `forge/web/app/admin/apps/[id]/page.tsx:28` | `TABS 7` + `useState<TabId>((searchParams.get("tab") …) \|\| "overview"):43` | **IA defect:** tab is `useState` initialised from URL once; `setTab` never writes back to `searchParams` → refresh/back lose tab, no deep link |
| `forge/web/components/app/deployment-progress.tsx:41` | `POLL_INTERVAL_MS 2000`, `MAX_POLL 10m`, `refetchInterval:70` | 2 s poller, manual `DeploymentStep 15` API via `fetchDeploymentSteps` |
| `forge/web/components/charts/DeploymentTimeline.tsx:27` | `refetchInterval: hasActive ? 5000 : false` | 5 s poller, same endpoint `fetchDeploymentSteps`, different queryKey (`["deployment-steps",deploymentId]` vs `DeploymentProgress:68` same key prefix) |
| `forge/web/lib/api/deployments.ts:16` + `apps.ts:265` | type + `fetchDeploymentSteps` 15 funcs | Canonical deployment API; two consumers above duplicate it |

### 2.4 Env Var Editors (TRIPLICATE)

| Variant | File:Line | Data model | API path |
|---|---|---|---|
| A (admin, scoped) | `forge/web/components/environment/env-var-editor.tsx:8` `EnvVarEditor({scopeType,scopeId})` | `EnvVarResponse[]` `fetchEnvVars:23`, `createEnvVar:42`, `deleteEnvVar:53`, `isSensitive`, `version`, `Pill Sensitive:92` | `GET/POST /{projects\|environments}/:id/env-vars`, `DELETE /env-vars/:varId` via `forge/web/lib/api/env-vars.ts:20` |
| B (app config, Record) | `forge/web/components/admin/AdminAppsShared.tsx:106` `EnvVarEditor({envVars: Record<string,string>})` | `ENV_KEY_REGEX:106`, upsert on duplicate `:128`, key input disabled `:153`, no sensitive/version/multiline/buildtime | `updateApp(391) bulk PUT /apps/:id { envVars: Record }` — bypasses `env-vars.ts` entirely |
| C (shared, array+encrypted) | `forge/web/components/environment/EnvironmentEditor.tsx:41` `EnvironmentEditor({variables, onChange})` | `EnvVar{key,value,encrypted}:7`, import/export `.env` via `parseEnvFormat:19`/`toEnvFormat:37`, search `:49`, `encrypted` toggle `:104` | Parent-owned array, no direct API; consumer decides whether to wire to A or B |

### 2.5 Docker / Gateways

| File | Line(s) | Defect |
|---|---|---|
| `forge/web/app/admin/docker/page.tsx:17` | `TABS 4`, `useState<DockerTab>("containers"):22`, render `ContainersView/ImagesView/NetworksView/VolumesView` | **Node-blind:** no `NodeSelect`, no grouping — flattens global `listContainers(all)` without per-node filter unlike `monitoring:89`/`host:209` |
| `forge/web/components/docker/containers-view.tsx:43` | `listContainers({all:true})`, `stateTone :25`, `OperateContainer 5 verbs :73`, `DeleteContainer` | Good search + operate + delete + logs modal, but node not a filter control |
| `forge/web/app/admin/traffic/page.tsx:14` | `RouteRule`, `TrafficPolicy`, tabs `routes/policies` | One of **7 gateway pages**; duplicate `OfflineBanner` `:140+:151`; `postJSON /admin/traffic/rules :84` vs `GET /admin/traffic/policies` |
| Other 6 gateway-adjacent pages | `forge/web/app/admin/{load-balancer,domains,certificates,security,firewall,endpoints}/page.tsx` + `traffic/page.tsx` | IA scatters one workflow (domain→DNS→cert→headers→route) across 7 routes; shell `admin-shell.tsx:55` already clusters them under `Networking & Security` (12 hrefs) but pages remain independent inventories |

### 2.6 Health / Heartbeat / Observability Services

| File | Line(s) | Evidence |
|---|---|---|
| `forge/api/internal/services/heartbeatmonitor/service.go:266` | `func (s *Service) classify(... )` | **6-state classifier** — `healthy/suspected/unreachable/offline/recovering/reconciling` with thresholds `Warning 30s :91`, `Offline 90s :92`, `Unavailable 300s :93`, `RecoveryThreshold 2 :94`, history window `RecoveryThreshold+3 :206` |
| `forge/api/internal/services/heartbeatmonitor/service.go:252` | `alerter.CheckStaleHeartbeat` on `Unreachable` | Alert wiring on transition; otherwise write-only monitoring (see observability) |
| `forge/api/internal/services/observability/service.go:28` | `StartNodeMetricsCollection` with jitter `40` | Collector reconciler `interval` param, `collectNodeMetrics:59` derives `cpu/memory/disk` from `NodeCapacitySnapshot:89` (allocation-based, not live OS) |
| `forge/api/internal/services/observability/service.go:59` | `CPULoad1m/5m/15m = 0`, `NetworkRx/Tx = 0 :86-87` | **Honest gap:** live load/network not collected by Beacon → monitoring `isSynthetic` guard `:51` handles it honestly in new build |
| `forge/api/internal/services/observability/metrics_collector.go:41` | `CollectSystemMetrics()`, `MetricsHistory :25` | Control-plane self-metrics (goroutines, heap) stored in memory history, not DB |
| `forge/api/internal/services/cronjob/service.go:117` | `executeJob`, `dispatchServerCommand :244`, `runShellCommand :212` | Cron split: `TargetType=="server"` → `serverDispatcher` (daemon channel, fail-closed `:250`), default → `sh -c` with `5*(i+1)s` retry `:161`, `HOME/PATH` scrub `:220` |

### 2.7 Design Tokens

| File | Line(s) | Evidence |
|---|---|---|
| `forge/web/app/globals.css:5` | `:root { --brand: #dc2626; --brand-hover: #ef4444; --canvas #0a0e16; --surface #111722; --surface-raised #171f2d; --surface-input #0d131d; --line rgba(148,163,184,.14); --text #f1f5f9; --text-subtle #94a3b8; --success #059669; --warning #d97706; --danger #dc2626 }` | Canonical 13 tokens + `data-theme="light" :30` alt + `ui-*` components `:71-117` (button/input/card/alert/badge/empty/status-pill/toast) |
| `forge/web/app/globals.css:71` | `@layer components .ui-button, .ui-input, .ui-card, .ui-alert, .ui-badge, .ui-empty, .ui-status-pill` | Token-consuming primitives; doc comment `:72 "Canonical semantic tokens — do not hardcode hex/rgba elsewhere; use var(--*)"` |
| `web/app/globals.css:5` | legacy `web/` with `--ink/--surface/--paper/--red` | Legacy-only — confirm `forge/web` is canonical |
| 4 exemplar pages | `AdminOverview:64`, `monitoring/page.tsx:62`, `AdminHealth:106`, `host/page.tsx:213` | All 4 use **exclusively** `border-[var(--line)] / text-[var(--text-subtle)] / bg-[var(--surface)]` — demonstrate target language |
| Counter-examples | `forge/web/components/server/backups-view.tsx:89`, `console-view.tsx:267`, `forge/web/app/admin/traffic/page.tsx:161`, `AdminAppsShared:30` etc | Still `bg-[#1e2536]` / `bg-[#161b28]` / `border-white/[0.06]` hard-coded — to be universalized |

### 2.8 Shared Status / Tone Maps

| File | Line(s) | Map |
|---|---|---|
| `forge/web/lib/api/apps.ts:392` | `statusTone(status: AppStatus): "green"|"red"|"yellow"|"blue"|"neutral"` + `deploymentStatusTone :403` | App 9-state tone map; deployment 6-state |
| `forge/web/components/shared/states-badge.tsx:55` | `ServerStatusType 7: running/stopped/installing/suspended/offline/transferring/errored` + `serverStatusConfig:64` with `success/neutral/loading/warning/danger/info` | Console-facing state lanes (7 two-dot lanes) |
| `forge/web/components/shared/states-badge.tsx:82` / `100` / `119` / `144` / `162` | `BuildStatus 6`, `DeploymentStatus 5 (pending/deploying/deployed/rolled-back/failed)`, `CertStatus 5`, `DBStatus 5`, `VerificationStatus 3` | Additional lane families — all duplicate the same `BadgeTone` hierarchy but per-type enums |
| `forge/web/components/app/deployment-progress.tsx:32` | `statusConfig: Record<string, {icon,className}> 6 states` | `pending/in_progress/completed/failed/cancelled/skipped` — locally invented color vs `apps.ts:403` |
| `forge/web/components/charts/DeploymentTimeline.tsx:14` | `statusConfig 6 states` with `color/bg` | Same 6-state but `loading blue vs emerald completed` — visually diverges from DeploymentProgress |
| `forge/web/components/charts/ServerCPUChart.tsx:66`, `ServerMemoryChart:71`, `ServerNetworkChart:73`, etc. | per-chart `sort(observedAt)` + `areaChart` duplicated logic | Not a defect but duplication to be unified later if a chart primitive is added |

### 2.9 Empty / Error / Loading

| File | Line(s) | Inventory |
|---|---|---|
| `forge/web/components/shared/states-empty.tsx:17` | `EmptyCard` + `EmptyList/EmptySearch/EmptyDeployments/EmptyBackups/EmptyDomains/EmptyServices/EmptyGit/EmptyCertificates/EmptyDNSProviders/EmptyOrganizations` — 10 semantic empties | Complete — shared inventory already exists; commission is to **wire consistently**, not rebuild |
| `forge/web/components/shared/states-error.tsx:18` | `ErrorCard` + `ErrorAlert:43`, `ErrorNotFound:98`, `ErrorPermission:122`, `ErrorNetwork:142`, `ErrorRateLimit:164` (countdown) | `ErrorRateLimit` exists but never consumed — `lib/api/http.ts:102` does not surface `429 Retry-After` |
| `forge/web/components/shared/states-offline.tsx:6` | `OfflineBanner` with `navigator.onLine` listener | Duplicated in `AdminShell` global banner + `host/monitoring/traffic/apps/[id]` per-page banners (see §6) |
| `forge/web/components/admin/admin-ui.tsx:220` | `AdminLoadingState`, `AdminLoadingRows:222`, `AdminErrorState:226`, `AdminOfflineBanner:228`, `AdminDegradedState:237`, `Card/CardHeader:61`, `Pill:15`, `Btn:81`, `Input:108` | Legacy/alternate family shadowing `shared/*` — convergence target |

### 2.10 Console / Backups / Transfer

| File | Line(s) | UX finding |
|---|---|---|
| `forge/web/components/server/console-view.tsx:299` | `{showTimestamps ? <span className="mr-2 text-slate-400">{new Date().toLocaleTimeString()}</span> : null}` | **Synthetic timestamps:** generated at render time (`new Date()`), not daemon `observedAt`; every line shows the same tick — must use server timestamp if available |
| `forge/web/components/server/console-view.tsx:203` | `const network = statsData.networkRxBytes + statsData.networkTxBytes` + `networkHistory :206` | **Network cumulative:** plots `rx+tx` byte total (monotonically growing) not rate (`bytes/s`); will stair-step toward max and be clamped by `Math.max(...values,1)` scaling — should sample deltas |
| `forge/web/components/server/backups-view.tsx:101` | `const busy = restoreMutation.isPending \|\| deleteMutation.isPending \|\| lockBackupMutation.isPending \|\| unlockBackupMutation.isPending` | **Global busy:** single `busy` disables lock/unlock/restore/delete on *every* row while *any* backing per-backup mutation is pending — should be per-`backup.name` pending state |
| `forge/web/components/server/backups-view.tsx:205` | `Storage Destination` hard-coded “not supported yet” copy | Honest but not tokenized (`bg-[#0f141f] border-white/[0.1]` vs 4 exemplar pages) |
| `forge/web/components/server/transfer-view.tsx:40` | `transferStatusQuery` + `fetchServerTransferStatus` vs `server.transferring/transferState/transferError :79-82` | **Dual source:** polls dedicated transfer status (`progress?: number :82`) but flags come from `server` prop (eventual-consistency gap — polling transfer can show idle while `server.transferring` still true, and vice versa) |
| `forge/web/components/server/transfer-view.tsx:53` | `.filter((a: ApiAllocation) => !a.server)` | Allocation filtering is local; no server-side guarantee the returned `ApiAllocation.server` is fresh |
| `forge/api/internal/services/observability/service.go:58` | `collectNodeMetrics` composes `allocation-based` `MemoryPercent/DiskPercent/CPUPercent` | Observability noted above — correctly sampled, not live OS |

---

## 3. Cross-Reference Against Prior Audits

### 3.1 FINAL_PARITY_AUDIT.md §16 (14 UX rows) — delta vs 2026-08-24 checkout

| ID (from §16) | Status at FINAL_PARITY | This audit (delta) | Notes |
|---|---|---|---|
| UX-01 Navigation IA (5 → 6 groups, ~62 → 50/62 mapped) | PARTIAL P2 | **STILL PARTIAL P2** — current `plan` in `admin-shell.tsx:48` now covers 54 hrefs (up from ~50) but Advanced still biggest; registry `5 groups 62` vs plan `8 groups 54` — 8 orphan entries remain plus `/admin/host`→Runtime oddity (should be Infrastructure per mismatch). `AdminPageLayout` now used on `host` and `traffic` but not universally. |
| UX-02 Dashboards split (overview inventory vs monitoring live) | COMPLETE | **STILL COMPLETE** — and now *exemplar* class: `AdminOverview` + `monitoring/page.tsx` split is explicitly called out as the pattern to keep |
| UX-03 App detail tabs (`useState` not routed) | BROKEN | **STILL BROKEN** — `apps/[id]/page.tsx:43` unchanged; §21 #45 still open |
| UX-04 Deployment dual pollers (2 s vs 5 s) | BROKEN duplicate | **STILL BROKEN** — unchanged |
| UX-05 Status tone maps per-file divergence | PARTIAL | **STILL PARTIAL** — now inventoried 5+ divergent `statusConfig` maps |
| UX-06 Empty/loading/error states (10 empties wired, RateLimit/OfflineBanner duplicated) | PARTIAL | **STILL PARTIAL** — `states-empty:17` now 10 wrappers still wired; `states-error:43` now 5; duplicate `OfflineBanner` present in `host`, `monitoring`, `traffic` per-page plus `AdminShell` global; `ErrorRateLimit:164` still unconsumed |
| UX-07 Logs placement (modal vs inline) | PARTIAL | **STILL PARTIAL** — `DeploymentLogViewer` lives in `forge/web/components/deployment/DeploymentLogViewer.tsx:37` (WS) not under `DeploymentTimeline` |
| UX-08 Domains gated behind `!!serverFilter` | BROKEN | **STILL BROKEN** — `admin/domains/page.tsx:57` `enabled: !!serverFilter` |
| UX-09 Triple env editors | BROKEN | **STILL BROKEN** — A/B/C still 3 editors, APIs overloaded |
| UX-10 Tenancy cascade display-only | BROKEN | **STILL BROKEN** — `fetchApps()` global not filtered; `protected` show-only |
| UX-11 Docker view node-blind | PARTIAL | **STILL PARTIAL** — confirmed `docker/page.tsx:17` no filter |
| UX-12 Gateway 7 scattered pages | IA debt | **STILL IA DEBT** — 7 pages still distinct; plan note §12 in FINAL_PARITY still accurate |
| UX-13 (implicit) Monitoring metric fallbacks synthetic-zero | BROKEN | **PARTIALLY MITIGATED** — `monitoring/page.tsx:51` `isSynthetic` banner now honestly discloses `allocated capacity (not live OS)` — UX fix landed but data gap remains |
| UX-14 (implicit) Metric tone divergence | PARTIAL | **STILL PARTIAL** — per-file metrics colors not centralized |

**No UX P0 was reclassified FIXED in final-parity addendum §A except runtime chains; UX remains intact.**

### 3.2 phase-01/subagent-04 17 comparisons (C01–C17) — condensed scoreboard

| # | Title | Prior status | Evidence path (this audit confirms) | Action for 110-* |
|---|---|---|---|---|
| C01 | IA / Navigation | PARTIAL P2 | `admin-registry.ts:22` vs `admin-shell.tsx:48` (8→54) — second-level collapse still absent | P2 — group audit + alias sweep (Containers/Logs alias note at `admin-shell.tsx:46`) |
| C02 | Dashboards | COMPLETE | `AdminOverview:30` + `monitoring/page.tsx:1` | **Preserve** — exemplars for universalization |
| C03 | App List / Detail | PARTIAL P2 | `apps/[id]/page.tsx:43` tab state, `app-detail.tsx:55` back to `/servers` vs `/admin/apps` (shared component second back hardcode) | P2 — route the tabs, fix back prop |
| C04 | Deployment Experience | PARTIAL P2 | `deployment-progress:41` vs `DeploymentTimeline:27` 2s/5s | P2 — merge to single `DeploymentTimeline` |
| C05 | Deployment Status Communication | PARTIAL P3 | `states-badge:55` vs `apps.ts:392` vs per-file `statusConfig` | P3 — central tone map |
| C06 | Empty States | COMPLETE | `states-empty:17` 10 | P3 — wire CTAs, audit messages |
| C07 | Loading / Failure | PARTIAL P3 | `states-error:43` 5 + `http.ts:102` no 429 surface, `OfflineBanner` dup | P3 — 429 wire + banner dedup |
| C08 | Logs / Metrics Placement | PARTIAL P2 | `DeploymentLogViewer:37` WS vs `AppLogs` polling | P2 — inline logs under timeline |
| C09 | Domains / TLS | PARTIAL P2 | `domains:57` gated, `traffic:14` dual domains types | P2 — aggregate view + DNS merge |
| C10 | Docker | COMPLETE* (*with caveat) | `docker/page:17` blind | P3 — add node filter |
| C11 | Env Vars / Secrets | **BROKEN P1** | Triplicate A/B/C | **P1** — normalize to one editor+API |
| C12 | Projects / Environments / Tenancy | PARTIAL P2 | Filters display-only, cascade three dropdowns | P2 — scope `fetchApps` |
| C13 | Permissions / Roles / OAuth | PARTIAL P2 | Tabs hidden vs disabled | P2 — central perm map |
| C14 | Charts / Monitoring / Metrics | COMPLETE* | Metric duplication + synthetic | P3 — keep one presentation, wire node filter |
| C15 | Domains vs DNS vs Certs vs Security vs Traffic | PARTIAL P3 | Over-decomposition 7 pages | P3 — cross-links + workflow CTA |
| C16 | Compose / Stacks | PARTIAL P2 | `/apps/:id/compose` vs `/compose/:id` dual model | P2 — unify |
| C17 | Translation / i18n | COMPLETE P4 | Double fallback hiding gaps | P4 — dev warn |

**Carry-forward P1 still open from subagent-04:** `FL-UX-02` fake CreateApp fields (`app-create-form.tsx:22`), `FL-UX-03` triple env (this audit §2.4), `FL-UX-01` duplicate nav entry (Recovery↔Migrations dup now resolved? Registry no longer has Recovery duplicate — `admin-registry.ts:31` shows Migrations only; `admin-shell:48` lists Operations → migrations/reconciliation — so **FIXED** since that phase), `FL-UX-05` dead empty CTA (`app-list.tsx:44`).

---

## 4. The Universalized Design Philosophy — What to Standardize

The ask (“universalize design philosophy — refer **monitoring**, **health**, **host**, **overview**”) maps to **four concrete canonical patterns already shipped** in the codebase. Treat these as the *law*, not as additional examples.

### 4.1 The 4 exemplars and why they’re canonical

| Page | File:Line | Philosophy dictum | Evidence |
|---|---|---|---|
| **Monitoring** | `forge/web/app/admin/monitoring/page.tsx:62` | “Time, metric, and node comparison — sampled every 15 s.” Labels sampling explicitly; live/historical toggle is a *control* not a mode. | Eyebrow `text-[11px] uppercase tracking-[0.12em] text-[var(--text-subtle)] :64`, `Live` pulse `bg-emerald-500 animate-pulse shadow-[0_0_0_4px_rgba(16,185,129,0.18)] :67`, hint `last · no data :67`, chart handles empty `No telemetry available — Beacon disconnected :111`, border `border-[var(--line)]`, bg `bg-[var(--surface)] :110` |
| **Health** | `forge/web/components/admin/AdminHealth.tsx:106` | “What is wrong, degraded, or at risk — and what should you do. Detection → explanation → impact → action.” | `Diagnose — Health :107`, `overallTone success/warning/danger :111` pill, `Failures requiring action :122` vs `Warnings :149`, per-check `remediation(c):16` cause & action box `:136`, `Section:56` collapsible with count |
| **Host** | `forge/web/app/admin/host/page.tsx:214` | “Live system data from the selected Beacon node — direct daemon proxy. 15 s poll.” | Subtitle honestly states live 15 s proxy `:216`, `useHostQuery :23` timeout 15 s `:27`, tabs use `usageBar :57` consistent bars, `fmtMB/fmtUptime :55-56` shared formatters, node picker top-level |
| **Overview** | `forge/web/components/admin/AdminOverview.tsx:62` | “What is happening across your platform right now.” Inventory first, capacity second, attention third, history last — honest about `configured not live :130`. | Eyebrow `Command Center — Overview :65`, `Live snapshot :73`, 4-col stat grid `Infrastructure/Workloads/Capacity/Access :98-137`, grouped `Attention → affected resources :154-199`, border `border-[var(--line)]` throughout |

### 4.2 Universal rules distilled (ready for lint)

| # | Rule | Enforce | Applies to |
|---|---|---|---|
| R01 | **Eyebrow is mandatory** — every admin page renders `<div text-[11px] uppercase tracking-[0.12em] text-[var(--text-subtle)]>{Section} — {Page}</div>` | `eslint` semantic class check or PR review | All 40+ admin routes |
| R02 | **Subtitle tells truth** — state sampling/edge (e.g. “heartbeat is authoritative; resource usage in Monitoring” `AdminOverview:109`, “direct daemon proxy 15 s” `Host:216`) | PR checklist copy review | Monitoring, Host, Nodes, Servers, any live-poll page |
| R03 | **48px border-y framing** — prose→controls→content use `border-y border-[var(--line)]` with `py-3–6` banding (Monitoring :75+125, Host :231+239, Health :106+120) | Snapshot grep `var(--line)` | Pages with charts/lists |
| R04 | **Semantic tokens only** — no hard `bg-[#hex]` / `bg-white/[0.06]` in new code; use `var(--surface)/--surface-raised/--surface-input/--line/--text/--text-subtle/--success/--warning/--danger` (`globals.css:5`) | ESLint `no-restricted-syntax` for `bg-\[#` | Entire `forge/web` |
| R05 | **Empty states from `states-empty.tsx:17`** — use `EmptyCard` family (10 wrappers), not ad-hoc divs | Grep ext — forbid `No .* yet` literal without `Empty*` import | Apps, Domains, Backups, Services, DNS, Git, Certs, Organizations |
| R06 | **Error states from `states-error.tsx:43`** — use `ErrorAlert/ErrorNotFound/ErrorRateLimit` with `retry` prop, not inline red divs | Idem | Every `QueryError` branch |
| R07 | **Tables are canonical** — `thead border-b text-xs uppercase tracking-wider text-[var(--text-subtle)]` + `tbody divide-y divide-[var(--line)]` + `font-mono text-xs` for values (`AdminHealth:174`, `monitoring:134`, `host:104`) | Grep `tracking-wider` | Host, Health, Monitoring, Servers, Nodes |
| R08 | **Status is a pill, not inline text** — use `ServerStatus/BuildStatus/...` lanes from `states-badge.tsx:55` (not local `span class=`) | Enforce import — no local `stateTone` unless consuming canonical map | All server/build/deployment/cert/DB/verification status |
| R09 | **Single poll cadence per intent** — `5m: 10s`, `longer: 30s`, `host direct proxy: 30s or 10s (processes)`, `deploy pending: 2s-or-5s (pick one)` | Document in `docs/`; enforce shared `queryKey` prefixes | Monitoring, host, deployments |

---

## 5. Detailed Gap Register (UX P0/P1/P2 — Observability/IA only)

> Severity scale: **P0** = operator-misled / appears broken / data-invisible; **P1** = config drift / silent dedup miss; **P2** = discoverability/friction; **P3** = polish/inconsistency.

### 5.1 P0 (must gate any public observability launch)

| # | Title | File:Line | Evidence | Impact | Mitigation sketch (preparation only) |
|---|---|---|---|---|---|
| P0-01 | **Monitoring network chart plots cumulative bytes** | `console-view.tsx:203` `network = rx+tx`, `monitoring/page.tsx:24` `networkRxBytes` as raw bytes vs `%` domain mismatch | Cumulative `rx+tx` will reach `~GB` vs `YAxis domain [0,"auto" :117]` differing from CPU/disk `[0,100]`; network chart dwarfs or is invisible | Derive rate: store previous `rx+tx`, plot `delta/(interval)`; or split `Network RX vs TX` like `ServerNetworkChart`; unify domain logic |
| P0-02 | **Synthetic timestamps on console lines** | `console-view.tsx:299` `new Date().toLocaleTimeString()` per line | Every line stamped at render time — identical ticks hide true chronology; trust-hostile | Use daemon timestamp if WS carries it (`{data, error}` payload attempt `:154`), else show single header `connectedAt :160` with relative age; fall back to no per-line stamp |
| P0-03 | **Monitoring hint says sampled every 15 s but poll is 10 s/30 s** | `monitoring/page.tsx:65` copy “sampled every 15s” vs `metricsQ refetch 10s :46` / `sysQ 30s :33` / `alerts 30s :49` | Copy contradicts implementation; live tick row says `Last point ... Stale after 90s :124` which matches `10s * 9` but not 15 s | Mutate copy to “live · 10 s (stale after 90 s)” for `5m` and soften to 30 s for others; align interval const with telemetry |
| P0-04 | **Backups row busy is global** | `backups-view.tsx:101` `busy = a\|b\|c\|d isPending` used `disabled={busy}` on all 4 per-row buttons `:120,132,139,140` | Locking one backup freezes every other row’s lock/restore/delete UI — operator cannot parallelize or abort one idempotent failure | Per-row pending: key on `backup.name` (e.g. `mutations[backup.name].isPending` with `useMutation` scope per row or `pendingKey` state) |
| P0-05 | **Transfer “dual source” eventual-consistency trap** | `transfer-view.tsx:33-40` poll `fetchServerTransferStatus` vs `server.transferring :79` + `phase/server.transferState :80` | `isTransferring` derives from stale `server` prop while `progress` derives from fresh `transfer` query — user sees “No active transfer” `:161` while progress bar still animates, or cancel disabled when `server` says idle | Canonicalize: `isTransferring = server.transferring \|\| transfer?.transferring`; `phase = transfer?.phase ?? server.transferState`; `progress` null-check already ok |

### 5.2 P1 (activation-blockers)

| # | Title | File:Line | Evidence | Impact |
|---|---|---|---|---|
| P1-01 | **Triple env editor → config drift** (same as FINAL_PARITY UX-09, C11, FL-UX-03) | `env-var-editor.tsx:8` + `AdminAppsShared:106` + `EnvironmentEditor:41`, APIs `env-vars.ts:20` vs `updateApp(391)` | Same label, three models; `Record` upserts silently, `isSensitive` exists only in A, `encrypted` only in C; import/export only in C. Drift between `Configuration` tab (B) and `Environments` page (A) undetectable. | Normalize to one model (`EnvVarResponse[] + isSensitive + version`) + one component (+ import/export from C as bulk helper) |
| P1-02 | **Deployment dual-poller dedup miss** | `deployment-progress.tsx:68` `["deployment-steps",deploymentId]` + `DeploymentTimeline.tsx:25` same key prefix but `2s` vs `5s` + no shared `staleTime` | Two caches, same endpoint, never dedup — doubles load; one uses raw `setInterval` legacy with leak risk if `deploymentId` changes; Dokploy canonical is single `useQuery(refetchInterval:1000)` | Merge to single `DeploymentTimeline`; or share `queryKey` exactly and use `placeholderData` pattern on both |
| P1-03 | **Status tone maps per-file (no single source)** | `apps.ts:392` + `states-badge:55` + `deployment-progress:32` + `DeploymentTimeline:14` | Same logical state renders `blue vs emerald vs amber` per page; compose 9 states inventoried separately (`compose/page.tsx:statusConfig` from C10). New states require N edits. | Centralize to `forge/web/lib/api/status.ts` (currently implicit; create it) consumed by all four lane families |
| P1-04 | **Monitoring node filter not scoped** | `monitoring/page.tsx:35` `queryKey ["node-metrics", period, metric, nodeId]` is scoped — **fixed in this build** — but older `metrics-chart.tsx:73` pattern in subagent-04 reported it wasn’t (`["metrics-chart", period]` dropping `nodeId`). | Ensure no regressions reintroduce bare `["metrics-chart"]` — add lint for key completeness | Document as DONE but guardrail-ize |
| P1-05 | **Gateway 7 pages with single workflow fragmented** | `admin-shell:55` 12 Networking & Security hrefs vs 7 pages; `traffic:14` `RouteRule.targetGroup` never cross-linked from `domains` | Add domain row adds, but cert/headers/route remain undiscoverable — user adds domain then never makes TLS/traffic succeed | Keep pages but add workflow CTA + cross-links (domain row → cert, security, traffic) |

### 5.3 P2 (paper cut / discoverability — ordered by operator-visible first)

| # | Title | File:Line | Prior audit map | Notes |
|---|---|---|---|---|
| P2-01 | **IA 5→8 groups still orphans 8 entries** | `admin-registry:22` 5→`admin-shell:48` 8 | C01 FL-UX-01 (now FIXED) but gap remains | Audit `registry - plan.flatMap(hrefs) = orphans`; add 2nd-level collapse inside Platform Configuration (10 items) |
| P2-02 | **App detail tabs not URL-routed** (`apps/[id]/page.tsx:43` `useState`) | UX-03 C03 FL-UX-04 | `apps/[id]/{compose,git}` already `nested routes`, `{overview,deployments,configuration,logs,console,domains,backups}` still state | `parallel routes / intercepting` or `/(tabs)` segment; push tab to `searchParams` with `router.replace` |
| P2-03 | **Docker node-blind** | UX-11 C10 | Should add `NodeSelect` like monitoring/host | `listContainers` already returns `nodeId/nodeName`, just filter client-side like `containers-view:82` search filter |
| P2-04 | **Gateways scattered** | UX-12 C15 | `admin-shell:55` clusters but pages don’t | Add nav badges cross-links, not IA re-architecture yet |
| P2-05 | **Logs not under deployment timeline** (modal only) | UX-07 C08 | `DeploymentLogViewer.tsx:37` not mounted in `apps/[id]/deployments-tab` | Inline `LogViewer` under `DeploymentsTab:263` inline for active deploy |
| P2-06 | **Domains gated `!!serverFilter`** | UX-08 | `domains:57` | Default aggregated or auto-select first server |
| P2-07 | **Tenancy cascade display-only** | UX-10 C12 | Org→project→env cascade but `fetchApps()` global | Scope post-IA work; for observability phase just surface `selected env == app.envId mismatch` hint |
| P2-08 | **Env `RECORD` key input disabled** (rename requires delete+add) | C11 | `AdminAppsShared:153` `onChange={() => {}}` | Unify to A (row editor supports rename via delete+confirm?) or enable edit |
| P2-09 | **Translation double-fallback hides gaps** | C17 | `TranslationProvider:19` | Dev `console.warn(value===key)` (P4 silent — deprioritize) |

---

## 6. Design Token Locations — Map & Gap

### 6.1 Canonical definition

```
forge/web/app/globals.css:5  -- tokens (13)
forge/web/app/globals.css:30 [data-theme="light"] alt tokens
forge/web/app/globals.css:71 @layer components .ui-* primitives mapping to tokens
forge/web/components/admin/admin-ui.tsx:15  .Pill tones map to tokens implicitly
```

### 6.2 Correct consumers (4 exemplars) — green list

| File | Pattern |
|---|---|
| `forge/web/components/admin/AdminOverview.tsx` | `border-[var(--line)]`, `text-[var(--text-subtle)]`, `bg-[var(--surface)]` exclusively; `bg-white/[0.02]` only as deliberate subtle wash |
| `forge/web/app/admin/monitoring/page.tsx` | Same + `bg-[var(--surface)]` card, progress `bg-[var(--surface)]` controls with `border-[var(--line)]` |
| `forge/web/components/admin/AdminHealth.tsx` | Idem + `border-amber-500/30` semantic (amber coupled to `--warning` not tokenized yet — ok as accent) |
| `forge/web/app/admin/host/page.tsx` | Idem, tab `border-[var(--text)]` active vs `border-transparent text-[var(--text-subtle)]` inactive |

### 6.3 Hard-coded color hotspots (to be codified)

These are **not blockers for this audit** but are the mechanical universalization targets — each will be a single-line `bg-[#…] → var(--…)`.

| File | Line(s) | Hard-coded | Canonical replacement |
|---|---|---|---|
| `forge/web/components/server/backups-view.tsx:89` | `border-white/[0.08] bg-[#1e2536]` 3× | `border-[var(--line)] bg-[var(--surface)]` |
| `forge/web/components/server/backups-view.tsx:183` | `bg-[#1e2536] px-4`, `border border-white/[0.1] bg-[#0f141f]` | `bg-[var(--surface)]`, `bg-[var(--surface-input)] border-[var(--line)]` |
| `forge/web/components/server/console-view.tsx:267` | `border-white/[0.07] bg-[#151b27]` chart tiles + `bg-[#060a11]` console chrome | `border-[var(--line)] bg-[var(--surface)]` + `bg-[var(--surface-raised)]` or keep `#060a11` as intentional terminal chrome with comment |
| `forge/web/components/admin/admin-shell.tsx:119` | `bg-[#0a0e14]` canvas, `bg-[#0f1520]` header, `bg-[#11161f]` sidebar all explicit | `bg-[var(--canvas)]`, `bg-[var(--surface)]`, etc. (shell is currently exempt — decide to keep as branded shell or tokenize) |
| `forge/web/components/docker/containers-view.tsx:111` | `border-white/[0.06] bg-[#161b28]` table header, `bg-[#1a1f2e]` modal at `:217`, etc | `border-[var(--line)] bg-[var(--surface-raised)]` |
| `forge/web/components/environment/EnvironmentEditor.tsx:110` | `border-white/[0.06] bg-[#111722]` chrome, `bg-[#0d131d]` inputs | `border-[var(--line)] bg-[var(--surface)]`, `bg-[var(--surface-input)]` |
| `forge/web/components/admin/AdminAppsShared.tsx:28` (port/volume maps) | inputs `bg-[#161b28]` inline | `bg-[var(--surface-input)]` |
| `forge/web/app/admin/traffic/page.tsx:176` | `border-white/[0.06]` / `bg-[#161b28]` rows | `border-[var(--line)] / bg-[var(--surface-raised)]` |

**Decision point for shell tokens:** `admin-shell.tsx` shell colors (`#0a0e14` canvas vs `#11161f` sidebar vs `#0f1520` header) are 1shade apart intentional layering. Two stances: (a) keep as branded three-layer chrome with comment + map to new `--nav` token (like `web/app/globals.css:5 --nav:#0d1320`), or (b) flatten to canonical `canvas/surface/surface-raised`. Recommend **(a)** — add ` --nav / --nav-line` to `forge/web/app/globals.css:5` matching `web` legacy but tokenized; keeps sidebar distinct height sense while lint clean.

### 6.4 Token gaps (needed before bulk mechanize)

| Token | Current | Need | For |
|---|---|---|---|
| `--nav` + `--nav-line` | missing in `forge/web` (exists in `web/app/globals.css:5` as `--nav:#0d1320`) | Add to `forge/web/app/globals.css:5` dark/light block | Shell header+sidebar chrome |
| `--surface-hover` | implicit via `hover:bg-white/[0.06]` everywhere | Add ` --surface-hover: rgba(255,255,255,0.04)` dark, `rgba(0,0,0,0.04)` light | Table rows, nav hover |
| `--brand-subtle` | implied via `bg-red-500/10` etc | Add `--brand-subtle: rgba(220,38,38,0.10)` | Active nav pill |
| `--focus-visible-ring` | per-file `focus:ring-red-500/15` | Map to `--focus: #fb7185` already present `:21` + light `:42` | All inputs; already token exists — just enforce use |

---

## 7. Component Inventory to Unify — Single Writers & Scope of Fix

> The task calls out: **“Component inventory to unify (ServerStatus two-dot State Lanes, DeploymentTimeline single, LogViewer inline)”** — this section is that inventory.

### 7.1 ServerStatus / State Lanes (two-dot vs pill divergence)

**Current primitives:**

| Primitive | File | Enumerates | Visual | Consumers |
|---|---|---|---|---|
| `ServerStatus` (badge still) | `forge/web/components/shared/states-badge.tsx:77` | 7 `running/stopped/installing/suspended/offline/transferring/errored` + icon+tone+optional pulse | `StatusBadge` pill `border-*/bg-*/text-*` + icon | `app-detail:66`, `dev/states`, others |
| `DeploymentStatus` | `states-badge:114` | 5 `pending/deploying/deployed/rolled-back/failed` | same pill | `apps/[id]` deployment modal |
| `BuildStatus` | `states-badge:96` | 6 `running/succeeded/failed/canceled/abandoned/pending` | same pill | builds |
| `CertStatus` | `states-badge:132` | 5 `valid/expiring/expired/issuing/failed` | same pill | certificates |
| `DBStatus` | `states-badge:157` | 5 `provisioning/running/backing-up/stopped/error` | same pill | db hosts |
| `VerificationStatus` | `states-badge:173` | 3 `pending/verified/failed` | same pill | domains |
| `statusTone` | `forge/web/lib/api/apps.ts:392` | App 9 `running/stopped/deploying/failed/...` | `Pill tone=green/red/...` via `AdminAppsShared:30` | `apps/page.tsx:35`, `apps/[id]` tabs |
| `ServerStatusBadge` (admin servers table) | `forge/web/components/admin/AdminServers.tsx:333` | `healthy/unhealthy/offline` heartbeat | inline pill variant — third vocabulary | admin `Servers` inventory |

**Also present (requested “two-dot State Lanes” semantics in the task):** the 6 heartbeat states from `heartbeatmonitor/service.go:266` (`healthy/suspected/unreachable/offline/recovering/reconciling`) are rendered in `AdminOverview:42` (`offline/degraded` filter) + `AdminHealth:84` (`healthyNodes/degradedNodes/offlineNodes` derived) but **no single** `StateLane` primitive exposes the mapping — each page recomputes predicates locally.

**Intended single writer (preparation spec):**

- Create `forge/web/components/shared/state-lanes.tsx` (naming proposal) exporting one family:
  - `StateLane` base (dot+label+severity: `strong/soft/alert/idle`), used by `ServerStatus` and `NodeHealth` alike.
  - `HeartbeatLane` preset that maps `heartbeatmonitor:266` 6 states onto `StateLane` with single `heartbeatToneMap` consumed by `AdminOverview:42`, `AdminHealth:83`, `AdminServers:333` (eliminating the `healthy/unhealthy/offline` divergence).
  - `WorkloadLane` preset for `ApiServer` states.
  - `DeploymentLane` preset for deployment step vs deployment record (resolves `apps.ts:392` vs `states-badge:102` enum mismatch).
- `states-badge.tsx:55` becomes shim re-exports for compat; per-file `statusConfig` copies in `deployment-progress:32`/`DeploymentTimeline:14`/`compose/page.tsx:statusConfig` become **consumers** of the single map.
- Risk: signature change breaks 5 call sites — additive shim phase first, removal later.

**Files to touch (unify lane):**

```
forge/web/components/shared/states-badge.tsx:55        (shim → forward)
forge/web/lib/api/apps.ts:392                           (share tone map)
forge/web/components/app/deployment-progress.tsx:32     (drop local map)
forge/web/components/charts/DeploymentTimeline.tsx:14   (drop local map)
forge/web/app/admin/compose/page.tsx:statusConfig       (consume)
forge/web/components/admin/AdminOverview.tsx:41         (consume HeartbeatLane)
forge/web/components/admin/AdminHealth.tsx:81           (consume)
forge/web/components/admin/AdminServers.tsx:333         (consume + rename)
forge/web/components/admin/AdminNodes.tsx:??            (same)
forge/web/app/servers/page.tsx:20                       (inline ServerStatus → shared)
forge/web/app/console/servers/page.tsx:21               (same)
```

### 7.2 DeploymentTimeline — single poller (2s vs 5s)

**Current duality:**

| File | Interval | QueryKey pattern | `fetchDeploymentSteps` call site | Style | Leak risk |
|---|---|---|---|---|---|
| `forge/web/components/app/deployment-progress.tsx:46` | `2000 ms` constant `41` + `MAX_POLL 10m` cap | `["deployment-steps", deploymentId] :68` (react-query) but `placeholderData` prev, `staleTime 1s`, `gcTime 5m` | `36 fetchDeploymentSteps` with `{signal}` | Progress bar `pct=completed/total*100 :105` + expandable error `:188` + degraded `onComplete/onError` via `prevStatusRef` | `useRef startTimeRef:50`, no `AbortSignal` leak but old `setInterval` legacy removed — okay |
| `forge/web/components/charts/DeploymentTimeline.tsx:27` | `5000 ms if hasActive else false` | same `["deployment-steps", deploymentId] :25` — identical prefix, **would dedup if intervals aligned**, but 2s vs 5s means stagger | `26 fetchDeploymentSteps(deploymentId)` bare (no signal) | Vertical timeline with connector line `:84`, icons `completed/in_progress/failed/...` `:14` | none, but no `refetchIntervalInBackground:false` unlike progress |

**Single-writer decision:**

- **Winner: `DeploymentTimeline` variant** — it has the visual of choice (connector line `absolute left-[11px] top-7 bottom-0 w-px bg-white/[0.08]` :85, `statusConfig` with `bg` wash `:14`, skeleton `isLoading :35`/`isError:54`/empty `63`) and a lighter poll interval (5 s → less write amplification on `operations` table behind `GET /admin/deployments/:id/steps`).
- Merge `DeploymentProgress` affordances into it:
  - progress `%` (from `:104` math) as optional `showProgressBar` prop
  - expandable error (from `:188`)
  - terminal handler that `onComplete/onError` idempotently (from `:88-101` stable ref pattern) — keep that callback contract so `apps/[id]/page.tsx:263 DeploymentsTab` doesn’t rebind

**Files to touch (timeline single):**

```
forge/web/components/app/deployment-progress.tsx:46     (deprecate → shim that re-exports DeploymentTimeline with progress bar enabled)
forge/web/components/charts/DeploymentTimeline.tsx:23   (add props: showProgress?, onComplete?, onError?, refetchInterval? override)
forge/web/lib/api/deployments.ts:16                    (no change — endpoint already canonical)
forge/web/app/admin/apps/[id]/page.tsx:263            (consume single timeline)
forge/web/app/admin/deployments/page.tsx               (consume single timeline)
forge/web/app/admin/preview-deployments/page.tsx       (same)
forge/web/app/admin/source-deployments/page.tsx        (same)
tests: forge/web/app/admin/apps/[id]/__tests__ or spec — add dedup test: two mount points assert single fetch
```

**Interval decision post-merge:** default `5000 ms` (DeploymentTimeline current) with escalation `2000 ms` only while `in_progress` (poll adapts) — satisfies both prior consumers.

### 7.3 LogViewer inline (modal vs inline)

**Current primitives:**

| Viewer | File | Data source | UX spot | Reconnect |
|---|---|---|---|---|
| `DeploymentLogViewer` | `forge/web/components/deployment/DeploymentLogViewer.tsx:37` | WS `wsUrl: string | ()=>Promise<string>` ticketed (`apps.ts:359`), `MAX_RECONNECT 20`, exponential backoff, `autoScroll:12`, `search stripAnsi`, `level colors`, `line numbers+timestamps` | modal inside `apps/[id]/page.tsx` deployment detail **modal `:366` `log: string`** only — not on page | `onMessage push + MAX_LINES` pattern but not visible inline |
| `LogViewer` | `forge/web/components/admin/AdminAppsShared.tsx:28` | `AppLogEntry[]` + poll `fetchAppLogs every 5s` in `LogsTab:460`, search, auto-scroll, download `Blob .log` | `Logs` tab **polling**, not WS | poll |
| `ServerMemoryChart` family | `forge/web/components/charts/*` + `monitoring/*` | SSE/poll JSON | Global monitoring | interval |

**Target:** inline live deployment logs under `DeploymentTimeline` when a deployment is active — exactly the split noted in `subagent-04 C08` and `FINAL_PARITY §16 UX-07`.

**Wiring plan (preparation):**

- `DeploymentTimeline` gains optional slot `logViewer?: ReactNode` or `wsUrl?: string` — when `deploymentId` has `in_progress` steps, the surrounding `DeploymentsTab` passes the **active deployment’s WS url** (`ws://… /admin/deployments/:id/ws/logs` or beacon `backupProgressWS:2155`-like) into `DeploymentTimeline` + `DeploymentLogViewer` co-rendered inside `Card :73`.
- `apps/[id]/page.tsx` DeploymentsTab flow becomes:
  1. `DeploymentTimeline rows` (pending → completed)
  2. **below** connector, `<DeploymentLogViewer wsUrl={activeDeploymentWs} height=300 />`
  3. static `log: string` in modal is preserved as fallback when WS is unavailable (firewall/proxy stripped WS).
- `DeploymentLogViewer` itself needs no refactor — it already handles `wsUrl` factory pattern; just mount it outside the modal.

**Files to touch (LogViewer inline):**

```
forge/web/components/deployment/DeploymentLogViewer.tsx:37  (no code change — reuse)
forge/web/components/admin/AdminAppsShared.tsx:28          (keep polling LogViewer for runtime logs tab — distinct use case; do not merge WS variant)
forge/web/app/admin/apps/[id]/page.tsx:263                 (DeploymentsTab — render DeploymentTimeline + inline logViewer for in_progress)
forge/web/components/charts/DeploymentTimeline.tsx:1        (accept children/slot API)
forge/web/lib/api/deployments.ts                           (add helper fetchDeploymentLogStream? optional)
forge/web/lib/api/ws/websocket-manager.ts                   (already generic — reuse)
```

---

## 8. Additional Inventories for Near-Term Activation (§20 pointers reused)

These were built and are activation-ready — surfaced here as “keep in sight while unifying observability/UX”.

| Prio | Capability | Where | Why keep visible |
|---|---|---|---|
| P0-adj | Env `env_file/include` silently dropped | `compose/service.go:14/92` vs `beacon/compose:213` `["80"]` privileged-port bypass | `§20 #26-27` — must fail-fast before token bulk-migration |
| P0 | Deployment stubs `execution.go:249` report `completed` | Same domain as timeline issues | Same fix path as P1-02 |
| P1 | Beacon `backupProgressWS:2155` never proxied | `beacon/server.go:1995 realtimeProxy only stats|logs|console` | `§20 #41` — surface per-backup progress in `backups-view.tsx` after timeline unifies (re-wire `backups-view:41 pagination` to show per-backup bar) |
| P1 | Offline banners duplicated | `admin-shell:365` vs `shared/states-offline:6` vs per-page `OfflineBanner` on host/monitoring/traffic/apps | Use global `admin-shell:365` single writer; convert per-page `OfflineBanner` to `AdminLoadingState` context variant that hides when shell already shows offline |
| P1 | RateLimit dead component | `states-error:164` vs `http.ts:102` | Wire 429↔`ErrorRateLimit` + backoff header → `Countdown` prop |
| P2 | Filters show-only | Tenancy `Pill Protected`/mount allowlist display-only | Guard first but UX hint `Pill yellow isLocked :116` — same pattern: display-only pill must become disabled control with tooltip |

---

## 9. Files to Touch — Consolidated Touch List (ordered by dependency; no code in this audit)

**Phase A — token foundation (must precede all UI unifications):**

```
forge/web/app/globals.css:5                      add --nav/--nav-line/--surface-hover/--brand-subtle; optionally --focus variant docs
forge/web/components/admin/admin-ui.tsx:61       Card/AdminCard should map to var(--surface) not implicit ui-card alias; ensure Pill variants map to tokens explicitly
forge/web/lib/utils.ts (cn helper)               no change but confirm Tailwind safelist for var(--*) not purged (already fine)
```

**Phase B — state lane single writer:**

```
forge/web/lib/api/status.ts                      CREATE — centralize all tone maps (apps.ts:392 + states-badge:64 et al.) into one importable module (or keep in apps.ts but re-export via lib/api/status.ts for compatibility with task brief)
forge/web/components/shared/states-badge.tsx:55  turn into shim forwarding to state-lanes.tsx; keep exports compat
forge/web/components/shared/state-lanes.tsx      CREATE (see §7.1)
forge/web/components/app/deployment-progress.tsx:32  consume shared config
forge/web/components/charts/DeploymentTimeline.tsx:14 consume shared config
forge/web/app/admin/compose/page.tsx            consume shared config
forge/web/components/admin/AdminOverview.tsx:41  consume HeartbeatLane
forge/web/components/admin/AdminHealth.tsx:81    consume HeartbeatLane
forge/web/components/admin/AdminServers.tsx:333  consume
forge/web/components/admin/AdminNodes.tsx       similar
forge/web/app/servers/page.tsx:20               remove local ServerStatus
forge/web/app/console/servers/page.tsx:21       remove local ServerStatus
```

**Phase C — DeploymentTimeline single + LogViewer inline:**

```
forge/web/components/charts/DeploymentTimeline.tsx:23  extend props (showProgress, onComplete, onError, children/logSlot)
forge/web/components/app/deployment-progress.tsx:46    deprecate to shim
forge/web/app/admin/apps/[id]/page.tsx:263            DeploymentsTab inline wiring
forge/web/app/admin/deployments/page.tsx              timeline consume
forge/web/app/admin/preview-deployments/page.tsx
forge/web/app/admin/source-deployments/page.tsx
forge/web/components/deployment/DeploymentLogViewer.tsx:37  consumers only
forge/web/lib/api/deployments.ts:16                   optionally add stream URL helper
```

**Phase D — env editor convergence (separate track but highest P1, must not conflict with A–C):**

```
forge/web/components/environment/env-var-editor.tsx:8   winner (keep + augment with C's import/export + Coolify flags isMultiline/isLiteral/isBuildtime/isRuntime/comment)
forge/web/components/admin/AdminAppsShared.tsx:106        deprecate (replace imports with winner, keep ENV_KEY_REGEX:106 inside winner)
forge/web/components/environment/EnvironmentEditor.tsx:19  become bulk helper wrapper around winner
forge/web/lib/api/env-vars.ts:6                         remove overload; explicit scoped `fetchEnvVarsByScope("environment",id)` and legacy `fetchEnvVars(envId)` shim
forge/web/app/admin/environments/page.tsx:1              consume winner
forge/web/app/admin/apps/[id]/page.tsx:393 ConfigurationTab  consume winner (remove Record state)
```

**Phase E — Node-blind + gateways scattering (P2/P3, post-P0):**

```
forge/web/app/admin/docker/page.tsx:17            add NodeSelect filter (reuse host:209 / monitoring:89 pattern)
forge/web/components/docker/containers-view.tsx:43 add node filter prop + grouping by node
forge/web/app/admin/domains/page.tsx:57          remove `enabled: !!serverFilter` gate
forge/web/app/admin/traffic/page.tsx:62          deduplicate OfflineBanner, add domain→cert header cross-link
forge/web/app/admin/{load-balancer,domains,endpoints,firewall,security,certificates,mtls,social}/page.tsx  cross-link layer (hrefs minimal)
forge/web/components/admin/admin-shell.tsx:55     (no change, but document cluster in comment; Phase II would collapse if product decides)
```

**Phase F — Observability correctness (P0 # P0-01–05):**

```
forge/web/components/server/console-view.tsx:299  fix synthetic timestamps (use daemon time if available)
forge/web/components/server/console-view.tsx:203  fix network cumulative → rate (delta)
forge/web/app/admin/monitoring/page.tsx:65        fix copy 15s↔10s/30s copy-interval alignment + YAxis domain for networkRxBytes vs %
forge/web/components/server/backups-view.tsx:101  per-row busy
forge/web/components/server/transfer-view.tsx:33  dual-source canonicalize
```

**Do NOT touch (out of scope or correctly isolated):**

```
forge/api/internal/services/heartbeatmonitor/service.go:266  — classifier correct + best-in-corpus; no change
forge/api/internal/services/cronjob/service.go               — dispatcher isolation is defense-in-depth; no UX change
forge/api/internal/services/observability/service.go         — allocation-based sampling is intentional until Beacon live OS collectors land (document, don’t revert)
infra/*, beacon/*, gateway adapters, LB/Traefik wiring       — governance: UX audit does not touch networking P0s; reference FINAL_PARITY §8 grounding
```

---

## 10. Design Token Locations Reference (copy-paste ready for engineers)

**File:** `forge/web/app/globals.css:5` — dark block defines `13` tokens; `:30` light block defines inverted pair.

| Token | Dark `forge/web/app/globals.css:5` | Light `:30` | Usage in exemplars |
|---|---|---|---|
| `--brand` | `#dc2626` | inherited | primary buttons, attention |
| `--brand-hover` | `#ef4444` | — | button hover |
| `--brand-dark` | `#991b1b` | — | scrollbar |
| `--canvas` | `#0a0e16` | `#f4f7fb` | page background — `overview:63` hero band, `shell:119` canvas |
| `--surface` | `#111722` | `#ffffff` | cards `overview:141`, monitoring chart `110` |
| `--surface-raised` | `#171f2d` | `#eef2f7` | modal/dialog `globals.css:103 .ui-dialog` |
| `--surface-input` | `#0d131d` | `#ffffff` | inputs `ui-input :84` |
| `--line` | `rgba(148,163,184,.14)` | `rgba(15,23,42,.13)` | borders `overview:64 border-[var(--line)]` |
| `--line-strong` | `rgba(..., .25)` | `(.24)` | focus input border `ui-input :84` |
| `--text` | `#f1f5f9` | `#0f172a` | body `color:var(--text)` |
| `--text-subtle` | `#94a3b8` | `#475569` | eyebrow, hint, header `overview:65 text-[var(--text-subtle)]` |
| `--focus` | `#fb7185` | `#dc2626` | `:focus-visible outline :59` |
| `--success / --warning / --danger` + `--*-subtle` | `059669 / d97706 / dc2626` + 12% | `047857 / b45309 / dc2626` + 10% | success/warning/danger subtle backgrounds `alerts` `AdminHealth:131` |
| _(proposed)_ `--nav` / `--nav-line` | none yet (add) | mirror `web/app/globals.css:5 --nav:#0d1320 --nav-line:rgba(..., .12)` | `admin-shell:119 header/sidebar chrome` |
| _(proposed)_ `--surface-hover` | none yet | add | `hover:bg-white/[0.06]` → `hover:bg-[var(--surface-hover)]` |

**Tailwind bridge:** tokens are consumed via `border-[var(--line)]`, `bg-[var(--surface)]`, `text-[var(--text-subtle)]` etc — no runtime plugin required; behavior noted in `globals.css:7` comment.

---

## 11. Risks & Mitigations

| Risk | If ignored | Mitigation (scope-limited) | Files at risk |
|---|---|---|---|
| **Two deployment polls diverge further** adding `FetchDeploymentLogs` call from a third place → triple load | `operations` table noisy, stale UI when 2 s poll wins over 5 s timeline which freezes while still `pending` | Make `DeploymentTimeline` sole writer; keep single `refetchInterval:5000` (adaptive 2 s during `in_progress`); add `queryKey` dedup test | `deployment-progress:46`, `DeploymentTimeline:27`, `lib/api/deployments:16` |
| **State lane tones re-fragment** if `states-badge` shim is removed before consumers migrate | 5+ maps reappear locally (compose 9-state page inventoried will hard-fork) | Keep `states-badge.tsx` re-exports shim phase; add ESLint `no-restricted-syntax` forbidding `const statusConfig: Record<string, { icon` locally outside lane primitive | `states-badge:55`, `AdminAppsShared:32`, `compose/page.tsx` |
| **Env editor convergence blocks deployment fix** (both touch `apps/[id]/page.tsx`) | `ConfigurationTab` refactor conflicts with `DeploymentsTab` inline-log wiring | Branch independently; `apps/[id]/page.tsx:263` DeploymentsTab owns cols 1–2; `:393` ConfigurationTab owns cols 3–4 — merge order irrelevant | `apps/[id]/page.tsx:263` vs `:393` |
| **Token universalize regresses dark legibility** if `--line` is mistakenly increased globally | `host/page.tsx:57 usageBar` red/amber/green vs `var(--line)` background dims incorrectly | Keep `globals.css:15 --line .14` / `.25` unchanged; token universalize replaces hard `bg-[#…]` with mapped var — no contrast change expected; verify 4 exemplars already legible | `globals.css:5` only |
| **WS logs leak behind firewalls** (`DeploymentLogViewer :37` WS upgrade fails) | Modal fallback `log:string` gone after inline wiring | Keep fallback — modal `log: string` read-path remains even after inline WS mounts; logViewer has clear fail branch `isError` already | `DeploymentLogViewer:46` |
| **Shell chrome defacement** if `--canvas/--nav` flatten collapses 3 layers | Side/top/nav contrast lost | Keep three distinct values (proposed `--nav` token) and document intent; visual diff before merging | `admin-shell:119` |
| **Heartbeat classifier drift** (`heartbeatmonitor:266`) diverges from UX lane predicates | `AdminOverview:42`/`AdminHealth:83` undercount offline if threshold constants hard-fork | Lane predicate must import same constants (`Warning 30s / Offline 90s / Unavailable 300s / Recovery 2`) from API types or document `# TODO heartbeatmonitor drift guard` with test mirroring classifier table | `heartbeatmonitor:89` ↔ `states-badge`/`state-lanes` |
| **Observability OOM** if `Store.CreateNodeMetric` cadence rises (sampling guard `observability/service.go:198 rand.Intn(6)`) | DB write amplification — “60 inserts/s → 10/s” guard defeated by extra writers | Keep sampling policy visible in both `server.go` handler and `RecordNodeHeartbeat:198`; do not add third writer in token pass | `observability/service.go:198`, `internal/http/server.go` metrics handler |
| **Metrics `period` drift thrash** (caller computes `since: new Date().toISOString()` per render) | Cache miss every render (reported `FINAL_PARITY §16` as `periodWindow` local compute) — currently FIXED in `monitoring/page.tsx:37` `map` inside `queryFn` determinized via `period` but snapshot test must still guard | No writer adds second `getNodeMetrics` caller without shared `metricWindow` helper (`monitoring.ts:15`) | `monitoring:37` |

---

## 12. What Was Not Audited (out-of-scope deferred)

* **Gateway write-path correctness** (five-writer conflict, fictional Caddy handlers `rate_limit`, empty-sync every 30 s, cert delivery `SetCertificate`, HTTP-01 `HTTPSolver` not mounted) — covered exhaustively in `FINAL_PARITY_AUDIT.md §8` and `phase-02/phase-03/networking` remediations; this UX audit did not re-probe gateway handlers beyond IA cross-links.
* **Backup encryption/sidecar/KDF** (per-blob HKDF+GCM, `beacon/local.go:757` SHA, `retention.go:61` AND-vs-OR) — `FINAL_PARITY §9` remains authority; UX preparation only notes the per-backup progress WS end-to-end wire (`§8` `backupProgressWS:2155` not proxied at `server.go:1995`) as inline display after backups-view busy fix.
* **Queue/operation duality, leader election, fencing, overlay mesh** — `FINAL_PARITY §10`; not re-audited for UX except heartbeat classifier lane mapping.
* **Auth/session/CSP/mTLS/XFF attack surfaces** (`SE-*`) — defer to security audit.

---

## 13. Bring-Forward Items (verbatim from brief to confirm coverage)

The brief asked this audit to address each bullet; status:

| Bullet | Found | Where in this doc |
|---|---|---|
| `admin-registry.ts:22 (5 groups ~62 → 6 goal groups)` | `AdminNavGroup` 5×62 documented; plan 8×54 current | `§2.1`, `§5.3 P2-01`, `§9 Phase E` |
| `admin-shell.tsx:42 plan` | 8-group plan currently 54 hrefs | `§2.1` |
| `AdminOverview.tsx:86 + monitoring/page.tsx:1` | Both inventoried as exemplars + quirks (period copy, network domain) | `§2.2`, `§4`, `§5.1 P0-01/03` |
| `apps/[id]/page.tsx:28 useState not routed` | `useState<TabId>(searchParams.get("tab") …)` never writes back | `§2.3`, `§5.3 P2-02` |
| `deployment-progress.tsx:46 2s vs DeploymentTimeline:27 5s` | 2 s constant vs 5 s adaptive; duplicate key | `§2.3`, `§7.2`, `§5.2 P1-02` |
| `env-var-editor triplicate` | A/B/C catalog with data-model divergence table | `§2.4`, `§5.2 P1-01`, `§9 Phase D` |
| `Docker node-blind` | `docker/page.tsx:17` no filter, `containers-view:43` flattens global | `§2.5`, `§5.3 P2-03` |
| `Gateways scattered 7 pages` | 7 pages distinct, 12 hrefs clustered in shell | `§2.5`, `§5.3 P2-04` |
| `status.ts tone maps` | `apps.ts:392` + `states-badge:55` 7 families + per-file `statusConfig` divergence | `§2.8`, `§7.1`, `§5.2 P1-03` |
| `states-empty.tsx:17` | `EmptyCard + 10 semantic wrappers` | `§2.9` |
| `console-view.tsx:299 synthetic timestamps + network cumulative` | `new Date().toLocaleTimeString()` per line + `rx+tx` cumulative | `§2.10` `P0-01/02` |
| `backups-view.tsx global busy` | `busy = a\|b\|c\|d isPending` per-row disable global | `§2.10` `P0-04` |
| `transfer-view.tsx dual source` | poll `fetchServerTransferStatus` vs prop `server.transferring` | `§2.10` `P0-05` |
| `cronjob` | `executeJob:117`, `dispatchServerCommand:244` correctly fail-closed | `§2.6` |
| `heartbeatmonitor/service.go:266 6-state classifier` | 6 states documented with thresholds `89` + lane mapping gap | `§2.6`, `§7.1`, `§11` drift guard |
| `observability` | allocation-sampling dual-source (handler 1/6 + service random 1/6) | `§2.6` |
| `metrics/middleware_metrics.go` | no `metrics/` middleware package on disk; observability collector `metrics_collector.go:41` covers system metrics; API metrics live under `observability` not standalone middleware | `§2.6` table last row |
| `forge/web/design tokens (globals.css)` | `forge/web/app/globals.css:5` 13 tokens + `ui-*` raw + proposed `nav` tokens | `§2.7`, `§6`, `§10` |
| `FINAL_PARITY §16 UX 14 rows` | delta table 14 rows preserved | `§3.1` |
| `phase-01 subagent-04 17 comps` | scoreboard 17 comps + 6 FL-UX findings re-verified | `§3.2` |
| `universalize design philosophy (refer monitoring, health, host, overview)` | 4 exemplars distilled into 9 lint-able rules | `§4` |

---

## 14. Definition-of-Done Checklist (for the implementation phase)

Before any PR merging the fixes above, assert:

- [ ] Every admin route that existed before this audit renders the same count/placement of controls — only `var(--*)` substitution and lint-visible tokens changed (`§6.3` hotspot table).
- [ ] `DeploymentTimeline` single writer: with one `mounted` `DeploymentTimeline` + one legacy shim `DeploymentProgress` both present, `GET /admin/deployments/:id/steps` is fetched at most once per `refetchInterval` (React Query dedup + identical `queryKey`); no concurrent 2 s + 5 s poll observed (add `__tests__/timeline-dedup.test.tsx`).
- [ ] Live `DeploymentLogViewer` mounts under the timeline **without removing** the existing modal fallback (safe for proxy-stripped WS) — per `§7.3`.
- [ ] `states-badge.tsx:55` remains the **only** place defining status→icon→tone mappings; grep `statusConfig: Record<string, { icon` returns 0 outside `state-lanes.tsx` + shim.
- [ ] `forge/web/app/globals.css:5` adds `nav` family comments and no token value changes — contrast pass visual.
- [ ] `fetchDeploymentSteps` is still the single API import for both timelines (no `fetchAppDeployments` confused with deployment step API) — per `deployments.ts:16` canonical import count `2`.
- [ ] Env editor convergence has a **feature-flag or path gate** (e.g. `apps/[id]/page.tsx` ConfigurationTab consumes new `EnvVarEditor` behind `NEW_ENV_EDITOR=true` until `Record` writes are rolled over) so config drift isn’t re-introduced mid-migration.
- [ ] No `bg-[#` remains in files listed in `§6.3` except documented intentional `console bg-[#060a11]` terminal chrome with inline comment `/* intentional terminal chrome, not surface */`.

---

## 15. Open Questions for Maintainers

1. **Shell chrome token decision** — keep three distinct `#0a0e14/#11161f/#0f1520` as branded layers (add `--nav/--nav-line` tokens) or flatten to canonical tiers per `R04`? Current audit assumes (a) preserve brand layers, but flatten is a single-line change if desired.
2. **Observability live-OS metrics** — Are `CPULoad/Network/Speed` beacons pluggable per node? If the TODO at `AdminHealth:395-446` typed guards for `heapAllocMb`/`goroutines` is actioned via `beacon/sysinfo_linux.go:94` → `store_heartbeat`, then `monitoring/page.tsx:51 isSynthetic` guard can be downgraded to `alert` but not removed. Confirm intent before reducing the honest banner.
3. **Tenancy gating depth** — Should `fetchApps()` remain global during observability activation (Phase E says no), or is an alert `@APPLIES_TO` metadata field expected first (`domain.PlacementRequest:114` tenant-less core path)? UX audit assumed **UI-scoped filter first, DB/gating second**.
4. **Domain workflow depth** — Is traffic/route policy attachment expected to remain tenant-scoped `POST /admin/traffic/rules` (`FINAL_PARITY §8` NG-09 all-policies-all-routes still broken)? The cross-link work (Phase E) can either link to existing pages unchanged or wait for the gateway single-writer gate — confirm sequencing.

---

## 16. Source Directory Tree (for repeatability)

```
forge/web/
  app/
    globals.css:5                         ← canonical design tokens
    admin/
      monitoring/page.tsx:1               ← Monitoring exemplar
      host/page.tsx:1                     ← Host exemplar
      apps/[id]/page.tsx:28               ← detail tabs useState defect
      docker/page.tsx:17                  ← node-blind
      traffic/page.tsx:14                 ← one of 7 gateway pages
  components/
    admin/
      admin-registry.ts:22                ← 5 groups ~62
      admin-shell.tsx:42                  ← goal re-group
      AdminOverview.tsx:30                ← Overview exemplar
      AdminHealth.tsx:70                  ← Health exemplar
      AdminAppsShared.tsx:106             ← env editor variant B
      admin-ui.tsx:61                     ← Card/Pill/Btn legacy family
    server/
      console-view.tsx:299                ← synthetic timestamp + network cumulative
      backups-view.tsx:101                ← global busy
      transfer-view.tsx:33                ← dual source
    deployment/
      DeploymentLogViewer.tsx:37           ← WS viewer
    environment/
      env-var-editor.tsx:8                ← variant A
      EnvironmentEditor.tsx:41             ← variant C
    shared/
      states-empty.tsx:17                  ← 10 semantic empties
      states-error.tsx:43                  ← 5 error states (RateLimit dead)
      states-badge.tsx:55                  ← 7 lane families + statusBadge
      states-offline.tsx:6                 ← OfflineBanner (duplicated)
    charts/
      DeploymentTimeline.tsx:23            ← 5 s timeline
      Server{CPU,Memory,Disk,Network}Chart:*  ← per-metric charts
  lib/api/
    apps.ts:392                            ← statusTone / deploymentStatusTone
    deployments.ts:16                      ← fetchDeploymentSteps canonical
    env-vars.ts:20                         ← overloaded scope API
    monitoring.ts:15                       ← metricWindow + isSynthetic consumer
  stores/
    use-server-store.ts:1                  ← activeTab / selectedServerId
forge/api/internal/services/
  heartbeatmonitor/service.go:266          ← 6-state classifier
  observability/service.go:59             ← allocation-sampling
  observability/metrics_collector.go:41    ← system metrics history
  cronjob/service.go:117                  ← server vs shell dispatch
audits/
  FINAL_PARITY_AUDIT.md:326               ← UX §16 (14 rows)
  phase-01/subagent-04-ux-control-plane.md:164 ← 17 comps C01–C17, FL-UX-01–06
```

---

## 17. Verifying No Code Was Modified (for CI)

```bash
git status --porcelain -- forge/ web/ audits/110-phase-01-audit/ 2>/dev/null | grep -v '^?? audits/110-phase-01-audit/'
# expected: no output (this subagent wrote only  audits/110-phase-01-audit/subagent-10-ux-observability.md)
wc -l audits/110-phase-01-audit/subagent-10-ux-observability.md
```

Prepared by **subagent-10 (ux-observability)** — `muse-spark-1.2-contributor`. Source inspected on disk; no product code modified; synthesis derives from verified `file:line` reads during this session.
