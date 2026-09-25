import { postJSON } from "./http";

export type PlacementRequest = {
  serverId?: string;
  memory?: number;
  cpu?: number;
  disk?: number;
  constraints?: Array<{ type: string; key: string; operator: string; values: string[]; required?: boolean }>;
};

export type EnrichedPlacement = {
  request: PlacementRequest;
  envGroup?: string;
  constraints: Array<{ type: string; key: string; operator: string; values: string[]; required: boolean }>;
};

export type PlacementConstraint = {
  type: string;
  key?: string;
  operator: string;
  values?: string[];
  required?: boolean;
};

export type ExplainRanking = {
  nodeId: string;
  score: number;
  env?: string;
  reasons: string[];
};

// ExplainResult mirrors the Go envaffinity.ExplainResult returned by
// POST /placement/explain (see forge/api/internal/services/envaffinity/explain.go).
export type ExplainResult = {
  id?: string;
  nodeId: string;
  requestedEnv?: string;
  nodeEnvGroups: string[];
  nodeLabels: Record<string, string>;
  constraints: PlacementConstraint[];
  matchedLabels: string[];
  missingLabels: string[];
  isCandidate: boolean;
  ranking?: ExplainRanking[];
};

// PatchResult mirrors envaffinity.PatchResult from POST /placement/patch-constraints.
export type PatchResult = {
  serversPinned: number;
  envsMapped: string[];
  rulesRegistered: number;
};

function unwrap<T>(data: unknown): T {
  if (data && typeof data === "object" && "data" in (data as Record<string, unknown>)) {
    return (data as { data: T }).data;
  }
  return data as T;
}

export async function explainPlacement(nodeId: string, place: PlacementRequest): Promise<ExplainResult> {
  const res = await postJSON<ExplainResult | { data: ExplainResult }>("/placement/explain", { nodeId, place });
  return unwrap<ExplainResult>(res);
}

export async function enrichPlacement(place: PlacementRequest): Promise<EnrichedPlacement> {
  const res = await postJSON<EnrichedPlacement | { data: EnrichedPlacement }>("/placement/enrich", place);
  return unwrap<EnrichedPlacement>(res);
}

export async function patchConstraints(): Promise<PatchResult> {
  const res = await postJSON<PatchResult | { data: PatchResult }>("/placement/patch-constraints", {});
  return unwrap<PatchResult>(res);
}
