# FINAL REFERENCE ECOSYSTEM REPORT — Forge Control Plane vs 27-Repository Corpus

**Program:** 5-phase, 25-subagent forensic research audit
**Corpus:** 27 local reference checkouts under `reference/` (app-platforms ×9, game-hosting ×6, backup ×2, networking ×3, operations ×1, orchestration ×3, large-systems ×2)
**Forge surface:** `forge/api` (Go/Fiber, ~197 migrations, 87 service packages, 126 handler files), `forge/web` (Next.js, 125 routes), `beacon` (Go daemon, ~80 handlers), `packages/*`
**Evidence base:** 25 subagent reports + 5 phase syntheses + clustering rationale, all under `audits/`, all file:line cited (SOURCE_VERIFIED unless marked)
**Date:** 2026-08-23

---

## 1. Executive Summary

Three headline conclusions:

1. **Forge's capability breadth exceeds every single reference project.** No one reference combines game hosting + app PaaS + compose + GitOps + multi-runtime + placement/reservations/drain/evacuation + integrated backup + gateway primitives + org tenancy. Forge has all of it as code.
2. **The dominant defect class is not missing capability — it is integration debt.** Across 5 phases and ~90 consolidated findings, the recurring pattern is: correct primitive built, wrong wiring attached. Examples: an outbox with zero subscribers; fencing tokens nothing enforces; certs that never leave Postgres; a backup-progress WS beacon publishes and the panel never proxies; an HTTP-01 solver implemented but never mounted; LXC/KVM adapters whose provider field the beacon silently drops.
3. **A smaller but critical class is false completion and unsafe defaults** — deployment stubs reporting `completed`, health gates probing `localhost`, policies applying to all routes, failover defaulting to Evacuate, retention AND-vs-OR inversion risking data loss.

The strategic answer to "rebuild or activate" is unambiguous: **activate**.

## 2. Reference Corpus Inventory

See `audits/reference-clustering.md` for the full verified 27-row table (commit, branch, language, size, maturity). Highlights: nomad 6k files (largest), portainer 5.3k, coolify 3k, rancher 3k; river/kopia/restic/incus/caddy/netbird active Go; pufferpanel in maintenance.

## 3. Forge Capability Inventory

| Domain | Status | Key evidence |
|---|---|---|
| Game lifecycle | ✅ superset of Wings/Pelican | desired/actual split absent in all refs; Beacon ≈4× Wings routes |
| Eggs/templates | 🟡 ported + broken regex validator | `validateVariableValue` blocks PTDL imports (F-G-08) |
| App platform | 🟡 strong model, broken execution | deployment stubs report completed (F-01); health gate localhost (F-05) |
| Compose | ✅ solid | queue-backed deploy, policy validation, GitOps controller |
| Git/build | 🟡 API claims 5 builders, executes 1 | `source_deploy.go:81`; preview duplicate impls |
| Backup | 🟡 artifact model works, crypto+retention defects | AND-vs-OR retention; OOM encryption; dead progress WS |
| Networking/Gateway | 🔴 densest P0 cluster | empty-sync wipes config; fictional Caddy modules; all-policies-all-routes; certs undelivered; admin UX non-functional |
| Queue/operations | ♻️ duplicated engines | dual-write same tables; verdict: no River needed |
| Placement/scheduling | 🟡 sound core, missing policy layer | constraint bonus overflow; no reschedule/spread/blocked-eval |
| Runtime | ⚠️ phantom providers | lxc/kvm dropped on floor; firecracker unsafe; capability system dead |
| Tenancy/RBAC | ✅ superset + one escalation hole | subuser `*` grant without subset check (F-G-22) |
| Observability | 🟡 good primitives, unwired durability | outbox write-only; heartbeat classifier best-in-corpus |

## 4. Clustering Rationale

Documented in `audits/reference-clustering.md`. Five clusters ordered by comparative value: app-platform (widest overlap) → game-hosting (canonical correctness horizon for Beacon) → backup (deep technical) → networking (gateway abstraction test) → orchestration/ops (control-plane capstone).

## 5. Phase-by-Phase Findings

- Phase 1 (App Platform): 31 indexed findings; P0s: deployment stubs lie complete, health-gate localhost, restart-as-deploy, Beacon create/prune 404s → `audits/phase-01/synthesis.md`
- Phase 2 (Game Hosting): 22 findings; P1s: restoring_backup lock missing, subuser `*` escalation, regex validator blocks egg imports, CPU conflation → `audits/phase-02/synthesis.md`
- Phase 3 (Backup): 18 findings; P0/P1: sidecar transplant forgery, EncryptReader OOM, retention inversion, prune orphans S3, progress WS dead end → `audits/phase-03/synthesis.md`
- Phase 4 (Networking): 25+ findings — densest P0 cluster of the program → `audits/phase-04/synthesis.md`
- Phase 5 (Orchestration/Ops): 3 structural facts (write-only events, no leader election, unenforced fences) + queue/runtime/storage findings → `audits/phase-05/synthesis.md`

Canonical index with CONFIRMED_BY chains: `audits/MASTER_FINDING_INDEX.md`.

## 6. Capability Parity Matrix

Legend: ✅ parity-or-better · 🟡 partial · 🟠 unwired · 🔴 missing/broken · ⚠️ false-completion · ♻️ duplicate

| Capability | Best reference | Forge | Verdict |
|---|---|---|---|
| Game lifecycle/state machine | Pelican/Wings | superset (desired/actual, transitions audit) | ✅ |
| Console/SFTP/files | Wings | parity+ (bounded replay, quota, openat2 confinement) | ✅ |
| Node heartbeat/health | Rancher ping / Incus threshold | 6-state classifier richer than all refs | ✅ |
| Transfers | Ptero wings→wings | control-plane mediated v1, resumable chunks | ✅ |
| Egg variables | Pterodactyl | ported but regex bug | 🟡 |
| Config-file patching | Wings parser | stored-not-applied | 🔴 |
| App deployments | Coolify/Dokploy | 4 strategies modeled, steps are stubs | ⚠️ |
| Health gates | Dokku checks | probes wrong host | 🔴 |
| Build pipeline | Dokploy 6 builders | 2 wired, contract overstates | ⚠️ |
| Preview envs | Dokploy/Coolify TTL+status | two impls, legacy wired | ♻️ |
| Compose stacks | compose spec + Portainer | solid incl. GitOps poll | ✅ |
| Container mgmt UI | Portainer | UI present, Beacon handlers missing (create/prune/exec) | 🟠 |
| Backup integrity | Restic/Kopia | sidecar trust + streaming bugs | 🔴 |
| Backup progress UX | Kopia counters / restic JSON | WS exists, panel drops it | 🟠 |
| Retention/prune | restic forget --prune union OR | three engines disagree, AND-inverted | 🔴 |
| Proxy config authority | Traefik tree / Caddy doc / NPM rows | five writers, one config doc | 🔴 |
| ACME | certmagic/lego refs | DNS-01 works; HTTP-01 dead; certs undelivered | 🔴 |
| Middleware attachment | Traefik named refs | all-to-all leakage | 🔴 |
| Durable queue | River | two hand-rolled engines, fixable locally | ♻️ |
| Periodic jobs | River/Nomad leader-gated | per-replica ticker duplicates | 🔴 |
| Placement scoring | Nomad normalized iterators | bonus overflow dominates | 🟡 |
| Drain/evacuation | Nomad drainer | plan ledger exists; dangerous default action | 🟡⚠️ |
| Multi-runtime | Incus honesty | phantom providers | ⚠️ |
| Overlay mesh | NetBird | none | 🔴 |
| Storage locality | Longhorn selectors | scored-but-dead vocabulary | 🟠 |
| Fleet membership | Rancher/Incus consensus | DB bookkeeping only (acceptable at scale) | 🟡 |
| Tenancy | Portainer ResourceControl | org/project/env superset; subuser hole | ✅⚠️ |
| Audit trails | NPM audits everything | game/app audited; network silent | 🟡 |

## 7. Hidden Forge Capabilities (implemented, invisible)

1. `GatewayAdapter.SetCertificate` — zero callers; TLS pipeline ends at Postgres
2. `acme.HTTPSolver()` — implemented challenger never mounted
3. `backupProgressWS` — beacon publishes; panel proxy union omits `backup`
4. `installer/service.go` 6-step workflow engine — persists rows, never executed
5. crossnode IngressSynchronizer — running every 30s, populated by nobody (destructively)
6. servicediscovery REST + PrivateNetworkPolicy — complete API, no UI, ACL test-only
7. `previewenv.Service` — TTL/reaper/commit-status exists; legacy `preview` wired instead
8. `security_headers` table + client lib — write-only, page link 404s
9. `river_queue` table — migrated, orphaned
10. StorageLocality scoring — computed field no caller sets
11. `server_orphan_remediations` — tracked, no resolution UI
12. Capabilities delta endpoint — beacon implements, nodes page doesn't show

## 8. Forge Logic Bugs Discovered Through Reference Comparison

~90 findings indexed (`MASTER_FINDING_INDEX.md`). Load-bearing P0s:
- Deployment execution stubs report success (`execution.go:249-311`)
- Health gate probes localhost, clamped so (`healthgate.go:11`)
- Subuser permission escalation via `*` (`store_users.go:297`)
- Regex validator blocks standard egg imports (`store_egg_variables.go:177`)
- Empty-rule ingress sync wipes gateway config every 30s
- All traffic policies apply to all routes (cross-tenant)
- Fictional Caddy modules brick route updates when any policy enabled
- Certificates never delivered to any gateway
- Failover defaults to Evacuate with no policy
- Retention AND-vs-OR inversion + SQL-only prune orphans S3 objects
- Unauthenticated backup sidecar enables cross-server transplant
- LXC/KVM silently execute as Docker

## 9–13. Integration Maps

- **Backend without UI:** servicediscovery, capabilities delta, nodeautoscale policies, billing service, queue inspector, transfer internals, security_headers, redirect rules, forward-auth.
- **UI without backend:** traffic page schema mismatch, cert upload → unregistered route (405), Recovery nav duplicating Migrations href, Security hardcoded pills + dead `/admin/domains/:id`, LB "Test Next" GET-only POSTed.
- **UI+API without runtime:** Docker admin create/prune/exec (daemon client targets nonexistent beacon routes); plugins metadata-only; buildpack Build disabled honestly.
- **Runtime without observation:** backup progress (WS dead end), install stage stream (exists, banner static), drain/transfer progress (state columns exist, no WS surface).
- **Observation without reconciliation:** reconciler publishes drift events before verifying actual state; git deployments never create revision rows (rollback can't reach them); firewall ephemeral (no desired state).

## 14. Duplicate Forge Systems

1. queue.Service vs operation.Service dual-writing operations tables
2. Dead vendored River fork + orphaned river_queue table
3. preview vs previewenv services
4. Three EnvVar editors + two APIs
5. Three template systems (DB eggs / FS game-templates / localStorage app-templates)
6. Three runtime abstractions
7. Five gateway writers + four adapter abstractions
8. Four retention/verification paths in backup
9. Two security-header middlewares kept in sync manually
10. Fencing implemented twice, different lease durations

## 15. Dead / Legacy

River fork (`forge/api/queue/`), river_queue, CaddyTLSManager, Traefik adapter (1,139 lines, doubly broken), installer workflow engine, legacy servers.transfer_state shim, volumes FK-stub table, security_headers consumers, legacy `/compose/projects/*`.

## 16. False Completion / Decorative

Deployment steps completing without provisioning; app start/stop desired_state writes with no reconciler; proxy-domain verify returning hardcoded true; fabricated adapter health for Caddy; scale silently no-op without ReplicaAppID; buildTypes accepted then rejected at deploy; tenancy cascade not filtering workload lists; LXC/KVM labels executing Docker.

## 17. Security Findings

Subuser `*` escalation; mount allowlist gaps (host-breakout via compromised admin); plaintext DNS zone credentials; Traefik rule injection via backticks; DNS-provider SSRF; trusted-proxy fail-open (XFF from any private peer; XFP in mTLS gate); constant CSP nonce fallback with strict-dynamic; L4 LB as internal-network probe primitive; unauthenticated backup sidecar transplant; session store pointer races + raw token keys.

## 18. Reliability Findings

Operation reaper duplicate-execution; lease-steal double retry-count (operation path); unhealthy-upstream oscillation loop; LB health checks ignoring their config; heartbeat history truncation stalling recovery; beacon reconnect declaring connected without probe; non-tx multi-table job transitions; per-replica periodic duplication; blocking advisory migration lock; auto-renewal goroutine panic kills loop permanently.

## 19. Architecture Findings

Write-only event durability; absent leader election (~30 daemons); fencing written-not-enforced firing on wrong edge; six unsynchronized controllers per server; tenant-blind scheduling/event/planning paths; config sprawl with unsafe failover default; idempotency per-entry-point not per-intent; publisher blocked by slowest subscriber with in-memory DLQ.

## 20. UX / Discoverability Findings

Entire networking admin UX non-functional against real schemas; console lacks ANSI fidelity, uses synthetic timestamps, cumulative network graph; triple env-var editor incoherence; tenancy not scoping lists; duplicate deployment polling; rate-limit error component never consumed; offline banners duplicated; empty-state CTAs dead; status tone maps divergent per page.

## 21. Best Reference Patterns

| Subsystem | Winner | Pattern worth adopting |
|---|---|---|
| Gateway model | Traefik | provider→router→middleware(named ref)→service declarative tree |
| True dry-run | Caddy /adapt vs /load semantics | validate must not apply |
| Apply discipline | NPM configure→test→reload w/ .err rename | per-host failure isolation + persisted online/offline meta |
| Repo integrity | restic index+prune / kopia maintenance | content-addressed verification, exclusive-lock prune |
| Progress protocol | restic message_type:status JSON cadence | throttled bytes_done/total_bytes |
| Job uniqueness signal | River UniqueSkippedAsDuplicate | callers must distinguish insert vs dedupe |
| Leader gating | Nomad leadership / River TTL row | maintenance daemons gated, event work SKIP LOCKED |
| Reschedule policy | Nomad delay functions + penalty nodes | attempts/backoff live ON the record |
| Profile composition | Incus ordered profiles last-wins | eggs become composable layers |
| Policy enforcement = mechanism | NetBird NetworkMap push | revocation takes effect via distribution, not advisory flags |
| Access-control binding | Portainer ResourceControl / NPM access lists | per-resource explicit grants, subset-checked |

## 22. Patterns NOT To Copy

Uncloud CRDT-no-quorum (trades consistency Forge needs); Rancher fleet/K8s dependency (opposite of single-VPS bootstrap); Longhorn block replication (backups+evacuation is the right durability model); monolithic CE Portainer codebase; Nginx string-template config generation; 1Panel verbatim-YAML single-host assumptions; host-tool sprawl at control plane; Komodo token-in-clone-URL credentials.

## 23. Forge Advantages

1. Game+app on one scheduling fabric (no reference does both)
2. Desired/actual split + state_transitions audit + generation concept — ahead of every game panel
3. Hysteresis heartbeat classifier — richest in corpus
4. Lease-outbox primitive structurally River-quality
5. Steal≠retry accounting — better than River's uniform attempts
6. Destructive-action confirm gates incl. mid-execution visibility — stricter than all references
7. Cross-node resumable chunked transfers with recovery tokens
8. Kernel-enforced file confinement (openat2 RESOLVE_BENEATH) absent in Wings
9. Correlation-ID plumbing end-to-end
10. Org→project→environment tenancy richer than any PaaS reference
11. Executor reuse discipline in recovery (seam drawn once, done right)

## 24. Genuine Missing Capabilities

Overlay mesh option; config-file parser (eggs config.files inert); HTTP-01 mounting + TLS-ALPN; typed install pipelines (PufferPanel-style ops) for heavy modded games; per-blob AEAD backup envelope; attribute-spread + reschedule policy in placement; distributed backup locks; per-file backup progress transport (data exists).

## 25. Existing Capabilities Needing Activation

The §7 hidden list plus: previewenv, installer workflows, capabilities badges, billing page, node autoscale UI, transfer cockpit, orphan center, servicediscovery self-registration, security-header editor wiring, queue inspector.

## 26. Highest-Value Integration Opportunities

1. Single-writer gateway (Traefik-shaped model behind one reconciler)
2. Event loop made real (PublishTx + relay consumers)
3. Leader election gate for maintenance daemons
4. Wire cert delivery + mount HTTPSolver
5. Queue consolidation (six correctness fixes)
6. Fix deployment execution stubs + health-gate target
7. Backup crypto/retention correction + progress WS end-to-end
8. Subuser subset enforcement + mount allowlist hardening
9. Provider honesty (delete/quarantine phantoms, capability-gate scheduling)
10. Tenant-tagging core records while volumes are small

## 27. Recommended Product Information Architecture

Keep goal-grouped shell; add Gateways (Routers/Services/Middlewares/Certs tabs replacing 7 scattered pages), Operations timeline (jobs+ops+drains+transfers+orphans unified), Nodes detail tabs (Health/Capabilities/Drain/Autoscale), Backups tabs (Jobs/Policies/Providers/Verifications); remove decorative entries until functional.

## 28–36. Recommended Architectures (per subsystem)

- **Control plane:** keep Postgres-as-single-writer; add leader election (advisory-lock or TTL row) gating maintenance daemons; add PublishTx bound to business transactions; demote in-memory registry to best-effort fast-path fan-out.
- **Beacon:** keep single daemon per node; reject unknown `provider` values instead of dropping; honest capability report (RuntimeProvider actually set); tunnel IP support for mesh.
- **Runtime:** fold the three abstractions into one; registry honesty rule (listed only if factory+capability+routing agree); capability-gate scheduling via CheckCapability; profile composition over eggs.
- **Gateway:** GatewayRouter/GatewayMiddleware/GatewayService/GatewayTarget tables; domains becomes a provider emitting routers; one reconciler owns desired-state convergence; true dry-run before apply; merge-not-replace writes; deterministic middleware order (rate-limit→blacklist→whitelist→CB→redirect); group-aware withdrawal.
- **Storage:** mounts + S3 artifacts remain the durability model; schedule-time locality checks with a single vocabulary; delete or wire the volumes stub.
- **Scheduler:** keep placement.Engine minus global mutex; normalize scores into bounded iterators; move attempts/backoff/penalty onto instance records; try-preferred-then-fallback sticky semantics; blocked-eval equivalent = pending-placement rows re-evaluated on capacity events.
- **Backup:** streaming AEAD with per-backup salt + AAD binding server_id:backup_name; union-OR retention converged across engines; prune inside advisory lock incl. S3 sweep marks; journal recovery on startup; throttled progress with early TotalBytes.
- **Operations:** queue.Service sole execution writer; operation.Service read-model; state-guarded CAS cancel; available_at delay column instead of sleeping workers; tx-wrapped transitions; unified idempotency namespace with dedupe signal; background-context heartbeats.
- **Networking:** explicit TRUSTED_PROXIES CIDR config replacing peer-private trust; validate DNS-provider URLs; fresh leaf key per renewal; encrypted DNS credentials migration mirroring 157; firewall desired-state table with reconciliation.
- **Game hosting:** wire restoring_backup server lock; fix regex validator; gate internal variables at creation; implement or remove config.files patching; seed FS templates into eggs.

## 37. Capability Activation Roadmap (ordered)

1. Stop destructive defaults (empty ingress sync, Evacuate fallback, fictional Caddy handlers)
2. Wire the four dead-but-built pipelines (certs, backup WS, HTTPSolver, event relay)
3. Close security holes (subuser subset, mount allowlist, DNS creds, trusted proxies, sidecar AAD)
4. Consolidate duplicates (queue/operation, gateway writers, preview pair, runtime abstractions)
5. Make execution real (deployment stubs, health gates, Beacon create/prune routes)
6. Surface hidden capabilities (Gateways page, Operations timeline, capabilities badges, billing)
7. Add missing policy layers (reschedule policy, spread, middleware attachment joins)
8. Honest providers (delete lxc/kvm/firecracker claims or implement behind build tags)
9. Tenant-tag core records
10. Polish UX debts (console fidelity, env-var unification, status tone centralization)

## 38. P0/P1/P2/P3/P4 Priorities

- **P0 (security/data-loss):** subuser escalation; retention inversion + orphaned S3 deletes; sidecar transplant; Evacuate-by-default failover; empty-sync gateway wipe; all-policies-all-routes leakage; plaintext DNS creds; mount allowlist gaps.
- **P1 (core broken):** deployment stubs; health-gate localhost; queue cancellation/backoff/tx bugs; certs undelivered; HTTP-01 dead; restoring_backup lock absent; reinstall-on-Docker failure; leader election absence.
- **P2 (unwired):** backup progress WS; previewenv; servicediscovery self-registration; capabilities delta UI; installer workflows; StorageLocality wiring; firewall persistence.
- **P3 (discoverability):** networking admin rebuild; console fidelity; env-var consolidation; status tone maps; empty-state CTAs; audit trails for network mutations.
- **P4 (polish):** metrics naming honesty; WRR counter isolation; redirect rule unification; template catalog seeding job.

## 39. "We Already Built This" Inventory

Deployment strategy DAG + revisions + health-gate plumbing; compose GitOps controller; multi-runtime factory; resumable chunked transfer protocol v1; lease-outbox store with DLQ; 6-state heartbeat classifier; hysteresis reconciler with plan dedupe/TTL; recovery coordinator lifecycle with ack timeouts; ACME service w/ 36 DNS providers; backup artifact/manifest/journal model; SFTP quota/rate-limit/idle controls; org/project/env tenancy + policy layer; correlation-ID propagation; openat2 file confinement; enrollment + token rotation.

## 40. "Users Cannot See This" Inventory

Backup progress bars (data flowing, transport dropped); cert issuance results (rows without TLS); service discovery health; node capabilities; autoscale policies/events; drain ledgers; transfer progress; orphan remediations; billing usage; queue depth/stuck ops; preview TTL behavior; install workflow stages.

## 41. "We Actually Need To Build This" Inventory

Overlay mesh option (tunnel IP minimum); egg config.files patcher; typed install pipeline for modded games; HTTP prober honoring HealthCheckConfig; distributed backup namespace locks; attribute-spread iterator; per-file backup progress transport; firewall desired-state table; leader-election wrapper; PublishTx API.

## 42. Final Strategic Conclusions

**The verdict:** Forge is not missing its product. It is missing its *wiring*. The reference corpus proves that each Forge primitive has an analogue that ships — Traefik's single config authority, restic's exclusive-lock prune, River's leader-gated periodic jobs, NetBird's enforcement-as-distribution, Nomad's records-carry-retry-policy. Forge already built structurally comparable (sometimes superior) versions of these primitives; what fails is the last hop: publisher to subscriber, DB to gateway, beacon to panel, desired to actual, claimant to enforcer.

Rough decomposition of the ~90 indexed findings:
- genuinely missing subsystem-level capability: ~8 items (§24) ≈ 9%
- partially implemented needing completion: ~25 ≈ 28%
- implemented but unwired/unexposed: ~30 ≈ 33%
- implemented but broken logic: ~20 ≈ 22%
- duplicated systems: ~7 pairs
- false-completion/decorative: ~10 surfaces

The next engineering phase should be an **activation program**, sequenced by §37's roadmap, not new subsystem construction. Every P0 in §38 is fixable within existing tables and services; none requires adopting any reference architecture wholesale. The corpus's best patterns (§21) slot into Forge's existing seams precisely because Forge drew those seams correctly in the first place.
---
## Phase 6 Addendum — Remaining Reference Deep-Dive (10 Parallel Agents)

This addendum extends Phases 1–5 with the surfaces that were shallow or explicitly excluded, now mined in parallel by 10 subagents (all file:line verified).

**Covered:** 1Panel app store (SA-01), 1Panel website/SSL (SA-02), 1Panel database+runtime (SA-03), 1Panel infra/monitor (SA-04), PufferPanel daemon & server operations (SA-05), PufferPanel templates vs game-templates (SA-06), Komodo stacks/periphery (SA-07), Uncloud WireGuard mesh (SA-08), Dokku+CapRover minimal PaaS (SA-09), Docker-Compose spec fidelity + Coolify/Dokploy builds (SA-10). Reports: `audits/phase-06/subagent-*.md` (3112 lines); synthesis: `audits/phase-06/synthesis.md`.

### New parity updates (amend §6)

- **App catalog:** 1Panel per-site catalog vs Forge `store_catalog.go` — Forge correctly stricter on compose policy; but Forge appstore has fail-fast template bug and orphan-on-uninstall (P1). Status stays 🟡.
- **Website/SSL:** 1Panel OpenResty per-site routing confirms Forge's central gateway is the right multi-node equivalent. But Forge verify stub (hardcoded true), chain-leaving renewOnce, DNS env leakage, global mu across network I/O are newly filed P1s — Networking stays 🔴 with added evidence from second reference family.
- **Database:** MySQL multi-host/user+grant, Redis status/persistence, language runtimes (php/node/java/go/python/dotnet) are MISSING by design divergence — not gaps to chase. TLS lattice and encrypted-at-rest are stronger in Forge. But 6 new P1 logic findings in centralization seam (list omits encrypted columns, mysql TLS global leak, deprovision swallowing, etc.).
- **Compose spec fidelity:** The deepest new work. `env_file` silently dropped (P0), volume policy bifurcation (API 200 then beacon 400), `["80"]` short-form bypasses privileged-port check, DeployFromGit Update-on-fresh-ID fails — these make spec-fidelity the most load-bearing remaining gateway-adjacent fix after Phase 4's five-writers.
- **Templates:** PufferPanel 44 JSONs with 24 typed ops vs Forge 14 on branch / 1 on main — 93% seeding deficit + typed→stringly `internal/groups` loss. Valid claims across both trees.
- **Uncloud mesh:** Central Postgres placement confirmed as correct choice for Forge fleet; borrow per-node service discovery subscription pattern.

### New activation priorities (amend §26/§37)

Bounce to the top of activation:
- Fix `resolveTemplate` fail-fast (appstore/service.go:239) — stale compose on interpolation error.
- Unify compose volume policy predicate (single `ValidateHostMountWithAllowlist`) — `200 valid`→`400 policy violation` trap.
- Wire correct uninstall (DB delete only after daemon success) + respect CrossVersion guard.
- Fix or delete verify stub (`handlers_proxy_domains.go:199`).
- Encrypt DNS creds path already filed; add global os.Setenv scoping fix and renewOnce fullchain fix.
- Wire `env_file` handling: either mount/resolve external env files pre-deploy or fail fast when present (document as unsupported).
- Seed game-templates (14 → DB seeding job like `SeedDefaultApps`).
- Host-tool freeze documentation (fail2ban/SSH/FTP/snapshot correctly out-of-scope; mark explicitly).

### Original thesis — unchanged, strengthened

Phases 1–5 concluded integration debt dominates. Phase 6 strengthens that verdict with a second host truth reference (1Panel) proving Forge correctly refuses host-tool sprawl, and a second compose fidelity reference (docker-compose upstream) proving the remaining debt is again wiring: same compose validated twice with different allowlists, same template interpolated with swallowed errors, same env_file resolved nowhere.
