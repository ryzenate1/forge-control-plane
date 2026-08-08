import { fetchJSON, postJSON } from './http';
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

export interface SystemInfo {
  nodes: NodeMetrics[];
  unacknowledgedAlerts: number;
  recentHealthChecks: ApiEndpointHealthRecord[];
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
  const res = await fetchJSON<{ data: NodeMetrics[] }>(`/monitoring/nodes/metrics${qs ? `?${qs}` : ''}`);
  return res.data ?? [];
}

export function getSystemInfo(): Promise<SystemInfo> {
  return fetchJSON<SystemInfo>('/monitoring/summary');
}

export async function getAlertHistory(params?: { page?: number; limit?: number }): Promise<AlertEvent[]> {
  const query = new URLSearchParams();
  if (params?.page) query.set('page', String(params.page));
  if (params?.limit) query.set('limit', String(params.limit));
  const qs = query.toString();
  const res = await fetchJSON<{ alerts: AlertEvent[] }>(`/alerts${qs ? `?${qs}` : ''}`);
  return res.alerts ?? [];
}

export function acknowledgeAlert(id: string): Promise<void> {
  return postJSON<void>(`/alerts/${encodeURIComponent(id)}/acknowledge`);
}
