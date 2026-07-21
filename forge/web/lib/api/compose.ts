import { fetchJSON, postJSON, patchJSON, deleteJSON } from './http';

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

export interface ComposeServiceState {
  name: string;
  image: string;
  status: string;
  state: string;
  ports: string;
}

export interface StackStatusResponse {
  stack: ComposeStack;
  services: ComposeServiceState[];
}

export interface ValidateResult {
  valid: boolean;
  errors: { field: string; message: string }[];
  warnings: { field: string; message: string }[];
  summary?: {
    services: { name: string; image: string }[];
    networks: unknown[];
    volumes: unknown[];
  };
}

export async function listStacks(): Promise<ComposeStack[]> {
  return fetchJSON<ComposeStack[]>('/compose');
}

export async function getStack(id: string): Promise<ComposeStack> {
  return fetchJSON<ComposeStack>(`/compose/${id}`);
}

export async function getStackStatus(id: string): Promise<StackStatusResponse> {
  return fetchJSON<StackStatusResponse>(`/compose/${id}/status`);
}

export async function getStackLogs(id: string, service?: string, tail?: number): Promise<{ stackId: string; services: Record<string, string> }> {
  const params = new URLSearchParams();
  if (service) params.set('service', service);
  if (tail) params.set('tail', String(tail));
  const qs = params.toString();
  return fetchJSON(`/compose/${id}/logs${qs ? `?${qs}` : ''}`);
}

export async function createStack(data: {
  name: string;
  composeYaml: string;
  nodeId?: string;
  envVars?: Record<string, string>;
  memoryMb?: number;
  cpuShares?: number;
  diskMb?: number;
  composeType?: string;
  sourceType?: string;
  environmentId?: string;
}): Promise<ComposeStack> {
  return postJSON<ComposeStack>('/compose', data);
}

export async function updateStack(id: string, data: {
  composeYaml?: string;
  envVars?: Record<string, string>;
  memoryMb?: number;
  cpuShares?: number;
  diskMb?: number;
}): Promise<ComposeStack> {
  return patchJSON<ComposeStack>(`/compose/${id}`, data);
}

export async function deleteStack(id: string): Promise<void> {
  return deleteJSON(`/compose/${id}`);
}

export async function deployStack(id: string): Promise<ComposeStack> {
  return postJSON<ComposeStack>(`/compose/${id}/deploy`, {});
}

export async function stopStack(id: string): Promise<ComposeStack> {
  return postJSON<ComposeStack>(`/compose/${id}/stop`, {});
}

export async function startStack(id: string): Promise<ComposeStack> {
  return postJSON<ComposeStack>(`/compose/${id}/start`, {});
}

export async function validateCompose(content: string): Promise<ValidateResult> {
  return postJSON<ValidateResult>('/compose/validate', { content });
}
