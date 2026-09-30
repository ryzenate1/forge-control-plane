import { deleteJSON, fetchJSON, patchJSON, postJSON } from "./http";

export interface BackupPolicy {
  id: string;
  serverId: string;
  appId?: string;
  serviceId?: string;
  databaseId?: string;
  databaseType?: string;
  volumeBackup: boolean;
  interval: string;
  maxBackups: number;
  retentionDays: number;
  storage: string;
  compress: boolean;
  encrypted: boolean;
  encryptionAlgorithm: string;
  enabled: boolean;
  isLocked: boolean;
  nextRunAt?: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface CreateBackupPolicyInput {
  interval: string;
  maxBackups?: number;
  retentionDays?: number;
  storage?: string;
  compress?: boolean;
  encrypted?: boolean;
  encryptionAlgorithm?: string;
  encryptionKey?: string;
  enabled?: boolean;
}

export interface UpdateBackupPolicyInput {
  interval?: string;
  maxBackups?: number;
  retentionDays?: number;
  storage?: string;
  compress?: boolean;
  encrypted?: boolean;
  encryptionAlgorithm?: string;
  enabled?: boolean;
}

export interface BackupPolicyList {
  policies: BackupPolicy[];
  total: number;
  limit: number;
  offset: number;
}

export async function listBackupPolicies(serverId: string): Promise<BackupPolicy[]> {
  const res = await fetchJSON<{ policies: BackupPolicy[]; total: number; limit: number; offset: number }>(
    `/servers/${encodeURIComponent(serverId)}/backups/policies`,
  );
  return res.policies;
}

export async function createBackupPolicy(serverId: string, input: CreateBackupPolicyInput): Promise<{ policy: BackupPolicy }> {
  return postJSON<{ policy: BackupPolicy }>(
    `/servers/${encodeURIComponent(serverId)}/backups/policies`,
    input,
  );
}

export async function getBackupPolicy(serverId: string, policyId: string): Promise<{ policy: BackupPolicy }> {
  const res = await fetchJSON<{ policy: BackupPolicy }>(
    `/servers/${encodeURIComponent(serverId)}/backups/policies/${encodeURIComponent(policyId)}`,
  );
  return res;
}

export async function updateBackupPolicy(serverId: string, policyId: string, input: UpdateBackupPolicyInput): Promise<{ policy: BackupPolicy }> {
  return patchJSON<{ policy: BackupPolicy }>(
    `/servers/${encodeURIComponent(serverId)}/backups/policies/${encodeURIComponent(policyId)}`,
    input,
  );
}

export async function deleteBackupPolicy(serverId: string, policyId: string): Promise<{ ok: boolean }> {
  return deleteJSON<{ ok: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/backups/policies/${encodeURIComponent(policyId)}`,
  );
}

export async function lockBackupPolicy(serverId: string, policyId: string): Promise<{ ok: boolean; locked: boolean }> {
  return postJSON<{ ok: boolean; locked: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/backups/policies/${encodeURIComponent(policyId)}/lock`,
    {},
  );
}

export async function unlockBackupPolicy(serverId: string, policyId: string): Promise<{ ok: boolean; locked: boolean }> {
  return postJSON<{ ok: boolean; locked: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/backups/policies/${encodeURIComponent(policyId)}/unlock`,
    {},
  );
}
