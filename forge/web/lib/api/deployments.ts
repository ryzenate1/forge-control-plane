import { fetchJSON, postJSON } from "./http";

export type DeploymentStep = {
  id: string;
  deploymentId: string;
  stepNumber: number;
  stepName: string;
  status: "pending" | "in_progress" | "completed" | "failed" | "cancelled" | "skipped";
  startedAt?: string;
  completedAt?: string;
  error?: string;
  createdAt: string;
  updatedAt: string;
};

export type Deployment = {
  id: string;
  serverId: string;
  strategy: string;
  status: string;
  image: string;
  blueTargetId: string;
  greenTargetId: string;
  activeTarget: string;
  healthCheckPath?: string;
  healthCheckPort?: number;
  error?: string;
  currentRevisionId?: string;
  progressPct: number;
  nextStep: number;
  version: number;
  createdAt: string;
  updatedAt: string;
  completedAt?: string;
};

export type DeploymentRecord = {
  id: string;
  serverId: string;
  serviceId?: string;
  status: "pending" | "running" | "done" | "error" | "cancelled";
  logPath?: string;
  commitHash?: string;
  commitMessage?: string;
  errorMessage?: string;
  rollbackId?: string;
  startedAt?: string;
  finishedAt?: string;
  createdAt: string;
  updatedAt: string;
};

export type Rollback = {
  id: string;
  deploymentId: string;
  status: "pending" | "in_progress" | "completed" | "failed";
  createdAt: string;
};

export type Revision = {
  id: string;
  deploymentId: string;
  revisionNumber: number;
  imageRef: string;
  composeManifestRef: string;
  gitCommitSha: string;
  configHash: string;
  status: string;
  deployedAt?: string;
  description: string;
  metadata: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
};

export type RevisionChange = {
  field: string;
  oldValue: string;
  newValue: string;
};

export type RevisionDiff = {
  fromRevisionId: number;
  toRevisionId: number;
  changes: RevisionChange[];
};

export async function fetchDeploymentSteps(deploymentId: string): Promise<DeploymentStep[]> {
  return fetchJSON<DeploymentStep[]>(`/admin/deployments/${encodeURIComponent(deploymentId)}/steps`);
}

export async function fetchDeploymentStep(deploymentId: string, stepId: string): Promise<DeploymentStep> {
  return fetchJSON<DeploymentStep>(`/admin/deployments/${encodeURIComponent(deploymentId)}/steps/${encodeURIComponent(stepId)}`);
}

export async function fetchDeployment(id: string): Promise<Deployment> {
  return fetchJSON<Deployment>(`/admin/deployments/${encodeURIComponent(id)}`);
}

export async function fetchDeploymentRevisions(deploymentId: string): Promise<Revision[]> {
  return fetchJSON<Revision[]>(`/admin/deployments/${encodeURIComponent(deploymentId)}/revisions`);
}

export async function compareRevisions(deploymentId: string, fromId: string, toId: string): Promise<RevisionDiff> {
  return fetchJSON<RevisionDiff>(`/admin/deployments/${encodeURIComponent(deploymentId)}/compare?from=${encodeURIComponent(fromId)}&to=${encodeURIComponent(toId)}`);
}

export async function rollbackToRevision(deploymentId: string, revisionId: string): Promise<Deployment> {
  return postJSON<Deployment>(`/admin/deployments/${encodeURIComponent(deploymentId)}/revisions/${encodeURIComponent(revisionId)}/rollback`);
}

export async function rollbackToPrevious(deploymentId: string): Promise<Deployment> {
  return postJSON<Deployment>(`/admin/deployments/${encodeURIComponent(deploymentId)}/rollback-previous`);
}

export async function cancelDeployment(deploymentId: string): Promise<Deployment> {
  return postJSON<Deployment>(`/admin/deployments/${encodeURIComponent(deploymentId)}/cancel`);
}

export async function executeDeployment(deploymentId: string): Promise<void> {
  await postJSON(`/admin/deployments/${encodeURIComponent(deploymentId)}/execute`);
}

export async function completeDeployment(deploymentId: string): Promise<Deployment> {
  return postJSON<Deployment>(`/admin/deployments/${encodeURIComponent(deploymentId)}/complete`);
}

export async function fetchDeploymentRecords(serverId?: string): Promise<DeploymentRecord[]> {
  const path = serverId
    ? `/admin/deployment-history?serverId=${encodeURIComponent(serverId)}`
    : "/admin/deployment-history";
  return fetchJSON<DeploymentRecord[]>(path);
}

export async function fetchDeploymentRecord(id: string): Promise<DeploymentRecord> {
  return fetchJSON<DeploymentRecord>(`/admin/deployment-history/${encodeURIComponent(id)}`);
}

export async function fetchDeploymentLogs(deploymentId: string): Promise<{ id: string; content: string; createdAt: string }[]> {
  return fetchJSON(`/admin/deployment-history/${encodeURIComponent(deploymentId)}/logs`);
}

export async function fetchRollbacks(deploymentId: string): Promise<Rollback[]> {
  return fetchJSON<Rollback[]>(`/admin/deployment-history/${encodeURIComponent(deploymentId)}/rollbacks`);
}
