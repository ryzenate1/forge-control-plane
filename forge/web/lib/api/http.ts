// HTTP helper functions for API calls
export const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ??
  (process.env.NODE_ENV === "development" ? "http://localhost:8080/api/v1" : "/api/v1");
export const LEGACY_TOKEN_KEY = 'modern-game-panel-token';

export class ApiError extends Error {
  constructor(message: string, readonly status: number) {
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

async function handleResponse<T>(response: Response, method: string, path: string): Promise<T> {
  if (!response.ok) {
    const errorMessage = await getErrorMessage(response, `API ${method} ${path} failed with`);
    throw new ApiError(errorMessage, response.status);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const text = await response.text();
  if (!text) {
    return undefined as T;
  }

  return JSON.parse(text) as T;
}

export async function fetchJSON<T>(path: string): Promise<T> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...getAuthHeaders(),
  };

  const response = await fetch(`${API_BASE_URL}${path}`, {
    headers,
    credentials: 'include',
  });

  return handleResponse<T>(response, 'GET', path);
}

export async function postJSON<T>(path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...getAuthHeaders(),
  };
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
  }
  await addCSRFToHeaders(headers, 'POST');

  const response = await fetch(`${API_BASE_URL}${path}`, {
    method: 'POST',
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'include',
  });

  if (!response.ok) {
    const errorMessage = await getErrorMessage(response, `API POST ${path} failed with`);
    throw new ApiError(errorMessage, response.status);
  }

  return response.json() as Promise<T>;
}

export async function putJSON<T>(path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...getAuthHeaders(),
  };
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
  }
  await addCSRFToHeaders(headers, 'PUT');

  const response = await fetch(`${API_BASE_URL}${path}`, {
    method: 'PUT',
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'include',
  });

  if (!response.ok) {
    const errorMessage = await getErrorMessage(response, `API PUT ${path} failed with`);
    throw new ApiError(errorMessage, response.status);
  }

  return response.json() as Promise<T>;
}

export async function patchJSON<T>(path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...getAuthHeaders(),
  };
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
  }
  await addCSRFToHeaders(headers, 'PATCH');

  const response = await fetch(`${API_BASE_URL}${path}`, {
    method: 'PATCH',
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'include',
  });

  if (!response.ok) {
    const errorMessage = await getErrorMessage(response, `API PATCH ${path} failed with`);
    throw new ApiError(errorMessage, response.status);
  }

  return response.json() as Promise<T>;
}

export async function deleteJSON<T = void>(path: string, body?: unknown): Promise<T> {
  const headers = {
    Accept: 'application/json',
    ...getAuthHeaders(),
  };
  await addCSRFToHeaders(headers, 'DELETE');

  const options: RequestInit = {
    method: 'DELETE',
    headers,
    credentials: 'include',
  };

  if (body) {
    options.headers = {
      ...options.headers,
      'Content-Type': 'application/json',
    };
    options.body = JSON.stringify(body);
  }

  const response = await fetch(`${API_BASE_URL}${path}`, options);

  if (!response.ok) {
    const errorMessage = await getErrorMessage(response, `API DELETE ${path} failed with`);
    throw new ApiError(errorMessage, response.status);
  }

  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

async function getErrorMessage(response: Response, prefix: string): Promise<string> {
  try {
    const error = await response.json();
    return error.message || error.error || `${prefix} ${response.status}`;
  } catch {
    return `${prefix} ${response.status}`;
  }
}
