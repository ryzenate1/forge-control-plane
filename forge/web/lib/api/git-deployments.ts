import { fetchJSON, postJSON, deleteJSON } from './http';

export interface GitDeployment {
  id: string;
  gitSourceId: string;
  commitSha: string;
  branch: string;
  status: string;
  statusMessage: string;
  imageTag: string;
  buildLog: string;
  deployLog: string;
  error: string;
  startedAt: string;
  completedAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface GitDeploymentHook {
  id: string;
  gitSourceId: string;
  secret: string;
  events: string[];
  createdAt: string;
  updatedAt: string;
}

// The panel returns either a bare array or a `{ data: [...] }` envelope for the
// git lists, so both shapes are unwrapped here rather than in each component.
function asList<T>(payload: T[] | { data?: T[] } | null | undefined): T[] {
  if (Array.isArray(payload)) return payload;
  return payload?.data ?? [];
}

export async function listGitDeployments(serverId: string): Promise<GitDeployment[]> {
  const res = await fetchJSON<GitDeployment[] | { data?: GitDeployment[] }>(
    `/git/servers/${encodeURIComponent(serverId)}/deployments`,
  );
  return asList(res);
}

export async function listGitDeploymentHooks(serverId: string): Promise<GitDeploymentHook[]> {
  const res = await fetchJSON<GitDeploymentHook[] | { data?: GitDeploymentHook[] }>(
    `/git/servers/${encodeURIComponent(serverId)}/hooks`,
  );
  return asList(res);
}

export function triggerGitDeployment(
  serverId: string,
  input: { repoUrl: string; branch: string },
): Promise<void> {
  return postJSON<void>(`/git/servers/${encodeURIComponent(serverId)}/deployments`, input);
}

export function createGitDeploymentHook(
  serverId: string,
  events: string[] = ['push'],
): Promise<GitDeploymentHook | void> {
  return postJSON<GitDeploymentHook | void>(`/git/servers/${encodeURIComponent(serverId)}/hooks`, { events });
}

export function deleteGitDeploymentHook(serverId: string, hookId: string): Promise<void> {
  return deleteJSON<void>(`/git/servers/${encodeURIComponent(serverId)}/hooks/${encodeURIComponent(hookId)}`);
}
