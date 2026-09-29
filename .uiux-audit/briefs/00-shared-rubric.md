# Forge admin UI/UX audit — shared rubric

You are auditing a **slice** of the Forge admin dashboard for UI/UX inconsistency. You are
**READ-ONLY on source code**. The only files you may write are your own brief/report files
under `.uiux-audit/`. Do **not** edit, create, rename or delete anything under `forge/web`,
`beacon`, `forge/api` or anywhere else in the product. A later pass implements fixes.

Repo root: `/Users/riyaz/forge-plane/forge-test`
Web app:   `/Users/riyaz/forge-plane/forge-test/forge/web`

Note: `rtk` is NOT installed on this machine. Run shell commands plainly.

---

## 1. Why this audit exists

The sidebar has 88 visible routes. Four of them — Overview, Monitoring, Activity, Game Servers —
read and behave like one product. The other ~84 do not: their page title, icon, subtitle and the
order of the sections underneath disagree with each other and with the sidebar.

The cause is structural, and you must understand it before filing findings:

**Three page-frame systems coexist, and none of them is the documented canonical one.**

| System | File | Header component | Live users |
| --- | --- | --- | --- |
| Canonical (documented, **dead — zero consumers**) | `components/admin/admin-page.tsx` | `AdminPage` → `ForgePage` + `ForgePageHeader` (`components/ui/forge/layout.tsx`) | **0** |
| Legacy "admin-ui" | `components/admin/admin-ui.tsx` (677 lines) | `AdminPageLayout` + `AdminPageHeader` → `SectionHeader` | ~30 files |
| Legacy "dashboard-cards" | `components/admin/dashboard-cards.tsx` | `DashHeader` + `InfoCard`/`KpiGrid`/`QuickActionsCard`/`TrendChart` | 8 files |
| Hand-rolled | various | inline `<div>`/`<h2>`/`<p>`, no shared header at all | ~55 `page.tsx` files |

`admin-page.tsx` documents the intended rule: *page language is a property of the route*, derived
from `components/admin/admin-registry.ts`, so the sidebar row, the breadcrumb tail and the `<h1>`
cannot disagree. Nothing consumes it. **Treat the registry as the source of truth for label /
description / icon / group / breadcrumb**, and treat every page that restates those by hand as a
drift finding. Do not propose inventing a fourth system.

**The reference pattern** (what "good" looks like) is `AdminOverview`, `AdminMonitoring`,
`AdminActivityLog`, `AdminServers`, `AdminNodes`:
`AdminPageLayout` → `DashHeader` or `SectionHeader` → `AdminPageToolbar` → KPI row →
filter/toolbar → primary table/list → secondary sections, with `FreshnessBadge` bound to a real
`dataUpdatedAt`, and `PageInfoDisclosure` on the title. Read at least two of them before auditing.

---

## 2. Invariants to check on EVERY page in your scope

Cite `file:line` for every finding. Severity: **S1** misleading/deceptive or broken · **S2** clearly
inconsistent with the reference pattern · **S3** polish.

**A. Title** — Does the rendered heading text equal the registry `label` for that href, exactly?
Report every mismatch ("App Store" vs "Application Store", plural/singular, trailing "Management",
an emoji, a count baked into the title). Report pages with **two** headings, or a heading that
changes with tab state.

**B. Subtitle / description** — Does it match the registry `description`? Missing entirely? Contradicts
what the sidebar row promised? Note `DashHeader` renders `description` with Tailwind `truncate`, so a
long registry description is silently cut with a `title` attribute — that is a real S2.

**C. Eyebrow** — `DashHeader` hardcodes the eyebrow colour to `text-red-400` for every page. Report
what each page passes as `eyebrow`: group title, section name, free text, or nothing.

**D. Icon** — Does the icon rendered in the page header match the registry `icon` for that href?
Also flag registry-side icon **collisions** you notice in your scope (e.g. `Boxes` is used for
Image Registries, Kubernetes *and* Incus; `Globe` for Domains and Environments; `KeyRound` for
ACME, OAuth Clients and Vault; `HardDrive` for App Storage, Image & Cache Cleanup, Backups and
Storage Mounts; `Workflow` for Nomad and Procedures; `FlaskConical` for Preview Environments,
Reconciliation and State Components). Three different nav entries sharing one glyph is a nav
legibility problem, not just a page problem.

**E. Group/section membership** — Does the page feel like it belongs to the group it is filed under?
Is a `secondary: true` ("More") page more important in practice than its primary siblings? Report
IA doubts with reasoning, don't silently re-file them.

**F. Card & section vocabulary** — Which container does the page use: `Card` / `AdminCard` /
`PanelCard` / `StatusCard` / `dashboard-cards` variants / raw `div` with its own border? Are section
headings `SectionHeader`, `AdminSection`, `ForgeSection`, `CardHeader`, or a bare `<h3>`? Report
heading-level jumps (`h1` → `h3`).

**G. Loading / empty / error** — Uses `AdminLoadingState` / `AdminLoadingRows` / `EmptyState` /
`AdminErrorState` / `error.tsx` / `loading.tsx`, or an ad-hoc "Loading..." string, a spinner, or
nothing? Does an empty list render **0** for a value that was never measured?
Project rule (AGENTS.md): *"Never report success for work not performed. Unknown is not zero,
not-reported is not zero, and a stale reading is not a healthy one."* Any place the UI shows a
number, a health dot, a "Live"/"Healthy"/"Up to date" badge, or a percentage it did not actually
measure is **S1**.

**H. Actions & toolbar** — `AdminPageToolbar` or a hand-rolled button row? Are primary actions
(singleton, right) vs batch vs destructive distinguished? Do destructive actions go through
`useConfirm` / `AdminConfirmDialog`? Is search/filter state persisted or lost on nav?

**I. Design tokens** — AGENTS.md: *"Design tokens are CSS variables in `app/globals.css`. Use
`var(--token)`, never a raw hex."* Flag raw hex, `text-slate-*` / `bg-slate-*` / `red-400`,
`bg-white/[0.08]`, arbitrary `text-[10px]` / `p-[13px]`, and magic spacing. `dashboard-cards.tsx`
is believed to be **saturated** with these — count them precisely there if in your scope.

**J. Responsive & density** — Does the layout collapse on narrow viewports? Tables that overflow
without a scroll container or a card fallback? Fixed `min-w-[900px]`? Column counts that break at
`md`?

**K. Accessibility** — Icon-only buttons without accessible names; `title` used as the only tooltip;
missing `aria-label` on selects/inputs (`AdminPageToolbar`'s range `<select>` is labelled, ad-hoc
ones usually are not); heading order; focus-visible rings; contrast of `text-slate-400`/`text-[10px]`
on dark surfaces; tab semantics (`AdminTabs` uses real tab roles?); truncated text with no
accessible full value.

**L. Breadcrumb & routing** — Detail routes (`<id>`, `/new`, `/edit`, `/history`) present a correct
trail via `adminBreadcrumbTrail`; do pages pass explicit `breadcrumbs` overriding it, and are those
overrides wrong/hardcoded? Redirect stubs for aliases in `ADMIN_ALIAS_ROUTES` — do they render a
blank flash instead of a loading state?

**M. Capability gating** — Project convention: controls for a capability the nodes don't report must
be **rendered disabled with a reason line**, not hidden, and never enabled-and-lying. Flag create/run
buttons that are enabled for an unavailable runtime.

**N. Code hygiene visible to UX** — Duplicate imports (confirmed example:
`components/admin/AdminServers.tsx:23` and `:24` both import `fetchWorkloadKinds` from
`@/lib/api/capabilities`), a file importing two rival header systems, dead `hidden` registry entries,
`metadata-only` capability pages rendering full product chrome as if functional.

---

## 3. How to work

1. Read `components/admin/admin-registry.ts` (your slice's rows) and this rubric fully.
2. Read at least two reference pages (`AdminOverview.tsx`, `AdminServers.tsx`, `AdminMonitoring.tsx`
   — pick two) so you can name the gap concretely.
3. For **every** page in your scope: find its `app/admin/<route>/page.tsx` AND the `components/admin/*`
   component it delegates to. Audit both. Many "pages" are a 7-line wrapper — the real UI is the
   component. Do not report a wrapper as clean if you never opened the component.
4. Follow detail routes too (`[id]/page.tsx`, `new/`, `edit/`) — they are in scope even though the
   registry does not list them.
5. **Append findings to your report file after every page you finish** (crash-resume requirement —
   your run may be cut off at any moment; the file must always be a complete record of work done).
6. Do not fix anything. Do not run builds, tests or linters. Do not start a dev server.

## 4. Report format (write to your assigned report path)

```markdown
# Scope <N> — <name>
Audited: <n> registry routes, <m> page files, <k> component files.
Frames in use: DashHeader <count> · AdminPageHeader/SectionHeader <count> · hand-rolled <count> · ForgePage <count>

## Per-page findings
### <Registry label> — <href>
- files: app/admin/.../page.tsx (N lines), components/admin/....tsx (M lines)
- frame: <DashHeader | SectionHeader | hand-rolled | none>
- title: "<rendered>" vs registry "<label>"  → MATCH | MISMATCH | OVERRIDDEN
- description: <match | missing | contradicts | truncated>
- icon: <match | differs | collision>
- S1 · <one-line problem> — `file:line` — why it misleads an operator
- S2 · ... — `file:line`
- S3 · ... — `file:line`

## Scope-level patterns
<3–8 bullets: things that recur across the whole slice and should be fixed once, centrally>

## Proposed remediation for this scope
<ordered, concrete steps. Name the shared primitive or registry edit when that is cheaper than
per-page patching. Mark each item: [frame] [copy] [layout] [state] [tokens] [a11y] [ia>

## Open questions for the orchestrator
<anything needing a product decision you must not unilaterally make>
```

Be dense and factual. No preamble, no restating this rubric, no praise. Every finding needs
`file:line`. If a page is genuinely consistent, write one line saying so — do not pad.
