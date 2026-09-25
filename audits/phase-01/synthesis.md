# Phase 01 Synthesis — Application Platform Cluster

**Cluster:** coolify, dokploy, dokku, caprover, komodo, portainer, 1panel, uncloud, docker-compose (spec control)
**Why this cluster first:** Largest Forge overlap (app → container(s) → proxy → domain), most divergent architectures (single-host bash-Dokku vs Rust-Komodo periphery vs quorum-less Uncloud vs minimal CapRover), highest user-visible integration debt. Value discovered here guides later phases without re-inspecting same compose/auth primitives.
**Evidence base:** 5 subagent reports (≥15 comparisons each, ≥3 logic findings each) — source-verified file:line, not marketing. Reports read and reconciled 2026-08-23.
**Method:** Each subagent inspected references under `reference/app-platforms/<project>` and Forge under `forge/web/*`, `forge/api/internal/{http,services,store,daemon,runtime,auth,eventstore}`, `beacon/internal/{runtime,server}`, `forge/api/migrations/*.sql`.

---

## 1. Cluster rationale restated

Coolify/Dokploy/Komodo are modern PaaS (Nixpacks/buildpacks/traefik, multi-server); Dokku/CapRover are minimal single-host (CaptainDefinition, plugin model); Portainer is container-mgmt authority; 1Panel is widest single-host admin surface; Uncloud is no-control-plane counter-reference; Docker-Compose is spec truth. Together they test Forge's entire app-platform surface: `services/{apphosting,compose,deployment,preview,build,buildpack,appstore,catalog,zerodowntime,replicamanager,operation,queue}`, runtime adapters, and `admin-shell.tsx` IA.

---

## 2. Capabilities discovered (per dimension)

| Dim | Reference capabilities inventoried |
|-----|-------------------------------------|
| Lifecycle (01) | Coolify `Application.php:118` + `ApplicationDeploymentJob.php:42` queue-backed lifecycle (tags, rolling_update:1904, health_check:1955, handleStatusTransition:4772); Dokku `plugins/{ps,checks,apps}` (`ps:restart`, `checks:enable`, `scheduler-docker-local`); CapRover `ICaptainDefinition.ts:1` + `ImageMaker.ts:46` definition→dockerfile; Komodo `stack.rs` Create/Update/Deploy/Start/Stop/Remove + periphery `compose up -d`; Portainer `stackbuilders/*` builder pattern + `StackDeployer.Deploy`; 1Panel `app_install.go:11` AppInstall + `Operate{start\|stop\|restart\|rebuild\|upgrade}`; Uncloud `pkg/client/service.go:24` RunService→Validate→Inspect→VolumeSchedule→Deployment.Run (fan-out wg per container); Docker-Compose `pkg/compose/compose.go` primitives |
| Git/Build (02) | Dokploy 4 provider types (`git-provider.ts:8`), 6 builders (`dockerfile\|nixpacks\|heroku\|paketo\|static\|railpack`); Coolify 5 build packs + `PrivateKey` SSH deploy keys; CapRover `GitHelper.ts` SSH_RE generic host; Komodo token-in-URL clone; Portainer `GitAuthentication` + `libstack` compose build; Branch/commit pinning, Registry auth, Build logs CircularQueue, Revisions hash, Preview per-PR, Webhooks HMAC+idempotency, Auto-provision webhooks, Commit status `checks`, Compose git stacks, Tar upload, GitOps polling |
| Runtime/Compose (03) | Portainer `ContainerService.Recreate:55` transactional recreate, `ClientFactory:28` edge vs direct, `StackDeployer`; 1Panel `container.go` list/create/prune, `image.go`, `compose_template.go`, `docker.go` daemon JSON; Uncloud `RunService` fan-out + auth retrieval; Docker-Compose `NewComposeService:80` dep graph `InDependencyOrder:78`, `getRestartPolicy:592`, `resources:641`, `envresolver.go` interpolation; Coolify/Dokploy granular `compose/network/volume.ts` |
| UX (04) | Coolify resource-centric IA, 6 status components, per-type new app blades, `configuration-diff`, 4-layer env vars (`multiline\|literal\|buildtime\|runtime`), per-resource logs+terminal, global search, per-channel notifications; Dokploy depth (advanced/{cluster,traefik,security}, CodeEditor env, per-container logs, webhook URL+stuckDeployment >9m); Komodo monitor polling `compose ps`; Portainer endpoint-scoped nav + `ResourceControl` RBAC matrix; 1Panel 12-module system-domain layout; CapRover single-tenant; Uncloud no UI (counter); Docker-Compose CLI primitives |
| Arch/Sec (05) | Coolify `config/queue.php:28` Redis 24h retry_after, `ScheduledTaskJob tries=3 timeout=300`, `ShouldBeEncrypted`, `DeploymentJob timeout=3600`; Dokploy sync services; Portainer `libcrypto encrypt.go` AES-GCM, `ResourceControl`, chisel edge tunnel; 1Panel Unix socket `agent.sock`; Komodo `periphery_info` heartbeat; Uncloud Corrosion gossip+heartbeat ticker |

---

## 3. Forge equivalents (map)

| Concern | Forge location | Pattern |
|---------|----------------|---------|
| Apps | `100_z_app_platform_applications.sql:5` `applications(desired_state,observed_status,current_deployment_id)` + `store/store_apphosting.go:11` `Application`, `service.go:92` CreateApp | Control-plane row, scheduler decides placement; desired vs observed split |
| Services/replicas | `102_a_uncloud_service_model.sql` `replica_app_id`, `replicamanager/service.go:100` CreateApp/DeployApp/ScaleApp with `reservations.Manager` + `BeaconCommandLog` | Uncloud-inspired replica manager, generation-gated (`NoDoubleReservation`) |
| Deployments | `095_deployments.sql` + `103_a_deployment_steps.sql` + `114_e_zero_downtime_deploy.sql` + `services/deployment/{service,execution,rollout,healthgate,revisions,steps}.go` | 4 strategies (`recreate\|rolling\|blue-green\|canary`) as DAG steps; lease-claimed execution + health gate + auto-rollback |
| Revisions | `099_deployment_revisions.sql` `deployment_revisions` + `revisions.go:82` CreateRevision/rollbackToRevision | Image/compose/commit hash versioning |
| Compose | `098_app_platform_foundations.sql:31` `compose_stacks(status,compose_yaml,hash,env_vars,reservation_id)` + `services/compose/lifecycle.go:249` DeployComposeStack + `service.go:145` Parse/Validate + `beacon/internal/server/compose.go:344` `docker compose up -d` shellout | Port validation via `compose-go`, per-stack RWMutex lock, status via `ps --format json` |
| Build | `services/build/service.go:28` BuilderDockerfile/Nixpacks only, `buildpack_service.go`, `beacon/internal/server/build.go:76` Dockerfile/Nixpacks | Only 2 builders wired; params CacheFrom/CacheTo/Platform exist but not UI |
| Git | `services/git/*` 5 provider types incl generic, `git/service.go:87` GenerateDeployKeyPair (ed25519/rsa), `deploy_service.go:96` cloneWithOptions (SSH askpass 0600 or token askpass), `store_git_*.go` encrypted tokens | Control-plane clone vs Beacon clone divergence |
| Preview | Two services: `preview/service.go:42` legacy (hardcoded `preview-<suffix>.example.com`, no TTL) vs `previewenv/service.go:121` TTL+MaxPerOrg+wildcard domain+cert+commit-status+reaper — latter not wired to HTTP | Duplicate |
| Env vars | Three editors: `environment/env-var-editor.tsx:8` admin (API-backed Sensitive pill), `AdminAppsShared:120` Record-editor (upsert, key disabled), `EnvironmentEditor.tsx:37` encrypted+import/export — two APIs (`/env-vars/:id` vs `/{projects\|environments}/:id/env-vars`) | Incoherent |
| Runtime | `beacon/internal/runtime/docker.go:42` + `forge/api/internal/runtime/multiruntime.go` + `factory.go` docker/containerd/podman/firecracker/k8s/LXC/KVM stubs; `daemon/client.go` Admin* fleet fan-out | Multi-runtime abstraction, Docker primary |
| Queue/Operation | `queue/*.go` lease30s/timeout30m + `operation/*.go` reaper5m dual queues; `periodic.go:97` in-memory scheduler | Deprecated duality (`OpCompose*` vs `JobCompose*`) |
| Nav | `admin-registry.ts:22` 5 groups ~62 entries + `admin-shell.tsx:41` goal plan 6 groups with search | Goal-oriented plan overlay |

---

## 4. Forge hidden / unwired (activation candidates)

- **Unwired compose restart** — `lifecycle.go:781` RestartStack + `beacon/compose.go:506` handleComposeRestart + `daemon ComposeRestart:58` + `queue_handler.go:111` HandleRestart exist, but `handlers_compose.go:442/455` only mount stop/start; admin cannot restart stacks (`compose.ts` exports no restart).
- **Unwired Docker admin create/prune/exec** — UI+daemon exist but Beacon has no `POST /api/admin/{containers,networks,volumes}` for create (beacon `server.go:440-466` missing), and `dockerPruneVolumes` has no Beacon handler while `handleImagePrune` exists but never called (§5 FL-01/FL-02/FL-03).
- **Unwired env Pipeline triggers** — `pipeline/service.go:40` queue+schedule loop, `189_pipeline_artifacts.sql` exist, but no HTTP/UI registered (handler not found via grep).
- **Unwired previewenv** — TTL/reaper/commit-status logic lives in `previewenv` but HTTP `handlers_preview_deployments.go:9` wires legacy `preview` only; TTL never expires (`preview/service.go:42`).
- **Unwired Build caching** — `build/service.go:95` CacheFrom/CacheTo/Platform forwarded, but `beacon/build.go:59` dockerfileBuildRequest struct ignores them; UI never exposes.
- **Tenancy cascade** — `environments/page.tsx:66` org→project→env cascade works but `apps` list `fetchApps()` is unscoped (`GET /apps` global), so tenancy does not filter workload view (duplicates Dokploy's discipline).
- **Compose git polling/env_file/deps/order** — `GitAutoUpdate/GitPollIntervalSec`, `env_file` interpolation, `depends_on` ordering delegated to `docker compose` itself rather than Forge sequencer; `restart: always` warning vs no Beacon block inconsistency.

---

## 5. Missing functionality (genuine)

- Builder parity — Dokploy 6 (railpack/heroku/paketo/static) vs Forge 2 (`dockerfile|nixpacks`) — but Dokploy extras matter for Next/Nuxt static; Forge must decide static publish dir vs document gap (severity P1 as false-completion, not missing).
- Tar upload (CapRover `Uploaded Tar` + `captain-definition`) — not in Forge; intentional omission unless targeting CapRover parity.
- Per-resource configuration diff before deploy (Coolify `configuration-diff.blade.php:1`) — Forge has `CompareRevisions(225)` backend but no pre-deploy diff modal in UX.
- Generic git provider OAuth dance (`Authorize+Callback`) — Forge only does PAT insertion (Dokploy/Coolify both have `/authorize`).

---

## 6. Broken / incorrect logic (FORGE LOGIC FINDINGS — 28 unique across 5 subagents)

Reported in MASTER_FINDING_INDEX.md (canonical). Consolidated here by severity:

**P0 (false-completion / user-visible lie):**
- F-01 lifecycle `executeProvision/Promote/Drain/Scale/Cleanup` are stubs returning nil — `ExecuteDeployment` can report `completed 100%` with zero containers (`deployment/execution.go:249-311`, tests `deployment_test.go:40` assert completed).
- F-05 health gate `host=localhost` (`healthgate.go:11` + `validateHealthGateTarget:71`) probes API host not container node — every health-gated deploy fails.
- F-12 app restart (`handlers_apphosting.go:500`) delegates to `TriggerDeploy` empty-image deployment rather than container restart.
- F-07 app Deploy validates env but `TriggerDeploy:666` `Image:""` unconditionally.

**P1 (core broken / duplicate execution / stale state):**
- F-02 app start/stop lie (`handlers_apphosting.go:450` sets `desired_state` only, no reconciler — `observed_status` stale).
- F-08 compose redeploy (`handlers_compose.go:413`) creates new `compose_stacks` row per redeploy (leak).
- F-16 admin container/network/volume create → daemon `POST /api/admin/{containers,networks,volumes}` 404 (Beacon has no handler `server.go:440-466`).
- F-17 prune crossed wires (volume prune called→missing handler; image prune handler exists→never called).
- F-04 rollback version race (`revisions.go:147` non-version-checked `UpdateDeployment` on stale `Version`).
- F-13 scale silent no-op when `ReplicaAppID==nil` (DB replicas updated, no placement) — `handlers_apphosting.go:416` guard.
- F-18 lease too short for installs (queue `lease30s timeout30m`, steal `store.go:55` + `retry_count++` double increment).
- F-19 operation reaper 5m resets long-running installs/backups while handler still runs (`operation/service.go:164-182`, no heartbeat).

**P1-P2 UX/lifecycle lies:**
- F-20 app update `PUT /apps/:id` saves metadata but never enqueues deploy — version appears updated while running version stale.
- F-21 compose scale per-service unavailable via UI/API (only YAML edit).
- F-22 four app types created but converge to single lifecycle (type: git|compose|image distinction lost after creation — wizard region/template/description validated then discarded at `app-create-form.tsx:22→64` hard-codes `type:"image"`).

**P2 (reliability/security/wiring):**
- F-09 delete unconstrained (`DeleteApplication` allows delete while `in_progress` deploying; `CreatePlacementReservation` not cancelled; beacon containers orphaned as `removeOrphans` defaults false).
- F-10 ValidateCompose missing on `PATCH /apps/:id/compose` update (sensitive host path check bypassed via `app->compose` path).
- F-06 admin `POST /user/compose/:id/restart` alive but admin `POST /compose/:id/restart` missing (wiring inversion).
- F-23 credential script divergence (Forge direct `deploy_service.go:287` embeds token into script content vs Beacon `git.go:387` env-var script; shallow SHA fetch fragility `deploy_service.go:207`).
- F-24 restartAttempts never decays (`reconciler/service.go:639-646` permanently gates after 3 recoveries).
- F-25 dedupe window `hasPendingDuplicatePlan 30m` (`reconciler/service.go:654-676`) suppresses legitimate flapping drifts.
- F-26 reconciler publishes desired/actual events before verifying state (`reconciler/service.go:476-496`).
- F-27 generation fencing without transactional reservation coupling (`recovery/service.go:648-657`).
- F-28 Beacon `ReconnectClient` blindly `StateConnected` + `lastHb=Now()` per ticker defeats `heartbeatmonitor` offline detection (`beacon/internal/remote/reconnect.go:98-169` vs `heartbeatmonitor/service.go:283`).
- F-29 InMemorySessionStore pointer sharing + raw token storage (`auth/session.go:36-98`).
- F-30 Global `placement.Engine.mu` serializes all scheduling (`placement/engine.go:14-77`).
- F-31 Migration advisory lock blocking (`store/store.go:39`) vs dialect runner divergence.
- F-32 In-memory preview uniqueness race (`previewenv/service.go:144` scan without DB partial unique index).
- F-33 Remote log streaming post-hoc (`build/service.go:530` streams after completion, not live, vs `handlers_builds.go:91` SSE contract).
- F-34 Remote `executeRemoteBuild` Beacon drops CacheFrom/CacheTo (`beacon/internal/server/build.go:112` missing fields).

**UX-decorated findings (Phase1-04):**
- F-35 Recovery nav duplicates `/admin/migrations` href (`admin-registry.ts:32`), breadcrumb spill (17 tabs), tab state lost on refresh (`useState` not routed), status enums not centralized (3 divergent tones), empty-state CTA dead (`AppList:44` default button no onClick), metrics-chart `queryKey` missing node dimension, env var editors triple incoherence (Literal/Buildtime/Runtime/Multiline flags missing), tenancy un-scoped listing, duplicate deployment polling (2s vs 5s), rate-limit `ErrorRateLimit` never consumed, two offline banners.

---

## 7. Duplicates

- `queue` vs `operation` dual durable queues with dual idempotent namespaces (`forge-job:` vs `forge-op:`) and conflicting deprecation comments (`queue is single writer` vs `operation OpCompose` kept for back-compat) — no single-writer enforcement (`MASTER_FINDING_INDEX` REF-APP-DUP-001).
- `preview` vs `previewenv` duplicate services (legacy no-TTL vs TTL+commit-status+reaper) — `preview` wired, `previewenv` hidden (`REF-APP-DUP-002`).
- `EnvVarEditor` ×3 components + `fetchEnvVars` overload indirection (project|environment) (`REF-APP-DUP-003`).
- `DeploymentProgress` (`setInterval 2s`) vs `DeploymentTimeline` (`useQuery 5s`) dual polling for same `GET /:id/steps` (`REF-APP-DUP-004`).
- `AdminScopes` vs `KnownScopes` overlap and `hasAnyAdminScope` breadth (`REF-APP-DUP-005`).

---

## 8. False completion / decorative features

- `ExecuteDeployment` stubs report `completed` (P0).
- App start/stop desired_state writes pretend to actuate (`BROKEN`).
- Compose restart gateway exists behind daemon but has no admin route — button missing but infra claims “restart supported”.
- Docker admin Create/Prune/Exec UIs exist while Beacon handlers absent — UI is декорация without end-to-end verification.
- BuildTypes `heroku|paketo|static|railpack` accepted at creation (`handlers_source_deployments.go:90`) then fail at `source_deploy.go:81` — contract lie.
- Tenancy org/project/environment cascade pretends to scope workloads — `fetchApps()` ignores tenancy.

---

## 9. Dead / legacy

- `ValidStackID` strict (`^[a-z0-9][a-z0-9_-]{0,127}$`) but `GenerateStackID` can emit `cps-` prefix — valid but historical mismatch noted.
- `ComposeProject` legacy `/compose/projects/*` (`handlers_compose.go:498-609` import/export/summary) mirrors `ComposeStack` but unexposed to new UI — duplicate legacy path.

---

## 10. Architecture lessons (what to adopt / reject)

| Lesson | Source | ADOPT / ADAPT / INSPIRE / REJECT | Rationale |
|--------|--------|----------------------------------|-----------|
| Queue-backed deploy with status transition (`handleStatusTransition`) | Coolify `ApplicationDeploymentJob.php:4772` | **ADAPT** | Forge's step DAG is stronger than Coolify's switch; fix health-gate target and wire provision/promote |
| CaptainDefinition single-file contract | CapRover `ICaptainDefinition.ts:1` + `ImageMaker.ts:258` | **ADAPT** | Forge's `Forgefile` absent — single-file `compose + health path` as source of truth simplifies redeploy vs current `source_config` JSON |
| GitOps polling `SourceScheduler` per-stack webhook+interval | Portainer | **ADOPT** | Forge has polling fields persisted but not exposed; unify `autoDeploy` flags |
| Builder matrix with `railpack/static` publishDirectory handling | Dokploy `utils/builders/*` | **ADAPT** | Forge's 2 builders suffice unless targeting Next/Nuxt static — then add publishDirectory copy |
| Transactional `ContainerService.Recreate` (pull→stop→rename→reconnect→remove) | Portainer | **INSPIRE** | Forge never recreates containers ad-hoc; compose path relies on `compose up -d` but admin container lacks atomic recreate |
| Corrosion gossip DB (no central Postgres) | Uncloud | **REJECT** | Forge correctly centralizes in Postgres+advisory lock; Uncloud's CRDT trades consistency for liveness — wrong for panel |
| Monolithic AppInstall twin (store + docker compose verbatim) | 1Panel | **REJECT** | Store verbatim YAML (Forge does) but keep normalized spec via `Compose` parser; avoid single-host assumption |
| Chisel reverse tunnel per edge | Portainer | **INSPIRE** | Forge's HMAC per-request + mTLS is actually stronger than chisel, but chisel's persistent tunnel helps WebSocket stability — evaluate if WS polling fallback can be tunnled |
| Secrets encrypted per-resource with AAD | Portainer `libcrypto` | **ADAPT** | Forge's keyring uses caller-supplied AAD but not per-resource namespace consistently — bind AAD to `type:id` |
| Configuration diff before deploy | Coolify | **ADOPT** | `CompareRevisions(225)` backend exists; add pre-deploy diff modal like `configuration-diff.blade.php` |

---

## 11. Recommended activation order (existing capability → wired)

Without adding a fundamentally new subsystem:

1. **P0 Fix `ExecuteDeployment` stubs** — wire `executeProvision/Promote/Drain/Cleanup` to replicamanager/daemon/traffic (or fail deploy until wired); stop lying about `completed`. Unblocks every health-gated rollout.
2. **P0 Fix health gate host** — run `CheckHealth` via beacon on target node (or `docker inspect HealthStatus`), not `localhost`; merge `health_check_configs` with gate.
3. **P0 Enforce `Image:""` guard** — `TriggerDeploy:666` must require `Image` or derive from previous revision; surface 422.
4. **P1 Wire compose restart + deployment-after-update atomicity** — add `POST /compose/:id/restart`, make `PUT /apps/:id` (`UpdateApp`) optionally enqueue `StartRollout` with new env/image atomically.
5. **P1 Fix Beacon handler gaps** — implement `POST /api/admin/{containers,networks,volumes}` and `POST /api/admin/volumes/prune`; or remove Create/Prune UI affordances.
6. **P1 Fix queue vs operation duality** — choose single writer (queue) and enforce (remove `operation.DispatchCompose`), raise lease/timeout to `tries=1 timeout=3600` for installs (Coolify discipline), add heartbeat for `operation`.
7. **P1 Deduplicate preview** — switch `registerPreviewDeploymentRoutes` to `previewenv` (TTL+commit-status+reaper) and add DB partial unique index on `(pr_number, repo_owner, repo_name) WHERE status IN ('deploying','running')`.
8. **P1 Fix `RestartApp` and `ScaleService` semantics** — `restart` must not be `TriggerDeploy`; scale must fail if `ReplicaAppID==nil` instead of silent DB-only write.
9. **P2 Gate delete** — `DeleteApplication` guard `!isActiveStatus` + placement reservation cancel + orphan beacon sweep; `removeOrphans` true.
10. **P2 Unify env var stack** — converge three editors into one API-backed `EnvVarResponse[]` with flags (`isSensitive/version`) and add `isMultiline/isLiteral/isBuildtime/isRuntime` like Coolify; remove overload.
11. **P2 Add `revisions` diff modal pre-deploy** — `CompareRevisions` already diffs `composeManifestRef/gitCommitSha/configHash/metadata:225`, add UI modal.
12. **P3 Fix deregisters** — `restartAttempts` decay, plan dedupe window narrowing, event-before-verify ordering, generation↔reservation atomicity.
13. **P3 Move `periodic.go` to durable cron** — persist `nextRun` via DB or use `ScheduledTask` table, add `pg_advisory_lock` with retry.
14. **P3 Centralize nav/UX debts** — router-driven tabs for apps, single statusTone map, single `DeploymentTimeline`, rate-limit `429 Retry-After → ErrorRateLimit`, dedupe offline banners.

---

## 12. Phase 1 evidence inventory

- 5 reports, each ≥15 comparisons + ≥3 logic findings, all file:line verifiable, read-only verified.
- References covered: 9 platforms with real file hits listed in each report's §2 inventory tables.
- Forge inspected: `forge/web/*`, `forge/api/internal/{http,services/{apphosting,deployment,compose,build,git,preview,replicamanager,operation,queue,...},store,daemon,runtime,auth,eventstore}`, `beacon/internal/{server,runtime}`, `forge/api/migrations/*.sql`.
- Repro commands provided where noted (e.g., deploy with `healthGateEnabled + port != node port` to reproduce F-05, or `deploy via trigger then health gate` etc.)

---

## 13. Handoff to Phase 2

Phase 2 (Game Hosting: `pelican-panel`, `pelican-wings`, `pterodactyl-panel`, `pterodactyl-wings`, `pufferpanel`, `pufferpanel-templates`) should avoid re-inspecting pure compose/queue IA already covered, and instead focus on Wings vs Beacon isomorphism (`PP→Forge API`, `Wings→Beacon`), including PufferPanel's `pufferpanel-templates` vs `packages/game-templates`, and lifecycle correctness already partially covered here (reuse `ExecuteDeployment`/`healthgate` findings without duplicating).

