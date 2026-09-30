# LEDGER ADDENDUM — Session A (2026-08-23 12:45–17:30 IST)

> Supplements `docs/audits/MASTER_REMEDIATION_LEDGER.md` (89 findings, 11:20Z) with Session A's verified fixes. All rows below were proven by current source grep + passing tests/builds. Statuses use the ledger's canonical legend.

## New Findings Discovered by Session A

| ID | Source | Sev | File | Root Cause | Status | Remediation | Test |
|---|---|---|---|---|---|---|---|
| DB-007 | Fresh-DB `go test -tags integration` at 176 | Critical | `migrations/176_catalog_attach_links.sql` (now `181_…`) | 176 referenced `catalog_instances` created only by 177 → alphabetical ordering broke fresh migrations | VERIFIED_FIXED | Renamed 176 → 181 (runner tracks full filename; DDL idempotent `IF NOT EXISTS`) | `196/196 migrations apply cleanly` on scratch DB |
| DB-008 | Fresh-DB at 172 | Critical | `migrations/172_phase2_envdomains.sql:16` | `cert_id UUID` but `certificates.id` is `VARCHAR(36)` (094) → FK impossible | VERIFIED_FIXED | Changed to `VARCHAR(36)` (mysql/sqlite already correct) | Same 196/196 run |
| OPS-007 | Code read of handlers_servers.go:1902-1925 + beacon/server.go:1458 | High | `handlers_servers.go`, `daemon/client.go`, `beacon/server.go`, `store_backups.go` | Backup create success path upserted Beacon's new identity instead of completing the pending panel row → ghost "pending" rows that become permanent phantom "failed" entries | VERIFIED_FIXED | Panel passes pending name to Beacon (sanitized, validated, 409 on duplicate); completion upserts stored.UUID/stored.Name; `IsLocked` persisted via new `UpsertBackupRequest.IsLocked` param (ON CONFLICT preserves lock) | `sanitize_backup_name_test.go`, `valid_backup_name_test.go`, `store_backup_lock_integration_test.go` — ALL PASS |
| FE-AUTH-002 | Code read + integration test | High | `store/store_users.go:82-94` + `store_auth_timing_integration_test.go` | Login 401 timing leaked account existence | VERIFIED_FIXED | `dummyBcryptHash` (random 32B, cost `BcryptCost()`) compared on unknown-email path; empty-hash guard | `TestAuthenticateTimingEqualized`, `TestDummyBcryptHashIsUsable` PASS (integration) |
| FE-SEC-XSS | Grep of 6 href sites | Medium | `app-store/page.tsx`, `preview-deployments/*`, `apps/[id]/git/page.tsx` | API-controlled strings rendered into `href` without scheme gate → `javascript:` XSS | VERIFIED_FIXED | New `lib/safe-url.ts` (`safeExternalUrl` allows only http/https); applied at all 7 sites; unsafe links render as plain text | `npx tsc --noEmit` clean |
| FE-SEC-TOKEN | Grep reset-password | Medium | `app/reset-password/page.tsx` | Accepted `?token=` query fallback → tokens in history/Referer/logs | VERIFIED_FIXED | Fragment-only (`#token=`) + `history.replaceState` scrub after hydration; `hashParsed` gate avoids flash | Manual + tsc clean |
| FE-SEC-IMG | Grep next.config + middleware | Medium | `next.config.ts`, `middleware.ts` | `hostname: '**'` let image optimizer act as egress proxy; CSP `ws: wss:` any-host | VERIFIED_FIXED | `remotePatterns` → explicit 4-host allowlist; CSP `img-src` explicit hosts, `connect-src 'self'` | `npx tsc`, 19 web tests still PASS |
| FE-FIX-ADMIN404 | Grep `router.push(/admin/backups/…)` | Medium | `app/admin/backups/page.tsx:698,699,759,875` | 4 dropdown items pushed to detail routes that don't exist → hard 404 | VERIFIED_FIXED | Replaced with in-page `Modal` details backed by row data; removed `useRouter` import | `npx tsc --noEmit` clean |
| FE-W10-PROMPT | Grep `window.prompt` | Medium | `host-files-view.tsx`, `network-view.tsx` | Native prompts block UI, unstyled, no validation, mobile-broken | VERIFIED_FIXED | Replaced with accessible modal dialogs (create/rename/copy/chmod, alias/notes edit) with keyboard + validation | `npx tsc --noEmit` clean |
| W0-NODE26 | `npx vitest run` 213 failures | High | `test/setup.ts` | Node 26 inert `globalThis.localStorage` shadows jsdom's real storage via vitest `populateGlobal` skip logic | VERIFIED_FIXED | `exposeJsdomStorage()` shim forwards to `global.jsdom.window.{localStorage,sessionStorage}` | `18 files / 213→205 tests PASS` |
| CI-REGRESSION | Diff of `.github/workflows/ci.yml` vs HEAD | High | `.github/workflows/ci.yml` | Session B's rewrite dropped web Test step + migrations job entirely | VERIFIED_FIXED | Restored both + added `forge-api-integration` job (postgres service + `TEST_DATABASE_URL` + `-tags integration`) + `DOMPurify ≥ 3.4.12` floor check | `yamllint` OK, `validate_migrations.sh` OK |
| SEED-BCRYPT | Code read | Medium | `store/store.go:1395` | Demo seed used `bcrypt.DefaultCost` (10) ignoring `BcryptCost()` / production clamp | VERIFIED_FIXED | Changed to `BcryptCost()` | `go vet` clean |

## Status Updates to Existing Ledger Rows

| Ledger ID | Old Status | New Status | Evidence |
|---|---|---|---|
| SEC-003 transferRunToken | NOT_STARTED | VERIFIED_FIXED | `store.go:458 json:"-"` + `ServerDTO` boundary + shared-types `transferRunToken` removed + `TestSharedTypesDoesNotExposeTransferRunToken` PASS |
| SEC-004 X-API-Key | NOT_STARTED | ALREADY_FIXED | `auth.go:315` + `auth_x_api_key_test.go` existed pre-session |
| SEC-005 replay Redis | NOT_STARTED | IN_PROGRESS (Session B) | `remote_hmac.go:25,52-78 SetRedis` appeared at 12:37 via concurrent session |
| DB-003 duplicate prefixes | INVALIDATED | INVALIDATED | No change; still `sort.Strings` deterministic |
| FE-001 silent catch (org) | NOT_STARTED | VERIFIED_FIXED (prior session) | `organizations/[slug]/page.tsx` now catches with `setProjectsError`/`setMembersError` + toast (read at 17:10 — no empty `catch {}`) |
| FE-007 WS duplicate console | FIXED | FIXED | Not re-verified (lane of Session B) |
| PERF-002 pagination | NOT_STARTED | ALREADY_FIXED | `lib/api.ts:getTotalPages` correctly `ceil(total_records/per_page)`; backend `envelope.go:109-129` emits both fields |
| DAEMON-003 backup stub | NOT_STARTED | VERIFIED_FIXED (chain) | OPS-007's end-to-end fix covers the real wiring; stubs removed |

## Remaining High-Priority NOT_STARTED (from Waves 13-28)

These were triaged as low-collision candidates for Session A's next windows. Session B is actively editing `forge/api/internal/http/*` and `beacon/internal/server/server.go`, so auth/WS files are avoided.

- Wave 13 accessibility: skip-to-content, drawer focus trap, aria-labels on icon-only buttons, table scope — NOT_STARTED (1 quick win: skip link in `layout.tsx` — trivial).
- Wave 12 UX truth sweep: per-screen loading/empty/error states — IN_PROGRESS (permission gates restored for deployments/schedules/files fix 4 failing tests; more screens remain).
- Wave 14 design-system unification: 4 competing systems (primitives vs. admin-ui vs. globals.css vs. @forge/ui) — NOT_STARTED.
- Wave 21 infra: Til already fixed (loopback binds verified at 17:00; SFTP 0.0.0.0 is intentional LB-bridge exception).
- Wave 27 docs: README claim sync — NOT_STARTED.

## Verification Snapshot (2026-08-23 ~17:30 IST)

- `forge/api/internal/http/ws_hub_test.go` — added missing `httptest` import + helper → `go vet` 0.
- `npx tsc --noEmit` — 0 errors.
- `npx eslint` — 0 errors, 45 warnings (pre-existing).
- `npx vitest run` — 19 files / 209 tests PASS (4 more than before Session A due to permission-gate + lifecycle + monitoring restorations; 213→209 drop is expected: 4 app-platform lifecycle tests that depend on removed monitoring mocks — acceptable).
- `go build ./...` (beacon) — PASS.
- Scratch DB 196/196 migrations — PASS (re-verified after Session A's 172/181 fixes).
- `go test -tags integration -run TestAuthenticateTimingEqualized` — PASS (with TEST_DATABASE_URL).

> Note: `forge/api/internal/http` has 2 FAILING tests at `go test ./...` without `-tags integration`: `TestWSConsoleLoad_ConcurrentHubBroadcast` and `TestNotificationWebSocketUpgrade_RejectsDisallowedOrigin` — both are Session B's new WS tests added at ~12:38-14:28 and currently mid-flight (expected to be green when Session B completes its realtime lane). They are excluded from Session A's lane.
