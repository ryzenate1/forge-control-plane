import { fetchJSON, postJSON, deleteJSON } from './http';

export interface EnvVarResponse {
  id: string;
  key: string;
  value?: string;
  isSensitive: boolean;
  version: number;
  scope: string;
}

export interface CreateEnvVarInput {
  key: string;
  value: string;
  isSensitive?: boolean;
}

export function fetchEnvVars(scopeType: 'project' | 'environment', scopeId: string): Promise<EnvVarResponse[]> {
  const path = scopeType === 'project'
    ? `/projects/${encodeURIComponent(scopeId)}/env-vars`
    : `/environments/${encodeURIComponent(scopeId)}/env-vars`;
  return fetchJSON<EnvVarResponse[]>(path);
}

export function createEnvVar(scopeType: 'project' | 'environment', scopeId: string, input: CreateEnvVarInput): Promise<EnvVarResponse> {
  const path = scopeType === 'project'
    ? `/projects/${encodeURIComponent(scopeId)}/env-vars`
    : `/environments/${encodeURIComponent(scopeId)}/env-vars`;
  return postJSON<EnvVarResponse>(path, input);
}

export function deleteEnvVar(varId: string): Promise<void> {
  return deleteJSON(`/env-vars/${encodeURIComponent(varId)}`);
}
