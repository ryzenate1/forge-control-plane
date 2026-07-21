import { fetchJSON, postJSON, patchJSON, deleteJSON } from './http';

export type DBContainerEngine = 'postgresql' | 'mysql' | 'mariadb' | 'redis' | 'mongodb';

export type DBContainer = {
  id: string;
  serverId: string;
  engine: string;
  version: string;
  containerId: string;
  connectionString: string;
  credentials?: Record<string, string>;
  status: string;
  port: number;
  volumeId: string;
  memoryMb: number;
  cpuShares: number;
  createdAt: string;
  updatedAt: string;
};

export type ProvisionDBContainerRequest = {
  engine: DBContainerEngine;
  version: string;
  memoryMb?: number;
  cpuShares?: number;
};

export type DBContainerCredentials = {
  credentials: Record<string, string>;
  connectionString: string;
};

export type EnginesMap = Record<string, string[]>;

export function listDBContainers(params?: { engine?: string }): Promise<DBContainer[]> {
  const query = params?.engine ? `?engine=${encodeURIComponent(params.engine)}` : '';
  return fetchJSON<DBContainer[]>(`/databases/containers${query}`);
}

export function getDBContainer(id: string): Promise<DBContainer> {
  return fetchJSON<DBContainer>(`/databases/containers/${encodeURIComponent(id)}`);
}

export function provisionDBContainer(config: ProvisionDBContainerRequest): Promise<DBContainer> {
  return postJSON<DBContainer>('/databases/provision', config);
}

export function deprovisionDBContainer(id: string): Promise<{ ok: boolean }> {
  return deleteJSON<{ ok: boolean }>(`/databases/containers/${encodeURIComponent(id)}`);
}

export function backupDBContainer(id: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/databases/containers/${encodeURIComponent(id)}/backup`);
}

export function restartDBContainer(id: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/databases/containers/${encodeURIComponent(id)}/restart`);
}

export function getDBContainerCredentials(id: string): Promise<DBContainerCredentials> {
  return fetchJSON<DBContainerCredentials>(`/databases/containers/${encodeURIComponent(id)}/credentials`);
}

export function fetchDBEngines(): Promise<EnginesMap> {
  return fetchJSON<EnginesMap>('/databases/engines');
}

export type BackupProvidersResponse = {
  providers: string[];
};

export function fetchBackupProviders(): Promise<BackupProvidersResponse> {
  return fetchJSON<BackupProvidersResponse>('/backup/providers');
}

// Managed Database API (one-click DB containers with backup/restore)
export type ManagedDatabaseEngine = 'postgresql' | 'mysql' | 'mariadb' | 'mongodb' | 'redis' | 'libsql';

export type ManagedDatabase = {
  id: string;
  serverId?: string;
  name: string;
  engine: string;
  version: string;
  dockerImage?: string;
  status: string;
  host?: string;
  port: number;
  username?: string;
  databaseName?: string;
  connectionString?: string;
  credentials?: Record<string, string>;
  memoryMb: number;
  cpuShares: number;
  volumeId?: string;
  containerId?: string;
  createdAt: string;
  updatedAt: string;
};

export type CreateManagedDatabaseRequest = {
  name: string;
  engine: ManagedDatabaseEngine;
  version: string;
  serverId?: string;
  memoryMb?: number;
  cpuShares?: number;
};

export type ManagedDatabaseBackup = {
  id: string;
  managedDatabaseId: string;
  name: string;
  engine: string;
  status: string;
  size: number;
  checksum?: string;
  storagePath?: string;
  storageAdapter?: string;
  createdAt: string;
  completedAt?: string;
  updatedAt: string;
};

export type ManagedDatabaseRestore = {
  id: string;
  managedDatabaseId: string;
  backupId?: string;
  status: string;
  errorMessage?: string;
  createdAt: string;
  completedAt?: string;
  updatedAt: string;
};

export function listManagedDatabases(): Promise<ManagedDatabase[]> {
  return fetchJSON<ManagedDatabase[]>('/managed-databases');
}

export function getManagedDatabase(id: string): Promise<ManagedDatabase> {
  return fetchJSON<ManagedDatabase>(`/managed-databases/${encodeURIComponent(id)}`);
}

export function createManagedDatabase(req: CreateManagedDatabaseRequest): Promise<ManagedDatabase> {
  return postJSON<ManagedDatabase>('/managed-databases', req);
}

export function updateManagedDatabase(id: string, req: Partial<CreateManagedDatabaseRequest>): Promise<ManagedDatabase> {
  return patchJSON<ManagedDatabase>(`/managed-databases/${encodeURIComponent(id)}`, req);
}

export function deleteManagedDatabase(id: string): Promise<{ ok: boolean }> {
  return deleteJSON<{ ok: boolean }>(`/managed-databases/${encodeURIComponent(id)}`);
}

export function backupManagedDatabase(id: string): Promise<ManagedDatabaseBackup> {
  return postJSON<ManagedDatabaseBackup>(`/managed-databases/${encodeURIComponent(id)}/backup`);
}

export function restoreManagedDatabase(id: string, backupId: string): Promise<ManagedDatabaseRestore> {
  return postJSON<ManagedDatabaseRestore>(`/managed-databases/${encodeURIComponent(id)}/restore`, { backupId });
}

export function rotateManagedDatabasePassword(id: string): Promise<ManagedDatabase> {
  return postJSON<ManagedDatabase>(`/managed-databases/${encodeURIComponent(id)}/rotate-password`);
}

export function listManagedDatabaseBackups(id: string): Promise<ManagedDatabaseBackup[]> {
  return fetchJSON<ManagedDatabaseBackup[]>(`/managed-databases/${encodeURIComponent(id)}/backups`);
}

export function listManagedDatabaseRestores(id: string): Promise<ManagedDatabaseRestore[]> {
  return fetchJSON<ManagedDatabaseRestore[]>(`/managed-databases/${encodeURIComponent(id)}/restores`);
}

export function fetchManagedDBEngines(): Promise<EnginesMap> {
  return fetchJSON<EnginesMap>('/managed-databases/engines');
}
