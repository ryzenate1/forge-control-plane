# Impl scope 4 (continued) — Deploy, remainder

Previous agent hit the turn limit. Read `.uiux-audit/impl/scope-4.md` first — it completed and
**currently compiles**: all four `deployments` pages (list, history, `[id]`, `[id]/revisions`),
`deployments/new`, `compose/page.tsx`, `compose/[id]/page.tsx`. Verify, don't redo.

Your remaining work, in priority order:

1. **`app/admin/compose/new/page.tsx`** and any remaining compose page — the previous agent was cut off
   while starting Compose; check for half-applied edits there first.
2. **`app/admin/pipelines/page.tsx`** — it stacks `DashHeader` inside a `Modal`, repeating
   title/status/description 2-3×. A modal needs a plain title. Also check run history vs pipeline
   definition are distinguishable, and step failures are not rendered as skips.
3. **`app/admin/git/page.tsx`** — I repaired its 22 missing identifiers so it compiles; it still needs
   its audit findings: real loading/error states per tab, credential masking on provider tokens and
   deploy keys, no failed read rendering as "no sources".
4. **`app/admin/preview-environments/**`, `app/admin/preview-deployments/**` (+`[id]`),
   `app/admin/source-deployments/**` (+`[id]`)** — four near-identical record lists. Use ONE shared
   status vocabulary (`lib/api/status.ts`) and ONE time formatter (`lib/utils` `formatDate`).
   Audit S1s here: preview TTL showing "uncapped" while loading, `0 || "—"`, unknown build status
   defaulting to `failed`.
5. **`app/admin/compose-templates/**`** — parameterised template authoring: check a template with
   missing required parameters cannot be deployed silently.
6. **`components/admin/zerodowntime-manager.tsx`** — "health gates" must show pass/fail/pending from
   real evidence, not static labels.
7. **Alias stub you own:** `app/admin/deploy/**` (→ `/admin/deployments`).
   `test/route-integrity.test.ts` asserts every alias source is served by a real page.

Also from the audit: this slice had **9 private status maps**; `sourceStatusTone`,
`serverDeploymentStatusTone` and `buildStatusTone` had zero consumers. If they live in a file you own,
delete them; if they live in a shared file, report it.

Follow `.uiux-audit/briefs/IMPL-shared.md`. Append to `.uiux-audit/impl/scope-4.md` under
`## Continued`. Frozen: all shared primitives, `AdminAppsShared.tsx`, `lib/api/status.ts`, all Go.
Do not touch `components/database/**`. `rtk` is not installed.
