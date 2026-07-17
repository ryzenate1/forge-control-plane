# Dependency & Package Health Audit

**Date:** 2026-07-16
**Project:** `forge-gamepanel` (Monorepo)
**Node:** v24.16.0, npm 11.13.0

---

## 1. Project Structure

The project is an npm workspaces monorepo with the following workspaces defined in `package.json`:

```
"workspaces": ["forge/web", "packages/*"]
```

| Workspace | Path | Private | Has own lockfile | Has own node_modules |
|---|---|---|---|---|
| `@forge/web` | `forge/web/` | yes | yes (8763 lines) | yes (585 MB) |
| `@forge/sdk` | `packages/sdk/` | yes | no | no |
| `@forge/shared-types` | `packages/shared-types/` | yes | no | no |
| `@forge/ui` | `packages/ui/` | yes | no | no |

**Size of root `node_modules`:** 32 MB (only contains workspace symlinks + 3 hoisted packages: react, react-dom, typescript)
**Size of `forge/web/node_modules`:** 585 MB (423 directories, full dependency tree)

---

## 2. CRITICAL: Monorepo Workspace Configuration Broken

### 2.1 Root Lockfile Out of Sync with Workspace Declaration

The root `package-lock.json` was generated with a **different workspace configuration** than what is currently in `package.json`.

| Source | Workspaces |
|---|---|
| `package.json` (current) | `["forge/web", "packages/*"]` |
| `package-lock.json` (stale) | `["apps/panel", "packages/*"]` |

- The directories `apps/panel` and `apps/frontend` referenced in the lockfile **do not exist** on disk.
- The workspace `forge/web` is **not properly resolved** from the root. `npm ls` shows:
  ```
  UNMET DEPENDENCY @forge/web@file:/Users/riyaz/project/gamepanel/forge/web
  ```
- The root `node_modules/@forge/` contains symlinks for `sdk`, `shared-types`, and `ui` but **no symlink for `web`**.

### 2.2 Dual Installation: Root vs forge/web

- `forge/web` has its **own fully independent** `node_modules/` (585 MB) and `package-lock.json` (8763 lines, v3 lockfile).
- The **root** `node_modules/` is only 32 MB and covers only the three shared packages.
- The CI pipeline (`ci.yml`), Dockerfile, and Dependabot all treat `forge/web` as a **standalone project** (running `npm ci` inside `forge/web/`), not as a root workspace.
- This dual setup is functional but confusing and suggests the monorepo migration is incomplete.

### 2.3 Recommended Actions

1. **Regenerate the root lockfile:** Run `npm install` from root to sync the lockfile with current workspaces.
2. **Decide on strategy:** Either (a) fully commit to monorepo mode (remove forge/web's independent lockfile + node_modules) or (b) remove `forge/web` from the root workspaces list and treat it as a fully independent project.
3. **If keeping monorepo:** Remove `forge/web/package-lock.json` and `forge/web/node_modules/`; let npm hoist everything to the root.

---

## 3. Engine Requirements

| Policy | Value |
|---|---|
| `package.json` engines.node | `>=20` |
| CI node version | `20` |
| Docker image | `node:20-alpine` |
| Currently running | v24.16.0 |

✅ **Compliant.** The root and CI/Docker all meet the minimum requirement.

⚠️ No `.nvmrc` or `.node-version` file found. Add one for contributor convenience.

---

## 4. Outdated / Major-Version-Behind Packages

Checked via `npm outdated` in both root and `forge/web/`:

### 🔴 TypeScript: 2 major versions behind

| Package | Declared | Current | Wanted | Latest |
|---|---|---|---|---|
| `typescript` | `^5.7.2` | `5.9.3` | `5.9.3` | **7.0.2** |

TypeScript 7 is the latest stable release. The project is pinned to the 5.x range. Upgrading to 7.x may require significant refactoring but would unlock new features and performance improvements.

### 🟡 Minor/patch versions behind (within semver range)

| Package | Declared | Resolved (lockfile) | Latest |
|---|---|---|---|
| `@types/react` | `^19.0.2` | `19.2.14` | `19.2.17` |
| `react` | `^19.0.0` | `19.2.6` | `19.2.7` |
| `react-dom` | `^19.0.0` | `19.2.6` | `19.2.7` |

These are minor updates within the same major version. Run `npm update` to pick them up.

---

## 5. Deprecated Packages

**None found.** All direct dependencies in `forge/web` and the workspaces were checked for deprecation notices. No deprecated packages detected.

---

## 6. Lockfile Health

### Root `package-lock.json`
- Format: npm lockfile v2 (older format)
- **Lines:** 195
- **Status:** ⚠️ STALE — lockfile workspaces do not match `package.json` (see Section 2)
- **Git-tracked:** ✅ Yes

### `forge/web/package-lock.json`
- Format: npm lockfile v3 (current format)
- **Lines:** 8763
- **Status:** ✅ Healthy (all dependencies resolved)
- **Git-tracked:** ✅ Yes

### `.gitignore`
- `node_modules/` is ignored (correct)
- `package-lock.json` files are **not** ignored (correct)

---

## 7. Security Audit

```
npm audit (root):      found 0 vulnerabilities
npm audit (forge/web):  found 0 vulnerabilities
```

✅ **No known vulnerabilities** in any dependency tree.

---

## 8. Missing Peer Dependencies

### @forge/ui (`packages/ui/package.json`)

The package declares `react` and `react-dom` as **peerDependencies**:

```json
"peerDependencies": {
  "react": "^19.0.0",
  "react-dom": "^19.0.0"
}
```

✅ This is correct — it properly declares React as a peer dependency since it provides UI components. The consumer (`forge/web`) has `react` and `react-dom` as direct dependencies.

---

## 9. Missing `@types/*` Packages

| Package | @types installed? | Notes |
|---|---|---|
| `react` | ✅ `@types/react` v19.0.2+ | In devDeps of forge/web and packages/ui |
| `react-dom` | ✅ `@types/react-dom` v19.0.2+ | In devDeps of forge/web and packages/ui |
| `node` | ✅ `@types/node` v22.10.2+ | In devDeps of forge/web |
| `next` | ✅ Built-in types via `next` package | Next.js ships its own types |
| `vitest` | ✅ Built-in types | Vitest ships its own types |

✅ **All required `@types/*` packages are present.**

---

## 10. Packages Missing from `dependencies`/`devDependencies` (Used But Not Declared)

### 🔴 `@forge/ui` missing dependencies

The package `packages/ui/` imports the following packages that are **not declared** in its `package.json`:

| Imported in Source | From File | Declared? |
|---|---|---|
| `clsx` | `packages/ui/src/lib/utils.ts` | ❌ Missing |
| `tailwind-merge` | `packages/ui/src/lib/utils.ts` | ❌ Missing |
| `next/link` | `packages/ui/src/admin/Sidebar.tsx` | ❌ Missing |
| `next/navigation` | `packages/ui/src/admin/Sidebar.tsx` | ❌ Missing |

These work at runtime only because `forge/web` (the consumer) has them in its `node_modules` and npm hoists them up. If `@forge/ui` were published and installed standalone, these imports would fail.

**Recommended action:** Add `clsx`, `tailwind-merge`, and `next` to either `dependencies` or `peerDependencies` in `packages/ui/package.json`.

---

## 11. Dependency Placement Audit (dependencies vs devDependencies)

### `forge/web/package.json`

| Package | Current Location | Assessment |
|---|---|---|
| `clsx` | `dependencies` | ✅ Correct — used in runtime code |
| `lucide-react` | `dependencies` | ✅ Correct — runtime icon components |
| `zustand` | `dependencies` | ✅ Correct — runtime state management |
| `tailwind-merge` | `dependencies` | ✅ Correct — runtime utility |
| `@monaco-editor/react` | `dependencies` | ✅ Correct — runtime code editor |
| `monaco-editor` | `dependencies` | ✅ Correct — runtime dependency of the above |
| `next` | `dependencies` | ✅ Correct — core framework |
| `react`, `react-dom` | `dependencies` | ✅ Correct |
| `@xterm/*` | `dependencies` | ✅ Correct — terminal emulator at runtime |
| `@tanstack/react-query` | `dependencies` | ✅ Correct — runtime data fetching |
| `autoprefixer` | `devDependencies` | ✅ Correct — build-time PostCSS plugin |
| `postcss` | `devDependencies` | ✅ Correct — build-time |
| `tailwindcss` | `devDependencies` | ✅ Correct — build-time (CSS generation) |
| `typescript` | `devDependencies` | ✅ Correct — not needed at runtime |
| `eslint` / `eslint-config-next` | `devDependencies` | ✅ Correct |
| `vitest` / `@vitest/coverage-v8` | `devDependencies` | ✅ Correct |
| `jsdom` | `devDependencies` | ✅ Correct — test environment |
| `@testing-library/*` | `devDependencies` | ✅ Correct — test utilities |
| `@types/*` | `devDependencies` | ✅ Correct |

**Assessment:** All packages in `forge/web` are correctly categorized.

### `packages/*` packages

| Package | Dependency | Assessment |
|---|---|---|
| `@forge/sdk` → `@forge/shared-types` | `dependencies` | ✅ Correct |
| `@forge/shared-types` | (none) | ✅ Correct for a types-only package |
| `@forge/ui` → `@forge/shared-types` | `dependencies` | ✅ Correct |
| `@forge/ui` → `react`, `react-dom` | `peerDependencies` | ✅ Correct model for a component library |

---

## 12. Unused Packages

Source code imports were cross-referenced against declared dependencies.

### forge/web

| Package | Used? | Evidence |
|---|---|---|
| `@tanstack/react-query` | ✅ Yes | 54 imports across components |
| `@xterm/addon-fit` | ✅ Yes | Imported in `console.tsx` |
| `@xterm/addon-search` | ✅ Yes | Imported in `console.tsx` |
| `@xterm/addon-web-links` | ✅ Yes | Imported in `console.tsx` |
| `@xterm/xterm` | ✅ Yes | Imported in `console.tsx` |
| `clsx` | ✅ Yes | Used in `lib/utils.ts` |
| `lucide-react` | ✅ Yes | ~70 imports across all components |
| `monaco-editor` | ✅ Yes | Dynamic import in `files-view.tsx` |
| `@monaco-editor/react` | ✅ Yes | Dynamic import in `files-view.tsx` |
| `next` | ✅ Yes | App router, config |
| `react`, `react-dom` | ✅ Yes | Core framework |
| `tailwind-merge` | ✅ Yes | Used in `lib/utils.ts` |
| `zustand` | ✅ Yes | Used in `stores/use-server-store.ts` |
| `autoprefixer` | ✅ Yes | PostCSS config |
| `eslint` | ✅ Yes | ESLint config |
| `eslint-config-next` | ✅ Yes | ESLint config extends |
| `postcss` | ✅ Yes | PostCSS config |
| `tailwindcss` | ✅ Yes | Tailwind config |
| `typescript` | ✅ Yes | TypeScript compiler |
| `vitest` | ✅ Yes | Test config + test files |
| `@vitest/coverage-v8` | ✅ Yes | Vitest coverage config |
| `jsdom` | ✅ Yes | Vitest test environment |
| `@testing-library/jest-dom` | ✅ Yes | Test setup |
| `@testing-library/react` | ✅ Yes | Test files |
| `@testing-library/user-event` | ✅ Yes | Test files |
| `@types/node` | ✅ Yes | TS compilation |
| `@types/react` | ✅ Yes | TS compilation |
| `@types/react-dom` | ✅ Yes | TS compilation |

**Assessment: No unused packages detected in forge/web.** All declared dependencies are used in source or configuration files.

### packages/ui

- `clsx` — used in `src/lib/utils.ts` ✅
- `tailwind-merge` — used in `src/lib/utils.ts` ✅
- `@forge/shared-types` — used for type definitions ✅
- `react` — used across components ✅
- `next` — used in `Sidebar.tsx` (`next/link`, `next/navigation`) ✅

### packages/sdk & packages/shared-types

No third-party dependencies beyond `@forge/shared-types` (sdk) and `typescript` (dev). ✅

---

## 13. Duplicate Packages

- **Root lockfile:** No duplicate versions of the same package found.
- **forge/web lockfile:** No duplicate versions of the same package found.

---

## 14. Summary of Issues Found

### 🔴 Critical (Must Fix)

| # | Issue | Location |
|---|---|---|
| 1 | Root `package-lock.json` workspaces out of sync with `package.json`. References `apps/panel` and `apps/frontend` which don't exist. | `package.json` + `package-lock.json` |
| 2 | `@forge/web` workspace is not properly resolved from root — shows as UNMET DEPENDENCY. | Root `package-lock.json` |

### 🟡 High Priority

| # | Issue | Location |
|---|---|---|
| 3 | `@forge/ui` imports `clsx`, `tailwind-merge`, `next/link`, and `next/navigation` but does NOT declare them as dependencies or peerDependencies. | `packages/ui/package.json` |
| 4 | Dual package installation: 585 MB in `forge/web/node_modules` + 32 MB in root `node_modules`. | Project structure |

### 🟢 Low Priority / Informational

| # | Issue | Location |
|---|---|---|
| 5 | TypeScript is 2 major versions behind (5.x vs 7.x latest). | `forge/web/package.json` and all workspace `package.json` files |
| 6 | No `.nvmrc` or `.node-version` file for contributor convenience. | Project root |
| 7 | `react`, `react-dom`, and `@types/react` are a few patch versions behind — run `npm update`. | `forge/web/` |
| 8 | ESLint config ignores several source files from linting (`lib/api.ts`, `lib/api/*.ts`, several components). | `forge/web/eslint.config.mjs` |

---

## 15. Overall Health Score

| Category | Verdict |
|---|---|
| Security (vulnerabilities) | ✅ 0 vulnerabilities |
| Package deprecations | ✅ None |
| Lockfile committed | ✅ Both lockfiles tracked in git |
| Engine compliance | ✅ Node >=20, currently running v24 |
| Missing peer deps | ⚠️ Minor (Section 10) |
| Unused packages | ✅ None found |
| Missing @types | ✅ None |
| Duplicate packages | ✅ None |
| Monorepo integrity | 🔴 Broken (Section 2) |
| Outdated major versions | ⚠️ TypeScript 2 major versions behind |
| Dependency categorization | ✅ Correct |

**Score: 7/10** — The project is generally healthy with no security issues, but has a significant monorepo configuration problem and some missing dependency declarations in the UI package.
