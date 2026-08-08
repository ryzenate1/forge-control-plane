# @forge/ui

Shared React UI components for the Forge control plane.

## 📦 Installation

```bash
npm install @forge/ui
```

Requires `react` (and `next` for the admin navigation components) as peer
dependencies.

## 🚀 Usage

```tsx
import { cn, EmptyState, StatsCard, DataTable } from '@forge/ui';
```

## 📁 Exports

### Utilities
- `cn(...inputs)` — `clsx`-based class name combiner.

### Admin layout
- `AdminLayout` — Full admin shell (sidebar + top bar + content).
- `Sidebar` — Collapsible navigation sidebar.
- `TopBar` — Admin header bar.
- `FormCard` — Card with header and footer action area.
- `StatsCard` — Statistic display card.
- `EmptyState` — Empty state placeholder.
- `DataTable` — Searchable, sortable, paginated table.

## 🧱 Building

```bash
npm run build   # emits dist/ via tsc
npm run typecheck
```

The package is built as part of the monorepo `build:packages` script.

## 🔗 Related Packages

- [@forge/sdk](../sdk/) - GamePanel API client
- [@forge/shared-types](../shared-types/) - Shared type definitions
- [@forge/game-templates](../game-templates/) - Game server templates
