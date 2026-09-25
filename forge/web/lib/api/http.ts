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
  if (typeof window !== "undefined") {
    const runtime = (window as unknown as { __FORGE_CONFIG__?: { apiBaseUrl?: string } }).__FORGE_CONFIG__?.apiBaseUrl;
    if (runtime) {
      const normalizedRuntime = runtime.replace(/\/+$/, "");
      if (normalizedRuntime) return normalizedRuntime;
    }
  }
  const envValue = process.env.NEXT_PUBLIC_API_URL ?? process.env.NEXT_PUBLIC_API_BASE_URL ?? "/api/v1";
  const normalized = envValue.replace(/\/+$/, "");
  return normalized || "/api/v1";
}

export const API_BASE_URL = resolveApiBaseUrl();

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export function getAuthHeaders(): Record<string, string> {
  // No-op by design: authentication is session-cookie based (HttpOnly cookie),
  // so there is no client-side token to attach. Cross-origin requests rely on
  // SameSite=None cookies and CORS credentials; WS streams use short-lived
  // tickets issued by the API instead of headers.
  return {};
}

export function getCSRFToken(): string | null {
  if (typeof document === 'undefined') return null;
  const match = document.cookie.match(/__Host-forge_csrf=([^;]+)/) ?? document.cookie.match(/(?:^|;\s*)forge_csrf=([^;]+)/);
  return match ? decodeURIComponent(match[1]) : null;
}

function addCSRFToHeaders(headers: Record<string, string>, method: string): void {
  if (!['POST', 'PUT', 'PATCH', 'DELETE'].includes(method.toUpperCase())) return;
  const csrfToken = getCSRFToken();
  if (csrfToken) {
    headers['X-CSRF-Token'] = csrfToken;
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
  const onLoginPage = /^\/$/.test(window.location.pathname) && window.location.search.includes('reason=session-expired');
  if (onLoginPage) return;
  sessionExpiredNotified = true;
  window.dispatchEvent(
    new CustomEvent('forge:session-expired', {
      detail: { next: window.location.pathname + window.location.search },
    }),
  );
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
};

const DEFAULT_RETRY_POLICY: Required<ForgeRetryPolicy> = {
  retries: 3,
  baseDelay: 500,
  maxDelay: 10000,
  retryOnStatus: [408, 429, 500, 502, 503, 504],
};

function resolveRetryPolicy(options: ForgeRequestOptions): Required<ForgeRetryPolicy> | null {
  if (!options.retry) return null;
  return { ...DEFAULT_RETRY_POLICY, ...(typeof options.retry === 'object' ? options.retry : {}) };
}

function resolveRequestUrl(path: string, options: ForgeRequestOptions): string {
  if (options.sameOrigin || /^https?:\/\//i.test(path) || path.startsWith('//')) return path;
  return `${API_BASE_URL}${path}`;
}

/** Backoff with jitter so parallel retries do not synchronize on the API. */
function retryDelay(policy: Required<ForgeRetryPolicy>, attempt: number): number {
  const delay = Math.min(policy.baseDelay * Math.pow(2, attempt), policy.maxDelay);
  return delay + Math.random() * (policy.baseDelay > 0 ? 500 : 0);
}

function wait(ms: number): Promise<void> {
  return new Promise((resolve) => { setTimeout(resolve, ms); });
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
  let lastError: unknown;

  for (let attempt = 0; attempt < attempts; attempt++) {
    try {
      return await executeRequest(method, url, path, init, options);
    } catch (err) {
      if (isCancellation(err)) throw err;
      lastError = err;
      const status = err instanceof ApiError ? err.status : 0;
      // Status 0 is a transport failure (offline, DNS, reset) — always transient
      // when the caller asked for retries.
      const retryable = policy !== null && attempt < attempts - 1 && (status === 0 || policy.retryOnStatus.includes(status));
      if (!retryable) break;
      await wait(retryDelay(policy as Required<ForgeRetryPolicy>, attempt));
    }
  }

  throw lastError instanceof ApiError
    ? lastError
    : new ApiError(lastError instanceof Error ? lastError.message : 'Unknown error', 0);
}

async function executeRequest(
  method: string,
  url: string,
  path: string,
  init: RequestInit,
  options: ForgeRequestOptions,
): Promise<Response> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...(init.headers as Record<string, string> | undefined),
  };
  addCSRFToHeaders(headers, method);
  try {
    const response = await fetch(url, {
      ...init,
      headers,
      credentials: init.credentials ?? 'include',
    });
    if (!response.ok) {
      if (response.status === 401 && !options.suppressSessionExpired) notifySessionExpired();
      const errorMessage = await getErrorMessage(response, `API ${method} ${path} failed with`);
      throw new ApiError(errorMessage, response.status);
    }
    return response;
  } catch (err) {
    if (err instanceof ApiError) throw err;
    if (isCancellation(err)) throw err;
    const message = err instanceof TypeError
      ? "Network error — check your connection and ensure the API server is running"
      : err instanceof Error ? err.message : "Unknown error";
    throw new ApiError(message, 0);
  }
}

async function parseJSONBody<T>(response: Response, method: string, path: string): Promise<T> {
  if (response.status === 204) return undefined as T;
  const text = await response.text();
  if (!text) return undefined as T;
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

export async function fetchJSON<T>(path: string, init?: RequestInit, options?: ForgeRequestOptions): Promise<T> {
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

export async function postJSON<T>(path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...getAuthHeaders(),
  };
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
  }
  addCSRFToHeaders(headers, 'POST');

  return requestJSON<T>(path, {
    method: 'POST',
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'include',
  });
}

export async function putJSON<T>(path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...getAuthHeaders(),
  };
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
  }
  addCSRFToHeaders(headers, 'PUT');

  return requestJSON<T>(path, {
    method: 'PUT',
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'include',
  });
}

export async function patchJSON<T>(path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...getAuthHeaders(),
  };
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
  }
  addCSRFToHeaders(headers, 'PATCH');

  return requestJSON<T>(path, {
    method: 'PATCH',
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'include',
  });
}

export async function deleteJSON<T = void>(path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...getAuthHeaders(),
  };
  addCSRFToHeaders(headers, 'DELETE');

  return requestJSON<T>(path, {
    method: 'DELETE',
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'include',
  });
}

function statusHint(status: number): string {
  if (status === 0 || status >= 600) return "API server unreachable — is the Go backend running?";
  if (status === 401) return "Not authenticated — try logging out and back in";
  if (status === 403) return "Access denied — admin role required";
  if (status === 404) return "Endpoint not found — check API server is running and up to date";
  if (status === 503) return "Service unavailable — a required dependency (database/daemon) is not ready";
  return "";
}

export async function getErrorMessage(response: Response, prefix: string): Promise<string> {
  const cloned = response.clone();
  try {
    const error = await response.json();
    const msg = error.message || error.error || "";
    const hint = statusHint(response.status);
    return msg ? `${msg}${hint ? " — " + hint : ""}` : `${prefix} ${response.status}${hint ? " — " + hint : ""}`;
  } catch {
    const hint = statusHint(response.status);
    try {
      const text = await cloned.text();
      if (text) return `${prefix} ${response.status}: ${text.slice(0, 300)}${hint ? " — " + hint : ""}`;
    } catch {}
    return `${prefix} ${response.status}${hint ? " — " + hint : ""}`;
  }
}

/**
 * Reachability probe for the offline banner / setup screens. Uses the canonical
 * {@link sendRequest} (with the 401 signal suppressed) so even this call does
 * not bypass the single HTTP path; any non-ok or transport error is "not
 * reachable".
 */
export async function checkApiReachable(): Promise<boolean> {
  try {
    await sendRequest('GET', '/health', { signal: AbortSignal.timeout(3000) }, { suppressSessionExpired: true });
    return true;
  } catch {
    return false;
  }
}
