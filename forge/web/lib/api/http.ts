// HTTP helper functions for API calls

// Resolution order (all client side, so NEXT_PUBLIC_* are inlined at build time):
//   1. runtime override injected without a rebuild, e.g. an inline <script>
//      `window.__FORGE_CONFIG__ = { apiBaseUrl: "https://api.example.com/api/v1" }`
//      placed before the app bundle by the reverse proxy / deployment layer;
//   2. NEXT_PUBLIC_API_URL  (existing convention, see .env.example);
//   3. NEXT_PUBLIC_API_BASE_URL (alias kept for parity with the audit naming);
//   4. same-origin default "/api/v1" so unconfigured deployments still work.
// Trailing slashes are stripped so URL joins never produce "//".
function resolveApiBaseUrl(): string {
  if (typeof window !== 'undefined') {
    const runtime = (window as unknown as { __FORGE_CONFIG__?: { apiBaseUrl?: string } })
      .__FORGE_CONFIG__?.apiBaseUrl;
    if (runtime) {
      const normalizedRuntime = runtime.replace(/\/+$/, '');
      if (normalizedRuntime) return normalizedRuntime;
    }
  }
  const envValue =
    process.env.NEXT_PUBLIC_API_URL ?? process.env.NEXT_PUBLIC_API_BASE_URL ?? '/api/v1';
  const normalized = envValue.replace(/\/+$/, '');
  return normalized || '/api/v1';
}

export function getApiBaseUrl(): string {
  const next = resolveApiBaseUrl();
  // Keep the legacy named export live (ESM live bindings): anyone holding
  // `API_BASE_URL` sees the refreshed value after any request-time lookup.
  API_BASE_URL = next;
  return next;
}

/**
 * Legacy snapshot export. Historically frozen at module load, which made a
 * runtime `window.__FORGE_CONFIG__.apiBaseUrl` override invisible to holders
 * of this value. It is now a live `let` refreshed on every `getApiBaseUrl()`
 * call (and on every request, which resolves via `getApiBaseUrl`). Prefer
 * calling `getApiBaseUrl()` at request time; this remains for
 * backward-compatible imports only.
 */
export let API_BASE_URL = resolveApiBaseUrl();

export class ApiError extends Error {
  /**
   * Structured validation errors from a 422 response body, when present
   * (`{ errors }`, `{ details }` or `{ fields }`). Same contract as the SDK's
   * `ApiError.details` (`packages/sdk/src/client.ts`).
   *
   * Constructor order differs from the SDK on purpose: the web client uses
   * `(message, status, details)` while the SDK keeps the legacy
   * `(status, statusText, data, details)`. Both extract `details` with the
   * same `{ details }` → `{ errors }` → `{ fields }` precedence
   * (`readErrorDetails` here, `ApiError.extractDetails` in the SDK).
   */
  readonly details?: unknown;

  constructor(
    message: string,
    readonly status: number,
    details?: unknown,
  ) {
    super(message);
    this.name = 'ApiError';
    this.details = details;
  }
}

/** Type guard for `ApiError` (same contract as the SDK's `isApiError`). */
export function isApiError(err: unknown): err is ApiError {
  return err instanceof ApiError || (err instanceof Error && err.name === 'ApiError');
}

/**
 * Envelope guards. Backend list routes variously return a bare array or
 * `{ data: [...] }`; single-object routes return the object or `{ data: obj }`.
 * These helpers keep every domain module from re-inventing
 * `as unknown as { data }` casts and from throwing on either shape.
 *
 * Naming is the mirror of the SDK (`packages/sdk/src/client.ts`): `unwrapList`
 * here is `unwrapList` there, and `unwrapData` here is `unwrapSingleData`
 * there. The SDK additionally keeps a deprecated `unwrapData` alias for its
 * list helper — do not import that name without checking which one you mean.
 */
type ListEnvelope<T> = { data?: T[] } | T[];
type DataEnvelope<T> = { data: T } | T;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

export function unwrapList<T>(body: ListEnvelope<T> | null | undefined): T[] {
  if (body == null) return [];
  if (Array.isArray(body)) return body;
  if (isRecord(body) && Array.isArray((body as { data?: unknown }).data)) {
    return (body as { data: T[] }).data;
  }
  // Never silently present "no data" for a shape the backend never emits: a
  // non-empty unexpected body means the contract drifted and must surface,
  // not render as an empty list. An empty object is not an empty list
  // either — returning [] for `{}` would hide the drift the same way.
  throw new Error('Unexpected response: expected an array or { data: [...] }');
}

/**
 * Unwrap a single-object `{ data: T }` envelope (or a bare `T`).
 * Same semantics as the SDK's `unwrapSingleData`.
 */
export function unwrapData<T>(body: DataEnvelope<T>): T {
  if (isRecord(body) && 'data' in body) {
    return (body as { data: T }).data;
  }
  return body as T;
}

export function unwrapNullableData<T>(body: DataEnvelope<T | null> | null | undefined): T | null {
  if (body == null) return null;
  if (isRecord(body) && 'data' in (body as Record<string, unknown>)) {
    return (body as { data: T | null }).data ?? null;
  }
  return body as T;
}

export function getCSRFToken(): string | null {
  if (typeof document === 'undefined') return null;
  // Both patterns are anchored to a cookie boundary so a cookie named e.g.
  // `evil__Host-forge_csrf` or `xforge_csrf` can never satisfy the match.
  const match =
    document.cookie.match(/(?:^|;\s*)__Host-forge_csrf=([^;]+)/) ??
    document.cookie.match(/(?:^|;\s*)forge_csrf=([^;]+)/);
  if (!match) return null;
  // A malformed cookie value (stray `%`) must never throw out of a request
  // path — fall back to the raw value so callers still attempt the request
  // and the backend remains the authority on rejection.
  try {
    return decodeURIComponent(match[1]);
  } catch {
    return match[1] ?? null;
  }
}

function addCSRFToHeaders(headers: Record<string, string>, method: string): void {
  if (!['POST', 'PUT', 'PATCH', 'DELETE'].includes(method.toUpperCase())) return;
  const csrfToken = getCSRFToken();
  if (csrfToken) {
    headers['X-CSRF-Token'] = csrfToken;
  } else if (typeof console !== 'undefined' && typeof document !== 'undefined') {
    // A state-changing request without a CSRF token will be rejected by the
    // API — warn loudly instead of failing silently so the missing cookie is
    // diagnosed at the call site rather than as a bare 403.
    console.warn(
      `[forge] ${method.toUpperCase()} request issued without a CSRF token (no __Host-forge_csrf cookie present)`,
    );
  }
}

let sessionExpiredNotified = false;

/**
 * Signals a 401 to the session layer (components/providers.tsx), which clears
 * the react-query cache and redirects to login. Fires at most once per page
 * load to avoid redirect loops when parallel requests all hit 401.
 */
export function notifySessionExpired(): void {
  if (typeof window === 'undefined' || sessionExpiredNotified) return;
  const onLoginPage =
    /^\/$/.test(window.location.pathname) &&
    window.location.search.includes('reason=session-expired');
  if (onLoginPage) return;
  sessionExpiredNotified = true;
  window.dispatchEvent(
    new CustomEvent('forge:session-expired', {
      detail: { next: window.location.pathname + window.location.search },
    }),
  );
}

/**
 * Re-arm the 401 → session-expired signal after a successful (re-)login.
 * `notifySessionExpired` fires at most once per expiry; without a reset a
 * re-login in the same page lifetime would never surface a second expiry.
 */
export function resetSessionExpiredNotification(): void {
  sessionExpiredNotified = false;
}

/**
 * Opt-in retry policy for the canonical primitive. Disabled by default so a
 * caller that has not asked for retries keeps the exact previous semantics
 * (single attempt, {@link ApiError} on failure).
 */
export type ForgeRetryPolicy = {
  /** Extra attempts after the first one. Defaults to 3. */
  retries?: number;
  /** Base backoff in ms, doubled per attempt. Defaults to 500. */
  baseDelay?: number;
  /** Upper bound for a single backoff sleep in ms. Defaults to 10000. */
  maxDelay?: number;
  /** HTTP statuses considered transient. Defaults to 408/429/5xx gateways. */
  retryOnStatus?: number[];
  /**
   * Allow retries for non-idempotent methods (POST/PUT/PATCH/DELETE).
   * Retrying a mutation without the server deduplicating it can execute the
   * effect twice, so unsafe methods are never retried unless the caller
   * explicitly opts in with `idempotent: true` (and ideally an
   * `Idempotency-Key` header, which the backend honors on power dispatch).
   * GET/HEAD/OPTIONS are always safe to retry.
   */
  idempotent?: boolean;
};

export type ForgeRequestOptions = {
  /**
   * Suppress the 401 → `notifySessionExpired()` side effect. Used by credential
   * checks (e.g. login) where a 401 is an expected "bad credentials" result and
   * must not be mistaken for an expired session.
   */
  suppressSessionExpired?: boolean;
  /**
   * Retry transient failures (network errors and {@link ForgeRetryPolicy.retryOnStatus}
   * statuses) with exponential backoff. `true` uses the defaults; pass a policy
   * to tune them. Aborted requests are never retried.
   */
  retry?: ForgeRetryPolicy | boolean;
  /**
   * `path` is already a fully resolved same-origin path (e.g. a Next.js route
   * handler under `/api/*`) and must not be prefixed with the API base URL.
   */
  sameOrigin?: boolean;
  /**
   * Default abort timeout in ms applied when the caller did not provide
   * `init.signal`. Defaults to {@link DEFAULT_REQUEST_TIMEOUT_MS}. Pass
   * `false` to opt out (e.g. long-polling or streaming callers that manage
   * their own AbortController lifetime).
   */
  timeoutMs?: number | false;
};

export const DEFAULT_REQUEST_TIMEOUT_MS = 30000;

const DEFAULT_RETRY_POLICY: Required<ForgeRetryPolicy> = {
  retries: 3,
  baseDelay: 500,
  maxDelay: 10000,
  retryOnStatus: [408, 429, 500, 502, 503, 504],
  idempotent: false,
};

function resolveRetryPolicy(options: ForgeRequestOptions): Required<ForgeRetryPolicy> | null {
  if (!options.retry) return null;
  return { ...DEFAULT_RETRY_POLICY, ...(typeof options.retry === 'object' ? options.retry : {}) };
}

const IDEMPOTENT_METHODS = new Set(['GET', 'HEAD', 'OPTIONS']);

function isMethodRetryable(method: string, policy: Required<ForgeRetryPolicy>): boolean {
  if (IDEMPOTENT_METHODS.has(method.toUpperCase())) return true;
  return policy.idempotent === true;
}

function ensureIdempotencyKey(
  method: string,
  init: RequestInit,
  policy: Required<ForgeRetryPolicy>,
): RequestInit {
  if (IDEMPOTENT_METHODS.has(method.toUpperCase())) return init;
  if (policy.idempotent !== true || policy.retries < 1) return init;
  const headers = new Headers(init.headers ?? {});
  if (headers.has('Idempotency-Key')) return init;
  try {
    const key =
      typeof crypto !== 'undefined' && 'randomUUID' in crypto
        ? crypto.randomUUID()
        : `${Date.now()}-${Math.random().toString(36).slice(2)}`;
    headers.set('Idempotency-Key', key);
  } catch {
    return init;
  }
  return { ...init, headers };
}

function resolveRequestUrl(path: string, options: ForgeRequestOptions): string {
  if (options.sameOrigin || /^https?:\/\//i.test(path) || path.startsWith('//')) return path;
  // Callers occasionally pass `servers/123` without a leading slash — without
  // the join below that becomes `/api/v1servers/123`. Normalize so every
  // API-relative path resolves under the base.
  const normalizedPath = path.startsWith('/') ? path : `/${path}`;
  return `${getApiBaseUrl()}${normalizedPath}`;
}

/**
 * Cookies (session) and CSRF are only meaningful same-origin. For relative
 * URLs (`/api/v1/...`, `/api/...` with `sameOrigin`) and absolute URLs whose
 * origin matches `window.location.origin`, credentials default to `include`
 * and CSRF is signed. Cross-origin API hosts keep the caller's explicit
 * `init.credentials` or fall back to `same-origin` so cookies are never
 * leaked to a third party by default.
 */
function isSameOriginRequest(url: string, options: ForgeRequestOptions): boolean {
  if (options.sameOrigin) return true;
  if (url.startsWith('/') && !url.startsWith('//')) return true;
  if (typeof window !== 'undefined' && /^https?:\/\//i.test(url)) {
    try {
      return new URL(url, window.location.origin).origin === window.location.origin;
    } catch {
      return false;
    }
  }
  return false;
}

function resolveTimeoutMs(options: ForgeRequestOptions): number | null {
  if (options.timeoutMs === false) return null;
  if (
    typeof options.timeoutMs === 'number' &&
    Number.isFinite(options.timeoutMs) &&
    options.timeoutMs > 0
  ) {
    return options.timeoutMs;
  }
  return DEFAULT_REQUEST_TIMEOUT_MS;
}

/**
 * Default abort signal: callers that pass no `signal` still time out instead
 * of hanging forever. A caller-provided signal is combined with the timeout
 * via `AbortSignal.any` when available so both the caller's cancellation and
 * the default timeout can fire; opt out with `{ timeoutMs: false }`.
 */
function resolveSignal(
  initSignal: AbortSignal | null | undefined,
  options: ForgeRequestOptions,
): AbortSignal | undefined {
  const timeoutMs = resolveTimeoutMs(options);
  if (!initSignal) {
    if (timeoutMs == null) return undefined;
    return AbortSignal.timeout(timeoutMs);
  }
  if (timeoutMs == null) return initSignal;
  try {
    const timeoutSignal = AbortSignal.timeout(timeoutMs);
    if (typeof AbortSignal.any === 'function') {
      return AbortSignal.any([initSignal, timeoutSignal]);
    }
  } catch {
    // Fall through to the caller's signal when timeout construction fails.
  }
  return initSignal;
}

/** Backoff with jitter so parallel retries do not synchronize on the API. */
function retryDelay(policy: Required<ForgeRetryPolicy>, attempt: number): number {
  const delay = Math.min(policy.baseDelay * Math.pow(2, attempt), policy.maxDelay);
  return delay + Math.random() * (policy.baseDelay > 0 ? 500 : 0);
}

function wait(ms: number): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
}

/**
 * Cancellation surfaces (AbortController, AbortSignal.timeout) must stay
 * distinguishable from transport failures: callers such as the i18n loader gate
 * on `err.name === "AbortError"`, so these are re-thrown untouched instead of
 * being wrapped in an {@link ApiError}.
 */
function isCancellation(err: unknown): boolean {
  return err instanceof Error && (err.name === 'AbortError' || err.name === 'TimeoutError');
}

/**
 * Shared request execution for every response shape. Applies CSRF signing,
 * cookie credentials, the 401 session-expiry signal and error shaping, then
 * returns a guaranteed-ok {@link Response}. Every public helper
 * ({@link requestJSON}, {@link requestText}, {@link requestBlob},
 * {@link requestVoid}) is built on this so the app keeps a single HTTP
 * primitive instead of ad-hoc `fetch` call sites.
 */
async function sendRequest(
  method: string,
  path: string,
  init: RequestInit = {},
  options: ForgeRequestOptions = {},
): Promise<Response> {
  const url = resolveRequestUrl(path, options);
  const policy = resolveRetryPolicy(options);
  const attempts = policy ? Math.max(1, policy.retries + 1) : 1;
  // Mutations are only retried when the caller explicitly marks them
  // idempotent; otherwise a retried POST could execute twice.
  const methodRetryable = policy ? isMethodRetryable(method, policy) : true;
  const effectiveInit = policy && methodRetryable ? ensureIdempotencyKey(method, init, policy) : init;
  let lastError: unknown;

  for (let attempt = 0; attempt < attempts; attempt++) {
    try {
      return await executeRequest(method, url, path, effectiveInit, options);
    } catch (err) {
      if (isCancellation(err)) throw err;
      lastError = err;
      const status = err instanceof ApiError ? err.status : 0;
      // Status 0 is a transport failure (offline, DNS, reset) — always transient
      // when the caller asked for retries.
      const retryable =
        policy !== null &&
        methodRetryable &&
        attempt < attempts - 1 &&
        (status === 0 || policy.retryOnStatus.includes(status));
      if (!retryable) break;
      await wait(retryDelay(policy as Required<ForgeRetryPolicy>, attempt));
    }
  }

  throw lastError instanceof ApiError
    ? lastError
    : new ApiError(lastError instanceof Error ? lastError.message : 'Unknown error', 0);
}

/**
 * Normalize any `HeadersInit` (plain object, `Headers` instance or entries)
 * to a plain record. Spreading `init.headers` directly drops `Headers`
 * instances (their entries are not own enumerable props), silently losing
 * caller-supplied headers such as `Idempotency-Key` or test fixtures.
 */
function normalizeHeadersToRecord(input: HeadersInit | undefined): Record<string, string> {
  if (!input) return {};
  if (input instanceof Headers) {
    const out: Record<string, string> = {};
    input.forEach((value, key) => {
      out[key] = value;
    });
    return out;
  }
  if (Array.isArray(input)) {
    const out: Record<string, string> = {};
    for (const [key, value] of input) out[key] = value;
    return out;
  }
  return { ...(input as Record<string, string>) };
}

async function executeRequest(
  method: string,
  url: string,
  path: string,
  init: RequestInit,
  options: ForgeRequestOptions,
): Promise<Response> {
  const sameOrigin = isSameOriginRequest(url, options);
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...normalizeHeadersToRecord(init.headers as HeadersInit | undefined),
  };
  if (sameOrigin) {
    addCSRFToHeaders(headers, method);
  }
  const { signal: initSignal, ...restInit } = init;
  const signal = resolveSignal(initSignal ?? null, options);
  try {
    const response = await fetch(url, {
      ...restInit,
      ...(signal ? { signal } : {}),
      headers,
      credentials: init.credentials ?? (sameOrigin ? 'include' : 'same-origin'),
    });
    if (!response.ok) {
      if (response.status === 401 && !options.suppressSessionExpired) notifySessionExpired();
      // Clone before getErrorMessage consumes the body so 422 validation
      // details survive on `ApiError.details` (same contract as the SDK).
      const detailsClone = response.clone();
      const errorMessage = await getErrorMessage(response, `API ${method} ${path} failed with`);
      throw new ApiError(errorMessage, response.status, await readErrorDetails(detailsClone));
    }
    return response;
  } catch (err) {
    if (err instanceof ApiError) throw err;
    if (isCancellation(err)) throw err;
    const message =
      err instanceof TypeError
        ? 'Network error — check your connection and ensure the API server is running'
        : err instanceof Error
          ? err.message
          : 'Unknown error';
    throw new ApiError(message, 0);
  }
}

async function parseJSONBody<T>(response: Response, method: string, path: string): Promise<T> {
  // A 204 or empty body carries no payload. `requestJSON<void>` is the honest
  // contract for those routes; the double cast keeps the generic honest — a
  // bare `undefined as T` would claim `undefined` is every `T`.
  if (response.status === 204) return undefined as unknown as T;
  const text = await response.text();
  if (!text) return undefined as unknown as T;
  try {
    return JSON.parse(text) as T;
  } catch {
    throw new ApiError(`API ${method} ${path} returned invalid JSON`, 0);
  }
}

/** Canonical request primitive for every web API client. */
export async function requestJSON<T>(
  path: string,
  init: RequestInit = {},
  options: ForgeRequestOptions = {},
): Promise<T> {
  const method = init.method ?? 'GET';
  const response = await sendRequest(method, path, init, options);
  return parseJSONBody<T>(response, method, path);
}

export async function fetchJSON<T>(
  path: string,
  init?: RequestInit,
  options?: ForgeRequestOptions,
): Promise<T> {
  return requestJSON<T>(path, init, options);
}

/**
 * Fetch a response body as a {@link Blob} (binary downloads). Shares the
 * canonical CSRF signing, cookie credentials, 401 session-expiry handling and
 * error shaping from {@link sendRequest} rather than calling bare `fetch`.
 */
export async function requestBlob(
  path: string,
  init: RequestInit = {},
  options: ForgeRequestOptions = {},
): Promise<Blob> {
  const response = await sendRequest(init.method ?? 'GET', path, init, options);
  if (response.status === 204) return new Blob();
  return response.blob();
}

/** Fetch a response body as text. See {@link requestBlob} for rationale. */
export async function requestText(
  path: string,
  init: RequestInit = {},
  options: ForgeRequestOptions = {},
): Promise<string> {
  const response = await sendRequest(init.method ?? 'GET', path, init, options);
  return response.text();
}

/**
 * POST a {@link FormData} body (multipart file upload) and parse the JSON
 * response. `Content-Type` is deliberately left unset so the browser can
 * generate the multipart boundary; everything else (CSRF signing, cookie
 * credentials, 401 session-expiry handling, error shaping, opt-in retry) is the
 * same as {@link requestJSON}, so file uploads never need an ad-hoc `fetch`.
 */
export async function postMultipartJSON<T>(
  path: string,
  form: FormData,
  options: ForgeRequestOptions = {},
): Promise<T> {
  const response = await sendRequest('POST', path, { method: 'POST', body: form }, options);
  return parseJSONBody<T>(response, 'POST', path);
}

/**
 * Issue a request whose response body is irrelevant (fire-and-forget uploads or
 * raw-body writes). Still routes through {@link sendRequest} so CSRF,
 * credentials, 401 handling and error shaping are never skipped. The body is
 * drained so the connection can be reused.
 */
export async function requestVoid(
  path: string,
  init: RequestInit = {},
  options: ForgeRequestOptions = {},
): Promise<void> {
  const response = await sendRequest(init.method ?? 'GET', path, init, options);
  if (response.status !== 204) {
    await response.text().catch(() => {});
  }
}

/**
 * JSON convenience wrappers. CSRF signing, credentials, timeouts and error
 * shaping all live in {@link sendRequest} — these only set the method and the
 * JSON body. Authentication is session-cookie based (HttpOnly), so no
 * Authorization header is attached here.
 */
function mergeJsonInit(method: string, body: unknown, init?: RequestInit): RequestInit {
  const contentHeaders: Record<string, string> =
    body !== undefined ? { 'Content-Type': 'application/json' } : {};
  const extraHeaders = normalizeHeadersToRecord(init?.headers as HeadersInit | undefined);
  const { headers: _omit, method: _m, body: _b, ...rest } = init ?? {};
  void _omit;
  void _m;
  void _b;
  return {
    ...rest,
    method,
    headers: { ...contentHeaders, ...extraHeaders },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  };
}

export async function postJSON<T>(
  path: string,
  body?: unknown,
  init?: RequestInit,
  options?: ForgeRequestOptions,
): Promise<T> {
  return requestJSON<T>(path, mergeJsonInit('POST', body, init), options);
}

export async function putJSON<T>(
  path: string,
  body?: unknown,
  init?: RequestInit,
  options?: ForgeRequestOptions,
): Promise<T> {
  return requestJSON<T>(path, mergeJsonInit('PUT', body, init), options);
}

export async function patchJSON<T>(
  path: string,
  body?: unknown,
  init?: RequestInit,
  options?: ForgeRequestOptions,
): Promise<T> {
  return requestJSON<T>(path, mergeJsonInit('PATCH', body, init), options);
}

export async function deleteJSON<T = void>(
  path: string,
  body?: unknown,
  init?: RequestInit,
  options?: ForgeRequestOptions,
): Promise<T> {
  return requestJSON<T>(path, mergeJsonInit('DELETE', body, init), options);
}

function statusHint(status: number): string {
  if (status === 0 || status >= 600) return 'API server unreachable — is the Go backend running?';
  if (status === 401) return 'Not authenticated — try logging out and back in';
  if (status === 403) return 'Access denied — you may lack permission for this resource';
  if (status === 404) return 'Endpoint not found — check API server is running and up to date';
  if (status === 503)
    return 'Service unavailable — a required dependency (database/daemon) is not ready';
  return '';
}

/**
 * Read structured 422 validation details (`{ errors }`, `{ details }` or
 * `{ fields }`) from a cloned error response. Never throws — returns
 * `undefined` when the body is absent or has no details shape.
 */
export async function readErrorDetails(res: Response): Promise<unknown> {
  try {
    const body: unknown = await res.json();
    if (body !== null && typeof body === 'object') {
      const obj = body as Record<string, unknown>;
      if (obj['details'] !== undefined) return obj['details'];
      if (obj['errors'] !== undefined) return obj['errors'];
      if (obj['fields'] !== undefined) return obj['fields'];
    }
  } catch {
    // Non-JSON or unreadable body — no structured details.
  }
  return undefined;
}

export async function getErrorMessage(response: Response, prefix: string): Promise<string> {
  const cloned = response.clone();
  try {
    const error = await response.json();
    const msg = error.message || error.error || '';
    const hint = statusHint(response.status);
    return msg
      ? `${msg}${hint ? ' — ' + hint : ''}`
      : `${prefix} ${response.status}${hint ? ' — ' + hint : ''}`;
  } catch {
    const hint = statusHint(response.status);
    try {
      const text = await cloned.text();
      if (text)
        return `${prefix} ${response.status}: ${text.slice(0, 300)}${hint ? ' — ' + hint : ''}`;
    } catch {}
    return `${prefix} ${response.status}${hint ? ' — ' + hint : ''}`;
  }
}

/**
 * Build an absolute WebSocket URL for an API-relative path (e.g.
 * `/servers/:id/ws/console`).
 *
 * The API base is usually same-origin (`/api/v1`), and a naive
 * `base.replace(/^http/, "ws")` leaves it untouched — `new WebSocket` then
 * throws because the URL is not absolute. Resolving against
 * `window.location.origin` and swapping the scheme keeps same-origin
 * deployments working while absolute API hosts map http→ws / https→wss.
 * On the server (no `window`) the relative path is returned so SSR never
 * fabricates a host.
 */
export function buildWebSocketUrl(apiPath: string): string {
  const base = getApiBaseUrl();
  const path = apiPath.startsWith('/') ? apiPath : `/${apiPath}`;
  if (/^wss?:\/\//i.test(base)) {
    return `${base.replace(/\/$/, '')}${path}`;
  }
  if (/^https?:\/\//i.test(base)) {
    const protocol = base.toLowerCase().startsWith('https:') ? 'wss:' : 'ws:';
    try {
      const url = new URL(`${base.replace(/\/$/, '')}${path}`);
      url.protocol = protocol;
      return url.toString();
    } catch {
      return `${base.replace(/\/$/, '').replace(/^http/, 'ws')}${path}`;
    }
  }
  if (typeof window !== 'undefined') {
    try {
      const url = new URL(`${base.replace(/\/$/, '')}${path}`, window.location.origin);
      url.protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      return url.toString();
    } catch {
      // Fall through to the relative path below.
    }
  }
  return `${base.replace(/\/$/, '')}${path}`;
}

/**
 * Reachability probe for the offline banner / setup screens. Uses the canonical
 * {@link sendRequest} (with the 401 signal suppressed) so even this call does
 * not bypass the single HTTP path; any non-ok or transport error is "not
 * reachable".
 */
export async function checkApiReachable(): Promise<boolean> {
  try {
    await sendRequest(
      'GET',
      '/health',
      { signal: AbortSignal.timeout(3000) },
      { suppressSessionExpired: true },
    );
    return true;
  } catch {
    return false;
  }
}

export type FetchExternalOptions = {
  signal?: AbortSignal;
  timeoutMs?: number | false;
};

/**
 * Fetch an arbitrary third-party URL (not the Forge API) with the same error
 * shaping as the canonical primitive: transport failures surface as
 * {@link ApiError} with status 0, non-2xx as {@link ApiError} with the real
 * status, and AbortError/TimeoutError propagate untouched. Routes through
 * {@link sendRequest} so there is still exactly one HTTP execution path —
 * absolute URLs bypass the API base prefix, cross-origin calls default to
 * `same-origin` credentials (never `include`), and CSRF is only signed for
 * same-origin targets (see `isSameOriginRequest`).
 */
export async function fetchExternalBlob(url: string, options?: FetchExternalOptions): Promise<Blob> {
  const response = await sendRequest(
    'GET',
    url,
    options?.signal ? { signal: options.signal } : {},
    options?.timeoutMs !== undefined ? { timeoutMs: options.timeoutMs } : {},
  );
  if (response.status === 204) return new Blob();
  return response.blob();
}

/** Text variant of {@link fetchExternalBlob} with identical error shaping. */
export async function fetchExternalText(url: string, options?: FetchExternalOptions): Promise<string> {
  const response = await sendRequest(
    'GET',
    url,
    options?.signal ? { signal: options.signal } : {},
    options?.timeoutMs !== undefined ? { timeoutMs: options.timeoutMs } : {},
  );
  return response.text();
}
