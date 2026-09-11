import { fetchJSON, postJSON, unwrapData, unwrapList } from './http';
import type { ApiEndpointHealthRecord } from './types';

export interface NodeMetrics {
  id: string;
  nodeId: string;
  cpuPercent: number;
  memoryPercent: number;
  diskPercent: number;
  memoryUsedMb: number;
  memoryTotalMb: number;
  diskUsedMb: number;
  diskTotalMb: number;
  cpuLoad1m: number;
  cpuLoad5m: number;
  cpuLoad15m: number;
  networkRxBytes: number;
  networkTxBytes: number;
  containerRunning: number;
  containerTotal: number;
  observedAt: string;
}

// The summary returns health_history rows, not complete endpoint telemetry.
// Optional endpoint fields are retained only when actually reported.
export type MonitoringHealthRecord = Partial<ApiEndpointHealthRecord> & {
  checkName?: string;
  name?: string;
  message?: string;
  latencyMs?: number;
  critical?: boolean;
  details?: Record<string, unknown>;
};

export interface SystemInfo {
  nodes?: NodeMetrics[] | null;
  unacknowledgedAlerts?: number | null;
  recentHealthChecks?: MonitoringHealthRecord[] | null;
  totalServers?: number;
  totalUsers?: number;
}

export interface AlertEvent {
  id: string;
  type: string;
  message: string;
  severity: string;
  acknowledged: boolean;
  createdAt: string;
}

export async function getNodeMetrics(params?: { nodeId?: string; period?: string; limit?: number; since?: string }): Promise<NodeMetrics[]> {
  const query = new URLSearchParams();
  if (params?.nodeId) query.set('nodeId', params.nodeId);
  if (params?.period) query.set('period', params.period);
  if (params?.limit != null) query.set('limit', String(params.limit));
  if (params?.since) query.set('since', params.since);
  const qs = query.toString();
  // Backend returns `{ data: [...] }` (or a bare array on older routes) —
  // unwrap via the shared helper so contract drift throws instead of
  // rendering as an empty chart.
  return unwrapList(
    await fetchJSON<{ data: NodeMetrics[] } | NodeMetrics[]>(`/monitoring/nodes/metrics${qs ? `?${qs}` : ''}`),
  );
}

export async function getSystemInfo(): Promise<SystemInfo> {
  // Summary is a single object (bare or `{ data }`); unwrap without inventing
  // missing sections — absent stays `undefined` so the UI renders "unknown",
  // never a confident zero.
  const data = unwrapData(await fetchJSON<SystemInfo | { data: SystemInfo }>('/monitoring/summary'));
  return {
    ...data,
    recentHealthChecks: data.recentHealthChecks?.map((row) => ({
      ...row,
      endpointId: row.endpointId ?? row.checkName ?? row.name,
    })),
  };
}

export async function getAlertHistory(params?: { page?: number; limit?: number }): Promise<AlertEvent[]> {
  const query = new URLSearchParams();
  if (params?.page) query.set('page', String(params.page));
  if (params?.limit) query.set('limit', String(params.limit));
  const qs = query.toString();
  const res = await fetchJSON<{ alerts: AlertEvent[] } | AlertEvent[]>(`/alerts${qs ? `?${qs}` : ''}`);
  if (Array.isArray(res)) return res;
  return res.alerts ?? [];
}

export function acknowledgeAlert(id: string): Promise<void> {
  return postJSON<void>(`/alerts/${encodeURIComponent(id)}/acknowledge`);
}

export function getAlert(id: string): Promise<AlertEvent> {
  return fetchJSON<AlertEvent>(`/alerts/${encodeURIComponent(id)}`);
}

export function resolveAlert(id: string): Promise<void> {
  return postJSON<void>(`/alerts/${encodeURIComponent(id)}/resolve`);
}

/**
 * @deprecated Use {@link resolveAlert} (POST). A state-changing GET is
 * uncacheable-unfriendly and can fire from prefetching; kept as a same-shape
 * alias that issues the canonical POST so legacy callers keep compiling
 * without issuing a GET mutation.
 */
export function resolveAlertGET(id: string): Promise<void> {
  return resolveAlert(id);
}

export type MetricPeriod = "1h" | "6h" | "24h" | "7d" | "30d";

export function metricWindow(period: MetricPeriod): { limit: number; since: string } {
  const now = Date.now();
  const map: Record<MetricPeriod, number> = {
    "1h": 60 * 60 * 1000,
    "6h": 6 * 60 * 60 * 1000,
    "24h": 24 * 60 * 60 * 1000,
    "7d": 7 * 24 * 60 * 60 * 1000,
    "30d": 30 * 24 * 60 * 60 * 1000,
  };
  const duration = map[period] ?? map["1h"];
  const limit = period === "1h" ? 60 : period === "6h" ? 72 : period === "24h" ? 96 : period === "7d" ? 168 : 240;
  return { limit, since: new Date(now - duration).toISOString() };
}
