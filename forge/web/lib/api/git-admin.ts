import { deleteJSON, fetchJSON, postJSON } from "./http";
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
