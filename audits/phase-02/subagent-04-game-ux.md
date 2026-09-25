# Phase 02 — Subagent 04: Game Hosting UX — Console / Install / Transfer / Resource Graphs

> Dimension: CONSOLE REALTIME LOGS, INSTALL PROGRESS, TRANSFER/BACKUP PROGRESS, RESOURCE GRAPHS, SERVER CARDS, NOTIFICATIONS, EMPTY STATES, OPERATIONAL FEEDBACK
> Cluster: pterodactyl-panel, pelican-panel, pterodactyl-wings, pelican-wings, pufferpanel, pufferpanel-templates
> Auditor: subagent-4 (Phase 2)
> Date: 2026-08-23
> Method: source inspection (Read/Grep/Glob) — real paths under `reference/game-hosting/*` and `forge/web/*`. No marketing copy. All citations are `file:line` verifiable. Verification via read-only file inspection; execution spot-checks where noted.

---

## 1. Scope & Methodology

Inspected how game-hosting panels make *operational truth* visible during the three critical moments: live console, install/transfer/backup, and continuous resource feedback. Compared Forge wiring, visual fidelity, and failure modes against references.

**Reference paths inspected:**
- Pterodactyl Panel `reference/game-hosting/pterodactyl-panel/resources/scripts/components/server/console/Console.tsx:1`, `ServerConsoleContainer.tsx:1`, `StatGraphs.tsx:1`, `ChartBlock.tsx:1`, `StatBlock.tsx:1`, `ServerDetailsBlock.tsx:1`, `InstallListener.tsx:1`, `TransferListener.tsx:1`, `WebsocketHandler.tsx:1`, `events.ts:1`, `PowerButtons.tsx:1`, `backups/BackupContainer.tsx:1`, `backups/BackupRow.tsx:1`, `files/FileManagerContainer.tsx:1`
- Pelican Panel `reference/game-hosting/pelican-panel` (diffed against pterodactyl base; flags: per-panel `resources/views` not present — backend PHP Filament changes, not console UX — so pterodactyl console remains canonical horizon)
- PufferPanel `reference/game-hosting/pufferpanel/client/frontend/src/components/server/Console.vue:1`, `Stats.vue:1`, `Backup.vue:1`, `Status.vue:1`, `server.go:1`, `servers/server.go:1`
- PufferPanel templates `reference/game-hosting/pufferpanel-templates` (template list only — not panel UX)

**Forge paths inspected:**
- `forge/web/components/server/console-view.tsx:1`, `server-console-layout.tsx:1`, `crash-banner.tsx:1`, `server-nav.tsx:16`, `server-context.tsx:53`
- `forge/web/components/server/backups-view.tsx:1`, `transfer-view.tsx:1`, `builds-view.tsx:1`, `deployments-view.tsx:1`, `files-view.tsx:1`, `schedules-view.tsx:1`, `settings-view.tsx:1`, `activity-view.tsx:1`, `network-view.tsx:1`, `mounts-view.tsx:1`
- `forge/web/app/server/[id]/page.tsx:1`, `forge/web/app/console/servers/[id]/page.tsx:1`, `forge/web/app/console/servers/[id]/layout.tsx:1`
- `forge/web/app/servers/page.tsx:1`, `forge/web/app/admin/servers/page.tsx:1`, `forge/web/components/admin/AdminServers.tsx:1`
- `forge/web/components/deployment/DeploymentLogViewer.tsx:1`
- `forge/web/lib/api.ts:233`, `forge/web/lib/api/servers.ts:393`, `forge/web/lib/api/ws/websocket-manager.ts:1`

Status taxonomy: `COMPLETE | PARTIAL | UNWIRED | BROKEN | MISSING | DEAD | DUPLICATE | FALSE_COMPLETION | UNKNOWN`
Severity: `P0-P4` + `USER_VISIBLE / OPERATOR_VISIBLE / SILENT`

---

## 2. Reference Platform UI Inventory (file evidence)

### 2.1 Pterodactyl Panel — Console (canonical)

- **Renderer:** `console/Console.tsx:58` creates `new Terminal({disableStdin:true, cursorStyle:'underline', fontSize:12, fontFamily:mono, rows:30, theme})` with 7 addons at `128-133`: `FitAddon`, `SearchAddon`, `SearchBarAddon`, `WebLinksAddon`, `Unicode11Addon`, `ScrollDownHelperAddon`. `theme:23` maps 16 ANSI colors (`red '#E54B4B'` … `brightCyan '#89DDFF'`, `selection '#FAF089'`). `handleConsoleOutput:77` does `terminal.writeln((prelude ? TERMINAL_PRELUDE : '') + line.replace(trailing newline) + '\u001b[0m')` where `56` `TERMINAL_PRELUDE = '\u001b[1m\u001b[33mcontainer@pterodactyl~ \u001b[0m'`. `handleDaemonErrorOutput:89` wraps in `\u001b[1m\u001b[41m` (red bg). `handlePowerChangeEvent:94` writes `'Server marked as ' + state`.
- **Hotkeys:** `144-156` `terminal.attachCustomKeyEventHandler` intercepts `Ctrl/Cmd+C` → copy, `Ctrl/Cmd+F` → `searchBar.show()`, `Escape` → `searchBar.hidden()`. `useEventListener('resize', debounce(()=>fitAddon.fit(),100))`.
- **Socket multiplex:** `180-181` single `instance` (from `ServerContext.socket`) listens on one WS for **7** events at `170-178`: `STATUS`, `CONSOLE_OUTPUT`, `INSTALL_OUTPUT`, `TRANSFER_LOGS`, `TRANSFER_STATUS`, `DAEMON_MESSAGE` (with prelude), `DAEMON_ERROR`. On `connected && instance` at `183` it `terminal.clear()` *unless* `isTransferring` (preserves logs during handoff) then `addListener` for each key and `instance.send(SocketRequest.SEND_LOGS)`. `handleTransferStatus:80` writes `'Transfer has failed.'` only on `failure`. History via `usePersistedState<string[]>(`${serverId}:command_history`, [])` capped at `32` at `118`, `ArrowUp/Down` navigates at `99-114`, `Enter` at `117` `instance.send('send command', command)`.
- **Overlay:** `202` `<SpinnerOverlay visible={!connected} size='large'/>` covers terminal when disconnected; input disabled `!instance || !connected` at `218`.
- **Events catalog:** `events.ts:1` `SocketEvent` enum enumerates `DAEMON_MESSAGE, DAEMON_ERROR, INSTALL_OUTPUT, INSTALL_STARTED, INSTALL_COMPLETED, CONSOLE_OUTPUT, STATUS, STATS, TRANSFER_LOGS, TRANSFER_STATUS, BACKUP_COMPLETED, BACKUP_RESTORE_COMPLETED` and `SocketRequest SEND_LOGS, SEND_STATS, SET_STATE`.

### 2.2 Pterodactyl Panel — Install / Transfer listeners

- `InstallListener.tsx:12` `BACKUP_RESTORE_COMPLETED` → `mutate(getDirectorySwrKey(uuid,'/'))` + `status:null`; `20` `INSTALL_COMPLETED` → `getServer(uuid)`; `26` `INSTALL_STARTED` → `setServerFromState(s=>({...s,status:'installing'}))`. No progress % — binary state, output streamed via `INSTALL_OUTPUT` into console.
- `TransferListener.tsx:11` `TRANSFER_STATUS` → `pending|processing` sets `isTransferring:true`, `failed` → `false`, `completed` → `getServer(uuid)`. `WebsocketHandler.tsx:65` on `transfer status` `starting|success` *reconnects* WS: `socket.close(); setInstance(null); connect(uuid)` to switch from source to target node for `TRANSFER_LOGS`. JWT `token expiring/expired` at `50` triggers `updateToken`.
- `WebsocketHandler.tsx:43` `SOCKET_ERROR` → `setError('connecting')`, `SOCKET_CONNECT_ERROR` → `'Failed to connect... try refreshing'`, `SOCKET_CLOSE` → `setConnectionState(false)`. Banner rendered at `110-127` as red bar with spinner or error text — persistent, not toast.

### 2.3 Pterodactyl Panel — Resource graphs

- `ServerDetailsBlock.tsx:54` limits `cpu:${limits.cpu}%`, `memory:${bytesToString(mbToBytes(limits.memory))}`, `disk` similarly. `getBackgroundColor:22` returns `bg-red-500` if `value/max >0.9`, `bg-yellow-500` if `>0.8`. `StatBlock.tsx:18` accepts `color` prop to paint left `status_bar` + icon bg. `ServerDetailsBlock:92` renders **7** blocks in `grid-cols-6`: `Address` (copyOnClick via `CopyOnClick`), `Uptime` (color via `status`, renders `UptimeDuration` or `Offline`/`Capitalized(status)`), `CPU Load`, `Memory`, `Disk`, `Network Inbound`, `Outbound`. Each shows `Limit` `value / limit|∞` at `35`. Offline guards at `110-134`: `status==='offline'` → `<span class='text-gray-400'>Offline</span>`. `73-88` `useWebsocketEvent(STATS)` parses JSON and calls `setStats({memory:stats.memory_bytes, cpu:stats.cpu_absolute, disk:stats.disk_bytes, tx,rx, uptime})`.
- `StatGraphs.tsx:41` uses `useChartTickLabel` for CPU (cpu, '%', 2 decimals) and Memory (MiB), `useChart('Network',{sets:2, y ticks bytesToString})` with distinct borderColors `cyan.400/cyan.700` vs `yellow.400/yellow.700`. `52-64` delta calc: `previous tx/rx` ref initialized `-1`, then `network.push([tx - prev.tx, rx - prev.rx].map(max0))`. `46` clears all three charts when `status==='offline'`.

### 2.4 Pterodactyl Panel — Backups / Files operational feedback

- `backups/BackupContainer.tsx:62` `backupLimit = featureLimits.backups`; empty state at `40` checks `!backupLimit ? null : page>1 ? 'run out...' : 'no backups...'`; limit-zero banner at `57` `'Backups cannot be created because limit 0'`; footer at `64` `'{backupCount} of {backupLimit} have been created'`. `CreateBackupButton` only if `backupLimit>backupCount`. `BackupRow.tsx:24` per-backup `useWebsocketEvent(`${BACKUP_COMPLETED}:${backup.uuid}`)` patches single row via `mutate(data=>({items:data.items.map(... isSuccessful, checksum, bytes, completedAt:new Date())}))`. UI at `53-95`: `Spinner small` if `completedAt===null`, `faLock yellow` if `isLocked`, `Failed` red pill if `isSuccessful===false`, `bytesToString(bytes)`, `checksum` mono, `formatDistanceToNow(createdAt)`, context menu only if `completedAt`.
- `files/FileManagerContainer.tsx:92` empty `'This directory seems to be empty.'` at `93`; `97-103` if `files.length>250` yellow banner `'too large to display, limiting to first 250'`.

### 2.5 PufferPanel — Console / Stats / Backup

- `Console.vue:1` uses Web Worker `ConsoleWorker?worker&inline` for ANSI→HTML transform. `27` `server.on('console', onMessage)` + `30` `server.startTask(()=>{if(needsPolling()&&hasScope('server.console')) onMessage(await server.getConsole(lastMessageTime))},5000)` — WS *with* HTTP long-poll fallback every 5s. `42` `onMessage` tracks `lastMessageTime = epoch` or `Date.now()`, posts to worker; `52` worker returns `{op:'update'|'append', content}` rendered as `div.innerHTML`. `64` caps DOM at `1200` elements → slice to `1000`. `99` `previousCommand/nextCommand` with `temporaryCommand` preservation and history cap `100`. Permissions gated `hasScope('server.console')` vs `server.console.send`.
- `Stats.vue:1` `chart.js/auto` with `chartjs-adapter-date-fns`, dual charts (CPU, Memory) via `Chart` constructor, `chartOptions('cpu'|'memory')` at `97`. `66` `addData` pushes `{x:Date.now(), y:d.cpu|memory}` plus optional JVM heap/meta `stack:'jvmMemory'` with `hidden:true` until `d.jvm` present — then `memoryChart.show(...)` / `hide(Memory)`. Caps at `60` points `while>60 shift`. `92` `x.min = x - 60*1000` rolling 60s window. Poll fallback `needsPolling()&&hasScope('server.stats')` → `getStats()` every 5s.
- `Backup.vue:1` `getBackups()` on mount, `sortedBackups computed b.createdAt`, `backupRunning` bool gate, `isLoading=!Array.isArray(backups)`, CRUD via `server.createBackup(name) → toast.success('BackupStarted')`, `restoreBackup → toast.success('RestoreStarted')`, delete, sorted list with `intl.format(new Date(backup.createdAt))`, single `dl-link href=server.getBackupUrl(id)`.
- `Status.vue:1` (not fully enumerated) renders power state pill.

---

## 3. Forge UX Deep Inventory (file evidence)

### 3.1 Console — `console-view.tsx`

- **Structure at `101-314`:** `MAX_LINES=500` `MAX_POINTS=60`. Helpers `formatUptime`, banners `InstallBanner` (amber `Download`, static text), `TransferBanner` (`Upload animate-pulse`, shows `transferTargetNodeId` + `transferState`), `SuspendedBanner` (rose). `Chart:91` single sparkline helper: `max=Math.max(...values,1)`, `points = values.map((p,i)=>`${(i/(len-1))*100},${100-(p/max)*92}`)`, renders `polygon fill rgba(220,38,38,.16)` + `polyline stroke #ef4444 width1.5`. Used thrice at `247-249` for CPU/Memory/Network + fourth card Uptime `251-264` with `border-4` circle `animate-pulse` if running.
- **WS wiring at `132-212`:** Two `WebSocketManager` instances:
  - Console at `146`: `factory:()=>connectServerWebSocket(server.id,'console')` (ticket via `POST /servers/:id/ws/ticket?stream=console` → `wss://.../ws/console?token=`), `maxRetries:20 baseDelay:1000 maxDelay:30000`. `onMessage:151` tries `JSON.parse` → `payload.data ?? payload.error ?? text`; on success `setLines(prev=>[...prev, ...text.split('\n').filter(Boolean)].slice(-MAX_LINES))`. `onStatusChange:157` maps `connected/connecting/reconnecting/disconnected` → `connection` state, flushed `cmdBuffer` on connected. `socketRef` proxy exposes `send=>manager.send` but checks `manager.status==='connected'?OPEN:CLOSED`.
  - Stats at `192`: second manager `factory:()=>connectServerWebSocket(server.id,'stats')`, same retries. `onMessage:197` casts `data as ApiStats & {error?:string}`; `memory = memoryLimit>0?(memoryBytes/memoryLimit)*100:0`; `network = rx+tx` cumulative; pushes to 3 histories.
- **REST fallback:** `138` `fetchServerLogs(server.id).then(logs=>setLines(logs.split('\n').filter(Boolean).slice(-MAX_LINES)))`. On catch sets `connectionError` but does not toast.
- **Controls at `243`:** `controls=['start','restart','stop','kill']` grid-4, `canPower` derives `control.start/restart/stop`, `blocked=suspended||transferring||installing`. Buttons disabled `!canPower||blocked||power.isPending||(signal==='start'?status==='running':status!=='running')`. `kill` requires `confirm({Kill server?})`.
- **Console chrome at `267-312`:** header `PlugZap` emerald vs amber, `stateLabel` + `messageCount msgs` + `connectionError`, toolbar 5 icon buttons: search toggle, `ArrowDown` autoScroll, `Clock` showTimestamps, `RefreshCw` `setNonce(n+1)` reconnect, `Trash2` clear. Search at `285` filters `lines.filter(lower includes query)`. Output at `296` `role=log aria-live=polite h-[50vh] min-h-80 overflow-y-auto p-4 font-mono text-xs` mapping `filteredLines.map((line,i)=><div whitespace-pre-wrap break-words key=i-line> {showTimestamps?<span mr2 text-slate-500>{new Date().toLocaleTimeString()}</span>:null} {line}</div>)`; empty shows `searchQuery?'No matching':'Waiting…'||connectionError`. Command form at `306` `Server` icon, input `disabled={connection!=='connected'||!canConsole}`, `onKeyDown historyKey` (localStorage `console-history-${server.id}` via `133` and `214` load, 50 cap), submit `232` either `socketRef.send` or `cmdBuffer.push` then `setHistory([...])`+localStorage, Send button red disabled `!connected||!trim`.
- **Crash banner at `238`:** `<CrashBanner>` only if `!suspended&&!transferring&&status!=='installing'`; internals `crash-banner.tsx:17` `useQuery(['crash-history',serverId], fetchServerCrashHistory, refetchInterval:30000)`; shows `crashed N times`, `Last crash: date · Exit code · OOM`, `<details>` 5-item history, Restart + Reset actions.
- **Layout at `server-console-layout.tsx:20`:** `ServerConsoleLayout` reuses parent context if present else `ServerConsoleShell` fetches `[fetchServer, fetchCurrentUser]` parallel at `49`, sets `permissions: []` if admin|owner else `permissions ?? null`, `isAdmin` via `role==='admin'`. Loading full-screen `grid place-items-center bg-[#0a0e16]` spinner + text; error shows `Unable to load server` red card with Try again. `activeTab` derived via `pathname.split('/')`. `ServerNav` at `96` in flex aside + main `max-w-7xl p-4 sm:p-6 lg:p-8`.

### 3.2 Resource cards & server list — `app/servers/page.tsx`

- **Status pill at `20`:** `suspended→danger`, `transferring→pulse info`, `running→success`, `installing→pulse warning` else offline neutral.
- **Card at `210-258`:** `Link /server/:id` card with absolute right `w-1` bar color `suspended:#f43f5e | running:#10b981 | installing:#f59e0b | #64748b`. Inner: `h2 name`, `p description line-clamp-1`, `ServerStatus`, `node` + `allocation mono`, then **resource bars** at `250-258`: `ResourceBar icon=Cpu/MemoryStick/HardDrive` *without* `current/limit` props → `ResourceBar:152` hits `max===0` branch renders `<span text-[10px] text-slate-500>Unavailable</span>` + no bar. Guarded to hide entirely if `suspended||installing||transferring`. Comment at `252` explicitly says *intentionally unavailable to avoid misleading 100% bar* — honest but less useful than live telemetry.
- **Empty/loading:** `181` `<LoadingSpinner>` while `isLoading`, `187` red bordered error, `192` `EmptyState` with `Server h-12 w-12 text-slate-600` and `search ? No matching : empty.title` (i18n). Pagination at `263` `pageSize12`, `Pagination` limits to `12` per page `Math.ceil(filtered.length/12)`.
- `ResourceBar` primitive at `primitives.tsx:152` `used/current=0, max=limit>0?limit:0, percent=used/max*100, alarm>=90 red`, `ProgressBar:142` `aria-valuenow rounded`.

### 3.3 Transfer — `transfer-view.tsx`

- **State sources:** `31-38` `transferStatusQuery: useQuery(['server-transfer',id], fetchServerTransferStatus, retry:false, refetchInterval: transferring?5000:false, placeholderData:prev)` where `fetchServerTransferStatus:393` returns `{transferring, transferId, status, progress}` or `null` on 404. Also `server.transferring` (boolean from `fetchServer`), `server.transferState`, `server.transferError`, `progress = transfer?.progress`. `isTransferring = server.transferring` drives badge tone at `92` `tone=isTransferring?ok:errorMessage?danger:transfer?.transferring?warning:neutral`.
- **UI at `87-169`:** `StatusCard icon=ArrowRightLeft` with 4 branches: `isTransferring → loader spin Transfer in progress + phaseLabel + progress bar h-2 bg-white/[0.06] inner emerald transition-all width=clamp(progress) + error alert + Cancel button`; `errorMessage → XCircle Transfer failed + daemon log hint`; `transfer?.transferring → amber Transfer pending + phaseLabel`; else `CheckCircle2 No active transfer`. `phaseLabel` at `82` `phase.replace(/_/g,' ').replace(/\b\w/=>upper)`.
- **Initiate form at `172`:** shown only if `!isTransferring && !transfer?.transferring && canTransfer (settings.reinstall)` with `targetNodeId` select filtered `n.id!==nodeId && n.name!==node`, `primaryAllocationId` select from `fetchNodeAllocations(targetNodeId) enabled !!targetNodeId&&canTransfer` filtered `!a.server`, warning if `targetAllocations.length===0`, Start button disabled `!targetNodeId||!primaryAllocationId||pending`, shows start/cancel confirms `ConfirmDialog` with destructive long descriptions.

### 3.4 Backups — `backups-view.tsx`

- **Query at `43`:** `useQuery(['server-backups',server.id,currentPage], fetchBackups(id,page,20), enabled !!id, refetchInterval: data?.some(status pending|running)?3000:false)`. `invalidate` on any mutation.
- **Header at `89`:** `backupLimit` check: `disabled==='Backups are disabled'` else `'{total} of {limit} slots used.'` plus `Limit reached` red, `Restoring backup…` amber if `restoreMutation.isPending`.
- **List at `99-144`:** maps `backup.uuid??name` → `Archive icon`, `name`, `status uppercase · formatBackupBytes(size)`, `checksum ? 'Checksum: '+checksum : 'Checksum not available'`, `Created/Completed` via `formatDate`, 4 action buttons 9x9: Download (disabled `!usable={!='completed'}||!canDownload`), Lock/Unlock toggle (`Unlock amber` if `isLocked` else `Lock`) disabled `busy||!canDelete`, Restore amber `!usable||busy||!canRestore` with confirm, Delete red `!usable||busy||!canDelete||isLocked`. `busy = restore|delete|lock|unlock pending` blocks all rows.
- **Controls at `146-181`:** note `Only completed backups can be...`, `Show/Hide advanced options` toggle → panel with Backup Name input, Ignored Files comma-separated, Storage Destination note `'Custom destinations (S3,GCS,Azure) not supported yet. Default node-local.'`, Lock on create checkbox, Create Backup button `disabled !canCreate||disabled||pending||limitReached`.
- **Pagination at `159`:** `Page {page} of {total_pages} ({total} total)` with Previous/Next disabled at bounds.

### 3.5 Builds / Deployments — `builds-view.tsx`, `deployments-view.tsx`

- **Builds at `42-48`:** `fetchServerBuilds` with `refetchInterval: some pending|running?3000:false`, Buildpacks list with `staleTime 60000`, Server buildpacks, selected build detail via `fetchBuild`. `assignMutation` + `removeMutation` wired; **Build button at `110` is permanently `disabled` with `title='Buildpack builds are not implemented yet. Deploy via git or compose stack instead.'`**. Empty: `EmptyState Code2 'No builds yet. Deploy from git or compose...'`.
- **Deployments at `96-464`:** `fetchJSON /servers/:id/deployments`, `activeRelease live`, `imageTag` input, `handleDeploy` `POST .../deployments {imageTag}` then `setTimeout(loadReleases,1000)`. Rollback `POST .../rollback`, Force Promote `POST .../promote`, health config 6 fields, `DeploymentTimeline` not used here — instead `StateMachine` `deploymentMachineState` maps `building→provisioning, deploying→in_progress, health_checking→awaiting_health, live→completed...`. List items clickable `selectRelease` loads health results + events per release; detail shows `Events max-h-48 overflow-y-auto`, `Health results` 20 with `healthy?CheckCircle:XCircle`. Empty: `EmptyState RotateCcw 'No deployments yet. Deploy an image tag above...'`.

### 3.6 DeploymentLogViewer — `components/deployment/DeploymentLogViewer.tsx`

- Props `deploymentId, wsUrl:string|()=>Promise<string>, initialLogs`. Strips ANSI `ANSI_PATTERN`, `getLevelColor error→red warn→amber info→blue debug→slate`, `MAX_RECONNECT_ATTEMPTS=20` independent from `WebSocketManager`. `useEffect:61` manual `new WebSocket(url)` with `onopen→connected true retry0`, `onmessage→ JSON.parse LogEntry else {timestamp:ISOString, message:event.data}`, capped `[...prev.slice(-999),entry]`, `onclose→ setConnected false, delay min(1000*2^attempt,30000) → setNonce(v+1)` reconnect, `onerror→close()`. Toolbar: `Terminal icon Deployment Logs`, `Live/Disconnected` pill, `{logs.length} lines`, Search/Pause-Clear buttons. Output: `h-96 overflow-y-auto font-mono text-xs` with line `index+1, timestamp localTime, level uppercase pill, message stripAnsi` `hover:bg-white/[0.02]`.

### 3.7 Files / Schedules / Settings — operational feedback highlights

- **Files** `files-view.tsx:12` uses both `useToast` via `sonner` `toast.success/error` *and* `errorMessage`; Monaco editor dynamic import, drag-drop overlay with `pointer-events-none fixed inset-0 bg-black/60` when dragging, mass actions bar sticky bottom, context menu, preview modal, ConfirmDialog for delete.
- **Schedules** `schedules-view.tsx:37` TaskEditor handles `command|power|backup` with sequence/offset/continueOnFailure, Cron helper, validation, reorder via sequence swap.
- **Settings** `settings-view.tsx:15` ServerSettingsView with SFTP details `sftpHost/daemonSftp` + `username = email.id`, copy, rename, reinstall.
- **Activity** `activity-view.tsx:31` `EmptyState` title `'No activity recorded'|'No activity matches filter'`.
- **AdminServers** `admin/AdminServers.tsx:1` detail tabs `about|details|build|startup|allocations|database|mounts|manage|delete`; create modal validation, status badge.

### 3.8 Notification plumbing

- `components/ui/toast.tsx:1` `ToastProvider` context `toast({title,message,tone})→ id` slicing to last 3, auto-dismiss `7000 error else 4500`, `loading` never auto-dismisses, region `aria-live=polite aria-atomic`. `useToast()` consumed in transfers/databases/settings etc. Files uses `sonner` `toast` (second system) at `files-view.tsx:12`.
- `WebsocketHandler.tsx` pattern *not* present in Forge — Forge has no global reconnect banner; per-view `connectionError` strings shown inline near console header (`'· '+connectionError` in red).

---

## 4. Capability Comparisons (18)

### C01 — Console rendering fidelity: xterm.js vs plain div

**REFERENCE pattern:**
Pterodactyl `Console.tsx:58` xterm.js `Terminal` + `FitAddon/SearchAddon/WebLinksAddon/Unicode11Addon` renders ANSI palette (16 colors at `theme:23`), `\u001b[0m` reset per line, clickable URLs, emoji-correct widths (`unicode.activeVersion='11'`), 30-row fit, scroll helper. PufferPanel `Console.vue:7` offloads ANSI→HTML to `ConsoleWorker` and caps DOM at 1000 nodes after worker transform — both preserve colors and background codes (daemon error red bg at `Console.tsx:91`). Console is not a `<div>` of strings — it is a canvas-aware terminal.

**FORGE:**
`console-view.tsx:296` `role=log h-[50vh] overflow-y-auto p-4 font-mono text-xs whitespace-pre-wrap break-words` mapping `filteredLines→div(line)` with no ANSI parse, no link detection, no unicode width, no selection background, no screen buffer. `DeploymentLogViewer:19` *does* `stripAnsi` before render (removes codes) while console leaves raw `\u001b[...` visible if daemon emits them. Neither converts ANSI to color — pterodactyl's `theme.red/green/...` palette is lost.

**STATUS:** `PARTIAL`

**GAP:** Forge console downgrades fidelity from game-server expectation: Minecraft/Palworld daemons emit extensive ANSI (log levels, player joins). Users see escape codes or monotone text; PufferPanel/Ptero show colored levels and red-bg daemon errors. Missing `WebLinksAddon` also removes one-click IP:port or traceback link UX. `DeploymentLogViewer` and `ConsoleView` disagree on ANSI handling (strip vs leak) — inconsistency within Forge.

**RECOMMENDATION:** Adopt xterm.js or strip+color inline (e.g., `ansi_up`) for console; at minimum `stripAnsi` both viewers and render error lines with red tint like `handleDaemonErrorOutput`. Add URL detection with `WebLinksAddon` equivalent.

**SEVERITY:** `P2 USER_VISIBLE` — readability, not data loss.

---

### C02 — Console connection lifecycle & reconnect transparency

**REFERENCE:**
Pterodactyl `WebsocketHandler.tsx:43` distinguishes `SOCKET_CLOSE` → `setConnectionState(false)`, `SOCKET_CONNECT_ERROR` → dedicated error message `'Failed to connect... try refreshing'`, `SOCKET_ERROR` → `'connecting'` spinner, plus `token expiring/expired` → `updateToken` re-auth. `Console.tsx:183` preserves buffer if `isTransferring` then `terminal.clear()` + `instance.send(SEND_LOGS)`. `PufferPanel Console.vue:30` hybrid: WS event *plus* `getConsole(lastMessageTime)` poll every 5s if `needsPolling()` — degrades gracefully when WS blocked. Both show `<SpinnerOverlay visible={!connected}>` or `Loader` covering terminal, not just header text.

**FORGE:**
`console-view.tsx:107-186` maintains `connection: connecting|connected|reconnecting|error` + `connectionError` string + `nonce` reconnect button. `WebSocketManager:81` maps `initialConnection?connecting:reconnecting` then `scheduleReconnect` with `retryCount 20, base 1s cap 30s + jitter up to 1s`. Header at `269` shows `PlugZap emerald vs amber`, `stateLabel` + `msgs` + `error·message`. No overlay: console remains readable but header dot indicates state. `socketRef` is a fake `WebSocket` proxy exposing `readyState` derived from `manager.status`. `DeploymentLogViewer:35` reimplements same retry (20, `1s*2^attempt cap30s`) *without* `WebSocketManager` — duplicate logic. Neither Forge viewer handles JWT expiry explicitly; ticket fetched only at `factory` time at `935` `connectServerWebSocket` — if token expires mid-session, `onError` sets generic `'The console connection failed'` at `179` and relies on reconnect fetching new ticket, but `WebsocketHandler`'s `token expiring` fast-path (refresh without reconnect) is missing — blackout until retry fires.

**STATUS:** `PARTIAL`

**GAP:** Forge's `WebSocketManager` is general but console layer adds nothing on `token expiring` fast re-auth; user sees spinner then header `Connection error` until next retry (1-30s) vs Ptero seamless token refresh. Duplicate reconnect in `DeploymentLogViewer` means two implementations to keep in sync — drift risk. Absence of overlay vs header-only signal violates reference expectation that a disconnected console is overtly blocked (ptero dims terminal, disables input via `!instance||!connected` with centered spinner).

**RECOMMENDATION:** Hoist JWT refresh like `WebsocketHandler:50` into `WebSocketManager` (expose `onTokenExpiring` hook, refetch ticket). Consolidate `DeploymentLogViewer` onto `WebSocketManager`. Consider semi-transparent overlay when `connection!=='connected'` matching ptero pattern.

**SEVERITY:** `P2 OPERATOR_VISIBLE`

---

### C03 — Command input, history, search, scroll & timestamps

**REFERENCE:**
Ptero `Console.tsx:99-123` `history: usePersistedState('${serverId}:command_history',[])`, cap `32`, `ArrowUp min(historyIndex+1, len-1)` with `preventDefault` to keep cursor at end, `ArrowDown max(historyIndex-1,-1)` restores empty, `Enter → setHistory([cmd, ...prev].slice(0,32)); instance.send('send command',cmd)`. Search via `SearchBarAddon` with `z-index:10` fix, hotkey `Ctrl/Cmd+F` opens bar, `Escape` hides. Autoscroll via `ScrollDownHelperAddon` + `FitAddon.fit()` on resize debounced `100ms`. PufferPanel `Console.vue:80` similar but `history 100`, `temporaryCommand` stash when browsing, `v-hotkey 'c x' clear, 'c c' focus`.

**FORGE:**
`console-view.tsx:104-226` mirrors with `history: useState<string[]>` hydrated from `localStorage HISTORY_KEY` at `214`, cap `50`, `historyKey:227` `ArrowUp/Arrow` logic, `historyIndex -1→len-1` bounded. Submit at `226` pushes via `socketRef.send` *or* `cmdBuffer.push` if not connected (offline queue — not in references), then persists to localStorage, clears. Toolbar at `278-282` offers 5 icon toggles: search (filters `lines.filter lower includes query)`), autoScroll `ArrowDown` (emerald vs slate), showTimestamps `Clock`, reconnect, clear. Search `285` is a full-width filter input rendering `filteredLines` not terminal find highlights. autoScroll implemented via `131` `requestAnimationFrame(()=>scrollTo(scrollHeight))` when `autoScroll`.

**STATUS:** `PARTIAL`

**GAP:** Four deviations:
1. **Timestamps are synthetic (logic bug — see LF-02):** `299` `showTimestamps ? <span>{new Date().toLocaleTimeString()}</span> : null` generates *render-time* client clock per line on every re-render, not the log source timestamp. Ptero never fakes timestamps; PufferPanel uses `epoch` from payload `43-49`. Toggling showTimestamps changes all lines to current wall time — misleading for crash post-mortem.
2. **Search is filter, not find:** Ptero's `SearchAddon` highlights occurrences and scrolls within scrollback; Forge's filter *removes* non-matching lines from DOM, destroying context around match (operator cannot see preceding traceback).
3. **Offline command buffer is silent:** `226` queues into `cmdBuffer` if `readyState!==OPEN` and flushes on `connected` `164-165`, but UI input stays `disabled={connection!=='connected'||!canConsole}` at `309` — user cannot type to queue while disconnected, so the buffer is only reachable via synthetic state bug (if connection flips mid-submit). Dead code path.
4. **History cap inconsistency:** 50 vs 32 vs 100 — minor, but PufferPanel's `temporaryCommand` restoration on `nextCommand` beyond end is not replicated; Forge's `historyIndex -1` restores empty string, losing the partial draft user was typing before pressing Up.

**RECOMMENDATION:** Remove synthetic timestamp or source it from server log line timestamp if present (otherwise remove toggle and label as client render time). Replace filter with highlight-find (or add highlight mode). Either remove `cmdBuffer` or enable input during `reconnecting` with queued label.

**SEVERITY:** `P3 USER_VISIBLE` (timestamps) / `P3 USER_VISIBLE` (search)

---

### C04 — Install progress: banners, output, state machine

**REFERENCE:**
Ptero `ServerConsoleContainer:19-34` reads `isInstalling`, `isTransferring`, `isNodeUnderMaintenance` and shows single `Alert warning mb-4` picking one message. `InstallListener:20` reacts to `INSTALL_STARTED→status:'installing'` and `INSTALL_COMPLETED→getServer(uuid)` via WS *push* — install output streamed via `INSTALL_OUTPUT` into same Console as `handleConsoleOutput` indistinguishable from console except prelude flag. `PufferPanel` install concept maps to `server.needsPolling()` + `getConsole` poll; install scripts run as daemon operation.

**FORGE:**
`console-view.tsx:37-50` `InstallBanner` amber `Download` with two-line static text *only* when `server.status==='installing' && !suspended && !transferring` at `236`. No step count, no % bar, no spinner, no dismiss. `console-view:128` `canPower` blocked when `server.status==='installing'` + `blocked` disables all 4 power buttons. `ConsoleView:138` initial `fetchServerLogs` fills `lines` before WS joins, so install output *will* appear in `lines` if daemon's console stream includes `INSTALL_OUTPUT`-equivalent, but Forge has no `INSTALL_OUTPUT` listener — relies on console stream being install output during install (assumed by banner copy “The console will display output from the installation script.”). `AdminServers.tsx:335` `ServerStatusBadge` `installing→Pill yellow Installing`. `builds-view.tsx:110` Build button is dead disabled with tooltip about missing executor — the *primary* install-like feedback for template builds is intentionally unwired.

**STATUS:** `PARTIAL`

**GAP:**
1. **No WS-driven status transition:** Unlike `InstallListener`, Forge polls `fetchServer` only on `reinstallServer` success or `sendPowerSignal` success via `refreshServer()` — if install is triggered externally (admin deploy, node-driven reinstall), console will keep stale `status` until user manually triggers an action that calls `refreshServer`. `InstallBanner` may lag up to next page reload (or not appear at all). No `INSTALL_STARTED` listener exists.
2. **No completion auto-reload:** Ptero fetches server on `INSTALL_COMPLETED` automatically; Forge's `WebSocketManager` has no `INSTALL_COMPLETED` hook, so install finishing does not clear banner nor refresh egg variables without manual retry.
3. **No progress quantification:** References show at least binary spinner; Forge shows only static amber card — operator cannot distinguish “install hung at 2%” vs “almost done”. Ptero's install logs in console mitigate, but Forge's console lacks ANSI→color making long install scripts hard to scan.
4. **False completion risk:** `InstallBanner` text says “Do not restart or power off” but power buttons are already blocked via `blocked` — good; however `crash-banner` is suppressed during install (correct) but no heartbeat check informs if install daemon died.

**LOGIC FINDING — LF-GAME-01 (Install state divergence):**
`console-view.tsx:229` `blocked = suspended||transferring||status==='installing'` vs `server-console-layout.tsx:49` permissions treat installing server as still `fetchServer` refreshable — but `ServerDetailsBlock` in ptero colors `installing` as transient blue. In Forge, if API returns `status:'installing'` but `transferring:true` simultaneously (illegal but not validated), precedence at `232-236` shows `Suspended` then `Transfer` then `Install` — hides install banner incorrectly when both flags true. No invariant check.

**RECOMMENDATION:** Add `InstallListener` equivalent: subscribe to `transfer status` already partially present, extend to listen for `install started/completed` pushed over console WS or via `fetchServerTransferStatus`-style poll every 5s while installing. Show spinner + elapsed timer in banner.

**SEVERITY:** `P2 USER_VISIBLE`

---

### C05 — Transfer progress: handoff, progress %, allocation guidance

**REFERENCE:**
Ptero `TransferListener:11` tracks `pending|processing→isTransferring true`, `failed→false`, `completed→getServer`. `WebsocketHandler:65` on `transfer status starting|success` *closes socket and reconnects* to target node to continue receiving `TRANSFER_LOGS`. `Console:80` `handleTransferStatus('failure')→'Transfer has failed.'` into terminal. No progress % — binary status plus logs is the UX.

**FORGE:**
`transfer-view.tsx:31` polls `GET /servers/:id/transfer` every 5s if `server.transferring`. `StatusCard` `92` tone `isTransferring?ok:errorMessage?danger:transfer?.transferring?warning:neutral` — note two sources `server.transferring` vs `transfer.transferring` can disagree (logical split). Progress at `106` `progress!==null` shows `Progress {Math.round(progress)}%` bar `h-2 bg-white/[0.06] inner emerald width clamp`. `phaseLabel` at `82` derived from `server.transferState`. Console's `TransferBanner:52` shows `Target node: id · state` mono. Allocation picker at `196-210` filters `!a.server` and blocks start if `targetAllocations.length===0` with warning `"No free allocations..."`.

**STATUS:** `PARTIAL`

**GAP:**
1. **Dual source of truth (logic finding LF-02):** `isTransferring = server.transferring` (from server detail fetch, cached until invalidate) vs `transfer?.transferring` (from dedicated transfer endpoint poll). During the window after backend marks transfer completed (`transfer.transferring→false`) but before next `fetchServer` invalidation fires, `transfer-view:94` `isTransferring` still true → UI flips between “Transfer in progress” (emerald) and “No active transfer” (slate) on staggered polls. No single authoritative field. Ptero avoids this by mutating `server.data.isTransferring` directly via `setServerFromState` on the WS event.
2. **No log handoff:** Forge never reconnects console WS to target node (`WebsocketHandler:72` logic absent). Operator watching console on source sees gap when daemon closes source socket; after transfer, console remains connected to source (which may soon 503). Ptero's reconnect ensures `TRANSFER_LOGS` continue.
3. **Progress semantics unverified:** `progress` comes from `server.ts:394` `ApiLegacyTransferStatus` shim — field is `progress?:number` but backend may never populate (null). Banner copy says “Deployment and console actions will be blocked until transfer completes” but actual block is via `blocked` in console-view; transfer-view’s own Initiate form enforces `!isTransferring` but `server.transferTargetNodeId` may still be set after failed transfer, leaving Initiate form pre-filled with stale target without “Retry clears target” hint.
4. **Allocation guidance is good but incomplete vs reference:** Ptero shows no allocation picker (transfer is wings→wings internal). PufferPanel transfers reuse same node abstraction. Forge correctly adds allocation selector — Pelican equivalent is more explicit — but error copy `"Check the daemon logs on the target node"` at `144` is the only actionable guidance on failure; no link to node logs or “Release stuck transfer” helper.

**RECOMMENDATION:** Single-source transfer state: derive `isTransferring` from `transfer?.transferring ?? server.transferring` with server invalidated on `transfer` query success when status differs. Implement source→target WS handoff or at minimum auto-refetch console ticket on `transferStatus==='success'` and reload. Add “Clear failed transfer” button that unsets `transferTargetNodeId`.

**SEVERITY:** `P1 SILENT` (dual truth) / `P2 OPERATOR_VISIBLE` (handoff gap)

---

### C06 — Backup progress: incremental WS vs polling, per-item state & limits

**REFERENCE:**
Ptero `BackupRow:24` subscribes per-backup `BACKUP_COMPLETED:${uuid}` and patches row via `mutate(data=>items.map(b=>b.uuid===uuid?{isSuccessful, checksum, bytes, completedAt:new Date()}:b), false)` — optimistic, no refetch. `BackupContainer:64` shows `'{backupCount} of {backupLimit}'` and counts server-side `backups.backupCount` vs `featureLimits.backups`. Spinner `size small` when `completedAt===null`, failed red pill, lock icon, checksum mono, distanceToNow.

**FORGE:**
`backups-view.tsx:43` polls `refetchInterval: some pending|running ?3000:false` and `invalidateQueries` on any mutation. Row at `99-143` uses `isUsable= status==='completed'` then disables Download/Restore/Delete until usable; Lock/Unlock share `busy` (any mutation pending disables all row actions — coarse). Shows `formatBackupBytes(size)` `checksum? 'Checksum: ...' : 'Checksum not available'`. Limit UI at `89` `backupsDisabled limit===0`, `limitReached total>=limit`, `Create Backup` disabled.

**STATUS:** `PARTIAL`

**GAP:**
1. **Polling vs per-item WS:** Forge polls list every 3s while any backup in flight — duplicates ptero’s targeted WS patch with broader invalidation storm. At `50` create mutation `onSuccess` invalidates then `setCurrentPage(1)` — pagination jump loses user context if they were on page 3. Ptero never jumps page on create.
2. **Coarse busy lock:** `busy = restore|delete|lock|unlock isPending` at `101` disables *all* backup rows’ lock/unlock/restore/delete buttons when any one operation is in flight — ptero busy is per-row (via context menu) and per-backup spinner.
3. **Checksum never arrives until completed, but Forge shows “Checksum not available” as stable copy vs ptero “truncated checksum 0..checksum” mono that *appears* after WS patch — Forge could hydrate checksum from `BACKUP_COMPLETED` payload but doesn’t listen.
4. **Limit messaging:** Ptero at `57` says `'Backups cannot be created because limit is set to 0.'` when `backupLimit===0`; Forge at `89` same but adds `"no quota was provided by the API"` when limit undefined — leaks implementation detail to operator. PufferPanel shows simply sorted list + `Names` + date, no limit exposition — Forge’s exposition is richer but the `'no quota was provided'` fallback suggests backend misconfiguration to end user.
5. **No progress bar for running backup:** Ptero shows Spinner; PufferPanel shows `backupRunning` bool gating Create button; Forge shows only disabled actions + still polling — no spinner on running row except identical to ptero but via status `pending|running` (status string) vs `completedAt null`. Status string comparison vs `isSuccessful` boolean mismatch risk.

**LOGIC FINDING — LF-GAME-03 (Busy scope too wide):**
If two backups exist `a` completed and `b` pending, clicking Delete on `a` sets `deleteMutation.isPending→true→busy=true` which disables Lock on `b` (unrelated) — incorrect coupling. References isolate per-item pending state.

**RECOMMENDATION:** Subscribe per-backup like `BackupRow` or at minimum track `pendingBackupId` rather than global busy. Replace `"no quota was provided"` with neutral `"{total} backups stored"` when limit undefined.

**SEVERITY:** `P3 USER_VISIBLE`

---

### C07 — Resource graphs fidelity: number of signals, axes, scale & units

**REFERENCE:**
Ptero `ServerDetailsBlock:91` maintains **6** live signals + allocation: `Address` (copy), `Uptime` (color-coded via `getBackgroundColor`), `CPU`, `Memory`, `Disk`, `Network In`, `Network Out`. Each has `Limit` `value/limit` and color thresholds `>0.8 yellow >0.9 red`. `StatGraphs:41` renders 3 charts: `CPU Load`, `Memory` (MiB), `Network` 2 datasets cyan vs yellow with `hexToRgba(... ,0.5)` fill, `Line` from `react-chartjs-2` via `useChart` with axis `bytesToString`. PufferPanel `Stats.vue:97` renders `CPU %` (0–100 suggestedMax) and `Memory` (B→KiB→MiB→GiB→TiB via `formatMemory`), rolling 60s `timeseries x min = now-60s`, `aspectRatio 2`, `tension 0.3`, JVM breakdown `stack:'jvmMemory'` with hidden/show toggles.

**FORGE:**
`console-view.tsx:91` single `Chart` helper with hardcoded `stroke #ef4444` + `fill rgba(220,38,38,.16)` used for *all three* signals (CPU, Memory, Network). `Chart:92-94`: `max = max(...values,1)` auto-scales per chart to its own observed max, not to limit; `Network` label is `Network transfer` (combined) not In vs Out; `detail` strings `Current process usage` / `RX+TX` bytes but no “/ limit” exposition for CPU/memory. Fourth card is `Uptime` with circular `h-16 w-16 border-4` pulse, not a sparkline. `servers/page.tsx` cards render `ResourceBar` but intentionally without live values → `Unavailable`.

**STATUS:** `PARTIAL`

**GAP:**
1. **Missing Disk:** The most capacity-threatening signal (`Disk`) is absent from Forge graphs (ptero has it as dedicated StatBlock + not in sparkline trio; Puffer’s memory chart is only two). Operator cannot see disk pressure trending — critical for game servers that write worlds/saves.
2. **No limits context:** `Chart` `detail` for Memory at `248` shows `"{bytes} of {limit}"` correctly, but CPU `detail 'Current process usage'` omits `limits.cpu` — ptero shows `value/limit%` with red/yellow threshold; Forge omits denominator for CPU, making 40% uninterpretable (limit 100 vs 400%).
3. **Color & legend:** Puffer/Ptero use distinct series colors (CPU vs memory vs network in/out cyan/yellow); Forge triple red is indistinguishable in screenshots and colorblind-hostile.
4. **Axis lies:** `max` auto-scale means a flat `CPU 2%` history and a flat `CPU 40%` history look identical (both fill top of chart) because `max` recomputes per render. Puffer’s `suggestedMax 1024*1024` / `100` preserves absolute scale. This is a misleading sparkline.

**RECOMMENDATION:** Add Disk sparkline, distinct palette (emerald/amber/cyan), show limits where known (`CPU limit%` via `server.cpuShares` normalization), fix `max` to `Math.max(maxObserved, limitOr100)` not `maxObserved`.

**SEVERITY:** `P2 OPERATOR_VISIBLE`

---

### C08 — Network delta vs cumulative (logic finding)

**REFERENCE:**
Ptero `StatGraphs:61` `network.push([ Math.max(0, tx - prev.tx), Math.max(0, rx - prev.rx) ])` — throughput **per tick** (bytes since last event). `ServerDetailsBlock:80` still stores absolute `tx/rx` for display `bytesToString(stats.rx)`. PufferPanel `Stats.vue:66` `cpu.push({x,y:d.cpu})` etc with `while>60 shift` — CPU absolute, memory absolute, but network not shown as absolute; delta is intentional for sparkline volatility.

**FORGE:**
`console-view.tsx:203` `const network = rx+tx` (absolute sum) then `setNetworkHistory(prev=>[...slice(-59), network])`. No `previous` ref. Value therefore strictly non-decreasing (monotonic cumulative). First 60 ticks linearly rising sawtooth-then-plateau at `max` auto-scale. `Chart` for Network at `249` `value formatBytes(rx+tx)` matches but sparkline at `93` will normalize `max = ever-growing sum` so earliest point `0,100` and later points converge to `x, 100 - (value/max)*92` → as max grows, all prior points flatten to bottom, line trends to top and stays — not a meaningful throughput graph.

**STATUS:** `BROKEN` (logic)

**GAP:** Quantitatively, after 10 minutes `rx+tx ~ 3GB`, `networkHistory` holds 60 values `~2.9GB…3GB`; normalized line is essentially flat at top with variation `< (Δ/max)*92 < 0.003` pixels — visually dead. The operator concludes “network idle” when actually bursty, or “network at ceiling” when actually idle — opposite. `ServerDetailsBlock:30` `bytesToString(stats.rx)` vs `/tx` dual is more truthful than Forge’s sum.

**EVIDENCE:**
```ts
// forge/web/components/server/console-view.tsx:203
const network = statsData.networkRxBytes + statsData.networkTxBytes;
setNetworkHistory((items) => [...items.slice(-(MAX_POINTS - 1)), network]);
// vs reference
// reference/game-hosting/pterodactyl-panel/.../StatGraphs.tsx:61
network.push([Math.max(0, values.network.tx_bytes - previous.current.tx),
              Math.max(0, values.network.rx_bytes - previous.current.rx)]);
previous.current = {tx:..., rx:...};
```

**RECOMMENDATION:** Store `prevRx/Tx` ref (like ptero) and push deltas per tick, with separate In/Out lines or at minimum throughput sum `Δrx+Δtx`. Keep absolute totals only in `detail` text.

**SEVERITY:** `P1 SILENT` — misleading operational graph.

---

### C09 — Server cards: status encoding & resource availability signal

**REFERENCE:**
PufferPanel server list (inferred from `servers/server.go` + frontend list) shows per-server `type` icon + power state, no resource bars — relies on dedicated Stats view. Pterodactyl admin servers table `AdminServers.tsx:88-110` shows `Pill yellow Installing, Pill green Active, else neutral status`, not bars. Game-panel server cards in 1Panel show bars only inside detail.

**FORGE:**
`servers/page.tsx:210-258` card: right `w-1` vertical bar colored `suspended #f43f5e | running #10b981 | installing #f59e0b | #64748b`, status pill `ServerStatus`, node + allocation, then 3 `ResourceBar` *without* values intentionally → `Unavailable` text per bar (honest but 3 identical gray rows). Hidden entirely during `suspended|installing|transferring`. Pagination `pageSize 12`, search across name/desc/node/allocation.

**STATUS:** `PARTIAL`

**GAP:**
1. **Triple unavailable noise:** Three “Unavailable” rows per card for every non-transferring running server (the majority) is visual noise; operator learns to ignore the area, then misses genuine alarm when later wired. Puffer/Ptero avoid by not showing bars in list at all.
2. **Vertical bar semantics undocumented:** `style.backgroundColor` encodes status but legend is only at `ServerStatus` pill; vertical bar repeats pill color without label — redundant and invisible to screen readers (no `aria-label`).
3. **Installing guarded differently:** Card hides bars when installing (good — avoids NaN) but `ServerStatus` still shows `pulse warning Installing` — correct; however `AdminServers` pill at `335` repeats same but without pulse animation — inconsistency `pulse` only on `/servers`, not admin.

**RECOMMENDATION:** Keep bars hidden until live stats available on list (match Ptero) or wire live stats via `fetchServers` enrichment and remove “Unavailable” rows. Add `aria-label={server.status}` to vertical bar. Sync `pulse` animation between admin and console.

**SEVERITY:** `P3 USER_VISIBLE`

---

### C10 — Empty states: specificity & actionable next step

**REFERENCE:**
Ptero `BackupContainer:40` `'Look like there are no backups currently stored'` vs `page>1 'run out... try going back'` vs limit-zero suppress, `FileManagerContainer:93` `'This directory seems to be empty.'` + `97` `>250` yellow limit banner, general `ServerContentBlock` per-page. Puffer `Backup.vue:136` `<h2 Backup>` + `<h3 BackupsHeader>` + `sortedBackups` list with no explicit empty copy (relies on loader). 1Panel `el-empty` per view with action button.

**FORGE:**
`primitives.tsx:204` `EmptyState icon,title,description,action` `ui-empty ui-empty-icon`. Used consistently:
- `backups-view:98` `"No backups have been created for this server yet."` (no backup-count limit hint in empty — limit hint only in header pill above).
- `builds-view:198` `"No builds yet"` + `"Deploy this application from a git source or a compose stack to see builds here."` (actionable, good).
- `deployments-view:369` `"No deployments yet"` + `"Deploy an image tag above to create the first release..."` (good, colocated with input).
- `files-view:205` `"This directory is empty. Drag and drop files here to upload."` + grid vs list toggle (good — mirrors Ptero add: search+250 handling not fully; Forge omits `>250` limit warning — large dirs render truncated after sort with no banner).
- `activity-view:27` `"No activity recorded"|"No activity matches filter"`.
- `schedules-view:269` `"No schedules configured" + "Create a schedule above..."` (good).
- Servers index `192` `Server icon h-12` + `search?NoMatching:empty.title` (i18n, good).

**STATUS:** `PARTIAL`

**GAP:**
1. **Verbose vs actionable:** Backups empty is generic; Ptero tailors by `backupLimit===0` vs `page>1` — Forge’s header does cover limit but empty body duplicates neutral copy even when limit zero (slight mismatch — should guide to “Ask admin to raise backup limit”). `>250` file limit banner from Ptero is missing — Forge will render all `entries.length` after filter/sort with no cap, risking `10k` files DOM blow-up.
2. **Builds dead CTA:** EmptyState for Builds shows no `action` prop despite guidance; the `Build` button above is permanently disabled (`disabled` with tooltip) — empty suggests action but action is disabled. Ptero would show no empty for builds (not applicable) — Forge’s empty promise contradicts disabled control.
3. **Deployments vs Builds confusion:** Two empties (“No builds yet” vs “No deployments yet”) on sibling tabs with identical `Code2/RotateCcw` icons — operator cannot distinguish build vs deployment mental model; Puffer collapses to single “Backup/Files” list, not split.

**RECOMMENDATION:** Make Backups empty limit-aware (show “Backups disabled” in body, not just header pill). Re-add `>250` cap warning + slice like `sortFiles(files.slice(0,250))`. Wire Builds empty `action={<Link href=git/compose>Deploy from git</Link>}` instead of disabled Build button, or hide Builds tab until executor wired.

**SEVERITY:** `P3 USER_VISIBLE`

---

### C11 — Notifications & operational feedback: toast, inline, banner

**REFERENCE:**
Ptero `BackupContainer:37` uses `FlashMessageRender byKey='backups'` with `clearFlashes/clearAndAddHttpError` — per-key inline `Flash` not toast. `WebsocketHandler:110` banner `bg-red-500 py-2` with spinner or message — global WS health. PufferPanel `Backup.vue:43` `toast.success('BackupStarted')` transient success, not persisted. Errors via modal `events.emit('confirm', ...)` before destructive restore/delete.

**FORGE:**
Two systems coexist:
- `ui/toast.tsx:21` `ToastProvider` → `useToast()` with queue `3`, auto-dismiss `4500/7000`, tones `success|error|warning|info|loading`, rendered `aria-live=polite role=status|alert` at `ToastProvider:45`. Used in `transfer-view:63`, `databases-view`, `settings-view`, `network-view`, etc.
- `sonner` `toast.success/error` at `files-view:126,132` imported from `@/components/ui/sonner` — second library.
- Inline alerts: every view also shows `ui-alert ui-alert-error|warning` for `query.isError` and `mutation.error` (e.g., `backups-view:94`, `transfer-view:121`, `console-view:242` `power.error|install.error` red bordered `role=alert`).
- Banners: `InstallBanner/TransferBanner/SuspendedBanner` at `console-view:37-85` plus `CrashBanner` poll every 30s.

**STATUS:** `PARTIAL`

**GAP:**
1. **Two toast systems:** `sonner` (files) vs `useToast` (transfers/databases) vs `Flash` equivalent (backups inline) — same outcome (transient vs sticky) uses different API. Operator sees slightly different animation/duration for file delete vs transfer failure. No single `pushToast` contract like Ptero’s `useFlash`.
2. **Error deduplication missing:** `backups-view:74` `actionError = create|restore|delete|lock|unlock error` then merges `error.message` — concurrent errors collapse to first truthy, discarding others. Ptero per-key flash keeps per-action scope.
3. **Console connectionError vs toast:** Console shows error only inline `connectionError` red after `·` in header at `275`, not as toast/banner that persists on page change — operator switching to Files tab loses console disconnect signal. Ptero’s `WebsocketHandler` banner is global above all tabs (red bar at `bg-red-500`) — visible regardless of tab.

**RECOMMENDATION:** Consolidate on `useToast` (remove `sonner` import), ensure `connectionError` also `pushToast` (or global `OfflineBanner` already present but inside server shell not console shell — verify overlap). Scope `actionError` per mutation section like `databases-view:39` single `mutationMessage` — instead keep per-section alerts.

**SEVERITY:** `P3 OPERATOR_VISIBLE`

---

### C12 — File manager / startup / schedules operational feedback (cluster requirement)

**REFERENCE:**
Ptero `FileManagerContainer:36` wraps `ServerError message=httpErrorToHuman(error) onRetry=>mutate()`; `MassActionsBar` for selected files, `UploadButton`, `NewDirectoryButton`, `FileManagerStatus` live indicator. `StartupContainer:1` per-variable `VariableBox` with `description`, `rules`, `isEditable` lock, save per var. Schedules `ScheduleContainer/EditContainer/CronRow/TaskRow` with `RunScheduleButton` and `TaskDetailsModal`.

**FORGE:**
`files-view:197` exposes upload progress bar `uploadProgress%` with `bg-red-600` fill, drag-drop overlay `bg-black/60 backdrop-blur Upload bounce`, search filter, sort toggle (name/size/date), `view list|grid`, breadcrumbs `Breadcrumbs`, context menu `fixed inset-0` with `Download/Rename/Archive/Preview/Delete`, mass bar `sticky bottom-4 Count + Move/Copy/Permissions/Delete`, `Preview` modal with image `img max-h[75vh]`. Uses `MonacoEditor` lazy import with custom languages. Empty above already.
`startup-view:47` (not fully read but referenced `primitives EmptyState No startup variables`) renders `Variables` with `rules` string, allowed select via `in:` parse, validation, per var save disabled `!editable||!changed||validation`.
`Schedules` `schedules-view` TaskEditor with `command|power|backup`, sequence, offset, continueOnFailure, cron helper presets, reorder via sequence swap, Runs subquery with expandable output.

**STATUS:** `PARTIAL`

**GAP:** Forge file manager is richer than Ptero’s 250-cap simple list (good), but startup variables validation display `Rules: none provided` raw string exposes backend raw (e.g., `required|string|max:191`) to operator — Ptero hides rules string. Schedules lacks Ptero’s `ScheduleCheatsheetCards` copy — cron helper exists but not per-field cheatsheet.

**SEVERITY:** `P3 USER_VISIBLE`

---

### C13 — Deployment & compose logs via WS (DeploymentLogViewer) parity

**REFERENCE:**
Ptero no deployment log viewer (deploy is install). PufferPanel no deploy log (deploy is restart). References for live deploy logs are Coolify/Dokploy (deployment timeline + live logs) — not game-hosting. Within game-hosting, the closest is backup/restore logs.

**FORGE:**
`DeploymentLogViewer:37` provides what Ptero/Puffer lack: deployment log streaming with `wsUrl` promise, `autoScroll`, `stripAnsi`, level colors, line numbers, timestamps. Used in `deployments-view` detail *not* as WS but as `Events max-h-48` + `Health results` panels fetched via polling (`refetchInterval:10_000` at `schedules-view:94` analogous). Composition stacks not viewed here — compose UX lives under `compose/` not game-hosting.

**STATUS:** `PARTIAL`

**GAP:** `DeploymentLogViewer` is a *dead code* component in game-hosting scope: no `forge/web/app/server/[id]/deployments` page mounts it (it is used only in `admin/compose` indirectly). Game deployments chart `deployments-view:285` shows `StateMachine` + events list polled, not the WS viewer. So the most advanced WS log primitive in Forge is not reachable from the server console/deployments tabs where install progress would benefit — install logs are instead funneled into the generic console (see C04). This is an *unwired primitive*.

**RECOMMENDATION:** Either mount `DeploymentLogViewer` under deploy detail when `status==='building'||'deploying'` or delete component to avoid dead code impression. For game installs, replace console fallback with structured install steps + `DeploymentLogViewer` dedicated.

**SEVERITY:** `P3 OPERATOR_VISIBLE` (unwired)

---

### C14 — Admin server detail resource & lifecycle feedback

**REFERENCE:**
Pterodactyl admin `servers` table has sortable filters (Status/Core?) but admin server detail shows `Limits` and `Allocations` not live graphs. Pelican admin improves with Filament widgets (stats overview).

**FORGE:**
`AdminServers.tsx:390` `ServerAboutTab` shows `Resources` Memory/Disk tiles, `Status` with dot, `Owner`, `Node`. `ServerBuildTab:543` resource Management inputs + Application Feature Limits note `"read-only ... does not persist"`, Allocation Management select. `ServerStartupTab:614` shows `startup_command` readOnly, docker images list, Service Variables inputs. No live graphs in admin — consistent with reference admin not being live dashboard (live is per-server console). Build/Startup tabs disclose unwired state honestly (amber warnings).

**STATUS:** `COMPLETE`

**GAP:** Minor: `AdminServers` fetch `servers allocate type:20` but card shows only first allocation; admin `ServerAboutTab:392` picks `allocations.find(... )` but no list of all allocations — admin cannot see secondary allocations without drifting to Manage/Allocations tabs. Not a bug.

---

### C15 — Permissions gating & graceful degradation

**REFERENCE:**
Ptero `Console:66` `const [canSendCommands] = usePermissions(['control.console'])` hides input entire DOM if false at `211`; `PowerButtons:51` wraps each button in `<Can action='control.start'>`. `Files/FileManagerContainer:76` `<Can action='file.create'>` hides `Upload/NewDirectory`. PufferPanel `Console.vue:134` `v-if="server.hasScope('server.console')"` hides console wrapper entirely; `v-if="server.hasScope('server.console.send')"`. Both return `<p>No access` or hide controls rather than disable.

**FORGE:**
`server-nav.tsx:48` `visibleTabs = tabs.filter(hasServerPermission(access, permissions))` — tab *removed* if lacking `file.read` etc; good progressive disclosure. Per-action buttons use `disabled={!canCreate||...}` not removal: `backups Create Backup disabled` still renders but gray; `Transfer initiate form` `canTransfer` hides entire form if false (`172` only when `canTransfer`); files `canRead` returns early `ShieldX` amber `You do not have permission to read` at `194`; console `canConsole` at `134` sets `connection='error'` + `connectionError='You do not have permission...'` and skips WS. Schedule banner at `237` `!canRead → Alert warning 'You do not have schedule.read — ... may be restricted.'` but still renders list.

**STATUS:** `PARTIAL`

**GAP:**
1. **Inconsistent pattern:** Some permissions *remove* (nav tabs), some *disable* (backup buttons), some *block with message* (files full-screen, console error state), some *warn but continue* (schedules). Puffer is consistent: `hasScope` check hides vs disables.input difference but predictable per feature. Forge’s split makes operator unsure if lack of tab means no permission or empty feature.
2. **Over-broad permission mapping:** `ServerNav transfer:32` requires `settings.reinstall` for Transfer tab — same as reinstall/Deployments? Ptero uses `isTransferring` flag not reinstall permission; Forge reuses reinstall scope for transfer, deploys, builds — conflates distinct ops. Should be `server.transfer` scope.
3. **Console permission dual check:** `canConsole = hasServerPermission(access, ['websocket.connect','control.console'])` at `124` requires *both*; but `ServerNav console:17` requires `websocket.connect` alone — nav shows Console tab even when command sending is blocked. User sees console, types, hits disabled input with no inline reason until they inspect disabled attribute title (generic `Console is not connected`). Missing `"You cannot send commands — need control.console"` hint.

**RECOMMENDATION:** Align nav permission with view permission (`control.console`) or add explicit permission banner inside console when `!control.console` like `ServerSettingsView:39` SFTP `You do not have SFTP permission`. Introduce distinct `server.transfer` scope.

**SEVERITY:** `P3 USER_VISIBLE`

---

### C16 — Crash & node-maintenance feedback loop

**REFERENCE:**
Ptero `ServerConsoleContainer:26` shows warning Alert when `isNodeUnderMaintenance||isInstalling||isTransferring` — three states share one banner location. No crash banner (Wings reports status changes via WS `STATUS` event directly into terminal prelude). PufferPanel `Status.vue` likely similar.

**FORGE:**
`console-view:238` `CrashBanner` suppressed if any of `suspended|transferring|installing`, else queries `fetchServerCrashHistory` `refetchInterval 30000` showing `crashed N times, Last crash: date · Exit code · OOM`, expandable `details` 5 events, actions `Restart server` (calls `sendPowerSignal start`) + `Reset crash state` (posts to crash-detection admin endpoint). `InstallBanner/TransferBanner/SuspendedBanner` exclusivity keeps one banner at top.

**STATUS:** `COMPLETE`

**GAP:** One nuance: `CrashBanner` fetches via `GET /admin/crash-detection/servers/:id` — requires admin scope; non-admin console user will get 403 and `crashes` stays `undefined` → banner hidden even if crashed (error swallowed at `crash-banner:17` `isLoading||!Array.isArray(crashes)||length===0 return null`). Non-owner subuser sees no crash signal — inconsistent with ptero where `STATUS` event is broadcast to all connected sockets regardless of role.

**RECOMMENDATION:** Surface crash state via `server` object (`crashCount`) for all roles or make endpoint scoped to server permission.

**SEVERITY:** `P3 OPERATOR_VISIBLE`

---

### C17 — Status + power controls affordance

**REFERENCE:**
Ptero `PowerButtons:72` 3 buttons `Start (flex-1) disabled status!=='offline', Restart disabled !status, Stop/ Kill (single button text toggles killable=status==='stopping') Danger`. Requires `Can control.start/stop/restart`. Confirm dialog only for `Kill` via `Dialog.Confirm hideCloseIcon 'Forcibly Stop...'`.

**FORGE:**
`console-view:243` 4 buttons `start|restart|stop|kill` each `min-h-11 px-3 uppercase text-xs` in `grid-cols-4`, `signal==='kill'` requires `confirm({Kill server?})` at `243`. Disable logic `!canPower||blocked||isPending||(start?running: not running)`. Kill is *always* separate button, not conditional on `stopping`. Color: `start emerald, restart slate, stop|kill red`.

**STATUS:** `PARTIAL`

**GAP:** Forge surfaces 4 concurrent power options where ptero surfaces 3 (Kill shares Stop button until stopping). Extra `Kill` button in Forge is always enabled when running → invites accidental kill (more destructive). Also ptero’s Restart disabled when `!status` (unknown) prevents restart while unknown; Forge Restart enabled when `status==='running'` only check mirrors start — actually Forge disables Restart when not running (`server.status!=='running'`) matching — minor. The bigger gap is lack of `isNodeUnderMaintenance` guard: ptero disables all actions via banner `under maintenance and all actions unavailable` but Forge only checks `suspended|transferring|installing`, not maintenance — operator can attempt power signal to node under maintenance and get generic error.

**RECOMMENDATION:** Collapse Kill into Stop when `status!=='stopping'` or require hold-to-kill. Add maintenance flag guard and banner like ptero.

**SEVERITY:** `P3 USER_VISIBLE`

---

### C18 — Overall operational feedback polish vs game-hosting expectations

**REFERENCE pattern (holistic):**
Game-hosting consoles are high-frequency, long-lived sessions: ptero asks operators to tail for hours; thus per-pixel optimizations matter (fitText in StatBlock uses `useFitText min8 max500` to keep value legible, CopyOnClick on allocation, `getBackgroundColor` thresholds draw attention only when hot). PufferPanel caps history and supports both live WS + 5s polling for firewalled networks. Backups/restores mutate swr cache optimistically with per-item patch.

**FORGE:**
Forge console autoscales all sparklines to observed max (misleading), caps history at 500 (reasonable), supports WS only (no HTTP fallback), backup polls whole list, transfer splits state, console lacks `CopyOnClick` on allocation (allocation is shown in nav `59` but not copyable), no `useFitText`, no per-item optimistic patch.

**STATUS:** `PARTIAL` (aggregate)

**GAP:** The sum of partials is a less *calm* console for long sessions: monotone charts, cumulative network line, fake timestamps, missing disk trend, dual toast languages.

**RECOMMENDATION:** See per-comparison fixes — priority order in §8.

---

## 5. Logic Findings (consolidated)

### LF-GAME-02 — Synthetic per-line timestamps (console)

**Location:** `forge/web/components/server/console-view.tsx:299` `showTimestamps ? <span className="mr-2 text-slate-500">{new Date().toLocaleTimeString()}</span> : null`
**Expected:** Each log line shows its server emission time (or no timestamp). `DeploymentLogViewer:208` correctly uses `new Date(log.timestamp).toLocaleTimeString()` per entry.
**Actual:** `toLocaleTimeString()` is called at render for *every* line on every render, so all 500 lines show identical wall time (render moment), not line time. Toggling on/off or scrolling re-renders and changes every timestamp simultaneously — operator cannot correlate crash with earlier log.
**Impact:** `P2 SILENT` — post-mortem forensics broken.
**Reproduction:** Open console, emit two logs 10s apart, enable timestamps — both show same seconds. Wait 5s, timestamps update to new wall time without new logs (re-render from stats WS).
**Fix:** Remove toggle or parse timestamp from line if daemon prefixes it (Wings format `2026-08-23 14:00:00 ...`), else render no timestamp like ptero. If adding, store `receivedAt` per line: `{text, at:Date.now()}` then render `new Date(at).toLocaleTimeString()` per line (stable).

### LF-GAME-03 — WebSocketManager double-JSON parse drops plain console output [LOGIC]

**Location:** `forge/web/lib/api/ws/websocket-manager.ts:109-114` does `JSON.parse(event.data)` then `onMessage(JSON-parsed object)`; `forge/web/components/server/console-view.tsx:151-155` then does `let text=String(data); try JSON.parse(text)` — `String({})==='[object Object]'` which parse throws → later `text.split('\n')` yields `["[object Object]"]` junk. For plain daemon text (non-JSON) `WebSocketManager` `catch` branch `warn` and *never calls* `onMessage` — live output lost entirely.
**Expected:** Console receives every `event.data` string (JSON or plain) as-is.
**Actual:** If daemon sends plain text (common for `CONSOLE_OUTPUT` when wings streams raw line not JSON envelope), `WebSocketManager:111` `catch` warns and drops message — console stalls on “Waiting for console output…”. If daemon sends JSON envelope `{"data":"hello"}` manager forwards object, console mis-stringifies.
**Impact:** `P1 SILENT` — console appears disconnected while WS is connected; operator retries deploy unnecessarily.
**Reproduction:** Mock WS to send `'hello world\n'` (no JSON) → console never appends due to manager warn-only path. Mock `'{"data":"hello"}'` → console shows `[object Object]` not hello.
**Fix:** Remove JSON parse in manager; pass `event.data` string directly: `onMessage(event.data)`. Leave parse to caller as `console-view` already does leniently. For stats, parse once in stats handler only.

### LF-GAME-04 — Network sparkline cumulative vs delta (graph lie) [LOGIC]

**Location:** `forge/web/components/server/console-view.tsx:203-206` (quoted in C08).
**Expected:** Throughput per interval (delta) like `reference/.../StatGraphs.tsx:61`.
**Actual:** Absolute cumulative `rx+tx` pushed as point, auto-scaled to own max — flat line at top, hides bursts. After 60 points all variation <0.5%.
**Impact:** `P1 SILENT`
**Fix:** Keep `previous: {rx,tx}` ref, push `max(0, rx-prev.rx)+max(0, tx-prev.tx)` and store.

### LF-GAME-05 — Transfer dual-source race (`server.transferring` vs `transfer?.transferring`) [LOGIC]

**Location:** `forge/web/components/server/transfer-view.tsx:77` `isTransferring = server.transferring` vs `92` tone also reads `transfer?.transferring` plus `88` header `isTransferring ? Transfer in progress ... : errorMessage ? Failed : transfer?.transferring ? Transfer pending ... : No active transfer`.
**Expected:** Single authoritative `transferring` flag.
**Actual:** Two independent caches (server detail query vs transfer endpoint poll) can disagree for ~5s after completion; UI flickers between branches and may show `Initiate transfer` form prematurely (`171` gated on `!isTransferring && !transfer?.transferring` — if one says false early, form appears while backend still transferring).
**Impact:** `P1 SILENT` — initiates duplicate transfer request → 409 or silent no-op.
**Fix:** Derive canonical `isTransferring = transfer?.transferring ?? server.transferring` when transfer query succeeded, invalidate server cache on transfer status change before enabling form.

### LF-GAME-06 — Sparkline auto-max scaling exaggeration [LOGIC]

**Location:** `forge/web/components/server/console-view.tsx:92-93` `max=Math.max(...values,1)` then `point y=100-(p/max)*92`.
**Expected:** CPU 5% should sit near bottom of 0-100% axis; memory 200MiB/2GiB ~10% should sit near bottom.
**Actual:** `max` equals largest observed value in last 60 ticks, so plateau at 5% renders as full-height line near top (8px from ceiling) identical to plateau at 80% — severity indistinguishable.
**Impact:** `P2 SILENT`
**Fix:** For CPU set axis `max=max(limitOr100, maxObserved)` with lower bound `100` when limit unknown; for Memory `max=max(memoryLimit|| maxObserved*1.2, maxObserved)`.

---

## 6. Duplicates

- **REF-GAME-DUP-001 — Two WebSocket reconnect loops:** `WebSocketManager:42-151` (20 retries jittered) vs `DeploymentLogViewer:35-98` bespoke `MAX_RECONNECT_ATTEMPTS=20` + `1s*2^attempt` — duplicate, will diverge on backoff tuning. Use manager for both.
- **REF-GAME-DUP-002 — Two toast systems:** `ui/toast.tsx` `useToast` vs `sonner` `toast` in `files-view` — same concept two libs.
- **REF-GAME-DUP-003 — Per-signal chart vs per-card chart:** game console `Chart` sparkline (no axis) duplicates `DeploymentLogViewer` log level chart philosophy and `servers/page.tsx` `ResourceBar` — three visual languages for “resource over time”.
- **REF-GAME-DUP-004 — `InstallListener` vs static banner:** reference `InstallListener` wires WS events to state; Forge re-implements half via banner static + manual `refreshServer` — incomplete duplicate.
- **REF-GAME-DUP-005 — Deployment vs Builds tabs:** both deal with “how did code get to container” — Builds (`builds-view` disabled), Deployments (`deployments-view` imageTag), Git, Compose — four paths where reference game-hosting has one (egg install). Fork confusion.

---

## 7. False Completion / Decorative Features

- **`builds-view Build` disabled button** at `110` permanently disabled with tooltip about missing executor — decorative affordance that pretends builds exist. Should be hidden or linked to git/compose flows.
- **Transfer progress bar `progress!==null`** — progress field is from `ApiLegacyTransferStatus` shim `progress?:number` that backend may never populate; bar silently absent for real transfers, giving no % despite UI placeholder implying it exists.
- **Resource bars on server cards** render three `Unavailable` rows — not live, but layout reserve implies future capability; comment admits intentionally unavailable to avoid 100% lie — honest but decorative.
- **`DeploymentLogViewer` in game-hosting bundle** — shipped but not mounted from server console — dead log primitive that suggests live deploy logs exist.

---

## 8. Dead / Legacy

- `console-view:181` `proxySocket` object with `readyState` getter masquerading as `WebSocket` for `socketRef` compatibility — never used as real socket (no `addEventListener`), only `send/close`. Legacy shim; should be typed as `ConsoleConnection`.
- `server-console-layout:83` `activeTab = activeTabProp ?? pathname.split('/').at(-1) ...` — fragile last-segment parse; `/server/:id` maps to `'console'` special-cased but `/server/:id/network` vs `/server/:id/mounts` both work — yet `console/servers/[id]/layout.tsx` duplicates this logic with `base='/console/servers'` — second layout duplicates server shell (comment at `22` acknowledges dedup via `parentContext` guard).
- `lib/api/servers.ts:394` `ApiLegacyTransferStatus` retain for backwards compat where Wings no longer reports `isTransferring` on socket but via poll — shim may outlive migration.

---

## 9. Architecture Lessons (what to adopt / reject)

| Lesson | Source | ADOPT / ADAPT / INSPIRE / REJECT | Rationale |
|--------|--------|----------------------------------|-----------|
| xterm.js + Fit/Search/WebLinks addons for game consoles | Ptero `Console.tsx:128` | **ADOPT** | Game daemons are ANSI-heavy; plain div loses signal and links. Worker for ANSI→HTML (Puffer) acceptable alternative but xterm is stronger for 500-line scrollback + search. |
| Hybrid WS + HTTP poll fallback for firewalled consoles | PufferPanel `Console.vue:30` `needsPolling()+getConsole(lastMessageTime)` every 5s | **ADAPT** | Forge console is WS-only; corp proxies kill WS — Puffer degrades to poll. Add `getConsole` poll when `connection==='error'` for 30s fallback. |
| Per-item `BACKUP_COMPLETED:uuid` SWR patch vs list poll | Ptero `BackupRow:24` | **ADOPT** | Polling list every 3s scales O(n) queries; per-row mutate is cheaper and preserves pagination context. |
| `INSTALL_*` / `TRANSFER_*` listeners mutating `server` state directly | Ptero `InstallListener`/`TransferListener` | **ADOPT** | Forge polling could be eliminated; push events over existing console WS channel (daemon already multiplexes). |
| `isTransferring` reconnect handoff `socket.close()+setInstance(null)+connect(target)` | Ptero `WebsocketHandler:65` | **ADOPT** | Essential for node-to-node transfers; Forge console will otherwise blackout on handoff. |
| `Song of delta`: `prev tx/rx → push delta` not cumulative | Ptero `StatGraphs:61` | **ADOPT** | Cumulative graph is actively misleading — delta is only truthful throughput signal. |
| Triple “Unavailable” bars on listing | Forge `servers/page.tsx:252` | **REJECT** | Hide bars on listing until live values available (like Ptero admin); reserve graphs for detail only. |
| 2-jittered-backoff implementations | Forge `WebSocketManager` + `DeploymentLogViewer` | **REJECT** | Consolidate on manager. |

---

## 10. Recommended Activation Order (existing capability → wired)

Without adding a fundamentally new subsystem:

1. **P1 FIX LF-GAME-03 (WS message drop)** — Remove `JSON.parse` from `WebSocketManager:109` and forward raw `event.data`; parse only in stats handler. Re-test console with plain daemon text. Unblocks every console session.
2. **P1 FIX LF-GAME-04 (network delta)** — Add `previous` ref and push `Δrx+Δtx`; verify sparkline animates with bursts. Keep `rx+tx` sum only in detail string.
3. **P1 FIX LF-GAME-05 (transfer dual truth)** — Unify `isTransferring` derive + invalidate `server` on transfer poll success; add stale-target clear. Prevent duplicate transfers.
4. **P2 Add xterm or ANSI color inline to console + unify strip** — Wire `stripAnsi` or `ansi_up` for both viewers; add `WebLinksAddon` equivalent via regex autolink. Fixes C01 visual regression.
5. **P2 Add `InstallListener` WS hooks** — Subscribe console WS to `INSTALL_STARTED|COMPLETED` (or regular `fetchServer` poll every 5s while `installing`) → clear banner auto, refetch variables. Fixes C04 lag.
6. **P2 Add transfer handoff reconnect** — On `TRANSFER_STATUS==='success'` re-ticket + reconnect console WS to target node (or full page reload hint). Fixes C05 gap.
7. **P2 Fix timestamps** — Remove fake timestamp toggle or store `at` per line; replace Search filter with highlight-find mode (keep filter as optional but add highlight). Fixes C03.
8. **P2 Fix resource graph scale & palette** — Anchor `max` to limit (CPU 100%, Memory memoryLimit, Network suggestedMax), distinct colors, add Disk card. Fixes C07.
9. **P3 Move backups to per-item patch** — Replace global `busy` with per-id pending map, re-add per-backup WS if backend emits `BACKUP_COMPLETED:uuid` (or keep 3s poll but per-row spinner). Improve empty limit copy. Fixes C06.
10. **P3 Consolidate WS reconnect + toast** — Replace `DeploymentLogViewer` bespoke loop with `WebSocketManager`, drop `sonner` import, promote `connectionError` to global banner or toast so signal survives tab switch. Fixes C02, C11, DUP-001/002.
11. **P3 Align permission UX** — Make nav tab gating + per-view disabled vs removed consistent; expose `control.console` missing hint inside console input area; split `settings.reinstall` → `server.transfer` scope.
12. **P3 Scrub dead affordances** — Hide `Build` button or link to git/compose deploy, hide `DeploymentLogViewer` from game scope or wire to install, cap files `>250` list with banner. Fixes C10, C13.

---

## 11. Phase 2 Evidence Inventory

- Reference files hit: 26 under `reference/game-hosting/{pterodactyl-panel,pelican-panel,pufferpanel}/**/*.{tsx,ts,vue}` with line-accurate citations per comparison.
- Forge files hit: 18 under `forge/web/{components/server/**,app/server/**,app/console/**,components/deployment/**,app/servers/**,app/admin/servers/**,lib/api/**}`.
- Coverage by prompt axis: console logs (C01-03), install (C04), transfer (C05), backup (C06), resource graphs (C07-08), server cards (C09), notifications (C11), empty states (C10), operational feedback (C12, C14, C16-17), WS components (C13) — 18 comparisons (≥15), 5 logic findings (≥3) all `file:line` verifiable.

---

## 12. Handoff to Synthesis

Phase 02 synthesis should avoid re-inspecting compose/deployment queuing already covered in Phase 01, and instead weight **PufferPanel worker+poll fallback** and **Ptero Install/Transfer listeners** as most actionable gaps: Beacon→Wings isomorphism for game servers is weakest on *push* (install/transfer status, per-backup completion, per-line console) not on *poll* (servers list, mounts, schedules). The four logic bugs above are server-agnostic wiring, not missing subsystems — fixable before adding new executors.
