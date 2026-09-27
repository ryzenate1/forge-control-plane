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

`build:packages` is mandatory before `forge/web` typecheck/build, and the root
`typecheck`/`build` scripts already run it as a pre-step (pre-typecheck hook).
If you add a `paths` fallback that maps `@forge/*` to `src/`, keep the hook:
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
