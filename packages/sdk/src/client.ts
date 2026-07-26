import type {
  ApiUser,
  ApiServer,
  ApiNode,
  ApiAllocation,
  ApiDatabase,
  ApiBackup,
  ApiSchedule,
  ApiScheduleTask,
  ApiDatabaseHost,
  ApiMount,
  ApiRegion,
  ApiLocation,
  ApiNest,
  ApiEgg,
  ApiRole,
  ApiStats,
  ApiKey,
  ApiSSHKey,
  ApiOAuthClient,
  ApiPlugin,
  ApiWebhook,
  ApiActivityLog,
  ApiAuditEvent,
  ApiFileEntry,
  ApiFileContent,
  ApiTask,
  ApiNotification,
  ApiAlert,
  ApiHealthReport,
  ApiPanelSettings,
  ApiPublicPanelSettings,
  ApiSetupStatus,
  LoginResponse,
  ServerCreateInput,
  ServerUpdateInput,
  CreateNodeInput,
  UpdateNodeInput,
  CreateAllocationInput,
  DatabaseCreateInput,
  ScheduleCreateInput,
  ScheduleUpdateInput,
  ScheduleTaskCreateInput,
  BackupCreateInput,
  CreateDatabaseHostInput,
  CreateMountInput,
  CreateEggInput,
  PaginatedResponse,
} from '@forge/shared-types';

export interface ApiClientConfig {
  baseUrl: string;
  token?: string;
  apiKey?: string;
  fetch?: typeof globalThis.fetch;
  headers?: Record<string, string>;
  useCookies?: boolean;
  timeoutMs?: number;
}

export class ApiError extends Error {
  constructor(
    public status: number,
    public statusText: string,
    public data?: unknown
  ) {
    super(`API Error ${status}: ${statusText}`);
    this.name = 'ApiError';
  }
}

export class ForgeApiClient {
  private baseUrl: string;
  private token?: string;
  private apiKey?: string;
  private fetcher: typeof globalThis.fetch;
  private defaultHeaders: Record<string, string>;
  private useCookies: boolean;
  private timeoutMs: number;

  constructor(config: ApiClientConfig) {
    this.baseUrl = config.baseUrl.replace(/\/$/, '');
    this.token = config.token;
    this.apiKey = config.apiKey;
    this.fetcher = config.fetch || globalThis.fetch.bind(globalThis);
    this.defaultHeaders = config.headers || {};
    this.useCookies = config.useCookies ?? false;
    this.timeoutMs = config.timeoutMs ?? 30000;
  }

  public setToken(token: string | undefined) {
    this.token = token;
  }

  public setApiKey(apiKey: string | undefined) {
    this.apiKey = apiKey;
  }

  private getCSRFToken(): string | null {
    if (typeof document === 'undefined') return null;
    const match = document.cookie.match(/__Host-forge_csrf=([^;]+)/);
    return match ? decodeURIComponent(match[1]) : null;
  }

  private async request<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    const url = `${this.baseUrl}${endpoint.startsWith('/') ? endpoint : `/${endpoint}`}`;
    const method = (options.method ?? 'GET').toUpperCase();

    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      Accept: 'application/json',
      ...this.defaultHeaders,
      ...(options.headers as Record<string, string>),
    };

    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`;
    } else if (this.apiKey) {
      headers['X-API-Key'] = this.apiKey;
    }

    if (this.useCookies && ['POST', 'PUT', 'PATCH', 'DELETE'].includes(method)) {
      const csrfToken = this.getCSRFToken();
      if (csrfToken) {
        headers['X-CSRF-Token'] = csrfToken;
      }
    }

    const isRetryable = method === 'GET';
    const maxAttempts = isRetryable ? 3 : 1;

    let lastError: unknown;
    for (let attempt = 0; attempt < maxAttempts; attempt++) {
      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(new Error('Request timeout')), this.timeoutMs);

      const requestInit: RequestInit = {
        ...options,
        headers,
        signal: options.signal
          ? combineSignals(options.signal, controller.signal)
          : controller.signal,
      };

      if (this.useCookies) {
        (requestInit as Record<string, unknown>).credentials = 'include';
      }

      try {
        const response = await this.fetcher(url, requestInit);
        clearTimeout(timer);

        if (!response.ok) {
          if (isRetryable && [502, 503, 504, 429].includes(response.status) && attempt < maxAttempts - 1) {
            await new Promise((res) => setTimeout(res, (attempt + 1) * 300));
            continue;
          }
          let data: unknown;
          try {
            data = await response.json();
          } catch {
            data = await response.text();
          }
          throw new ApiError(response.status, response.statusText, data);
        }

        if (response.status === 204) {
          return {} as T;
        }

        return response.json() as Promise<T>;
      } catch (err) {
        clearTimeout(timer);
        if (err instanceof ApiError) throw err;
        if (isRetryable && err instanceof TypeError && attempt < maxAttempts - 1) {
          await new Promise((res) => setTimeout(res, (attempt + 1) * 300));
          continue;
        }
        lastError = err;
        break;
      }
    }

    throw lastError instanceof Error
      ? new ApiError(0, lastError.message)
      : new ApiError(0, 'Unknown error');
  }

  // Auth & System
  public async getSetupStatus(): Promise<ApiSetupStatus> {
    return this.request<ApiSetupStatus>('/setup/status');
  }

  public async login(credentials: { email: string; password: string }): Promise<LoginResponse> {
    return this.request<LoginResponse>('/auth/login', {
      method: 'POST',
      body: JSON.stringify(credentials),
    });
  }

  public async getHealth(): Promise<ApiHealthReport> {
    return this.request<ApiHealthReport>('/health');
  }

  public async getPublicSettings(): Promise<ApiPublicPanelSettings> {
    return this.request<ApiPublicPanelSettings>('/panel/settings/public');
  }

  public async getSettings(): Promise<ApiPanelSettings> {
    return this.request<ApiPanelSettings>('/admin/settings');
  }

  public async updateSettings(settings: Partial<ApiPanelSettings>): Promise<ApiPanelSettings> {
    return this.request<ApiPanelSettings>('/admin/settings', {
      method: 'PATCH',
      body: JSON.stringify(settings),
    });
  }

  // Users
  public async listUsers(): Promise<PaginatedResponse<ApiUser>> {
    return this.request<PaginatedResponse<ApiUser>>('/users');
  }

  public async getUser(id: string): Promise<ApiUser> {
    return this.request<ApiUser>(`/users/${id}`);
  }

  public async createUser(user: Partial<ApiUser>): Promise<ApiUser> {
    return this.request<ApiUser>('/users', {
      method: 'POST',
      body: JSON.stringify(user),
    });
  }

  public async updateUser(id: string, user: Partial<ApiUser>): Promise<ApiUser> {
    return this.request<ApiUser>(`/users/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(user),
    });
  }

  public async deleteUser(id: string): Promise<void> {
    return this.request<void>(`/users/${id}`, {
      method: 'DELETE',
    });
  }

  // Servers
  public async listServers(): Promise<PaginatedResponse<ApiServer>> {
    return this.request<PaginatedResponse<ApiServer>>('/servers');
  }

  public async getServer(id: string): Promise<ApiServer> {
    return this.request<ApiServer>(`/servers/${id}`);
  }

  public async createServer(server: ServerCreateInput): Promise<ApiServer> {
    return this.request<ApiServer>('/servers', {
      method: 'POST',
      body: JSON.stringify(server),
    });
  }

  public async updateServer(id: string, server: ServerUpdateInput): Promise<ApiServer> {
    return this.request<ApiServer>(`/servers/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(server),
    });
  }

  public async deleteServer(id: string): Promise<void> {
    return this.request<void>(`/servers/${id}`, {
      method: 'DELETE',
    });
  }

  public async sendPowerAction(serverId: string, signal: 'start' | 'stop' | 'restart' | 'kill'): Promise<void> {
    return this.request<void>(`/servers/${serverId}/power`, {
      method: 'POST',
      body: JSON.stringify({ signal }),
    });
  }

  public async sendCommand(serverId: string, command: string): Promise<void> {
    return this.request<void>(`/servers/${serverId}/command`, {
      method: 'POST',
      body: JSON.stringify({ command }),
    });
  }

  public async getServerStats(serverId: string): Promise<ApiStats> {
    return this.request<ApiStats>(`/servers/${serverId}/resources`);
  }

  // Files
  public async listFiles(serverId: string, directory = '/'): Promise<ApiFileEntry[]> {
    return this.request<ApiFileEntry[]>(`/servers/${serverId}/files/list?directory=${encodeURIComponent(directory)}`);
  }

  public async readFile(serverId: string, file: string): Promise<ApiFileContent> {
    return this.request<ApiFileContent>(`/servers/${serverId}/files/contents?file=${encodeURIComponent(file)}`);
  }

  public async writeFile(serverId: string, file: string, content: string): Promise<void> {
    return this.request<void>(`/servers/${serverId}/files/write`, {
      method: 'POST',
      body: JSON.stringify({ file, content }),
    });
  }

  // Nodes
  public async listNodes(): Promise<ApiNode[]> {
    return this.request<ApiNode[]>('/admin/nodes');
  }

  public async getNode(id: string): Promise<ApiNode> {
    return this.request<ApiNode>(`/admin/nodes/${id}`);
  }

  public async createNode(node: CreateNodeInput): Promise<ApiNode> {
    return this.request<ApiNode>('/admin/nodes', {
      method: 'POST',
      body: JSON.stringify(node),
    });
  }

  public async updateNode(id: string, node: UpdateNodeInput): Promise<ApiNode> {
    return this.request<ApiNode>(`/admin/nodes/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(node),
    });
  }

  public async deleteNode(id: string): Promise<void> {
    return this.request<void>(`/admin/nodes/${id}`, {
      method: 'DELETE',
    });
  }

  // Allocations
  public async listAllocations(nodeId: string): Promise<ApiAllocation[]> {
    return this.request<ApiAllocation[]>(`/admin/nodes/${nodeId}/allocations`);
  }

  public async createAllocation(allocation: CreateAllocationInput): Promise<ApiAllocation[]> {
    return this.request<ApiAllocation[]>(`/admin/nodes/${allocation.nodeId}/allocations`, {
      method: 'POST',
      body: JSON.stringify(allocation),
    });
  }

  public async deleteAllocation(nodeId: string, allocationId: string): Promise<void> {
    return this.request<void>(`/admin/nodes/${nodeId}/allocations/${allocationId}`, {
      method: 'DELETE',
    });
  }

  // Databases
  public async listDatabases(serverId: string): Promise<ApiDatabase[]> {
    return this.request<ApiDatabase[]>(`/servers/${serverId}/databases`);
  }

  public async createDatabase(serverId: string, input: DatabaseCreateInput): Promise<ApiDatabase> {
    return this.request<ApiDatabase>(`/servers/${serverId}/databases`, {
      method: 'POST',
      body: JSON.stringify(input),
    });
  }

  public async deleteDatabase(serverId: string, databaseId: string): Promise<void> {
    return this.request<void>(`/servers/${serverId}/databases/${databaseId}`, {
      method: 'DELETE',
    });
  }

  public async listDatabaseHosts(): Promise<ApiDatabaseHost[]> {
    return this.request<ApiDatabaseHost[]>('/admin/database-hosts');
  }

  public async createDatabaseHost(host: CreateDatabaseHostInput): Promise<ApiDatabaseHost> {
    return this.request<ApiDatabaseHost>('/admin/database-hosts', {
      method: 'POST',
      body: JSON.stringify(host),
    });
  }

  // Schedules
  public async listSchedules(serverId: string): Promise<ApiSchedule[]> {
    return this.request<ApiSchedule[]>(`/servers/${serverId}/schedules`);
  }

  public async createSchedule(serverId: string, schedule: ScheduleCreateInput): Promise<ApiSchedule> {
    return this.request<ApiSchedule>(`/servers/${serverId}/schedules`, {
      method: 'POST',
      body: JSON.stringify(schedule),
    });
  }

  public async updateSchedule(serverId: string, scheduleId: string, schedule: ScheduleUpdateInput): Promise<ApiSchedule> {
    return this.request<ApiSchedule>(`/servers/${serverId}/schedules/${scheduleId}`, {
      method: 'PATCH',
      body: JSON.stringify(schedule),
    });
  }

  public async deleteSchedule(serverId: string, scheduleId: string): Promise<void> {
    return this.request<void>(`/servers/${serverId}/schedules/${scheduleId}`, {
      method: 'DELETE',
    });
  }

  public async createScheduleTask(serverId: string, scheduleId: string, task: ScheduleTaskCreateInput): Promise<ApiScheduleTask> {
    return this.request<ApiScheduleTask>(`/servers/${serverId}/schedules/${scheduleId}/tasks`, {
      method: 'POST',
      body: JSON.stringify(task),
    });
  }

  // Backups
  public async listBackups(serverId: string): Promise<ApiBackup[]> {
    return this.request<ApiBackup[]>(`/servers/${serverId}/backups`);
  }

  public async createBackup(serverId: string, input?: BackupCreateInput): Promise<ApiBackup> {
    return this.request<ApiBackup>(`/servers/${serverId}/backups`, {
      method: 'POST',
      body: JSON.stringify(input || {}),
    });
  }

  public async deleteBackup(serverId: string, backupUuid: string): Promise<void> {
    return this.request<void>(`/servers/${serverId}/backups/${backupUuid}`, {
      method: 'DELETE',
    });
  }

  // Mounts
  public async listMounts(): Promise<ApiMount[]> {
    return this.request<ApiMount[]>('/admin/mounts');
  }

  public async createMount(mount: CreateMountInput): Promise<ApiMount> {
    return this.request<ApiMount>('/admin/mounts', {
      method: 'POST',
      body: JSON.stringify(mount),
    });
  }

  // Nests & Eggs
  public async listNests(): Promise<ApiNest[]> {
    return this.request<ApiNest[]>('/admin/nests');
  }

  public async listEggs(nestId: string): Promise<ApiEgg[]> {
    return this.request<ApiEgg[]>(`/admin/nests/${nestId}/eggs`);
  }

  public async getEgg(nestId: string, eggId: string): Promise<ApiEgg> {
    return this.request<ApiEgg>(`/admin/nests/${nestId}/eggs/${eggId}`);
  }

  public async createEgg(egg: CreateEggInput): Promise<ApiEgg> {
    return this.request<ApiEgg>(`/admin/nests/${egg.nestId}/eggs`, {
      method: 'POST',
      body: JSON.stringify(egg),
    });
  }

  // Tasks
  public async getTask(serverId: string, taskId: string): Promise<ApiTask> {
    return this.request<ApiTask>(`/servers/${serverId}/tasks/${taskId}`);
  }
}

function combineSignals(...signals: AbortSignal[]): AbortSignal {
  const controller = new AbortController();
  for (const signal of signals) {
    if (signal.aborted) {
      controller.abort(signal.reason);
      return controller.signal;
    }
    signal.addEventListener('abort', () => controller.abort(signal.reason), { once: true });
  }
  return controller.signal;
}

export function createApiClient(config: ApiClientConfig): ForgeApiClient {
  return new ForgeApiClient(config);
}
