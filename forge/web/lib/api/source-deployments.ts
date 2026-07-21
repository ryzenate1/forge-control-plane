import { fetchJSON, postJSON, patchJSON, deleteJSON, ApiError } from './http';

export interface GitProvider {
  id: string;
  userId: string;
  name: string;
  type: 'github' | 'gitlab' | 'bitbucket' | 'gitea' | 'generic';
  accessToken?: string;
  tokenType: string;
  expiresAt?: string;
  scope: string;
  baseUrl: string;
  username: string;
  avatarUrl: string;
  createdAt: string;
  updatedAt: string;
}

export interface SourceDeployment {
  id: string;
  serverId?: string;
  gitProviderId?: string;
  repository: string;
  branch: string;
  buildType: 'dockerfile' | 'nixpacks' | 'heroku' | 'paketo' | 'static';
  buildContext: string;
  dockerfilePath: string;
  status: string;
  commitHash: string;
  commitMessage: string;
  commitAuthor: string;
  imageTag: string;
  registry: string;
  registryCredentialId: string;
  autoDeploy: boolean;
  webhookId: string;
  webhookUrl: string;
  healthCheckPath: string;
  healthCheckPort: number;
  rollbackOnHealthFailure: boolean;
  createdBy?: string;
  createdAt: string;
  updatedAt: string;
}

export interface BuildLog {
  id: string;
  deploymentId: string;
  stage: string;
  message: string;
  createdAt: string;
}

export interface CreateSourceDeploymentRequest {
  serverId?: string;
  gitProviderId?: string;
  repository: string;
  branch?: string;
  buildType?: string;
  buildContext?: string;
  dockerfilePath?: string;
  autoDeploy?: boolean;
  registry?: string;
  registryCredentialId?: string;
  healthCheckPath?: string;
  healthCheckPort?: number;
  rollbackOnHealthFailure?: boolean;
}

export interface GitProviderRepo {
  name: string;
  fullName: string;
  cloneUrl: string;
  sshUrl: string;
  defaultBranch: string;
  private: boolean;
  description: string;
}

export interface GitProviderBranch {
  name: string;
  sha: string;
  isMain: boolean;
}

export async function listGitProviders(): Promise<GitProvider[]> {
  return fetchJSON<GitProvider[]>('/git/providers');
}

export async function connectGitProvider(body: {
  provider: string;
  accessToken: string;
  baseUrl?: string;
  providerName?: string;
}): Promise<GitProvider> {
  return postJSON<GitProvider>('/git/providers', body);
}

export async function disconnectGitProvider(id: string): Promise<void> {
  await deleteJSON(`/git/providers/${encodeURIComponent(id)}`);
}

export async function listProviderRepos(providerId: string): Promise<GitProviderRepo[]> {
  return fetchJSON<GitProviderRepo[]>(`/git/providers/${encodeURIComponent(providerId)}/repos`);
}

export async function listProviderBranches(providerId: string, repoFullName: string): Promise<GitProviderBranch[]> {
  return fetchJSON<GitProviderBranch[]>(`/git/providers/${encodeURIComponent(providerId)}/branches?repo=${encodeURIComponent(repoFullName)}`);
}

export async function listSourceDeployments(): Promise<SourceDeployment[]> {
  return fetchJSON<SourceDeployment[]>('/source-deployments');
}

export async function getSourceDeployment(id: string): Promise<SourceDeployment> {
  return fetchJSON<SourceDeployment>(`/source-deployments/${encodeURIComponent(id)}`);
}

export async function createSourceDeployment(req: CreateSourceDeploymentRequest): Promise<SourceDeployment> {
  return postJSON<SourceDeployment>('/source-deployments', req);
}

export async function updateSourceDeployment(id: string, req: Partial<CreateSourceDeploymentRequest>): Promise<SourceDeployment> {
  return patchJSON<SourceDeployment>(`/source-deployments/${encodeURIComponent(id)}`, req);
}

export async function deleteSourceDeployment(id: string): Promise<void> {
  await deleteJSON<void>(`/source-deployments/${encodeURIComponent(id)}`);
}

export async function deploySourceDeployment(id: string): Promise<void> {
  return postJSON<void>(`/source-deployments/${encodeURIComponent(id)}/deploy`);
}

export async function cancelSourceDeployment(id: string): Promise<void> {
  return postJSON<void>(`/source-deployments/${encodeURIComponent(id)}/cancel`);
}

export async function getDeploymentBuildLogs(id: string): Promise<BuildLog[]> {
  return fetchJSON<BuildLog[]>(`/source-deployments/${encodeURIComponent(id)}/logs`);
}
