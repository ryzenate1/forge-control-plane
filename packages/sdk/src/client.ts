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
  /**
   * Called once per 401 response (session expired / not authenticated).
   * Mirrors the web client's `forge:session-expired` signal
   * (`notifySessionExpired` in `forge/web/lib/api/http.ts`): the SDK has no
   * router, so the host app supplies the redirect/clearing behaviour here.
   */
  onUnauthorized?: (err: ApiError) => void;
  /**
   * Default idempotency key for mutations. When set, every
   * POST/PUT/PATCH/DELETE automatically carries an `Idempotency-Key` header
   * unless the individual call supplies one. Mutations are never retried by
   * default (see retry policy on `request`); an idempotency key is what makes
   * an explicit retry safe.
   */
  idempotencyKey?: string;
}

/**
 * Error thrown for non-2xx API responses.
 *
 * Shape mirrors the web client's `ApiError` (`forge/web/lib/api/http.ts`):
 * `message` is human-readable, `status` is the HTTP status (0 when no
 * response was received), `statusText` the reason phrase, `data` the parsed
 * body, and `details` the structured validation errors of a 422 response
 * (`{ errors }`, `{ details }` or `{ fields }` in the body, when present).
 *
 * Constructor order differs from the web client on purpose: the SDK keeps
 * `(status, statusText, data, details)` for backward compatibility, while the
 * web client uses `(message, status, details)`. Both derive `message` from the
 * body (`message`/`error` keys, else raw text, else status line) and both
 * extract `details` with the same `{ details }` → `{ errors }` → `{ fields }`
 * precedence — see `ApiError.buildMessage` / `extractDetails` here and
 * `getErrorMessage` / `readErrorDetails` in the web client.
 */
export class ApiError extends Error {
  /** Structured validation errors from a 422 response body, when present. */
  public details?: unknown;

  constructor(
    /** HTTP status code of the response (0 when no response was received). */
    public status: number,
    /** HTTP status text of the response. */
    public statusText: string,
    /** Parsed response body (JSON object, or raw text when the body was not JSON). */
    public data?: unknown,
    details?: unknown,
  ) {
    super(ApiError.buildMessage(status, statusText, data));
    this.name = 'ApiError';
    this.details = details ?? ApiError.extractDetails(data);
  }

  private static buildMessage(status: number, statusText: string, data: unknown): string {
    if (data !== null && typeof data === 'object') {
      const body = data as Record<string, unknown>;
      const msg = body['message'] ?? body['error'];
      if (typeof msg === 'string' && msg.length > 0) return msg;
    }
    if (typeof data === 'string' && data.length > 0) return data.slice(0, 300);
    return `API Error ${status}: ${statusText}`;
  }

  /** Pull structured 422 validation details out of a parsed error body. */
  private static extractDetails(data: unknown): unknown {
    if (data !== null && typeof data === 'object') {
      const body = data as Record<string, unknown>;
      if (body['details'] !== undefined) return body['details'];
      if (body['errors'] !== undefined) return body['errors'];
      if (body['fields'] !== undefined) return body['fields'];
    }
    return undefined;
  }
}

/** Type guard for `ApiError` (same contract as the web client's `isApiError`). */
export function isApiError(err: unknown): err is ApiError {
  return err instanceof ApiError || (err instanceof Error && err.name === 'ApiError');
}

/**
 * Unwrap a list payload that must be a bare array or a `{ data: T[] }`
 * envelope. `null`/`undefined` unwrap to `[]`, but any other unexpected shape
 * throws — silently presenting "no data" for a shape the backend never emits
 * hides contract drift as an empty list. Same strictness as the web client's
 * `unwrapList` (`forge/web/lib/api/http.ts`).
 */
export function unwrapList<T>(value: T[] | { data?: T[] } | undefined | null): T[] {
  if (value == null) return [];
  if (Array.isArray(value)) return value;
  if (typeof value === 'object' && Array.isArray((value as { data?: unknown }).data)) {
    return (value as { data: T[] }).data;
  }
  // Never silently present "no data" for a shape the backend never emits: a
  // non-empty unexpected body means the contract drifted and must surface,
  // not render as an empty list. An empty object is not an empty list
  // either — returning [] for `{}` would hide the drift the same way.
  throw new Error('Unexpected response: expected an array or { data: [...] }');
}

/** @deprecated Use `unwrapList` instead (matches the web client's naming). */
export function unwrapData<T>(value: T[] | { data?: T[] } | undefined | null): T[] {
  return unwrapList(value);
}

/**
 * Unwrap a single-object payload that may be `{ data: T }` or a bare `T`.
 * Same semantics as the web client's `unwrapData` (`forge/web/lib/api/http.ts`).
 */
export function unwrapSingleData<T>(value: { data: T } | T | undefined | null): T | undefined {
  if (value !== null && typeof value === 'object' && 'data' in (value as Record<string, unknown>)) {
    return (value as { data: T }).data;
  }
  return (value ?? undefined) as T | undefined;
}

/** Body accepted by `POST /setup` (initial panel setup). Alias of `ApiSetupRequest` from `@forge/shared-types` (only `email` and `password` are required). */
export type SetupRequest = import('@forge/shared-types').ApiSetupRequest;

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

/** Response of `GET /servers/:id/backups`. Canonical alias of `BackupListResponse` from `@forge/shared-types` (flat `{ data, pagination }`, not `PaginatedResponse`). */
export type BackupListResponse = import('@forge/shared-types').BackupListResponse;

/** Generic `{ "ok": true }` body returned by several DELETE endpoints. */
export type OkResponse = {
  ok: boolean;
};

// ---- Pipeline (CI/CD) ----

export type PipelineStage = {
  name: string;
  action: string;
  config?: Record<string, unknown>;
  timeoutSec?: number;
  continueOnFailure?: boolean;
};

export type PipelineDefinition = {
  id: string;
  name: string;
  description?: string;
  categories?: string[];
  stages: PipelineStage[];
  trigger?: { type: string; cron?: string; enabled?: boolean };
  createdBy?: string;
  createdAt?: string;
  updatedAt?: string;
};

export type PipelineRun = {
  id: string;
  pipelineId: string;
  pipelineName?: string;
  trigger: string;
  status: string;
  progressPct?: number;
  currentStage?: string;
  error?: string;
  retryOf?: string | null;
  retryCount?: number;
  requestedBy?: string;
  createdAt: string;
  startedAt?: string | null;
  finishedAt?: string | null;
  stages?: unknown[];
};

export type PipelineLogEntry = {
  id: number;
  runId: string;
  stageId?: string;
  level: string;
  message: string;
  timestamp: string;
};

export type CreatePipelineInput = {
  name: string;
  description?: string;
  categories?: string[];
  stages: PipelineStage[];
  trigger?: { type: string; cron?: string; enabled?: boolean };
};

// ---- Billing (admin) ----

export type BillingPlan = {
  id: string;
  code: string;
  name: string;
  centsPerMonth: number;
  entitlements: unknown;
  trialDays: number;
  active: boolean;
  createdAt: string;
  updatedAt: string;
};

export type OrgQuota = {
  orgId: string;
  planCode: string;
  trialUntil?: string | null;
  memoryUsageBytes: number;
  serversCount: number;
  environmentsCount: number;
  nodesCount: number;
  storageBytes: number;
  prevPlanCode: string;
  quotaUpdatedAt: string;
};

export type BillingUsageSummary = {
  orgId: string;
  plan: string;
  servers: number;
  memoryBytes: number;
  storageBytes: number;
  environments: number;
  nodes: number;
  meterEvents: number;
};

// ---- Placement (env-affinity, admin) ----

export type PlacementRequest = {
  serverId?: string;
  regionId?: string;
  nodeId?: string;
  cpu?: number;
  memoryMb?: number;
  diskMb?: number;
};

export type PlacementExplainResult = {
  nodeId: string;
  requestedEnv?: string;
  nodeEnvGroups: string[];
  nodeLabels: Record<string, string>;
  constraints: unknown[];
  matchedLabels: string[];
  missingLabels: string[];
  isCandidate: boolean;
};

// ---- Fencing (admin) ----

export type FencePreviewRow = {
  id: string;
  name: string;
  status: string;
  generation: number;
};

export type FenceResult = {
  nodeId: string;
  fenced: boolean;
  serverCount: number;
  serverIds?: string[];
  reason?: string;
};

// ---- Platform upgrade (admin) ----

export type UpgradeVersionInfo = {
  component: string;
  current: string;
  latest: string;
  upgradable: boolean;
};

export type UpgradePlan = {
  id: string;
  type: string;
  fromVersion: string;
  toVersion: string;
  components: string[];
  status: string;
  progress: number;
  totalSteps: number;
  currentStep: string;
  error?: string;
  backupPath?: string;
  startedAt: string;
  completedAt?: string | null;
  createdAt: string;
  updatedAt: string;
};

export type UpgradeResult = {
  success: boolean;
  message: string;
  upgradePlan?: UpgradePlan;
  versionInfo?: UpgradeVersionInfo[];
  error?: string;
};

function normalizeBaseUrl(baseUrl: string): string {
  let url = baseUrl;
  while (url.endsWith('/')) {
    url = url.slice(0, -1);
  }
  if (!url.endsWith('/api/v1')) {
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
  private onUnauthorized?: (err: ApiError) => void;
  private idempotencyKey?: string;

  constructor(config: ApiClientConfig) {
    this.baseUrl = normalizeBaseUrl(config.baseUrl);
    this.token = config.token;
    this.apiKey = config.apiKey;
    this.fetcher = config.fetch || globalThis.fetch.bind(globalThis);
    this.defaultHeaders = normalizeHeaders(config.headers);
    this.useCookies = config.useCookies ?? false;
    this.timeoutMs = config.timeoutMs ?? 30000;
    this.onUnauthorized = config.onUnauthorized;
    this.idempotencyKey = config.idempotencyKey;
  }

  /** Set (or clear, with `undefined`) the 401 hook. See `ApiClientConfig.onUnauthorized`. */
  public setOnUnauthorized(handler: ((err: ApiError) => void) | undefined) {
    this.onUnauthorized = handler;
  }

  /** Set (or clear, with `undefined`) the bearer token used for requests. */
  public setToken(token: string | undefined) {
    this.token = token;
  }

  /** Set (or clear, with `undefined`) the API key used for requests. */
  public setApiKey(apiKey: string | undefined) {
    this.apiKey = apiKey;
  }

  /** Read the CSRF token from the `__Host-forge_csrf` (or dev-mode `forge_csrf`) cookie. Same contract as `getCSRFToken` in `forge/web/lib/api/http.ts`: both names are anchored to a cookie boundary so `evil__Host-forge_csrf` can never match. */
  private getCSRFToken(): string | null {
    if (typeof document === 'undefined') return null;
    const match =
      document.cookie.match(/(?:^|;\s*)__Host-forge_csrf=([^;]+)/) ??
      document.cookie.match(/(?:^|;\s*)forge_csrf=([^;]+)/);
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

  /**
   * Core request pipeline.
   *
   * Retry policy: only idempotent `GET` requests are retried (up to 3 attempts
   * on 429/502/503/504 with `Retry-After` support, plus transient transport
   * failures). This is the same transient set the web client retries when
   * called with `{ retry: true }` (`forge/web/lib/api/http.ts`), except the
   * SDK retries GETs by default while the web client is opt-in per call.
   * Mutations (`POST`/`PUT`/`PATCH`/`DELETE`) are executed exactly
   * once: automatically retrying a non-idempotent write can double-apply it
   * (duplicate server, double charge). To make a mutation safely retryable,
   * pass an idempotency key — via `opts.idempotencyKey` or the client-level
   * `ApiClientConfig.idempotencyKey` — which is sent as the `Idempotency-Key`
   * header so the server can de-duplicate repeated submissions.
   */
  private async request<T>(
    endpoint: string,
    options: RequestInit = {},
    opts: { raw?: boolean; idempotencyKey?: string } = {},
  ): Promise<T> {
    const url = `${this.baseUrl}${endpoint.startsWith('/') ? endpoint : `/${endpoint}`}`;
    const method = (options.method ?? 'GET').toUpperCase();
    const isMutation = ['POST', 'PUT', 'PATCH', 'DELETE'].includes(method);

    const headers: Record<string, string> = {
      Accept: 'application/json',
      ...this.defaultHeaders,
      ...normalizeHeaders(options.headers),
    };
    const hasContentType = Object.keys(headers).some((k) => k.toLowerCase() === 'content-type');
    if (!hasContentType && !['GET', 'HEAD'].includes(method) && !opts.raw) {
      headers['Content-Type'] = 'application/json';
    }

    const idempotencyKey = opts.idempotencyKey ?? this.idempotencyKey;
    if (
      isMutation &&
      idempotencyKey &&
      !Object.keys(headers).some((k) => k.toLowerCase() === 'idempotency-key')
    ) {
      headers['Idempotency-Key'] = idempotencyKey;
    }

    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`;
    } else if (this.apiKey) {
      headers['X-API-Key'] = this.apiKey;
    }

    if (this.useCookies && isMutation) {
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
              await new Promise((resolve) =>
                setTimeout(resolve, retryDelayMs(response.headers.get('retry-after'), attempt)),
              );
              continue;
            }
          }
          const data = await this.readErrorBody(response);
          const apiError = new ApiError(response.status, response.statusText, data);
          if (response.status === 401) this.onUnauthorized?.(apiError);
          throw apiError;
        }

        if (response.status === 204) {
          return undefined as T;
        }

        if (opts.raw) {
          return (await response.text()) as unknown as T;
        }

        const text = await response.text();
        if (!text) {
          return undefined as T;
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

    throw lastError instanceof Error ? lastError : new ApiError(0, 'Unknown error');
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
    const body = await this.request<PaginatedEnvelope<ApiServer> | ApiServer[]>('/servers');
    return unwrapList(body);
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
  public async sendPowerAction(
    serverId: string,
    signal: 'start' | 'stop' | 'restart' | 'kill',
  ): Promise<void> {
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
    return this.request<ApiFileEntry[]>(
      `/servers/${serverId}/files?path=${encodeURIComponent(path)}`,
    );
  }

  /** Read a file's raw text content. `GET /servers/:id/files/content?path=`. */
  public async readFile(serverId: string, file: string): Promise<string> {
    return this.request<string>(
      `/servers/${serverId}/files/content?path=${encodeURIComponent(file)}`,
      {},
      { raw: true },
    );
  }

  /** Write raw string content to a file. `PUT /servers/:id/files/content?path=`. */
  public async writeFile(serverId: string, file: string, content: string): Promise<void> {
    return this.request<void>(
      `/servers/${serverId}/files/content?path=${encodeURIComponent(file)}`,
      {
        method: 'PUT',
        headers: { 'Content-Type': 'text/plain; charset=utf-8' },
        body: content,
      },
    );
  }

  // -------------------------------------------------------------------------
  // Nodes
  // -------------------------------------------------------------------------

  /** List nodes (paginated; unwraps the `data` array). `GET /nodes`. */
  public async listNodes(): Promise<ApiNode[]> {
    const body = await this.request<PaginatedEnvelope<ApiNode> | ApiNode[]>('/nodes');
    return unwrapList(body);
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

  /** Create allocations (admin, `nodeId` in body; `port` or `ports` required). `POST /allocations`. */
  public async createAllocation(allocation: CreateAllocationInput): Promise<ApiAllocation[]> {
    if (allocation.port == null && (allocation.ports ?? '').trim() === '') {
      throw new ApiError(400, 'Bad Request', {
        message: 'port or ports is required',
      });
    }
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
  public async createSchedule(
    serverId: string,
    schedule: ScheduleCreateInput,
  ): Promise<ApiSchedule> {
    return this.request<ApiSchedule>(`/servers/${serverId}/schedules`, {
      method: 'POST',
      body: JSON.stringify(schedule),
    });
  }

  /** Update a schedule. `PATCH /servers/:id/schedules/:scheduleId`. */
  public async updateSchedule(
    serverId: string,
    scheduleId: string,
    schedule: ScheduleUpdateInput,
  ): Promise<ApiSchedule> {
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

  /**
   * Create a task on a schedule. `action` is required server-side (the store
   * 400s on a missing/unsupported action), so the full
   * `ScheduleTaskCreateInput` is accepted and forwarded — never a bare
   * `timeOffsetSeconds`, which can never succeed on its own.
   * `POST /servers/:id/schedules/:scheduleId/tasks`.
   */
  public async createScheduleTask(
    serverId: string,
    scheduleId: string,
    input: ScheduleTaskCreateInput,
  ): Promise<ApiScheduleTask> {
    return this.request<ApiScheduleTask>(`/servers/${serverId}/schedules/${scheduleId}/tasks`, {
      method: 'POST',
      body: JSON.stringify(input),
    });
  }

  // -------------------------------------------------------------------------
  // Backups
  // -------------------------------------------------------------------------

  /** List backups with pagination metadata. `GET /servers/:id/backups`. */
  public async listBackups(serverId: string): Promise<BackupListResponse> {
    return this.request<BackupListResponse>(`/servers/${serverId}/backups`);
  }

  /**
   * Create a backup. The server only honors the `ignored` file list (it
   * generates the backup name itself; see `BackupCreateInput`), so the full
   * input is forwarded as-is and nothing is silently dropped.
   * `POST /servers/:id/backups`.
   */
  public async createBackup(serverId: string, input?: BackupCreateInput): Promise<ApiBackup> {
    return this.request<ApiBackup>(`/servers/${serverId}/backups`, {
      method: 'POST',
      body: JSON.stringify(input ?? {}),
    });
  }

  /** Delete a backup by its NAME (not uuid). `DELETE /servers/:id/backups/:backupName`. */
  public async deleteBackup(serverId: string, backupName: string): Promise<OkResponse> {
    return this.request<OkResponse>(
      `/servers/${serverId}/backups/${encodeURIComponent(backupName)}`,
      {
        method: 'DELETE',
      },
    );
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

  // -------------------------------------------------------------------------
  // Pipelines (CI/CD)
  // -------------------------------------------------------------------------

  /** List pipeline definitions (admin). `GET /pipelines`. */
  public async listPipelines(): Promise<PipelineDefinition[]> {
    const body = await this.request<PaginatedEnvelope<PipelineDefinition> | PipelineDefinition[]>(
      '/pipelines',
    );
    return unwrapList(body);
  }

  /** Get a single pipeline definition (admin). `GET /pipelines/:id`. */
  public async getPipeline(id: string): Promise<PipelineDefinition> {
    const envelope = await this.request<{ data: PipelineDefinition }>(`/pipelines/${id}`);
    return envelope.data;
  }

  /** Create a pipeline definition (admin). `POST /pipelines`. */
  public async createPipeline(input: CreatePipelineInput): Promise<PipelineDefinition> {
    const envelope = await this.request<{ data: PipelineDefinition }>('/pipelines', {
      method: 'POST',
      body: JSON.stringify(input),
    });
    return envelope.data;
  }

  /** Delete a pipeline definition (admin). `DELETE /pipelines/:id`. */
  public async deletePipeline(id: string): Promise<OkResponse> {
    return this.request<OkResponse>(`/pipelines/${id}`, { method: 'DELETE' });
  }

  /** Trigger a manual run of a pipeline (admin). `POST /pipelines/:id/runs`. */
  public async triggerPipeline(id: string, trigger = 'manual'): Promise<PipelineRun> {
    const envelope = await this.request<{ data: PipelineRun }>(`/pipelines/${id}/runs`, {
      method: 'POST',
      body: JSON.stringify({ trigger }),
    });
    return envelope.data;
  }

  /** List pipeline runs, optionally filtered (admin). `GET /pipeline-runs`. */
  public async listPipelineRuns(opts?: {
    pipelineId?: string;
    status?: string;
  }): Promise<PipelineRun[]> {
    const q = new URLSearchParams();
    if (opts?.pipelineId) q.set('pipelineId', opts.pipelineId);
    if (opts?.status) q.set('status', opts.status);
    const suffix = q.toString() ? `?${q.toString()}` : '';
    const body = await this.request<PaginatedEnvelope<PipelineRun> | PipelineRun[]>(
      `/pipeline-runs${suffix}`,
    );
    return unwrapList(body);
  }

  /** Stream logs for a run after a cursor (admin). `GET /pipeline-runs/:id/logs`. */
  public async listPipelineRunLogs(runId: string, after = 0): Promise<PipelineLogEntry[]> {
    const suffix = after > 0 ? `?after=${after}` : '';
    const body = await this.request<{ data: PipelineLogEntry[] } | PipelineLogEntry[]>(
      `/pipeline-runs/${runId}/logs${suffix}`,
    );
    return unwrapList(body);
  }

  /** Cancel a running pipeline (admin). `POST /pipeline-runs/:id/cancel`. */
  public async cancelPipelineRun(runId: string): Promise<OkResponse> {
    return this.request<OkResponse>(`/pipeline-runs/${runId}/cancel`, { method: 'POST' });
  }

  /** Retry a failed/cancelled run (admin). `POST /pipeline-runs/:id/retry`. */
  public async retryPipelineRun(runId: string): Promise<PipelineRun> {
    const envelope = await this.request<{ data: PipelineRun }>(`/pipeline-runs/${runId}/retry`, {
      method: 'POST',
    });
    return envelope.data;
  }

  // -------------------------------------------------------------------------
  // Billing (admin)
  // -------------------------------------------------------------------------

  /** List billing plans (admin). `GET /billing/plans`. */
  public async listBillingPlans(): Promise<BillingPlan[]> {
    const body = await this.request<{ data: BillingPlan[] } | BillingPlan[]>('/billing/plans');
    return unwrapList(body);
  }

  /** Assign an org to a billing plan (admin). `POST /billing/org/:orgId/plan`. */
  public async setOrgBillingPlan(
    orgId: string,
    planCode: string,
    trial = false,
  ): Promise<OrgQuota> {
    const envelope = await this.request<{ data: OrgQuota }>(`/billing/org/${orgId}/plan`, {
      method: 'POST',
      body: JSON.stringify({ planCode, trial }),
    });
    return envelope.data;
  }

  /** Get an org's usage summary (admin). `GET /billing/org/:orgId/usage`. */
  public async getOrgUsage(orgId: string): Promise<BillingUsageSummary> {
    const envelope = await this.request<{ data: BillingUsageSummary }>(
      `/billing/org/${orgId}/usage`,
    );
    return envelope.data;
  }

  // -------------------------------------------------------------------------
  // Placement (env-affinity, admin)
  // -------------------------------------------------------------------------

  /** Explain why a node would or would not host a workload. `POST /placement/explain`. */
  public async explainPlacement(
    nodeId: string,
    place: PlacementRequest,
  ): Promise<PlacementExplainResult> {
    const envelope = await this.request<{ data: PlacementExplainResult }>('/placement/explain', {
      method: 'POST',
      body: JSON.stringify({ nodeId, place }),
    });
    return envelope.data;
  }

  // -------------------------------------------------------------------------
  // Fencing (admin)
  // -------------------------------------------------------------------------

  /** Preview which servers would be fenced on a node. `GET /fencing/nodes/:id/preview`. */
  public async previewFence(nodeId: string): Promise<FencePreviewRow[]> {
    const body = await this.request<{ data: FencePreviewRow[] } | FencePreviewRow[]>(
      `/fencing/nodes/${nodeId}/preview`,
    );
    return unwrapList(body);
  }

  /** Manually fence a node — bumps workload generations for all its servers. `POST /fencing/nodes/:id`. */
  public async fenceNode(nodeId: string): Promise<FenceResult> {
    const envelope = await this.request<{ data: FenceResult }>(`/fencing/nodes/${nodeId}`, {
      method: 'POST',
    });
    return envelope.data;
  }

  // -------------------------------------------------------------------------
  // Platform upgrade (admin)
  // -------------------------------------------------------------------------

  /** Check component versions and upgradability. `GET /upgrade/versions`. */
  public async checkForUpgrades(): Promise<UpgradeVersionInfo[]> {
    const body = await this.request<{ data: UpgradeVersionInfo[] } | UpgradeVersionInfo[]>(
      '/upgrade/versions',
    );
    return unwrapList(body);
  }

  /** List upgrade plans (history). `GET /upgrade/plans`. */
  public async listUpgradePlans(limit = 50): Promise<UpgradePlan[]> {
    const body = await this.request<{ data: UpgradePlan[] } | UpgradePlan[]>(
      `/upgrade/plans?limit=${limit}`,
    );
    return unwrapList(body);
  }

  /** Create an upgrade plan. `POST /upgrade/plans`. */
  public async createUpgradePlan(type: string, components: string[]): Promise<UpgradePlan> {
    const envelope = await this.request<{ data: UpgradePlan }>('/upgrade/plans', {
      method: 'POST',
      body: JSON.stringify({ type, components }),
    });
    return envelope.data;
  }

  /** Execute an upgrade plan (backup → upgrade → verify → rollback-on-fail). `POST /upgrade/plans/:id/execute`. */
  public async executeUpgradePlan(id: string): Promise<UpgradeResult> {
    const envelope = await this.request<{ data: UpgradeResult }>(`/upgrade/plans/${id}/execute`, {
      method: 'POST',
    });
    return envelope.data;
  }

  /** Cancel an in-progress upgrade. `POST /upgrade/plans/:id/cancel`. */
  public async cancelUpgradePlan(id: string): Promise<OkResponse> {
    return this.request<OkResponse>(`/upgrade/plans/${id}/cancel`, { method: 'POST' });
  }
}

/** Create a ForgeApiClient. See `ApiClientConfig` for options. */
export function createApiClient(config: ApiClientConfig): ForgeApiClient {
  return new ForgeApiClient(config);
}
