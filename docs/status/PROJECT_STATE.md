# Project State

Current Phase: Phase 16.7 - Reality Enforcement, Workflow Completion, Daemon Hardening, and Pterodactyl Parity.

Reality rule: a workflow is production ready only when UI, API client, API route, handler/service, store, database, daemon/runtime, response, refresh, reload, and persistence have been verified. Anything not fully proven is marked not ready.

## Reference Workflow Baseline

Reviewed workflow expectations against local refs plus current public docs for Pterodactyl Panel, Wings, Pelican, and PufferPanel.

- Pterodactyl/Pelican pattern: Location -> Node -> Allocations -> Nest/Egg -> Server -> install -> console/power/files/backups/schedules/subusers.
- Wings pattern: panel sends authenticated daemon HTTP requests, daemon validates auth, executes runtime work, and streams state/logs over websockets.
- PufferPanel pattern: templates drive install/runtime configuration, with local disk backups available without object storage.

## Fixes Implemented In This Pass

- Daemon power actions are asynchronous. `start`, `stop`, `restart`, and `kill` now accept the request, launch runtime work in a goroutine, and do not bind shutdown/startup waits to the HTTP request lifecycle.
- Daemon power actions no longer hold the server mutex while Docker operations run.
- Runtime failures now clear `RunningAction`, preventing one failed Docker operation from wedging later power actions.
- Power requests no longer fake success as `mock-missing-container` when Docker reports a missing container.
- Panel-to-daemon power/create/delete/stats/install/config sync paths now support node-specific persisted tokens from the database instead of relying only on one global process token.
- Panel-to-daemon file manager, backup, scheduled backup, and realtime websocket paths now also pass the target node token. This closes the previously mixed trust model for core daemon calls.
- Daemon now exposes a signed `GET /auth/verify` probe, API startup logs signed daemon-auth verification per registered node, and daemon startup logs explicit panel remote-auth success/failure.
- Daemon auth middleware now rejects missing signatures with a clear message and signs/verifies the exact path plus query string consistently.
- API daemon power errors now include daemon response body details such as `invalid signature`.
- Admin node parity pass: node list/detail now exposes UUID, location/region identity, FQDN, scheme, proxy mode, daemon/SFTP ports, daemon base path, upload limit, token ID, heartbeat diagnostics, Docker/OS/architecture data, capacity, allocation counts, and server counts from API/store data.
- Admin node create/update now persists Pterodactyl-style node fields already supported by the Go API: location/region, FQDN/base URL, scheme, proxy mode, memory, disk, upload limit, daemon base, daemon listen port, SFTP port, maintenance, and drain state.
- Node configuration workflow now returns copyable daemon configuration and supports token regeneration from the configuration tab with the new token shown immediately.
- Node allocation workflow now supports range creation plus row-level alias/notes editing and guarded deletion from the node detail view.
- Nest/egg admin workflow now persists Docker images, startup command, stop command, install container/entrypoint/script, feature flags, and startup variables in the egg config payload. Egg import/export includes those fields.
- Tier 3 client console parity pass: the console connected badge now follows the real daemon websocket open/close lifecycle, exposes reconnect/error/last-close state, streams prior logs before live output, supports local search, clear, autoscroll, and command history, and disables command send unless the websocket is actually open.
- Tier 3 file manager parity pass: removed hardcoded editor content; list/read/write/upload/download/rename/delete/mkdir/archive/extract/copy/chmod/pull now route through frontend API helpers to API routes to daemon filesystem handlers with node-token auth and daemon safe-path checks. Multi-select delete/chmod, copy, pull-from-URL, download, dirty editor state, breadcrumbs, search, and refresh after mutation were added.
- Tier 3 network parity pass: server network now lists primary/additional allocations, can assign free same-node allocations, remove non-primary allocations, set primary allocation, respects allocation limits in UI, and refreshes server/allocation/activity queries after mutation. Backend allocation rows now mark the real primary allocation.
- Final refs parity pass: server network allocation alias/notes editing now has a server-scoped API route with ownership/assignment guard instead of relying on admin-only allocation editing.
- Final refs parity pass: schedule tasks now expose the supported Pterodactyl/Pelican action set in the client UI: command, power, and backup. Existing tasks can now be edited through the already-present backend task update route.
- Final refs parity pass: obsolete Templates/Packs concepts were removed from the active admin shell/type model. The active service-definition workflow is Nests/Eggs, matching Pterodactyl/Pelican.
- Phase 16.1 reality pass: server database create/rotate/delete were disabled in the client UI and changed to explicit `501 Not Implemented` API responses because the current backend only stores metadata and does not provision against a live MySQL/PostgreSQL host.
- Tier 3 startup parity pass: startup responses now preserve snake_case fields and add frontend-safe `rawStartupCommand`, `startupCommand`, `dockerImages`, and always-array `variables`. The startup page renders resolved/default commands, Docker image selection where available, editable variables only when permitted, and no longer depends on nullable variable arrays.
- Tier 3 activity parity pass: server activity now supports filtering by action, actor, target, metadata, and date range, and renders structured metadata instead of raw JSON blocks. New file actions append server-scoped audit events.
- Tier 3 settings/SFTP parity pass: server settings now support real rename/description update, Docker image update, reinstall, delete with confirmation, backend-confirmed delete exit from the selected server view, server metadata display, and SFTP host/port/username/command copy blocks from node/server data.
- Daemon file handlers now reject copy-into-self/copy-into-subtree, reject directory downloads, use request contexts for pull-from-URL, enforce URL scheme for pull, skip symlinks during copy/size walks, and cap unknown remote pulls.
- Daemon create no longer converts missing-image runtime failures into mock success. Delete of an already-missing container is reported as `docker-missing-container` instead of `mock-missing-container`.
- Allocation range parsing has a regression test for `25565`, `25565-25569`, and comma/range combinations.
- `start-dev.sh` now kills existing listeners on `8080`, `9090`, `3000`, and `2022` before startup, verifies ports are free, defaults daemon mock runtime to false, checks SFTP, and verifies tracked API/daemon/frontend PIDs remain alive after health checks.
- `stop-dev.sh` and `status.sh` now include SFTP port `2022`.

## Broken Workflows Found

- Full runtime smoke could not be completed in this execution environment. `./scripts/start-dev.sh` initially reported success, but a manual health curl immediately afterward found API/daemon/frontend/SFTP down. The process manager used by the tool appears to reap background `go run`/Next processes after script exit. The script now checks process survival before reporting success, but full browser/API smoke remains not proven.
- Panel/daemon auth model was inconsistent: node records persisted per-node tokens, but runtime requests used a global daemon client token. Core daemon paths have been converted to node tokens; database host provisioning remains outside daemon auth because it is panel/store driven.
- User-facing mock risk remains in daemon mock runtime mode. It is now not the dev default, but explicit `DAEMON_ALLOW_MOCK_RUNTIME=true` still exists as a safe development mode only.
- Database creation/rotation/deletion is not connected to actual DB host provisioning. These actions are now disabled rather than faking success, and remain not ready until implemented against a real MySQL/PostgreSQL host.
- Schedules have CRUD and runner code, but cron execution was not runtime-smoked. Marked not ready.
- Subuser permission enforcement was not fully runtime-smoked. Marked not ready.
- Tier 3 client workflows were statically repaired but not runtime-smoked because the user explicitly requested no build/test commands unless asked. Console websocket, file mutations, allocation assignment, startup variable persistence, settings mutations, and SFTP login remain not production-proven until a live smoke pass is run.
- Final refs-driven patches were statically inspected only. Server allocation metadata update and schedule task action/update flows still require browser/API runtime verification before they can be marked production ready.
- Database workflow remains honest but not ready: no usable database-host integration was runtime-proven in this pass, so database create/rotate/delete must be verified against a real host before being marked ready.
- Mount runtime consumption remains not proven from client workflows. Admin CRUD/assignment exists, but daemon/container rebuild consumption still needs live verification.

## Mock Workflows Found

- `DAEMON_ALLOW_MOCK_RUNTIME=true` is a safe dev mock only. It must remain disabled for production and is no longer the default in `scripts/start-dev.sh`.
- Some docs/reference files mention old mock/demo behavior; these are documentation debt, not active user flows.
- Frontend placeholder text remains in some search/input placeholders only. No new fake success states were added.
- Existing daemon mock runtime remains as explicit development mode only. It still emits honest "mock mode unavailable" console/log messages when enabled and must not be treated as production hosting runtime.

## Backend Gaps Found

- Admin database hosts have real CRUD and delete-in-use guards, but no live connection test endpoint. The UI should not show a fake test-success action until the backend can verify host credentials.
- Panel startup daemon probes are diagnostic-only and non-blocking. They log failures clearly but do not prevent the API from serving traffic.
- Real database host integration, S3/R2 adapters, and full SFTP permission workflows are not proven.
- Direct file copy/chmod/pull/download now exist through API and daemon, but chmod is local filesystem permission only and does not expose symbolic mode editing or ownership changes.
- Server database mutation endpoints intentionally return `501` because the current store layer only creates panel metadata. Real database host execution is still missing.

## Frontend/Backend Mismatches Found

- Allocation range UI sends `ports`; backend parser supports it and now has regression coverage. The originally reported failure is likely downstream of runtime/auth/startup state, not the parser.
- Power UI can only be trusted after websocket/state refresh is runtime-smoked. Backend now returns accepted asynchronously, but full UI refresh persistence was not proven here.
- Startup page crash contract mismatch was not reproduced in this pass and remains not ready until verified against live API payloads.
- Startup page null/array contract mismatch was fixed at the API response and frontend contract level, but not runtime-verified.
- File manager previously had hardcoded editor defaults and missing daemon routes for copy/chmod/pull/download. Those routes now exist, but browser upload/download UX was not live-smoked.
- Console previously trusted local React state too much. It now keys connection state from websocket lifecycle, but daemon auth/connect failure states still need live validation.
- Server settings delete previously had no confirmed exit path from the selected server view. It now clears the selected server only after backend delete success.
- Before this Tier 2 pass, the node UI omitted fields already required by the backend node contract. Create/update now sends those fields, and the list receives persisted server counts from the store.
- Egg startup variables had no admin editor despite the server startup workflow depending on egg variables as source of truth. Variables now persist inside egg config.

## Validation

- `npm install`: pass; npm reports 5 moderate dependency vulnerabilities.
- `npm run typecheck`: pass.
- `npm run lint`: pass.
- `npm run build`: pass.
- `cd apps/api && go build ./...`: pass.
- `cd apps/api && go test ./...`: pass.
- `cd apps/daemon && go build ./...`: pass.
- `cd apps/daemon && go test ./...`: pass.
- `bash -n scripts/start-dev.sh scripts/stop-dev.sh scripts/status.sh scripts/logs.sh`: pass.
- `./scripts/stop-dev.sh`: pass.
- `./scripts/start-dev.sh`: initial script health passed, but post-run manual health failed because managed background processes were gone in this tool environment. Full smoke remains blocked/not proven.
- Latest Tier 2 admin parity edits were not build-tested because the user explicitly requested no build commands unless asked. A non-build `git diff --check` passed for the touched admin/API files.
- Latest Tier 3 client parity edits were not build/typecheck/lint/go-test/runtime-smoke tested because the user explicitly requested no build commands unless asked.
- Latest Tier 3 static validation: `gofmt` was run on touched Go files, `git diff --check` passed for touched Tier 3 files, and targeted `rg` inspections found no new user-facing fake success or "coming soon" branches in the repaired client paths.
- Latest final refs parity static validation: `gofmt` was run on touched Go files, and `git diff --check` passed for the touched allocation/schedule/admin shell files. No build/typecheck/lint/test commands were run.

## Reality Audit Table

| Page | Feature | UI Exists | API Exists | Persistence | Daemon | Reload Safe | Production Ready |
|---|---:|---:|---:|---:|---:|---:|---:|
| Dashboard | Server list/navigation | Yes | Yes | Partial | No | Not proven | Not ready |
| Settings | General/security/mail/storage/API/daemon settings | Partial | Partial | Partial | No | Not proven | Not ready |
| API | Token create/delete/scope enforcement | Yes | Yes | Yes | No | Not proven | Not ready |
| Activity | Login/logout/create/delete/power/install/suspend logs | Yes | Partial | Yes | No | Not proven | Not ready |
| Locations | CRUD and provisioning consumption | Yes | Yes | Yes | No | Not proven | Not ready |
| Nodes | About/settings/config/token/heartbeat | Yes | Yes | Yes | Partial | Improved, not runtime-smoked | Not ready |
| Allocations | Single/range/bulk/search/delete | Yes | Yes | Yes | No | Parser and row actions improved | Not ready |
| Users | CRUD/role/password/ownership | Yes | Yes | Yes | No | Not proven | Not ready |
| Nests | CRUD/import/export | Yes | Yes | Yes | No | Not runtime-smoked | Not ready |
| Eggs | CRUD/clone/import/export/runtime fields | Yes | Yes | Yes | Partial | Not runtime-smoked | Not ready |
| Mounts | CRUD/assign/remove/server consumption | Yes | Partial | Partial | Partial | Not proven | Not ready |
| Servers | Create/install/delete/suspend | Yes | Yes | Yes | Partial | Not proven | Not ready |
| Console | Websocket/output/reconnect/power/state | Improved | Yes | No | Yes | Static only | Not ready |
| Files | List/read/write/upload/download/rename/delete/archive/extract/copy/chmod/pull | Improved | Yes | No | Yes | Static only | Not ready |
| Databases | Create/delete/rotate/limits | Disabled honestly | Partial | Metadata only | No | Static only | Not ready |
| Backups | Local create/delete/restore/download | Yes | Yes | Partial | Yes | Not proven | Not ready |
| Schedules | CRUD/run now/task execution/task edit | Improved | Yes | Yes | Partial | Static only | Not ready |
| Subusers | Invite/update/remove/permissions | Yes | Yes | Yes | No | Not proven | Not ready |
| Startup | Variables/docker image/command | Improved | Yes | Yes | Partial | Static only | Not ready |
| Network | Primary/additional allocation assignment/metadata | Improved | Yes | Yes | No | Static only | Not ready |
| Server Settings | Rename/description/SFTP/reinstall/delete/docker image | Improved | Yes | Yes | Partial | Static only | Not ready |
| Server Activity | Per-server activity/filtering/metadata | Improved | Yes | Yes | No | Static only | Not ready |

## Production Readiness Scores

- Build readiness: 85/100.
- API/store readiness: 65/100.
- Daemon/runtime readiness: 55/100.
- Frontend workflow readiness: 58/100.
- Security readiness: 45/100.
- Full hosting-platform readiness: 48/100.

Overall status: not production ready. Core build health was previously good, and the critical power/auth/admin/client workflow contracts are improved, but the latest Tier 3 pass has only static validation because build/runtime validation was intentionally not run.
