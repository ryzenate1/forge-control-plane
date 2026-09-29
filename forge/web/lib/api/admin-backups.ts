import { fetchJSON, postJSON, patchJSON, deleteJSON, requestBlob } from "./http";

// The field names below mirror the Go JSON tags in
// forge/api/internal/services/backup/{config,job,artifact,restore}.go — read them
// against the handler, not against the UI, because a field that does not exist on
// the wire reads as `undefined` and silently becomes a missing measurement.
// Optional/nullable here means "the control plane does not always report this";
// callers must render it as unknown, never as zero.

export interface BackupConfiguration {
  id: string;
  name: string;
  description?: string;
  backupType: "app" | "volume" | "database" | "server";
  serverId?: string;
  appId?: string;
  databaseId?: string;
  volumeId?: string;
  isScheduled: boolean;
  /** `cronExpression,omitempty` — absent for manual-only policies. */
  cronExpression?: string | null;
  storageProvider: string;
  maxBackups: number;
  retentionDays: number;
  compressionEnabled: boolean;
  encryptionEnabled: boolean;
  enabled: boolean;
  lastStatus?: string | null;
  lastError?: string | null;
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
  /** Go: `progressPercentage`. There is no `progress` field on the wire. */
  progressPercentage: number;
  bytesProcessed: number;
  /** `totalBytes,omitempty` — the denominator is often not reported. */
  totalBytes?: number | null;
  currentPhase?: string;
  errorMessage?: string | null;
  startedAt: string | null;
  completedAt: string | null;
  durationSeconds?: number | null;
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
  displayName?: string;
  artifactType: "app" | "volume" | "database" | "server";
  storageProvider: string;
  /**
   * Go `int64` with no `omitempty`, so an artifact registered before its upload
   * lands arrives as `0`. That is "size not measured yet", not "empty backup" —
   * renderers must treat 0/absent as unknown unless the artifact is uploaded.
   */
  fileSize: number;
  fileHash?: string | null;
  hashAlgorithm?: string;
  status: string;
  isVerified: boolean;
  /**
   * Incremented by the API on every failed integrity check and reset to 0 on
   * success (`artifact.go:561-568`). `attempts > 0 && !isVerified` therefore
   * means "checked and bad" — distinct from "never checked".
   */
  verificationAttempts: number;
  lastVerifiedAt?: string | null;
  isLocked: boolean;
  lockReason?: string | null;
  expiresAt?: string | null;
  /** Restore targets default to the artifact's own source resource. */
  sourceServerId?: string | null;
  sourceAppId?: string | null;
  sourceDatabaseId?: string | null;
  sourceVolumeId?: string | null;
  jobId?: string | null;
  configurationId?: string | null;
  createdAt: string;
  updatedAt: string;
  uploadedAt: string | null;
}

/** `CreateRestoreRequest` in internal/services/backup/restore.go. */
export interface RestoreOptions {
  overwriteExisting?: boolean;
  stopAppBeforeRestore?: boolean;
  startAppAfterRestore?: boolean;
  restoreToOriginalLocation?: boolean;
  customRestorePath?: string;
  skipVerification?: boolean;
  createBackupBeforeRestore?: boolean;
}

export interface CreateRestoreInput {
  artifactId: string;
  restoreType: "app" | "volume" | "database" | "server";
  /** The API requires exactly one of these four targets (`restore.go:218-223`). */
  targetServerId?: string;
  targetAppId?: string;
  targetDatabaseId?: string;
  targetVolumeId?: string;
  name?: string;
  description?: string;
  restoreOptions?: RestoreOptions;
  maxRetries?: number;
  triggeredBy?: string;
}

export interface BackupRestore {
  id: string;
  name: string;
  description?: string;
  artifactId?: string | null;
  restoreType: "app" | "volume" | "database" | "server";
  targetServerId?: string | null;
  targetAppId?: string | null;
  targetDatabaseId?: string | null;
  targetVolumeId?: string | null;
  status: "pending" | "running" | "completed" | "failed" | "cancelled";
  /** Go: `progressPercentage`. */
  progressPercentage: number;
  bytesProcessed: number;
  totalBytes?: number | null;
  currentPhase?: string;
  errorMessage?: string | null;
  verificationStatus?: string | null;
  canRollback?: boolean;
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

export function downloadBackupArtifact(id: string): Promise<Blob> {
  return requestBlob(`/admin/backups/artifacts/${encodeURIComponent(id)}/download`, { method: "GET" });
}

export function fetchBackupRestores(): Promise<BackupRestore[]> {
  return fetchJSON<BackupRestore[]>("/admin/backups/restores");
}

/**
 * Re-hash a stored artifact against its recorded digest.
 * `POST /admin/backups/artifacts/:id/verify` — the handler returns
 * `{"ok":true,"verified":true}` on success and a 400 with the mismatch reason
 * when the digest does not match, so a rejected promise here means "the backup
 * is corrupt", not "the request failed".
 */
export async function verifyBackupArtifact(id: string): Promise<void> {
  await postJSON<{ ok?: boolean; verified?: boolean }>(
    `/admin/backups/artifacts/${encodeURIComponent(id)}/verify`,
  );
}

export function createRestore(data: CreateRestoreInput): Promise<BackupRestore> {
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

/* ------------------------------------------------------------------ *
 * Honest readings
 *
 * `isVerified` and `status` alone cannot tell "not checked yet" from
 * "checked and bad", and `fileSize` arrives as 0 for artifacts whose upload
 * has not landed. These two helpers are the only place that judgement is
 * made, so no page can render an unmeasured value as a measurement.
 * ------------------------------------------------------------------ */

export type ArtifactVerificationVerdict =
  | { state: "verified"; verifiedAt: string | null; attempts: number }
  | { state: "failed"; verifiedAt: null; attempts: number }
  | { state: "unverified"; verifiedAt: null; attempts: number };

/**
 * Three-way verification verdict.
 *
 * The API resets `verificationAttempts` to 0 when a check passes and increments
 * it when a hash mismatches, so an attempt count with no `isVerified` is
 * evidence of a *failed* check — the case a plain Yes/No column hides.
 */
export function artifactVerification(artifact: BackupArtifact): ArtifactVerificationVerdict {
  const attempts = Number.isFinite(artifact.verificationAttempts) ? artifact.verificationAttempts : 0;
  if (artifact.isVerified) {
    return { state: "verified", verifiedAt: artifact.lastVerifiedAt ?? null, attempts };
  }
  if (attempts > 0) {
    return { state: "failed", verifiedAt: null, attempts };
  }
  return { state: "unverified", verifiedAt: null, attempts };
}

/** Size in bytes, or null when it was never measured. */
export function artifactSizeBytes(artifact: BackupArtifact): number | null {
  const size = Number(artifact.fileSize);
  if (!Number.isFinite(size) || size <= 0) {
    // 0 is what the API stores before an upload reports a size; an artifact that
    // has actually uploaded and still reports 0 is genuinely empty.
    return artifact.uploadedAt ? 0 : null;
  }
  return size;
}
