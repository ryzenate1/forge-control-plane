# UI Completion Audit

## Route-by-Route Completion

### Public & Auth Pages

| Route | forge/web | Pterodactyl | Pelican | Status | Issues |
|---|---|---|---|---|---|
| `/` (login) | ✅ `app/page.tsx` | ✅ | ✅ | Complete | No 2FA "remember device" checkbox |
| `/setup` (wizard) | ✅ `app/setup/page.tsx` | ✅ | ✅ | Complete | Basic but functional |
| `/forgot-password` | ✅ | ✅ | ✅ | Complete | — |
| `/reset-password` | ✅ | ✅ | ✅ | Complete | — |
| `/account` | ✅ `app/account/page.tsx` | ✅ (4 routes) | ✅ (Filament) | Partial | Merged into one 441-line file; should split into `/account/profile`, `/account/api`, `/account/ssh`, `/account/activity` |

### Server Pages

| Route | forge/web | Pterodactyl | Pelican | Status | Issues |
|---|---|---|---|---|---|
| `/server/[id]` (console) | ✅ | ✅ | ✅ | Complete | xterm.js, power, charts, stats |
| `/server/[id]/files` | ✅ | ✅ | ✅ | Complete | No chmod/permissions UI, no mass-select bar, no compress button |
| `/server/[id]/network` | ✅ | ✅ | ✅ | Complete | — |
| `/server/[id]/backups` | ✅ | ✅ | ✅ | Complete | No rename |
| `/server/[id]/databases` | ✅ | ✅ | ✅ | Complete | — |
| `/server/[id]/schedules` | ✅ | ✅ | ✅ | Complete | No deep-link for individual schedule edit |
| `/server/[id]/startup` | ✅ | ✅ | ✅ | Complete | — |
| `/server/[id]/users` | ✅ | ✅ | ✅ | Complete | — |
| `/server/[id]/settings` | ✅ | ✅ | ✅ | Complete | — |
| `/server/[id]/activity` | ✅ | ✅ | ✅ | Complete | — |
| `/server/[id]/mounts` | ❌ | ❌ | ✅ | **Missing** | Pelican has server-level mounts page; forge/web has API (`api.ts:1299,1307`) but no route or nav entry |

### Admin Pages

| Route | forge/web | Pterodactyl | Pelican | Status | Issues |
|---|---|---|---|---|---|
| `/admin` / `/admin/overview` | ✅ | ✅ (Blade) | ✅ (Filament) | Complete | |
| `/admin/servers` | ✅ | ✅ (Blade) | ✅ | Complete | |
| `/admin/users` | ✅ | ✅ | ✅ | Complete | |
| `/admin/nodes` | ✅ | ✅ | ✅ | Complete | |
| `/admin/locations` | ✅ | ✅ | ❌ | Complete | |
| `/admin/allocations` | ✅ | ✅ | ✅ | Complete | |
| `/admin/nests` | ✅ | ✅ | ✅ | Complete | |
| `/admin/templates` | ✅ | N/A | N/A | Complete | Forge-specific |
| `/admin/databases` | ✅ | ✅ | ✅ | Complete | |
| `/admin/mounts` | ✅ | ✅ | ✅ | Complete | |
| `/admin/regions` | ✅ | ❌ | ❌ | Complete | Forge-specific |
| `/admin/api` (keys) | ✅ | ✅ | ✅ | Complete | |
| `/admin/webhooks` | ✅ | ❌ | ✅ | Complete | Orphaned migration means backend 500s |
| `/admin/oauth-clients` | ✅ | ❌ | ❌ | Complete | Forge-specific |
| `/admin/plugins` | ✅ | ❌ | ✅ | Complete | DB-only, no runtime |
| `/admin/logs` | ✅ | ✅ | ✅ | Complete | |
| `/admin/health` | ✅ | ✅ | ✅ | Complete | |
| `/admin/operations` | ✅ | ❌ | ❌ | Complete | Forge-specific |
| `/admin/activity` | ✅ | ✅ | ✅ | Complete | |
| `/admin/settings` | ✅ | ✅ | ✅ | Complete | 60+ fields |
| `/admin/roles` | ✅ | ❌ | ✅ | Complete | |
| `/admin/monitoring` | ✅ | ❌ | ❌ | Complete | Forge-specific |

## UI Quality Assessment

| Category | forge/web | Pterodactyl | Pelican | PufferPanel |
|---|---|---|---|---|
| Visual quality | 7/10 | 7/10 | 8/10 (Filament) | 5/10 |
| Information architecture | 6/10 | 8/10 | 7/10 | 5/10 |
| Navigation | 7/10 | 8/10 | 7/10 | 5/10 |
| Consistency | 6/10 | 8/10 | 8/10 | 5/10 |
| Accessibility | 3/10 | 5/10 | 6/10 | 3/10 |
| Responsiveness | 7/10 | 5/10 | 8/10 | 4/10 |
| Feedback states (loading/empty/error) | 5/10 | 7/10 | 8/10 | 4/10 |
| Admin usability | 8/10 | 6/10 | 8/10 | 5/10 |
| User usability | 7/10 | 8/10 | 7/10 | 5/10 |
| Feature discoverability | 6/10 | 7/10 | 7/10 | 4/10 |
| Perceived speed | 7/10 | 6/10 | 8/10 | 5/10 |
| **Overall** | **6.3/10** | **6.8/10** | **7.5/10** | **4.5/10** |

## Key Issues

### Missing Features
1. **Server-level Mounts tab** — API exists (`api.ts:1299`), no UI route or nav entry
2. **No i18n** — 0 internationalization. Hardcoded English. Pterodactyl has full i18next
3. **No light mode toggle** — `globals.css` forces `color-scheme: dark`; admin pages use light tokens, creating inconsistent dual-theme
4. **Accessibility** — Zero `aria-*`/`role=`/`tabIndex` across all components

### Missing UI Features
5. **File manager** — No chmod/permissions UI, no mass-select bar, no compress button
6. **Account page** — Should be split into 4 separate pages (profile, API keys, SSH keys, activity)
7. **Deep-linkable sub-routes** — No `/files/:action(edit|new)` or `/schedules/:id` permalinks
8. **Upload progress bar** — Chunked upload exists but no visual progress

### Quality Issues
9. **Duplicate console components** — `console.tsx` (235 lines) and `console-view.tsx` (240 lines) are near-identical
10. **Empty directory** — `components/pterodactyl/` is empty
11. **Dark-light inconsistency** — Globals CSS forces dark, admin layout uses light background
12. **Token key mismatch** — localStorage `forge.accessToken` vs `modern-game-panel-token`
13. **No shared types with backend** — `lib/api.ts` types are hand-rolled, drift risk
