export type AppType = "image" | "git" | "compose" | "game_server";

export type AppStatus =
  | "running"
  | "stopped"
  | "deploying"
  | "failed"
  | "installing"
  | "pending"
  | "restarting"
  | "starting"
  | "stopping";

export type DeploymentStatus =
  | "pending"
  | "running"
  | "completed"
  | "failed"
  | "canceled";

export type ApiApp = {
  id: string;
  name: string;
  type: AppType;
  status: AppStatus;
  // Explicit desired vs observed lifecycle state, projected straight from the
  // backend store.Application row. Absent means unknown — never defaulted.
  desiredState?: string;
  observedStatus?: string;
  // Convenience primary domain (first configured domain); full list in `domains`.
  domain?: string;
  node?: string;
  region?: string;
  image?: string;
  version?: string;
  cpuUsage?: number;
  cpuLimit?: number;
  memoryUsage?: number;
  memoryLimit?: number;
  diskUsage?: number;
  diskLimit?: number;
  ports: AppPort[];
  domains: AppDomain[];
  envVars: Record<string, string>;
  volumes: AppVolume[];
  createdAt: string;
  updatedAt?: string;
  deployedAt?: string;
  ownerId?: string;
};

export type ApiAppDetail = ApiApp & {
  uptime?: string;
  gitRepo?: string;
  gitBranch?: string;
  gitProvider?: string;
  composeFile?: string;
  composeServices?: ComposeService[];
  healthCheckUrl?: string;
  healthCheckInterval?: number;
  resourceLimits: ResourceLimits;
  serverId?: string;
};

export type AppPort = {
  containerPort: number;
  hostPort: number;
  protocol: "tcp" | "udp";
  name?: string;
};

export type AppDomain = {
  id: string;
  domain: string;
  ssl: boolean;
  sslStatus?: "active" | "pending" | "failed" | "none";
  createdAt: string;
};

export type AppVolume = {
  source: string;
  target: string;
  readOnly: boolean;
};

export type ResourceLimits = {
  cpu: string;
  memory: string;
  disk: string;
};

export type ComposeService = {
  name: string;
  status: "running" | "stopped" | "failed" | "pending";
  image: string;
  ports: string[];
  createdAt: string;
};

export type AppDeployment = {
  id: string;
  status: DeploymentStatus;
  // Fields provided by the backend (store.Deployment projection over
  // GET /apps/:id/deployments).
  serverId?: string;
  strategy?: string;
  image?: string;
  currentRevisionId?: string;
  rolloutStrategy?: string;
  progressPct?: number;
  createdAt?: string;
  updatedAt?: string;
  error?: string;
  // Optional/legacy fields some views still reference; may be absent from the API.
  appId?: string;
  revision?: number;
  source?: AppType;
  trigger?: "manual" | "webhook" | "auto";
  commit?: string;
  commitMessage?: string;
  startedAt?: string;
  completedAt?: string;
  duration?: number;
  log?: string;
};

export type AppBackup = {
  id: string;
  appId: string;
  name: string;
  size?: number;
  status: "creating" | "completed" | "failed" | "restoring";
  createdAt: string;
  completedAt?: string;
};

export type AppTemplate = {
  id: string;
  name: string;
  description: string;
  type: AppType;
  icon?: string;
  image?: string;
  gitUrl?: string;
  composeContent?: string;
  defaultPorts: AppPort[];
  defaultEnvVars: Record<string, string>;
  defaultResources: ResourceLimits;
};

export type CreateAppInput = {
  name: string;
  type: AppType;
  nodeId?: string;
  regionId?: string;
  image?: string;
  registryUrl?: string;
  registryUsername?: string;
  registryPassword?: string;
  gitUrl?: string;
  gitBranch?: string;
  gitProvider?: string;
  composeContent?: string;
  templateId?: string;
  cpuLimit?: string;
  memoryLimit?: string;
  diskLimit?: string;
  ports: AppPort[];
  envVars: Record<string, string>;
  volumes: AppVolume[];
  domains: string[];
  enableTls: boolean;
  autoDeploy?: boolean;
};

export type UpdateAppInput = {
  name?: string;
  image?: string;
  gitBranch?: string;
  composeContent?: string;
  cpuLimit?: string;
  memoryLimit?: string;
  diskLimit?: string;
  ports?: AppPort[];
  envVars?: Record<string, string>;
  volumes?: AppVolume[];
  autoDeploy?: boolean;
};

export type AppCertificate = {
  id: string;
  domain: string;
  status: string;
  issuer: string;
  expiresAt?: string;
  issuedAt?: string;
  autoRenew: boolean;
};

export async function fetchAppCertificates(appId: string): Promise<AppCertificate[]> {
  const domains = await fetchJSON<AppDomain[]>(`/apps/${encodeURIComponent(appId)}/domains`);
  return domains
    .filter((domain) => domain.ssl || domain.sslStatus)
    .map((domain) => ({
      id: domain.id,
      domain: domain.domain,
      status: domain.sslStatus ?? (domain.ssl ? "active" : "none"),
      issuer: "",
      autoRenew: false,
      issuedAt: domain.createdAt,
    }));
}

export type GitSource = {
  repoUrl: string;
  branch: string;
  provider: string;
  commits: GitCommit[];
  webhookUrl: string;
  autoDeploy: boolean;
  lastBuildStatus?: string;
};

export type GitCommit = {
  sha: string;
  message: string;
  author: string;
  timestamp: string;
  url?: string;
};

export type AppLogEntry = {
  timestamp: string;
  line: string;
  stream: "stdout" | "stderr";
};

import { fetchJSON, postJSON, putJSON, patchJSON, deleteJSON, API_BASE_URL } from "./http";
import { getAllTemplates } from "@/lib/app-templates-data";

export async function fetchApps(): Promise<ApiApp[]> {
  const response = await fetchJSON<BackendApplication[] | { data: BackendApplication[] }>("/apps");
  const raw = Array.isArray(response)
    ? response
    : response && Array.isArray((response as { data?: BackendApplication[] }).data)
      ? (response as { data: BackendApplication[] }).data
      : [];
  return raw.map(mapApplication);
}

export async function fetchApp(id: string): Promise<ApiAppDetail> {
  const raw = await fetchJSON<BackendApplication>(`/apps/${encodeURIComponent(id)}`);
  return mapApplicationDetail(raw);
}

export async function createApp(input: CreateAppInput): Promise<ApiApp> {
  // Send both shapes: the canonical {sourceType, sourceConfig} the API
  // persists, plus the flat wizard fields older servers fold themselves.
  const raw = await postJSON<BackendApplication>("/apps", toBackendCreatePayload(input));
  return mapApplication(raw);
}

export async function updateApp(id: string, input: UpdateAppInput): Promise<ApiApp> {
  const raw = await putJSON<BackendApplication>(`/apps/${encodeURIComponent(id)}`, input);
  return mapApplication(raw);
}

// ---- Backend contract mapping -------------------------------------------
// GET /apps returns store.Application rows:
//   {id, name, sourceType: "GIT"|"DOCKER_IMAGE"|"COMPOSE", sourceConfig: {...},
//    desiredState, observedStatus, serverId, ...}
// which this module normalizes into the ApiApp shape the UI renders.

export type BackendApplication = {
  id: string;
  name: string;
  description?: string;
  orgId?: string;
  type?: string;
  sourceType?: string;
  sourceConfig?: unknown;
  status?: string;
  desiredState?: string;
  observedStatus?: string;
  node?: string;
  region?: string;
  serverId?: string;
  image?: string;
  version?: string;
  ports?: AppPort[];
  domains?: AppDomain[];
  envVars?: Record<string, string>;
  volumes?: AppVolume[];
  cpuLimit?: number;
  memoryLimit?: number;
  diskLimit?: number;
  createdAt: string;
  updatedAt?: string;
  deployedAt?: string;
  ownerId?: string;
};

type SourceConfigDoc = {
  image?: string;
  gitUrl?: string;
  gitBranch?: string;
  gitProvider?: string;
  composeContent?: string;
  content?: string;
  nodeId?: string;
  regionId?: string;
  envVars?: Record<string, string>;
  ports?: AppPort[];
  volumes?: AppVolume[];
  memoryMb?: number;
  cpuShares?: number;
  diskMb?: number;
  cpuLimit?: string;
  memoryLimit?: string;
  diskLimit?: string;
};

const SOURCE_TYPE_TO_APP: Record<string, AppType> = {
  DOCKER_IMAGE: "image",
  GIT: "git",
  COMPOSE: "compose",
};

function parseSourceConfig(raw: unknown): SourceConfigDoc {
  if (!raw) return {};
  try {
    const doc = typeof raw === "string" ? (JSON.parse(raw) as unknown) : raw;
    if (doc && typeof doc === "object" && !Array.isArray(doc)) return doc as SourceConfigDoc;
  } catch {
    // Unreadable config is not fatal for the list view.
  }
  return {};
}

function splitImageTag(image?: string): { image?: string; version?: string } {
  if (!image) return {};
  const at = image.indexOf("@");
  const ref = at >= 0 ? image.slice(0, at) : image;
  const slash = ref.lastIndexOf("/");
  const colon = ref.lastIndexOf(":");
  if (colon > slash) {
    return { image, version: ref.slice(colon + 1) || undefined };
  }
  return { image, version: undefined };
}

function toBackendCreatePayload(input: CreateAppInput): Record<string, unknown> {
  const sourceType =
    input.type === "image" ? "DOCKER_IMAGE" : input.type === "git" ? "GIT" : input.type === "compose" ? "COMPOSE" : input.type;
  const sourceConfig: Record<string, unknown> = {};
  if (input.image?.trim()) sourceConfig.image = input.image.trim();
  if (input.gitUrl?.trim()) sourceConfig.gitUrl = input.gitUrl.trim();
  if (input.gitBranch?.trim()) sourceConfig.gitBranch = input.gitBranch.trim();
  if (input.gitProvider?.trim()) sourceConfig.gitProvider = input.gitProvider.trim();
  if (input.composeContent?.trim()) sourceConfig.composeContent = input.composeContent;
  if (input.nodeId?.trim()) sourceConfig.nodeId = input.nodeId.trim();
  if (input.regionId?.trim()) sourceConfig.regionId = input.regionId.trim();
  return { ...input, sourceType, sourceConfig };
}

export function mapApplication(raw: BackendApplication): ApiApp {
  const cfg = parseSourceConfig(raw.sourceConfig);
  const upperType = typeof raw.sourceType === "string" ? raw.sourceType.toUpperCase() : "";
  const type: AppType =
    SOURCE_TYPE_TO_APP[upperType] ??
    (raw.type === "image" || raw.type === "git" || raw.type === "compose" || raw.type === "game_server" ? raw.type : "image");
  const split = splitImageTag(cfg.image ?? raw.image);
  return {
    id: raw.id,
    name: raw.name,
    type,
    status: (raw.observedStatus || raw.status || "idle") as ApiApp["status"],
    desiredState: raw.desiredState,
    observedStatus: raw.observedStatus,
    domain: (raw.domains ?? [])[0]?.domain,
    node: raw.node ?? cfg.nodeId,
    region: raw.region ?? cfg.regionId,
    image: split.image,
    version: split.version ?? raw.version,
    cpuUsage: undefined,
    cpuLimit: raw.cpuLimit,
    memoryUsage: undefined,
    memoryLimit: raw.memoryLimit,
    diskUsage: undefined,
    diskLimit: raw.diskLimit,
    ports: cfg.ports ?? raw.ports ?? [],
    domains: raw.domains ?? [],
    envVars: cfg.envVars ?? raw.envVars ?? {},
    volumes: cfg.volumes ?? raw.volumes ?? [],
    createdAt: raw.createdAt,
    updatedAt: raw.updatedAt,
    deployedAt: raw.deployedAt,
    ownerId: raw.ownerId,
  };
}

export function mapApplicationDetail(raw: BackendApplication): ApiAppDetail {
  const cfg = parseSourceConfig(raw.sourceConfig);
  const base = mapApplication(raw);
  return {
    ...base,
    gitRepo: cfg.gitUrl,
    gitBranch: cfg.gitBranch,
    gitProvider: cfg.gitProvider,
    resourceLimits: {
      cpu: cfg.cpuLimit ?? "",
      memory: cfg.memoryLimit ?? (cfg.memoryMb != null ? String(cfg.memoryMb) : ""),
      disk: cfg.diskLimit ?? (cfg.diskMb != null ? String(cfg.diskMb) : ""),
    },
  };
}

export function deleteApp(id: string): Promise<void> {
  return deleteJSON(`/apps/${encodeURIComponent(id)}`);
}

export function startApp(id: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/apps/${encodeURIComponent(id)}/start`);
}

export function stopApp(id: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/apps/${encodeURIComponent(id)}/stop`);
}

export function restartApp(id: string): Promise<{ id: string; status: string }> {
  return postJSON<{ id: string; status: string }>(`/apps/${encodeURIComponent(id)}/restart`);
}

export function fetchAppDeployments(appId: string): Promise<AppDeployment[]> {
  return fetchJSON<AppDeployment[]>(`/apps/${encodeURIComponent(appId)}/deployments`);
}

export async function fetchAllDeployments(): Promise<AppDeployment[]> {
  return fetchJSON<AppDeployment[]>("/admin/deployments");
}

export function triggerDeploy(appId: string): Promise<{ id: string; status: string }> {
  return postJSON<{ id: string; status: string }>(`/apps/${encodeURIComponent(appId)}/deploy`);
}

export function fetchAppLogs(appId: string): Promise<AppLogEntry[]> {
  return fetchJSON<AppLogEntry[]>(`/apps/${encodeURIComponent(appId)}/logs`);
}

export async function fetchAppServiceLogs(appId: string, service: string): Promise<AppLogEntry[]> {
  const logs = await fetchJSON<Array<{ stage: string; message: string; createdAt: string }>>(
    `/apps/${encodeURIComponent(appId)}/logs?service=${encodeURIComponent(service)}`,
  );
  return logs.map((entry) => ({
    timestamp: entry.createdAt,
    line: `[${entry.stage}] ${entry.message}`,
    stream: "stdout",
  }));
}

export function fetchAppDomains(appId: string): Promise<AppDomain[]> {
  return fetchJSON<AppDomain[]>(`/apps/${encodeURIComponent(appId)}/domains`);
}

export function addAppDomain(appId: string, domain: string, enableTls?: boolean): Promise<{ ok: boolean; domain: string }> {
  return postJSON<{ ok: boolean; domain: string }>(`/apps/${encodeURIComponent(appId)}/domains`, { domain, enableTls });
}

export function deleteAppDomain(appId: string, domainId: string): Promise<void> {
  return deleteJSON(`/apps/${encodeURIComponent(appId)}/domains/${encodeURIComponent(domainId)}`);
}

export function fetchAppBackups(appId: string): Promise<AppBackup[]> {
  return fetchJSON<AppBackup[]>(`/apps/${encodeURIComponent(appId)}/backups`);
}

export function createAppBackup(appId: string, name?: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/apps/${encodeURIComponent(appId)}/backups`, { name });
}

export function restoreAppBackup(appId: string, backupId: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/apps/${encodeURIComponent(appId)}/backups/${encodeURIComponent(backupId)}/restore`);
}

export function deleteAppBackup(appId: string, backupId: string): Promise<void> {
  return deleteJSON(`/apps/${encodeURIComponent(appId)}/backups/${encodeURIComponent(backupId)}`);
}

export function fetchAppComposeConfig(appId: string): Promise<{ sourceType: string; sourceConfig: unknown }> {
  return fetchJSON<{ sourceType: string; sourceConfig: unknown }>(`/apps/${encodeURIComponent(appId)}/compose`);
}

export function updateAppComposeConfig(appId: string, sourceConfig: unknown): Promise<{ sourceType: string; sourceConfig: unknown }> {
  return putJSON<{ sourceType: string; sourceConfig: unknown }>(`/apps/${encodeURIComponent(appId)}/compose`, { sourceConfig });
}

export function redeployComposeStack(appId: string): Promise<{ id: string; status: string }> {
  return postJSON<{ id: string; status: string }>(`/apps/${encodeURIComponent(appId)}/compose/redeploy`);
}

export function fetchAppGitSource(appId: string): Promise<{ sourceType: string; sourceConfig: unknown }> {
  return fetchJSON<{ sourceType: string; sourceConfig: unknown }>(`/apps/${encodeURIComponent(appId)}/git`);
}

export function updateAppGitBranch(appId: string, branch: string): Promise<{ sourceType: string; sourceConfig: unknown }> {
  return patchJSON<{ sourceType: string; sourceConfig: unknown }>(`/apps/${encodeURIComponent(appId)}/git`, { branch });
}

export function toggleAppAutoDeploy(appId: string, enabled: boolean): Promise<{ ok: boolean }> {
  return patchJSON<{ ok: boolean }>(`/apps/${encodeURIComponent(appId)}/git/auto-deploy`, { autoDeploy: enabled });
}

export function fetchAppConsoleWSURL(serverId: string): string {
  const protocol = typeof window !== "undefined" && window.location.protocol === "https:" ? "wss:" : "ws:";
  const wsBase = API_BASE_URL.replace("http:", protocol).replace("https:", protocol);
  return `${wsBase}/servers/${encodeURIComponent(serverId)}/ws/console`;
}

/**
 * Opens the app console WebSocket through the short-lived ticket flow so the
 * connection works across origins (the ticket replaces the session cookie,
 * which browsers will not attach to cross-origin WebSockets). Falls back to a
 * bare same-origin URL when no ticket endpoint is reachable is NOT used here:
 * the ticket call is mandatory because /ws/console requires session auth.
 */
export async function connectAppConsoleWebSocket(serverId: string): Promise<WebSocket> {
  const ticket = await postJSON<{ token: string }>(
    `/servers/${encodeURIComponent(serverId)}/ws/ticket?stream=console`,
  );
  return new WebSocket(`${fetchAppConsoleWSURL(serverId)}?token=${encodeURIComponent(ticket.token)}`);
}

export async function fetchAppTemplates(): Promise<AppTemplate[]> {
  try {
    return await fetchJSON<AppTemplate[]>("/admin/app-templates");
  } catch (error) {
    if (error instanceof TypeError) {
      console.warn("API unreachable, using local templates", error);
      return getAllTemplates();
    }
    throw error;
  }
}

export function typeLabel(type: AppType | string | null | undefined): string {
  switch (type) {
    case "image": return "Docker Image";
    case "git": return "Git Repository";
    case "compose": return "Docker Compose";
    case "game_server": return "Game Server";
    default: return typeof type === "string" && type ? type : "unknown";
  }
}

export function statusLabel(status: AppStatus | string | null | undefined): string {
  if (typeof status !== "string" || !status) return "unknown";
  return status.replace(/_/g, " ");
}

// `statusTone` and `deploymentStatusTone` used to live here, returning colour
// words and defaulting an unrecognised status to "neutral". They had no
// importers, but `lib/api.ts` does `export * from './api/apps'` while *not*
// exporting `./api/status` — so `import { statusTone } from "@/lib/api"`
// resolved to these instead of the real ones. Both now live only in
// `lib/api/status.ts`; import from there.

export { fetchDnsProviders } from "./dns";
