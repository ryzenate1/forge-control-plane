import { fetchJSON, postJSON, deleteJSON, putJSON } from './http';

export type DatabaseService = {
  id: string;
  name: string;
  type: string;
  version: string;
  status: string;
  host: string;
  port: number;
  username: string;
  databaseName: string;
  containerId: string;
  volumeId: string;
  memoryMb: number;
  cpuShares: number;
  serverId: string | null;
  connectionString: string;
  credentials?: Record<string, string>;
  templateId: string | null;
  createdAt: string;
  updatedAt: string;
};

export type DatabaseServiceBackup = {
  id: string;
  serviceId: string;
  status: string;
  filePath: string;
  sizeBytes: number;
  createdAt: string;
};

export type DatabaseServiceCredential = {
  id: string;
  serviceId: string;
  username: string;
  databaseName: string;
  permissions: string;
  createdAt: string;
  revokedAt: string | null;
};

export type ServiceTemplate = {
  id: string;
  type: string;
  version: string;
  dockerImage: string;
  defaultPort: number;
  defaultDatabase: string;
  minMemoryMb: number;
  createdAt: string;
};

export type ProvisionDBServiceRequest = {
  name: string;
  type: string;
  version: string;
  memoryMb?: number;
  cpuShares?: number;
};

export type TestConnectionRequest = {
  host: string;
  port: number;
  engine: string;
  username: string;
  password: string;
  databaseName: string;
};

// Database Services
export function listDatabaseServices(): Promise<DatabaseService[]> {
  return fetchJSON<DatabaseService[]>('/admin/database-services');
}

export function getDatabaseService(id: string): Promise<DatabaseService> {
  return fetchJSON<DatabaseService>(`/admin/database-services/${encodeURIComponent(id)}`);
}

export function provisionDatabaseService(config: ProvisionDBServiceRequest): Promise<DatabaseService> {
  return postJSON<DatabaseService>('/admin/database-services', config);
}

export function deleteDatabaseService(id: string): Promise<{ ok: boolean }> {
  return deleteJSON<{ ok: boolean }>(`/admin/database-services/${encodeURIComponent(id)}`);
}

export function restartDatabaseService(id: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/admin/database-services/${encodeURIComponent(id)}/restart`);
}

// Backups
export function createServiceBackup(serviceId: string): Promise<DatabaseServiceBackup> {
  return postJSON<DatabaseServiceBackup>(`/admin/database-services/${encodeURIComponent(serviceId)}/backups`);
}

export function listServiceBackups(serviceId: string): Promise<DatabaseServiceBackup[]> {
  return fetchJSON<DatabaseServiceBackup[]>(`/admin/database-services/${encodeURIComponent(serviceId)}/backups`);
}

export function restoreServiceBackup(serviceId: string, backupId: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/admin/database-services/${encodeURIComponent(serviceId)}/backups/${encodeURIComponent(backupId)}/restore`);
}

// Logs
export function getServiceLogs(serviceId: string): Promise<{ logs: string[] }> {
  return fetchJSON<{ logs: string[] }>(`/admin/database-services/${encodeURIComponent(serviceId)}/logs`);
}

// Credentials
export function createServiceCredential(serviceId: string, data: { username: string; password: string; database?: string; permissions?: string }): Promise<DatabaseServiceCredential> {
  return postJSON<DatabaseServiceCredential>(`/admin/database-services/${encodeURIComponent(serviceId)}/credentials`, data);
}

export function listServiceCredentials(serviceId: string): Promise<DatabaseServiceCredential[]> {
  return fetchJSON<DatabaseServiceCredential[]>(`/admin/database-services/${encodeURIComponent(serviceId)}/credentials`);
}

export function revokeServiceCredential(serviceId: string, credId: string): Promise<{ ok: boolean }> {
  return deleteJSON<{ ok: boolean }>(`/admin/database-services/${encodeURIComponent(serviceId)}/credentials/${encodeURIComponent(credId)}`);
}

// Test Connection
export function testConnection(req: TestConnectionRequest): Promise<{ ok: boolean; message: string }> {
  return postJSON<{ ok: boolean; message: string }>('/admin/database-services/test-connection', req);
}

// Service Templates
export function listServiceTemplates(): Promise<ServiceTemplate[]> {
  return fetchJSON<ServiceTemplate[]>('/admin/database-service-templates');
}

export function createServiceTemplate(data: {
  type: string;
  version: string;
  dockerImage: string;
  defaultPort: number;
  defaultDatabase?: string;
  minMemoryMb?: number;
}): Promise<ServiceTemplate> {
  return postJSON<ServiceTemplate>('/admin/database-service-templates', data);
}

// Server-linked database services
export function listServerDatabaseServices(serverId: string): Promise<DatabaseService[]> {
  return fetchJSON<DatabaseService[]>(`/servers/${encodeURIComponent(serverId)}/database-services`);
}

export function linkDatabaseServiceToServer(serverId: string, serviceId: string): Promise<{ ok: boolean }> {
  return putJSON<{ ok: boolean }>(`/servers/${encodeURIComponent(serverId)}/database-service`, { serviceId });
}

export function unlinkDatabaseServiceFromServer(serverId: string, serviceId: string): Promise<{ ok: boolean }> {
  return deleteJSON<{ ok: boolean }>(`/servers/${encodeURIComponent(serverId)}/database-service`, { serviceId });
}
