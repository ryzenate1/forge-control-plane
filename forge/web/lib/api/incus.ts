import { deleteJSON, fetchJSON, postJSON } from "./http";

// Types mirror the subset of the Incus REST API shapes surfaced by the
// /admin/incus/* endpoints (see forge/api/internal/services/incus/service.go).

export type IncusInstance = {
  name: string;
  project?: string;
  instanceType?: string;
  status: string;
  statusCode?: number;
  config?: Record<string, string>;
  devices?: Record<string, Record<string, string>>;
  profiles?: string[];
  ephemeral?: boolean;
  createdAt?: string;
};

export type IncusImage = {
  fingerprint: string;
  aliases?: { name: string; description?: string }[];
  architecture?: number;
  size?: number;
  type?: string;
  createdAt?: string;
  uploadedAt?: string;
  properties?: Record<string, string>;
};

export type IncusProfile = {
  name: string;
  description?: string;
  config?: Record<string, string>;
  devices?: Record<string, Record<string, string>>;
};

export type IncusStoragePool = {
  name: string;
  description?: string;
  driver?: string;
  config?: Record<string, string>;
  status?: string;
  usedBy?: string[];
};

export type IncusClusterMember = {
  serverName: string;
  url?: string;
  database?: boolean;
  roles?: string[];
  failureDomain?: string;
};

function nodeQuery(nodeId?: string): string {
  return nodeId ? `?nodeId=${encodeURIComponent(nodeId)}` : "";
}

export type IncusNode = {
  id: string;
  name: string;
  baseUrl: string;
  runtimeProvider: string;
  runtimeStatus?: string;
  status: string;
  regionId?: string;
};

export function fetchIncusNodes() {
  return fetchJSON<{ nodes: IncusNode[] }>("/admin/incus/nodes").then((r) => r.nodes ?? []);
}

export function fetchIncusInstances(nodeId?: string) {
  return fetchJSON<{ instances: IncusInstance[] }>(`/admin/incus/instances${nodeQuery(nodeId)}`).then((r) => r.instances ?? []);
}

export function fetchIncusInstance(name: string, nodeId?: string) {
  const sep = nodeId ? "&" : "?";
  return fetchJSON<IncusInstance>(`/admin/incus/instances/${encodeURIComponent(name)}${nodeId ? `${sep}nodeId=${encodeURIComponent(nodeId)}` : ""}`);
}

export function createIncusInstance(spec: Record<string, unknown>, nodeId?: string) {
  return postJSON<{ ok: boolean }>(`/admin/incus/instances${nodeQuery(nodeId)}`, spec);
}

export function startIncusInstance(name: string, nodeId?: string) {
  return postJSON<{ ok: boolean }>(`/admin/incus/instances/${encodeURIComponent(name)}/start${nodeQuery(nodeId)}`);
}

export function stopIncusInstance(name: string, opts: { force?: boolean; nodeId?: string } = {}) {
  const params = new URLSearchParams();
  if (opts.force) params.set("force", "true");
  if (opts.nodeId) params.set("nodeId", opts.nodeId);
  const query = params.toString();
  return postJSON<{ ok: boolean }>(`/admin/incus/instances/${encodeURIComponent(name)}/stop${query ? `?${query}` : ""}`);
}

export function restartIncusInstance(name: string, nodeId?: string) {
  return postJSON<{ ok: boolean }>(`/admin/incus/instances/${encodeURIComponent(name)}/restart${nodeQuery(nodeId)}`);
}

export async function deleteIncusInstance(name: string, opts: { force?: boolean; nodeId?: string } = {}) {
  const params = new URLSearchParams();
  if (opts.force) params.set("force", "true");
  if (opts.nodeId) params.set("nodeId", opts.nodeId);
  const query = params.toString();
  await deleteJSON<void>(`/admin/incus/instances/${encodeURIComponent(name)}${query ? `?${query}` : ""}`);
  return { ok: true };
}

export function fetchIncusImages(nodeId?: string) {
  return fetchJSON<{ images: IncusImage[] }>(`/admin/incus/images${nodeQuery(nodeId)}`).then((r) => r.images ?? []);
}

export function fetchIncusProfiles(nodeId?: string) {
  return fetchJSON<{ profiles: IncusProfile[] }>(`/admin/incus/profiles${nodeQuery(nodeId)}`).then((r) => r.profiles ?? []);
}

export function fetchIncusStoragePools(nodeId?: string) {
  return fetchJSON<{ storagePools: IncusStoragePool[] }>(`/admin/incus/storage-pools${nodeQuery(nodeId)}`).then((r) => r.storagePools ?? []);
}

export function fetchIncusCluster(nodeId?: string) {
  return fetchJSON<{ clusterMembers: IncusClusterMember[] }>(`/admin/incus/cluster${nodeQuery(nodeId)}`).then((r) => r.clusterMembers ?? []);
}
