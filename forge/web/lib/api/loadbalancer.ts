import { fetchJSON, postJSON, putJSON, deleteJSON, patchJSON } from "./http";

export type TargetStatus = "healthy" | "unhealthy" | "draining";
export type Algorithm = "round_robin" | "least_connections" | "ip_hash" | "weighted_round_robin";

export type Target = {
  id: string;
  serverId: string;
  nodeId: string;
  ip: string;
  port: number;
  weight: number;
  status: TargetStatus;
  connections: number;
};

export type TargetGroup = {
  id: string;
  name: string;
  algorithm: Algorithm;
  port: number;
  protocol: string;
  targets: Target[];
  createdAt: string;
  updatedAt: string;
};

export type LBMetrics = {
  groups: number;
  totalTargets: number;
  healthyTargets: number;
};

export function listTargetGroups() {
  return fetchJSON<{ data: TargetGroup[] }>("/admin/load-balancer/groups").then(r => r.data);
}
export function getTargetGroup(id: string) {
  return fetchJSON<{ data: TargetGroup }>(`/admin/load-balancer/groups/${encodeURIComponent(id)}`).then(r => r.data);
}
export function createTargetGroup(group: Partial<TargetGroup>) {
  return postJSON<{ data: TargetGroup }>("/admin/load-balancer/groups", group);
}
export function updateTargetGroup(id: string, group: Partial<TargetGroup>) {
  return putJSON<{ data: TargetGroup }>(`/admin/load-balancer/groups/${encodeURIComponent(id)}`, group);
}
export function deleteTargetGroup(id: string) {
  return deleteJSON(`/admin/load-balancer/groups/${encodeURIComponent(id)}`);
}
export function addTarget(groupId: string, target: { serverId: string; nodeId?: string; ip: string; port: number; weight: number }) {
  return postJSON<{ data: Target }>(`/admin/load-balancer/groups/${encodeURIComponent(groupId)}/targets`, target);
}
export function removeTarget(groupId: string, targetId: string) {
  return deleteJSON(`/admin/load-balancer/groups/${encodeURIComponent(groupId)}/targets/${encodeURIComponent(targetId)}`);
}
export function setTargetStatus(groupId: string, targetId: string, status: TargetStatus) {
  return patchJSON(`/admin/load-balancer/groups/${encodeURIComponent(groupId)}/targets/${encodeURIComponent(targetId)}`, { status });
}
export function getNextTarget(groupId: string) {
  return fetchJSON<{ data: Target }>(`/admin/load-balancer/groups/${encodeURIComponent(groupId)}/next`).then(r => r.data);
}
export function getLBMetrics() {
  return fetchJSON<{ data: LBMetrics }>("/admin/load-balancer/metrics").then(r => r.data);
}
