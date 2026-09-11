import { fetchJSON, putJSON, unwrapData, unwrapList } from "./http";

export type SFTPGlobalConfig = {
  enabled: boolean;
  defaultPort: number;
  defaultMaxConnections: number;
  defaultIdleTimeout: number;
  defaultRateLimit: number;
  logLevel: string;
  allowedCiphers?: string[];
  allowedMACs?: string[];
  allowedKexAlgos?: string[];
  hostKeyAlgorithms?: string[];
};

export type SFTPNodeConfig = {
  nodeId: string;
  enabled: boolean;
  listenPort: number;
  listenIP: string;
  maxConnections: number;
  maxAuthAttempts: number;
  idleTimeout: number;
  rateLimit: number;
  readOnly: boolean;
  allowedIps?: string[];
  banner?: string;
  logLevel: string;
  updatedAt: string;
};

export async function fetchSFTPGlobalConfig(): Promise<SFTPGlobalConfig> {
  return unwrapData(
    await fetchJSON<SFTPGlobalConfig | { data: SFTPGlobalConfig }>("/admin/sftp/settings"),
  );
}

export async function updateSFTPGlobalConfig(cfg: SFTPGlobalConfig): Promise<{ ok: boolean }> {
  return putJSON<{ ok: boolean }>("/admin/sftp/settings", cfg);
}

export async function fetchSFTPNodeConfigs(): Promise<SFTPNodeConfig[]> {
  return unwrapList(
    await fetchJSON<SFTPNodeConfig[] | { data: SFTPNodeConfig[] }>("/admin/sftp/nodes"),
  );
}

export async function fetchSFTPNodeConfig(nodeId: string): Promise<SFTPNodeConfig> {
  return unwrapData(
    await fetchJSON<SFTPNodeConfig | { data: SFTPNodeConfig }>(`/admin/nodes/${encodeURIComponent(nodeId)}/sftp`),
  );
}

export async function updateSFTPNodeConfig(nodeId: string, cfg: SFTPNodeConfig): Promise<{ ok: boolean }> {
  // Ensure nodeId in payload matches param (store overwrites with param anyway)
  const payload = { ...cfg, nodeId };
  return putJSON<{ ok: boolean }>(`/admin/nodes/${encodeURIComponent(nodeId)}/sftp`, payload);
}
