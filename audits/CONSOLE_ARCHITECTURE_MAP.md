# Console — Internal Architecture Map (Audit Before Refactor)

## 1. Topology Verified (Frontend → Beacon)

```
Browser
  → forge/web/components/server/console-view.tsx (active, 315 lines, div log + WebSocketManager)
    → forge/web/lib/api.ts: fetchWSTicket POST /servers/:id/ws/ticket?stream=console
      → forge/api/internal/http/handlers_ws_ticket.go — HMAC-SHA256 signed, 60s, single-use, Redis/in-memory store, checkServerPermission(websocket.connect)
    → connectServerWebSocket → WebSocket GET /api/v1/servers/:id/ws/console?token=...
      → forge/api/internal/http/server.go v1.Get("/servers/:id/ws/console", requireRealtimeServices, fiberws realtimeProxy)
        → forge/api/internal/http/realtime.go realtimeProxy:
           inspectWSTicket (peek), validate session (parseToken + validateCurrentSession, userId must match ticket UserID), check UserCanAccessServer websocket.connect + control.console, consume ticket (single-use), resolve ServerControlTarget (store lookup NodeURL/Token), dial beacon via daemon.Client.WebSocketURL + SignedHeaders (HMAC timestamp/nonce/body), ping keepalive 30s, rate limit 10/s, bidirectional pump with configureClientSocket/configureUpstreamSocket (1MB limit, 60s deadline)
        → beacon/internal/server/server.go GET /servers/{id}/ws/console (Go http.ServeMux) → consoleWS handler
          → beacon/internal/server/console.go consoleManager: Ensure(serverID) → runtime.AttachConsole (docker attach), run() reads 32k buffer, replay 128 entries / 256kb, subscribe fanout bounded 64, Write appends \n and WriteString to session
          → beacon/internal/runtime/docker.go: AttachConsole → docker client ContainerAttach (modern-game-panel.server_id label)
  ← bytes → pumpUpstreamToClient → WebSocketManager onMessage → setLines
  → frontend input: form submit → manager.send(cmd) → pumpClientToUpstream rate limited → consoleManager.Write → io.WriteString(session)
```

Stats path parallel: second WebSocketManager to /ws/stats, same ticket flow, onMessage parses ApiStats, updates sparks.

## 2. Current Files

- `forge/web/components/server/console-view.tsx` — PRIMARY (used by /console/servers/[id]/page.tsx and /server/[id]/page.tsx). Div-based log (role=log, 50vh, font-mono 12px), filter search (destructive), autoScroll toggle, timestamps, CrashBanner, InstallBanner, TransferBanner, SuspendedBanner, Chart sparklines (CPU/memory/network+uptime), power buttons (start/restart/stop/kill), reinstall, history localStorage arrow nav.
- `forge/web/components/server/console.tsx` — DEAD (258 lines, not imported anywhere). Real xterm (Terminal, FitAddon, WebLinksAddon, SearchAddon, theme slate-950, font 13px, scrollback 1000, clipboard Ctrl+C, resize window listener). Manual reconnect timer exponential 1s*2^n capped 30s infinite, no maxRetries, direct `new WebSocket(url)` not via manager.
- `forge/web/lib/api/ws/websocket-manager.ts` — 236 lines, class WebSocketManager {status, retryCount, buffer, pingTimeout 60s warning, jitter backoff, maxRetries 10 default (console-view overrides 20), factory or url, onMessage JSON.parse fallback warn, onStatusChange, onError }. Methods: connect(), send() buffers when not OPEN, flushBuffer on open, disconnect() aborted=true, reconnect().
- `forge/web/lib/api.ts:905-923` — fetchWSTicket + connectServerWebSocket + serverWebSocketURL
- `forge/api/internal/http/handlers_ws_ticket.go` — ticket store, signTicket, verify
- `forge/api/internal/http/realtime.go:54-260` — realtimeProxy full chain, configure sockets
- `beacon/internal/server/console.go` — manager
- Layout: `server-console-layout.tsx` + `server-context.tsx` (permissions via lib/permissions can()), `server-nav.tsx` 18 tabs, `crash-banner.tsx`

## 3. What Already Works (Preserve)

- Ticket auth is sound: HMAC v1 with RawURLEncoding, 60s TTL, single-use consume via Redis Lua or in-memory, stream must match, ticket server must match param id, session token must be present (cookie or Bearer) and validated, identity must match ticket UserID, permission checks websocket.connect + control.console for console stream before consume.
- WebSocketManager reconnect with jitter, exponential backoff, buffer pending commands, initialConnection vs reconnecting status, aborted flag prevents reconnect after intentional disconnect, flushBuffer on open.
- History persistence (localStorage `console-history-${id}` 50 entries), arrow Up/Down navigation, dedup filter.
- Stats streaming separate WS, sparkline history 60 points, memoryPercent calc, uptime card.
- Power actions state-aware disabled logic, blocked when suspended/transferring/installing, permission check per signal.
- Banners correct: suspended, transferring (targetNodeId), installing, CrashBanner when not in those states.
- ServerContext permission model (isAdmin/isOwner bypass, else server.permissions).
- Beacon console replay (128 entries) gives new subscribers last output, bounded subscriber channel prevents slow client blocking runtime.

## 4. What Is Broken / Stubbed / Misleading

1. **No real terminal**: console-view uses `<div>` log, loses ANSI colors, cursor, terminal semantics. console.tsx HAS xterm but is dead. Result: Minecraft colored output, progress bars broken. Brief §8 requires xterm.
2. **Search destructive**: filter hides non-matching lines instead of highlight overlay via SearchAddon (§19). No next/prev, no count.
3. **No fullscreen (§20)**, no copy selection UX (§8), no scrollback config exposed, FitAddon not used in active path.
4. **Resize not propagated (§12)**: neither view sends terminal dimensions to beacon; FitAddon.fit() never called after mount in console-view, window resize not handled. Beacon console session is not resized (docker resize not wired).
5. **Connection state collapsed (§5)**: only "Connecting/Connected/Reconnecting/Connection error" + generic "Console disconnected". No distinction Beacon unavailable vs Server offline vs Auth failure vs Session expired vs Timeout. realtime.go returns JSON errors but frontend only does onError generic. WebSocket close codes not surfaced.
6. **WebSocketManager swallows plain output**: onMessage tries JSON.parse, warns on non-JSON slice 200 chars. Beacon console output is raw bytes (not JSON), so every message hits catch warn noise. Also JSON envelope {data?:string, error?:string} check is for daemon JSON but beacon sends raw.
7. **Proxy socket hack**: console-view creates `proxySocket = {send: manager.send, close: manager.disconnect, get readyState}` to fake WebSocket for cmdBuffer readyState check. Fragile, duplicates buffer (manager.buffer + cmdBuffer).
8. **Double buffering + stale pending**: cmdBuffer separate from manager.buffer, both flushed on connected but manager's buffer never cleared if send before open? Actually manager buffers too, so command sent twice risk on reconnect? Flow: user types while reconnecting → cmdBuffer.push → on connected flush manager.buffer + manually loop cmdBuffer and manager.send — if manager buffered during reconnect, duplicate not, but complexity risk duplication.
9. **Stats error clobbers console error**: statsManager onError sets same connectionError state as console. Stats failure overwrites console error.
10. **Infinite reconnect variations**: console-view 20 retries (~5min), console.tsx infinite (no cap). No "give up" UI after max.
11. **Server state desync (§13)**: console stays "Connected" even if server stopped elsewhere, beacon offline, or deployment started. No poll of server.status. refreshServer only on power success, not interval.
12. **Permission granularity**: canConsole gates entire connection; viewer should be able to observe but not send input (§15). Currently input disabled but connection still requires control.console — viewer with websocket.connect but not control.console cannot connect at all (realtime.go also requires both). Requirement ambiguous but viewer observe-only needs at least connect.
13. **Stale error after success**: connectionError not cleared on reconnecting, shows previous error alongside "Reconnecting".
14. **Duplicate console architecture**: two files, two WS lifecycles, two themes — violates §30 no duplicate.
15. **Mobile/narrow not tested**: 4-col controls grid collapses poorly, stats 4-col becomes scroll, header not compressing per §25.

## 5. What to Redesign (Keep Architecture)

- Single canonical console: refactor `console-view.tsx` IN PLACE to use xterm (import from console.tsx), keep its state machine, banners, stats, power logic, but replace div log with Terminal instance. Delete/archive console.tsx after.
- Layout per §3/§22 hierarchy: Header Identity (name, status pill derived from server.status/actualState + beacon name via node.fqdn, env label, runtime) → Connection state badge prominent → Primary actions row (Start/Stop/Restart state-aware) → Toolbar (Clear/Search/Copy/Reconnect/Fullscreen + connection status) → Terminal (flex-1) → Diagnostics footer (recent operation status/reconnect info).
- Visual tokens: use --canvas (#0a0e16), --surface (#111722), --line (rgba 148..), --text, --brand. Terminal theme maps to tokens, not hard hex slate-950.
- Failure UI per §16 distinct cards, not generic; Empty offline per §18 intentional stopped state with Start button.

## 6. Realtime Lifecycle Fixes (No Backend Change Needed Unless Resize)

- Extend WebSocketManager to expose `lastCloseReason` and `closeCode` to map to distinct states: map `onclose` event code 1008→Auth failure, 1011→Beacon unavailable, 1006→Beacon disconnect. For now frontend infers from error JSON payload received before close: realtime.go writes JSON errors before return without upgrade; those arrive as first message. Parse error string to set beaconUnavailable/serverOffline/sessionExpired.
- Add `onClose` callback to manager to surface code/message.
- Tune manager: keep maxRetries 20 but after exhaustion transition to "disconnected" with Retry button, not infinite. Ensure disconnect() truly stops retries (already does via aborted).
- Fix buffering: single buffer (manager's), remove cmdBuffer, send via manager.send directly.
- Fix JSON parse warn: try parse, if fails treat as raw string, no console.warn.
- Separate stats error state from console error (own state var).
- Add server polling: useEffect interval 10s fetchServer to detect status change → if server.status becomes stopped while connected, show "Server stopped — console preserved" and keep terminal visible.
- Add resize: add `terminal.onResize(({cols,rows})=> manager.send(JSON.stringify({action:"resize", cols, rows})))` ; beacon console.go currently ignores resize action (no handler) — log only, add TODO comment, make sending non-breaking (beacon will receive as input string but with JSON it will be written to console session as command; need to check if beacon filters resize). Safer: send as JSON and have beacon manager detect JSON with action field and not write to session. For now, implement frontend send and add beacon TODO; if beacon doesn't handle, it will echo JSON as command input which is undesirable. So gate resize behind feature flag `if (false)` or send binary? Better to inspect beacon server consoleWS handler to see if it already handles resize. Quick check: beacon server consoleWS reads from client and calls consoleManager.Write — it will write JSON string as command. That's wrong. So either add beacon handling or don't send yet. Decision: implement frontend resize with comment and send only if beacon capability advertised (check via fetch?). For now implement resize locally (fit on window resize/fullscreen) without sending to beacon; add beacon task to backlog.

## 7. API/Backend Changes Actually Necessary

- NONE for visual redesign. Ticket flow, realtimeProxy, consoleManager already correct.
- OPTIONAL backend enhancement for §12 resize: beacon/internal/server/server.go consoleWS to detect JSON {"action":"resize"} and call docker ContainerResize if runtime supports it. Not blocking — can ship frontend fit-only first.
- OPTIONAL: expose beacon name/env in server object already has nodeId → fetch Node for header. No new endpoint.

## 8. Implementation Sequence

1. Refactor console-view.tsx: add xterm imports, theme from tokens, init terminal in useEffect, wire manager onMessage → terminal.write, handle search via SearchAddon, add toolbar actions, fullscreen, copy, clear, resize observer, connection state finite machine with distinct banners, server polling.
2. Merge terminal theme & behavior from console.tsx, then delete console.tsx (or leave re-export to avoid import break).
3. Patch websocket-manager.ts: add close code handling, fix JSON warn, separate error states.
4. Verify: tsc, vitest, manual WS lifecycle tests (open/close/reconnect/auth fail/beacon offline).
