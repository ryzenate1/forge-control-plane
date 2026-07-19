// Server management API functions
import { fetchJSON, postJSON, putJSON, patchJSON, deleteJSON, API_BASE_URL, getAuthHeaders } from './http';
import type {
  ApiServerSubuser,
  ApiAuditEvent,
  ApiDatabaseOrphanRemediation,
  ApiOrphanRemediations,
  ApiServerOrphanRemediation,
  CrashEvent,
} from './types';
import type { ApiServer, ApiAllocation, ApiDatabase, ApiBackup, ApiSchedule, ApiScheduleTask, BackupCreateInput, ServerCreateInput, ServerUpdateInput, DatabaseCreateInput, ScheduleCreateInput, ScheduleUpdateInput, ScheduleTaskCreateInput, ScheduleTaskUpdateInput } from './types';

export async function fetchServers(): Promise<ApiServer[]> {
  return fetchJSON<ApiServer[]>('/servers');
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
  await postJSON<any>(`/servers/${encodeURIComponent(serverId)}/command`, {
    command,
  });
}

export async function sendPowerSignal(
  serverId: string,
  signal: 'start' | 'stop' | 'restart' | 'kill',
): Promise<{ serverId: string; signal: string; accepted: boolean }> {
  return postJSON<{ serverId: string; signal: string; accepted: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/power`,
    { signal },
  );
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

export async function deleteServerDatabase(serverId: string, databaseId: string, force = false): Promise<void> {
  const query = force ? "?force=true" : "";
  await deleteJSON(`/servers/${encodeURIComponent(serverId)}/databases/${encodeURIComponent(databaseId)}${query}`);
}

export async function fetchOrphanRemediations(status?: "pending" | "resolved"): Promise<ApiOrphanRemediations> {
  const query = status ? `?status=${encodeURIComponent(status)}` : "";
  return fetchJSON<ApiOrphanRemediations>(`/admin/orphan-remediations/${query}`);
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
  const response = await fetch(`${API_BASE_URL}/servers/${encodeURIComponent(serverId)}/backups/download?name=${encodeURIComponent(backupId)}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw new Error(`Failed to download backup: ${response.status}`);
  return response.blob();
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
export async function fetchServerStartup(serverId: string): Promise<any> {
  return fetchJSON<any>(`/servers/${encodeURIComponent(serverId)}/startup`);
}

export async function updateServerStartupVariable(
  serverId: string,
  variableId: string,
  value: string,
): Promise<void> {
  await putJSON<void>(`/servers/${encodeURIComponent(serverId)}/startup/variable`, {
    variableId,
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
  return { url: `${API_BASE_URL}/download/file?token=${encodeURIComponent(ticket.token)}` };
}

export async function fetchServerActivity(
  serverId: string,
  page = 1,
  perPage = 50,
): Promise<{ data: ApiAuditEvent[]; pagination: { page: number; per_page: number; total: number; total_pages: number } }> {
  return fetchJSON<{ data: ApiAuditEvent[]; pagination: { page: number; per_page: number; total: number; total_pages: number } }>(
    `/servers/${encodeURIComponent(serverId)}/activity?page=${page}&per_page=${perPage}`,
  );
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
  return fetchJSON<CrashEvent[]>(`/admin/crash-detection/servers/${encodeURIComponent(serverId)}`);
}

export async function resetServerCrashState(serverId: string): Promise<{ ok: boolean }> {
  return postJSON(`/admin/crash-detection/servers/${encodeURIComponent(serverId)}/reset`, {});
}

export async function fetchServerTransferStatus(
  serverId: string,
): Promise<{ transferring: boolean; transferId?: string; status?: string; progress?: number } | null> {
  try {
    return await fetchJSON<{ transferring: boolean; transferId?: string; status?: string; progress?: number }>(
      `/servers/${encodeURIComponent(serverId)}/transfer`,
    );
  } catch {
    return null;
  }
}

export async function cancelServerTransfer(serverId: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/servers/${encodeURIComponent(serverId)}/transfer/cancel`);
}

export async function transferServer(
  serverId: string,
  targetNodeId: string,
): Promise<{ ok: boolean; transferId?: string }> {
  return postJSON<{ ok: boolean; transferId?: string }>(
    `/servers/${encodeURIComponent(serverId)}/transfer`,
    { targetNodeId },
  );
}
