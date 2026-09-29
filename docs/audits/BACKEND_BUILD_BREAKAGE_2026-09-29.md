# Backend build breakage — `mvp-4` @ `bce7085`

**Date:** 2026-09-29
**Branch:** `mvp-4` (HEAD `bce7085`, working tree clean)
**Severity:** Blocker — neither Go module compiles
**Scope:** `forge/api`, `beacon`. Frontend not implicated.

---

## 1. Summary

Both Go modules on `mvp-4` fail to compile. This is committed breakage, not
uncommitted work-in-progress: the tree is clean and `HEAD` itself is broken.

```
cd forge/api && go build ./...   → exit 1 (5 errors, 3 packages)
cd beacon    && go build ./...   → exit 1 (1 error,  1 package)
```

Consequently `make build`, `make test`, `make api-test` and `make beacon-test`
are all blocked, and CI on this branch cannot be green.

All five API errors and the one Beacon error trace to the **last two commits**,
both of which are bulk working-tree snapshots:

| Commit | Date | Subject |
| --- | --- | --- |
| `7389900` | 2026-09-27 | chore: commit working tree on mvp-4 |
| `bce7085` | 2026-09-27 | chore: commit working tree follow-ups on mvp-4 |

Neither was compiled before being committed. Each carried a legitimate,
well-motivated hardening change that was applied to *some* call sites and not
others.

---

## 2. Reproduction

The Go build cache must be redirected, because the agent sandbox denies writes
to `~/Library/Caches/go-build`:

```bash
export GOCACHE="$TMPDIR/go-build" GOTMPDIR="$TMPDIR"; mkdir -p "$GOCACHE"
cd forge/api && go build ./...
cd ../../beacon && go build ./...
```

> **Note on a misleading first read.** Without the `GOCACHE` override, the build
> emits ~20 `open /Users/muni/Library/Caches/go-build/...` errors that bury the
> real diagnostics. Those are sandbox artifacts, not code defects. The same
> applies to `open /Users/muni/go/pkg/mod/cache/download/...` errors under
> `go vet ./...` — the module cache needs a write lock. Neither masks nor causes
> the defects below; both must be filtered out before reading build output.

---

## 3. Defects

### D1 — `crossnode`: deleted helper, orphaned call sites

**Location:** `forge/api/internal/services/crossnode/ingress_sync.go:170,171`
**Error:** `undefined: itoa`
**Introduced:** `7389900`

```go
ID:   primary.ID   + "-replica-" + itoa(i),
Name: primary.Name + "-replica-" + itoa(i),
```

Commit `7389900` deliberately removed the package-local hand-rolled
`func itoa(n int) string` from `health_filter.go` and switched that file to
`strconv.Itoa`. The replacement is explicitly justified in the code:

> `// every int: strconv.Itoa is used precisely because a hand-rolled formatter that …`
> — `health_filter.go:69`

The two call sites in `ingress_sync.go` — same package, and the helper's only
other consumers — were not converted, so they lost their only definition.

**Fix:** add `"strconv"` to the `ingress_sync.go` import block and replace both
`itoa(i)` with `strconv.Itoa(i)`. This completes the migration `7389900`
intended; do not reintroduce the hand-rolled helper.

---

### D2 — `scheduler`: value→pointer migration missed one backend

**Location:** `forge/api/internal/scheduler/scheduler_nomad.go:310`
**Error:** `cannot use totalMem (variable of type int64) as *int64 value in struct literal`
**Introduced:** `7389900`

Commit `7389900` converted the shared `scheduler.ResourceUsage` from value
fields to pointer fields — an "unknown is not zero" hardening that matches the
project rule in `AGENTS.md`:

```go
// before 7389900                  // after 7389900 (scheduler.go:87)
type ResourceUsage struct {        type ResourceUsage struct {
    CPUPercent float64                 CPUPercent *float64 `json:"cpuPercent,omitempty"`
    MemoryMB   int64                   CPUMHz     *int64   `json:"cpuMHz,omitempty"`
    DiskMB     int64                   MemoryMB   *int64   `json:"memoryMb,omitempty"`
}                                      DiskMB     *int64   `json:"diskMb,omitempty"`
                                   }
```

`scheduler_k3s.go` was migrated correctly and is the reference implementation —
it takes addresses and documents why one field stays `nil`:

```go
// DiskMB stays nil: metrics-server does not report filesystem usage, and
// "not reported" must not be encoded as zero.
return ResourceUsage{CPUPercent: &cpuPercent, MemoryMB: &memMB}, nil
```

`scheduler_nomad.go` was left on the old value form.

**Fix:** mirror the k3s pattern — `return ResourceUsage{MemoryMB: &totalMem, CPUMHz: &totalCPU}, nil`.
See **L1** below: the CPU half of this is a live semantic bug, and this is the
right moment to close it.

---

### D3 — `placement`: refactor applied to caller, not callee

**Location:** `forge/api/internal/placement/replica.go:187`
**Error:** `not enough arguments in call to e.placeSingleReplica`
**Introduced:** `bce7085`

`bce7085` introduced a request-scoped working set — `replicaPlacementState`
(bundling `candidates` + `usedNodeCount`), `prepareReplicaPlacement()` and
`state.apply()` — and rewrote `PlaceReplicas` to pass it:

```go
// replica.go:187 — new 4-arg call
placement, err := e.placeSingleReplica(ctx, state, replica, req)

// replica.go:209 — callee still on the old 5-arg signature
func (e *Engine) placeSingleReplica(ctx context.Context, candidates []Candidate,
    replica ReplicaSpec, req ReplicaPlacementRequest, usedNodeCount map[string]int) (*ReplicaPlacement, error)
```

Before `bce7085` both sides agreed at five arguments
(`e.placeSingleReplica(ctx, workingCandidates, replica, req, usedNodeCount)`).

**Fix:** migrate the callee to `(ctx, state *replicaPlacementState, replica, req)`
and read `state.candidates` / `state.usedNodeCount` internally. `placeSingleReplica`
passes both through to `scoreReplicaCandidate`, which also takes
`usedNodeCount map[string]int` — decide whether that helper takes `state` too, or
keeps the map parameter. Note `ExplainReplicaPlacement` builds its own
`usedNodeCount` map independently (`replica.go:360`); if it is meant to stay in
step with the decision path, it should be moved onto `prepareReplicaPlacement`
as well, or it will drift from the engine it claims to explain.

---

### D4 — `placement`: unused import

**Location:** `forge/api/internal/placement/replica.go:9`
**Error:** `"gamepanel/forge/internal/runtime" imported and not used`
**Introduced:** `7389900`

`7389900` *added* this import already-unused — the import is absent at
`7389900^`, and no version of the file has ever contained a qualified
`runtime.` reference. It is stray debris from the bulk snapshot, not the
residue of a removed use.

**Fix:** drop the import. No restored use is needed.

---

### D5 — `beacon`: `context.AfterFunc` given a channel

**Location:** `beacon/internal/remote/reconnect.go:252`
**Error:** `cannot use rc.stopCh (variable of type chan struct{}) as context.Context value`
**Introduced:** `bce7085` (new code — `probeContext` did not exist at `bce7085^`)

```go
// probeContext bounds a round-trip by the parent context, the probe timeout and
// Stop(), so no probe can outlive shutdown.
func (rc *ReconnectClient) probeContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
    probeCtx, cancel := context.WithTimeout(ctx, timeout)
    watchStop := context.AfterFunc(rc.stopCh, cancel)   // ← stopCh is chan struct{}
    return probeCtx, func() { watchStop(); cancel() }
}
```

`context.AfterFunc` takes a `context.Context`. `ReconnectClient` has no context
for shutdown — it signals stop by closing `stopCh chan struct{}`
(`reconnect.go:61`, closed in `Stop()` at `:308`).

This is brand-new code that has never compiled, so the stated guarantee — "no
probe can outlive shutdown" — is currently unproven in either direction.

**Fix, two options:**

1. **Keep the channel.** Replace `AfterFunc` with an explicit watcher:

   ```go
   done := make(chan struct{})
   go func() {
       select {
       case <-rc.stopCh: cancel()
       case <-done:
       }
   }()
   return probeCtx, func() { close(done); cancel() }
   ```

   Consistent with the rest of the file, which already selects on `stopCh`
   (`:160`, `waitOrCancelled` at `:294`).

2. **Add a stop context.** Give `ReconnectClient` a `stopCtx`/`stopCancel` pair
   alongside `stopCh`, cancel it in `Stop()`, and pass `rc.stopCtx` to
   `AfterFunc`. Cleaner long-term, but touches the lifecycle of a struct with
   `startOnce`/`stopOnce`/`stopped`/`started` plumbing — more care needed.

Option 1 is the smaller, lower-risk change and matches existing idiom.

---

## 4. Latent issues masked by the compile failure

These are not compile errors. They are in the same code paths and will ship
silently the moment the build is fixed, so they should be resolved together.

### L1 — Nomad reads CPU, then discards it

**Location:** `forge/api/internal/scheduler/scheduler_nomad.go:302-311`

```go
var totalMem, totalCPU int64
for _, tg := range job.TaskGroups {
    for _, t := range tg.Tasks {
        totalMem += t.Resources.MemoryMB
        totalCPU += t.Resources.CPU      // accumulated…
    }
}
return ResourceUsage{
    MemoryMB: totalMem,                  // …never returned
}, nil
```

`totalCPU` is summed and dropped. This predates the pointer migration (it dates
to `7835806`, 2026-07-21, when the file was created), but the migration changes
its meaning and makes it worse: under pointer semantics a `nil` `CPUMHz` means
*"not reported"*, so Nomad now actively asserts it has no CPU reading for a
value it successfully parsed. That is precisely the failure mode `AGENTS.md`
prohibits — *"not-reported is not zero"*, and a reading that was taken must not
be reported as absent.

Fixing D2 by setting `MemoryMB: &totalMem` alone would compile while leaving
this in place. Set both.

### L2 — `ReplicaPlacement.Reserved` has no producer

**Location:** `forge/api/internal/placement/replica.go:59-65`

The field was added in `bce7085` with an unusually strong contract:

> `// Reserved is the capacity this replica occupies on NodeID from the moment`
> `// the engine chose it. The engine holds nothing across requests, so the`
> `// caller must make this amount durable … before another placement reads the`
> `// same node's free capacity; reporting a placement nobody reserved would be`
> `// reporting work that was not performed.`

Nothing assigns it. Both `return &ReplicaPlacement{...}` sites in
`placeSingleReplica` (`:229`, `:268`) omit `Reserved`, no other file in
`internal/placement`, `internal/http` or `internal/services` writes it, and no
test references it.

Every placement therefore reports `Reserved: {CPU:0, MemoryMB:0, DiskMB:0}`. A
caller that follows the documented contract and reserves the reported amount
reserves **nothing**, then the next placement reads stale free capacity and
over-commits the node — the exact outcome the comment warns about. The
serialized field (`json:"reserved"`) exposes the same zeros over the API.

The spec for this is already in the file (`ReplicaSpec` carries
`CPU`/`MemoryMB`/`DiskMB`, and `state.apply()` at `:157` already debits exactly
those three values), so populating it during D3's rewrite is a small change.

### L3 — Nit: `gofmt` drift

`forge/api/internal/placement/replica.go:16` — `RequiredNode string` is not
column-aligned with its neighbours in the struct. `make format` will fix it;
flagged only so it is not mistaken for a manual edit later.

---

## 5. Root cause

Two consecutive bulk snapshot commits — `7389900` and `bce7085`, both titled
"chore: commit working tree …" — were committed without a compile.

The changes they carry are not sloppy in intent. All four are defensible
hardenings, three of them directly serving rules in `AGENTS.md`:

- delete a hand-rolled int formatter in favour of `strconv.Itoa` (D1)
- make "unknown" representable rather than encoding it as zero (D2)
- give replica placement a single request-scoped working set so an explanation
  cannot describe a winner the engine would not pick (D3)
- bound a Beacon probe so it cannot outlive shutdown (D5)

The failure is uniformly one of **incomplete application**: each migration
updated part of its blast radius. D1 and D2 converted one file and missed a
sibling in the same package; D3 updated a caller and not its callee; D5 wrote
new code against an API whose signature was never checked. A single
`go build ./...` in either module would have caught all five.

**Process gap:** `go build ./...` on both modules is not currently enforced
before commit on this branch. Given `go.work` spans both modules, one
`go build ./...` per module is sufficient and fast.

Note also that the sandbox's `GOCACHE` denial (§2) makes a naive `go build`
output look like ~20 unrelated filesystem errors. Anyone who ran a build here
without the `GOCACHE` override would plausibly have dismissed it as a local
environment problem rather than a code failure. Worth adding the override to
the dev docs.

---

## 6. Recommended remediation order

Smallest and most certain first; each step is independently verifiable.

| # | Defect | Change | Risk |
| --- | --- | --- | --- |
| 1 | D1 | `strconv.Itoa` + import in `ingress_sync.go` | trivial |
| 2 | D2 + L1 | `&totalMem` **and** `&totalCPU` in nomad `GetResources` | trivial |
| 3 | D5 | channel watcher in `probeContext` (option 1) | low |
| 4 | D4 | drop unused import, or restore its use with step 5 | trivial |
| 5 | D3 + L2 | migrate `placeSingleReplica` to `*replicaPlacementState`; populate `Reserved`; decide on `scoreReplicaCandidate` and `ExplainReplicaPlacement` | moderate — only judgement call in the set |

Then:

```bash
export GOCACHE="$TMPDIR/go-build" GOTMPDIR="$TMPDIR"
cd forge/api && go build ./... && cd ../../beacon && go build ./...
make format
make test        # first full-suite signal since 7389900
```

`make test` has not been able to run since `7389900`, so expect its first green
run to surface further drift — in particular, `AGENTS.md` notes that
`go vet ./...` in `forge/api` already reports failures in test files referencing
symbols production code no longer exports. Triage that separately from this
breakage; the two are not the same problem.

---

## 7. Verification performed

- `go build ./...` on both modules, with `GOCACHE` redirected — exit 1 each.
- `go vet ./...` on both modules — same four defect classes, no additional
  production-code errors beyond D1–D5.
- `git log -L` line-blame on each failing line to attribute the introducing commit.
- `git show <commit>^:<path>` on each file to confirm the pre-change state
  compiled, establishing all five as regressions rather than pre-existing breakage.
- `git show 7389900 -- health_filter.go` to confirm the `itoa` helper deletion
  and its intent.
- Grepped `internal/placement`, `internal/http`, `internal/services` and the
  package tests for `Reserved` writers — none found (L2).
- Base branch `mvp-3` could **not** be build-verified: a temporary worktree
  requires a writable Go module cache (`~/go/pkg/mod`), which the sandbox denies.
  Attribution above rests on line-blame and pre-commit file state instead, which
  is decisive for all five defects. A `mvp-3` build outside the sandbox would
  confirm the branch-level claim directly.

---

*Analysis only — no code changed. The working tree was clean before this report
and remains clean; the temporary worktree used for the `mvp-3` attempt was
removed.*
