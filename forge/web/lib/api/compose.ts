export interface ComposeValidationError {
  field: string;
  message: string;
}

export interface ComposeValidateResult {
  valid: boolean;
  errors?: ComposeValidationError[];
  warnings?: ComposeValidationError[];
  summary?: { services?: { name: string; image: string }[]; networks?: unknown[]; volumes?: unknown[] };
}

import { deleteJSON, fetchJSON, patchJSON, postJSON } from './http';

export interface ComposeStack {
  id: string;
  userId: string;
  name: string;
  nodeId: string;
  status: string;
  composeYaml: string;
  composeHash: string;
  envVars: Record<string, string>;
  memoryMb: number;
  cpuShares: number;
  diskMb: number;
  error: string;
  reservationId: string;
  composeType: string;
  sourceType: string;
  environmentId: string;
  createdAt: string;
  updatedAt: string;
}

export interface ServiceState {
  name: string;
  image: string;
  status: string;
  state: string;
  ports: string;
}

export interface StackStatusResponse {
  stack: ComposeStack;
  services: ServiceState[];
}

export function validateCompose(content: string) {
  return postJSON<ComposeValidateResult>('/compose/validate', { content });
}

export function createComposeStack(body: {
  name: string;
  composeYaml: string;
  nodeId?: string;
  envVars?: Record<string, string>;
  memoryMb?: number;
  cpuShares?: number;
  diskMb?: number;
  composeType?: string;
  sourceType?: string;
}) {
  return postJSON<ComposeStack>('/compose', body);
}

export function listComposeStacks() {
  return fetchJSON<unknown[]>('/compose');
}

export function getComposeStack(id: string) {
  return fetchJSON<unknown>(`/compose/${encodeURIComponent(id)}`);
}

export function updateComposeStack(id: string, body: {
  composeYaml: string;
  envVars?: Record<string, string>;
  memoryMb?: number;
  cpuShares?: number;
  diskMb?: number;
}) {
  return patchJSON<ComposeStack>(`/compose/${encodeURIComponent(id)}`, body);
}

export function deleteComposeStack(id: string, opts?: { volumes?: boolean; force?: boolean }) {
  const qs = new URLSearchParams();
  if (opts?.volumes) qs.set("volumes", "true");
  if (opts?.force) qs.set("force", "true");
  const suffix = qs.toString() ? `?${qs.toString()}` : "";
  return deleteJSON<void>(`/compose/${encodeURIComponent(id)}${suffix}`);
}

export function deployComposeStack(id: string) {
  return postJSON(`/compose/${encodeURIComponent(id)}/deploy`);
}

export function restartComposeStack(id: string) {
  return postJSON(`/compose/${encodeURIComponent(id)}/restart`);
}

export function stopComposeStack(id: string) {
  return postJSON(`/compose/${encodeURIComponent(id)}/stop`);
}

export function startComposeStack(id: string) {
  return postJSON(`/compose/${encodeURIComponent(id)}/start`);
}

export function getComposeStackStatus(id: string) {
  return fetchJSON<StackStatusResponse>(`/compose/${encodeURIComponent(id)}/status`);
}

export function getComposeStackLogs(id: string, service?: string, tail?: number) {
  const params = new URLSearchParams();
  if (service) params.set('service', service);
  if (tail !== undefined) params.set('tail', String(tail));
  const qs = params.toString();
  return fetchJSON<unknown>(`/compose/${encodeURIComponent(id)}/logs${qs ? `?${qs}` : ''}`);
}

// ---- Compose import ----
export function importCompose(body: { name: string; content: string }) {
  return postJSON<unknown>('/compose/import', body);
}

// ---- GitOps 9 endpoints ----
export interface GitOpsDeployRequest {
  name: string;
  nodeId?: string;
  repositoryUrl: string;
  repositoryPath?: string;
  branch?: string;
  envVars?: Record<string, string>;
  memoryMb?: number;
  cpuShares?: number;
  diskMb?: number;
  autoUpdate?: boolean;
  pollIntervalSec?: number;
  credentialId?: string;
}

export function deployFromGit(body: GitOpsDeployRequest) {
  return postJSON<ComposeStack>('/compose/git/deploy', body);
}

export function redeployFromGit(id: string) {
  return postJSON<ComposeStack>(`/compose/git/${encodeURIComponent(id)}/redeploy`);
}

export function checkUpdate(id: string) {
  return fetchJSON<unknown>(`/compose/git/${encodeURIComponent(id)}/check-update`);
}

export function getUpdatePreview(id: string) {
  return fetchJSON<unknown>(`/compose/git/${encodeURIComponent(id)}/preview`);
}

export function rollbackCompose(id: string) {
  return postJSON<ComposeStack>(`/compose/git/${encodeURIComponent(id)}/rollback`);
}

export function pullAndRedeploy(id: string) {
  return postJSON<ComposeStack>(`/compose/git/${encodeURIComponent(id)}/pull-redeploy`);
}

export function setComposeBranch(id: string, branch: string) {
  return fetchJSON<ComposeStack>(`/compose/git/${encodeURIComponent(id)}/branch`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ branch }),
  });
}

export function setComposeAutoUpdate(id: string, enabled: boolean, pollIntervalSec?: number) {
  return fetchJSON<ComposeStack>(`/compose/git/${encodeURIComponent(id)}/auto-update`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enabled, pollIntervalSec }),
  });
}

export function detectDrift(id: string) {
  return fetchJSON<unknown>(`/compose/git/${encodeURIComponent(id)}/drift`);
}

export function getComposeGitStatus(id: string) {
  return fetchJSON<unknown>(`/compose/git/${encodeURIComponent(id)}/status`);
}

export function getLastWebhook(id: string) {
  return fetchJSON<{ lastWebhookAt: string | null }>(`/compose/git/${encodeURIComponent(id)}/last-webhook`);
}

export function handleWebhook(webhookId: string, body: unknown, signature?: string, deliveryId?: string) {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  if (signature) headers['X-Hub-Signature-256'] = signature;
  if (deliveryId) headers['X-GitHub-Delivery'] = deliveryId;
  return postJSON<{ accepted: boolean }>(`/compose/webhook/${encodeURIComponent(webhookId)}`, body);
}

// ---- Compose Projects CRUD ----
export interface ComposeProject {
  id: string;
  name: string;
  serverId?: string;
  composeContent: string;
  parsedConfig: unknown;
  status: string;
  revision: number;
  createdAt: string;
  updatedAt: string;
}

export function listComposeProjects() {
  return fetchJSON<ComposeProject[]>('/compose/projects');
}

export function getComposeProject(id: string) {
  return fetchJSON<ComposeProject>(`/compose/projects/${encodeURIComponent(id)}`);
}

export function updateComposeProject(id: string, body: { name?: string; content: string }) {
  return fetchJSON<ComposeProject>(`/compose/projects/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: body.name, content: body.content }),
  });
}

export function deleteComposeProject(id: string) {
  return deleteJSON<void>(`/compose/projects/${encodeURIComponent(id)}`);
}

export function exportComposeProject(id: string) {
  return fetchJSON<string>(`/compose/projects/${encodeURIComponent(id)}/export`);
}

export function getComposeProjectSummary(id: string) {
  return fetchJSON<{ project: ComposeProject; summary: unknown }>(`/compose/projects/${encodeURIComponent(id)}/summary`);
}
