// Server management API functions
import { fetchJSON, postJSON, putJSON, patchJSON, deleteJSON, requestBlob, getApiBaseUrl, ApiError } from './http';
import type {
  ApiServerSubuser,
  ApiAuditEvent,
  ApiDatabaseOrphanRemediation,
  ApiOrphanRemediations,
  ApiServerOrphanRemediation,
  CrashEvent,
  ProcessType,
  ProcessScalingEvent,
  OneOffTask,
  ProcfileEntry,
} from './types';
import type { ApiServer, ApiAllocation, ApiDatabase, ApiBackup, ApiSchedule, ApiScheduleTask, ApiServerDatabaseDeleteResult, ApiStartupVariable, BackupCreateInput, ServerCreateInput, ServerUpdateInput, DatabaseCreateInput, ScheduleCreateInput, ScheduleUpdateInput, ScheduleTaskCreateInput, ScheduleTaskUpdateInput, PaginatedEnvelope, ApiLegacyTransferStatus } from './types';
// ApiStartupVariable is canonical in @forge/shared-types (re-exported via ./types).
// ServerStartupVariable is kept as a backward-compatible alias so existing
// imports (`import { ServerStartupVariable } from "@/lib/api"`) keep working.
/** @deprecated Use `ApiStartupVariable` from `@forge/shared-types` instead. */
export type ServerStartupVariable = ApiStartupVariable;

async function fetchWithEnvelope<T>(path: string): Promise<PaginatedEnvelope<T>> {
  const body = await fetchJSON<PaginatedEnvelope<T> | T[]>(path);
  // Backend paginated lists return `{ data, meta.pagination }`; older routes
  // may return a bare array. Normalize to the envelope so callers never
  // branch on the shape.
  if (Array.isArray(body)) return { data: body };
  if (body && typeof body === 'object' && Array.isArray((body as PaginatedEnvelope<T>).data)) {
    return body as PaginatedEnvelope<T>;
  }
  if (body == null) return { data: [] };
  throw new Error(`Unexpected response: expected an array or { data: [...] }`);
}

/** Page count from a pagination meta. `total_records / per_page` is authoritative; `total`/`total_pages` are fallback. */
function pageCountOf(pagination?: { total?: number; total_pages?: number; total_records?: number; per_page?: number }): number {
  if (!pagination) return 1;
  if (
    typeof pagination.total_records === 'number' &&
    typeof pagination.per_page === 'number' &&
    pagination.per_page > 0 &&
    pagination.total_records >= 0
  ) {
    return Math.max(1, Math.ceil(pagination.total_records / pagination.per_page));
  }
  if (typeof pagination.total_pages === 'number' && pagination.total_pages > 0) return Math.floor(pagination.total_pages);
  // `total` is already a page count — never divide it by per_page again.
  if (typeof pagination.total === 'number' && pagination.total > 0) return Math.floor(pagination.total);
  return 1;
}

export async function fetchServers(): Promise<ApiServer[]> {
  const firstPage = await fetchWithEnvelope<ApiServer>('/servers?page=1&per_page=100');
  const totalPages = pageCountOf(firstPage.meta?.pagination);
  if (totalPages <= 1) {
    return firstPage.data ?? [];
  }

  const remainingPages = await Promise.all(
    Array.from({ length: totalPages - 1 }, (_, i) =>
      fetchWithEnvelope<ApiServer>(`/servers?page=${i + 2}&per_page=100`),
    ),
  );

  return [
    ...firstPage.data,
    ...remainingPages.flatMap((r) => r.data ?? []),
  ];
}

/** Fetch one server page and retain the response pagination metadata. Canonical for the `@/lib/api` barrel. */
export async function fetchServersPage(
  page = 1,
  perPage = 100,
): Promise<PaginatedEnvelope<ApiServer>> {
  return fetchWithEnvelope<ApiServer>(`/servers?page=${page}&per_page=${perPage}`);
}

/** Fetch every server page. Alias of {@link fetchServers} for callers that name the aggregation explicitly. */
export async function fetchAllServers(): Promise<ApiServer[]> {
  return fetchServers();
}

export async function fetchServer(id: string): Promise<ApiServer> {
  return fetchJSON<ApiServer>(`/servers/${encodeURIComponent(id)}`);
}

export async function createServer(input: ServerCreateInput): Promise<ApiServer> {
  return postJSON<ApiServer>('/servers', input);
}

export async function updateServer(id: string, input: ServerUpdateInput): Promise<ApiServer> {
  return patchJSON<ApiServer>(`/servers/${encodeURIComponent(id)}`, input);
}

export async function deleteServer(id: string, force?: boolean): Promise<void> {
  const query = force ? "?force=true" : "";
  await deleteJSON(`/servers/${encodeURIComponent(id)}${query}`);
}

export async function sendServerCommand(
  serverId: string,
  command: string,
): Promise<void> {
  await postJSON<void>(`/servers/${encodeURIComponent(serverId)}/command`, {
    command,
  });
}

export type PowerSignal = 'start' | 'stop' | 'restart' | 'kill';

/**
 * The API accepts a power signal by dispatching a durable operation and answers
 * with its id. Accepted means queued — never completed — so the id is required
 * to report real progress instead of assuming the workload obeyed.
 */
export type PowerDispatch = {
  serverId: string;
  signal: string;
  accepted: boolean;
  mode?: 'durable' | 'queued' | string;
  operationId?: string;
};

export async function sendPowerSignal(
  serverId: string,
  signal: PowerSignal,
): Promise<PowerDispatch> {
  return postJSON<PowerDispatch>(
    `/servers/${encodeURIComponent(serverId)}/power`,
    { signal },
  );
}

/**
 * Progress of a dispatched operation. `GET /operations/:id` serves either an
 * operation record or a queue job, and the two use different status words; the
 * raw status is preserved alongside the normalized one so nothing is invented.
 */
export type OperationProgressStatus = 'pending' | 'running' | 'succeeded' | 'failed' | 'cancelled' | 'unknown';

export type OperationProgress = {
  id: string;
  status: OperationProgressStatus;
  rawStatus: string;
  error?: string;
  startedAt?: string;
  completedAt?: string;
  terminal: boolean;
};

function normalizeOperationStatus(raw: string): OperationProgressStatus {
  switch (raw) {
    case 'queued':
    case 'pending':
      return 'pending';
    case 'running':
    case 'retrying':
      return 'running';
    case 'succeeded':
    case 'completed':
      return 'succeeded';
    case 'failed':
      return 'failed';
    case 'cancelled':
      return 'cancelled';
    default:
      // An unrecognised status is reported as unknown. Guessing "succeeded"
      // here would claim an operation finished that may still be running.
      return 'unknown';
  }
}

export async function fetchOperation(operationId: string): Promise<OperationProgress> {
  const raw = await fetchJSON<{
    id?: string;
    status?: string;
    error?: string;
    startedAt?: string;
    completedAt?: string;
  }>(`/operations/${encodeURIComponent(operationId)}`);
  const rawStatus = raw.status ?? '';
  const status = normalizeOperationStatus(rawStatus);
  return {
    id: raw.id ?? operationId,
    status,
    rawStatus,
    error: raw.error,
    startedAt: raw.startedAt,
    completedAt: raw.completedAt,
    terminal: status === 'succeeded' || status === 'failed' || status === 'cancelled',
  };
}

export async function reinstallServer(serverId: string): Promise<{ accepted: boolean }> {
  return postJSON<{ accepted: boolean }>(`/servers/${encodeURIComponent(serverId)}/reinstall`);
}

export async function suspendServer(serverId: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/servers/${encodeURIComponent(serverId)}/suspension`, { action: "suspend" });
}

export async function unsuspendServer(serverId: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/servers/${encodeURIComponent(serverId)}/suspension`, { action: "unsuspend" });
}

// Server allocations
export async function fetchServerAllocations(serverId: string): Promise<ApiAllocation[]> {
  return fetchJSON<ApiAllocation[]>(`/servers/${encodeURIComponent(serverId)}/allocations`);
}

export async function assignServerAllocation(serverId: string, allocationId: string): Promise<void> {
  await postJSON<void>(`/servers/${encodeURIComponent(serverId)}/allocations`, { allocationId });
}

export async function unassignServerAllocation(serverId: string, allocationId: string): Promise<void> {
  await deleteJSON(`/servers/${encodeURIComponent(serverId)}/allocations/${encodeURIComponent(allocationId)}`);
}

export async function setPrimaryServerAllocation(serverId: string, allocationId: string): Promise<void> {
  await postJSON<void>(`/servers/${encodeURIComponent(serverId)}/allocations/${encodeURIComponent(allocationId)}/primary`);
}

export async function updateServerAllocation(
  serverId: string,
  allocationId: string,
  data: { alias?: string; notes?: string },
): Promise<void> {
  await patchJSON<void>(`/servers/${encodeURIComponent(serverId)}/allocations/${encodeURIComponent(allocationId)}`, data);
}

// Server databases
export async function fetchServerDatabases(serverId: string): Promise<ApiDatabase[]> {
  return fetchJSON<ApiDatabase[]>(`/servers/${encodeURIComponent(serverId)}/databases`);
}

export async function createServerDatabase(serverId: string, input: DatabaseCreateInput): Promise<ApiDatabase> {
  return postJSON<ApiDatabase>(`/servers/${encodeURIComponent(serverId)}/databases`, input);
}

export async function deleteServerDatabase(serverId: string, databaseId: string, force = false): Promise<ApiServerDatabaseDeleteResult> {
  const query = force ? "?force=true" : "";
  return deleteJSON<ApiServerDatabaseDeleteResult>(`/servers/${encodeURIComponent(serverId)}/databases/${encodeURIComponent(databaseId)}${query}`);
}

export async function fetchOrphanRemediations(status?: "pending" | "resolved"): Promise<ApiOrphanRemediations> {
  const query = status ? `?status=${encodeURIComponent(status)}` : "";
  return fetchJSON<ApiOrphanRemediations>(`/admin/orphan-remediations${query}`);
}

export async function resolveDatabaseOrphanRemediation(id: string): Promise<ApiDatabaseOrphanRemediation> {
  return postJSON<ApiDatabaseOrphanRemediation>(`/admin/orphan-remediations/databases/${encodeURIComponent(id)}/resolve`);
}

export async function resolveServerOrphanRemediation(id: string): Promise<ApiServerOrphanRemediation> {
  return postJSON<ApiServerOrphanRemediation>(`/admin/orphan-remediations/servers/${encodeURIComponent(id)}/resolve`);
}

export async function rotateServerDatabasePassword(serverId: string, databaseId: string): Promise<{ password: string }> {
  return postJSON<{ password: string }>(`/servers/${encodeURIComponent(serverId)}/databases/${encodeURIComponent(databaseId)}/rotate-password`);
}

// Server backups

export async function fetchBackups(serverId: string, page = 1, perPage = 20): Promise<{ data: ApiBackup[]; pagination: { page: number; per_page: number; total: number; total_pages: number } }> {
  return fetchJSON<{ data: ApiBackup[]; pagination: { page: number; per_page: number; total: number; total_pages: number } }>(`/servers/${encodeURIComponent(serverId)}/backups?page=${page}&per_page=${perPage}`);
}

export async function createBackup(serverId: string, input?: BackupCreateInput): Promise<ApiBackup> {
  return postJSON<ApiBackup>(`/servers/${encodeURIComponent(serverId)}/backups`, input || {});
}

export async function deleteBackup(serverId: string, backupId: string): Promise<void> {
  await deleteJSON(`/servers/${encodeURIComponent(serverId)}/backups/${encodeURIComponent(backupId)}`);
}

export async function lockBackup(serverId: string, backupId: string): Promise<void> {
  await postJSON<void>(`/servers/${encodeURIComponent(serverId)}/backups/${encodeURIComponent(backupId)}/lock`);
}

export async function unlockBackup(serverId: string, backupId: string): Promise<void> {
  await postJSON<void>(`/servers/${encodeURIComponent(serverId)}/backups/${encodeURIComponent(backupId)}/unlock`);
}

export async function downloadBackup(serverId: string, backupId: string): Promise<Blob> {
  return requestBlob(
    `/servers/${encodeURIComponent(serverId)}/backups/download?name=${encodeURIComponent(backupId)}`,
  );
}

export async function restoreBackup(serverId: string, backupId: string, truncate?: boolean): Promise<{ ok: boolean; status: string; name: string }> {
  return postJSON<{ ok: boolean; status: string; name: string }>(
    `/servers/${encodeURIComponent(serverId)}/backups/restore`,
    { name: backupId, truncate }
  );
}

// Server schedules
export async function fetchServerSchedules(serverId: string): Promise<ApiSchedule[]> {
  return fetchJSON<ApiSchedule[]>(`/servers/${encodeURIComponent(serverId)}/schedules`);
}

export async function createServerSchedule(serverId: string, input: ScheduleCreateInput): Promise<ApiSchedule> {
  return postJSON<ApiSchedule>(`/servers/${encodeURIComponent(serverId)}/schedules`, input);
}

export async function updateServerSchedule(serverId: string, scheduleId: string, input: ScheduleUpdateInput): Promise<ApiSchedule> {
  return patchJSON<ApiSchedule>(`/servers/${encodeURIComponent(serverId)}/schedules/${encodeURIComponent(scheduleId)}`, input);
}

export async function deleteServerSchedule(serverId: string, scheduleId: string): Promise<void> {
  await deleteJSON(`/servers/${encodeURIComponent(serverId)}/schedules/${encodeURIComponent(scheduleId)}`);
}

export async function fetchServerScheduleTasks(
  serverId: string,
  scheduleId: string,
): Promise<ApiScheduleTask[]> {
  return fetchJSON<ApiScheduleTask[]>(
    `/servers/${encodeURIComponent(serverId)}/schedules/${encodeURIComponent(scheduleId)}/tasks`,
  );
}

export async function createServerScheduleTask(
  serverId: string,
  scheduleId: string,
  input: ScheduleTaskCreateInput,
): Promise<ApiScheduleTask> {
  return postJSON<ApiScheduleTask>(
    `/servers/${encodeURIComponent(serverId)}/schedules/${encodeURIComponent(scheduleId)}/tasks`,
    input,
  );
}

export async function updateServerScheduleTask(
  serverId: string,
  scheduleId: string,
  taskId: string,
  input: ScheduleTaskUpdateInput,
): Promise<ApiScheduleTask> {
  return patchJSON<ApiScheduleTask>(
    `/servers/${encodeURIComponent(serverId)}/schedules/${encodeURIComponent(scheduleId)}/tasks/${encodeURIComponent(taskId)}`,
    input,
  );
}

export async function deleteServerScheduleTask(
  serverId: string,
  scheduleId: string,
  taskId: string,
): Promise<void> {
  await deleteJSON(
    `/servers/${encodeURIComponent(serverId)}/schedules/${encodeURIComponent(scheduleId)}/tasks/${encodeURIComponent(taskId)}`,
  );
}

// Server startup
export type { ApiStartupVariable } from './types';

export interface ServerStartup {
  startupCommand: string;
  rawStartupCommand: string;
  dockerImages: Record<string, string>;
  variables: ServerStartupVariable[];
  startup_command?: string;
  raw_startup_command?: string;
  docker_images?: Record<string, string>;
}

export async function fetchServerStartup(serverId: string): Promise<ServerStartup> {
  return fetchJSON<ServerStartup>(`/servers/${encodeURIComponent(serverId)}/startup`);
}

export async function updateServerStartupVariable(
	serverId: string,
	key: string,
	value: string,
): Promise<void> {
	await putJSON<void>(`/servers/${encodeURIComponent(serverId)}/startup/variable`, {
		key,
		value,
	});
}

export async function updateServerStartupCommand(serverId: string, command: string): Promise<void> {
  await patchJSON<void>(`/servers/${encodeURIComponent(serverId)}/startup/command`, { command });
}

export async function updateServerDockerImage(serverId: string, image: string): Promise<void> {
  await patchJSON<void>(`/servers/${encodeURIComponent(serverId)}/startup/image`, { image });
}

export async function getBackupDownloadURL(serverId: string, backupId: string): Promise<{ url: string }> {
  const ticket = await postJSON<{ token: string }>(
    `/servers/${encodeURIComponent(serverId)}/backups/download-ticket`,
    { name: backupId }
  );
  return { url: `${getApiBaseUrl()}/download/file?token=${encodeURIComponent(ticket.token)}` };
}

export interface ActivityPage {
  data: ApiAuditEvent[];
  pagination: { page: number; per_page: number; total: number; total_pages: number };
}

export async function fetchServerActivity(
  serverId: string,
  page = 1,
  perPage = 50,
): Promise<ActivityPage> {
  // The backend has no server-side pagination for this route
  // (handlers_servers.go returns the full event list, capped at 100) — the
  // page/per_page query is accepted for backward compatibility but ignored
  // server-side. The returned `pagination` block is an honest client-side
  // synthesis over the complete list, not a backend cursor; ActivityView
  // paginates locally and labels it as such.
  const response = await fetchJSON<ApiAuditEvent[] | ActivityPage>(
    `/servers/${encodeURIComponent(serverId)}/activity?page=${page}&per_page=${perPage}`,
  );
  if (Array.isArray(response)) {
    return {
      data: response,
      pagination: { page: 1, per_page: response.length, total: response.length, total_pages: 1 },
    };
  }
  if (response && typeof response === 'object' && Array.isArray((response as ActivityPage).data)) {
    return response as ActivityPage;
  }
  return { data: [], pagination: { page: 1, per_page: 0, total: 0, total_pages: 1 } };
}

export async function fetchServerUsers(serverId: string): Promise<ApiServerSubuser[]> {
  return fetchJSON<ApiServerSubuser[]>(`/servers/${encodeURIComponent(serverId)}/users`);
}

export async function fetchServerSubuser(
  serverId: string,
  userId: string,
): Promise<ApiServerSubuser> {
  return fetchJSON<ApiServerSubuser>(
    `/servers/${encodeURIComponent(serverId)}/users/${encodeURIComponent(userId)}`,
  );
}

export async function deleteServerUser(
  serverId: string,
  userId: string,
): Promise<void> {
  await deleteJSON(`/servers/${encodeURIComponent(serverId)}/users/${encodeURIComponent(userId)}`);
}

export async function updateServerUser(
  serverId: string,
  userId: string,
  data: { permissions: string[] },
): Promise<ApiServerSubuser> {
  return patchJSON<ApiServerSubuser>(
    `/servers/${encodeURIComponent(serverId)}/users/${encodeURIComponent(userId)}`,
    data,
  );
}

export async function upsertServerUser(
  serverId: string,
  data: { email: string; permissions: string[] },
): Promise<ApiServerSubuser> {
  return postJSON<ApiServerSubuser>(
    `/servers/${encodeURIComponent(serverId)}/users`,
    data,
  );
}

// Crash detection
export async function fetchServerCrashHistory(serverId: string): Promise<CrashEvent[]> {
  const res = await fetchJSON<{ events: CrashEvent[] }>(`/admin/crash-detection/servers/${encodeURIComponent(serverId)}`);
  return res.events ?? [];
}

export async function resetServerCrashState(serverId: string): Promise<{ ok: boolean }> {
  return postJSON(`/admin/crash-detection/servers/${encodeURIComponent(serverId)}/reset`, {});
}

export async function fetchServerTransferStatus(
  serverId: string,
): Promise<ApiLegacyTransferStatus | null> {
  try {
    // Backend shape (handlers_servers.go): { state, transferring, targetNodeId, error }.
    return await fetchJSON<ApiLegacyTransferStatus>(
      `/servers/${encodeURIComponent(serverId)}/transfer`,
    );
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) return null;
    throw error;
  }
}

export async function cancelServerTransfer(serverId: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/servers/${encodeURIComponent(serverId)}/transfer/cancel`);
}

export async function transferServer(
  serverId: string,
  targetNodeId: string,
  primaryAllocationId?: string,
): Promise<{ ok: boolean; transferId?: string }> {
  return postJSON<{ ok: boolean; transferId?: string }>(
    `/servers/${encodeURIComponent(serverId)}/transfer`,
    { targetNodeId, primaryAllocationId },
  );
}

// Procfile process management

export async function fetchProcesses(serverId: string): Promise<ProcessType[]> {
  return fetchJSON<ProcessType[]>(`/servers/${encodeURIComponent(serverId)}/processes`);
}

export async function setProcesses(serverId: string, processes: ProcfileEntry[]): Promise<ProcessType[]> {
  return postJSON<ProcessType[]>(`/servers/${encodeURIComponent(serverId)}/processes`, { processes });
}

export async function scaleProcess(serverId: string, processType: string, quantity: number): Promise<ProcessType> {
  return putJSON<ProcessType>(`/servers/${encodeURIComponent(serverId)}/processes/${encodeURIComponent(processType)}/scale`, { quantity });
}

export async function runOneOffTask(serverId: string, command: string): Promise<OneOffTask> {
  return postJSON<OneOffTask>(`/servers/${encodeURIComponent(serverId)}/processes/run`, { command });
}

export async function fetchOneOffTasks(serverId: string): Promise<OneOffTask[]> {
  return fetchJSON<OneOffTask[]>(`/servers/${encodeURIComponent(serverId)}/processes/tasks`);
}

export async function fetchScalingHistory(serverId: string): Promise<ProcessScalingEvent[]> {
  return fetchJSON<ProcessScalingEvent[]>(`/servers/${encodeURIComponent(serverId)}/processes/history`);
}

export async function parseProcfile(serverId: string, content: string): Promise<ProcfileEntry[]> {
  return postJSON<ProcfileEntry[]>(`/servers/${encodeURIComponent(serverId)}/processes/parse-procfile`, { content });
}
