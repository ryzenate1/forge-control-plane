# 11 — Prioritized Reconstruction Roadmap

## Phase 0 — Compilation & Runtime Stability (Days 1–2)

These must be completed before anything else works. Each item is under 1 hour.

| # | Task | File | Change | Time |
|---|---|---|---|---|
| 1 | Fix beacon `go.mod` — add `golang.org/x/sync` | `beacon/go.mod` | `go get golang.org/x/sync` | 5 min |
| 2 | Register `deleteBackup` route in HTTP mux | `beacon/internal/server/server.go` | Add `DELETE` route | 10 min |
| 3 | Add mutex to `wsTicketStore` | `forge/api/internal/http/handlers_ws_ticket.go` | Add `sync.RWMutex` | 15 min |
| 4 | Read `PLUGINS_DIR` env var in `main.go` | `forge/api/cmd/api/main.go` | `os.Getenv("PLUGINS_DIR")` | 5 min |

---

## Phase 1 — Wire the Service Graph (Days 2–4)

The most impactful single change. Instantiates all 10 services and wires the event bus. Services must be instantiated in dependency order:

1. `events.Registry` — no deps
2. `runtime.Registry` — no deps
3. `scheduler.Scheduler` — store
4. `reservations.Manager` — store, events
5. `clustermanager.Service` — store, runtime, scheduler, reservations, events
6. `heartbeatmonitor.Service` — store, events
7. `reconciler.Service` — store, clusterManager, events
8. `evacuationplanner.Service` — store, scheduler, events
9. `migration.Service` — store, reservations, events
10. `recovery.Coordinator` — store, scheduler, reservations, migration, events
11. `observability.Service` — store, events → subscribe to events registry

Then start background goroutines:
- `heartbeatMonitor.Start(ctx)`
- `reconciler.Start(ctx)`
- `reservations.Manager.Start(ctx)`
- `observability.Subscribe(eventRegistry)`

**Expected outcome:** Server creation, power control, and deletion all become functional. All orchestration routes stop panicking.

---

## Phase 2 — Frontend Core Fixes (Days 3–5, parallel with Phase 1)

| # | Task | File | Fix |
|---|---|---|---|
| 1 | Wire `FilesView` to files page | `app/server/[id]/files/page.tsx` | Change import from `FileManager` to `FilesView` |
| 2 | Add logout button | `admin-shell.tsx` + `server-nav.tsx` | Call `logout()` + redirect to `/` |
| 3 | Add `/servers` user dashboard | `app/servers/page.tsx` | New page listing `fetchServers()` |
| 4 | Add 2FA login checkpoint UI | `app/page.tsx` | Conditional render checkpoint form when token required |
| 5 | Fix `PowerControls` | `components/server/power-controls.tsx` | Use `sendPowerSignal()` from `api.ts` |
| 6 | Fix `AdminLogs` endpoint | `components/admin/AdminLogs.tsx` | Call correct log endpoint |
| 7 | Pass `node` prop to `ServerSettingsView` | `components/server/server-console-layout.tsx` | Add `node` to props |
| 8 | Fix Tailwind CSS variables | `tailwind.config.ts` | Define `panel-brand`, `panel-ink`, `panel-line`, `panel-muted` |

---

## Phase 3 — Beacon Reliability (Week 2)

| # | Task | Priority |
|---|---|---|
| 1 | Implement `notifyPanelInstallStatus` — POST install completion to panel API | P0 |
| 2 | Implement `sendActivityLogs` calls after file/power operations | P1 |
| 3 | Implement `sendBackupCompletion` after backup creation | P1 |
| 4 | Fix S3 backup — initialize client from env vars (`S3_ENDPOINT`, `S3_BUCKET`, etc.) | P1 |
| 5 | Switch `statsWS` to `ContainerStats` streaming mode (not one-shot polling) | P1 |
| 6 | Switch `logsWS` to Docker log streaming with `follow=true` | P1 |
| 7 | Add real-time install log streaming via `installWS` | P1 |
| 8 | Add TLS support via cert/key path env vars | P1 |
| 9 | Add SSRF protection to `pullRemoteFile` | P1 |
| 10 | Add per-server WebSocket authorization check | P1 |
| 11 | Implement `syncEnvironmentFromPanel` to fetch startup variables pre-start | P2 |
| 12 | Implement `postUpdate` config reload | P2 |
| 13 | Fix `heartbeat` `memoryMb` to report system RAM, not Go heap | P2 |
| 14 | Implement server state persistence (`states.json` save/restore) | P1 |

---

## Phase 4 — Feature Parity (Weeks 3–5)

| Feature | Description | Effort |
|---|---|---|
| 2FA setup/management UI | Account page with TOTP QR code flow | Medium |
| SSH key management UI | List/add/remove keys | Small |
| Schedule command task support | Implement the errored-stub task type | Medium |
| Schedule `only_when_online` field | Skip run if server is offline | Small |
| Schedule `continue_on_failure` per task | Control task chaining on error | Small |
| Backup `is_locked` field | Prevent deletion of locked backups | Small |
| `dbprovisioner` — actual database provisioning | Wire MySQL/Postgres create/drop | Large |
| Docker volume cleanup on server delete | Remove named volumes on teardown | Small |
| SFTP disk quota enforcement | Check quota before writes | Medium |
| SFTP session cancellation via deauthorize | Kill active sessions on token revoke | Small |
| Mail SMTP backend | Replace stub with real SMTP sender | Medium |
| Server transfer UI | Initiate and track node-to-node transfer | Medium |
| Regions admin management UI | CRUD for regions | Medium |

---

## Phase 5 — Advanced Orchestration (Weeks 6–10)

Once Phase 1 wires the services, these become integration and UI work rather than foundational engineering.

| Feature | Effort |
|---|---|
| Evacuation planner end-to-end testing and UI | Large |
| Recovery coordinator activation and UI | Large |
| Migration engine UI (view/cancel migrations) | Medium |
| Observability timeline UI | Large |
| Redis realtime pub/sub fanout (`realtime/` package) | Large |
| Multiple Docker images per egg (label→image map) | Medium |
| Egg import/export PTDL_v2 format | Medium |

---

## Gantt Overview

```mermaid
gantt
    title GamePanel Reconstruction Roadmap
    dateFormat YYYY-MM-DD
    section Phase 0 — Stability
    Fix go.mod + beacon compile     :crit, p0a, 2026-06-18, 1d
    Wire deleteBackup route         :crit, p0b, 2026-06-18, 1d
    Add wsTicketStore mutex         :p0c, 2026-06-18, 1d
    section Phase 1 — Service Wiring
    Instantiate events.Registry     :crit, p1a, 2026-06-19, 1d
    Wire all 10 services in main.go :crit, p1b, after p1a, 3d
    Start background goroutines     :p1c, after p1b, 1d
    section Phase 2 — Frontend
    Wire FilesView to files page    :crit, p2a, 2026-06-19, 1d
    Add logout + user dashboard     :p2b, 2026-06-19, 2d
    2FA checkpoint UI               :p2c, after p2b, 2d
    Fix PowerControls + AdminLogs   :p2d, 2026-06-19, 1d
    Fix Tailwind CSS vars           :p2e, 2026-06-19, 1d
    section Phase 3 — Beacon
    Panel install notification      :crit, p3a, 2026-06-23, 2d
    Backup callbacks                :p3b, after p3a, 2d
    Fix S3 backup                   :p3c, after p3a, 2d
    Streaming WS stats/logs         :p3d, after p3b, 3d
    TLS + SSRF protection           :p3e, after p3c, 3d
    section Phase 4 — Parity
    2FA and SSH UI                  :p4a, 2026-07-07, 3d
    Schedule features               :p4b, after p4a, 3d
    dbprovisioner                   :p4c, after p4b, 5d
    Server transfer UI              :p4d, after p4c, 4d
    section Phase 5 — Orchestration
    Evacuation UI                   :p5a, 2026-07-28, 5d
    Recovery UI                     :p5b, after p5a, 5d
    Observability timeline          :p5c, after p5b, 5d
```
