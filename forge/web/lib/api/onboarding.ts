import { fetchJSON, postJSON, unwrapData, unwrapList } from "./http";

export type OnboardingToken = {
  id: string;
  nodeId: string;
  createdAt: string;
  expiresAt: string;
  approvedAt?: string | null;
  approvedBy?: string;
  revokedAt?: string | null;
  revokedReason?: string;
  state: string; // pending | approved | rejected | revoked | consumed
};

export type CreateOnboardingTokenResult = {
  token: string;
  tokenId: string;
  nodeId: string;
  expiresAt: string;
  state: string;
};

export async function createOnboardingToken(input: { nodeId: string; ttlHours?: number }): Promise<CreateOnboardingTokenResult> {
  const res = await postJSON<CreateOnboardingTokenResult | { data: CreateOnboardingTokenResult }>(
    "/onboarding-tokens",
    { nodeId: input.nodeId, ttlHours: input.ttlHours ?? 24 },
  );
  // Handlers return { token, tokenId, nodeId, expiresAt, state } directly (201), not { data: ... }
  return unwrapData(res);
}

export async function listOnboardingTokens(nodeId: string): Promise<OnboardingToken[]> {
  const res = await fetchJSON<{ data: OnboardingToken[] } | OnboardingToken[]>(
    `/onboarding-tokens?nodeId=${encodeURIComponent(nodeId)}`,
  );
  return unwrapList(res);
}

export async function fetchOnboardingToken(tokenId: string): Promise<OnboardingToken> {
  const res = await fetchJSON<OnboardingToken | { data: OnboardingToken }>(`/onboarding-tokens/${encodeURIComponent(tokenId)}`);
  return unwrapData(res);
}

export async function approveOnboardingToken(tokenId: string): Promise<{ approved: boolean }> {
  return postJSON<{ approved: boolean }>(`/onboarding-tokens/${encodeURIComponent(tokenId)}/approve`, {});
}

export async function rejectOnboardingToken(tokenId: string, reason?: string): Promise<{ rejected: boolean }> {
  return postJSON<{ rejected: boolean }>(`/onboarding-tokens/${encodeURIComponent(tokenId)}/reject`, { reason: reason ?? "" });
}

export async function revokeOnboardingToken(tokenId: string, reason?: string): Promise<{ revoked: boolean }> {
  return postJSON<{ revoked: boolean }>(`/onboarding-tokens/${encodeURIComponent(tokenId)}/revoke`, { reason: reason ?? "" });
}

// ---------------------------------------------------------------------------
// Phase 8 wizard types & helpers (ported from web/lib/api/onboarding.ts)
// These satisfy forge/web/components/admin/onboarding-manager.tsx which was
// copied from web but the forge API module only exported token helpers.
// ---------------------------------------------------------------------------

export type StatusView = {
  connected: boolean;
  providers: string[];
  sourceCount: number;
  hasApps: boolean;
  templateKeys: string[];
  oauthUrl?: string;
};

export type GitProviderRepo = {
  id: string;
  name: string;
  fullName: string;
  cloneUrl: string;
  defaultBranch: string;
};

export type GitProviderBranch = {
  name: string;
  commit: string;
};

export type DeployResult = {
  deploymentId: string;
  status: string;
  appId: string;
  appName: string;
  url: string;
  internalUrl: string;
  builder: string;
};

export type LoadedSource = {
  id: string;
  name: string;
  owner: string;
  branch: string;
  cloneUrl: string;
  defaultBranch: string;
  files: string[];
};

export async function fetchOnboardingStatus(): Promise<StatusView> {
  const data = await fetchJSON<StatusView | { data: StatusView }>("/onboarding/status");
  const status = unwrapData(data);
  // Backend may serialize nil slices as null; normalize to arrays.
  return {
    ...status,
    providers: status.providers ?? [],
    templateKeys: status.templateKeys ?? [],
  };
}

// legacy alias used by web copy
export async function getStatus(): Promise<StatusView> {
  return fetchOnboardingStatus();
}

export async function fetchRepos(providerId?: string): Promise<GitProviderRepo[]> {
  const qs = providerId ? `?providerId=${encodeURIComponent(providerId)}` : "";
  const data = await fetchJSON<{ data: GitProviderRepo[] } | GitProviderRepo[]>(`/onboarding/repos${qs}`);
  return unwrapList(data);
}

export async function listRepos(providerId?: string): Promise<GitProviderRepo[]> {
  return fetchRepos(providerId);
}

export async function fetchBranches(repo: string, providerId?: string): Promise<GitProviderBranch[]> {
  const qs = providerId ? `?providerId=${encodeURIComponent(providerId)}` : "";
  const data = await fetchJSON<{ data: GitProviderBranch[] } | GitProviderBranch[]>(
    `/onboarding/repos/${encodeURIComponent(repo)}/branches${qs}`,
  );
  return unwrapList(data);
}

export async function listBranches(repo: string, providerId?: string): Promise<GitProviderBranch[]> {
  return fetchBranches(repo, providerId);
}

export async function fetchConnect(payload: {
  provider: string;
  providerName?: string;
  accessToken: string;
  refreshToken?: string;
  baseUrl?: string;
  username?: string;
}): Promise<unknown> {
  return postJSON<unknown>("/onboarding/connect", payload);
}

export async function connect(payload: {
  provider: string;
  providerName?: string;
  accessToken: string;
  refreshToken?: string;
  baseUrl?: string;
  username?: string;
}): Promise<unknown> {
  return fetchConnect(payload);
}

export async function fetchDeploy(payload: {
  repo: string;
  branch?: string;
  root?: string;
  buildType: string;
  dockerfile?: string;
  name?: string;
  port?: number;
  providerTokenId?: string;
}): Promise<DeployResult> {
  return postJSON<DeployResult>("/onboarding/deploy", payload);
}

export async function deploy(payload: {
  repo: string;
  branch?: string;
  root?: string;
  buildType: string;
  dockerfile?: string;
  name?: string;
  port?: number;
  providerTokenId?: string;
}): Promise<DeployResult> {
  return fetchDeploy(payload);
}

export async function fetchLoadedSources(): Promise<LoadedSource[]> {
  const data = await fetchJSON<{ data: LoadedSource[] } | LoadedSource[]>("/ide/files");
  return unwrapList(data);
}

export async function listLoadedSources(): Promise<LoadedSource[]> {
  return fetchLoadedSources();
}
