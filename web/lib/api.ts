// Types only — components use raw fetch via /api/proxy/* routes

export type HealthStatus = 'healthy' | 'unhealthy' | 'unreachable';
export type BackupStatus = 'completed' | 'pending' | 'failed' | 'restoring' | 'restored' | 'restore_failed';
export type ServerStatus = 'running' | 'stopped' | 'installing';

export type HealthCheck = {
  status: HealthStatus;
  checks: Array<{ name: string; status: HealthStatus; message?: string }>;
  checkedAt: string;
};

export type SystemInfo = {
  version: string;
  os: string;
  architecture: string;
  cpuThreads: number;
  memory: { total: number; used: number; free: number };
  disk: { total: number; used: number; free: number };
  runtime: { reachable: boolean };
  uptime: number;
  activeSessions: number;
};

export type Backup = {
  uuid: string;
  name: string;
  size?: number;
  checksum?: string;
  status: BackupStatus;
  createdAt: string;
  completedAt?: string;
  isLocked: boolean;
};

export type Server = {
  id: string;
  name: string;
  node: string;
  status: ServerStatus;
};
