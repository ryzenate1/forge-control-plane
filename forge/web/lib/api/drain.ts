import { fetchJSON, postJSON } from "./http";

// Drain ledger types — mirror Go store.DrainState (migration 191)
export type DrainProgressStep = {
  name: string;
  state: string; // pending | active | done
  detail?: string;
};

export type DrainProgress = {
  state: string;
  total: number;
  remaining: number;
  current: string;
  steps: DrainProgressStep[];
};

export type DrainState = {
  nodeId: string;
  planId?: string;
  status: string; // draining | drained | cancelled | failed
  desiredFinal: boolean;
  startedAt: string;
  completedAt?: string | null;
  progress: DrainProgress;
  updatedAt: string;
};

export async function fetchDrainStates(): Promise<DrainState[]> {
  // Durable ledger: every node that has ever been drained, newest first.
  const res = await fetchJSON<{ data: DrainState[] }>("/nodes/drain");
  return res.data ?? [];
}

export async function fetchDrainState(nodeId: string): Promise<DrainState | null> {
  // Ledger per-node progress. Separate from clustermembership's lightweight
  // GET /nodes/:id/drain status, so the durable step-by-step record survives
  // restarts. Returns { data: null } when no drain has ever been recorded.
  const res = await fetchJSON<{ data: DrainState | null }>(`/nodes/${encodeURIComponent(nodeId)}/drain/progress`);
  return (res as unknown as { data: DrainState | null }).data ?? null;
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

// Evacuation planner preview — lightweight re-export for the center
export type EvacuationPlanPreview = {
  plan: { id: string; nodeId: string; status: string; items: unknown[] };
  items: unknown[];
  preview: boolean;
};

export async function fetchEvacuationOrphanCandidates(): Promise<
  { serverId: string; nodeId: string; status: string; storageLocality: string; replacementPolicy: string }[]
> {
  // This is derived from the planner's DetectOrphans side but surfaced via evacuation + nodes.
  // We approximate by listing nodes that are offline/unreachable and their servers — the
  // center's forensic view will join this with heartbeat lanes. No dedicated endpoint exists
  // today; return empty and let the UI derive orphans from heartbeat+server listing.
  return [];
}
