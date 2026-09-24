# REGRESSION_STATUS

> Machine-readable verdict for the Host / Node / Beacon regression forensic audit.

```
KNOWN_GOOD_COMMIT:       4177276aa7f881a50c2771da874fca2ac6be265b  "bug fixed"
                         (server fix — 10s hostCtx + mapDaemonError 504/502 + useHostQuery)
                         Parent: de811c4  Date: 2026-07-29 20:17:43 +0530

KNOWN_GOOD_CLIENT_COMMIT: stash@{0} (6bebe02a71bc11151693af2ba342c3440578adb6)
                         "On main: tmp stash"  Date: 2026-08-23 12:03:11 +0530
                         host.ts f7c4bf8 (resolveHostArgs + ?nodeId= on 5 fetches)
                         page.tsx 6985eb5 ((init)=>fetchHostX(nodeId,init) on 5 tabs)
                         — the complete, never-landed client fix

CURRENT_COMMIT:          ca06f74a16c5b93e7b7d3205c03c8724b7e303223  "feat: mvp-v2 snapshot"
                         Branch: mvp-2 (HEAD, also tag freebuff-snapshot/eabeeebd…)
                         Workdir: dirty (674 entries in git diff HEAD) but
                         forge/web/lib/api/host.ts == HEAD (no nodeId) and
                         forge/web/app/admin/host/page.tsx == HEAD for nodeId (still bare)

REGRESSION_COMMIT:       (none — no revert)
                         The regression is an ABSENCE, not a commit: the stashed client fix
                         (6bebe02 / be53ab7→f7c4bf8 / 0f74221→6985eb5) was stashed away via
                         git stash push on 2026-08-23 and never committed to mvp-2; ca06f74
                         predates it (2026-08-08) and workdir never received it.
                         4177276 itself is still an ancestor of HEAD (is-ancestor: true).

ROOT_CAUSE:              Frontend Host client omits ?nodeId=.
                         forge/web/lib/api/host.ts:50 (HEAD) defines fetchHostInfo(init?: RequestInit)
                         and fetches '/host/info' with no query; forge/web/app/admin/host/page.tsx:71-223
                         holds nodeId state but passes bare fetchHostInfo to useHostQuery, so
                         GET /api/v1/host/info carries no ?nodeId=. Backend resolveNodeHostTarget("")
                         then falls back to ListNodes()[0] (first node with BaseURL+credential),
                         querying the WRONG node's Beacon and reporting ITS 502/504 as if it were
                         the selected node's status (and caching stale data under wrong queryKey).

AFFECTED_COMPONENTS:     frontend:HostPage, frontend:host-api-library
                         (forge/web/app/admin/host/page.tsx, forge/web/lib/api/host.ts)
                         NOT affected: forge/api host handlers/daemon client,
                         beacon host handlers/heartbeat, main branch (no Host files),
                         Monitoring, Health, Terminal, Files

CONFIDENCE:              HIGH — proven by git diff HEAD stash@{0} for host.ts (be53ab7→f7c4bf8, 0 vs 20× nodeId)
                         and page.tsx (0f74221→6985eb5, 0 vs 5× nodeId closure), empty diffs for
                         forge/api/internal/http/handlers_host.go and daemon/host.go between 4177276
                         and ca06f74, git log --all --follow for Host files, and stash inventory
                         (only stash@{0} contains the fix).
```

## Minimal repair (not yet applied — per charter §11)

```
forge/web/lib/api/host.ts:
  add resolveHostArgs(nodeIdOrInit?, init?) and make each fetchHost* accept
  (nodeIdOrInit?: string|RequestInit, init?: RequestInit) emitting ?nodeId= when string

forge/web/app/admin/host/page.tsx:
  change 5 call sites from  fetchHostX  →  (init) => fetchHostX(nodeId, init)
  (InfoTab, DiskTab, MemoryTab, NetworkTab, ProcessesTab)

No server or Beacon change required (4177276 already correct).
```

