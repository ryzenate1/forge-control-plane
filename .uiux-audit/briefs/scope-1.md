# Scope 1 — Reference pattern spec + Access group

Read `.uiux-audit/briefs/00-shared-rubric.md` first and follow it exactly. READ-ONLY on source;
write only `.uiux-audit/reports/scope-1.md`, appending after each page.

You have a **dual mission**.

## 1a. Write the spec everyone else will be measured against
Overview, Monitoring, Activity (and Game Servers, which Scope 2 covers — reference it but don't own it)
are the pages the user says are right. Produce a precise, reusable description of *what makes them
right*, because the remediation plan will turn this into the target frame for all 88 routes:
- Exact render order of the page (header → toolbar → KPI → filters → content), with `file:line`.
- Which primitives they compose and from where (`AdminPageLayout`, `DashHeader` vs `SectionHeader`,
  `AdminPageToolbar`, `KpiGrid`, `FreshnessBadge`, `PageInfoDisclosure`).
- Heading levels actually emitted. Title sizes/weights used. Vertical gaps (`space-y-*`).
- How they bind `title`/`description`/`eyebrow`/icon — hardcoded strings or read from
  `admin-registry.ts`? **This is the crux: name exactly which strings are duplicated between the
  registry and the page.**
- How each handles loading / empty / error / stale, and how freshness is derived.
- Any inconsistency *even among these four* — the user's premise is that they're the good ones;
  verify it rather than assuming it. If `AdminMonitoring` disagrees with `AdminOverview` on some
  dimension, that dimension cannot be used as a standard.
- Where these pages violate the rubric themselves (e.g. `dashboard-cards.tsx` raw-colour tokens,
  `truncate`d descriptions, hardcoded `text-red-400` eyebrow). The reference set is the best
  available, not automatically correct.

End section 1a with a short **"Target frame contract"** block: a bullet list of rules a page must
satisfy to be considered consistent. This is the single most useful output you produce.

## 1b. Audit the Access group (and its hidden/detail routes)
| Registry label | href | likely files |
| --- | --- | --- |
| Organizations | `/admin/organizations` | `app/admin/organizations/page.tsx` |
| Projects | `/admin/projects` | `app/admin/projects/page.tsx` |
| Environments | `/admin/environments` | `app/admin/environments/page.tsx` |
| Users | `/admin/users` | `components/admin/AdminUsers.tsx` |
| Roles & Permissions | `/admin/roles` | `app/admin/roles/` |
| Single Sign-On | `/admin/social` | `app/admin/social/` |
| OAuth Clients | `/admin/oauth-clients` | `app/admin/oauth-clients/` |
| API Keys | `/admin/api` | `components/admin/ApiKeys*`, `app/admin/api/` |
| Security Headers | `/admin/security` | `components/admin/AdminSecurity.tsx` |
| Private CA & mTLS | `/admin/mtls` | `app/admin/mtls/` |
| Vault | `/admin/vault` | `components/admin/vault-provider-manager.tsx`, `webauthn-manager.tsx` |

Also: `app/admin/users/**` detail routes, `app/admin/organizations/**`, and the alias stub
`app/admin/access/page.tsx` if it exists (`ADMIN_ALIAS_ROUTES` maps `/admin/access → /admin/users`).

Access-specific things to look for: tenancy hierarchy (org → project → environment) expressed
visually or not at all; scope/permission matrices rendered as unreadable walls of checkboxes;
secret/API-key reveal + copy affordances (is a plaintext secret persisted in the DOM?); destructive
actions on users/roles without `useConfirm`; whether the three pages of the same hierarchy look
like three different products; whether `Roles & Permissions` and `Security Headers` and
`Private CA & mTLS` share any visual language despite all being "authorization".
