"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Database, RotateCcw, Trash2, Archive, Plus } from "lucide-react";
import { type DBContainer, listDBContainers, backupDBContainer, restartDBContainer, deprovisionDBContainer } from "@/lib/api/database-containers";
import { Btn, Card, CardHeader, EmptyState, SectionHeader, Pill } from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";
import { DBContainerCreateModal } from "./container-create-modal";
import { DBContainerCredentialsModal } from "./container-credentials-modal";

export function DBContainerView() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [showCreate, setShowCreate] = useState(false);
  const [credsModal, setCredsModal] = useState<{ id: string; name: string } | null>(null);

  const containersQuery = useQuery({
    queryKey: ["db-containers"],
    queryFn: () => listDBContainers(),
  });
  const containers = containersQuery.data ?? [];

  const invalidate = () => qc.invalidateQueries({ queryKey: ["db-containers"] });

  const restartMut = useMutation({
    mutationFn: (id: string) => restartDBContainer(id),
    onSuccess: () => { invalidate(); toast({ tone: "success", title: "Container restart initiated" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Restart failed", message: e.message }),
  });

  const backupMut = useMutation({
    mutationFn: (id: string) => backupDBContainer(id),
    onSuccess: () => { invalidate(); toast({ tone: "success", title: "Backup initiated" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Backup failed", message: e.message }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deprovisionDBContainer(id),
    onSuccess: () => { invalidate(); toast({ tone: "success", title: "Container deprovisioned" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Deletion failed", message: e.message }),
  });

  return (
    <div>
      <SectionHeader
        title="DB Containers"
        sub="Managed database containers running on cluster nodes"
        action={<Btn onClick={() => setShowCreate(true)}><Plus size={14} /> Create Container</Btn>}
      />

      <Card className="overflow-hidden">
        <CardHeader title="Containers" icon={Database} />
        {containersQuery.isLoading ? (
          <div className="py-10 text-center text-sm text-slate-500">Loading</div>
        ) : containersQuery.isError ? (
          <div className="p-5">
            <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
              <span>Could not load DB containers: {containersQuery.error.message}</span>
              <Btn size="sm" tone="ghost" onClick={() => void containersQuery.refetch()}>Retry</Btn>
            </div>
          </div>
        ) : containers.length === 0 ? (
          <EmptyState icon={Database} message="No database containers. Create one to get started." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-xs text-slate-500 uppercase tracking-wider">
                  <th className="px-4 py-3 font-semibold">Name / ID</th>
                  <th className="px-4 py-3 font-semibold">Engine</th>
                  <th className="px-4 py-3 font-semibold">Status</th>
                  <th className="px-4 py-3 font-semibold">Host : Port</th>
                  <th className="px-4 py-3 font-semibold">Resources</th>
                  <th className="px-4 py-3 font-semibold">Connection</th>
                  <th className="px-4 py-3 font-semibold" />
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {containers.map((db) => (
                  <DBContainerRow
                    key={db.id}
                    db={db}
                    onRestart={(id) => restartMut.mutate(id)}
                    onBackup={(id) => backupMut.mutate(id)}
                    onDelete={(id) => deleteMut.mutate(id)}
                    onShowCreds={(id) => setCredsModal({ id, name: `${db.engine}-${db.version}` })}
                    isPending={restartMut.isPending || backupMut.isPending || deleteMut.isPending}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {showCreate && <DBContainerCreateModal onClose={() => setShowCreate(false)} onCreated={invalidate} />}

      {credsModal && (
        <DBContainerCredentialsModal
          containerId={credsModal.id}
          containerName={credsModal.name}
          onClose={() => setCredsModal(null)}
        />
      )}
    </div>
  );
}

function DBContainerRow({ db, onRestart, onBackup, onDelete, onShowCreds, isPending }: {
  db: DBContainer;
  onRestart: (id: string) => void;
  onBackup: (id: string) => void;
  onDelete: (id: string) => void;
  onShowCreds: (id: string) => void;
  isPending: boolean;
}) {
  const statusTone = db.status === "running" ? "green" : db.status === "stopped" ? "red" : "yellow";

  return (
    <tr className="transition-colors hover:bg-white/[0.02]">
      <td className="px-4 py-3">
        <span className="font-mono text-xs text-slate-400">{db.id.slice(0, 8)}</span>
      </td>
      <td className="px-4 py-3">
        <Pill tone="blue">{db.engine} {db.version}</Pill>
      </td>
      <td className="px-4 py-3">
        <Pill tone={statusTone}>{db.status}</Pill>
      </td>
      <td className="px-4 py-3 font-mono text-xs text-slate-400">
        {db.containerId ? `${db.containerId.slice(0, 12)}:${db.port}` : "-"}
      </td>
      <td className="px-4 py-3 text-xs text-slate-400">
        {db.memoryMb}MB / {db.cpuShares} CPU
      </td>
      <td className="px-4 py-3">
        {db.connectionString ? (
          <Btn size="sm" tone="ghost" onClick={() => onShowCreds(db.id)}>View</Btn>
        ) : (
          <span className="text-xs text-slate-500">Pending</span>
        )}
      </td>
      <td className="px-4 py-3">
        <div className="flex items-center justify-end gap-1">
          <button
            className="grid h-8 w-8 place-items-center rounded text-slate-400 transition-colors hover:bg-white/[0.06] hover:text-slate-200 disabled:opacity-40"
            disabled={isPending}
            onClick={() => onRestart(db.id)}
            title="Restart"
            type="button"
          >
            <RotateCcw size={14} />
          </button>
          <button
            className="grid h-8 w-8 place-items-center rounded text-slate-400 transition-colors hover:bg-white/[0.06] hover:text-amber-200 disabled:opacity-40"
            disabled={isPending}
            onClick={() => onBackup(db.id)}
            title="Backup"
            type="button"
          >
            <Archive size={14} />
          </button>
          <button
            className="grid h-8 w-8 place-items-center rounded text-slate-400 transition-colors hover:bg-white/[0.06] hover:text-red-200 disabled:opacity-40"
            disabled={isPending || db.status === "provisioning"}
            onClick={() => { if (window.confirm(`Delete DB container ${db.id.slice(0, 8)}?`)) onDelete(db.id); }}
            title="Delete"
            type="button"
          >
            <Trash2 size={14} />
          </button>
        </div>
      </td>
    </tr>
  );
}
