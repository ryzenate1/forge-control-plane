import { deleteJSON, fetchJSON, patchJSON, postJSON } from './http';
import type {
  ApiMount,
  ApiMountAssignmentResponse,
  ApiServer,
  AssignMountInput,
  CreateMountInput,
} from './types';

export async function fetchMount(id: string): Promise<ApiMount> {
  return fetchJSON<ApiMount>(`/mounts/${encodeURIComponent(id)}`);
}

export async function fetchMounts(): Promise<ApiMount[]> {
  return fetchJSON<ApiMount[]>('/mounts');
}

export async function createMount(input: CreateMountInput): Promise<ApiMount> {
  return postJSON<ApiMount>('/mounts', input);
}

export async function updateMount(
  id: string,
  input: Partial<CreateMountInput>,
): Promise<ApiMount> {
  return patchJSON<ApiMount>(`/mounts/${encodeURIComponent(id)}`, input);
}

export async function deleteMount(id: string): Promise<{ ok: boolean }> {
  return deleteJSON<{ ok: boolean }>(`/mounts/${encodeURIComponent(id)}`);
}

export async function fetchServerMounts(serverId: string): Promise<ApiMount[]> {
  return fetchJSON<ApiMount[]>(`/servers/${encodeURIComponent(serverId)}/mounts`);
}

export async function fetchMountServers(mountId: string): Promise<ApiServer[]> {
  return fetchJSON<ApiServer[]>(`/mounts/${encodeURIComponent(mountId)}/servers`);
}

export async function assignMountToServer(
  serverId: string,
  input: AssignMountInput,
): Promise<ApiMountAssignmentResponse> {
  return postJSON<ApiMountAssignmentResponse>(
    `/servers/${encodeURIComponent(serverId)}/mounts`,
    input,
  );
}

export async function unassignMountFromServer(
  serverId: string,
  mountId: string,
): Promise<ApiMountAssignmentResponse> {
  return deleteJSON<ApiMountAssignmentResponse>(
    `/servers/${encodeURIComponent(serverId)}/mounts/${encodeURIComponent(mountId)}`,
  );
}

export async function assignServerToMount(
  mountId: string,
  serverId: string,
): Promise<ApiMountAssignmentResponse> {
  return postJSON<ApiMountAssignmentResponse>(
    `/mounts/${encodeURIComponent(mountId)}/servers`,
    { serverId },
  );
}

export async function unassignServerFromMount(
  mountId: string,
  serverId: string,
): Promise<ApiMountAssignmentResponse> {
  return deleteJSON<ApiMountAssignmentResponse>(
    `/mounts/${encodeURIComponent(mountId)}/servers/${encodeURIComponent(serverId)}`,
  );
}

export async function attachEggsToMount(
  mountId: string,
  eggIds: string[],
): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/mounts/${encodeURIComponent(mountId)}/eggs`, { eggs: eggIds });
}

export async function attachNodesToMount(
  mountId: string,
  nodeIds: string[],
): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/mounts/${encodeURIComponent(mountId)}/nodes`, { nodes: nodeIds });
}

export async function detachEggFromMount(mountId: string, eggId: string): Promise<{ ok: boolean }> {
  return deleteJSON<{ ok: boolean }>(`/mounts/${encodeURIComponent(mountId)}/eggs/${encodeURIComponent(eggId)}`);
}

export async function detachNodeFromMount(mountId: string, nodeId: string): Promise<{ ok: boolean }> {
  return deleteJSON<{ ok: boolean }>(`/mounts/${encodeURIComponent(mountId)}/nodes/${encodeURIComponent(nodeId)}`);
}

export function assignMount(serverId: string, mountId: string): Promise<ApiMountAssignmentResponse> {
  return assignMountToServer(serverId, { mountId });
}

export function removeMount(serverId: string, mountId: string): Promise<ApiMountAssignmentResponse> {
  return unassignMountFromServer(serverId, mountId);
}

export function assignServerMount(serverId: string, mountId: string): Promise<ApiMountAssignmentResponse> {
  return assignMount(serverId, mountId);
}

export function removeServerMount(serverId: string, mountId: string): Promise<ApiMountAssignmentResponse> {
  return removeMount(serverId, mountId);
}
