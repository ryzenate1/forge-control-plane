import { fetchJSON, postJSON, putJSON } from "./http";

export type Release = {
  id: string;
  serverId: string;
  version: number;
  imageTag: string;
  status: string;
  createdAt: string;
  completedAt?: string | null;
};

export type HealthCheckConfig = {
  path: string;
  port: number;
  protocol: string;
  intervalSeconds: number;
  timeoutSeconds: number;
  healthyThreshold: number;
  unhealthyThreshold: number;
};

export type HealthCheckResult = {
  id: string;
  deploymentId: string;
  checkTimestamp: string;
  status: string;
  responseCode: number;
  responseTimeMs: number;
  errorMessage: string;
};

export type DeploymentEvent = {
  id: string;
  eventType: string;
  message: string;
  createdAt: string;
};

function unwrap<T>(data: unknown): T {
  if (data && typeof data === "object" && "data" in (data as Record<string, unknown>)) {
    return (data as { data: T }).data;
  }
  return data as T;
}

export async function createRelease(serverId: string, imageTag: string): Promise<Release> {
  const res = await postJSON<Release | { data: Release }>(`/servers/${encodeURIComponent(serverId)}/deployments`, { imageTag });
  return unwrap<Release>(res);
}

export async function listReleases(serverId: string): Promise<Release[]> {
  const res = await fetchJSON<Release[] | { data: Release[] }>(`/servers/${encodeURIComponent(serverId)}/deployments`);
  if (Array.isArray(res)) return res;
  return unwrap<Release[]>(res);
}

export async function getRelease(serverId: string, releaseId: string): Promise<Release> {
  const res = await fetchJSON<Release | { data: Release }>(`/servers/${encodeURIComponent(serverId)}/deployments/${encodeURIComponent(releaseId)}`);
  return unwrap<Release>(res);
}

export async function rollbackRelease(serverId: string, releaseId: string): Promise<Release> {
  const res = await postJSON<Release | { data: Release }>(`/servers/${encodeURIComponent(serverId)}/deployments/${encodeURIComponent(releaseId)}/rollback`, {});
  return unwrap<Release>(res);
}

export async function promoteRelease(serverId: string, releaseId: string): Promise<Release> {
  const res = await postJSON<Release | { data: Release }>(`/servers/${encodeURIComponent(serverId)}/deployments/${encodeURIComponent(releaseId)}/promote`, {});
  return unwrap<Release>(res);
}

export async function getHealthCheckConfig(serverId: string): Promise<HealthCheckConfig> {
  const res = await fetchJSON<HealthCheckConfig | { data: HealthCheckConfig }>(`/servers/${encodeURIComponent(serverId)}/health-check`);
  return unwrap<HealthCheckConfig>(res);
}

export async function upsertHealthCheckConfig(serverId: string, config: HealthCheckConfig): Promise<HealthCheckConfig> {
  const res = await putJSON<HealthCheckConfig | { data: HealthCheckConfig }>(`/servers/${encodeURIComponent(serverId)}/health-check`, config);
  return unwrap<HealthCheckConfig>(res);
}

export async function getHealthCheckResults(serverId: string, releaseId: string): Promise<HealthCheckResult[]> {
  const res = await fetchJSON<HealthCheckResult[] | { data: HealthCheckResult[] }>(
    `/servers/${encodeURIComponent(serverId)}/deployments/${encodeURIComponent(releaseId)}/health`,
  );
  if (Array.isArray(res)) return res;
  return unwrap<HealthCheckResult[]>(res);
}

export async function getDeploymentEvents(serverId: string, releaseId: string): Promise<DeploymentEvent[]> {
  const res = await fetchJSON<DeploymentEvent[] | { data: DeploymentEvent[] }>(
    `/servers/${encodeURIComponent(serverId)}/deployments/${encodeURIComponent(releaseId)}/events`,
  );
  if (Array.isArray(res)) return res;
  return unwrap<DeploymentEvent[]>(res);
}
