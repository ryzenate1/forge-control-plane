import { fetchJSON } from './http';

export interface HostInfo {
  hostname: string;
  os: string;
  kernel: string;
  /**
   * Wall time since the machine booted. Distinct from `daemonUptimeSeconds`:
   * a host that has run for weeks and a Beacon restarted seconds ago report
   * different numbers, and only showing one hides which happened.
   */
  uptimeSeconds: number;
  /** Time since the Beacon process started. Absent on older nodes. */
  daemonUptimeSeconds?: number;
  cpuModel: string;
  cpuCores: number;
  arch: string;
  time: string;
}

export interface DiskPartition {
  mountPoint: string;
  device: string;
  fsType: string;
  totalMb: number;
  usedMb: number;
  freeMb: number;
  usedPercent: number;
}

export interface MemoryInfo {
  totalMb: number;
  usedMb: number;
  freeMb: number;
  usedPercent: number;
  swapTotalMb: number;
  swapUsedMb: number;
  swapFreeMb: number;
}

export interface NetworkInterface {
  name: string;
  ips: string;
  mac: string;
  speedMbps: number;
  status: string;
}

export interface ProcessEntry {
  pid: number;
  name: string;
  cpuPercent: number;
  memoryPercent: number;
  state: string;
}

function hostQuery(nodeId?: string): string {
  return nodeId ? `?nodeId=${encodeURIComponent(nodeId)}` : '';
}

// All host endpoints resolve the target Beacon node via optional `nodeId` query
// param (fallback to first active node on the API side). Threading nodeId
// ensures multi-node deployments query the intended host rather than always
// hitting the default. Signature retains init-only backwards compat: if the
// first arg is a RequestInit object it is treated as init with no nodeId.

export function fetchHostInfo(nodeId?: string | RequestInit, init?: RequestInit): Promise<HostInfo> {
  // Back-compat: allow fetchHostInfo(init) call where init is passed as first arg
  let url = '/host/info';
  let requestInit: RequestInit | undefined = init;
  if (nodeId != null && typeof nodeId === 'object') {
    requestInit = nodeId as unknown as RequestInit;
  } else if (typeof nodeId === 'string' && nodeId) {
    url += hostQuery(nodeId);
  }
  return fetchJSON<HostInfo>(url, requestInit);
}

export function fetchHostDisk(nodeId?: string | RequestInit, init?: RequestInit): Promise<DiskPartition[]> {
  let url = '/host/disk';
  let requestInit: RequestInit | undefined = init;
  if (nodeId != null && typeof nodeId === 'object') {
    requestInit = nodeId as unknown as RequestInit;
  } else if (typeof nodeId === 'string' && nodeId) {
    url += hostQuery(nodeId);
  }
  return fetchJSON<DiskPartition[]>(url, requestInit);
}

export function fetchHostMemory(nodeId?: string | RequestInit, init?: RequestInit): Promise<MemoryInfo> {
  let url = '/host/memory';
  let requestInit: RequestInit | undefined = init;
  if (nodeId != null && typeof nodeId === 'object') {
    requestInit = nodeId as unknown as RequestInit;
  } else if (typeof nodeId === 'string' && nodeId) {
    url += hostQuery(nodeId);
  }
  return fetchJSON<MemoryInfo>(url, requestInit);
}

export function fetchHostNetwork(nodeId?: string | RequestInit, init?: RequestInit): Promise<NetworkInterface[]> {
  let url = '/host/network';
  let requestInit: RequestInit | undefined = init;
  if (nodeId != null && typeof nodeId === 'object') {
    requestInit = nodeId as unknown as RequestInit;
  } else if (typeof nodeId === 'string' && nodeId) {
    url += hostQuery(nodeId);
  }
  return fetchJSON<NetworkInterface[]>(url, requestInit);
}

export function fetchHostProcesses(nodeId?: string | RequestInit, init?: RequestInit): Promise<ProcessEntry[]> {
  let url = '/host/processes';
  let requestInit: RequestInit | undefined = init;
  if (nodeId != null && typeof nodeId === 'object') {
    requestInit = nodeId as unknown as RequestInit;
  } else if (typeof nodeId === 'string' && nodeId) {
    url += hostQuery(nodeId);
  }
  return fetchJSON<ProcessEntry[]>(url, requestInit);
}
