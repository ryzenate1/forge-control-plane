// Typed client for the cross-node admin API (/admin/crossnode/*).
// Shapes mirror the Go structs behind handlers_crossnode.go, field for field.
import { fetchJSON, isApiError, postJSON, unwrapData, unwrapList } from "./http";

/** `trafficmanager.AdapterHealth.status`. */
export type CrossNodeGatewayStatus = "unknown" | "healthy" | "degraded" | "down";

export type CrossNodeGatewayHealth = {
  status: CrossNodeGatewayStatus;
  message?: string;
  /** Go marshals `time.Duration` as whole nanoseconds, and omits it when zero. */
  uptime?: number;
  version?: string;
  errCount: number;
};

/** `crossnode.IngressSyncStats`, returned by /ingress/stats and as health.ingress. */
export type CrossNodeIngressStats = {
  ruleCount: number;
  policyCount: number;
  trackingCount: number;
  backendCount: number;
  /** Go `time.Time`; the zero value "0001-01-01T00:00:00Z" means never synced. */
  lastSync: string;
  syncCount: number;
  errCount: number;
  running: boolean;
  reconcilerConfigured: boolean;
};

/** `crossnode.BackendHealth`. `status` is the Go int: 0 unknown, 1 healthy, 2 degraded, 3 down. */
export type CrossNodeBackendHealth = {
  /** Always empty: the prober records host:port and never attributes a verdict to a server. */
  serverId: string;
  nodeId: string;
  host: string;
  port: number;
  status: number;
  lastChecked: string;
  failCount: number;
  reason?: string;
};

/** `crossnode.RouteKey` has no json tags, so Go emits its exported field names verbatim. */
export type CrossNodeRouteKey = {
  Domain: string;
  Path: string;
  Protocol: string;
};

/** `crossnode.RouteGenerationRecord`. */
export type CrossNodeRouteGroup = {
  routeKey: CrossNodeRouteKey;
  groupId: string;
  ruleIds: string[];
  serverIds: string[];
  backendCount: number;
  hasWebSocket: boolean;
  strategy: string;
  strategyConflict?: string;
};

/** `trafficmanager.RoutingRule`. /ingress/rules only ever carries enabled rules. */
export type CrossNodeRoutingRule = {
  id: string;
  name: string;
  serverId?: string;
  domain: string;
  path: string;
  targetHost?: string;
  targetPort: number;
  protocol: string;
  strategy: string;
  weight: number;
  headers?: Record<string, string>;
  enabled: boolean;
  webSocketSupport: boolean;
  createdAt: string;
};

/** `trafficmanager.TrafficPolicy`, keyed by policy id in /ingress/policies. */
export type CrossNodeTrafficPolicy = {
  id: string;
  name: string;
  rateLimit: number;
  rateLimitBurst: number;
  ipWhitelist?: string[];
  ipBlacklist?: string[];
  tlsEnabled: boolean;
  tlsCertFile?: string;
  tlsKeyFile?: string;
  circuitBreaker: boolean;
  circuitBreakerThreshold: number;
  circuitBreakerTimeout: number;
};

/** `crossnode.SyncResult`. */
export type CrossNodeSyncResult = {
  observedRules: number;
  groups: number;
  healthyBackends: number;
  skipped: boolean;
  reason?: string;
};

/** Only "ok" may render healthy; any other string the backend emits is unverified. */
export type CrossNodeHealthStatus = "ok" | "degraded";

export type CrossNodeHealth = {
  resolver_available: boolean;
  ingress_sync_available: boolean;
  status: CrossNodeHealthStatus;
  reasons?: string[];
  ingress?: CrossNodeIngressStats;
  gateway?: CrossNodeGatewayHealth;
};

/** Discriminated so a mutation that merely returned 200 cannot be read as a resolved target. */
export type CrossNodeResolveResult =
  | { resolved: true; host: string }
  | { resolved: false; reason: string };

export type CrossNodeSyncOutcome = {
  synced: boolean;
  result: CrossNodeSyncResult | null;
};

export type CrossNodeMutationAck = {
  message?: string;
  ttl?: string;
};

/** A 404 here is the endpoint's honest "no reachable target", not a transport failure. */
export async function resolveCrossNodeTarget(params: {
  serverId?: string;
  nodeId?: string;
}): Promise<CrossNodeResolveResult> {
  const q = new URLSearchParams();
  if (params.serverId) q.set("server_id", params.serverId);
  if (params.nodeId) q.set("node_id", params.nodeId);
  try {
    const data = unwrapData(
      await fetchJSON<{ data: { host: string } } | { host: string }>(`/admin/crossnode/resolve${q.toString() ? `?${q}` : ""}`),
    );
    const host = typeof data?.host === "string" ? data.host.trim() : "";
    if (!host) return { resolved: false, reason: "Resolver answered with an empty host" };
    return { resolved: true, host };
  } catch (err) {
    if (isApiError(err) && err.status === 404) {
      return { resolved: false, reason: err.message };
    }
    throw err;
  }
}

export async function fetchCrossNodeHealth(): Promise<CrossNodeHealth> {
  return unwrapData(await fetchJSON<{ data: CrossNodeHealth } | CrossNodeHealth>("/admin/crossnode/health"));
}

export async function describeCrossNodeHost(host: string, port: number): Promise<{ description: string }> {
  return unwrapData(
    await fetchJSON<{ data: { description: string } } | { description: string }>(
      `/admin/crossnode/describe/${encodeURIComponent(host)}/${encodeURIComponent(String(port))}`,
    ),
  );
}

export async function clearCrossNodeCache(): Promise<CrossNodeMutationAck> {
  return postJSON<CrossNodeMutationAck>("/admin/crossnode/cache/clear", {});
}

export async function setCrossNodeCacheTTL(ttl: string): Promise<CrossNodeMutationAck> {
  return postJSON<CrossNodeMutationAck>("/admin/crossnode/cache/ttl", { ttl });
}

export async function fetchCrossNodeIngressRules(): Promise<CrossNodeRoutingRule[]> {
  return unwrapList(await fetchJSON<{ data: CrossNodeRoutingRule[] } | CrossNodeRoutingRule[]>("/admin/crossnode/ingress/rules"));
}

export async function fetchCrossNodeIngressPolicies(): Promise<CrossNodeTrafficPolicy[]> {
  const byId = unwrapData(
    await fetchJSON<{ data: Record<string, CrossNodeTrafficPolicy> } | Record<string, CrossNodeTrafficPolicy>>(
      "/admin/crossnode/ingress/policies",
    ),
  );
  return Object.entries(byId ?? {})
    .filter((entry): entry is [string, CrossNodeTrafficPolicy] => Boolean(entry[1]))
    .sort((a, b) => (a[1].name || a[0]).localeCompare(b[1].name || b[0]))
    .map(([, policy]) => policy);
}

export async function fetchCrossNodeBackends(): Promise<CrossNodeBackendHealth[]> {
  return unwrapList(await fetchJSON<{ data: CrossNodeBackendHealth[] } | CrossNodeBackendHealth[]>("/admin/crossnode/ingress/backends"));
}

export async function fetchCrossNodeRouteGroups(): Promise<CrossNodeRouteGroup[]> {
  return unwrapList(await fetchJSON<{ data: CrossNodeRouteGroup[] } | CrossNodeRouteGroup[]>("/admin/crossnode/ingress/route-groups"));
}

export async function fetchCrossNodeIngressHealthStats(): Promise<CrossNodeGatewayHealth> {
  return unwrapData(
    await fetchJSON<{ data: CrossNodeGatewayHealth } | CrossNodeGatewayHealth>("/admin/crossnode/ingress/health/stats"),
  );
}

export async function fetchCrossNodeIngressStats(): Promise<CrossNodeIngressStats> {
  return unwrapData(await fetchJSON<{ data: CrossNodeIngressStats } | CrossNodeIngressStats>("/admin/crossnode/ingress/stats"));
}

/** 503 (no reconciler wired) throws an ApiError, so only a real reconcile reaches this. */
export async function triggerCrossNodeIngressSync(): Promise<CrossNodeSyncOutcome> {
  const res = await postJSON<{ data?: CrossNodeSyncResult; synced?: boolean }>("/admin/crossnode/ingress/sync", {});
  return { synced: res?.synced === true, result: res?.data ?? null };
}

export async function cleanupCrossNodeIngressStale(): Promise<{ cleaned: boolean; message?: string }> {
  return postJSON<{ cleaned: boolean; message?: string }>("/admin/crossnode/ingress/cleanup", {});
}
