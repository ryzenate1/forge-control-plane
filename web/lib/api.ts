// Types only — components use raw fetch via /api/proxy/* routes

export type HealthStatus = 'healthy' | 'unhealthy' | 'unreachable';
export type BackupStatus = 'completed' | 'pending' | 'failed';
export type BackupFormat = 'zip' | 'tar.gz';
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
  docker: { running: boolean; version?: string };
  uptime: number;
  activeSessions: number;
};

export type Backup = {
  uuid: string;
  name: string;
  size?: number;
  checksum?: string;
  format: BackupFormat;
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
