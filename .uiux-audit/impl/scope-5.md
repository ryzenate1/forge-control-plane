# Impl scope 5 — Infrastructure A (nodes, regions, containers, mounts, locations, capabilities, onboarding tokens, cloud)

Appended page by page; sections are complete for the pages listed under them.

## Done

### Nodes — `/admin/nodes` + `AdminNodes.tsx`
- `components/admin/AdminNodes.tsx`: verified the half-finished `AdminTable` migration is complete (all 12 cells carry `className`, table inside `AdminTable`'s `overflow-x-auto`); the repaired `desiredState ?? draining` line is now `nodeStatus(node).label`, so no page in this file invents a state word any more.
- Copy: deleted `title`/`sub` from `SectionHeader` (registry supplies both). Added `<FreshnessBadge state={sourceState(nodesQuery, REFRESH.inventory)} />` to the previously unused `status` slot — first real freshness claim on this page.
- Telemetry honesty: a failed `/system-information` read renders `NotReported` with the failure reason instead of `"Offline"`; `Docker` renders "Available/Unavailable" only once `sys` exists and stays a dashed unknown while loading (it used to read *unknown* in red).
- Unknown ≠ 0: memory/disk totals now go through `capacityTotal()`, which refuses to sum a half-reported pair (was `(allocated ?? 0) + (available ?? 0)`); declared caps use `mbLabel` so an unset limit is a dash, not `" MiB"`; the server count is `null` (→ "Server inventory not loaded") until the servers query answers, and the `N / M` header count is withheld until the nodes query resolves.
- `KpiDatum.live` is now `source.status === "ready" && !stale` per source, not `Boolean(cap)`; each tile's sub line names its source and its age.
- Status vocabulary: list dot, list pill and the detail modal all read `nodeStatus()`; the modal's three-way branch that called any non-maintenance node "Active" (so an offline node pill'd Active) is gone. Added a drift marker from `hasDrift()` on rows and a "Not reconciled" pill in the modal, because `actualState` and `desiredState` are different measurements that the page never reconciled.
- `DashHeader` deleted from inside the node modal (rule: never a page-header primitive in a `Modal`; it also carried the hardcoded red eyebrow and an unmeasured live pulse); replaced with a plain card that keeps the pill, tags, meta and the delete action.
- SSL no longer defaults to https: an unset scheme is a dashed unknown, not a green lock. Public/visibility unset renders unknown rather than "Private".
- Region: `CreateNodeModal` now sends `regionId` from an explicit region select, so `ListRegions`' node count and `DeleteRegion`'s guard key off a column the UI actually writes. Settings shows the linked region read-only and states that the API has no update field for it. A legacy free-text region reads "not linked".
- Capability gating: scheduler selects now name the runtime the host actually reported and mark the others unverified, instead of offering Nomad as if it were known.
- Table: sortable name/state/heartbeat/location/memory/disk/servers headers (button + spoken direction inside `AdminTh`); servers-per-node computed once per render instead of an O(n·m) scan per row, with identical semantics.
- A11y/tokens: search `Input` gained a label; icon-only cells gained `sr-only` text; `bg-emerald-500`/`text-red-400`/`bg-slate-500`/`text-amber-400` and the red-ring `bg-surface-card-header` selects replaced with `toneStyles`/token classes; truncated capability dumps now say how much was cut; page-local `toLocaleString` replaced with the shared `formatDate` + `relativeTime`.
- Write-only create fields are disclosed as such next to the controls they affect (stored at create, never read back), and the disabled Create button states its reason.
- `app/admin/nodes/[id]/layout.tsx`: document title "Beacon — Forge Admin" → "Node — Forge Admin", to match the registry label the sidebar and breadcrumb use.
- Note: `/admin/nodes` and `/admin/nodes/[id]` are already covered by `app/admin/loading.tsx` and `app/admin/error.tsx` at the parent segment, so no per-route files were added.

### Alias stubs
- `app/admin/containers/page.tsx`: `permanentRedirect` → `redirect`. A permanent redirect is cached by the browser, so the alias could never be repointed without a hard reload.
- Created the alias routes `ADMIN_ALIAS_ROUTES` maps but that did not exist on disk — `app/admin/beacons/`, `app/admin/infra/`, `app/admin/infra/beacons/`, `app/admin/infra/storage/`, `app/admin/infra/networking/`, `app/admin/storage/`, `app/admin/volumes/`. Each was a 404 for a bookmark or saved deep link, since the registry's alias table only affects nav resolution, never routing.

### Regions — `/admin/regions` + `AdminRegions.tsx`
- S1 fixed: a failed read no longer renders "No regions configured". The table branch now runs loading → error → empty in precedence, and the error box says outright that the list is unread rather than empty.
- Copy: dropped duplicated `title`/`sub` so the frame derives them from the registry; added `refetchInterval: REFRESH.inventory` and a `FreshnessBadge` in the frame's `status` slot and on the card header.
- The card header count is withheld until the query resolves; `createdAt`/`updatedAt` (on the wire, never rendered) now appear as an Updated column.
- Region↔node link: `nodeCount` zero now explains itself ("no node has this region linked") and the info disclosure states the one real rule — a node links a region only at creation, and the node update endpoint has no region field. Paired with the `regionId` write in Nodes, the count and the delete guard finally key off the same column the UI fills.
- Destructive: delete button gained an accessible name and a visible reason line when withheld (it used to be an unnamed icon button whose disabled state explained nothing); confirm text now names the slug and what is/isn't affected.
- Notify: `useToast` like the rest of the group (previously create/update/delete succeeded in total silence); a failed delete now surfaces on screen as well as in the toast.
- A11y/tokens: search field labelled, headers given `sr-only` names, the slug validation hint moved off body-copy colour onto the danger tone, token classes throughout, registry glyph `GlobeGridIcon` replaces the ad-hoc `Map`.

### Locations — `/admin/locations` + `AdminLocations.tsx`
- The page's own disclosure promised a delete guard the UI did not implement; now the delete control is withheld on rows with nodes and states the count, matching Regions, instead of letting the server refuse after the click.
- `short` now has a stated contract: required, and a duplicate is blocked with the reason (the code is what the Nodes table and a node's legacy region string display, so two identical codes are ambiguous). The API only requires non-empty, which is recorded in the comment rather than being silently over- or under-enforced.
- Copy/`title`/`sub` deleted in favour of the registry; freshness badge + `refetchInterval` added; `createdAt` rendered.
- Icon-only delete button named and labelled; the brand-coloured short code that pretended to be a link is neutral; error/empty/loading copy aligned with Regions; switched from the lone `sonner` `toast` import to the shared `useToast` so the group has one notify policy.
- Create/edit state leakage between modal openings fixed (opening create after an edit inherited the edited row's values).

### Node Capabilities — `/admin/capabilities` + `AdminCapabilities.tsx`
- S1 fixed: the tab no longer reads `Inventory · 20` and the card no longer reads "20 nodes". Both now say what they are — "N of 20 snapshots on this page" with the `offset/limit` window — and the footer states plainly that the endpoint reports no total, so no fleet count is implied.
- S1 fixed: search no longer answers "no node has ever reported". A miss says "No match on this page", names how many rows it actually searched and points at Next; only a genuinely empty page renders the no-snapshots state. The endpoint-named remedy ("Beacons report via POST /nodes/capabilities") is gone from operator copy.
- Probe is now rendered **disabled with a reason** when no node is selected instead of vanishing.
- The dead `nodesQ` (`fetchNodes`, fetched and never used) is now `useNodesQuery` and actually feeds the picker and the node column: hosts read `name · id · os · version` instead of a bare truncated hex id, which was the fingerprint of the missing naming contract.
- "No drift — snapshot is stable" no longer fires on an empty comparison: with all buckets empty and nothing unchanged the panel says stability is **unknown**, matching the server's zero-entry case.
- Freshness: all four queries gained `refetchInterval` and the frame/card carry a `FreshnessBadge`; the Updated column pairs the absolute timestamp with its age and flags a snapshot older than twice the cadence.
- Uptime renders unknown rather than "0h" when unreported; untimestamped fetches say so instead of `new Date(undefined).toLocaleString()`.
- Truncated JSON dumps (`slice(0, 600)` / `slice(0, 400)`) now state how much was cut; the raw-JSON panels are labelled "Raw report (diagnostic)".
- `AdminTabs` gained its accessible name; tokens replace `emerald/red/amber-*`, `bg-black/20` and `color-mix` brand washes; the info disclosure now states outright that these capabilities are **reported, not enforced** — the gating gap that the Nodes page inherits.

## Verified

## Needs central change (do NOT edit these yourself)

## Removed

## Deferred
