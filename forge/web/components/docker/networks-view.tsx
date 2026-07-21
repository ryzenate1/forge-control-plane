"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Trash2, RefreshCw, Plus } from "lucide-react";
import { listNetworks, createNetwork, deleteNetwork, type DockerNetwork } from "@/lib/api/docker";
import { Btn, Card, EmptyState, Input, Modal, ModalFooter } from "@/components/admin/admin-ui";
import { ConfirmDialog, Alert } from "@/components/ui/primitives";

export function NetworksView() {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<DockerNetwork | null>(null);

  const networksQuery = useQuery({
    queryKey: ["docker", "networks"],
    queryFn: listNetworks,
    refetchInterval: 30_000,
  });

  const networks = networksQuery.data ?? [];

  const createMut = useMutation({
    mutationFn: (data: { name: string; driver: string; subnet: string }) =>
      createNetwork({ name: data.name, driver: data.driver, subnet: data.subnet || undefined }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["docker", "networks"] }); setShowCreate(false); },
  });

  const deleteMut = useMutation({
    mutationFn: ({ id, nodeId }: { id: string; nodeId: string }) => deleteNetwork(id, nodeId),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["docker", "networks"] }); setDeleteTarget(null); },
  });

  const filtered = networks.filter(
    (n) => !search || n.name.toLowerCase().includes(search.toLowerCase()) || n.driver.toLowerCase().includes(search.toLowerCase()),
  );

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <div className="flex-1">
          <Input placeholder="Search networks..." value={search} onChange={setSearch} />
        </div>
        <Btn tone="ghost" onClick={() => void networksQuery.refetch()}>
          <RefreshCw size={14} />
        </Btn>
        <Btn tone="primary" onClick={() => setShowCreate(true)}>
          <Plus size={14} /> Create Network
        </Btn>
      </div>

      <Card>
        {networksQuery.isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading networks...</div>
        ) : networksQuery.isError ? (
          <div className="p-4 text-sm text-red-400">Failed to load networks.</div>
        ) : filtered.length === 0 ? (
          <EmptyState icon={RefreshCw} message={search ? "No networks match your search." : "No networks found."} title={search ? "No results" : "No networks"} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] bg-[#161b28] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3">Name</th>
                  <th className="px-4 py-3">Driver</th>
                  <th className="px-4 py-3">Scope</th>
                  <th className="px-4 py-3">Attached Containers</th>
                  <th className="px-4 py-3">Node</th>
                  <th className="px-4 py-3">Actions</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((n, idx) => (
                  <tr key={`${n.id}-${n.nodeId}-${idx}`} className="border-b border-white/[0.03] hover:bg-white/[0.02]">
                    <td className="px-4 py-3 font-medium text-slate-200">{n.name}</td>
                    <td className="px-4 py-3 text-slate-400">{n.driver}</td>
                    <td className="px-4 py-3 text-slate-400">{n.scope}</td>
                    <td className="px-4 py-3 text-slate-400">{n.attached}</td>
                    <td className="px-4 py-3 text-slate-400">{n.nodeName || n.nodeId?.slice(0, 8)}</td>
                    <td className="px-4 py-3">
                      {n.name !== "bridge" && n.name !== "host" && n.name !== "none" && (
                        <button className="rounded p-1 text-slate-500 hover:bg-white/[0.06] hover:text-red-400" onClick={() => setDeleteTarget(n)} title="Delete" type="button"><Trash2 size={13} /></button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {showCreate && (
        <CreateNetworkModal
          onClose={() => setShowCreate(false)}
          onCreate={(data) => createMut.mutate(data)}
          loading={createMut.isPending}
          error={createMut.error?.message}
        />
      )}

      <ConfirmDialog
        closeAction={() => setDeleteTarget(null)}
        confirmAction={() => deleteMut.mutate({ id: deleteTarget!.id, nodeId: deleteTarget!.nodeId })}
        confirmLabel="Delete"
        destructive
        loading={deleteMut.isPending}
        open={!!deleteTarget}
        title="Delete Network"
        description={`Are you sure you want to delete network "${deleteTarget?.name}"?`}
      />
    </div>
  );
}

function CreateNetworkModal({ onClose, onCreate, loading, error }: { onClose: () => void; onCreate: (data: { name: string; driver: string; subnet: string }) => void; loading: boolean; error?: string }) {
  const [name, setName] = useState("");
  const [driver, setDriver] = useState("bridge");
  const [subnet, setSubnet] = useState("");

  return (
    <Modal onClose={onClose} title="Create Network">
      <div className="space-y-4">
        {error && <Alert tone="error" title="Create failed">{error}</Alert>}
        <Input label="Network Name *" placeholder="my-network" value={name} onChange={setName} />
        <div>
          <label className="mb-1.5 block text-sm font-medium text-slate-300">Driver</label>
          <select className="ui-input" value={driver} onChange={(e) => setDriver(e.target.value)}>
            <option value="bridge">bridge</option>
            <option value="host">host</option>
            <option value="overlay">overlay</option>
            <option value="macvlan">macvlan</option>
            <option value="ipvlan">ipvlan</option>
          </select>
        </div>
        <Input label="Subnet (optional)" placeholder="172.20.0.0/16" value={subnet} onChange={setSubnet} />
        <ModalFooter
          onCancel={onClose}
          onConfirm={() => onCreate({ name, driver, subnet })}
          confirmLabel={loading ? "Creating..." : "Create"}
          disabled={!name || loading}
        />
      </div>
    </Modal>
  );
}
