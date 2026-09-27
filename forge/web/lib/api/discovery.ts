import { fetchJSON, postJSON, deleteJSON, unwrapList, unwrapData } from "./http";

export type DiscoveryEndpointStatus = "healthy" | "unhealthy" | "unknown" | "draining";
export type DiscoveryProtocol = "tcp" | "udp";
export type NetworkAccess = "public" | "private" | "isolated";

export interface DiscoveryEndpoint {
  id: string;
  serviceName: string;
  serviceId: string;
  nodeId: string;
  nodeName: string;
  regionId?: string;
  address: string;
  port: number;
  protocol: DiscoveryProtocol;
  status: DiscoveryEndpointStatus;
  replicaIndex: number;
  tenantId?: string;
  lastHeartbeat: string;
  createdAt: string;
  updatedAt: string;
  metadata?: Record<string, string>;
}

export interface DiscoveryEndpointSet {
  serviceName: string;
  serviceId: string;
  endpoints: DiscoveryEndpoint[];
  tenantId?: string;
}

export interface DiscoveryFilter {
  service?: string;
  nodeId?: string;
  tenantId?: string;
  healthyOnly?: boolean;
}

export interface EndpointVisibility {
  id: string;
  nodeId: string;
  nodeName: string;
  address: string;
  port: number;
  protocol: DiscoveryProtocol;
  status: DiscoveryEndpointStatus;
  replicaIndex: number;
  reachable?: boolean;
  network: NetworkAccess;
  lastSeen: string;
}

export interface ServiceVisibilityView {
  serviceName: string;
  serviceId: string;
  tenantId?: string;
  endpointCount: number;
  healthyCount: number;
  nodes: string[];
  access: NetworkAccess;
  endpoints: EndpointVisibility[];
}

export interface NetworkVisibilityView {
  services: ServiceVisibilityView[];
  totalEndpoints: number;
  healthyCount: number;
  unhealthyCount: number;
  nodesCount: number;
  lastUpdated: string;
}

export interface NodeNetworkView {
  nodeId: string;
  nodeName: string;
  services: string[];
  endpoints: EndpointVisibility[];
  reachability?: ReachabilityResult[];
}

export interface ReachabilityResult {
  sourceNodeId: string;
  targetNodeId: string;
  serviceName: string;
  reachable: boolean;
  latency?: string;
  error?: string;
  checkedAt: string;
}

export interface ReaperStats {
  lastRun: string | null;
  count: number;
  interval: number;
}

export interface PolicyView {
  privateCIDRs: string[];
  allowedPorts: Record<string, number[]>;
}

function queryFromFilter(f: DiscoveryFilter = {}): string {
  const p = new URLSearchParams();
  if (f.service) p.set("service", f.service);
  if (f.nodeId) p.set("node_id", f.nodeId);
  if (f.tenantId) p.set("tenant_id", f.tenantId);
  if (f.healthyOnly) p.set("healthy_only", "true");
  const q = p.toString();
  return q ? `?${q}` : "";
}

// ---- Services / Endpoints ----

export function fetchDiscoveryServices(): Promise<DiscoveryEndpointSet[]> {
  return fetchJSON<{ data: DiscoveryEndpointSet[] } | DiscoveryEndpointSet[]>("/admin/service-discovery/services").then(unwrapList);
}

export function fetchDiscoveryEndpoints(filter?: DiscoveryFilter): Promise<DiscoveryEndpoint[]> {
  return fetchJSON<{ data: DiscoveryEndpoint[] } | DiscoveryEndpoint[]>(
    `/admin/service-discovery/endpoints${queryFromFilter(filter)}`,
  ).then(unwrapList);
}

export function fetchDiscoveryEndpoint(id: string): Promise<DiscoveryEndpoint> {
  return fetchJSON<{ data: DiscoveryEndpoint } | DiscoveryEndpoint>(
    `/admin/service-discovery/endpoints/${encodeURIComponent(id)}`,
  ).then(unwrapData);
}

export function registerDiscoveryEndpoint(
  input: Partial<DiscoveryEndpoint> & { serviceName: string; nodeId: string; address: string; port: number },
): Promise<DiscoveryEndpoint> {
  return postJSON<{ data: DiscoveryEndpoint } | DiscoveryEndpoint>("/admin/service-discovery/endpoints", input).then(unwrapData);
}

export function deleteDiscoveryEndpoint(id: string): Promise<void> {
  return deleteJSON<void>(`/admin/service-discovery/endpoints/${encodeURIComponent(id)}`);
}

export function updateDiscoveryEndpointStatus(id: string, status: DiscoveryEndpointStatus): Promise<void> {
  return postJSON<void>(`/admin/service-discovery/endpoints/${encodeURIComponent(id)}/status`, { status });
}

export function heartbeatDiscoveryEndpoint(id: string): Promise<void> {
  return postJSON<void>(`/admin/service-discovery/endpoints/${encodeURIComponent(id)}/heartbeat`, {});
}

export function resolveDiscoveryService(service: string, tenantId?: string): Promise<DiscoveryEndpoint[]> {
  const q = new URLSearchParams({ service });
  if (tenantId) q.set("tenant_id", tenantId);
  return fetchJSON<{ data: DiscoveryEndpoint[] } | DiscoveryEndpoint[]>(`/admin/service-discovery/resolve?${q.toString()}`).then(unwrapList);
}

// ---- Visibility / Reachability ----

export function fetchNetworkVisibility(): Promise<NetworkVisibilityView> {
  return fetchJSON<{ data: NetworkVisibilityView } | NetworkVisibilityView>("/admin/service-discovery/network/visibility").then(unwrapData);
}

export function fetchNodeNetworkView(nodeId: string): Promise<NodeNetworkView> {
  return fetchJSON<{ data: NodeNetworkView } | NodeNetworkView>(
    `/admin/service-discovery/network/nodes/${encodeURIComponent(nodeId)}`,
  ).then(unwrapData);
}

export function verifyReachability(input: {
  sourceNodeId: string;
  targetNodeId: string;
  serviceName: string;
}): Promise<ReachabilityResult> {
  return postJSON<{ data: ReachabilityResult } | ReachabilityResult>("/admin/service-discovery/reachability/verify", input).then(unwrapData);
}

export function sweepReachability(): Promise<ReachabilityResult[]> {
  return postJSON<{ data: ReachabilityResult[] } | ReachabilityResult[]>("/admin/service-discovery/reachability/sweep", {}).then(unwrapList);
}

export function fetchReaperStats(): Promise<ReaperStats> {
  return fetchJSON<{ data: ReaperStats } | ReaperStats>("/admin/service-discovery/reaper/stats").then(unwrapData);
}

// ---- Policy (PrivateNetworkPolicy) ----

export function fetchDiscoveryPolicy(): Promise<PolicyView> {
  return fetchJSON<{ data: PolicyView } | PolicyView>("/admin/service-discovery/policy").then(unwrapData);
}

export function addPrivateCIDR(cidr: string): Promise<PolicyView> {
  return postJSON<{ data: PolicyView } | PolicyView>("/admin/service-discovery/policy/cidrs", { cidr }).then(unwrapData);
}

export function removePrivateCIDR(cidr: string): Promise<PolicyView> {
  return deleteJSON<{ data: PolicyView } | PolicyView>(`/admin/service-discovery/policy/cidrs/${encodeURIComponent(cidr)}`).then(unwrapData);
}

export function allowPolicyPort(serviceName: string, port: number): Promise<PolicyView> {
  return postJSON<{ data: PolicyView } | PolicyView>("/admin/service-discovery/policy/ports/allow", { serviceName, port }).then(unwrapData);
}

export function revokePolicyPort(serviceName: string, port: number): Promise<PolicyView> {
  return postJSON<{ data: PolicyView } | PolicyView>("/admin/service-discovery/policy/ports/revoke", { serviceName, port }).then(unwrapData);
}
