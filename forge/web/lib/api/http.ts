// HTTP helper functions for API calls
export const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ??
  (process.env.NODE_ENV === 'development' ? 'http://localhost:8080/api/v1' : '/api/v1');

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
  return {};
}

export function getCSRFToken(): string | null {
  if (typeof document === 'undefined') return null;
  const match = document.cookie.match(/__Host-forge_csrf=([^;]+)/);
  return match ? decodeURIComponent(match[1]) : null;
}

function addCSRFToHeaders(headers: Record<string, string>, method: string): void {
  if (!['POST', 'PUT', 'PATCH', 'DELETE'].includes(method.toUpperCase())) return;
  const csrfToken = getCSRFToken();
  if (csrfToken) {
    headers['X-CSRF-Token'] = csrfToken;
  }
}

export async function fetchJSON<T>(path: string): Promise<T> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...getAuthHeaders(),
  };

  return apiFetch<T>(path, {
    headers,
    credentials: 'include',
  });
}

async function apiFetch<T>(path: string, init: RequestInit): Promise<T> {
  try {
    const response = await fetch(`${API_BASE_URL}${path}`, init);
    if (!response.ok) {
      const errorMessage = await getErrorMessage(response, `API ${init.method ?? "GET"} ${path} failed with`);
      throw new ApiError(errorMessage, response.status);
    }
    if (response.status === 204) return undefined as T;
    const text = await response.text();
    if (!text) return undefined as T;
    return JSON.parse(text) as T;
  } catch (err) {
    if (err instanceof ApiError) throw err;
    const message = err instanceof TypeError
      ? "Network error — check your connection and ensure the API server is running"
      : err instanceof Error ? err.message : "Unknown error";
    throw new ApiError(message, 0);
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

  return apiFetch<T>(path, {
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

  return apiFetch<T>(path, {
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

  return apiFetch<T>(path, {
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

  return apiFetch<T>(path, {
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

async function getErrorMessage(response: Response, prefix: string): Promise<string> {
  try {
    const error = await response.json();
    const msg = error.message || error.error || "";
    const hint = statusHint(response.status);
    return msg ? `${msg}${hint ? " — " + hint : ""}` : `${prefix} ${response.status}${hint ? " — " + hint : ""}`;
  } catch {
    const hint = statusHint(response.status);
    try {
      const text = await response.clone().text();
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
