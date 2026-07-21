"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Trash2, RefreshCw, Plus, Eraser } from "lucide-react";
import { listVolumes, createVolume, deleteVolume, pruneVolumes, type DockerVolume } from "@/lib/api/docker";
import { Btn, Card, EmptyState, Input, Modal, ModalFooter } from "@/components/admin/admin-ui";
import { ConfirmDialog, Alert } from "@/components/ui/primitives";

function formatDate(ts: string): string {
  if (!ts) return "";
  try {
    return new Date(ts).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric", hour: "2-digit", minute: "2-digit" });
  } catch {
    return ts;
  }
}

export function VolumesView() {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<DockerVolume | null>(null);
  const [showPruneConfirm, setShowPruneConfirm] = useState(false);

  const volumesQuery = useQuery({
    queryKey: ["docker", "volumes"],
    queryFn: listVolumes,
    refetchInterval: 30_000,
  });

  const volumes = volumesQuery.data ?? [];

  const createMut = useMutation({
    mutationFn: (data: { name: string; driver: string }) =>
      createVolume({ name: data.name, driver: data.driver || undefined }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["docker", "volumes"] }); setShowCreate(false); },
  });

  const deleteMut = useMutation({
    mutationFn: ({ id, nodeId }: { id: string; nodeId: string }) => deleteVolume(id, nodeId),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["docker", "volumes"] }); setDeleteTarget(null); },
  });

  const pruneMut = useMutation({
    mutationFn: () => pruneVolumes(),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["docker", "volumes"] }); setShowPruneConfirm(false); },
  });

  const filtered = volumes.filter(
    (v) => !search || v.name.toLowerCase().includes(search.toLowerCase()) || v.driver.toLowerCase().includes(search.toLowerCase()),
  );

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <div className="flex-1">
          <Input placeholder="Search volumes..." value={search} onChange={setSearch} />
        </div>
        <Btn tone="ghost" onClick={() => void volumesQuery.refetch()}>
          <RefreshCw size={14} />
        </Btn>
        <Btn tone="warning" onClick={() => setShowPruneConfirm(true)}>
          <Eraser size={14} /> Prune Unused
        </Btn>
        <Btn tone="primary" onClick={() => setShowCreate(true)}>
          <Plus size={14} /> Create Volume
        </Btn>
      </div>

      <Card>
        {volumesQuery.isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading volumes...</div>
        ) : volumesQuery.isError ? (
          <div className="p-4 text-sm text-red-400">Failed to load volumes.</div>
        ) : filtered.length === 0 ? (
          <EmptyState icon={RefreshCw} message={search ? "No volumes match your search." : "No volumes found."} title={search ? "No results" : "No volumes"} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] bg-[#161b28] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3">Name</th>
                  <th className="px-4 py-3">Driver</th>
                  <th className="px-4 py-3">Mount Point</th>
                  <th className="px-4 py-3">Node</th>
                  <th className="px-4 py-3">Created</th>
                  <th className="px-4 py-3">Actions</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((v, idx) => (
                  <tr key={`${v.name}-${v.nodeId}-${idx}`} className="border-b border-white/[0.03] hover:bg-white/[0.02]">
                    <td className="px-4 py-3 font-medium text-slate-200">{v.name}</td>
                    <td className="px-4 py-3 text-slate-400">{v.driver}</td>
                    <td className="max-w-[200px] truncate px-4 py-3 font-mono text-[11px] text-slate-400" title={v.mountpoint}>{v.mountpoint}</td>
                    <td className="px-4 py-3 text-slate-400">{v.nodeName || v.nodeId?.slice(0, 8)}</td>
                    <td className="px-4 py-3 text-slate-400">{formatDate(v.createdAt)}</td>
                    <td className="px-4 py-3">
                      <button className="rounded p-1 text-slate-500 hover:bg-white/[0.06] hover:text-red-400" onClick={() => setDeleteTarget(v)} title="Delete" type="button"><Trash2 size={13} /></button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {showCreate && (
        <CreateVolumeModal
          onClose={() => setShowCreate(false)}
          onCreate={(data) => createMut.mutate(data)}
          loading={createMut.isPending}
          error={createMut.error?.message}
        />
      )}

      <ConfirmDialog
        closeAction={() => setDeleteTarget(null)}
        confirmAction={() => deleteMut.mutate({ id: deleteTarget!.name, nodeId: deleteTarget!.nodeId })}
        confirmLabel="Delete"
        destructive
        loading={deleteMut.isPending}
        open={!!deleteTarget}
        title="Delete Volume"
        description={`Are you sure you want to delete volume "${deleteTarget?.name}"?`}
      />

      <ConfirmDialog
        closeAction={() => setShowPruneConfirm(false)}
        confirmAction={() => pruneMut.mutate()}
        confirmLabel="Prune"
        destructive
        loading={pruneMut.isPending}
        open={showPruneConfirm}
        title="Prune Unused Volumes"
        description="Remove all unused local volumes. This cannot be undone."
      />
    </div>
  );
}

function CreateVolumeModal({ onClose, onCreate, loading, error }: { onClose: () => void; onCreate: (data: { name: string; driver: string }) => void; loading: boolean; error?: string }) {
  const [name, setName] = useState("");
  const [driver, setDriver] = useState("local");

  return (
    <Modal onClose={onClose} title="Create Volume">
      <div className="space-y-4">
        {error && <Alert tone="error" title="Create failed">{error}</Alert>}
        <Input label="Volume Name *" placeholder="my-volume" value={name} onChange={setName} />
        <div>
          <label className="mb-1.5 block text-sm font-medium text-slate-300">Driver</label>
          <select className="ui-input" value={driver} onChange={(e) => setDriver(e.target.value)}>
            <option value="local">local</option>
            <option value="nfs">nfs</option>
            <option value="tmpfs">tmpfs</option>
          </select>
        </div>
        <ModalFooter
          onCancel={onClose}
          onConfirm={() => onCreate({ name, driver })}
          confirmLabel={loading ? "Creating..." : "Create"}
          disabled={!name || loading}
        />
      </div>
    </Modal>
  );
}
