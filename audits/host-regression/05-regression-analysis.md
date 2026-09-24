# 05 — Regression Analysis

> Proves which commit fixes, which commit regresses, and why infinite-load / "node offline"
> reappear in current HEAD+workdir.  SHA prefixes are unambiguous within this repo.

## 5.1 Fix commit and its scope

```
4177276  bug fixed   (Wed Jul 29 20:17:43 2026 +0530, parent de811c4)
  git show --stat:
   forge/web/app/admin/host/page.tsx          140 insertions  (useHostQuery, error UI, NodeSelect)
   forge/web/lib/api/host.ts                  + init?: RequestInit  (abort support)
   forge/api/internal/daemon/host.go          + 10*time.Second timeout
   forge/api/internal/http/handlers_host.go   + mapDaemonError (504 vs 502)
```

Before 4177276 (`de811c4` / `9921ccc`) Host was broken in two ways:

* **Infinite load:** `InfoTab` did `useQuery({queryKey:["host-info"], queryFn:fetchHostInfo})`
  with no timeout, no error branch, and only `if (isLoading) return "Loading..."`.  Any
  Beacon stall (no timeout before 4177276) or queue stall left the spinner forever.
* **Undifferentiated 502:** handler returned bare `502 err.Error()` so every daemon failure
  looked like "offline" with no timeout hint.

After 4177276 both are **server-fixed**: daemon Host calls bounded at 10s, handlers emit
504 vs 502, client wraps fetches in 15s AbortSignal with retry 1 and human-readable errors.
The fix is present in HEAD (`ca06f74`): `git diff 4177276:handlers_host.go HEAD:handlers_host.go` is empty.

What 4177276 did **not** fix: the `?nodeId=` threading.  After 4177276 Host still called
`fetchHostInfo` bare while holding `nodeId` state:

```ts
// 4177276 page.tsx:71
function InfoTab({nodeId}: {nodeId:string}) {
  const {data} = useHostQuery(["host-info", nodeId], fetchHostInfo, 30000) // nodeId unused
}
// 4177276 host.ts:50
export function fetchHostInfo(init?: RequestInit) {
  return fetchJSON('/host/info', init)  // no ?nodeId
}
```

The 4177276 Host fix is therefore **necessary but not sufficient** — it guarantees termination
and error taxonomy but not correct node targeting.

## 5.2 The remaining client fix and where it landed

A complete client fix was authored after `ca06f74` and appears only in the stash:

```
stash@{0} = 6bebe02  "On main: tmp stash"
  parents: ca06f74 (mvp-2 HEAD) + 7bc341b (index on mvp-2)
  date: Sun Aug 23 12:03:11 2026 +0530
  git show -p:
    host.ts be53ab7 → f7c4bf8  resolveHostArgs + ?nodeId= on all 5 fetches
    page.tsx 0f74221 → 6985eb5  (init)=>fetchHostInfo(nodeId, init) on 5 tabs
```

Only `stash@{0}` has `20× nodeId` in `host.ts` and `5× "(nodeId, init)"` in `page.tsx`.
All other refs and the workdir lack it:

```
grep -c nodeId forge/web/lib/api/host.ts:
  HEAD ca06f74: 0
  stash@{0}: 20
  workdir (current): 0

grep -c "fetchHostInfo(nodeId" forge/web/app/admin/host/page.tsx:
  HEAD: 0
  stash@{0}: 1 (plus 4 more for other tabs)
  workdir: 0
```

No branch between `4177276` and `ca06f74` carries this fix:

```
git log --oneline 4177276..ca06f74 --name-only | grep -i host
  → (no host.ts / page.tsx between)
git log --all --oneline --follow -- forge/web/lib/api/host.ts
  → bb893a5, 4177276, 9921ccc   (no post-ca06f74 commit)
```

Stash history:

```
stash@{0}: On main: tmp stash   — contains fix
stash@{1}: On main: temp         — no fix
stash@{2}: On mvp-2: temp         — no fix
… only @{0} has it
```

## 5.3 Proving "same class of failure" without confusion

The user-visible symptom is similar — Host loading forever / "node offline" — but the
mechanism differs from the pre-4177276 bug:

| Symptom | Pre-4177276 cause (9921ccc/de811c4) | Current cause (ca06f74 + workdir) |
|---|---|---|
| Infinite loading | No timeout at any layer; `useQuery` never settles, "Loading…" persists | Timeouts exist (10s daemon + 15s client) so **Host itself settles**; however `nodes` query may fail or return empty (see §5.4), leaving the shell at `nodesLoading` spinner.  Additionally, `queryKey`/`fetch` divergence causes stale display that looks like a hang. |
| "Node Offline" | Generic 502 from handler for every daemon error | 502 is now correct for *the fallback node* (first node with BaseURL) when that node happens to be unreachable — not the *selected* node.  Wrong target, correct status for that target. |

The two eras share the **UI outcome** but not the code path.  Calling the current issue
"Host infinite loading" is imprecise: Host tabs themselves would timeout and show
`AdminErrorState`, but the shell may still appear stuck on node selection because the
`nodeId` never reaches the server.

## 5.4 Why infinite loading can still be observed

In modern Head (`ca06f74`):

```ts
// page.tsx:32-49
function useHostQuery<T>(queryKey, fetchFn, refetchInterval=30000) {
  return useQuery({
    queryKey,
    queryFn: ({signal}) => {
      const timeout = AbortSignal.timeout(15000);
      const combined = signal ? AbortSignal.any([signal, timeout]) : timeout;
      return fetchFn({signal: combined});
    },
    retry: 1,
    refetchInterval,
    placeholderData: (prev)=>prev,
    staleTime: 10000,
  })
}
// page.tsx:323
const nodesQuery = useQuery({queryKey:["nodes"], queryFn: fetchNodes});
```

* Host queries: bounded (15s AbortSignal + daemon 10s) + `retry:1`.  `placeholderData`
  keeps previous data visible while refetching, so no spinner flash.  Every Host tab
  reaches either `data`, `error`, or `EmptyState` — never infinite.
* Shell: `if (nodesLoading) return <AdminLoadingState label="Loading nodes…">` and
  `if (noNodes)` and `if (!nodeId)`.  If `fetchNodes` itself fails or the SDK retries
  (GET `/nodes` is idempotent, so SDK retries up to 3× on 502/503/504), the shell can
  appear stuck at "Loading nodes…".  This is not a Host bug per se but is part of the
  reported "infinite loading" when the panel cannot reach `GET /nodes`.
* NodeId divergence: switching node from A→B updates `queryKey` to `["host-info",B]` but
  fetches `GET /host/info` (no param) → backend returns data for fallback node C.
  React Query caches under key B the data for C, so the UI shows stale/wrong data that
  never converges on the selected node — user perceives a hang.

## 5.5 Determining why "Node Offline" is shown (and whether it is correct)

Source of Host offline:

```
forge/api/internal/http/handlers_host.go:11-28 mapDaemonError():
  DeadlineExceeded / Canceled          → 504 (timeout)
  url.Error.Timeout                    → 504
  url.Error (non-timeout)              → 502 "Node offline — Beacon daemon unreachable"
  store GetNodeDaemonCredential fail    → 502 "node credential unavailable"
  store GetNode not found               → 404
  no nodes / no reachable nodes         → 404
forge/web/app/admin/host/page.tsx:60-65 formatError():
  504 → "Connection timeout — Beacon daemon did not respond in time"
  502 → "Node offline — Beacon daemon is unreachable"
  404 → "No node found — register a node first"
  503 → "Service unavailable — daemon proxy is not ready"
```

Heartbeat is **not** consulted by Host (`handlers_host.go` does not read `heartbeatState`);
heartbeat staleness appears in `AdminHealth` / `AdminNodes` overview, not in Host tabs.

**Authorship of the current "Node Offline" is the fallback path:**

```
page.tsx (current) fetchHostInfo() → GET /api/v1/host/info (no nodeId)
  → handlers_host.go: resolveNodeHostTarget(cfg, "")  // nodeID == ""
  → ListNodes() → pick first with BaseURL != "" && credential OK → target
  → daemon hostGet(target.URL, target.Token) → fails if that first node is down
  → mapDaemonError → 502 → page.tsx formatError 502 → "Node offline — Beacon daemon is unreachable"
```

Correctness assessment:

* 502 accurately reflects the node that **was** queried (the fallback), but that is the
  wrong node.  The UI's message is therefore misleading: it blames "Node offline" for the
  *selected* node's Host when the *queried* node is a different one.
* If the first node is healthy, Host **masks** failure of the selected node — Host looks
  healthy even though the intended target is down.
* With stash fix applied, `GET /api/v1/host/info?nodeId=selected` would hit the selected
  node's Beacon and return per-node-correct 502/504/200.

## 5.6 Whether other pages regress

Checked `grep -rn fetchHost / monitoring / health / terminal / files` and `git diff HEAD --stat`:

* `forge/web/app/admin/monitoring/page.tsx` — uses `NodeList`, `ServerCPU/Memory/Disk/NetworkChart`,
  `SystemMetrics`, no `host.ts` dependency; unaffected by Host nodeId bug.
* `forge/web/components/admin/AdminHealth.tsx` — aggregates `/health`, `/nodes`, `/servers`, `/reservations`;
  shows per-node heartbeat but does not call Host routes; unaffected.
* `forge/web/app/admin/terminal/page.tsx` — has its own `NodeSelect` and calls WS tickets per-node
  correctly (ticket is `POST /servers/{id}/ws/ticket`, not Host); separate implementation.
* `forge/web/components/admin/host-files-view.tsx` — on HEAD uses `listFiles(directory, nodeId)`
  correctly with nodeId (checked `host-files-view.tsx:96` `queryFn: () => listFiles(directory, nodeId||undefined)`).
  **However** current `git diff HEAD -- host-files-view.tsx` shows uncommitted dialog rework
  (prompt→state dialogs) that is not Host-connectivity related.
* `main` branch: no Host feature at all (only `handlers_database_hosts_test.go`); no cross-page
  regression concept there.

Verdict: **HOST PAGE ONLY** (and its `host.ts` library).  Monitoring, Health, Terminal, Files are
on separate, intact code paths.

## 5.7 Accidental restoration / overwrite check

Sought evidence of `git restore`, `git checkout`, cherry-pick, rebase, merge conflict:

```
git log --all --grep=restore|checkout|reset|revert|cherry -i → (none relevant)
git reflog: only reset: moving to HEAD, checkout between branches, stash
git log --all --diff-filter=D --oneline -- forge/web/app/admin/host/page.tsx → (empty)
git log --all --oneline --graph: linear de811c4→4177276→2fe255e→ca06f74, no revert commit
git diff main..mvp-2 --stat: Host files are +additions, not reversions
```

No older implementation replaced a newer one on any branch.  The current regression is a
**never-landed** fix, not a reversion:

```
OLD GOOD CODE            → does not exist as committed code (only stash@{0} 6bebe02)
PREVIOUS FIX             → 4177276 (server fix, landed and preserved)
REGRESSION INTRODUCED BY → absence of 6bebe02 on mvp-2 (stash push on 2026-08-23 12:03)
CURRENT CODE (HEAD)      → ca06f74 + workdir (client still without ?nodeId)
```

Proven by `git diff HEAD stash@{0} -- host.ts` (`be53ab7→f7c4bf8`) and
`host/page.tsx` (`0f74221→6985eb5`) documented in 01.

## 5.8 Minimal correct repair

Apply the stashed client diff to `mvp-2` (and workdir):

```
forge/web/lib/api/host.ts:
  add resolveHostArgs(), make each fetchHost* accept (nodeIdOrInit?, init?)
  and emit `?nodeId=` when string arg present.

forge/web/app/admin/host/page.tsx:
  change 5 call sites from  fetchHostX  →  (init)=>fetchHostX(nodeId, init)
  (InfoTab, DiskTab, MemoryTab, NetworkTab, ProcessesTab)
```

No server change needed (4177276 already correct).  No Beacon change.  After the repair,
Host requests become `GET /api/v1/host/info?nodeId=<selected>` and the chain in 03 terminates
with the correct node's Host data or per-node 502/504.

