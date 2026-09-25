# Phase 1 Remediation — Final Report

**Date:** 2026-08-23 · **Branch:** mvp-2 · **Owner:** remediation pass (Phase 1 — Application Platform Ecosystem)

## 1. Scope

All actionable findings from `audits/phase-01/` (5 subagent reports + synthesis, 35 indexed findings) were
classified against the CURRENT tree before any change. A concurrent writer was active on this worktree
throughout; fixes were layered without destroying in-flight work (fresh-read-before-edit; their untracked
files untouched).

## 2. Root causes fixed (one fix per cluster)

| Cluster | Findings collapsed into it | Fix |
|---|---|---|
| Deployment false completion | FORGE-LOGIC-001, REF-APP-C02/FLF01, F-07 empty-image | RuntimeExecutor bridge (beacon sync-config + power-start), fail-closed steps, verify-steps observe real state, image admission resolution |
| Health gate wrong target | FORGE-LOGIC-002, REF-APP-C08/FLF02 | Gate derives node host from ServerControlTarget; unresolved ⇒ honest failure |
| Restart conflation | C03a app-restart-as-deploy, C03b compose-restart-unwired, F-06 | Real beacon power-cycle for apps; existing RestartStack chain wired to admin route |
| Redeploy row leak | C04/F-08 | `/compose/:id/deploy` now updates in place with rollback+health |
| Scale no-op | C07/F-13 | Placement validated before mutation; honest refusal |
| Beacon Docker-admin gaps | RT-FL01, RT-FL02 | 5 new handlers (container/network/volume create, network/volume delete, volume prune) + routes |
| Queue retry accounting | F-18 | Steal no longer consumes retries; SQL extracted to testable constants |
| Reaper vs long ops | F-19/ARCH01 | Worker heartbeat (Touch every 30s); reaper safe for hour-long installs |
| Second execution engine | DUP-001 | Dead DispatchCompose removed; queue documented single-writer |
| Reconciler truth | F-24, F-25(verified-by-design+pinned), F-26 | Attempt decay after stability; dedupe semantics pinned by test; events only on verified transitions |
| Build contract lies | GIT04, GIT05 | Admission admits only executable build types; beacon now honors cacheFrom/cacheTo/platform (with injection-safe validation) |
| Webhook HMAC mask | GIT10/LF08 | Bad signature ⇒ 401 across GitHub/GitLab/Bitbucket/Gitea; recon-preserving 200 kept for unknown repos |
| Compose side door | F-10 | PUT /apps/:id/compose validates compose security before persisting |
| UX truth batch | UX02, UX04a/b, UX05a | Create-form sends only honored fields (+image that deployments actually run); back-href fixed; tab state URL-persisted; dead CTA wired |

Plus: go.work toolchain drift repaired (builds green again).

## 3. Files changed (summary)

Backend (forge/api): services/deployment/{execution,service,healthgate}.go, beacon_executor.go (new),
provision_regression_test.go, healthgate_target_test.go; services/apphosting/service.go;
store/store_apphosting.go; internal/http/{handlers_apphosting,handlers_compose,handlers_source_deployments,handlers_git}.go;
services/queue/store.go + retry_accounting_test.go; services/operation/{service,store}.go + test;
services/reconciler/service.go + plan_dedupe_test.go.
Beacon: server/build.go, server/container_admin.go, server/server.go (routes).
Web: components/app/app-create-form.tsx, components/app/app-list.tsx, components/app/app-detail.tsx,
app/admin/apps/[id]/page.tsx, lib/api/apps.ts.

## 4. Status ledger

See `finding-status.md` — 24 findings VERIFIED_FIXED / ALREADY_FIXED / INVALIDATED with evidence,
13 DEFERRED_WITH_REASON or BLOCKED (each with the specific blocker).

## 5. Verification

See `verification.md`. All modules build + vet clean; all touched-area suites pass; forge/web 209 tests +
tsc clean. Remaining red tests exist only inside the concurrent writer's untracked ws_hub files.

## 6. Remaining gaps (honest)

1. Live E2E flows not executed (no integrated environment run in this pass).
2. Preview dedup, env-var unification, git credential hardening, session-store race, reconnect-truth:
   deferred with explicit coordination reasons (concurrent writer owns those files today).
3. Exec surface intentionally not exposed (documented absence, not a hidden capability).
4. Node-health placement gate covers node status + executor failure paths; heartbeat-freshness gating
   lands with ARCH03 fix.

## 7. Readiness assessment

Phase-1 P0s are closed with regression coverage: deployments can no longer report success without real
beacon execution; health gates probe the right host; restart/redeploy/scale tell the truth at API and UI
layers. Phase-1 is remediated for correctness-of-contract; full "production-ready" remains gated on the
deferred coordination items and live E2E runs listed above.
