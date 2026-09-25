import { fetchJSON, postJSON, requestJSON } from "./http";

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

// ---------------------------------------------------------------------------
// Project-scoped preview environments
//
// Separate surface from the legacy per-server `/admin/preview-deployments`
// routes above: these are backed by `preview_environments`, address a project,
// and are gated by organization membership on the server. Every request goes
// through `requestJSON` so CSRF signing, cookie credentials and the 401 session
// signal stay in one place.
// ---------------------------------------------------------------------------

/** Lifecycle states persisted by the API (CHECK-constrained, so exact). */
export type ProjectPreviewStatus =
  | "pending"
  | "deploying"
  | "active"
  | "expired"
  | "failed"
  | "teardown";

export type ProjectPreviewDeployment = {
  id: string;
  projectId: string;
  environmentId?: string | null;
  serverId?: string | null;
  branch: string;
  prNumber?: number;
  commitSha?: string;
  /** Subdomain label; the preview is reachable at `<slug>.<baseDomain>`. */
  slug: string;
  status: ProjectPreviewStatus;
  url?: string;
  composeContent?: string;
  stackId?: string;
  source: string;
  title?: string;
  prUrl?: string;
  closeReason?: string;
  error?: string;
  expiresAt?: string;
  createdAt: string;
  updatedAt: string;
  createdBy?: string | null;
};

export type ProjectPreviewConfig = {
  baseDomain: string;
  ttlSeconds: number;
  /** Effective live-preview cap for one project; `<= 0` means uncapped. */
  maxPerProject: number;
  webhookUrl: string;
};

export type ProjectPreviewWebhookSecret = {
  secret: string;
  webhookUrl: string;
};

export type CreateProjectPreviewInput = {
  branch: string;
  prNumber?: number;
  commitSha?: string;
  title?: string;
  prUrl?: string;
  /** Compose document to deploy; omit to clone the project's base stack. */
  composeContent?: string;
  /** Compose stack in this project to use as the preview source. */
  baseStackId?: string;
  serverId?: string;
  environmentId?: string;
  /** Overrides the platform TTL for this preview only. */
  ttlSeconds?: number;
};

const PROJECT_PREVIEW_JSON_HEADERS: Record<string, string> = { "Content-Type": "application/json" };

function projectPreviewsPath(projectId: string, ...segments: string[]): string {
  const parts = [`/projects/${encodeURIComponent(projectId)}/previews`];
  for (const segment of segments) {
    if (segment) parts.push(encodeURIComponent(segment));
  }
  return parts.join("/");
}

function envelopeError(body: unknown): string | null {
  if (!body || typeof body !== "object") return null;
  const record = body as { error?: unknown; message?: unknown };
  if (typeof record.message === "string" && record.message.trim()) return record.message;
  if (typeof record.error === "string" && record.error.trim()) return record.error;
  return null;
}

/**
 * Unwraps the `{ data: ... }` envelope the preview routes return. A missing
 * `data` is not silently converted to a value the caller will misread: callers
 * that need an object throw, list callers treat it as empty.
 */
function envelopeData(body: unknown): unknown {
  if (body === undefined || body === null) return null;
  if (typeof body !== "object") return body;
  const message = envelopeError(body);
  if (message) throw new Error(message);
  if ("data" in (body as Record<string, unknown>)) {
    return (body as Record<string, unknown>).data ?? null;
  }
  return body;
}

function envelopeObject<T>(body: unknown, resource: string): T {
  const data = envelopeData(body);
  if (data === null || typeof data !== "object") {
    throw new Error(`The ${resource} response did not contain a payload.`);
  }
  return data as T;
}

function envelopeList<T>(body: unknown): T[] {
  const data = envelopeData(body);
  return Array.isArray(data) ? (data as T[]) : [];
}

export async function fetchProjectPreviewConfig(projectId: string): Promise<ProjectPreviewConfig> {
  const body = await requestJSON<unknown>(projectPreviewsPath(projectId, "config"));
  return envelopeObject<ProjectPreviewConfig>(body, "preview config");
}

export async function listProjectPreviews(projectId: string): Promise<ProjectPreviewDeployment[]> {
  const body = await requestJSON<unknown>(projectPreviewsPath(projectId));
  return envelopeList<ProjectPreviewDeployment>(body);
}

export async function getProjectPreview(projectId: string, previewId: string): Promise<ProjectPreviewDeployment> {
  const body = await requestJSON<unknown>(projectPreviewsPath(projectId, previewId));
  return envelopeObject<ProjectPreviewDeployment>(body, "preview deployment");
}

/**
 * Creates a preview. The API returns 201 with status `pending`: the row is
 * durable but the deployment is still running in the background, so the caller
 * must keep polling rather than assume the environment is live.
 */
export async function createProjectPreview(
  projectId: string,
  input: CreateProjectPreviewInput,
): Promise<ProjectPreviewDeployment> {
  const body = await requestJSON<unknown>(projectPreviewsPath(projectId), {
    method: "POST",
    headers: PROJECT_PREVIEW_JSON_HEADERS,
    body: JSON.stringify(input),
  });
  return envelopeObject<ProjectPreviewDeployment>(body, "preview deployment");
}

/** Queues a fresh deployment of an existing preview; returns the updated row. */
export async function redeployProjectPreview(
  projectId: string,
  previewId: string,
): Promise<ProjectPreviewDeployment> {
  const body = await requestJSON<unknown>(projectPreviewsPath(projectId, previewId, "redeploy"), {
    method: "POST",
    headers: PROJECT_PREVIEW_JSON_HEADERS,
  });
  return envelopeObject<ProjectPreviewDeployment>(body, "preview deployment");
}

/** Tears the environment down but keeps the row as an audit trail. */
export async function closeProjectPreview(
  projectId: string,
  previewId: string,
  reason?: string,
): Promise<{ id: string; status: ProjectPreviewStatus }> {
  const body = await requestJSON<unknown>(projectPreviewsPath(projectId, previewId, "close"), {
    method: "POST",
    headers: PROJECT_PREVIEW_JSON_HEADERS,
    body: JSON.stringify({ reason: reason ?? "" }),
  });
  return envelopeObject<{ id: string; status: ProjectPreviewStatus }>(body, "preview close");
}

/** Tears the environment down and removes the row. */
export async function deleteProjectPreview(projectId: string, previewId: string): Promise<void> {
  const body = await requestJSON<unknown>(projectPreviewsPath(projectId, previewId), { method: "DELETE" });
  const data = envelopeData(body);
  if (data && typeof data === "object" && "ok" in (data as Record<string, unknown>)) return;
  throw new Error("The preview delete response did not confirm removal.");
}

/** The signing secret doubles as the credential that can create previews. */
export async function fetchProjectPreviewWebhookSecret(projectId: string): Promise<ProjectPreviewWebhookSecret> {
  const body = await requestJSON<unknown>(projectPreviewsPath(projectId, "webhook-secret"));
  return envelopeObject<ProjectPreviewWebhookSecret>(body, "preview webhook secret");
}

/** Rotating invalidates the provider webhook configuration immediately. */
export async function rotateProjectPreviewWebhookSecret(projectId: string): Promise<ProjectPreviewWebhookSecret> {
  const body = await requestJSON<unknown>(projectPreviewsPath(projectId, "webhook-secret", "rotate"), {
    method: "POST",
    headers: PROJECT_PREVIEW_JSON_HEADERS,
  });
  return envelopeObject<ProjectPreviewWebhookSecret>(body, "preview webhook secret");
}
