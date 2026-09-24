# 02 — Known-Good vs Current

> Comparison basis: A = known-good server fix (`4177276` = `mvp-production-ready`, commit
> `4177276aa7f881a50c2771da874fca2ac6be265b`) and its stashed client completion
> (`stash@{0}` = `6bebe02`, Sun Aug 23 12:03:11 2026 +0530); B = current HEAD (`ca06f74`,
> branch `mvp-2`) and its dirty workdir.  File paths are repo-root-relative.

## 2.1 Known-good server fix (4177276)

SHA `4177276`, parent `de811c4`, date Wed Jul 29 20:17:43 2026 +0530, author Riyaz Akthar,
message "bug fixed". Files changed (Host-relevant):

| file | before (de811c4) problem | fix (4177276) | after expected behavior |
|---|---|---|---|
| `forge/web/app/admin/host/page.tsx` (`forge/web/app/admin/host/page.tsx:1-256` at `de811c4`) | `InfoTab`/`DiskTab`/… used `useQuery({queryKey:["host-info"], queryFn:fetchHostInfo})` with no timeout, no retry, no error UI, no node selection. Any Beacon hang hung forever at "Loading…". | Introduced `useHostQuery()` (`page.tsx:32-49` at `4177276`) with `AbortSignal.timeout(15000)` merged via `AbortSignal.any`, `retry:1`, `staleTime:10s`, `placeholderData`, `formatError()` mapping 504/502/404/503, `AdminErrorState` with retry, `NodeSelect`/`pickDefaultNode`, per-tab `isLoading && !data` guard. | UI never infinite-loads: 15s client timeout + 1 retry + stale-while-revalidate. Errors surface as human-readable, retriable messages. |
| `forge/web/lib/api/host.ts` (`host.ts:50-66` at `de811c4`) | `fetchHostInfo(): Promise<…>` took no `RequestInit`, so signal/timeout could not be injected. | Changed to `fetchHostInfo(init?: RequestInit)` passing `init` to `fetchJSON` (`host.ts:50`). | `useHostQuery` can now abort the HTTP request. |
| `forge/api/internal/daemon/host.go` (`daemon/host.go:1-35` at `de811c4`) | `hostGet()` used caller `ctx` directly (often background, no deadline), so `httpClient.Do` could block indefinitely. No status distinction. | Added `hostCtx, cancel := context.WithTimeout(ctx, 10*time.Second)` and `c.newRequest(hostCtx, …)` (`daemon/host.go:11-13` at `4177276`). | Beacon host calls always bound to 10s; `context.DeadlineExceeded` propagates as 504. |
| `forge/api/internal/http/handlers_host.go` (`handlers_host.go:1-110` at `de811c4`) | Every handler did `return fiber.NewError(502, err.Error())`, conflating timeout, unreachable, and credential failures as "502 Bad Gateway". No `nodeId` error shaping. | Added `mapDaemonError()` (`handlers_host.go:11-28`) — `DeadlineExceeded/Canceled` → 504, `url.Error.Timeout` → 504, other `url.Error` → 502, else → 502 — and wired all 5 routes through it. | UI can now distinguish timeout (504) vs unreachable (502) vs missing node (404). |
| `beacon/internal/server/handlers_host.go` | Already correct (no change in 4177276). | — | — |

## 2.2 The stashed client completion (stash@{0} = 6bebe02)

This is the fix that *should have* landed on `mvp-2` after `4177276` but only exists in the stash.

* `forge/web/lib/api/host.ts` (`host.ts:1-90` at `stash@{0}`):

  ```ts
  function resolveHostArgs(nodeIdOrInit?: string|RequestInit, init?: RequestInit) {
    if (typeof nodeIdOrInit === "string") return {nodeId: nodeIdOrInit, init};
    if (nodeIdOrInit && typeof nodeIdOrInit === "object") return {nodeId: undefined, init: nodeIdOrInit as RequestInit};
    return {nodeId: undefined, init};
  }
  export function fetchHostInfo(nodeIdOrInit?: string|RequestInit, init?: RequestInit) {
    const {nodeId, init: resolvedInit} = resolveHostArgs(nodeIdOrInit, init);
    const qs = nodeId ? `?nodeId=${encodeURIComponent(nodeId)}` : "";
    return fetchJSON<HostInfo>(`/host/info${qs}`, resolvedInit);
  }
  // same for fetchHostDisk / fetchHostMemory / fetchHostNetwork / fetchHostProcesses
  ```

  BEFORE (HEAD): `fetchHostInfo(init?: RequestInit)` → `fetchJSON('/host/info', init)` — no nodeId.
  FIX: preserves backward compat (bare `init` still works via `resolveHostArgs`) while allowing
  `fetchHostInfo(nodeId, init)` to emit `?nodeId=…`.
  AFTER: every Host request carries its target node.

* `forge/web/app/admin/host/page.tsx` (`page.tsx:70-223` at `stash@{0}`):

  ```ts
  function InfoTab({nodeId}: {nodeId: string}) {
    const {data,…} = useHostQuery(["host-info", nodeId],
      (init) => fetchHostInfo(nodeId, init), 30000)   // ← nodeId threaded
  }
  ```

  BEFORE (HEAD): `useHostQuery(["host-info", nodeId], fetchHostInfo, …)` — queryKey varies
  with `nodeId` but fetcher ignores it (cache key and request key diverge).
  FIX: fetcher closure captures `nodeId` and calls `fetchHostInfo(nodeId, init)`.

* Minor: `SectionHeader` → `AdminPageHeader`, `sub` → `description`.

This stash diff is `be53ab7→f7c4bf8` (`host.ts`) and `0f74221→6985eb5` (`page.tsx`).

## 2.3 Current HEAD (ca06f74, mvp-2) and workdir

* Server side: identical to 4177276 (good) — `handlers_host.go`, `daemon/host.go` unchanged
  (`git diff 4177276..ca06f74 -- forge/api/.../handlers_host.go` → empty;
  `git diff 4177276..ca06f74 -- forge/api/.../daemon/host.go` → empty).
* Client side: **identical to de811c4** for nodeId purposes (broken) — no `?nodeId=` plumbing.
  Workdir `forge/web/lib/api/host.ts` has 0 occurrences of `"nodeId"`; `page.tsx` has 0
  occurrences of `"nodeId, init"` fetcher.  The only workdir diff vs HEAD for Host is
  cosmetic (`text-slate-500→400`, `OfflineBanner` import) — `git diff HEAD -- host.ts` is empty,
  `git diff HEAD -- page.tsx` does not fix nodeId.

## 2.4 Area comparison table

| Area | Known-good (4177276 + stash@{0}) | Current (ca06f74 + workdir) | Changed? | Impact |
|---|---|---|---|---|
| Host `fetchHost*` signatures | `fetchHostInfo(nodeIdOrInit?: string\|RequestInit, init?: RequestInit)` with `?nodeId=` (`host.ts:59-61` stash) | `fetchHostInfo(init?: RequestInit)` (`host.ts:50` HEAD) | **YES — regression** | Requests hit `/host/info` without node selector; backend falls back to first node (wrong node or 404). |
| Host page query wiring | `(init)=>fetchHostInfo(nodeId, init)` per tab (`page.tsx:73` stash) | `fetchHostInfo` bare (`page.tsx:74` HEAD) | **YES** | `queryKey` includes `nodeId` but fetcher ignores it → stale cache, wrong node, or "no reachable nodes". |
| Host page fetch lifecycle | `useHostQuery` with `AbortSignal.timeout(15000)` + `AbortSignal.any` + `retry:1` (`page.tsx:39-43`) | same as known-good (present since 4177276) | No | Timeouts work, but masked by missing nodeId. |
| Host page error handling | `formatError` maps 504→timeout, 502→offline, 404→no node, 503→not ready + `AdminErrorState` retry (`page.tsx:60-69`) | same | No | Good once request carries correct node. |
| `forge/api` host handlers | `mapDaemonError` → 504/502, `resolveNodeHostTarget` with `?nodeId=` (`handlers_host.go:11-28`, `53`) | same | No | Would correctly surface 504 vs 502 if called with right node. |
| `forge/api` daemon host client | `context.WithTimeout(ctx,10s)` (`daemon/host.go:11`) | same | No | Prevents infinite backend hang. |
| Beacon host handlers | `handleHostInfo`/Disk/Memory/Network/Processes (`beacon/.../handlers_host.go:45-120`) | same (present) | No | — |
| Node selection UI | `NodeSelect` + `pickDefaultNode` shared by Host, terminal, files (`node-select.tsx:11-14`) | same | No | Selection itself works. |
| `main` branch | no Host files (`git ls-tree main` has no `host.ts`) | — | N/A | Main cannot be source of Host regression. |

**Every meaningful regression is client-side nodeId plumbing.**

## 2.5 Verdict for §2

* The **server-side Host fix of 4177276 survived** into HEAD — no server regression.
* The **client-side completion** (thread `nodeId` into every Host fetch) was built,
  proved correct (stash nodeId ×20, fetcher ×5), stashed (`6bebe02`), and **never
  landed** — HEAD and workdir still use the pre-fix broken wiring.
* The diff that matters is exactly `be53ab7→f7c4bf8` + `0f74221→6985eb5`, and its
  absence is the sole Host regression.

