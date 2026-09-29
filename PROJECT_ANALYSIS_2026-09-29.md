# Forge — whole-project analysis, 2026-09-29

Produced by five parallel analysis passes (control-plane API, Beacon agent, web +
packages, infra/CI, docs/tests/health) against `mvp-4` at `e35b580`. Every number
below was counted from the tree; the load-bearing claims were independently
re-verified before writing.

## What this project is

A control plane for deploying and operating game servers and app workloads across
distributed hosts. `forge/api` decides what should happen; `beacon` makes it happen
on a machine; `forge/web` is the operator UI. Roughly 420,000 lines of first-party
code: 259k in the Go API (925 files), 59k in Beacon (281 files), 100k in the Next.js
web app (504 files), 4k across four npm packages, plus 80k lines of markdown.

For a codebase of this size the engineering is better than the headline metrics
suggest, and the failure mode is specific and consistent: **the declarative
artifacts are trustworthy, the imperative ones are not, and nothing in CI closes
the gap.** Almost every serious problem found is a guard that exists as a file and
is wired to nothing.

## The five things that would hurt first

**1. The installer the documentation calls canonical cannot boot the product.**
`forge/install/install.sh:542` generates its own `docker-compose.yml` rather than
using `infra/`. The generated file has no `redis` service and no `beacon`/daemon
service — the entire host-agent half of the system — and the api service's
environment block contains only `TZ`, `PUID`, `PGID`, so there is no
`DATABASE_URL`, `API_AUTH_SECRET`, `APP_KEY` or `FORGE_MASTER_KEY` and the API's
own production guard rejects startup. It pins `postgres:15-alpine` against
`infra/`'s 16 (a non-downgradable data directory if anyone switches), uses `:latest`
tags, and publishes both postgres `5432` and the API `8080` on `0.0.0.0` where
`infra/compose.yml` binds everything to `127.0.0.1`. `scripts/install/install.sh`
is the correct installer and does the right thing (`--env-file infra/.env`,
`compose.yml -f compose.production.yml`, trap-based rollback, secrets never
printed). The two differ by 1,544 lines and the docs cannot agree which is real:
`AGENTS.md:16` and `docs/installation.md` say `forge/install`, `README.md:102` says
`scripts/install`, and `scripts/test/test-install.sh` only tests the latter. An
operator following the docs installs the broken one.

**2. Alerting is configured and completely dead.** Three independent failures
stack. `infra/compose.yml:408` passes `--config.expand-env=true` to
`prom/alertmanager:v0.27.0`, a flag that does not exist in that version — kingpin
errors on unknown flags, so the container crash-loops. The repo already knows: both
`alertmanager.yml.tmpl` and the legacy `infra/alertmanager.yml` carry header
comments saying so, which is why `scripts/gen-alertmanager.sh` exists. If it did
start, the tracked generated `infra/alertmanager/alertmanager.yml` routes both
`critical` and `warning` to a `default` receiver that has no notifier of any kind,
so every alert is acked and dropped. And 3 of 10 rules in
`infra/prometheus/alerts.yml` query metrics nothing exports
(`caddy_tls_certificate_expiration_seconds` — caddy's scrape job is commented out;
`node_filesystem_avail_bytes` — no node-exporter anywhere;
`backup_last_success_seconds` — no pushgateway). 6 of 20 Grafana panels query
`request_latency_seconds` and `server_status`, neither of which appears anywhere in
`forge/api`. No CI job ever starts the monitoring services, so none of this would
be caught before production.

**3. Unbuildable commits reach HEAD, and the enabling process is unchanged.**
`docs/audits/BACKEND_BUILD_BREAKAGE_2026-09-29.md` records that both Go modules
failed to compile at committed HEAD on a clean tree — 9 errors across 5 packages,
traced to two bulk working-tree snapshots that "were never compiled before being
committed." They are fixed now (`29d1600`, `b7df85e`; build exits 0). The
conditions are not: 14 of 75 commits are untitled tree dumps (`chore: commit
working tree on mvp-3` ×6), there is 1 merge commit in 75, and `ci.yml` is
`push: branches: [main]` on a repo whose branches are `mvp-3` and `mvp-4` — **there
is no `main`**. With no CODEOWNERS, no dependabot, no code scanning and PR-triggered
CI that barely fires, nothing requires CI to pass before code lands.
`CONTRIBUTING.md`'s "at least one review before merging" has no history behind it.
The root cause named repeatedly in the audits is multiple agent sessions sharing one
checkout with no locking; three findings are literally marked
`BLOCKED (concurrent edits)`.

**4. Layering is inverted in the HTTP package, and the DI container is a decoy.**
`AGENTS.md` sends every contributor to `internal/app/container.go` as the DI
container. Zero production files import that package; `InitServices` and
`BuildHTTP` are documented no-ops, and `ServicesBundle`'s own comment says "nothing
in production constructs this bundle." Real startup is `cmd/api/main.go` `run()`,
one ~1,586-line function with ~220 service constructions, 32 background goroutine
launches, inline queue executors for install/uninstall/backup/restore/transfer, and
five helpers duplicated verbatim from the dead container. Meanwhile
`internal/http` reaches `cfg.Store` 826 times across 65 of 118 handler files
against a `Store` type carrying **1,276 methods**, and four handler files bypass the
store entirely to run raw SQL (`handlers_deployment_rollback.go` at 1,381 lines has
a complete private DAO). `http.Config` has 123 exported fields;
`registerServerRoutes` is a single ~3,360-line function. Handlers *are* the business
layer here; the services package is 93 sub-packages of which many are one-file
shells.

**5. Three of four workspace packages are dead code that duplicate live code.**
`@forge/sdk` (1,792 lines), `@forge/ui` (418) and `@forge/game-templates` (253) have
zero importers anywhere in the repo — verified, not inferred — and `forge/web` does
not even declare them as dependencies. CI builds all three and runs tests for two of
them. The SDK is the worst case: it is a second HTTP client whose comments
*cross-reference* `lib/api/http.ts` five times cataloguing deliberate divergences,
including `unwrapData` meaning different things in the two files. Each dead package
shadows something the web app reimplements locally (`lib/api/http.ts`,
`components/ui/`, `lib/egg-templates.ts`).

## What is genuinely good, and worth protecting

The security engineering is the strongest dimension and it is *uniform* rather than
spot-applied. `infra/compose.yml` gives all 14 services `cap_drop: [ALL]`,
`no-new-privileges`, healthchecks, resource limits and reservations, capped logging
and loopback-only port binding, with six services on `read_only` rootfs; missing
secrets and unpinned tags are hard Compose parse failures
(`${API_AUTH_SECRET:?...}`, `${TAG:?...}`), not silent degradation. Docker access is
brokered through a socket proxy with `EXEC=0, BUILD=0`. On Beacon,
`internal/rootfs` uses `openat2` with `RESOLVE_BENEATH|RESOLVE_NO_MAGICLINKS|
RESOLVE_NO_SYMLINKS` — a confinement primitive that actually defeats symlink and
procfs escapes — and the container-exec endpoint requires an infra-admin JWT,
refuses managed containers, and enforces a read-only command allowlist with fixed
argv and no shell. Installer containers run as `nobody` with dropped caps and a
noexec tmpfs. `infra/postgres-backup.sh` is the only script in the repo that fails
closed on every branch: flock single-flight, `.partial` staging, and a real
`createdb`/`pg_restore`/`dropdb` scratch restore that deletes the dump and retries
if verification fails. On the frontend, `lib/api/http.ts` is exemplary — one HTTP
primitive with **zero bare `fetch` calls** across 340 client files, method-aware
retry with injected idempotency keys, boundary-anchored CSRF regexes that reject
`evil__Host-forge_csrf`, same-origin-only cookie defaults, and envelope guards that
*throw on unexpected shapes rather than rendering an empty list*.

That last detail points at the best thing about this codebase: a stated principle —
"unknown is not zero", "never report success for work not performed" — that is
enforced structurally in several places rather than by convention. The `unknown`
design token is deliberately not green, with a comment saying why. `migration.go`
returns an error telling you to add a dialect override instead of silently skipping
DDL. `native.sh` has a `STACK_DEGRADED` flag so partial startup reports as partial,
and a fail-closed Docker probe commented "Do NOT silently fall back to a mock
runtime." The last ~20 commits are the best-written in the repo and are all in this
register: `fix(deployment): stop reporting success for steps that did no work`,
`fix(backup): stop recording unchecked restores as verified`, `fix(host): require an
explicit nodeId instead of guessing the target`. Comment quality throughout is high
and consistently records *why*, including several comments that honestly flag their
own file's weakness.

Also strong: 1,866 Go test functions in the API and 596 in Beacon at a 0.41/0.58
test-file ratio; 554 `requireAdminScope` and 129 `requireServerPermission` call
sites, making authorization the most densely enforced part of the system; 628 nil
guards paired with 608 `503` returns so an unwired service degrades honestly;
`internal/placement` with an `explain.go` that produces human-readable placement
rationale; a 109-variable design-token system that removes options on purpose
(three radii, named type scale) and is documented in `DESIGN_TOKENS.md`; structural
invariant tests (`route-integrity`, `permission-gates`, `ui-contracts`) chosen over
shallow render tests, with `route-integrity.test.ts` documenting the 21-broken-href
bug it was written to prevent; and an audit culture with an explicit evidence ladder
(`SOURCE_VERIFIED > RUNTIME_VERIFIED > TEST_VERIFIED > DOC_DERIVED > INFERRED`) plus
four commits that exist purely to correct earlier audit claims.

## The next tier of problems

**Every quality gate that would catch the above is off.** `.golangci.yml` enables 12
linters including `gocyclo`, `funlen` and `gosec` with `max-issues-per-linter: 0` —
and is invoked by no workflow; `scripts/dev/lint.sh` degrades silently to `go vet`
when the binary is absent, so a developer without it gets a green lint with none of
the 12 linters run. Six Go files exceed 2,000 lines
(`beacon/internal/server/server.go` at 4,431), so the config would fail loudly the
moment it was wired. `.hadolint.yaml` and `.markdownlint.json` are likewise
referenced nowhere, shellcheck has no job, prettier is checked only by `lint.sh`,
and `ci.yml:15`'s `timeout-minutes: 30` is at workflow top level where it is not a
valid key and is ignored — every job inherits the 6-hour default. ESLint is the one
gate actually running, and it is 24 lines extending only Next's defaults, so every
frontend convention in `AGENTS.md` (no second HTTP client, no raw hex — 81 present,
no inline query keys) is honour-system.

**Two more unwired guards are worth naming individually.**
`scripts/release/check-version.sh` is a well-written pre-release gate that fails
roughly ten of its own assertions today: no Dockerfile declares `ARG VERSION` or an
OCI version label (all three hardcode `version="1.0.0"` against a project version of
`0.1.0`), and `infra/ship/kubernetes/` points at `ghcr.io/anomalyco/...:latest` —
wrong GHCR org *and* a floating tag. `publish-images.yml` dutifully passes
`VERSION`/`GIT_COMMIT`/`BUILD_TIME` build-args into builds where no Dockerfile
declares them, so BuildKit discards them and the published images are unversioned at
the metadata level. Separately, `infra/ci/validate-api-migrations.sh` boots the real
API and compares `/health`'s reported `migrationCount` against the file count — a
genuinely good end-to-end guard with **zero references anywhere in the repo**.

**`.github/workflows/deploy.yml` is named "Production Deploy" and deploys nothing**
— its entire body is three `test -f` checks, a `bash -n`, and
`echo "Deployment script and compose configs verified."`
`scripts/deploy/deploy-prod.sh` claims blue-green in its header, does an in-place
`up -d`, and prints "Deployment Complete" with exit 0 even when
`ALL_HEALTHY=false`. `scripts/cleanup/production-guard.sh:200` runs the API's own
`--production-guard` with `|| true`, discarding its verdict, then unconditionally
prints "All production guard checks passed." `scripts/deploy/deploy.sh`'s rollback
gates on a `.deploy.backup` file that nothing in the repository creates, and its
`TAG` defaults to `latest`, overriding the `TAG:?` pin that is compose's best safety
mechanism. These are the four clearest violations of the repo's own first rule.

**Deployment and dev surfaces have no single answer.** Seven deployment entry
points with no document stating precedence; 14 root-level `scripts/*.sh` each with a
divergent subdirectory twin (only two are honest `exec` wrappers); two
`production-guard.sh` files that *disagree on whether `sslmode=disable` is allowed*
for the bundled TLS-less postgres, referenced from five places at three paths; four
macOS dev runners of which `native.sh` and `dev.sh` register the same launchd label
`com.gamepanel.beacon` and clobber each other undetected. `.mcp.json` hardcodes
`/Users/riyaz/.local/bin/graphify-mcp`. Three Node baselines coexist
(`engines >= 20`, CI on 22, web Dockerfile on `node:20-alpine`), and `vite` is a
root *runtime* dependency.

**The frontend pays App Router's cost for SPA behaviour.** 340 of 504 files carry
`"use client"`; there are **zero** server-side fetches in `app/`, 2 metadata
definitions across 151 pages, 4 Suspense boundaries, and one `next/dynamic` call in
production — while `recharts` is statically imported in 11 files including the
default admin landing page. The query-key factory in `lib/api/query-keys.ts` covers
about 13% of keys (780 inline literals vs 118 factory uses, including 20 inline
`["servers"…]` in a namespace the factory owns), which makes stale-list-after-
mutation bugs structurally likely. Six components exceed 1,190 lines
(`AdminOverview.tsx` at 2,034) with roughly one test between them, and
`app/admin/git/page.tsx` at 1,454 lines breaks the otherwise-consistent thin-page
convention.

**i18n is infrastructure without adoption.** Eight locale catalogs, all with exactly
641 leaf keys, synced by a careful script that cross-validates against a TypeScript
union in another package — and only **9 files in the entire 100k-line app call
`t()`**, 220 call sites using 246 keys. The ~120 admin pages and all 56k lines of
`components/` are hardcoded English. Because the sync backfills missing keys with
English, the catalogs report 100% coverage while the product is ~2% localised. The
RTL path in `app/layout.tsx` targets five locales none of which are supported, and
`sync:locales:check` is not in CI.

**No Go→TS type generation.** 123 types in `shared-types` are hand-transcribed from
Go structs; `forge/api/docs/openapi.json` exists and nothing consumes it, and it
covers 175 paths against **1,029 registered routes** with zero swaggo annotations.
The three contract test files validate the TS client against itself, so a Go handler
renaming a JSON tag passes CI on both sides. The drift is already load-bearing debt:
`PaginationMeta` documents that `current_page`/`total_pages` are deprecated aliases
"still emitted by a few admin routes — readers must accept both, writers must send
both."

**Testing is lopsided in a way that maps to risk.** 43 of ~90 API service packages
have zero tests, including `billing`, `installer`, `upgrade`, `tenancy`,
`zerodowntime`, `autoscaler`, `fencing`, `notifications` (11 files) and
`backupengine` (1,685 lines in one file). `internal/scheduler` and `internal/app`
are untested; so is `beacon/internal/crypto`. 75 of 118 handler files have no
adjacent test. On the frontend it is 36 test files against 536 sources — **2 tests
for 191 components** — with coverage thresholds set at lines 25 / branches 15, and
`packages/ui` and `packages/shared-types` untested. Playwright is well configured
with 18 tests against a mock API and **is not run in any CI workflow**. There are 82
`t.Skip` sites, mostly environmental, and three that disable tests because the
behaviour they guarded was deleted.

**Debt is invisible to tooling.** Two TODO/FIXME markers repo-wide, against 148
occurrences of "stub", 54 of "placeholder" and 23 of "not implemented". The real
ledger is `audits/MASTER_FINDING_INDEX.md` — 41 fixed, 12 partial, 13 deferred, 2
open, 3 blocked on concurrent edits — which already under-reports its own closures,
since several findings appear to have been fixed by the honest-status commit run
without the rows being updated. Any bot or reviewer measuring debt by grepping
markers reads this repo as immaculate.

## Smaller specific findings

Beacon's `internal/crypto/hmac.go` is the *better* HMAC implementation —
length-prefixed canonicalisation, a fail-closed nonce cache, a 32-byte minimum
secret — and **has no importers at all**. The live auth path in `server.go:4411`
uses a hand-rolled newline-delimited variant with an inline nonce map that evicts
the oldest entry when full, which under sustained load can drop still-valid nonces
from the replay set, and it never enforces a minimum token length. Also on Beacon:
`handleHostTerminalWS` is a stub returning "not implemented on this platform" for
all non-Windows builds, while the route is registered so the UI advertises it on
Linux; `/v1/firewall/*` shells out to iptables, requiring `CAP_NET_ADMIN` that
compose's `cap_drop: [ALL]` removes; and the "read-only" Kubernetes proxy includes a
mutating `ScaleDeployment`. LXC and KVM are honest stubs that report
`Available() == false`; containerd and firecracker are real but build-tag gated;
only Docker (and Podman, which embeds it) is verified.

`internal/store/migrations/` is a second migrations directory holding 26 dead SQL
files whose README says "RETIRED — no code path executes these," then claims two
files were removed that are both still present, and where numbers 023–043 mean
*entirely different migrations* than the canonical directory. The driver triad is
decorative at the query level: 2,113 Postgres `$1` placeholders and zero `?`
placeholders means MySQL and SQLite cannot execute the query layer. Migration
dialect overrides cover 10 and 25 of 243 files, and only 9 have rollbacks (3.7%).
`registerPhaseHooks` logs registrar failures and continues, so a phase whose routes
fail to mount yields a running server silently missing an API surface — the one
place the codebase's own honesty rule is violated in the routing core; its
priorities are bare magic integers with one outlier of 2000 that reads as an
accident, and phases 4, 6 and 7 do not exist.

`FORGE_MASTER_EXECUTION.md` — 4,053 lines of agent prompt — sits at repo root
unreferenced by anything, and is a new reader's most likely first file. `docs/` is
5,762 lines of which 60% is itself audit material, with 27 of 31 files unreachable
from any index and `docs/README.md` still branded "GamePanel" and indexing 5
destinations. `CONTRIBUTING.md`'s only link is broken. `VERSION` is `0.1.0` with
**zero git tags**, so every compare/tag link in `CHANGELOG.md` is dead, and a single
`[Unreleased]` block — mis-dated by five weeks — carries 243 migrations and ~55
fixes. Nothing has shipped that anyone can name. There is also 241 MB of untracked
build output in the worktree (`forge/api/api` 165 MB, `beacon/daemon` 56 MB,
`store.test` 20 MB) plus two large vitest log files, and `gofmt -l -s` reports 126
files needing formatting, 26 of them missing a trailing newline.

## Where I would start

The cheapest changes with the largest effect are all wiring, not writing:

Point the docs at one installer and delete or fix the other. Drop
`--config.expand-env=true`, untrack the generated alertmanager config, and either
give `default` a notifier or delete the rules and panels that cannot fire — dead
monitoring is worse than none, because it looks like coverage. Change `ci.yml`'s
push trigger off the nonexistent `main`, move `timeout-minutes` into the jobs, and
add branch protection so CI actually gates. Wire the three guards that already
exist: `scripts/release/check-version.sh`, `infra/ci/validate-api-migrations.sh`,
and a `golangci-lint` job (use `--new-from-rev` to cap the one-time bill). Fix the
four scripts that report success they did not earn — `deploy.yml`,
`deploy-prod.sh`'s `ALL_HEALTHY=false` exit 0, `production-guard.sh`'s `|| true`,
and `deploy.sh`'s unreachable rollback — since those are the repo's own stated
cardinal sin. Delete the three unused packages and Beacon's unused-but-better crypto
package, or wire them in; today they are traps that read as API. Then tag `v0.1.0`
so the changelog's links resolve, and adopt the query-key factory before the stale-
cache bugs arrive.

The deeper work — decomposing `run()` and `registerServerRoutes`, pulling business
logic out of handlers, giving the 43 untested service packages tests, deciding
whether the frontend wants App Router or should admit it is an SPA — is real but
none of it is urgent in the way the above is. The repository's own audit trail is
good enough to plan against; its main defect is that the ledger is maintained by
hand while the gates that would maintain it automatically are all switched off.
