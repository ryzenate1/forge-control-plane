import { fetchJSON, postJSON, unwrapList } from './http';

export type InstallStatus = 'pending' | 'running' | 'completed' | 'failed';
export type WorkflowType = 'install' | 'uninstall' | 'reinstall';

export interface InstallStep {
  id: string;
  workflowId?: string;
  sequence: number;
  name: string;
  action: string;
  status: InstallStatus;
  startedAt?: string;
  completedAt?: string;
  error?: string;
}

export interface InstallWorkflow {
  id: string;
  serverId: string;
  type: WorkflowType;
  status: InstallStatus;
  steps: InstallStep[];
  metadata?: unknown;
  createdAt: string;
  completedAt?: string;
}

export async function listInstallWorkflows(serverId: string): Promise<InstallWorkflow[]> {
  const res = await fetchJSON<{ data: InstallWorkflow[] } | InstallWorkflow[]>(`/servers/${encodeURIComponent(serverId)}/install-workflows`);
  return unwrapList(res);
}

export async function listRecentInstallWorkflows(limit = 20): Promise<{ data: InstallWorkflow[]; executionEnabled: boolean }> {
  const res = await fetchJSON<{ data: InstallWorkflow[]; meta?: { executionEnabled?: boolean } }>(`/admin/install-workflows?limit=${limit}`);
  const executionEnabled = Boolean((res as { meta?: { executionEnabled?: boolean } }).meta?.executionEnabled);
  const data = Array.isArray(res.data) ? res.data : [];
  return { data, executionEnabled };
}

export async function getInstallWorkflow(id: string): Promise<InstallWorkflow> {
  return fetchJSON<InstallWorkflow>(`/install-workflows/${encodeURIComponent(id)}`);
}

export async function createInstallWorkflow(serverId: string, type: WorkflowType = 'install'): Promise<InstallWorkflow> {
  return postJSON<InstallWorkflow>(`/servers/${encodeURIComponent(serverId)}/install-workflows`, { type });
}

export async function executeInstallWorkflow(id: string): Promise<{ accepted: boolean; workflowId: string }> {
  return postJSON<{ accepted: boolean; workflowId: string }>(`/install-workflows/${encodeURIComponent(id)}/execute`);
}
