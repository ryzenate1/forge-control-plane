# Packages

Shared packages for the GamePanel ecosystem.

## 📦 Available Packages

- [game-templates/](game-templates/) - Game server templates for easy deployment
- [sdk/](sdk/) - TypeScript SDK for GamePanel API
- [shared-types/](shared-types/) - Shared TypeScript type definitions
- [ui/](ui/) - Shared React UI components

## 🚀 Usage

Each package can be installed independently:

```bash
# Install the SDK
npm install @forge/sdk

# Install shared types
npm install @forge/shared-types

# Install UI components
npm install @forge/ui
```

## 🏗️ Building (read this first)

Consumers resolve `@forge/*` through the workspace symlinks in
`node_modules/@forge/*`, which point at each package's **`dist/` output** —
not `src/`. A stale `dist/` therefore typechecks and runs like current code
while actually being old code. Never trust `dist/` after pulling or switching
branches.

```bash
npm run build:packages   # shared-types → sdk → ui → game-templates, in dependency order
```

`build:packages` is mandatory before `forge/web` typecheck/build, and it runs
exactly once per flow:

- `next build` triggers the web workspace's `prebuild` hook, so the root
  `build` script and `make build` delegate to it — do not add an explicit
  `build:packages` call to either or every build compiles the packages twice.
- `tsc --noEmit` has no working hook: npm runs `prebuild` but does NOT run
  `prestypecheck` (verified on npm 11 — the hook is dead weight, so it was
  removed), so the root `typecheck` script and `scripts/dev/lint.sh` build the
  packages with an explicit call. That call cannot double-build precisely
  because the hook never fires.
If you add a `paths` fallback that maps `@forge/*` to `src/`, keep the hooks:
`src/` fallback without a staleness check just moves the same hazard.

Intra-workspace dependencies use the `"*"` pin (e.g.
`"@forge/shared-types": "*"`). npm resolves `"*"` to the local workspace —
the `workspace:` protocol is pnpm/yarn-only and breaks `npm install`
(`EUNSUPPORTEDPROTOCOL`), so do not "upgrade" the pins.

TypeScript module settings differ per package on purpose: `@forge/sdk` uses
`module`/`moduleResolution: NodeNext` (it ships real ESM with `.js`
extension imports and must resolve the shared-types `exports` map at runtime),
while the other packages use `bundler` resolution. Do not "align" the SDK to
`bundler` — NodeNext is what keeps its runtime import of
`@forge/shared-types` honest.

## 🔌 Wiring status

Only `@forge/shared-types` is a runtime dependency of `forge/web`
(`forge/web/lib/api/types.ts` re-exports it; `PaginatedEnvelope`,
`ApiStartupVariable`, `ApiSetupRequest` and the rest are imported from there).
`@forge/sdk`, `@forge/ui` and `@forge/game-templates` are standalone packages
with no import path into the dashboard bundle — this is intentional, not dead
code:

- `@forge/sdk` is the external TypeScript client for third-party integrations.
  The dashboard uses its own cookie-session primitive
  (`forge/web/lib/api/http.ts`) instead, because the dashboard needs
  same-origin credentials, CSRF signing and the `forge:session-expired` signal
  while the SDK needs bearer-token auth and Node-compatible timeouts.
- `@forge/ui` holds reusable React primitives for future surfaces; the
  dashboard keeps its hand-rolled `components/ui/` set until a migration is
  scheduled. Both expose the same `cn()` helper (see below).
- `@forge/game-templates` is the template catalog consumed by operators and
  tooling, not imported by the dashboard at runtime.

## 🌐 HTTP stack alignment (`sdk` vs `web`)

Both stacks share one contract and differ only where the runtime demands it:

| Concern | `packages/sdk` (`client.ts`) | `forge/web` (`lib/api/http.ts`) |
| --- | --- | --- |
| Error shape | `ApiError(status, statusText, data, details)`; `details` from `{ details }` → `{ errors }` → `{ fields }` | `ApiError(message, status, details)`; same `details` precedence via `readErrorDetails` |
| Envelope helpers | `unwrapList` (canonical) + deprecated `unwrapData` list alias + `unwrapSingleData` for single objects | `unwrapList` for lists, `unwrapData` for single objects |
| Retry | GETs auto-retry (3 attempts, 429/502/503/504 + `Retry-After`); mutations run once unless an `Idempotency-Key` is supplied | Opt-in per call via `{ retry: true \| policy }` (408/429/500/502/503/504 + transport failures) |
| CSRF | `X-CSRF-Token` from `__Host-forge_csrf` (fallback `forge_csrf`), mutations only, cookie-anchored match | Same two cookies, same anchoring, same mutation-only rule, plus a console warning when the cookie is missing |
| 401 signal | `onUnauthorized` hook (host app redirects) | `forge:session-expired` window event (providers.tsx redirects) |

Constructor argument order is intentionally not unified (changing either
would break existing callers); the documented equivalence is message
derivation, `details` extraction and the 401 hook/signal pair above.

## 🏷️ Versioning

All four packages are `private: true` at `0.1.0` and are not published to any
registry — they are consumed exclusively through the npm workspace symlinks.
Intra-workspace dependencies use the `"*"` pin, which npm resolves to the
local workspace (the `workspace:` protocol is pnpm/yarn-only).

Breaking-change policy while `0.x` and private: breaking changes are allowed
without a major bump, but they must be called out in the PR description and in
the affected package's README section above, with a migration note for the old
name/shape (deprecated aliases such as `unwrapData` and
`ServerStartupVariable` stay until their callers are migrated). No CHANGELOG
is kept while the packages are private; cut one when the first package is
published.

## `cn()` convention

`@forge/ui` (`src/lib/utils.ts`) and the dashboard (`forge/web/lib/utils.ts`)
both implement `cn()` as `twMerge(clsx(...))`. Do not regress `ui` to
`clsx`-only: without `tailwind-merge`, conflicting Tailwind classes
(`p-2 p-4`) merge unpredictably instead of resolving to the last one.

## 📁 Package Structure

Each package follows this structure:

```
package/
├── src/          # Source TypeScript code
├── dist/         # Compiled output
├── package.json  # Package configuration
├── tsconfig.json # TypeScript configuration
└── README.md     # Package-specific documentation
```
