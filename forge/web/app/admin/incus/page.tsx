"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Boxes, HardDrive, Play, RefreshCw, Server, Square, Tag, Trash2, RotateCw, Users } from "lucide-react";
import {
  deleteIncusInstance,
  fetchIncusCluster,
  fetchIncusImages,
  fetchIncusInstances,
  fetchIncusNodes,
  fetchIncusProfiles,
  fetchIncusStoragePools,
  restartIncusInstance,
  startIncusInstance,
  stopIncusInstance,
  createIncusInstance,
  type IncusClusterMember,
  type IncusImage,
  type IncusInstance,
  type IncusProfile,
  type IncusStoragePool,
} from "@/lib/api/incus";
import { incusStatusTone } from "@/lib/api/status";
import { AdminPageLayout, AdminTabs, Btn, Card, CardHeader, Input, Modal, Pill, SectionHeader, AdminLoadingState, AdminErrorState } from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";

const TABS = [
  { id: "instances", label: "Instances", icon: Boxes },
  { id: "images", label: "Images", icon: Tag },
  { id: "profiles", label: "Profiles", icon: Users },
  { id: "storage", label: "Storage", icon: HardDrive },
  { id: "cluster", label: "Cluster", icon: Server },
] as const;

type Tab = (typeof TABS)[number]["id"];


export default function IncusAdminPage() {
  const [tab, setTab] = useState<Tab>("instances");
  const [nodeId, setNodeId] = useState<string | undefined>(undefined);
  const [createOpen, setCreateOpen] = useState(false);
  const qc = useQueryClient();
  const { toast } = useToast();

  const nodesQ = useQuery({ queryKey: ["incus-nodes"], queryFn: fetchIncusNodes, retry: 1 });
  const instancesQ = useQuery({ queryKey: ["incus-instances", nodeId], queryFn: () => fetchIncusInstances(nodeId), enabled: tab === "instances" });
  const imagesQ = useQuery({ queryKey: ["incus-images", nodeId], queryFn: () => fetchIncusImages(nodeId), enabled: tab === "images" });
  const profilesQ = useQuery({ queryKey: ["incus-profiles", nodeId], queryFn: () => fetchIncusProfiles(nodeId), enabled: tab === "profiles" });
  const poolsQ = useQuery({ queryKey: ["incus-storage", nodeId], queryFn: () => fetchIncusStoragePools(nodeId), enabled: tab === "storage" });
  const clusterQ = useQuery({ queryKey: ["incus-cluster", nodeId], queryFn: () => fetchIncusCluster(nodeId), enabled: tab === "cluster" });

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["incus-nodes"] });
    void qc.invalidateQueries({ queryKey: ["incus-instances"] });
  };

  const onError = (action: string) => (err: Error) =>
    toast({ tone: "error", title: `Failed to ${action}`, message: err instanceof Error ? err.message : "An error occurred" });
  const onChanged = (action: string, name: string) => () => {
    toast({ tone: "success", title: `${action} · ${name}` });
    void qc.invalidateQueries({ queryKey: ["incus-instances"] });
  };

  const startMut = useMutation({ mutationFn: (name: string) => startIncusInstance(name, nodeId), onSuccess: onChanged("Started", "instance"), onError: onError("start instance") });
  const stopMut = useMutation({ mutationFn: (name: string) => stopIncusInstance(name, { nodeId }), onSuccess: onChanged("Stopped", "instance"), onError: onError("stop instance") });
  const restartMut = useMutation({ mutationFn: (name: string) => restartIncusInstance(name, nodeId), onSuccess: onChanged("Restarted", "instance"), onError: onError("restart instance") });
  const deleteMut = useMutation({ mutationFn: (name: string) => deleteIncusInstance(name, { nodeId }), onSuccess: onChanged("Deleted", "instance"), onError: onError("delete instance") });

  const nodes = nodesQ.data ?? [];
  const pending = startMut.isPending || stopMut.isPending || restartMut.isPending || deleteMut.isPending;

  return (
    <AdminPageLayout>
      <SectionHeader
        title="Incus"
        sub="System containers and virtual machines on nodes with runtime=incus — managed over the Incus REST API with client TLS certificates."
        action={
          <div className="flex flex-wrap items-center gap-2">
            <select
              value={nodeId ?? ""}
              onChange={(e) => setNodeId(e.target.value || undefined)}
              className="h-8 rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 text-xs text-slate-200 outline-none focus:border-[var(--focus)]"
            >
              <option value="">Auto — first incus node</option>
              {nodes.map((n) => <option key={n.id} value={n.id}>{n.name}</option>)}
            </select>
            <Btn size="sm" tone="ghost" onClick={refresh}><RefreshCw size={14} /> Refresh</Btn>
            <Btn size="sm" tone="primary" onClick={() => setCreateOpen(true)}>New instance</Btn>
            <span className="text-xs font-mono text-slate-400">{nodes.length} incus nodes · {tab}</span>
          </div>
        }
      />

      {nodesQ.isSuccess && nodes.length === 0 && (
        <div className="rounded-xl border border-dashed border-[var(--line)] bg-white/[0.02] p-8 text-center">
          <div className="text-sm font-medium text-slate-200">No Incus nodes</div>
          <div className="mt-1 text-xs leading-5 text-slate-400">
            Register a node with <code className="font-mono">runtime=incus</code> and trust the panel&rsquo;s client certificate, or set
            {" "}<code className="font-mono">INCUS_TLS_CERT</code> / <code className="font-mono">INCUS_TLS_KEY</code>.
          </div>
        </div>
      )}

      <AdminTabs tabs={TABS.map((t) => ({ id: t.id, label: t.label, icon: t.icon }))} active={tab} onChange={(id) => setTab(id as Tab)} label="Incus sections" />

      {tab === "instances" && (
        <Card>
          <CardHeader title="Instances" icon={Boxes} />
          {instancesQ.isLoading ? <AdminLoadingState label="Loading instances…" /> : instancesQ.isError ? <div className="p-4"><AdminErrorState message={(instancesQ.error as Error).message} retry={() => void instancesQ.refetch()} /></div> : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="text-left text-xs uppercase tracking-wider text-slate-500"><tr className="border-b border-white/[0.06]"><th className="px-4 py-2.5 font-medium">Name</th><th className="px-4 py-2.5 font-medium">Status</th><th className="px-4 py-2.5 font-medium">Type</th><th className="px-4 py-2.5 font-medium">Profiles</th><th className="px-4 py-2.5 font-medium">Actions</th></tr></thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {(instancesQ.data ?? []).length === 0 ? <tr><td colSpan={5} className="py-8 text-center text-sm text-slate-500">No instances.</td></tr> : (instancesQ.data as IncusInstance[]).map((i) => {
                    const running = i.status === "Running";
                    return (
                      <tr key={`${i.project ?? "default"}/${i.name}`} className="hover:bg-white/[0.02]">
                        <td className="px-4 py-3 font-mono text-xs text-slate-200">{i.name}</td>
                        <td className="px-4 py-3"><Pill tone={incusStatusTone(i.status)}>{i.status}</Pill></td>
                        <td className="px-4 py-3 text-xs text-slate-400">{i.instanceType || "container"}</td>
                        <td className="px-4 py-3 text-xs text-slate-400">{(i.profiles ?? []).join(", ") || "—"}</td>
                        <td className="px-4 py-3">
                          <div className="flex flex-wrap gap-1">
                            <Btn size="sm" tone="ghost" disabled={running || pending} loading={startMut.isPending} onClick={() => startMut.mutate(i.name)} title="Start"><Play size={12} /></Btn>
                            <Btn size="sm" tone="warning" disabled={!running || pending} loading={stopMut.isPending} onClick={() => stopMut.mutate(i.name)} title="Stop"><Square size={12} /></Btn>
                            <Btn size="sm" tone="ghost" disabled={!running || pending} loading={restartMut.isPending} onClick={() => restartMut.mutate(i.name)} title="Restart"><RotateCw size={12} /></Btn>
                            <Btn size="sm" tone="danger" disabled={pending} loading={deleteMut.isPending} onClick={() => deleteMut.mutate(i.name)} title="Delete"><Trash2 size={12} /></Btn>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      )}

      {tab === "images" && (
        <Card>
          <CardHeader title="Images" icon={Tag} />
          {imagesQ.isLoading ? <AdminLoadingState label="Loading images…" /> : imagesQ.isError ? <div className="p-4"><AdminErrorState message={(imagesQ.error as Error).message} retry={() => void imagesQ.refetch()} /></div> : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="text-left text-xs uppercase tracking-wider text-slate-500"><tr className="border-b border-white/[0.06]"><th className="px-4 py-2.5 font-medium">Fingerprint</th><th className="px-4 py-2.5 font-medium">Aliases</th><th className="px-4 py-2.5 font-medium">Type</th><th className="px-4 py-2.5 font-medium">Size</th></tr></thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {(imagesQ.data ?? []).length === 0 ? <tr><td colSpan={4} className="py-8 text-center text-sm text-slate-500">No images.</td></tr> : (imagesQ.data as IncusImage[]).map((img) => (
                    <tr key={img.fingerprint} className="hover:bg-white/[0.02]">
                      <td className="px-4 py-3 font-mono text-xs text-slate-200">{img.fingerprint.slice(0, 12)}</td>
                      <td className="px-4 py-3 text-xs text-slate-400">{(img.aliases ?? []).map((a) => a.name).join(", ") || "—"}</td>
                      <td className="px-4 py-3 text-xs text-slate-400">{img.type || "—"}</td>
                      <td className="px-4 py-3 text-xs text-slate-400">{img.size ? `${Math.round(img.size / (1024 * 1024))} MiB` : "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      )}

      {tab === "profiles" && (
        <Card>
          <CardHeader title="Profiles" icon={Users} />
          {profilesQ.isLoading ? <AdminLoadingState label="Loading profiles…" /> : profilesQ.isError ? <div className="p-4"><AdminErrorState message={(profilesQ.error as Error).message} retry={() => void profilesQ.refetch()} /></div> : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="text-left text-xs uppercase tracking-wider text-slate-500"><tr className="border-b border-white/[0.06]"><th className="px-4 py-2.5 font-medium">Name</th><th className="px-4 py-2.5 font-medium">Description</th><th className="px-4 py-2.5 font-medium">Config</th><th className="px-4 py-2.5 font-medium">Devices</th></tr></thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {(profilesQ.data ?? []).length === 0 ? <tr><td colSpan={4} className="py-8 text-center text-sm text-slate-500">No profiles.</td></tr> : (profilesQ.data as IncusProfile[]).map((p) => (
                    <tr key={p.name} className="hover:bg-white/[0.02]">
                      <td className="px-4 py-3 font-mono text-xs text-slate-200">{p.name}</td>
                      <td className="px-4 py-3 text-xs text-slate-400">{p.description || "—"}</td>
                      <td className="px-4 py-3 text-xs text-slate-400">{Object.keys(p.config ?? {}).length}</td>
                      <td className="px-4 py-3 text-xs text-slate-400">{Object.keys(p.devices ?? {}).length}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      )}

      {tab === "storage" && (
        <Card>
          <CardHeader title="Storage Pools" icon={HardDrive} />
          {poolsQ.isLoading ? <AdminLoadingState label="Loading storage pools…" /> : poolsQ.isError ? <div className="p-4"><AdminErrorState message={(poolsQ.error as Error).message} retry={() => void poolsQ.refetch()} /></div> : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="text-left text-xs uppercase tracking-wider text-slate-500"><tr className="border-b border-white/[0.06]"><th className="px-4 py-2.5 font-medium">Name</th><th className="px-4 py-2.5 font-medium">Driver</th><th className="px-4 py-2.5 font-medium">Status</th></tr></thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {(poolsQ.data ?? []).length === 0 ? <tr><td colSpan={3} className="py-8 text-center text-sm text-slate-500">No storage pools.</td></tr> : (poolsQ.data as IncusStoragePool[]).map((pool) => (
                    <tr key={pool.name} className="hover:bg-white/[0.02]">
                      <td className="px-4 py-3 font-mono text-xs text-slate-200">{pool.name}</td>
                      <td className="px-4 py-3 text-xs text-slate-400">{pool.driver || "—"}</td>
                      <td className="px-4 py-3"><Pill tone={incusStatusTone(pool.status || "unknown")}>{pool.status || "—"}</Pill></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      )}

      {tab === "cluster" && (
        <Card>
          <CardHeader title="Cluster Members" icon={Server} />
          {clusterQ.isLoading ? <AdminLoadingState label="Loading cluster…" /> : clusterQ.isError ? <div className="p-4"><AdminErrorState message={(clusterQ.error as Error).message} retry={() => void clusterQ.refetch()} /></div> : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="text-left text-xs uppercase tracking-wider text-slate-500"><tr className="border-b border-white/[0.06]"><th className="px-4 py-2.5 font-medium">Server</th><th className="px-4 py-2.5 font-medium">Roles</th><th className="px-4 py-2.5 font-medium">URL</th></tr></thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {(clusterQ.data ?? []).length === 0 ? <tr><td colSpan={3} className="py-8 text-center text-sm text-slate-500">Standalone host — no cluster members.</td></tr> : (clusterQ.data as IncusClusterMember[]).map((m) => (
                    <tr key={m.serverName} className="hover:bg-white/[0.02]">
                      <td className="px-4 py-3 font-mono text-xs text-slate-200">{m.serverName}</td>
                      <td className="px-4 py-3 text-xs text-slate-400">{(m.roles ?? []).join(", ") || "—"}</td>
                      <td className="px-4 py-3 font-mono text-xs text-slate-400">{m.url || "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      )}

      {createOpen && <CreateInstanceModal nodeId={nodeId} onClose={() => setCreateOpen(false)} onCreated={(name) => { onChanged("Creating", name)(); setCreateOpen(false); }} onError={onError("create instance")} />}
    </AdminPageLayout>
  );
}

function CreateInstanceModal({ nodeId, onClose, onCreated, onError }: { nodeId?: string; onClose: () => void; onCreated: (name: string) => void; onError: (err: Error) => void }) {
  const [name, setName] = useState("");
  const [image, setImage] = useState("");
  const qc = useQueryClient();
  const createMut = useMutation({
    mutationFn: () => createIncusInstance({ name: name.trim(), source: { type: "image", alias: image.trim() } }, nodeId),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["incus-instances"] }); onCreated(name.trim()); },
    onError: (err: Error) => onError(err),
  });
  return (
    <Modal title="New Incus instance" onClose={onClose} description="Creates an instance from an image alias using the Incus InstancesPost shape.">
      <div className="space-y-4">
        <Input label="Instance name" value={name} onChange={setName} placeholder="my-container" mono />
        <Input label="Image alias / fingerprint" value={image} onChange={setImage} placeholder="ubuntu/24.04" mono />
        <p className="text-xs leading-5 text-slate-500">The panel maps the name and image into a minimal InstancesPost document. For full control (profiles, devices, limits), POST the raw spec via the API.</p>
        <div className="flex justify-end gap-2 pt-2">
          <Btn tone="ghost" onClick={onClose}>Cancel</Btn>
          <Btn tone="primary" loading={createMut.isPending} disabled={!name.trim() || !image.trim()} onClick={() => createMut.mutate()}>Create</Btn>
        </div>
      </div>
    </Modal>
  );
}
