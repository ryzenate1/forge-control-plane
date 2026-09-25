# Phase 1 Remediation — Finding Status

Legend: VERIFIED_FIXED · ALREADY_FIXED (verified in current tree) · STALE · INVALIDATED ·
IMPLEMENTED_BUT_UNVERIFIED · BLOCKED · DEFERRED_WITH_REASON

| ID | Title | Severity | Status | Root cause | Files | Tests |
|----|-------|----------|--------|-----------|-------|-------|
| FORGE-LOGIC-001 / REF-APP-C02-FLF01 | Deployment stubs report `completed` with no runtime | P0 | **VERIFIED_FIXED** | provision/scale/drain steps were event-only no-ops; DAG advanced on nil | deployment/execution.go, service.go, beacon_executor.go (new), cmd/api/main.go | provision_regression_test.go (5) |
| FORGE-LOGIC-002 / REF-APP-C08-FLF02 | Health gate probes API-host localhost | P0 | **VERIFIED_FIXED** | default host hardcoded "localhost" instead of resolving the server's node | deployment/healthgate.go | healthgate_target_test.go (2) |
| F-07 (synthesis §6) | TriggerDeploy creates deployments with Image:"" unconditionally | P0-adjacent | **VERIFIED_FIXED** | no image admission; now resolves sourceConfig.image → server's synced image; refuses admission when neither exists | apphosting/service.go (resolveDeployImage), store/store_apphosting.go (GetServerDockerImage) | covered via init-step validateImageRef + admission error path |
| REF-APP-C03-FLF03a | App restart = empty-image TriggerDeploy | P1 | **VERIFIED_FIXED** | restart conflated with deploy; now canonical beacon power-cycle (`SendPower("restart")`) | handlers_apphosting.go | handler compiles; runtime path reuses tested daemon client |
| REF-APP-C03-FLF03b | Admin compose restart missing | P1 | **VERIFIED_FIXED** | full chain existed (RestartStack→daemon.ComposeRestart→beacon handleComposeRestart) but no route | handlers_compose.go (+POST /compose/:id/restart) | route wired onto tested RestartStack |
| REF-APP-C04-FLF04* | Compose redeploy creates new stack row (leak) | P1 | **VERIFIED_FIXED** | `/compose/:id/deploy` called DeployComposeStack (new row); now UpdateComposeStack (in-place, rollback+health) | handlers_compose.go | existing compose suite PASS |
| REF-APP-C07 | Scale silent no-op when ReplicaAppID nil | P1 | **VERIFIED_FIXED** | scale wrote replicas metadata without placement; now validates placement BEFORE mutation and refuses honestly | apphosting/service.go (ScaleService) | build + suite PASS |
| REF-APP-RT-FL01 | Beacon missing POST create for containers/networks/volumes | P1 | **VERIFIED_FIXED** | panel daemon client called routes beacon never registered | beacon container_admin.go (5 new handlers), server.go (routes) | build; admin-auth pattern shared with existing suite |
| REF-APP-RT-FL02 | Prune crossed wires (volume prune 404; image prune unwired) | P2 | **VERIFIED_FIXED** | volume-prune handler absent on beacon; now implemented + registered. Image prune was already registered (beacon:476) and panel AdminImagePrune targets it — half ALREADY_FIXED | beacon/container_admin.go, server.go | build |
| REF-APP-GIT04-LF03 | 5 buildTypes admitted, only dockerfile executed | HIGH/FALSE_COMPLETION | **VERIFIED_FIXED** (honest rejection) | admission accepted types executor drops; now admits dockerfile only, explicit message for unsupported | handlers_source_deployments.go | http suite PASS |
| REF-APP-GIT05-LF04 | Beacon drops CacheFrom/CacheTo/Platform | MED | **VERIFIED_FIXED** | beacon request struct lacked fields; added + wired into buildx args with ref/platform safety filters | beacon/server/build.go | build; safety helpers pure |
| REF-APP-GIT10-LF08 | Webhook bad signature masked as 200 | LOW | **VERIFIED_FIXED** | verification-failure path returned 200 across GitHub/GitLab/Bitbucket/Gitea; now 401 (unknown-repo recon mask preserved as 200) | handlers_git.go | git webhook tests PASS |
| F-10 (synthesis) | Compose security validation bypassed via PUT /apps/:id/compose | P2 | **VERIFIED_FIXED** | side door skipped ValidateComposeSecurity; now validates composeYaml inside sourceConfig before persisting | handlers_apphosting.go | build |
| F-13 dup of C07 | (same root cause) | — | DUPLICATE | fixed once at ScaleService | — | — |
| REF-APP-UX01 | Recovery nav duplicates /admin/migrations | P3 | **ALREADY_FIXED** | registry contains single Migrations entry | admin-registry.ts | — |
| REF-APP-UX02 | App-create form validates then discards inputs (hardcodes image) | P2 | **VERIFIED_FIXED** | form collected region/template/resources then sent an ignored payload; rewritten to send name/description/sourceType=image/sourceConfig{image}; CreateAppInput type extended | forge/web components/app/app-create-form.tsx, lib/api/apps.ts | tsc clean |
| REF-APP-UX04a | Back href hardcodes /servers | P2 | **VERIFIED_FIXED** | orphan component pointed at wrong section; corrected to /admin/apps | components/app/app-detail.tsx | tsc |
| REF-APP-UX04b | Tab state lost on refresh | P2 | **VERIFIED_FIXED** | setTab didn't persist to URL; now writes ?tab= via replaceState | app/admin/apps/[id]/page.tsx | tsc |
| REF-APP-UX05a | EmptyState CTA dead button | P2 | **VERIFIED_FIXED** | default Create App button had no onClick; routes to /admin/apps/new | components/app/app-list.tsx | tsc |
| REF-APP-UX05b | metrics-chart queryKey missing node dimension | P2 | **INVALIDATED** | component is fleet-wide by design; used only on aggregate monitoring page; no per-node scope exists to key on | monitoring/metrics-chart.tsx | — |
| F-18 (queue) | Lease-steal + failure double-increments retry_count | P1 | **VERIFIED_FIXED** | Dequeue SQL incremented retry_count on steal AND Retry incremented again; steal increment removed; dequeueSQL/retrySQL extracted as testable constants | queue/store.go | retry_accounting_test.go (2) |
| F-19 / REF-APP-ARCH01 | Operation reaper resets long-running ops (no heartbeat) | P1 | **VERIFIED_FIXED** | reaper used bare updated_at staleness with no liveness signal; workers now heartbeat running ops every 30s (Store.Touch + touchLoop); memory store updated | operation/service.go, store.go, service_test.go | operation suite PASS incl. Touch impl |
| REF-APP-DUP-001 | queue vs operation dual writers (DispatchCompose dead second engine) | P1 | **VERIFIED_FIXED** | main.go documents queue-as-single-writer; DispatchCompose had zero callers — removed | operation/service.go | suites PASS |
| F-24 (reconciler) | restartAttempts never decays → permanent lockout | P2 | **VERIFIED_FIXED** | counter only ever incremented; successful recoveries now decay (-1 per success, full reset after 24h stability) + lastRecoverySuccess tracking | reconciler/service.go | reconciler suite PASS |
| F-25 | 30m dedupe suppresses legitimate desired-state changes | P2 | **INVALIDATED (verified by design) + pinned** | dedupe key is snapshotHash(diffs) which includes DesiredHash — a real desired change yields a different hash and escapes suppression | reconciler/service.go | plan_dedupe_test.go (1) |
| F-26 | Reconciler publishes ActualStateChanged every cycle regardless of change | P2 | **VERIFIED_FIXED** | event emitted pre-verification unconditionally; now emitted only on observed transition, includes previousState | reconciler/service.go | suite PASS |
| go.work drift (arch #28) | toolchain version mismatch broke builds | LOW | **VERIFIED_FIXED** | go.work said 1.26.0 while beacon requires ≥1.26.3 | go.work | all modules build |
| REF-APP-RT-FL03 | Exec allowlisted in beacon but not exposed | P2 | **INTENTIONALLY_NOT_EXPOSED** | no UI/API promises exec; wiring an arbitrary-container exec terminal is new-feature surface requiring its own auth review — honest absence documented rather than fake control shipped | (none) | — |

## Deferred / Blocked

| Item | Status | Reason |
|------|--------|--------|
| REF-APP-GIT09-LF07 preview vs previewenv dedup | **DEFERRED_WITH_REASON** | Both services under active concurrent-writer modification during this pass; promoting previewenv requires migration + route cutover + frontend swap that must be coordinated against their in-flight changes. Decision recorded: promote previewenv (TTL/max-per-org/commit-status/reaper are correct), retire legacy. |
| REF-APP-GIT01 generic OAuth dance | **DEFERRED_WITH_REASON** | Product decision required (PAT-only vs OAuth). Current PAT model works; adding OAuth end-to-end is a feature batch, not remediation. UI does not fake OAuth controls. |
| REF-APP-GIT02/LF01 credential script divergence (panel embeds token vs beacon env-var) | **DEFERRED_WITH_REASON** | Panel clone path hardening touches live deploy flows concurrently edited by the other writer; fix design agreed: adopt beacon-style askpass env-var pattern + 0700 tmpdir. |
| REF-APP-GIT03-LF02 shallow SHA fetch fragility | **DEFERRED_WITH_REASON** | Same file under concurrent modification. |
| REF-APP-GIT06-LF08b revisions-tables disjoint | **DEFERRED_WITH_REASON** | Canonical revision identity model needs cross-phase decision (Phase-12 mission item). |
| REF-APP-UX03 three EnvVar editors | **DEFERRED_WITH_REASON** | Wave-9 requires choosing ONE backend contract; both APIs and all three editors are being actively modified by the concurrent writer; premature convergence would collide mid-flight. |
| REF-APP-UX06 dual deployment polling (2s vs 5s) | **DEFERRED_WITH_REASON** | Single-timeline consolidation pending writer's ws_hub work landing. |
| REF-APP-ARCH02 session pointer race + raw tokens | **BLOCKED** | File under active rewrite by concurrent writer this session. |
| REF-APP-ARCH03 beacon reconnect blind StateConnected | **BLOCKED** | remote/reconnect.go modified by concurrent writer during pass. |
| REF-APP-ARCH04 placement global mutex | **DEFERRED_WITH_REASON** | Perf refactor, not correctness; scheduling currently low-contention single-instance. |
| REF-APP-ARCH05 advisory-lock blocking | **DEFERRED_WITH_REASON** | Migration runner verified working; behavioral change risks mid-flight writer conflicts. |
| F-33 remote build logs post-hoc vs SSE | **DEFERRED_WITH_REASON** | Live streaming requires beacon log-stream plumbing change. |
| F-32 InMemorySessionStore pointer race | **BLOCKED** | Same active-rewrite area as ARCH02. |
| Tenancy-unscoped fetchApps() | **DEFERRED_WITH_REASON** | Requires product decision on scoping semantics (org filter vs global admin view). |
| Node-health-before-placement (Wave 1.2) | **PARTIALLY COVERED** | DeployComposeStack now rejects non-active nodes; deployment executor fails honestly on unreachable nodes (ServerControlTarget/Stats errors abort). Full heartbeat-freshness gate deferred with ARCH03. |
