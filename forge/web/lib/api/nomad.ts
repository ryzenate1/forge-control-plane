import { fetchJSON, postJSON } from "./http";

// Types mirror the subset of the Nomad HTTP API shapes surfaced by the
// /admin/nomad/* endpoints (see forge/api/internal/services/nomad/service.go).

export type NomadJob = {
  ID: string;
  Name: string;
  Namespace?: string;
  Status: string;
  StatusDescription?: string;
  Type?: string;
  Priority?: number;
  Datacenter?: string;
  SubmitTime?: number;
  CreateTime?: number;
  UpdateTime?: number;
  Meta?: Record<string, string>;
};

export type NomadAllocation = {
  ID: string;
  Name?: string;
  NodeID?: string;
  JobID?: string;
  TaskGroup?: string;
  Namespace?: string;
  ClientStatus?: string;
  ClientDescription?: string;
  DesiredStatus?: string;
  DeployID?: string;
  CreatedAt?: number;
};

export type NomadNode = {
  ID: string;
  Name?: string;
  HTTPAddr?: string;
  NodeClass?: string;
  NodePool?: string;
  Version?: string;
  Status?: string;
  Drain?: boolean;
  Eligibility?: string;
};

export type NomadDeployment = {
  ID: string;
  JobID?: string;
  Namespace?: string;
  JobVersion?: number;
  Status?: string;
  StatusDescription?: string;
  DesiredStatus?: string;
  Canary?: boolean;
  Pause?: boolean;
  CreatedAt?: number;
};

export function fetchNomadJobs(namespace?: string) {
  const query = namespace ? `?namespace=${encodeURIComponent(namespace)}` : "";
  return fetchJSON<{ jobs: NomadJob[] }>(`/admin/nomad/jobs${query}`).then((r) => r.jobs ?? []);
}

// submitNomadJob registers a job. Pass the Nomad job document as a JSON object
// (the HTTP API does not parse HCL — render HCL to JSON first).
export function submitNomadJob(spec: Record<string, unknown>) {
  return postJSON<{ ok: boolean; job: NomadJob }>("/admin/nomad/jobs", spec);
}

export function fetchNomadJob(id: string) {
  return fetchJSON<NomadJob>(`/admin/nomad/jobs/${encodeURIComponent(id)}`);
}

export function stopNomadJob(id: string, purge = false) {
  return postJSON<{ ok: boolean; evalId?: string }>(`/admin/nomad/jobs/${encodeURIComponent(id)}/stop${purge ? "?purge=true" : ""}`);
}

export function fetchNomadAllocations(jobId?: string) {
  const query = jobId ? `?jobId=${encodeURIComponent(jobId)}` : "";
  return fetchJSON<{ allocations: NomadAllocation[] }>(`/admin/nomad/allocations${query}`).then((r) => r.allocations ?? []);
}

export function promoteNomadAllocation(allocId: string) {
  return postJSON<{ ok: boolean; deployment?: string }>(`/admin/nomad/allocations/${encodeURIComponent(allocId)}/promote`);
}

export function fetchNomadNodes() {
  return fetchJSON<{ nodes: NomadNode[] }>("/admin/nomad/nodes").then((r) => r.nodes ?? []);
}

export function fetchNomadNode(id: string) {
  return fetchJSON<NomadNode>(`/admin/nomad/nodes/${encodeURIComponent(id)}`);
}

export function drainNomadNode(id: string, drain = true) {
  return postJSON<{ ok: boolean; node: string; drain: boolean }>(`/admin/nomad/nodes/${encodeURIComponent(id)}/drain`, { drain });
}

export function fetchNomadDeployments() {
  return fetchJSON<{ deployments: NomadDeployment[] }>("/admin/nomad/deployments").then((r) => r.deployments ?? []);
}

export function fetchNomadDeployment(id: string) {
  return fetchJSON<NomadDeployment>(`/admin/nomad/deployments/${encodeURIComponent(id)}`);
}
