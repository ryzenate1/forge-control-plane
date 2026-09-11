import { fetchJSON, postJSON } from './http';

/**
 * Counters over the `reconcile_plans` table (see store.ReconcileSummary).
 *
 * Note what is *not* here: there is no `lastRunAt` and no "was a scan performed"
 * flag, because the endpoint has no such column to report. Every one of these
 * numbers is a count of recorded plans, so an all-zero summary is ambiguous
 * between "the fleet is in sync" and "reconciliation has never run". The UI must
 * resolve that ambiguity from the plan list rather than assuming the calm
 * reading — see `AdminReconciliation`.
 */
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

/**
 * A summary whose counters are all present, or `null`.
 *
 * `/admin/reconcile/summary` reports counts over `reconcile_plans` and carries no
 * `lastRunAt` / `hasScanned` field, so an all-zero summary cannot by itself
 * distinguish "the fleet is in sync" from "reconciliation has never run".
 * Returning `null` for a short or non-numeric payload lets the UI show *unknown*
 * instead of showing zeros it has to claim are clean.
 */
export function readReconcileSummary(raw: unknown): ReconcileSummary | null {
  if (!raw || typeof raw !== "object") return null;
  const keys = ["totalPlans", "pendingPlans", "failedPlans", "totalDrifts", "unresolved"] as const;
  const out: Record<string, number> = {};
  for (const key of keys) {
    const value = (raw as Record<string, unknown>)[key];
    if (typeof value !== "number" || !Number.isFinite(value)) return null;
    out[key] = value;
  }
  return out as unknown as ReconcileSummary;
}

/**
 * The newest timestamp we can actually evidence a reconciliation from, or `null`
 * when nothing has been recorded.
 *
 * The summary endpoint has no run marker, so the plans (and their execution
 * events) are the only truthful signal that a scan has ever produced a record.
 * `null` here means *never*, and the UI must say so rather than render an
 * all-zero board as a healthy fleet.
 */
export function latestReconcileActivityAt(
  plans: Pick<ReconcilePlanRow, "createdAt">[],
  events: Pick<ReconcileEventRow, "createdAt">[] = [],
): string | null {
  let newest: number | null = null;
  for (const row of [...plans, ...events]) {
    const at = Date.parse(row.createdAt);
    if (Number.isFinite(at) && (newest === null || at > newest)) newest = at;
  }
  return newest === null ? null : new Date(newest).toISOString();
}

export async function fetchReconcileSummary(): Promise<ReconcileSummary> {
  const res = await fetchJSON<{ data: ReconcileSummary }>("/admin/reconcile/summary");
  const summary = readReconcileSummary(res?.data);
  if (!summary) {
    throw new Error("Reconciliation summary did not report its counters — drift state is unknown.");
  }
  return summary;
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
