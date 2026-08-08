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

export function fetchHostInfo(init?: RequestInit): Promise<HostInfo> {
  return fetchJSON<HostInfo>('/host/info', init);
}

export function fetchHostDisk(init?: RequestInit): Promise<DiskPartition[]> {
  return fetchJSON<DiskPartition[]>('/host/disk', init);
}

export function fetchHostMemory(init?: RequestInit): Promise<MemoryInfo> {
  return fetchJSON<MemoryInfo>('/host/memory', init);
}

export function fetchHostNetwork(init?: RequestInit): Promise<NetworkInterface[]> {
  return fetchJSON<NetworkInterface[]>('/host/network', init);
}

export function fetchHostProcesses(init?: RequestInit): Promise<ProcessEntry[]> {
  return fetchJSON<ProcessEntry[]>('/host/processes', init);
}
