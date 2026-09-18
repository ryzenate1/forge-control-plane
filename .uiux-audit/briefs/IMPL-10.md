# Impl scope 10 — Platform group + shared widgets

Read `.uiux-audit/briefs/IMPL-shared.md` first, then `.uiux-audit/reports/scope-10.md`, then
implement. Report to `.uiux-audit/impl/scope-10.md`. Your slice had 34 S1 / 70 S2.

You own: `app/admin/settings/**`, `app/admin/billing/**`, `app/admin/notifications/**`,
`app/admin/mail/**`, `app/admin/webhooks/**`, `app/admin/plugins/**`, `app/admin/onboarding/**`,
`app/admin/upgrade/**`, `app/admin/dev/**`, `app/admin/platform/**` (alias stub → settings),
`app/admin/command/**` (alias stub → overview), `components/admin/AdminSettings.tsx`,
`AdminBilling.tsx`, `billing-manager.tsx`, `AdminNotifications.tsx`, `notifications-manager.tsx`,
`mail-manager.tsx`, `AdminWebhooks.tsx`, `AdminPlugins.tsx`, `onboarding-manager.tsx`,
`AdminUpgrade.tsx`, `lib/api/panel-settings.ts`, `lib/api/plugins*`.

**Frozen (report only):** `components/ui/page-info-disclosure.tsx`, `status-card.tsx`,
`panel-card.tsx`, `telemetry-ui.tsx`, `admin-shell.tsx`, `admin-page-guides.ts`.

1. **Plugins is deceptive and broken.** `GET /admin/plugins/marketplace` returns
   `pluginSvc.List()` — the *installed* records (`handlers_plugins_extended.go:14-25`) — so every
   "Install" click fails "already installed" (`plugin.go:182`). The API never returns `enabled`
   (`plugin.go:57-67`), so all rows read "Disabled", the button can't disable, and it toasts
   "State updated". Loading renders as "No plugin manifests registered" (`AdminPlugins.tsx:199`).
   **A `PUT /:id/settings (example)` button writes `{note:"example"}` to live data (`:310`)** — delete
   that immediately. The registry marks this `capability: "metadata-only"`: turn the page into an
   honest directory of what is installed and say plainly that no marketplace exists, or wire it to a
   real endpoint if one does. Do not leave an Install button that cannot work.
2. **Settings overwrites config after a failed load.** `PanelSettingsTab` seeds `DEFAULT_GENERAL`,
   gates on `isLoading` only (`AdminSettings.tsx:175`) so a failed read leaves defaults in the form,
   and Save PUTs them — silently reconfiguring the panel. Gate on error, disable Save when the load
   failed, and track dirty state. `Number("")` → `0` at `:148` writes zero where the operator left a
   field blank.
3. **Notifications = one route, two backends, two `<h1>`s** (`page.tsx:11`, `:14`); failed reads
   become empty states or permanent skeletons.
4. **Mail and Settings→Mail use different endpoints sharing one cache key**
   (`mail-manager.tsx:48` vs `AdminSettings.tsx:296`) — one page's data silently fills the other.
   Fix the key or unify the source.
5. **`/admin/dev/states` demos only 1 of 4 state vocabularies**, missing `DataState`/`FreshnessBadge`.
   You flagged this as the strongest lever in the whole plan — make the gallery show the real set so
   other pages have something to be measured against.
6. Platform Upgrade is the highest-stakes destructive action: confirm backup-before-upgrade is
   asserted not assumed, versions are real, rollback explained, progress measured, and no "up to date"
   from a stale read.
