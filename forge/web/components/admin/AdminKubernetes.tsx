"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, Container, Globe, Layers, RefreshCw, Scale, Boxes, Network } from "lucide-react";
import { fetchKubernetesNodes, fetchK8sPods, fetchK8sDeployments, fetchK8sServices, fetchK8sEvents, scaleK8sDeployment } from "@/lib/api/kubernetes";
import { AdminPageLayout, AdminTabs, AdminSelect, AdminLoadingState, AdminErrorState, Btn, Card, CardHeader, EmptyState, Pill, SectionHeader } from "./admin-ui";
import { useToast } from "@/components/ui/toast";

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
    <AdminPageLayout>
      <SectionHeader
        title="Kubernetes"
        sub="Cluster workloads on nodes with the Kubernetes runtime — pods, deployments, services, and events."
        info={{
          title: "Kubernetes workloads",
          triggerLabel: "About Kubernetes",
          eyebrow: "Architecture & Semantics",
          description: "Inspect Kubernetes workloads running on Beacon nodes that report the Kubernetes runtime.",
          sections: [
            {
              title: "Node scope",
              icon: Network,
              content:
                "Pick a node to scope pods, deployments, services and events, or leave automatic to use the first Kubernetes node. Nodes without the runtime never appear here.",
            },
            {
              title: "Scaling deployments",
              icon: Scale,
              content:
                "Deployment actions adjust desired replicas through the control plane. Counts refresh from the cluster — a scale that the cluster rejects surfaces as an error toast, not a silent count.",
            },
          ],
        }}
        action={
          <div className="flex flex-wrap items-center gap-2">
            <div className="min-w-48">
              <AdminSelect
                value={nodeId ?? ""}
                onChange={(v) => setNodeId(v || undefined)}
                options={nodes.map((n) => ({ value: n.id, label: `${n.name} — ${n.runtimeProvider}` }))}
                placeholder="Auto — first k8s node"
              />
            </div>
            <Btn size="sm" tone="ghost" onClick={() => { void qc.invalidateQueries({ queryKey: ["k8s-nodes"] }); void qc.invalidateQueries({ queryKey: ["k8s-pods"] }); }}>
              <RefreshCw size={14} /> Refresh
            </Btn>
            {nodes.length > 0 ? <Pill tone="neutral">{nodes.length} nodes</Pill> : null}
          </div>
        }
      />

      {nodesQ.isLoading ? <AdminLoadingState label="Loading clusters…" /> : nodesQ.isError ? <AdminErrorState message={nodesQ.error instanceof Error ? nodesQ.error.message : "Could not load Kubernetes nodes."} retry={() => void nodesQ.refetch()} /> : noK8s ? (
        <EmptyState icon={Boxes} title="No Kubernetes nodes" message="Add a node with the Kubernetes runtime provider and valid kubeconfig or in-cluster auth." />
      ) : (
        <>
          <AdminTabs tabs={TABS.map((t) => ({ id: t.id, label: t.label, icon: t.icon }))} active={tab} onChange={(id) => setTab(id as typeof tab)} label="Kubernetes sections" />

          {tab === "pods" && (
            <Card>
              <CardHeader title="Pods" icon={Container} />
              {podsQ.isLoading ? <AdminLoadingState label="Loading pods…" /> : podsQ.isError ? <div className="p-4"><AdminErrorState message={podsQ.error instanceof Error ? podsQ.error.message : "Could not load pods."} retry={() => void podsQ.refetch()} /></div> : (podsQ.data ?? []).length === 0 ? <EmptyState icon={Container} message="No pods." /> : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead className="text-left text-xs uppercase tracking-wider text-[var(--text-subtle)]"><tr className="border-b border-[var(--line)]"><th className="px-4 py-2.5 font-medium">Name</th><th className="px-4 py-2.5 font-medium">Status</th><th className="px-4 py-2.5 font-medium">Node</th><th className="px-4 py-2.5 font-medium">Restarts</th></tr></thead>
                  <tbody className="divide-y divide-[var(--line)]">
                    {podsQ.data!.map((p) => (
                      <tr key={p.name} className="hover:bg-[var(--surface-hover)]"><td className="px-4 py-3 font-mono text-xs">{p.name}</td><td className="px-4 py-3"><Pill tone={p.status === "Running" ? "green" : "neutral"}>{p.status}</Pill></td><td className="px-4 py-3 text-xs">{p.nodeName || "—"}</td><td className="px-4 py-3 font-mono text-xs">{p.restartCount}</td></tr>
                    ))}
                  </tbody>
                </table>
              </div>
              )}
            </Card>
          )}
          {tab === "deployments" && (
            <Card>
              <CardHeader title="Deployments" icon={Layers} />
              {depsQ.isLoading ? <AdminLoadingState label="Loading deployments…" /> : depsQ.isError ? <div className="p-4"><AdminErrorState message={depsQ.error instanceof Error ? depsQ.error.message : "Could not load deployments."} retry={() => void depsQ.refetch()} /></div> : (depsQ.data ?? []).length === 0 ? <EmptyState icon={Layers} message="No deployments." /> : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead className="text-left text-xs uppercase tracking-wider text-[var(--text-subtle)]"><tr className="border-b border-[var(--line)]"><th className="px-4 py-2.5 font-medium">Name</th><th className="px-4 py-2.5 font-medium">Image</th><th className="px-4 py-2.5 font-medium">Replicas</th><th className="px-4 py-2.5 font-medium">Actions</th></tr></thead>
                  <tbody className="divide-y divide-[var(--line)]">
                    {depsQ.data!.map((d) => (
                      <tr key={d.name} className="hover:bg-[var(--surface-hover)]"><td className="px-4 py-3 font-mono text-xs">{d.name}</td><td className="px-4 py-3 font-mono text-xs truncate max-w-[240px]">{d.image || "—"}</td><td className="px-4 py-3 font-mono text-xs">{d.readyReplicas}/{d.replicas}</td><td className="px-4 py-3"><div className="flex gap-1"><Btn size="sm" tone="ghost" onClick={() => scaleMut.mutate({ name: d.name, replicas: Math.max(0, d.replicas - 1) })}>-1</Btn><Btn size="sm" tone="primary" onClick={() => scaleMut.mutate({ name: d.name, replicas: d.replicas + 1 })}><Scale size={12} /> +1</Btn></div></td></tr>
                    ))}
                  </tbody>
                </table>
              </div>
              )}
            </Card>
          )}
          {tab === "services" && (
            <Card>
              <CardHeader title="Services" icon={Globe} />
              {svcsQ.isLoading ? <AdminLoadingState label="Loading services…" /> : svcsQ.isError ? <div className="p-4"><AdminErrorState message={svcsQ.error instanceof Error ? svcsQ.error.message : "Could not load services."} retry={() => void svcsQ.refetch()} /></div> : (svcsQ.data ?? []).length === 0 ? <EmptyState icon={Globe} message="No services." /> : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead className="text-left text-xs uppercase tracking-wider text-[var(--text-subtle)]"><tr className="border-b border-[var(--line)]"><th className="px-4 py-2.5 font-medium">Name</th><th className="px-4 py-2.5 font-medium">Type</th><th className="px-4 py-2.5 font-medium">Cluster IP</th><th className="px-4 py-2.5 font-medium">Ports</th></tr></thead>
                  <tbody className="divide-y divide-[var(--line)]">
                    {svcsQ.data!.map((s) => (
                      <tr key={s.name} className="hover:bg-[var(--surface-hover)]"><td className="px-4 py-3 font-mono text-xs">{s.name}</td><td className="px-4 py-3"><Pill tone="neutral">{s.type}</Pill></td><td className="px-4 py-3 font-mono text-xs">{s.clusterIp}</td><td className="px-4 py-3 text-xs">{s.ports.join(", ") || "—"}</td></tr>
                    ))}
                  </tbody>
                </table>
              </div>
              )}
            </Card>
          )}
          {tab === "events" && (
            <Card>
              <CardHeader title="Events" icon={Activity} />
              {evtsQ.isLoading ? <AdminLoadingState label="Loading events…" /> : evtsQ.isError ? <div className="p-4"><AdminErrorState message={evtsQ.error instanceof Error ? evtsQ.error.message : "Could not load events."} retry={() => void evtsQ.refetch()} /></div> : (evtsQ.data ?? []).slice(0, 50).length === 0 ? <EmptyState icon={Activity} message="No events." /> : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead className="text-left text-xs uppercase tracking-wider text-[var(--text-subtle)]"><tr className="border-b border-[var(--line)]"><th className="px-4 py-2.5 font-medium">Type</th><th className="px-4 py-2.5 font-medium">Reason</th><th className="px-4 py-2.5 font-medium">Message</th></tr></thead>
                  <tbody className="divide-y divide-[var(--line)]">
                    {evtsQ.data!.slice(0, 50).map((e, i) => (
                      <tr key={i} className="hover:bg-[var(--surface-hover)]"><td className="px-4 py-3"><Pill tone={e.type === "Warning" ? "red" : "neutral"}>{e.type}</Pill></td><td className="px-4 py-3 text-xs">{e.reason}</td><td className="px-4 py-3 text-xs line-clamp-2 max-w-[400px]">{e.message}</td></tr>
                    ))}
                  </tbody>
                </table>
              </div>
              )}
            </Card>
          )}
        </>
      )}
    </AdminPageLayout>
  );
}
