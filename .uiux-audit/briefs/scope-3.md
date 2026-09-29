# Scope 3 — Workloads B: definitions, templates, registries, storage, tags

Read `.uiux-audit/briefs/00-shared-rubric.md` first and follow it exactly. READ-ONLY on source;
write only `.uiux-audit/reports/scope-3.md`, appending after each page.

All of these are `secondary: true` ("More") pages — the long tail the user is complaining about.

| Registry label | href | files to open |
| --- | --- | --- |
| Service Definitions (nests/eggs) | `/admin/nests` | `components/admin/AdminNestsEggs.tsx`, `app/admin/nests/**` incl. `[nestId]/eggs/page.tsx`, `[nestId]/eggs/[eggId]/variables/page.tsx`, `components/admin/AdminEggVariables.tsx` |
| App Templates | `/admin/app-templates` | `app/admin/app-templates/**` |
| Compatibility Templates | `/admin/compatibility-templates` | `components/admin/AdminTemplates.tsx` |
| Image Registries | `/admin/registries` | `app/admin/registries/**` |
| Forgefile | `/admin/forgefile` | `components/admin/forgefile-manager.tsx` |
| App Storage | `/admin/app-mounts` | `components/admin/app-mounts-manager.tsx` |
| Tags | `/admin/tags` | `components/admin/tags-manager.tsx` |

Also check the alias stubs `app/admin/templates/**` (`/admin/templates → /admin/compatibility-templates`).

Focus areas:
- **Four template-ish pages, four identities**: Service Definitions, App Templates, Compatibility
  Templates, Stack Templates (Scope 4). Do they share a list/detail/edit language? Can an operator
  tell what each one *is*? "Compatibility Templates … kept for imported installs" is a registry
  description that reads like a comment to developers — judge whether the page makes the legacy
  status honest (deprecated banner? create button still enabled?).
- Nest → Egg → Variables is a three-level drill-down. Check breadcrumb correctness, back affordance,
  whether each level reinvents the header, and whether the variables editor looks like a form
  product or a raw table dump.
- Registries hold **credentials**: check secret masking, reveal/copy affordances, whether a token is
  ever rendered plain in a table cell or `title` attribute. S1 if so.
- App Storage describes "volumes, binds, tmpfs and seed files" — verify the page actually presents
  those four distinctly rather than one undifferentiated list.
- Tags: "colour-coded labels". Verify the colour picker/wheel exists, that tag colours are actually
  applied elsewhere (grep `TagBadge`/`tag-badge.tsx` usage), and that the empty state teaches the
  concept.
- Forgefile is environment-as-code: check for a validation result state, an apply action with
  confirmation, and honest reporting of what changed vs nothing applied.
- The `*-manager.tsx` naming convention (`forgefile-manager`, `app-mounts-manager`, `tags-manager`)
  vs `Admin*.tsx` — note the split and whether the managers render headers at all or rely on the
  page wrapper.
