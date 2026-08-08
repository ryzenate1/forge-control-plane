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
  PaginatedEnvelope,
} from '@forge/shared-types';

/** Configuration for the ForgeApiClient. */
export interface ApiClientConfig {
  /** Base URL of the API. Must include the `/api/v1` suffix (auto-appended when missing), e.g. `https://panel.example.com/api/v1`. */
  baseUrl: string;
  /** Bearer token used for `Authorization: Bearer` header authentication (takes precedence over `apiKey`). */
  token?: string;
  /** API key used for `X-API-Key` header authentication. */
  apiKey?: string;
  /** Custom fetch implementation (defaults to `globalThis.fetch`). */
  fetch?: typeof globalThis.fetch;
  /** Extra headers applied to every request. Accepts any `HeadersInit`. */
  headers?: HeadersInit;
  /** Enable cookie-based session auth (sends credentials and the `X-CSRF-Token` header on mutations). */
  useCookies?: boolean;
  /** Per-request timeout in milliseconds (default: 30000). */
  timeoutMs?: number;
}

/** Error thrown for non-2xx API responses. */
export class ApiError extends Error {
  constructor(
    /** HTTP status code of the response (0 when no response was received). */
    public status: number,
    /** HTTP status text of the response. */
    public statusText: string,
    /** Parsed response body (JSON object, or raw text when the body was not JSON). */
    public data?: unknown
  ) {
    super(`API Error ${status}: ${statusText}`);
    this.name = 'ApiError';
  }
}

/** Body accepted by `POST /setup` (initial panel setup). Only `email` and `password` are required. */
export type SetupRequest = {
  email: string;
  password: string;
  name?: string;
  orgName?: string;
  nodeName?: string;
  nodeFqdn?: string;
  smtpHost?: string;
  smtpPort?: string;
  smtpUser?: string;
  smtpPass?: string;
  smtpFrom?: string;
  smtpEncryption?: string;
  backupDriver?: string;
  s3Bucket?: string;
  s3Region?: string;
  s3Endpoint?: string;
  domainName?: string;
};

/** Response of `POST /setup`. */
export type SetupResponse = {
  ok: boolean;
  userId: string;
  email: string;
  nodeId?: string;
  nodeToken?: string;
};

/** Body accepted by `POST /auth/login/checkpoint` (2FA step of session login). */
export type LoginCheckpointInput = {
  /** Token returned by `login()` when 2FA is required. */
  confirmationToken: string;
  /** TOTP verification code. */
  code?: string;
  /** Recovery code, alternative to `code`. */
  recoveryToken?: string;
};

/** Body accepted by `POST /users` (admin user creation). */
export type UserCreateInput = {
  email: string;
  password: string;
  role?: string;
  cpuLimit?: number;
  memoryMbLimit?: number;
  diskMbLimit?: number;
  backupLimit?: number;
  databaseLimit?: number;
  allocationLimit?: number;
  subuserLimit?: number;
  scheduleLimit?: number;
  serverLimit?: number;
};

/** Response of `POST /nodes` — the node record plus its one-time registration token. */
export type CreateNodeResult = {
  node: ApiNode;
  token: string;
  meta: { resource?: string };
};

/** Paginated response of `GET /servers/:id/backups`. */
export type BackupListResponse = {
  data: ApiBackup[];
  pagination: {
    page: number;
    per_page: number;
    total: number;
    total_pages: number;
  };
};

/** Generic `{ "ok": true }` body returned by several DELETE endpoints. */
export type OkResponse = {
  ok: boolean;
};

function normalizeBaseUrl(baseUrl: string): string {
  let url = baseUrl.replace(/\/+$/, '');
  if (!/\/api\/v1$/.test(url)) {
    url += '/api/v1';
  }
  return url;
}

function normalizeHeaders(headers?: HeadersInit): Record<string, string> {
  if (!headers) return {};
  const out: Record<string, string> = {};
  if (headers instanceof Headers) {
    headers.forEach((value, key) => {
      out[key] = value;
    });
  } else if (Array.isArray(headers)) {
    for (const [key, value] of headers) {
      out[key] = value;
    }
  } else {
    Object.assign(out, headers);
  }
  return out;
}

function retryDelayMs(retryAfter: string | null, attempt: number): number {
  if (retryAfter) {
    const seconds = parseInt(retryAfter, 10);
    if (!Number.isNaN(seconds)) {
      return Math.max(seconds * 1000, 100);
    }
  }
  return Math.min(1000 * 2 ** attempt, 4000);
}

const RETRYABLE_STATUSES = [429, 502, 503, 504];

function isAbortError(err: unknown): boolean {
  return err instanceof Error && err.name === 'AbortError';
}

/**
 * Combine multiple abort signals into one. The returned signal aborts when any
 * input signal aborts, and removes all abort listeners once it has settled.
 */
export function combineSignals(...signals: AbortSignal[]): AbortSignal {
  const controller = new AbortController();
  const cleanup = () => {
    for (const signal of signals) {
      signal.removeEventListener('abort', onAbort);
    }
  };
  function onAbort() {
    controller.abort(signals.find((s) => s.aborted)?.reason);
    cleanup();
  }
  for (const signal of signals) {
    if (signal.aborted) {
      controller.abort(signal.reason);
      return controller.signal;
    }
    signal.addEventListener('abort', onAbort, { once: true });
  }
  if (!controller.signal.aborted) {
    controller.signal.addEventListener('abort', cleanup, { once: true });
  }
  return controller.signal;
}

/**
 * Client for the GamePanel (Forge) HTTP API. All endpoints live under the
 * `/api/v1` prefix; `baseUrl` may omit it and it will be appended.
 *
 * Authentication is cookie-session based: `login()` sets HttpOnly cookies, so
 * enable `useCookies: true` (browser) or a cookie jar (server) when using it.
 */
export class ForgeApiClient {
  private baseUrl: string;
  private token?: string;
  private apiKey?: string;
  private fetcher: typeof globalThis.fetch;
  private defaultHeaders: Record<string, string>;
  private useCookies: boolean;
  private timeoutMs: number;

  constructor(config: ApiClientConfig) {
    this.baseUrl = normalizeBaseUrl(config.baseUrl);
    this.token = config.token;
    this.apiKey = config.apiKey;
    this.fetcher = config.fetch || globalThis.fetch.bind(globalThis);
    this.defaultHeaders = normalizeHeaders(config.headers);
    this.useCookies = config.useCookies ?? false;
    this.timeoutMs = config.timeoutMs ?? 30000;
  }

  /** Set (or clear, with `undefined`) the bearer token used for requests. */
  public setToken(token: string | undefined) {
    this.token = token;
  }

  /** Set (or clear, with `undefined`) the API key used for requests. */
  public setApiKey(apiKey: string | undefined) {
    this.apiKey = apiKey;
  }

  /** Read the CSRF token from the `__Host-forge_csrf` (or dev-mode `forge_csrf`) cookie. */
  private getCSRFToken(): string | null {
    if (typeof document === 'undefined') return null;
    const match = document.cookie.match(/(?:^|;\s*)(?:__Host-)?forge_csrf=([^;]+)/);
    return match ? decodeURIComponent(match[1]) : null;
  }

  private async readErrorBody(response: Response): Promise<unknown> {
    try {
      const text = await response.text();
      if (!text) return undefined;
      try {
        return JSON.parse(text);
      } catch {
        return text;
      }
    } catch {
      return undefined;
    }
  }

  private async request<T>(
    endpoint: string,
    options: RequestInit = {},
    opts: { raw?: boolean } = {}
  ): Promise<T> {
    const url = `${this.baseUrl}${endpoint.startsWith('/') ? endpoint : `/${endpoint}`}`;
    const method = (options.method ?? 'GET').toUpperCase();

    const headers: Record<string, string> = {
      Accept: 'application/json',
      ...this.defaultHeaders,
      ...normalizeHeaders(options.headers),
    };
    const hasContentType = Object.keys(headers).some((k) => k.toLowerCase() === 'content-type');
    if (!hasContentType && !['GET', 'HEAD'].includes(method) && !opts.raw) {
      headers['Content-Type'] = 'application/json';
    }

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
      let timedOut = false;
      const timer = setTimeout(() => {
        timedOut = true;
        controller.abort(new Error('Request timeout'));
      }, this.timeoutMs);

      const requestInit: RequestInit = {
        ...options,
        method,
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

        if (!response.ok) {
          if (isRetryable && RETRYABLE_STATUSES.includes(response.status)) {
            if (attempt < maxAttempts - 1) {
              await new Promise((resolve) => setTimeout(resolve, retryDelayMs(response.headers.get('retry-after'), attempt)));
              continue;
            }
          }
          const data = await this.readErrorBody(response);
          throw new ApiError(response.status, response.statusText, data);
        }

        if (response.status === 204) {
          return {} as T;
        }

        if (opts.raw) {
          return (await response.text()) as unknown as T;
        }

        const text = await response.text();
        if (!text) {
          return {} as T;
        }
        try {
          return JSON.parse(text) as T;
        } catch {
          return text as unknown as T;
        }
      } catch (err) {
        if (err instanceof ApiError) throw err;
        if (options.signal?.aborted) throw err;
        if (isAbortError(err)) throw err;
        if (timedOut && isRetryable && attempt < maxAttempts - 1) {
          await new Promise((resolve) => setTimeout(resolve, retryDelayMs(null, attempt)));
          continue;
        }
        if (isRetryable && attempt < maxAttempts - 1) {
          await new Promise((resolve) => setTimeout(resolve, retryDelayMs(null, attempt)));
          continue;
        }
        lastError = err;
        break;
      } finally {
        clearTimeout(timer);
        controller.abort();
      }
    }

    throw lastError instanceof Error
      ? lastError
      : new ApiError(0, 'Unknown error');
  }

  // -------------------------------------------------------------------------
  // Auth & System
  // -------------------------------------------------------------------------

  /** Check whether the panel still needs initial setup. `GET /setup/status`. */
  public async getSetupStatus(): Promise<ApiSetupStatus> {
    return this.request<ApiSetupStatus>('/setup/status');
  }

  /** Run initial panel setup with an admin account. `POST /setup`. */
  public async setup(input: SetupRequest): Promise<SetupResponse> {
    return this.request<SetupResponse>('/setup', {
      method: 'POST',
      body: JSON.stringify(input),
    });
  }

  /**
   * Log in with email/password. Authentication is cookie-session only: the
   * server sets HttpOnly session/CSRF cookies, so pass `useCookies: true`.
   * When 2FA is required, `confirmationToken` is set — call `loginCheckpoint()`. `POST /auth/login`.
   */
  public async login(credentials: { email: string; password: string }): Promise<LoginResponse> {
    return this.request<LoginResponse>('/auth/login', {
      method: 'POST',
      body: JSON.stringify(credentials),
    });
  }

  /** Complete a 2FA-protected login with the token returned by `login()`. `POST /auth/login/checkpoint`. */
  public async loginCheckpoint(input: LoginCheckpointInput): Promise<LoginResponse> {
    return this.request<LoginResponse>('/auth/login/checkpoint', {
      method: 'POST',
      body: JSON.stringify(input),
    });
  }

  /** Get the current session's user. `GET /auth/me`. */
  public async me(): Promise<ApiUser> {
    return this.request<ApiUser>('/auth/me');
  }

  /** Log out and revoke the current session. `POST /auth/logout`. */
  public async logout(): Promise<void> {
    return this.request<void>('/auth/logout', { method: 'POST' });
  }

  /** Re-issue the session cookie with a new expiry. `POST /auth/session/refresh`. */
  public async refreshSession(): Promise<void> {
    return this.request<void>('/auth/session/refresh', { method: 'POST' });
  }

  /** Get the overall health report. `GET /health`. */
  public async getHealth(): Promise<ApiHealthReport> {
    return this.request<ApiHealthReport>('/health');
  }

  /** Get public panel settings (no auth required). `GET /panel/settings/public`. */
  public async getPublicSettings(): Promise<ApiPublicPanelSettings> {
    return this.request<ApiPublicPanelSettings>('/panel/settings/public');
  }

  /** Get full panel settings (admin). `GET /admin/settings`. */
  public async getSettings(): Promise<ApiPanelSettings> {
    return this.request<ApiPanelSettings>('/admin/settings');
  }

  /** Update panel settings (admin). `PUT /admin/settings`. */
  public async updateSettings(settings: Partial<ApiPanelSettings>): Promise<ApiPanelSettings> {
    return this.request<ApiPanelSettings>('/admin/settings', {
      method: 'PUT',
      body: JSON.stringify(settings),
    });
  }

  // -------------------------------------------------------------------------
  // Users
  // -------------------------------------------------------------------------

  /** List all users (plain array). `GET /users`. */
  public async listUsers(): Promise<ApiUser[]> {
    return this.request<ApiUser[]>('/users');
  }

  /** Get a single user. `GET /users/:id`. */
  public async getUser(id: string): Promise<ApiUser> {
    return this.request<ApiUser>(`/users/${id}`);
  }

  /** Create a user (admin). `POST /users`. */
  public async createUser(user: UserCreateInput): Promise<ApiUser> {
    return this.request<ApiUser>('/users', {
      method: 'POST',
      body: JSON.stringify(user),
    });
  }

  /** Update a user (admin). `PATCH /users/:id`. */
  public async updateUser(id: string, user: Partial<ApiUser>): Promise<ApiUser> {
    return this.request<ApiUser>(`/users/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(user),
    });
  }

  /** Delete a user (admin). Returns `{ ok: true }`. `DELETE /users/:id`. */
  public async deleteUser(id: string): Promise<OkResponse> {
    return this.request<OkResponse>(`/users/${id}`, {
      method: 'DELETE',
    });
  }

  // -------------------------------------------------------------------------
  // Servers
  // -------------------------------------------------------------------------

  /** List servers (paginated; unwraps the `data` array). `GET /servers`. */
  public async listServers(): Promise<ApiServer[]> {
    const envelope = await this.request<PaginatedEnvelope<ApiServer>>('/servers');
    return envelope.data ?? [];
  }

  /** Get a single server. `GET /servers/:id`. */
  public async getServer(id: string): Promise<ApiServer> {
    return this.request<ApiServer>(`/servers/${id}`);
  }

  /** Create a server (admin). `POST /servers`. */
  public async createServer(server: ServerCreateInput): Promise<ApiServer> {
    return this.request<ApiServer>('/servers', {
      method: 'POST',
      body: JSON.stringify(server),
    });
  }

  /** Update a server. `PATCH /servers/:id`. */
  public async updateServer(id: string, server: ServerUpdateInput): Promise<ApiServer> {
    return this.request<ApiServer>(`/servers/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(server),
    });
  }

  /** Delete a server (admin). `DELETE /servers/:id`. */
  public async deleteServer(id: string): Promise<void> {
    return this.request<void>(`/servers/${id}`, {
      method: 'DELETE',
    });
  }

  /** Send a power action (start/stop/restart/kill). `POST /servers/:id/power`. */
  public async sendPowerAction(serverId: string, signal: 'start' | 'stop' | 'restart' | 'kill'): Promise<void> {
    return this.request<void>(`/servers/${serverId}/power`, {
      method: 'POST',
      body: JSON.stringify({ signal }),
    });
  }

  /** Send a console command to a server. `POST /servers/:id/command`. */
  public async sendCommand(serverId: string, command: string): Promise<void> {
    return this.request<void>(`/servers/${serverId}/command`, {
      method: 'POST',
      body: JSON.stringify({ command }),
    });
  }

  /** Get live resource usage stats. `GET /servers/:id/stats`. */
  public async getServerStats(serverId: string): Promise<ApiStats> {
    return this.request<ApiStats>(`/servers/${serverId}/stats`);
  }

  // -------------------------------------------------------------------------
  // Files
  // -------------------------------------------------------------------------

  /** List files in a directory. `GET /servers/:id/files?path=`. */
  public async listFiles(serverId: string, path = '/'): Promise<ApiFileEntry[]> {
    return this.request<ApiFileEntry[]>(`/servers/${serverId}/files?path=${encodeURIComponent(path)}`);
  }

  /** Read a file's raw text content. `GET /servers/:id/files/content?path=`. */
  public async readFile(serverId: string, file: string): Promise<string> {
    return this.request<string>(`/servers/${serverId}/files/content?path=${encodeURIComponent(file)}`, {}, { raw: true });
  }

  /** Write raw string content to a file. `PUT /servers/:id/files/content?path=`. */
  public async writeFile(serverId: string, file: string, content: string): Promise<void> {
    return this.request<void>(`/servers/${serverId}/files/content?path=${encodeURIComponent(file)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'text/plain; charset=utf-8' },
      body: content,
    });
  }

  // -------------------------------------------------------------------------
  // Nodes
  // -------------------------------------------------------------------------

  /** List nodes (paginated; unwraps the `data` array). `GET /nodes`. */
  public async listNodes(): Promise<ApiNode[]> {
    const envelope = await this.request<PaginatedEnvelope<ApiNode>>('/nodes');
    return envelope.data ?? [];
  }

  /** Get a single node. `GET /nodes/:id`. */
  public async getNode(id: string): Promise<ApiNode> {
    return this.request<ApiNode>(`/nodes/${id}`);
  }

  /** Create a node (admin). Returns the node plus its one-time registration token. `POST /nodes`. */
  public async createNode(node: CreateNodeInput): Promise<CreateNodeResult> {
    return this.request<CreateNodeResult>('/nodes', {
      method: 'POST',
      body: JSON.stringify(node),
    });
  }

  /** Update a node (admin). `PATCH /nodes/:id`. */
  public async updateNode(id: string, node: UpdateNodeInput): Promise<ApiNode> {
    return this.request<ApiNode>(`/nodes/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(node),
    });
  }

  /** Delete a node (admin). `DELETE /nodes/:id`. */
  public async deleteNode(id: string): Promise<void> {
    return this.request<void>(`/nodes/${id}`, {
      method: 'DELETE',
    });
  }

  // -------------------------------------------------------------------------
  // Allocations
  // -------------------------------------------------------------------------

  /** List allocations for a node. `GET /nodes/:nodeId/allocations`. */
  public async listAllocations(nodeId: string): Promise<ApiAllocation[]> {
    return this.request<ApiAllocation[]>(`/nodes/${nodeId}/allocations`);
  }

  /** Create allocations (admin, `nodeId` in body). `POST /allocations`. */
  public async createAllocation(allocation: CreateAllocationInput): Promise<ApiAllocation[]> {
    return this.request<ApiAllocation[]>('/allocations', {
      method: 'POST',
      body: JSON.stringify(allocation),
    });
  }

  /** Delete an allocation (admin). `DELETE /allocations/:allocationId`. */
  public async deleteAllocation(allocationId: string): Promise<void> {
    return this.request<void>(`/allocations/${allocationId}`, {
      method: 'DELETE',
    });
  }

  // -------------------------------------------------------------------------
  // Databases
  // -------------------------------------------------------------------------

  /** List databases of a server. `GET /servers/:id/databases`. */
  public async listDatabases(serverId: string): Promise<ApiDatabase[]> {
    return this.request<ApiDatabase[]>(`/servers/${serverId}/databases`);
  }

  /** Create a database on a server. `POST /servers/:id/databases`. */
  public async createDatabase(serverId: string, input: DatabaseCreateInput): Promise<ApiDatabase> {
    return this.request<ApiDatabase>(`/servers/${serverId}/databases`, {
      method: 'POST',
      body: JSON.stringify(input),
    });
  }

  /** Delete a database from a server. `DELETE /servers/:id/databases/:databaseId`. */
  public async deleteDatabase(serverId: string, databaseId: string): Promise<void> {
    return this.request<void>(`/servers/${serverId}/databases/${databaseId}`, {
      method: 'DELETE',
    });
  }

  /** List database hosts (admin). `GET /database-hosts`. */
  public async listDatabaseHosts(): Promise<ApiDatabaseHost[]> {
    return this.request<ApiDatabaseHost[]>('/database-hosts');
  }

  /** Create a database host (admin). `POST /database-hosts`. */
  public async createDatabaseHost(host: CreateDatabaseHostInput): Promise<ApiDatabaseHost> {
    return this.request<ApiDatabaseHost>('/database-hosts', {
      method: 'POST',
      body: JSON.stringify(host),
    });
  }

  // -------------------------------------------------------------------------
  // Schedules
  // -------------------------------------------------------------------------

  /** List schedules of a server. `GET /servers/:id/schedules`. */
  public async listSchedules(serverId: string): Promise<ApiSchedule[]> {
    return this.request<ApiSchedule[]>(`/servers/${serverId}/schedules`);
  }

  /** Create a schedule on a server. `POST /servers/:id/schedules`. */
  public async createSchedule(serverId: string, schedule: ScheduleCreateInput): Promise<ApiSchedule> {
    return this.request<ApiSchedule>(`/servers/${serverId}/schedules`, {
      method: 'POST',
      body: JSON.stringify(schedule),
    });
  }

  /** Update a schedule. `PATCH /servers/:id/schedules/:scheduleId`. */
  public async updateSchedule(serverId: string, scheduleId: string, schedule: ScheduleUpdateInput): Promise<ApiSchedule> {
    return this.request<ApiSchedule>(`/servers/${serverId}/schedules/${scheduleId}`, {
      method: 'PATCH',
      body: JSON.stringify(schedule),
    });
  }

  /** Delete a schedule. `DELETE /servers/:id/schedules/:scheduleId`. */
  public async deleteSchedule(serverId: string, scheduleId: string): Promise<void> {
    return this.request<void>(`/servers/${serverId}/schedules/${scheduleId}`, {
      method: 'DELETE',
    });
  }

  /** Create a task on a schedule; the API only accepts `timeOffsetSeconds`. `POST /servers/:id/schedules/:scheduleId/tasks`. */
  public async createScheduleTask(serverId: string, scheduleId: string, timeOffsetSeconds: number): Promise<ApiScheduleTask> {
    return this.request<ApiScheduleTask>(`/servers/${serverId}/schedules/${scheduleId}/tasks`, {
      method: 'POST',
      body: JSON.stringify({ timeOffsetSeconds }),
    });
  }

  // -------------------------------------------------------------------------
  // Backups
  // -------------------------------------------------------------------------

  /** List backups with pagination metadata. `GET /servers/:id/backups`. */
  public async listBackups(serverId: string): Promise<BackupListResponse> {
    return this.request<BackupListResponse>(`/servers/${serverId}/backups`);
  }

  /** Create a backup; the API only accepts an optional `ignored` file list. `POST /servers/:id/backups`. */
  public async createBackup(serverId: string, input?: Pick<BackupCreateInput, 'ignored'>): Promise<ApiBackup> {
    return this.request<ApiBackup>(`/servers/${serverId}/backups`, {
      method: 'POST',
      body: JSON.stringify({ ignored: input?.ignored ?? [] }),
    });
  }

  /** Delete a backup by its NAME (not uuid). `DELETE /servers/:id/backups/:backupName`. */
  public async deleteBackup(serverId: string, backupName: string): Promise<OkResponse> {
    return this.request<OkResponse>(`/servers/${serverId}/backups/${encodeURIComponent(backupName)}`, {
      method: 'DELETE',
    });
  }

  // -------------------------------------------------------------------------
  // Mounts
  // -------------------------------------------------------------------------

  /** List mounts (admin). `GET /mounts`. */
  public async listMounts(): Promise<ApiMount[]> {
    return this.request<ApiMount[]>('/mounts');
  }

  /** Create a mount (admin). `POST /mounts`. */
  public async createMount(mount: CreateMountInput): Promise<ApiMount> {
    return this.request<ApiMount>('/mounts', {
      method: 'POST',
      body: JSON.stringify(mount),
    });
  }

  // -------------------------------------------------------------------------
  // Nests & Eggs
  // -------------------------------------------------------------------------

  /** List nests (admin). `GET /nests`. */
  public async listNests(): Promise<ApiNest[]> {
    return this.request<ApiNest[]>('/nests');
  }

  /** List eggs in a nest (admin). `GET /nests/:nestId/eggs`. */
  public async listEggs(nestId: string): Promise<ApiEgg[]> {
    return this.request<ApiEgg[]>(`/nests/${nestId}/eggs`);
  }

  /** Get a single egg (admin). `GET /eggs/:id`. */
  public async getEgg(eggId: string): Promise<ApiEgg> {
    return this.request<ApiEgg>(`/eggs/${eggId}`);
  }

  /** Create an egg (admin; `nestId` in body). `POST /eggs`. */
  public async createEgg(egg: CreateEggInput): Promise<ApiEgg> {
    return this.request<ApiEgg>('/eggs', {
      method: 'POST',
      body: JSON.stringify(egg),
    });
  }
}

/** Create a ForgeApiClient. See `ApiClientConfig` for options. */
export function createApiClient(config: ApiClientConfig): ForgeApiClient {
  return new ForgeApiClient(config);
}
