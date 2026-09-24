# 06 — Final Diagnosis

> Answers the 16 questions from §12, plus summary verdict.  Evidence pointers are
> file:line + commit SHA so every claim is auditable.

## 6.1 Verdict in one paragraph

The Host server fix (`4177276`, Jul 29 2026) is **still present** in HEAD — it was never
reverted, reset away, or overwritten.  The current Host regression is **not a revert**.
It is caused by a **client-side nodeId threading fix that was finished after HEAD,
proved correct in `stash@{0}` (`6bebe02`, Aug 23 12:03), then stashed and never landed**,
leaving `HEAD ca06f74` and the workdir with `fetchHostInfo()` that never sends
`?nodeId=…`.  The backend therefore queries the wrong node (fallback first node with
BaseURL), producing the reported "infinite loading / node offline / Beacon unreachable"
symptoms for the wrong target while the selected node is misreported.

## 6.2 Question-by-question answers

**1. Was the previously working Host/Beacon implementation actually reverted?**

No.  Commit `4177276` ("bug fixed", `4177276aa7f881a`) is an ancestor of `HEAD ca06f74`
(`git merge-base --is-ancestor 4177276 ca06f74` → 0).  No revert commit exists
(`git log --all --grep=revert` empty, `git log --diff-filter=D` for Host files empty),
`git diff 4177276..ca06f74 -- forge/api/internal/http/handlers_host.go` is empty,
same for `daemon/host.go`.  The server fix survives verbatim.

**2. If yes — which commit restored/reintroduced the old code?**

Not applicable — no revert occurred.  The closest analogue is the stash operation
`6bebe02` ("On main: tmp stash", `ca06f74` + `7bc341b`) which **removed** the new
client fix from the worktree rather than restoring old code.  It is a never-landed
fix, not a restore.

**3. If no — what new change caused the same behavior?**

No new change introduced it; the **absence** of a new change did.  HEAD `ca06f74`
(`feat: mvp-v2 snapshot`, Aug 8) postdates `4177276` (Jul 29) but predates the
nodeId fix (stashed Aug 23).  The mvp-2 snapshot simply did not include that fix
because it had not been authored yet.  The subsequent `git stash push` that created
`6bebe02` then left HEAD and workdir without it.  Symptoms recur because
`4177276` was necessary but not sufficient (it bounded requests but did not thread nodeId);
the similar UI outcome has a different mechanism (wrong node vs unbounded hang).

**4. Is the Host problem frontend-only?**

Yes.  Frontend-only client wiring:

* `forge/web/lib/api/host.ts:50-67` at HEAD — `fetchHost*` never emits `?nodeId=`.
* `forge/web/app/admin/host/page.tsx:71-223` — 5 tabs pass `fetchHostX` bare, ignoring `nodeId`.

Backend and Beacon are healthy (see 5-9 below).  Repair is `host.ts` + `page.tsx` only.

**5. Is it API/backend wiring?**

No regression.  `forge/api/internal/http/handlers_host.go:11-28` `mapDaemonError`
and `resolveNodeHostTarget` (supports `?nodeId=`), and
`forge/api/internal/daemon/host.go:11` `context.WithTimeout(10s)` are present in HEAD
and identical to 4177276.  `git diff HEAD stash@{0} -- handlers_host.go` is empty,
same for `daemon/host.go`.  Backend correctly serves `GET /host/info?nodeId=…` when
the frontend supplies it.

**6. Is it Beacon communication?**

No.  `beacon/internal/server/handlers_host.go:45-130` (`handleHostInfo`/Disk/Memory/Network/Processes)
exists in HEAD and workdir.  Forge→Beacon dial is `daemon/host.go`'s `httpClient.Do` under
the 10s `hostCtx`.  Heartbeat `heartbeatLoop` (`beacon/cmd/daemon/main.go:696-790`,
30s + 0-5s jitter) is independent.  Beacon would respond correctly if the right node were targeted.

**7. Is it authentication?**

No.  `Store.GetNodeDaemonCredential` (`handlers_host.go:27,43`), `daemon.Client.newRequest`
setting `Authorization: Bearer <token>`, and `beacon/internal/auth/middleware.go` are all
present.  The failing Host fetch gets a 502/504 from a *wrong* but authenticated target,
not a 401.  mTLS (`beacon/cmd/daemon/main.go:388-403`, `forge/api/config/mtls.go`) is
optional defense-in-depth and absent-HMAC-sufficient when no CA is configured — not implicated.

**8. Is it WebSocket?**

No.  Host does not use WebSockets.  The Host chain is `GET /api/v1/host/info` → `GET {BaseURL}/v1/host/info`
(plain HTTPS), documented in 03.  WS is only for `forge/api/internal/http/realtime.go` /
`ws_console` / `ws_ticket` (console/logs/stats).  No WS code touches Host.

**9. Is it node heartbeat/state calculation?**

No.  Host handlers do not read `heartbeatState`; "node offline" for Host is the
`mapDaemonError` 502/504 from `daemon hostGet`, not `AdminHealth`'s
`healthyNodes = nodes.filter(n=>heartbeatState==="healthy")` (`AdminHealth.tsx:230`) or the
`heartbeatLoop` failure path.  Heartbeat staleness would show in Infrastructure/Monitoring
overview, not per-Host tab, and the two are intentionally independent (04 §4.3).

**10. Is it a combination?**

No multi-component failure.  Single root cause: missing `?nodeId=` in Host fetches.
No auth + WS + heartbeat interaction.  The overall "infinite loading" impression comes from
the shell's `nodesQuery` plus Host's queryKey/fetch mismatch (05 §5.4), but the single nodeId
fix resolves both.

**11. Is the node actually offline?**

Not determinable from the Host UI alone in current HEAD — the UI queries the **wrong**
node (fallback `ListNodes()[0]`), so its 502 reflects that fallback's state, not the
selected node's.  A selected node that is actually healthy can be reported as "off-line"
if the fallback node is down, and vice versa.  True offline status lives in
`heartbeatState` (heartbeat path, `AdminHealth`); Host's per-request status after the
fix will correctly reflect the selected node's reachability.

**12. Is Beacon actually offline?**

Same as 11: the Host error reflects whatever fallback node's Beacon returned.
The selected node's Beacon may be online while the UI reports offline (and conversely).
Heartbeat metrics (`CPUPercent`, etc., in `remote.NodeHeartbeat`) remain authoritative
for aggregate Beacon liveness; Host is on-demand.

**13. Or is Forge incorrectly reporting either one as offline?**

Yes — Forge's Host proxy correctly reports *the node it was asked to query*, but because
the frontend omits `?nodeId=`, Forge is asked about the wrong node.  The error taxonomy
(502 unreachable, 504 timeout, 404 no node, 503 not ready) is correct for that node;
what is incorrect is the **node identity**.  This is a frontend-originated misreport,
not a backend misclassification.

**14. What was the previous fix?**

`4177276` ("bug fixed", Jul 29) fixed the **server side**:

* Added 10s `context.WithTimeout` in `forge/api/internal/daemon/host.go:11`.
* Added `mapDaemonError()` in `forge/api/internal/http/handlers_host.go:11-28`
  (DeadlineExceeded/Canceled → 504, url.Timeout → 504, other url.Error → 502) and wired
  all five host routes through it.
* Introduced `useHostQuery()` (`page.tsx:32-49`) with `AbortSignal.timeout(15000)`,
  `retry:1`, `placeholderData`, `formatError()` (502→offline, 504→timeout, 404→no node,
  503→not ready), `AdminErrorState` with retry, `NodeSelect`/`pickDefaultNode`,
  `AdminTabs`, `AdminLoadingState` (`page.tsx:60-69,71-106,319-397`).
* Made `host.ts` accept `init?: RequestInit` so signal can flow.

This fixed infinite hangs and gave the UI distinct error states.  It did **not** yet
thread `nodeId` into the fetch.

**15. Why did that fix stop being effective?**

It never stopped — it is still effective and present in HEAD (see §1.3 `git diff`
empty).  What happened is a **second, independent bug** that looks similar but
overlaps: because `nodeId` is not sent, the now-well-bounded Host request succeeds
(or fails) against the **wrong** node, so the *correctly-typed* 502/504 is shown for
the wrong target.  The 4177276 timeout/retry/error UI still works — it just operates
on the wrong node's response, which masks the selected node's true state.

The *complete* fix (nodeId threading) was finished after `ca06f74` and is provably
working in `stash@{0}` (`6bebe02`, Aug 23 12:03, `host.ts:59-61` `?nodeId=`,
`page.tsx:73` `(init)=>fetchHostInfo(nodeId,init)` ×5), but was stashed away
(`git stash push` removed it from workdir) and never committed — HEAD and workdir
never received it.  That absence is the sole regression.

**16. What is the smallest correct root-cause repair?**

Apply the stashed diff `be53ab7→f7c4bf8` + `0f74221→6985eb5` to `mvp-2` (no server/Beacon changes):

* `forge/web/lib/api/host.ts` — add `resolveHostArgs()` and change each `fetchHost*`
  to `fetchHost*(nodeIdOrInit?: string|RequestInit, init?: RequestInit)` emitting
  `` `/?nodeId=${encodeURIComponent(nodeId)}` `` when a string is supplied
  (backward-compatible: bare `init` still works).

* `forge/web/app/admin/host/page.tsx` — change 5 call sites:

  ```ts
  // InfoTab, DiskTab, MemoryTab, NetworkTab, ProcessesTab
  - useHostQuery(["host-info", nodeId], fetchHostInfo, 30000)
  + useHostQuery(["host-info", nodeId], (init) => fetchHostInfo(nodeId, init), 30000)
  ```

  (same for Disk/Memory/Network/Processes; types already expect `nodeId: string`).

No change to `forge/api/internal/http/handlers_host.go`, `forge/api/internal/daemon/host.go`,
`beacon/internal/server/handlers_host.go`, or `forge/web/components/admin/node-select.tsx`.

Do **not** yet in this investigation step: change UI strings, add retries, increase timeouts,
create a second Host API, or alter WS/heartbeat.  (Repair is explicitly deferred by charter §11.)

## 6.3 Regression committed / root cause summary

```
LAST KNOWN GOOD (server): 4177276  "bug fixed"               2026-07-29
LAST KNOWN GOOD (client): stash@{0} 6bebe02 (host.ts f7c4bf8 / page.tsx 6985eb5) 2026-08-23 12:03
CURRENT (HEAD):            ca06f74  "feat: mvp-v2 snapshot"   2026-08-08  (client missing nodeId)
CURRENT (workdir):         same as ca06f74 for Host (only cosmetic diff)
REGRESSION COMMIT:         (none) — never-landed fix: git stash push of 6bebe02 removed the
                          correct client from workdir; ca06f74 never contained it
ROOT CAUSE:                forge/web/lib/api/host.ts:50 and page.tsx:74-223 omit ?nodeId=;
                          resolveNodeHostTarget("") falls back to ListNodes()[0], querying the
                          wrong node's Beacon and misreporting its status for the selected node
EXACT FAILURE PATH:        AdminHost nodeId state → InfoTab({nodeId}) → useHostQuery(["host-info",nodeId], fetchHostInfo)
                          → host.ts fetchHostInfo(init) → fetchJSON('/host/info', init)  // NO ?nodeId
                          → GET /api/v1/host/info → handlers_host.go resolveNodeHostTarget("") → ListNodes()[0]
                          → daemon hostGet(firstNode.URL, firstNode.Token) → Beacon handleHostInfo → 502/504 for WRONG node
                          → mapDaemonError → 502/504 → page.tsx formatError → "Node offline" for WRONG node,
                          or stale cache divergence (queryKey vs fetch identity)
```

## 6.4 Confidence and affected components

Confidence: **HIGH** (proven by `git diff HEAD stash@{0} -- host.ts/page.tsx`, 0 vs 20 occurrences
of `nodeId`, per-tab call-site analysis, and empty server diffs).

`AFFECTED_COMPONENTS: frontend:HostPage, frontend:host-api-library` only.
Not affected: `forge/api` host handlers/daemon client, Beacon host handlers/heartbeat, `main` branch,
`Monitoring`, `Health`, `Terminal`, `Files` (except workdir Host-files-view rework which is unrelated).

