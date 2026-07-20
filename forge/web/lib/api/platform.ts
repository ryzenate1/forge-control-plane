import { fetchJSON, postJSON } from './http';

export type PlatformScope = {
  organizationId: string;
  projectId: string;
  environmentId: string;
};

export type PlatformWorkload = {
  id: string;
  environmentId: string;
  kind: string;
  name: string;
  desiredGeneration: number;
  observedGeneration: number;
  desiredState: string;
  observedState: string;
  currentRevisionId?: string;
  lastObservationAt?: string;
  lastReconcileError?: string;
  createdAt: string;
  updatedAt: string;
};

export type PlatformRevision = {
  id: string;
  workloadId: string;
  number: number;
  schemaVersion: number;
  spec: unknown;
  createdAt: string;
};

export type PlatformWorkloadInstance = {
  id: string;
  workloadId: string;
  revisionId: string;
  nodeId?: string;
  desiredState: string;
  observedState: string;
  createdAt: string;
  updatedAt: string;
};

export type PlatformApplicationDetail = {
  application: PlatformWorkload;
  revision: PlatformRevision;
  instances: PlatformWorkloadInstance[];
  operations: PlatformOperation[];
};

export type PlatformOperation = {
  id: string;
  kind: string;
  resourceType: string;
  resourceId: string;
  status: 'queued' | 'running' | 'waiting' | 'retrying' | 'cancelling' | 'rolling_back' | 'succeeded' | 'failed' | 'cancelled';
  desiredGeneration: number;
  observedGeneration: number;
  error?: string;
  createdAt: string;
  updatedAt: string;
};

export type CreatePlatformWorkloadInput = {
  environmentId: string;
  kind: string;
  name: string;
  desiredState?: string;
  spec?: unknown;
};

export type CreatePlatformApplicationInput = {
  environmentId: string;
  nodeId: string;
  name: string;
  source: 'image' | 'git' | 'compose';
  image?: string;
  repositoryUrl?: string;
  branch?: string;
  baseDirectory?: string;
  dockerfilePath?: string;
  buildArgs?: Record<string, string>;
  composeFile?: string;
  deployment?: 'rolling' | 'blue-green' | 'recreate';
  healthCheckPath?: string;
  healthCheckPort?: number;
  command?: string[];
  environment?: Record<string, string>;
  memoryMb?: number;
  cpuPercent?: number;
  diskMb?: number;
};

export type PlatformComposeValidation = {
  name: string;
  services: Array<{ name: string; image?: string; build: boolean; dependsOn?: string[]; ports: number }>;
  networks: string[];
  volumes: string[];
  warnings?: string[];
};

export async function fetchDefaultPlatformScope(): Promise<PlatformScope> {
  return fetchJSON<PlatformScope>('/platform/scope/default');
}

export async function fetchPlatformWorkloads(environmentId?: string): Promise<PlatformWorkload[]> {
  const query = environmentId ? `?environmentId=${encodeURIComponent(environmentId)}` : '';
  return fetchJSON<PlatformWorkload[]>(`/platform/workloads${query}`);
}

export async function fetchPlatformApplications(environmentId?: string): Promise<PlatformWorkload[]> {
  const query = environmentId ? `?environmentId=${encodeURIComponent(environmentId)}` : '';
  return fetchJSON<PlatformWorkload[]>(`/platform/applications${query}`);
}

export async function fetchPlatformApplication(id: string): Promise<PlatformApplicationDetail> {
  return fetchJSON<PlatformApplicationDetail>(`/platform/applications/${encodeURIComponent(id)}`);
}

export async function createPlatformWorkload(input: CreatePlatformWorkloadInput): Promise<{ workload: PlatformWorkload; revision: PlatformRevision }> {
  return postJSON('/platform/workloads', input);
}

export async function createPlatformApplication(input: CreatePlatformApplicationInput): Promise<{ workload: PlatformWorkload; operation: { id: string; status: string } }> {
  return postJSON('/platform/applications', input);
}

export async function validatePlatformCompose(input: { content: string; environment?: Record<string, string> }): Promise<PlatformComposeValidation> {
  return postJSON('/platform/compose/validate', input);
}

export async function fetchPlatformOperations(resourceId?: string): Promise<PlatformOperation[]> {
  const query = resourceId ? `?resourceId=${encodeURIComponent(resourceId)}` : '';
  return fetchJSON<PlatformOperation[]>(`/platform/operations${query}`);
}
