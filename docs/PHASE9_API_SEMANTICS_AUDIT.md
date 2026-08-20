# Phase 9 API Semantic Correctness — Route & Status Audit

This document audits the Phase 9 requirements: normalized status codes, cron admission validation, and duplicate/aliased route handling.

## 1. Status Code Normalization

Canonical mapping (enforced via `forge/api/internal/http/errors.go:domainErrorStatus`):

| Domain / Error Class | HTTP Status | Example |
|---|---|---|
| Conflict (duplicate/already exists/limit reached/conflict/invalid transition) | **409** | `a backup with this name already exists`, `backup limit reached`, `already in use`, `evacuate or remove` |
| Validation (is required/must be/too long/between/unsupported/invalid) | **422** | `name is required`, `invalid cron expression`, `command too long`, `between 1 and 100 paths are required` |
| Malformed (JSON parse failure) | **400** | `invalid request body` from `c.BodyParser` |
| Unauthorized | **401** | `missing session`, `invalid session` |
| Forbidden | **403** | `backup is locked and cannot be deleted`, `not assigned`, `permission` |
| Missing | **404** | `not found`, `does not exist` |

Helper `respondStoreError(err)` (`errors.go:respondStoreError`) replaces ad-hoc `fiber.NewError(fiber.StatusBadRequest, err.Error())` after store calls. All `handlers_servers.go` and `handlers_admin.go` store-error paths now use it (bulk migration of ~90 call sites). Handlers that previously returned `400` for duplicate/conflict now correctly return `409`.

Manual validation returns previously using `400` for field checks (`name is required`, `allocationId is required`, etc.) were migrated to `422` via bulk script matching `is required`/`must be`/`may only contain` patterns across `handlers_*.go`.

## 2. Cron Admission Validation (422 at creation, not at runner)

- **CronJob schedule** (`handlers_cronjob.go:validateCronSchedule`):
  - Now uses `robfig/cron` parser `cron.NewParser(Minute|Hour|Dom|Month|Dow)` to validate full syntax.
  - Returns `422` for: empty, wrong field count (≠5), interval <1m (`* *`), or parser error (e.g. `60 * * * *`, `foo * * * *`).
  - Previously only checked field count + interval; syntax errors were deferred to `cronjob.Service.scheduleJob` at runner time.

- **Server schedules** (`handlers_servers.go:validateServerScheduleCron`):
  - New helper validates 5-field cron from `CreateScheduleRequest`/`PatchScheduleRequest`.
  - Applied in `POST /servers/:id/schedules` and `PATCH /servers/:id/schedules/:scheduleId` before `Store.CreateSchedule`/`PatchSchedule`.
  - Ensures invalid cron returns `422` at admission; `schedule_runner.go:nextScheduleRun` and `cronjob.Service` no longer are first discovery point.

- **Procedure schedules** (`handlers_procedures.go:validateProcedureCron`):
  - Validates `CronExpression` on `POST /procedures` and `PUT /procedures/:id` via same cron parser, returning `422`.

All three validate at HTTP admission layer; `store` retains defense-in-depth but handler guarantees `422`.

## 3. Duplicate / Aliased Routes — Canonical vs Alias

Audit of `handlers_servers.go` and `server.go`:

| Canonical (preferred) | Alias (deprecated, retained) | Handler Sharing |
|---|---|---|
| `DELETE /servers/:id/files?path=` (RESTful query param) | `DELETE /servers/:id/files/delete?path=` (legacy PufferPanel compat) | Shared `deleteFileHandler` (`handlers_servers.go:deleteFileHandler`) — both routes delegate to same function; alias commented `// alias — deprecated` |
| `PATCH /servers/:id/files/rename` (RESTful) | `POST /servers/:id/files/rename` (legacy) | Shared `renameFileHandler` — PATCH canonical per REST; POST alias |
| `POST /servers/:id/databases/:databaseId/rotate-password` (path param, JSON response) | `PATCH /servers/:id/databases/reset-password` body `{database:<id>}` (204) | Documented in `handlers_servers.go:Route alias audit` comment; both call `DBProvisioner.RotatePassword` |
| `DELETE /servers/:id/databases/:databaseId` (200 JSON) | `DELETE /servers/:id/databases/:databaseId/delete` (204) | Both exist; alias returns 204 for compat |
| `POST /api/v1/oauth2/token` (RFC 6749 canonical) | `POST /api/v1/oauth/token` (legacy) | `server.go:OAuth2 token endpoint — canonical vs alias audit` — both mount `IssueOAuth2Token`; alias documented deprecated |
| `DELETE /servers/:id/backups?name=` & `DELETE /servers/:id/backups/:backupName` | — | Not de-duplicated (query vs path variants serve different clients); documented as intentional dual-form, not deprecated. |

No alias was removed via `git rm`; duplicates retained for backward compat but now delegate to shared handlers and are explicitly documented as `canonical` vs `alias — deprecated`. The second-copy file-delete/rename blocks at `handlers_servers.go:3085` were removed (in-file edit, not git delete) after consolidating to shared handlers to avoid double registration.

## 4. go vet

- Fixes: `store_reservations.go:197,223` (`return nil, err` → `return nil, 0, err`), `store_users.go:168,179` same, `handlers_admin_extras.go` removed unused `strconv`, `handlers_tenancy.go` removed unused `strconv`.
- After fixes: `go vet ./...` passes in `forge/api` and `beacon` (verified `2026-08-23`).

## Verification

- `go vet ./...` — pass
- `go test ./internal/http -run TestCronJobs_Create -v` — pass (expects 422 for validation, new `invalid cron syntax` case)
- Full `go test ./internal/http` — pass after updating 3 legacy tests that expected `400` for validation to `422` (now correct).
