"use client";

import { useState, useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Cloud, Server, Plus, Trash2, Building2, Link2 } from "lucide-react";
import { deleteJSON, fetchJSON, fetchNodes, postJSON, type ApiNode } from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, AdminPageLayout, AdminLoadingState, AdminErrorState } from "@/components/admin/admin-ui";
import { useConfirm } from "@/components/ui/confirm-dialog";

type CloudProvider = {
  kind: string;
  name: string;
  region?: string;
};

type CloudInstance = {
  id: string;
  name: string;
  provider: string;
  region: string;
  instanceType: string;
  publicIp?: string;
  privateIp?: string;
  status: string;
  createdAt: string;
};

type CloudNodeLink = {
  provider: string;
  instanceId: string;
  nodeId: string;
};

type DataResponse<T> = { data: T };

export default function AdminCloudPage() {
  const [confirm, renderConfirm] = useConfirm();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [showProvision, setShowProvision] = useState(false);
  const [selectedProvider, setSelectedProvider] = useState("");
  const [form, setForm] = useState({ name: "", instanceType: "", image: "", nodeId: "", beaconImage: "", subnetId: "", securityGroupIds: "", iamInstanceProfile: "", diskGb: "" });

  const providersQuery = useQuery({
    queryKey: ["admin", "cloud", "providers"],
    queryFn: () => fetchJSON<DataResponse<CloudProvider[]>>("/admin/cloud/providers"),
  });
  const providers = useMemo(() => providersQuery.data?.data ?? [], [providersQuery.data]);
  const provider = Array.isArray(providers) ? providers.find((item) => item.kind === selectedProvider) : undefined;

  const instancesQuery = useQuery({
    queryKey: ["admin", "cloud", "instances", selectedProvider],
    queryFn: () => fetchJSON<DataResponse<CloudInstance[]>>(`/admin/cloud/instances?provider=${encodeURIComponent(selectedProvider)}`),
    enabled: Boolean(selectedProvider),
  });
  const instances = useMemo(() => instancesQuery.data?.data ?? [], [instancesQuery.data]);

  const linksQuery = useQuery({
    queryKey: ["admin", "cloud", "links"],
    queryFn: () => fetchJSON<DataResponse<CloudNodeLink[]>>("/admin/cloud/links"),
  });
  const links = useMemo(() => linksQuery.data?.data ?? [], [linksQuery.data]);
  const nodesQuery = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });
  const nodes = useMemo(() => Array.isArray(nodesQuery.data) ? nodesQuery.data : [], [nodesQuery.data]);

  const provisionMutation = useMutation({
    mutationFn: () => postJSON<DataResponse<CloudInstance>>("/admin/cloud/provision", {
      provider: selectedProvider,
      request: {
        name: form.name.trim(),
        region: provider?.region ?? "",
        instanceType: form.instanceType.trim(),
        image: form.image.trim(),
        beaconImage: form.beaconImage.trim() || undefined,
        subnetId: form.subnetId.trim() || undefined,
        securityGroupIds: form.securityGroupIds.split(",").map((value) => value.trim()).filter(Boolean),
        iamInstanceProfile: form.iamInstanceProfile.trim() || undefined,
        diskGb: form.diskGb ? Number(form.diskGb) : undefined,
      },
      nodeId: form.nodeId || undefined,
    }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["admin", "cloud", "instances", selectedProvider] });
      void queryClient.invalidateQueries({ queryKey: ["admin", "cloud", "links"] });
      setShowProvision(false);
      setForm({ name: "", instanceType: "", image: "", nodeId: "", beaconImage: "", subnetId: "", securityGroupIds: "", iamInstanceProfile: "", diskGb: "" });
    },
    onError: (error) => toast({ tone: "error", title: "Provisioning failed", message: error instanceof Error ? error.message : "Could not provision instance." }),
  });

  const terminateMutation = useMutation({
    mutationFn: (instance: CloudInstance) => deleteJSON(`/admin/cloud/instances/${encodeURIComponent(instance.provider)}/${encodeURIComponent(instance.id)}`),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["admin", "cloud", "instances", selectedProvider] });
      void queryClient.invalidateQueries({ queryKey: ["admin", "cloud", "links"] });
    },
    onError: (error) => toast({ tone: "error", title: "Terminate failed", message: error instanceof Error ? error.message : "Could not terminate instance." }),
  });

  const linkedNode = (instance: CloudInstance): ApiNode | undefined => {
    const link = Array.isArray(links) ? links.find((item) => item.provider === instance.provider && item.instanceId === instance.id) : undefined;
    return link && Array.isArray(nodes) ? nodes.find((node) => node.id === link.nodeId) : undefined;
  };
  const canProvision = Boolean(provider && form.name.trim() && form.instanceType.trim() && form.image.trim());

  return (
    <AdminPageLayout>
      <SectionHeader
        title="Cloud Instances"
        sub="Provision cloud provider instances and bootstrap them as beacons automatically. Links cloud compute to Forge placement."
        info={{
          title: "Cloud instances",
          triggerLabel: "About Cloud Instances",
          eyebrow: "Architecture & Semantics",
          description: "Provision cloud compute and bootstrap it as a Beacon node, linking provider instances to Forge placement.",
          sections: [
            {
              title: "Providers and bootstrap",
              icon: Building2,
              content:
                "A configured provider supplies region and credentials. Provisioning launches the instance, then cloud-init installs Docker and starts Beacon so the host appears under Nodes.",
            },
            {
              title: "Links to placement",
              icon: Link2,
              content:
                "Linked instances map to a panel node used for scheduling. Terminating destroys the remote instance permanently; the node record keeps its own lifecycle.",
            },
          ],
        }}
        action={<Btn tone="primary" onClick={() => setShowProvision(true)}><Plus size={14} /> Provision Instance</Btn>}
      />

      {providersQuery.isError ? <div className="p-4"><AdminErrorState message={providersQuery.error instanceof Error ? providersQuery.error.message : "Could not load providers."} retry={() => void providersQuery.refetch()} /></div> : null}
      <Card>
        <CardHeader title="Configured Providers" icon={Cloud} />
        {providersQuery.isLoading ? <AdminLoadingState label="Loading providers…" /> : !Array.isArray(providers) || providers.length === 0 ? (
          <EmptyState icon={Cloud} message="No cloud provider is configured. Set AWS_REGION (or AWS_DEFAULT_REGION) and restart the API to enable AWS." />
        ) : (
          <div className="divide-y divide-[var(--line)]">
            {Array.isArray(providers) && providers.map((item) => (
              <button key={item.kind} type="button" onClick={() => setSelectedProvider(item.kind)} className="flex w-full items-center justify-between px-4 py-3 text-left hover:bg-[var(--surface-hover)]">
                <div><p className="text-sm font-medium text-[var(--text)]">{item.name}</p><p className="text-xs text-[var(--text-subtle)]">{item.kind.toUpperCase()} · {item.region ?? "region not reported"}</p></div>
                <Pill tone={selectedProvider === item.kind ? "green" : "blue"}>{selectedProvider === item.kind ? "selected" : "configured"}</Pill>
              </button>
            ))}
          </div>
        )}
      </Card>

      <Card>
        <CardHeader title="Provider Instances" icon={Server} />
        {!selectedProvider ? <EmptyState icon={Server} message="Select a configured provider to load its instances." /> : instancesQuery.isLoading ? <AdminLoadingState label="Loading instances…" /> : instancesQuery.isError ? <div className="p-4"><AdminErrorState message={instancesQuery.error instanceof Error ? instancesQuery.error.message : "Could not load instances."} retry={() => void instancesQuery.refetch()} /></div> : !Array.isArray(instances) || instances.length === 0 ? <EmptyState icon={Server} message="No instances returned by this provider." /> : (
          <div className="overflow-x-auto"><table className="w-full text-sm"><thead><tr className="border-b border-[var(--line)] text-left text-[10px] uppercase tracking-widest text-[var(--text-subtle)]"><th className="px-4 py-3">Name</th><th className="px-4 py-3">Instance ID</th><th className="px-4 py-3">Type</th><th className="px-4 py-3">Region</th><th className="px-4 py-3">IP</th><th className="px-4 py-3">Panel node</th><th className="px-4 py-3">Status</th><th className="px-4 py-3" /></tr></thead><tbody className="divide-y divide-[var(--line)]">
            {Array.isArray(instances) && instances.map((instance) => { const node = linkedNode(instance); return <tr key={instance.id} className="hover:bg-[var(--surface-hover)]"><td className="px-4 py-3 font-medium text-[var(--text)]">{instance.name || "—"}</td><td className="px-4 py-3 font-mono text-xs text-[var(--text-subtle)]">{instance.id}</td><td className="px-4 py-3 text-xs text-[var(--text-subtle)]">{instance.instanceType}</td><td className="px-4 py-3 text-xs text-[var(--text-subtle)]">{instance.region}</td><td className="px-4 py-3 font-mono text-xs text-[var(--text-subtle)]">{instance.publicIp || instance.privateIp || "—"}</td><td className="px-4 py-3 text-xs text-[var(--text-subtle)]">{node?.name ?? "Not linked"}</td><td className="px-4 py-3"><Pill tone={instance.status === "running" ? "green" : "yellow"}>{instance.status}</Pill></td><td className="px-4 py-3"><Btn size="sm" tone="danger" disabled={terminateMutation.isPending} onClick={() => { void (async () => { if (await confirm({ title: `Terminate ${instance.name || instance.id}?`, description: "The cloud instance will be permanently destroyed. This cannot be undone.", danger: true, confirmLabel: "Terminate" })) terminateMutation.mutate(instance); })(); }}><Trash2 size={12} /> Terminate</Btn></td></tr>; })}
          </tbody></table></div>
        )}
      </Card>

      {showProvision ? <Modal title="Provision Provider Instance" onClose={() => setShowProvision(false)} wide><div className="space-y-4">
        {!Array.isArray(providers) || providers.length === 0 ? <div className="rounded-xl border border-amber-500/25 bg-amber-500/[0.07] p-4 text-sm text-amber-100"><p className="font-semibold">Cloud provisioning needs a configured provider.</p><p className="mt-1 leading-6 text-amber-200/80">Configure provider credentials and a region on the API, then restart it. The provisioning form will become available here automatically.</p></div> : <label className="block text-sm"><span className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-[var(--text-subtle)]">Provider</span><select value={selectedProvider} onChange={(event) => setSelectedProvider(event.target.value)} className="h-10 w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 text-sm text-[var(--text)]"><option value="">Select provider…</option>{Array.isArray(providers) && providers.map((item) => <option key={item.kind} value={item.kind}>{item.name} ({item.region})</option>)}</select></label>}
        <Input label="Instance name" value={form.name} onChange={(value) => setForm({ ...form, name: value })} placeholder="game-node-1" />
        <Input label="Instance type" value={form.instanceType} onChange={(value) => setForm({ ...form, instanceType: value })} placeholder="t3.medium" />
        <Input label="Image ID" value={form.image} onChange={(value) => setForm({ ...form, image: value })} placeholder="ami-…" />
        <div className="block text-sm"><span className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-[var(--text-subtle)]">Configured region</span><p className="rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text-subtle)]">{provider?.region ?? "Select a provider"}</p></div>
        <label className="block text-sm"><span className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-[var(--text-subtle)]">Bootstrap as panel node</span><select value={form.nodeId} onChange={(event) => setForm({ ...form, nodeId: event.target.value })} className="h-9 w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 text-sm text-[var(--text)]"><option value="">Provision compute only</option>{Array.isArray(nodes) && nodes.map((node) => <option key={node.id} value={node.id}>{node.name}</option>)}</select></label>
        <Input label="Beacon container image" value={form.beaconImage} onChange={(value) => setForm({ ...form, beaconImage: value })} placeholder="ghcr.io/gamepanel/beacon:<release-tag>" />
        <Input label="Subnet ID (optional)" value={form.subnetId} onChange={(value) => setForm({ ...form, subnetId: value })} placeholder="subnet-…" />
        <Input label="Security group IDs (comma-separated)" value={form.securityGroupIds} onChange={(value) => setForm({ ...form, securityGroupIds: value })} placeholder="sg-…" />
        <Input label="IAM instance profile (optional)" value={form.iamInstanceProfile} onChange={(value) => setForm({ ...form, iamInstanceProfile: value })} placeholder="gamepanel-beacon" />
        <Input label="Root disk GB (optional)" type="number" value={form.diskGb} onChange={(value) => setForm({ ...form, diskGb: value })} placeholder="50" />
        <p className="text-xs text-[var(--text-subtle)]">When linked, Ubuntu cloud-init installs Docker and starts Beacon with the selected node credential. Use a private panel API URL and an IAM role for shared backup access.</p>
        {provisionMutation.isError ? <div className="p-4"><AdminErrorState message={provisionMutation.error instanceof Error ? provisionMutation.error.message : "Provisioning failed."} retry={() => provisionMutation.mutate()} /></div> : null}
      </div><ModalFooter onCancel={() => setShowProvision(false)} onConfirm={() => provisionMutation.mutate()} confirmLabel={provisionMutation.isPending ? "Provisioning…" : "Provision"} disabled={!Array.isArray(providers) || providers.length === 0 || provisionMutation.isPending || !canProvision} /></Modal> : null}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
