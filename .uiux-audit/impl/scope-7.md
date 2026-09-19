# Impl scope 7 — Networking (12 routes)

Working order: S1 list first (DNS plaintext credentials → orphaned `domains/[id]` → the ten routes
where a failed read renders as an empty list), then the remaining S1s per page, then copy drift, then
tokens. `AdminFirewall.tsx:161-168` is the state pattern being copied:
`isLoading → AdminLoadingState`, `isError → AdminErrorState + retry`, *then* `length === 0 → EmptyState`.

## Done

### app/admin/dns/page.tsx (DNS Providers) — S1 #1 complete
- **Credential masking (the scope's highest-severity single line):** the credential `Input`s now pass
  `type={field.type || "text"}`. `forge/api/internal/services/dns/service.go` `CredentialField.Type`
  classifies 32 fields `password` and 37 `text`; the form previously used that value only to choose a
  font (`mono={field.type !== "password"}`), so every API token / secret key / account key displayed in
  cleartext under copy promising encryption. Secrets also get `autoComplete="new-password"` and a
  `••••••••` placeholder; non-secret fields keep `autoComplete="off"` and mono.
- Copy at the credential block now states what is actually true: masked while typing, never re-displayed
  after saving. The old line implied display safety it did not provide.
- Custom-credentials escape hatch: was re-derived from the parsed map on every keystroke, silently
  discarding any line without `=`. It now owns its raw text (`customCredsText`), is a labelled control
  (`htmlFor`/`id`), `spellCheck`/`autoComplete` off, and reports `N key(s) recognised · M line(s)
  ignored` instead of dropping input invisibly.
- `field.required` is enforced: Create is blocked and the label names the first missing credential
  (previously a provider could be created with zero credentials).
- Per-row Verify / Set-default / Delete pending (`rowBusy`), so one shared `isPending` no longer freezes
  every row and the operator can see which provider is being probed.
- Delete confirm now states the blast radius (DNS-01 renewal breaks), and the icon-only delete has an
  `ariaLabel`.
- `p.providerType ?? p.provider ?? "—"` (could render `undefined`); `—` negative pills replaced with
  "Not default"; "Unverified" → "Not verified"; missing `createdAt` → "Not reported".
- Registry drift: deleted hand-passed `title`/`sub` that restated `admin-registry.ts:176`; the frame now
  derives title, description and the `GlobeGridIcon` glyph. Card header used `Globe` (the **Domains**
  nav glyph) → `GlobeGridIcon`; empty-state icon likewise.
- Added `status={<FreshnessBadge state={sourceState(configuredQuery)} />}` — the group had zero
  liveness affordances across twelve routes.
- Tokens: `text-slate-*`, `border-white/[0.06]`, `bg-white/[0.04]`, `text-[11px]`, `text-red-300` →
  `text-text*`, `border-line`, `bg-overlay-subtle`, `text-xs`, `text-danger`. Credential-field chips
  carried their description only in a `title` attribute; it is now visible text.

### app/admin/domains/page.tsx (Domains list)
- Deleted hand-passed `title`/`sub` (`admin-registry.ts:171` supplies them); removed the inline
  `bg-[var(--brand)]` override so the primary action uses `tone="primary"`.
- S1 "DNS mismatch or not verified": the Check DNS verdict now separates four states that previously
  shared one amber box — lookup failed (danger + the error text), does not resolve yet (warning),
  resolves but not to the expected address (warning), matches (success). A failed probe no longer reads
  as a complaint about the domain.
- S1 gating: kept the server filter (the API only serves `GET /servers/:id/domains`; there is no
  fleet-wide domain list) but the no-selection state now explains that constraint instead of reading as
  "this fleet has no domains", and the report asks for the endpoint.
- Attribution: the card header names the selected server and links to `/admin/servers/:id`;
  `AdminTable label` carries it too. Previously nothing on the page said whose domains these were.
- Failed read → empty list was already repaired for `isError`; the "no results" case now distinguishes
  "no domains match the search" (with the term) from "this server has no domains".
- Per-row Verify pending (`variables === d.id`) instead of one global `isPending` freezing every row;
  delete is per-row too and states what stops serving.
- `verifiedAt`: a verified domain with no timestamp reads "Verified — time not reported", an unverified
  one "Never verified" — the bare `—` was an unknown pretending to be a value.
- Add Domain modal: the bare unlabelled `<select>` became `AdminSelect label="Server"` (disabled with a
  reason when there are no servers); hostname validated before submit against a wildcard-aware pattern
  instead of `!addForm.domain`; the wildcard hint no longer does string surgery silently.
- Check DNS submit is blocked on a malformed hostname rather than posting it.
- Tokens throughout (`text-slate-*` → `text-text*`); status icons use `text-ok`/`text-warn`, never a raw
  `emerald-400`/`amber-400`. FreshnessBadge added.
- Left in place: the search/filter state is still local `useState` (S3 — URL sync is a refactor, not a
  UI fix).

### app/admin/domains/[id]/page.tsx (orphaned detail route) — S1 #2
Read `forge/api/internal/http/handlers_proxy_domains.go` and `internal/store/store_proxy_domains.go`
before deciding. **The two routes are genuinely different backend resources**: the list manages
`DomainRecord` via `GET /servers/:id/domains` (`handlers_domains.go:41`, fields `domain`, `wildcard`,
`verified`); the detail manages `store.ProxyDomain` via `GET /domains/:id` (`handlers_proxy_domains.go:121`,
fields `hostname`, `serviceId`, `serviceType`, `https`, `port`, `certType`). `ProxyDomain` has **no**
`serverId`, `organizationId` or `verified` column at all — so "make it consistent with the list" is not
achievable without a backend change, and removing the page would delete the only UI for security headers
and redirects, both of which are wired and working. Chosen fix: keep it and stop it lying about what it is.
- Header now names the resource: title is the hostname, description says "Gateway proxy domain · HTTPS
  on port N · path …" instead of interpolating the raw record ID into a sentence and promising a
  "certificate" section that did not exist.
- **Attribution added.** When `serviceType === "server"`, `serviceId` is resolved with `fetchServer()`
  and rendered as a linked server name plus its `owner` and `node`; unresolved, unattributable and
  in-flight each get their own honest text ("Not attributable from this record", "Not reported",
  "Resolving…"). It is never a bare UUID and never a silent `—`.
- Certificate row now renders the fields the record actually carries (`certType`, `autoRenew`) with a
  link to `/admin/certificates` — the promise the old subtitle made and did not keep.
- S1 "any error is Domain not found": 404 (`ApiError.status === 404`) is distinguished from every other
  failure; a 500 or timeout now says "Domain could not be loaded … a request failure, not a missing
  domain" with Retry, instead of telling the operator the endpoint was deleted.
- S1 "Saved — gateway route will pick it up on next sync." printed on **delete** as well as save, and
  stuck because `isSuccess` never self-clears. Replaced with an explicit notice set per mutation:
  save → "Override saved. The gateway applies it on its next configuration reload — this page cannot
  confirm the reload has happened."; delete → "Override removed. This domain now falls back to the
  global security-header defaults." Cleared on the next `mutate()`, and both mutations now toast on error.
- S1 no validation on controls that can take a site down: HSTS Max-Age bounded to 1–63072000 s, CSP
  cannot be enabled with an empty policy, and Referrer-Policy changed from free text to the eight values
  browsers accept. Errors list above the Save button, which is disabled while any remain.
- S2 the page's own invalidation overwrote operator edits: the hydrate-from-server effect is now keyed
  on the override id through a ref, so a save that refetches `existing` no longer replaces what was
  typed while the response was in flight.
- S2 same colour for "request failed" and "no override yet": failure is `AdminErrorState` with Retry,
  loading is `AdminLoadingState`, absence is a neutral line — three different surfaces.
- **Removed the "Wiring notes" card** (six bullets of Go filenames, line numbers and a migration
  filename) and the Go-file paragraphs inside the Security Headers and Redirects cards; replaced with
  one operator-facing sentence each. The Redirects card heading is "Redirects", not a route path.
- Redirects: converted to `AdminTable`, error/empty/loading separated, `title` no longer the only copy of
  the truncated target URL (AdminTd `title` + visible truncation), status-code pill no longer has a dead
  branch (`neutral` was unreachable for 301/302/307/308), delete is per-row with an `ariaLabel` and a
  blast-radius description naming the hostname.
- Redirect modal: source path must start with `/`, target must be an absolute http(s) URL, both errors
  shown inline; the `PUT /domains/…` description replaced with plain language; Regex and Preserve-path
  checkboxes finally say what they do; the "Wires createRedirect → admin handlers" line deleted.
- Removed the duplicate third "Back" button (the header already has `backAction` and a Security
  Overview action). Tokens converted; `text-ok`/`text-danger` instead of `emerald-300`/`red-300`.

## Verified
- (pending full run)

## Needs central change (do NOT edit these myself)
- `lib/api/domains.ts`: no fleet-wide domain list — add a client for a `GET /domains`-style server-less
  endpoint once the API has one; until then `/admin/domains` cannot answer "how many unverified domains
  does the fleet have".
- `lib/api/proxy-domains.ts`: missing `verifyAdminProxyDomain(id)`. The backend has a real
  `POST /domains/:id/verify` (`handlers_proxy_domains.go:246`, does an actual DNS lookup and returns
  `verified` + `addresses`), so `/admin/domains/[id]` cannot offer the Verify action its list page exists
  to manage. Add the client function and the detail page should expose it.
- `store.ProxyDomain` carries no `organizationId`/`serverId`/`tenantId`, so org tenancy on this resource
  is not inventable in the UI. Attribution is currently derived by resolving `serviceId` through
  `fetchServer`. A backend field (or an owner on the GET response) is the real fix.
- `components/admin/admin-page-guides.ts` has **no entry for any Networking route**, so no page in this
  slice can pass `info={…}` — twelve `PageInfoDisclosure` slots stay empty until guides are added.
- `components/admin/admin-ui.tsx` `AdminStatCard`/`StatsRow` type `value` as `string | number`, so a
  loading tile cannot pass `null` to get `ForgeMetric`'s unknown dash; pages must hand it a "—" string.
  Widen to `string | number | null | undefined`.

## Removed
- `domains/[id]` "Wiring notes" card and three Go-file/migration citation paragraphs.
- `domains/[id]` duplicate Back button.
- Hand-passed `title`/`sub` on `dns` and `domains` headers (now registry-derived).

## Deferred
- Merging or re-IA-ing Gateways/Traffic/Load Balancer (product decision; the brief says do not merge).
- Shared `Address`/`Hostname`/`UrlValue` cell primitive (scope pattern 1 — central change).
- URL-persisted search/filter state (S3 on several pages).

## Continued (second agent, remaining 9 routes + alias stubs)

### Interrupt repair
- `app/admin/domains/[id]/page.tsx:162`: `form.cspPolicy` is optional on
  `UpdateSecurityHeadersInput`, so `.trim()` was a type error. Now
  `!(form.cspPolicy ?? "").trim()` — identical validation behaviour (CSP cannot be enabled with an
  empty policy), no crash if the field is absent.

### State of the tree on arrival (already repaired by the interrupted agent, verified not redone)
`app/admin/acme/page.tsx`, `app/admin/certificates/page.tsx` (apart from the fix below),
`components/admin/AdminEndpoints.tsx` and `components/admin/AdminEndpointDetail.tsx` were already
converted: registry-derived headers, `isPending → isLoading → isError → empty` precedence,
`AdminTable`, `FreshnessBadge`+`sourceState`, `STATUS_TONE`/`Reachability` with `tone="unknown"`,
`formatMB`/`countOrUnknown`/`percentOrUnknown` (no more "undefined MB" / "NaN%"), 404-vs-500 split on
the detail page, `ariaLabel`s, token colours. `npx tsc --noEmit` reported no errors in any of them.

### app/admin/certificates/page.tsx — honesty fix on the upload form
- The upload now posts `{domainId, certificate, privateKey, issuer}` to `POST /certificates`.
  **Verified against the handler** (`forge/api/internal/http/handlers_proxy_domains.go:309-375`): it
  binds exactly those four fields and requires `domainId`.
- **Removed the "Renew automatically" checkbox.** The handler ignores `autoRenew` for imported
  certificates and answers with a `warning` ("an imported certificate has no ACME order behind it"), so
  the control could not do its job — an enabled-and-lying input. The hint now says plainly that an
  imported certificate is never renewed automatically. The mutation reads the response and surfaces the
  server `warning` as a `tone: "warning"` toast instead of a flat success.

### components/admin/AdminAllocations.tsx — IP Allocations
- **S1 node auto-pick deleted.** `setNodeId(nodes[0]?.id ?? "")` on modal open and
  `nodeId || nodes[0]?.id || ""` in the mutation both picked a host the operator never chose (AGENTS.md
  pitfall). The node select now starts empty with a "Select a node" prompt and a visible reason line;
  `mutationFn` throws if no node is chosen; Create is disabled until one is.
- **S1 three KPI tiles read 0 while loading.** `StatsRow` now takes
  `isPending → "—"`, `isError → "Unavailable"`, else the count.
- **S1 `EmptyState` was the loading state** (its default heading is "Nothing to show", so the primary
  card's headline was a no-results message during every fetch). Now `AdminLoadingState` →
  `AdminErrorState` + retry → `EmptyState`, and the empty case distinguishes "no allocation matches the
  filters" from "none recorded".
- S2 security default: the IP field was pre-filled `0.0.0.0` (widest exposure as the unchosen value).
  Now empty with a placeholder and a hint naming the exposure.
- S2 IPv6 validation accepted `::`, `1:2:3`, `ffff:`. New `isIPv6Address` requires ≤7 hextets with one
  `::`, or exactly eight, each 1-4 hex digits. Validation also gates `disabled` now (it used to appear
  one click late) and `onConfirm` re-checks it.
- S2 four unlabelled selects → `htmlFor`/`id` + `aria-label`; search `Input` gets an `aria-label`.
- S2 "Export" was labelled as a file download but copied CSV to the clipboard: renamed **Copy as CSV**,
  the toast states the row count and that it is the filtered set, and CSV values are now quote-escaped
  (raw interpolation corrupted the file on any name containing `"`).
- S2 inert `sticky top-0` header removed; sortable `<th>`s are real `<button>`s with `aria-sort`
  (was mouse-only, direction visible only on hover). Disabled bulk-select checkboxes carry a reason.
- S2 `0`-vs-unknown: alias cell renders "Not named"; `free` pill → "Free"; icon-only edit/delete get
  `ariaLabel`s.
- Frame: the route had **no `AdminPageLayout` at all** (only unconstrained page in the group) — added.
  Hand-passed `title="Allocations"`/`sub` deleted (registry says "IP Allocations"), stats glyph
  `Globe` (Domains' icon) → `Cable` (this route's registry glyph).
- Tokens: `text-slate-*`, `border-white/[0.0x]`, `bg-surface-card-header`, and the **red focus ring on
  non-danger filters** → `text-text*`, `border-line`, `bg-overlay-subtle`, `border-danger-line
  bg-danger-subtle text-danger` for error strips, brand focus rings.
