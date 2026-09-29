# Impl scope 1 — Access group

Read `.uiux-audit/briefs/IMPL-shared.md` first, then your own audit `.uiux-audit/reports/scope-1.md`,
then implement it. Report to `.uiux-audit/impl/scope-1.md`.

You own (edit nothing outside this list):
`app/admin/organizations/**`, `app/admin/projects/**`, `app/admin/environments/**`,
`app/admin/users/**`, `app/admin/roles/**`, `app/admin/social/**`, `app/admin/oauth-clients/**`,
`app/admin/api/**`, `app/admin/security/**`, `app/admin/mtls/**`, `app/admin/vault/**`,
`app/admin/access/**` (create the alias stub → `/admin/users`), `app/admin/tenancy/**` (stub → orgs),
`components/admin/AdminUsers.tsx`, `AdminSecurity.tsx`, `AdminApiKeys.tsx`, `AdminActivityLog.tsx`,
`user-limits.tsx`, `vault-provider-manager.tsx`, `webauthn-manager.tsx`, `scope-switcher.tsx`,
`lib/api/tags.ts` is NOT yours.

Top priority, in order:
1. **Environments secret reveal is enabled-and-lying** — the Eye toggle flips state
   (`app/admin/environments/page.tsx:200-202`) but `v.value` is never rendered. Either render the
   value from the real endpoint or remove the toggle. Also: env-var delete at `:206` has no
   `useConfirm`; revision failure renders as "No history" (`:105-107`).
2. **mTLS renders "Node Certificates (0) / No certificates" while loading** (`mtls/page.tsx:90`,
   `:237-239`). `Users` `StatsRow` shows 0 total users during load/error (`AdminUsers.tsx:189-193`).
3. **Security Headers ships static header values with unmeasured "active" pills**
   (`AdminSecurity.tsx:8-39`, `:63`). The page presents configured policy it never read. Show what
   the API actually returns; where nothing is measured, render unknown.
4. **Copy drift**: 8 of your 11 pages restate registry descriptions wrongly. Delete the hand-passed
   `title`/`sub`/`description` and let the frame derive them.
5. `webauthn-manager.tsx` has **zero importers** and uses a dead palette. Determine whether anything
   should render it (grep for a WebAuthn/passkey setting in users or security pages). If nothing
   consumes it, report it for deletion — do not delete a file yourself.
6. Overview/Activity: you found these hand-roll hero headers with an invented "Command" breadcrumb
   and ~160 raw slate tokens each. Fix Activity (`AdminActivityLog.tsx`) to the frame contract. For
   `AdminOverview.tsx` (1992 lines), do the header + copy derivation and the raw-token sweep only —
   do not restructure its cards.
