import { fetchJSON, postJSON } from './http';

export type ReconcileSummary = {
  totalPlans: number;
  pendingPlans: number;
  failedPlans: number;
  totalDrifts: number;
  unresolved: number;
};

export type ReconcileDiffType = "create" | "update" | "delete" | "noop";

export type ReconcileDiff = {
  resourceId: string;
  resourceKind: string;
  diffType: ReconcileDiffType;
  desiredHash: string;
  observedHash: string;
  description: string;
  details?: Record<string, unknown>;
};

export type DriftKind = "config_drift" | "missing_resource" | "orphaned_resource" | "state_mismatch";

export type DriftRecord = {
  resourceId: string;
  resourceKind: string;
  driftKind: DriftKind;
  desired: string;
  observed: string;
  severity: string;
  detectedAt: string;
  details?: Record<string, unknown>;
};

export type ReconcilePlanRow = {
  id: string;
  resourceId: string;
  resourceKind: string;
  state: string;
  destructive: boolean;
  confirmed: boolean;
  diffCount: number;
  driftCount: number;
  diffs: ReconcileDiff[];
  drifts: DriftRecord[];
  error?: string;
  createdAt: string;
  executedAt?: string | null;
};

export type ReconcileEventRow = {
  id: string;
  planId: string;
  resourceId: string;
  resourceKind: string;
  eventType: string;
  summary: string;
  createdAt: string;
};

export type ReconcileResult = {
  planId: string;
  resourceId: string;
  resourceKind: string;
  state: string;
  operationCount: number;
  error?: string;
  startedAt: string;
  completedAt?: string | null;
};

export async function fetchReconcileSummary(): Promise<ReconcileSummary> {
  const res = await fetchJSON<{ data: ReconcileSummary }>("/admin/reconcile/summary");
  return res.data;
}

export async function fetchReconcilePlans(offset = 0, limit = 50): Promise<{ data: ReconcilePlanRow[]; total: number }> {
  return fetchJSON<{ data: ReconcilePlanRow[]; total: number }>(`/admin/reconcile/plans?offset=${offset}&limit=${limit}`);
}

export async function fetchReconcilePlan(id: string): Promise<ReconcilePlanRow> {
  const res = await fetchJSON<{ data: ReconcilePlanRow }>(`/admin/reconcile/plans/${encodeURIComponent(id)}`);
  return res.data;
}

export async function confirmReconcilePlan(id: string): Promise<ReconcileResult> {
  const res = await postJSON<{ data: ReconcileResult }>(`/admin/reconcile/plans/${encodeURIComponent(id)}/confirm`);
  return res.data;
}

export async function executeReconcilePlan(id: string): Promise<ReconcileResult> {
  const res = await postJSON<{ data: ReconcileResult }>(`/admin/reconcile/plans/${encodeURIComponent(id)}/execute`);
  return res.data;
}

export async function triggerReconcileAll(kind = "all"): Promise<ReconcileResult[]> {
  const res = await postJSON<{ data: ReconcileResult[] }>("/admin/reconcile/trigger-all", { kind });
  return res.data;
}

export async function fetchReconcileEvents(resourceId?: string, limit = 50): Promise<ReconcileEventRow[]> {
  const res = await postJSON<{ data: ReconcileEventRow[] }>("/admin/reconcile/events", { resourceId: resourceId ?? "", limit });
  return res.data;
}
