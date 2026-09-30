import { fetchJSON, postJSON, putJSON } from "./http";

export type CatalogEntry = {
  id: string;
  key: string;
  displayName: string;
  description: string;
  category: string;
  versions: string[];
  defaultVersion: string;
  icon: string;
  requires: string[];
  enabled: boolean;
  sortOrder: number;
  createdAt: string;
  updatedAt: string;
};

export type CatalogInstance = {
  id: string;
  entryKey: string;
  kind: string;
  version: string;
  environmentId?: string;
  nodeId?: string;
  refType?: string;
  instanceRef?: string;
  host?: string;
  port: number;
  connString?: string;
  status: string;
  errorMessage?: string;
  createdAt: string;
  updatedAt: string;
};

export type CatalogAttachLink = {
  id: string;
  catalogInstanceId: string;
  environmentId: string;
  varPrefix: string;
  createdAt: string;
};

export type BackupRetention = {
  kind: string;
  enabled: boolean;
  retentionDays: number;
  retentionMax: number;
  updatedAt: string;
};

export type ProvisionInput = {
  kind: string;
  version?: string;
  envId?: string;
  nodeId: string;
  resources?: { memoryMb?: number; cpuShares?: number; diskMb?: number };
};

export async function fetchCatalogEntries(): Promise<CatalogEntry[]> {
  const res = await fetchJSON<{ data: CatalogEntry[] } | CatalogEntry[]>("/catalog");
  if (Array.isArray(res)) return res;
  return (res as { data: CatalogEntry[] }).data ?? [];
}

export async function fetchCatalogEntry(key: string): Promise<CatalogEntry> {
  const res = await fetchJSON<{ data: CatalogEntry } | CatalogEntry>(`/catalog/${encodeURIComponent(key)}`);
  const maybeData = (res as { data: CatalogEntry }).data;
  return maybeData ?? (res as CatalogEntry);
}

export async function provisionCatalog(input: ProvisionInput): Promise<CatalogInstance> {
  const res = await postJSON<{ data: CatalogInstance } | CatalogInstance>("/admin/catalog/provision", {
    kind: input.kind,
    version: input.version,
    envId: input.envId,
    nodeId: input.nodeId,
    resources: input.resources ?? {},
  });
  const maybeData = (res as { data: CatalogInstance }).data;
  return maybeData ?? (res as CatalogInstance);
}

export async function fetchCatalogInstances(entryKey: string, envId?: string): Promise<CatalogInstance[]> {
  const q = envId ? `?envId=${encodeURIComponent(envId)}` : "";
  const res = await fetchJSON<{ data: CatalogInstance[] } | CatalogInstance[]>(
    `/catalog/${encodeURIComponent(entryKey)}/instances${q}`,
  );
  if (Array.isArray(res)) return res;
  return (res as { data: CatalogInstance[] }).data ?? [];
}

export async function attachCatalogInstance(entryKey: string, instanceId: string, envId: string): Promise<CatalogAttachLink> {
  const res = await postJSON<{ data: CatalogAttachLink } | CatalogAttachLink>(
    `/catalog/${encodeURIComponent(entryKey)}/instances/${encodeURIComponent(instanceId)}/attach`,
    { envId },
  );
  const maybeData = (res as { data: CatalogAttachLink }).data;
  return maybeData ?? (res as CatalogAttachLink);
}

export async function fetchCatalogRetention(): Promise<BackupRetention[]> {
  const res = await fetchJSON<{ data: BackupRetention[] } | BackupRetention[]>("/catalog/backups/retention");
  if (Array.isArray(res)) return res;
  return (res as { data: BackupRetention[] }).data ?? [];
}

export async function setCatalogRetention(input: {
  kind: string;
  retentionDays?: number;
  retentionMax?: number;
  enabled?: boolean;
}): Promise<BackupRetention> {
  const res = await putJSON<{ data: BackupRetention } | BackupRetention>("/catalog/backups/retention", input);
  const maybeData = (res as { data: BackupRetention }).data;
  return maybeData ?? (res as BackupRetention);
}

export async function runCatalogRetention(): Promise<{ deleted: number }> {
  return postJSON<{ deleted: number }>("/catalog/backups/retention/run", {});
}
