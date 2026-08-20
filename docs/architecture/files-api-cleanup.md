# Files API Cleanup — CLEANUP phase

**Status:** PLANNED — inventory and normalization target per `target-api-map.md §5` and `plan §9 CLEANUP`
**Source:** `forge/api/internal/http/handlers_servers.go:2333` and `docs/architecture/overview.md:103`

## Current truth — 18+ verb+path variants on `/servers/:id/files`

Inventoried via `grep -n "/servers/:id/files" forge/api/internal/http/handlers_servers.go`:

| # | Method | Path | Handler | Purposes |
|---|--------|------|---------|----------|
| 1 | GET | `/servers/:id/files` | `ListFiles` via `cfg.Daemon.ListFiles(…, c.Query("path"))` | List dir @ `?path=` |
| 2 | GET | `/servers/:id/files/content` | `ReadFile` | Read file content `?path=` |
| 3 | PUT | `/servers/:id/files/content` | `WriteFile` | Write file content |
| 4 | POST | `/servers/:id/files/archive` | `Archive` | Create zip/tar at `path` |
| 5 | POST | `/servers/:id/files/decompress` | `Decompress` | Extract archive |
| 6 | PUT | `/servers/:id/files/upload` | `UploadFiles` (multipart `file`/`content`) | Upload |
| 7 | DELETE | `/servers/:id/files` | `DeleteFile` `?path=` |
| 8 | POST | `/servers/:id/files/mkdir` | `Mkdir` |
| 9 | PATCH | `/servers/:id/files/rename` | `Rename` single |
|10 | POST | `/servers/:id/files/delete-batch` | `DeleteBatch` `mutationLimiter` |
|11 | POST | `/servers/:id/files/rename-batch` | `RenameBatch` |
|12 | POST | `/servers/:id/files/copy` | `Copy` |
|13 | POST | `/servers/:id/files/chmod` | `Chmod` |
|14 | DELETE | `/servers/:id/files/delete` | `Delete` alias (duplicate of 10?) |
|15 | POST | `/servers/:id/files/rename` | `Rename` POST alias of PATCH 9 |
|16 | POST | `/servers/:id/files/chmod-batch` | `ChmodBatch` |
|17 | POST | `/servers/:id/files/create-directory` | `CreateDirectory` alias of mkdir |
|18 | POST | `/servers/:id/files/pull` | `PullRemoteFile` (canonical `spec` contract) |
|19 | POST | `/servers/:id/files/download` | `PullRemoteFile` alias (PufferPanel-compat) |
| + | GET | `/servers/:id/files/download-url` | `DownloadURL` |
| + | WebSocket | `/servers/:id/ws/*` | `stats/logs/console/install/backup` | Not files but co-located |

OpenAPI `openapi.json` 256 paths lags: spec lists `GET /files/list` vs handler `GET /files?path=` (see `overview.md:103`), phantom `/roles` without `admin/` prefix.

## Target normalization

Resource-oriented, single surface: `/servers/:id/files` with action model, plus download-url. Keep `pull` as canonical, `download` alias deprecated with warning header until `findAdminPage` shows 0 legacy hits.

```
GET    /api/v1/servers/:id/files?path=/           — list (existing)
GET    /api/v1/servers/:id/files/content?path=…   — read (keep)
PUT    /api/v1/servers/:id/files/content           — write (keep)
POST   /api/v1/servers/:id/files/actions/archive   — {path}
POST   /api/v1/servers/:id/files/actions/archive-extract — {path}
POST   /api/v1/servers/:id/files/actions/mkdir     — {path}
POST   /api/v1/servers/:id/files/actions/copy      — {from,to}
POST   /api/v1/servers/:id/files/actions/move      — replaces rename PATCH+POST + rename-batch
POST   /api/v1/servers/:id/files/actions/chmod     — {path,mode} (+ batch)
POST   /api/v1/servers/:id/files/actions/delete    — {paths:[…]} replaces DELETE + delete-batch + delete alias
POST   /api/v1/servers/:id/files/actions/upload    — multipart (keep PUT /files/upload as compat)
POST   /api/v1/servers/:id/files/actions/pull      — {url,path} canonical (keep POST /files/download alias)
GET    /api/v1/servers/:id/files/download-url?path= — presigned (keep)
```

- All `?path=` validated against `fieldSpec` allowlist; verbs that mutate go through `mutationLimiter` + `PermFile*` guards already present.
- Response envelope `{data,meta,error:{code,message,requestId}}` — no raw DB errors.
- WebSocket streams remain `/servers/:id/ws/{stats,logs,console,install,backup}` via `realtimeProxy`.

## Migration safety

- Keep every existing verb+path as compat shim that delegates to the new `actions/*` handler and logs `Deprecation: use POST /servers/:id/files/actions/*`.
- Additive only: add new `actions/*` routes before removing old ones; deprecate after `findAdminPage` + API access logs show 0 hits for 30d.
- Tests per refactor: contract + authz (`PermFile*`), state (`store_state.go:121`), and job (`pipeline/service.go`) for file ops; frontend route/loading/empty/error.

## Diagnostics leakage

`infra/ship/kubernetes/secret.yaml:10` `CHANGE_ME_*` placeholders remain install-safe but `handlers_admin.go` debug endpoints currently leak internal state; move behind `Advanced → Diagnostics` (next CLEANUP step) and gate with `role: admin` + audit.

## Next

- Add `actions/*` handlers in `handlers_servers.go` wrapping existing `cfg.Daemon.*` calls.
- Regenerate `openapi.json` from handlers; remove 26 phantom paths without `admin/` prefix, add missing `GET /setup/status`, `POST /auth/session/refresh`.
- Keep `admin-registry.ts:24` 77 hrefs aliased; no delete before `findAdminPage` confirms 0 legacy hits.
