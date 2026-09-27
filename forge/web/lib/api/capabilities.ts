import { fetchJSON, postJSON } from "./http";

export type NodeCapability = {
  id: string;
  nodeId: string;
  beaconVersion: string;
  os: string;
  architecture: string;
  cpuThreads: number;
  memoryMb: number;
  diskMb: number;
  uptimeSeconds: number;
  runtimeAvailable: boolean;
  runtimeStatus: string;
  runtimeVersion: string;
  runtimeProvider: string;
  dockerBuildEnabled: boolean;
  nixpacksEnabled: boolean;
  composeEnabled: boolean;
  composeVersion?: string;
  stackCount: number;
  localBackups: boolean;
  s3Backups: boolean;
  transferEnabled: boolean;
  sftpEnabled: boolean;
  webSocketEnabled: boolean;
  consoleEnabled: boolean;
  databaseProvisioningEnabled: boolean;
  rawReport?: unknown;
  fetchedAt: string;
  createdAt: string;
  updatedAt: string;
  // joined fields when listing inventory (node_capabilities JOIN nodes)
  nodeName?: string;
  nodeStatus?: string;
};

export type CapabilityHistoryEntry = {
  id: string;
  nodeId: string;
  beaconVersion: string;
  capabilities: unknown;
  rawReport: unknown;
  observedAt: string;
};

export type CapabilityDelta = {
  nodeId: string;
  fetchedAt: string;
  added: unknown[];
  removed: unknown[];
  changed: unknown[];
  unchanged: unknown[];
};

export type ProbeResult =
  | { online: false; error?: string }
  | { online: true; capabilities: NodeCapability };

export function fetchCapabilities(offset = 0, limit = 50): Promise<NodeCapability[]> {
  const q = new URLSearchParams({ offset: String(offset), limit: String(limit) }).toString();
  return fetchJSON<{ data: NodeCapability[] } | NodeCapability[]>(`/capabilities?${q}`).then((r) => {
    if (Array.isArray(r)) return r;
    return (r as { data: NodeCapability[] }).data ?? [];
  });
}

export function fetchCapability(nodeId: string): Promise<NodeCapability> {
  return fetchJSON<NodeCapability | { data: NodeCapability }>(`/capabilities/${encodeURIComponent(nodeId)}`).then((r) => {
    const maybeData = (r as { data: NodeCapability }).data;
    return maybeData ?? (r as NodeCapability);
  });
}

export function fetchCapabilityHistory(nodeId: string, limit = 20): Promise<CapabilityHistoryEntry[]> {
  const q = new URLSearchParams({ limit: String(limit) }).toString();
  return fetchJSON<{ data: CapabilityHistoryEntry[] } | CapabilityHistoryEntry[]>(
    `/capabilities/${encodeURIComponent(nodeId)}/history?${q}`,
  ).then((r) => {
    if (Array.isArray(r)) return r;
    return (r as { data: CapabilityHistoryEntry[] }).data ?? [];
  });
}

export function fetchCapabilityDelta(nodeId: string): Promise<CapabilityDelta> {
  return fetchJSON<CapabilityDelta | { data: CapabilityDelta }>(`/capabilities/${encodeURIComponent(nodeId)}/delta`).then((r) => {
    // handlers return { nodeId, fetchedAt, added, removed, changed, unchanged } directly
    if (r && typeof r === "object" && "nodeId" in (r as Record<string, unknown>)) return r as CapabilityDelta;
    const maybeData = (r as { data: CapabilityDelta }).data;
    return maybeData ?? (r as CapabilityDelta);
  });
}

export function probeCapabilities(nodeId: string): Promise<ProbeResult> {
  return postJSON<ProbeResult | { online: boolean; error?: string; capabilities?: NodeCapability }>(
    `/capabilities/${encodeURIComponent(nodeId)}/probe`,
    {},
  ).then((r) => r as ProbeResult);
}

/**
 * One runtime engine as the control plane actually reports it.
 *
 * `available` means an adapter is registered *and* some node can serve it, so a
 * create form can offer the engine without lying about what would happen if it
 * were chosen. `reason` explains every refusal.
 */
export type WorkloadKind = {
  provider: string;
  description: string;
  supported: boolean;
  experimental: boolean;
  registered: boolean;
  available: boolean;
  reason?: string;
  capabilities?: Record<string, boolean>;
  nodes: Array<{ id: string; name: string; state: string; eligible: boolean }>;
};

export function fetchWorkloadKinds(): Promise<WorkloadKind[]> {
  return fetchJSON<{ data: WorkloadKind[] } | WorkloadKind[]>("/workload-kinds").then((r) =>
    Array.isArray(r) ? r : (r.data ?? []),
  );
}
