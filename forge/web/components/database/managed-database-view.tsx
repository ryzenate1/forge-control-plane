"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Database, RotateCcw, Trash2, Archive, RefreshCw, Plus, Download } from "lucide-react";
import {
  type ManagedDatabase,
  type ManagedDatabaseBackup,
  type ManagedDatabaseRestore,
  type ManagedDatabaseEngine,
  listManagedDatabases,
  backupManagedDatabase,
  restoreManagedDatabase,
  rotateManagedDatabasePassword,
  deleteManagedDatabase,
  listManagedDatabaseBackups,
} from "@/lib/api/database-containers";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, SectionHeader, Pill } from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";

const selectStyle = "h-10 w-full rounded-lg border border-white/10 bg-surface-card-header px-3.5 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15";

const engineVersions: Record<string, string[]> = {
  postgresql: ["13", "14", "15", "16"],
  mysql: ["8.0", "8.1", "8.2", "8.3"],
  mariadb: ["10", "11"],
  redis: ["6", "7"],
  mongodb: ["6", "7"],
};

export function ManagedDatabaseView() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [selected, setSelected] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);

  const dbsQuery = useQuery({
    queryKey: ["managed-databases"],
    queryFn: () => listManagedDatabases(),
  });
  const dbs = dbsQuery.data ?? [];

  const backupsQuery = useQuery({
    queryKey: ["managed-database-backups", selected],
    queryFn: () => (selected ? listManagedDatabaseBackups(selected) : Promise.resolve([])),
    enabled: !!selected,
  });
  const backups = backupsQuery.data ?? [];

  const invalidate = () => qc.invalidateQueries({ queryKey: ["managed-databases"] });

  const backupMut = useMutation({
    mutationFn: (id: string) => backupManagedDatabase(id),
    onSuccess: () => { invalidate(); toast({ tone: "success", title: "Backup initiated" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Backup failed", message: e.message }),
  });

  const restoreMut = useMutation({
    mutationFn: ({ dbId, backupId }: { dbId: string; backupId: string }) => restoreManagedDatabase(dbId, backupId),
    onSuccess: () => { invalidate(); toast({ tone: "success", title: "Restore initiated" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Restore failed", message: e.message }),
  });

  const rotateMut = useMutation({
    mutationFn: (id: string) => rotateManagedDatabasePassword(id),
    onSuccess: () => { invalidate(); toast({ tone: "success", title: "Password rotation initiated" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Rotation failed", message: e.message }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteManagedDatabase(id),
    onSuccess: () => { setSelected(null); invalidate(); toast({ tone: "success", title: "Database deleted" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Deletion failed", message: e.message }),
  });

  return (
    <div>
      <SectionHeader
        title="Managed Databases"
        sub="One-click database containers with backup and restore"
        action={<Btn onClick={() => setShowCreate(true)}><Plus size={14} /> Create Database</Btn>}
      />

      <Card className="overflow-hidden">
        <CardHeader title="Databases" icon={Database} />
        {dbsQuery.isLoading ? (
          <div className="py-10 text-center text-sm text-slate-500">Loading...</div>
        ) : dbsQuery.isError ? (
          <div className="p-5">
            <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
              <span>Failed to load: {dbsQuery.error.message}</span>
              <Btn size="sm" tone="ghost" onClick={() => void dbsQuery.refetch()}>Retry</Btn>
            </div>
          </div>
        ) : dbs.length === 0 ? (
          <EmptyState icon={Database} message="No managed databases. Create one to get started." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-xs text-slate-500 uppercase tracking-wider">
                  <th className="px-4 py-3 font-semibold">Name</th>
                  <th className="px-4 py-3 font-semibold">Engine</th>
                  <th className="px-4 py-3 font-semibold">Status</th>
                  <th className="px-4 py-3 font-semibold">Port</th>
                  <th className="px-4 py-3 font-semibold">Resources</th>
                  <th className="px-4 py-3 font-semibold">Backups</th>
                  <th className="px-4 py-3 font-semibold" />
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {dbs.map((db) => (
                  <ManagedDBRow
                    key={db.id}
                    db={db}
                    isSelected={selected === db.id}
                    onSelect={() => setSelected(selected === db.id ? null : db.id)}
                    onBackup={(id) => backupMut.mutate(id)}
                    onRestore={(id, backupId) => restoreMut.mutate({ dbId: id, backupId })}
                    onRotate={(id) => rotateMut.mutate(id)}
                    onDelete={(id) => deleteMut.mutate(id)}
                    backups={selected === db.id ? backups : []}
                    isPending={backupMut.isPending || restoreMut.isPending}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {showCreate && (
        <ManagedDBCreateModal
          onClose={() => setShowCreate(false)}
          onCreated={() => { setShowCreate(false); invalidate(); }}
        />
      )}
    </div>
  );
}

function ManagedDBRow({
  db, isSelected, onSelect, onBackup, onRestore, onRotate, onDelete, backups, isPending,
}: {
  db: ManagedDatabase;
  isSelected: boolean;
  onSelect: () => void;
  onBackup: (id: string) => void;
  onRestore: (id: string, backupId: string) => void;
  onRotate: (id: string) => void;
  onDelete: (id: string) => void;
  backups: ManagedDatabaseBackup[];
  isPending: boolean;
}) {
  const statusTone = db.status === "running" ? "green" : db.status === "error" ? "red" : "yellow";
  const completed = backups.filter((b) => b.status === "completed").length;

  return (
    <>
      <tr
        className="cursor-pointer transition-colors hover:bg-white/[0.02]"
        onClick={onSelect}
      >
        <td className="px-4 py-3">
          <div className="font-medium text-slate-200">{db.name}</div>
          <div className="font-mono text-xs text-slate-500">{db.id.slice(0, 8)}</div>
        </td>
        <td className="px-4 py-3">
          <Pill tone="blue">{db.engine} {db.version}</Pill>
        </td>
        <td className="px-4 py-3">
          <Pill tone={statusTone}>{db.status}</Pill>
        </td>
        <td className="px-4 py-3 font-mono text-xs text-slate-400">
          {db.port > 0 ? db.port : "-"}
        </td>
        <td className="px-4 py-3 text-xs text-slate-400">
          {db.memoryMb}MB / {db.cpuShares} CPU
        </td>
        <td className="px-4 py-3 text-xs text-slate-400">
          {completed} completed
        </td>
        <td className="px-4 py-3" onClick={(e) => e.stopPropagation()}>
          <div className="flex items-center justify-end gap-1">
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
              className="grid h-8 w-8 place-items-center rounded text-slate-400 transition-colors hover:bg-white/[0.06] hover:text-blue-200 disabled:opacity-40"
              disabled={isPending}
              onClick={() => onRotate(db.id)}
              title="Rotate Password"
              type="button"
            >
              <RefreshCw size={14} />
            </button>
            <button
              className="grid h-8 w-8 place-items-center rounded text-slate-400 transition-colors hover:bg-white/[0.06] hover:text-red-200 disabled:opacity-40"
              disabled={isPending}
              onClick={() => { if (window.confirm(`Delete managed database ${db.name}?`)) onDelete(db.id); }}
              title="Delete"
              type="button"
            >
              <Trash2 size={14} />
            </button>
          </div>
        </td>
      </tr>
      {isSelected && backups.length > 0 && (
        <tr>
          <td colSpan={7} className="px-4 pb-3">
            <div className="rounded-lg bg-white/[0.02] p-3">
              <div className="mb-2 text-xs font-medium uppercase tracking-wider text-slate-400">Backups</div>
              <div className="space-y-1">
                {backups.map((b) => (
                  <div key={b.id} className="flex items-center justify-between rounded bg-white/[0.02] px-3 py-2 text-xs">
                    <div className="flex items-center gap-2">
                      <Pill tone={b.status === "completed" ? "green" : b.status === "failed" ? "red" : "yellow"}>{b.status}</Pill>
                      <span className="text-slate-300">{b.name}</span>
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="text-slate-500">{b.size > 0 ? `${(b.size / 1024 / 1024).toFixed(2)} MB` : "-"}</span>
                      {b.status === "completed" && (
                        <button
                          className="text-slate-400 transition-colors hover:text-blue-200"
                          onClick={() => onRestore(db.id, b.id)}
                          title="Restore"
                          type="button"
                        >
                          <Download size={14} />
                        </button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </td>
        </tr>
      )}
    </>
  );
}

function ManagedDBCreateModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const { toast } = useToast();
  const [name, setName] = useState("");
  const [engine, setEngine] = useState("postgresql");
  const [version, setVersion] = useState("16");
  const [memoryMb, setMemoryMb] = useState(256);
  const [cpuShares, setCpuShares] = useState(0);

  const createMut = useMutation({
    mutationFn: () =>
      import("@/lib/api/database-containers").then((m) =>
        m.createManagedDatabase({ name, engine: engine as ManagedDatabaseEngine, version, memoryMb, cpuShares })
      ),
    onSuccess: () => { toast({ tone: "success", title: "Database created" }); onCreated(); },
    onError: (e: Error) => toast({ tone: "error", title: "Failed to create", message: e.message }),
  });

  return (
    <Modal title={<span className="text-base font-semibold text-slate-100">Create Managed Database</span>} onClose={onClose} wide>
      <div className="space-y-5">
        <Input label="Name" value={name} onChange={setName} placeholder="my-database" />
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Engine</label>
            <select className={selectStyle} value={engine} onChange={(e) => { setEngine(e.target.value as ManagedDatabaseEngine); setVersion(engineVersions[e.target.value]?.[engineVersions[e.target.value].length - 1] ?? "latest"); }}>
              <option value="postgresql">PostgreSQL</option>
              <option value="mysql">MySQL</option>
              <option value="mariadb">MariaDB</option>
              <option value="redis">Redis</option>
              <option value="mongodb">MongoDB</option>
            </select>
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Version</label>
            <select className={selectStyle} value={version} onChange={(e) => setVersion(e.target.value)}>
              {(engineVersions[engine] ?? []).map((v) => (
                <option key={v} value={v}>{v}</option>
              ))}
            </select>
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Memory (MB)</label>
            <input
              type="number"
              className={selectStyle}
              value={memoryMb}
              onChange={(e) => setMemoryMb(Number(e.target.value))}
              min={64}
              step={64}
            />
            <p className="mt-1 text-[11px] text-slate-500">Min 64 MB. Default 256 MB.</p>
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">CPU Shares</label>
            <input
              type="number"
              className={selectStyle}
              value={cpuShares}
              onChange={(e) => setCpuShares(Number(e.target.value))}
              min={0}
              max={1024}
            />
            <p className="mt-1 text-[11px] text-slate-500">Relative CPU weight. 0 = default (1024).</p>
          </div>
        </div>
      </div>
      <ModalFooter
        onCancel={onClose}
        onConfirm={() => createMut.mutate()}
        disabled={!name || createMut.isPending}
        confirmLabel={createMut.isPending ? "Creating..." : "Create"}
      />
    </Modal>
  );
}
