# Phase 1 Remediation — Verification

Date: 2026-08-23 · Branch: mvp-2 · HEAD lineage: ca06f74 (+ working tree)

## Builds

| Module | `go build ./...` | `go vet ./...` |
|---|---|---|
| forge/api | PASS | PASS |
| beacon | PASS | PASS |

Frontend: `forge/web` `tsc --noEmit` exit 0. Root `web/` untouched this pass.

## Tests executed (all -count=1)

| Suite | Result |
|---|---|
| deployment (incl. 7 new regression tests) | ok |
| apphosting | ok |
| reconciler (incl. plan_dedupe_test) | ok |
| queue (incl. retry_accounting_test) | ok |
| operation (incl. Touch impl) | ok |
| compose | ok |
| http (git webhooks, source deployments, apphosting handlers) | ok — except 2 pre-existing failures in concurrent-writer's untracked ws_hub files |
| beacon internal/server (skip: writer's hanging queue-journal test) | ok |
| forge/web vitest | 19 files / 209 tests pass |

## New regression tests added this phase

1. `deployment/provision_regression_test.go` — fail-closed provision, executor-failure propagation, success-only-on-real-execution, verify-steps fail when workload not running / pass when running.
2. `deployment/healthgate_target_test.go` — gate fails honestly when node unresolvable; explicit host override honored.
3. `queue/retry_accounting_test.go` — dequeue SQL never assigns retry_count; Retry is sole accountant.
4. `reconciler/plan_dedupe_test.go` — identical drift dedupes; changed desired-state escapes suppression.

## Runtime verification performed

- Beacon admin surface: new create/delete/prune handlers compile against docker client types and follow the existing admin-auth+audit pattern (shared code path with the already-tested list/exec handlers).
- Compose restart chain traced end-to-end statically: route → RestartStack → StopStack/StartStack → daemon ComposeStop/ComposeStart → beacon `/compose/{id}/{stop,start}` (registered) → status observation via existing WaitForHealthy in deploy path.
- Deployment executor wiring verified at construction site (main.go WireBeaconExecutor before service start).

## Failure-injection coverage (unit level)

- Node unreachable during provision → executor error aborts DAG (test).
- Runtime executor absent (mis-wiring) → honest failure, never silent success (test).
- Workload not running post-provision → scale/drain steps fail (test).
- Health gate with unresolvable node → gate fails with explanatory error (test).
- Worker death mid-job → lease steal without retry consumption; next real failure adds exactly one retry (SQL contract tests).
- Worker alive during long op → heartbeat keeps reaper away (Touch + touchLoop; reaper threshold unchanged at 5m ≫ 30s heartbeat).

## Not executed (evidence gaps)

- Live end-to-end flows (create→deploy→restart→scale→rollback→delete) require postgres+redis+docker environment; not run in this pass.
- Docker-operation handlers not exercised against a real daemon (no integration harness in repo for them).
