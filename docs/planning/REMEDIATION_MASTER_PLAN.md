# Forge / Beacon Full Remediation Ledger

Last updated: 2026-07-15

This document is the authoritative checklist for correcting the evidence-based audit against Pelican, Pterodactyl, PufferPanel, and Wings. An item is complete only when its acceptance tests pass; compilation alone is insufficient.

## Status legend

- `[ ]` not started
- `[-]` in progress
- `[x]` implemented and validated
- `[!]` blocked by external infrastructure/credentials

## Phase 0 — Stop unsafe installation and false success

- [x] Repair invalid migration 020 (`pCREATE`).
- [x] Add webhook schema to the canonical migration directory.
- [x] Remove unconditional demo seeding from API startup.
- [-] Add clean PostgreSQL migration validation in CI (configured; pending first CI execution).
- [x] Prevent migration/evacuation/recovery services from reporting execution/completion without workload movement.
- [x] Prevent legacy transfer endpoints from reporting success without transfer execution.
- [x] Prevent plugin lifecycle endpoints from reporting installed/enabled without a runtime.
- [x] Prevent mail-test/password-reset flows from reporting delivery when no mailer exists.
- [x] Remove Beacon per-operation mock-success fallbacks.
- [x] Disable the non-durable Forge operation pipeline that swallowed step failures and reported immediate success.
- [x] Make Beacon remote client reject non-2xx responses.
- [x] Correct Beacon heartbeat route/contract.
- [x] Fix CI Go module paths and dependency caching.
- [x] Remove insecure static-password SFTP deployment.
- [x] Stop exposing PostgreSQL and Redis publicly by default.

### Phase 0 acceptance

- [ ] A clean PostgreSQL database applies every migration.
- [x] Production startup creates no demo user/node/server/allocation (covered by seed-gate tests; clean DB CI pending).
- [x] Unsupported operational endpoints return explicit non-success statuses.
- [x] CI runs against `forge/api` and `beacon` modules.
- [x] HTTP 401/404/500 from panel callbacks are observable failures in Beacon.

## Phase 1 — Canonical node runtime and storage

- [x] Replace per-server Docker named volumes with the canonical host server root bind mount.
- [x] Ensure installer and runtime use the same canonical root; remaining file subsystems already target that root but still need secure descriptor-relative access.
- [x] Reconstruct all server manager objects on Beacon startup.
- [x] Inspect and reconcile existing labeled containers on startup (fake-runtime covered; real Docker test pending).
- [ ] Persist/recover desired state, actual state, installation state, suspension, and crash state.
- [x] Remove Docker auto-restart conflict with Beacon crash policy.
- [x] Implement configured graceful stop command/signal/timeout semantics.
- [x] Reconnect Docker event watcher with cancellation and bounded backoff; inspect OOM state after die events.
- [x] Implement per-node outbound daemon credentials and immediate rotation behavior.
- [x] Require Beacon API authentication by default in every environment; insecure no-auth mode requires an explicit development-only override and is rejected in production.
- [ ] Remove plaintext reusable node credentials from ordinary API responses.
- [-] Apply persisted Beacon configuration or remove misleading update support (runtime request persistence/config hashes added; global `/api/update` hot reload still limited).

### Phase 1 acceptance

- [ ] A file written via panel is visible inside the game container.
- [ ] A file written inside the container is visible via panel and SFTP.
- [ ] Beacon restart preserves control and correct status of existing containers.
- [ ] Two nodes with different credentials can both be controlled independently.

## Phase 2 — Secure filesystem, SFTP, archives, backups

- [x] Implement descriptor-relative filesystem abstraction (`openat2`/`openat`, no-follow).
- [x] Route mounted HTTP file operations through the safe filesystem abstraction.
- [x] Route SFTP operations through the safe filesystem abstraction.
- [x] Validate server identifiers as strict UUIDs/identifiers.
- [x] Make uploads atomic, locked, size-bounded, and safely finalized; expected-total checksum protocol remains optional.
- [-] Implement quota accounting for HTTP, SFTP, pulls, archives, and concurrent writes (all daemon-facing writes covered; kernel/container-write hard quota still pending).
- [x] Harden remote pull against SSRF, redirects, DNS rebinding, truncation, and partial files.
- [x] Make archive creation close files promptly and extraction staged/rollback-protected.
- [x] Move backups outside server roots.
- [x] Validate complete backup archives before modifying live data.
- [x] Fix S3 retry state, multipart upload, pagination, checksum verification, staging cleanup, and fail-closed adapter identity.
- [ ] Implement accurate `.pteroignore` semantics.
- [x] Implement SFTP activity delivery, concurrent quota reservation, WS/SFTP session revocation, idle/session limits, suspension recheck, public-key auth, read-only mode, and safe attribute behavior.

## Phase 3 — Real server provisioning and egg model

- [x] Consolidate `eggs` and `server_templates` into canonical eggs with compatibility aliases/migration.
- [x] Consolidate operational mount assignment on `mount_server` with legacy backfill.
- [-] Implement egg variables, validation rules, scripts, import/export, inheritance, and config parsing (variables/scripts/schema done; import/export/full parser pending).
- [x] Persist supported build/startup/resource fields and variables.
- [x] Replace broad destructive server PATCH with patch-safe semantics.
- [x] Implement durable truthful create → config sync → daemon create → explicit install lifecycle.
- [x] Implement compensation/rollback and orphan remediation for failed provisioning.
- [x] Implement real reinstall daemon call.
- [x] Implement real server deletion, allocation release, relationship cleanup, and orphan remediation.
- [ ] Enforce allowed mount roots and safe mount targets.
- [x] Implement validated Docker CPU quota/pinning, swap, IO weight, OOM/PID limits, stop settings, UID/GID, DNS, managed network, allocation protocol validation, config-drift handling, and scoped registry auth (real Docker integration pending).

## Phase 4 — Console, realtime, and sessions

- [x] Implement one lifecycle-managed Docker attach/output producer per running server.
- [x] Implement server-scoped console sinks and bounded replay.
- [x] Wire bounded/drop behavior and disconnect cleanup.
- [x] Correct frontend WebSocket ticket endpoint.
- [x] Use one canonical token/session accessor.
- [x] Remove JWT query-string WebSocket authentication.
- [-] Make tickets replica-safe and consume only after full validation (identity binding and post-validation consumption implemented; shared replica-safe ticket storage remains).
- [ ] Implement reconnect and error states for console/stats/logs.
- [x] Implement functioning shared user/session deauthorization for WS and SFTP.

## Phase 5 — Authentication, authorization, and secrets

- [ ] Replace browser-localStorage long-lived sessions with Secure HttpOnly session/BFF design.
- [x] Add logout JTI revocation, session-version invalidation, and password/2FA security-event invalidation; full session listing remains.
- [x] Resolve current user and effective roles rather than trusting stale JWT role/email.
- [x] Make current user-role assignments authoritative for role checks; granular role-rules evaluation remains.
- [-] Separate client and application API keys (admin scope issuance restrictions done; explicit key classes pending).
- [x] Restrict admin-scope issuance and enforce IP/CIDR restrictions.
- [x] Enforce OAuth `server_id` and fail closed on revocation-store errors.
- [x] Encrypt TOTP secrets and hash/consume recovery codes transactionally.
- [x] Encrypt database, SMTP, node, webhook, reCAPTCHA, and integration secrets at rest with a versioned keyring and rotation path.
- [x] Return masked configured-state fields instead of secret plaintext.
- [x] Move node probes/HMAC signing out of browser code.

## Phase 6 — Databases, schedules, mail, webhooks, backups

- [x] Add configurable TLS, validation, write-only API handling, and encrypted-at-rest credentials for DB hosts.
- [x] Make DB create/delete/rotation state compensating and failure-visible.
- [x] Correct global database-host handling and identifier validation.
- [x] Add durable replica-safe schedule claim/lease model.
- [x] Respect task offsets, online-only, continuation, cancellation, and restart lease recovery.
- [x] Implement real SMTP delivery with durable outbox/retries.
- [x] Make password reset enqueue transactional and revoke prior sessions on completion.
- [x] Add durable webhook queue, retries, delivery history, SSRF controls, signatures, and redaction.
- [-] Make backup lifecycle asynchronous with pending/terminal callbacks and lock semantics (Beacon adapter lifecycle is truthful; panel callback model pending).

## Phase 7 — Real transfer, migration, evacuation, recovery

- [x] Define authenticated versioned source/destination transfer protocol with scoped expiring credentials.
- [x] Implement source staging/archive/checksum.
- [x] Implement negotiated resume and correct source seeking.
- [x] Implement destination staging, checksum verification, rollback-protected activation, and finalize.
- [x] Persist transfer state/leases/cancellation and clean terminal in-memory/staging records.
- [x] Create/reconcile destination runtime and remove source only after PostgreSQL finalization.
- [x] Add rollback, idempotency, restart reclaim, and ordered panel callbacks.
- [-] Enable migration execution through the real transfer engine (implemented and unit/protocol tested; real two-node Docker validation pending).
- [ ] Execute evacuation items through real migrations after two-node validation.
- [ ] Execute recovery items and report real outcomes after two-node validation.

## Phase 8 — Frontend contract and feature completion

- [ ] Establish generated/shared API schemas and remove `as never` operational payloads.
- [x] Stop fabricating memory, CPU, uptime, allocation, and health values in audited production views.
- [x] Surface setup/settings/API outages instead of substituting success/default state.
- [x] Implement frontend admin role guard.
- [-] Implement complete admin server creation/edit/build/startup/allocations/databases/mounts (safe supported flows done; backend-unsupported controls remain disabled).
- [x] Return caller permissions and caller-safe SFTP endpoint data with the server resource; server navigation no longer depends on permission to list every subuser.
- [x] Route reinstall, logout revocation, per-signal power controls, and server allocation metadata edits through their correct permission-scoped backend contracts.
- [x] Expose real 2FA status from `/auth/me` and require explicit delegated scopes when creating personal API keys.
- [x] Correct backup timestamp DTO handling and enable supported startup command/container-image editing.
- [x] Add account reachability, search, client pagination, and polling to the server dashboard.
- [x] Expose existing self-service OAuth client registration, scoped credentials, one-time secret display, inventory, and revocation in the account UI.
- [x] Prevent destructive server settings updates; safe rename remains disabled until dedicated endpoint/UI contract.
- [x] Fetch real permissions and implement catalog-driven subuser permissions.
- [x] Complete files: loading safety, single-use ticketed streaming download, conflict-safe copy, validated chmod, bounded failure-visible mass delete/move, and errors.
- [x] Complete backup status/checksum/size/failure-aware UI and guarded actions; live progress transport remains future work.
- [x] Complete schedule/task editing, command/power/backup actions, offsets, continuation, validation, and run history.
- [x] Complete supported network allocation assign/unassign/add/remove/primary semantics without index assumptions.
- [-] Complete account password/email/security/session UI (password flow done; email update and session listing remain backend-limited).
- [x] Remove audited dead/mock components and inert production controls.
- [-] Add component and API-contract tests (36 tests passing across 4 files; 36.8% statements/lines, 63.35% branches, 29.59% functions); full browser end-to-end suite remains.

### Phase 8 latest validation

- [x] `npm --prefix forge/web test -- --run` — 4 files, 36 tests passed.
- [x] `npm --prefix forge/web run test:coverage` — 36.8% statements/lines, 63.35% branches, 29.59% functions after file-management expansion; generated output removed afterward.
- [x] `npm --prefix forge/web run typecheck` — passed.
- [x] `npm --prefix forge/web run lint` — passed with zero warnings; generated coverage is excluded.
- [x] `npm --prefix forge/web run build` — passed; 32 pages generated and workspace-root tracing warning resolved.
- [x] `go -C forge/api test ./...` — full API suite passed after frontend-discovered contract remediation.
- [ ] Add Playwright browser tests against PostgreSQL, API, and a real Beacon/Docker node.

## Phase 9 — Production deployment, monitoring, and release

- [x] Use same-origin browser REST API routing through the standalone Next.js proxy; WebSocket deployment validation remains.
- [ ] Deploy nginx (or another proxy) rather than shipping an unused template.
- [ ] Implement FQDN substitution and certificate provisioning/renewal.
- [ ] Correct nginx WebSocket-only routing and rate limiting.
- [x] Remove default required production secrets and bind PostgreSQL/Redis only to Compose networks.
- [ ] Reduce/proxy Docker socket privilege where feasible; document node trust boundary.
- [x] Connect Prometheus to Alertmanager.
- [x] Remove invalid Redis metric alert and add an exported Beacon runtime-health alert.
- [ ] Configure real Alertmanager receivers.
- [ ] Provision Grafana datasource, dashboards, credentials, and access policy.
- [ ] Validate multi-arch images and runtime configuration.
- [ ] Add security scanning, SBOM, dependency review, and image scanning.
- [ ] Add external-node, DB-host, SMTP, S3, TLS, backup/restore, and upgrade smoke tests.

## Final parity/release gate

- [ ] Repeat full source audit against all four reference projects.
- [ ] Run clean install and upgrade tests.
- [ ] Run real external node lifecycle test.
- [ ] Run real game database lifecycle test for PostgreSQL and MySQL/MariaDB.
- [ ] Run console/files/SFTP/backup/restore/transfer tests.
- [ ] Run role/API key/OAuth/security regression tests.
- [ ] Remove or clearly document every deliberate non-parity decision.
- [ ] Update README and status documentation to describe only verified behavior.
