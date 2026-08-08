import { fetchJSON, postJSON } from "./http";

export type PreviewDeployment = {
  id: string;
  serverId: string;
  serviceId?: string;
  prNumber: number;
  prTitle?: string;
  prUrl?: string;
  branch?: string;
  repoOwner?: string;
  repoName?: string;
  commitSha?: string;
  status: "deploying" | "running" | "stopped" | "failed" | "cleaned_up";
  previewUrl?: string;
  deploymentUrl?: string;
  source: "github" | "gitlab";
  uniqueSuffix?: string;
  isIsolated: boolean;
  createdBy?: string;
  createdAt: string;
  updatedAt: string;
  cleanedAt?: string;
};

export async function fetchPreviewDeployments(): Promise<PreviewDeployment[]> {
  return fetchJSON<{ data: PreviewDeployment[] }>("/admin/preview-deployments").then(r => r.data);
}

export async function fetchPreviewDeployment(id: string): Promise<PreviewDeployment> {
  return fetchJSON<{ data: PreviewDeployment }>(`/admin/preview-deployments/${encodeURIComponent(id)}`).then(r => r.data);
}

export async function createPreviewDeployment(data: {
  serverId: string;
  prNumber: number;
  prTitle?: string;
  prUrl?: string;
  branch?: string;
  repoOwner?: string;
  repoName?: string;
  commitSha?: string;
  source?: string;
}): Promise<PreviewDeployment> {
  return postJSON<{ data: PreviewDeployment }>("/admin/preview-deployments", data).then(r => r.data);
}

export async function deployPreview(id: string): Promise<void> {
  await postJSON(`/admin/preview-deployments/${encodeURIComponent(id)}/deploy`);
}

export async function cleanupPreview(id: string): Promise<void> {
  await postJSON(`/admin/preview-deployments/${encodeURIComponent(id)}/cleanup`);
}

export async function updatePreviewStatus(id: string, status: string): Promise<void> {
  await postJSON(`/admin/preview-deployments/${encodeURIComponent(id)}/status`, { status });
}
