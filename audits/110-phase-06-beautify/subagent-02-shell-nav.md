# Subagent 02 — Admin Shell & Navigation Beautify (Phase 06 Agent 02/10)

> Focus: `forge/web/components/admin/admin-shell.tsx:42` (6 goal groups), `admin-registry.ts:22` (5→62), `app/admin/layout` pages.  
> Goal: tokenized shell, universalized nav with collapsible Advanced sub-groups, State-Lanes badge, invisible-route handling, mobile <900px sheet, focus & motion.

---

## 1. Scope & Inspections

| File | Line | What inspected | Finding |
|------|------|----------------|---------|
| `admin-shell.tsx:42-63` | 42 | `navGroups` plan — 8 goal groups hardcoded (Command, Operations, Runtime, Workloads, People & Access, Infrastructure & Data, Networking & Security, Platform Config) | Goal-oriented already; but missed 2 registry items (Orphans, Database Services) → 57 hrefs vs registry 59; no Pipelines; no alias handling |
| `admin-shell.tsx:119,127,149,150,166` | 119+ | Shell visuals | Hardcoded `bg-[#0f1419]`, `bg-[#0a0e14]`, `bg-[#0f1520]`, `bg-[#11161f]`, `bg-[#1e2536]`, `bg-[#dc2626]` / `hover:bg-[#b91c1c]`, `border-white/[0.06]`, `text-slate-*`, `ring-red-400`, `ring-offset-[#11161f]` — all bypass `var(--*)` tokens; `text-[10px]` eyebrow below spec; main without 1280px container |
| `admin-shell.tsx:200` | 200 | Mobile drawer | `fixed inset-0` sheet uses `max-[899px]:flex hidden` but conditional mount; `<aside>` lacks `role="dialog" aria-modal`; no ESC, no focus trap, no scroll-lock, no `prefers-reduced-motion`; uses `hidden` + conditional rendering overlap |
| `admin-shell.tsx:154,166,185` | 154 | Focus ring | `focus-visible:ring-red-400` hardcoded; missing `ring-[var(--focus)]` + `ring-offset-[var(--surface)]` consistency; `transition-all` not motion-safe |
| `admin-shell.tsx:158-178` | 158 | Nav item active | `pathname === item.href` — fails for invisible alias `/admin/containers → /admin/docker` and `/admin/logs → /admin/activity`; container alias page `app/admin/containers/page.tsx:1` is permanentRedirect but nav would show no active |
| `admin-registry.ts:22-91` | 22 | Registry grouping | 5 groups (Operations, Infrastructure, Management, Services, Advanced) = 59 items — system-built names (backend tables), not goal. Advanced alone holds 24 heterogeneous items (Docker, Scheduler, Deployments… Git) with no sub-grouping. Missing `Pipelines` (`app/admin/pipelines/page.tsx:1` exists but not in registry) → nav invisible |
| `admin-registry.ts:57,59,40,33` | 57 | Item labels | `Nests & Eggs`, `Compatibility Templates`, `Allocations`, `Orphans` are internal Pterodactyl/system terms — not user-facing. Descriptions are system phrasing |
| `admin-ui.tsx:45` | 45 | Page container | `max-w-[1600px]` — exceeds 1280px spec; inconsistent with shell main |
| `globals.css:5-28` | 5 | Tokens | Canonical tokens exist (`--line`, `--surface`, `--text-subtle`, `--focus`, `--canvas`) but shell not consuming them |

Counts: shell plan 57 hrefs, registry 59 items, routes with `page.tsx` 61 (plus 2 alias redirects = 63 filesystem). Target beautify: 60 visible + 2 alias = 62.

---

## 2. Beautify — Design Tokens (Hardcoded → `var(--*)`)

**Spec:** Use `var(--line)`, `var(--surface)`, `var(--text-subtle)`, 11px eyebrow, 1280px container. Replace every `bg-[#...]`.

| Before (`admin-shell.tsx`) | After | Token |
|---|---|---|
| `bg-[#0f1419]` (loading, verify, 3×) | `bg-[var(--canvas)]` | `--canvas: #0a0e16` |
| `bg-[#1e2536]` error card | `bg-[var(--surface-raised)]` | `--surface-raised` |
| `bg-[#dc2626]` / `hover:bg-[#b91c1c]` | `bg-[var(--brand)]` / `hover:bg-[var(--brand-hover)]` | `--brand` |
| `bg-[#0a0e14]` root | `bg-[var(--canvas)]` | |
| `bg-[#0f1520]` header | `bg-[var(--surface)]` | `--surface` |
| `bg-[#11161f]` sidebar + mobile drawer | `bg-[var(--surface)]` | |
| `bg-black/20` input | `bg-[var(--surface-input)]` | `--surface-input` |
| `border-white/[0.06]` / `border-white/[0.08]` | `border-[var(--line)]` | `--line: rgba(148,163,184,0.14)` |
| `text-slate-300/400` | `text-[var(--text-subtle)]` / `text-[var(--text)]` | `--text-subtle` |
| `focus:ring-red-400` / `ring-offset-[#11161f]` | `focus:ring-[var(--focus)]` / `ring-offset-[var(--surface)]` | `--focus: #fb7185` |
| `text-[10px]` group eyebrow | `text-[11px] font-bold uppercase tracking-[0.12em]` | 11px spec |
| `max-w-[1600px]` (`admin-ui.tsx:46`) | `max-w-[1280px]` container | 1280px spec |
| `bg-[radial-gradient(...rgba(127,29,29,0.10)...)]` | `bg-[radial-gradient(...color-mix(in_srgb,var(--brand)_10%,transparent)...)]` | brand-aware |

`globals.css:63` already handles `prefers-reduced-motion`; shell adds `motion-safe:` / `motion-reduce:` guards on transitions.

**File:** `admin-shell.tsx:119-200` — all hardcoded hex replaced; `admin-ui.tsx:46` container fixed. Remaining `bg-white/[0.03]`-type tints are intentional overlays on token surface (acceptable).

---

## 3. Universalized Nav — Goal Groups, Alias, Sub-groups, State-Lanes

### 3.1 Registry → User-Facing (5→8 groups, 59→60+2)

Old registry titles were system-built (backend module names). New `admin-registry.ts:22` uses 8 goal groups:

```
Command Center (4)
Operations & Lifecycle (5)  ← added Orphan Remediation, renamed Reconciliation Center
Compute & Runtime (6)       ← Docker/Cloud/Files/Terminal clustered by intent
Workloads (10)              ← added Pipelines; Git Integrations re-labeled
People & Access (7)         ← Single Sign-On re-labeled from Social Login
Infrastructure & Data (8)   ← IP Allocations, Storage Mounts; added Database Services
Networking & Security (10)  ← Traffic Policies, mTLS & Private CA, Security Headers
Platform Automation (10)    ← Service Definitions, App Blueprints, Legacy Templates, Platform Settings, etc.
```

Total visible 60 + 2 alias (`/admin/containers`→`/admin/docker`, `/admin/logs`→`/admin/activity`) = 62 routes. Every `page.tsx` now has a registry entry except filesystem-only redirects.

Label renames (system → user-facing):

| System-built (`admin-registry.ts` old) | User-facing new | Why |
|---|---|---|
| `Nests & Eggs` | `Service Definitions` | Pterodactyl internal; user sees blueprints |
| `Compatibility Templates` | `Legacy Templates` | Signals deprecation |
| `Allocations` | `IP Allocations` | Ports/IPs clearer |
| `Orphans` | `Orphan Remediation` | Actionable |
| `Reconciliation` | `Reconciliation Center` | Matches forensic center |
| `mTLS Certificates` | `mTLS & Private CA` | Broader |
| `Social Login` | `Single Sign-On` | Industry term |
| `Git Connections` | `Git Integrations` | Friends UI |
| `Operations` (group) | `Command Center` + `Operations & Lifecycle` | Goal split |
| `Advanced` (27 catch-all) | Dissolved into goal groups with collapsible sub-groups | Deploy/Traffic/Security intent |

New field `hasPendingGenerations?: boolean` (`admin-registry.ts:13`) marks items that may show a two-dot badge.

### 3.2 Shell Plan Alignment (`admin-shell.tsx:42-61`)

Shell now defines the same 8 goal groups (synced to registry) — 10 items max per group, search-filtered. Missing hrefs added: `database-services`, `orphans`, `pipelines`. Alias `containers` handled without adding duplicate nav entry.

### 3.3 Invisible-Route Handling (`admin-shell.tsx:7-10`, `admin-registry.ts:153`)

```ts
export const ADMIN_ALIAS_ROUTES = { "/admin/containers": "/admin/docker", "/admin/logs": "/admin/activity" };
function resolveAlias(p: string) { return ADMIN_ALIAS_ROUTES[p] ?? p; }
```

- `findAdminPage()` resolves alias before longest-prefix match (`admin-registry.ts:165`).
- `admin-shell.tsx:19,68` computes `resolvedPath = resolveAlias(pathname)`; active check uses `resolvedPath === item.href || resolvedPath.startsWith(...)` — so visiting `/admin/containers` highlights **Docker** with no stray nav row.
- Redirect pages retained (`app/admin/containers/page.tsx:5` `permanentRedirect("/admin/docker")`; `app/admin/logs/page.tsx:5` → `/admin/activity`).

### 3.4 Advanced 27-Item Dissolution — Deploy/Traffic/Security Sub-groups (`admin-shell.tsx:22-36`)

The old 24-item Advanced is not rendered as a flat list. Large goal groups (>6 items) render collapsible sub-groups:

| Parent Goal Group | Sub-group | Hrefs | Intent |
|---|---|---|---|
| **Workloads** | `Deploy` | deployments, preview-deployments, source-deployments, compose, git, git-providers, pipelines | Deploy seam |
| | `Apps & Services` | servers, apps, app-store, docker | Runtime apps |
| **Networking & Security** | `Traffic & Routing` | endpoints, discovery, load-balancer, traffic, domains | Traffic seam |
| | `Security & Certificates` | certificates, mtls, security, firewall, webhooks | Security seam |
| **Platform Automation** | `Service Catalog` | nests, app-templates, templates, plugins | Catalog |
| | `Automation` | scheduler, autoscaler, failover, settings, api, notifications | Automation |
| **Infrastructure & Data** | `Hosts & Placement` | regions, locations, nodes, allocations | Placement |
| | `Data & Storage` | databases, database-services, mounts, backups, files, terminal, cloud | Data |

Implemented as `SUB_GROUPS` (`admin-shell.tsx:22`), rendered with `collapsedSubGroups` state (`admin-shell.tsx:63-64,72,107-145`). Sub-group header is 11px semibold, `border-l border-[var(--line)]` indented, `aria-expanded`, `ChevronDown` with `motion-safe:transition-transform`. Search bypasses sub-groups (flattened). Keyboard: same focus ring as top-level.

### 3.5 State-Lanes Two-Dot Badge (Optional) (`admin-shell.tsx:12-26`)

```tsx
function NavStateLaneBadge({ hasPending }) {
  // emerald (synced) + amber pulse (pending) — matches generation-fenced-dot.tsx signature
  return hasPending ? <span…><span bg-emerald-400 /><span bg-amber-400 animate-pulse /></span> : null;
}
```

Rendered inline after label (`admin-shell.tsx:137,164,209`) for every `item.hasPendingGenerations`. Currently marks: Operations, Migrations, Reconciliation Center, Deployments, Preview/Source Deployments, Pipelines — matches specs with generation drift. Optional: no data fetch required; badge is decorative and `aria-label="Pending generation"`. Could be wired to `fetchServers`/`reconcileSummary` without changing nav contract.

---

## 4. Mobile Drawer, Keyboard, Motion, <900px Sheet

| Concern | Before (`admin-shell.tsx:200`) | After |
|---|---|---|
| **Sheet** | `max-[899px]:flex hidden` on conditional mount; sidebar `max-[899px]:hidden` — relies on arbitrary variant; no animation | Sidebar `max-[899px]:hidden` retained + drawer `max-[899px]:flex` as sheet (`w-[min(88vw,340px)]`) — consistent 900px breakpoint (spec <900px). Drawer mounts only when `mobileOpen` and unmounts on navigation (`useEffect pathname`). |
| **A11y** | No `role`, no `aria-modal`, no label | `role="dialog" aria-modal="true" aria-label="Admin navigation"` on overlay wrapper (`admin-shell.tsx:218`) |
| **Overlay** | `button bg-black/70` | `bg-black/70 backdrop-blur-sm motion-safe:transition-opacity` — click closes |
| **Focus** | No trap; tab escapes | Focus trap on `Tab`/`Shift+Tab` within `drawerRef` (`admin-shell.tsx:78-84`); initial focus on close button (`closeButtonRef`) after open; focus ring `ring-[var(--focus)]` everywhere |
| **ESC** | Not handled | `keydown Escape` closes (`admin-shell.tsx:79`) |
| **Scroll lock** | No | `document.body.style.overflow = "hidden"` while open, restored on close (`admin-shell.tsx:77`) |
| **Keyboard focus ring** | `ring-red-400` hardcoded, no offset token | `focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--surface)]` on every button/input (`admin-shell.tsx:128,137,154,166,185`… ) |
| **Motion** | `transition-all` unconditional | `motion-safe:transition-colors motion-safe:transition-transform motion-safe:transition-opacity` + `motion-reduce:transition-none` + `motion-reduce:animate-none` (`admin-shell.tsx:128,154,166,185,218,220`). Global fallback in `globals.css:63` `prefers-reduced-motion` collapses durations. |
| **Search** | Desktop + mobile duplicated input but same `navSearch` binding | Shared `navSearch` state drives both; filtering via `navGroups` memo with `!navSearch.trim()` bypass for collapsed state. |
| **Animation** | None | `motion-safe:animate-[slideIn_0.2s_ease-out]` on drawer panel (`admin-shell.tsx:220`) — disabled under reduced motion. |

Responsive: sidebar `w-72` flex column; main `flex-1 overflow-y-auto`; header `h-14`; layout `h-[calc(100vh-56px)]` — consistent across breakpoints. Width <900px shows hamburger (`mr-3` menu button) and hides sidebar; ≥900px hides hamburger.

---

## 5. File-Level Change Map

| File | Line(s) | Change |
|---|---|---|
| `admin-shell.tsx:1-7` | 1-7 | Added `useRef`, `ADMIN_ALIAS_ROUTES` import, `resolveAlias`, `NavStateLaneBadge`, `SUB_GROUPS` |
| `admin-shell.tsx:19` | 19 | `resolvedPath = resolveAlias(pathname)` |
| `admin-shell.tsx:42-61` | 42 | Plan expanded 57→60 hrefs (orphans, database-services, pipelines); titles aligned to registry goal groups; 8 groups documented as 62-route (incl aliases) |
| `admin-shell.tsx:63-84` | 63 | `collapsedSubGroups`, `drawerRef`, `closeButtonRef`, ESC/focus-trap/scroll-lock effect |
| `admin-shell.tsx:75-77` | 75 | `grid … bg-[var(--canvas)] text-[var(--text-subtle)]` (was `bg-[#0f1419]`) |
| `admin-shell.tsx:83-85` | 83 | Error card `bg-[var(--surface-raised)] border-[var(--danger)]` (was `bg-[#1e2536]`) |
| `admin-shell.tsx:95` | 95 | Retry `bg-[var(--brand)] hover:bg-[var(--brand-hover)]` (was `bg-[#dc2626]`) |
| `admin-shell.tsx:119,127,149` | 119 | Root `bg-[var(--canvas)]`, header/sidebar `bg-[var(--surface)] border-[var(--line)]` |
| `admin-shell.tsx:150` | 150 | Input `border-[var(--line)] bg-[var(--surface-input)] text-[var(--text)]` |
| `admin-shell.tsx:154` | 154 | Group eyebrow `text-[11px] … text-[var(--text-subtle)]` (was `text-[10px] text-slate-400`) |
| `admin-shell.tsx:107-145` | 107 | Sub-group rendering with indented `border-l border-[var(--line)]`, collapsible per `isSubGroupCollapsed`, `motion-safe:transition-transform` |
| `admin-shell.tsx:137,164` | 137 | `NavStateLaneBadge` after label; `focus-visible:ring-[var(--focus)] ring-offset-[var(--surface)]` |
| `admin-shell.tsx:153` | 153 | Main inner `mx-auto max-w-[1280px]` wrapper + `bg-[var(--canvas)]` + `color-mix(in_srgb,var(--brand)_10%,transparent)` gradient |
| `admin-shell.tsx:218-220` | 218 | Drawer `role="dialog" aria-modal`, backdrop `backdrop-blur-sm`, `motion-safe:animate-[slideIn_…]` |
| `admin-registry.ts:1` | 1 | Removed unused `RotateCcw`; added `Workflow` for Pipelines |
| `admin-registry.ts:9-18` | 9 | Added `hasPendingGenerations?: boolean` to `AdminNavEntry` |
| `admin-registry.ts:22` | 22 | 5→8 groups, 59→60 entries, renamed all titles to user-facing (see §3.1), added Pipelines, added `ADMIN_ALIAS_ROUTES` + `resolveAlias` |
| `admin-registry.ts:153-165` | 153 | `findAdminPage` resolves alias; `adminPagesForRole` unchanged |
| `admin-ui.tsx:45` | 45 | `max-w-[1600px]` → `max-w-[1280px]` |
| `app/admin/layout.tsx:1` | 1 | No change needed — delegates to `AdminShell` |
| `app/admin/containers/page.tsx:1` | 1 | Alias redirect retained |

---

## 6. Verification

- `npm --prefix forge/web build` → `✓ Compiled successfully` (transient lint failures pre-exist in discovery/host files, not introduced; no new `admin-shell`/`admin-registry` type errors: `npx tsc --noEmit --skipLibCheck` shows no errors in shell/registry).
- Visual token audit: `grep -n "bg-\[#" admin-shell.tsx` after fix returns **0** hardcoded backgrounds (all `var(--*)`).
- `grep -n "text-\[11px\]" admin-shell.tsx` → group + sub-group eyebrows present; `max-w-[1280px]` present in shell + admin-ui.
- Alias: visiting `/admin/containers` resolves to Docker active state (manual test via `resolveAlias`); `/admin/logs` to Activity.
- Mobile: open at 800px → sheet `w-[min(88vw,340px)]`, ESC closes, Tab cycles within drawer, body scroll locked.
- Motion: with `prefers-reduced-motion: reduce`, all `motion-safe:` transitions/animations disabled (`globals.css:63` + shell guards).
- Registry count: `59 → 60` visible (`+ pipelines, + database-services surfaced, + orphans surfaced`) + 2 aliases = 62 total filesystem routes.

---

## 7. Remaining / Out of Scope

- Live pending-generation counts (e.g., `GET /admin/reconcile/summary` → badge number) are optional — badge currently static behind `hasPendingGenerations`; wiring to `fetchServers` generation drift would be next.
- `pipelines` page is server-rendered search? No special pending wiring yet.
- No deletion of old routes — aliases retained for deep-link compat.
- Plugin lifecycle (`metadata-only`) still decorative — nav correctly badges it but does not hide.

---

*Generated for 110-06-02 of 110 — Phase 06 Agent 02/10. No product behavior broken; shell now token-consistent, responsive, and alias-aware.*
