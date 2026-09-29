import { fetchJSON, postJSON, unwrapList, unwrapNullableData } from "./http";

// Drain ledger types — mirror Go store.DrainState (migration 191)
export type DrainProgressStep = {
  name: string;
  state: string; // pending | active | done — read by the UI as the authoritative
                 // per-step verdict; an unreported state is unknown, never "done".
  detail?: string;
};

export type DrainProgress = {
  state: string;
  total?: number;
  remaining?: number;
  current?: string;
  steps?: DrainProgressStep[];
};

export type DrainState = {
  nodeId: string;
  planId?: string;
  status: string; // draining | drained | cancelled | failed
  desiredFinal: boolean;
  startedAt: string;
  completedAt?: string | null;
  /**
   * Optional on purpose. This used to be declared required while the UI wrote
   * `s.progress?.remaining ?? 0`, and the `?? 0` turned an absent progress block
   * into "0 remaining of 0 workloads" — a reading that looks finished. Callers now
   * test each number individually and render "not reported" when it is missing.
   */
  progress?: DrainProgress;
  updatedAt: string;
};

export async function fetchDrainStates(): Promise<DrainState[]> {
  // Durable ledger: every node that has ever been drained, newest first.
  const res = await fetchJSON<{ data: DrainState[] } | DrainState[]>("/drain-ledger");
  return unwrapList(res);
}

export async function fetchDrainState(nodeId: string): Promise<DrainState | null> {
  // Ledger per-node progress (distinct from clustermembership's GET
  // /nodes/:id/drain status). Returns { data: null } when nothing recorded.
  const res = await fetchJSON<{ data: DrainState | null } | DrainState | null>(`/drain-ledger/${encodeURIComponent(nodeId)}`);
  return unwrapNullableData(res);
}

export async function beginDrain(nodeId: string): Promise<{ status: string }> {
  // Orchestration lives in clustermembership: it sets the node draining, withdraws
  // gateway targets and runs the evacuation plan. The durable ledger records
  // progress asynchronously from the emitted events — poll fetchDrainState().
  return postJSON<{ status: string }>(`/nodes/${encodeURIComponent(nodeId)}/drain`);
}

export async function cancelDrain(nodeId: string): Promise<{ status: string }> {
  return postJSON<{ status: string }>(`/nodes/${encodeURIComponent(nodeId)}/drain/cancel`);
}

/*
 * Deleted: `fetchEvacuationOrphanCandidates()`, which returned a hardcoded `[]`
 * behind a comment admitting "no dedicated endpoint exists today".
 *
 * A function shaped like a discovery call that always answers "no orphans" is the
 * exact defect this slice is being fixed for — an unrun check rendered as a clean
 * result — and it had no callers. Orphan discovery does not exist server-side; the
 * honest surface for what *is* known is the failed-deletion queue at
 * `/admin/orphans`, which reports deletions the daemon said it could not complete.
 */
