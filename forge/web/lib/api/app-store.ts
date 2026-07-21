const API_BASE =
  process.env.NEXT_PUBLIC_API_URL ??
  (process.env.NODE_ENV === "development" ? "http://localhost:8080/api/v1" : "/api/v1");

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
  const res = await fetch(`${API_BASE}${path}`, {
    credentials: "include",
    headers: { Accept: "application/json", ...options?.headers },
    ...options,
  });
  if (!res.ok) {
    const err = await res.text();
    throw new Error(`API ${options?.method ?? "GET"} ${path}: ${res.status} ${err}`);
  }
  const body = await res.json();
  return body?.data ?? body;
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
