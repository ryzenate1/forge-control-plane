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
  return patchJSON(`/compose/${encodeURIComponent(id)}`, body);
}

export function deleteComposeStack(id: string) {
  return deleteJSON(`/compose/${encodeURIComponent(id)}`);
}

export function deployComposeStack(id: string) {
  return postJSON(`/compose/${encodeURIComponent(id)}/deploy`);
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
