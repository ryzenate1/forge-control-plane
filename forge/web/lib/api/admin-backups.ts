import { fetchJSON, getAuthHeaders, getCSRFToken, postJSON, patchJSON, deleteJSON, API_BASE_URL } from "./http";

export interface BackupConfiguration {
  id: string;
  name: string;
  description: string;
  backupType: "app" | "volume" | "database" | "server";
  serverId?: string;
  appId?: string;
  databaseId?: string;
  volumeId?: string;
  isScheduled: boolean;
  cronExpression: string;
  storageProvider: string;
  maxBackups: number;
  retentionDays: number;
  compressionEnabled: boolean;
  encryptionEnabled: boolean;
  enabled: boolean;
  lastStatus: string;
  lastRunAt: string | null;
  nextRunAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface BackupJob {
  id: string;
  name: string;
  jobType: "app" | "volume" | "database" | "server" | "manual";
  status: "pending" | "running" | "completed" | "failed" | "cancelled";
  progress: number;
  bytesProcessed: number;
  totalBytes: number;
  startedAt: string | null;
  completedAt: string | null;
  triggeredBy: string;
  createdAt: string;
}

export interface CreateBackupJobRequest {
  name: string;
  jobType: "app" | "volume" | "database" | "server";
  serverId?: string;
  appId?: string;
  databaseId?: string;
  volumeId?: string;
  description?: string;
}

export interface BackupArtifact {
  id: string;
  name: string;
  displayName: string;
  artifactType: "app" | "volume" | "database" | "server";
  storageProvider: string;
  fileSize: number;
  fileHash: string;
  status: string;
  isVerified: boolean;
  isLocked: boolean;
  createdAt: string;
  uploadedAt: string | null;
}

export interface BackupRestore {
  id: string;
  name: string;
  description?: string;
  overwrite?: boolean;
  restoreType: "app" | "volume" | "database" | "server";
  status: "pending" | "running" | "completed" | "failed" | "cancelled";
  progress: number;
  startedAt: string | null;
  completedAt: string | null;
  triggeredBy: string;
  createdAt: string;
}

export interface StorageProvider {
  id: string;
  name: string;
  type: string;
  enabled: boolean;
  isDefault: boolean;
}

export interface BackupSystemStatus {
  backupConfigurations: {
    total: number;
    scheduled: number;
    enabled: number;
  };
  backupJobs: {
    total: number;
    running: number;
    pending: number;
    failed: number;
  };
  backupArtifacts: {
    total: number;
    verified: number;
    locked: number;
    expired: number;
  };
  backupRestores: {
    total: number;
    completed: number;
    failed: number;
  };
  storageProviders: number;
}

export function fetchBackupConfigs(): Promise<BackupConfiguration[]> {
  return fetchJSON<BackupConfiguration[]>("/admin/backups/configs");
}

export function createBackupConfig(data: Partial<BackupConfiguration>): Promise<BackupConfiguration> {
  return postJSON<BackupConfiguration>("/admin/backups/configs", data);
}

export function updateBackupConfig(id: string, data: Partial<BackupConfiguration>): Promise<BackupConfiguration> {
  return patchJSON<BackupConfiguration>(`/admin/backups/configs/${encodeURIComponent(id)}`, data);
}

export function deleteBackupConfig(id: string): Promise<void> {
  return deleteJSON(`/admin/backups/configs/${encodeURIComponent(id)}`);
}

export function executeBackupConfig(id: string): Promise<BackupJob> {
  return postJSON<BackupJob>(`/admin/backups/configs/${encodeURIComponent(id)}/execute`);
}

export function fetchBackupJobs(): Promise<BackupJob[]> {
  return fetchJSON<BackupJob[]>("/admin/backups/jobs");
}

export function createBackupJob(data: CreateBackupJobRequest): Promise<BackupJob> {
  return postJSON<BackupJob>("/admin/backups/jobs", data);
}

export async function cancelBackupJob(id: string): Promise<void> {
  await postJSON(`/admin/backups/jobs/${encodeURIComponent(id)}/cancel`);
}

export function deleteBackupJob(id: string): Promise<void> {
  return deleteJSON(`/admin/backups/jobs/${encodeURIComponent(id)}`);
}

export function fetchBackupArtifacts(): Promise<BackupArtifact[]> {
  return fetchJSON<BackupArtifact[]>("/admin/backups/artifacts");
}

export function deleteBackupArtifact(id: string): Promise<void> {
  return deleteJSON(`/admin/backups/artifacts/${encodeURIComponent(id)}`);
}

export async function lockBackupArtifact(id: string, reason: string): Promise<void> {
  await postJSON(`/admin/backups/artifacts/${encodeURIComponent(id)}/lock`, { reason });
}

export async function unlockBackupArtifact(id: string): Promise<void> {
  await postJSON(`/admin/backups/artifacts/${encodeURIComponent(id)}/unlock`);
}

export async function downloadBackupArtifact(id: string): Promise<Blob> {
  const headers = new Headers(getAuthHeaders());
  const csrfToken = getCSRFToken();
  if (csrfToken) headers.set("X-CSRF-Token", csrfToken);
  const response = await fetch(`${API_BASE_URL}/admin/backups/artifacts/${encodeURIComponent(id)}/download`, {
    credentials: "include",
    headers,
  });
  if (!response.ok) throw new Error("Failed to download backup artifact");
  return response.blob();
}

export function fetchBackupRestores(): Promise<BackupRestore[]> {
  return fetchJSON<BackupRestore[]>("/admin/backups/restores");
}

export function createRestore(data: Partial<BackupRestore> & { artifactId: string }): Promise<BackupRestore> {
  return postJSON<BackupRestore>("/admin/backups/restore", data);
}

export function deleteBackupRestore(id: string): Promise<void> {
  return deleteJSON(`/admin/backups/restores/${encodeURIComponent(id)}`);
}

export function fetchBackupStorageProviders(): Promise<StorageProvider[]> {
  return fetchJSON<StorageProvider[]>("/admin/backups/storage-providers");
}

export function fetchBackupSystemStatus(): Promise<BackupSystemStatus> {
  return fetchJSON<BackupSystemStatus>("/admin/backups/status");
}
