# 01 — Repository History Forensics

> Generated: 2026-08-23 (forensic audit, DO NOT MODIFY CODE)
> Investigator: Muse Spark (opencode)

## 1.1 Current HEAD

```
HEAD        = ca06f74  (refs/heads/mvp-2, tag freebuff-snapshot/eabeeebd…)
Message     : feat: mvp-v2 snapshot — phase registrars, billing, catalog, pipeline, env manifest, node autoscale, drain, preview env, git phase1, new migrations 165-201
Date        : Sat Aug  8 23:38:00 2026 +0530
Author      : Riyaz Akthar <riyazakthar46@gmail.com>
Workdir     : dirty (674 entries in `git diff --stat HEAD`, see §1.7)
Branch      : mvp-2  (checked out, see .git/HEAD -> refs/heads/mvp-2)
```

Other notable refs (all share the same root `b023414c`):

| ref | SHA | message |
|---|---|---|
| `main` | `8bd9ba7` | Build production-ready Forge control plane |
| `origin/main` | `9dda2a8` | Add durable operations and Beacon command recovery |
| `mvp-production-ready` | `4177276` | **bug fixed** (the Host/Beacon fix, see §2) |
| `origin/mvp-production-ready` | `de811c4` | feat: ship production-ready Forge MVP |
| `modularity-integration` | `9921ccc` | Expand control plane platform capabilities |
| `stash@{0}` | `6bebe02` | `On main: tmp stash`  (merge: `ca06f74` + `7bc341b`; see §1.5) |
| `stash@{0}^2` (`7bc341b`) | index on mvp-2 | staged subset of the same stash |

```
git log --all --oneline --graph (top 15):
*   6bebe02 On main: tmp stash
|\
| * 7bc341b index on mvp-2: ca06f74 …
|/
* ca06f74 (HEAD -> mvp-2) feat: mvp-v2 snapshot …
* 2fe255e Update game panel implementation
* 4177276 (mvp-production-ready) bug fixed          <-- HOST FIX
* de811c4 (origin/mvp-production-ready) feat: ship production-ready Forge MVP
* 9921ccc Expand control plane platform capabilities
* … (shared root)
* 8bd9ba7 (main) Build production-ready Forge control plane
* 9dda2a8 (origin/main) Add durable operations and Beacon command recovery
```

`git status` (abridged, `mvp-2`):

```
 M .env.example
 M beacon/cmd/daemon/main.go
 M beacon/internal/remote/client.go
 M beacon/internal/server/server.go
 M forge/api/internal/daemon/client.go
 M forge/api/internal/http/handlers_host.go   (no diff — file == HEAD, i.e. still broken)
 M forge/web/app/admin/host/page.tsx          (cosmetic only, no nodeId fix)
 … 674 entries total in `git diff HEAD --stat`
?? many untracked (infra/ship, forge/web/app/admin/containers, docs/architecture …)
```

Branch tracking:

```
mvp-2                  ca06f74 [freebuff/analyse-entire-project…]
main                   8bd9ba7 [origin/main: ahead 4, behind 4]
origin/main            9dda2a8
modularity-integration 9921ccc
```

## 1.2 Commit lineage relevant to Host/Beacon

```
b023414c  Initial (Little bit of everything v2)
  → … → 9921ccc  Expand control plane platform capabilities
        (first appearance of: forge/web/lib/api/host.ts,
         forge/web/app/admin/host/page.tsx,
         beacon/internal/server/handlers_host.go,
         forge/api/internal/daemon/host.go,
         forge/api/internal/http/handlers_host.go)
        Host page at 9921ccc: simple useQuery without nodeId, no error handling,
        fetchHostX() with no query param, daemon hostGet() with no timeout,
        handlers with generic 502 only.

  → de811c4  ship production-ready Forge MVP  (no Host change)
  → 4177276  bug fixed                        (THE FIX — detailed in 02)
  → 2fe255e  Update game panel implementation (unrelated mvp-2 wiring)
  → ca06f74  feat: mvp-v2 snapshot             (HEAD — Host/Beacon files UNCHANGED from 4177276)
  → stash@{0} (6bebe02 / 7bc341b) — uncommitted Host nodeId plumbing that was stashed
  → workdir (current) — same as ca06f74 for Host (fix never applied to workdir)
```

`git log --all --oneline -- forge/web/lib/api/host.ts` :

```
bb893a5 feat: mvp-v2 — Forge Plane full snapshot  (alias for ca06f74 on mvp-v2)
4177276 bug fixed
9921ccc Expand control plane platform capabilities  (file creation)
```

`git log --all --oneline -- forge/api/internal/http/handlers_host.go` :

```
bb893a5 …
4177276 bug fixed
9921ccc …  (creation)
```

`git log --all --oneline -- forge/api/internal/daemon/host.go` :

```
bb893a5 …
9921ccc …  (creation)
```
(Note: `host.go` timeout fix appears only in the diff 9921ccc→4177276; it is present in `ca06f74` HEAD.)

Host files **do not exist** on `main` (`8bd9ba7`, `9dda2a8`) or `origin/main`:

```
git ls-tree -r main --name-only | grep -i host  →  only migration 015_db_hosts_constraints.sql
git ls-tree -r mvp-2 --name-only | grep -i host →  beacon/internal/server/handlers_host.go
                                                  forge/api/internal/daemon/host.go
                                                  forge/api/internal/http/handlers_host.go
                                                  forge/web/app/admin/host/page.tsx
                                                  forge/web/lib/api/host.ts  (+ host-files variants)
```

So `main` is not a regression source for Host — it simply never had the feature.

## 1.3 The fixing commit (4177276) in detail

```
4177276 bug fixed
Author: Riyaz Akthar
Date:   Wed Jul 29 20:17:43 2026 +0530
Parent: de811c4

git show --stat 4177276:
 forge/web/app/admin/host/page.tsx  |  140 +++++++++++++++++++++++++++++
 forge/web/lib/api/host.ts          |   10 + (add RequestInit passthrough)
 forge/api/internal/daemon/host.go  |    4 + (10s context timeout)
 forge/api/internal/http/handlers_host.go | 154  (add mapDaemonError, 504/502 distinction)
 … (≈120 other non-Host files)
```

Key Host diffs  (de811c4 → 4177276):

* `forge/web/app/admin/host/page.tsx` — Introduced `useHostQuery()` with `AbortSignal.timeout(15000)`,
  `retry:1`, `staleTime`, per-tab `isLoading && !data` / `error && !data` branches,
  `AdminErrorState` with `formatError()` mapping 504→timeout, 502→offline, 404→no nodes,
  `NodeSelect` + `pickDefaultNode`, `AdminTabs`, and `AdminPageLayout`.  **Still passes
  `fetchHostInfo` bare** (no nodeId) — the page holds `nodeId` state but never threads it
  into the fetcher.
* `forge/web/lib/api/host.ts` — Changed `fetchHostInfo(): Promise<…>` → `fetchHostInfo(init?: RequestInit)`
  so `useHostQuery` can supply `AbortSignal`. No `nodeId` query string.
* `forge/api/internal/daemon/host.go` — Wrapped request context in `context.WithTimeout(ctx, 10*time.Second)`
  so Beacon calls cannot hang forever.
* `forge/api/internal/http/handlers_host.go` — Added `mapDaemonError()`:
  `context.DeadlineExceeded/Canceled` → 504, `url.Error{Timeout}` → 504,
  other `url.Error` → 502, and wired all 5 host handlers through it (previously bare `502`).

This is the **known-good** server-side fix. The **client-side nodeId plumbing is still incomplete**
after it (see §1.5 — the complete client fix was developed after 4177276 and stashed).

## 1.4 Ancestor / revert / cherry-pick / rebase checks

* Is fixing commit ancestor of HEAD?

  ```
  git merge-base --is-ancestor 4177276 ca06f74  → exit 0 (YES)
  git merge-base --is-ancestor 8bd9ba7 ca06f74  → exit 0
  git merge-base --is-ancestor 9dda2a8 ca06f74  → exit 1 (origin/main diverged)
  ```

  So every file introduced/fixed in `4177276` is present in HEAD `ca06f74`
  (confirmed by `git diff 4177276..ca06f74 --stat` — **no Host files listed**;
  `git diff 4177276:forge/web/app/admin/host/page.tsx HEAD:forge/web/app/admin/host/page.tsx`
  is empty aside from cosmetic `text-slate-500→400` in the **workdir**, not in HEAD).

* Revert?

  ```
  git log --all --oneline --grep=revert -i  → (empty)
  git log --all --diff-filter=D --oneline -- forge/web/app/admin/host/page.tsx → (empty)
  git log --all --diff-filter=D --oneline -- forge/api/internal/http/handlers_host.go → (empty)
  ```

  No revert commit exists. No Host file was deleted.

* Cherry-pick / restore / checkout:

  Search commit messages for restore/checkout/reset/revert: none that touch Host.
  `git reflog -n 80` shows only `reset: moving to HEAD`, `checkout` between
  `mvp-v2-clean` / `mvp-2` / `mvp-production-ready`, and stash operations — no
  `cherry-pick`, no `rebase` completing on `mvp-2` (the one rebase that did
  complete landed on `modularity-integration` at `9921ccc`, before Host existed).

* Merge-conflict overwrite:

  No merge commit on the `mvp-2` line after `4177276` (linear `de811c4→4177276→2fe255e→ca06f74`).
  `bb893a5` is a parallel `mvp-v2` branch pointing at the same tree as `ca06f74`;
  it is not merged into `mvp-2`.

**Conclusion of §1.4:** There is **no git revert, reset, or merge that overwrote the
4177276 Host fix**. The fix is still an ancestor and its files are unchanged in HEAD.

## 1.5 The stashed nodeId fix — why a new Host regression exists anyway

The **complete** Host client fix (thread `nodeId` into every fetch) was developed
*after* HEAD and then **stashed**, leaving HEAD and the workdir broken:

```
stash@{0} = 6bebe02  (merge ca06f74 + 7bc341b)  "On main: tmp stash"
         parents: ca06f74 (mvp-2 HEAD) + 7bc341b (index on mvp-2)
         date:    Sun Aug 23 12:03:11 2026 +0530
         `git stash show -p` diffs `forge/web/lib/api/host.ts` be53ab7→f7c4bf8
           and `forge/web/app/admin/host/page.tsx` 0f74221→6985eb5

git show stash@{0}:forge/web/lib/api/host.ts  (STASH — FIXED):

  function resolveHostArgs(nodeIdOrInit?: string|RequestInit, init?: RequestInit)
  export function fetchHostInfo(nodeIdOrInit?: string|RequestInit, init?: RequestInit) {
    const {nodeId, init: resolvedInit} = resolveHostArgs(nodeIdOrInit, init);
    const qs = nodeId ? `?nodeId=${encodeURIComponent(nodeId)}` : "";
    return fetchJSON<HostInfo>(`/host/info${qs}`, resolvedInit);
  }
  // same for fetchHostDisk / fetchHostMemory / fetchHostNetwork / fetchHostProcesses (20× "nodeId")

git show stash@{0}:forge/web/app/admin/host/page.tsx  (STASH — FIXED):

  function InfoTab({nodeId}) {
    const {data,…} = useHostQuery(["host-info", nodeId],
      (init) => fetchHostInfo(nodeId, init), 30000)  // ← nodeId threaded
  }
  // same for DiskTab / MemoryTab / NetworkTab / ProcessesTab (5×)
  // plus SectionHeader → AdminPageHeader rename

git show HEAD:forge/web/lib/api/host.ts  (HEAD ca06f74 — BROKEN):

  export function fetchHostInfo(init?: RequestInit): Promise<HostInfo> {
    return fetchJSON<HostInfo>('/host/info', init);   // ← no nodeId
  }
  // same for all 5 functions (0× "nodeId")

HEAD host page (ca06f74 — BROKEN):

  function InfoTab({nodeId}) {
    const {data,…} = useHostQuery(["host-info", nodeId],
      fetchHostInfo, 30000)   // ← bare function, nodeId NEVER sent
  }

Working directory (current): identical to HEAD for these two files
  (git diff HEAD -- forge/web/lib/api/host.ts → empty;
   git diff HEAD -- forge/web/app/admin/host/page.tsx → only cosmetic color tweaks)

Stash inventory:

  stash@{0}: On main: tmp stash          — contains the fix (6bebe02)
  stash@{1}: On main: temp                — no host nodeId
  stash@{2}: On mvp-2: temp                — no host nodeId
  stash@{3}: current-wip (recovered)      — no host nodeId
  stash@{4..8}: older WIPs                — no host nodeId

  Only stash@{0} (and its index parent 7bc341b) holds the complete nodeId plumbing.
```

So the forensics prove:

* The **server-side** Host fix (`4177276`) is present in HEAD — not reverted.
* The **client-side nodeId** fix was finished after HEAD, proved correct in the stash,
  but was **stashed away and never committed to any branch nor applied to the workdir**.
* The current HEAD/workdir therefore exhibits the **same class** of failure (infinite load /
  wrong node) as before `4177276`, but via a different mechanism: the request is made
  without `?nodeId=…` so the backend falls back to "first node", which is either wrong
  or unreachable.

This is **not** a `git reset`/`revert`/`cherry-pick` overwrite. It is an **uncommitted fix
that never landed** (a "stash regression") — the working directory and HEAD diverged
from the developer's fixed state because `git stash push` removed it from the worktree.

## 1.6 Search for relevant commit messages

```
git log --all --oneline --grep="host|beacon|heartbeat|offline|unreachable" -i
  → 7bc341b index on mvp-2
  → bb893a5 feat: mvp-v2 — Forge Plane full snapshot
  → ca06f74 feat: mvp-v2 snapshot
  → c96dd9a Add Beacon-backed image app deployments
  → 9dda2a8 Add durable operations and Beacon command recovery

git log --all --oneline --grep="Host|Beacon|Node|daemon" -i
  → same set + no dedicated "fix Host" message beyond 4177276 "bug fixed"
```

File-level searches (`git log --all --oneline -- forge/web`, `-- forge/api`, `-- beacon`,
`-- packages`) confirm: no Host/Beacon commit between `4177276` and `ca06f74` except
`ca06f74` itself (which does not touch Host).

## 1.7 Working directory vs HEAD vs stash@{0}

| Area | HEAD (`ca06f74`, mvp-2) | stash@{0} (`6bebe02`, fixed) | Current workdir |
|---|---|---|---|
| `forge/web/lib/api/host.ts` | broken (no nodeId) | **fixed** (`?nodeId=`) | broken (= HEAD) |
| `forge/web/app/admin/host/page.tsx` | broken (bare `fetchHostInfo`) | **fixed** (`(init)=>fetchHostInfo(nodeId,init)`) | broken (= HEAD, + cosmetic) |
| `forge/api/internal/daemon/host.go` | fixed (10s timeout) | fixed (same) | fixed (= HEAD) |
| `forge/api/internal/http/handlers_host.go` | fixed (`mapDaemonError`) | fixed (same) | fixed (= HEAD) |
| `beacon/internal/server/handlers_host.go` | present | present | present (modified) |
| `main` branch | **no Host files** | — | — (workdir on mvp-2) |

Full `git diff HEAD --stat` in workdir: 674 entries (beacon, forge/api, forge/web, infra…).
Only `forge/web/app/admin/host/page.tsx` and `forge/web/components/admin/host-files-view.tsx`
are Host-adjacent and both lack the nodeId fix.

## 1.8 No evidence of `git restore` / `git checkout` / `git reset` overwriting Host

* `git reflog` on `mvp-2`: linear `reset: moving to HEAD` (no file checkout), `checkout`
  between `mvp-2` ↔ `mvp-production-ready` ↔ `temp-apply` / `modularity-integration`.
* The only `git restore`-like effect is `git stash push` that moved the fixed Host
  client into `stash@{0}` and cleaned the workdir — not a restore of old code, but a
  **removal of new code**.
* Duplicate-implementation check: no second Host API exists; `forge/web/lib/api/host.ts`
  is canonical, `forge/web/lib/api/host-files.ts` is separate.

## 1.9 Summary for §1

* Fixing commit **4177276** is ancestor of HEAD — not reverted, not reset away.
* No branch switch, merge, cherry-pick, or conflict resolution reintroduced old Host code.
* The **current Host regression** is caused by an **unlanded fix**: the nodeId plumbing
  that would have completed `4177276` was stashed (`stash@{0}` = `6bebe02`) and never
  committed, so HEAD (`ca06f74`) and the workdir remain in the pre-fix client state.
* `main` never had Host at all, so it cannot be the source of a "restored older
  implementation" for Host.
