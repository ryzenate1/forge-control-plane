import { fetchJSON, postJSON, putJSON, deleteJSON } from "./http";

export type RoutingRule = {
  id: string;
  name?: string;
  serverId?: string;
  domain: string;
  path: string;
  targetHost?: string;
  targetPort: number;
  protocol?: string;
  strategy?: string;
  weight?: number;
  headers?: Record<string, string>;
  enabled: boolean;
  webSocketSupport?: boolean;
  createdAt?: string;
};

export type TrafficPolicy = {
  id: string;
  name: string;
  rateLimit?: number;
  rateLimitBurst?: number;
  ipWhitelist?: string[];
  ipBlacklist?: string[];
  tlsEnabled?: boolean;
  tlsCertFile?: string;
  tlsKeyFile?: string;
  circuitBreaker?: boolean;
  circuitBreakerThreshold?: number;
  circuitBreakerTimeout?: number;
  // legacy UI
  type?: string;
  config?: Record<string, unknown>;
  enabled?: boolean;
  createdAt?: string;
};

export function listRoutingRules() {
  return fetchJSON<{ data: RoutingRule[] }>("/admin/traffic/rules").then(r => r.data);
}
export function getRoutingRule(id: string) {
  return fetchJSON<{ data: RoutingRule }>(`/admin/traffic/rules/${encodeURIComponent(id)}`).then(r => r.data);
}
export function listRoutingRulesByServer(serverId: string) {
  return fetchJSON<{ data: RoutingRule[] }>(`/admin/traffic/rules/server/${encodeURIComponent(serverId)}`).then(r => r.data);
}
export function createRoutingRule(rule: Partial<RoutingRule>) {
  return postJSON<{ data: RoutingRule }>("/admin/traffic/rules", rule);
}
export function updateRoutingRule(id: string, rule: Partial<RoutingRule>) {
  return putJSON<{ data: RoutingRule }>(`/admin/traffic/rules/${encodeURIComponent(id)}`, rule);
}
export function deleteRoutingRule(id: string) {
  return deleteJSON(`/admin/traffic/rules/${encodeURIComponent(id)}`);
}
export function syncRoutes() {
  return postJSON<{ message: string }>("/admin/traffic/sync");
}

export function listTrafficPolicies() {
  return fetchJSON<{ data: TrafficPolicy[] }>("/admin/traffic/policies").then(r => r.data);
}
export function getTrafficPolicy(id: string) {
  return fetchJSON<{ data: TrafficPolicy }>(`/admin/traffic/policies/${encodeURIComponent(id)}`).then(r => r.data);
}
export function createTrafficPolicy(policy: Partial<TrafficPolicy>) {
  return postJSON<{ data: TrafficPolicy }>("/admin/traffic/policies", policy);
}
export function updateTrafficPolicy(id: string, policy: Partial<TrafficPolicy>) {
  return putJSON<{ data: TrafficPolicy }>(`/admin/traffic/policies/${encodeURIComponent(id)}`, policy);
}
export function deleteTrafficPolicy(id: string) {
  return deleteJSON(`/admin/traffic/policies/${encodeURIComponent(id)}`);
}
