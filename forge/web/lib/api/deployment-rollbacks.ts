import { requestJSON } from './http';

/**
 * Typed client for the deployment history / rollback surface.
 *
 * Mirrors forge/api/internal/http/handlers_deployment_rollback.go. The routes
 * live under `deployment-versions` rather than `deployments` because
 * /servers/:id/deployments is already the zero-downtime *release* surface
 * (handlers_zerodowntime.go) and Fiber matches in registration order — a
 * second /deployments/:id/rollback there would silently shadow it.
 *
 * A "version" is one deployments row: one thing the workload was actually
 * asked to run, carrying the image digest, commit SHA and compose ref that a
 * rollback restores.
 */

/** Which workload's history to read. Both scopes hit the same handler. */
export type DeploymentHistoryScope =
  | { kind: 'server'; id: string }
  | { kind: 'app'; id: string };

function scopePath(scope: DeploymentHistoryScope): string {
  const encoded = encodeURIComponent(scope.id);
  return scope.kind === 'server' ? `/servers/${encoded}` : `/apps/${encoded}`;
}

/** One deployment as serialized by deployment.Deployment, plus enrichment. */
export type DeploymentVersion = {
  id: string;
  serverId: string;
  strategy: string;
  status: string;
  image: string;
  blueTargetId?: string;
  greenTargetId?: string;
  activeTarget?: string;
  healthCheckPath?: string;
  healthCheckPort?: number;
  healthCheckHost?: string;
  currentRevisionId?: string;
  rolloutStrategy?: string;
  timeoutSeconds?: number;
  healthGateEnabled?: boolean;
  autoRollbackEnabled?: boolean;
  rollbackOnHealthFailure?: boolean;
  cleanupOnFailure?: boolean;
  targetReplicas?: number;
  progressPct?: number;
  nextStep?: number;
  timeoutAt?: string;
  executorId?: string;
  executionLeaseUntil?: string;
  version: number;
  createdAt: string;
  updatedAt: string;
  completedAt?: string | null;
  error?: string;

  /** Set for app-scoped reads: the application whose history this is. */
  applicationId?: string;
  revisionNumber?: number;
  /** Empty when the version recorded no commit — the UI says so, never guesses. */
  commitSha?: string;
  composeManifestRef?: string;
  configHash?: string;
  description?: string;
  /** Server-computed: the API would accept a rollback to this version. */
  canRollbackTo: boolean;
  isLive: boolean;
  /** Present when this row exists to carry out an explicit rollback. */
  rollback?: DeploymentRollbackRecord;
  /** Rollbacks that chose this version, so history can show restore dates. */
  rolledBackTo?: Array<{
    id: string;
    status: string;
    reason?: string;
    createdAt: string;
  }>;
};

export type DeploymentRollbackRecord = {
  id: string;
  /** The version that was rolled back *to*. */
  deploymentId: string;
  /** The deployment that was newest when the rollback was requested. */
  triggeredByDeploymentId?: string;
  /** The new deployment row created to carry the rollback out. */
  rollbackDeploymentId?: string;
  applicationId?: string;
  serverId: string;
  initiatedBy?: string;
  status: 'pending' | 'in_progress' | 'completed' | 'failed' | 'cancelled' | string;
  reason?: string;
  snapshot?: Record<string, unknown>;
  createdAt: string;
  completedAt?: string | null;
};

export type DeploymentVersionDetail = DeploymentVersion & {
  steps: Array<{
    id: string;
    deploymentId: string;
    stepNumber: number;
    stepName: string;
    status: string;
    startedAt?: string | null;
    completedAt?: string | null;
    error?: string;
  }>;
  revisions: Array<{
    id: string;
    deploymentId: string;
    revisionNumber: number;
    imageRef: string;
    composeManifestRef?: string;
    gitCommitSha?: string;
    configHash?: string;
    status?: string;
    description?: string;
    metadata?: Record<string, unknown>;
    createdAt: string;
  }>;
  composeSnapshot?: Record<string, unknown>;
  environment?: Record<string, unknown>;
  rollbacks: DeploymentRollbackRecord[];
};

export type DeploymentVersionPage = {
  data: DeploymentVersion[];
  page: number;
  perPage: number;
  total: number;
  pageCount: number;
  serverId: string;
  liveDeploymentId: string;
};

export type DeploymentVersionSide = {
  deploymentId: string;
  status: string;
  image: string;
  strategy: string;
  commitSha?: string;
  composeManifestRef?: string;
  revisionNumber?: number;
  createdAt: string;
  completedAt?: string | null;
  metadata?: Record<string, unknown>;
};

export type RevisionChange = {
  field: string;
  oldValue: string;
  newValue: string;
};

export type DeploymentVersionDiff = {
  from: DeploymentVersionSide;
  to: DeploymentVersionSide;
  changes: RevisionChange[];
  envDiff: RevisionChange[];
  /** Set when a side had no revision snapshot — the diff then covers less than it appears to. */
  notes?: string[];
};

export type RollbackAccepted = {
  rollbackId: string;
  /** The new deployment row now running the older image. Poll this. */
  deploymentId: string;
  targetDeploymentId: string;
  triggeredByDeploymentId?: string | null;
  serverId: string;
  image: string;
  status: string;
  reason: string;
  createdAt: string;
};

export type ListDeploymentVersionsOptions = {
  page?: number;
  perPage?: number;
  status?: string;
};

export async function listDeploymentVersions(
  scope: DeploymentHistoryScope,
  options: ListDeploymentVersionsOptions = {},
): Promise<DeploymentVersionPage> {
  const params = new URLSearchParams();
  if (options.page) params.set('page', String(options.page));
  if (options.perPage) params.set('perPage', String(options.perPage));
  if (options.status) params.set('status', options.status);
  const query = params.toString();
  const path = `${scopePath(scope)}/deployment-versions${query ? `?${query}` : ''}`;
  return requestJSON<DeploymentVersionPage>(path);
}

export async function getDeploymentVersion(
  scope: DeploymentHistoryScope,
  deploymentId: string,
): Promise<DeploymentVersionDetail> {
  const res = await requestJSON<{ data: DeploymentVersionDetail }>(
    `${scopePath(scope)}/deployment-versions/${encodeURIComponent(deploymentId)}`,
  );
  return res.data;
}

/**
 * Roll the workload back to `deploymentId`. Returns 202 with the deployment
 * row that now carries the rollback — the request only *starts* the work, so
 * callers must poll {@link getDeploymentVersion} or the list until it settles.
 */
export async function rollbackDeploymentVersion(
  scope: DeploymentHistoryScope,
  deploymentId: string,
  reason?: string,
): Promise<RollbackAccepted> {
  const res = await requestJSON<{ data: RollbackAccepted }>(
    `${scopePath(scope)}/deployment-versions/${encodeURIComponent(deploymentId)}/rollback`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ reason: reason ?? '' }),
    },
  );
  return res.data;
}

/**
 * Compare a version against another. With no `withDeploymentId` the API
 * compares against what is live now, which is the question the rollback
 * confirmation asks: "what changes if I go back to this?"
 */
export async function diffDeploymentVersions(
  scope: DeploymentHistoryScope,
  deploymentId: string,
  withDeploymentId?: string,
): Promise<DeploymentVersionDiff> {
  const params = new URLSearchParams();
  if (withDeploymentId) params.set('with', withDeploymentId);
  const query = params.toString();
  const res = await requestJSON<{ data: DeploymentVersionDiff }>(
    `${scopePath(scope)}/deployment-versions/${encodeURIComponent(deploymentId)}/diff${query ? `?${query}` : ''}`,
  );
  return res.data;
}

export async function listDeploymentRollbacks(
  scope: DeploymentHistoryScope,
  options: { limit?: number; deploymentId?: string } = {},
): Promise<DeploymentRollbackRecord[]> {
  const params = new URLSearchParams();
  if (options.limit) params.set('limit', String(options.limit));
  if (options.deploymentId) params.set('deploymentId', options.deploymentId);
  const query = params.toString();
  const res = await requestJSON<{ data: DeploymentRollbackRecord[] }>(
    `${scopePath(scope)}/deployment-rollbacks${query ? `?${query}` : ''}`,
  );
  return res.data ?? [];
}

export async function getDeploymentRollback(
  scope: DeploymentHistoryScope,
  rollbackId: string,
): Promise<DeploymentRollbackRecord> {
  const res = await requestJSON<{ data: DeploymentRollbackRecord }>(
    `${scopePath(scope)}/deployment-rollbacks/${encodeURIComponent(rollbackId)}`,
  );
  return res.data;
}

/** In-flight deployment statuses — used to decide whether to keep polling. */
export const ROLLBACK_PROGRESS_STATUSES = [
  'pending',
  'provisioning',
  'in_progress',
  'awaiting_health',
  'promoting',
  'rollback_pending',
  'rolling_back',
];

export function rollbackInFlight(status: string | undefined): boolean {
  return !!status && ROLLBACK_PROGRESS_STATUSES.includes(status);
}

/** Truncate a digest for display without losing the identifying prefix/suffix. */
export function shortImage(image: string): string {
  const trimmed = image.trim();
  if (trimmed.length <= 46) return trimmed;
  const digestIndex = trimmed.lastIndexOf('@sha256:');
  if (digestIndex > 0) {
    return `${trimmed.slice(digestIndex + 8, digestIndex + 20)}… ${trimmed.slice(0, 18)}`;
  }
  return `${trimmed.slice(0, 28)}…${trimmed.slice(-10)}`;
}
