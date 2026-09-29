# Forge admin UI — implementation pass (shared brief)

Repo root `/Users/riyaz/forge-plane/forge-test`, web app `forge/web`. READ-ONLY on Go code unless
your scope brief says otherwise. `rtk` is not installed; run commands plainly. On macOS, BSD grep
needs `--include` **before** path operands.

## What already happened (do not redo it, do not undo it)

The tree was broken and is now clean: `npx tsc --noEmit` reports **0 errors** at the start of this
pass. Six files had mangled JSX from a half-finished migration, and 19 pages passed an `info` prop
the header primitive did not declare.

The shared frame in `components/admin/admin-ui.tsx` has been fixed. **It is the contract now:**

- `SectionHeader` and `AdminPageHeader` resolve **title, subtitle and icon from `admin-registry.ts`**
  via `findAdminPage(usePathname())`. If you omit `title`, you get the sidebar label. If you omit
  `sub`/`description`, you get the sidebar description. If you omit `icon`, you get the sidebar glyph
  rendered beside the `<h1>`.
- `info` is accepted and renders a real `PageInfoDisclosure`. Pass an entry from
  `components/admin/admin-page-guides.ts`.
- Exactly one `<h1>` (`t-page`) per page, from the frame.

**Therefore: the single most valuable edit you can make in your scope is to DELETE hand-passed
`title`/`sub`/`description`/`icon` props that merely restate the registry.** Let the frame derive
them. Fewer strings on disk means no future drift. Only keep an explicit `title` on a detail route
(the resource name) or a wizard step.

## Hard rules

1. **Never edit a shared primitive.** Frozen for you:
   `components/admin/admin-ui.tsx`, `admin-registry.ts`, `admin-page.tsx`, `admin-shell.tsx`,
   `admin-page-toolbar.tsx`, `admin-page-guides.ts`, `dashboard-cards.tsx`, `AdminAppsShared.tsx`,
   `node-select.tsx`, `beacon-workspace.tsx`, `telemetry-ui.tsx`, everything in `components/ui/`,
   `lib/api/http.ts`, `lib/api/status.ts`, `lib/admin/telemetry.ts`, and all Go files.
   If a fix truly belongs in one of these, **write it into your report under "Needs central change"**
   with the exact edit — the orchestrator applies those serially. Do not edit them yourself; ten
   agents editing one primitive destroys the tree.
2. Only touch files your scope brief lists. If you need a neighbouring file changed, report it.
3. **Do not run `next build`, `npm test`, `vitest`, or start a dev server.** You may run
   `npx tsc --noEmit 2>&1 | grep -E "yourfile"` at the end to check your own files.
4. **Never invent a number, status, or live-claim.** AGENTS.md: unknown is not zero, not-reported is
   not zero, a stale reading is not healthy. If the data isn't there, render `—` / "Not reported" /
   a dashed unknown, or an explicit error — never `0`, "Healthy", "None found", or "Live".
5. Do not add a new page-frame system. Use `AdminPageLayout` + `SectionHeader`/`AdminPageHeader`.
   `DashHeader` (`dashboard-cards.tsx`) stays only as a *sub*-heading below the frame's `<h1>` on
   pages that already use it — never as a page header, and never inside a `Modal`.

## Per-page checklist

Work through your scope's audit report (`.uiux-audit/reports/scope-N.md`) finding by finding. For
each page:

- [ ] **Frame**: one `AdminPageLayout` root, one `SectionHeader`/`AdminPageHeader`, one `<h1>`.
      Remove any second header, hero card, or duplicated breadcrumb row.
- [ ] **Copy**: delete hand-passed `title`/`sub`/`description`/`icon` that duplicate the registry.
      Fix any real mismatch you keep (detail-route titles must still name the resource).
- [ ] **Disclosure**: if the page has an entry in `admin-page-guides.ts`, pass `info={adminPageGuides.x}`.
- [ ] **States**: loading → error → empty, in that precedence, using `AdminLoadingState` /
      `AdminLoadingRows` / `AdminErrorState` (with a retry) / `EmptyState`. **A failed read must
      never render as an empty list.** Destructure `isError`/`error` from `useQuery` — this is the
      most common S1 in the audit.
- [ ] **Unknown ≠ 0**: replace `?? 0`, `|| 0`, `Number(x) || 0`, `formatBytes(missing) → "0 Bytes"`,
      `value ? value : "—"` on unmeasured data, and counts shown before the query resolves.
- [ ] **Buttons actually work**: every button must call something real. If it targets an endpoint
      that does not exist or a client function with no caller, either wire it correctly against the
      actual Go handler (read the handler to get the contract right) or **remove the control** and
      say why. A button that toasts success while doing nothing is S1.
- [ ] **Destructive actions** go through `useConfirm()` (`components/ui/confirm-dialog`) and state
      what will be affected. No `window.confirm`, no double-click-to-delete.
- [ ] **Capability gating**: controls needing an unavailable runtime/node must render **disabled with
      a one-line reason**, never hidden and never enabled-and-lying.
- [ ] **Node targeting**: no page may silently auto-pick a node. Require an explicit selection; if
      none, show a prompt, not data from `nodes[0]`.
- [ ] **Tokens**: replace `text-slate-*`, `bg-white/[0.0x]`, `text-red-400`/`emerald-400`/`amber-400`,
      `text-[10px]`, raw hex with `text-text*` / `border-line*` / `bg-overlay-*` / `var(--*)` / `t-*`.
      Status colour comes from `Pill tone=` / `resolveTone`, never a class at the call site.
- [ ] **A11y**: icon-only buttons need `aria-label`; every `<select>`/`<input>` needs a label or
      `aria-label`; headings go h1 → h2 → h3 with no jumps; tables live in an `overflow-x-auto`
      container; don't put the only copy of truncated text in a `title` attribute.
- [ ] **Time**: use the shared formatter (`lib/utils` `formatDate`), not a page-local one.

## Working style

- Append to your report file after each page. It must always be a complete record of work done.
- Small, compiling edits. If you change a mutation's payload, re-read the Go handler it hits.
- Prefer deleting code and controls that cannot work over decorating them.
- Keep the page's existing data-fetching intact unless it is dishonest; this is a UI/UX pass, not a
  refactor of the queries.

## Report file format (`.uiux-audit/impl/scope-N.md`)

```markdown
# Impl scope N — <name>
## Done
- <file>: what changed, why (one line each)
## Verified
- `npx tsc --noEmit` filtered to my files: <clean | errors quoted>
## Needs central change (do NOT edit these yourself)
- <shared file>: <exact edit needed and why>
## Removed
- <control/feature deleted because it could not do its job>
## Deferred
- <finding you did not address, and why>
```
