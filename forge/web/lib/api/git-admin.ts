import { deleteJSON, fetchJSON, patchJSON, postJSON } from "./http";
import type { GitProviderRepo, GitProviderBranch } from "./source-deployments";

export interface GitCredentialInternal {
  id: string;
  userId: string;
  name: string;
  credentialType: "ssh_key" | "https_password" | "https_token";
  credential?: string;
  publicKey: string;
  description: string;
  createdAt: string;
  updatedAt: string;
}

export type GitCredential = Omit<GitCredentialInternal, 'credential'>;

export interface GitProviderToken {
  id: string;
  userId: string;
  provider: "github" | "gitlab" | "bitbucket" | "gitea";
  providerName: string;
  /** @internal Should never be returned by the API; stored server-side only */
  accessToken?: string;
  tokenType: string;
  baseUrl: string;
  username: string;
  avatarUrl: string;
  createdAt: string;
  updatedAt: string;
}

export interface GitSource {
  id: string;
  userId: string;
  credentialId?: string;
  providerTokenId?: string;
  provider: string;
  repositoryUrl: string;
  repositoryName: string;
  repositoryOwner: string;
  branch: string;
  autoDeploy: boolean;
  webhookId: string;
  webhookUrl: string;
  /** @internal Should never be returned by the API; stored server-side only */
  webhookSecret?: string;
  lastCommitSha: string;
  lastCommitMessage: string;
  lastCommitAuthor: string;
  lastDeployedAt?: string;
  createdAt: string;
  updatedAt: string;
}


export const listGitCredentials = () => fetchJSON<GitCredential[]>("/git/credentials");
export const listGitProviderTokens = () => fetchJSON<GitProviderToken[]>("/git/providers");
export const listGitSources = () => fetchJSON<GitSource[]>("/git/sources");

export const createGitCredential = (body: {
  name: string;
  credentialType: string;
  credential: string;
  description: string;
}) => postJSON<GitCredential>("/git/credentials", body);

export const deleteGitCredential = (id: string) =>
  deleteJSON<void>(`/git/credentials/${encodeURIComponent(id)}`);

export const generateGitDeployKey = (id: string) =>
  postJSON<{ type: string; publicKey: string }>(
    `/git/credentials/${encodeURIComponent(id)}/generate-key`,
  );

export const connectGitProviderToken = (body: {
  provider: string;
  providerName: string;
  accessToken: string;
  refreshToken: string;
  tokenType: string;
  baseUrl: string;
  username: string;
}) => postJSON<GitProviderToken>("/git/providers", body);

export const disconnectGitProviderToken = (id: string) =>
  deleteJSON<void>(`/git/providers/${encodeURIComponent(id)}`);

export const createGitSource = (body: {
  credentialId?: string;
  providerTokenId?: string;
  provider: string;
  repositoryUrl: string;
  repositoryName: string;
  repositoryOwner: string;
  branch: string;
  autoDeploy: boolean;
}) => postJSON<GitSource>("/git/sources", body);

export const deleteGitSource = (id: string) =>
  deleteJSON<void>(`/git/sources/${encodeURIComponent(id)}`);

export const listGitProviderRepos = (providerId: string) =>
  fetchJSON<GitProviderRepo[]>(`/git/providers/${encodeURIComponent(providerId)}/repos`);

export const listGitProviderBranches = (providerId: string, repo: string) =>
  fetchJSON<GitProviderBranch[]>(
    `/git/providers/${encodeURIComponent(providerId)}/branches?repo=${encodeURIComponent(repo)}`,
  );

// ---- Missing wrappers (P4 polish) ----

export const getGitCredential = (id: string) =>
  fetchJSON<GitCredential>(`/git/credentials/${encodeURIComponent(id)}`);

export const testProviderInline = (body: {
  provider: string;
  accessToken: string;
  baseUrl?: string;
  username?: string;
}) =>
  postJSON<{ ok: boolean; message?: string; provider?: string }>(`/git/providers/test`, body);

export const updateGitProvider = (id: string, body: Partial<{
  providerName: string;
  accessToken: string;
  refreshToken: string;
  tokenType: string;
  baseUrl: string;
  username: string;
  avatarUrl: string;
}>) =>
  patchJSON<GitProviderToken>(`/git/providers/${encodeURIComponent(id)}`, body);

export const testGitProvider = (id: string) =>
  postJSON<{ ok: boolean; message?: string; latencyMs?: number }>(`/git/providers/${encodeURIComponent(id)}/test`);

export const getGitProvider = (id: string) =>
  fetchJSON<GitProviderToken>(`/git/providers/${encodeURIComponent(id)}`);

// ---- Phase1 OAuth / tree wrappers ----

export const getGitOAuthAuthorizeUrl = (provider: string, redirectTo?: string) => {
  const qs = redirectTo ? `?redirect_to=${encodeURIComponent(redirectTo)}` : "";
  return fetchJSON<{ url: string }>(`/git/oauth/${encodeURIComponent(provider)}/authorize${qs}`);
};

export const getGitOAuthStatus = (provider: string) =>
  fetchJSON<{ provider: string; configured: boolean }>(`/git/oauth/${encodeURIComponent(provider)}/status`);

export const fetchGitRepoTree = (providerId: string, owner: string, repo: string, params?: { path?: string; branch?: string }) => {
  const qs = new URLSearchParams();
  if (params?.path) qs.set("path", params.path);
  if (params?.branch) qs.set("branch", params.branch);
  const suffix = qs.toString() ? `?${qs.toString()}` : "";
  return fetchJSON<unknown[]>(`/git/providers/${encodeURIComponent(providerId)}/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/tree${suffix}`);
};

export const fetchGitRepoContents = (providerId: string, owner: string, repo: string, path: string, branch?: string) => {
  const qs = new URLSearchParams({ path });
  if (branch) qs.set("branch", branch);
  return fetchJSON<unknown>(`/git/providers/${encodeURIComponent(providerId)}/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/contents?${qs.toString()}`);
};

export const fetchGitRepoReadme = (providerId: string, owner: string, repo: string, branch?: string) => {
  const qs = branch ? `?branch=${encodeURIComponent(branch)}` : "";
  return fetchJSON<unknown>(`/git/providers/${encodeURIComponent(providerId)}/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/readme${qs}`);
};

export const fetchGitRepoCommits = (providerId: string, owner: string, repo: string, params?: { branch?: string; path?: string }) => {
  const qs = new URLSearchParams();
  if (params?.branch) qs.set("branch", params.branch);
  if (params?.path) qs.set("path", params.path);
  const suffix = qs.toString() ? `?${qs.toString()}` : "";
  return fetchJSON<unknown[]>(`/git/providers/${encodeURIComponent(providerId)}/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/commits${suffix}`);
};

export const linkGitSourceToEnv = (sourceId: string, environmentId: string) =>
  postJSON<{ sourceId: string; environmentId: string }>(`/git/sources/${encodeURIComponent(sourceId)}/link-env`, { environmentId });

export const unlinkGitSourceFromEnv = (sourceId: string) =>
  deleteJSON<void>(`/git/sources/${encodeURIComponent(sourceId)}/link-env`);

export const listGitWebhookEvents = (sourceId: string) =>
  fetchJSON<unknown[]>(`/git/sources/${encodeURIComponent(sourceId)}/webhook-events`);

export const provisionDeployKey = (sourceId: string, body: { repository?: string; title?: string }) =>
  postJSON<{ credentialId: string; publicKey: string; providerKeyId: string }>(
    `/git/sources/${encodeURIComponent(sourceId)}/provision-deploy-key`,
    body,
  );
