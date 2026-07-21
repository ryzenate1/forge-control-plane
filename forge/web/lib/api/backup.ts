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
  enabled?: boolean;
};

export type BackupProvider = {
  name: string;
};

export type BackupProvidersResponse = {
  providers: string[];
};

export function listBackupProviders(): Promise<BackupProvidersResponse> {
  return fetchJSON<BackupProvidersResponse>('/backup/providers');
}

export function listBackupPolicies(serverId: string): Promise<{ policies: BackupPolicy[] }> {
  return fetchJSON<{ policies: BackupPolicy[] }>(`/servers/${encodeURIComponent(serverId)}/backups/policies`);
}

export function createBackupPolicy(serverId: string, config: BackupPolicyConfig): Promise<{ policy: BackupPolicy }> {
  return postJSON<{ policy: BackupPolicy }>(`/servers/${encodeURIComponent(serverId)}/backups/policies`, config);
}

export function deleteBackupPolicy(serverId: string, policyId: string): Promise<void> {
  return deleteJSON(`/servers/${encodeURIComponent(serverId)}/backups/policies/${encodeURIComponent(policyId)}`);
}

export function triggerBackup(serverId: string, policyId: string): Promise<{ uuid: string; name: string }> {
  return postJSON<{ uuid: string; name: string }>(`/servers/${encodeURIComponent(serverId)}/backups`, { policyId });
}

export function cleanupExpiredBackups(serverId: string): Promise<{ ok: boolean; cleaned: number }> {
  return postJSON<{ ok: boolean; cleaned: number }>(`/servers/${encodeURIComponent(serverId)}/backups/cleanup`);
}
