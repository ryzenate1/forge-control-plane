# Subagent 02 — Backup Progress WS Beacon→Panel Gap (Phase 05 Wiring)

**Slice:** Wire `backupProgressWS` beacon→panel gap — ensure end-to-end live progress
**Re-verifies:** 110-03-10 fix (`beacon/server.go:62,352,1596,2237` via `SetProgressCallback → eventBus.Publish` vs `forge/api/internal/http/server.go:1995` `realtimeProxy` only `stats|logs|console`)
**Status:** Verified — no additional code change required (gap already closed in Phase 03). Re-verified wiring end-to-end and documented live chain.

---

## 1. Task Checklist (Required Wiring Points)

| Required | Expected State | Actual State | Verdict |
|----------|---------------|--------------|---------|
| `forge/api/internal/http/server.go:1995` add `"backup"` to `realtimeProxy` union | 4 routes `stats|logs|console|backup` via `realtimeProxy` | `forge/api/internal/http/server.go:2036`, `2047`, `2058`, `2069` — 4 routes wired, each `wsOriginMiddleware` + `RecoverHandler` + `wsUpgraderOrigins(getWebSocketAllowedOrigins(cfg))` | PASS |
| `forge/web/lib/api.ts` `ServerStream` includes `"backup"` | `ServerStream = "stats"|"logs"|"console"|"backup"` | `forge/web/lib/api.ts:236` `export type ServerStream = "stats" | "logs" | "console" | "backup"` | PASS |
| `forge/web/components/server/backups-view.tsx:13` `useBackupProgress` renders live progress (not just polling) | WS hook + live bar + `● live` indicator | `forge/web/components/server/backups-view.tsx:20-91` hook def, `155` invocation, `174-183` live bar with `bytesProcessed/totalBytes/phase/%` + pulse + `● live backup progress` | PASS |
| Beacon `eventBus → API WS → web hook` chain | `BackupProgress` → `SetProgressCallback` → `eventBus.Publish` → `backupProgressWS` → `daemon.WebSocketURL` → `realtimeProxy` pump → `useBackupProgress` | Verified below §2-§5 | PASS |

---

## 2. Beacon — `BackupProgressEvent` via `SetProgressCallback` → `eventBus.Publish`

**Constant + bus:**
- `beacon/internal/server/server.go:95` `const BackupProgressEvent = "backup progress"` (`beacon/internal/events/events.go:42` `Publish` marshals `Event{Topic, Data}` as JSON to buffered chans with ring-buffer drop-oldest).
- `beacon/internal/server/server.go:98-114` `Server{ backups BackupInterface, eventBus *events.Bus }`, `NewServerWithBackup:273-315` initializes `events.NewBus()`.

**Interface:**
- `beacon/internal/backup/backup.go:33-41` `BackupProgress{BytesProcessed, TotalBytes, Phase}` + `ProgressFunc`, `67-68` `BackupInterface{ SetProgressCallback(ProgressFunc) }`.
- `beacon/internal/backup/local.go:45` `LocalBackup{ progress ProgressFunc }`, `78-82` `SetProgressCallback`, `244-251` `reportProgress` (thread-safe), `s3.go:121-124` `S3Backup.SetProgressCallback` delegates to `local`.

**Publish site (per-backup):**
- `beacon/internal/server/server.go:1593-1597` inside `createBackup` (holds `backupMu` so concurrent creates serialize):
  ```go
  s.backupMu.Lock()
  if s.eventBus != nil {
    s.backups.SetProgressCallback(func(p backup.BackupProgress) {
      s.eventBus.Publish(BackupProgressEvent+":"+serverID, p)
    })
  }
  // ... idempotency check then s.backups.Create(...)
  s.backupMu.Unlock()
  ```

**Subscribe site (WS):**
- `beacon/internal/server/server.go:385` `mux.HandleFunc("GET /servers/{id}/ws/backup", server.backupProgressWS)`
- `beacon/internal/server/server.go:2210-2262` `backupProgressWS`:
  - checks `s.backups != nil`, `authenticateWebSocket` (`2220`), scope `ScopeWebsocket||ScopeBackupDownload` (`2224`), upgrades via `websocketUpgrader` (`2229`), `trackWebSocket` + `configureWebSocket` + `pingWebSocket` (`2234-2246`),
  - validates `claims.ServerID == serverID` (`2240`),
  - subscribes `ch := s.eventBus.Subscribe(BackupProgressEvent+":"+serverID)` (`2247`), defers `Unsubscribe` (`2248`),
  - loops `select { case <-r.Context().Done(): return; case msg, ok := <-ch: writer.Write(msg) }` (`2250-2261`). `webSocketWriter.Write:3700` sends `websocket.TextMessage` with the `Event` JSON (already marshaled by Bus).

**Test:** `beacon/internal/server/backup_progress_wiring_test.go:11` `TestBackupProgressEventViaSetProgressCallback` asserts constant, handler, `SetProgressCallback+Publish`, route, and `LocalBackup` implements interface. `go test ./beacon/internal/server -run TestBackupProgress -v` → PASS.

---

## 3. Forge API — `realtimeProxy` Now Includes `backup`

**Before (Phase 03 gap):** `forge/api/internal/http/server.go:1995` wired only `stats|logs|console`. `/servers/:id/ws/backup` unreachable from panel.

**After (present):** `forge/api/internal/http/server.go:2036-2079`
```go
v1.Get("/servers/:id/ws/stats",   requireRealtimeServices(cfg), wsOriginMiddleware(cfg), fiberws.New(realtimeProxy(cfg, wsTickets, "stats"),   ...))
v1.Get("/servers/:id/ws/logs",    requireRealtimeServices(cfg), wsOriginMiddleware(cfg), fiberws.New(realtimeProxy(cfg, wsTickets, "logs"),    ...))
v1.Get("/servers/:id/ws/console", requireRealtimeServices(cfg), wsOriginMiddleware(cfg), fiberws.New(realtimeProxy(cfg, wsTickets, "console"), ...))
v1.Get("/servers/:id/ws/backup",  requireRealtimeServices(cfg), wsOriginMiddleware(cfg), fiberws.New(realtimeProxy(cfg, wsTickets, "backup"), ...)) // NEW
```
Each uses same `RecoverHandler`, `wsUpgraderOrigins(getWebSocketAllowedOrigins(cfg))` (`forge/api/internal/http/realtime.go:29`).

**`realtimeProxy` logic** `forge/api/internal/http/realtime.go:124-361`:
- Origin enforcement (`128-147`), `requireRealtimeServices` check.
- Ticket vs JWT auth (`163-232`): peeks ticket, validates `wsTicket.Stream == stream` (`167`), `ticket server mismatch` (`171`), `current.Sub == wsTicket.UserID` (`199`), defers consume until success (`275-278`).
- UserCanAccessServer checks (`237-273`):
  ```go
  // 237 base: PermWebsocketConnect
  if stream == "console" { // 250
    check PermControlConsole
  }
  if stream == "backup" { // 263
    backupAllowed, _ := cfg.Store.UserCanAccessServer(ctx, id, userID, userRole, store.PermBackupRead)
    if !backupAllowed { return "missing server permission: backup.read" }
  }
  ```
  `forge/api/internal/store/permissions.go:45` defines `PermBackupRead = "backup.read"`.
- Node liveness (`288-296`), `daemon.WebSocketURL(target.NodeURL, target.ServerID, stream):298` (`forge/api/internal/daemon/client.go:1408` path `"/servers/"+serverID+"/ws/"+stream`), `SignedHeaders` + `MintWebsocketToken(target.NodeToken, target.ServerID, userID):308` (`beacon` expects `ScopeWebsocket`), dials via `gorilla.DefaultDialer.DialContext`, sends `{"status":"connected","stream":stream}` (`330`), rate-limits client→upstream 10/s (`352`), pumps `pumpUpstreamToClient` ↔ `pumpClientToUpstream` (`349-360`).

**Ticket handler:** `forge/api/internal/http/handlers_ws_ticket.go:159-198` `IssueWSTicket` — `stream := c.Query("stream","console")` (`165`), `checkServerPermission("websocket.connect")` (`172`), signs `subject+"."+HMAC(Secret, "forge:ws-ticket:v1\x00"+subject)` (`207-213`), stores `wsTicket{ServerID, Stream, ExpiresAt 60s}`. No whitelist filter — `backup` accepted. `ws_backup_wiring_test.go:48-62` verifies no rejection of `backup`.

**Tests:** `forge/api/internal/http/ws_backup_wiring_test.go:9-46`:
- `TestRealtimeProxySupportsBackupStream` checks `server.go` contains `/ws/backup` + `realtimeProxy(...,"backup")` and all 4 streams.
- `TestRealtimeProxyBackupPermissionCheck` checks `realtime.go` has `stream == "backup"` + `PermBackupRead` (and console remains).
- `TestWSTicketSupportsBackupStream` checks handler does not reject `"backup"`.
`go test ./forge/api/internal/http -run TestRealtimeProxy -v` → PASS.

---

## 4. Forge Web — `ServerStream` + `useBackupProgress` Live Rendering

**Type union** `forge/web/lib/api.ts:236`
```ts
export type ServerStream = "stats" | "logs" | "console" | "backup";
export function serverWebSocketURL(serverId:string, stream:ServerStream): string {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  const wsBase = API_BASE_URL.replace(/^https?:/, protocol);
  return `${wsBase}/servers/${encodeURIComponent(serverId)}/ws/${stream}`;
}
export async function connectServerWebSocket(serverId:string, stream:ServerStream): Promise<WebSocket> {
  const ticket = await fetchWSTicket(serverId, stream); // 923: fetchWSTicket(serverId, stream:string)
  return new WebSocket(serverWebSocketURL(serverId, stream) + `?token=${encodeURIComponent(ticket.token)}`);
}
```
`fetchWSTicket:922-931` `POST /servers/{id}/ws/ticket?stream=backup` returns `{token, expiresAt}`.

**Hook** `forge/web/components/server/backups-view.tsx:13-91`:
```ts
export type BackupProgress = { bytesProcessed:number; totalBytes:number; phase:string; };
export function useBackupProgress(serverId:string|undefined){
  const [progress,setProgress] = useState<BackupProgress|null>(null);
  const [connected,setConnected]=useState(false); const [error,setError]=useState<string|null>(null);
  useEffect(()=>{ if(!serverId) return; let cancelled=false, ws:WebSocket|null=null, timer:any;
    const connect = async()=>{ if(cancelled) return;
      ws = await connectServerWebSocket(serverId,"backup"); // 35
      setConnected(true); ws.onmessage = (event)=>{
        const raw = typeof event.data==="string" ? JSON.parse(event.data) : event.data;
        const payload = raw?.data ?? raw; // unwrap Event{topic,data}
        if(payload && typeof payload.bytesProcessed==="number")
          setProgress({bytesProcessed:payload.bytesProcessed, totalBytes:payload.totalBytes??0, phase:payload.phase??"unknown"});
        else if(payload?.phase) setProgress(prev=>({bytesProcessed:prev?.bytesProcessed??0, totalBytes:prev?.totalBytes??0, phase:payload.phase}));
      };
      ws.onclose=()=>{ if(cancelled) return; setConnected(false); timer=setTimeout(connect,3000); };
      ws.onerror=()=> setError("backup progress connection failed");
    }; void connect(); return ()=>{ cancelled=true; if(timer) clearTimeout(timer); try{ws?.close()}catch{} setConnected(false); };
  },[serverId]); return {progress, connected, error};
}
```

**Live rendering (not polling-only)** `forge/web/components/server/backups-view.tsx:108-183`:
- Polling still present for completeness: `backups:123-127` `refetchInterval: data?.some(status==="pending"||"running") ? 3000 : false` (3s when pending/running).
- Live layer `155` `const backupProgress = useBackupProgress(server?.id);` renders `174-183`:
  ```tsx
  {backupProgress.progress ? (
    <div className="mt-2 flex items-center gap-2 text-xs">
      <span className={backupProgress.connected ? "h-2 w-2 rounded-full bg-emerald-500 animate-pulse" : "h-2 w-2 rounded-full bg-slate-500"} />
      <span>Backup {phase}: {formatBackupBytes(bytesProcessed)} / {formatBackupBytes(totalBytes)} ({%})</span>
    </div>
  ) : null}
  {backupProgress.connected ? <span className="ml-2 text-emerald-300 text-xs">● live backup progress</span> : null}
  ```
Polling ensures eventual consistency; WS provides sub-second phase/bytes updates.

**Build:** `forge/web/lib/api.ts` typecheck passes with `"backup"` union (no string-literal error on `connectServerWebSocket(id,"backup")`).

---

## 5. End-to-End Live Path (Beacon eventBus → API WS → Web Hook)

```
Beacon backup.Create(ctx, serverRoot, serverID, name, ignored)
  ├─ reportProgress(0,0,"creating backup") → ProgressFunc(p)
  │    └─ SetProgressCallback closure: eventBus.Publish("backup progress:"+serverID, p)
  │         └─ events.Bus.Publish marshals Event{Topic:"backup progress:<id>", Data:BackupProgress} → chan []byte
  ├─ reportProgress(0,0,"archiving files")
  │    └─ same Publish
  ├─ … zip WalkDir … (per-file Write via zipWriter)
  └─ reportProgress(size,size,"completed") → Publish

Beacon backupProgressWS GET /servers/{id}/ws/backup
  └─ eventBus.Subscribe("backup progress:"+id) → receives Event JSON
       └─ webSocketWriter.Write(Event JSON) → websocket.TextMessage to upstream

Forge realtimeProxy GET /api/v1/servers/{id}/ws/backup?token=<WS-ticket>
  ├─ inspectWSTicket → stream=="backup" ok, UserCanAccessServer(backup.read) ok
  ├─ consumeWSTicket (single-use), ServerControlTarget → NodeURL/NodeToken
  ├─ WebSocketURL(NodeURL, id, "backup") → "wss://<beacon>/servers/<id>/ws/backup"
  ├─ SignedHeaders(nodeToken, GET, requestURI)+MintWebsocketToken(serverID,userID) → Bearer WS token
  ├─ gorilla.DialContext(upstreamURL, headers) → beacon backupProgressWS
  ├─ client.WriteJSON({status:"connected", stream:"backup"})
  ├─ pumpUpstreamToClient: upstream.ReadMessage() → client.WriteMessage() (verbatim Event JSON)
  └─ pumpClientToUpstream: rate-limited 10/s (no-op for backup, read-only)

Web useBackupProgress("backup")
  ├─ fetchWSTicket(id,"backup") → POST /servers/{id}/ws/ticket?stream=backup
  ├─ new WebSocket("/api/v1/servers/{id}/ws/backup?token=<ticket>")
  ├─ onmessage: JSON.parse(event.data) → raw = Event{topic,data:BackupProgress}
  ├─ payload = raw.data ?? raw → setProgress({bytesProcessed,totalBytes,phase})
  └─ BackupsView renders live bar + pulse + ● live
```

Control flow is push-based; polling (`refetchInterval:3000`) remains as fallback for `pending|running` list refresh but live bytes/phase come from WS.

---

## 6. Verification (Evidence)

```bash
# Beacon wiring test
go test ./beacon/internal/server -run TestBackupProgressEventViaSetProgressCallback -v
# → PASS (checks BackupProgressEvent, backupProgressWS, SetProgressCallback+Publish, route, LocalBackup impl)

# Forge wiring tests
go test ./forge/api/internal/http -run TestRealtimeProxy -v
# → TestRealtimeProxySupportsBackupStream PASS
# → TestRealtimeProxyBackupPermissionCheck PASS
# → TestWSTicketSupportsBackupStream PASS (static)

# Static grep (all points present)
grep -n 'BackupProgressEvent' beacon/internal/server/server.go          # 95,1596,2247
grep -n 'SetProgressCallback' beacon/internal/server/server.go           # 1595
grep -n 'eventBus.Publish(BackupProgressEvent' beacon/internal/server/server.go  # 1596
grep -n '/servers/{id}/ws/backup' beacon/internal/server/server.go      # 385
grep -n '/servers/:id/ws/backup' forge/api/internal/http/server.go      # 2069
grep -n 'realtimeProxy.*backup' forge/api/internal/http/server.go       # 2069
grep -n 'stream == "backup"' forge/api/internal/http/realtime.go        # 263
grep -n 'PermBackupRead' forge/api/internal/http/realtime.go            # 264
grep -n 'ServerStream' forge/web/lib/api.ts                               # 236
grep -n 'useBackupProgress' forge/web/components/server/backups-view.tsx # 20,155
```

No file needed ad-hoc `sed`; wiring already present. `go vet ./beacon/internal/server ./forge/api/internal/http` clean.

---

## 7. Gap Assessment & Action Taken

**Original gap (110-03-10):** `forge/api/internal/http/server.go:1995` only proxied `stats|logs|console`. `backup` was dead — beacon had handler but panel could not reach it; `ServerStream` union rejected `backup`; frontend polled only.

**Current state (Phase 05 re-verify):** All 4 layers closed:
- Beacon publish+subscribe verified (§2)
- Forge 4-route union + `PermBackupRead` verified (§3)
- Ticket handler pass-through verified (§3)
- Web union + `useBackupProgress` live bar verified (§4)
- E2E `Event` envelope unwrap (`raw.data ?? raw`) verified (§4)

**No new code written** — `git diff` null for this slice. If this audit had found the old gap, required edits would have been:
- `forge/api/internal/http/server.go:2036-2069` add `v1.Get("/servers/:id/ws/backup", realtimeProxy(...,"backup"))`
- `forge/api/internal/http/realtime.go:263-273` add `if stream=="backup"` `PermBackupRead` block
- `forge/web/lib/api.ts:236` add `"backup"` to `ServerStream`
- `forge/web/components/server/backups-view.tsx:13` add `useBackupProgress` + rendering (§4 snippet)

All are already present.

---

## 8. Edge Cases & Notes

- **Concurrent backups per daemon:** `createBackup` overwrites global `SetProgressCallback` closure per `serverID`. `backupMu` serializes creates, but overlapping creates for different `serverID` would race callback ownership. Acceptable for single-concurrent-backup daemon (typical); true per-namespace callback map would be `Create(ctx,...,ProgressFunc)` signature — left as documented limitation from `audits/110-phase-03-impl/subagent-10-backup-progress.md:248`.
- **Phase granularity:** `LocalBackup:374,405,544` currently reports `creating backup` → `archiving files` → `completed` (total bytes only at completion). Live bytes mid-archive would need per-file `reportProgress(bytesWritten, totalEstimate, "archiving")` inside `WalkDir` — not yet implemented, but channel supports it without frontend change (hook already handles `bytesProcessed`).
- **S3 staging:** `s3.go:127` `Create` stages locally then uploads; progress still via `local` callback, so `archiving` phase covers S3 path.
- **Auth:** Beacon accepts `ScopeWebsocket` or `ScopeBackupDownload` (`2211:2224`); Forge mints `ScopeWebsocket` via `daemon.MintWebsocketToken` — scope check passes. Cookie-auth WS still requires allowed Origin (`realtime.go:128-147`, `wsOriginMiddleware`).
- **Event buffering:** `events.Bus:55-68` drops oldest when channel full (32-cap, 10ms) — prevents `backupProgressWS` backpressure from blocking `Create`.

---

## 9. Files Referenced

- `beacon/internal/server/server.go:95,385,1593,2210,2247,3695,3700`
- `beacon/internal/backup/backup.go:33,67`
- `beacon/internal/backup/local.go:45,78,244`
- `beacon/internal/backup/s3.go:121`
- `beacon/internal/events/events.go:42,72`
- `beacon/internal/server/backup_progress_wiring_test.go:11`
- `forge/api/internal/http/server.go:2036-2079`
- `forge/api/internal/http/realtime.go:124,263,298,308`
- `forge/api/internal/http/handlers_ws_ticket.go:159`
- `forge/api/internal/http/ws_backup_wiring_test.go:9`
- `forge/api/internal/daemon/client.go:1408`
- `forge/api/internal/store/permissions.go:45`
- `forge/web/lib/api.ts:236,240,922`
- `forge/web/components/server/backups-view.tsx:13,20,108,155,174`
