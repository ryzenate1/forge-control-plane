import { deleteJSON, fetchJSON, postJSON } from "./http";

// Restic / Kopia backup engines. Mirrors the Go types in
// internal/services/backupengine and the /admin/backup-engines routes.

export type BackupEngine = "restic" | "kopia";

export interface BackupEnginePrunePolicy {
  /** Standard 5-field cron expression; empty disables scheduled snapshots. */
  schedule: string;
  keepLast: number;
  keepHourly: number;
  keepDaily: number;
  keepWeekly: number;
  keepMonthly: number;
  /** Run a garbage collection pass after forgetting expired snapshots. */
  prune: boolean;
  /** Default paths snapshotted by the cron schedule. */
  paths: string[];
}

export interface BackupEngineRepository {
  id: string;
  name: string;
  engine: BackupEngine;
  /** Backend URL or path: s3:…, rest:…, sftp:…, /mnt/backups, … */
  location: string;
  /**
   * Non-secret descriptor: either an `env:VAR` reference or a marker that a
   * sealed password is stored server-side. The password itself never leaves
   * the API.
   */
  passwordRef?: string;
  encryption: string;
  prunePolicy: Partial<BackupEnginePrunePolicy>;
  nodeId?: string;
  serverId?: string;
  artifactId?: string;
  initialized: boolean;
  lastSnapshotAt?: string | null;
  nextRunAt?: string | null;
  lastError?: string;
  createdAt: string;
}

export interface BackupEngineSnapshot {
  id: string;
  repoId: string;
  /** Opaque snapshot identifier as reported by the CLI. */
  snapshotId: string;
  paths: string[];
  hostname: string;
  timestamp?: string | null;
  parent?: string;
  tree?: string;
  dataFiles: number;
  totalSizeBytes: number;
  verifiedAt?: string | null;
}

export type BackupEngineRestoreStatus = "pending" | "running" | "completed" | "failed" | "cancelled";

export interface BackupEngineRestoreJob {
  id: string;
  repoId: string;
  snapshotId: string;
  targetPath: string;
  status: BackupEngineRestoreStatus;
  progressPct: number;
  startedAt?: string | null;
  completedAt?: string | null;
  error?: string;
  createdAt: string;
}

export interface AddBackupRepositoryRequest {
  name: string;
  engine: BackupEngine;
  location: string;
  /** Repository password, or an `env:VAR` name resolved on the target node. */
  password?: string;
  encryption?: string;
  prunePolicy?: Partial<BackupEnginePrunePolicy>;
  nodeId?: string;
  serverId?: string;
  artifactId?: string;
  initialized?: boolean;
}

/** Endpoint returns `{"data": […]}` envelopes; keep the unwrapping here. */
async function unwrapList<T>(promise: Promise<{ data?: T[] } | undefined>): Promise<T[]> {
  const response = await promise;
  return response?.data ?? [];
}

export function fetchBackupRepositories(): Promise<BackupEngineRepository[]> {
  return unwrapList(fetchJSON<{ data?: BackupEngineRepository[] }>("/admin/backup-engines/repositories"));
}

export function addBackupRepository(data: AddBackupRepositoryRequest): Promise<BackupEngineRepository> {
  return postJSON<BackupEngineRepository>("/admin/backup-engines/repositories", data);
}

export async function removeBackupRepository(id: string): Promise<void> {
  await deleteJSON(`/admin/backup-engines/repositories/${encodeURIComponent(id)}`);
}

export async function testBackupRepository(id: string): Promise<void> {
  await postJSON(`/admin/backup-engines/repositories/${encodeURIComponent(id)}/test`);
}

export async function initBackupRepository(id: string): Promise<void> {
  await postJSON(`/admin/backup-engines/repositories/${encodeURIComponent(id)}/init`);
}

export function fetchBackupSnapshots(repoId: string): Promise<BackupEngineSnapshot[]> {
  return unwrapList(
    fetchJSON<{ data?: BackupEngineSnapshot[] }>(
      `/admin/backup-engines/snapshots?repoId=${encodeURIComponent(repoId)}`,
    ),
  );
}

export function createBackupSnapshot(repoId: string, paths: string[]): Promise<BackupEngineSnapshot> {
  return postJSON<BackupEngineSnapshot>("/admin/backup-engines/snapshots", { repoId, paths });
}

export async function verifyBackupSnapshot(repoId: string, snapshotId: string): Promise<void> {
  await postJSON("/admin/backup-engines/snapshots/verify", { repoId, snapshotId });
}

export async function pruneBackupRepository(repoId: string): Promise<void> {
  await postJSON("/admin/backup-engines/prune", { repoId });
}

export function restoreBackupSnapshot(
  repoId: string,
  snapshotId: string,
  targetPath: string,
): Promise<BackupEngineRestoreJob> {
  return postJSON<BackupEngineRestoreJob>("/admin/backup-engines/restore", { repoId, snapshotId, targetPath });
}

export function fetchBackupRestoreJobs(repoId?: string): Promise<BackupEngineRestoreJob[]> {
  const query = repoId ? `?repoId=${encodeURIComponent(repoId)}` : "";
  return unwrapList(
    fetchJSON<{ data?: BackupEngineRestoreJob[] }>(`/admin/backup-engines/restore${query}`),
  );
}

export const defaultBackupEnginePrunePolicy: BackupEnginePrunePolicy = {
  schedule: "0 3 * * *",
  keepLast: 14,
  keepHourly: 0,
  keepDaily: 7,
  keepWeekly: 4,
  keepMonthly: 6,
  prune: true,
  paths: [],
};

export function decodeBackupEnginePrunePolicy(
  policy: Partial<BackupEnginePrunePolicy> | undefined,
): BackupEnginePrunePolicy {
  return { ...defaultBackupEnginePrunePolicy, ...(policy ?? {}) };
}

export function backupEngineLabel(engine: BackupEngine | string): string {
  return engine === "kopia" ? "Kopia" : "Restic";
}
