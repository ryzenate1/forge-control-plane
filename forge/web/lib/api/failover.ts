import { fetchJSON, postJSON, putJSON, deleteJSON } from "./http";

export type FailoverPolicy = {
  id: string;
  name: string;
  nodeId: string;
  maxFailures: number;
  failureWindowSec: number;
  cooldownSec: number;
  action: "evacuate" | "restart" | "notify";
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
};

export type FailoverEvent = {
  id: string;
  policyId: string;
  nodeId: string;
  serverId?: string;
  eventType: string;
  action: string;
  status: string;
  message: string;
  timestamp: string;
};

export type FailoverMetrics = {
  failuresDetected: number;
  evacuationsTriggered: number;
  restartsTriggered: number;
  notificationsSent: number;
};

export function listFailoverPolicies() {
  return fetchJSON<{ data: FailoverPolicy[] }>("/admin/failover/policies").then(r => r.data);
}
export function getFailoverPolicy(id: string) {
  return fetchJSON<{ data: FailoverPolicy }>(`/admin/failover/policies/${encodeURIComponent(id)}`).then(r => r.data);
}
export function listFailoverPoliciesByNode(nodeId: string) {
  return fetchJSON<{ data: FailoverPolicy[] }>(`/admin/failover/policies/node/${encodeURIComponent(nodeId)}`).then(r => r.data);
}
export function createFailoverPolicy(policy: Partial<FailoverPolicy>) {
  return postJSON<{ data: FailoverPolicy }>("/admin/failover/policies", policy);
}
export function updateFailoverPolicy(id: string, policy: Partial<FailoverPolicy>) {
  return putJSON<{ data: FailoverPolicy }>(`/admin/failover/policies/${encodeURIComponent(id)}`, policy);
}
export function deleteFailoverPolicy(id: string) {
  return deleteJSON(`/admin/failover/policies/${encodeURIComponent(id)}`);
}
export function recordFailoverFailure(nodeId: string) {
  return postJSON<{ data: FailoverEvent }>(`/admin/failover/record-failure/${encodeURIComponent(nodeId)}`);
}
export function handleServerCrash(serverId: string, nodeId: string) {
  return postJSON<{ data: FailoverEvent }>(`/admin/failover/crash/${encodeURIComponent(serverId)}/${encodeURIComponent(nodeId)}`);
}
export function getFailoverMetrics() {
  return fetchJSON<{ data: FailoverMetrics }>("/admin/failover/metrics").then(r => r.data);
}
