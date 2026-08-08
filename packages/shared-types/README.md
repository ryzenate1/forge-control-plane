# @forge/shared-types

Shared TypeScript type definitions for the Forge control plane. This package is
the cross-package type contract shared by `forge/web` and `packages/sdk`.

## Installation

The package is a workspace package and is referenced by the other workspaces:

```json
{
  "dependencies": {
    "@forge/shared-types": "*"
  }
}
```

## Usage

```typescript
import type {
  ApiServer,
  ApiNode,
  ApiUser,
  ApiAllocation,
  ApiDatabase,
  ApiBackup,
  ApiSchedule,
  ApiFileEntry,
  PaginatedResponse,
} from '@forge/shared-types';
```

`forge/web` re-exports the entire package from `lib/api/types.ts`, so web
modules can import via `./types`:

```typescript
import type { ApiServer } from './types';
```

## Available Types

All types are exported from `src/index.ts` (i18n types + everything in
`src/api.ts`).

### Core API entities
- `ApiUser` - user account as returned by `/auth/me`, `/users`, ...
- `ApiServer` - server record as returned by `/servers` (fields include
  `id`, `name`, `status`, `desiredState`, `actualState`, `node`, `nodeId`,
  `owner`, `ownerId`, `template`, `dockerImage`, `startupCommand`, `cpuLimit`,
  `cpuShares`, `memoryMb`, `diskMb`, `generation`, ...)
- `ApiNode` - node record as returned by `/nodes`
- `ApiAllocation`, `ApiDatabase`, `ApiBackup`, `ApiSchedule`,
  `ApiScheduleTask`, `ApiDatabaseHost`, `ApiMount`, `ApiRegion`, `ApiLocation`,
  `ApiEgg`, `ApiNest`, `ApiTemplate`, `ApiRole`, `ApiPlugin`, `ApiWebhook`,
  `ApiSSHKey`, `ApiKey`, `ApiOAuthClient`, ...

### Pagination
- `PaginatedResponse<T>` - **canonical** paginated envelope:
  `{ data: T[], meta?: { pagination?: PaginationMeta } }`
- `PaginatedEnvelope<T>` - alias of `PaginatedResponse<T>` kept for backward
  compatibility. Prefer `PaginatedResponse<T>` in new code.
- `PaginationMeta` - `{ current, total, count, per_page, total_records }`

### Inputs
- `ServerCreateInput`, `ServerUpdateInput`, `DatabaseCreateInput`,
  `ScheduleCreateInput`, `ScheduleUpdateInput`, `ScheduleTaskCreateInput`,
  `ScheduleTaskUpdateInput`, `CreateAllocationInput`, `CreateNodeInput`,
  `UpdateNodeInput`, `CreateDatabaseHostInput`, `CreateMountInput`,
  `CreateEggInput`, `UpdateEggInput`, `CreateMigrationInput`, ...

### File manager
- `ApiFileEntry`, `ApiFileContent`, `ApiFileRead`, `RenameFileInput`

### Operations / recovery
- `ApiEvacuationPlan`, `ApiRecoveryPlan`, `ApiReservation`, `ApiMigration`,
  `ApiMigrationHistory`, `ApiOrphanRemediations`, ...

### Misc
- `ApiStats`, `ApiActivityLog`, `ApiAuditEvent`, `ApiAdminAuditEvent`,
  `ApiHealthReport`, `ApiHealthCheck`, `ApiNotification`, `ApiAlert`,
  `ApiWSTicket`, `TwoFactorSetup`, `LoginResponse`, `ApiSetupStatus`,
  `ApiPublicPanelSettings`, `ApiPanelSettings`, `CrashEvent`,
  `ApiNodeHealth`, `ApiNodeHealthScore`, `ApiNodeCapacity`,
  `ApiNodeLifecycle`, `ApiServerConfiguration`, `ApiNodeSystemInformation`,
  `ApiWebhookDelivery`, `ApiWebhookStats`, `SocialProvider`, `ApiEndpoint`,
  `ProcessType`, `OneOffTask`, `ProcfileEntry`, ...

### i18n
- `Locale`, `I18nConfig`, `DEFAULT_I18N_CONFIG`

## Conventions

- Types model the JSON payloads returned by the Forge API
  (`forge/api/internal/store` / `forge/api/internal/http`). Response types are
  supersets of the actual payload; required fields are only those the API
  always returns.
- `ApiScheduleTask.sequence` is the ordering field (the API and all consumers
  use it). `sequenceOrder` is deprecated and only kept as an alias.

## Package Structure

```
shared-types/
├── src/
│   ├── index.ts              # Re-exports i18n + api types
│   ├── api.ts                # API entity/input/response types
│   └── i18n.ts               # Locale + i18n config
├── dist/                     # Compiled output (tsc)
├── package.json
└── tsconfig.json
```

## Build

```bash
npm --workspace @forge/shared-types run build   # emits dist/
npm --workspace @forge/shared-types run typecheck
```
