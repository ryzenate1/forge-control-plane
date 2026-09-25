"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, GitBranch, RefreshCw, Rocket, Server, Square, Workflow } from "lucide-react";
import {
  drainNomadNode,
  fetchNomadAllocations,
  fetchNomadDeployments,
  fetchNomadJobs,
  fetchNomadNodes,
  stopNomadJob,
  submitNomadJob,
  type NomadAllocation,
  type NomadDeployment,
  type NomadJob,
  type NomadNode,
} from "@/lib/api/nomad";
import { AdminPageLayout, AdminTabs, Btn, Card, CardHeader, Modal, Pill, SectionHeader, Textarea, AdminLoadingState, AdminErrorState } from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";

const TABS = [
  { id: "jobs", label: "Jobs", icon: Workflow },
  { id: "allocations", label: "Allocations", icon: Activity },
  { id: "nodes", label: "Nodes", icon: Server },
  { id: "deployments", label: "Deployments", icon: GitBranch },
] as const;

type Tab = (typeof TABS)[number]["id"];

function statusTone(status?: string): "green" | "yellow" | "red" | "neutral" {
  switch (status) {
    case "running":
    case "successful":
    case "active":
    case "ready":
      return "green";
    case "dead":
    case "stopped":
    case "complete":
    case "inactive":
      return "neutral";
    case "failed":
    case "lost":
    case "unhealthy":
      return "red";
    default:
      return "yellow";
  }
}

export default function NomadAdminPage() {
  const [tab, setTab] = useState<Tab>("jobs");
  const [submitOpen, setSubmitOpen] = useState(false);
  const qc = useQueryClient();
  const { toast } = useToast();

  const jobsQ = useQuery({ queryKey: ["nomad-jobs"], queryFn: () => fetchNomadJobs(), enabled: tab === "jobs" });
  const allocsQ = useQuery({ queryKey: ["nomad-allocations"], queryFn: () => fetchNomadAllocations(), enabled: tab === "allocations" });
  const nodesQ = useQuery({ queryKey: ["nomad-nodes"], queryFn: () => fetchNomadNodes(), enabled: tab === "nodes" });
  const deploymentsQ = useQuery({ queryKey: ["nomad-deployments"], queryFn: () => fetchNomadDeployments(), enabled: tab === "deployments" });

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["nomad-jobs"] });
    void qc.invalidateQueries({ queryKey: ["nomad-allocations"] });
    void qc.invalidateQueries({ queryKey: ["nomad-nodes"] });
    void qc.invalidateQueries({ queryKey: ["nomad-deployments"] });
  };

  const onError = (action: string) => (err: Error) =>
    toast({ tone: "error", title: `Failed to ${action}`, message: err instanceof Error ? err.message : "An error occurred" });

  const stopMut = useMutation({
    mutationFn: (id: string) => stopNomadJob(id, true),
    onSuccess: (_r, id) => { toast({ tone: "success", title: "Stopped job", message: id }); void qc.invalidateQueries({ queryKey: ["nomad-jobs"] }); },
    onError: onError("stop job"),
  });
  const drainMut = useMutation({
    mutationFn: (vars: { id: string; drain: boolean }) => drainNomadNode(vars.id, vars.drain),
    onSuccess: (data) => { toast({ tone: "success", title: data.drain ? "Draining node" : "Node eligible", message: data.node }); void qc.invalidateQueries({ queryKey: ["nomad-nodes"] }); },
    onError: onError("update node drain"),
  });

  const pending = stopMut.isPending || drainMut.isPending;

  return (
    <AdminPageLayout>
      <SectionHeader
        title="Nomad"
        sub="Jobs, allocations, client nodes and deployments on the Nomad control plane — configured via NOMAD_ADDR / NOMAD_TOKEN."
        action={
          <div className="flex flex-wrap items-center gap-2">
            <Btn size="sm" tone="ghost" onClick={refresh}><RefreshCw size={14} /> Refresh</Btn>
            <Btn size="sm" tone="primary" onClick={() => setSubmitOpen(true)}><Rocket size={14} /> Submit job</Btn>
            <span className="text-xs font-mono text-slate-400">{tab}</span>
          </div>
        }
      />

      <AdminTabs tabs={TABS.map((t) => ({ id: t.id, label: t.label, icon: t.icon }))} active={tab} onChange={(id) => setTab(id as Tab)} label="Nomad sections" />

      {tab === "jobs" && (
        <Card>
          <CardHeader title="Jobs" icon={Workflow} />
          {jobsQ.isLoading ? <AdminLoadingState label="Loading jobs…" /> : jobsQ.isError ? <div className="p-4"><AdminErrorState message={(jobsQ.error as Error).message} retry={() => void jobsQ.refetch()} /></div> : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="text-left text-xs uppercase tracking-wider text-slate-500"><tr className="border-b border-white/[0.06]"><th className="px-4 py-2.5 font-medium">Name</th><th className="px-4 py-2.5 font-medium">Status</th><th className="px-4 py-2.5 font-medium">Type</th><th className="px-4 py-2.5 font-medium">Actions</th></tr></thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {(jobsQ.data ?? []).length === 0 ? <tr><td colSpan={4} className="py-8 text-center text-sm text-slate-500">No jobs.</td></tr> : (jobsQ.data as NomadJob[]).map((j) => (
                    <tr key={j.ID} className="hover:bg-white/[0.02]">
                      <td className="px-4 py-3 font-mono text-xs text-slate-200">{j.Name || j.ID}</td>
                      <td className="px-4 py-3"><Pill tone={statusTone(j.Status)}>{j.Status || "—"}</Pill></td>
                      <td className="px-4 py-3 text-xs text-slate-400">{j.Type || "service"}</td>
                      <td className="px-4 py-3">
                        <Btn size="sm" tone="danger" disabled={pending || j.Status === "stopped"} loading={stopMut.isPending} onClick={() => stopMut.mutate(j.ID)} title="Stop job"><Square size={12} /> Stop</Btn>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      )}

      {tab === "allocations" && (
        <Card>
          <CardHeader title="Allocations" icon={Activity} />
          {allocsQ.isLoading ? <AdminLoadingState label="Loading allocations…" /> : allocsQ.isError ? <div className="p-4"><AdminErrorState message={(allocsQ.error as Error).message} retry={() => void allocsQ.refetch()} /></div> : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="text-left text-xs uppercase tracking-wider text-slate-500"><tr className="border-b border-white/[0.06]"><th className="px-4 py-2.5 font-medium">ID</th><th className="px-4 py-2.5 font-medium">Job</th><th className="px-4 py-2.5 font-medium">Task Group</th><th className="px-4 py-2.5 font-medium">Client Status</th><th className="px-4 py-2.5 font-medium">Node</th></tr></thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {(allocsQ.data ?? []).length === 0 ? <tr><td colSpan={5} className="py-8 text-center text-sm text-slate-500">No allocations.</td></tr> : (allocsQ.data as NomadAllocation[]).map((a) => (
                    <tr key={a.ID} className="hover:bg-white/[0.02]">
                      <td className="px-4 py-3 font-mono text-xs text-slate-200">{a.ID.slice(0, 8)}</td>
                      <td className="px-4 py-3 font-mono text-xs text-slate-400">{a.JobID || "—"}</td>
                      <td className="px-4 py-3 text-xs text-slate-400">{a.TaskGroup || "—"}</td>
                      <td className="px-4 py-3"><Pill tone={statusTone(a.ClientStatus)}>{a.ClientStatus || "—"}</Pill></td>
                      <td className="px-4 py-3 font-mono text-xs text-slate-400">{a.NodeID ? a.NodeID.slice(0, 8) : "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      )}

      {tab === "nodes" && (
        <Card>
          <CardHeader title="Client Nodes" icon={Server} />
          {nodesQ.isLoading ? <AdminLoadingState label="Loading nodes…" /> : nodesQ.isError ? <div className="p-4"><AdminErrorState message={(nodesQ.error as Error).message} retry={() => void nodesQ.refetch()} /></div> : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="text-left text-xs uppercase tracking-wider text-slate-500"><tr className="border-b border-white/[0.06]"><th className="px-4 py-2.5 font-medium">Name</th><th className="px-4 py-2.5 font-medium">Address</th><th className="px-4 py-2.5 font-medium">Status</th><th className="px-4 py-2.5 font-medium">Version</th><th className="px-4 py-2.5 font-medium">Actions</th></tr></thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {(nodesQ.data ?? []).length === 0 ? <tr><td colSpan={5} className="py-8 text-center text-sm text-slate-500">No nodes.</td></tr> : (nodesQ.data as NomadNode[]).map((n) => (
                    <tr key={n.ID} className="hover:bg-white/[0.02]">
                      <td className="px-4 py-3 font-mono text-xs text-slate-200">{n.Name || n.ID.slice(0, 8)}</td>
                      <td className="px-4 py-3 font-mono text-xs text-slate-400">{n.HTTPAddr || "—"}</td>
                      <td className="px-4 py-3"><Pill tone={statusTone(n.Status)}>{n.Status || "—"}{n.Drain ? " · draining" : ""}</Pill></td>
                      <td className="px-4 py-3 text-xs text-slate-400">{n.Version || "—"}</td>
                      <td className="px-4 py-3">
                        <Btn size="sm" tone={n.Drain ? "success" : "warning"} disabled={pending} loading={drainMut.isPending} onClick={() => drainMut.mutate({ id: n.ID, drain: !n.Drain })} title={n.Drain ? "Mark eligible" : "Start drain"}>{n.Drain ? "Eligible" : "Drain"}</Btn>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      )}

      {tab === "deployments" && (
        <Card>
          <CardHeader title="Deployments" icon={GitBranch} />
          {deploymentsQ.isLoading ? <AdminLoadingState label="Loading deployments…" /> : deploymentsQ.isError ? <div className="p-4"><AdminErrorState message={(deploymentsQ.error as Error).message} retry={() => void deploymentsQ.refetch()} /></div> : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="text-left text-xs uppercase tracking-wider text-slate-500"><tr className="border-b border-white/[0.06]"><th className="px-4 py-2.5 font-medium">ID</th><th className="px-4 py-2.5 font-medium">Job</th><th className="px-4 py-2.5 font-medium">Status</th><th className="px-4 py-2.5 font-medium">Desired</th></tr></thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {(deploymentsQ.data ?? []).length === 0 ? <tr><td colSpan={4} className="py-8 text-center text-sm text-slate-500">No deployments.</td></tr> : (deploymentsQ.data as NomadDeployment[]).map((d) => (
                    <tr key={d.ID} className="hover:bg-white/[0.02]">
                      <td className="px-4 py-3 font-mono text-xs text-slate-200">{d.ID.slice(0, 8)}</td>
                      <td className="px-4 py-3 font-mono text-xs text-slate-400">{d.JobID || "—"}</td>
                      <td className="px-4 py-3"><Pill tone={statusTone(d.Status)}>{d.Status || "—"}</Pill></td>
                      <td className="px-4 py-3 text-xs text-slate-400">{d.DesiredStatus || "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      )}

      {submitOpen && <SubmitJobModal onClose={() => setSubmitOpen(false)} onSubmitted={(id) => { toast({ tone: "success", title: "Job submitted", message: id }); void qc.invalidateQueries({ queryKey: ["nomad-jobs"] }); setSubmitOpen(false); }} onError={onError("submit job")} />}
    </AdminPageLayout>
  );
}

function SubmitJobModal({ onClose, onSubmitted, onError }: { onClose: () => void; onSubmitted: (id: string) => void; onError: (err: Error) => void }) {
  const [spec, setSpec] = useState("");
  const submitMut = useMutation({
    mutationFn: async () => {
      const text = spec.trim();
      let payload: Record<string, unknown>;
      try {
        payload = JSON.parse(text) as Record<string, unknown>;
      } catch {
        // Nomad's HTTP API only accepts a JSON job descriptor; forward raw text
        // under an "hcl" key so the backend returns an actionable error.
        payload = { hcl: text };
      }
      return submitNomadJob(payload);
    },
    onSuccess: (res) => onSubmitted(res?.job?.ID ?? "job"),
    onError: (err: Error) => onError(err),
  });
  return (
    <Modal title="Submit Nomad job" onClose={onClose} description="Paste a Nomad job document. The control-plane HTTP API consumes JSON; HCL must be rendered to JSON first.">
      <div className="space-y-4">
        <Textarea label="Job specification (JSON)" value={spec} onChange={setSpec} rows={14} placeholder={'{\n  "Job": {\n    "ID": "example",\n    "Type": "service",\n    "Datacenters": ["dc1"],\n    "TaskGroups": []\n  }\n}'} />
        <p className="text-xs leading-5 text-slate-500">Non-JSON input is forwarded as HCL and the API will reject it with guidance — run <code className="font-mono">nomad job run -output</code> or a JSON converter to render HCL to a job descriptor.</p>
        <div className="flex justify-end gap-2 pt-2">
          <Btn tone="ghost" onClick={onClose}>Cancel</Btn>
          <Btn tone="primary" loading={submitMut.isPending} disabled={!spec.trim()} onClick={() => submitMut.mutate()}><Rocket size={14} /> Submit</Btn>
        </div>
      </div>
    </Modal>
  );
}
