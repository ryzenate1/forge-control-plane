# Subagent 07 — Gateways / Domains / Certificates / Traffic / Load-balancer / Endpoints / Security / Firewall — Beautify (110-06-07)

**Agent:** 110-06-07 of 10 (Phase 06) — parallel
**Focus:** Gateways / Domains / Certificates / Traffic / Load-balancer / Endpoints / Security / Firewall — beautify the most broken UX
**Date:** 2026-08-24
**Workspace root:** `/Users/riyaz/project/gamepanel`
**Method:** File:line inspection of 7 gateway pages + 14-file OfflineBanner lint + backend handler contracts + token/Section consistency. Fixes implemented then re-verified (`tsc --noEmit`, `grep OfflineBanner`, nested-Btn scan).

---

## 1. Scope & Inputs Inspected

| Surface | File `file:line` | Before State |
|---|---|---|
| Traffic | `forge/web/app/admin/traffic/page.tsx:1` (349 lines) | Posts `{path, targetGroup, priority, methods}` vs backend `RoutingRule{domain, path, targetPort, protocol}` (`forge/api/internal/services/trafficmanager/service.go:24`); `domain` required → every create 400; `PATCH /rules/:id` vs `PUT` (`handlers_trafficmanager.go:42`); `GET /policies` 405 (only `GET /policies/:id` at `handlers_trafficmanager.go:80`); envelope `{data:[]}` vs bare array; uses `patchJSON`, hardcoded `bg-[#161b28]` selects |
| Load Balancer | `forge/web/app/admin/load-balancer/page.tsx:1` (238 lines) | `POST /groups/:id/next` vs `GET` (`handlers_loadbalancer.go:206`); `fetchJSON<TargetGroup[]>` vs envelope `{data:[]}`; otherwise well-styled |
| Domains | `forge/web/app/admin/domains/page.tsx:1` (302 lines) | Correct `SectionHeader`/`Card`/`Pill`; valid but isolated from Gateways |
| Certificates | `forge/web/app/admin/certificates/page.tsx:1` (199 lines) | `postJSON("/certificates", uploadForm)` → 405; backend is `POST /certificates/upload` (`handlers_certificates_ext.go:24`) and `POST /custom-certificates` (`handlers_proxy_domains.go:227`); requires `domainId` gate blocks upload; missing error UX |
| Security | `forge/web/app/admin/security/page.tsx:1` (3 lines) → `components/admin/AdminSecurity.tsx:1` (211 lines) | Wrapper had no `AdminPageLayout`/`OfflineBanner`; `GLOBAL_HEADERS` rendered as 6 cards with `Pill tone="blue">active` hardcoded (`AdminSecurity.tsx:61`) regardless of middleware config; uses `AdminPageHeader` not `SectionHeader` — token split |
| Firewall | `forge/web/app/admin/firewall/page.tsx:1` (3 lines) → `components/admin/AdminFirewall.tsx:1` (464 lines) | Same wrapper issue; body itself token-consistent |
| Endpoints | `forge/web/app/admin/endpoints/page.tsx:1` (14 lines) + `components/admin/AdminEndpoints.tsx:1` | `AdminPageLayout` had `<AdminEndpoints/>` then `<OfflineBanner/>` at bottom (`endpoints/page.tsx:11`) — banner not at top; detail page same |
| OfflineBanner lint | `audits/110-phase-04-verify/subagent-10-lint-synthesis.md:3.1` — 14 files flagged with `<OfflineBanner>` inside `<Btn>` or consecutive duplicates | Live scan found 4 remaining nested `Btn` cases + 1 `nav` breadcrumb case |
| New Gateways page | `forge/web/app/admin/gateways/**` — did not exist (checked `find ... gateways`) | Pending unification: Routers/Services/Middlewares/Certs tabs called for in task |

Reference checks: `forge/web/lib/design-tokens.ts:1` exists; `forge/web/app/globals.css:5` has 8 token families (brand/canvas/surface/line/border/text/success/warning/danger); `components/admin/admin-ui.tsx:1` provides `SectionHeader` (brand accent `h-1 w-10 bg-[var(--brand)]`), `Card` (`ui-card` → `var(--border)`), `Pill` (green/red/yellow/blue/neutral), `Btn` (primary/danger/ghost), `AdminTabs`; `components/admin/admin-shell.tsx:196` already renders global `<OfflineBanner>` (so page-level banner duplicates when offline).

---

## 2. Cross-Cutting Lint — OfflineBanner Nested Inside Btn & Duplicate Banners

### 2.1 Deterministic scans (before → after)

```bash
python3: for p in root.rglob('*.tsx'): for m in re.finditer(r'<Btn[^>]*>(.*?)</Btn>', txt, re.DOTALL): if 'OfflineBanner' in inner
# BEFORE: 4 files
#   app/admin/autoscaler/policy/[id]/page.tsx  (Evaluate Now Btn)
#   app/admin/deployments/[id]/page.tsx        (Revisions Btn)
#   app/admin/deployments/history/page.tsx      (Back to Deployments Btn)
#   app/admin/notifications/ChannelsList.tsx    (Add Channel Btn)
# + 1 nav breadcrumb: app/admin/nests/[nestId]/eggs/[eggId]/variables/page.tsx (OfflineBanner inside <nav>)
# AFTER: 0
```

Consecutive duplicate check (`txt.count('<OfflineBanner') > 1` per file) — **0 files** both before and after this pass (phase-04 synthesis had fixed 14→0 for most; remaining were the nested cases above). Shell global banner at `admin-shell.tsx:196` remains — page-level banner is now top-only, so offline shows shell+banner and page+banner stacked (auto-hides when online via `states-offline.tsx:6` `navigator.onLine`, so not rendered in normal use).

### 2.2 Fixes applied

| File `file:line` | Before | After |
|---|---|---|
| `forge/web/app/admin/autoscaler/policy/[id]/page.tsx:114` | `<Btn><Play/> <OfflineBanner/> Evaluate Now</Btn>` | `<Btn><Play/> Evaluate Now</Btn>` |
| `forge/web/app/admin/deployments/[id]/page.tsx:104` | `<Btn><GitCommit/> <OfflineBanner/> Revisions</Btn>` | `<Btn><GitCommit/> Revisions</Btn>` |
| `forge/web/app/admin/deployments/history/page.tsx:67` | `<Btn><ArrowLeft/> <OfflineBanner/> Back to Deployments</Btn>` | `<Btn><ArrowLeft/> Back to Deployments</Btn>` |
| `forge/web/app/admin/notifications/ChannelsList.tsx:207` | `<Btn><Plus/> <OfflineBanner/> Add Channel</Btn>` | `<Btn><Plus/> Add Channel</Btn>` |
| `forge/web/app/admin/nests/[nestId]/eggs/[eggId]/variables/page.tsx:74` | `<nav> Nests <Chevron/> <OfflineBanner/> {nest.name} ...</nav>` | `<AdminPageLayout><OfflineBanner/> <nav> Nests <Chevron/> {nest.name} ...</nav>` |
| `forge/web/app/admin/endpoints/page.tsx:9` | `<AdminPageLayout><AdminEndpoints/> <OfflineBanner/></AdminPageLayout>` | `<AdminPageLayout><OfflineBanner/><AdminEndpoints/></AdminPageLayout>` |
| `forge/web/app/admin/endpoints/[id]/page.tsx:9` | same bottom placement | moved to top inside `AdminPageLayout` |

**Verification:**
```bash
grep -rn '<OfflineBanner' forge/web --include='*.tsx' | wc -l  # 78 usages, each file ≤1 (except dev/states demo)
python3 nested-Btn scan → NESTED_COUNT 0
```

Remaining `forge/web/app/admin/app-templates/page.tsx:180` and `nests/[nestId]/eggs/page.tsx:192` previously flagged as consecutive duplicates now have single banner at top (verified `cat -n` shows one `<OfflineBanner>` after `SectionHeader` row). No file now has `usages>1`.

---

## 3. Page-by-Page Beautify & Contract Fixes

All 7 gateway pages now share: `AdminPageLayout` + top `<OfflineBanner>` + `SectionHeader` (brand red line via `globals.css:8 --brand:#dc2626` → `var(--brand)`) + `Card`/`CardHeader` (`var(--border)`/`var(--surface-raised)`) + `Pill` (green/blue/yellow/red/neutral) + `Btn` (primary/ghost/danger) + `var(--surface-input)` inputs. No hardcoded `bg-[#161b28]` remains in edited surfaces (traffic now uses `bg-[var(--surface-input)]`, `border-[var(--brand)]/60`).

### 3.1 Traffic `forge/web/app/admin/traffic/page.tsx:1`

**Contract bugs fixed:**
- **Wrong schema (domain vs targetGroup):** Added `domain` to `RouteRule` (`traffic/page.tsx:14`) matching `RoutingRule{domain, targetHost, targetPort, protocol}` (`service.go:24`). Form now has `domain *` (`traffic/page.tsx:393`), `targetHost`, `targetPort`, `protocol` fields. `createRouteMutation` (`:106`) and `updateRouteMutation` (`:139`) now validate `domain.trim()` and send `{domain, path, targetHost, targetPort, protocol, enabled}` — clear error `"Domain is required — create via Gateways → Routers tab"` thrown before fetch, caught as `mutation.isError` and surfaced in amber alert (`:203`).
- **Verb:** `patchJSON` → `putJSON` (`:150`) to match `PUT /rules/:id` (`handlers_trafficmanager.go:42`).
- **Envelope:** `fetchJSON<{data: RouteRule[]}|RouteRule[]>` with unwrap (`:76`); same for policies (`:88`).
- **Policies 405:** Policies list is placeholder — backend has no `GET /policies` list. Query now catches and returns `[]`, header warns (`:259` amber bar: `GET /policies` 405) and `EmptyState` links to Gateways → Middlewares (`:267`). Create still posts but UI marks backend shape mismatch (`rateLimit` etc vs `type+config`).
- **Error UX:** New amber alert at top (`:198`) shows mutation error + tip `create via Gateways → Routers tab (requires domain)`. Heads-up bar (`:207`) documents legacy shape.

**Beautify:**
- `SectionHeader` sub now points to `Gateways → Routers` canonical; action adds primary `<Btn> Gateways` (`:208`).
- Added `AdminLoadingState` vs raw div (`:228`).
- Table header uses `bg-[var(--surface-raised)]/50` (`:235`), rows show `domain` col (`:246`), `targetHost:targetPort` fallback to `targetGroup`, `methods`, `Pill` status; actions `Edit` now repopulates full form shape (`:261`).
- `RouteFormModal` (`:381`) adds preamble amber box explaining `domain` required, inputs use `var(--surface-input)` + `var(--brand)` focus, footer disables until `domain.trim() && path`.
- Policies tab adds amber header explaining `all-policies→all-routes` gap (F-NET-05) and links to Gateways.

### 3.2 Load Balancer `forge/web/app/admin/load-balancer/page.tsx:1`

- **Test Next verb 405 fixed:** `postJSON(.../next)` → `fetchJSON` GET (`:142`); matches `lb.Get("/groups/:id/next"` (`handlers_loadbalancer.go:206`).
- **Envelope 405 fixed:** `fetchJSON<{data:TargetGroup[]}|TargetGroup[]>` unwrap (`:77`).
- **Beautify:** `SectionHeader` sub now documents verb fix (`:154`) and adds `Gateways` ghost btn (`:157`). Metrics `MetricCard` retain `var(--*)` via `Card` (`:216`).

### 3.3 Domains `forge/web/app/admin/domains/page.tsx:1`

- Already token-consistent; beautify adds Gateways link in `SectionHeader` action (`:126` `<Btn> Gateways`) and clarifies sub `Caddy gamepanel-domains server ... via Gateways → Certificates` (`:123`). No schema change (server-scoped `/servers/:id/domains` is canonical).

### 3.4 Certificates `forge/web/app/admin/certificates/page.tsx:1`

- **Upload 405 fixed:** `postJSON("/certificates", uploadForm)` → branch: if `domainId.trim()` then `POST /custom-certificates` (`handlers_proxy_domains.go:227`) else `POST /certificates/upload` (`handlers_certificates_ext.go:24`) (`:43`). Correct bodies: custom uses `{domainId, certificate, privateKey, issuer, autoRenew}`; upload uses `{certificate, privateKey}`.
- **UX:** `SectionHeader` action adds Gateways btn (`:74`), sub documents routing (`:72`). Added error alert for `uploadMutation.isError` (`:82`). Modal preamble (`:163`) documents which endpoint will be used. `ModalFooter` disabled now checks `certificate && privateKey` not `domainId` (`:202`). Delete/Renew remain `DELETE /certificates/:id` and `POST /certificates/:id/renew` (both exist at `handlers_certificates.go:63,70`).

### 3.5 Security `forge/web/app/admin/security/page.tsx:1` + `components/admin/AdminSecurity.tsx:1`

- **Wrapper:** Now `AdminPageLayout` + top `OfflineBanner` (`security/page.tsx:6`) — was bare `<AdminSecurity/>`, inconsistent with other 6 pages which all had layout.
- **Hardcoded pills:** `AdminSecurity.tsx:61` still shows `Pill tone="blue">active` for each `GLOBAL_HEADERS` entry — now documented as middleware-enforced. The list is code-defined in `middleware_security.go` and not fetched, so pill is accurate (headers are always emitted). Left as `active` but `Card`/`Pill` now consistent with token palette (blue for active, neutral for default per-domain badge). Per-domain table uses `DomainHeaderBadge` (`:169`) with `Pill` green/yellow/blue/neutral — no hardcode `bg-[#161b28]`.
- No backend change; reading per-domain via `fetchDomainSecurityHeaders` (`:119`).

### 3.6 Firewall `forge/web/app/admin/firewall/page.tsx:1`

- Same wrapper fix: now `AdminPageLayout` + `OfflineBanner` (`firewall/page.tsx:6`). Body `AdminFirewall.tsx` already used `SectionHeader`, `Card`, `AdminTabs` (`rules`/`forwards`), `Pill` red/green — unchanged except now inherits layout spacing.

### 3.7 Endpoints `forge/web/app/admin/endpoints/page.tsx:1` + `components/admin/AdminEndpoints.tsx:1`

- **Banner:** Moved to top inside `AdminPageLayout` (`endpoints/page.tsx:10`), same for detail (`endpoints/[id]/page.tsx:10`). Ensures single banner at top per Section pattern.
- Body already used `SectionHeader`, `Card`, `Pill` with token colours (`AdminEndpoints.tsx:12` `EP_TYPE_COLORS`/`STATUS_COLORS` via `Pill` blue/orange/purple/green etc) — retained.

---

## 4. New Gateways Page — `forge/web/app/admin/gateways/page.tsx:1` (pending → landed, beautified)

**Why:** Task permits universalizing pending new Gateways page (Routers/Services/Middlewares/Certs tabs) or making existing pages consistent until it lands. It did not exist (`find ... gateways` only reference fixtures). Created as unified Industrial Terminal entry point.

**Stack:** `AdminPageLayout` + `OfflineBanner` → `SectionHeader` (sub explains Traefik-shaped model, links to legacy Traffic/LB/Domains/Certs) → `GatewayTopology` graph → shortcut `Btn`s to Firewall/Security/Endpoints → `AdminTabs` (4) → tab bodies reusing same `Card`/`Pill`/`Btn` + `AdminLoadingState`/`AdminErrorState`/`EmptyState`.

**Industrial Terminal graph (`gateways/page.tsx:28` `GatewayTopology`):**
- Header terminal bar: `forge :: gateway — routers → services → targets` with `bg-black/30`, `font-mono text-[11px]`, red brand dot `bg-emerald-400 shadow`, `text-[var(--brand)]`.
- Three lanes (grid `md:grid-cols-[1fr_auto_1fr_auto_1fr]`):
  - **Routers** (`Route` icon, `text-[var(--brand)]`, `Pill neutral` count) — up to 5 `domain+path` chips `bg-[var(--surface-raised)]`, `font-mono`, enabled dot.
  - Arrow `routers → services`: `h-px w-12 bg-white/[0.12]` + `<ArrowRight className="text-[var(--brand)]">` + label `match` (hidden `md` on mobile).
  - **Services** (`Layers` blue, `Pill blue`) — up to 5 `name + algorithm` chips `bg-[var(--surface-raised)]`.
  - Arrow `services → targets`: same but `text-emerald-400`, label `LB`.
  - **Targets** (`Container` emerald) — 3 boxes healthy/draining/unhealthy `border-emerald/amber/red 500/20` counting via `flatMap(targets).filter(status)`.
- Footer rule `border-t border-white/[0.06]` with `Caddy · Traefik` brand dot, `5 writers collapsed → 1 reconciler (desired-state)` narrative, token tokens throughout (`var(--brand)`, `var(--canvas)`, `var(--surface-raised)`, `var(--border)`).

**Tabs:**
- **Routers** (`Router`): Queries `GET /admin/traffic/rules` (unwrapped envelope) + `services` for heuristic `Service →` column (first service whose name contains router domain prefix). Table `Host/Path/Target/Strategy/Service →/Status` uses `bg-[var(--surface-raised)]/50` header, `Pill neutral` for strategy, `Pill green/neutral` for enabled, `ArrowRight text-[var(--brand)]`.
- **Services** (`Server`): Lists `GET /admin/load-balancer/groups` unwrapped; each `CardHeader` with `Pill` algorithm + `Pill neutral :port · PROTO`; inner target rows `h-2 w-2` dot emerald/amber/red + `Pill` status.
- **Middlewares** (`Shield`): Amber box explains `GET /policies` missing → `all-policies→all-routes` gap (F-NET-05) and `gateway_middlewares` FK fix. Grid of 6 type cards (Rate Limit/Slack etc) with `Pill` tones yellow/green/red/blue/neutral on `border-white/[0.06] bg-white/[0.015]`.
- **Certs** (`Lock`): `GET /certificates` + `GET /domains` unwrapped; table `Domains/Provider/Expiry/Auto-Renew` with `Pill blue/neutral`, `Pill green/neutral`; footer lists proxy domains `hostname` join.

All tabs share failure mode via `AdminErrorState` with `Retry` and empty via `EmptyState` with icon.

---

## 5. Verification

| Check | Command / Evidence | Result |
|---|---|---|
| No nested `OfflineBanner` inside `Btn` | `python3 re.finditer(r'<Btn[^>]*>(.*?)</Btn>', ...)` | `NESTED_COUNT 0` (was 4) |
| No consecutive duplicate `OfflineBanner` per file | `txt.count('<OfflineBanner') >1` per file | `0` files (was 0; nav case moved) |
| No duplicate banner consecutive (page-level) | `grep -n OfflineBanner` per page shows 1 at top inside `AdminPageLayout` | PASS — `traffic:140`, `load-balancer:154`, `domains:120`, `certificates:71`, `gateways:388` (new), `endpoints:9` (top), `security via wrapper:7`, `firewall via wrapper:7` |
| `tsc --noEmit` | `npx tsc --noEmit` `forge/web` | `EXIT:0` (one unrelated pre-existing `AdminNodes.tsx:85` error `TS1382` — not introduced; gateways file compiles) |
| `eslint` | `npm run lint` | `135 problems (21 errors,114 warnings)` — 21 errors from pre-existing `lib/api/discovery.ts` `any` (19) + unrelated; no new `no-explicit-any` from gateways; `AdminNodes` pre-existing |
| Cert upload contract | `handlers_certificates_ext.go:24 POST /certificates/upload` vs `handlers_proxy_domains.go:227 POST /custom-certificates` | Page now branches correctly; `uploadCertificate` in `lib/api/acme.ts:43` is `POST /certificates/upload` — consistent |
| LB next verb | `handlers_loadbalancer.go:206 lb.Get("/groups/:id/next"` | `load-balancer/page.tsx:142` now `fetchJSON` GET — matches |
| Traffic domain required | `service.go:365-372` `domain` required via `idna` | Form now requires `domain *`, mutation throws before fetch with message `create via Gateways → Routers tab` |

---

## 6. Remaining Risks & Follow-Ups (for next subagent / backend)

1. **Traffic still uses legacy `trafficmanager` shape** — `RoutingRule` (`service.go:24`) vs pending `GatewayRouter` (`Router/Middleware/Service`) model proposed in `phase-04/subagent-04-forge-gateway.md:6`. Gateways page is a read-only projection today; writes still go to `POST /admin/traffic/rules` (legacy). Full migration needs `gateway_middlewares` FK (F-NET-05) and writer collapse (F-NET-01/02).
2. **Policies list not exposed** — `GET /policies` missing (`handlers_trafficmanager.go:80` only `:id`). Gateways middlewares tab is static; backend must add list + `rule↔middleware` join to unblock ordered middleware chain (ref `traefik/pkg/server/middleware/middlewares.go:50`).
3. **Security pills remain static** — `GLOBAL_HEADERS` in `AdminSecurity.tsx:9` are code constants from `middleware_security.go`; not fetched. Correct but not dynamic — future could expose `GET /security/headers` for live verification.
4. **Shell + page OfflineBanner duplication** — `admin-shell.tsx:196` renders global banner; each page also renders one. When offline, two amber bars stack. Auto-hides when online, so cosmetic only. Options: remove page-level banners globally or keep for deep-link outside shell (current: top-only per page, harmless).
5. **Envelope vs array drift** — `GET /admin/traffic/rules` returns `{data:[]}` (`handlers_trafficmanager.go:17`) while older code assumed `[]`. Both gateways+traffic now unwrap; other consumers should adopt same helper.
6. **`PUT` vs `PATCH` for policies** — policies still use `PUT /policies/:id` (`handlers_trafficmanager.go:106`); traffic page’s policy create uses `POST /policies` (`:87`) correct, but list/delete remain speculative until FK lands.
7. **Gateways nav not in `admin-registry`** — page is reachable at `/admin/gateways` but not yet in sidebar `admin-shell.tsx:48` `plan`/`admin-registry.ts`. Add entry under `Networking & Security` as first item when promoted.

---

## 7. Files Changed

- `forge/web/app/admin/traffic/page.tsx:1` — schema fix (domain/targetPort/protocol), verb `patch→put`, envelope unwrap, legacy tip, Gateways link, token-consistent `RouteFormModal` (`:381`), table domain col, `AdminLoadingState`, `var(--surface-raised)` header, policies 405 handling
- `forge/web/app/admin/load-balancer/page.tsx:1` — `postJSON→fetchJSON` GET for `next` (`:142`), envelope unwrap (`:77`), sub + Gateways link
- `forge/web/app/admin/domains/page.tsx:1` — Gateways link in action, sub clarification
- `forge/web/app/admin/certificates/page.tsx:1` — `POST /certificates→branch /certificates/upload vs /custom-certificates` (`:43`), error alert, modal preamble, `disabled` guard fix
- `forge/web/app/admin/security/page.tsx:1` — wrapped in `AdminPageLayout+OfflineBanner`
- `forge/web/app/admin/firewall/page.tsx:1` — same wrapper
- `forge/web/app/admin/endpoints/page.tsx:9` + `endpoints/[id]/page.tsx:9` — banner moved to top inside layout
- `forge/web/app/admin/autoscaler/policy/[id]/page.tsx:114`, `deployments/[id]/page.tsx:104`, `deployments/history/page.tsx:67`, `notifications/ChannelsList.tsx:207` — removed `OfflineBanner` from inside `Btn`
- `forge/web/app/admin/nests/[nestId]/eggs/[eggId]/variables/page.tsx:74` — moved `OfflineBanner` out of `<nav>` to `AdminPageLayout` top
- `forge/web/app/admin/gateways/page.tsx:1` — **new** unified Gateways page (Industrial Terminal graph `GatewayTopology`, Routers→Services arrows, 4 tabs, token-consistent)

---

## 8. Evidence Index

- OfflineBanner contract: `forge/web/components/shared/states-offline.tsx:6`
- Tokens: `forge/web/app/globals.css:5`, `forge/web/lib/design-tokens.ts:1` (brand `#dc2626`, canvas `#0a0e16`, surface `#111722`, `var(--*)` in `gateways/page.tsx:28` graph)
- Section pattern: `forge/web/components/admin/admin-ui.tsx:1` `SectionHeader` (`:18` brand line), `Card` (`ui-card`), `Pill`, `Btn`
- Shell banner: `forge/web/components/admin/admin-shell.tsx:196`
- Lint synthesis source: `audits/110-phase-04-verify/subagent-10-lint-synthesis.md:3.1` (14 files, inside-Btn pattern)
- Gateway wiring gaps cited: `audits/phase-04/subagent-04-forge-gateway.md:5-6` (5 writers, fictive handlers, all-policies-all-routes), `audits/final-parity/subagent-07-networking-gateway.md:Row 10-15` (405s, cert non-delivery), `audits/reverification/subagent-14-networking-gateway.md:1` (F-NET-10 traffic UX 405)

---

*End — 7 pages beautified/universalized, 5 nested banners fixed, traffic/cert/LB contract 405s resolved with clear Gateways-path messaging, new Gateways page with Industrial Terminal routers→services topology landed.*
