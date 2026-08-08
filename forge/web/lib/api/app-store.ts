import { API_BASE_URL, getCSRFToken } from './http';

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

async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
  const method = options?.method ?? 'GET';
  const headers: Record<string, string> = { Accept: 'application/json', ...options?.headers as Record<string, string> };
  if (['POST', 'PUT', 'PATCH', 'DELETE'].includes(method)) {
    const csrf = getCSRFToken();
    if (csrf) headers['X-CSRF-Token'] = csrf;
  }

  // Rate limiting retry configuration
  const maxRetries = 3;
  const baseDelay = 1000; // 1 second
    let lastError: Error = new Error("Unknown error");

    for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      const res = await fetch(`${API_BASE_URL}${path}`, {
        credentials: "include",
        ...options,
        headers,
      });

      if (!res.ok) {
        const err = await res.text();

        // Handle rate limiting (429) with retry
        if (res.status === 429) {
          const retryAfterHeader = res.headers.get('Retry-After');
          const retryAfter = retryAfterHeader ? parseInt(retryAfterHeader) : 5; // Default 5 seconds
          const delay = Math.min(retryAfter * 1000, 30000); // Max 30 seconds

          if (attempt < maxRetries) {
            await new Promise(resolve => setTimeout(resolve, delay));
            continue; // Retry the request
          }

          throw new Error(`API ${method} ${path}: ${res.status} ${err}`);
        }

        throw new Error(`API ${method} ${path}: ${res.status} ${err}`);
      }

      const body = await res.json();
      return body?.data ?? body;
    } catch (error) {
      lastError = error instanceof Error ? error : new Error(String(error));

      // For network errors, retry with exponential backoff
      if (attempt < maxRetries) {
        const delay = baseDelay * Math.pow(2, attempt); // Exponential backoff
        await new Promise(resolve => setTimeout(resolve, delay));
      }
    }
  }

  throw lastError;
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

export async function uninstallApp(id: string): Promise<{ data: string }> {
  return apiFetch<{ data: string }>(`/app-store/${encodeURIComponent(id)}/uninstall`, {
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
  return apiFetch<{ data: string }>("/app-store/sync", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ registryUrl }),
  });
}
