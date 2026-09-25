import { requestJSON, type ForgeRequestOptions } from './http';

export type AppStoreApp = {
  id: string;
  key: string;
  name: string;
  shortDesc: string;
  description: string;
  icon: string;
  category: string;
  tags: string[];
  version: string;
  composeContent: string;
  params: Record<string, { label: string; type: string; default: string | number; description: string }>;
  minMemoryMb: number;
  minDiskMb: number;
  maintainer: string;
  sourceUrl: string;
  createdAt: string;
  updatedAt: string;
};

export type AppStoreInstall = {
  id: string;
  appKey: string;
  appVersion: string;
  projectId: string;
  environmentId: string;
  name: string;
  status: string;
  params: Record<string, string>;
  composeContent: string;
  composeProjectId: string;
  errorMessage: string;
  createdAt: string;
  updatedAt: string;
};

export type InstallRequest = {
  appKey: string;
  name: string;
  projectId?: string;
  environmentId?: string;
  params: Record<string, string>;
  nodeId: string;
  memoryMb: number;
  cpuShares: number;
  diskMb: number;
};

/**
 * App-store requests. This used to be a third private HTTP client (own `fetch`,
 * own CSRF header, own retry/backoff loop, own error strings); it is now a thin
 * wrapper over the canonical primitive so CSRF, cookie credentials, the 401
 * session-expiry signal and {@link ApiError} shaping are handled in exactly one
 * place. Retry/backoff intentionally moved to the react-query `retry` option at
 * the call sites — a client-side retry loop hidden behind an API function
 * double-retries against the server's rate limiter and is invisible to the
 * components that own the request lifecycle.
 *
 * The Go handlers wrap every payload in `{"data": ...}`, so the envelope is
 * unwrapped here to keep the exported function signatures unchanged.
 */
async function apiFetch<T>(path: string, init: RequestInit = {}, options: ForgeRequestOptions = {}): Promise<T> {
  const body = await requestJSON<T | { data?: T }>(path, init, options);
  if (body && typeof body === 'object' && !Array.isArray(body) && 'data' in body) {
    const unwrapped = (body as { data?: T }).data;
    if (unwrapped !== undefined) return unwrapped;
  }
  return body as T;
}

export async function listApps(category?: string, search?: string): Promise<AppStoreApp[]> {
  const params = new URLSearchParams();
  if (category) params.set("category", category);
  if (search) params.set("search", search);
  const qs = params.toString();
  return apiFetch<AppStoreApp[]>(`/app-store/apps${qs ? `?${qs}` : ""}`);
}

export async function getApp(key: string): Promise<AppStoreApp> {
  return apiFetch<AppStoreApp>(`/app-store/apps/${encodeURIComponent(key)}`);
}

export async function installApp(req: InstallRequest): Promise<AppStoreInstall> {
  return apiFetch<AppStoreInstall>("/app-store/install", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

/** Uninstall returns the plain `{"data":"ok"}` acknowledgement. */
export async function uninstallApp(id: string, force?: boolean): Promise<{ data: string }> {
  const qs = force ? "?force=true" : "";
  return requestJSON<{ data: string }>(`/app-store/${encodeURIComponent(id)}/uninstall${qs}`, {
    method: "POST",
  });
}

export async function listInstalls(): Promise<AppStoreInstall[]> {
  return apiFetch<AppStoreInstall[]>("/app-store/installed");
}

export async function upgradeApp(id: string): Promise<AppStoreInstall> {
  return apiFetch<AppStoreInstall>(`/app-store/${encodeURIComponent(id)}/upgrade`, {
    method: "POST",
  });
}

export async function syncRegistry(registryUrl?: string): Promise<{ data: string }> {
  return requestJSON<{ data: string }>("/app-store/sync", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ registryUrl }),
  });
}
