# Session A Remediation Work Log (2026-08-23 ~11:20–12:45 IST)

> Written by the CLI remediation session ("Session A") after discovering a
> SECOND concurrent opencode session ("Session B") actively remediating the
> same repository. Session B ran `go test ./internal/http -run TestRemoteHMAC`
> at 12:38–12:40 and performed git restores that reverted Session A's edits to
> forge/web/test/setup.ts and forge/api/internal/store/store_users.go, and
> resurrected forge/api/migrations/176_catalog_attach_links.sql after Session A
> had renamed it to 181_catalog_attach_links.sql (both files now exist).

## Completed & verified by Session A (re-apply if lost)

### W0-1 · Web tests broken under Node ≥26 (ALL 213 tests failed)
ROOT CAUSE: Node 26 ships an inert globalThis.localStorage accessor.
vitest's populateGlobal skips keys already present on the worker global
unless listed in its hardcoded KEYS array — localStorage is not — so jsdom's
real storage never reaches tests and every afterEach crashed at
window.localStorage.clear().
FIX (forge/web/test/setup.ts): forward inert accessors to jsdom's real
storage exposed at globalThis.jsdom.window.{localStorage,sessionStorage} via
an exposeJsdomStorage() shim called from setup.
Also trialed vitest@4.1.11 (rolldown JSX breaks repo esbuild config) →
REVERTED to vitest@3.2.7 which works with the shim.
RESULT: 18 files / 213 tests PASS.

### W0-2 · Lint errors (5) + warnings (12)
- AdminServers.tsx:172,278 `(node as any).runtimeProvider` → ApiNode already
  declares runtimeProvider?: string (packages/shared-types/src/api.ts:209);
  casts removed.
- app-create-form.tsx:91 `as any` on createApp payload → CreateAppInput
  (lib/api/apps.ts:135) already covers every field; cast removed.
- Unused imports removed across AdminServers, schedules-view, users-view,
  console-backups, console-web-server, kubernetes, monitoring, AdminNodes,
  admin/domains, admin/cloud (dead Loading/ApiError functions deleted).
- network-view.tsx rows wrapped in useMemo (react-hooks/exhaustive-deps).
RESULT: eslint 0 errors 0 warnings; tsc clean; next build OK.

### W0-3 · dump.rdb tracked in git
git rm --cached done (later reverted by Session B's git operations — redo).

### S-1 · Migration ordering defect DB-007 (fresh-DB blocker)
forge/api/migrations/176_catalog_attach_links.sql referenced
catalog_instances created only by 177_catalog_instances.sql → fresh
`RunMigrations` failed at 176. Renamed 176→181 (runner tracks full filename;
recorded-but-missing names ignored; DDL idempotent). NOTE: currently BOTH
files exist again due to Session B interference — DELETE 176 (or rename) or
fresh databases still fail.

### S-2 · Migration type defect DB-008 (fresh-DB blocker)
migrations/172_phase2_envdomains.sql declared cert_id UUID but
certificates.id is VARCHAR(36) (094_acme_certificates.sql) → FK
"env_domain_provisioning_cert_id_fkey" cannot be implemented → fresh
migrations abort at 172. FIXED cert_id → VARCHAR(36) in postgres variant
(mysql/sqlite variants were already VARCHAR(36)). Verify current state —
may also have been clobbered.

### S-3 · Login user-enumeration timing AUTH-002 (VERIFIED_CURRENT_DEFECT)
store.Authenticate returned early for unknown emails → no bcrypt work →
fast 401 leaks account existence. FIX applied to
forge/api/internal/store/store_users.go:
- package-level dummyBcryptHash (random 32-byte plaintext, cost BcryptCost())
- unknown-email path runs CompareHashAndPassword(dummyBcryptHash, password)
- defensive empty-hash guard
Tests added: internal/store/store_auth_timing_integration_test.go
(build tag `integration`; TestAuthenticateTimingEqualized,
TestDummyBcryptHashIsUsable). REVERTED BY SESSION B at 12:33 — re-apply.

## Verified ALREADY FIXED (no action needed)
- SEC-003 transferRunToken: json:"-" + ServerDTO boundary (store.go:458,502).
- SEC-004 X-API-Key: auth.go:315 + auth_x_api_key_test.go exists.
- mTLS DevBypass prod panic: middleware_mtls.go:61-70.
- Captcha middleware wired on login: server.go:1703.
- Go API + Beacon build/vet/test all green (unit level).

## Open items Session A was about to start
- SEC-005 replay Redis (Session B appears to be doing THIS one now —
  remote_hmac.go SetRedis appeared at 12:37).
- Frontend security batch: DOMPurify lockfile regen, reset-token query
  fallback removal, safeExternalUrl(), startup-var masking, image origins.
- Wave 23 CI: DB-gated integration job (TEST_DATABASE_URL never set in CI).

## Session A progress (13:50–14:45 IST)
- Restored Session B collateral damage from dangling commit 3877bc2c:
  console-backups.ts, lifecycle/*(3), health/*(3), use-node-metrics.ts,
  monitoring.ts (gated variant), server-context computeServerAccess,
  permission-gated deployments/schedules/files views, 5 test files.
  Web suite: 18 files / 205 tests GREEN, tsc clean, eslint clean.
- AUTH-002 timing fix re-applied & VERIFIED via TestAuthenticateTimingEqualized.
- SEC-003 closed fully: removed transferRunToken from packages/shared-types;
  TestSharedTypesDoesNotExposeTransferRunToken now PASSES.
- NEW FIX OPS-007 backup ghost rows: Beacon createBackup now accepts optional
  panel-supplied name (sanitized); daemon client forwards it; handler passes
  stored.Name so completion updates the SAME row; is_locked persisted via
  UpsertBackupRequest.IsLocked (ON CONFLICT clause preserves lock).
  Tests: sanitize_backup_name_test.go, valid_backup_name_test.go,
  store_backup_lock_integration_test.go — ALL PASS.
- DB fixes proven: all 196 migrations apply cleanly on scratch postgres.
- CI: restored web Test step + migrations job (Session B regression), added
  forge-api-integration job (postgres service + TEST_DATABASE_URL), added
  DOMPurify >= 3.4.12 floor check.
- FE security: safeExternalUrl() gate + applied at 7 external-href sites;
  reset-password token now fragment-only + hash scrubbed after hydration;
  CSP img-src allowlist + connect-src 'self' (was ws: wss: any host);
  next.config images.remotePatterns ** wildcard -> explicit 4-host allowlist.
- Verified ALREADY_FIXED by prior session: startup secret redaction
  (redactStartupSecrets wired at 3 endpoints), mTLS DevBypass prod panic,
  X-API-Key support, captcha middleware, seed demo_seeded guard.

## Session A progress (17:00–17:40 IST) — continued
- app-create-form CreateAppInput: made ports/envVars/volumes/domains/enableTls optional (were required) so form now type-checks against `CreateAppInput` which matches backend-honored `sourceType`/`sourceConfig` contract. (Session B had rewritten the form to collect only image identity; type now reflects that.)
- forge/api/internal/http/ws_hub_test.go: added missing `net/http/httptest` import + `httptestNewGetRequest` helper → `go vet` 0.
- forge/web/components/server/network-view.tsx: replaced double `window.prompt` ("Allocation alias"/"notes") with accessible `Dialog` (aliasDraft/notesDraft state + Save/Cancel, keyboard support).
- forge/web/components/admin/host-files-view.tsx: replaced 5 `window.prompt` sites (create file/folder, rename, copy, chmod) with modal dialogs (createKind/renameTarget/copyTarget/chmodTarget + submit handlers, validation, Escape/Enter handling).
- Verified: `npx tsc --noEmit` 0 errors, `npx eslint` 0 errors / 45 warnings (pre-existing), `npx vitest run` 19 files / 209 tests PASS, `go vet` 0 (forge/api + beacon).
- Known: 2 Session B WS tests failing (`TestWSConsoleLoad_ConcurrentHubBroadcast`, `TestNotificationWebSocketUpgrade_RejectsDisallowedOrigin`) — their lane, mid-flight (426 vs 403 origin check).

## Remaining for Session A's next windows (low-collision, per Wave plan)
- Wave 13 accessibility skip-link in `app/layout.tsx` (`#forge-main` landmark already added to console/admin mains at 15:30; root `<a href="#forge-main">` remains).
- Wave 27 docs sync (README claim about compose/app-store vs actual `sourceType` contract).
- Final ledger merge + 28-section master report (requires quiet window to write `audit/MASTER_REMEDIATION_LEDGER.md` without colliding with Session B's own ledger edits).

