# Scope 7 — Networking group (12 routes)
Audited: 12 registry routes, 14 page files (incl. `domains/[id]`, `endpoints/[id]`, 6 thin wrappers),
7 component files, 4 shared-primitive files, 1 route-integrity test.

Frames in use: DashHeader 0 · AdminPageHeader/SectionHeader 12 · hand-rolled root wrappers 3
(`AdminDiscovery.tsx:115`, `AdminEndpoints.tsx:75`, `AdminAllocations.tsx:161`) · ForgePage 0 ·
`AdminPageLayout` applied inconsistently — see scope pattern 9.

Reference gap stated once, applies to all 12 below: **not one page in this slice renders
`FreshnessBadge` or `PageInfoDisclosure`** (`grep FreshnessBadge|PageInfoDisclosure` across all 12 route
directories + 5 delegated components → 0 hits), while the reference pattern (`AdminOverview`,
`AdminMonitoring`, `AdminServers`) binds both. No page in the slice uses `AdminPageToolbar` either, and
no page passes an icon to its header — `SectionHeader` (`components/admin/admin-ui.tsx:140-161`) has no
`icon` prop at all, so every registry `icon` in this group exists only in the sidebar.

---

## Per-page findings

### Domains — `/admin/domains`
- files: `app/admin/domains/page.tsx` (306 lines), `app/admin/domains/[id]/page.tsx` (505 lines, audited below)
- frame: `AdminPageLayout` + `SectionHeader` (`app/admin/domains/page.tsx:123-124`)
- title: `"Domain Management"` vs registry `"Domains"` → **MISMATCH** — `app/admin/domains/page.tsx:125`
- description: **rewritten, not matched**. Registry: "Custom domains with DNS and TLS status". Page adds a
  cross-sell sentence naming four other pages, two of which live under different registry groups
  (`DNS Providers`, `ACME Accounts`) and one of which (`Security Headers`) was **moved out of Networking
  into Access** (`components/admin/admin-registry.ts:216`, and the registry comment at `:106-107` says so
  explicitly) — `app/admin/domains/page.tsx:126`. The sub-copy therefore re-teaches the old IA.
- icon: registry `Globe` (`admin-registry.ts:171`); page renders no header icon. `Globe` is also the
  registry icon for `Environments` (`admin-registry.ts:209`) — collision outside this slice, but it makes
  "Domains" ambiguous in the sidebar/palette.
- S1 · The list is **gated behind a server selection** and there is no fleet view at all:
  `enabled: !!serverFilter` (`app/admin/domains/page.tsx:48`) with `EmptyState "Select a server to view
  its domains."` (`:149`). Registry promises "Custom domains with DNS and TLS status" — an operator
  arriving from search or a deep link sees an empty card and no count, and cannot answer "how many
  unverified domains do I have fleet-wide". Also no org/tenant scope control anywhere, which is the exact
  surface the past env-var cross-tenant leak lived on.
- S1 · "Check DNS" result panel colours a **mismatch** as amber and labels it
  `"DNS mismatch or not verified"` (`app/admin/domains/page.tsx:280`) — the same chip covers "resolved to
  the wrong IP" and "we could not check", two opposite operational states. A hard DNS error
  (`dnsResult.error`, `:282`) renders inside the same amber box, so a failed lookup reads as a warning
  about the domain rather than a failed probe.
- S2 · No `Verified` column value for the pending state: `verifyMutation` only ever toasts
  (`:84-96`). While it is in flight the row still shows the old `Unverified` pill and the Verify button is
  disabled globally (`:199` `disabled={verifyMutation.isPending}`) — a single shared `isPending` freezes
  every Verify button in the table, with no per-row spinner, so an operator cannot tell which domain is
  being checked.
- S2 · Raw-Tailwind page, in a group where `app/globals.css` tokens are the rule (AGENTS.md):
  `text-slate-500`/`text-slate-200`/`text-slate-400` and `border-white/[0.06]`,
  `divide-y divide-white/[0.04]`, `bg-white/[0.02]`, `text-[10px] uppercase tracking-widest`
  (`app/admin/domains/page.tsx:151, 158, 166, 169, 189, 229, 250`), plus `border-emerald-500/30
  bg-emerald-500/10` / `border-amber-500/30` (`:278`) and `text-red-300` (`:282`).
- S2 · Hand-rolled `<table>` with a bespoke header row (`app/admin/domains/page.tsx:156-165`) instead of
  `AdminTable/AdminTHead/AdminTh` used three pages down the same group (`app/admin/dns/page.tsx:94-95`).
  Result: two table grammars inside one group, and the raw table has no `aria-label`
  (`AdminTable` accepts one, `components/admin/admin-ui.tsx:471-473`).
- S2 · Ad-hoc loading string `<div className="p-8 text-center text-sm text-slate-500">Loading domains...
  </div>` (`:151`) instead of `AdminLoadingState` / `AdminLoadingRows`. `domainsQuery.isError` is **never
  rendered** — a failed fetch falls to `filteredDomains.length === 0` and shows
  "No domains configured for this server" (`:153`), i.e. an error reported as an empty list.
- S2 · Two different select implementations on one page: the toolbar uses `AdminSelect` (`:144`) while the
  Add Domain modal uses a bare `<select>` with its own class string and **no accessible name**
  (`:230-234`; the `<label>` at `:229` is not associated — it is not a `<label for>`/wrapper pair).
- S2 · Toolbar row is a hand-rolled `<div className="flex gap-2">` of four buttons
  (`:128-137`), three of which are cross-page navigation links and one is the primary create action. They
  carry equal weight, and the destructive-capable primary action is styled by an inline
  `bg-[var(--brand)]` override (`:134`) rather than the `tone="primary"` path.
- S3 · Search (`:37`) and server filter (`:38`) are local `useState` — both are lost on navigation and
  never reach the URL, so a filtered view cannot be shared or re-opened from the sidebar.
- S3 · Wildcard hint text builds a hostname by string surgery
  (`test.${addForm.domain.replace("*.", "")}`, `:250`) and is only shown when the typed value starts with
  `*.`; no validation of the domain string itself is performed before submit (`:257` checks only non-empty).

### Certificates — `/admin/certificates`
- files: `app/admin/certificates/page.tsx` (255 lines) — no delegated component
- frame: `AdminPageLayout` + `SectionHeader` (`:95-96`)
- title: `"Certificate Management"` vs registry `"Certificates"` → **MISMATCH** — `:97`
- description: rewritten — registry "Public TLS certificates and automated issuance"; page
  "Manage TLS/SSL certificates for proxy domains. Upload custom certificates or use Let's Encrypt
  auto-provisioning." (`:98`). "proxy domains" is a third vocabulary term for what the registry calls
  Domains and the Gateways page calls `hostname`.
- icon: registry `Award` (`admin-registry.ts:172`); page renders none in the header and uses `Shield`
  for the `CardHeader` (`:112`) — `Shield` is the registry icon for **Firewall** (`:174`). So the
  Certificates card is branded with Firewall's glyph while its nav row uses `Award`.
- S1 · The **Upload Certificate form silently discards three of its five fields.** `uploadMutation` posts
  only `{ certificate, privateKey }` (`:47`) while the form collects `domainId` (`:219`), `issuer`
  (`:238`) and `autoRenew` (`:239-242`, labelled "Auto-renew (Caddy managed)"). The operator supplies a
  domain ID and ticks auto-renew, gets no error, and the server never sees either value. The success
  path (`:48-52`) closes the modal with **no success toast at all** — every other mutation on this page
  toasts, so silence reads as success while three inputs were dropped.
- S1 · Certificate status is computed **client-side from a 30-day constant**, not from the API:
  `isExpiring` = `days < 30` (`:87-90`). The list also renders "Renew" only when
  `cert.autoRenew` is true (`:168`), so a certificate whose auto-renew flag was never persisted (see the
  upload bug above) is silently un-renewable from this page while still displaying `Valid` (`:163`).
- S2 · Status vocabulary invented per page. Certificates uses `Valid`/`Expiring`/`Expired`
  (`:159-163`) with `Pill tone="red|yellow|green"` plus a *second* redundant icon in the same cell
  (`XCircle text-red-400`, `AlertTriangle text-amber-400`, `CheckCircle text-emerald-400`, `:159-163`).
  Domains uses `Verified`/`Unverified` with the same icon+pill doubling (`domains/page.tsx:178-186`);
  Gateways uses lowercase `enabled`/`disabled` (`gateways/page.tsx:409`); Traffic uses
  `Active`/`Inactive` for the same boolean field (`traffic/page.tsx:215`); Load Balancer uses
  `Enabled`/`Disabled`. Five spellings of "the flag is on".
- S2 · The `Expiry` column renders a bare `toLocaleDateString()` (`:150`) — no remaining-lifetime text,
  even though the 30-day rule that drives the pill is a lifetime. The Gateways page renders the *same
  field from the same endpoint* as `toLocaleDateString()` too (`gateways/page.tsx:561`), and neither
  page surfaces `issuer` at all despite the type carrying it (`:15`).
- S2 · Ad-hoc `Loading certificates...` div (`:118`); `certsQuery.isError` never rendered, so a failed
  fetch shows "No certificates configured. Upload a certificate or use the ACME service to provision
  one." (`:120`) — an error presented as an empty state, and it actively invites the operator to create
  more.
- S2 · Raw table again (`:123-133`) with the same bespoke `text-[10px] uppercase tracking-widest
  text-slate-500` header row as Domains, plus bare `<select>` (`:193-200`) and two bare `<textarea>`s
  (`:222-227`, `:232-237`) with **unassociated `<label>`s** — the `Challenge Type` label at `:192` and
  `Certificate (PEM)` at `:221` are sibling `<label>` elements with no `htmlFor`, so neither control has
  an accessible name (a11y K).
- S2 · Private-key paste field is a plain `<textarea>` with default autofill/spellcheck and no
  `autoComplete="off"` or `spellCheck={false}` (`:232-237`), and the value is held in plain React state
  (`:31`) with no redaction on modal close beyond the reset at `:51`.
- S2 · Certificate "domains" cell renders one `<span>` per SAN with no truncation and no copy
  (`:139-141`); a wildcard + 8 SANs certificate grows the row height arbitrarily. Contrast Gateways,
  which renders the same array joined with `", "` in a single truncated-free cell
  (`gateways/page.tsx:556`).
- S3 · Two `Plus`-icon buttons side by side, `Request Certificate` (primary) and `Upload Certificate`
  (ghost) (`:101-106`), differing only by 12px of colour — same glyph for two different verbs.
- S3 · `Array.isArray(filtered) &&` guard inside the `<tbody>` (`:135`) duplicates the `useMemo`
  normalisation at `:40`, the same defensive leftover as `domains/page.tsx:167`.

### Gateways — `/admin/gateways`
- files: `app/admin/gateways/page.tsx` (584 lines)
- frame: `AdminPageLayout` + `SectionHeader` + `AdminTabs` + `Card`/`CardHeader` (`:304-306`)
- title: `"Gateways"` vs registry `"Gateways"` → **MATCH** (`:307`)
- description: **massively rewritten and self-contradicting** — a 260-char paragraph
  (`:308`) vs registry "Edge gateway routers, services and middlewares" (`admin-registry.ts:173`). It
  ends by telling the operator that four sibling nav entries are "Legacy".
- icon: registry `GatewayRouterIcon` (`:173`); header renders none; `CardHeader` icons are `GitBranch`,
  `Router`, `Shield`, `Lock` (`:95, 360, 487, 526`) — none of them the registry glyph.
- S1 · **The same endpoint is modelled with two different field sets.** Gateways fetches
  `/admin/traffic/rules` (`:258`) into a `RoutingRule` typed `{domain, path, targetHost, targetPort,
  protocol, strategy, enabled}` (`:42-52`) and renders Host / Path / Target / Strategy / Service → /
  Status columns (`:381-386`). Traffic fetches **the same URL** (`traffic/page.tsx:66`) into a
  `RouteRule` typed `{path, targetGroup, priority, methods, enabled}` and renders Path / Target Group /
  Priority / Methods. One of the two pages is therefore rendering columns the API may not emit (and
  `domains`/`domainId` in Gateways' `ProxyDomain` type, `:72-77`, is a third spelling). An operator
  comparing the two screens cannot tell which one is true.
- S1 · The "Service →" column is **invented by string matching, not by data.**
  `matchServiceName` (`:80-86`) takes the first label of the router's domain, lowercases it and
  case-matches it against target-group names, falling back to `"—"`. It is presented as a topology edge
  with an arrow (`:403-406`) and captioned "match" (`:155`), so a guessed name is drawn as a confirmed
  binding. There is no `serviceId` on the wire even though the type declares one (`:75`).
- S1 · The Targets lane counts healthy/draining/unhealthy by filtering `t.status` for exactly those
  three strings (`:215, 221, 227`) inside a fixed three-bucket grid. Any other status value — including
  empty/unknown — is counted in the header total (`:91`) but appears in **no bucket**, so the three
  numbers do not sum to `targetCount` and the panel silently under-reports. The count is drawn as a
  confident green/amber/red tile (`:213-230`).
- S1 · The Middlewares tab renders a card whose visible chip text is literally the tone name:
  `<Pill tone={m.tone}>{m.tone}</Pill>` (`:506`) — the operator reads "yellow", "green", "red", "blue",
  "neutral" as status labels on six middleware tiles (`:493-499`).
- S2 · Header actions are four `window.location.assign` "navigation" buttons
  (`:311-323`) and the *last* one is `tone="primary"` (`:320`) — the primary action slot is occupied by a
  link to Certificates, and there is **no create action on the page at all**, while the tab strip claims
  routers/services are the page's own subject. A second orphan button row (`:329-354`) duplicates that
  cross-navigation with three more `location.assign` calls. `window.location.assign` also throws away
  the client-side cache that `Link` would keep.
- S2 · IA: this page is the group's own admission that the grouping is broken. Its sub-copy says
  "Legacy pages remain at Traffic / Load Balancer / Domains / Certificates" (`:308`), its empty state
  says "Create via Traffic → Create Route (requires domain)" (`:374`) — and the Traffic route form has no
  domain field at all (`traffic/page.tsx:395-398`), so the instruction is unsatisfiable. The Targets lane
  caption tells the operator the data lives at a raw path string:
  `via /admin/load-balancer` (`:233`).
- S2 · Tab labels bake counts into the page chrome: `` `Routers · ${routers.length}` `` etc.
  (`:297-300`). While the query is loading this reads `Routers · 0`, i.e. a measured zero for a value
  never measured (AGENTS.md). Traffic's tabs in the same situation print no counts
  (`traffic/page.tsx:157-160`) — two conventions for the same problem.
- S2 · `title="Hover router to see its service binding"` (`:245`) is both a `title`-as-only-tooltip a11y
  issue and **false**: no router row has a hover handler (`:391` has only `hover:bg-…` styling), so the
  advertised interaction does not exist.
- S2 · The topology diagram is hidden below `md`: the two connector arrows are
  `hidden … md:grid` (`:150`, `:194`), and the whole thing collapses to
  `md:grid-cols-[1fr_auto_1fr_auto_1fr]` → one column (`:110`). On a narrow viewport the "routers →
  services → targets" story loses every arrow, leaving three unlabelled stacks.
- S2 · Explanatory dev text as UI copy: "5 writers collapsed → 1 reconciler (desired-state)" (`:244`),
  audit ID "(see audit F-NET-05)" (`:490`), a backend route contract in a `<code>` block
  (`:490`), and "The legacy Traffic page's `POST /policies` shape … differs from backend's typed fields"
  (`:516-517`). None of this is actionable by an operator, and it appears only on this page in the
  whole group.
- S2 · This is the only page in the slice that mostly uses tokens (`var(--line)`, `var(--surface-raised)`,
  `var(--text-subtle)`, `var(--success)`, `var(--danger)`, `var(--warning)` — `:98-137`, `:380-414`),
  and it *still* leaks raw colours: `text-blue-400`/`text-blue-300` (`:162, 179`), `text-emerald-400`
  (`:197, 206, 214`), `border-emerald-500/20 bg-emerald-500/10` (`:213`), `text-amber-400` (`:220`),
  `border-red-500/20 bg-red-500/10 text-red-400` (`:225-227`), and a raw rgba glow
  `shadow-[0_0_8px_rgba(5,150,105,0.4)]` (`:99`). Two token systems in one file is worse than one.
- S2 · Status dot vocabulary: Gateways uses `bg-[var(--success)]`/`bg-[var(--warning)]`/`bg-[var(--danger)]`
  (`:466`), Load Balancer uses `bg-emerald-400`/`bg-amber-400`/`bg-red-400`
  (`load-balancer/page.tsx:247`) and Domains/Certificates use icon+pill doubling instead of a dot
  (`domains/page.tsx:178-186`, `certificates/page.tsx:159-163`) — three renderings of healthy/warn/fail.
- S3 · `domainsQuery` (`:273-280`) fetches `/domains` but is rendered only inside a footer sentence on
  the *Certs* tab (`:574-578`), listing `d.hostname` for at most three rows with no link to the Domains
  page — an unclickable inventory of a resource that has its own nav entry.
- S3 · Free text in the copy uses an en-dash "—" as a placeholder for "no match" (`:82, 85`) while the
  Target cell renders `:${r.targetPort}` with a leading colon when the host is absent (`:395`) — an
  IP:port rendered as a fragment.
- S3 · `OfflineBanner` (`:305`) is imported from `components/shared/states-offline` and used on no other
  page in this slice, so only one of the twelve routes warns about connectivity.

### Firewall — `/admin/firewall`
- files: `app/admin/firewall/page.tsx` (7-line wrapper, bare), `components/admin/AdminFirewall.tsx` (520 lines)
- frame: component renders its own `AdminPageLayout` (`:93`) + `SectionHeader` + `AdminTabs` +
  `Card`/`CardHeader` + `AdminTable` — the most conventional frame in the group, and the correct
  wrapper/component split.
- title: `"Firewall"` vs registry `"Firewall"` → **MATCH** (`:95`)
- description: `"Firewall rules and port forwarding."` (`:96`) — matches the registry verbatim
  (`admin-registry.ts:174`). **The only page in the slice that does**, and it is the shortest description
  in the group.
- icon: registry `Shield` (`:174`); header renders none, but `CardHeader icon={Shield}` (`:159`) matches
  the nav glyph — the only such match in twelve routes. `Network` for Port Forwards (`:193`) is NetBird's
  / Placement Affinity's registry glyph (`:167`, `:203`).
- S1 · **Disabling a node's firewall is a one-click action with no confirmation.**
  `onClick={() => status.enabled ? disableMut.mutate() : enableMut.mutate()}` (`:128`) fires immediately;
  `useConfirm` is wired up and used for deleting a single rule (`:273`) and a port forward (`:304`), but
  not for the action that removes the host's entire protection. The button is even
  `tone="danger"` (`:126`), so the styling says destructive and the flow says no.
- S1 · **An invalid firewall rule is not blocked before submit.** `AddRuleModal`'s only gate is
  `disabled={addMut.isPending}` (`:355`): the `Port` field is `type="number"` with no range check
  (`:342`), so empty, `0`, `-5` or `99999` all submit as `port: Number(port)` or `undefined`
  (`:325`); `Source IP` (`:347`) is free text with a CIDR placeholder and **no validation whatsoever**, so
  `0.0.0.0/33`, `10.0.0.` or `any` submit and only surface as a backend toast (`:335`). The `Action`
  select (`:348-351`) can be emptied by any future option and defaults to `allow` (`:320`), meaning the
  least-safe value is the initial value of a security control. **The same file validates the same field
  correctly elsewhere**: `QuickOpenPortModal` checks `Number(port) < 1 || Number(port) > 65535`
  (`:472`) and blocks the button (`:492`), and `AddForwardModal` checks presence of all three fields
  (`:432`, `:449`). Two validators for one concept, applied to two of four modals.
- S1 · **There is no port-range support anywhere.** `AddRuleModal` (`:342`) and `QuickOpenPortModal`
  (`:478`) take a single scalar port; `FirewallRule.port` is rendered as one number
  (`:265`). By contrast the sibling IP Allocations page has a real range parser supporting
  `25565-25580`, comma lists and a 2 000-port cap (`AdminAllocations.tsx:26-33`), documented in its form
  (`:355`). An operator who needs "open UDP 25565-25580" finds the vocabulary on one page and not the
  other, and nothing links them.
- S2 · **`Protocol` has three different vocabularies in one file.** Rules and Forwards offer
  `tcp | udp` only (`:343-346`, `:390-393`, `:441-444`); Quick Open adds `both` → "TCP+UDP"
  (`:479-483`) writing the literal string `"both"` into the same `protocol` field the other modals
  constrain to `tcp|udp`. The list column then renders `rule.protocol` upper-cased raw
  (`text-xs uppercase`, `:266`), so a Quick-Open-created rule displays as **BOTH** in the Rules table —
  and Discovery's policy port form, which also writes firewall allow-rules per its own copy
  (`:400`, `:433`), hardcodes protocol tcp (`AdminDiscovery.tsx:428`).
- S2 · Three-way status-word split for `enabled`: the tile reads **"Firewall Enabled" / "Firewall
  Disabled"** with the *disabled* state in `tone="red"` (`:120`, `:122`), while every other page in the
  group renders a false flag as `neutral`/`yellow` (`gateways:409`, `traffic:215`, `dns:102`,
  `domains/[id]:436`). Loading is also a pill (`tone="neutral">Loading status…`, `:114`) and an error is a
  pill (`tone="red">Status error`, `:116`), so four different states of one control share one component
  width and two of them share one colour.
- S2 · Literal `\u2026` escapes render as text in three JSX attribute strings, because JSX string
  attributes do not process escape sequences: `placeholder="Select a node\u2026"` (`:110`),
  `label="Loading rules\u2026"` (`:162`), `label="Loading forwards\u2026"` (`:196`). The node picker
  therefore shows a visible backslash sequence, and both loading spinners read
  "Loading rules\u2026". Compare `AdminCrossnode.tsx:83` which uses the same component correctly
  (`label="Loading cross-node health…"`).
- S2 · `AddRuleModal` and `EditRuleModal` are a 45-line verbatim duplicate (`:314-359` vs `:361-406`) —
  same five fields, same options, same `AdminFormSection title="Rule Details"`, same missing validation —
  so any fix must be applied twice. `AddForwardModal` is a third near-copy (`:408-453`).
- S2 · These are the **only modals in the group wrapped in a `<form onSubmit>`** (`:340`, `:387`, `:436`,
  `:476`), so Enter submits here and does nothing on the other 11 routes. Two keyboard contracts.
- S2 · The node link is mislabelled: it renders the selected node's name as link text
  (`{selectedNode.name}`, `:143`) but points at the **fleet list**
  `href={`/admin/nodes`}` (`:140`), and it is a raw `<a>` (`:138`) rather than `Link`, so it discards the
  router cache. `ExternalLink` on an internal destination.
- S2 · The page is node-scoped but the node control sits in an unlabelled flex row (`:109-146`) outside
  any toolbar primitive; `AdminSelect` provides the label here (`:110`) — good — but the second and third
  items in that row are pills acting as status text, and the whole row is `flex flex-wrap gap-4` with no
  grouping against the tab strip below.
- S2 · Tenancy: correct node scoping and an explicit refusal to guess
  (`activeNodeId = nodeId` with the comment "falling back to nodes[0] silently targets a host the operator
  did choose", `:39-41`) — **this is the pattern `AdminAllocations.tsx:70`/`:165` violates**. No
  organization/project column on rules or forwards, and no indication whether a rule came from a workload
  allocation or was hand-created here, even though Discovery claims to reconcile allocated ports into
  this same table (`AdminDiscovery.tsx:400`, `:433`).
- S3 · Icon-only delete buttons correctly use `Trash2` but no `ariaLabel`, and swap to the bare string
  `"…"` while pending (`:274`, `:305`) — the accessible name disappears exactly when the button is
  disabled.
- S3 · Redundant double guards `!Array.isArray(rules) || rules.length === 0` (`:167`) then
  `Array.isArray(rules) &&` (`:173`); same pair at `:201`/`:207`.
- S3 · `AdminLoadingState` and `AdminErrorState` are used properly (`:162`, `:165`) but wrapped in
  `div.p-4` (`:161`, `:164`) rather than the card's own padding, so the loading box is inset
  inconsistently against Gateways' unwrapped usage (`gateways/page.tsx:362`).
- Good and worth copying: `useConfirm` with a real blast-radius description on both destructive row
  actions (`:273`, `:304`), `AdminTable` with `label` (`:170`, `:204`), `role="alert"`-free but
  retry-bearing error states, and honest `—` fallbacks for absent rule fields (`:265-269`).

### DNS Providers — `/admin/dns`
- files: `app/admin/dns/page.tsx` (194 lines), `lib/api/dns.ts`
- frame: `AdminPageLayout` + `SectionHeader` + `AdminTabs` + `Card`/`CardHeader` + `AdminTable` (`:78-79`, `:94`)
- title: `"DNS Providers"` vs registry `"DNS Providers"` → **MATCH** (`:80`)
- description: close paraphrase — registry "DNS providers used for DNS-01 challenges"
  (`admin-registry.ts:176`); page adds "Verify a provider, then set a default for issuance." (`:81`).
  Acceptable, but it is still hand-authored copy, not the registry string.
- icon: registry `GlobeGridIcon` (`:176`); header renders none. `CardHeader` uses `Globe` (`:89`) — which
  is the **Domains** registry icon (`:171`) — and `ShieldCheck` (`:121`), which is the registry icon for
  `Roles & Permissions` (`:211`) and `Zero-Downtime Releases` (`:146`). Three nav glyphs reused inside one
  secondary page.
- S1 · **Credential secrets are rendered in plaintext.** `Input` accepts a `type` prop
  (`components/admin/admin-ui.tsx:386`, default `"text"`) and the field descriptor carries a real
  `type` (`lib/api/dns.ts:22`), but the create form only uses it to decide font:
  `mono={field.type !== "password"}` (`:160`). `type` is never forwarded, so every API token, secret key
  and account key the operator pastes into `CF_DNS_API_TOKEN` (`:173`) is displayed in cleartext, is
  captured by password managers, and is readable over a shoulder or in a screen share. The user-facing
  promise at `:149` — "Credentials are stored encrypted" — is about storage, not display, so the page
  actively implies safety it does not provide.
- S1 · A **broken credential is presented as valid.** The `Verified` column reads the persisted flag only:
  `p.verified ? <Pill tone="green">Verified</Pill> : <Pill tone="yellow">Unverified</Pill>` (`:102`).
  There is no per-provider last-verification timestamp, no failure reason, and `verifyMut`'s success
  handler (`:51`) says "Provider verified" without surfacing `result` content. Worse, `verifyMut` and
  `defaultMut` are **single shared mutations**, so `disabled={verifyMut.isPending}` (`:106`) disables the
  Verify button on every row at once, and `isPending` on the wrong row means an operator cannot tell
  which provider is being probed. A `404`/`405` "route not wired" response would toast a failure, but a
  `200` with `ok:false` throws a synthetic message (`:48`) that reads as an opaque server fault.
- S2 · The custom-credentials escape hatch has **no parse feedback**: `key=value` textarea re-parses into
  a map on every keystroke (`:174-182`) and silently drops any line without `=` (`:179`). The operator
  can type a whole credential block, see nothing change, and submit an empty `creds`.
- S2 · Create validation gates only `name` and `providerType` (`:188`); `field.required` is rendered as a
  literal `*` in the label (`:156`) but is **never enforced**, so a provider can be created with zero
  credentials and then shown as a row the operator must Discover why it fails to verify.
- S2 · Raw `border-white/[0.06]` / `bg-white/[0.04]` / `text-[11px] font-mono text-slate-300` chips
  (`:127-133`), `text-slate-400`/`text-slate-100` inputs (`:171`), `text-sm text-slate-400` loading text
  (`:90`, `:122`) — plus a loading string that leaks the implementation: "Loading configured providers
  via fetchDnsProviders…" (`:90`).
- S2 · Two divergent loading/error idioms in one file: `configured` renders a bespoke red box
  (`:91`, `border-red-500/20 bg-red-950/10 text-red-200`) while `supported` renders bare
  `text-red-300` text with no retry (`:123`). Neither uses `AdminErrorState` (`:517 of admin-ui`), and
  `AdminLoadingState` is imported-adjacent but unused here while Gateways uses it
  (`gateways/page.tsx:362`).
- S2 · Inline action row is `flex justify-end gap-1.5` of three `Btn`s (`:105-109`), with the destructive
  delete carrying only a `Trash2` glyph (`:108`) and no `ariaLabel` — `Btn` supports `ariaLabel`
  (`components/admin/admin-ui.tsx:368`) and it is not passed. Icon-only destructive button with no
  accessible name (a11y K).
- S3 · "Configured" / "Supported Types" tabs (`:85`) — the second tab is a read-only reference gallery,
  so the `AdminTabs` strip mixes a data view with a documentation view on the same level.
- S3 · `supportedQuery` is enabled by `tab === "supported" || showCreate` (`:29`) — an unvisited-tab
  dependency; opening the modal on an unloaded gallery renders `AdminSelect` with an empty option list
  and no explanation (`:148`).

### ACME Accounts — `/admin/acme`
- files: `app/admin/acme/page.tsx` (93 lines), `lib/api/acme.ts`
- frame: `AdminPageLayout` + `SectionHeader` + `Card`/`CardHeader` + raw table (`:34-41`)
- title: `"ACME / Let's Encrypt"` vs registry `"ACME Accounts"` → **MISMATCH** (`:36`). Also bakes a
  vendor name into the H1, and the registry `keywords` already carry "letsencrypt"
  (`admin-registry.ts:177`), so the sidebar row and the page heading disagree by design.
- description: matches registry closely (`:37` vs `admin-registry.ts:177`), extended with "plus DNS
  accounts for DNS-01 challenges" — which is the second DNS Providers page living on this route.
- icon: registry `KeyRound` (`:177`); header renders none and both `CardHeader`s use `Shield`
  (`:41`, `:69`) — Firewall's registry glyph (`:174`). `KeyRound` is *also* the icon for OAuth Clients
  (`:214`) and Vault (`:218`), so the group's only credential page shows neither its own icon nor a
  distinct one.
- S1 · The card header prints a **hard zero for a failed query**:
  `{accountsQuery.data?.length ?? 0} accounts` (`:41`). `accountsQuery` is `retry: false` (`:19`), so on
  the first failure the error box renders (`:43`) *and* the header still asserts "0 accounts". AGENTS.md:
  not-reported is not zero.
- S1 · `DNSProvider.createdAt`/`caUrl`/`email` are trusted unconditionally:
  `new Date(d.createdAt).toLocaleDateString()` with no guard (`:75`) while the sibling table on the same
  page guards the identical field with `? … : "—"` (`:55`). A missing timestamp renders `Invalid Date`
  instead of unknown.
- S2 · This page duplicates the DNS Providers list as a read-only second card (`:68-78`) — same entities,
  different columns (`{d.name} · {d.provider}` at `:75` vs the six-column table at
  `dns/page.tsx:95`), and it uses `p.provider` where the DNS page uses the
  `p.providerType ?? p.provider` fallback (`dns/page.tsx:100`). Two spellings of the same field, one of
  which can render `undefined`.
- S2 · No `Verified`/health concept for an ACME account at all, even though account validity is the
  precondition for the whole Certificates flow. The table's only status column is
  `Default` (`:54`) rendered `<Pill tone="green">default</Pill>` / `<Pill>—</Pill>` — a *dash inside a
  pill* as the negative state, which is neither `neutral` nor `unknown`.
- S2 · Raw-table variant #3: same `text-[10px] uppercase tracking-widest` header row as Domains and
  Certificates but a **different colour** (`text-slate-400` at `:48` vs `text-slate-500` at
  `domains/page.tsx:158` and `certificates/page.tsx:125`), and a footer band
  `border-white/[0.06] bg-white/[0.015] text-[11px]` (`:63`) that exists on no other page.
- S2 · `Loading…` (`:70`) as a full loading state for the DNS card, and the DNS card's error is bare
  `text-amber-300` text (`:71`) while the ACME card's error is `AdminErrorState` (`:43`) — amber vs red
  for the same "request failed" condition on one screen.
- S2 · Create form: `Email *` label carries the asterisk (`:83`) but validation is only `!email`
  (`:87`) — no email format check, and `CA URL` (`:84`) is a free-text field with no URL validation, so a
  typo produces an ACME account that cannot be diagnosed from this page (no verify action exists).
- S3 · Delete confirm omits a `description` (`:56`), unlike every other destructive confirm in the group
  which explains the blast radius (`domains/page.tsx:208`, `certificates/page.tsx:173`,
  `dns/page.tsx:108`). Deleting the ACME account that issuance depends on gets the least warning.
- S3 · `Input` used for `caUrl` (`:84`) is not `mono`, so a URL renders in Manrope while the same page's
  email column renders `font-mono` (`:52`) and the CA column renders **sans + truncated**
  (`text-xs … truncate max-w-[200px]`, `:53`) — three treatments of the URL concept on one route (see
  scope pattern #1).

### Endpoints — `/admin/endpoints`
- files: `app/admin/endpoints/page.tsx` (12-line wrapper), `components/admin/AdminEndpoints.tsx` (196 lines)
- frame: wrapper adds `AdminPageLayout` (`app/admin/endpoints/page.tsx:8`) around a component whose own
  root is `<div className="space-y-6">` (`AdminEndpoints.tsx:75`) → **nested frames, two spacing scales
  (space-y-5 inside space-y-6)**. Inside it: `SectionHeader` + `Card`/`CardHeader` + raw table.
- title: `"Endpoints"` vs registry `"Endpoints"` → **MATCH** (`:77`)
- description: **contradicts the registry, and the registry contradicts the table.** Registry: "Public
  endpoint inventory across the fleet" (`admin-registry.ts:178`). Page: "Logical endpoint inventory and
  beacon groupings for traffic routing and network ingress." (`:78`). The rendered table is
  Name / Type / Connection / Status / Reachable / Version where Type is
  `docker | swarm | kubernetes | edge` (`:170-175`) — i.e. **container-runtime API endpoints**, not
  public service endpoints. An operator searching "endpoint" for an ingress address gets a Docker host
  list, from a sidebar row that promises "Public endpoint".
- icon: registry `EndpointPlugIcon` (`:178`) never rendered; `CardHeader` uses `Box` (`:86`) and the
  Gateways page links here with a `Box` icon too (`gateways/page.tsx:352`). `Box` is not a registry glyph
  anywhere — it is the ad-hoc name for `Boxes`, which the registry gives to Image Registries, Kubernetes
  **and** Incus (`admin-registry.ts:130, 164, 165`). Three nav entries plus this page on one glyph.
- S1 · The detail page **reports `0` for attached nodes while its query is still running, and swallows
  three of six query failures as empty states.** `AdminEndpointDetail.tsx:102` renders
  `{nodes.length}` from `nodesQuery.data ?? []` (`:45`) with no loading/error branch anywhere, so during
  load the KPI card reads "0 attached nodes" — not-reported is not zero. On the same page the Access
  Policies (`:215-216`), Attached Nodes (`:190-191`) and Health History (`:242-243`) cards each test only
  `!Array.isArray(x) || x.length === 0` and render `EmptyState "No … configured."`: `policiesQuery`,
  `nodesQuery` and `healthQuery` `.isError` are never read, so three failed requests present as three
  confidently empty sections on a page whose header shows the endpoint as `online`.
- S1 · **Every unknown status renders as an unstyled chip containing the raw wire word.**
  `Pill className={STATUS_COLORS[ep.status] ?? ""}` (`AdminEndpoints.tsx:132`, also `:128`) — the
  `className` path bypasses `tone` entirely (`admin-ui.tsx:43-49`), so `?? ""` yields a borderless,
  colourless chip reading `stale`, `error` or `""`. `AdminEndpointDetail.tsx:84`, `:173`, `:204`, `:259`
  repeat the idiom against a **second, visually different** `STATUS_COLORS` map (`:15-21`,
  `border-emerald-500/30 bg-emerald-900/30 text-emerald-300`) for the same five keys — so an endpoint's
  `online` chip looks different on its list row than on its own detail page.
  `AdminDiscovery.tsx:61-66` carries a comment deleting exactly this pattern ("Pill defaults to the
  neutral chip, so an endpoint health value this table did not know rendered as a confident
  'inactive'"); the two files reached opposite conclusions about the same defect, and both hand-written
  maps duplicate `discoveryStatusTone` / `resolveTone` (`lib/api/status.ts`, `components/ui/forge/status.ts:139-190`).
- S1 · `Reachable` is a two-value column with no unknown: `ep.reachable ? "Yes" : "No"` in raw
  `text-green-400`/`text-red-400` (`:135-139`), while the detail page spells the same field
  "Reachable"/"Unreachable" (`AdminEndpointDetail.tsx:86`) and the diagnostics card a third time
  (`:150-152`). An un-probed endpoint is indistinguishable from a confirmed-unreachable one, and a red
  "No" reads as an outage.
- S1 · The create form has **no validation at all** — `disabled={createMut.isPending}` (`:189`) with
  `Name *` labelled required (`:167`), so an empty name posts `name: ""`. The `URL` field (`:182`,
  placeholder `https://docker.example.com:2375`) accepts any string with no scheme/host/port check, and
  it is the field class this group's cross-tenant env-var resolution bug travelled through. No
  conditional either: `URL` is shown and posted for `connectionMode: edge` (`:179`) where it is
  meaningless, and `endpointType: edge` (`:174`) is offered beside it with no stated relation.
- S2 · The registry says "across the fleet" and the page renders **no search, no filter, no sort, no
  pagination, no node/org scope** — one unbounded table (`:105-154`). Compare Allocations (search + 2
  filters + 3 sort keys, `AdminAllocations.tsx:181-198`) and Discovery (3 filters,
  `AdminDiscovery.tsx:152-158`). Same group, same "inventory" claim.
- S2 · Second INFRA banner with route paths as copy: `AdminEndpoints.tsx:81-83` names
  `Services/Traffic/LB` (there is no `Services` route in the registry), `mTLS` (moved to Access,
  `admin-registry.ts:217`) and prints a literal Next.js dynamic segment `/admin/nodes/[id]` (`:82`) as
  the operator's escape hatch. Structurally identical to `AdminDiscovery.tsx:125-127`, and the two
  banners disagree: this one calls Discovery "Advanced", Discovery's calls Endpoints "primary" and lists
  a different set.
- S2 · Icon-only destructive control written as a raw `<button title="Delete">` (`:143-149`) — no
  accessible name, bespoke `hover:bg-red-500/10 hover:text-red-400` styling, and `deleteMut` carries
  **no `disabled` guard** (`:68-72`), so a double-click issues two deletes.
- S2 · Fifth table variant in the group: raw `<table>` with `border-slate-700/50 … text-xs uppercase
  tracking-wider text-slate-500` header (`:107`) and `border-slate-800/50 hover:bg-slate-800/30` rows
  (`:119`) — solid **slate** borders, where every other page uses `border-white/[0.06]` or
  `border-[var(--line)]`.
- S2 · Tenancy: a credential-bearing fleet object with no organization, project, owner or node-count
  column; the only ownership data on the route is the detail page's Access Policies table, which prints
  `p.principalId` as a raw mono UUID (`AdminEndpointDetail.tsx:230`) with no name resolution and no link.
- S3 · `ExternalLink` used for an **internal** `Link` navigation (`:123`) — the glyph that means "leaves
  the app" in `DashActionButton` (`dashboard-cards.tsx:83`) and `domains/page.tsx:129`.
- S3 · Bare `Loading` string, no skeleton (`:88`).

### Endpoints detail — `/admin/endpoints/[id]`
- files: `app/admin/endpoints/[id]/page.tsx` (12-line wrapper), `components/admin/AdminEndpointDetail.tsx` (276 lines)
- frame: same double frame as the list (`page.tsx:8` + `:74` `space-y-6`); `SectionHeader` + four KPI
  `Card`s + five raw tables.
- title: `{ep.name}` (`:76`) — a resource name is right for a detail route, but the page **reads as a
  peer, not a child**: no eyebrow, no `breadcrumbs`, no "Endpoint" kicker, and `SectionHeader`
  deliberately renders no trail inside `/admin` (`admin-ui.tsx:201-219`), so the H1 is a bare
  "Production Cluster" stating nothing about the object type. The only type signal is the fallback
  subtitle `${ep.endpointType} · ${ep.connectionMode}` (`:77`), which **vanishes as soon as a description
  exists** — the subtitle's meaning depends on the data.
- S1 · Any error other than 403 is reported as **"Endpoint not found"** (`:59-66`) — 500, timeout and
  upstream failure all render the same missing-resource message plus a back button. An operator will
  conclude the endpoint was deleted.
- S1 · `formatMB(undefined)` renders **"undefined MB"**: `mb >= 1024` is false for `undefined` so the
  function falls to `mb + " MB"` (`:273-276`), and the Inventory Summary calls it unguarded on
  `usedMemoryMb`/`totalMemoryMb`/`usedDiskMb`/`totalDiskMb` (`:126-132`).
- S1 · `(r.healthScore * 100).toFixed(0)}%` (`:260`) renders **`NaN%`** for a record with no score, in a
  column whose neighbours do guard absence (`:109`, `:137` use `?? "-"`).
- S2 · `new Date(diag.checkedAt).toLocaleString()` (`:154`) and
  `new Date(r.observedAt).toLocaleString()` (`:258`) render **`Invalid Date`** when absent.
- S2 · Two cards **disappear** when their query fails: `{inv && …}` (`:116`) and `{diag && …}` (`:145`)
  render nothing at all on error — no error state, no empty state, no "unavailable" line. Absence of a
  section is indistinguishable from absence of the capability; Discovery at least names the reason
  (`AdminDiscovery.tsx:49-51`).
- S2 · Six queries fire on mount (`:29-34`) for a page whose first screen is four tiles; `health` pulls
  20 records (`:33`) into an unpaginated table (`:256-264`).
- S2 · KPI "Containers" card (`:107`) is sub-labelled "servers running" (`:110`) and valued from
  `inv?.totalServers` (`:109`) — three nouns for one number — while the Inventory Summary lists `Servers`
  (`:121`) and `Containers / Images / Volumes` (`:135`) as separate figures.
- S3 · `Loading` as a full-page return (`:49`) — the header disappears during load, so the H1 arrives
  late. `Allocated CPU` prints a bare number with no unit (`:176`) beside `formatMB` mem/disk
  (`:175`, `:177`). `Back` is a `router.push` `Btn` (`:78`, `:55`, `:64`) while `AdminBackButton`
  (`admin-ui.tsx:276-278`) and `SectionHeader backAction` (used by `domains/[id]:164`) both exist — three
  back implementations in the group.

### IP Allocations — `/admin/allocations`
- files: `app/admin/allocations/page.tsx` (7-line wrapper, **no `AdminPageLayout`** — `:6`),
  `components/admin/AdminAllocations.tsx` (396 lines)
- frame: root is a bare `<div>` (`:161`) with `SectionHeader`, `StatsRow`, a bespoke filter grid, one
  `Card`/`CardHeader` and a raw table. **The only route in the group with no `max-w-page` measure**, so it
  renders edge-to-edge while the other eleven are constrained — and the only one that redefines its own
  `selectStyle` and `thClass` (`:156`, `:158`).
- title: `"Allocations"` vs registry `"IP Allocations"` → **MISMATCH** (`:163`)
- description: rewritten — registry "Network ports and IP bindings assigned to workloads"
  (`admin-registry.ts:179`); page "IP:port bindings available to servers." (`:164`). "workloads" →
  "servers" imports the Workloads group's vocabulary for a value that is literally a server name
  (`:247`, `:279`).
- icon: registry `Cable` (`:179`) never rendered. `StatsRow` uses `Network`/`Server`/`Globe`
  (`:176-178`) and the card uses `Network` (`:210`) — `Globe` is Domains' glyph (`:171`), `Network` is
  NetBird's and Placement Affinity's (`:167`, `:203`).
- S1 · **All three KPI tiles read 0 while the list loads.** `allocations = allocationsQuery.data ??
  EMPTY_ALLOCATIONS` (`:67`) feeds `StatsRow` `Total: allocations.length` / `In use: used.length` /
  `Free: free.length` (`:175-179`, computed `:95-96`) while the table below renders "Loading
  allocations…" (`:213`). The page states "Total 0" and "still loading" in one screen.
- S1 · **`EmptyState` is used as the loading state, and its default heading is "Nothing to show"**:
  `EmptyState` defaults `title` to `"Nothing to show"` (`admin-ui.tsx:617`) and `:213` passes only
  `icon` + `message="Loading allocations…"`. During every fetch the primary card's headline is a
  no-results message. Identical at `:222` ("No allocations found." under a "Nothing to show" title).
- S1 · **The create form silently picks a node the operator never chose.** Opening the modal pre-fills
  `setNodeId(nodes[0]?.id ?? "")` (`:165`) and the mutation re-falls-back at submit
  (`nodeId: nodeId || nodes[0]?.id || ""`, `:70`). This is the AGENTS.md pitfall verbatim — "If a request
  omits the node it refers to, reject it; do not pick the first one that happens to have credentials" —
  and binding a public IP:port on the wrong host is precisely the request that must not be guessed.
  `AdminFirewall.tsx:39-41` implements the correct rule and documents it in the same group.
- S2 · Security-posture default: the IP field is pre-filled `0.0.0.0` (`:57`) while the help text
  directly beneath warns "or a specific IP to limit exposure" (`:351`). Widest exposure is the initial
  value of a firewall-adjacent control.
- S2 · Validation is **the best in the group and still inconsistent**: `validateCreateInput` (`:17-35`)
  does real IPv4 octet checks, port ranges, ordering and a 2 000-port cap, but the IPv6 branch
  (`address.includes(":") && /^[0-9a-fA-F:.]+$/`, `:20`) accepts `::`, `1:2:3` and `ffff:`. Validation
  runs **only inside `onConfirm`** (`:383-384`) and is absent from the `disabled` expression (`:388`), so
  Create stays enabled with a malformed address and the error appears one click late. The Edit-alias
  modal validates nothing (`:321-323`).
- S2 · Local `selectStyle` (`:156`) shadows the exported canonical one (`admin-ui.tsx:500`) with a
  different control, and its focus treatment is `focus:border-red-400/70 focus:ring-red-500/15` —
  **a red focus ring on a non-danger filter control** — plus `bg-surface-card-header` (`:156`, `:158`,
  `:303`), which is not a documented `app/globals.css` token, and `text-red-500 focus:ring-red-500/30` on
  both checkboxes (`:236`, `:263`). The two filter `<select>`s (`:183`, `:187`) have **no label and no
  `aria-label`** — only option text — and the create form's Node and Protocol selects (`:340`, `:363`)
  use unassociated sibling `<label>`s (`:339`, `:362`): four unlabelled selects on one route.
- S2 · "Export" copies CSV to the clipboard (`:145-148`) under a `Copy` icon (`:196`) — the label
  promises a file, the affordance is a paste buffer, the toast says "Copied to clipboard". The CSV is
  built by raw string interpolation with **no quote escaping** (`:146`), so a node or server name
  containing `"` corrupts it. It exports `filtered` (search-scoped) while the only scope hint,
  `(N of M)`, is rendered **only when a search is active** (`:209`).
- S2 · IP:port is the group's best cell — mono, colour-separated, protocol chip, container-port arrow
  (`:267-274`) — and it still has **no copy affordance**, while its own column header spells the concept
  `IP : Port / Protocol` with spaces (`:242`) that the cell does not (`:268-270`). The Alias column is
  `max-w-[200px] truncate` with no `title` (`:276`), and the modal's Alias placeholder is a hostname
  (`:311` "minecraft.example.com") while the column is called "Alias" — the two names for one value never
  co-occur.
- S2 · Sticky header (`thClass`, `:158`) sits inside `overflow-x-auto` (`:224`) with no vertical scroll
  container and no `min-w-` on the table, so `sticky top-0` is inert; the sortable `<th>`s (`:228-248`)
  are `cursor-pointer` cells with **no keyboard handler, no `role`, no `aria-sort`** — sorting is
  mouse-only — and `SortIcon` renders `opacity-0 group-hover:opacity-40` (`:152`), so the active sort is
  undisclosed until hover.
- S2 · Tenancy: the `Server` column renders a bare name or a `free` pill (`:278-281`) with no link to the
  server, no org column and no scoping control — an allocation bound to another organization's server is
  visible with no attribution.
- S3 · Bulk delete correctly restricts to free rows (`:118-120`, `:136-142`) and confirms — one of two
  pages in the group doing both. Row edit is an icon-only `Edit3` with no `ariaLabel` (`:284`); the
  select-all checkbox is `disabled` with no explanation (`:238`); the checkbox column sits between Node
  and IP (`:231-240`).

### Cross-Node Routing — `/admin/crossnode`
- files: `app/admin/crossnode/page.tsx` (7-line wrapper, bare), `components/admin/AdminCrossnode.tsx` (347 lines)
- frame: `AdminPageLayout` (`:54`) + `SectionHeader` + `AdminTabs` + `Card`/`CardHeader`; **no table
  anywhere** — the content is four `<pre>` JSON dumps.
- title: `"Cross-Node"` vs registry `"Cross-Node Routing"` → **MISMATCH** (`:57`) — the short form is a
  different noun (a thing vs a mechanism).
- description: **carries a source-file citation with a line number** — "11 routes from
  handlers_crossnode.go:12. Health, resolve, cache, ingress sync and cleanup." (`:58`) vs registry
  "Cross-node resolver cache and ingress synchronisation" (`admin-registry.ts:183`).
- icon: registry `Repeat` (`:183`) never rendered; card icons are `Activity`/`Globe`/`Trash2`/`Server`/
  `Shield`/`Network` (`:80`, `:180`, `:206`, `:217`, `:231`, `:295`) — `Globe` is Domains', `Shield` is
  Firewall's, and `Trash2` serves as a **card** icon (`:206`) as well as a delete glyph (`:210`, `:302`).
- S1 · **An enabled button that does nothing.** The Cache TTL card's own body copy says: "Note: current
  backend is a stub — it parses but does not persist TTL. Wired for future impl." (`:225`), yet
  `Set TTL` is enabled (`:220`), posts (`:166`) and renders the server message in a green success panel
  (`:223`). The operator sets a resolver TTL, sees success, nothing changes. This is the rubric's
  "enabled-and-lying" case, and the group already contains the correct treatment
  (`AdminDiscovery.tsx:49-51`, `:316-317`).
- S1 · **Absent health data is rendered as failure.** `healthQ` has `retry: false` (`:44`) and the panel
  gates only on `isLoading`/`isError` (`:82-86`); every value below reads `h?.resolver_available ? … : …`
  (`:91-94`, `:99-102`), so a successful-but-empty body — or any outcome landing in neither branch —
  renders a red dot, "Unavailable" and an `inactive`/`red` pill. A missing reading is reported as a downed
  resolver. The pair also disagrees with itself: `resolver_available` false → `tone="red"` (`:94`) but
  `ingress_sync_available` false → `tone="yellow"` (`:102`) for the identical `false`.
- S1 · The **Describe** tool truncates its own answer to 80 characters into a toast title
  (`data.description.slice(0, 80)`, `:173`) with no full view anywhere — the card exists to explain why a
  host:port is unreachable, and the explanation is cut mid-sentence. The same result is *also* dumped as
  raw JSON (`:241-243`), so the page both hides and dumps the value.
- S2 · **The page is an API inspector wearing product chrome.** Every `CardHeader` title is an HTTP route
  (`:80`, `:180`, `:206`, `:217`, `:231`), the two action buttons are labelled with HTTP verbs
  (`:299` `POST /ingress/sync`, `:302` `POST /ingress/cleanup`), field labels are raw snake_case query
  params (`:183-184`, `:234-235`), help text explains status codes (`:199`), permission scopes
  (`:208` "Requires `routing.write`", `:342` "admin : routing.write … rate-limited by the mutation
  limiter") and Go nil-checks (`:135` "`resolver != nil`, `ingressSync != nil`"), and Rules/Policies/stats
  are unformatted `JSON.stringify(…, null, 2)` dumps (`:311`, `:317`, `:326`, `:332`) in `max-h-48`/
  `max-h-32` scroll boxes. Nothing on this route is a list, a row or a column.
- S2 · This is the group's second **resolve** tool: Cross-Node resolves `server_id`/`node_id` → host
  (`:180-201`), Discovery resolves `service`+`tenant` → endpoints (`AdminDiscovery.tsx:541-549`), and a
  third resolution path is asserted in the Gateways topology caption (`gateways/page.tsx:232-234`) — with
  the Discovery banner claiming "Discovery is the dynamic resolver behind Endpoints/Domains"
  (`AdminDiscovery.tsx:126`). Two enabled tools, same verb, different inputs, no cross-reference.
- S2 · `refetchInterval: 30_000` (`:43`) polls health every 30s and the page **never says so** — no
  `FreshnessBadge`, no "updated" text — so a frozen tile looks live. The only route in the group with
  automatic refresh, and per pattern 5, the group has zero freshness affordances.
- S2 · Best result handling in the group, and it is thrown away: `syncMut` distinguishes skipped /
  partial / no-result and reports observed rules, groups and healthy backends (`:261-279`) — but only
  inside a transient toast (`:266-274`), with no persistent panel, so the one honest account of an
  ingress reconcile disappears with the toast.
- S2 · Raw colour despite using tokens elsewhere: `bg-emerald-500`/`bg-red-500` dots (`:91`, `:99`),
  `border-emerald-500/20 bg-emerald-500/10 text-emerald-200` panels (`:193`, `:212`, `:223`),
  `border-red-500/30 bg-red-500/10 text-red-200` (`:197`, `:213`, `:224`, `:245`), `text-red-300`
  (`:325`, `:331`), `bg-white/[0.06]` (`:107`, `:341`), and `text-[11px]` mixed into the same class
  strings as `text-[var(--text-subtle)]` (`:89`, `:107`).
- S3 · `AdminTabs` is typed `Array<{ id: string; label: string }>` (`:47`) while the page's `Tab` union is
  `health | resolver | ingress` (`:34`) and `onChange` casts (`:66`); Gateways types the same thing
  properly (`gateways/page.tsx:296`).
- S3 · `OfflineBanner` here (`:55`) renders above the header, matching Gateways
  (`gateways/page.tsx:305`) but not `domains/[id]:160` (after the header) or
  `app/admin/discovery/page.tsx:11` (after the component) — three mount positions for one component.

### Load Balancer — `/admin/load-balancer`
- files: `app/admin/load-balancer/page.tsx` (262 lines)
- frame: `AdminPageLayout` + `SectionHeader` + `Card`/`CardHeader` + local `MetricCard`/`Loading`
  helpers (`:162-163`, `:240-244`)
- title: `"Load Balancer"` vs registry `"Load Balancer"` → **MATCH** (`:164`)
- description: **narrower than registry**. Registry: "Target groups and traffic distribution"
  (`admin-registry.ts:180`); page: "Manage target groups and traffic routing for game servers." (`:165`)
  — it scopes a fleet-wide LB to game servers, which is the `Workloads` group's vocabulary, not this
  group's.
- icon: registry `LoadBalancerSplitIcon` (`:180`); header renders none. Cards use `GanttChart` (`:185`),
  which is **also** the icon Traffic uses for its Route Rules card (`traffic/page.tsx:180`) and is the
  icon `Target`/`HeartPulse`/`Network`/`Zap` mix around (`:177-179`, `:207`).
- S1 · **The operator can hand-set a target's health to `healthy`.** `TargetRow` exposes a `<select>`
  whose options are `Healthy`/`Draining`/`Unhealthy` and PATCHes `status` directly
  (`:236-239` mutation, `:248` control). A target that failed its probe can therefore be marked healthy
  from the UI, and the row then draws the green dot (`:247`) and feeds the page-level `Healthy` metric
  (`:179`). This is the exact inverse of the project rule "a stale reading is not a healthy one", and it
  is the only place in the group where a measured value is writable. The Gateways page renders the same
  `t.status` read-only (`gateways/page.tsx:474`), so the two pages disagree about whether health is data
  or an opinion.
- S1 · Metric tiles **silently substitute a client-side count for the server metric** and then still
  label it as the server's number: `metricsQuery.data?.groups ?? groups.length` (`:177`),
  `?? allTargets.length` (`:178`), `?? allTargets.filter(… "healthy").length` (`:179`). When
  `/admin/load-balancer/metrics` fails, `Healthy` becomes a locally derived fraction while the tile's
  emerald styling (`tone="text-emerald-400"`, `:179`) is unchanged — the fallback is invisible to anyone
  not reading the debug line beneath.
- S1 · `selectedGroup.targets.map(…)` (`:210`) is reached whenever `targets?.length === 0` is **false**,
  which includes `targets === undefined`. The page's own type declares `targets: Target[]` (`:30`) but
  the sibling Gateways page defensively writes `s.targets ?? []` four times (`gateways/page.tsx:91, 215,
  221, 227`) for the same wire object, so the field is known to be absent in practice. On a group
  without a `targets` array this is a render-time crash, not a degraded state.
- S2 · Error presented as empty: `groupError ? <EmptyState … "Target groups could not be loaded.">`
  (`:187`). The red alert above (`:169-174`) does render, but the primary content area tells the
  operator there are no groups. `filtered.length === 0` then renders `No target groups.` (`:187`) for
  the same case when the search box is empty.
- S2 · Raw HTTP contract as product copy in three places: `GET /admin/load-balancer/metrics — loading…/
  failed to load metrics (backend may be down)/…` (`:181`), `GET
  /admin/load-balancer/groups/:id/next → Next target:` (`:213`), `Group Detail — GET
  /admin/load-balancer/groups/:id` (`:216`). The `Group Detail` block is a JSON-ish key/value dump of
  fields already visible in the list (`id`, `name`, `algorithm`, `protocol :port`, `targets`,
  `createdAt` — `:219-224`), i.e. the right-hand card duplicates the left-hand card's content in a debug
  format. Traffic does the same thing (`traffic/page.tsx:332, 345, 394`).
- S2 · `Protocol` for a target group is a **free-text `Input`** with placeholder "tcp" (`:257`) while the
  LB's own algorithm field next to it is a proper `AdminSelect` (`:252`) and the `Target` type has no
  protocol at all (`:13-22`). The row then calls `group.protocol.toUpperCase()` (`:193`) and the Gateways
  page calls `s.protocol.toUpperCase()` (`gateways/page.tsx:453`) — an unvalidated free-text field is
  upper-cased and displayed as if it were an enum, and an empty value renders an empty string.
- S2 · "Add Target" demands **raw UUIDs**: `Server ID` placeholder "server UUID" and `Node ID` placeholder
  "node UUID (optional)" (`:261`), while `NodeViewCard` in the same group resolves the same node set from
  `useNodesQuery()` into a labelled select (`discovery:333`, `:524`). No picker, no validation of UUID
  shape, so a mistyped node id is submitted and only visible in the list afterwards.
- S2 · IP:port rendering: `{target.ip}:{target.port}` in `font-mono text-sm` (`:248`), and the test
  result as `{testResult.ip}:{testResult.port}` in `font-mono text-emerald-400` (`:213`), and the group
  row as `{group.protocol.toUpperCase()} :{group.port}` with a **space before the colon** (`:193`), and
  the detail dump as `{protocol} :{port}` (`:222`). Four spellings of the same value in one file, none
  with a copy affordance — compare `AdminAllocations.tsx:148` which is the only page in the group that
  implements copy-to-clipboard.
- S2 · Hand-rolled KPI tiles: local `MetricCard` (`:240-242`) rather than `AdminStatCard`/`ForgeMetric`
  (`components/admin/admin-ui.tsx:647-659`) which the reference pages use, and it renders its own
  `Card` with `p-4` (`:241`) instead of `CardHeader` + body, so the three tiles are cards with no
  header band on a page whose other cards all have one.
- S2 · `Loading({message})` local helper (`:244`) + ad-hoc `Loading groups...` (`:187`) + `Loading
  detail...` (`:217`) — three bespoke loading strings; `AdminLoadingState` is used by Gateways, Traffic
  uses none, Discovery uses raw text.
- S2 · Toolbar is a single `Create Target Group` button in the header (`:166`) plus an `Input` inside the
  card body (`:186`); no `AdminPageToolbar`, so search placement differs from every reference page.
- S3 · The `✕` dismiss control for the test result is a text glyph button with **no accessible name**
  (`:213`).
- S3 · `weight < 1` blocks submit (`:261`) but there is no upper bound, and `port < 1 || port > 65535`
  is validated (`:257`, `:261`) while `ip` is validated only for non-emptiness — the one genuinely
  parseable field in the group with no format check.
- S3 · No `updatedAt` is ever displayed although the type carries it (`:32`), and `createdAt` is shown
  only inside the debug dump (`:224`) as a raw ISO string.

### Traffic Policies — `/admin/traffic`
- files: `app/admin/traffic/page.tsx` (407 lines)
- frame: `AdminPageLayout` + `SectionHeader` + `AdminTabs` + `Card`/`CardHeader` + raw table (`:163-174`)
- title: `"Traffic Policies"` vs registry `"Traffic Policies"` → **MATCH** (`:165`) — **but the title
  describes only one of the two tabs**, and the page's default tab (`:54`) is `routes`, so the heading
  under the breadcrumb contradicts the first thing rendered. The registry href is `/admin/traffic`
  (`admin-registry.ts:181`) while the alias source `/admin/traffic-policies` exists in
  `ADMIN_ALIAS_ROUTES` (`:273`), so the label and the route already disagree.
- description: partial — registry "Route rules, rate limits and traffic shaping" (`:181`); page "Route
  rules and traffic policies for the API gateway." (`:166`). "traffic shaping" has no UI anywhere; the
  policy `type` enum is only `rate_limit | ip_whitelist | ip_blacklist | circuit_breaker` (`:26`,
  `:320-323`), so the registry promises a capability the page does not have.
- icon: registry `Route` (`:181`); header renders none; cards use `GanttChart` (`:180`) — the Load
  Balancer card glyph (`load-balancer/page.tsx:185`) — and `Shield` (`:238`), the **Firewall** registry
  icon (`:174`). Neither `Route` nor `SlidersHorizontal` (used for rate-limit rows at `:258`) is the
  nav glyph.
- S1 · **Policy `config` is a raw JSON string typed into a one-line `Input`**: `Input label="Config
  (JSON)"` (`:326`, `:360`) with placeholder `'{"requests_per_second": 100}'`. There is no per-type
  schema — `rate_limit`, `ip_whitelist`, `ip_blacklist` and `circuit_breaker` all get the same free box —
  and the JSON is only parsed inside `mutationFn` (`:116`, `:138`), so the `Create` button stays enabled
  for syntactically invalid config (`:338` gates only `!policyForm.name`) and the operator learns the
  shape by 400. Editing an existing policy re-serialises into the same single-line box
  (`:275` `JSON.stringify(p.config ?? {}, null, 2)`) where the `null, 2` newlines collapse into one
  visual line, so the value shown and the value submitted differ.
- S1 · `policyConfigError` is set by the mutation body and thrown as a sentinel
  (`:117`, `:139`), then suppressed from the toast by string-matching its message (`:126`, `:148`). If the
  backend ever returns the literal message "Invalid JSON config" for a different reason the user sees no
  error at all. It also never clears on the `Config` field's own `onChange` (`:326`), so a stale "Config
  must be valid JSON." sticks under a now-valid value until the next submit.
- S1 · Route `Target Group` is a **free-text field naming a Load Balancer entity** (`:396`, placeholder
  "prod-servers"). The LB page lists groups by `name` (`load-balancer/page.tsx:192`) but its `Target`
  rows key on `id`, and Gateways displays `targetHost`/`targetPort` for the same rules
  (`gateways/page.tsx:395`). So a route can point at a group that does not exist, nothing on this page
  can detect it, and the three pages disagree on what a route even contains (see Gateways S1).
- S2 · Raw HTTP contract as modal body copy in **both** modals, including a changelog note: `POST
  /admin/traffic/rules … backend now correctly uses PUT for updates` (`:394`), `POST
  /admin/traffic/policies — backend persists via trafficmanager.Service` (`:332`), `PUT
  /admin/traffic/policies/:id — wired to backend UpdateTrafficPolicy` (`:345`). These are the only
  "help" text in the create/edit flows.
- S2 · No route/domain field: Gateways' empty state instructs "Create via Traffic → Create Route
  (requires domain)" (`gateways/page.tsx:374`) and the routers table keys everything on `Host`
  (`gateways/page.tsx:381`, `:392`), but this page's route form collects only
  `path/targetGroup/priority/methods` (`:395-398`). One of the two pages is wrong about the product, and
  the operator following the instruction cannot complete the task.
- S2 · `Sync Routes` occupies the **sole header action slot** (`:167-171`) as a `tone="ghost"` button,
  while the actual primary create actions live in `CardHeader action` inside each tab (`:181-185`,
  `:239-243`). So the page-level toolbar has no create affordance, and clicking Sync gives no feedback
  beyond an invalidation (`:151-155`) — no count, no result, no freshness, and on success no toast at
  all (only `onError` toasts, `:154`), which violates "never report success for work not performed" in
  the other direction: an unverified sync looks identical to nothing happening.
- S2 · Loading is a raw `Loading routes...` / `Loading policies...` div (`:191`, `:249`) and
  `routesQuery.isError` / `policiesQuery.isError` are **never rendered** — a failed fetch falls through
  to `filteredRoutes.length === 0` → `EmptyState "No route rules configured."` (`:193`) / "No traffic
  policies configured." (`:251`). Both tabs report an error as an empty fleet.
- S2 · Enabled/disabled is rendered as `Active`/`Inactive` on routes (`:215`) and `Enabled`/`Disabled` on
  policies (`:274`) **on the same page**, both from an `enabled` boolean; plus a raw checkbox with a
  label wrapped but no hint of what "Enabled" does (`:329-331`, `:363-365`).
- S2 · Policy rows are a bespoke `divide-y` list with per-type lucide icons coloured in raw Tailwind
  (`text-amber-400`/`text-emerald-400`/`text-red-400`/`text-slate-400`, `:257-265`) while routes in the
  same file are a raw `<table>` with its own header styling (`:196-205`) — two row grammars one tab
  apart, and neither is the `AdminTable` DNS uses (`dns/page.tsx:94`).
- S2 · Policy row config is flattened with `Object.entries(p.config).map(([k,v]) => \`${k}: ${v}\`)`
  (`:269`) — nested objects render as `[object Object]`, and secret-ish keys (e.g. an allow-token) are
  printed unmasked in the list.
- S2 · Search state is local `useState` for both tabs (`:55`, `:62`) with two separate boxes; nothing
  persists across the tab switch or across navigation.
- S3 · Raw Tailwind throughout the table chrome: `border-white/[0.06]`, `divide-white/[0.04]`,
  `bg-white/[0.02]`, `text-[10px] uppercase tracking-widest text-slate-500` (`:198-209`), plus
  `border-white/10 bg-[var(--surface-input)]` selects with unassociated `<label>`s (`:314-324`,
  `:348-358`) — the Type `<select>` has **no accessible name** in either modal (a11y K).
- S3 · `Edit` buttons are bare text `Btn`s (`:219`, `:275`) with no icon, unlike the icon-led action rows
  on Domains/Certificates; and the row's delete is icon-only with no `ariaLabel` (`:220`, `:276`).
- S3 · `Array.isArray(filteredRoutes) &&` / `Array.isArray(filteredPolicies) &&` guards inside render
  (`:208`, `:254`) after `unwrapList` already normalised (`:66`, `:71`) — the same belt-and-braces
  leftover as Domains `:167` and Certificates `:135`.
- S3 · `priority` accepts any number (`:397`) with no min/max, and `methods` is comma-split with no
  upper-case or HTTP-verb validation (`:86`, `:97`), so `get,get,GET,foo` submits unchanged.

### Service Discovery — `/admin/discovery`
- files: `components/admin/AdminDiscovery.tsx` (563 lines) behind a 14-line `app/admin/discovery/page.tsx`
- frame: **hand-rolled wrapper** — the component's root is `<div className="space-y-6">` (`:115`), not
  `AdminPageLayout`; inside it, `SectionHeader` + `AdminTabs` + `Card`/`CardHeader` + raw tables. This is
  the only page in the group that abandons the frame at the root, and the one place a `max-w-page`
  measure is missing, so it renders full-bleed beside eleven constrained pages.
- title: `"Service Discovery"` vs registry `"Service Discovery"` → **MATCH** (`:117`)
- description: rewritten and 190 chars (`:118`) vs registry "Service discovery records and network
  policy" (`admin-registry.ts:182`). It bakes an SLA number into the subtitle ("reaped after 3m TTL")
  that is also hardcoded in the staleness logic (`:307`) and re-explained in a banner (`:130`) — three
  places, one constant.
- icon: registry `NetworkMeshNodesIcon` (`:182`); header renders none; the six tabs use `Server`, `Globe`,
  `Network`, `Shield`, `Heart`, `Activity` (`:188`, `:226`, `:394`, `:459`, `:515`) — `Globe` is the
  **Domains** registry icon (`:171`) and `Shield` is **Firewall**'s (`:174`).
- S1 · **A failed policy query renders "Loading policy…" forever.** `loading ? <div>Loading policy…</div>
  : !policy ? <div>Loading policy…</div>` (`:397`) — the `!policy` branch catches every non-loading,
  dataless outcome, which is exactly what an errored query is. The error strip at `:396` is rendered too,
  but the body of the card claims work is still in progress that has already stopped. The file's own
  comment at `:131-132` documents this precise bug for the reaper badge ("a failed reaper query used to
  render 'loading…' indefinitely, which tells the operator to wait for something that is never coming")
  and the identical defect was left in the policy card three lines later.
- S1 · An endpoint with **no heartbeat timestamp is reported as live, not stale**: `ageMs = Date.now() -
  new Date(ep.lastHeartbeat).getTime()` → `NaN` when `lastHeartbeat` is absent, `NaN > 180` is false, so
  `stale` is false (`:305-307`) and the amber `stale Ns` badge is suppressed — while the same row prints
  `—` in the `LastHeartbeat` column (`:314`) and shows whatever `status` the wire carries through
  `discoveryStatusTone` (`:313`). `discoveryStatusTone` maps a missing value to `unknown`
  (`lib/api/status.ts:12-16`), but a wire status of `active` with no heartbeat renders as a green pill
  with no staleness marker at all.
- S1 · Stale threshold is **hardcoded per page** (`:307`, `ageSec > 180`) and duplicated as prose (`:118`,
  `:130`); there is no shared `STALE_AFTER` constant. Certificates hardcodes a different one (`< 30`
  days, `certificates/page.tsx:89`), and `components/ui/forge/status.ts` already ships a
  `staleAfterMs`-driven freshness tone resolver that this slice never uses.
- S1 (capability gating, done right — one instance) · the disabled-with-reason controls at `:316-317`
  follow the project rule (`M`), but the reason reaches the operator only through `title={HEARTBEAT_UNROUTED}`
  / `title={STATUS_UPDATE_UNROUTED}` on a `disabled` button. Disabled buttons do not fire hover/focus
  tooltips in most browsers and are skipped by AT, so the three-sentence explanation is unreachable; the
  reason belongs as a visible line under the Actions column. `Btn` also supports `ariaLabel`
  (`admin-ui.tsx:368`) which is not used.
- S2 · **Internal development changelog is the visible section heading**, four times in one card:
  "Discovery Operations — wires orphaned manage funcs" (`:515`), "Fetch Endpoint by ID — wires
  fetchDiscoveryEndpoint" (`:531`), "Resolve Service — wires resolveDiscoveryService" (`:541`), "Reaper
  Stats — wires fetchReaperStats" (`:552`). Same idiom as `traffic:332` and `load-balancer:181`; the
  group has no rule against shipping route names and function names as UI copy.
- S2 · Raw route paths as navigation copy in the sub-header banner: `<code>/admin/endpoints</code> ·
  <code>/admin/domains</code> · <code>/admin/traffic /load-balancer</code>` (`:126`) — unclickable, and
  the third code span is a **malformed path** ("`/admin/traffic /load-balancer`" with a space, which
  resolves to nothing). The banner also names 6 sibling routes as "primary networking" vs "advanced",
  re-rankings the registry does not make (`admin-registry.ts:170-184` lists all twelve flat, seven
  `secondary`).
- S2 · `NETWORK_COLORS` (`:68-72`) hand-maps access classes to raw Tailwind
  (`bg-cyan-500/10 text-cyan-400`, `bg-purple-500/10 text-purple-400`, `bg-slate-500/10 text-slate-400`)
  and `AccessPill` bypasses the tone vocabulary entirely for known values (`:76-79`). This is precisely
  the pattern the comment at `:61-66` says was deleted from this file for `STATUS_COLORS`, re-implemented
  for the neighbouring concept. Unknown access does correctly use `tone="unknown"` (`:79`).
- S2 · Two vocabularies for the same reachability answer on one page: `reachable ? "ok" : "fail"` (`:348`)
  vs `reachable ? "reachable" : "unreachable …"` (`:469`), with raw `text-green-400`/`text-red-400`
  (`:469`) and `text-green-300`/`text-red-300` (`:475`) instead of `Pill`/tone.
- S2 · Address rendering, three more variants of the scope-wide problem: `{ep.address}:{ep.port}/{ep.protocol}`
  (`:311`), `{e.address}:{e.port}` (`:208`, `:344`), `{ev.address}:{ev.port} {ev.protocol}` (`:255`), and
  `{r.address}:{r.port}` inside a toast title (`:503`) and a comma-joined span (`:548`). The `/` separator
  at `:311` is invented here and appears nowhere else in the product.
- S2 · IDs truncated to 8 chars + ellipsis in four tables (`:253`, `:312`, `:344`, `:318` in the confirm
  title) with no full-value `title`, no link to a node page, and no copy button.
- S2 · Filter state (`filterService`, `filterNodeId`, `healthyOnly` — `:85-87`) and the active tab
  (`:84`) are component state; all six tabs and three filters reset on every visit.
- S2 · `PolicyCard` CIDR input has **no format validation**: `disabled={!cidr || !editable || …}`
  (`:411`) accepts `hello`, and the "Add CIDR"/"Port"/"Service" controls sit in a `flex` with `pt-6`
  nudges (`:409-412`, `:426-430`, `:534`, `:545`) to line up against labelled inputs — magic spacing
  instead of a form grid.
- S2 · `ReachabilityCard` takes `Source Node ID` / `Target Node ID` as free text (`:463-464`) while the
  very next card in the same tab resolves nodes by name from `useNodesQuery()` (`:333`) — the node picker
  exists and is not used.
- S3 · The `Endpoints` tab's count handling is genuinely honest — `count unavailable` / `loading…` /
  `N total` (`:151`) — and is the only such case in the slice; it should be the template, not the
  exception.
- S3 · Raw JSON dump in a `<pre className="bg-black/30">` (`:537`) as the product's "fetch endpoint"
  result view; `bg-black/30` is a raw palette value, not a token.
- S3 · `grid-cols-3` (`:462`) and `grid-cols-2` (`:522`) with no responsive fallback, so the three
  reachability inputs and the register form stay three-across on a narrow viewport and overflow.

---

### Domains detail — `/admin/domains/[id]` (not in the registry)
- files: `app/admin/domains/[id]/page.tsx` (505 lines)
- frame: `AdminPageLayout` + **`AdminPageHeader`** (`:161`) — the only page in the slice using the
  `AdminPageHeader` spelling rather than `SectionHeader` directly (they are the same component,
  `admin-ui.tsx:280-314`, so this is a call-style split, not a frame split).
- title: `{domain?.hostname ?? id}` (`:162`) — a hostname as H1 with no object-type kicker; the
  `description` interpolates the raw record ID into the subtitle sentence
  (`"Proxy domain ${id} — per-domain security headers, certificate, and gateway routing"`, `:163`).
- S1 · **The detail page is a different resource from its list page, and is not reachable from it.**
  `/admin/domains` manages a `DomainRecord` typed `{serverId, domain, wildcard, verified, verifiedAt,
  verificationToken}` (`app/admin/domains/page.tsx:13-22`) fetched via `fetchServerDomains`
  (`domains/page.tsx:8`). `/admin/domains/[id]` fetches `fetchAdminProxyDomain(id)` (`:63`) into a
  `ProxyDomain` typed `{hostname, serviceId, serviceType, https, port, certType, path, createdAt}`
  (`:42-52`) and renders ID / Hostname / Service / HTTPS (`:187-206`) — **no `verified`, no `wildcard`,
  no `verificationToken`, no `serverId`**. The `verified` state and the `Verify` action the list page
  exists to manage are absent here, and `domain` is renamed `hostname`. The file declares both types and
  glues them with a cast (`as Promise<ProxyDomain & ApiProxyDomain>`, `:63`, importing the API shape at
  `:22-25` while re-declaring a local one at `:42-52`), which is how the divergence survived.
- S1 · **Tenancy is dropped exactly where the brief asks for it.** The list requires a `serverId`
  (`domains/page.tsx:48`) and every record carries one (`domains/page.tsx:16`), but the detail renders
  four fields — ID, Hostname, Service (`serviceType / serviceId`, both raw strings or `—`, `:197-200`)
  and HTTPS (`:203-204`) — and **never states which server or organization the domain belongs to**. The
  "Service" value is an unresolved ID with no link. This is the one page in the group positioned to
  answer "whose domain is this", and it cannot.
- S1 · `/admin/domains/[id]` is an **orphan route**: nothing in `app/admin/domains/` links to it
  (`domains/page.tsx` has no row link; its only `Link`s are to `/admin/dns` and `/admin/security`,
  `:129-130`). A repo-wide search for `admin/domains/` returns exactly one consumer,
  `components/admin/AdminSecurity.tsx:137` (`router.push(\`/admin/domains/${d.id}\`)`) — the detail page
  for the Networking group's **Domains** route is entered only from **Security Headers**, which the
  registry files under **Access** (`admin-registry.ts:216`) after being moved out of Networking
  (`:106-107`). The sidebar's Domains row leads to a list that cannot reach its own detail view, and the
  detail view's contents (security headers, redirects) belong to two different groups.
- S1 · **"Saved — gateway route will pick it up on next sync."** (`:357-359`) renders on
  `saveMutation.isSuccess || deleteMutation.isSuccess` — so **deleting** an override prints "Saved", the
  claim that the gateway applied it is never verified (no sync status, no freshness, no "next sync"
  time), and the message sticks until the next mutation because `isSuccess` does not self-clear. An
  operator can remove a security-header override, read "Saved", and conclude the headers are enforced.
- S1 · **No validation on controls that can take a site down.** `cspEnabled` (`:309-316`) can be checked
  with an empty `cspPolicy` textarea (`:320-326`) and saved — `disabled={saveMutation.isPending}`
  (`:340`) is the only gate. `hstsMaxAge` is a bare `type="number"` with no min/max
  (`:252-257`), so `0` saves alongside `hstsEnabled: true`. `referrerPolicy` is free text (`:302-307`)
  while its sibling directives are constrained selects (`:279-299`). All three write response headers
  into a live gateway route with no pre-submit check.
- S2 · **The page's own invalidation overwrites operator edits**: `saveMutation.onSuccess` invalidates
  the headers query (`:113-117`), which changes `existing`, which re-runs
  `useEffect(… setForm({ …existing … }), [existing])` (`:88-104`) — anything typed while the response
  was in flight is replaced from the server copy. The same effect fires after `deleteMutation`
  (`:124-127`).
- S2 · Developer documentation is a first-class card: the "Wiring notes" section (`:367-395`) is a
  six-bullet list of Go files with line numbers, migration filenames and API paths
  (`handlers_proxy_domains.go:312 registerSecurityHeadersRoutes` `:371`; `store_security_headers.go` +
  `117_domains_certificates.sql:49` `:375-376`; `lib/api/security.ts:37` `:379`; `caddy_proxy.go:777`
  `:386`; `handlers_user_web.go:30`/`:46` `:372`, `:381`). The Security Headers card body repeats it with
  five more file references (`:212-225`) and the Redirects card twice more (`:421`, `:422`, `:499`).
  **The most severe instance of the group-wide copy defect, and it is a card, not a stray string.**
- S2 · `title="Domain not found"` for every non-403 error (`:144-153`), and the error card tells the
  operator about `GET /api/v1/domains/:id` (`:151-152`).
- S2 · Certificate promised, never delivered: the subtitle claims the page covers "certificate"
  (`:163`) and the local type carries `certType` (`:49`), but no certificate, expiry, issuer or link to
  `/admin/certificates` appears anywhere — on the detail page of the route whose registry description is
  "Custom domains with DNS and TLS status" (`admin-registry.ts:171`).
- S2 · Redirects are CRUD'd here (`:400-452`) while the Gateways Middlewares tab lists
  "Forward Auth / Redirect" as a middleware that is "not yet exposed as a list" and instructs creating
  them via API (`gateways/page.tsx:490`, `:499`, `:516-517`). A redirect can therefore be created on a
  domain and is invisible to the page that claims to own redirect middleware — the answer to "where do I
  add a redirect" is two pages, and neither mentions the other.
- S2 · `Pill tone={r.statusCode >= 300 && r.statusCode < 400 ? "green" : "neutral"}` (`:435`) has a dead
  branch: the form can only produce 301/302/307/308 (`:487-491`), so `neutral` is unreachable for
  anything this page creates — the signal will misfire the moment the backend holds another code.
- S2 · Table variant #6: `bg-[var(--surface-input)] … text-[10px] uppercase tracking-widest
  text-slate-500` header (`:429`) — token background mixed with raw slate text, unlike Gateways'
  `bg-[var(--surface-raised)] text-[var(--text-subtle)]` (`gateways/page.tsx:380`). The `Target` URL cell
  is `truncate max-w-[260px]` with no `title` and no copy (`:434`) — the third arbitrary URL-truncation
  width in the group after `acme/page.tsx:53` (`max-w-[200px]`) and `AdminAllocations.tsx:276`.
- S2 · Three navigation controls to the same two destinations: `backAction="Domains"` (`:164`),
  `Security Overview` (`:168`) and a `Btn <ArrowLeft/> Back` to `/admin/security` (`:348-350`) — the
  latter two both go to Security, one labelled "Overview".
- S2 · Loading waits on both queries (`:130`) and renders only "Loading domain…" (`:135`);
  `headersQuery.isError` renders an amber strip (`:227-231`) and the *absence* of an override renders an
  amber strip (`:233-238`) — same colour for "request failed" and "no override yet".
- S3 · Modals here use `Modal`/`ModalFooter` with **no `<form>` element** (`:479-503`), so Enter does
  not submit — unlike `AdminFirewall`, whose four modals wrap in `<form onSubmit>`
  (`AdminFirewall.tsx:340`, `:387`, `:436`, `:476`). `Regex` and `Preserve path` checkboxes (`:496-497`)
  are the only two fields on the page with no explanation at all, on a page otherwise saturated with Go
  file names.

### Alias stubs and the `/admin/networking` deep link
- files: `components/admin/admin-registry.ts:242-276`, `test/route-integrity.test.ts:188-202`,
  `app/admin/` directory listing, `app/admin/containers/page.tsx:1-6`
- The brief lists six alias stub directories plus `/admin/networking`. **None exist.** Verified absent:
  `app/admin/network`, `app/admin/networking`, `app/admin/infra`, `app/admin/traffic-policies`,
  `app/admin/dns-providers`, `app/admin/acme-accounts`. Only four aliases in the entire admin tree have
  stub pages (`containers`, `logs`, `git-providers`, `database-services`), and `containers` is the
  documented exemplar — a server component calling `permanentRedirect`
  (`app/admin/containers/page.tsx:1-6`).
- S1 · **Every Networking alias in `ADMIN_ALIAS_ROUTES` 404s in the browser while the registry claims it
  resolves.** The map's comment asserts these paths "affect *navigation resolution only* … a stale deep
  link or bookmark still resolves to the right nav position. Routing itself is unchanged: the few
  aliases that also need to move the browser have their own redirect stub page"
  (`admin-registry.ts:236-240`) — and lists `/admin/networking → /admin/endpoints` (`:253`),
  `/admin/infra/networking → /admin/endpoints` (`:262`), `/admin/network → /admin/domains` (`:267`),
  `/admin/traffic-policies → /admin/traffic` (`:273`), `/admin/dns-providers → /admin/dns` (`:274`),
  `/admin/acme-accounts → /admin/acme` (`:275`). The header comment at `:91-93` states the invariant
  "every `href` resolves to a real page under `app/` (**or is an alias source**)", and the test enforces
  exactly that loophole: `it.each(Object.entries(ADMIN_ALIAS_ROUTES))("alias %s targets a registered,
  resolvable href", …)` asserts only that the **target** has a page
  (`test/route-integrity.test.ts:188-194`) and that no alias shadows a registered route (`:197-202`).
  Nothing asserts an alias **source** is routable, so the suite is green and the URL is a 404. That
  test's own header comment (`:26`) records that a previous comment "claimed a 404 was structurally
  impossible" — this failure mode has already been mis-asserted once here.
- S2 · Consequence: a bookmark, Slack link, docs link or runbook URL on any of the six Networking
  aliases lands on a 404 with no in-product path back, and the sidebar highlights nothing
  (`findAdminPage` resolves the alias, `admin-registry.ts:345-347`, but there is no page to render).
  `/admin/networking` is the worst case because it is the **group deep link** — the URL external docs
  would naturally use for the whole Networking section — and its declared target (`/admin/endpoints`) is
  the group's most ambiguous page (see the Endpoints description conflict).
- S2 · Three spellings of the group name map to two destinations: "network" → Domains (`:267`),
  "networking" → Endpoints (`:253`), "infra/networking" → Endpoints (`:262`).

---

## The five "how traffic moves" pages, judged
Verdict: **not navigable.** An operator following one realistic runbook task — "rate-limit `/api/login`
at the edge" — must read all five descriptions and cannot resolve it from any of them.

| Route | Registry label | Registry description | What it actually owns |
| --- | --- | --- | --- |
| `/admin/gateways` | Gateways | Edge gateway routers, services and middlewares | read-only view of Traffic's rules + LB's groups (`gateways/page.tsx:258`, `:267`); **no create action at all** |
| `/admin/traffic` | Traffic Policies | Route rules, rate limits and traffic shaping | the only write path for rules **and** policies (`traffic/page.tsx:86`, `:118`) |
| `/admin/load-balancer` | Load Balancer | Target groups and traffic distribution | target groups, targets, and manual health writes (`load-balancer/page.tsx:90`, `:137`) |
| `/admin/crossnode` | Cross-Node Routing | Cross-node resolver cache and ingress synchronisation | resolver/cache/TTL/ingress debug panel, no list (`AdminCrossnode.tsx:180-247`) |
| `/admin/discovery` | Service Discovery | Service discovery records and network policy | endpoints, visibility, port ACL policy + a second resolve tool (`AdminDiscovery.tsx:149-291`, `:541`) |

- **Where do I add a rate limit?** "Traffic Policies" is the only label containing the phrase and the only
  entry with the keyword `rate limit` (`admin-registry.ts:181`), so sidebar and palette both point there.
  The form is a single-line raw JSON box labelled `Config (JSON)` with a placeholder
  (`traffic/page.tsx:326`). Gateways concurrently says middlewares including rate limit "are not yet
  exposed as a list", that creating them here is wrong, and that `POST /policies` has "a shape … [that]
  differs from backend's typed fields" (`gateways/page.tsx:490`, `:494`, `:516-517`). Two pages, one verb,
  one of them explicitly disclaiming itself.
- **Where do I add a redirect?** Neither label mentions one. The real CRUD lives on a page reached from
  Security Headers, not from this group (`app/admin/domains/[id]/page.tsx:400-452`, entered only via
  `components/admin/AdminSecurity.tsx:137`), while Gateways lists "Forward Auth / Redirect" as a
  middleware (`gateways/page.tsx:499`) and Cross-Node offers `POST /ingress/cleanup` for "stale routes"
  (`AdminCrossnode.tsx:302`, `:342`).
- **Where do I add an upstream?** Six names for one concept across five routes: "Target groups" (Load
  Balancer), "Services" (Gateways tab, `gateways/page.tsx:420-483`), "Targets" (Gateways topology lane,
  `:204-235`), "Endpoints" (Discovery tab, `AdminDiscovery.tsx:149-184`), "Endpoints" (the nav entry,
  which is Docker hosts), "backends" (Cross-Node toast copy, `AdminCrossnode.tsx:271`). "Endpoints" is
  itself two different resources (`admin-registry.ts:178` vs `AdminEndpoints.tsx:170-175`).
- **Grouping judgement:** seven of the twelve entries are `secondary: true`
  (`admin-registry.ts:176-183`) and collapse under "More", so the visible Networking group is Domains /
  Certificates / Gateways / Firewall. **Gateways is the only one of the five that is visible — and it is
  the one with no create path**, whose sub-copy calls its four siblings "Legacy"
  (`gateways/page.tsx:308`). The IA shows the read-only mirror as the group's representative page and
  hides the write surfaces. `Service Discovery` and `Cross-Node Routing` are the two entries in the group
  with **no `keywords` at all** (`:182-183`), so they are unfindable by the terms an operator would type
  (resolver, cache, TTL, upstream, healthcheck, sync).

## Scope-level patterns

1. **No shared renderer for the group's core nouns.** Across all 12 routes: hostname/domain renders
   `font-mono text-xs font-medium text-slate-200` (`domains/page.tsx:169`), one `<span>` per SAN
   `font-mono text-xs` (`certificates/page.tsx:140`), `truncate font-mono text-xs`
   (`gateways/page.tsx:132`, `:392`), `font-mono text-sm text-white` (`AdminDiscovery.tsx:197`, `:243`),
   joined with `", "` (`gateways/page.tsx:556`), truncated sans (`acme/page.tsx:53`), an H1
   (`domains/[id]/page.tsx:162`) and a `max-w-[200px] truncate` cell (`AdminAllocations.tsx:276`) — eight
   treatments, no shared component. URL: three different truncation widths and zero protocol handling
   (`acme:53` `max-w-[200px]`, `domains/[id]:434` `max-w-[260px]`, `AdminAllocations.tsx:267`
   `whitespace-nowrap`); `https://` is prepended by hand in an `<a href>` (`domains/[id]:173`), lives in
   a placeholder (`AdminEndpoints.tsx:182`), and is a default form value (`domains/[id]:458`). IP:port:
   twelve spellings across `load-balancer/page.tsx:193, 213, 222, 248`; `gateways/page.tsx:395, 470`;
   `AdminDiscovery.tsx:208, 255, 311, 344, 548`; `AdminAllocations.tsx:242, 264, 267-274, 305-307`;
   `AdminCrossnode.tsx:194, 234-235`. Certificate: two pages render expiry, neither renders remaining
   lifetime, neither renders `issuer` although the type carries it (`certificates/page.tsx:15`), and one
   renders a Renew button gated on a flag its own upload form cannot set (`:168` vs `:47`). **Copy
   affordance exists on exactly one page in twelve** (`AdminAllocations.tsx:148`, and only for CSV export,
   never for a value). There is no `Address`/`Hostname`/`UrlValue`/`Certificate` primitive in
   `components/ui/forge`, so every page re-invents the cell — the highest-leverage central fix in the
   group.
2. **Status text is per-page, tone spelling is per-page, thresholds are per-page.** Pill `tone` inputs use
   the legacy colour words (`green`/`yellow`/`red`/`blue`) on all twelve pages instead of the canonical
   `ok`/`warn`/`danger`/`info` (`components/ui/forge/status.ts:47-56`); `resolveTone` absorbs them
   (`:139-190`), so colours are consistent only by accident of the shim while the **words are not**:
   Verified/Unverified (`domains:185`), Valid/Expiring/Expired (`certificates:159-163`),
   enabled/disabled lowercase (`gateways:409`, `domains/[id]:436`), Active/Inactive (`traffic:215`),
   Enabled/Disabled (`traffic:274`), Firewall Enabled/Firewall Disabled with *disabled* in red
   (`AdminFirewall.tsx:120-122`), `active`/`inactive` beside `Available`/`Unavailable` in the same tile
   (`AdminCrossnode.tsx:92-94`), default/— (`acme:54`), healthy/draining/unhealthy
   (`gateways:474`, `load-balancer:248`), free/in use (`AdminAllocations.tsx:279-280`), Yes/No vs
   Reachable/Unreachable (`AdminEndpoints.tsx:135-139` vs `AdminEndpointDetail.tsx:86`), ok/fail vs
   reachable/unreachable (`AdminDiscovery.tsx:348` vs `:469`), and one chip whose label is the tone name
   itself (`gateways/page.tsx:506`). Three hardcoded thresholds (`certificates:89` 30 days;
   `AdminDiscovery.tsx:307` 180s; `AdminDiscovery.tsx:118`/`:130` 3m prose) and the canonical
   `staleAfterMs` freshness resolver is unused in all twelve.
3. **Six table grammars and three non-table grammars in one group.** `AdminTable/AdminTh/AdminTd`
   (`dns:94`, `AdminFirewall.tsx:170`, `:204`); raw `<table>` with `border-white/[0.06] … text-[10px]
   uppercase tracking-widest text-slate-500` (`domains:156`, `certificates:123`, `traffic:196`,
   `acme:48`, `domains/[id]:429`); raw `<table>` with `bg-[var(--surface-input)]` header
   (`AdminDiscovery.tsx:166`); raw `<table>` with solid `border-slate-700/50` (`AdminEndpoints.tsx:107`,
   `AdminEndpointDetail.tsx:160`, `:195`, `:220`, `:247`); raw `<table>` with sticky sortable `<th>`
   (`AdminAllocations.tsx:226-251`); unstyled nested tables (`AdminDiscovery.tsx:202`, `:248`, `:342`) —
   plus `divide-y` row lists (`gateways:461`, `traffic:253`, `load-balancer:188`, `:209`) and five `<pre>`
   JSON dumps (`AdminCrossnode.tsx:311`, `:317`, `:326`, `:332`; `AdminDiscovery.tsx:537`). The same
   "Status" column is a different width, alignment, font size and border colour on nearly every page.
4. **Errors become empty states, "not found", or vanish — on 10 of 12 routes.**
   `domains:153`, `certificates:120`, `traffic:193`/`:251`, `load-balancer:187`,
   `AdminDiscovery.tsx:397`, `AdminEndpoints.tsx:190`/`:215`/`:242` (detail),
   `AdminAllocations.tsx:213`/`:222`, `domains/[id]:144`, `AdminCrossnode.tsx:82-86` (soft-empty as
   failure) all render a "nothing here", "still loading" or "not found" surface for a failed request;
   six routes never read `.isError` at all (`domains:45`, `certificates:35`, `traffic:64`/`:69`,
   `load-balancer` group query is read but the target query is not, `AdminEndpointDetail.tsx:45`/`:47`,
   `AdminAllocations.tsx:66`). `AdminFirewall.tsx:161-168`/`:195-202` and `AdminCrossnode.tsx:82-86` are
   the two correct implementations — the template, and the minority.
5. **Zero liveness, zero provenance.** No `FreshnessBadge`, no `PageInfoDisclosure`, no
   `AdminPageToolbar` anywhere in the twelve (`grep` across all 12 route dirs + 7 components → 0 hits).
   `AdminLoadingState` appears on Gateways, ACME, Firewall and Cross-Node only; `AdminLoadingRows` on
   none; `AdminErrorState` on the same four. Every other page invents its own loading string — **19
   distinct ones** in the group, three of which leak the implementation
   (`dns:90` "via fetchDnsProviders…", `domains/[id]:423` "via fetchRedirects…", `acme:70` "Loading…").
   `AdminCrossnode.tsx:43` polls every 30s with no freshness affordance at all.
6. **The registry icon is decorative in this group.** All twelve use
   `SectionHeader`/`AdminPageHeader`, which have no `icon` prop (`admin-ui.tsx:140-161`, `:280-314`), so
   the twelve Networking glyphs exist only in the sidebar. Glyphs actually rendered on the pages:
   `Shield` (Firewall's — reused on Certificates, ACME, Gateways, Traffic, Discovery, Cross-Node),
   `Globe` (Domains' — reused on DNS, Gateways, Discovery, Cross-Node, Allocations, Endpoints-detail),
   `GanttChart` (Traffic's route card, LB's group card), `Box`, `Server`, `Network` (NetBird's +
   Placement Affinity's), `Lock`, `Heart`, `Activity`, `Cpu` (Node Capabilities'), `Database`
   (Databases'), `Trash2`, `Target`, `Zap`, `GitBranch`, `Router`, `Container`, `ArrowUpRight`,
   `Waypoints`, `Edit3`, `Copy`. Registry-side collisions inside/adjacent to this group: `Globe`
   Domains↔Environments (`:171`/`:209`), `GlobeGridIcon` Regions↔DNS Providers (`:151`/`:176`),
   `KeyRound` ACME↔OAuth Clients↔Vault (`:177`/`:214`/`:218`), `ShieldCheck`
   Zero-Downtime↔Roles (`:146`/`:211`), `Network` NetBird↔Placement Affinity (`:167`/`:203`), `Boxes`
   Registries↔Kubernetes↔Incus↔(Endpoints' `Box`) (`:130`/`:164`/`:165`). Only Firewall's card icon
   matches its own nav glyph (`AdminFirewall.tsx:159`).
7. **Backend contract text is UI copy on 8 of 12 routes**, escalating from a stray string to a page
   section: `load-balancer:181`/`:213`/`:216`; `traffic:332`/`:345`/`:394`; `gateways:233`/`:490`/`:516-517`;
   `AdminDiscovery.tsx:126`/`:461`/`:515`/`:531`/`:541`/`:552`; `AdminEndpoints.tsx:82`;
   `AdminCrossnode.tsx:58`/`:80`/`:105-117`/`:135`/`:180`/`:199`/`:206`/`:208`/`:217`/`:231`/`:299`/`:302`/`:309`/`:315`/`:324`/`:330`/`:341-342`;
   `domains/[id]:151`/`:212-225`/`:367-395`/`:421`/`:422`/`:425`/`:499`. Worst cases: a Go file *with a
   line number* as the page subtitle (`AdminCrossnode.tsx:58`), HTTP verbs as button labels
   (`AdminCrossnode.tsx:299`, `:302`), a "Wiring notes" card listing migrations
   (`domains/[id]:367-395`), and a card heading that is a route (`domains/[id]:421`).
8. **Tenancy is invisible across the group.** None of the twelve routes renders an organization, project
   or server scope control, and only two show any ownership hint. `domains` requires a *server* to see
   anything (`domains/page.tsx:48`) and its detail page drops `serverId` entirely
   (`domains/[id]:187-206`); `certificates`, `gateways`, `acme`, `load-balancer`, `traffic`, `dns`,
   `crossnode` and `endpoints` are unscoped fleet-wide lists with no owner column;
   `AdminAllocations.tsx:278-281` shows a server name with no link and no org;
   `AdminDiscovery.tsx:200`/`:310` print raw `tenantId` UUIDs with no name resolution and no filter;
   `AdminEndpointDetail.tsx:230` prints a raw `principalId`. For the group that previously leaked a
   cross-tenant secret through the env-var resolved route, an operator still cannot answer "whose is
   this?" on any Networking page — and on Endpoints, the page whose registry description says
   "across the fleet", the answer is a UUID with no name.
9. **The frame is applied four different ways.** (a) component renders `AdminPageLayout` internally
   (`AdminFirewall.tsx:93`, `AdminCrossnode.tsx:54`) under a bare 7-line wrapper
   (`app/admin/firewall/page.tsx:6`, `app/admin/crossnode/page.tsx:6`); (b) wrapper supplies it around a
   component that also opens its own `space-y-6` div (`app/admin/endpoints/page.tsx:8` +
   `AdminEndpoints.tsx:75`; `app/admin/endpoints/[id]/page.tsx:8` + `AdminEndpointDetail.tsx:74`;
   `app/admin/discovery/page.tsx:9` + `AdminDiscovery.tsx:115`); (c) page file renders it inline
   (`domains`, `certificates`, `gateways`, `dns`, `acme`, `load-balancer`, `traffic`, `domains/[id]`);
   (d) `/admin/allocations` has none (`AdminAllocations.tsx:161`). `OfflineBanner` mounts in four
   positions (`gateways:305` before header, `AdminCrossnode.tsx:55` before header, `domains/[id]:160`
   after header, `app/admin/discovery/page.tsx:11` after the component; absent on the other eight).
10. **Titles: 5 of 12 disagree with the registry label** — "Domain Management" (`domains:125`),
    "Certificate Management" (`certificates:97`), "ACME / Let's Encrypt" (`acme:36`), "Allocations"
    (`AdminAllocations.tsx:163`), "Cross-Node" (`AdminCrossnode.tsx:57`) — and two more describe only
    part of the page ("Traffic Policies" over a default tab of Route Rules, `traffic:165`/`:54`; "Load
    Balancer" scoped to "game servers" in its sub, `:165`). Descriptions are hand-authored on 12 of 12,
    spanning an exact registry match (`AdminFirewall.tsx:96`) to a 260-char paragraph calling four
    sibling nav entries "Legacy" (`gateways:308`), and one flatly contradicting the registry's own
    definition of the page (`admin-registry.ts:178` vs `AdminEndpoints.tsx:78`). No page passes an
    `eyebrow` — because `SectionHeader` has no such slot — so the group has no kicker vocabulary at all.

## Proposed remediation for this scope

1. `[frame]` Give `SectionHeader` the `icon`/`eyebrow`/`status` slots `DashHeader` already has
   (`admin-ui.tsx:163-266` vs `dashboard-cards.tsx:20-64`) and have it read
   `label`/`description`/`icon` from `admin-registry.ts` via `usePathname()`, exactly as `ForgePage` was
   designed to do (`components/admin/admin-page.tsx`, zero consumers). The 5 title mismatches, 12
   hand-authored descriptions, 12 missing icons and the missing freshness slot then resolve once, rather
   than as 29 separate edits.
2. `[frame]` Decide and enforce the wrapper/component ownership contract for `AdminPageLayout` in
   `test/route-integrity.test.ts` — either the page file or the component, never both, never neither
   (`AdminAllocations.tsx:161`). Add the missing layout and delete the nested `space-y-6` divs.
3. `[ia]` Resolve the five "how traffic moves" routes to one model, write it into the registry
   descriptions, then delete the competing accounts from page copy (`gateways:308`,
   `AdminDiscovery.tsx:126`, `AdminEndpoints.tsx:82`). Concretely: Gateways is the only one of the five
   that is not `secondary` (`admin-registry.ts:173` vs `:176-183`) and the only one with no write path —
   either give it writes and demote Traffic's Rules tab, or mark it `secondary` and promote Traffic.
   Whichever wins must own a single `RouteRule`/`RoutingRule` type, because two pages currently GET
   `/admin/traffic/rules` with non-overlapping field sets (`gateways:258` + `:42-52` vs
   `traffic:66` + `:14-22`).
4. `[ia]` Add the six missing alias stub pages using the existing `permanentRedirect` pattern
   (`app/admin/containers/page.tsx:1-6`), and close the test loophole that hid them: assert every
   `ADMIN_ALIAS_ROUTES` **key** is routable, not only its value
   (`test/route-integrity.test.ts:188-194`). Add `keywords` to `Service Discovery` and `Cross-Node
   Routing` (`admin-registry.ts:182-183` have none): resolver, cache, TTL, upstream, healthcheck, sync.
5. `[state]` Shared list skeleton for the group: `AdminLoadingRows` for loading, `AdminErrorState` with
   retry for error, and a hard rule that `.isError` is checked **before** `.length === 0`; never feed a
   loading flag into `EmptyState` (`AdminAllocations.tsx:213`). Apply at `domains:150-153`,
   `certificates:117-120`, `traffic:190-193`/`:248-251`, `load-balancer:187`, `AdminDiscovery.tsx:397`,
   `AdminEndpointDetail.tsx:102`/`:190`/`:215`/`:242`, `AdminAllocations.tsx:213`/`:222`,
   `domains/[id]:144`. Fix the five "0 while loading" tiles
   (`AdminAllocations.tsx:175-179`, `AdminEndpointDetail.tsx:102`, `acme:41`, `gateways:297-300`,
   `load-balancer:177-179`) by passing `null` through `ForgeMetric`, which already renders `—` for
   absent values (`admin-ui.tsx:647-659`).
6. `[state]` Capability gating: disable `Set TTL` with a visible reason line instead of shipping an
   enabled button whose own card says the backend is a stub (`AdminCrossnode.tsx:220`-`:225`), following
   `AdminDiscovery.tsx:49-51`. Make unknown/absent readings resolve to `tone="unknown"` instead of
   failure: `AdminCrossnode.tsx:91-102`, `AdminEndpoints.tsx:135-139`.
7. `[state]` Replace both hand-written `STATUS_COLORS` maps (`AdminEndpoints.tsx:21-27`,
   `AdminEndpointDetail.tsx:15-21`) and `NETWORK_COLORS` (`AdminDiscovery.tsx:68-72`) with `Pill tone=`
   plus one status-word map in `lib/api/status.ts`; centralise thresholds
   (`CERT_RENEW_WINDOW_DAYS`, `DISCOVERY_STALE_AFTER_MS` from the existing `staleAfterMs` resolver)
   replacing `certificates:89`, `AdminDiscovery.tsx:118`/`:130`/`:307`. Add a shared
   `FIREWALL_PROTOCOLS` list so `both` cannot be written into a `tcp|udp` field
   (`AdminFirewall.tsx:482`).
8. `[tokens]` Retire `dashboard-cards.tsx` as a styling source (`border-white/[0.08]`, `text-slate-*`,
   `text-[10px]`, the hardcoded `text-red-400` eyebrow — `dashboard-cards.tsx:31-63`) and convert the
   raw-Tailwind call sites to `app/globals.css` variables, which Gateways already uses
   (`gateways:98-137`). Highest-density files: `AdminEndpoints.tsx:14-27`/`:107`/`:119`/`:136-141`,
   `AdminEndpointDetail.tsx:15-21`/`:160`/`:173`/`:262`, `AdminAllocations.tsx:156-158`/`:236`/`:263`,
   `domains/page.tsx:158-166`/`:278`, `certificates/page.tsx:125-134`/`:159-163`.
9. `[layout]` One `Address`/`Hostname`/`UrlValue`/`Certificate` cell primitive in `components/ui/forge`
   — mono, middle-truncated with the full value in accessible text, optional copy button, protocol-aware
   — and swap the ~25 hand-written variants listed in pattern 1. Adopt `AdminAllocations.tsx:267-274` as
   the starting shape since it is already the closest to correct.
10. `[a11y]` Pass `type={field.type}` through DNS credential inputs
    (`app/admin/dns/page.tsx:153-162`) so `password` fields mask — highest-severity single line in the
    scope. Associate every bare `<label>` with its control (`domains:229-234`, `certificates:192-200`/`:221`,
    `traffic:314-324`/`:348-358`, `domains/[id]:251-336`, `AdminAllocations.tsx:339`/`:362`); add
    `aria-label` to the four unlabelled filter selects (`AdminAllocations.tsx:183`, `:187`, `:340`,
    `:363`) and to every icon-only control (`domains:213`, `certificates:173`, `dns:108`, `acme:56`,
    `traffic:221`/`:277`, `load-balancer:198`/`:213`, `AdminAllocations.tsx:284`,
    `AdminEndpoints.tsx:143-149`, `AdminFirewall.tsx:274`/`:305`); make sortable headers
    keyboard-operable with `aria-sort` (`AdminAllocations.tsx:228-248`); replace `title`-only disabled
    reasons with a visible reason line (`AdminDiscovery.tsx:316-317`); wrap every create/edit modal in a
    `<form>` so Enter works group-wide, matching `AdminFirewall.tsx:340`.
11. `[copy]` Strip route paths, HTTP verbs, Go filenames and migration numbers from all user-facing text
    on 8 routes (pattern 7), delete the `Wiring notes` card (`domains/[id]:367-395`), and replace with
    either nothing or a `PageInfoDisclosure` on the title — the primitive exists
    (`components/ui/page-info-disclosure.tsx`) and is used on zero pages in this group. Fix the three
    literal `\u2026` JSX attribute escapes (`AdminFirewall.tsx:110`, `:162`, `:196`).
12. `[ia]` Add an owner column (organization / server) with a link, plus a scope filter, to `domains`,
    `endpoints`, `allocations`, `certificates` and `traffic`; restore `serverId` to
    `domains/[id]`'s Domain card (`domains/[id]:187-206`); resolve `tenantId`/`principalId` UUIDs to names
    instead of printing them (`AdminDiscovery.tsx:200`, `:310`; `AdminEndpointDetail.tsx:230`). Bind every
    value to `Link` rather than `window.location.assign` (`gateways:311-323`, `:333-353`) or a raw `<a>`
    (`AdminFirewall.tsx:138-144`).

## Open questions for the orchestrator
- Is Gateways the intended replacement for Traffic + Load Balancer (its copy calls them "Legacy",
  `gateways/page.tsx:308`) or a parallel read-only view? The answer decides whether Traffic's Routes tab
  and Load Balancer's Services list are removed, demoted under Gateways, or kept as peers — and whether
  Gateways gains a create path. It is a product decision, not a rename.
- What *is* an "Endpoint"? `admin-registry.ts:178` says public; `AdminEndpoints.tsx:78`/`:170-175` are
  Docker/Swarm/K8s API hosts. If the latter is correct, the label should not be "Endpoints" under
  **Networking**, and `/admin/networking` should not deep-link to it (`admin-registry.ts:253`).
- `Load Balancer`'s target-status `<select>` lets an operator write `healthy`
  (`load-balancer/page.tsx:137`, `:248`). Is manual health override a supported runbook action, or should
  the write be limited to `draining` with displayed health always derived from probes? It cannot be both:
  Gateways renders the same field read-only (`gateways/page.tsx:474`).
- Certificates' upload flow drops three of five fields (`certificates/page.tsx:47` vs `:219`/`:238`/`:239`).
  Wiring them needs a `POST /certificates/upload` body change, outside a UI pass — remove the inputs, or
  take the API change into scope?
- `/admin/domains/[id]` renders security headers + redirects for a resource whose list page manages
  verification. Correct fix: (a) move it under `/admin/security`, (b) rewrite it as a true Domains detail
  and move headers to the Access-group page, or (c) reconcile the two resource types
  (`domains/page.tsx:13-22` vs `domains/[id]:42-52`)? All three are defensible and all three change
  routes.
- Is a node-scoped resource permitted to auto-select a node? `AdminAllocations.tsx:70`/`:165` does,
  `AdminFirewall.tsx:39-41` explicitly refuses to. One of the two must be treated as the group rule.
