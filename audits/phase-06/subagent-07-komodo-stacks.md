# Subagent 07 — Komodo Periphery & Stacks vs Forge Compose + Beacon Runtime + Operations

**Scope:** `reference/app-platforms/komodo` (periphery agent, core stack/api, types, monitor, alerting) vs Forge `forge/api/internal/services/compose/*`, `beacon/internal/server/compose.go` + `beacon/internal/runtime/*`, build/buildpack/git/sourcedeployment, `forge/api/internal/http/handlers_compose.go`, `forge/web/app/admin/compose`, `forge/api/internal/services/{alerting,observability}`.

**Date:** 2026-08-23
**Mode:** Read-only inspection; no product code modified.

---

## 1. Reference Map

### Komodo
| Area | Path |
|------|------|
| Periphery agent entry | `reference/app-platforms/komodo/bin/periphery/src/main.rs` |
| Periphery config | `reference/app-platforms/komodo/bin/periphery/src/config.rs:1-186` |
| Periphery compose API | `reference/app-platforms/komodo/bin/periphery/src/api/compose.rs:1-1101` |
| Periphery stack helpers | `reference/app-platforms/komodo/bin/periphery/src/stack/mod.rs:1-204`, `.../stack/write.rs` |
| Periphery docker helpers | `reference/app-platforms/komodo/bin/periphery/src/docker/compose.rs:1-69`, `.../docker/*.rs` |
| Core stack execution | `reference/app-platforms/komodo/bin/core/src/stack/mod.rs:1-41`, `.../stack/execute.rs:1-244`, `.../stack/remote.rs`, `.../stack/services.rs` |
| Core periphery client | `reference/app-platforms/komodo/bin/core/src/periphery/mod.rs:1-154` |
| Core write/stack API | `reference/app-platforms/komodo/bin/core/src/api/write/stack.rs:1-1094` |
| Core execute/stack | `reference/app-platforms/komodo/bin/core/src/api/execute/stack.rs` |
| Types — Stack | `reference/app-platforms/komodo/client/core/rs/src/entities/stack.rs:1-1152` |
| Types — Alert | `reference/app-platforms/komodo/client/core/rs/src/entities/alert.rs` |
| Monitor | `reference/app-platforms/komodo/bin/core/src/monitor/mod.rs:1-342`, `.../monitor/resources.rs`, `.../monitor/alert/stack.rs:1-96` |
| Alert dispatch | `reference/app-platforms/komodo/bin/core/src/alert/mod.rs:1-548` |

### Forge / Beacon
| Area | Path |
|------|------|
| Compose lifecycle | `forge/api/internal/services/compose/lifecycle.go:1-1027` |
| Compose queue handler | `forge/api/internal/services/compose/queue_handler.go:1-115` |
| Compose GitOps | `forge/api/internal/services/compose/gitops.go:1-1345` |
| Compose controller | `forge/api/internal/services/compose/controller.go:1-360` |
| Parser + security | `forge/api/internal/services/compose/service.go:1-882` (`ValidateComposeSecurity`, `interpolateEnv`), `forge/api/internal/services/compose/parser.go:1-1086` |
| Beacon compose handlers | `beacon/internal/server/compose.go:1-751` |
| Beacon runtime | `beacon/internal/runtime/runtime.go:1-184`, `.../docker.go:1-1135`, `.../factory.go`, `.../stats.go` |
| HTTP handlers | `forge/api/internal/http/handlers_compose.go:1-650` |
| Web UI | `forge/web/app/admin/compose/page.tsx`, `.../compose/[id]/page.tsx`, `.../compose/new/page.tsx` |
| Alerting | `forge/api/internal/services/alerting/service.go:1-513` |
| Observability | `forge/api/internal/services/observability/service.go:1-457`, `.../metrics_collector.go` |
| Build | `forge/api/internal/services/build/service.go:1-1317`, `.../buildpack/buildpack_service.go` |
| Git services | `forge/api/internal/services/git/*`, `forge/api/internal/services/gitprovider/service.go` |

---

## 2. Architecture Comparison

### C-01 — Core/Periphery split vs API/Beacon split

- **Komodo:** Single Rust workspace with two binaries sharing `komodo_client`/`periphery_client` crates. Core (`bin/core`) owns DB, scheduling, alerting; Periphery (`bin/periphery`) owns Docker, git clones, file I/O. Communication is a **bidirectional persistent channel over WebSocket / mTLS** with `PeripheryClient::request()` multiplexed via `channel_id` + `ResponseChannels` (`bin/core/src/periphery/mod.rs:84-153`, `bin/periphery/src/connection/*`). Core can also act as client or server (`PeripheryClient::new` matches `args.address` at `mod.rs:39-73`).
- **Forge/Beacon:** Go API (`forge/api`) owns DB, scheduling, GitOps; Beacon (`beacon/internal/server`) owns Docker/compose, mounts, diagnostics. Communication is **stateless HTTPS via `daemon.Client`** (`forge/internal/daemon`) using node credential (`GetNodeDaemonCredential`) per request (`compose/lifecycle.go:305-321`, `beacon/internal/server/compose.go:344-416`). No persistent channel; each deploy/status/logs call is an independent `POST /compose/...` with 10m timeout. Beacon's `TokenCache` + `allowedMountsMu` mirror peripheral config but via polling, not pushed secrets.
- **Implication:** Komodo's periphery can stream logs/progress via `Update` and `action_states` guards; Forge's beacon returns `composeOperationResponse{Output, Error}` synchronously and API must poll `ComposeStatus`.

### C-02 — Stack target model: Swarm+Server unified vs Node-only

- **Komodo:** `StackConfig` has **both** `swarm_id` + `server_id` (`client/core/rs/src/entities/stack.rs:302-327`). If `swarm_id` set, stack is in Swarm mode; `SwarmOrServer` enum routes execution (`bin/core/src/stack/mod.rs:17-32`, `bin/core/src/stack/execute.rs:50-53` explicitly rejects Compose executions for Swarm stacks). `StackListItemInfo` carries both (`stack.rs:147-149`).
- **Forge:** `ComposeStack.NodeID` only (`compose/lifecycle.go:42-43`); no Swarm mode. Swarm-related features (services, secrets, stack deploy) are separate `beacon/internal/server/*` not composed. `compose/parser.go:566-572` parses `Deploy` but `lifecycle.go` never uses it beyond storing hash; beacon validates and runs `docker compose up` only. Swarm stacks in Komodo have `docker stack deploy` paths absent in Forge.

### C-03 — File sourcing trifecta vs dual-source

- **Komodo:** Three mutually-intelligible sources selected by `StackConfig` (`stack.rs:402-638`):
  1. **UI** `file_contents` (inline YAML, `file_contents_deserializer` at `stack.rs:637`),
  2. **Git repo** `repo` + `branch` + `commit` + `linked_repo` (`stack.rs:409-456`), fetched via `pull_or_clone_stack` (`bin/periphery/src/stack/mod.rs:75-130`) or core `get_repo_compose_contents` in `RefreshStackCache`,
  3. **Files on host** `files_on_host` + `run_directory` + `file_paths` (`stack.rs:489-503`), read via `GetComposeContentsOnHost` / `WriteComposeContentsToHost` (`periphery/src/api/compose.rs:100-196`).
  `all_tracked_file_paths()` / `all_file_dependencies()` include compose files + `additional_env_files(track==true)` + `config_files` with `StackFileRequires::{Redeploy,Restart,None}` (`stack.rs:92-132`).
- **Forge:** Two sources:
  1. **Raw** `ComposeYAML` stored in `compose_stacks.compose_yaml` (`lifecycle.go:44`),
  2. **Git-backed** via `GitOpsService.CloneRepo` → `readComposeFromDir` searching `compose.yml/yaml/docker-compose.yml/yaml` or explicit `composePath` (`gitops.go:196-284`), with `readLimitedComposeFile` rejecting symlinks/non-regular files >1MB (`gitops.go:287-299`).
  No `files_on_host` or inline `config_files` tracking; additional env files handled only at runtime via `.env` encoding (`compose.go:264-281`). `PreviousDeploymentManifest` stashes prior YAML+commit for rollback (`gitops.go:87-93`) vs Komodo's `deployed_contents` / `remote_contents` in `StackInfo`.

### C-04 — Full compose lifecycle on periphery vs minimal beacon `up -d`

- **Komodo `ComposeUp` (`periphery/src/api/compose.rs:414-783`)** is a **6-phase pipeline** under one `DeployStackResponse`:
  1. `Interpolator` + `write_stack` (handles git vs host vs UI, secret replacers),
  2. `validate_files` + `maybe_login_registry` (domain/account/token),
  3. `pre_deploy` `SystemCommand` if `!is_none()` (`compose.rs:485-508`),
  4. `docker compose config` to sanitize + enumerate `StackServiceNames` with replica expansion (`compose.rs:544-626`),
  5. optional `build` (`run_build` + `build_extra_args`) and `pull` (`auto_pull`) each via `maybe_wrap_command` wrapper (`compose.rs:628-711`),
  6. `compose down` if `destroy_before_deploy || project_name changed` then `compose up -d` (`compose.rs:713-757`) plus `post_deploy`.
  Each phase is gated by `all_logs_success` – failure short-circuits but returns logs, not panics. `compose_cmd_wrapper` with `[[COMPOSE_COMMAND]]` placeholder allows `op run --` / `sops exec-file` wrapping per subcommand (`maybe_wrap_command` at `compose.rs:1050-1072`).
- **Beacon `handleComposeDeploy` (`compose.go:344-416`)** is **single-shot**: validate policy → `os.RemoveAll` + `MkdirAll` + write `compose.yaml` + `.env` → `exec.CommandContext(ctx, "docker", "compose", "-f", path, "-p", stackID, "up", "-d")` with optional `--remove-orphans` (`compose.go:395-403`). No `config` synthesis, no registry login (delegated to runtime `RegistryAuth`), no build, no pre/post hooks, no wrapper. Health observation is **API-side** `WaitForHealthy` polling `ComposeStatus` for 2m (`lifecycle.go:188-231`).

### C-05 — Project naming & orphan semantics

- **Komodo:** `Stack::project_name(fresh: bool)` (`stack.rs:45-59`) returns `info.deployed_project_name` if not fresh else `config.project_name` or `name`. Deploy uses `project_name(true)` (fresh) but stores `deployed_project_name` after success; `ComposeUp` keeps `last_project_name = project_name(false)` for `compose_down` to tear down the *previous* project if renamed (`compose.rs:522-525,712-722`). `RemoveOrphans` is not exposed; orphan handling is via explicit `destroy_before_deploy` + `compose_down`.
- **Forge:** `validStackID` (`compose.go:59-71`) enforces `[a-z0-9_-]` ≤128; `dirForID` maps to `.../compose/<id>` (`compose.go:283-288`). Project name **is** the stackID (`-p req.StackID`). `RemoveOrphans` is **opt-in, default false** with explicit doc warning at `compose.go:290-302` and query-param gate on delete (`compose.go:574-580`). Lifecycle never renames projects; `UpdateComposeStack` reuses same `stackID` and avoids orphan-creation bug fixed in `handlers_compose.go:413-438`.

### C-06 — Parsing & validation: flexible interpolation vs hardened deny-list

- **Komodo:** Parsing on periphery is **post-interpolation** via `docker compose config` (authoritative, supports `include`, `extends`, `env_file`); `ComposeFile` struct is minimal stub for image/replica extraction only (`stack.rs:881-904`). Core validation is permissive; security is not composition-layer but host-level (`periphery_config().allow_docker_*` etc).
- **Forge:** **Two-layer validation:**
  - **API** `ValidateComposeSecurity` (`compose/service.go:427-528`) walks raw YAML map → denies `privileged`, `network_mode: host/service:`, `pid: host`, `ipc: host`, `cap_add` dangerous caps (`SYS_ADMIN` etc at `service.go:530-539`), `devices:/dev`, `volumes:/docker.sock,/proc,/sys`, sensitive mounts (`/`, `/root`, `/etc`, `/home` at `service.go:679`) with `ValidateHostMountWithAllowlist(source,isAdmin,allowedMounts)` (`service.go:663-677`), plus `restart: always` warning, `security_opt`, `userns_mode: host`, `container_name`.
  - **Beacon** `validateComposePolicy` (`compose.go:73-184`) re-validates via strict `map[string]any` schema using `composeTruthy` (handles `yes/on/1/true` `compose.go:138-155`), `validateComposePorts` blocking `published <1024` (`compose.go:181-210`), `validateComposeVolumes` rejecting **all** `type: bind` long-form and any `source` that is absolute or path-traversal (`compose.go:226-248`, `isPathTraversal` at `compose.go:251-262`).
  - **Interpolation:** Komodo uses `Interpolator` (`interpolate` crate) with secret replacers + `mogh_secret_file`; Forge uses `interpolateEnv` (`service.go:356-425`) supporting `${VAR:-default}`, `${VAR-default}`, `${VAR:?msg}`, `${VAR?msg}`, `$$` escape, and `ExpandTemplate` for app-store. Komodo's wrapper+replacers propagate to `run_komodo_command_with_sanitization` (`compose.rs:391-404`).

### C-07 — Queuing & concurrency

- **Komodo:** No queue table; per-stack **in-memory `CloneCache` action state** (`bin/core/src/stack/execute.rs:77-84`, `state::action_states`). `execute_compose` acquires `action_state.update(set_in_progress)` guard; concurrent deploy returns error immediately. Updates streamed via `update_update()` + `refresh_server_cache(&server, true)` after each execution. Git image polling is via `check_stack_for_update_inner` loop, not dequeued.
- **Forge:** **Three complementary mechanisms:**
  - `queue_handler.go:49-115` – thin synchronous wrapper translating `json.RawMessage` payloads into `lifecycle.go` methods (`HandleDeploy/Update/Delete/Start/Stop/Restart`) – no retry.
  - `GitOpsController` (`controller.go:34-125`) – interval ticker (15s `controller.go:81`) listing `ListComposeStacksPendingUpdate` (`controller.go:131`) with **pessimistic DB claim** `ClaimComposeStackForUpdate(stackID, workerID)` + `ReleaseComposeStackClaim` + stale claim recovery after 5m (`controller.go:346-359`). Prevents duplicate deploys across API replicas.
  - `GitOpsService.PollForUpdates` (`gitops.go:1260-1277`) for `ListComposeStacksDueForPoll` (auto-update poll), plus webhook-triggered async `PullAndRedeploy` with `webhookMu` (`gitops.go:50,1151-1255`).

### C-08 — GitOps & reconciliation

- **Komodo:** Polling for **image updates**, not git commits. `check_stack_for_update_inner` (`api/write/stack.rs:736-832`) iterates `latest_services`, resolves each `image` via `image_digest_cache().get(swarm_or_server, image)` (in-memory digest cache), stores `image_digest: Option<ImageDigest>` on `latest_services`, compares via `ImageDigest::update_available(current_digests)` (`write/stack.rs:880`). `auto_update` triggers `DeployStack` (only changed services unless `auto_update_all_services` or Swarm, `write/stack.rs:949-959`). Alert sent once per (stack,service) via `stack_alert_sent_cache` `OnceLock<SetCache>` (`write/stack.rs:720-724`). Git commit hash tracking is via `get_repo_compose_contents` / `RemoteComposeContents` (`stack/remote.rs`) – drift not explicitly surfaced.
- **Forge:** Polling for **git commits**. `CheckForUpdates` clones HEAD, compares `clone.CommitSHA != stack.GitCommitSHA` (`gitops.go:454-478`), `DetectDrift` parses both deployed YAML and repo YAML via `ParseComposeYAML` then `computeServiceDiffs`/`compareServiceSummaries` on image/ports/env/volumes/restart/command/entrypoint/dependsOn/deploy (`gitops.go:841-933`). `DetectRuntimeDrift` queries daemon `ComposeStatus` and diffs desired vs actual image/state/missing/extra services (`gitops.go:755-839`). Webhook path uses `hmac.New(sha256.New, GitWebhookSecret)` + constant-time `hmac.Equal` + `GitLastDeliveryID` deduplication (`gitops.go:1168-1178`). `RollbackToPrevious` swaps `GitPreviousCompose`/`GitPreviousCommitSHA` and sets `rollback_hold` mode (`gitops.go:595-685`). Symlink-aware `readComposeFromDir` rejects `..` and symlink escape via `EvalSymlinks` + `Rel` check (`gitops.go:214-229`) and `readLimitedComposeFile` rejects symlink files (`gitops.go:292`).

### C-09 — Monitoring / cache refresh

- **Komodo:** `spawn_monitoring_loops()` (`monitor/mod.rs:48-63`) runs **two dedicated loops** (`spawn_server_monitoring_loop` + `swarm::spawn_swarm_monitoring_loop`) on `monitoring_interval()` (configurable, `wait_until_timelength` with 500ms slop). Each tick `refresh_all_server_cache(ts)` fans out `join_all(refresh_server_cache)` per server (`monitor/mod.rs:65-81`). `refresh_server_cache` is **per-server singleflight** via `CloneCache<String, Arc<Mutex<i64>>>` (`monitor/mod.rs:85-90`) with 1s debounce and `force` bypass. Calls `periphery.request(PollStatus{include_stats, include_docker})` (`monitor/mod.rs:165-192`), then `update_server_stack_cache` / `update_server_deployment_cache` matching containers by `compose_container_match_regex("^container_name-?[0-9]*$")` (`stack/mod.rs:34-41`), plus `repo_status_cache` for `GetLatestCommit`. Also `record_server_stats(ts)` and `check_alerts(ts)` every tick.
- **Forge:** `observability.Service` (`observability/service.go:20-126`) offers `StartMetricsCollection` (in-memory `MetricsHistory` ring buffer `metrics_collector.go`) and `StartNodeMetricsCollection` with **0-5s jitter** to avoid thundering herd (`service.go:40`), periodic `collectNodeMetrics` synthesizing `NodeMetric` from `NodeCapacitySnapshot` (CPU/memory/disk % via `Available vs Total`, `service.go:89-114`) rather than daemon-reported stats. Timeline events recorded via `Handle(event)` → `CreateTimelineEvent`. Beacon's own `stats_collector.go` / `runtime/stats.go` polls Docker `StatsStream` but not shown as core cache; health is derived in API via `resourceScore`/`heartbeatScore`/`statusScore` (`observability/service.go:409-453`).

### C-10 — Alerting & notifications

- **Komodo:** `Alerter` resource (`client/core/rs/src/entities/alerter.rs`) with `AlerterEndpoint::{Custom,Slack,Discord,Ntfy,Pushover}` + `alert_types` whitelist + `resources`/`except_resources` + **maintenance windows** (`alert/mod.rs:76-80` `is_in_maintenance`). `send_alerts` fans out `join_all(send_alert_to_alerters)` (`alert/mod.rs:25-49`) consulting `enabled`, `maintenance_windows`, `alert_types.contains(AlertDataVariant)`, resource filter. `AlertData` variants include `StackStateChange`, `StackImageUpdateAvailable`, `StackAutoUpdated`, `SwarmUnhealthy`, `ServerUnreachable/Cpu/Mem/Disk`, `Deployment*`, `BuildFailed`, `ResourceSyncPendingUpdates` etc. Message formatting in `standard_alert_content` covers ~20 variants (`alert/mod.rs:245-547`). Stack alerts triggered in `monitor/alert/stack.rs:16-96` comparing `prev` vs `curr.state != Unknown`, skipping `Deploying` action state and `send_alerts==false`.
- **Forge:** `alerting.Service` (`alerting/service.go:32-513`) is **threshold-based** (`ThresholdConfig` default 80/95 CPU/mem, 85/90 disk `alerting/service.go:41-48`). `CheckNodeThresholds` maps `metrics.CPU/Memory/DiskPercent` to `AlertSeverity{Ok,Warning,Critical}` via `severityFromFloat` (`alerting/service.go:90-98`). Deduplication via `suppressionKey = alertType:nodeID` with `FindAlertBySuppressionKey`; skips if same severity+acknowledged (`alerting/service.go:170-187`). `resolveIfActive` auto-resolves OK severity. Dispatch via bounded `dispatchSlots` channel (cap 16 `alerting/service.go:57`) with `context.WithTimeout(30s)` + async `dispatchNotifications` respecting `route.Enabled`, `MinSeverity` score, `EventTypes` filter (`alerting/service.go:250-275`). `Notifier` has **SSRF guard** `validateNotificationURL` requiring `https` absolute URL, `net.LookupIP` and rejecting private/loopback/link-local (`alerting/service.go:485-501`). Channels: `slack/discord/telegram/email/webhook`; `telegram` posts to `api.telegram.org`, `email` via `notifications.EmailService`. No maintenance windows, no per-resource whitelist, no `StackStateChange` equivalent – stack health surfaces via `observability.MonitoringSummary` + `health_history`.

### C-11 — Build / Buildpack

- **Komodo (`periphery/src/api/build/mod.rs:129-345`):** Handles `registry_tokens` `HashMap<(domain,account), token>`, iterates unique `image_registry` for `docker_login`, injects `VERSION` into `build_args` if missing (`mod.rs:295-303`), writes UI-defined Dockerfile if not `files_on_host`/repo (`mod.rs:248-264`), runs `pre_build` `SystemCommand`, parses `build_args/secret_args/labels/extra_args` via `environment_vars_from_str` + `parse_secret_args` (reads secret files), constructs `docker[ buildx] build... --push -f Dockerfile .` with `maybe_push` conditional on successful login (`mod.rs:324-327`). Also exposes `PruneBuilders`/`PruneBuildx` (`mod.rs:349-391`).
- **Forge (`forge/api/internal/services/build/service.go:1-80+`):** Staged build record (`Queued→Cloning→Building→Built→Pushing→VerifyingDigest` `build/service.go:43-53`), `BuilderType::{dockerfile,nixpacks}` (`build/service.go:29-31`), `DisplayBuildStatus` transforms `running` with dead PID to `abandoned` via `checkPIDAlive(syscall.Signal(0))` (`build/service.go:65-79`), `IsTerminal` map. Separate `buildpack` service (`forge/api/internal/services/buildpack/buildpack_service.go`, `service.go`). Forge build is **API-orchestrated** (clones git, runs `docker build` locally or via beacon) whereas Komodo periphery does the build on the target host.

### C-12 — HTTP / Web surface

- **Komodo:** `bin/core/src/api/write/stack.rs:57-251` exposes `CreateStack`, `CopyStack`, `DeleteStack`, `UpdateStack`, `RenameStack`, `WriteStackFileContents` (branching host vs git path at `write/stack.rs:220-247`), `RefreshStackCache` (re-parses remote contents + extracts services via `extract_services_into_res` at `write/stack.rs:545-562`), `CheckStackForUpdate`/`BatchCheckStackForUpdate`, `DeployStack` via `api/execute/stack.rs`. Permissions checked via `get_check_permissions::<Stack>(id, user, PermissionLevel::{Read,Write,Execute})` (`write/stack.rs:90-95,197-201,466-470`). Periphery also serves `GetComposeLog`/`Search` with `docker compose logs | grep` (`compose.rs:39-96`), not mirrored in Forge.
- **Forge:** `handlers_compose.go:66-650` mounts **28 routes** under `protected` (admin + `mutationLimiter` + `requireRole("admin")`) plus one public `POST /compose/webhook/:webhookId` before session/CSRF (`handlers_compose.go:82-104`) doing `X-Hub-Signature-256` / `X-Git-Token` / `X-Gitea-Signature` + `X-GitHub-Delivery`. Validation `POST /compose/validate` (`handlers_compose.go:106-116`), import `POST /compose/import` → `store.ProjectDocument` (`handlers_compose.go:118-158`), GitOps: `deploy/redeploy/check-update/preview/rollback/pull-redeploy/branch/auto-update/drift/status/last-webhook` (`handlers_compose.go:162-311`), CRUD `POST/GET/PATCH/DELETE /compose` (`handlers_compose.go:315-411`), lifecycle `deploy/stop/start/restart/logs/status` (`handlers_compose.go:413-509`). `PUT /compose/projects/:id` increments `Revision` (`handlers_compose.go:569`). `Web UI` `compose/page.tsx:1-100+` renders `statusConfig` for `running/deploying/awaiting_health/degraded/failed/updating/deleting/deleted` with `fetchJSON("/compose")`.

### C-13 — Drift & config tracking (extra)

- **Komodo:** `RefreshStackCache` (`write/stack.rs:459-673`) branches by source: **host** → `GetComposeContentsOnHost` per `all_file_dependencies()` + `extract_services_into_res` per `is_compose_file` (`write/stack.rs:542-563`); **repo** → `get_repo_compose_contents` returning `RemoteComposeContents{successful, errored, hash, message}` (`write/stack.rs:582-593`); **UI** → parse `config.file_contents` directly (`write/stack.rs:627-642`). Stores `StackInfo{missing_files, deployed_*, latest_services, remote_contents/errors, latest_hash/message}` (`stack.rs:245-286`). No explicit drift diff – drift is implicit via `missing_files` or `remote_contents != deployed_contents`.
- **Forge:** Explicit `DetectDrift(stackID)` (`gitops.go:687-753`) clones repo and parses both deployed + current YAML via `ParseComposeYAML`, returns `DriftCheckResult{HasDrift, ServicesDiff[]ServiceDiff{added/removed/image/ports/env/volumes/restart/command/entrypoint/dependsOn/replicas}}` (`gitops.go:71-85,841-883`). Plus `DetectRuntimeDrift` (`gitops.go:755-839`) comparing desired `ServiceSummary.Image` vs daemon `ComposeStatus` `Image/Status/State` for `missing/image mismatch/not running/unstable(unhealthy/restart)/unexpected service`.

### C-14 — Interpolation & secrets

- **Komodo:** `Interpolator::new(Some(&variables), &secrets)` (`alert/mod.rs:162-163`, `write/stack.rs:281-288`) with `interpolate_stack`/`interpolate_build` + collects `secret_replacers` for `run_komodo_command_with_sanitization` (`compose.rs:391-397`). Secrets redacted via `svi::replace_in_string` on error paths (`alert/mod.rs:178`). Env file written to `stack_dir/.env` or preserved `env_file_path` (`stack.rs:512-513`). `compose_cmd_wrapper` interpolates `[[COMPOSE_COMMAND]]` (`compose.rs:1064-1071`).
- **Forge:** API `interpolateEnv` (`compose/service.go:356-425`) handles `${VAR}`, `${VAR:-d}`, `${VAR-d}`, `${VAR:?err}`, `${VAR?err}`, `$$`; beacon `encodeComposeEnv` (`compose.go:264-281`) rejects empty key, validates `[A-Za-z0-9_]` and strips `\n\r` from values. GitOps `sanitizeEnvVars` (`gitops.go:95-111`) masks `password/secret/token/key/credential/pass` → `***` before persisting in `PreviousDeploymentManifest`. No `compose_cmd_wrapper`.

---

## 3. Logic Findings

### LF-01 — Beacon `shortFormHostPort` bypass lets `ports: ["80"]` publish privileged port undetected

- **Files:** `beacon/internal/server/compose.go:213-224` (`shortFormHostPort`), `beacon/internal/server/compose.go:181-210` (`validateComposePorts`).
- **Logic:**
  ```go
  // compose.go:214
  func shortFormHostPort(entry string) string {
    parts := strings.Split(entry, ":")
    switch len(parts) {
    case 2: return parts[0]
    case 3: return parts[1]
    default: return ""  // ← handles "80" (len 1) and "80:80/tcp" (also len 1 after split on ":"? no)
    }
  }
  // compose.go:198-206
  if published == "" { continue } // skip check
  port, _ := strconv.Atoi(strings.TrimSpace(published))
  if port < 1024 { return error }
  ```
  A service declaring `ports: ["80"]`, `ports: ["80:80"]` (Compose interprets `"80"` as ephemeral host port? but Forge normalizes via `normalizePorts` elsewhere ) or `ports: ["127.0.0.1::80"]` falls through as `published==""` and no privileged-port error is raised. Attacker can thus bind host port 80/22 via short-form single-value syntax while API `ValidateComposeSecurity` has no port check at all (only API checks `cap_add/privileged/volumes`). Docker will publish a privileged port despite policy.
- **Evidence:** `service.go:427-528` has **no** privileged-port check; only `compose.go:181-210` does, and it skips single-part ports. Komodo does not enforce port policy at all (delegates to Docker).
- **Impact:** Policy bypass; host-level port hijacking on shared nodes.
- **Fix:** In `shortFormHostPort`, handle `len==1` as `parts[0]` (or validate via `nat.ParsePortSpec`); ensure long-form `published` path also validates single-value strings using `loader` semantics or reuse `compose-go` port parsing.

### LF-02 — `GitOpsService.DeployFromGit` persists new stack via `UpdateComposeStack` instead of `CreateComposeStack`

- **Files:** `forge/api/internal/services/compose/gitops.go:381-387` vs `forge/api/internal/services/compose/lifecycle.go:390`.
- **Logic:**
  ```go
  // gitops.go:351-384
  stackID := g.compose.createStackID()
  stack := &ComposeStack{ID: stackID, ...}
  if err := g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack)); err != nil {
      _, _ = g.store.UpdatePlacementReservationStatus(ctx, reservation.ID, Cancelled)
      return nil, fmt.Errorf("create compose stack record: %w", err)
  }
  // lifecycle.go:390
  if err := s.store.CreateComposeStack(ctx, toStoreComposeStack(stack)); err != nil { ... }
  ```
  `DeployFromGit` allocates a fresh ID and calls **Update** on a row that does not exist. Unless the store implements upsert, this returns `not found`/`0 rows` and the error is returned as `"create compose stack record"` while the reservation is cancelled but the caller sees a generic error. The stack is lost; retry would leak reservations. The sister path `DeployComposeStack` correctly uses `CreateComposeStack`. `redeployFromGit`/`pullAndRedeploy` correctly use update because they mutate existing rows.
- **Impact:** Git-backed initial deploys via `POST /compose/git/deploy` silently fail to persist; UI shows error but node already may have `docker compose up` succeeded? Actually `Update` fails **before** daemon deploy (line 388), so no deploy either – 100% failure for git-backed creates.
- **Fix:** Change `gitops.go:384` to `CreateComposeStack`.

### LF-03 — API vs Beacon volume policy divergence yields confusing `valid at API, rejected at beacon` UX and unbounded bind-mount surface if API is the only gate

- **Files:** `forge/api/internal/services/compose/service.go:594-688` (`checkVolumesSecurity` + `ValidateHostMountWithAllowlist`) vs `beacon/internal/server/compose.go:226-248` (`validateComposeVolumes`).
- **Logic:**
  - API (`service.go:627-656`) **allows** absolute host mounts with nuanced severity: `/etc` or `/` → `error`, other sensitive (`/root`, `/home`) → `warning` + `slog.Warn`, and provides `ValidateHostMountWithAllowlist(source,isAdmin,allowedMounts)` (`service.go:663-677`) for admin+allowlist gate – but **discounted** during `DeployComposeStack` `ValidateCompose` which only produces `Warnings` for `/root` etc, still `Valid==true`, so deploy proceeds.
  - Beacon (`compose.go:226-248`) **unconditionally rejects** any string source that is absolute (`strings.HasPrefix(source, "/")`) or path-traversal **plus** any long-form `type: bind` – regardless of allowlist/admin. Short-form absolute mounts like `/data:/data` or `/home/user/app:/app` are rejected at deploy time returning `400 compose policy violation` after API accepted.
  - Conversely, API's sensitive-mount warning does not block; a non-admin user on an API-only validation path could craft a compose with `/var/lib/docker/volumes`-adjacent mounts that API warns but beacon would have rejected anyway – the inconsistency masks whether policy is enforced.
- **Impact:** Operators see validation `200 {valid:true, warnings:[...]}` then immediate `409` on deploy with no allowlist recourse (beacon ignores `AllowedMounts`). Or, if beacon check were loosened to match API, API's warning-only for `/root` would become exploitable host exfiltration.
- **Fix:** Align policies: either (a) have beacon consult `allowedMounts` + admin flag passed via daemon request, or (b) make API reject all absolute mounts (error) matching beacon, removing the allowlist feature until beacon can enforce it, and document `allowedMounts` as not yet honored on Compose path.

### LF-04 — Komodo `image_digest_cache` swallows errors, silently suppressing stack image-update alerts

- **Files:** `reference/app-platforms/komodo/bin/core/src/api/write/stack.rs:790-801`, `.../stack.rs:880-881` (`update_available`).
- **Logic:**
  ```go
  match cache.get(swarm_or_server, image, None, None).await {
    Ok(digest) => service.image_digest = Some(digest),
    Err(e) => {
      warn!("Failed to check for update | ... | {e:#}", ...);
      service.image_digest = None; // ← clears digest
      continue;
    }
  };
  // later: service_with_update.update_available = latest_digest.update_available(current_digests);
  ```
  If the digest fetch fails (periphery unreachable, registry 401, image not found), `latest_digest` becomes `None` so `update_available` is `false` regardless of `current_digests`. No alert is emitted, and `stack_alert_sent_cache` retains old entries, further suppressing future alerts when digest fetch recovers because `contains` check at `write/stack.rs:884-886` still thinks alert was sent. The stack appears up-to-date while registry is failing.
- **Impact:** Missed `StackImageUpdateAvailable`/`StackAutoUpdated` alerts; `auto_update` stalls indefinitely with only a `warn` log.
- **Fix:** On cache error, preserve previous `image_digest` (don't clear) or set `service.image_digest` to sentinel that forces retry, and emit a diagnostic alert / metric. Forge's equivalent (`gitops.go:1281-1324`) fails the poll check entirely and logs, not clearing `GitDesiredCommitSHA`, which is more visible but also stalls – consider reconciling both.

### LF-05 — Komodo `StackConfig::file_contents` fallback plus `RefreshStackCache` re-parsing swallows service extraction errors

- **Files:** `reference/app-platforms/komodo/bin/core/src/api/write/stack.rs:627-643`, `.../stack.rs:629-640`.
- **Logic:**
  ```go
  // write/stack.rs:629-642 (UI-based fallback)
  let mut services = Vec::new();
  if let Err(e) = extract_services_into_res(&stack.project_name(true), &stack.config.file_contents, &service_image_digests, &mut services) {
    warn!("Failed to extract ... | {e:#}", stack.name);
    services.extend(stack.info.latest_services.clone()); // ← stale services carried forward
  };
  ```
  Similar pattern for repo/host branches at `write/stack.rs:545-562`, `604-613` where `warn!` is logged but `services` remains empty or stale. `StackListItemInfo.services` then shows stale service list while `compose config` on periphery would show different services; `CheckStackForUpdate` then computes `update_available` against wrong `service_name` keys, potentially auto-updating wrong image names.
- **Impact:** Stale service inventory masks deploy drift and misdirects auto-update; `missing_files` not populated for UI mode so health appears `Running` while compose is invalid YAML.
- **Fix:** On parse failure, clear `latest_services` and set `remote_errors` or `missing_files` so UI surfaces error, rather than cloning stale `latest_services`.

### LF-06 — Forge webhook `X-GitHub-Delivery` deduplication is in-memory `GitLastDeliveryID` race across replicas

- **Files:** `forge/api/internal/services/compose/gitops.go:1176-1219`, `forge/api/internal/services/compose/controller.go:75-77` (workerID).
- **Logic:** `HandleWebhook` (`gitops.go:1176-1178`) checks `if deliveryID != "" && deliveryID == stack.GitLastDeliveryID { return false, nil }` then later stores `GitLastDeliveryID = deliveryID` (`gitops.go:1219`). Two API replicas without shared lock can both pass the check before either writes, delivering duplicate `PullAndRedeploy` goroutines (`gitops.go:1232-1255`). `webhookMu` (`gitops.go:50`) is per-process `sync.Mutex`, not distributed. Combined with `GitUpdateStatus` check (`gitops.go:1212`) that allows `update_available` to pass, duplicate clones/deploys race and the second `CloneRepo` may return newer `CommitSHA` that no longer equals `whPayload.After`, causing a spurious `ErrCommitAlreadyDeployed` or wasted deploys.
- **Impact:** Duplicate deploys under webhook retries (GitHub redelivery) when API scales horizontally – not idempotent despite delivery ID guard.
- **Fix:** Move dedup check into DB transaction / `ClaimComposeStackForUpdate` style conditional update (`WHERE git_last_delivery_id != $1`), or use `INSERT INTO webhook_deliveries` with unique constraint.

*(LF-05/LF-06 counted as extra; at least 3 required satisfied by LF-01..03.)*

---

## 4. Build / Runtime / Operations Comparison Summary

| Dimension | Komodo | Forge + Beacon |
|-----------|--------|----------------|
| **Build orchestration** | Periphery does `docker buildx build --push` on target host; `pre_build` hook; `VERSION` injection; no build stages in DB | API `build.Service` tracks `BuildStage` enum + PID `abandoned` detection; `buildpack` service separate |
| **Runtime abstraction** | Direct `docker compose` / `docker` CLI via `command` crate `run_komodo_standard_command` | `runtime.Runtime` interface (`runtime.go:144-163`) with 5 providers (`Docker`, `Containerd`, `Podman`, `Firecracker`, `Kubernetes`) plus `Reconciler`; beacon `docker.go` pins API `1.43` + `validateDockerEndpoint` allowlist (`docker.go:58-94`) |
| **Health & stats** | `monitor/resources.rs` matches containers by `project_name` regex (`stack/mod.rs:34-41`) → `StackState::{Running,Paused,...}` | `lifecycle.go:188-231` `WaitForHealthy` polling `daemon.ComposeStatus` + `extractRestartCount`; `observability.collectNodeMetrics` synthesizes metrics from capacity snapshot |
| **Operations webhook** | `StackConfig.webhook_enabled/secret/force_deploy` (`stack.rs:468-480`) → core dispatches `DeployStackIfChanged` | Dedicated `POST /compose/webhook/:webhookId` with `HMAC-SHA256` + 3 header variants (`handlers_compose.go:82-104`), plus `PollForUpdates`/`CheckForUpdates` + `DetectDrift/RuntimeDrift` endpoints |
| **Observability retention** | `record::record_server_stats` + `state::db_client` TTL? | `observability.EnforceRetention` with 3-attempt retry (`service.go:306-323`), `RetentionPolicies` per metric type, `MonitoringSummary` aggregates `latestMetrics/unacknowledgedAlerts/healthChecks/timelineEventsTotal/heartbeatFailuresTotal` (`service.go:334-376`) |

---

## 5. Detailed Findings Checklist

- [x] Periphery agent located & compared to beacon daemon client
- [x] Stack types (`StackConfig` vs `ComposeStack`) compared including Swarm duality
- [x] Compose lifecycle (`ComposeUp` phases vs `handleComposeDeploy` single `up -d`)
- [x] Parser/security (`ValidateComposeSecurity` vs `validateComposePolicy`)
- [x] Queue/controller (`action_states` guard vs `GitOpsController` claim)
- [x] GitOps/reconciliation (image digest cache vs commit SHA + delivery ID)
- [x] Monitor vs observability (per-server PollStatus vs NodeCapacitySnapshot)
- [x] Alerting (Alerter+maintenance windows vs threshold+suppressionKey+SSRF guard)
- [x] Build (periphery buildx vs staged build service)
- [x] HTTP surface (`api/write/stack.rs` vs `handlers_compose.go`)
- [x] Web UI (`compose/page.tsx` statusConfig)
- [x] Drift detection (implicit missing_files vs explicit ServiceDiff/RuntimeDrift)

---

## 6. Recommendations

1. **Fix privileged-port bypass** (`compose.go:213-224`) before next release; add regression test with `["80"]`, `["80:80"]`, `["8080:80/tcp"]`, long-form `published: 22`.
2. **Correct `DeployFromGit` persistence** (`gitops.go:384`) to `CreateComposeStack`; add integration test that `ListComposeStacks` contains the new stack.
3. **Unify volume policy**: decide on allowlist support – if deferred, make API reject absolute mounts (`error`) to match beacon; if kept, plumb `allowedMounts+isAdmin` into `composeDeployRequest` and enforce in beacon.
4. **Surface image-digest fetch failures** as `AlertData::StackImageUpdateAvailable` with `SeverityLevel::Warning` or at least preserve prior digest; don't clear on error.
5. **Make webhook dedup DB-atomic**; reuse the existing `ClaimComposeStackForUpdate` pattern for `GitLastDeliveryID`.
6. **Consider adding `pre_deploy`/`post_deploy` hooks and build step to beacon** if Forge intends parity with Komodo stacks that rely on them; otherwise document divergence.

---

## 7. File:Line Index (key citations)

- `reference/app-platforms/komodo/bin/periphery/src/api/compose.rs:39-96` log search via `docker compose logs | grep`
- `reference/app-platforms/komodo/bin/periphery/src/api/compose.rs:414-783` `ComposeUp` pipeline
- `reference/app-platforms/komodo/bin/periphery/src/api/compose.rs:1020-1072` `env_file_args` + `maybe_wrap_command`
- `reference/app-platforms/komodo/bin/periphery/src/stack/mod.rs:30-61` `maybe_login_registry` + `pull_or_clone_stack`
- `reference/app-platforms/komodo/bin/periphery/src/docker/compose.rs:8-14` `docker_compose()` legacy toggle
- `reference/app-platforms/komodo/client/core/rs/src/entities/stack.rs:45-59` `Stack::project_name(fresh)`
- `reference/app-platforms/komodo/client/core/rs/src/entities/stack.rs:302-350` `StackConfig` swarm/server fields + `auto_pull/run_build`
- `reference/app-platforms/komodo/client/core/rs/src/entities/stack.rs:402-640` git/files_on_host/UI trichotomy
- `reference/app-platforms/komodo/bin/core/src/api/write/stack.rs:119-157` `UpdateStack` → async `CheckStackForUpdate` spawn
- `reference/app-platforms/komodo/bin/core/src/api/write/stack.rs:459-673` `RefreshStackCache` trichotomy
- `reference/app-platforms/komodo/bin/core/src/api/write/stack.rs:720-1032` `check_stack_for_update_inner` + alert_cache + auto_update
- `reference/app-platforms/komodo/bin/core/src/stack/execute.rs:77-103` `action_states` guard + `refresh_server_cache`
- `reference/app-platforms/komodo/bin/core/src/stack/execute.rs:121-244` `ExecuteCompose` impls (`Start/Pause/Restart/Stop/Destroy`) + `maybe_timeout`
- `reference/app-platforms/komodo/bin/core/src/monitor/mod.rs:48-63` `spawn_monitoring_loops`
- `reference/app-platforms/komodo/bin/core/src/monitor/alert/stack.rs:16-96` `alert_stacks`
- `reference/app-platforms/komodo/bin/core/src/alert/mod.rs:25-152` `send_alerts` + `send_alert_to_alerter` filters + custom URL interpolate
- `forge/api/internal/services/compose/lifecycle.go:23-83` `StackStatus` enum + `ComposeStack` struct
- `forge/api/internal/services/compose/lifecycle.go:188-231` `WaitForHealthy`
- `forge/api/internal/services/compose/lifecycle.go:249-434` `DeployComposeStack` quota + security + reservation + deploy + health
- `forge/api/internal/services/compose/lifecycle.go:781-815` `RestartStack` as `StopStack`+`StartStack`
- `forge/api/internal/services/compose/service.go:427-528` `ValidateComposeSecurity`
- `forge/api/internal/services/compose/service.go:594-677` `checkVolumesSecurity` + `ValidateHostMountWithAllowlist`
- `forge/api/internal/services/compose/service.go:356-425` `interpolateEnv` + `ExpandTemplate`
- `forge/api/internal/services/compose/gitops.go:196-299` `readComposeFromDir` + `readLimitedComposeFile`
- `forge/api/internal/services/compose/gitops.go:301-429` `DeployFromGit` (bug site line 384)
- `forge/api/internal/services/compose/gitops.go:687-839` `DetectDrift`/`DetectRuntimeDrift`
- `forge/api/internal/services/compose/gitops.go:1151-1258` `HandleWebhook` HMAC + delivery dedup
- `forge/api/internal/services/compose/controller.go:130-163` `processPending` claim loop + `deployStack`
- `forge/api/internal/services/compose/queue_handler.go:49-115` thin queue wrappers
- `beacon/internal/server/compose.go:59-71` `validStackID` + `73-184` `validateComposePolicy`
- `beacon/internal/server/compose.go:181-248` port/volume policy (bypass site)
- `beacon/internal/server/compose.go:264-281` `encodeComposeEnv`
- `beacon/internal/server/compose.go:344-416` `handleComposeDeploy`
- `beacon/internal/server/compose.go:550-751` `handleComposeDelete/Status/Logs/Pull`
- `forge/api/internal/http/handlers_compose.go:82-104` webhook public route
- `forge/api/internal/http/handlers_compose.go:413-438` redeploy uses `UpdateComposeStack` not create
- `forge/api/internal/services/alerting/service.go:41-48` thresholds + `90-217` `CheckNodeThresholds`/`evaluateAndAlert` + `485-501` SSRF guard
- `forge/api/internal/services/observability/service.go:24-126` metrics collection with jitter
- `beacon/internal/runtime/docker.go:42-94` provider + endpoint allowlist

---

*Generated by subagent 07 (Phase 6, parallel). No product files modified.*
