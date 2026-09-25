import { fetchJSON, postJSON, patchJSON, deleteJSON } from "./http";

// Client for the admin Docker disk-usage & automated cleanup surface
// (/api/v1/admin/docker-cleanup). Mirrors forge/api/internal/services/dockerleanup
// wire types: per-node disk accounting, unused-image analysis, manual prune
// actions and cron-based cleanup policies that preserve the N most recent
// deployed images.

export type DockerImageInfo = {
  id: string;
  tags: string[];
  size: number;
  /** RFC3339 timestamp as serialized by the Go service's time.Time. */
  createdAt: string;
  inUse: boolean;
};

export type DockerDiskUsage = {
  nodeId: string;
  nodeName: string;
  imagesBytes: number;
  containersBytes: number;
  volumesBytes: number;
  buildCacheBytes: number;
  totalBytes: number;
  images: DockerImageInfo[];
};

export type DockerPruneResult = {
  nodeId: string;
  reclaimedBytes: number;
  removedCount: number;
};

export type DockerCleanupPolicy = {
  id: string;
  /** Absent for a global policy (applies to every reachable node). */
  nodeId?: string;
  schedule: string;
  mostRecentLimit: number;
  enabled: boolean;
  pruneBuildCache: boolean;
  pruneVolumes: boolean;
  lastRunAt?: string;
  nextRunAt?: string;
  lastStatus?: string;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
  createdBy?: string;
};

export type CreateCleanupPolicyRequest = {
  nodeId?: string;
  schedule: string;
  mostRecentLimit?: number;
  enabled?: boolean;
  pruneBuildCache?: boolean;
  pruneVolumes?: boolean;
};

export type UpdateCleanupPolicyRequest = Partial<CreateCleanupPolicyRequest>;

export async function getDiskUsage(nodeId: string): Promise<DockerDiskUsage> {
  return fetchJSON<DockerDiskUsage>(`/admin/docker-cleanup/disk-usage/${encodeURIComponent(nodeId)}`);
}

export async function listUnusedImages(nodeId: string, limit = 1): Promise<DockerImageInfo[]> {
  const params = new URLSearchParams({ limit: String(limit) });
  const res = await fetchJSON<{ data: DockerImageInfo[] }>(
    `/admin/docker-cleanup/unused-images/${encodeURIComponent(nodeId)}?${params.toString()}`,
  );
  return res.data ?? [];
}

export async function pruneImages(nodeId: string, imageIds: string[]): Promise<DockerPruneResult> {
  return postJSON<DockerPruneResult>("/admin/docker-cleanup/prune/images", { nodeId, imageIds });
}

export async function pruneBuildCache(nodeId: string): Promise<DockerPruneResult> {
  return postJSON<DockerPruneResult>(`/admin/docker-cleanup/prune/build-cache/${encodeURIComponent(nodeId)}`);
}

export async function pruneVolumes(nodeId: string): Promise<DockerPruneResult> {
  return postJSON<DockerPruneResult>(`/admin/docker-cleanup/prune/volumes/${encodeURIComponent(nodeId)}`);
}

export async function listPolicies(): Promise<DockerCleanupPolicy[]> {
  const res = await fetchJSON<{ data: DockerCleanupPolicy[] }>("/admin/docker-cleanup/policies");
  return res.data ?? [];
}

export async function createPolicy(input: CreateCleanupPolicyRequest): Promise<DockerCleanupPolicy> {
  return postJSON<DockerCleanupPolicy>("/admin/docker-cleanup/policies", input);
}

export async function updatePolicy(id: string, input: UpdateCleanupPolicyRequest): Promise<DockerCleanupPolicy> {
  return patchJSON<DockerCleanupPolicy>(`/admin/docker-cleanup/policies/${encodeURIComponent(id)}`, input);
}

export async function deletePolicy(id: string): Promise<void> {
  await deleteJSON<{ ok: boolean }>(`/admin/docker-cleanup/policies/${encodeURIComponent(id)}`);
}

export async function runPolicyNow(id: string): Promise<DockerCleanupPolicy> {
  return postJSON<DockerCleanupPolicy>(`/admin/docker-cleanup/policies/${encodeURIComponent(id)}/run-now`);
}
