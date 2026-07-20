"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Boxes, RefreshCw } from "lucide-react";
import { createPlatformApplication, fetchDefaultPlatformScope, fetchPlatformOperations, fetchPlatformWorkloads } from "@/modules/platform/api";
import { fetchNodes } from "@/modules/infrastructure/api";
import { useState } from "react";

export function PlatformWorkloads() {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [image, setImage] = useState("");
  const [nodeId, setNodeId] = useState("");
  const scope = useQuery({ queryKey: ["platform-default-scope"], queryFn: fetchDefaultPlatformScope });
  const workloads = useQuery({ queryKey: ["platform-workloads"], queryFn: () => fetchPlatformWorkloads() });
  const operations = useQuery({ queryKey: ["platform-operations"], queryFn: () => fetchPlatformOperations() });
  const nodes = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });
  const createApplication = useMutation({
    mutationFn: createPlatformApplication,
    onSuccess: async () => {
      setName("");
      setImage("");
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["platform-workloads"] }),
        queryClient.invalidateQueries({ queryKey: ["platform-operations"] }),
      ]);
    },
  });
  const submit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const targetEnvironment = scope.data?.environmentId;
    if (!targetEnvironment) return;
    createApplication.mutate({ environmentId: targetEnvironment, nodeId, name, image, source: "image", deployment: "rolling" });
  };

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-xs font-bold uppercase tracking-[0.2em] text-red-400">Control plane</p>
          <h1 className="mt-1 text-2xl font-bold text-slate-100">Workloads</h1>
          <p className="mt-2 max-w-2xl text-sm text-slate-400">Canonical desired and observed state across applications, games, databases, and future workload modules. Image applications are deployed through a durable Forge operation to the selected Beacon node.</p>
        </div>
        <button className="inline-flex items-center gap-2 rounded-lg border border-white/10 px-3 py-2 text-sm text-slate-200 hover:bg-white/5" onClick={() => void workloads.refetch()} type="button">
          <RefreshCw size={15} className={workloads.isFetching ? "animate-spin" : ""} /> Refresh
        </button>
      </div>

      <form className="grid gap-3 rounded-xl border border-white/10 bg-[#151920] p-4 md:grid-cols-[1fr_1fr_2fr_auto]" onSubmit={submit}>
        <label className="grid gap-1 text-xs text-slate-400">Application name
          <input required value={name} onChange={(event) => setName(event.target.value)} placeholder="my-api" className="rounded-md border border-white/10 bg-[#0f1419] px-3 py-2 text-sm text-slate-100 outline-none focus:border-red-500" />
        </label>
        <label className="grid gap-1 text-xs text-slate-400">Container image
          <input required value={image} onChange={(event) => setImage(event.target.value)} placeholder="ghcr.io/org/my-api:latest" className="rounded-md border border-white/10 bg-[#0f1419] px-3 py-2 text-sm text-slate-100 outline-none focus:border-red-500" />
        </label>
        <label className="grid gap-1 text-xs text-slate-400">Beacon node
          <select required value={nodeId} onChange={(event) => setNodeId(event.target.value)} className="rounded-md border border-white/10 bg-[#0f1419] px-3 py-2 text-sm text-slate-100 outline-none focus:border-red-500" disabled={nodes.isPending}>
            <option value="">{nodes.isPending ? "Loading nodes…" : "Select a node"}</option>
            {nodes.data?.filter((node) => node.actualState !== "offline" && !node.draining && !node.maintenanceMode).map((node) => <option key={node.id} value={node.id}>{node.name}{node.actualState ? ` · ${node.actualState}` : ""}</option>)}
          </select>
        </label>
        <button disabled={createApplication.isPending || !scope.data?.environmentId || !nodeId} className="self-end rounded-md bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-500 disabled:cursor-not-allowed disabled:opacity-50" type="submit">{createApplication.isPending ? "Creating…" : "Deploy app"}</button>
        {nodes.isError ? <p className="md:col-span-4 text-sm text-amber-200">Unable to load deployable Beacon nodes.</p> : null}
        {createApplication.isError ? <p className="md:col-span-4 text-sm text-red-300">{createApplication.error instanceof Error ? createApplication.error.message : "Unable to create application"}</p> : null}
      </form>

      {workloads.isError ? (
        <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-200">Unable to load workloads. The platform foundation requires PostgreSQL migrations to be applied.</div>
      ) : workloads.isPending ? (
        <div className="rounded-xl border border-white/10 bg-[#151920] p-8 text-sm text-slate-400">Loading workloads…</div>
      ) : workloads.data?.length === 0 ? (
        <div className="rounded-xl border border-dashed border-white/15 bg-[#151920] p-10 text-center text-slate-400"><Boxes className="mx-auto mb-3" size={28} /><p>No canonical workloads yet.</p><p className="mt-1 text-xs">Game servers continue to work through the compatibility bridge while new modules adopt this model.</p></div>
      ) : (
        <div className="overflow-hidden rounded-xl border border-white/10 bg-[#151920]">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-white/10 text-xs uppercase tracking-wide text-slate-500"><tr><th className="px-4 py-3">Workload</th><th className="px-4 py-3">Kind</th><th className="px-4 py-3">Desired</th><th className="px-4 py-3">Observed</th><th className="px-4 py-3">Generation</th></tr></thead>
            <tbody>{workloads.data?.map((workload) => <tr key={workload.id} className="border-b border-white/[0.06] last:border-0"><td className="px-4 py-3 font-medium text-slate-100">{workload.name}</td><td className="px-4 py-3 text-slate-300">{workload.kind}</td><td className="px-4 py-3 text-slate-300">{workload.desiredState}</td><td className="px-4 py-3 text-slate-300">{workload.observedState}</td><td className="px-4 py-3 text-slate-400">{workload.observedGeneration} / {workload.desiredGeneration}</td></tr>)}</tbody>
          </table>
        </div>
      )}

      <section className="overflow-hidden rounded-xl border border-white/10 bg-[#151920]">
        <div className="flex items-center justify-between border-b border-white/10 px-4 py-3">
          <div><h2 className="font-semibold text-slate-100">Recent operations</h2><p className="mt-0.5 text-xs text-slate-500">Durable intent and execution progress.</p></div>
          <button className="text-xs text-slate-400 hover:text-slate-200" onClick={() => void operations.refetch()} type="button">Refresh</button>
        </div>
        {operations.isPending ? <p className="p-4 text-sm text-slate-400">Loading operations…</p> : operations.isError ? <p className="p-4 text-sm text-red-300">Unable to load operations.</p> : operations.data?.length === 0 ? <p className="p-4 text-sm text-slate-400">No platform operations yet.</p> : <div className="divide-y divide-white/[0.06]">{operations.data?.map((operation) => <div className="flex items-center justify-between gap-4 px-4 py-3" key={operation.id}><div className="min-w-0"><p className="truncate text-sm font-medium text-slate-200">{operation.kind}</p><p className="mt-0.5 truncate text-xs text-slate-500">{operation.resourceType} · {operation.resourceId}</p>{operation.error ? <p className="mt-1 text-xs text-red-300">{operation.error}</p> : null}</div><span className={`shrink-0 rounded-full px-2 py-1 text-xs font-medium ${operation.status === "succeeded" ? "bg-emerald-500/10 text-emerald-300" : operation.status === "failed" ? "bg-red-500/10 text-red-300" : "bg-amber-500/10 text-amber-200"}`}>{operation.status}</span></div>)}</div>}
      </section>
    </div>
  );
}
