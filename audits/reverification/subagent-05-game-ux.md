# Reverification — Subagent 05: Game Hosting UX — Console Realtime Logs, Install/Transfer/Backup Progress, Resource Graphs, Server Cards, Notifications vs Pterodactyl

> Focus: Console realtime logs, install/transfer/backup progress, resource graphs, server cards, notifications
> Reference horizon: Pterodactyl Panel console UX (`pterodactyl-panel/resources/scripts/components/server/console/*`, `backups/*`, `BackupContainer.tsx:62`, `ServerDetailsBlock.tsx:54`, `StatGraphs.tsx:41`, `Console.tsx:58`)
> Forge layers: `forge/web/components/server/console-view.tsx:1`, `server-console-layout.tsx:20`, `backups-view.tsx:1`, `transfer-view.tsx:1`, `builds-view.tsx:1`, `app/servers/page.tsx:210 ResourceBar`, `components/ui/toast.tsx:1`, `DeploymentLogViewer.tsx:1`, `forge/web/lib/api/ws/websocket-manager.ts:109`, `forge/web/components/server/crash-banner.tsx:1`, `forge/web/lib/api.ts:931`, `forge/web/lib/api/servers.ts:431`
> Prior artefacts reconciled: `phase-02/subagent-04-game-ux.md` (18 comparisons, synthetic timestamps, cumulative network graph), `FINAL_PARITY_AUDIT.md §16 UX`, `final-parity/subagent-10-security-ux.md U01-U14`, `MASTER_FINDING_INDEX.md REF-GAME-F-G-17..21`
> Date: 2026-08-24
> Method: source inspection only, file:line verified, no product code modified. Re-inspected both trees at listed refs.

---

## 1. Scope & Reconciliation Mandate

This reverification re-inspects *only* the game-hosting operational UX surface — the moments when liveness, failure and progress must be truthful:

* **Live console** (ANSI, scroll, search, command history, connection lifecycle)
* **Install / Transfer / Backup** (binary state vs % progress, dual sources, polling vs WS)
* **Resource graphs** (CPU/Memory/Disk/Network, limits, delta vs cumulative, autoscale)
* **Server cards & lists** (status encoding, unavailable signals)
* **Notifications & banners** (toast duality, global banner vs header dot)

It reconciles three prior verdict layers:

| Layer | Claim to reconcile | Current check |
|-------|--------------------|---------------|
| `phase-02/subagent-04-game-ux.md` | 18 comparisons C01-C18, 5 logic findings LF-GAME-02..06, `PARTIAL` console, `BROKEN` network cumulative, `BROKEN` double JSON parse, `BROKEN` transfer dual-source | Re-read 18 vs Forge head; confirm still `PARTIAL`/`BROKEN` |
| `FINAL_PARITY_AUDIT.md §16 + §11 UX` | "synthetic timestamps & cumulative network graph, offline banners duplicated, empty CTAs dead, tone maps per-file, triple env editors" | §16 aggregates U03-U06; still open per file reads |
| `final-parity/subagent-10 U01-U14` | U01 nav IA, U02 dashboards, U03 app tabs not routed, U04 dual pollers, U05 per-file tones, U06 empty/offline, U07 logs placement (modal vs inline), U08 domains gated, U09 triple env editors, U10 tenancy display-only | U03, U04, U06 fleet-relevant to game console UX; reconcile |
| `MASTER_FINDING_INDEX.md REF-GAME-F-G-17..21` | F-G-17 double JSON parse `websocket-manager.ts:109` P1, F-G-18 cumulative network `console-view.tsx:203` P1, F-G-19 transfer dual source `transfer-view.tsx:77` P1 | Verify none FIXED since 2026-08-23 |

Result: **No P0 FIX landed on this UX surface since phase-02.** All three indexed BROKENs remain BROKEN; 13 of 18 prior PARTIALs remain PARTIAL; one COMPLETE (crash banner) regressed to PARTIAL via admin gating; one new nuance (busy scope) added.

---

## 2. Reference Platform — Canonical UX Inventory (file evidence)

### 2.1 Pterodactyl Console (`Console.tsx:58`) — fidelity benchmark

* Renderer `Console.tsx:58` `new Terminal({disableStdin:true,cursorStyle:'underline',fontSize:12,fontFamily:mono,rows:30,theme})` with 6 addons `128-133`: `FitAddon`, `SearchAddon`, `SearchBarAddon`, `WebLinksAddon`, `Unicode11Addon`, `ScrollDownHelperAddon`. `theme:23` 16 ANSI colors (`red '#E54B4B'` … `brightCyan '#89DDFF'`, `selection '#FAF089'`). `handleConsoleOutput:77` `terminal.writeln((prelude?TERMINAL_PRELUDE:'') + line.replace(/\r?\n$/,'') + '\u001b[0m')` where `TERMINAL_PRELUDE:56` `'\u001b[1m\u001b[33mcontainer@pterodactyl~ \u001b[0m'`. `handleDaemonErrorOutput:89` wraps in `'\u001b[1m\u001b[41m'` red bg. `handlePowerChangeEvent:94` writes `'Server marked as ' + state`.
* Hotkeys `144-156` `terminal.attachCustomKeyEventHandler` intercepts `Ctrl/Cmd+C` copy, `Ctrl/Cmd+F` `searchBar.show()`, `Escape` `searchBar.hidden()`. `useEventListener('resize', debounce(()=>fitAddon.fit(),100))`.
* Socket multiplex `170-189`: single `instance` listens 7 events `STATUS, CONSOLE_OUTPUT, INSTALL_OUTPUT, TRANSFER_LOGS, TRANSFER_STATUS, DAEMON_MESSAGE, DAEMON_ERROR`. On `connected && instance` `183` `terminal.clear()` *unless* `isTransferring`, then `addListener` per key + `instance.send(SocketRequest.SEND_LOGS)`. History `usePersistedState<string[]>(`${serverId}:command_history`,[])` capped `32:118`, `ArrowUp/Down 99-114`, `Enter 117` `instance.send('send command',command)`.
* Overlay `202` `<SpinnerOverlay visible={!connected} size='large'/>` covering terminal; input disabled `!instance || !connected:218`.
* Events catalog `events.ts:1` enumerates `DAEMON_MESSAGE, DAEMON_ERROR, INSTALL_OUTPUT, INSTALL_STARTED, INSTALL_COMPLETED, CONSOLE_OUTPUT, STATUS, STATS, TRANSFER_LOGS, TRANSFER_STATUS, BACKUP_COMPLETED, BACKUP_RESTORE_COMPLETED` and `SocketRequest SEND_LOGS,SEND_STATS,SET_STATE`.

### 2.2 ServerConsoleContainer (`ServerConsoleContainer.tsx:16`) — state machine HUD

* Reads `isInstalling, isTransferring, isNodeUnderMaintenance:16-22` → single `Alert warning mb-4` picking one message `26-34`. `isInstalling ? 'installation process…' : isTransferring ? 'being transferred…' : 'node under maintenance…'` — exclusive banner, power buttons gated via `<Can action={['control.start',...]} matchAny>`.

### 2.3 ServerDetailsBlock (`ServerDetailsBlock.tsx:54`) — live value/limit with threshold

* `textLimits:51` `cpu: limits?.cpu ? `${limits.cpu}%` : null`, `memory: bytesToString(mbToBytes(limits.memory))`. `getBackgroundColor:22` `value/max>0.9 red, >0.8 yellow`. 7 blocks `91-136`: `Address copyOnClick`, `Uptime color-coded`, `CPU Load`, `Memory`, `Disk`, `Network In`, `Network Out`. Each `Limit` `35` renders `value / limit|∞`. Offline guards `110-134` `status==='offline' → <span class='text-gray-400'>Offline</span>`. `useWebsocketEvent(STATS):73` parses JSON → `setStats({memory_bytes,cpu_absolute,disk_bytes,tx,rx,uptime})`.

### 2.4 StatGraphs (`StatGraphs.tsx:41`) — three-chart throughput benchmark

* `useChartTickLabel` for CPU (%,2 decimals) and Memory (MiB), `useChart('Network',{sets:2, y ticks bytesToString})` distinct colors `cyan.400/cyan.700` vs `yellow.400/yellow.700`. Delta: `previous:17` ref `{tx:-1,rx:-1}`, then `network.push([tx-prev.tx, rx-prev.rx].map(max0)):61-64`. `46` clears all three when `status==='offline'`. Reference truth: **throughput, not cumulative**.

### 2.5 BackupContainer (`BackupContainer.tsx:62`) — per-item WS + quota copy

* `backupLimit = featureLimits.backups:19`. Empty `40` checks `!backupLimit ? null : page>1 ? 'run out…' : 'no backups…'`. Limit-zero banner `57` `'Backups cannot be created because limit is set to 0.'`. Footer `64` `'{backupCount} of {backupLimit} have been created'`. `BackupRow:24` per-backup `useWebsocketEvent(`${BACKUP_COMPLETED}:${backup.uuid}`)` patches row via `mutate(items.map(...), false)`. Spinner `completedAt===null`, lock icon, failed red pill, checksum mono, distanceToNow, context menu only if `completedAt`.

---

## 3. Forge Re-inspection — Current Wiring (file evidence, unchanged vs phase-02)

### 3.1 ConsoleView (`forge/web/components/server/console-view.tsx:1`)

* Structure `14` `MAX_LINES=500 MAX_POINTS=60`. Banners `37-85` `InstallBanner` amber `Download` static 2 lines, `TransferBanner:52` sky `Upload animate-pulse` with `transferTargetNodeId + transferState` mono, `SuspendedBanner` rose. `Chart:91` single sparkline: `max=Math.max(...values,1)`, `points=values.map((p,i)=>`${(i/(len-1))*100},${100-(p/max)*92}`)`, renders `polygon fill rgba(220,38,38,.16)` + `polyline stroke #ef4444 width1.5` reused for CPU/Memory/Network (triple red).
* WS wiring `132-212` two `WebSocketManager` instances:
  * Console `146` `factory:()=>connectServerWebSocket(server.id,'console')` (`POST /servers/:id/ws/ticket?stream=console` → `wss://.../ws/console?token=` at `forge/web/lib/api.ts:931`), `maxRetries:20 baseDelay:1000 maxDelay:30000`. `onMessage:151` `let text=String(data); try JSON.parse → payload.data ?? payload.error ?? text` then `setLines(prev=>[...prev,...text.split('\n').filter(Boolean)].slice(-MAX_LINES))`. `onStatusChange:157` maps `connected/connecting/reconnecting/disconnected` → local `connection` state, flushes `cmdBuffer` on `connected`. `socketRef:182` fake WS proxy `readyState = manager.status==='connected'?OPEN:CLOSED`.
  * Stats `192` second manager `factory:()=>connectServerWebSocket(server.id,'stats')`. `onMessage:197` casts `data as ApiStats & {error?:string}`; `memory = memoryLimit>0?(memoryBytes/memoryLimit)*100:0`; `network = rx+tx` cumulative bug; pushes 3 histories.
* REST fallback `138` `fetchServerLogs(server.id).then(logs=>setLines(logs.split('\n').filter(Boolean).slice(-MAX_LINES)))`. On catch sets `connectionError` but no toast.
* Controls `243` 4 buttons `start|restart|stop|kill` grid-4, `canPower` per signal `control.start/restart/stop`, `blocked=suspended||transferring||installing` disables all. `kill` requires `confirm({Kill server?})`.
* Console chrome `267-312` header `PlugZap` emerald vs amber, `stateLabel` + `messageCount msgs` + `connectionError`, 5 icon toggles: search, autoScroll, showTimestamps, reconnect `setNonce(n+1)`, clear. Search `285` filters `lines.filter(lower includes query)` (filter, not find). Output `296` `role=log aria-live=polite h-[50vh] min-h-80 overflow-y-auto p-4 font-mono text-xs` mapping `filteredLines.map((line,i)=><div whitespace-pre-wrap break-words key=i-line>{showTimestamps?<span mr2 text-slate-500>{new Date().toLocaleTimeString()}</span>:null}{line}</div>)`; empty shows `searchQuery?'No matching':'Waiting…'||connectionError`. Command form `306` `disabled={connection!=='connected'||!canConsole}`, `historyKey` `localStorage console-history-${server.id}` cap 50, submit `232` via `socketRef.send` or `cmdBuffer.push`, Send button red disabled `!connected||!trim`.
* Crash banner `238` only if `!suspended&&!transferring&&status!=='installing'`; `crash-banner.tsx:13` delegates (see §3.5).

### 3.2 WebSocketManager (`forge/web/lib/api/ws/websocket-manager.ts:1`)

* `WebSocketConfig:8` `url|factory, onMessage,onStatusChange,onError, maxRetries,baseDelay,maxDelay`. `connect():73` sets `connecting|reconnecting`, creates WS via `factory()` or `new WebSocket(url)`, on catch `scheduleReconnect`. `onopen:97` reset `retryCount`, `connected`, `flushBuffer`. **`onmessage:104-113`** does `JSON.parse(event.data)` then `onMessage(JSON-parsed object)`; on catch `warn '[WebSocketManager] received non-JSON message'` and **never calls** `onMessage` — plain text dropped. `onclose:121` `setStatus(disconnected)` + `scheduleReconnect`. `scheduleReconnect:133` `status=reconnecting, delay=min(base*2^retryCount,maxDelay)+jitter`, `retryCount++`, `setTimeout(connect,delay)` capped `maxRetries`. No JWT/WS-ticket expiry hook; ticket fetched only at factory time, no `token expiring` fast refresh (cf. Ptero `WebsocketHandler:50`).

### 3.3 CrashBanner (`forge/web/components/server/crash-banner.tsx:1`)

* `useQuery(['crash-history',serverId], fetchServerCrashHistory, enabled: access.isAdmin, refetchInterval:30000):19`. `fetchServerCrashHistory: fetchServerCrashHistory` hits `GET /admin/crash-detection/servers/:id` (admin-only at `forge/web/lib/api/servers.ts:410`). `isLoading` → `Loading crash history…` banner, `isError` → red `Failed to load crash history Retry`, empty `!Array.isArray||length===0 → null`. Shows `crashed N times`, `Last crash: date · Exit code · OOM`, `<details>` 5 items, Restart (`sendPowerSignal start`) + Reset (`resetServerCrashState`) buttons. **Gate `access.isAdmin` means non-admin/owner subuser sees nothing even if crashed** — regresses from `phase-02/subagent-04` COMPLETE (now PARTIAL).

### 3.4 ServerConsoleLayout (`forge/web/components/server/server-console-layout.tsx:20`)

* `ServerConsoleLayout:20` reuses parent context if present else `ServerConsoleShell` fetches `[fetchServer, fetchCurrentUser]` parallel `49`, `permissions: []` if admin|owner else `permissions ?? null` (honors `*` → `["*"]`). Loading full-screen spinner, error red card `Unable to load server` + Try again. `activeTab` derived `pathname.split('/').at(-1)` with `"console"` fallback. `ServerNav` in flex aside + `max-w-7xl p-4`.

### 3.5 BackupsView (`forge/web/components/server/backups-view.tsx:1`)

* Query `43` `useQuery(['server-backups',id,page], fetchBackups(id,page,20), enabled !!id, refetchInterval: data?.some(pending|running)?3000:false)`. `invalidate` on mutation. Header `89` `backupLimit` check: `disabled` vs `'{total} of {limit} slots used.'` vs `'no quota was provided'` fallback (leaks impl). List `99-144` `isUsable=status==='completed'`, 4 buttons 11×11: Download (`!usable||!canDownload`), Lock/Unlock (`busy||!canDelete`), Restore amber (`!usable||busy||!canRestore`), Delete red (`!usable||busy||!canDelete||isLocked`). **`busy = restore|delete|lock|unlock isPending` `101` disables *all* rows globally**. Controls `146-181` note `Only completed backups…`, `Show/Hide advanced` panel with name, ignored comma-split, storage note `'Custom destinations (S3,GCS,Azure) not supported yet. Default node-local.'`, lock checkbox, Create disabled `!canCreate||disabled||pending||limitReached`. Pagination `159` `Page {page} of {total_pages} ({total} total)` with Previous/Next.

### 3.6 TransferView (`forge/web/components/server/transfer-view.tsx:1`)

* State `31-38` `transferStatusQuery useQuery(['server-transfer',id], fetchServerTransferStatus, retry:false, refetchInterval: transferring?5000:false, placeholderData:prev)` where `fetchServerTransferStatus:431` returns `{transferring,transferId,status,progress}|null` on 404. Also `server.transferring` boolean, `transferState`, `transferError`, `progress = transfer?.progress`. `isTransferring = server.transferring:79` drives `tone` `92` `isTransferring?ok:errorMessage?danger:transfer?.transferring?warning:neutral` — two sources. `StatusCard:91` 4 branches: `isTransferring→loader Transfer in progress + phaseLabel + progress bar h-2 emerald width clamp + error alert + Cancel`; `errorMessage→XCircle Transfer failed`; `transfer?.transferring→amber Transfer pending`; else `CheckCircle2 No active transfer`. `phaseLabel:82` `phase.replace(/_/g,' ').replace(/\b\w/=>upper)`. Initiate form `172` only if `!isTransferring && !transfer?.transferring && canTransfer (admin && settings.reinstall)` with targetNode select filtered `n.id!==nodeId`, allocation select filtered `!a.server`, warning if `targetAllocations.length===0`, Start disabled `!targetNodeId||!primaryAllocationId||pending`.

### 3.7 BuildsView (`forge/web/components/server/builds-view.tsx:1`)

* Query `42` `fetchServerBuilds` `refetchInterval pending|running?3000:false`, buildpacks `staleTime 60000`. **Build button `110` permanently `disabled` with `title='Buildpack builds are not implemented yet. Deploy via git or compose stack instead.'`** — dead affordance. Empty `198` `EmptyState Code2 'No builds yet. Deploy from git or compose...'`.

### 3.8 Servers Index (`forge/web/app/servers/page.tsx:1`)

* Status pill `20` `suspended→danger, transferring→pulse info, running→success, installing→pulse warning`. Card `210-258` `Link /server/:id` with absolute right `w-1` bar color `suspended:#f43f5e|running:#10b981|installing:#f59e0b|#64748b`, inner `h2 name`, `p description line-clamp-1`, `ServerStatus`, `node + allocation mono`, then **resource bars `250-258`**: `ResourceBar icon=Cpu/MemoryStick/HardDrive` *without* `current/limit` → hits `max===0` branch renders `<span text-[10px] text-slate-500>Unavailable</span>` + no bar. Hidden when `suspended||installing||transferring`. Comment `252` admits *intentionally unavailable to avoid misleading 100% bar* — honest but 3 identical gray rows per card for every running server.

### 3.9 DeploymentLogViewer (`forge/web/components/deployment/DeploymentLogViewer.tsx:1`)

* Props `deploymentId, wsUrl:string|()=>Promise<string>, initialLogs`. Strips ANSI `ANSI_PATTERN 19`, `getLevelColor error→red warn→amber info→blue debug→slate`, `MAX_RECONNECT_ATTEMPTS=20` independent from `WebSocketManager`. `useEffect:61` manual `new WebSocket(url)` with `onopen→connected true retry0`, `onmessage→ JSON.parse LogEntry else {timestamp:ISOString,message:event.data}` capped `slice(-999)`, `onclose→ disconnected delay min(1000*2^attempt,30000)→nonce+1` reconnect, `onerror→close()`. Toolbar `Terminal icon Deployment Logs`, `Live/Disconnected` pill, `{logs.length} lines`, Search/Pause/Clear. Output `h-96 overflow-y-auto font-mono text-xs` with lines `index+1, timestamp localTime, level pill, message stripAnsi`. **Not mounted from any game server page** — unwired primitive (see C13).

### 3.10 Notifications (`forge/web/components/ui/toast.tsx:1` + sonner)

* `ToastProvider:21` `toast({title,message,tone})→id` slicing to last 3, auto-dismiss `7000 error else 4500`, `loading` never auto-dismisses, region `aria-live=polite`. Used in transfers/databases/settings. Files uses `sonner toast.success/error` second system `files-view:126`. Inline alerts per view also show `ui-alert-error|warning` for `query.isError` and `mutation.error`. **No global WS banner** like Ptero `WebsocketHandler:110` red bar; console `connectionError` only inline `· text` in header `console-view.tsx:275`.

---

## 4. Reverification Parity Matrix (≥12 rows, file:line verifiable)

> STATUS: `COMPLETE | PARTIAL | BROKEN | MISSING | DUPLICATE | FALSE_COMPLETION`
> RECONCILIATION: how current head compares to prior verdict.
> SEVERITY: `P0 data-loss/security | P1 silent correctness | P2 operator-visible | P3 polish`

| # | Capability | Reference file:line | Forge file:line | Prior verdict (phase-02 / FINAL §16 / U) | Reverified STATUS | Gap (what lies / what leaks) | Evidence snippet |
|---|------------|---------------------|-----------------|------------------------------------------|-------------------|------------------------------|------------------|
| G-UX-01 | **ANSI fidelity & terminal** — colored logs, bg error, links, unicode, fit/search | `Console.tsx:58` xterm+6 addons, `theme:23` 16 colors, `handleDaemonErrorOutput:89` red bg, `WebLinksAddon`, `Unicode11Addon activeVersion 11` | `console-view.tsx:296` plain `div` `whitespace-pre-wrap break-words` no ANSI parse, raw `\u001b[...` leaks; `DeploymentLogViewer.tsx:19` *does* `stripAnsi` (disagreement) | C01 PARTIAL | **RECONFIRMED PARTIAL — no change** — FINAL §16 carries forward | Forge console downgrades from game-server expectation (Minecraft/Palworld ANSI-heavy). No palette, no WebLinks one-click IP:port, no unicode width. Strip vs leak inconsistency within Forge. | `console-view.tsx:299` `<div ...>{line}</div>` vs `DeploymentLogViewer.tsx:22 stripAnsi(line)` — same repo two policies |
| G-UX-02 | **WS reconnect vs JWT/ticket expiry** — fast refresh without blackout | `WebsocketHandler.tsx:50` `token expiring/expired → updateToken` fast-path; `SOCKET_CLOSE→setConnectionState(false)`, `SOCKET_CONNECT_ERROR→'Failed to connect…'` | `websocket-manager.ts:109` no JWT hook, ticket only at `factory` `api.ts:931` `POST /servers/:id/ws/ticket`; `console-view.tsx:179` generic `connectionError 'The console connection failed'` then 1-30s retry; `DeploymentLogViewer.tsx:35` duplicate loop 20 retries | C02 PARTIAL, F-G-17 BROKEN | **RECONFIRMED PARTIAL / BROKEN** — F-G-17 still BROKEN | Double-JSON bug masks reconnect (next row) and JWT fast-path missing → blackout until next ticket fetch vs Ptero seamless refresh. Duplicate loops will diverge. | `websocket-manager.ts:104-113` vs `WebsocketHandler.tsx:50` |
| G-UX-03 | **Double JSON parse drops plain console output** — plain daemon text lost | `Console.tsx:172` `CONSOLE_OUTPUT: handleConsoleOutput` receives raw string, no JSON gate | `websocket-manager.ts:109` `JSON.parse(event.data)` then warn-and-drop if non-JSON; `console-view.tsx:151` `String(data)` then `JSON.parse` again → `[object Object]` | C03 logic LF-GAME-03 P1, **MASTER F-G-17 P1** | **RECONFIRMED BROKEN — untouched** | `WebSocketManager` warn-only path drops every plain `CONSOLE_OUTPUT` (common for Wings raw line). JSON envelope path mis-stringifies to `[object Object]`. Console stalls "Waiting…" while WS connected. | `websocket-manager.ts:104-113` ```try{data=JSON.parse(event.data); onMessage(data)}catch{warn}``` vs `console-view.tsx:154` second parse |
| G-UX-04 | **Command history, persistence, hotkeys** | `Console.tsx:99` `history cap 32`, `ArrowUp min(idx+1,len-1)` + `preventDefault`, `ArrowDown max(idx-1,-1)`, `SearchBarAddon` zIndex fix, `ScrollDownHelperAddon` `FitAddon.fit()` debounced 100ms | `console-view.tsx:133-227` `localStorage console-history-${id}` cap 50, `historyKey:227` similar, `requestAnimationFrame scrollTo` `131`, 5 toolbar toggles but no xterm Search/Scroll addons, no `temporaryCommand` stash, no `Ctrl+F/Escape` hotkeys, offline `cmdBuffer:132` never reachable because `disabled={connection!=='connected'}` | C03 PARTIAL | **RECONFIRMED PARTIAL** — U not applicable | History cap drift 50 vs 32; missing `temporaryCommand` (Puffer `Console.vue:99`) loses draft on Up→Down; `cmdBuffer` dead code (disabled input prevents queuing); no resize FitAddon. | `console-view.tsx:214` cap 50 vs `Console.tsx:118` cap 32; `console-view.tsx:309` `disabled` blocks `cmdBuffer` at `226` |
| G-UX-05 | **Search: filter vs find + highlights** | `Console.tsx:128` `SearchAddon + SearchBarAddon` highlights + scroll within scrollback; hotkey `Ctrl+F` `144` | `console-view.tsx:278-285` search toggle filters `lines.filter(lower includes query)` removing non-matching lines; no highlight, no in-viewport jump; `DeploymentLogViewer.tsx:49` also filters list | C03 PARTIAL P3, FINAL §16 carries | **RECONFIRMED PARTIAL** | Filter destroys context around match (operator loses preceding traceback). Ptero highlight preserves. | `console-view.tsx:222` `filteredLines=lines.filter(...)` vs `Console.tsx:60-61` `searchAddon/searchBar` |
| G-UX-06 | **Synthetic per-line timestamps** — render-time wall clock | No synthetic — Ptero never fakes; Puffer `Console.vue:42` uses `epoch` payload | `console-view.tsx:299` `{showTimestamps?<span>{new Date().toLocaleTimeString()}</span>:null}` called per line on every render | C03 P3, LF-GAME-02 P2, FINAL §16 explicit | **RECONFIRMED BROKEN — logic bug** | All 500 lines share identical wall time (render moment); toggling or stats WS re-render changes every timestamp simultaneously; post-mortem correlation broken. `DeploymentLogViewer:208` correctly uses `new Date(log.timestamp).toLocaleTimeString()` per stored entry — inconsistency proves bug. | `console-view.tsx:299` `new Date().toLocaleTimeString()` inside `filteredLines.map` — should be stored `at:Date.now()` per line |
| G-UX-07 | **Resource graphs — signals, axes, palette, disk** | `ServerDetailsBlock.tsx:54` 7 signals with `Limit value/limit` + `getBackgroundColor >0.8 yellow >0.9 red`; `StatGraphs.tsx:41` 3 charts CPU/Memory/Network(2 sets cyan/yellow) with `bytesToString` ticks | `console-view.tsx:91` single `Chart` helper triple red `stroke #ef4444 fill rgba(220,38,38,.16)` for all 3 signals; `Chart detail` CPU omits limit, Memory shows `bytes of limit`, Network sum `RX+TX` bytes; Disk absent | C07 PARTIAL P2, U02 COMPLETE but admin split | **RECONFIRMED PARTIAL** — no fix | Missing Disk (capacity-threatening), CPU no denominator, triple red colorblind-hostile, no `getBackgroundColor` thresholds. | `console-view.tsx:92-94` hard-coded red vs `StatGraphs.tsx:34-39` cyan/yellow per series |
| G-UX-08 | **Network delta vs cumulative (graph lie)** | `StatGraphs.tsx:61` `network.push([max(0,tx-prev.tx), max(0,rx-prev.rx)])` + `previous:17` ref `{tx:-1,rx:-1}` | `console-view.tsx:203` `const network = rx+tx; setNetworkHistory([...slice(-59), network])` no previous ref, absolute cumulative monontonic | C08 **BROKEN P1**, LF-GAME-04 P1, **MASTER F-G-18 P1**, FINAL §16 explicit | **RECONFIRMED BROKEN — untouched** | After 10min `rx+tx ~3GB`, 60 values `~2.9–3GB` normalized `max=ever-growing sum` → sparkline flat at top, bursts <0.3% pixels, operator reads idle as ceiling (inverted). | `console-view.tsx:203` vs `StatGraphs.tsx:61` quoted |
| G-UX-09 | **Sparkline auto-max exaggeration** — flat 2% ≡ 40% | Puffer `Stats.vue:97` `suggestedMax 100` + `x.min=now-60s`; Ptero axis via `useChartTickLabel` with limit | `console-view.tsx:92` `max=Math.max(...values,1)` then `y=100-(p/max)*92` auto-scales to observed max | C08 sub-finding LF-GAME-06 P2 | **RECONFIRMED BROKEN** | Flat 2% idle and flat 40% loaded look identical (both fill top). CPU should anchor to `limit||100`, Memory to `memoryLimit|| max*1.2`. | `console-view.tsx:92` `Math.max(...values,1)` — no limit anchor |
| G-UX-10 | **Server cards — status encoding & triple unavailable** | Ptero admin `AdminServers.tsx:88` pills only, no bars; Puffer list no bars | `app/servers/page.tsx:210-258` right `w-1` bar `suspended:#f43f5e|running:#10b981|installing:#f59e0b` + `ServerStatus` pill + 3 `ResourceBar` without values → `Unavailable` per row, hidden when `suspended||installing||transferring` | C09 PARTIAL P3 | **RECONFIRMED PARTIAL** — comment at `252` admits intentional | Triple "Unavailable" per card for majority of running servers is visual noise; operator learns to ignore area. Vertical bar repeats pill without `aria-label`. | `app/servers/page.tsx:216` `backgroundColor: server.suspended?...` + `255` `<ResourceBar icon={Cpu} />` → `primitives:152` `Unavailable` |
| G-UX-11 | **Install progress — banners, output, state machine** | `ServerConsoleContainer.tsx:19` single `Alert warning` picking one; `InstallListener.tsx:12` `INSTALL_COMPLETED→getServer`, `INSTALL_STARTED→status:'installing'`; output via `INSTALL_OUTPUT` into console | `console-view.tsx:37-50` `InstallBanner` amber static 2 lines only when `status==='installing' && !suspended&&!transferring:236`; `canPower blocked` when installing; no WS `INSTALL_*` listener; `AdminServers 335` pill `Installing`; `builds-view.tsx:110` Build button permanently disabled (decorative) | C04 PARTIAL P2 | **RECONFIRMED PARTIAL** | No WS-driven transition if install triggered externally/admin → banner lags until manual `refreshServer()` (only on power/reinstall success). No completion auto-reload, no spinner/elapsed, no %/step. CrashBanner suppressed correctly during install, but no heartbeat check if install daemon died. | `console-view.tsx:236` banner guard vs `InstallListener.tsx:20` WS push |
| G-UX-12 | **Transfer progress — handoff, % progress, allocation guidance, dual truth** | `TransferListener:11` `pending|processing→true, failed→false, completed→getServer`; `WebsocketHandler:65` `transfer status starting|success → socket.close(); setInstance(null); connect(uuid)` target handoff; `Console:80` failure writes `'Transfer has failed.'` into terminal | `transfer-view.tsx:31` polls `GET /servers/:id/transfer` 5s if `transferring`; `StatusCard 92` `tone=isTransferring?ok:errorMessage?danger:transfer?.transferring?warning:neutral` — two truths; `phaseLabel 82`, `progress bar 108` if `progress!==null` (shim `ApiLegacyTransferStatus` may never populate); `TransferBanner:52` shows `Target node: id · state`; allocation picker `196` filters `!a.server` | C05 PARTIAL P1/P2, **MASTER F-G-19 P1**, U not directly | **RECONFIRMED PARTIAL / BROKEN dual-source** — untouched | Two independent caches disagree ~5s after completion → flickers `Transfer in progress`↔`No active transfer`; form `!isTransferring&&!transfer?.transferring` may appear prematurely → duplicate `POST /transfer` 409. No WS handoff (`WebsocketHandler:65` absent) → console blacks out after target switch. `progress` often null → bar silently absent despite UI reserving space. | `transfer-view.tsx:79` `isTransferring=server.transferring` vs `92` `transfer?.transferring` — logical split; no `socket.close()+connect(target)` |
| G-UX-13 | **Backup progress — polling vs per-item WS, per-row busy, limits copy** | `BackupRow:24` per-backup `BACKUP_COMPLETED:${uuid}` patches row via `mutate(data=>items.map(...isSuccessful,checksum,bytes,completedAt:new Date()))`; `BackupContainer:40/57/64` limit-aware empty + `'limit 0'` banner + `'{count} of {limit}'` | `backups-view.tsx:43` polls whole list 3s while pending/running + `invalidateQueries` on mutation; `101` `busy = restore|delete|lock|unlock isPending` disables *all* rows globally; `89` header `limit===0` vs `no quota was provided` fallback; `pagination jump to page 1` on create loses context; no per-row WS `BACKUP_COMPLETED:uuid` | C06 PARTIAL P3, LF-GAME-03 (busy scope), FINAL §16 duplicate offline but also polling | **RECONFIRMED PARTIAL** | Polling O(n) invalidation vs targeted patch; coarse global busy (delete on row A disables lock on row B); pagination jump; checksum appears only after `completedAt` but no WS hydration; `no quota` leaks impl detail. | `backups-view.tsx:101` `busy` global vs `BackupRow:24` per-item; `62` `setCurrentPage(1)` jump vs Ptero never jumps |
| G-UX-14 | **Notifications & operational feedback — toast duality, inline, global banner** | `BackupContainer:6` `useFlash`/`FlashMessageRender byKey='backups'` per-key inline; `WebsocketHandler:110` global `bg-red-500 py-2` bar with spinner or error text — persistent not toast; Puffer `toast.success('BackupStarted')` transient | `toast.tsx:21` `ToastProvider useToast` queue 3 `4500/7000`, `sonner toast.success/error` in `files-view:126` second lib, plus per-view `ui-alert-error|warning` for `query.isError`/`mutation.error`, `connectionError` only inline `· text` at `console-view.tsx:275` not toast/banner | C11 PARTIAL P3, U06 fleet UX (offline/rate-limit dead) | **RECONFIRMED PARTIAL** | Two toast systems same outcome different animation/duration; `actionError` collapses concurrent errors to first truthy (`backups-view:74`); console disconnect signal lost when switching tab (no global overlay — Ptero covers terminal with `SpinnerOverlay visible={!connected}`). | `toast.tsx:21` vs `files-view:12` `sonner` — same concept two libs; `console-view.tsx:275` header-only vs `Console.tsx:203` `SpinnerOverlay` |
| G-UX-15 | **Empty states & file manager caps** | `BackupContainer:40` `!backupLimit?null:page>1?'run out…':'no backups…'`; `FileManagerContainer:93` `'This directory seems to be empty.'` + `97` `>250` yellow banner `'too large to display, limiting to first 250'` | `primitives EmptyState` title/description/action `ui-empty`; backups empty `98` neutral generic (limit hint only in header pill); builds empty `198` good but Build button permanently disabled above contradicts guidance; deployments empty `deployments-view:369` good; files empty `205` good but **no `>250` cap banner** → large dirs render all entries after sort, risk DOM blow-up; triple empty icons `Code2/RotateCcw` confusing build vs deployment | C10 PARTIAL P3, U06 per-resource empty | **RECONFIRMED PARTIAL** | Backups empty should tailor `backupLimit===0`→`Ask admin to raise limit`; files missing 250 cap warning; builds dead CTA contradicts empty promise. | `backups-view.tsx:98` generic vs `BackupContainer:40` per-state; `files-view` no 250 cap (Ptero `FileManagerContainer:97` has) |
| G-UX-16 | **Crash & node-maintenance feedback loop** | `ServerConsoleContainer:26` one `Alert warning` exclusive for `isNodeUnderMaintenance||isInstalling||isTransferring`; Wings status via `STATUS` WS into terminal | `console-view.tsx:238` `CrashBanner` suppressed if any of `suspended|transferring|installing`, else `crash-banner.tsx:19` `useQuery` admin-only `refetch 30s` showing `crashed N times, Last crash: date · Exit code · OOM`, `<details>` 5, Restart+Reset | C16 **regressed** — phase-02 COMPLETE → now PARTIAL | **RECONFIRMED PARTIAL — new gate** | `CrashBanner` now `enabled: access.isAdmin:22` → non-admin subuser (owner with `control.console` only) gets 403 swallowing → banner hidden even if crashed; Ptero `STATUS` broadcast is role-agnostic. Maintenance flag `isNodeUnderMaintenance` (Ptero banner) has no Forge equivalent — `console-view` only checks `suspended|transferring|installing`, not maintenance → power signals sent to maintenance node get generic error not banner. | `crash-banner.tsx:22` `access.isAdmin` gate vs `ServerConsoleContainer:28` `isNodeUnderMaintenance` |
| G-UX-17 | **Status + power controls affordance** | `PowerButtons:72` 3 buttons `Start flex-1 disabled offline, Restart disabled !status, Stop/Kill toggle Danger` text switches `killable=status==='stopping'`; confirm only for Kill via `Dialog.Confirm hideCloseIcon` | `console-view:243` 4 concurrent `start|restart|stop|kill` `grid-cols-4` `min-h-11` `start emerald restart slate stop|kill red`; `kill` requires `confirm({Kill server?})`; disable `!canPower||blocked||isPending||(start?running:!running)`; Kill always separate, not conditional on `stopping` | C17 PARTIAL P3 | **RECONFIRMED PARTIAL** | Always-visible Kill invites accidental destruction; missing `isNodeUnderMaintenance` guard; no `Restart disabled when status unknown` nuance; 4 buttons waste space vs Ptero 3 with toggle. | `console-view.tsx:243` `controls=['start','restart','stop','kill']` vs `PowerButtons:72` 3 with toggle |
| G-UX-18 | **Permissions gating & graceful degradation** | `Console:66` `usePermissions(['control.console'])` hides input entirely if false `211`; `PowerButtons:51` `<Can action='control.start'>` per button; `Files:76` `<Can action='file.create'>` hides Upload | `server-nav.tsx:48` `visibleTabs = tabs.filter(hasServerPermission(...))` removes tab if lacking `file.read` etc; but per-action buttons use `disabled` not removal: backups `Create Backup disabled` still renders gray; transfer `canTransfer` hides form `172`; files `canRead` early `ShieldX You do not have permission` amber `194`; console `canConsole:124` requires both `websocket.connect+control.console` but `ServerNav console:17` requires `websocket.connect` alone → nav shows Console tab even when sending blocked → disabled input with generic `Console is not connected` title not `need control.console` | C15 PARTIAL P3, U03 app tabs not routed (similar pattern) | **RECONFIRMED PARTIAL** | Inconsistent hide vs disable vs warn across features; `settings.reinstall` reused for Transfer/Deployments/Builds (conflated scope — Ptero transfer not tied to reinstall permission); console nav vs view permission split confuses subuser (sees tab, can't type, no hint). | `server-nav.tsx:17` `websocket.connect` vs `console-view.tsx:124` `websocket.connect+control.console`; `transfer-view.tsx:24` `settings.reinstall` for transfer |
| G-UX-19 | **Deployment/compose logs WS primitive — unwired game console** | No pterodactyl deploy log; Coolify/Dokploy inline deploy logs under timeline with `stuckDeployment >9min` detection | `DeploymentLogViewer.tsx:37` provider: `stripAnsi`, `getLevelColor`, `MAX_RECONNECT 20` bespoke, `search/filter`, line numbers, timestamps, Live/Disconnected pill; **not mounted from any `/server/:id/*` page** — game deployments use polling `Events max-h-48` not WS viewer; install logs funnel into generic console fallback (C04) | C13 PARTIAL unwired | **RECONFIRMED DUPLICATE/UNWIRED** — same in FINAL §16 | Most advanced WS log primitive in Forge is unreachable from server console/deployments tabs where install progress needs it. Duplicate reconnect logic with `WebSocketManager`. | `DeploymentLogViewer.tsx:35` duplicate vs `websocket-manager.ts:1`; no import in `deployments-view` or `console-view` |
| G-UX-20 | **ServerConsoleLayout & console routing vs app/detail tab routing** | Ptero no web route tabs; Dokploy/Coolify path-based `environment/:id/...` deep-linkable | `server-console-layout.tsx:83` `activeTab = pathname.split('/').at(-1)==serverId ? 'console' : lastSegment` fragile; `admin-shell` route-based (U03) critique of `apps/[id]/page.tsx:28 useState tab` (refresh loses tab) does not yet apply to game console because `server-console-layout` *is* path-based (good) — but game `deployments-view` detail tabs remain `useState` not routed, matching U03 defect | U03 BROKEN (app), phase-02 layout PARTIAL | **RECONFIRMED PARTIAL** — game layout better than app UX | Game console routing correctly path-based via `ServerNav` + `pathname` (cf. U03 fix expectation); app detail `useState` defect not present here, but deployment detail `selectRelease` state inside `deployments-view` is still `useState` (no deep link to `?release=`). | `server-console-layout.tsx:83` path-based good vs `apps/[id]/page.tsx:28` `useState` bad |

**Count: 20 rows (>12 required), each `file:line` verifiable; 3 BROKEN logic carried from MASTER, 12+ PARTIAL retained, 1 COMPLETE regressed, 1 DUPLICATE/UNWIRED.**

---

## 5. Logic Findings — Reverification (≥3, file:line, reproducible)

### LF-GAME-REV-01 — REF-GAME-F-G-17: WebSocketManager double-JSON parse drops plain daemon output [BROKEN, P1 SILENT, RECONFIRMED]

* **Location:** `forge/web/lib/api/ws/websocket-manager.ts:109-113` vs `forge/web/components/server/console-view.tsx:151-155`
* **Expected (per `Console.tsx:172`):** Every `event.data` string (JSON envelope or plain `hello world\n`) reaches `onMessage` and ultimately `terminal.writeln`.
* **Actual:** `WebSocketManager:104` `try { const data = JSON.parse(event.data); onMessage(data) } catch { warn('[WebSocketManager] received non-JSON…') }` — non-JSON daemon text **dropped** (`onMessage` never called). If daemon sends JSON envelope `{"data":"hello"}`, manager forwards *object*, `console-view:153` `String(data) → "[object Object]"` then `JSON.parse("[object Object]")` throws → `text.split('\n') → ["[object Object]"]` junk. Stats path `197` casts `data as ApiStats` and thus also receives mangled object stringified vs real payload depending on daemon branch.
* **Impact:** `P1 SILENT` — console appears disconnected ("Waiting for console output…") while WS `connected` dot is emerald; operator retries power/deploy, duplicates load. Intermittent — depends on daemon's `CONSOLE_OUTPUT` envelope choice.
* **Reproduction:** Mock WS to send `'hello world\n'` (plain, no JSON) → console `lines` unchanged (warn only). Mock `'{"data":"hello world"}'` → line is `[object Object]`.
* **Re-inspection proof:** `websocket-manager.ts:109` still wraps in `try JSON.parse` + catch warn; `console-view.tsx:154` still second-parses `String(data)` — identical to `phase-02 LF-GAME-03` report. No commit since changed this path (verified via re-read head).
* **Fix (from phase-02 §10):** Remove JSON parse in `WebSocketManager`; forward `event.data` string directly: `onMessage(event.data)`. Parse only where payload is known JSON (stats handler: `JSON.parse` once, console handler leniently try parse envelope else plain).

### LF-GAME-REV-02 — REF-GAME-F-G-18: Network sparkline cumulative vs delta (graph lies) [BROKEN, P1 SILENT, RECONFIRMED]

* **Location:** `forge/web/components/server/console-view.tsx:203-206` vs `reference/.../StatGraphs.tsx:61-66`
* **Expected:** `StatGraphs:61` `network.push([ Math.max(0, tx - prev.tx), Math.max(0, rx - prev.rx) ])`, `previous:17` ref initialized `-1`, clears on `offline:46`. PufferPanel `Stats.vue:66` homologous absolute push + shift loop—network delta intentional.
* **Actual Forge:** `203` `const network = statsData.networkRxBytes + statsData.networkTxBytes;` `setNetworkHistory(prev=>[...slice(-59), network])` — **monotonic cumulative sum**. `Chart:92` normalizes `max=Math.max(...values,1)` → after 60 ticks all variation `< (Δ/max)*92 < 0.5%` pixels, line sits at top, bursts invisible. `detail` text at `249` still shows absolute `RX · TX` but graph contradicts it.
* **Impact:** `P1 SILENT` — misleading operational graph. Operator concludes "network idle" when bursty, or "at ceiling" when idle — opposite. Worse than no graph (false confidence).
* **Quant:** After 10 min `rx+tx ~3GB`, 60 values `~2.9–3.0GB`; delta variation `~1MB` → height delta `~0.03px` (rendered flat). Reference delta for same window would show `0–50MB/s` spikes clearly.
* **Master reconciliation:** `MASTER_FINDING_INDEX.md:53` `REF-GAME-F-G-18 P1 BROKEN — REMEDIATION_STATUS` `—` (open). Final parity §16 still lists "cumulative network graph" as open UX defect. No fix commit observed (re-read `console-view.tsx:203` matches audit snapshot).
* **Fix:** Keep `prevRx/Tx` ref (`useRef({rx:-1,tx:-1})`), push `max(0,rx-prev.rx)+max(0,tx-prev.tx)` throughput (or two lines In/Out with distinct colors like ptero cyan/yellow), store absolutes only in detail.

### LF-GAME-REV-03 — REF-GAME-F-G-19: Transfer dual-source race flicker (canonical `isTransferring` split) [BROKEN, P1 SILENT, RECONFIRMED]

* **Location:** `forge/web/components/server/transfer-view.tsx:79` `isTransferring = server.transferring` vs `92` `tone=isTransferring?ok:errorMessage?danger:transfer?.transferring?warning:neutral` + `88` `isTransferring ? Transfer in progress ... : errorMessage ? Failed : transfer?.transferring ? Transfer pending ... : No active transfer`; `79-82` `progress = transfer?.progress`; `174` form gate `!isTransferring && !transfer?.transferring`.
* **Expected:** Single authoritative `transferring` flag (ptero mutates `server.data.isTransferring` directly via WS `TRANSFER_STATUS` event).
* **Actual:** Two independent caches: `server.transferring` (from `GET /servers/:id`, stale until `invalidateQueries(["server",id])` after `transfer` query success) vs `transfer?.transferring` (from `GET /servers/:id/transfer` poll every 5s while `server.transferring` true). Window after backend marks `transfer.transferring→false` but before next `fetchServer` invalidation → UI flips between `Transfer in progress` (emerald) and `Transfer pending` (amber) / `No active transfer` (slate) on staggered polls. Form `!isTransferring && !transfer?.transferring` may become true prematurely → user re-initiates duplicate transfer → `409` or silent no-op; conversely `progress` bar sourced from `transfer` may show while `server.transferring` still true but `errorMessage` from `server` overrides tone incorrectly.
* **Impact:** `P1 SILENT` — duplicate transfer race + progress misreading + premature allocation release.
* **Reproduction:** Complete transfer on backend; `transferStatusQuery` next poll (T+0) returns `transferring:false`; `server` still `transferring:true` for up to 30s (servers index poll rate) or until `startMut/cancelMut` invalidation fires → `92` evaluates `isTransferring true → ok` while next render `transfer?.transferring false` branch would have shown `warning` — flicker observable in React strict mode double-fetch.
* **Fix:** Derive `isTransferring = (transferStatusQuery.dataUpdatedAt ? transfer.transferring : undefined) ?? server.transferring`; invalidate `["server",id]` *synchronously* on `transferStatusQuery` data change when `transfer.transferring !== server.transferring`; unify `progress`/`phase`/`errorMessage` from one source or merge with precedence `transfer ?? server`.

### LF-GAME-REV-04 — Synthetic per-line timestamps (forensics broken) [BROKEN, P2 SILENT, RECONFIRMED]

* **Location:** `forge/web/components/server/console-view.tsx:299` `showTimestamps ? <span className="mr-2 text-slate-500">{new Date().toLocaleTimeString()}</span> : null` inside `filteredLines.map`.
* **Expected:** Each line shows server emission time or stable `receivedAt` (like `DeploymentLogViewer:208` `new Date(log.timestamp).toLocaleTimeString()` per stored entry with `timestamp: new Date().toISOString():85` fallback).
* **Actual:** `new Date().toLocaleTimeString()` called at *render* for every line on every render (stats WS triggers re-render every `fetch` tick). All 500 lines show identical wall time (render moment). Toggling `showTimestamps` or receiving a stats packet rewrites every timestamp simultaneously — operator cannot correlate crash with earlier log.
* **Impact:** `P2 SILENT` — post-mortem forensics broken; crash banner `crash-banner.tsx:57` shows correct `new Date(lastCrash.createdAt)` but console beside it misleads.
* **Reproduction:** Emit two logs 10s apart, enable timestamps — both show same seconds. Wait 5s, timestamps update to new wall time without new logs (re-render from stats WS).
* **Fix:** Remove toggle or store `{text, at: Date.now()}` per line at `onMessage` (`setLines(prev=>[...prev,{text,at:Date.now()}].slice(-MAX_LINES))`) then render `new Date(entry.at).toLocaleTimeString()` stable; or parse daemon-prefixed timestamp if `2026-08-23 14:00:00` present.

### LF-GAME-REV-05 — Sparkline auto-max scaling exaggeration (severity indistinguishable) [BROKEN, P2 SILENT, RECONFIRMED]

* **Location:** `forge/web/components/server/console-view.tsx:92-93` `max=Math.max(...values,1)` then `y=100-(p/max)*92`.
* **Expected:** CPU 5% sits near bottom of 0-100% axis; memory 200MiB/2GiB ~10% near bottom; Network ticks relative to interface capacity or 100MB.
* **Actual:** `max` equals largest observed value in last 60 ticks → plateau at 5% renders full-height near top identical to plateau at 80% — severity indistinguishable. Triple red compounds confusion (CPU vs memory vs network visually identical).
* **Impact:** `P2 SILENT` — operator misses CPU saturation because "all sparklines look hot" (red peak at any level).
* **Fix:** CPU `max=max(limitOr100||100, maxObserved)`, Memory `max=max(memoryLimit|| maxObserved*1.2, maxObserved)`, Network `max=suggestedMax|| maxObserved*1.5`; distinct palette (emerald/amber/cyan) like `StatGraphs:38-39`.

*(Extra carry from phase-02 LF-GAME-05 busy-scope)*

### LF-GAME-REV-06 — Backup global busy disables all rows (coarse lock) [PARTIAL, P3 USER_VISIBLE, RECONFIRMED]

* **Location:** `forge/web/components/server/backups-view.tsx:101` `const busy = restoreMutation.isPending || deleteMutation.isPending || lockBackupMutation.isPending || unlockBackupMutation.isPending;` applied per-row `disabled={busy || ...}:116`.
* **Expected:** Like `BackupRow:24` per-item patch, pending state per `backup.uuid`.
* **Actual:** Deleting backup A sets `busy=true` → Lock on unrelated backup B (already pending lane) becomes disabled incorrectly; parallel restore + delete serialized via UI not API.
* **Impact:** `P3` — operator cannot lock other backup while one restore runs, even though API allows.
* **Fix:** `const pendingId = useRef<string|null>(null)` or per-mutation `variables` map; `disabled={restoreMutation.isPending && restoreMutation.variables?.name===backup.name}` etc.

---

## 6. Duplicates & Dead Code (carried from phase-02 §6, reverified)

| ID | Duplicate | Files | Status |
|----|-----------|-------|--------|
| DUP-GAME-01 | Two WS reconnect loops (jittered backoff `20 retries`) | `websocket-manager.ts:42-151` vs `DeploymentLogViewer.tsx:35-98` `MAX_RECONNECT_ATTEMPTS=20 1s*2^attempt cap30s` | RECONFIRMED — still two impls, will diverge on tuning |
| DUP-GAME-02 | Two toast systems | `components/ui/toast.tsx:21` `useToast` vs `sonner toast` in `files-view` + `files-view` already mixes inline `ui-alert` | RECONFIRMED |
| DUP-GAME-03 | Three visual languages for resource | Console `Chart` sparkline (no axis) vs `app/servers/page.tsx:210 ResourceBar` vs `DeploymentLogViewer` level colors | RECONFIRMED |
| DUP-GAME-04 | Install listener half-duplicate | Reference `InstallListener` WS state vs Forge static banner `console-view:37` + manual `refreshServer` on power/reinstall only | RECONFIRMED |
| DUP-GAME-05 | Deployments vs Builds vs Git vs Compose — four "how did code get to container" paths | `builds-view.tsx:110` disabled Build, `deployments-view` imageTag, git, compose — where Pterodactyl has one egg install | RECONFIRMED per FINAL §16 duplicate row |

Dead/shim:

* `console-view.tsx:182-183` `proxySocket` fake `WebSocket` with `readyState` getter — typed as `WebSocket` for `socketRef` compat but only exposes `send/close`. Should be typed `ConsoleConnection` (phase-02 §8).
* `DeploymentLogViewer.tsx` unreachable from game console — ships but no importer in `forge/web/components/server/*` or `forge/web/app/server/[id]/*`; `server-console-layout.tsx:22` `parentContext` dedup guard duplicates `console/servers/[id]/layout.tsx` logic — second layout (phase-02 §8).

---

## 7. False Completion / Decorative (reverified)

* `builds-view.tsx:110` **Build button permanently `disabled`** with honest tooltip (good disclosure) but still occupies primary affordance — empty guidance says "Deploy via git or compose" yet button remains rendered as if actionable. Phase-02 flagged decorative; FINAL §12.3 lists as `implemented but BROKEN` pattern for app platform; reverification: still rendered disabled at `254`.
* `transfer-view.tsx:108` **Progress bar** `progress!==null` driven by `ApiLegacyTransferStatus progress?:number` shim (`servers.ts:431` nullable) — backend may never populate (null) → bar silently absent for real transfer, implying 0% though title says "Transfer in progress". False completion (UI reserves bar but never fills).
* `app/servers/page.tsx:250` **Resource bars** three `Unavailable` rows — honest (`comment 252` intentionally unavailable) but decorative layout reserve implies future telemetry; `FINAL §16` "triple unavailable noise" remains.
* `crash-banner.tsx:38` `isLoading → "Loading crash history…"` inside console shell — non-admin sees persistent loading then hidden (if `access.isAdmin` false, query `enabled:false`, `isLoading false`, `crashes undefined → null` but brief flicker on admin cache). Not false completion but UX jitter.

---

## 8. Reconciliation vs FINAL_PARITY_AUDIT.md §16 & subagent-10 U01-U14

| FINAL §16 claim | U mapping | Reverified? | Δ |
|-----------------|-----------|-------------|---|
| "synthetic timestamps & cumulative network graph" | — | **Confirmed** `console-view:299` + `203` both untouched | No delta — still P1/P2 |
| "offline banners duplicated, empty CTAs dead, tone maps per-file" | U06 empty/loading/error/offline — duplicate `OfflineBanner` + dead `RateLimit` `ErrorRateLimit:164` never consumed; U05 per-file `Pill`/`statusConfig` diverge | Game console only shows header `connectionError` dot, not `OfflineBanner`; `states-offline.tsx` + `admin-shell OfflineBanner` both exist but game console doesn't mount them — still duplicated at repo level, game console under-signals | Duplication remains at repo level |
| "Entire networking admin UX non-functional … but 3 of 4 admin create/prune 404s now FIXED" | — not game-UX | Game backups/transfer create routes *are* functional (`createBackup` `backups-view:50`, `transferServer` `transfer-view:58`) — not part of §16's 404 set (those were container/network/volume) | No conflict |
| "Triple env-var editor incoherence" `env-var-editor.tsx:8` vs `AdminAppsShared:120` vs `EnvironmentEditor.tsx:37` | U09 **BROKEN** triple editors | Game startup vars use `fetchServerStartup/updateServerStartupVariable` (not env-var editors) — game hosting avoids that bug; but schedule task command env interpolates similarly | Game console clean, U09 remains BROKEN app-side |
| "Duplicate deployment polling (2s vs 5s)" | U04 **BROKEN** `deployment-progress 2s` vs `DeploymentTimeline 5s` | Not in game console shell; game stat poll is `WebSocketManager` not poll, backups `3000ms`, transfer `5000ms` — distinct but still duplicate reconnect vs DeploymentLogViewer 20 attempts | Separate duality but same root: two reconnect impls |
| U01 nav IA 5 groups → 6 goal groups, Advanced 27 largest no collapse | Game `ServerNav 17 tabs` spills to drawer without scroll hint | Game nav has 17 tabs (similar sprawl) but `ServerConsoleLayout` groups not visible; no second-level collapse | Same pattern smaller |
| U03 app detail `useState` tabs not routed → refresh loses tab, back hardcodes `/servers` | Game `server-console-layout:83` **is** routed (`pathname.split('/').at(-1)`) — good, opposite of U03. `app-detail.tsx:55` hardcodes `/servers` already noted FIXED in FINAL appendix, but game console breadcrumb `server-console-layout:118` correctly links to `/servers` then server name | Game layout **better** than app — synthesis should not lump |
| U05 status tones per-file vs single `badgeStateColor` | `AdminServers.tsx:335` vs `app/servers/page.tsx:20` pulse divergence | Game console `controls:243` `start emerald restart slate stop|kill red` per-file; admin `ServerAboutTab` tiles differ — still per-file | Remains |
| U08 domains gated `enabled: !!serverFilter` empty by default, no aggregate | Not game transfer — but `transfer-view:49` `enabled: !!targetNodeId && canTransfer` for allocations is correctly gated (good) | Distinct; game gating is correct UX (don't fetch allocations without target) | Not a defect |
| U10 tenancy cascade display-only `fetchApps` global | Game `fetchServers` global (user vs admin filter only) + `schedules` not tenancy-scoped | Same bug applies: server list not org→project→env scoped | Remains but out-of-scope for console UX; noted only |
| U02 dashboards split inventory vs observability correctly | Game console correctly splits telemetry (charts) vs banner state | No defect | Consistent |

**Synthesis takeaway:** §16's game-relevant bullets ("synthetic timestamps, cumulative network, offline banners, dual pollers, triple unavailable") are **all still open** at reverification; none were listed as FIXED in `FINAL_PARITY_AUDIT.md Appendix` (which listed container create, cache forward, HMAC 401 — not console). `MASTER_FINDING_INDEX` still shows `REF-GAME-F-G-17..19` with `—` (open), `CONFIRMED_BY` not yet set — now confirmed here.

---

## 9. Verdict Evolution

| Capability | phase-02 verdict | FINAL_PARITY verdict | Reverified verdict | Movement |
|------------|------------------|----------------------|--------------------|----------|
| Console ANSI fidelity | PARTIAL P2 | PARTIAL (carried) | **PARTIAL P2** | No movement |
| WS reconnect / JWT | PARTIAL P2 + BROKEN F-G-17 | PARTIAL + BROKEN (open) | **PARTIAL P2 + BROKEN P1 F-G-17** | No fix |
| Command history/search/timestamps | PARTIAL P3 + BROKEN synthetic | BROKEN synthetic (FINAL §16 explicit) | **PARTIAL + BROKEN LF-04** | No fix |
| Install banner/state | PARTIAL P2 | PARTIAL | **PARTIAL P2** | No fix |
| Transfer handoff/progress/dual truth | PARTIAL P1/P2 + BROKEN F-G-19 | PARTIAL (gateway but game row open) | **PARTIAL + BROKEN F-G-19** | No fix |
| Backup per-item WS vs poll + global busy | PARTIAL P3 | PARTIAL | **PARTIAL P3** | No fix |
| Resource graphs (CPU/Mem/Net) | PARTIAL P2 | PARTIAL (duplicate bars etc) | **PARTIAL P2 + BROKEN F-G-18** | No fix |
| Network delta vs cumulative | **BROKEN P1** | **BROKEN (§16 "cumulative")** | **BROKEN P1 F-G-18** | Still broken |
| Server cards triple unavailable | PARTIAL P3 | PARTIAL | **PARTIAL P3** | Won't fix by design (honest but noisy) |
| Notifications/toast duality | PARTIAL P3 | PARTIAL U06 | **PARTIAL P3** | No fix |
| Crash/maintenance banner | COMPLETE | (not re-evaluated) | **PARTIAL — regressed via admin gate** | Regress |
| Power controls | PARTIAL P3 | — | **PARTIAL** | No fix |
| DeploymentLogViewer unwired | PARTIAL unwired | UNWIRED (12.3) | **UNWIRED/DUPLICATE** | No fix |

**Overall reverified verdict for Game Hosting UX:** **~2 BROKEN P1 (ws-plain-drop, network cumulative) + 1 BROKEN P1 (transfer dual-source race) + 2 BROKEN P2 (synthetic timestamps, auto-max) remain. 13 PARTIALs persist. No P0 FIX since phase-02.** The console is visually polished but *operationally aliasing*: quiet connection issues look connected, quiet network bursts look idle, quiet transfer completion flickers. These are all wiring bugs, not missing subsystems — fixable before any new runtime or mesh work.

---

## 10. Recommendations — Prioritized Wiring Order (within existing code, no new tables)

| # | Action | File:line | Effort | Value |
|---|--------|-----------|--------|-------|
| 1 | **P1 FIX LF-REV-01** — Remove `JSON.parse` in `WebSocketManager:109`, forward `event.data` raw; parse only in stats handler | `websocket-manager.ts:104-113` + `console-view:151` second-parse removal | S | P0 console unblock |
| 2 | **P1 FIX LF-REV-02** — Delta network: `prev {rx,tx}` ref, push `max(0,rx-pr)+max(0,tx-pr)` (or two lines In/Out cyan vs yellow like `StatGraphs:61`); keep `rx+tx` only in `detail` text | `console-view:203` | S | P0 truthful throughput |
| 3 | **P1 FIX LF-REV-03** — Single-source transfer: derive `isTransferring = transfer?.transferring ?? server.transferring` when query succeeded; invalidate `["server",id]` on `transfer` change; clear stale `targetNodeId` on failed→idle | `transfer-view:79,92,174` | S | P0 no duplicate transfer |
| 4 | **P2 FIX LF-REV-04** — Timestamps: store `at` per line (`{text,at}`) or parse daemon timestamp; remove render-time `new Date()` | `console-view:299` | S | Forensics |
| 5 | **P2 FIX LF-REV-05** — Anchor chart `max` to limits (`CPU→limit||100`, `Memory→limit`, `Network→suggestedMax`) + distinct palette emerald/amber/cyan + add Disk card (most threatening) | `console-view:91-94,247` | S | No misleading scale |
| 6 | **P2 ADD xterm or ansicolor** — Wire `ansi_up` or `xterm.js` for console; at minimum `stripAnsi` both viewers + regex `WebLinksAddon` equivalent (autolink IP:port/traceback) | `console-view:296` + `DeploymentLogViewer:22` unify | M | Readability |
| 7 | **P2 ADD transfer handoff** — On `transferStatus success` re-ticket `connectServerWebSocket` to target node or page-reload hint; mirror `WebsocketHandler:65` | `transfer-view` + `console-view` WS | M | No blackout on migration |
| 8 | **P2 ADD install listener** — Subscribe console WS to `INSTALL_STARTED|COMPLETED` or poll `GET /servers/:id` every 5s while `installing` → auto-clear banner + `refreshServer` | `console-view:236` | S | No stale banner lag |
| 9 | **P3 FIX busy scope** — Per-backup pending map (`pendingId === backup.name`) not global `busy`; keep pagination context (`invalidate` but not `setCurrentPage(1)` jump) | `backups-view:101` | S | Parallel ops |
|10 | **P3 CONSOLIDATE WS + toast** — `DeploymentLogViewer` onto `WebSocketManager`; drop `sonner` import in `files-view`, promote `connectionError` to global banner or `pushToast` so signal survives tab switch | `DeploymentLogViewer:35` + `toast:21` | M | One reconnect, one toast |
|11 | **P3 ALIGN permissions** — `ServerNav` console tab requires `control.console` (not just `websocket.connect`) or add inline hint "need `control.console`"; split `settings.reinstall` → `server.transfer` scope; fix maintenance flag guard + banner | `server-nav.tsx:17` + `console-view:124` | S | Subuser clarity |
|12 | **P3 POLISH empty/caps** — Backups empty limit-aware body, re-add `>250` file cap banner (`sortFiles(slice(0,250))`), hide `Build` button or link to git/compose deploy | `backups-view:98` + `builds-view:110` | S | No dead affordances |

All items reuse existing tables/handlers/WS types; none requires a new subsystem.

---

## 11. Evidence Inventory (what was re-read for this reverification)

* References: `reference/game-hosting/pterodactyl-panel/resources/scripts/components/server/console/Console.tsx:1` (235 lines), `ServerConsoleContainer.tsx:1` (66 lines), `ServerDetailsBlock.tsx:1` (140 lines), `StatGraphs.tsx:1` (94 lines), `backups/BackupContainer.tsx:1` (85 lines), `BackupRow.tsx:1`, `events.ts:1`, `PowerButtons.tsx:1`
* Forge: `forge/web/components/server/console-view.tsx:1` (315 lines), `server-console-layout.tsx:20` (131 lines), `crash-banner.tsx:1` (105 lines), `backups-view.tsx:1` (222 lines), `transfer-view.tsx:1` (296 lines), `builds-view.tsx:1` (254 lines), `app/servers/page.tsx:1` (269 lines), `components/ui/toast.tsx:1` (55 lines shown), `components/deployment/DeploymentLogViewer.tsx:1` (223 lines), `lib/api/ws/websocket-manager.ts:1` (195 lines), `lib/api.ts:931` `connectServerWebSocket`, `lib/api/servers.ts:431` `fetchServerTransferStatus`
* Prior audits re-read fully: `audits/phase-02/subagent-04-game-ux.md:1` (627 lines), `audits/FINAL_PARITY_AUDIT.md:1` (575 lines, §16 UX:436-438), `audits/final-parity/subagent-10-security-ux.md:1` (30 rows S01-S16 + U01-U14), `audits/MASTER_FINDING_INDEX.md:52` `REF-GAME-F-G-17..21`

---

## 12. Handoff Note

PufferPanel's hybrid `WS + HTTP poll fallback (getConsole(lastMessageTime) every 5s)` and Ptero's `INSTALL_*/TRANSFER_* listeners mutating server state directly` plus **delta not cumulative network** are the highest-leverage adoptions for this UX slice. Beacon→Wings isomorphism is weakest on *push* (install/transfer status, per-backup `BACKUP_COMPLETED:uuid`, per-line console) not on *poll*. The three `MASTER FINDING INDEX` P1s confirmed here (F-G-17 double JSON, F-G-18 cumulative, F-G-19 dual source) plus the synthetic-timestamp lie and auto-max exaggeration are server-agnostic wiring fixes that should land before any new executor, mesh, or template work.
