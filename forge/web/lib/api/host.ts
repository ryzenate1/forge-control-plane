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

// All host endpoints resolve the target Beacon node via optional `nodeId`
// query param. Callers must pass the node id explicitly as the first argument
// and request options as the second — `fetchHostInfo(nodeId, init)`. Passing a
// RequestInit object as the first argument is rejected rather than silently
// treated as "no node, use the default", because resolving an ambiguous target
// to the first active node hides multi-node misrouting.

function rejectAmbiguousTarget(first: unknown, fnName: string): asserts first is string | undefined {
  if (first != null && typeof first !== 'string') {
    throw new TypeError(
      `${fnName}(nodeId?: string, init?: RequestInit): first argument must be a node id string, got ${Object.prototype.toString.call(first)}. ` +
      `Pass the node id first and RequestInit second.`,
    );
  }
}

function hostUrl(base: string, nodeId: string | undefined, fnName: string): string {
  rejectAmbiguousTarget(nodeId, fnName);
  return typeof nodeId === 'string' && nodeId ? `${base}${hostQuery(nodeId)}` : base;
}

export function fetchHostInfo(nodeId?: string, init?: RequestInit): Promise<HostInfo>;
export function fetchHostInfo(init?: RequestInit): Promise<HostInfo>;
export function fetchHostInfo(nodeIdOrInit?: string | RequestInit, init?: RequestInit): Promise<HostInfo> {
  if (nodeIdOrInit != null && typeof nodeIdOrInit !== 'string') {
    throw new TypeError(
      `fetchHostInfo(nodeId?: string, init?: RequestInit): first argument must be a node id string. ` +
      `Use fetchHostInfo(nodeId, init) — ambiguous init-first calls are rejected.`,
    );
  }
  return fetchJSON<HostInfo>(hostUrl('/host/info', nodeIdOrInit as string | undefined, 'fetchHostInfo'), init);
}

export function fetchHostDisk(nodeId?: string, init?: RequestInit): Promise<DiskPartition[]>;
export function fetchHostDisk(init?: RequestInit): Promise<DiskPartition[]>;
export function fetchHostDisk(nodeIdOrInit?: string | RequestInit, init?: RequestInit): Promise<DiskPartition[]> {
  if (nodeIdOrInit != null && typeof nodeIdOrInit !== 'string') {
    throw new TypeError(
      `fetchHostDisk(nodeId?: string, init?: RequestInit): first argument must be a node id string. ` +
      `Use fetchHostDisk(nodeId, init) — ambiguous init-first calls are rejected.`,
    );
  }
  return fetchJSON<DiskPartition[]>(hostUrl('/host/disk', nodeIdOrInit as string | undefined, 'fetchHostDisk'), init);
}

export function fetchHostMemory(nodeId?: string, init?: RequestInit): Promise<MemoryInfo>;
export function fetchHostMemory(init?: RequestInit): Promise<MemoryInfo>;
export function fetchHostMemory(nodeIdOrInit?: string | RequestInit, init?: RequestInit): Promise<MemoryInfo> {
  if (nodeIdOrInit != null && typeof nodeIdOrInit !== 'string') {
    throw new TypeError(
      `fetchHostMemory(nodeId?: string, init?: RequestInit): first argument must be a node id string. ` +
      `Use fetchHostMemory(nodeId, init) — ambiguous init-first calls are rejected.`,
    );
  }
  return fetchJSON<MemoryInfo>(hostUrl('/host/memory', nodeIdOrInit as string | undefined, 'fetchHostMemory'), init);
}

export function fetchHostNetwork(nodeId?: string, init?: RequestInit): Promise<NetworkInterface[]>;
export function fetchHostNetwork(init?: RequestInit): Promise<NetworkInterface[]>;
export function fetchHostNetwork(nodeIdOrInit?: string | RequestInit, init?: RequestInit): Promise<NetworkInterface[]> {
  if (nodeIdOrInit != null && typeof nodeIdOrInit !== 'string') {
    throw new TypeError(
      `fetchHostNetwork(nodeId?: string, init?: RequestInit): first argument must be a node id string. ` +
      `Use fetchHostNetwork(nodeId, init) — ambiguous init-first calls are rejected.`,
    );
  }
  return fetchJSON<NetworkInterface[]>(hostUrl('/host/network', nodeIdOrInit as string | undefined, 'fetchHostNetwork'), init);
}

export function fetchHostProcesses(nodeId?: string, init?: RequestInit): Promise<ProcessEntry[]>;
export function fetchHostProcesses(init?: RequestInit): Promise<ProcessEntry[]>;
export function fetchHostProcesses(nodeIdOrInit?: string | RequestInit, init?: RequestInit): Promise<ProcessEntry[]> {
  if (nodeIdOrInit != null && typeof nodeIdOrInit !== 'string') {
    throw new TypeError(
      `fetchHostProcesses(nodeId?: string, init?: RequestInit): first argument must be a node id string. ` +
      `Use fetchHostProcesses(nodeId, init) — ambiguous init-first calls are rejected.`,
    );
  }
  return fetchJSON<ProcessEntry[]>(hostUrl('/host/processes', nodeIdOrInit as string | undefined, 'fetchHostProcesses'), init);
}
