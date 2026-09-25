import { deleteJSON, fetchJSON, postJSON, putJSON } from './http';

/**
 * Client for Forge's per-application process configuration: CPU/memory limits,
 * replica counts and deploy-gating health checks — the counterpart to Dokku's
 * `resource`, `ps` and `checks` plugins.
 *
 * Units are the ones the API stores, not the ones the UI shows:
 * `cpuLimitNanoCores` is a count of nanoCores (1e9 == one full core, matching
 * Docker) and `memoryLimitBytes` is a byte count. Use the helpers exported at
 * the bottom rather than dividing inline, so the conversion lives in one place.
 */

export type HealthCheckType = '' | 'http' | 'tcp' | 'command';

export type HealthStatus = 'healthy' | 'unhealthy' | 'pending' | 'unknown';

export interface ProcessConfig {
  id: string;
  applicationId: string;
  processType: string;
  /** null means "no CPU constraint", which is not the same as 0. */
  cpuLimitNanoCores: number | null;
  /** null means "unlimited". */
  memoryLimitBytes: number | null;
  /** 0 is a real value: it stops the process type. */
  replicas: number;
  healthCheckGating: boolean;
  healthCheckType: HealthCheckType;
  healthCheckPath: string;
  healthCheckPort: number;
  healthCheckCommand: string;
  healthCheckIntervalSeconds: number;
  healthCheckTimeoutSeconds: number;
  healthCheckRetries: number;
  healthCheckStartPeriodSeconds: number;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface ApplicationRef {
  id: string;
  name: string;
  serverId: string;
  organizationId: string;
  sourceType: string;
}

/**
 * Upsert payload. Every field is optional and omitted means "keep what is
 * stored" — the UI edits one process card at a time. Send `memoryLimitMb: 0`
 * or `cpuLimitNanoCores: 0` to clear a limit, and `healthCheckType: ''` to
 * remove a check entirely.
 */
export interface ProcessConfigInput {
  cpuLimitNanoCores?: number | null;
  memoryLimitBytes?: number | null;
  memoryLimitMb?: number | null;
  replicas?: number;
  healthCheckType?: HealthCheckType;
  healthCheckPath?: string;
  healthCheckPort?: number;
  healthCheckCommand?: string;
  healthCheckIntervalSeconds?: number;
  healthCheckTimeoutSeconds?: number;
  healthCheckRetries?: number;
  healthCheckStartPeriodSeconds?: number;
  enabled?: boolean;
  healthCheckGating?: boolean;
}

export interface ComposePresence {
  present: boolean;
  sourceKey?: string;
  parsable?: boolean;
  reason?: string;
}

export interface ProcessListResponse {
  application: Record<string, unknown>;
  processes: ProcessConfig[];
  compose: ComposePresence;
}

export interface UpsertProcessResponse {
  process: ProcessConfig;
}

export interface ScaleProcessResponse {
  process: ProcessConfig;
  previousReplicas: number;
  changed: boolean;
}

export interface HealthObservation {
  id: string;
  serverId: string;
  applicationId?: string;
  processType: string;
  healthy: boolean;
  detail: string;
  observedAt: string;
}

/**
 * One process type's rolled-up health. Note `status` distinguishes "no probe
 * has reported" (`pending`) from "the probe failed" (`unhealthy`) from "this
 * process has no check at all" (`unknown`) — they are different facts in the UI
 * and deliberately different states in the API.
 */
export interface ProcessHealthState {
  processType: string;
  replicas: number;
  cpuLimitNanoCores?: number;
  memoryLimitBytes?: number;
  healthCheckType: HealthCheckType;
  healthCheckGating: boolean;
  enabled: boolean;
  status: HealthStatus;
  detail?: string;
  lastObservedAt?: string;
  observedCount: number;
  graceWindowSeconds: number;
}

export interface RollbackDecision {
  rollback: boolean;
  reason: string;
  graceWindowSeconds: number;
  gatingProcesses: ProcessHealthState[];
  checkedAt: string;
}

export interface HealthStatusResponse {
  applicationId: string;
  statuses: ProcessHealthState[];
  rollback: RollbackDecision;
  shouldRollback: boolean;
}

export interface AppliedProcess {
  processType: string;
  service: string;
  replicas: number;
  limitsApplied: boolean;
  healthCheckType?: HealthCheckType;
  replacedExistingHealthCheck: boolean;
}

export interface SkippedProcess {
  processType: string;
  reason: string;
}

export interface ComposeRenderResponse {
  applicationId: string;
  sourceKey: string;
  composeYaml: string;
  applied: AppliedProcess[];
  skipped: SkippedProcess[];
  fullyApplied: boolean;
  persisted: boolean;
  deployed: boolean;
  stackId?: string;
  deployment?: Record<string, unknown>;
  error?: string;
}

export interface ComposeRenderOptions {
  deploy?: boolean;
  allowPartial?: boolean;
}

export interface ServerResourceLimitsResponse {
  serverId: string;
  application: ApplicationRef | null;
  processes: ProcessConfig[];
  health: ProcessHealthState[];
  observations: HealthObservation[];
  rollback: RollbackDecision | null;
  reason?: string;
}

export interface ServerHealthResponse {
  serverId: string;
  observations: HealthObservation[];
}

/** Process types a Procfile-style project commonly has; offered as suggestions,
 * not enforced — any lowercase name the compose document actually defines works. */
export const suggestedProcessTypes = ['web', 'worker', 'cron', 'release'] as const;

export const NANO_CORES_PER_CORE = 1_000_000_000;
export const BYTES_PER_MB = 1_048_576;
export const MAX_REPLICAS = 64;

const appPath = (appId: string) => `/apps/${encodeURIComponent(appId)}`;

export function fetchProcessConfigs(appId: string): Promise<ProcessListResponse> {
  return fetchJSON<ProcessListResponse>(`${appPath(appId)}/processes`);
}

export function upsertProcessConfig(
  appId: string,
  processType: string,
  input: ProcessConfigInput,
): Promise<ProcessConfig> {
  return putJSON<UpsertProcessResponse>(
    `${appPath(appId)}/processes/${encodeURIComponent(processType)}`,
    input,
  ).then((res) => res.process);
}

export function deleteProcessConfig(appId: string, processType: string): Promise<void> {
  return deleteJSON(`${appPath(appId)}/processes/${encodeURIComponent(processType)}`);
}

export function scaleProcess(
  appId: string,
  processType: string,
  replicas: number,
): Promise<ScaleProcessResponse> {
  return postJSON<ScaleProcessResponse>(
    `${appPath(appId)}/processes/${encodeURIComponent(processType)}/scale`,
    { replicas },
  );
}

export function fetchAppHealthStatus(appId: string): Promise<HealthStatusResponse> {
  return fetchJSON<HealthStatusResponse>(`${appPath(appId)}/processes/health-status`);
}

/** Render the stored process configuration into the application's compose
 * document without writing anything. */
export function previewComposeRender(
  appId: string,
  options: ComposeRenderOptions = {},
): Promise<ComposeRenderResponse> {
  return postJSON<ComposeRenderResponse>(`${appPath(appId)}/processes/compose/preview`, options);
}

/** Render and store the enriched document so the next deploy ships it. Pass
 * `deploy: true` to trigger a deployment through the existing app-hosting path. */
export function applyComposeRender(
  appId: string,
  options: ComposeRenderOptions = {},
): Promise<ComposeRenderResponse> {
  return postJSON<ComposeRenderResponse>(`${appPath(appId)}/processes/compose/apply`, options);
}

export function fetchServerResourceLimits(serverId: string | number): Promise<ServerResourceLimitsResponse> {
  return fetchJSON<ServerResourceLimitsResponse>(`/servers/${encodeURIComponent(String(serverId))}/resource-limits`);
}

export function fetchServerHealth(
  serverId: string | number,
  limit = 100,
): Promise<ServerHealthResponse> {
  return fetchJSON<ServerHealthResponse>(
    `/servers/${encodeURIComponent(String(serverId))}/health?limit=${encodeURIComponent(String(limit))}`,
  );
}

export function reportServerHealth(
  serverId: string | number,
  payload: { processType: string; healthy: boolean; detail?: string },
): Promise<{ observation: HealthObservation }> {
  return postJSON(`/servers/${encodeURIComponent(String(serverId))}/health-report`, payload);
}

// ---------------------------------------------------------------------------
// Unit formatting — the display half of the canonical units above
// ---------------------------------------------------------------------------

export function formatCpuLimit(nanoCores: number | null | undefined): string {
  if (nanoCores === null || nanoCores === undefined || nanoCores <= 0) return 'No limit';
  const cores = nanoCores / NANO_CORES_PER_CORE;
  return `${Number(cores.toFixed(3))} CPU`;
}

export function formatMemoryLimit(bytes: number | null | undefined): string {
  if (bytes === null || bytes === undefined || bytes <= 0) return 'No limit';
  const mb = bytes / BYTES_PER_MB;
  if (mb >= 1024) {
    const gb = mb / 1024;
    return `${Number(gb.toFixed(gb >= 10 ? 0 : 2))} GB`;
  }
  return `${Math.round(mb)} MB`;
}

export function secondsToDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '0s';
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const rest = seconds % 60;
  return rest === 0 ? `${minutes}m` : `${minutes}m ${rest}s`;
}

export function coresToNanoCores(cores: number): number {
  return Math.round(cores * NANO_CORES_PER_CORE);
}

export function mbToBytes(mb: number): number {
  return Math.round(mb * BYTES_PER_MB);
}

/** Inverse of coresToNanoCores. Returns null for "no limit" so a caller can
 * distinguish an unlimited process from one capped below a core, instead of
 * comparing against 0 at every use site. */
export function nanoCoresToCores(nanoCores: number | null | undefined): number | null {
  if (!nanoCores || nanoCores <= 0) return null;
  return Number((nanoCores / NANO_CORES_PER_CORE).toFixed(6));
}

/** Inverse of mbToBytes, rounded — the UI edits whole megabytes. */
export function bytesToMegabytes(bytes: number | null | undefined): number | null {
  if (!bytes || bytes <= 0) return null;
  return Math.round(bytes / BYTES_PER_MB);
}
