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
  appId: string;
  revision: number;
  status: DeploymentStatus;
  source: AppType;
  trigger: "manual" | "webhook" | "auto";
  commit?: string;
  commitMessage?: string;
  image?: string;
  startedAt: string;
  completedAt?: string;
  duration?: number;
  log?: string;
  error?: string;
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

export function fetchAppCertificates(appId: string): Promise<AppCertificate[]> {
  return fetchJSON<AppCertificate[]>(`/apps/${encodeURIComponent(appId)}/certificates`);
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

import { fetchJSON, postJSON, putJSON, patchJSON, deleteJSON } from "./http";
import { getAllTemplates } from "@/lib/app-templates-data";

export function fetchApps(): Promise<ApiApp[]> {
  return fetchJSON<ApiApp[]>("/apps");
}

export function fetchApp(id: string): Promise<ApiAppDetail> {
  return fetchJSON<ApiAppDetail>(`/apps/${encodeURIComponent(id)}`);
}

export function createApp(input: CreateAppInput): Promise<ApiApp> {
  return postJSON<ApiApp>("/apps", input);
}

export function updateApp(id: string, input: UpdateAppInput): Promise<ApiApp> {
  return putJSON<ApiApp>(`/apps/${encodeURIComponent(id)}`, input);
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

export function restartApp(id: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/apps/${encodeURIComponent(id)}/restart`);
}

export function fetchAppDeployments(appId: string): Promise<AppDeployment[]> {
  return fetchJSON<AppDeployment[]>(`/apps/${encodeURIComponent(appId)}/deployments`);
}

export async function fetchAllDeployments(): Promise<AppDeployment[]> {
  const res = await fetchJSON<{ data: AppDeployment[] }>("/admin/deployments?type=app");
  return res.data ?? [];
}

export function triggerDeploy(appId: string): Promise<AppDeployment> {
  return postJSON<AppDeployment>(`/apps/${encodeURIComponent(appId)}/deploy`);
}

export function fetchAppLogs(appId: string): Promise<AppLogEntry[]> {
  return fetchJSON<AppLogEntry[]>(`/apps/${encodeURIComponent(appId)}/logs`);
}

export function fetchAppServiceLogs(appId: string, service: string): Promise<AppLogEntry[]> {
  return fetchJSON<AppLogEntry[]>(`/apps/${encodeURIComponent(appId)}/compose/services/${encodeURIComponent(service)}/logs`);
}

export function fetchAppDomains(appId: string): Promise<AppDomain[]> {
  return fetchJSON<AppDomain[]>(`/apps/${encodeURIComponent(appId)}/domains`);
}

export function addAppDomain(appId: string, domain: string, enableTls?: boolean): Promise<AppDomain> {
  return postJSON<AppDomain>(`/apps/${encodeURIComponent(appId)}/domains`, { domain, enableTls });
}

export function deleteAppDomain(appId: string, domainId: string): Promise<void> {
  return deleteJSON(`/apps/${encodeURIComponent(appId)}/domains/${encodeURIComponent(domainId)}`);
}

export function fetchAppBackups(appId: string): Promise<AppBackup[]> {
  return fetchJSON<AppBackup[]>(`/apps/${encodeURIComponent(appId)}/backups`);
}

export function createAppBackup(appId: string, name?: string): Promise<AppBackup> {
  return postJSON<AppBackup>(`/apps/${encodeURIComponent(appId)}/backups`, { name });
}

export function restoreAppBackup(appId: string, backupId: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/apps/${encodeURIComponent(appId)}/backups/${encodeURIComponent(backupId)}/restore`);
}

export function deleteAppBackup(appId: string, backupId: string): Promise<void> {
  return deleteJSON(`/apps/${encodeURIComponent(appId)}/backups/${encodeURIComponent(backupId)}`);
}

export function fetchAppComposeConfig(appId: string): Promise<{ content: string; services: ComposeService[] }> {
  return fetchJSON<{ content: string; services: ComposeService[] }>(`/apps/${encodeURIComponent(appId)}/compose`);
}

export function updateAppComposeConfig(appId: string, content: string): Promise<{ ok: boolean }> {
  return putJSON<{ ok: boolean }>(`/apps/${encodeURIComponent(appId)}/compose`, { content });
}

export function redeployComposeStack(appId: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/apps/${encodeURIComponent(appId)}/compose/redeploy`);
}

export function fetchAppGitSource(appId: string): Promise<GitSource> {
  return fetchJSON<GitSource>(`/apps/${encodeURIComponent(appId)}/git`);
}

export function updateAppGitBranch(appId: string, branch: string): Promise<{ ok: boolean }> {
  return patchJSON<{ ok: boolean }>(`/apps/${encodeURIComponent(appId)}/git`, { branch });
}

export function toggleAppAutoDeploy(appId: string, enabled: boolean): Promise<{ ok: boolean }> {
  return patchJSON<{ ok: boolean }>(`/apps/${encodeURIComponent(appId)}/git/auto-deploy`, { enabled });
}

export function fetchAppConsoleWSURL(appId: string): string {
  const API_BASE_URL =
    process.env.NEXT_PUBLIC_API_URL ??
    (process.env.NODE_ENV === "development" ? "http://localhost:8080/api/v1" : "/api/v1");
  const protocol = typeof window !== "undefined" && window.location.protocol === "https:" ? "wss:" : "ws:";
  const wsBase = API_BASE_URL.replace("http:", protocol).replace("https:", protocol);
  return `${wsBase}/apps/${encodeURIComponent(appId)}/ws/console`;
}

export async function fetchAppTemplates(): Promise<AppTemplate[]> {
  try {
    return await fetchJSON<AppTemplate[]>("/admin/app-templates");
  } catch {
    return getAllTemplates();
  }
}

export function typeLabel(type: AppType): string {
  switch (type) {
    case "image": return "Docker Image";
    case "git": return "Git Repository";
    case "compose": return "Docker Compose";
    case "game_server": return "Game Server";
    default: return type;
  }
}

export function statusLabel(status: AppStatus): string {
  return status.replace(/_/g, " ");
}

export function statusTone(status: AppStatus): "green" | "red" | "yellow" | "blue" | "neutral" {
  switch (status) {
    case "running": return "green";
    case "stopped": return "neutral";
    case "deploying": case "pending": case "installing": case "starting": case "restarting": return "blue";
    case "stopping": return "yellow";
    case "failed": return "red";
    default: return "neutral";
  }
}

export function deploymentStatusTone(status: DeploymentStatus): "green" | "red" | "yellow" | "blue" | "neutral" {
  switch (status) {
    case "completed": return "green";
    case "failed": return "red";
    case "canceled": return "neutral";
    case "running": return "blue";
    case "pending": return "yellow";
    default: return "neutral";
  }
}

export type DNSProvider = {
  id: string;
  name: string;
  provider: string;
  verificationStatus?: string;
  createdAt: string;
};

export function fetchDNSProviders(): Promise<DNSProvider[]> {
  return fetchJSON<DNSProvider[]>("/dns/providers");
}
