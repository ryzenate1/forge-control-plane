import { fetchJSON } from "./http";

export type OperationsTimelineItem = {
  id: string;
  kind: "job" | "operation" | "drain" | "transfer" | "orphan" | string;
  type: string;
  status: string;
  resourceType?: string;
  resourceId?: string;
  serverId?: string;
  nodeId?: string;
  generation: number;
  fenceGeneration?: number;
  isFenced: boolean;
  desiredState?: string;
  actualState?: string;
  progress?: number | null;
  error?: string;
  createdAt: string;
  updatedAt: string;
  correlationId?: string;
};

export type OperationsTimelineResponse = {
  data: OperationsTimelineItem[];
  meta: { total: number; limit: number };
};

export async function fetchOperationsTimeline(params?: {
  kind?: string;
  fenced?: boolean;
  limit?: number;
  signal?: AbortSignal;
}): Promise<OperationsTimelineResponse> {
  const q = new URLSearchParams();
  if (params?.kind) q.set("kind", params.kind);
  if (params?.fenced) q.set("fenced", "true");
  if (params?.limit) q.set("limit", String(params.limit));
  const qs = q.toString();
  return fetchJSON<OperationsTimelineResponse>(`/admin/operations/timeline${qs ? `?${qs}` : ""}`, {
    signal: params?.signal,
  });
}

export type UnifiedTransferStatus = {
  source: "migration" | "server";
  transferring: boolean;
  status?: string;
  phase?: string;
  progress?: number | null;
  migrationId?: string;
  migrationStatus?: string;
  migrationPhase?: string;
  targetNodeId?: string;
  state?: string;
  error?: string | null;
  generation?: number;
};

export async function fetchUnifiedTransferStatus(
  serverId: string,
  init?: { signal?: AbortSignal },
): Promise<UnifiedTransferStatus | null> {
  // Primary: unified migration-aware transfer (admin timeline helper) — if allowed, else fallback to legacy.
  try {
    const res = await fetchJSON<UnifiedTransferStatus>(`/admin/operations/transfer/${encodeURIComponent(serverId)}`, init);
    return res;
  } catch {
    // fallback to legacy per-server transfer (works for non-admin with server permission)
    try {
      const legacy = await fetchJSON<{ state?: string; transferring: boolean; targetNodeId?: string; error?: string | null; progress?: number }>(
        `/servers/${encodeURIComponent(serverId)}/transfer`,
        init,
      );
      return {
        source: "server",
        transferring: legacy.transferring,
        status: legacy.state,
        state: legacy.state,
        progress: legacy.progress ?? null,
        targetNodeId: legacy.targetNodeId,
        error: legacy.error,
      };
    } catch {
      return null;
    }
  }
}
