import { fetchJSON, postJSON, putJSON, deleteJSON } from "./http";

export type ProcedureStep = {
  id: string;
  procedureId: string;
  position: number;
  name: string;
  action: string;
  config: Record<string, unknown>;
  maxRetries: number;
  timeoutSeconds: number;
  requiresApproval: boolean;
  continueOnFailure: boolean;
  rollbackEnabled: boolean;
  createdAt: string;
};

export type ProcedureSchedule = {
  id: string;
  procedureId: string;
  cronExpression: string;
  timezone: string;
  enabled: boolean;
  lastRunAt?: string | null;
  nextRunAt?: string | null;
  createdAt: string;
  updatedAt: string;
};

export type Procedure = {
  id: string;
  name: string;
  description: string;
  tenantId?: string | null;
  enabled: boolean;
  steps: ProcedureStep[];
  schedule?: ProcedureSchedule | null;
  createdAt: string;
  updatedAt: string;
};

export type ProcedureStepExecution = {
  id: string;
  executionId: string;
  stepId: string;
  position: number;
  status: string;
  attempt: number;
  maxAttempts: number;
  output: string;
  error?: string;
  startedAt?: string | null;
  completedAt?: string | null;
  operationId?: string | null;
};

export type ProcedureExecution = {
  id: string;
  procedureId: string;
  status: string;
  trigger: string;
  tenantId?: string | null;
  actorId?: string | null;
  startedAt?: string | null;
  completedAt?: string | null;
  createdAt: string;
  updatedAt: string;
  steps: ProcedureStepExecution[];
};

export type ProcedureStepLog = {
  id: string;
  stepExecutionId: string;
  level: string;
  message: string;
  createdAt: string;
};

export type CreateProcedureRequest = {
  name: string;
  description: string;
  tenantId?: string | null;
  enabled: boolean;
  steps: {
    position: number;
    name: string;
    action: string;
    config: Record<string, unknown>;
    maxRetries: number;
    timeoutSeconds: number;
    requiresApproval: boolean;
    continueOnFailure: boolean;
    rollbackEnabled: boolean;
  }[];
  schedule?: { cronExpression: string; timezone: string; enabled: boolean } | null;
};

function unwrap<T>(data: unknown): T {
  if (data && typeof data === "object" && "data" in (data as Record<string, unknown>)) {
    return (data as { data: T }).data;
  }
  return data as T;
}

export async function listProcedures(tenantId?: string): Promise<Procedure[]> {
  const qs = tenantId ? `?tenantId=${encodeURIComponent(tenantId)}` : "";
  const res = await fetchJSON<Procedure[] | { data: Procedure[] }>(`/procedures/${qs}`);
  if (Array.isArray(res)) return res;
  return unwrap<Procedure[]>(res);
}

export async function getProcedure(id: string): Promise<Procedure> {
  const res = await fetchJSON<Procedure | { data: Procedure }>(`/procedures/${encodeURIComponent(id)}`);
  return unwrap<Procedure>(res);
}

export async function createProcedure(payload: CreateProcedureRequest): Promise<Procedure> {
  const res = await postJSON<Procedure | { data: Procedure }>("/procedures/", payload);
  return unwrap<Procedure>(res);
}

export async function updateProcedure(id: string, payload: CreateProcedureRequest): Promise<Procedure> {
  const res = await putJSON<Procedure | { data: Procedure }>(`/procedures/${encodeURIComponent(id)}`, payload);
  return unwrap<Procedure>(res);
}

export async function deleteProcedure(id: string): Promise<void> {
  await deleteJSON<void>(`/procedures/${encodeURIComponent(id)}`);
}

export async function executeProcedure(id: string): Promise<ProcedureExecution> {
  const res = await postJSON<ProcedureExecution | { data: ProcedureExecution }>(`/procedures/${encodeURIComponent(id)}/execute`, {});
  return unwrap<ProcedureExecution>(res);
}

export async function getExecution(id: string): Promise<ProcedureExecution> {
  const res = await fetchJSON<ProcedureExecution | { data: ProcedureExecution }>(`/procedures/executions/${encodeURIComponent(id)}`);
  return unwrap<ProcedureExecution>(res);
}

export async function listExecutions(procedureId: string, limit = 20): Promise<ProcedureExecution[]> {
  const res = await fetchJSON<ProcedureExecution[] | { data: ProcedureExecution[] }>(
    `/procedures/${encodeURIComponent(procedureId)}/executions?limit=${limit}`,
  );
  if (Array.isArray(res)) return res;
  return unwrap<ProcedureExecution[]>(res);
}

export async function cancelExecution(id: string): Promise<void> {
  await postJSON<void>(`/procedures/executions/${encodeURIComponent(id)}/cancel`, {});
}

export async function approveStep(stepId: string): Promise<void> {
  await postJSON<void>(`/procedures/executions/steps/${encodeURIComponent(stepId)}/approve`, {});
}

export async function rejectStep(stepId: string): Promise<void> {
  await postJSON<void>(`/procedures/executions/steps/${encodeURIComponent(stepId)}/reject`, {});
}

export async function listStepLogs(stepId: string): Promise<ProcedureStepLog[]> {
  const res = await fetchJSON<ProcedureStepLog[] | { data: ProcedureStepLog[] }>(
    `/procedures/executions/steps/${encodeURIComponent(stepId)}/logs`,
  );
  if (Array.isArray(res)) return res;
  return unwrap<ProcedureStepLog[]>(res);
}
