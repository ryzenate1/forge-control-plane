# Code Quality Audit

## Naming & Organization

| Project | Quality | Evidence |
|---|---|---|
| Forge | Good but inconsistent | `store.go` is a 1311-line monolith containing domain types, store interface, connection logic, and seed data. Handlers follow `handlers_<group>.go` pattern. Some names are Wings-inspired (`handlers_remote.go` vs Pterodactyl's `DaemonAuthenticate.php`). |
| Pterodactyl | Excellent | Laravel conventions: `Controller`, `Service`, `Repository`, `Transformer` suffixes. Models in `app/Models/`. Controllers in `app/Http/Controllers/Api/`. |
| Pelican | Excellent | Same as Pterodactyl + Filament conventions (`Resource`, `Page`, `Widget`). |
| PufferPanel | Good | Consistent `*api.go` / `*view.go` model separation. Clear package names. |
| Wings | Good | Clear package boundaries, consistent naming. |

## File Size & Function Size

| Metric | forge/api | forge/web | Pterodactyl | Pelican | PufferPanel | Wings |
|---|---|---|---|---|---|---|
| Largest file | 1311 (store.go) | 2523 (api.ts) | ~800 (Service classes) | ~900 | ~600 | ~865 (config.go) |
| Functions >100 lines | Several in handlers | Few in components | Rare | Rare | Few | Some |
| Functions >300 lines | Yes (handlers_servers.go) | Yes (api.ts) | Rare | Rare | Rare | config.go |

## Type Safety

| Project | Quality | Evidence |
|---|---|---|
| Forge (Go) | Strong | Go's type system ensures compile-time safety. DTOs with `DTO` suffix. |
| Forge (TS) | Poor | `lib/api.ts` uses `any` extensively, explicitly ignored by eslint. No shared types with backend. |
| Pterodactyl (PHP) | Medium | PHP 8.2 typed properties, but no compile-time check |
| Pelican (PHP) | Medium | Same |
| PufferPanel (Go) | Strong | Go types, consistent model layer |
| Wings (Go) | Strong | Go types, clear interfaces |

## Error Handling

| Project | Quality | Evidence |
|---|---|---|
| Forge | Inconsistent | Some handlers check errors properly, others use `_ = c.BodyParser(&body)` (silent fail). Health check is thorough. |
| Pterodactyl | Good | Laravel exception handler with consistent error responses. |
| Pelican | Good | Same Laravel pattern. |
| PufferPanel | Good | Gin recovery middleware + structured error types. |
| Wings | Good | structured `RequestError` type, middleware-based. |

## Code Quality Scores

| Category | forge/api | forge/web | beacon | Pterodactyl | Pelican | PufferPanel | Wings |
|---|---|---|---|---|---|---|---|
| Readability | 7/10 | 6/10 | 6/10 | 9/10 | 8/10 | 7/10 | 7/10 |
| Maintainability | 6/10 | 5/10 | 5/10 | 8/10 | 8/10 | 7/10 | 7/10 |
| Modularity | 8/10 | 7/10 | 6/10 | 8/10 | 8/10 | 6/10 | 7/10 |
| Type Safety | 8/10 | 4/10 | 8/10 | 6/10 | 6/10 | 8/10 | 8/10 |
| Error Handling | 6/10 | 5/10 | 5/10 | 8/10 | 8/10 | 7/10 | 7/10 |
| Testability | 5/10 | 4/10 | 2/10 | 7/10 | 7/10 | 5/10 | 6/10 |
| Extensibility | 9/10 | 7/10 | 6/10 | 7/10 | 8/10 | 6/10 | 6/10 |
| Documentation | 4/10 | 3/10 | 3/10 | 7/10 | 6/10 | 4/10 | 4/10 |
| Tech Debt | 5/10 | 4/10 | 3/10 | 8/10 | 7/10 | 6/10 | 7/10 |
| **Overall** | **6.4/10** | **5.0/10** | **4.9/10** | **7.6/10** | **7.3/10** | **6.2/10** | **6.6/10** |

## Key Issues

### forge/api
- `store.go` at 1311 lines is too large and mixes concerns (types, DTOs, connection, migration, seed)
- 35 files not `gofmt`-clean
- Duplicate migration prefixes (015, 018, 023)
- Stray `p` byte in migration 020
- Inconsistent error handling (silent BodyParser failure in multiple handlers)

### forge/web
- `lib/api.ts` at 2523 lines, uses `any` extensively, eslint-ignored
- 43 ESLint warnings (unused vars, exhaustive deps, no-img-element)
- Token storage key mismatch (uses `forge.accessToken` vs `modern-game-panel-token`)
- Duplicate console components (`console.tsx` and `console-view.tsx`)

### beacon
- Zero test files across all packages
- Missing `gopkg.in/yaml.v2` in go.mod
- `installer.go` has 5+ compile errors
- 10 files not `gofmt`-clean
