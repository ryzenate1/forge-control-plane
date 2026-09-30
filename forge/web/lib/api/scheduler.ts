import { fetchJSON, postJSON, putJSON, deleteJSON } from "./http";

export type PredictiveScore = {
  nodeId: string;
  baseScore: number;
  trendScore: number;
  affinityScore: number;
  antiAffinityScore: number;
  totalScore: number;
  predictedLoad: number;
  confidence: number;
};

export type AffinityRule = {
  id: string;
  name: string;
  serverId?: string;
  nodeId?: string;
  label: string;
  weight: number;
};

export type AntiAffinityRule = {
  id: string;
  name: string;
  serverId?: string;
  label: string;
  scope: string;
  weight: number;
};

export type Constraint = {
  type: "required" | "preferred" | "forbidden";
  key: string;
  operator: string;
  value: string;
};

export type BackendInfo = { type: string; name: string; description: string };

export type ResourceMetric = {
  timestamp?: string;
  cpuPercent: number;
  memoryUsedMb: number;
  diskUsedMb: number;
  networkRx?: number;
  networkTx?: number;
  serverCount?: number;
};

// Predictive
export function listPredictiveScores() {
  return fetchJSON<{ data: PredictiveScore[] }>("/admin/scheduler/predictive/scores").then(r => r.data);
}
export function getPredictiveScore(nodeId: string) {
  return fetchJSON<{ data: PredictiveScore }>(`/admin/scheduler/predictive/nodes/${encodeURIComponent(nodeId)}/score`).then(r => r.data);
}
export function ingestPredictiveMetrics(nodeId: string, metric: ResourceMetric) {
  return postJSON(`/admin/scheduler/predictive/metrics/${encodeURIComponent(nodeId)}`, metric);
}
export function listAffinityRules() {
  return fetchJSON<{ data: AffinityRule[] }>("/admin/scheduler/predictive/affinity-rules").then(r => r.data);
}
export function createAffinityRule(rule: Omit<AffinityRule, "id">) {
  return postJSON<{ data: AffinityRule }>("/admin/scheduler/predictive/affinity-rules", rule);
}
export function deleteAffinityRule(id: string) {
  return deleteJSON(`/admin/scheduler/predictive/affinity-rules/${encodeURIComponent(id)}`);
}
export function listAntiAffinityRules() {
  return fetchJSON<{ data: AntiAffinityRule[] }>("/admin/scheduler/predictive/anti-affinity-rules").then(r => r.data);
}
export function createAntiAffinityRule(rule: Omit<AntiAffinityRule, "id">) {
  return postJSON<{ data: AntiAffinityRule }>("/admin/scheduler/predictive/anti-affinity-rules", rule);
}
export function deleteAntiAffinityRule(id: string) {
  return deleteJSON(`/admin/scheduler/predictive/anti-affinity-rules/${encodeURIComponent(id)}`);
}

// Constraints - backend expects PUT array
export function listConstraints() {
  return fetchJSON<{ data: Constraint[] }>("/admin/scheduler/constraints").then(r => r.data);
}
export function putConstraints(constraints: Constraint[]) {
  return putJSON<{ data: Constraint[] }>("/admin/scheduler/constraints", constraints);
}
export function deleteConstraintByIndexOrId(id: string) {
  return deleteJSON(`/admin/scheduler/constraints/${encodeURIComponent(id)}`);
}

// Backends static
export function listSchedulerBackends() {
  return fetchJSON<{ data: BackendInfo[] }>("/admin/scheduler/backends").then(r => r.data);
}

// Node scheduler config
export function updateNodeScheduler(nodeId: string, schedulerType: string, schedulerConfig?: unknown) {
  return putJSON(`/admin/scheduler/nodes/${encodeURIComponent(nodeId)}/scheduler`, {
    schedulerType,
    schedulerConfig,
  });
}
