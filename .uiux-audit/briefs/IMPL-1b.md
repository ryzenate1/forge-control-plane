# Impl scope 1 (continued) — Access group, remainder

A previous agent worked this slice and was cut off by a turn limit. Its report is
`.uiux-audit/impl/scope-1.md`; read it first. It completed edits on:
`environments/page.tsx`, `mtls/page.tsx`, `organizations/page.tsx`, `projects/page.tsx`,
`social/page.tsx`, `AdminActivityLog.tsx`, `AdminSecurity.tsx`, `AdminUsers.tsx`,
`user-limits.tsx`. **All of those currently compile** — verify each is internally consistent
(no unused imports, no half-wired handler) rather than rewriting it.

## Your remaining work, in priority order

1. **`app/admin/users/**` + `components/admin/AdminUsers.tsx`** — the agent died here.
   From the audit: `StatsRow` shows **0 total users during load and on error**
   (`AdminUsers.tsx:189-193`) — a failed read must never render as a count of zero. Also check
   deleting a user that owns servers is blocked with the real reason (there is a test asserting
   this: `test/ui-contracts.test.tsx:116`), and that `user-limits.tsx` surfaces limit values honestly.
2. **`app/admin/api/**` + `components/admin/AdminApiKeys.tsx`** — API keys are secrets: verify masking,
   one-time reveal on create, that the plaintext is never left in DOM text or a `title` attribute, and
   that revoke is confirmed with its blast radius. Delete of a key that an integration still uses should
   say so.
3. **`app/admin/roles/**`** — permission matrices: check the matrix is readable (not a wall of unlabeled
   checkboxes), that each checkbox has an accessible name, and that saving reports what actually changed.
4. **`app/admin/oauth-clients/**`** — client secrets: same masking rules as API keys. Confirm rotation
   is destructive and confirmed.
5. **`app/admin/vault/**` + `components/admin/vault-provider-manager.tsx`** — a connection test must
   report the real outcome per provider, not fleet success.
6. **`app/admin/security/**` + `AdminSecurity.tsx`** — the agent already touched it. Confirm the audit's
   core finding is fixed: the page shipped **static header values with unmeasured "active" pills**
   (`AdminSecurity.tsx:8-39`, `:63`). If any pill still asserts a state the page never read, fix it.
7. **`components/admin/webauthn-manager.tsx`** — has zero importers. Search for a passkey/WebAuthn
   affordance in the users/account pages; if nothing should render it, recommend deletion in the report
   (do not delete it yourself) and note the dead palette classes it carries.
8. **Alias stub pages you own, which do not exist yet:** `app/admin/access/**` (→ `/admin/users`) and
   `app/admin/tenancy/**` (→ `/admin/organizations`). `test/route-integrity.test.ts` now asserts every
   alias source is served by a real page and currently fails. Copy the shape of an existing stub such as
   `app/admin/containers/page.tsx`.

Follow `.uiux-audit/briefs/IMPL-shared.md` exactly. Append to `.uiux-audit/impl/scope-1.md` under
`## Continued` after each page.

Do NOT edit: `admin-ui.tsx`, `admin-registry.ts`, `admin-page-guides.ts`, `telemetry-ui.tsx`,
`dashboard-cards.tsx`, `scope-switcher.tsx` (another agent may be in it), any Go file, or any file
outside the list above.
