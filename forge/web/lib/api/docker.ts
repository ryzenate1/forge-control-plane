import { fetchJSON, postJSON, deleteJSON } from "./http";

export type DockerContainer = {
  nodeId: string;
  nodeName: string;
  containers: unknown[];
};

export type DockerContainerInfo = {
  id: string;
  name: string;
  image: string;
  state: string;
  status: string;
  ports: string;
  created: string;
  nodeId: string;
  nodeName: string;
};

export type DockerImage = {
  nodeId: string;
  nodeName: string;
  id: string;
  tags: string;
  size: number;
  created: number;
};

export type DockerNetwork = {
  nodeId: string;
  nodeName: string;
  id: string;
  name: string;
  driver: string;
  scope: string;
  attached: number;
};

export type DockerVolume = {
  nodeId: string;
  nodeName: string;
  name: string;
  driver: string;
  mountpoint: string;
  createdAt: string;
};

export type CreateContainerRequest = {
  image: string;
  name?: string;
  ports?: Array<{ hostPort: number; containerPort: number; protocol: string }>;
  env?: Record<string, string>;
  volumes?: Array<{ hostPath: string; containerPath: string; readOnly?: boolean }>;
  network?: string;
  restartPolicy?: string;
  nodeId?: string;
};

export type CreateNetworkRequest = {
  name: string;
  driver?: string;
  subnet?: string;
  nodeId?: string;
};

export type CreateVolumeRequest = {
  name: string;
  driver?: string;
  nodeId?: string;
};

function pickNodeParam(nodeId?: string): string {
  return nodeId ? `?node=${encodeURIComponent(nodeId)}` : "";
}

export async function listContainers(params?: { all?: boolean }): Promise<DockerContainerInfo[]> {
  const all = params?.all ?? true;
  const result = await fetchJSON<DockerContainer[]>(`/docker/containers?all=${all}`);
  const flat: DockerContainerInfo[] = [];
  for (const node of result) {
    if (Array.isArray(node.containers)) {
      for (const c of node.containers as Array<Record<string, unknown>>) {
        const names = c.Names as string[] | undefined;
        const ports = c.Ports as Array<Record<string, unknown>> | undefined;
        flat.push({
          id: c.Id as string ?? c.id as string,
          name: names && names.length > 0 ? (names[0] as string).replace(/^\//, "") : "",
          image: c.Image as string ?? "",
          state: c.State as string ?? (c.state as string) ?? "",
          status: c.Status as string ?? (c.status as string) ?? "",
          ports: ports ? ports.map((p) => `${p.hostPort || ""}:${p.containerPort}/${p.type || "tcp"}`).join(", ") : "",
          created: c.Created as string ?? (c.created as string) ?? "",
          nodeId: node.nodeId,
          nodeName: node.nodeName,
        });
      }
    }
  }
  return flat;
}

export type DockerContainerDetail = Record<string, unknown>;

export type DockerContainerStats = Record<string, unknown>;

export type DockerOperationResult = { ok: boolean };

export async function getContainer(id: string): Promise<DockerContainerDetail> {
  return fetchJSON<DockerContainerDetail>(`/docker/containers/${encodeURIComponent(id)}`);
}

export async function createContainer(config: CreateContainerRequest): Promise<DockerContainerDetail> {
  const nodeParam = config.nodeId ? `?node=${encodeURIComponent(config.nodeId)}` : "";
  return postJSON<DockerContainerDetail>(`/docker/containers${nodeParam}`, config);
}

export async function operateContainer(
  id: string,
  action: "start" | "stop" | "restart" | "pause" | "unpause",
  nodeId?: string,
): Promise<DockerOperationResult> {
  return postJSON<DockerOperationResult>(`/docker/containers/${encodeURIComponent(id)}/operate${pickNodeParam(nodeId)}`, { action });
}

export async function deleteContainer(id: string, force?: boolean, nodeId?: string): Promise<DockerOperationResult> {
  const params = new URLSearchParams();
  if (force) params.set("force", "true");
  if (nodeId) params.set("node", nodeId);
  const qs = params.toString();
  return deleteJSON<DockerOperationResult>(`/docker/containers/${encodeURIComponent(id)}${qs ? `?${qs}` : ""}`);
}

export async function getContainerLogs(id: string, tail?: number, nodeId?: string): Promise<string> {
  const params = new URLSearchParams();
  if (tail) params.set("tail", String(tail));
  if (nodeId) params.set("node", nodeId);
  const qs = params.toString();
  const response = await fetchJSON<string>(`/docker/containers/${encodeURIComponent(id)}/logs${qs ? `?${qs}` : ""}`);
  return typeof response === "string" ? response : String(response);
}

export async function getContainerStats(id: string, nodeId?: string): Promise<DockerContainerStats> {
  return fetchJSON<DockerContainerStats>(`/docker/containers/${encodeURIComponent(id)}/stats${pickNodeParam(nodeId)}`);
}

export async function listImages(): Promise<DockerImage[]> {
  return fetchJSON<DockerImage[]>("/docker/images");
}

export async function pullImage(image: string, tag?: string, nodeId?: string): Promise<DockerOperationResult> {
  return postJSON<DockerOperationResult>("/docker/images/pull", { image, tag, nodeId });
}

export async function deleteImage(id: string, nodeId: string): Promise<DockerOperationResult> {
  return deleteJSON<DockerOperationResult>(`/docker/images/${encodeURIComponent(id)}?node=${encodeURIComponent(nodeId)}`);
}

export async function listNetworks(): Promise<DockerNetwork[]> {
  return fetchJSON<DockerNetwork[]>("/docker/networks");
}

export async function createNetwork(config: CreateNetworkRequest): Promise<DockerNetwork> {
  const nodeParam = config.nodeId ? `?node=${encodeURIComponent(config.nodeId)}` : "";
  return postJSON<DockerNetwork>(`/docker/networks${nodeParam}`, config);
}

export async function deleteNetwork(id: string, nodeId: string): Promise<DockerOperationResult> {
  return deleteJSON<DockerOperationResult>(`/docker/networks/${encodeURIComponent(id)}?node=${encodeURIComponent(nodeId)}`);
}

export async function listVolumes(): Promise<DockerVolume[]> {
  return fetchJSON<DockerVolume[]>("/docker/volumes");
}

export async function createVolume(config: CreateVolumeRequest): Promise<DockerVolume> {
  const nodeParam = config.nodeId ? `?node=${encodeURIComponent(config.nodeId)}` : "";
  return postJSON<DockerVolume>(`/docker/volumes${nodeParam}`, config);
}

export async function deleteVolume(id: string, nodeId: string): Promise<DockerOperationResult> {
  return deleteJSON<DockerOperationResult>(`/docker/volumes/${encodeURIComponent(id)}?node=${encodeURIComponent(nodeId)}`);
}

export async function pruneVolumes(): Promise<DockerOperationResult> {
  return postJSON<DockerOperationResult>("/docker/volumes/prune");
}

export async function buildImage(dockerfile: string, tag: string, nodeId?: string): Promise<unknown> {
  return postJSON(`/docker/images/build${pickNodeParam(nodeId)}`, { dockerfile, tag });
}

export async function pushImage(id: string, nodeId?: string): Promise<unknown> {
  return postJSON(`/docker/images/${encodeURIComponent(id)}/push${pickNodeParam(nodeId)}`);
}

export async function tagImage(id: string, tag: string, nodeId?: string): Promise<unknown> {
  return postJSON(`/docker/images/${encodeURIComponent(id)}/tag${pickNodeParam(nodeId)}`, { tag });
}

export async function searchImages(term: string, nodeId?: string): Promise<unknown> {
  const params = new URLSearchParams({ term });
  if (nodeId) params.set("node", nodeId);
  return fetchJSON(`/docker/images/search?${params.toString()}`);
}

export async function listContainerFiles(id: string, path?: string, nodeId?: string): Promise<unknown> {
  const params = new URLSearchParams();
  if (path) params.set("path", path);
  if (nodeId) params.set("node", nodeId);
  const qs = params.toString();
  return fetchJSON(`/docker/containers/${encodeURIComponent(id)}/files${qs ? `?${qs}` : ""}`);
}

export async function readContainerFile(id: string, path: string, nodeId?: string): Promise<unknown> {
  return postJSON(`/docker/containers/${encodeURIComponent(id)}/files/read${pickNodeParam(nodeId)}`, { path });
}

export async function uploadContainerFile(id: string, path: string, content: string, nodeId?: string): Promise<unknown> {
  return postJSON(`/docker/containers/${encodeURIComponent(id)}/files/upload${pickNodeParam(nodeId)}`, { path, content });
}

export async function deleteContainerFile(id: string, path: string, nodeId?: string): Promise<unknown> {
  return postJSON(`/docker/containers/${encodeURIComponent(id)}/files/delete${pickNodeParam(nodeId)}`, { path });
}
