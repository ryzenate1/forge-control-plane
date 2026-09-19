# Scope 6 — Infrastructure B: host access + alternative runtimes

Audited: 9 registry routes · 9 `page.tsx` (all nine routes contain exactly one file — no
`[id]/`, `new/`, `loading.tsx` or `error.tsx` anywhere in the slice) · 5 delegated component files
(`host-files-view.tsx` 773, `docker-cleanup-manager.tsx` 586, `AdminSftp.tsx` 341,
`AdminKubernetes.tsx` 139, `node-select.tsx` 60) · the Go/Beacon sides of every endpoint they call.
Frames in use: DashHeader 0 · AdminPageHeader 2 (`files`, `terminal`) · SectionHeader 7 (`host`,
`sftp`, `docker-cleanup`, `kubernetes`, `incus`, `nomad`, `netbird`) · hand-rolled 0 · ForgePage 0.
**The frame is the one thing this slice gets right; the dishonesty is inside the frames.**

Read as reference: `components/admin/AdminOverview.tsx`, `components/admin/admin-ui.tsx`
(`SectionHeader` :163-265, `AdminPageHeader` :280-314, `CardHeader` :349, `AdminTabs` :520,
`SubsystemHealthMeter` :80, `AdminStatCard` :647).

## Structural discovery that applies to the whole scope

**`AdminPageHeader` accepts no `info` prop, and renders no `PageInfoDisclosure`.** The only
definition is `components/admin/admin-ui.tsx:280-314`; its prop type is
`{ title, description, action, backAction, backLabel, breadcrumb, hideBreadcrumb, status, className }`
(and `SectionHeaderProps` at `components/admin/admin-ui.tsx:140-162` matches it). `PageInfoDisclosure`
is rendered only by `AdminOverview`, `AdminServers`, `AdminMonitoring`, `AdminActivityLog`,
`AdminHealth` — imported directly, never through a header prop.

Eight pages in this scope pass `info={{…}}` to `AdminPageHeader` and therefore believe they render an
"About …" architecture disclosure that **never appears**:
`app/admin/files/page.tsx:13`, `app/admin/terminal/page.tsx:272`,
`components/admin/AdminKubernetes.tsx:42`, `components/admin/AdminSftp.tsx:75`,
`components/admin/docker-cleanup-manager.tsx:260`, `app/admin/incus/page.tsx:78`,
`app/admin/nomad/page.tsx:72`, `app/admin/netbird/page.tsx:302`.
Object literals in JSX are excess-property-checked, so each of these is also a `tsc` error site. This
is the mechanism by which "capability not available" explanations were written and then silently lost.

**No route in this scope has `loading.tsx` or `error.tsx`.** `app/admin/{host,files,terminal,sftp,
docker-cleanup,kubernetes,incus,nomad,netbird}/` each contain exactly one `page.tsx`, so a thrown
resolver error lands on the app-level boundary instead of `AdminErrorState`.

**`NodeSelect` resolves an ambiguous node target in the UI.**
`components/admin/node-select.tsx:11-15` `pickDefaultNode` returns "first active node, else first
node", and `:26-30` an effect fires `onChange(pickDefaultNode(nodes))` the moment nodes arrive, with
no user interaction. AGENTS.md: *"Never resolve an ambiguous target silently. If a request omits the
node it refers to, reject it; do not pick the first one that happens to have credentials."* The
backend was already fixed to enforce this — `forge/api/internal/http/handlers_host.go:44-55` and
`forge/api/internal/http/handlers_files.go:40-43` both return **400 `nodeId is required`** when
`nodeId` is absent, and the comment at `handlers_host.go:41-49` explicitly records the old
first-node fallback as the bug that was removed. The UI still performs the same fallback client-side,
so the policy holds in Go and is broken in React. Consequences per page below.

---

## Per-page findings

### Host Inspector — `/admin/host`
- files: `app/admin/host/page.tsx` (403 lines, self-contained, no delegated component)
- frame: SectionHeader (via `AdminPageLayout`)
- title: **"Host Management"** (`app/admin/host/page.tsx:351`) vs registry **"Host Inspector"**
  (`components/admin/admin-registry.ts:158`) → **MISMATCH**. The sidebar row, the shell breadcrumb and
  the `<h1>` disagree on the same screen; and "Host Management" collides with `/admin/nodes`
  ("Nodes — Beacon hosts…", `admin-registry.ts:150`).
- description: registry "Per-node system, disk, memory and process inspection" is never shown; the
  page invents a node-dependent subtitle instead (`app/admin/host/page.tsx:344-346`).
- icon: registry `ServerRackIcon` is not rendered anywhere; the page renders no header icon. The
  `CardHeader` icon changes with the active tab (`:387-389`).
- **S1 · every tab silently reads a node the operator did not choose** —
  `app/admin/host/page.tsx:331-335` auto-assigns `pickDefaultNode(nodes)` on load, so the page
  immediately renders disk/memory/process data from the first *active* node even when several nodes
  exist and the URL names none. Data is real, but attributed to a machine the user never selected.
- **S1 · the "Real-time system data" promise is not what the page does** —
  `app/admin/host/page.tsx:345` claims "Real-time"; the queries poll at 30 s
  (`:74, :112, :148, :183`) and processes at 10 s (`:224`). There is no `FreshnessBadge`
  (`status` prop of `SectionHeader` is never passed, `:350-358`); freshness is a hand-rolled
  `Updated {…} ago` `<p>` per tab (`:103, :139, :176, :214, :281`) whose `fmtTime` (`:50-57`)
  rounds to "just now" and never distinguishes *stale* from *fresh* from *failed*. AGENTS.md:
  "a stale reading is not a healthy one".
- **S1 · `No disk data` / `No memory data` / `No network data` empty states cannot fire when the value
  is unmeasured, only when the array is empty** — `:120`, `:195`, `:232` gate on
  `data.length === 0`, and `:84` on `!data`; but per-field absence renders as a measured number:
  `fmtMB(part.usedMb)` (`:134`) and `usageBar(part.usedPercent)` (`:131`) print `0 MB` / `0.0%` and a
  green "healthy" bar for a partition Beacon never measured (`DiskPartition` fields are non-optional
  in `lib/api/host.ts:29-36`, so a missing JSON field arrives as `undefined` →
  `part.usedPercent.toFixed` throws, or renders 0 after a partial payload). `StatRow`
  `CPU Cores: String(data.cpuCores)` (`:97`) renders the literal string `"undefined"`, and
  `Time: new Date(data.time).toLocaleString()` (`:100`) renders `"Invalid Date"` — both are shown as
  values, not as unknowns. Contrast `fmtUptime` (`:304-317`), which in this same file *does* return
  "Not reported": the honest treatment exists here and is applied to exactly one of seven fields.
- **S1 · interface status is a two-state colour from a free-text field** — `:204-206` colours
  `iface.status` emerald if it equals `"up"` and **red otherwise**, so `""`, `unknown`, `down`
  (administratively disabled) and `lowerlayerdown` all render identically red. An operator reads
  "red NIC" as a fault on a machine whose NIC is fine.
- S2 · **the `!nodeId` "Select a Node" branch is unreachable** — `:372-381` renders a prompt card, but
  `:331-335` and `node-select.tsx:26-30` both auto-fill the id in the same commit, so an operator who
  wants to choose is never given the choice; the card only flashes.
- S2 · two headings per screen, and the second changes with tab state — `SectionHeader` `<h1>`
  (`admin-ui.tsx:247`) plus `CardHeader title={TABS.find(…)?.label}` (`app/admin/host/page.tsx:387-389`),
  i.e. the visible section title becomes "Info"/"Disk"/"Memory"… Rubric A flags a heading that changes
  with tab state; note `CardHeader` is a `<div>` (`admin-ui.tsx:349-356`), so the tab title is *not* a
  heading and is not announced as one either — the tab panel has no `aria-labelledby` link to it
  (`AdminTabs` `admin-ui.tsx:520-552` renders `role="tab"` buttons but no `role="tabpanel"` exists in
  the page).
- S2 · no `AdminPageToolbar`, no manual refresh, no `AdminLoadingRows`; loading is
  `AdminLoadingState` per tab (`:78, :116, :152, :189, :228`) and the skeleton shape is unrelated to
  the table that follows.
- S2 · processes table silently truncates to 100 rows — `sorted.slice(0, 100)` (`:269`) with no
  "showing 100 of N" footer, while `EmptyState`/header claim "Processes".
- S2 · error copy hides the cause for the most common failure — `formatError` (`:59-68`) maps
  400/401/403 to `error.message` and everything else to `"An unexpected error occurred"`; a
  `nodeId is required` 400 or a permission failure is reported as an unknown error.
- S3 · token violations throughout, hand-counted: `text-slate-200/300/400/500` ×20,
  `bg-white/[0.018]` ×4 (`:126, :163, :201`), `border-white/[0.07]` ×4, `bg-white/[0.04]`,
  `bg-white/10` (`:289`), `border-red-400/50 bg-red-500/10 text-red-200` for the sort toggle
  (`:243, :250`), `bg-red-500/amber-500/emerald-500` usage bar (`:287`), `text-red-400`/`text-emerald-400`
  (`:204`), `divide-white/[0.06]`, and `text-[11px]` ×5 + `text-[10px]`-scale sizes. `usageBar`'s red
  at >80 % is a colour chosen in the page, bypassing `resolveTone` (`admin-ui.tsx:80-127`), which
  exists precisely to own status colour.
- S3 · tab icon semantics wrong: `Memory` tab uses `Cpu`, `Processes` uses `Server`
  (`:26, :28`), and `Server` is also the empty-state icon (`:364, :374`).
- S3 · sort/filter state (`sortBy`, `activeTab`) is component state, lost on navigation, and the two
  node queries diverge: `:329` uses `useQuery(["nodes"], fetchNodes)` while the `NodeSelect` rendered
  at `:355` uses `useNodesQuery()` → `queryKeys.nodes.allLists()` = `["nodes","all"]` with
  `fetchAllNodes` (`lib/admin/telemetry.ts:484-491`). Two fetches, two pagination behaviours, one screen.
- S3 · the page re-fetches `["nodes"]` with default retry while `useNodesQuery` sets `retry:false`;
  a node-list failure here surfaces as a blank select rather than an error state.

### Host Files — `/admin/files`
- files: `app/admin/files/page.tsx` (41 lines) → `components/admin/host-files-view.tsx` (773 lines)
- frame: AdminPageHeader → SectionHeader, content in one `Card`
- title: "Host Files" (`app/admin/files/page.tsx:11`) vs registry "Host Files" → **MATCH**
- description: "Browse and manage files on a node" (`:12`) vs registry (`admin-registry.ts:159`) → MATCH
- icon: registry `FileText` never rendered (`AdminPageHeader` has no icon slot); `File` is used both
  as the "new file" action and the empty-state glyph (`host-files-view.tsx:501, :550`) and `Folder` as
  "new folder" and as every directory row (`:498, :590`).
- **S1 · `info` block never renders** — `app/admin/files/page.tsx:13-32`; the two carefully written
  explanations ("Node-local paths", "Host archive and decompress are not exposed by Beacon yet") are
  the page's only capability-honesty copy, and it is dropped by a header component that has no such
  prop. See structural discovery above.
- **S1 · the file list queries `/host/files/list` with no `nodeId` on mount and the API rejects it** —
  `host-files-view.tsx:74` initialises `nodeId=""`, `:103` calls
  `listFiles(directory, nodeId || undefined)`, and `lib/api/host-files.ts:23` omits the parameter when
  it is undefined → `handlers_files.go:41-43` returns 400. The error banner at
  `host-files-view.tsx:529-534` therefore renders **"nodeId is required: specify which Beacon this
  request targets"** on an otherwise healthy first visit, and the *entire write surface* (upload,
  mkdir, delete, chmod) is likewise un-targeted for the same window. `lib/api/host-files.ts:12-14`
  still documents the deleted fallback ("falling back to the first active node"), which is what
  invites exactly this call shape.
- **S1 · a root-privileged browsing surface auto-selects the node for you** —
  `host-files-view.tsx:478` renders `NodeSelect`, whose effect
  (`node-select.tsx:26-30`) picks the first active node, so "browse and edit any path on a machine"
  starts on a machine the operator did not name. There is no confirmation on any privileged action
  except delete (`:268-273, :284-289`); writing `/etc/*`, chmod and bulk chmod all fire on one click
  (`:327-340, :305-320`).
- **S1 · Archive and Decompress are enabled controls that cannot do their labelled job** —
  `:598-617` `Archive` is enabled on files and *performs a plain download* (`:358-365`), and on
  directories it just sets an error string; `:618-629` `Decompress` renders for `.zip/.tar/.tgz/.gz`,
  is enabled, and **always** fails with a toast (`:366-374`). Rubric M: an unavailable capability must
  be disabled with a reason line, not enabled and lying. A `PackageOpen` icon that can only ever error
  is the enabled-and-lying case.
- S2 · `formatBytes(entry.size)` (`:595`) renders `0 B` for an entry whose size Beacon did not
  report: `lib/api/host-files.ts:6` types `size: number` as required, and `:114` defensively writes
  `a.size ?? 0`, which concedes the field can be missing and then converts unknown to a measured zero.
- S2 · current permissions are never displayed. `mode` exists on the entry
  (`lib/api/host-files.ts:7`) and is used only to prefill the chmod box
  (`host-files-view.tsx:324`, `entry.mode ?? ""` → empty input when unreported); there is no Mode /
  Permissions column while Size and Modified both have one (`:559-571`).
- S2 · the file editor discards unsaved work silently — `Close` (`:438`) and breadcrumb navigation
  (`:435`) set `editing=null` with no dirty check, even though `status` tracks `"Edited"` (`:460`).
  On a host-filesystem surface, a lost edit is a data-integrity event.
- S2 · icon-only controls with no accessible name at narrow widths: the toolbar labels are
  `<span className="hidden sm:inline">` (`:485, :499, :502, :505, :513, :519`), so below `sm` the
  "New folder / New file / Pull URL / Go up / Upload / Terminal" controls reduce to an icon — and
  `Upload` is a `<label>` (`:518-521`), so its accessible name disappears entirely. The refresh button
  (`:515-517`) has no `aria-label` and no `title` at any width.
- S2 · `Save` (floppy) is the Rename icon (`:638`) while also being the real Save action (`:447`);
  `Lock` means "permissions" (`:659`) and `Archive` means "download" (`:612`). Every row action's
  meaning is carried by `title` only (`:603, :613, :624, :634, :644, :655, :666`) — rubric K.
- S2 · ad-hoc loading skeleton (`:542-547`) and ad-hoc empty state (`:548-555`) instead of
  `AdminLoadingRows`/`EmptyState`; the row grid is a hand-rolled `sm:grid-cols-[28px_1fr_100px_170px_auto]`
  declared twice (`:559`, `:580`), so header and body can drift, and below `sm` the Modified column is
  `hidden` (`:596`) leaving date invisible with no fallback.
- S2 · engineering notebook rendered as product copy: `:692` prints a 40-word implementation note
  ("beacon host archive endpoint not yet exposed – parity kept with server files
  archive/decompress/pull/bulk") as persistent body text; `:748` repeats it inside the bulk chmod
  dialog.
- S2 · search/sort/selection are local state (`:71-73, :75`) and reset on directory change (`:412-415`)
  and on navigation; nothing is in the URL, so a filter cannot be shared or survived. Contrast the
  `?nodeId=` handoff the Terminal link does attempt (`:479-486`).
- S3 · `const dragCounter = useState(0)` (`:78`) then `dragCounter[1](…)` (`:125, :130, :140`) — the
  value is never read, and the counter setter is used to drive `setDragging` from inside updater
  functions (`:125-131`), which React does not guarantee to call once per event.
- S3 · `handlePull` computes `dest` (`:388`) and discards it (`:397` `void dest`); the upload actually
  goes to `directory` with the derived name, so the URL dialog's promise about where the file lands is
  only accidentally right.
- S3 · bulk delete/bulk chmod are sequential loops over the selected paths (`:291-297`, `:313-319`)
  with no aggregate progress or partial-failure summary; `run` reports one label for N operations and
  clears the selection regardless of how many succeeded… only after the loop throws, so a mid-batch
  failure leaves the selection intact with no record of what already changed.
- S3 · tokens: `bg-white/[0.03]`, `hover:bg-white/[0.06]` (`:21-29`), `border-red-500/30
  bg-red-500/10 text-red-200` error bands (`:452, :530, :763`), `text-red-400 hover:bg-red-500/10`
  delete (`:664`), `text-amber-400/80` folder (`:590`), `text-white` (`:442, :585, :697…`),
  `bg-black/60` overlays, `h-[65vh] min-h-96`, `text-[11px]`, `max-w-[120px] sm:max-w-[200px]`. It also
  mixes two systems in one file: CSS-variable tokens (`var(--line)`, `var(--surface-input)`) *and*
  raw `ui-*` classes (`:698-701`) *and* ad-hoc button strings (`:20-29`), *and* hand-rolled modals
  (`:694-755`) next to the shared `Dialog` primitive (`:757`).

### Host Terminal — `/admin/terminal`
- files: `app/admin/terminal/page.tsx` (332 lines, no delegated component)
- frame: AdminPageHeader → SectionHeader, body inside one `Card` + `CardHeader`
- title: "Host Terminal" (`:270`) vs registry "Host Terminal" (`admin-registry.ts:160`) → MATCH —
  but the **title is rendered twice** on screen: `:270` and again as `CardHeader title="Host
  Terminal"` (`:295`).
- description: "Interactive shell on a node" (`:271`) vs registry "Secure host shell and console
  access" → **MISMATCH**; "Secure" and "console" both dropped.
- icon: registry `Terminal` rendered only in the inner `CardHeader` (`:296`), not in the page header.
- **S1 · the page opens a privileged host shell before a node is chosen, and the failure it causes is
  reported as a session problem** — `:54` `nodeId=""`, the WS effect deps are `[nonce, nodeId,
  terminalReady]` (`:238`) and the URL is built as
  `"/host/terminal/ws" + (nodeId ? "?nodeId=…" : "")` (`:147-148`). On mount `terminalReady` flips
  true (`:97`) while `nodeId` is still empty, so a socket is opened with no target →
  `resolveNodeMiddleware` → 400 (`handlers_files.go:41-43`) → `ws.onerror` → the operator sees
  **"WebSocket connection failed — … If the API is up, your session may have expired — sign in
  again"** (`:212`) and `Connection failed` written into the shell (`:213`) — a false diagnosis of a
  self-inflicted, always-reproducible mount race. Then `NodeSelect`'s auto-pick
  (`node-select.tsx:26-30`) changes `nodeId` and reconnects. Two connects, one bogus error, and a
  shell that attached to the *first active node* rather than none.
- **S1 · "Connected" is reported for the browser↔panel hop, not for the host session** — `:169` writes
  `Connected` and `:167`/`:300-310` set the badge green inside `ws.onopen`, which the Fiber route
  reaches via `fiberws.New` before/independent of the upstream Beacon dial succeeding
  (`handlers_files.go:107-140`: an upstream dial error is written back as a JSON frame *after* the
  client socket is already upgraded). The UI does parse that frame (`:187-192`) — so the badge flips
  green then red, and the operator's first reading of a *root shell* status is a success the system
  never achieved. AGENTS.md: "Never report success for work not performed."
- **S1 · no node identity inside the terminal, and no privileged-action confirmation** — the shell
  surface is the whole page (`:329-332`); nothing on it says which host it is attached to (hostname
  arrives only if the node's own prompt prints it), and the only node indication is a `<select>` in a
  card header (`:299`). The one piece of copy that *would* have warned — `info.sections[1].content`
  "Treat every keystroke as a privileged host action" (`:288`) — is inside the dead `info` prop
  (`:272`). There is no confirm step for any command; `useConfirm` is not imported here.
- S2 · `terminal.focus()` on every successful open (`:170`) steals focus after mount without user
  gesture, and the xterm container (`:329-332`) has no `aria-label`/`role` — a screen-reader user
  cannot tell what the region is; when `!terminalReady` the div is `hidden` (`:331`), so the loading
  state (`:324-328`) and the hidden surface coexist.
- S2 · auto-reconnect is invisible and unbounded from the UI's point of view: retries are in a ref
  (`:48`, `:226`) and the only visible count lives in the dev-only diagnostics block, whose
  `useMemo` deps are `[connected, nodeId, terminalReady]` (`:265`) — `reconnectAttempt.current` is
  read inside (`:259`) and in JSX (`:339`) but never triggers a re-render, so **"0/15" can be
  displayed after several retries**, and `statusJson` reports `retries: 0` for a session that has
  retried. A stale reading presented as live status.
- S2 · `checkApiReachable()` (`:152`) is fired but does not gate the connection: the socket at `:162`
  is created regardless, so an unreachable API produces "Connecting to host terminal…" (`:160`), then
  "API unreachable" (`:155`), then "Connection failed" (`:213`) in one pass — three overlapping
  claims about the same non-event.
- S2 · `AdminToolbar` is used as an *action slot inside `CardHeader`* (`:298`) with
  `className="border-0 bg-transparent p-0"` — i.e. the toolbar primitive is re-skinned per call to
  pretend it isn't a toolbar; rubric H (there is no `AdminPageToolbar`, and the primary action
  "Reconnect" sits in a card header next to a status pill).
- S2 · no `error.tsx`/`loading.tsx` for the route; the terminal runtime load failure path (`:119-123`)
  sets `error` to the string `"Terminal runtime failed to load"` and, because `!terminalReady` is
  still true, renders `AdminLoadingState "Initializing terminal…"` (`:326`) *alongside* the error —
  the page reports both "still starting" and "broken".
- S3 · tokens: `border-emerald-500/30 bg-emerald-500/10 text-emerald-300` /
  `border-red-500/30 bg-red-500/10 text-red-300` (`:304-305`), the same pair again amber
  (`:338`), `text-[11px]`/`text-[10px]` (`:302, :337, :338, :342`), `bg-black/20` (`:342`),
  `h-[calc(100vh-20rem)] min-h-[300px]` (`:331`) — a magic viewport subtraction that breaks against
  the shell chrome height.
- S3 · `/* intentional terminal chrome, not surface */` written into a `className` string (`:331`) —
  a comment in Tailwind's class list, not in the token system, so it is both dead documentation and a
  raw-var surface choice.
- S3 · `?nodeId=` is read once on mount (`:58-62`) and never written back on change, so a node switch
  is unshareable and a browser back/forward re-triggers the mount race above.

### SFTP — `/admin/sftp`
- files: `app/admin/sftp/page.tsx` (7-line passthrough, `"use client"`) →
  `components/admin/AdminSftp.tsx` (341 lines)
- frame: SectionHeader (directly, `AdminSftp.tsx:72`) inside `AdminPageLayout`
- title: "SFTP" (`AdminSftp.tsx:73`) vs registry "SFTP" (`admin-registry.ts:161`) → MATCH
- description: "Global defaults with per-node overrides for secure file access" (`:74`) vs registry
  "Global and per-node SFTP configuration" → paraphrased, not the registry string (S3)
- icon: registry `FolderLock` is used only as the *Per-node configs* card icon (`:182`); the page
  header renders no icon. `Shield` is the row action for "Edit" (`:225`) — an edit action wearing a
  security glyph.
- **S1 · "N nodes" is the count of override rows, and it renders 0 while the query is in flight** —
  `:182` `CardHeader action={<span>{nodesQ.data?.length ?? 0} nodes</span>}` is rendered *above* the
  `nodesQ.isLoading` branch (`:183-184`), so for the whole load window the card asserts "0 nodes",
  and afterwards it still asserts "1 nodes" for a fleet of twenty that has one override. The correct
  denominator is already loaded two lines away (`allNodesQ`, `:40`) and shown in the *third* card as
  "Nodes without override — 4" (`:271`). Two cards, one fleet, two incompatible counts.
- **S1 · blanking a numeric field writes `0`, and `0` means "no limit"** —
  `:128-131` and `:320-324` all use `Number(v) || 0`; the labels themselves say
  `"Rate limit (KB/s, 0=unlimited)"` (`:131`). Clearing the rate-limit input therefore *raises a
  bandwidth cap to unlimited* and clears `Max connections`/`Idle timeout` to zero, with a single
  Save button (`:110`) and no confirmation on a security-relevant change. `Number("abc") || 0` and
  `Number("") || 0` are indistinguishable from an intentional 0.
- **S1 · the "enabled/disabled" pill is a stored setting, never a node state** —
  `:109` and `:210` render `Pill tone={… ? "green" : "red"}` labelled `enabled`/`disabled` from
  `fetchSFTPGlobalConfig`/`fetchSFTPNodeConfigs` (config rows), while no query in this file reads a
  node heartbeat, a listener or a port check. An operator concludes "SFTP is up on this host" from a
  database column. Rubric G/M: a health indicator for something not measured; the green/red tone also
  borrows the status vocabulary (`admin-ui.tsx:43-55`).
- S2 · `info={{…}}` dead block — `:75-94`; `SectionHeaderProps` (`admin-ui.tsx:140-162`) has no `info`
  key, so the global-vs-per-node explanation and the "creates one from the current defaults on first
  save" promise never render. The same promise survives only as `:235` footer copy.
- S2 · **the "create an override for a node that has none" flow may be an error page, not a default
  form** — `UnconfiguredNodesCard` (`:256-282`) offers `Configure` for every node without a row, and
  `SftpNodeEditor` (`:287`) fetches `fetchSFTPNodeConfig(nodeId)`, whose `isError` branch renders
  `AdminErrorState` (`:310`). If the endpoint 404s for "no override yet" — which is exactly the state
  these rows represent — the advertised first-save path cannot open. The page has no
  create-vs-edit distinction anywhere.
- S2 · editor is mounted without a `key` and guards its own state with `!form` — `:250`
  `<SftpNodeEditor nodeId={editingNodeId} …>` and `:290` `if (q.data && !form) setForm(q.data)`. Any
  change of `editingNodeId` while the component stays mounted keeps node A's values under node B's
  title (`:309` "Edit node SFTP config", `:302` toast names B), then saves A's numbers onto B. Only
  the modal overlay currently prevents it.
- S2 · the same field is spelled two different ways in one file: the global log-level select offers
  `error/warn/info/debug/trace` (`:138-142`) and the node select offers four without `trace`
  (`:335-338`). A node legitimately configured to `trace` renders a select with no matching option,
  i.e. an empty/blank value presented as the setting.
- S2 · `Allowed IPs` (`:343-344`) is a free-text comma list with "blank = all" — an access-control
  field where clearing it widens access, again with no confirmation, no CIDR validation and no
  count. Contrast `useConfirm` usage in `host-files-view.tsx:268`.
- S2 · **edit vs create label lie**: modal title is always "Edit node SFTP config" (`:309`) even when
  opened from "Configure" on an unconfigured node (`:276`), so the operator cannot tell whether Save
  will create an override or mutate one.
- S2 · two competing toast systems inside one scope: `AdminSftp.tsx:17` and
  `docker-cleanup-manager.tsx:24` use `useToast` from `@/components/ui/toast`, while
  `host-files-view.tsx:17` uses `toast` from `@/components/ui/sonner`.
- S2 · three node-shaped queries fetched with different policies and no shared key:
  `:38` `["admin-sftp-settings"]`, `:39` `["admin-sftp-nodes"]` (via `fetchSFTPNodeConfigs`), `:40`
  `useNodesQuery()`. `retry:false` on two of them means a single blip leaves a permanent `enabled`
  pill-less card with no automatic recovery, while `UnconfiguredNodesCard` (`:240`) only renders when
  *both* `allNodesQ.data` and `nodesQ.data` exist — so a failed overrides query silently deletes the
  "nodes without override" section rather than reporting that the comparison could not be made.
- S3 · `import {  } from "@/lib/api";` (`:16`) — empty import statement, present verbatim; ditto
  `docker-cleanup-manager.tsx:8` and `node-select.tsx:4`.
- S3 · tokens: `border-red-500/30 bg-red-500/10 text-red-200` (`:174, :352`),
  `border-emerald-500/20 bg-emerald-500/10 text-emerald-200` (`:175`), `text-slate-400` (`:333, :343,
  :348`), `text-[10px]` + `tracking-[0.12em]` table head (`:193`), `text-[11px]` (`:208, :218, :220`),
  `max-w-[16rem]` (`:220`), `min-h-[60px]` (`:349`). Mixed `var(--*)` tokens and raw palette in one
  file.
- S3 · seven-column table in `overflow-x-auto` with no card fallback (`:190-231`); the Limits cell
  (`:213`) is four values concatenated into one string, so it cannot be sorted, compared or truncated
  per field.
- S3 · `updatedAt` (`:222`) is formatted with `new Date(…).toLocaleString()` inline instead of the
  shared `formatDate` (`lib/utils.ts:17`), and renders `—` for absent — the only honest cell on the
  page. The banner column uses `title` as its only full-value tooltip (`:220`).
- S3 · no `loading.tsx`/`error.tsx` on the route; the wrapper (`app/admin/sftp/page.tsx`) is a 7-line
  passthrough whose only content is the component, so the 26-line `OfflineBanner` retry
  (`AdminSftp.tsx:71`) is the entire route-level error surface.

### Image & Cache Cleanup — `/admin/docker-cleanup`
- files: `app/admin/docker-cleanup/page.tsx` (5 lines, **no `"use client"`**, unlike its sibling
  wrappers `app/admin/sftp/page.tsx:1`, `app/admin/kubernetes/page.tsx:1`) →
  `components/admin/docker-cleanup-manager.tsx` (586 lines)
- frame: SectionHeader (`:257`) → Cards; no `AdminPageToolbar`
- title: "Image & Cache Cleanup" (`:258`) vs registry (`admin-registry.ts:162`) → MATCH
- description: "Per-node disk usage and automated, retention-aware cleanup of unused images, build
  cache and dangling volumes." (`:259`) vs registry "…retention-aware image, cache and volume
  pruning" → reworded (S3)
- icon: registry `HardDrive` appears only as the "Total" KPI icon (`:319`); header renders no icon.
  `HardDrive` is a four-way registry collision (`admin-registry.ts:132, 162, 188` + Storage Mounts) —
  App Storage, this page, Backups and Mounts all share one glyph.
- **S1 · the prune toast reports "0 B reclaimed" for a prune that Beacon explicitly flagged as
  unmeasured, and the flag is thrown away one hop earlier.** Beacon returns
  `reclaimedBytesKnown:false` with `reclaimedBytes:0` when the engine could not report sizes
  (`beacon/internal/server/docker_cleanup.go:91-94, :260-263, :285`, and the comment "because '0 bytes
  freed' and 'we could not tell' are different answers"). The control-plane decoder target
  `forge/api/internal/services/dockerleanup/types.go:46-52` declares only `nodeId`, `reclaimedBytes`,
  `removedCount` — `reclaimedBytesKnown`, `failedCount`, `skippedCount` and `errors` are all dropped
  at `service.go:241-248`, and `forge/web/lib/api/docker-cleanup.ts:25-29` mirrors the loss. The UI
  then prints `${removedCount} removed · ${formatBytes(reclaimedBytes)} reclaimed` as a **success
  toast** (`docker-cleanup-manager.tsx:149-157`). AGENTS.md: "not-reported is not zero".
  Partial-failure counts are lost the same way: a prune that deleted 2 of 10 images toasts
  "2 removed" and nothing else.
- **S1 · disk "Total" is the sum of four categories, labelled as the node's disk, and every
  percentage on the page is a fraction of it** — the panel recomputes
  `TotalBytes = images + containers + volumes + buildCache`
  (`forge/api/internal/services/dockerleanup/service.go:115`) — Beacon never sends filesystem
  capacity (`docker_cleanup.go:52-55` comment: "The panel fills in node identity and recomputes
  totalBytes"). The UI labels this "Total" (`docker-cleanup-manager.tsx:319`), then
  `UsageBar` divides each segment by `safeTotal` (`:494, :499`) and prints `{pct}%` per category
  (`:512`). The bar is therefore always ~100 % full and "Images 62 %" means 62 % of what was counted,
  not of the disk. Free space, mount point and partition size are invisible here — while the sibling
  Host Inspector *does* show `used/total per mount` (`app/admin/host/page.tsx:126-136`). Two screens,
  two incompatible meanings of "disk usage", and the destructive one has the worse model.
- **S1 · unknown sizes become measured zeros in the same report** — Beacon counts them
  (`SizeIncompleteCount`, `AccountingNotes`, `UnattributedImageBytes`,
  `docker_cleanup.go:158-202`) and the panel struct has no field for any of them, so
  `docker-cleanup-manager.tsx:319-333` renders partial totals as complete and `reclaimable` (`:138`)
  sums `img.size || 0` — an image the engine did not size contributes **0 bytes to the reclaim
  estimate**. The "Nothing to prune" empty state (`:369-370`) is reachable when sizes were simply not
  reported.
- **S1 · the retention floor, the one guard against stranding the deployed image, is disabled by an
  empty input** — `:346` `onChange={(v) => setLimit(Math.max(0, Number(v) || 0))}`; clearing "Keep
  newest N" yields `0`, and `selectUnusedImages` with `mostRecentLimit = 0` returns *every*
  not-in-use image (`service.go:140-146`). The explanatory sentence right next to it still renders
  "the 0 most recent unused image(s) by creation date are always preserved so the deployed version is
  never stranded" (`:348`) — a self-contradicting claim in the same flex row, one keystroke from a
  destructive prune. The policy dialog repeats it (`:591`), and a policy with `mostRecentLimit: 0`
  persists in the table as "keep 0" (`:443`).
- **S1 · prune targets a node the operator never selected** —
  `:121-123` `const activeNodeId = nodeId || (nodes[0]?.id ?? "")` with the comment "Default to the
  first node once the list arrives so the panel is useful immediately without a manual selection".
  `activeNodeId` is then the target of all three destructive mutations (`:160`, `:165`, `:170`) and of
  `New policy`'s pre-filled scope (`:475 defaultNodeId={activeNodeId}`). Note this is `nodes[0]` —
  *first in list order* — whereas the other three host-access pages use `pickDefaultNode` = first
  *active* (`node-select.tsx:11-15`). Two different silent node-picking rules inside one scope, and
  the backend's ambiguity guard (`handlers_host.go:44-55`) is bypassed here because this route takes
  the node id in the path (`lib/api/docker-cleanup.ts:63-81`).
- S2 · **no preview for the two bulk prunes.** "Prune build cache" (`:358`) and "Prune volumes"
  (`:361`) are single clicks + one confirmation sentence (`:196-214`) with no list of what will be
  removed and no byte estimate — unlike images, which at least show a table. The confirmation text
  asserts an engine guarantee ("Volumes still in use are preserved") that the UI cannot verify;
  `pruneVolumes` maps to `docker volume prune` semantics and the response's `skippedCount` is dropped
  (see S1 above), so the claim is unverifiable after the fact too.
- S2 · destructive-action hierarchy is inverted: "Prune selected" is `tone="danger"` (`:350`) while
  the two *bulk, unpreviewed* prunes are `tone="subtle"` (`:358, :361`) in the same row.
- S2 · `Badge className="bg-sky-500/15 text-sky-300">{formatBytes(reclaimable)} reclaimable`
  (`:342`) describes **all** unused images while the adjacent button acts on the **selection**
  (`:356`), and while `unusedQuery` is loading `unused` is `[]` (`:137`) so the badge renders
  "0 B reclaimable" next to "Analyzing images…" (`:366`).
- S2 · **global policies cannot be created.** `AdminSelect label="Scope" …
  placeholder="All nodes (global)"` (`:582-588`) is fed `nodeOptions` (`:116-119`), which contains
  only real nodes — there is no `{ value: "" }` entry, so "all nodes" is unreachable; the create path
  would support it (`:554 form.nodeId || undefined`, and `types.go:64-66` "NodeID == '' means the
  policy is global"). Worse, editing an existing *global* policy starts from `nodeId: ""` (`:540`)
  and Save sends `nodeId: form.nodeId` (`:563`) — the operator cannot see or keep "global" deliberately;
  and `New policy` is disabled unless a node exists (`:409 disabled={!activeNodeId}`).
- S2 · `info={{…}}` dead block (`:260-279`) — the retention-floor and scheduled-policy explanations
  never render (structural discovery above), which is exactly the copy this page needs before a
  destructive action.
- S2 · status colours on non-status data, contradicting the file's own comment: `:490-491` says the
  bar hues are "intentionally outside the ok/warn/danger status vocabulary", yet `StatsRow` assigns
  `tone:"yellow"` to Containers and `tone:"green"` to Build cache (`:321-322`) — which *are* status
  tones (`admin-ui.tsx:43`, `resolveTone`). Also `Volumes` is a bar segment (`:330`) but not a KPI,
  and `Total` is a KPI but not a segment: the same five numbers, two vocabularies.
- S2 · no freshness anywhere: `usageQuery.dataUpdatedAt` is never surfaced, `SectionHeader status`
  is unused (`:257-285`), and the Refresh button's `loading` prop is bound to
  `usageQuery.isFetching` only (`:281`) while three other queries exist — so "unused images" can be
  minutes old behind an idle Refresh.
- S2 · `Next run` for an enabled policy renders `formatDate(p.nextRunAt)` whose fallback is **"Never"**
  (`lib/utils.ts:17-19`) — an enabled policy that has not been scheduled reads "Never", the same word
  used for `Last run` of a policy that has not run (`:447`), while a disabled policy shows the word
  "disabled" (`:444`). Three meanings, two of them identical strings.
- S2 · icon-only row actions with `title` as their only name: Run now (`:454-456`) and Delete policy
  (`:458-460`) have no `aria-label`; the table also has no `aria-label` on the actions column, so a
  screen reader announces an unlabeled control in a destructive column.
- S2 · policy mutations are fire-and-hope: `togglePolicyMut` (`:239-243`) and `runNowMut` (`:216-228`)
  have no optimistic state and no pending indicator per row, so clicking "Disable" on three rows
  quickly leaves the table showing values that may not have been written; `runNowMut` is shared, so
  `disabled={runNowMut.isPending}` (`:454`) disables *every* row's Run-now when any one is running.
- S3 · tokens: `bg-sky-500/15 text-sky-300` (`:342`), `bg-amber-500/15 text-amber-300` (`:438`),
  `bg-emerald-500/15` (`:439`), `bg-sky-500/violet-500/emerald-500/amber-500` (`:328-331`),
  `bg-white/[0.04]` (`:497`), `w-9` fixed column (`:512`), `sm:w-40` (`:345`), and `text-slate-100 /
  300 / 400 / 500 / 600` (`:392, :396, :436, :442-447, :510-512, :605`) — `text-slate-600` on a dark
  card is a contrast risk (rubric K).
- S3 · the reclaim badge, KPI row and bar all use raw hex-free-but-palette Tailwind hues while the
  project token file exists (`lib/design-tokens.ts`), and the comment at `:56` claims "this view is a
  thin, honest renderer over the admin API" — the renderer cannot be honest because its wire types
  discard the honesty fields.
- S3 · `formatAge` (`:58-67`) and `formatDate` are used side by side in the same cell (`:396`), giving
  "Jan 5, 2026, 09:12 (~2 months ago)" — two time vocabularies in one table column.

### Kubernetes — `/admin/kubernetes`
- files: `app/admin/kubernetes/page.tsx` (7-line passthrough) → `components/admin/AdminKubernetes.tsx`
  (139 lines)
- frame: SectionHeader (`AdminKubernetes.tsx:39`) + AdminTabs + 4 Cards
- title: "Kubernetes" (`:40`) vs registry (`admin-registry.ts:164`) → MATCH
- description: "Cluster workloads on nodes with the Kubernetes runtime — pods, deployments, services,
  and events." (`:41`) vs registry "Kubernetes pods, deployments and services" → the page adds
  "events" (a fourth tab the registry description omits) and the "on nodes with the Kubernetes
  runtime" qualifier the registry lacks
- icon: registry `Boxes` used only as the `EmptyState` icon (`:81`); no header icon. `Boxes` is a
  three-way registry collision (Image Registries / Kubernetes / Incus).
- **S1 · the page's default state is an "Auto" mode the API refuses: every tab errors on first
  visit.** `:69` `placeholder="Auto — first k8s node"` and `:18` `nodeId` starts `undefined`, so
  `:23-26` fetch `/admin/kubernetes/pods` with no `nodeId`
  (`lib/api/kubernetes.ts:55-56, :63` omit the param) →
  `forge/api/internal/http/handlers_kubernetes.go:50-56` returns
  **400 `nodeId is required: specify which kubernetes node this request targets`** — and the Go
  comment at `:44-49` records that auto-picking "whichever node happens to be a Kubernetes node" was
  deliberately removed. The UI still advertises it. The console therefore renders four working-looking
  tabs whose content is the raw Go error string (`:89, :106, :123, :140`), and the same failure hits
  the mutation path (`:29 scaleK8sDeployment(name, replicas, nodeId)` → `lib/api/kubernetes.ts:79-80` →
  400), so **-1/+1 do nothing until a node is manually chosen.**
- **S1 · pod health is a page-local, case-sensitive colour rule that colours failures as idle.**
  `:95` `Pill tone={p.status === "Running" ? "green" : "neutral"}` — `Failed`, `CrashLoopBackOff`,
  `Pending`, `Unknown`, `Evicted` all render the **same neutral grey as a Succeeded pod**.
  `lib/api/status.ts` is the project's single status→tone authority (`crashed: "danger"`,
  unknown → `"unknown"`, and its header comment at `status.ts:1-26` explains that page-local maps were
  consolidated precisely because of this failure mode) — `status.ts:221-248` has `nomad`, `incus`,
  `discovery` … **but no `kubernetes` kind at all**, so the correct thing to call does not exist and
  this page invented its own. A crash-looping pod is indistinguishable from a completed one.
- S2 · `Replicas` renders `${d.readyReplicas}/${d.replicas}` (`:112`) with `K8sDeployment` fields
  typed required (`lib/api/kubernetes.ts:28-29`) — an API that omits them shows `"/"`;
  `restartCount` (`:95`) and `s.clusterIp` / `s.ports.join(", ")` (`:129`) render blank when absent.
  No `—`/`Unavailable` treatment anywhere except the two hand-written `|| "—"` at `:95` and `:129`.
- S2 · events are silently truncated — `evtsQ.data!.slice(0, 50)` (`:145`) with no count, no "newest
  50 of N", and `key={i}` array-index keys (`:145`) on a list that refreshes. `K8sEvent.lastSeen`,
  `count`, `involved` and `firstSeen` (`lib/api/kubernetes.ts:45-53`) are never rendered — a warning
  event shows no age and no repetition count, so an operator cannot tell a 1-minute-old event from a
  6-hour one.
- S2 · the Services tab has no Namespace column, and none of the four tables shows `namespace` even
  though pod, deployment and service all carry it (`lib/api/kubernetes.ts:15, :27, :38`; `K8sEvent`
  does not) and the API is
  cluster-wide — a fleet console where identically named objects in different namespaces are
  indistinguishable, while scaling by `name` alone (`:29`, `lib/api/kubernetes.ts:79-80`) is exactly the
  ambiguous-target pattern in the cluster dimension.
- S2 · `New`/destructive parity: the only mutation is replica scaling, `-1` can reach `0`
  (`Math.max(0, d.replicas - 1)`, `:112`) with **no confirmation**, which takes a service to zero
  replicas on click — while Host Files confirms a single file delete
  (`host-files-view.tsx:268`). Same page also lacks `useConfirm` entirely.
- S2 · `noK8s` gates the console honestly (`:35, :80-82`) — *the only runtime page in this scope that
  does* (contrast Incus below) — but the `{nodes.length > 0 ? <Pill>N nodes</Pill> : null}` at `:75`
  hides the count rather than showing "0 k8s nodes", so the header reads as having no fleet metric
  instead of reporting zero.
- S2 · `info={{…}}` dead block (`:42-61`) — and its copy is the one place that states the (now false)
  behaviour: "or leave automatic to use the first Kubernetes node" (`:52`). Had it rendered, it would
  have documented a fallback the backend deleted.
- S3 · `AdminLoadingState`/`AdminErrorState` inline ternaries repeated four times (`:89, :106, :123,
  :140`) — NetBird factored this into `renderQuery` (`app/admin/netbird/page.tsx:286-295`) and this
  scope has three such helpers; the query-state rendering is not shared.
- S3 · refresh (`:72`) invalidates only `k8s-nodes` and `k8s-pods` — Deployments/Services/Events keep
  their stale data behind a Refresh button that appears to refresh the page; and the four queries
  have no `refetchInterval`, no `staleTime` and no freshness badge, so nothing says how old the
  cluster view is.
- S3 · no `EmptyState` title on three of four tabs (`message` only: `:89 "No pods."`, `:106`, `:123`,
  `:140`), which `EmptyState` renders as title "Nothing to show" (`admin-ui.tsx:616-618`) — a generic
  heading under a specific card.

### Incus — `/admin/incus`
- files: `app/admin/incus/page.tsx` (258 lines, self-contained + `CreateInstanceModal`)
- frame: SectionHeader (`:75`) + AdminTabs + 5 Cards
- title: "Incus" (`:76`) vs registry (`admin-registry.ts:165`) → MATCH
- description: "System containers and virtual machines on nodes with the Incus runtime — managed over
  the Incus REST API." (`:77`) vs registry "Incus system containers and virtual machines" → extended
- icon: registry `Boxes` rendered as the Instances tab icon and the Instances `CardHeader` icon
  (`:29, :123`) → MATCH, and the same three-way collision as Kubernetes.
- **S1 · a full five-tab console renders for a runtime no node reports.** `:115-117` shows
  `EmptyState "No Incus nodes"` when `nodes.length === 0` **but does not return or gate anything** —
  `:119` renders `AdminTabs` and `:121-243` every table unconditionally, and `:109` keeps the
  `New instance` primary action enabled. The queries are gated on tab only
  (`enabled: tab === "instances"`, `:47-51`), never on `nodes.length`, so with zero Incus nodes the
  page fires five real API calls with `nodeId` omitted; `handlers_incus.go:74-102` then answers
  "Zero nodes leaves the id empty so the service falls back to its environment-configured endpoint" —
  i.e. **the console can list instances from an Incus server that is not a registered Forge node at
  all, directly under a banner saying no Incus nodes exist.** Kubernetes gates this case
  (`AdminKubernetes.tsx:80-82`); Incus does not.
- **S1 · deleting an instance is one click, no confirmation.** `:142`
  `onClick={() => deleteMut.mutate(i.name)}` — `useConfirm` is not imported by this file at all
  (imports, `:20-26`), while the sibling `docker-cleanup-manager.tsx:107` and
  `host-files-view.tsx:63` confirm far smaller operations. A hypervisor VM and its disk are destroyed
  by a misclick on a 12px trash icon.
- **S1 · the four lifecycle toasts never name the instance they acted on.** `:60-63`
  `onChanged(action, name)` is called with the literal string:
  `onChanged("Started", "instance")`, `onChanged("Stopped", "instance")`,
  `onChanged("Restarted", "instance")`, `onChanged("Deleted", "instance")` (`:65-68`) — so every
  success toast reads "Deleted · instance". `mutationFn` receives the real name and discards it. The
  operator gets a success confirmation that identifies nothing, on a table of same-looking rows.
- **S1 · shared mutation state makes every row look busy.** `:71` `pending` is the OR of four
  mutations, and `loading={startMut.isPending}` / `disabled={… || pending}` are applied per row
  (`:139-142`): starting one instance draws a spinner on **every** row's Start button and disables
  Stop/Restart/Delete everywhere. The UI reports in-progress work on objects it was never asked to
  touch.
- **S1 · `Auto — first incus node` (`:105`) is a two-node lie.** `handlers_incus.go:93-99`
  auto-selects only when exactly one Incus node exists and otherwise returns
  **400 `nodeId is required: multiple Forge Virtualization nodes are registered`**. So the same
  placeholder is truthful on single-node fleets and produces that raw error string in
  `AdminErrorState` (`:124`) on any real fleet — and the operator is told about "Forge
  Virtualization", a product name that appears nowhere in the UI (registry label is "Incus",
  `admin-registry.ts:165`). Same class of stale naming in `Nomad` → "Forge Orchestration"
  (`handlers_nomad.go:12, :43-49`).
- S2 · **inconsistent client call shape for the same resource family** — `:65-68` mixes positional
  and object forms: `startIncusInstance(name, nodeId)`, `stopIncusInstance(name, { nodeId })`,
  `restartIncusInstance(name, nodeId)`, `deleteIncusInstance(name, { nodeId })`
  (`lib/api/incus.ts:85-108`). Two conventions in one module, four adjacent call sites; the `force`
  option exists on both `stop` and `delete` (`lib/api/incus.ts:89-91, :103`) and is never surfaced, so a
  running instance can only be stopped gracefully and there is no "force" affordance for an operator
  whose Stop did nothing.
- S2 · the cluster tab's empty state asserts a fact the page cannot know: `:231` renders
  "Standalone host — no cluster members." for `clusterMembers.length === 0`, but the same empty array
  is what an unreachable Incus API, a non-clustered project, or an authorization failure with a 200
  body all produce. The honest wording is in the dead `info` block (`:93-94`, "A standalone host shows
  no cluster members — that is expected, not an error") and is not rendered.
- S2 · no capacity or resource columns anywhere: `IncusInstance.config`/`devices`
  (`lib/api/incus.ts:12-13`) hold limits and disks, and the page shows none of them; Profiles renders
  only `Object.keys(config).length` and `Object.keys(devices).length` (`:190-191`) — two bare counts,
  no names, so "3 / 1" is the entire content of the Profiles tab. Storage Pools
  (`:207-215`) show Name/Driver/Status only, while `IncusStoragePool.usedBy` and `config`
  (`lib/api/incus.ts:41, :43`) exist — a *storage* tab that reports no usage, sitting one click from
  `Image & Cache Cleanup` which reports nothing but usage.
- S2 · `CreateInstanceModal` (`:250-272`) takes a free-text "Image alias / fingerprint"
  (`:263`) while the Images tab has the full alias list (`:163-170`) — no autocomplete, no picker,
  and `source.alias` is sent unchecked (`:255`). Its own footnote (`:264`) tells the operator to
  "POST the raw spec via the API" for profiles/devices/limits: the create path is a
  metadata-only form wearing product chrome (rubric N).
- S2 · `Create` disabled logic (`:267`) ignores whether any Incus node exists, and
  `createIncusInstance(spec, nodeId)` with `nodeId === undefined` hits the env-fallback path above —
  the operator can create an instance on a machine that Forge has not registered.
- S3 · the Instances tab has no project column though the row key is
  `` `${i.project ?? "default"}/${i.name}` `` (`:131`) — two instances with the same name in
  different projects render as two identical rows with the same visible name.
- S3 · tokens: `text-slate-500` table heads ×5 (`:127, :161, :184, :207, :229`),
  `border-white/[0.06]` ×5, `divide-white/[0.04]` ×5, `hover:bg-white/[0.02]` ×5,
  `text-slate-200/400` throughout, `colSpan` empty-row pattern with `py-8` magic padding — this file
  uses the raw-palette dialect while `AdminKubernetes.tsx:92-93` uses `var(--line)` /
  `var(--text-subtle)` for the identical table. Two dialects, adjacent pages.
- S3 · `Btn tone="success"` (`:166` in Nomad; here `:140 tone="warning"`) — `Btn` tones here are
  action styling, `Pill` tones are status styling, and both take the same
  `green/yellow/red/neutral` names (`admin-ui.tsx:43-55`), so "warning" means "Stop" on one component
  and "abnormal" on the other.
- S3 · tab state, and the absence of any `?tab=`/`?nodeId=` URL sync, so an Incus view cannot be
  linked; the `refresh` helper (`:53-56`) invalidates only nodes+instances, so switching to Images
  after Refresh shows cached data with a button that looks like it refreshed everything.

### Nomad — `/admin/nomad`
- files: `app/admin/nomad/page.tsx` (216 lines, self-contained + `SubmitJobModal`)
- frame: SectionHeader (`:69`) + AdminTabs + 4 Cards
- title: "Nomad" (`:70`) vs registry (`admin-registry.ts:166`) → MATCH
- description: "Jobs, allocations, client nodes and deployments on the Nomad control plane." (`:71`)
  vs registry "Nomad jobs, allocations, nodes and deployments" → near-match, reworded
- icon: registry `Workflow` rendered as the Jobs tab icon and the Jobs `CardHeader` icon
  (`:24, :104`) → MATCH; `Workflow` collides with Procedures (`admin-registry.ts:199`).
- **S1 · "Stop" is a purge, and it is one click.** `:55` `stopNomadJob(id, true)` — the second
  argument is `purge` (`lib/api/nomad.ts:74-76`, which appends `?purge=true` →
  `handlers_nomad.go:109 isTrueParam(c, "purge")`). The button is labelled "Stop" with a `title="Stop
  job"` (`:116`), there is no `useConfirm` anywhere in the file, and purging removes the job
  *definition and evaluation history*, not just the running work. A misclick is unrecoverable from the
  UI and the UI never says what it did.
- **S1 · "Drain" / "Eligible" toggle is also unconfirmed** (`:166`), and it changes fleet placement —
  `handlers_nomad.go:34` posts to the Nomad API immediately, while `/admin/drain` ("Node Drain
  lifecycle", `admin-registry.ts:196`) exists as a separate, presumably governed surface. Two ways to
  drain, one of them a single grey button on a runtime page.
- **S1 · the page offers "Submit job" unconditionally for a control plane that may not exist.**
  `:95` `Btn tone="primary" … onClick={() => setSubmitOpen(true)}`; the backend answers
  **503 `Forge Orchestration service unavailable`** / `ErrNotConfigured`
  (`handlers_nomad.go:11-16, :43-49`) when Nomad is not configured. There is no capability probe:
  unlike Kubernetes (`AdminKubernetes.tsx:35` gates on `nodes.length === 0`), Nomad has no node or
  configuration gate at all, so an unconfigured deployment renders four tabs of "No jobs." -shaped
  error cards plus a working-looking primary action. The registry also marks
  `/admin/nomad` `capability: "available"` (`admin-registry.ts:166`) — a static promise, never
  reconciled with what nodes report.
- S2 · the empty-table idiom is a fourth variant in this scope:
  `<tr><td colSpan={4} className="py-8 text-center text-sm text-slate-500">No jobs.</td></tr>`
  (`:110, :135, :159, :185`) — a table row used as an empty state, versus `EmptyState`
  (Kubernetes), a below-table `EmptyState` (NetBird `netbird/page.tsx:393`) and a replacing card
  (Host Files). Same information, four shapes.
- S2 · status vocabulary is handled correctly here — `nomadStatusTone` (`:113, :140, :163, :189`)
  from `lib/api/status.ts:157-181`, which maps `dead/complete → neutral`, `down → danger`, unknown →
  `unknown`. **This is the pattern Kubernetes and docker-cleanup should copy**
  (`AdminKubernetes.tsx:95`, `docker-cleanup-manager.tsx:80-84`). Credit where due; note it as the
  fix path.
- S2 · `Pill tone={nomadStatusTone(n.Status)}>{n.Status || "—"}{n.Drain ? " · draining" : ""}`
  (`:163`) packs two facts into one pill, so a drained node reads `ready · draining` where the tone
  describes only `ready`; `Eligibility` (`lib/api/nomad.ts:44`) is never shown, and the toggle button
  at `:166` uses `n.Drain` alone — a node set ineligible by other means shows a "Drain" button.
- S2 · `Type` column shows `j.Type || "service"` (`:114`) — an unreported type is displayed as the
  value "service", i.e. invented data, not a blank or `—`. Same class: `n.Name || n.ID.slice(0, 8)`
  (`:161`) silently substitutes a truncated UUID for a missing name with no marker, so a node ID and
  a node name are visually identical (`font-mono text-xs text-slate-200`).
- S2 · the Deployments tab shows `ID / Job / Status / Desired` (`:183-190`) while
  `NomadDeployment.Canary`, `Pause`, `StatusDescription` (`lib/api/nomad.ts:55-58`) are dropped —
  a **paused** or **canary** deployment, the two states an operator most needs to see during a
  rollout, are invisible; the pill's tone comes from `Status` only.
- S2 · `SubmitJobModal` non-JSON path (`:210-217`) wraps pasted HCL as `{ hcl: text }` and posts it as
  a job document; the copy promises "the API will reject it with guidance" (`:227`). The Go handler
  just `BodyParser`s into `map[string]any` and forwards it (`handlers_nomad.go:60-75`), so what comes
  back is Nomad's complaint about a job with a single `hcl` key — not guidance. A deliberately
  engineered failure path whose promised help does not exist.
- S2 · `onSubmitted(res?.job?.ID ?? "job")` (`:220`) → success toast "Job submitted / job" when the
  response has no job id, i.e. the same unidentified-success pattern as Incus.
- S3 · four `enabled: tab === …` queries (`:39-42`) with no `staleTime`/`refetchInterval` and no
  freshness; the header Refresh (`:94`) invalidates all four including the three never fetched, and
  `jobsQ` etc. refetch on every tab switch — so "Refresh" and "switch tab" mean the same thing here
  and neither is honest about age.
- S3 · tokens: identical raw-palette dialect as Incus (`:108-110` etc.), `text-slate-500` ×8,
  `border-white/[0.06]`, `divide-white/[0.04]`, `hover:bg-white/[0.02]`; `Btn … title="Stop job"` /
  `title={n.Drain ? …}` (`:116, :166`) used as the only tooltip; `AdminTabs` called without `label`
  (`:100`) so the tablist is announced as the default "Page sections" while Kubernetes/Incus pass
  explicit labels (`:84`, `:119`).
- S3 · `CardHeader title="Client Nodes"` vs tab label "Nodes" (`:26` vs `:153`) — the two headings
  for the same section differ by one word on the same screen.

### NetBird VPN — `/admin/netbird`
- files: `app/admin/netbird/page.tsx` (630 lines, self-contained, 5 modals + 7 tabs)
- frame: SectionHeader (`:299`) + AdminTabs + 7 Cards; **the only page in the scope that factors
  query state into one helper** (`renderQuery`, `:286-295`) — should be lifted to the shared layer
- title: "NetBird VPN" (`:300`) vs registry "NetBird VPN" (`admin-registry.ts:167`) → MATCH
- description: "WireGuard mesh VPN control plane: peers, networks, routes, ACLs, DNS and setup keys."
  (`:301`) vs registry "WireGuard mesh VPN control plane" → extends it with the tab list (fine)
- icon: registry `Network` used as the Peers tab icon (`:24`) → MATCH; `Network` also collides with
  Service Discovery (`admin-registry.ts:182`).
- **S1 · an approved, connected peer can be shown as permanently "Pending approval", with its real
  state suppressed.** `:42-44` `isPendingPeer = Boolean(peer.pending_approval ||
  peer.approval_required)` ORs a *state* flag with a *policy* flag; `forge/api/internal/services/netbird/service.go:50-51`
  forwards both straight from NetBird with `omitempty`. `:366-372` renders the pending pill **instead
  of** Connected/Disconnected and re-offers Approve/Deny (`:376-382`), so on any account with peer
  approval enabled, peers that were approved stay in the pending branch and their connection state is
  never displayed. The one field that distinguishes them — `approved`/`pending_approval` alone — is
  not read separately.
- **S1 · "0 routing peers" and "0 peers" are rendered for values that were never reported.**
  `:415` `Pill tone="neutral">{network.routing_peers_count ?? 0} routing peers` and
  `:444` `{group.peers_count ?? group.peers?.length ?? 0} peers` — both fields are optional in
  `lib/api/netbird.ts:41, :48`. A subnet network whose routing-peer count is absent renders as a
  confident "0 routing peers", which to an operator means "this route is dead". AGENTS.md: "Unknown is
  not zero, not-reported is not zero." The page already knows the difference elsewhere (`:521` checks
  `data === null`).
- **S1 · setup keys: expiry can be silently removed, the applied expiry is never shown, and no key
  can be listed or revoked from the UI.** `:261` `expires_in:
  Math.max(0, Number(setupKeyForm.expiresInDays) || 0) * 86400` — clearing "Expires in (days)"
  produces `0`, and `0` in NetBird's `expires_in` means *does not expire*; the field is
  `json:"expires_in"` with no default guard
  (`forge/api/internal/services/netbird/service.go:158-162`). The created-key panel (`:575-585`)
  shows only name + secret, never `createdKey.expires` / `state` / `valid` / `revoked` /
  `usage_limit`, all of which exist in `lib/api/netbird.ts:95-111`. And the Setup keys tab is a
  **create-only form**: there is no list endpoint call and no `Revoke` anywhere in the page, even
  though `revokeNetBirdSetupKey` exists in the client (`lib/api/netbird.ts:242-244`) and
  `POST /admin/netbird/setup-keys/:id/revoke` exists in the API
  (`forge/api/internal/http/handlers_netbird.go:274-281`). The page tells the operator "Copy it now —
  NetBird only reveals the secret once" (`:271`) and then gives them no inventory and no way to
  retire a key they leaked. The type also allows `usage_limit`
  (`lib/api/netbird.ts:113-122`) — `NetBirdSetupKeyInput` has no `usage_limit` field at all, so an
  unlimited-use reusable key is the only thing this page can mint.
- S2 · **the secret is rendered as permanent plain text** (`:579`) with `title={createdKey.key}`, so
  hovering exposes the full credential in a native tooltip, the accessible name of that element is
  the secret, and the value survives tab switches (state lives in the page component, `:61`) until
  the route unmounts — no reveal/hide control, no auto-clear, no "stored once" acknowledgement.
  Rubric: VPN key masking.
- S2 · **the DNS tab's honest unavailable state is unreachable dead code.** `:521-522` renders
  "DNS unavailable — NetBird is not configured (NETBIRD_API_URL / NETBIRD_API_TOKEN are unset)…"
  when `dnsQuery.data === null`, but `renderQuery` (`:293`) already returns `AdminErrorState` for
  `isError`, and the API answers **503** in exactly that case
  (`handlers_netbird.go:13-19, :24-27` — the comment says the endpoints answer 503 "never an empty
  list that reads as an empty mesh"). So the carefully-written message never renders and the operator
  gets the raw Go string. The backend's honesty is undone twice: once by an unreachable branch, once
  by un-localised error copy.
- S2 · **no destructive action on this page is confirmed** — delete peer (`:384`), network (`:416`),
  group (`:445`), route (`:476`), ACL (`:505`). `useConfirm` is not imported (`:6-19`). Removing a
  peer ejects it from the mesh (it needs a setup key to return); deleting an ACL changes reachability
  for every group in it. Meanwhile Host Files confirms a single file delete.
- S2 · the header primary action is a five-way chameleon whose behaviour does not match its label —
  `:323-336`: on tabs peers/dns/keys the button reads "New setup key" but just calls
  `setTab("setup-keys")` (`:331`), and on the setup-keys tab the `else` branch re-selects the tab it
  is already on, so **the singleton button is a no-op on the tab it names**. The other four open
  modals. Rubric A/H.
- S2 · ACL creation hardcodes one rule shape and the list then pretends to report on it:
  `:206-215` fixes `enabled: true`, `action: "accept"`, `bidirectional: true` and one sub-rule with a
  client-generated `crypto.randomUUID()`; the list renders an `Enabled`/`Disabled` pill (`:504`) that
  can only ever be Enabled for rows created here, and `acl.rules?.length ?? 0` (`:501`) is shown only
  when there is no description. Deny rules, port ranges, rule editing and per-rule enable
  (`lib/api/netbird.ts:68-80`) are unrepresentable — a page titled "Access-control policies"
  (`:491`) that can create exactly one kind of allow policy and cannot show what the other fields of
  a real policy say. No `updateNetBirdACL` is wired either, so a wrong ACL must be deleted and
  recreated.
- S2 · route rows show the routing peer as a raw UUID: `:471`
  `route.peer ? \`via peer ${route.peer}\` : \`metric ${route.metric ?? 0}\`` — `peer` is an id
  (`lib/api/netbird.ts:58`) while the names are already loaded in the same component
  (`peerOptions`, `:77`), and the ternary *hides the metric whenever a peer is set*, so the two
  numbers that decide route preference are never visible together. `route.enabled` is shown
  (`:475`) but there is no toggle — an operator can see a disabled route and cannot re-enable it.
- S2 · the peer table drops the fields an ACL operator needs: `groups`, `dns_label`, `version`,
  `hostname`, `user_id`, `ephemeral`, `ssh_enabled`, `login_expired`, `country_code`
  (`lib/api/netbird.ts:12-32`) are all unused — so a peer whose `login_expired` is true and
  `connected` true renders as a green **Connected** (`:368-370`), and group membership, the input to
  every ACL on the page, is invisible. `connected` is also rendered without any `last_seen` sanity
  check (`:364` shows the timestamp but nothing compares it), which is the connected-vs-enabled
  distinction the brief asks about.
- S2 · `saveDnsMutation` (`:243-254`) PATCHes a document containing **only**
  `disabled_management_groups` (`:246`), while `NetBirdDNSConfig` is typed with more
  (`lib/api/netbird.ts:91`, `updateNetBirdDNSSettings` at `:230`). One text box silently replaces
  the DNS settings object.
- S2 · empty states render *below an empty table* on Peers (`:393` after the `</table>`, so headers
  Name/IP/OS/Last seen/Status float over nothing) and as a sibling row elsewhere (`:422, :451, :482,
  :511`); `EmptyState` is called with `message` only, so every card gets the default title "Nothing to
  show" (`admin-ui.tsx:616-618`).
- S2 · `createNetBirdNetwork` / `createNetBirdGroup` responses are checked for an id
  (`:123-126, :147-150`) and mutations are checked for `!peer` (`:88-91, :101-104`) — **this is the
  only page in the scope that refuses to report success on an empty response**; the same discipline
  should be lifted to the other five. Its `failToast` fallback string (`:83`) is also the scope's
  best error copy.
- S3 · tokens: `border-white/[0.06]`, `divide-white/[0.04]`, `hover:bg-white/[0.02]`, `text-slate-*`
  ×20, `text-[10px] uppercase tracking-widest` table head (`:349`), and the secret panel's
  `border-emerald-500/25 bg-emerald-950/20 text-emerald-300 text-emerald-100` (`:576-579`) — a
  bespoke five-colour success treatment; `rounded border-white/10` checkboxes (`:567, :627, :631`).
- S3 · `AdminTabs` called with no `label` (`:340`) and no `danger` marking on the ACL tab; five modals
  with the same hand-written Cancel/Confirm row (`:596-666`) instead of `AdminConfirmDialog` /
  `ModalFooter` consistency (`ModalFooter` is used, but the create forms for setup keys live inline in
  a card, not a modal — two form patterns in one page).
- S3 · `peers`/`networks`/… `?? []` (`:70-74`) plus `useMemo` on data that is already a stable array
  reference; no `refetchInterval` on any of the six queries, so the mesh status is a snapshot taken
  when the tab was first opened — for a VPN page whose central question is "is this host reachable
  right now".

---

## Scope-level patterns

1. **The ambiguous-node rule is enforced in Go and broken in React, nine ways.** The backend was
   hardened to reject `nodeId`-less host/k8s requests
   (`forge/api/internal/http/handlers_host.go:44-55`, `handlers_files.go:40-43`,
   `handlers_kubernetes.go:44-60`). Every page in this slice defeats it from the client:
   `node-select.tsx:11-31` auto-picks the first *active* node (Host Inspector, Host Files, Host
   Terminal, SFTP-adjacent), `docker-cleanup-manager.tsx:123` picks `nodes[0]` (first *listed*),
   `AdminKubernetes.tsx:69` and `incus/page.tsx:105` advertise an "Auto" mode that the API now
   refuses, and `terminal/page.tsx:147` opens a root shell socket with no target at all. Two different
   silent-picking rules and two different failure modes (wrong-node data vs raw 400) in one slice.
   **This is the scope's headline defect: destructive commands can be sent to a machine the operator
   never named.**
2. **Confirmation policy is arbitrary.** `useConfirm` gates a single *file delete*
   (`host-files-view.tsx:268`) and image prune (`docker-cleanup-manager.tsx:187`) but is absent from
   every genuinely destructive surface in the slice: Incus instance delete (`incus/page.tsx:142`),
   Nomad job **purge** (`nomad/page.tsx:55, :116`), Nomad drain (`:166`), all five NetBird deletes
   (`netbird/page.tsx:384, :416, :445, :476, :505`), Kubernetes scale-to-zero
   (`AdminKubernetes.tsx:112`) and host file chmod/upload (`host-files-view.tsx:327`).
3. **Beacon reports uncertainty and the panel deletes the evidence.**
   `beacon/internal/server/docker_cleanup.go:57-94` ships `sizeIncompleteCount`,
   `unattributedImageBytes`, `accountingNotes`, `containerCountKnown` and `reclaimedBytesKnown`;
   `forge/api/internal/services/dockerleanup/types.go:34-52` has no field for any of them and
   `service.go:241-248` decodes into that struct, so `docker-cleanup-manager.tsx:153` prints "0 B
   reclaimed" for an unmeasured prune and `:319-333` prints partial totals as complete. The same
   `?? 0` collapse recurs in NetBird counts (`netbird/page.tsx:415, :444`), Host Files sizes
   (`host-files-view.tsx:595` via `a.size ?? 0` at `:114`), Incus metric
   (`netbird/page.tsx:471`), Nomad type (`nomad/page.tsx:114`), SFTP numbers
   (`AdminSftp.tsx:128-131`) and the docker retention floor (`docker-cleanup-manager.tsx:346`).
4. **`AdminPageHeader`/`SectionHeader` gained an `info` API that does not exist.** Eight call sites
   in this slice pass `info={{…}}` (`files:13`, `terminal:272`, `AdminKubernetes:42`,
   `AdminSftp:75`, `docker-cleanup-manager:260`, `incus:78`, `nomad:72`, `netbird:302`) and all eight
   blocks are silently dropped, because `admin-ui.tsx:280-314` / `:140-162` define no such prop and no
   `PageInfoDisclosure` is rendered there. **In this slice, every sentence written to explain a
   missing capability is a sentence that an operator never sees** — including "Treat every keystroke
   as a privileged host action" and "Host archive and decompress are not exposed by Beacon yet".
5. **Status colour is decided in five places at once.** `lib/api/status.ts` is the declared authority
   and `nomad/page.tsx` + `incus/page.tsx` use it correctly; `AdminKubernetes.tsx:95` writes a
   case-sensitive `=== "Running"` ternary; `docker-cleanup-manager.tsx:80-84` keeps a private map;
   `app/admin/host/page.tsx:204` colour-codes a NIC from a free-text string; `AdminSftp.tsx:109, :210`
   paint a config boolean green/red; `docker-cleanup-manager.tsx:319-323` spends status tones on
   non-status categories. There is no `kubernetes` kind in `status.ts:221-248` — the gap forces the
   page-local map.
6. **Three token dialects across nine sibling pages.** `AdminKubernetes.tsx` uses `var(--line)` /
   `var(--text-subtle)`; `incus/page.tsx`, `nomad/page.tsx`, `netbird/page.tsx` and
   `app/admin/host/page.tsx` use `text-slate-*` / `border-white/[0.06]` / `divide-white/[0.04]` /
   `hover:bg-white/[0.02]`; `host-files-view.tsx` and `AdminSftp.tsx` mix both plus `ui-*` classes
   and hand-rolled button strings in one file. `app/admin/host/page.tsx:287` and
   `docker-cleanup-manager.tsx:328-331` choose their own hues for health and category encoding.
7. **Every runtime page invents its own empty/table/toolbar vocabulary.** Empty lists: `EmptyState`
   card (Kubernetes, Host Inspector), below-table `EmptyState` (NetBird), `colSpan` row (Incus,
   Nomad), replacing card (Host Files), `Card`+`CardHeader`+`EmptyState` (Host Inspector's node
   prompt). Toolbars: `AdminPageToolbar` is used by **none** of the nine; instead `AdminToolbar`
   (`host-files-view.tsx:475`), `AdminToolbar` re-styled to nothing (`terminal/page.tsx:298`),
   `SectionHeader action` rows (`kubernetes:63`, `incus:99`, `nomad:93`), `CardHeader action`
   (`AdminSftp:104`, `docker-cleanup:340`) and raw button rows (`host/page.tsx:241-256`). No page
   passes `status` to the header, so **no page in this slice shows a freshness signal at all** —
   three hand-roll "Updated n ago"/"Ready"/"Connected" strings instead.
8. **Node selectors are three different components with three behaviours.** `NodeSelect`
   (auto-picks, has an `sr-only` label: `node-select.tsx:33-45`), `AdminSelect label="Report on node"`
   (`docker-cleanup-manager.tsx:290-300`), `AdminSelect` with no label and an "Auto" placeholder
   (`AdminKubernetes.tsx:65-70`, `incus/page.tsx:101-106`) and no selector at all
   (`nomad/page.tsx`, `netbird/page.tsx`). Which node a page is talking to is therefore marked
   differently, or not at all, on five of the nine screens — and none of them shows the node's
   runtime capability, status or heartbeat, which `/admin/nodes` and `/admin/capabilities` do.
9. **`aria` and focus regressions cluster in the two keyboard-first pages.** Terminal: focus stolen on
   connect (`terminal/page.tsx:170`), unlabelled xterm surface (`:329-332`), a `hidden` container
   while a loading state shows (`:324-331`). Files: icon-only controls whose accessible name is a
   `title` (`host-files-view.tsx:603-666`), toolbar labels behind `hidden sm:inline` so names vanish
   below `sm` (`:485-519`), an unlabelled textarea (`:457`). Scope-wide: `AdminTabs`
   (`admin-ui.tsx:520-552`) emits `role="tab"` buttons but no page renders a `role="tabpanel"` or
   links one via `aria-labelledby`, so tab semantics are half-implemented on Host Inspector,
   Kubernetes, Incus, Nomad and NetBird.
10. **No route in the slice has `loading.tsx` or `error.tsx`**, and `app/admin/docker-cleanup/page.tsx`
    is the one wrapper that omits `"use client"`; `AdminSftp.tsx:16`, `docker-cleanup-manager.tsx:8`
    and `node-select.tsx:4` each contain an empty `import { } from "…"` statement.

## Proposed remediation for this scope

1. **[state] Delete client-side node defaulting; make "which node" a first-class, shared gate.**
   Remove `pickDefaultNode` and the auto-`onChange` effect from `components/admin/node-select.tsx:11-31`;
   keep the select unselected until the operator picks, and render the (already written, currently
   unreachable) `!nodeId` prompt card on every host page. One shared `NodeTargetGate` used by
   Host Inspector, Host Files, Host Terminal, SFTP, Image & Cache Cleanup, Kubernetes and Incus.
2. **[state] Gate the WebSocket on an explicit node.** `app/admin/terminal/page.tsx:147-148` and
   `:238`: do not construct the socket while `nodeId` is empty, and replace the "session may have
   expired" copy at `:212` with a cause-specific message. Show the target hostname in the shell
   chrome, and only mark `Connected` after the upstream Beacon dial succeeds — the backend already
   sends a status frame the client parses at `:181-192`; drive the badge off that frame, not off
   `ws.onopen`.
3. **[ia] Add the `info` prop for real, once.** Either accept `info` in `SectionHeader`/
   `AdminPageHeader` and render `<PageInfoDisclosure>` (the primitive already exists at
   `components/ui/page-info-disclosure.tsx`, and `admin-page-guides.ts` already holds this shape for
   other pages), or move the eight blocks into `adminPageGuides` and render them explicitly. Until
   then, the scope's capability disclaimers are invisible. **[copy]**
4. **[frame] Extract one runtime-console frame** — header + capability gate + node selector +
   freshness + `AdminPageToolbar` + table + empty state — and port Kubernetes, Incus, Nomad and
   Image & Cache Cleanup onto it. This single primitive removes the "each runtime page invents its own
   console" failure and makes Docker (the only verified path) the template rather than the exception.
   Gate the console (as `AdminKubernetes.tsx:80-82` already does) whenever the runtime is not
   reported by any node, and render create/run actions **disabled with a reason** per rubric M —
   Nomad `Submit job`, Incus `New instance`, NetBird's five create actions and SFTP's Save all
   currently invite a guaranteed 503.
5. **[state] Carry the honesty fields end to end.** Add `reclaimedBytesKnown`, `failedCount`,
   `skippedCount`, `errors`, `sizeIncompleteCount`, `unattributedImageBytes`, `accountingNotes` to
   `forge/api/internal/services/dockerleanup/types.go:34-52` and
   `forge/web/lib/api/docker-cleanup.ts:10-34`; render "reclaimed — not measured" in
   `docker-cleanup-manager.tsx:149-157`, "partial totals" next to `UsageBar`, and replace the
   category-sum "Total" with real filesystem capacity from the host disk endpoint the Inspector
   already fetches. **[ia]**
6. **[state] Make "0" mean 0 or nothing.** Remove the `|| 0` / `?? 0` coercions at
   `docker-cleanup-manager.tsx:138, :346, :591`, `AdminSftp.tsx:128-131, :320-324`,
   `netbird/page.tsx:415, :444, :471`, `host-files-view.tsx:114, :595`, `nomad/page.tsx:114`;
   use `AdminStatCard`/`ForgeMetric` (which already render `—` for absent, `admin-ui.tsx:647-659`)
   and `formatBytes`'s existing `"Unavailable"` branch (`lib/utils.ts:9`).
   Guard the two dangerous zero-cases explicitly: retention floor `0` and SFTP rate-limit `0` need a
   confirm line, not a silent save.
7. **[frame] One confirmation policy for destructive host/runtime actions.** Route Incus delete,
   Nomad stop-purge (and rename it "Stop and purge"), Nomad drain, NetBird peer/network/group/route/ACL
   delete, Kubernetes scale-to-zero, and host chmod/upload/write through `useConfirm` +
   `AdminConfirmDialog`, matching `host-files-view.tsx:268-273`. **[copy]** Add a preview (what will be
   removed, how much it is worth) before build-cache and volume prune.
8. **[tokens] Collapse the three dialects onto `var(--*)` + `ui-*` + `resolveTone`.** No raw
   `text-slate-*`, no `border-white/[0.06]`, no `bg-white/[0.018]`, no `text-[10px]/[11px]`, no
   per-page health colours (`app/admin/host/page.tsx:287`, `docker-cleanup-manager.tsx:328-331`);
   chart-category hues come from `lib/design-tokens.ts` `chart`, status hues from
   `components/ui/forge/status.ts`.
9. **[a11y] Fix the tab/tabpanel contract in `AdminTabs` once (`admin-ui.tsx:520-552`)** — emit
   `role="tabpanel"` wiring, `aria-controls`/`aria-labelledby`, and require an accessible name for
   icon-only `Btn`s — then delete the mobile-hidden-label pattern from `host-files-view.tsx` and add
   labels to the Terminal surface, the file editor textarea and the NetBird/SFTP bare `<select>`s.
10. **[frame] Add `FreshnessBadge` to all nine headers via the already-documented `status` slot**
    (`admin-ui.tsx:156-164`), fed from each query's `dataUpdatedAt`; delete the ad-hoc
    "Updated n ago" (`app/admin/host/page.tsx:103, :139, :176, :214, :281`), "Ready"
    (`host-files-view.tsx:68`) and dev-only JSON retry counters
    (`terminal/page.tsx:334-345`, whose `retries` value cannot be right because it is a ref read
    inside a memo — `:259-265`).
11. **[ia] Registry edits:** title "Host Management" → "Host Inspector"
    (`app/admin/host/page.tsx:351`); align the three paraphrased descriptions
    (`app/admin/host/page.tsx:344-346`, `AdminSftp.tsx:74`, `AdminKubernetes.tsx:41`); resolve the
    `Boxes` (Registries/Kubernetes/Incus), `HardDrive` (App Storage/Cleanup/Backups/Mounts),
    `Workflow` (Nomad/Procedures) and `Network` (NetBird/Discovery) collisions; and stop marking
    `capability: "available"` (`admin-registry.ts:164-167`) for runtimes that no node reports —
    the vocabulary for that (`"metadata-only"`) already exists at `admin-registry.ts:49, :228` and is
    rendered in the command palette (`command-palette.tsx:204`) but never on a page.
12. **[copy] De-brand the error path.** `handlers_incus.go:99` says "Forge Virtualization",
    `handlers_nomad.go:43-49` says "Forge Orchestration", `handlers_netbird.go` leaks
    `NETBIRD_API_URL / NETBIRD_API_TOKEN`; each of the nine pages renders these strings verbatim into
    `AdminErrorState`. Map them to operator language at the handler or in `errorMessage`.
13. **[ia] Rename or remove the NetBird "About" mechanism mismatch, restore the DNS
    not-configured branch** by checking `dnsQuery.data === null` *before* `isError` in
    `renderQuery` (`netbird/page.tsx:286-295`), and wire the existing
    `revokeNetBirdSetupKey` (`lib/api/netbird.ts:242`) plus a setup-key list into the Setup keys tab.

## Open questions for the orchestrator

1. **Is the `info` prop intended to be added to `SectionHeader`, or are those eight blocks meant to
   move into `admin-page-guides.ts`?** The fix is one primitive edit versus eight page edits, and it
   affects roughly thirty pages outside this scope — a decision the audit must not make alone.
2. **Does host access require an explicit node, per AGENTS.md, or is a client-side default allowed
   when exactly one node exists?** The Go layer already answers "explicit" (`handlers_host.go:44-55`);
   `node-select.tsx` and `docker-cleanup-manager.tsx:123` answer "default". One policy must be chosen
   and applied to the four host-access pages plus the three runtime consoles. (Incus's
   "auto-select when count === 1" rule at `handlers_incus.go:93-99` is a third position.)
3. **Should the Host Terminal require a typed confirmation, a scoped "host shell" permission, or both**
   before attaching a root shell — and should `/admin/terminal` be gated on a node capability at all?
   The page currently confirms nothing and its own warning copy is unreachable (`terminal/page.tsx:288`).
4. **Is `Image & Cache Cleanup` meant to show filesystem capacity, or only the four Docker
   categories?** The current headline "Total" is a category sum
   (`dockerleanup/service.go:115`); making it honest needs either a Beacon disk-capacity field or a
   rename to "Measured usage". Host Inspector already reads partition totals
   (`app/admin/host/page.tsx:126-136`), so the data may exist one endpoint away.
5. **Do we ship runtime consoles for Kubernetes/Incus/Nomad as *view-only* until they are verified?**
   AGENTS.md says Docker is the only verified production path, yet this slice can scale deployments,
   delete VMs, purge jobs, drain nodes and mint mesh credentials for those runtimes. That is a product
   decision, not a UI fix.
6. **Where should non-Docker status vocabularies live?** `lib/api/status.ts` has `nomad`, `incus` and
   `discovery` kinds but no `kubernetes`, `docker-cleanup-policy` or `sftp-config` — adding them there
   is the right shape, but it changes a file owned by several other scopes.
