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

// ---------------------------------------------------------------------------
// Per-application declarative mounts
//
// These are distinct from the admin `mounts` catalogue above (host paths shared
// across servers). An AppMount is a persistent mount declared on a single
// application — a named volume, host bind mount, tmpfs, or a DB-stored seed
// file — that survives redeploys because the deploy pipeline injects it into
// the compose document. Backed by /api/v1/apps/:appId/mounts.
// ---------------------------------------------------------------------------

/** Persistent storage kinds. Mirrors the backend `mounts.MountType` enum. */
export type AppMountType = 'volume' | 'bind' | 'tmpfs' | 'seed-file';

export interface AppMount {
  id: string;
  applicationId: string;
  name: string;
  type: AppMountType;
  /** Host path (bind), volume name (volume), empty for tmpfs/seed-file. */
  source: string;
  /** Absolute container path the mount is materialized at. */
  target: string;
  readOnly: boolean;
  /** File body, only present for `seed-file` mounts. */
  content?: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface CreateAppMountInput {
  name: string;
  type: AppMountType;
  source?: string;
  target: string;
  readOnly?: boolean;
  content?: string | null;
}

export interface UpdateAppMountInput {
  name?: string;
  type?: AppMountType;
  source?: string;
  target?: string;
  readOnly?: boolean;
  content?: string | null;
}

export interface AppMountValidationResult {
  valid: boolean;
  error?: string;
}

type ListEnvelope<T> = { data?: T[] } | T[];

function unwrapMountList<T>(body: ListEnvelope<T>): T[] {
  return Array.isArray(body) ? body : body?.data ?? [];
}

function appMountBase(appId: string): string {
  return `/apps/${encodeURIComponent(appId)}/mounts`;
}

export async function fetchAppMounts(appId: string): Promise<AppMount[]> {
  const body = await fetchJSON<ListEnvelope<AppMount>>(appMountBase(appId));
  return unwrapMountList(body);
}

export async function createAppMount(
  appId: string,
  input: CreateAppMountInput,
): Promise<AppMount> {
  return postJSON<AppMount>(appMountBase(appId), input);
}

export async function updateAppMount(
  appId: string,
  mountId: string,
  input: UpdateAppMountInput,
): Promise<AppMount> {
  return patchJSON<AppMount>(`${appMountBase(appId)}/${encodeURIComponent(mountId)}`, input);
}

export async function deleteAppMount(
  appId: string,
  mountId: string,
): Promise<{ ok: boolean }> {
  return deleteJSON<{ ok: boolean }>(`${appMountBase(appId)}/${encodeURIComponent(mountId)}`);
}

/**
 * Ask the backend to run the authoritative mount validation against a draft
 * definition without persisting it, so the settings form can surface the exact
 * rule a path or type violates (reserved target, disallowed bind prefix, etc.).
 * Returns the validation message rather than throwing on an invalid draft.
 */
export async function validateAppMount(
  appId: string,
  input: CreateAppMountInput,
): Promise<AppMountValidationResult> {
  return postJSON<AppMountValidationResult>(`${appMountBase(appId)}/validate`, input);
}
