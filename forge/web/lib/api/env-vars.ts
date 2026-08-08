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

export function fetchEnvVars(envId: string): Promise<EnvVarResponse[]>;
export function fetchEnvVars(scopeType: 'project' | 'environment', scopeId: string): Promise<EnvVarResponse[]>;
export function fetchEnvVars(scopeTypeOrEnvId: string, scopeId?: string): Promise<EnvVarResponse[]> {
  const scopeType = scopeId ? (scopeTypeOrEnvId as 'project' | 'environment') : 'environment';
  const id = scopeId ?? scopeTypeOrEnvId;
  const path = scopeType === 'project'
    ? `/projects/${encodeURIComponent(id)}/env-vars`
    : `/environments/${encodeURIComponent(id)}/env-vars`;
  return fetchJSON<EnvVarResponse[]>(path);
}

export function createEnvVar(envId: string, key: string, value: string, isSensitive?: boolean): Promise<EnvVarResponse>;
export function createEnvVar(scopeType: 'project' | 'environment', scopeId: string, input: CreateEnvVarInput): Promise<EnvVarResponse>;
export function createEnvVar(
  scopeTypeOrEnvId: string,
  scopeIdOrKey: string | CreateEnvVarInput,
  inputOrValue?: CreateEnvVarInput | string,
  isSensitive?: boolean,
): Promise<EnvVarResponse> {
  if (typeof scopeIdOrKey === 'object') {
    const scopeType = scopeTypeOrEnvId as 'project' | 'environment';
    const path = scopeType === 'project'
      ? `/projects/${encodeURIComponent(scopeTypeOrEnvId)}/env-vars`
      : `/environments/${encodeURIComponent(scopeTypeOrEnvId)}/env-vars`;
    return postJSON<EnvVarResponse>(path, scopeIdOrKey);
  }
  if (typeof inputOrValue === 'object') {
    const scopeType = scopeTypeOrEnvId as 'project' | 'environment';
    const path = scopeType === 'project'
      ? `/projects/${encodeURIComponent(scopeIdOrKey)}/env-vars`
      : `/environments/${encodeURIComponent(scopeIdOrKey)}/env-vars`;
    return postJSON<EnvVarResponse>(path, inputOrValue);
  }
  const envId = scopeTypeOrEnvId;
  const key = scopeIdOrKey;
  const value = inputOrValue as string;
  const path = `/environments/${encodeURIComponent(envId)}/env-vars`;
  return postJSON<EnvVarResponse>(path, { key, value, isSensitive });
}

export function deleteEnvVar(varId: string): Promise<void> {
  return deleteJSON(`/env-vars/${encodeURIComponent(varId)}`);
}
