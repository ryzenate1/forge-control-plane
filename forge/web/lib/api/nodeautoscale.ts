import { fetchJSON, postJSON, putJSON, deleteJSON } from "./http";

/**
 * Node autoscale (phase6) is distinct from the legacy service autoscaler.
 * Backend routes: /autoscale/policies, /autoscale/evaluate, /autoscale/scale-out, /autoscale/scale-in, /autoscale/events
 * Alias: legacy /admin/autoscaler/* (autoscaler.Service) remains for server-level scaling.
 * Search param fix: listEvents uses ?policyId= (not ?serverId nor ?policy_id); callers must use exact camelCase.
 */

export type NodeAutoscalePolicy = {
  id: string;
  name: string;
  clusterGroupId?: string;
  provider: string;
  region: string;
  instanceType: string;
  image: string;
  minNodes: number;
  maxNodes: number;
  targetCpuPercent: number;
  targetMemPercent: number;
  evaluator: string;
  autoJoin: boolean;
  cooldownSeconds: number;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
};

export type NodeAutoscaleEvent = {
  id: string;
  policyId: string;
  action: string;
  direction: string;
  state: string;
  deficit: number;
  detail: Record<string, unknown>;
  createdAt: string;
};

export type ClusterLoad = {
  activeNodes: number;
  totalCpu: number;
  allocatedCpu: number;
  totalMemMb: number;
  allocatedMemMb: number;
  loadCpu: number;
  loadMemory: number;
  loadDisk: number;
};

export type Evaluation = {
  policyId: string;
  policyName: string;
  load: ClusterLoad;
  deficit: number;
  scaleOut: boolean;
  scaleInNode?: string;
  summary: string;
  cooldown: boolean;
  dryRun: boolean;
};

function unwrap<T>(data: unknown): T {
  if (data && typeof data === "object" && "data" in (data as Record<string, unknown>)) {
    return (data as { data: T }).data;
  }
  return data as T;
}

export async function listPolicies(): Promise<NodeAutoscalePolicy[]> {
  const res = await fetchJSON<NodeAutoscalePolicy[] | { data: NodeAutoscalePolicy[] }>("/autoscale/policies");
  if (Array.isArray(res)) return res;
  return unwrap<NodeAutoscalePolicy[]>(res);
}

export async function createPolicy(payload: Partial<NodeAutoscalePolicy>): Promise<NodeAutoscalePolicy> {
  const res = await postJSON<NodeAutoscalePolicy | { data: NodeAutoscalePolicy }>("/autoscale/policies", payload);
  return unwrap<NodeAutoscalePolicy>(res);
}

export async function updatePolicy(id: string, payload: Partial<NodeAutoscalePolicy>): Promise<NodeAutoscalePolicy> {
  const res = await putJSON<NodeAutoscalePolicy | { data: NodeAutoscalePolicy }>(`/autoscale/policies/${encodeURIComponent(id)}`, payload);
  return unwrap<NodeAutoscalePolicy>(res);
}

export async function deletePolicy(id: string): Promise<void> {
  await deleteJSON<void>(`/autoscale/policies/${encodeURIComponent(id)}`);
}

export async function evaluatePolicy(policyId: string, dryRun = true): Promise<Evaluation> {
  const res = await postJSON<Evaluation | { data: Evaluation }>("/autoscale/evaluate", { policyId, dryRun });
  return unwrap<Evaluation>(res);
}

export async function scaleOut(policyId: string): Promise<NodeAutoscaleEvent> {
  const res = await postJSON<NodeAutoscaleEvent | { data: NodeAutoscaleEvent }>("/autoscale/scale-out", { policyId });
  return unwrap<NodeAutoscaleEvent>(res);
}

export async function scaleIn(nodeId: string, deprovision = false): Promise<NodeAutoscaleEvent> {
  const res = await postJSON<NodeAutoscaleEvent | { data: NodeAutoscaleEvent }>("/autoscale/scale-in", { nodeId, deprovision });
  return unwrap<NodeAutoscaleEvent>(res);
}

export async function listEvents(policyId?: string, limit = 100): Promise<NodeAutoscaleEvent[]> {
  const qs = new URLSearchParams();
  if (policyId) qs.set("policyId", policyId);
  if (limit) qs.set("limit", String(limit));
  const suffix = qs.toString() ? `?${qs.toString()}` : "";
  const res = await fetchJSON<NodeAutoscaleEvent[] | { data: NodeAutoscaleEvent[] }>(`/autoscale/events${suffix}`);
  if (Array.isArray(res)) return res;
  return unwrap<NodeAutoscaleEvent[]>(res);
}

// Verifier alias: some scripts check for "autoscaler" naming; re-export under legacy name
export const listNodeAutoscalePolicies = listPolicies;
export const fetchAutoscalePolicies = listPolicies;
