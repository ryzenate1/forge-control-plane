import { fetchJSON, postJSON, putJSON, deleteJSON } from "./http";

// Pipeline (CI/CD) client — mirrors forge/api/internal/services/pipeline model
// types and the routes mounted by internal/http/phase5_registrar.go. All calls
// go through the shared http primitives and unwrap the `{ data }` envelope the
// admin pipeline endpoints return.

export type PipelineTrigger = {
  type: string;
  cron?: string;
  enabled?: boolean;
};

export type PipelineStage = {
  name: string;
  action: string;
  config?: Record<string, unknown>;
  timeoutSec?: number;
  continueOnFailure?: boolean;
};

export type PipelineDefinition = {
  id: string;
  name: string;
  description?: string;
  categories?: string[];
  stages: PipelineStage[];
  trigger?: PipelineTrigger;
  createdBy?: string;
  createdAt?: string;
  updatedAt?: string;
};

export type PipelineStageRun = {
  id: string;
  runId: string;
  pipelineId: string;
  position: number;
  name: string;
  action: string;
  status: string;
  attempts: number;
  error?: string;
  startedAt?: string | null;
  finishedAt?: string | null;
};

export type PipelineRun = {
  id: string;
  pipelineId: string;
  pipelineName?: string;
  trigger: string;
  status: string;
  progressPct?: number;
  currentStage?: string;
  error?: string;
  retryOf?: string | null;
  retryCount?: number;
  requestedBy?: string;
  createdAt: string;
  startedAt?: string | null;
  finishedAt?: string | null;
  stages?: PipelineStageRun[];
};

export type PipelineLogEntry = {
  id: number;
  runId: string;
  stageId?: string;
  level: string;
  message: string;
  timestamp: string;
};

export type PipelineArtifact = {
  id: string;
  runId: string;
  stageId?: string;
  name: string;
  relativePath: string;
  sizeBytes: number;
  contentType: string;
  createdBy?: string;
  createdAt: string;
};

export type CreatePipelineInput = {
  name: string;
  description?: string;
  categories?: string[];
  stages: PipelineStage[];
  trigger?: PipelineTrigger;
};

async function data<T>(promise: Promise<T | { data: T }>): Promise<T> {
  const res = await promise;
  if (res && typeof res === "object" && "data" in (res as Record<string, unknown>)) {
    return (res as { data: T }).data;
  }
  return res as T;
}

// ---- definitions ----

export async function listPipelines(): Promise<PipelineDefinition[]> {
  const res = await fetchJSON<{ data: PipelineDefinition[] }>("/pipelines");
  return res.data ?? [];
}

export async function getPipeline(id: string): Promise<PipelineDefinition> {
  return data(fetchJSON<PipelineDefinition | { data: PipelineDefinition }>(`/pipelines/${encodeURIComponent(id)}`));
}

export async function createPipeline(input: CreatePipelineInput): Promise<PipelineDefinition> {
  return data(postJSON<PipelineDefinition | { data: PipelineDefinition }>("/pipelines", input));
}

export async function updatePipeline(id: string, input: CreatePipelineInput): Promise<PipelineDefinition> {
  return data(putJSON<PipelineDefinition | { data: PipelineDefinition }>(`/pipelines/${encodeURIComponent(id)}`, input));
}

export async function deletePipeline(id: string): Promise<{ ok: boolean }> {
  return deleteJSON<{ ok: boolean }>(`/pipelines/${encodeURIComponent(id)}`);
}

// ---- runs ----

export async function triggerPipelineRun(id: string, trigger = "manual"): Promise<PipelineRun> {
  return data(
    postJSON<PipelineRun | { data: PipelineRun }>(`/pipelines/${encodeURIComponent(id)}/runs`, { trigger }),
  );
}

export async function listPipelineRuns(opts?: { pipelineId?: string; status?: string; limit?: number }): Promise<PipelineRun[]> {
  const q = new URLSearchParams();
  if (opts?.pipelineId) q.set("pipelineId", opts.pipelineId);
  if (opts?.status) q.set("status", opts.status);
  if (opts?.limit) q.set("limit", String(opts.limit));
  const suffix = q.toString() ? `?${q.toString()}` : "";
  const res = await fetchJSON<{ data: PipelineRun[] }>(`/pipeline-runs${suffix}`);
  return res.data ?? [];
}

export async function getPipelineRun(runId: string): Promise<PipelineRun> {
  return data(fetchJSON<PipelineRun | { data: PipelineRun }>(`/pipeline-runs/${encodeURIComponent(runId)}`));
}

export async function listPipelineRunLogs(runId: string, after = 0): Promise<PipelineLogEntry[]> {
  const q = after > 0 ? `?after=${after}` : "";
  const res = await fetchJSON<{ data: PipelineLogEntry[] }>(`/pipeline-runs/${encodeURIComponent(runId)}/logs${q}`);
  return res.data ?? [];
}

export async function cancelPipelineRun(runId: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/pipeline-runs/${encodeURIComponent(runId)}/cancel`);
}

export async function retryPipelineRun(runId: string): Promise<PipelineRun> {
  return data(postJSON<PipelineRun | { data: PipelineRun }>(`/pipeline-runs/${encodeURIComponent(runId)}/retry`));
}

export async function approvePipelineStage(runId: string, stageId: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(
    `/pipeline-runs/${encodeURIComponent(runId)}/stages/${encodeURIComponent(stageId)}/approve`,
  );
}

export async function rejectPipelineStage(runId: string, stageId: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(
    `/pipeline-runs/${encodeURIComponent(runId)}/stages/${encodeURIComponent(stageId)}/reject`,
  );
}

export async function listPipelineArtifacts(runId: string): Promise<PipelineArtifact[]> {
  const res = await fetchJSON<{ data: PipelineArtifact[] }>(`/pipeline-runs/${encodeURIComponent(runId)}/artifacts`);
  return res.data ?? [];
}

export function pipelineArtifactDownloadUrl(artifactId: string): string {
  return `/pipeline-artifacts/${encodeURIComponent(artifactId)}`;
}
