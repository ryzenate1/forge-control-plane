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

/** Canonical request primitive for every web API client. */
export async function requestJSON<T>(path: string, init: RequestInit = {}): Promise<T> {
  const method = init.method ?? 'GET';
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...(init.headers as Record<string, string> | undefined),
  };
  addCSRFToHeaders(headers, method);
  try {
    const response = await fetch(`${API_BASE_URL}${path}`, {
      ...init,
      headers,
      credentials: init.credentials ?? 'include',
    });
    if (!response.ok) {
      if (response.status === 401) notifySessionExpired();
      const errorMessage = await getErrorMessage(response, `API ${method} ${path} failed with`);
      throw new ApiError(errorMessage, response.status);
    }
    if (response.status === 204) return undefined as T;
    const text = await response.text();
    if (!text) return undefined as T;
    try {
      return JSON.parse(text) as T;
    } catch {
      throw new Error(`API ${method} ${path} returned invalid JSON`);
    }
  } catch (err) {
    if (err instanceof ApiError) throw err;
    const message = err instanceof TypeError
      ? "Network error — check your connection and ensure the API server is running"
      : err instanceof Error ? err.message : "Unknown error";
    throw new ApiError(message, 0);
  }
}

export async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  return requestJSON<T>(path, init);
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

export async function checkApiReachable(): Promise<boolean> {
  try {
    const res = await fetch(`${API_BASE_URL}/health`, { method: "GET", signal: AbortSignal.timeout(3000) });
    return res.ok;
  } catch {
    return false;
  }
}
