# Scope 10 — Platform group + settings surfaces

Read `.uiux-audit/briefs/00-shared-rubric.md` first and follow it exactly. READ-ONLY on source;
write only `.uiux-audit/reports/scope-10.md`, appending after each page.

| Registry label | href | primary | files |
| --- | --- | --- | --- |
| Platform Settings | `/admin/settings` | yes | `components/admin/AdminSettings.tsx` |
| Billing & Quotas | `/admin/billing` | yes | `components/admin/AdminBilling.tsx`, `components/admin/billing-manager.tsx` |
| Notifications | `/admin/notifications` | More | `components/admin/AdminNotifications.tsx`, `notifications-manager.tsx` |
| Mail | `/admin/mail` | More | `components/admin/mail-manager.tsx` |
| Webhooks | `/admin/webhooks` | More | `components/admin/AdminWebhooks.tsx` (uses `DashHeader`) |
| Plugins | `/admin/plugins` | More, **`capability: "metadata-only"`** | `components/admin/AdminPlugins.tsx` |
| Guided Setup | `/admin/onboarding` | More | `components/admin/onboarding-manager.tsx` |
| Platform Upgrade | `/admin/upgrade` | More | `components/admin/AdminUpgrade.tsx` |
| State Components | `/admin/dev/states` | **hidden** | `app/admin/dev/states/**` |

Plus alias stubs `app/admin/platform/**`. Also audit `components/admin/AdminAccess.tsx`,
`components/ui/page-info-disclosure.tsx`, `components/ui/status-card.tsx`, `components/ui/panel-card.tsx`
and `components/admin/telemetry-ui.tsx` — you own the shared disclosure/telemetry widgets, since
Scope 1 needs to know exactly what `FreshnessBadge` and `PageInfoDisclosure` guarantee before
declaring them the standard.

Focus areas:
- **Settings is the biggest "form" page and probably the most divergent from everything else.** Check
  tab vs section organisation, whether unsaved changes are tracked and warned, per-field help text,
  save feedback (toast vs inline), and whether any setting writes immediately without confirmation.
- **Plugins is `capability: "metadata-only"`** — the registry admits it is not real. Does the page
  render a full marketplace that looks installable? That is **S1** (deceptive functionality, the
  project's explicit UI convention violation).
- **Platform Upgrade** is the highest-stakes destructive action in the product: check backup-before-
  upgrade is asserted not assumed, version numbers are real, rollback is explained, progress is
  measured, and that no "up to date" is shown from a stale read.
- Notifications vs Mail vs Webhooks: three delivery surfaces. Do they share channel/recipient/test-
  send idioms? Each should have a real "send test" affordance — check for buttons that pretend.
  SMTP/webhook secrets: masking and one-time reveal.
- Billing & Quotas: quota usage bars — check numerator/denominator are real and that unlimited is
  distinguished from unknown.
- Guided Setup: a wizard among list pages. Check step state, whether it can be re-entered, and
  whether it duplicates `/setup` or the first-run flow.
- **`/admin/dev/states` (hidden) is a gallery of loading/empty/error states — if it exists and is
  good, it is the strongest lever in the whole plan.** Enumerate what it demonstrates, whether the
  real pages use those states, and whether it is reachable/maintained. Report whether it could
  become the shared-state vocabulary everyone else is measured against.
- Also grep `app/admin/layout.tsx`, `loading.tsx`, `error.tsx` and the shell in
  `components/admin/admin-shell.tsx` — you own route-level chrome (page-wide loading/error, sidebar,
  "More" disclosure, command palette). Note anything there that no page can override.
