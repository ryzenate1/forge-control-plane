"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, Cloud, Loader2, Plus, Server, Trash2 } from "lucide-react";
import { deleteJSON, fetchJSON, fetchNodes, postJSON, type ApiNode } from "@/modules/infrastructure/api";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader } from "@/components/admin/admin-ui";

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
  const queryClient = useQueryClient();
  const [showProvision, setShowProvision] = useState(false);
  const [selectedProvider, setSelectedProvider] = useState("");
  const [form, setForm] = useState({ name: "", instanceType: "", image: "", nodeId: "", beaconImage: "", subnetId: "", securityGroupIds: "", iamInstanceProfile: "", diskGb: "" });

  const providersQuery = useQuery({
    queryKey: ["admin", "cloud", "providers"],
    queryFn: () => fetchJSON<DataResponse<CloudProvider[]>>("/admin/cloud/providers"),
  });
  const providers = providersQuery.data?.data ?? [];
  const provider = providers.find((item) => item.kind === selectedProvider);

  const instancesQuery = useQuery({
    queryKey: ["admin", "cloud", "instances", selectedProvider],
    queryFn: () => fetchJSON<DataResponse<CloudInstance[]>>(`/admin/cloud/instances?provider=${encodeURIComponent(selectedProvider)}`),
    enabled: Boolean(selectedProvider),
  });
  const instances = instancesQuery.data?.data ?? [];

  const linksQuery = useQuery({
    queryKey: ["admin", "cloud", "links"],
    queryFn: () => fetchJSON<DataResponse<CloudNodeLink[]>>("/admin/cloud/links"),
  });
  const links = linksQuery.data?.data ?? [];
  const nodesQuery = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });
  const nodes = nodesQuery.data ?? [];

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
  });

  const terminateMutation = useMutation({
    mutationFn: (instance: CloudInstance) => deleteJSON(`/admin/cloud/instances/${encodeURIComponent(instance.provider)}/${encodeURIComponent(instance.id)}`),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["admin", "cloud", "instances", selectedProvider] });
      void queryClient.invalidateQueries({ queryKey: ["admin", "cloud", "links"] });
    },
  });

  const linkedNode = (instance: CloudInstance): ApiNode | undefined => {
    const link = links.find((item) => item.provider === instance.provider && item.instanceId === instance.id);
    return link ? nodes.find((node) => node.id === link.nodeId) : undefined;
  };
  const canProvision = Boolean(provider && form.name.trim() && form.instanceType.trim() && form.image.trim());

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Cloud Providers"
        sub="Provision provider instances and bootstrap linked Beacon nodes automatically."
        action={<Btn tone="primary" onClick={() => setShowProvision(true)} disabled={providers.length === 0}><Plus size={14} /> Provision Instance</Btn>}
      />

      {providersQuery.isError ? <ApiError message={`Could not load providers: ${providersQuery.error.message}`} /> : null}
      <Card>
        <CardHeader title="Configured Providers" icon={Cloud} />
        {providersQuery.isLoading ? <Loading /> : providers.length === 0 ? (
          <EmptyState icon={Cloud} message="No cloud provider is configured. Set AWS_REGION (or AWS_DEFAULT_REGION) and restart the API to enable AWS." />
        ) : (
          <div className="divide-y divide-white/[0.04]">
            {providers.map((item) => (
              <button key={item.kind} type="button" onClick={() => setSelectedProvider(item.kind)} className="flex w-full items-center justify-between px-4 py-3 text-left hover:bg-white/[0.02]">
                <div><p className="text-sm font-medium text-slate-200">{item.name}</p><p className="text-xs text-slate-500">{item.kind.toUpperCase()} · {item.region ?? "region not reported"}</p></div>
                <Pill tone={selectedProvider === item.kind ? "green" : "blue"}>{selectedProvider === item.kind ? "selected" : "configured"}</Pill>
              </button>
            ))}
          </div>
        )}
      </Card>

      <Card>
        <CardHeader title="Provider Instances" icon={Server} />
        {!selectedProvider ? <EmptyState icon={Server} message="Select a configured provider to load its instances." /> : instancesQuery.isLoading ? <Loading /> : instancesQuery.isError ? <ApiError message={`Could not load instances: ${instancesQuery.error.message}`} /> : instances.length === 0 ? <EmptyState icon={Server} message="No instances returned by this provider." /> : (
          <div className="overflow-x-auto"><table className="w-full text-sm"><thead><tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500"><th className="px-4 py-3">Name</th><th className="px-4 py-3">Instance ID</th><th className="px-4 py-3">Type</th><th className="px-4 py-3">Region</th><th className="px-4 py-3">IP</th><th className="px-4 py-3">Panel node</th><th className="px-4 py-3">Status</th><th className="px-4 py-3" /></tr></thead><tbody className="divide-y divide-white/[0.04]">
            {instances.map((instance) => { const node = linkedNode(instance); return <tr key={instance.id} className="hover:bg-white/[0.02]"><td className="px-4 py-3 font-medium text-slate-200">{instance.name || "—"}</td><td className="px-4 py-3 font-mono text-xs text-slate-400">{instance.id}</td><td className="px-4 py-3 text-xs text-slate-400">{instance.instanceType}</td><td className="px-4 py-3 text-xs text-slate-400">{instance.region}</td><td className="px-4 py-3 font-mono text-xs text-slate-400">{instance.publicIp || instance.privateIp || "—"}</td><td className="px-4 py-3 text-xs text-slate-400">{node?.name ?? "Not linked"}</td><td className="px-4 py-3"><Pill tone={instance.status === "running" ? "green" : "yellow"}>{instance.status}</Pill></td><td className="px-4 py-3"><Btn size="sm" tone="danger" disabled={terminateMutation.isPending} onClick={() => { if (confirm(`Terminate ${instance.id}? This cannot be undone.`)) terminateMutation.mutate(instance); }}><Trash2 size={12} /> Terminate</Btn></td></tr>; })}
          </tbody></table></div>
        )}
      </Card>

      {showProvision ? <Modal title="Provision Provider Instance" onClose={() => setShowProvision(false)}><div className="grid gap-4">
        {providers.length === 0 ? <ApiError message="No provider is configured." /> : <label className="block text-sm"><span className="mb-1.5 block font-medium text-slate-300">Provider</span><select value={selectedProvider} onChange={(event) => setSelectedProvider(event.target.value)} className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100"><option value="">Select provider…</option>{providers.map((item) => <option key={item.kind} value={item.kind}>{item.name} ({item.region})</option>)}</select></label>}
        <Input label="Instance name" value={form.name} onChange={(value) => setForm({ ...form, name: value })} placeholder="game-node-1" />
        <Input label="Instance type" value={form.instanceType} onChange={(value) => setForm({ ...form, instanceType: value })} placeholder="t3.medium" />
        <Input label="Image ID" value={form.image} onChange={(value) => setForm({ ...form, image: value })} placeholder="ami-…" />
        <div className="block text-sm"><span className="mb-1.5 block font-medium text-slate-300">Configured region</span><p className="rounded-lg border border-white/10 bg-[#161b28] px-3 py-2 text-sm text-slate-400">{provider?.region ?? "Select a provider"}</p></div>
        <label className="block text-sm"><span className="mb-1.5 block font-medium text-slate-300">Bootstrap as panel node</span><select value={form.nodeId} onChange={(event) => setForm({ ...form, nodeId: event.target.value })} className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100"><option value="">Provision compute only</option>{nodes.map((node) => <option key={node.id} value={node.id}>{node.name}</option>)}</select></label>
        <Input label="Beacon container image" value={form.beaconImage} onChange={(value) => setForm({ ...form, beaconImage: value })} placeholder="configured AWS_BEACON_IMAGE" />
        <Input label="Subnet ID (optional)" value={form.subnetId} onChange={(value) => setForm({ ...form, subnetId: value })} placeholder="subnet-…" />
        <Input label="Security group IDs (comma-separated)" value={form.securityGroupIds} onChange={(value) => setForm({ ...form, securityGroupIds: value })} placeholder="sg-…" />
        <Input label="IAM instance profile (optional)" value={form.iamInstanceProfile} onChange={(value) => setForm({ ...form, iamInstanceProfile: value })} placeholder="gamepanel-beacon" />
        <Input label="Root disk GB (optional)" type="number" value={form.diskGb} onChange={(value) => setForm({ ...form, diskGb: value })} placeholder="50" />
        <p className="text-xs text-slate-500">When linked, Ubuntu cloud-init installs Docker and starts Beacon with the selected node credential. Use a private panel API URL and an IAM role for shared backup access.</p>
        {provisionMutation.isError ? <ApiError message={`Provisioning failed: ${provisionMutation.error.message}`} /> : null}
      </div><ModalFooter onCancel={() => setShowProvision(false)} onConfirm={() => provisionMutation.mutate()} confirmLabel={provisionMutation.isPending ? "Provisioning…" : "Provision"} disabled={provisionMutation.isPending || !canProvision} /></Modal> : null}
    </div>
  );
}

function Loading() { return <div className="p-8 text-center text-sm text-slate-500"><Loader2 size={16} className="mr-2 inline animate-spin" />Loading…</div>; }
function ApiError({ message }: { message: string }) { return <div className="m-4 flex items-start gap-2 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200"><AlertCircle size={16} className="mt-0.5 shrink-0" />{message}</div>; }
