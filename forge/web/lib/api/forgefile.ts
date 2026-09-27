import { fetchJSON, postJSON, unwrapData, unwrapList } from "./http";

export type ManifestProject = { name: string; slug: string };
export type Manifest = {
  project: ManifestProject;
  deploy: Array<{
    name: string;
    type: string;
    source: { repo: string; branch: string; root: string };
    build: { builder: string; dockerfile: string };
    ports: number[];
    env: Record<string, string>;
    resources: { cpu: string; memory: number; replicas: number };
    kind?: string;
    version?: string;
  }>;
  database?: { name: string; kind: string; version: string } | null;
  environments: string[];
};

export type ApplyResult = {
  projectSlug: string;
  projectName: string;
  version: number;
  apps: Array<{ appId: string; appName: string; serviceId: string; domain: string; url: string; status: string }>;
  links: Record<string, string>;
  warnings: string[];
  appliedAt: string;
};

export type ValidateResult = { valid: boolean; warnings: string[]; error?: string };

export async function validateForgefile(content: string): Promise<ValidateResult> {
  try {
    const res = await postJSON<ValidateResult>("/forgefile/validate", { content });
    return res;
  } catch (err) {
    // Validation endpoint returns 400 with {valid:false, error:"..."} — surface that instead of throwing generic
    if (err instanceof Error && err.message.includes("valid")) {
      try {
        return JSON.parse(err.message) as ValidateResult;
      } catch {
        // fall through
      }
    }
    throw err;
  }
}

export async function applyForgefile(content: string): Promise<ApplyResult> {
  const res = await postJSON<{ data: ApplyResult } | ApplyResult>("/forgefile/apply", { content });
  return unwrapData(res);
}

export async function listManifests(): Promise<string[]> {
  const res = await fetchJSON<{ manifests: string[] } | string[] | { data: string[] } | { data: { manifests: string[] } }>("/forgefile");
  if (Array.isArray(res)) return res;
  const obj = res as Record<string, unknown>;
  if (Array.isArray(obj.manifests)) return obj.manifests as string[];
  if (obj.data && typeof obj.data === 'object' && Array.isArray((obj.data as Record<string, unknown>).manifests)) {
    return (obj.data as { manifests: string[] }).manifests;
  }
  if (obj.data && Array.isArray(obj.data)) return unwrapList(obj.data as string[] | { data: string[] });
  return [];
}

export async function getManifest(slug: string): Promise<{ slug: string; version: number; updatedAt: string; manifest: Manifest }> {
  type ManifestResult = { slug: string; version: number; updatedAt: string; manifest: Manifest };
  const res = await fetchJSON<ManifestResult | { data: ManifestResult }>(`/forgefile/${encodeURIComponent(slug)}`);
  // Handler returns { data: ... } or flat — handle both
  return unwrapData(res);
}
