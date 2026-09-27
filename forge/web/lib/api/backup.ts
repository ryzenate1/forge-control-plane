import { fetchJSON, postJSON, deleteJSON } from './http';

export type BackupPolicy = {
  id: string;
  serverId: string;
  interval: string;
  maxBackups: number;
  retentionDays: number;
  storage: string;
  compress: boolean;
  encryptionKey?: string;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
};

export type BackupPolicyConfig = {
  interval: string;
  maxBackups: number;
  retentionDays: number;
  storage: string;
  compress?: boolean;
  encrypted?: boolean;
  encryptionAlgorithm?: string;
  encryptionKey?: string;
  enabled?: boolean;
};

export type BackupProvider = {
  name: string;
};

export type BackupProvidersResponse = {
  providers: string[];
};

export type BackupPoliciesResponse = {
  policies: BackupPolicy[];
  total: number;
  limit: number;
  offset: number;
};

export function listBackupProviders(): Promise<BackupProvidersResponse> {
  return fetchJSON<BackupProvidersResponse>('/backup/providers');
}

export function listBackupPolicies(serverId: string): Promise<BackupPoliciesResponse> {
  return fetchJSON<BackupPoliciesResponse>(`/servers/${encodeURIComponent(serverId)}/backups/policies`);
}

export function createBackupPolicy(serverId: string, config: BackupPolicyConfig): Promise<{ policy: BackupPolicy }> {
  return postJSON<{ policy: BackupPolicy }>(`/servers/${encodeURIComponent(serverId)}/backups/policies`, config);
}

export function deleteBackupPolicy(serverId: string, policyId: string): Promise<void> {
  return deleteJSON(`/servers/${encodeURIComponent(serverId)}/backups/policies/${encodeURIComponent(policyId)}`);
}

export function lockBackupPolicy(serverId: string, policyId: string): Promise<{ ok: boolean; locked: boolean }> {
  return postJSON<{ ok: boolean; locked: boolean }>(`/servers/${encodeURIComponent(serverId)}/backups/policies/${encodeURIComponent(policyId)}/lock`);
}

export function unlockBackupPolicy(serverId: string, policyId: string): Promise<{ ok: boolean; locked: boolean }> {
  return postJSON<{ ok: boolean; locked: boolean }>(`/servers/${encodeURIComponent(serverId)}/backups/policies/${encodeURIComponent(policyId)}/unlock`);
}

export function triggerBackup(serverId: string, ignored?: string[]): Promise<{ uuid: string; name: string; status: string }> {
  return postJSON<{ uuid: string; name: string; status: string }>(`/servers/${encodeURIComponent(serverId)}/backups`, { ignored });
}

export { createBackup } from './servers';

/**
 * Applies retention (age limit and count limit) to one server's backups.
 *
 * The response field is `deleted`. This used to be typed as `cleaned`, which is
 * what the installation-wide sweep returns — that handler was registered on
 * this same path and shadowed by the per-server one, so the type described a
 * route that never answered and `result.cleaned` was always undefined.
 * The global sweep now lives at POST /admin/backups/cleanup.
 */
export function cleanupServerBackups(serverId: string): Promise<{ ok: boolean; deleted: number }> {
  return postJSON<{ ok: boolean; deleted: number }>(`/servers/${encodeURIComponent(serverId)}/backups/cleanup`);
}
