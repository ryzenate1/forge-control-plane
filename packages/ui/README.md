# @forge/ui

Shared React UI components for the Forge control plane. Adopted from — and
kept consistent with — the web app's primitives (`forge/web/components/ui/`)
and design tokens (`forge/web/app/globals.css`, `DESIGN_TOKENS.md`).

## 📦 Installation

```bash
npm install @forge/ui
```

Requires `react` (and `next` for the admin navigation components) as peer
dependencies. Consumers must provide the Forge design tokens (the
`--surface`, `--text`, … CSS variables) and the `ui-*` component classes —
in this repo that is `forge/web`. Components use the semantic Tailwind
utilities (`bg-surface`, `text-text-subtle`, `border-line`, `text-brand`,
`text-ok`/`text-danger`) and never hardcoded `gray-*`/`blue-*` palettes.

## 🚀 Usage

```tsx
import { cn, EmptyState, StatsCard, DataTable, AdminLayout } from '@forge/ui';
import type { SidebarNavItem } from '@forge/ui';

const navItems: SidebarNavItem[] = [
  { label: 'Servers', href: '/admin/servers' },
];

<AdminLayout title="Servers" navItems={navItems}>…</AdminLayout>
```

## 📁 Exports

### Utilities
- `cn(...inputs)` — `clsx`-based class name combiner.

### Admin layout
- `AdminLayout` — Full admin shell (sidebar + top bar + content). Takes
  `navItems` (required): navigation lives in the host app (e.g. the web
  admin registry), this package ships no hardcoded nav.
- `Sidebar` — Collapsible navigation sidebar rendering the given `items`.
- `TopBar` — Admin header bar.
- `FormCard` — Card with header and footer action area.
- `StatsCard` — Statistic display card.
- `EmptyState` — Empty state placeholder.
- `DataTable` — Searchable, sortable, paginated table. Searches every column
  unless `searchable: false` is set on it, sorts via `getValue` (or the raw
  cell), clamps the page when the data set shrinks, and keys rows with
  `keyExtractor` (falling back to `item.id`, then index).

## 🧱 Building

```bash
npm run build   # emits dist/ via tsc
npm run typecheck
```

The package is built as part of the monorepo `build:packages` script (see
`packages/README.md` — always rebuild before typechecking consumers, `dist/`
goes stale otherwise).

## 🔗 Related Packages

- [@forge/sdk](../sdk/) - GamePanel API client
- [@forge/shared-types](../shared-types/) - Shared type definitions
- [@forge/game-templates](../game-templates/) - Game server templates
