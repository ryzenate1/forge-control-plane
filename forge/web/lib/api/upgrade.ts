import { fetchJSON, postJSON, deleteJSON } from "./http";

export type UpgradeVersionInfo = {
  component: string;
  current: string;
  latest: string;
  upgradable: boolean;
};

export type UpgradePlan = {
  id: string;
  type: string;
  fromVersion: string;
  toVersion: string;
  components: string[];
  status: string;
  progress: number;
  totalSteps: number;
  currentStep: string;
  error?: string;
  backupPath?: string;
  startedAt: string;
  completedAt?: string | null;
  createdAt: string;
  updatedAt: string;
};

export type UpgradeResult = {
  success: boolean;
  message: string;
  upgradePlan?: UpgradePlan;
  versionInfo?: UpgradeVersionInfo[];
  error?: string;
};

export async function checkForUpgrades(): Promise<UpgradeVersionInfo[]> {
  const res = await fetchJSON<{ data: UpgradeVersionInfo[] }>("/upgrade/versions");
  return res.data ?? [];
}

export async function getSystemStatus(): Promise<Record<string, string>> {
  const res = await fetchJSON<{ data: Record<string, string> }>("/upgrade/system-status");
  return res.data ?? {};
}

export async function listUpgradePlans(limit = 50): Promise<UpgradePlan[]> {
  const res = await fetchJSON<{ data: UpgradePlan[] }>(`/upgrade/plans?limit=${limit}`);
  return res.data ?? [];
}

export async function getUpgradePlan(id: string): Promise<UpgradePlan> {
  const res = await fetchJSON<{ data: UpgradePlan }>(`/upgrade/plans/${encodeURIComponent(id)}`);
  return res.data;
}

export async function createUpgradePlan(type: string, components: string[]): Promise<UpgradePlan> {
  const res = await postJSON<{ data: UpgradePlan }>("/upgrade/plans", { type, components });
  return res.data;
}

export async function executeUpgradePlan(id: string): Promise<UpgradeResult> {
  const res = await postJSON<{ data: UpgradeResult }>(`/upgrade/plans/${encodeURIComponent(id)}/execute`, {});
  return res.data;
}

export async function cancelUpgradePlan(id: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/upgrade/plans/${encodeURIComponent(id)}/cancel`, {});
}

export async function verifyUpgradePlan(id: string): Promise<{ ok: boolean; healthy: boolean }> {
  return postJSON<{ ok: boolean; healthy: boolean }>(`/upgrade/plans/${encodeURIComponent(id)}/verify`, {});
}

export async function deleteUpgradePlan(id: string): Promise<{ ok: boolean }> {
  return deleteJSON<{ ok: boolean }>(`/upgrade/plans/${encodeURIComponent(id)}`);
}
