export type { Locale, I18nConfig } from '@forge/shared-types';

/**
 * Default i18n configuration. Inlined so the SDK has no runtime dependency on
 * the shared-types package (its dist is not directly importable by Node ESM).
 */
export const DEFAULT_I18N_CONFIG = {
  defaultLocale: 'en',
  supportedLocales: ['en', 'es', 'fr', 'de', 'zh', 'ja', 'ru', 'pt'],
} as const;

export { ForgeApiClient, createApiClient, ApiError, combineSignals } from './client.js';
export { ForgeApiClient as ForgeClient } from './client.js';
export type {
  ApiClientConfig,
  SetupRequest,
  SetupResponse,
  LoginCheckpointInput,
  UserCreateInput,
  CreateNodeResult,
  BackupListResponse,
  OkResponse,
} from './client.js';

export type {
  ApiUser, ApiServer, ApiNode, ApiAllocation, ApiDatabase, ApiBackup,
  ApiSchedule, ApiScheduleTask, ApiDatabaseHost, ApiMount, ApiRegion, ApiLocation,
  ApiNest, ApiEgg, ApiRole, ApiStats,
  ApiKey, ApiSSHKey, ApiOAuthClient,
  ApiPlugin, ApiWebhook, ApiWebhookDelivery,
  ApiNodeConfiguration,
  ApiActivityLog, ApiAuditEvent, ApiFileEntry, ApiServerSubuser,
  ApiStartupVariable, ApiHealthCheck, ApiHealthReport,
  ApiPanelSettings, ApiPublicPanelSettings,
  ApiSetupStatus, LoginResponse, ApiAlert, ApiNotification,
  ApiFileContent, ApiFileRead, ApiTask,
  ServerCreateInput, ServerUpdateInput,
  CreateNodeInput, UpdateNodeInput,
  CreateAllocationInput, UpdateAllocationInput,
  DatabaseCreateInput,
  ScheduleCreateInput, ScheduleUpdateInput, ScheduleTaskCreateInput, ScheduleTaskUpdateInput,
  BackupCreateInput,
  CreateDatabaseHostInput, UpdateDatabaseHostInput,
  CreateMountInput, AssignMountInput, ApiMountAssignmentResponse,
  CreateEggInput, UpdateEggInput,
  RenameFileInput, PatchScheduleTaskInput,
  PaginatedResponse, PaginationMeta, PaginatedEnvelope,
} from '@forge/shared-types';
