import { fetchJSON, postJSON } from "./http";

// Fencing admin client — exposes manual node-fence operations backed by
// services/fencing (which bumps workload generations on EventNodeRecovered).
// See forge/api/internal/http/handlers_fencing.go.

export type FencePreviewRow = {
  id: string;
  name: string;
  status: string;
  generation: number;
};

export type FenceResult = {
  nodeId: string;
  fenced: boolean;
  serverCount: number;
  serverIds?: string[];
  reason?: string;
};

export async function previewFence(nodeId: string): Promise<FencePreviewRow[]> {
  const res = await fetchJSON<{ data: FencePreviewRow[] }>(`/fencing/nodes/${encodeURIComponent(nodeId)}/preview`);
  return res.data ?? [];
}

export async function fenceNode(nodeId: string): Promise<FenceResult> {
  const res = await postJSON<{ data: FenceResult }>(`/fencing/nodes/${encodeURIComponent(nodeId)}`, {});
  return res.data;
}
