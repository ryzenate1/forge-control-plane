# Impl scope 6 — Infrastructure B (host access + alternative runtimes)

Read `.uiux-audit/briefs/IMPL-shared.md` first, then `.uiux-audit/reports/scope-6.md`, then
implement. Report to `.uiux-audit/impl/scope-6.md`.

You own: `app/admin/host/**`, `app/admin/files/**`, `app/admin/terminal/**`, `app/admin/sftp/**`,
`app/admin/docker-cleanup/**`, `app/admin/kubernetes/**`, `app/admin/incus/**`, `app/admin/nomad/**`,
`app/admin/netbird/**`, `components/admin/host-files-view.tsx`, `AdminSftp.tsx`,
`docker-cleanup-manager.tsx`, `lib/api/docker-cleanup.ts` (or the equivalent client).

**Frozen (report only):** `node-select.tsx`, `beacon-workspace.tsx`, `AdminKubernetes.tsx`.

1. **Ambiguous node target resolved silently — while the Go handlers now reject it.**
   `app/admin/terminal/page.tsx:147` opens a root shell socket with **no node at all**, and
   `docker-cleanup-manager.tsx:123` picks `nodes[0]`, while `handlers_host.go:44-55` and
   `handlers_kubernetes.go:50-56` `400` on exactly that. Destructive prunes and privileged shells must
   not land on a machine nobody named. Require an explicit selection in your own pages: no selection →
   a prompt, never data. `node-select.tsx`'s auto-pick is frozen — report the exact change you need
   there and the orchestrator applies it once.
2. **`AdminPageHeader` now accepts `info`** (it did not when you audited). Re-check the eight pages in
   your slice that pass it — the capability warnings, including "treat every keystroke as privileged",
   should now render. If a page passes `info` with content that is no longer true, fix the content.
3. **Incus renders its full five-tab console with zero Incus nodes** (`incus/page.tsx:115-119`) and
   deletes instances unconfirmed. Same for Kubernetes/Nomad: an uninstalled runtime is **disabled with
   a reason**, per the project's capability-gating convention — not a populated-looking console.
4. **Beacon's honesty fields are dropped upstream.** `reclaimedBytesKnown`, `accountingNotes`,
   `sizeIncompleteCount` exist in `beacon/internal/server/docker_cleanup.go:57-94` but have no
   counterpart in the Go API types (`dockerleanup/types.go:46-52`), so the UI toasts "0 B reclaimed"
   for an unmeasured prune. Go is frozen for you — surface what you can from the payload you receive
   and file the missing field under "Needs central change".
5. **Unmeasured rendered as zeros**: SFTP `Number(v) || 0`, NetBird `?? 0` counts, retention floor
   cleared to 0.
6. Files/Terminal/SFTP/Host Inspector must all share one explicit-node-selection idiom.
