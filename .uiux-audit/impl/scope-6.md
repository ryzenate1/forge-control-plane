# Impl scope 6 — Infrastructure B (host access + alternative runtimes)

Working record, appended page by page. Slice: host inspector, files, terminal, sftp, docker-cleanup,
kubernetes (component frozen), incus, nomad, netbird.

## Done

### Host Inspector — `app/admin/host/page.tsx` (rewritten)
- Explicit node targeting: deleted `pickDefaultNode` + the mount effect that auto-assigned a node and
  the page's own duplicate `useQuery(["nodes"], fetchNodes)`. One `useNodesQuery()`, one
  `AdminSelect label="Target node"` with a `Select a node…` placeholder; nothing fetches until the
  operator names a node, so the (previously unreachable) "no node selected" prompt is now the real
  first state. This idiom is what Files/Terminal/SFTP/docker-cleanup/Incus now match.
- Removed hand-passed `title="Host Management"`/`sub` — the frame derives "Host Inspector" + the
  registry description from `admin-registry.ts`; the node-dependent "Real-time system data…" subtitle
  is gone because the page polls at 30 s / 10 s and never streamed anything.
- Added a real `info` disclosure (renders now that `SectionHeader` accepts `info`) stating the named-node
  rule, the actual poll cadence, and how unmeasured fields are drawn.
- Unknown ≠ 0: `num()`/`orUnknown()`/`fmtMB(null)` helpers. `CPU Cores` no longer prints the literal
  string `"undefined"`, `Time` no longer prints `"Invalid Date"` (shared `formatDate` with a
  "Not reported" fallback), unmeasured partitions/memory render `—`-style meters instead of a green
  `0.0%` bar, `String(data.cpuCores)` replaced, and process percentages guard on `Number.isFinite`.
  Utilisation tone comes from `resolveTone`-backed `SubsystemHealthMeter` tones (`ok/warn/danger/unknown`),
  not a page-local `bg-red-500/bg-amber-500/bg-emerald-500` pick.
- Interface status: `emerald if "up" else red` replaced with `Pill tone={statusTone(iface.status, "discovery")}`,
  so `unknown`, `down` and `lowerlayerdown` are no longer the same red.
- Freshness: five hand-rolled `Updated {fmtTime}` strings (which rounded everything under 10 s to
  "just now" and could not say *stale*) replaced by `FreshnessBadge state={sourceState(query, interval)}`
  per tab, using the shared helper.
- States re-ordered to loading → error → empty for every tab; `placeholderData` kept-previous-data
  removed so a node switch can never show the previous machine's reading.
- Error copy: `hostErrorMessage` keeps 504/502/404/503 mappings, passes `ApiError.message` through for
  400/401/403 instead of swallowing a `nodeId is required` behind "An unexpected error occurred".
- Processes table: says "Showing the 100 busiest of N reported processes" instead of silently truncating;
  moved onto `AdminTable` primitives; sort toggles use `Btn` tones instead of `border-red-400/50 bg-red-500/10`.
- A11y/heading: removed the second header (`CardHeader` whose title changed with the tab) and rendered a
  `role="tabpanel"` region with an accessible label around the tab body; tab glyphs fixed
  (`Memory` → `MemoryStick`, `Processes` → `ListChecks` — it used `Cpu`/`Server`).
- Tokens: all `text-slate-*`, `bg-white/[0.018]`, `border-white/[0.07]`, `divide-white/[0.06]`,
  `text-[11px]` → `text-text*` / `border-line` / `bg-overlay-subtle` / `t-meta`-scale classes.

### Host Files — `app/admin/files/page.tsx` + `components/admin/host-files-view.tsx` (rewritten view)
- Explicit node targeting, same idiom as Host Inspector (`AdminSelect label="Target node"` with
  `Select a node…`). `NodeSelect` is no longer used, so its frozen auto-pick cannot fire here. The list
  query is `enabled: Boolean(nodeId)`, so a first visit renders the "No node selected" prompt instead of
  the 400 banner `nodeId is required: specify which Beacon this request targets`. `run()` refuses an
  untargeted action with the same message.
- `?nodeId=` is honoured as an explicit target (it is how the Terminal hand-off arrives) and written back
  with `history.replaceState`, so a directory view is shareable and back/forward does not drop the target.
- Removed the two controls that could not do their labelled job: **Archive** (enabled, and on files it only
  performed a plain download; on directories it only set an error string) and **Decompress** (rendered for
  `.zip/.tar/.tgz/.gz`, always failed with a toast). Verified against the Go side: `handlers_files.go`
  registers list/read/write/mkdir/rename/copy/remove/chmod/upload/download — there is no host archive,
  decompress or pull endpoint. The gap is now stated once, honestly, in the page's `info` disclosure
  ("No host archive surface") instead of as a 40-word implementation note in the body.
- The dead `info` block now renders (the frame accepts it), and its copy was corrected: it no longer says
  "Host archive and decompress are not exposed by Beacon yet — download single files here" as a footnote to
  a button that still exists; it says the page offers no archive action at all.
- Privileged writes are confirmed: chmod (single and bulk) and any `writeFile` to a system path
  (`/etc`, `/boot`, `/usr`, `/bin`, `/sbin`, `/lib*`, `/root`, `/sys`, `/proc`, `/dev`, `/var`) go through
  `useConfirm()` naming the path **and the node**. Deleting names the node and, for directories, says the
  contents go too. Bulk batch delete/chmod now report partial failure
  ("Permissions: 2 of 5 failed (first failure: …). The failed paths are still selected.") instead of one
  label for N operations and a selection cleared regardless of outcome.
- Unsaved-work loss fixed: closing the editor or navigating away while `status === "Edited"` asks
  "Discard unsaved changes?" before dropping the buffer.
- Unknown ≠ 0: size renders `Not reported` when `entry.size` is not a finite number (and unmeasured sizes
  sort last rather than being treated as 0 bytes); modified uses the shared `formatDate` with a
  `Not reported` fallback; a new **Permissions** column shows `entry.mode` (previously used only to prefill
  the chmod box, so an unreported mode silently became an empty input).
- A11y: every toolbar control keeps its short label at `sm`+ but has `ariaLabel`+`title`, so nothing
  reduces to an anonymous icon below `sm`; the Upload `<label>` wraps an `<input>` with an `sr-only` name;
  the refresh button is named; the editor `<textarea>` is labelled with the file path and node.
- Loading/empty moved to `AdminLoadingRows` / `EmptyState`; the duplicated row grid is one `ROW_GRID`
  constant used by header and body; the Modified/Permissions cells are visible below `sm` instead of
  `hidden`.
- The five hand-rolled `fixed inset-0` dialogs (create/rename/copy/chmod/bulk-chmod/pull, mixing `ui-*`
  classes, `bg-[color-mix(…)]` overlays and ad-hoc button strings) are one `FormModal` over the shared
  `Modal` + `Btn` primitives, with Enter-to-submit preserved through a real `<form>` and a `type="submit"`
  confirm. Drag-and-drop counting uses a `useRef` depth counter (the old `useState` tuple had an unread
  value and drove `setDragging` from inside an updater, which React does not guarantee once per event);
  `handlePull`'s dead `dest` is now the value reported back to the operator ("Pulled to /path/x").
- Tokens: `bg-white/[0.03]`, `hover:bg-white/[0.06]`, `text-white`, `border-red-500/30 bg-red-500/10
  text-red-200` bands, `text-red-400` delete, `text-amber-400/80` folder, `text-[11px]` → `text-text*` /
  `border-line` / `bg-overlay-*` / `text-warn` / `text-danger` and the shared `AdminErrorState` band.
- Dropped this file's `sonner` `toast` import: with the fake archive/decompress handlers gone the view has
  no toast call left, so it no longer runs a second toast system next to `useToast`.
- Removed the hand-passed `title`/`description` from `app/admin/files/page.tsx` (exact duplicates of the
  registry row) and kept only `info`.

### Host Terminal — `app/admin/terminal/page.tsx` (rewritten)
- **The scope's headline defect, closed.** No socket is ever constructed unless the operator has picked a
  node *and* pressed **Attach shell** (`if (!attachTarget || !terminalReady) return;` before the URL is
  built). The mount race that opened `/host/terminal/ws` with no `nodeId` → `resolveNodeMiddleware` 400 →
  `ws.onerror` is gone, and with it the false diagnosis "your session may have expired — sign in again"
  written into a root shell that had nothing to do with the session.
- The shell target can't drift: `handleNodeChange` detaches (closes the socket, resets the backoff) instead
  of silently re-pointing a live privileged session at another machine, and the Attach button names the host
  it is about to open (`title="Open a privileged shell on <name>"`).
- **Honest session states.** `Session` is a discriminated union (`unattached / checking / opening /
  open-awaiting-host / attached / retrying / closed / failed`) and the header `status` slot renders its
  `Pill` tone. `ws.onopen` no longer means *Connected* — the Fiber route upgrades the browser hop before the
  upstream Beacon dial is known (`handlers_files.go:107-140` writes an error frame after the upgrade), so the
  badge sits at "Socket open · host not confirmed" (info tone) and only becomes **Attached** when host bytes
  or a `status:"connected"` frame actually arrive. `{"error":…}` frames (which the route really sends — the
  old parser only looked for `status`) are now read as failures instead of being printed into the terminal as
  noise while the badge stayed green.
- `checkApiReachable()` now gates the connect: unreachable → one failure state, no socket, so the three
  overlapping claims about the same non-event ("Connecting…", "API unreachable", "Connection failed") are
  one honest message.
- Reconnect counter is real state (`attempts`, mirrored from `attemptRef`), so "attempt N/15" cannot show 0
  after several retries. The dev-only `statusJson` dump — whose `retries: 0` was a ref read inside a `useMemo`
  and therefore could not be current — is replaced by one always-visible, accurate caption:
  `Shell target: <name> · privileged host actions · reconnect attempt N/15`.
- Node identity and the privilege warning are now in the page chrome, not only in a disclosure: the caption
  above names the machine the keystrokes go to, and the `info` block (which renders for real now) keeps
  "Treat every keystroke as a privileged host action" plus a rewritten "What the badge means" section.
- Removed the duplicated page title (`CardHeader title="Host Terminal"` under the `<h1>`) and the
  `AdminToolbar` that had been re-skinned to `border-0 bg-transparent p-0` to pretend it was not a toolbar;
  controls sit in the card's target row, the frame supplies the single `<h1>`, and hand-passed
  `title`/`description` are gone so the registry description ("Secure host shell and console access") shows.
- States precedence fixed: runtime-load failure now renders `AdminErrorState` **only** (it used to render
  "Initializing terminal…" beside the error, claiming both "still starting" and "broken"), the xterm surface
  is no longer `hidden` while a loading state shows, and it has `role="region"` + an `aria-label` naming the
  target so a screen reader can tell what the region is.
- `?nodeId=` is hydrated once *and* written back via `history.replaceState`, so an attached target survives
  back/forward instead of re-triggering the mount race; tokens replaced (`border-emerald-500/30
  bg-emerald-500/10 text-emerald-300`, `text-[11px]/[10px]`, `bg-black/20`) with `Pill` tones and
  `text-meta`/`var(--canvas)`; the `/* intentional terminal chrome */` comment that had been written into a
  Tailwind class string is gone, and the `h-[calc(100vh-20rem)]` magic subtraction is now
  `h-[60vh] min-h-[320px]`.
- Focus is still taken on `onopen`, which is now inside the user's Attach gesture rather than on mount.

### SFTP — `components/admin/AdminSftp.tsx` (rewritten) + `app/admin/sftp/page.tsx` (untouched passthrough)
- **"N nodes" is no longer a lie in flight**: the card header read `nodesQ.data?.length ?? 0`, rendering
  "0 nodes" for the whole load window and "1 nodes" for a twenty-node fleet with one override. It now says
  `Counts unavailable while loading` / `Overrides not read` / `3 overrides of 20 nodes`, so the two cards
  (overrides, nodes-without-override) finally share one fleet denominator.
- **Blanking a numeric field can no longer write 0.** Both forms hold strings and validate on save:
  `parseNumber()` rejects `""` with "…needs a number. Leaving it blank is not the same as setting 0." and
  rejects non-integers/negatives. Rate limit 0 stays meaningful and is now *shown* as such — an inline
  amber line says "Rate limit 0 lifts the bandwidth cap for every node without an override", and Max
  connections 0 says "no node will accept an SFTP session on these defaults".
- **Every save goes through `useConfirm()`** with a sentence naming the concrete effect: the node the
  override belongs to, port/connections/auth/idle/rate values, and — the access-control case the audit
  called out — "Allowed IPs is empty, so any source address may connect." Allowed IPs now validates
  IPv4/CIDR/IPv6 shapes instead of accepting any text into an access-control list.
- **The enabled/disabled pill stopped impersonating node health.** It is a stored column, so it reads
  `enabled in config` / `disabled in config` in the neutral tone, and the card carries the line
  "Configuration only — the panel does not probe this node's SFTP listener." Same caption appears in the
  editor and in the `info` disclosure ("What 'enabled' means here").
- **Create vs edit is now visible and cannot cross-contaminate**: the editor is mounted with
  `key={mode:nodeId}`, the modal title and save label say *Create SFTP override* or *Edit SFTP override*,
  and the `Configure` flow no longer lands on an error page — `GET /admin/nodes/:id/sftp` 404s exactly when
  there is no override yet (`handlers_sftp.go:41-52`), so a 404 is detected (`ApiError.status === 404`) and
  the form opens prefilled from the current global defaults, which is the behaviour the page advertises.
  A non-404 read failure still shows `AdminErrorState` and says saving would overwrite a configuration the
  page has not read.
- Nodes-without-override no longer disappears when the overrides query fails: the failure is reported with
  "Without this read the page cannot say which nodes have an override", and a node-list failure gets its own
  error card instead of an absent section.
- Log level: one shared `LOG_LEVELS` list (`error/warn/info/debug/trace`) in both forms, and an unrecognised
  stored value is preserved as its own labelled option ("xyz (unrecognised)") rather than silently blanking.
- `updatedAt` uses the shared `formatDate` with "Not recorded" for absent; the concatenated Limits cell became
  a labelled `<dl>` (connections / auth attempts / idle / rate) so each value is readable and comparable;
  the banner is shown as visible text ("No banner set") instead of living only in a `title`; `listenIP`
  blank now renders "all addresses" rather than `0.0.0.0:`-style invention.
- The dead `info` block renders (frame accepts `info`), the empty `import {  } from "@/lib/api"` is gone, the
  table moved to `AdminTable`/`AdminTh`/`AdminTd` primitives, the Edit action's `Shield` glyph became a pencil
  (an edit action was wearing a security glyph), the retry-less `retry: false` queries now retry normally with
  an explicit error state, and the raw palette (`text-slate-400`, `border-red-500/30 bg-red-500/10`,
  `border-emerald-500/20 …`, `text-[10px]`, `text-[11px]`, `max-w-[16rem]`, `min-h-[60px]`) is replaced by
  `text-text*` / `border-line` / `bg-overlay-*` / `text-meta` and the shared `AdminErrorState` band.
- Header `title`/`sub` duplicates of the registry removed; the registry description is what renders now.

### Image & Cache Cleanup — `components/admin/docker-cleanup-manager.tsx` (rewritten) + `app/admin/docker-cleanup/page.tsx` + `lib/api/docker-cleanup.ts`
- **The destructive `nodes[0]` default is gone.** `activeNodeId = nodeId || (nodes[0]?.id ?? "")` — the line
  whose comment proudly said "Default to the first node once the list arrives so the panel is useful
  immediately" — is replaced by `nodeId`, which starts empty. Disk usage and unused-images queries are
  `enabled: Boolean(nodeId)`, all three prune mutations use `nodeId`, and with no selection the page shows
  "No node selected" and states that pruning is not reversible. Because this route takes the node in the
  **path**, the backend's ambiguity guard never saw these requests — the client was the only gate.
- **Prune results can no longer toast "0 B reclaimed" for an unmeasured prune.** `pruneSummary()` reads the
  payload honestly: 0 removed → "nothing was deleted" (a real result); >0 removed with
  `reclaimedBytes === 0` → "**reclaimed space not measured by the engine**" and a `warning` toast, not
  `success`. The reason it has to be inferred is documented at the type (`lib/api/docker-cleanup.ts`) and in
  the function comment: Beacon sends `reclaimedBytesKnown`, the Go `dockerleanup.PruneResult` has no field
  for it, so the flag is lost before this file ever sees it. Same treatment for the per-object
  `failedCount`/`skippedCount`/`errors` — see "Needs central change".
- **"Total" renamed to "Measured"** and the bar relabelled: the card says these four categories are what the
  engine reported, summed by the panel, "not the size of the disk and says nothing about free space", with
  Host Inspector → Disk named as the place that has per-mount totals. Percentages are captioned "each
  category's share of the N measured, not of the disk", and a read with no bytes renders a dashed
  "unmeasured read, not an empty disk" panel instead of an empty bar.
- **Retention floor can no longer be cleared into 0 by accident.** `limitText` + `limit` validation: a blank
  or non-integer value stops re-reading the list and says so; `0` renders the opposite of the old
  self-contradicting sentence — "Retention floor 0 preserves nothing: every image not in use becomes a prune
  candidate, including the version a workload is running" — and the policy editor carries the same wording,
  marks its footer destructive at 0, and shows `keep 0 (nothing preserved)` in the table. The floor is now
  also stated inside the image-prune confirmation.
- Reclaim badge fixed: while `unusedQuery` is loading it reads "Analyzing images…" instead of
  "0 B reclaimable" next to a spinner; otherwise it reports the sized total plus how many images arrived
  unsized ("… in 12 sized images + 3 unsized"), with a footer line explaining that the estimate is a lower
  bound because the engine's accounting gaps are not carried to this panel.
- Capability gating: a node whose `runtimeProvider` is not Docker-family disables all three prunes with a
  one-line reason instead of firing a command at a machine that has no Docker engine (AGENTS.md — disabled
  with a reason, never enabled-and-lying).
- Confirmation quality: the two bulk prunes now carry the last measured `buildCacheBytes` / `volumesBytes` in
  their confirmation text and say plainly that no per-volume list is previewable and that skipped counts may
  be unreported. The "Volumes still in use are preserved" claim is kept as the engine's behaviour but no
  longer presented as a verified post-condition. Both bulk prunes are `tone="danger"` — the destructive
  actions were the *subtle* ones next to "Prune selected" while being unpreviewed.
- Policy editor: `All nodes (global)` is a real option (`{ value: "" }` prepended), so a global policy can be
  created *and* an existing global policy can be edited without silently re-scoping it to a node; "New
  policy" is no longer disabled by the absence of a node selection. Schedule/retention validate before the
  request, with the errors rendered in an `AdminErrorState` band.
- `Next run` / `Last run` now say three different things: `Not scheduled` (enabled, nothing queued),
  `Never run` (no run yet), `disabled` — previously `formatDate`'s default "Never" made an unmeasured next
  run read like a permanent one, identical to "never run".
- Per-row mutation state: Run-now and Enable/Disable key off `mutation.variables`, so clicking one row no
  longer draws a spinner on every row or disables every other row.
- A11y/tokens: icon-only policy actions got `aria-label`s (the actions column previously held anonymous
  destructive buttons), tables use `AdminTable label=`, `text-slate-*`/`bg-white/[0.04]`/`text-[11px]`
  replaced, category KPIs no longer spend status tones (`yellow` on Containers, `green` on Build cache are
  all `neutral` now), and the bar hues come from `chart` in `lib/design-tokens.ts` instead of raw
  `bg-sky-500/bg-violet-500/bg-emerald-500/bg-amber-500` classes. `Badge className="bg-sky-500/15
  text-sky-300"` (an invented "reclaimable" chip colour) is gone.
- The header now carries a real `FreshnessBadge` from `usageQuery.dataUpdatedAt`, and Refresh is disabled
  until a node is chosen (it used to bind `loading` to one query while three others existed). The `info`
  block renders and was rewritten: no longer promises the retention floor "always" protects the deployed
  image without saying that 0 does the opposite. `app/admin/docker-cleanup/page.tsx` gained `"use client"`
  to match its sibling wrappers.

### Verified so far
- `npx tsc --noEmit` filtered to `app/admin/host/page.tsx`, `app/admin/files/page.tsx`,
  `components/admin/host-files-view.tsx`: clean. Repo-wide baseline during this pass is
  34 pre-existing errors in other scopes' files (`backups`, `dns`, `domains/[id]`, `AdminDatabases`,
  `AdminDrain`, `AdminNodes`, `AdminPlugins`, `AdminSettings`, `components/database/*`,
  `admin-registry.ts:41` TS1005) — none of them mine, none touched.

## Needs central change (do NOT edit these yourself)
- pending

## Removed
- pending

## Deferred
- pending
