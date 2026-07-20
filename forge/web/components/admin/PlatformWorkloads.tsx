"use client";

import { type UseMutationResult, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AppWindow, Boxes, FileCode2, GitBranch, RefreshCw, Server } from "lucide-react";
import { createPlatformApplication, fetchDefaultPlatformScope, fetchPlatformApplication, fetchPlatformApplications, fetchPlatformOperations, validatePlatformCompose, type PlatformApplicationDetail, type PlatformComposeValidation } from "@/modules/platform/api";
import { fetchNodes } from "@/modules/infrastructure/api";
import { useState } from "react";

export function AppManagement() {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [image, setImage] = useState("");
  const [source, setSource] = useState<"image" | "git">("image");
  const [repositoryUrl, setRepositoryUrl] = useState("");
  const [branch, setBranch] = useState("");
  const [baseDirectory, setBaseDirectory] = useState("");
  const [dockerfilePath, setDockerfilePath] = useState("");
  const [nodeId, setNodeId] = useState("");
  const [selectedApplicationId, setSelectedApplicationId] = useState<string | null>(null);
  const [composeContent, setComposeContent] = useState("services:\n  web:\n    image: nginx:alpine\n    ports:\n      - \"8080:80\"\n");
  const scope = useQuery({ queryKey: ["platform-default-scope"], queryFn: fetchDefaultPlatformScope });
  const applications = useQuery({ queryKey: ["platform-applications"], queryFn: () => fetchPlatformApplications() });
  const operations = useQuery({ queryKey: ["platform-operations"], queryFn: () => fetchPlatformOperations() });
  const nodes = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });
  const applicationDetail = useQuery({ queryKey: ["platform-application", selectedApplicationId], queryFn: () => fetchPlatformApplication(selectedApplicationId!), enabled: Boolean(selectedApplicationId) });
  const createApplication = useMutation({
    mutationFn: createPlatformApplication,
    onSuccess: async () => {
      setName("");
      setImage("");
      setRepositoryUrl("");
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["platform-applications"] }),
        queryClient.invalidateQueries({ queryKey: ["platform-operations"] }),
      ]);
    },
  });
  const submit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const targetEnvironment = scope.data?.environmentId;
    if (!targetEnvironment) return;
    createApplication.mutate({ environmentId: targetEnvironment, nodeId, name, image: source === "image" ? image : undefined, repositoryUrl: source === "git" ? repositoryUrl : undefined, branch: source === "git" ? branch || undefined : undefined, baseDirectory: source === "git" ? baseDirectory || undefined : undefined, dockerfilePath: source === "git" ? dockerfilePath || undefined : undefined, source, deployment: "rolling" });
  };
  const validateCompose = useMutation({ mutationFn: validatePlatformCompose });

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-xs font-bold uppercase tracking-[0.2em] text-red-400">App management</p>
          <h1 className="mt-1 text-2xl font-bold text-slate-100">Applications</h1>
          <p className="mt-2 max-w-2xl text-sm text-slate-400">Deploy images or public Git repositories through Forge’s durable operation engine. The API records desired state, the worker dispatches to Beacon, and observed state returns here.</p>
        </div>
        <button className="inline-flex items-center gap-2 rounded-lg border border-white/10 px-3 py-2 text-sm text-slate-200 hover:bg-white/5" onClick={() => void applications.refetch()} type="button">
          <RefreshCw size={15} className={applications.isFetching ? "animate-spin" : ""} /> Refresh
        </button>
      </div>

      <div className="grid gap-px overflow-hidden rounded-xl border border-white/10 bg-white/10 text-sm sm:grid-cols-3">
        <DeploymentPathStep index="1" title="Forge API" description="Stores the desired application revision." />
        <DeploymentPathStep index="2" title="Operation worker" description="Queues and retries the deployment safely." />
        <DeploymentPathStep index="3" title="Beacon" description="Builds or runs it, then reports observed state." />
      </div>

      <form className="grid gap-3 rounded-xl border border-white/10 bg-[#151920] p-4 md:grid-cols-[1fr_1fr_2fr_auto]" onSubmit={submit}>
        <div className="md:col-span-4 flex flex-wrap gap-2 border-b border-white/10 pb-3">
          <button onClick={() => setSource("image")} type="button" className={`inline-flex items-center gap-2 rounded-md px-3 py-1.5 text-xs font-semibold ${source === "image" ? "bg-red-600 text-white" : "bg-white/5 text-slate-300 hover:bg-white/10"}`}><Boxes size={14} /> Image</button>
          <button onClick={() => setSource("git")} type="button" className={`inline-flex items-center gap-2 rounded-md px-3 py-1.5 text-xs font-semibold ${source === "git" ? "bg-red-600 text-white" : "bg-white/5 text-slate-300 hover:bg-white/10"}`}><GitBranch size={14} /> Git + Dockerfile</button>
          <p className="self-center text-xs text-slate-500">Public HTTPS repositories only; private Git credentials and buildpacks follow in a later capability.</p>
        </div>
        <label className="grid gap-1 text-xs text-slate-400">Application name
          <input required value={name} onChange={(event) => setName(event.target.value)} placeholder="my-api" className="rounded-md border border-white/10 bg-[#0f1419] px-3 py-2 text-sm text-slate-100 outline-none focus:border-red-500" />
        </label>
        <label className="grid gap-1 text-xs text-slate-400">{source === "image" ? "Container image" : "Public HTTPS repository"}
          <input required value={source === "image" ? image : repositoryUrl} onChange={(event) => source === "image" ? setImage(event.target.value) : setRepositoryUrl(event.target.value)} placeholder={source === "image" ? "ghcr.io/org/my-api:latest" : "https://github.com/org/my-api.git"} className="rounded-md border border-white/10 bg-[#0f1419] px-3 py-2 text-sm text-slate-100 outline-none focus:border-red-500" />
        </label>
        <label className="grid gap-1 text-xs text-slate-400">Beacon node
          <select required value={nodeId} onChange={(event) => setNodeId(event.target.value)} className="rounded-md border border-white/10 bg-[#0f1419] px-3 py-2 text-sm text-slate-100 outline-none focus:border-red-500" disabled={nodes.isPending}>
            <option value="">{nodes.isPending ? "Loading nodes…" : "Select a node"}</option>
            {nodes.data?.filter((node) => node.actualState !== "offline" && !node.draining && !node.maintenanceMode).map((node) => <option key={node.id} value={node.id}>{node.name}{node.actualState ? ` · ${node.actualState}` : ""}</option>)}
          </select>
        </label>
        <button disabled={createApplication.isPending || !scope.data?.environmentId || !nodeId} className="self-end rounded-md bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-500 disabled:cursor-not-allowed disabled:opacity-50" type="submit">{createApplication.isPending ? "Creating…" : "Deploy app"}</button>
        {nodes.isError ? <p className="md:col-span-4 text-sm text-amber-200">Unable to load deployable Beacon nodes.</p> : null}
        {source === "git" ? <div className="md:col-span-4 grid gap-3 rounded-lg border border-white/[0.06] bg-black/10 p-3 md:grid-cols-3">
          <label className="grid gap-1 text-xs text-slate-400">Branch <input value={branch} onChange={(event) => setBranch(event.target.value)} placeholder="default branch" className="rounded-md border border-white/10 bg-[#0f1419] px-3 py-2 text-sm text-slate-100 outline-none focus:border-red-500" /></label>
          <label className="grid gap-1 text-xs text-slate-400">Base directory <input value={baseDirectory} onChange={(event) => setBaseDirectory(event.target.value)} placeholder="." className="rounded-md border border-white/10 bg-[#0f1419] px-3 py-2 text-sm text-slate-100 outline-none focus:border-red-500" /></label>
          <label className="grid gap-1 text-xs text-slate-400">Dockerfile <input value={dockerfilePath} onChange={(event) => setDockerfilePath(event.target.value)} placeholder="Dockerfile" className="rounded-md border border-white/10 bg-[#0f1419] px-3 py-2 text-sm text-slate-100 outline-none focus:border-red-500" /></label>
        </div> : null}
        {createApplication.isError ? <p className="md:col-span-4 text-sm text-red-300">{createApplication.error instanceof Error ? createApplication.error.message : "Unable to create application"}</p> : null}
      </form>

      <ComposeValidator content={composeContent} onChange={setComposeContent} mutation={validateCompose} />

      {applications.isError ? (
        <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-200">Unable to load applications. Apply the platform migrations and verify the API is online.</div>
      ) : applications.isPending ? (
        <div className="rounded-xl border border-white/10 bg-[#151920] p-8 text-sm text-slate-400">Loading applications…</div>
      ) : applications.data?.length === 0 ? (
        <div className="rounded-xl border border-dashed border-white/15 bg-[#151920] p-10 text-center text-slate-400"><AppWindow className="mx-auto mb-3" size={28} /><p>No applications yet.</p><p className="mt-1 text-xs">Deploy an image or public Git repository above. Game servers remain in their dedicated console experience.</p></div>
      ) : (
        <div className="overflow-hidden rounded-xl border border-white/10 bg-[#151920]">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-white/10 text-xs uppercase tracking-wide text-slate-500"><tr><th className="px-4 py-3">Application</th><th className="px-4 py-3">Desired</th><th className="px-4 py-3">Observed</th><th className="px-4 py-3">Revision</th><th className="px-4 py-3" /></tr></thead>
            <tbody>{applications.data?.map((application) => <tr key={application.id} className={`border-b border-white/[0.06] last:border-0 ${selectedApplicationId === application.id ? "bg-red-500/[0.06]" : ""}`}><td className="px-4 py-3"><button className="text-left font-medium text-slate-100 hover:text-red-300" onClick={() => setSelectedApplicationId(application.id)} type="button">{application.name}</button><p className="mt-0.5 font-mono text-[11px] text-slate-500">{application.id}</p></td><td className="px-4 py-3 text-slate-300">{application.desiredState}</td><td className="px-4 py-3"><StateBadge state={application.observedState} /></td><td className="px-4 py-3 text-slate-400">{application.observedGeneration} / {application.desiredGeneration}</td><td className="px-4 py-3 text-right"><button className="text-xs font-semibold text-red-300 hover:text-red-200" onClick={() => setSelectedApplicationId(application.id)} type="button">Manage</button></td></tr>)}</tbody>
          </table>
        </div>
      )}

      {selectedApplicationId ? <ApplicationDetail detail={applicationDetail.data} error={applicationDetail.error} loading={applicationDetail.isPending} nodes={nodes.data ?? []} onClose={() => setSelectedApplicationId(null)} /> : null}

      <section className="overflow-hidden rounded-xl border border-white/10 bg-[#151920]">
        <div className="flex items-center justify-between border-b border-white/10 px-4 py-3">
          <div><h2 className="font-semibold text-slate-100">Recent operations</h2><p className="mt-0.5 text-xs text-slate-500">Durable intent and execution progress.</p></div>
          <button className="text-xs text-slate-400 hover:text-slate-200" onClick={() => void operations.refetch()} type="button">Refresh</button>
        </div>
        {operations.isPending ? <p className="p-4 text-sm text-slate-400">Loading operations…</p> : operations.isError ? <p className="p-4 text-sm text-red-300">Unable to load operations.</p> : <ApplicationOperations operations={(operations.data ?? []).filter((operation) => applicationIds(applications.data).has(operation.resourceId))} />}
      </section>
    </div>
  );
}

function applicationIds(applications: Array<{ id: string }> | undefined) { return new Set((applications ?? []).map((application) => application.id)); }

function DeploymentPathStep({ index, title, description }: { index: string; title: string; description: string }) {
  return <div className="flex gap-3 bg-[#151920] p-3"><span className="grid h-6 w-6 shrink-0 place-items-center rounded-full bg-red-500/10 text-xs font-bold text-red-300">{index}</span><div><p className="font-medium text-slate-200">{title}</p><p className="mt-0.5 text-xs text-slate-500">{description}</p></div></div>;
}

function StateBadge({ state }: { state: string }) { const tone = state === "running" ? "bg-emerald-500/10 text-emerald-300" : state === "failed" ? "bg-red-500/10 text-red-300" : "bg-amber-500/10 text-amber-200"; return <span className={`rounded-full px-2 py-1 text-xs font-medium ${tone}`}>{state}</span>; }

function ApplicationOperations({ operations }: { operations: Array<{ id: string; kind: string; resourceType: string; resourceId: string; status: string; error?: string }> }) { return operations.length === 0 ? <p className="p-4 text-sm text-slate-400">No application operations yet.</p> : <div className="divide-y divide-white/[0.06]">{operations.map((operation) => <div className="flex items-center justify-between gap-4 px-4 py-3" key={operation.id}><div className="min-w-0"><p className="truncate text-sm font-medium text-slate-200">{operation.kind}</p><p className="mt-0.5 truncate text-xs text-slate-500">{operation.resourceType} · {operation.resourceId}</p>{operation.error ? <p className="mt-1 text-xs text-red-300">{operation.error}</p> : null}</div><StateBadge state={operation.status} /></div>)}</div>; }

function ApplicationDetail({ detail, error, loading, nodes, onClose }: { detail?: PlatformApplicationDetail; error: Error | null; loading: boolean; nodes: Array<{ id: string; name: string }>; onClose: () => void }) {
  if (loading) return <section className="rounded-xl border border-white/10 bg-[#151920] p-6 text-sm text-slate-400">Loading application details…</section>;
  if (error || !detail) return <section className="rounded-xl border border-red-500/30 bg-red-500/10 p-6 text-sm text-red-200">Unable to load this application’s deployment state.</section>;
  const spec = isRecord(detail.revision.spec) ? detail.revision.spec : {};
  const source = typeof spec.source === "string" ? spec.source : "unknown";
  const sourceValue = source === "git" ? String(spec.repositoryUrl ?? "") : String(spec.image ?? "");
  const nodeName = (nodeId?: string) => nodes.find((node) => node.id === nodeId)?.name ?? nodeId ?? "Unassigned";
  return <section className="overflow-hidden rounded-xl border border-red-500/20 bg-[#151920]"><div className="flex items-start justify-between gap-4 border-b border-white/10 px-4 py-4"><div><p className="text-xs font-bold uppercase tracking-[0.18em] text-red-400">Application detail</p><h2 className="mt-1 text-lg font-semibold text-slate-100">{detail.application.name}</h2><p className="mt-1 break-all font-mono text-xs text-slate-500">{source} · {sourceValue || "source not recorded"}</p></div><button onClick={onClose} className="rounded-md border border-white/10 px-3 py-1.5 text-xs text-slate-300 hover:bg-white/5" type="button">Close</button></div><div className="grid gap-4 p-4 lg:grid-cols-[1.1fr_.9fr]"><div className="space-y-3"><h3 className="text-sm font-semibold text-slate-100">Instances</h3>{detail.instances.length === 0 ? <p className="rounded-lg border border-dashed border-white/10 p-4 text-sm text-slate-500">No instance has been recorded yet. The queued deployment will create one on Beacon.</p> : detail.instances.map((instance) => <div className="flex items-center justify-between rounded-lg border border-white/[0.07] bg-black/10 p-3" key={instance.id}><div className="min-w-0"><p className="flex items-center gap-2 text-sm font-medium text-slate-200"><Server size={14} />{nodeName(instance.nodeId)}</p><p className="mt-1 font-mono text-xs text-slate-500">revision {instance.revisionId}</p></div><StateBadge state={instance.observedState} /></div>)}</div><div className="space-y-3"><h3 className="text-sm font-semibold text-slate-100">Deployment activity</h3><ApplicationOperations operations={detail.operations} /></div></div></section>;
}

function isRecord(value: unknown): value is Record<string, unknown> { return typeof value === "object" && value !== null && !Array.isArray(value); }

export const PlatformWorkloads = AppManagement;

function ComposeValidator({ content, onChange, mutation }: { content: string; onChange: (value: string) => void; mutation: UseMutationResult<PlatformComposeValidation, Error, { content: string }, unknown> }) {
  return <section className="rounded-xl border border-white/10 bg-[#151920] p-4">
    <div className="flex flex-wrap items-start justify-between gap-3"><div><p className="text-xs font-bold uppercase tracking-[0.18em] text-cyan-300">Compose import</p><h2 className="mt-1 font-semibold text-slate-100">Validate a Compose manifest</h2><p className="mt-1 max-w-2xl text-xs text-slate-500">Uses the official Compose Specification parser. Validation is available now; execution remains intentionally disabled until Beacon stack lifecycle policies are complete.</p></div><button type="button" onClick={() => mutation.mutate({ content })} disabled={mutation.isPending} className="inline-flex items-center gap-2 rounded-md border border-cyan-500/30 bg-cyan-500/10 px-3 py-2 text-sm font-semibold text-cyan-100 hover:bg-cyan-500/20 disabled:opacity-50"><FileCode2 size={15} />{mutation.isPending ? "Validating…" : "Validate"}</button></div>
    <textarea value={content} onChange={(event) => onChange(event.target.value)} spellCheck={false} className="mt-4 min-h-44 w-full rounded-lg border border-white/10 bg-[#0f1419] p-3 font-mono text-xs leading-5 text-slate-200 outline-none focus:border-cyan-500" aria-label="Compose manifest" />
    {mutation.isError ? <p className="mt-3 text-sm text-red-300">{mutation.error.message}</p> : null}
    {mutation.data ? <div className="mt-4 grid gap-3 rounded-lg border border-emerald-500/20 bg-emerald-500/[0.04] p-3 text-sm text-slate-300 md:grid-cols-3"><div><p className="text-xs uppercase tracking-wide text-slate-500">Services</p><p className="mt-1">{mutation.data.services.map((service) => service.name).join(", ") || "None"}</p></div><div><p className="text-xs uppercase tracking-wide text-slate-500">Networks</p><p className="mt-1">{mutation.data.networks.join(", ") || "Default"}</p></div><div><p className="text-xs uppercase tracking-wide text-slate-500">Volumes</p><p className="mt-1">{mutation.data.volumes.join(", ") || "None"}</p></div>{mutation.data.warnings?.map((warning) => <p className="md:col-span-3 text-xs text-amber-200" key={warning}>{warning}</p>)}</div> : null}
  </section>;
}
