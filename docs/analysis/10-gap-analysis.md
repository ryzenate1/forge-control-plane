# 10 — Gap Analysis

## What Is Missing Entirely (No Code Exists)

### Frontend Gaps

| Feature | API Backend | UI | Priority |
|---|---|---|---|
| User server dashboard (`/servers` page) | ✅ `fetchServers` | ❌ | P0 |
| 2FA login checkpoint | ✅ `loginCheckpoint` API + backend | ❌ | P0 |
| Logout button | ✅ `logout()` function | ❌ | P0 |
| 2FA setup/management UI | ✅ backend complete | ❌ | P1 |
| SSH key management | ✅ backend complete | ❌ | P1 |
| Server transfer UI | ✅ backend complete | ❌ | P2 |
| Regions admin management | ✅ `fetchRegions` | ❌ | P2 |
| Account settings page | N/A | ❌ | P1 |
| 404 not-found page | N/A | ❌ | P3 |
| Template edit/delete | ✅ API supports it | ❌ | P3 |

### Backend Gaps

| Feature | Status | Priority |
|---|---|---|
| `dbprovisioner` (actual MySQL/Postgres provisioning) | Empty package | P2 |
| SMTP backend for mail test | Stub endpoint | P2 |
| `realtime/` Redis pub/sub fanout | README placeholder only | P3 |
| Schedule command task | Returns error stub | P1 |
| Migrations 029–031 | Gap in numbering | P3 |

### Beacon Gaps

| Feature | Status | Priority |
|---|---|---|
| TLS support | Not implemented | P1 |
| S3 backup | Client nil | P1 |
| Panel install notification | Stub | P0 |
| Panel backup completion callback | Never called | P1 |
| Panel activity log submission | Never called | P1 |
| Streaming Docker stats (not polling) | Polling 2s | P1 |
| Real-time install log streaming | Batch after completion | P1 |
| Per-server WebSocket authorization | Not implemented | P1 |
| SSRF protection on file pull | Not implemented | P1 |
| Docker volume deletion on server delete | Not implemented | P2 |
| SFTP disk quota enforcement | Not implemented | P2 |
| SFTP session cancellation (deauthorize) | Not implemented | P2 |
| Server state persistence across restarts | Not implemented | P1 |

---

## What Is Broken (Code Exists But Wrong)

### Frontend Broken

| # | Item | Current Behavior | Correct Behavior |
|---|---|---|---|
| 1 | `/server/[id]/files` | Renders `FileManager` (alert stub) | Should render `FilesView` (Monaco, full featured) |
| 2 | `AdminLogs` | Calls `/activity`, returns audit events, maps as `LogEntry{level,message}` | Needs real logs endpoint |
| 3 | `PowerControls` | Uses raw `fetch()` with hardcoded `localStorage` key | Should use `sendPowerSignal()` |
| 4 | Console page WS | 3 separate WS connections for stats | Should share one connection |
| 5 | `ServerSettingsView` | Missing `node` prop, shows fallback SFTP values | `ServerConsoleLayout` must pass `node` |
| 6 | `ApiUserWithLimits` type | Defined as alias of `ApiUser` | Should extend with limit fields |
| 7 | `useServerStore` `currentUser` | Always null | Should be set on login |
| 8 | Zustand console/stats state | Never written to by any component | Either use it or remove it |

### Backend Broken

| # | Item | Severity | Fix |
|---|---|---|---|
| 1 | 10 nil service pointers in `NewServer` | 🔴 Panic | Wire in `main.go` |
| 2 | All 6 observability handlers dereference nil | 🔴 Panic | Wire `observabilitySvc` |
| 3 | `wsTicketStore` no mutex | 🟡 Race | Add `sync.RWMutex` |
| 4 | `PluginsDir` never read from env | 🟡 Config | Read `PLUGINS_DIR` env var in `main.go` |
| 5 | `UpdatePlugin` uses raw SQL | 🟠 Style | Move to store abstraction |

### Beacon Broken

| # | Item | Severity | Fix |
|---|---|---|---|
| 1 | Missing `golang.org/x/sync` | 🔴 Build | `go get golang.org/x/sync` |
| 2 | `deleteBackup` not registered | 🔴 Routing | Add `DELETE` route to mux |
| 3 | S3 client nil | 🔴 Runtime | Initialize from env vars |
| 4 | `notifyPanelInstallStatus` stub | 🔴 Data | Implement POST to panel |
| 5 | `postUpdate` does nothing | 🟡 Config | Implement config reload |
| 6 | `syncEnvironmentFromPanel` stub | 🟡 Runtime | Implement env var fetch |
| 7 | `heartbeatMb` uses Go heap | 🟡 Data | Use runtime system stats |

---

## What Is Orphaned / Dead Code

| File | Superseded By | Safe to Remove? |
|---|---|---|
| `forge/web/components/server/file-manager.tsx` | `files-view.tsx` | Yes (after wiring fix) |
| `forge/web/components/server/backup-list.tsx` | `backups-view.tsx` | Yes |
| `forge/web/components/server/database-list.tsx` | `databases-view.tsx` | Yes |
| `forge/web/components/server/console-view.tsx` | `console.tsx` (xterm) | Yes |
| `forge/web/components/admin/admin-layout.tsx` | `admin-shell.tsx` | Yes |
| `forge/web/components/pterodactyl/` | Empty directory | Yes |
| `beacon/internal/events/events.go` | (never imported) | Repurpose or remove |
| `beacon/internal/system/atomic.go` | (never imported) | Repurpose or remove |
| `beacon/internal/system/locker.go` | (never imported, has bug) | Fix then use, or remove |

---

## Pterodactyl Feature Parity Gaps

Critical Pterodactyl features that GamePanel is missing or has only partial support for:

| Feature | Pterodactyl | GamePanel |
|---|---|---|
| Egg variable validation | `VariableValidatorService` | Not present |
| Multiple Docker images per egg | `docker_images` map | Single image |
| Egg import/export (PTDL_v2) | Full JSON import/export | Not present |
| Egg file denylist | Per-egg JSON list | Not present |
| Egg config inheritance (`config_from`) | Supported | Not present |
| Database auto-deployment | `DeployServerDatabaseService` | Empty `dbprovisioner` |
| Backup `is_locked` field | Supported | Not present |
| Schedule `only_when_online` | Supported | Not present |
| Schedule `continue_on_failure` | Supported | Not present |
| SFTP disk quota enforcement | Wings enforces | Beacon missing |
| Activity log to panel from daemon | Wings sends batched events | Beacon never calls |
| State persistence across daemon restarts | `states.json` | Not implemented |
