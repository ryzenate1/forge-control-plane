import { fetchJSON, postJSON, deleteJSON } from './http';

export interface ApiBuildpack {
  id: string;
  name: string;
  description: string;
  url: string;
  builderType: string;
  createdAt: string;
}

export interface ApiServerBuildpack {
  id: string;
  serverId: string;
  buildpackId: string;
  priority: number;
  buildpack?: ApiBuildpack;
}

export interface ApiAppBuild {
  id: string;
  serverId: string;
  buildpackId?: string;
  status: string;
  buildLog: string;
  imageTag: string;
  createdAt: string;
  buildpack?: ApiBuildpack;
}

export interface LanguageDetectResult {
  language: string;
  confidence: number;
  buildpacks?: ApiBuildpack[];
}

export interface ApiBuildCreateResult {
  data: ApiAppBuild;
}

export interface ApiBuildpackListResult {
  data: ApiBuildpack[];
}

export interface ApiServerBuildpackListResult {
  data: ApiServerBuildpack[];
}

export interface ApiAppBuildListResult {
  data: ApiAppBuild[];
}

export interface ApiBuildGetResult {
  data: ApiAppBuild;
}

export interface ApiLanguageDetectResult {
  data: LanguageDetectResult;
}

// Admin buildpack management
export async function fetchBuildpacks(): Promise<ApiBuildpack[]> {
  const res = await fetchJSON<ApiBuildpackListResult>('/admin/buildpacks');
  return res.data;
}

export async function createBuildpack(input: { name: string; description?: string; url?: string; builderType: string }): Promise<ApiBuildpack> {
  const res = await postJSON<{ data: ApiBuildpack }>('/admin/buildpacks', input);
  return res.data;
}

// Server buildpack assignments
export async function fetchServerBuildpacks(serverId: string): Promise<ApiServerBuildpack[]> {
  const res = await fetchJSON<ApiServerBuildpackListResult>(`/servers/${encodeURIComponent(serverId)}/buildpacks`);
  return res.data;
}

export async function assignBuildpackToServer(serverId: string, buildpackId: string, priority = 0): Promise<ApiServerBuildpack> {
  const res = await postJSON<{ data: ApiServerBuildpack }>(`/servers/${encodeURIComponent(serverId)}/buildpacks`, { buildpackId, priority });
  return res.data;
}

export async function removeBuildpackFromServer(serverId: string, buildpackId: string): Promise<void> {
  await deleteJSON(`/servers/${encodeURIComponent(serverId)}/buildpacks/${encodeURIComponent(buildpackId)}`);
}

// App builds
export async function triggerBuild(serverId: string, buildpackId?: string): Promise<ApiAppBuild> {
  const res = await postJSON<ApiBuildCreateResult>(`/servers/${encodeURIComponent(serverId)}/builds`, { buildpackId });
  return res.data;
}

export async function fetchServerBuilds(serverId: string): Promise<ApiAppBuild[]> {
  const res = await fetchJSON<ApiAppBuildListResult>(`/servers/${encodeURIComponent(serverId)}/builds`);
  return res.data;
}

export async function fetchBuild(serverId: string, buildId: string): Promise<ApiAppBuild> {
  const res = await fetchJSON<ApiBuildGetResult>(`/servers/${encodeURIComponent(serverId)}/builds/${encodeURIComponent(buildId)}`);
  return res.data;
}

// Language detection
export async function detectLanguage(files: string[]): Promise<LanguageDetectResult> {
  const res = await postJSON<ApiLanguageDetectResult>('/builds/language/detect', { files });
  return res.data;
}
