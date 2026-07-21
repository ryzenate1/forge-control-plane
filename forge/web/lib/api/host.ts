import { fetchJSON } from './http';

export interface HostInfo {
  hostname: string;
  os: string;
  kernel: string;
  uptimeSeconds: number;
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

export function fetchHostInfo(): Promise<HostInfo> {
  return fetchJSON<HostInfo>('/host/info');
}

export function fetchHostDisk(): Promise<DiskPartition[]> {
  return fetchJSON<DiskPartition[]>('/host/disk');
}

export function fetchHostMemory(): Promise<MemoryInfo> {
  return fetchJSON<MemoryInfo>('/host/memory');
}

export function fetchHostNetwork(): Promise<NetworkInterface[]> {
  return fetchJSON<NetworkInterface[]>('/host/network');
}

export function fetchHostProcesses(): Promise<ProcessEntry[]> {
  return fetchJSON<ProcessEntry[]>('/host/processes');
}
