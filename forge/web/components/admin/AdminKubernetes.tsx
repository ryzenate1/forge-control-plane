"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, Container, Globe, Layers, RefreshCw, Scale } from "lucide-react";
import { fetchKubernetesNodes, fetchK8sPods, fetchK8sDeployments, fetchK8sServices, fetchK8sEvents, scaleK8sDeployment } from "@/lib/api/kubernetes";
import { SectionHeader } from "./admin-ui";
import { useToast } from "@/components/ui/toast";
import { cn } from "@/lib/utils";

const TABS = [
  { id: "pods", label: "Pods", icon: Container },
  { id: "deployments", label: "Deployments", icon: Layers },
  { id: "services", label: "Services", icon: Globe },
  { id: "events", label: "Events", icon: Activity },
] as const;

export function AdminKubernetes() {
  const [nodeId, setNodeId] = useState<string | undefined>(undefined);
  const [tab, setTab] = useState<(typeof TABS)[number]["id"]>("pods");
  const qc = useQueryClient();
  const { toast } = useToast();
  const nodesQ = useQuery({ queryKey: ["k8s-nodes"], queryFn: fetchKubernetesNodes, retry: 1 });
  const podsQ = useQuery({ queryKey: ["k8s-pods", nodeId], queryFn: () => fetchK8sPods(nodeId), enabled: tab === "pods" });
  const depsQ = useQuery({ queryKey: ["k8s-deployments", nodeId], queryFn: () => fetchK8sDeployments(nodeId), enabled: tab === "deployments" });
  const svcsQ = useQuery({ queryKey: ["k8s-services", nodeId], queryFn: () => fetchK8sServices(nodeId), enabled: tab === "services" });
  const evtsQ = useQuery({ queryKey: ["k8s-events", nodeId], queryFn: () => fetchK8sEvents(nodeId), enabled: tab === "events" });

  const scaleMut = useMutation({
    mutationFn: ({ name, replicas }: { name: string; replicas: number }) => scaleK8sDeployment(name, replicas, nodeId),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["k8s-deployments"] }),
    onError: (err) => toast({ tone: "error", title: "Failed to scale deployment", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const nodes = nodesQ.data ?? [];
  const noK8s = nodesQ.isSuccess && nodes.length === 0;

  return (
    <div className="mx-auto w-full max-w-[1280px] space-y-6">
      <SectionHeader
        title="Kubernetes"
        sub="Cluster workloads on nodes with DAEMON_RUNTIME_PROVIDER=kubernetes — pods, deployments, services, and events."
        action={
          <div className="flex flex-wrap items-center gap-2">
            <select
              value={nodeId ?? ""}
              onChange={(e) => setNodeId(e.target.value || undefined)}
              className="h-8 rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 text-xs text-slate-200 outline-none focus:border-[var(--focus)]"
            >
              <option value="">Auto — first k8s node</option>
              {nodes.map((n) => <option key={n.id} value={n.id}>{n.name} — {n.runtimeProvider}</option>)}
            </select>
            <button
              type="button"
              onClick={() => { void qc.invalidateQueries({ queryKey: ["k8s-nodes"] }); void qc.invalidateQueries({ queryKey: ["k8s-pods"] }); }}
              className="inline-flex items-center gap-1.5 rounded-lg border border-[var(--line)] bg-white/[0.03] px-3 py-1.5 text-xs text-slate-300 hover:text-white transition"
            >
              <RefreshCw size={12} /> Refresh
            </button>
            <span className="text-xs font-mono text-slate-400">{nodes.length} k8s nodes · {tab}</span>
          </div>
        }
      />

      {nodesQ.isLoading ? <div className="py-12 text-center text-sm text-[var(--text-subtle)]">Loading clusters…</div> : noK8s ? (
        <div className="mt-8 rounded-xl border border-dashed border-[var(--line)] bg-white/[0.02] p-8 text-center">
          <div className="mx-auto max-w-md">
            <div className="text-sm font-medium">No Kubernetes nodes</div>
            <div className="mt-1 text-xs leading-5 text-[var(--text-subtle)]">Add a node with <code className="font-mono">DAEMON_RUNTIME_PROVIDER=kubernetes</code> and valid <code className="font-mono">KUBECONFIG</code> or in-cluster auth.</div>
            <div className="mt-4 rounded-lg bg-black/20 p-3 font-mono text-xs text-left">DAEMON_RUNTIME_PROVIDER=kubernetes KUBECONFIG=/path/to/kubeconfig ./daemon</div>
          </div>
        </div>
      ) : (
        <>
          <div className="mt-6 flex gap-1 overflow-x-auto border-b border-[var(--line)] pb-px">
            {TABS.map((t) => (
              <button key={t.id} onClick={() => setTab(t.id)} className={cn("inline-flex items-center gap-1.5 whitespace-nowrap border-b-2 px-3 py-2.5 text-sm font-medium", tab === t.id ? "border-[var(--text)] text-white" : "border-transparent text-[var(--text-subtle)] hover:text-white")} type="button">
                <t.icon size={12} /> {t.label}
              </button>
            ))}
          </div>

          <div className="mt-6">
            {tab === "pods" && (
              <div className="overflow-hidden rounded-xl border border-[var(--line)]">
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead className="bg-white/[0.02] text-left text-xs uppercase tracking-wider text-[var(--text-subtle)]"><tr className="border-b border-[var(--line)]"><th className="px-4 py-2.5 font-medium">Name</th><th className="px-4 py-2.5">Status</th><th className="px-4 py-2.5">Node</th><th className="px-4 py-2.5">Restarts</th></tr></thead>
                    <tbody className="divide-y divide-[var(--line)]">
                      {podsQ.isLoading ? <tr><td colSpan={4} className="py-8 text-center text-sm text-[var(--text-subtle)]">Loading…</td></tr> : (podsQ.data ?? []).length === 0 ? <tr><td colSpan={4} className="py-8 text-center text-sm text-[var(--text-subtle)]">No pods.</td></tr> : podsQ.data!.map((p) => (
                        <tr key={p.name} className="hover:bg-white/[0.02]"><td className="px-4 py-3 font-mono text-xs">{p.name}</td><td className="px-4 py-3"><span className={cn("rounded-full border px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wider", p.status === "Running" ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-300" : "border-[var(--line)] bg-white/[0.03] text-[var(--text-subtle)]")}>{p.status}</span></td><td className="px-4 py-3 text-xs">{p.nodeName || "—"}</td><td className="px-4 py-3 font-mono text-xs">{p.restartCount}</td></tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}
            {tab === "deployments" && (
              <div className="overflow-hidden rounded-xl border border-[var(--line)]">
                <table className="w-full text-sm">
                  <thead className="bg-white/[0.02] text-left text-xs uppercase tracking-wider text-[var(--text-subtle)]"><tr className="border-b border-[var(--line)]"><th className="px-4 py-2.5">Name</th><th className="px-4 py-2.5">Image</th><th className="px-4 py-2.5">Replicas</th><th className="px-4 py-2.5">Actions</th></tr></thead>
                  <tbody className="divide-y divide-[var(--line)]">
                    {(depsQ.data ?? []).length === 0 ? <tr><td colSpan={4} className="py-8 text-center text-sm text-[var(--text-subtle)]">No deployments.</td></tr> : depsQ.data!.map((d) => (
                      <tr key={d.name} className="hover:bg-white/[0.02]"><td className="px-4 py-3 font-mono text-xs">{d.name}</td><td className="px-4 py-3 font-mono text-xs truncate max-w-[240px]">{d.image || "—"}</td><td className="px-4 py-3 font-mono text-xs">{d.readyReplicas}/{d.replicas}</td><td className="px-4 py-3"><div className="flex gap-1"><button onClick={() => scaleMut.mutate({ name: d.name, replicas: Math.max(0, d.replicas - 1) })} className="rounded-lg border border-[var(--line)] px-2 py-1 text-xs">-1</button><button onClick={() => scaleMut.mutate({ name: d.name, replicas: d.replicas + 1 })} className="rounded-lg bg-white px-2 py-1 text-xs font-medium text-slate-900"><Scale size={10} className="inline" /> +1</button></div></td></tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            {tab === "services" && (
              <div className="overflow-hidden rounded-xl border border-[var(--line)]">
                <table className="w-full text-sm">
                  <thead className="bg-white/[0.02] text-left text-xs uppercase tracking-wider text-[var(--text-subtle)]"><tr className="border-b border-[var(--line)]"><th className="px-4 py-2.5">Name</th><th className="px-4 py-2.5">Type</th><th className="px-4 py-2.5">Cluster IP</th><th className="px-4 py-2.5">Ports</th></tr></thead>
                  <tbody className="divide-y divide-[var(--line)]">
                    {(svcsQ.data ?? []).length === 0 ? <tr><td colSpan={4} className="py-8 text-center text-sm text-[var(--text-subtle)]">No services.</td></tr> : svcsQ.data!.map((s) => (
                      <tr key={s.name} className="hover:bg-white/[0.02]"><td className="px-4 py-3 font-mono text-xs">{s.name}</td><td className="px-4 py-3"><span className="rounded-full border border-[var(--line)] bg-white/[0.03] px-2 py-0.5 text-[11px]">{s.type}</span></td><td className="px-4 py-3 font-mono text-xs">{s.clusterIp}</td><td className="px-4 py-3 text-xs">{s.ports.join(", ") || "—"}</td></tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            {tab === "events" && (
              <div className="overflow-hidden rounded-xl border border-[var(--line)]">
                <table className="w-full text-sm">
                  <thead className="bg-white/[0.02] text-left text-xs uppercase tracking-wider text-[var(--text-subtle)]"><tr className="border-b border-[var(--line)]"><th className="px-4 py-2.5">Type</th><th className="px-4 py-2.5">Reason</th><th className="px-4 py-2.5">Message</th></tr></thead>
                  <tbody className="divide-y divide-[var(--line)]">
                    {(evtsQ.data ?? []).slice(0, 50).length === 0 ? <tr><td colSpan={3} className="py-8 text-center text-sm text-[var(--text-subtle)]">No events.</td></tr> : evtsQ.data!.slice(0, 50).map((e, i) => (
                      <tr key={i} className="hover:bg-white/[0.02]"><td className="px-4 py-3"><span className={cn("rounded-full border px-2 py-0.5 text-[11px] font-semibold", e.type === "Warning" ? "border-red-500/30 bg-red-500/10 text-red-300" : "border-[var(--line)] bg-white/[0.03]")}>{e.type}</span></td><td className="px-4 py-3 text-xs">{e.reason}</td><td className="px-4 py-3 text-xs line-clamp-2 max-w-[400px]">{e.message}</td></tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </>
      )}
    </div>
  );
}
