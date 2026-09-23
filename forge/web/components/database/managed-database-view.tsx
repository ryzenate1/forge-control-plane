"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Database, Trash2, Archive, RefreshCw, Plus, Download } from "lucide-react";
import {
  type ManagedDatabase,
  type ManagedDatabaseBackup,
  type ManagedDatabaseEngine,
  listManagedDatabases,
  backupManagedDatabase,
  restoreManagedDatabase,
  rotateManagedDatabasePassword,
  deleteManagedDatabase,
  listManagedDatabaseBackups,
  listManagedDatabaseRestores,
  updateManagedDatabase,
} from "@/lib/api/database-containers";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, SectionHeader, Pill, AdminConfirmDialog } from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";
import { statusTone } from "@/lib/api/status";

const selectStyle = "h-10 w-full rounded-lg border border-white/10 bg-surface-card-header px-3.5 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition hover:border-white/20 focus:border-[var(--brand)]/70 focus:ring-2 focus:ring-[var(--brand)]/15";

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
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const [forceDelete, setForceDelete] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [editingDb, setEditingDb] = useState<ManagedDatabase | null>(null);

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

  const restoresQuery = useQuery({
    queryKey: ["managed-database-restores", selected],
    queryFn: () => (selected ? listManagedDatabaseRestores(selected) : Promise.resolve([])),
    enabled: !!selected,
  });
  const restores = restoresQuery.data ?? [];

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["managed-databases"] });
    if (selected) {
      qc.invalidateQueries({ queryKey: ["managed-database-backups", selected] });
      qc.invalidateQueries({ queryKey: ["managed-database-restores", selected] });
    }
  };

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

  const updateMut = useMutation({
    mutationFn: ({ id, patch }: { id: string; patch: Partial<{ name: string; version: string; memoryMb: number; cpuShares: number }> }) =>
      updateManagedDatabase(id, patch as Partial<import("@/lib/api/database-containers").CreateManagedDatabaseRequest>),
    onSuccess: () => { invalidate(); setEditingDb(null); toast({ tone: "success", title: "Database updated" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Update failed", message: e.message }),
  });

  const deleteMut = useMutation({
    mutationFn: ({ id, force }: { id: string; force?: boolean }) => deleteManagedDatabase(id, force),
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
      <AdminConfirmDialog
        destructive
        loading={deleteMut.isPending}
        onCancel={() => { setConfirmDeleteId(null); setForceDelete(false); }}
        onConfirm={() => { if (confirmDeleteId) deleteMut.mutate({ id: confirmDeleteId, force: forceDelete }); setConfirmDeleteId(null); setForceDelete(false); }}
        open={Boolean(confirmDeleteId)}
        title={`Delete managed database ${dbs.find((db) => db.id === confirmDeleteId)?.name ?? ""}?`}
        description={`The database and all of its data will be permanently removed${forceDelete ? " (force=true will delete even if remote deprovision fails)" : ""}. This cannot be undone. DELETE /managed-databases/:id${forceDelete ? "?force=true" : ""}`}
      />
      {confirmDeleteId && (
        <div className="flex items-center gap-2 text-xs text-slate-400 mb-3 px-1">
          <input type="checkbox" id="forceDeleteChk" checked={forceDelete} onChange={(e) => setForceDelete(e.target.checked)} />
          <label htmlFor="forceDeleteChk">Force delete (?force=true)</label>
        </div>
      )}

      <Card className="overflow-hidden">
        <CardHeader title="Databases" icon={Database} />
        {dbsQuery.isLoading ? (
          <div className="py-10 text-center text-sm text-slate-300">Loading...</div>
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
                <tr className="border-b border-white/[0.06] text-left text-xs text-slate-400 uppercase tracking-wider">
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
                    onEdit={(db) => setEditingDb(db)}
                    onDelete={(id) => setConfirmDeleteId(id)}
                    backups={selected === db.id ? backups : []}
                    restores={selected === db.id ? restores : []}
                    isPending={backupMut.isPending || restoreMut.isPending || updateMut.isPending}
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
      {editingDb && (
        <ManagedDBEditModal
          db={editingDb}
          onClose={() => setEditingDb(null)}
          onSave={(patch) => updateMut.mutate({ id: editingDb.id, patch })}
          saving={updateMut.isPending}
        />
      )}
    </div>
  );
}

function ManagedDBRow({
  db, isSelected, onSelect, onBackup, onRestore, onRotate, onEdit, onDelete, backups, restores, isPending,
}: {
  db: ManagedDatabase;
  isSelected: boolean;
  onSelect: () => void;
  onBackup: (id: string) => void;
  onRestore: (id: string, backupId: string) => void;
  onRotate: (id: string) => void;
  onEdit: (db: ManagedDatabase) => void;
  onDelete: (id: string) => void;
  backups: ManagedDatabaseBackup[];
  restores: import("@/lib/api/database-containers").ManagedDatabaseRestore[];
  isPending: boolean;
}) {
  const tone = statusTone(db.status);
  const completed = backups.filter((b) => b.status === "completed").length;

  return (
    <>
      <tr
        className="cursor-pointer transition-colors hover:bg-white/[0.02]"
        onClick={onSelect}
      >
        <td className="px-4 py-3">
          <div className="font-medium text-slate-200">{db.name}</div>
          <div className="font-mono text-xs text-slate-400">{db.id.slice(0, 8)}</div>
        </td>
        <td className="px-4 py-3">
          <Pill tone="blue">{db.engine} {db.version}</Pill>
        </td>
        <td className="px-4 py-3">
          <Pill tone={tone}>{db.status}</Pill>
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
              className="grid h-11 w-11 place-items-center rounded text-slate-400 transition-colors hover:bg-white/[0.06] hover:text-[var(--brand)] disabled:opacity-40"
              disabled={isPending}
              onClick={() => onEdit(db)}
              title="Edit — PATCH /managed-databases/:id"
              type="button"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
            </button>
            <button
              className="grid h-11 w-11 place-items-center rounded text-slate-400 transition-colors hover:bg-white/[0.06] hover:text-amber-200 disabled:opacity-40"
              disabled={isPending}
              onClick={() => onBackup(db.id)}
              title="Backup"
              type="button"
            >
              <Archive size={14} />
            </button>
            <button
              className="grid h-11 w-11 place-items-center rounded text-slate-400 transition-colors hover:bg-white/[0.06] hover:text-blue-200 disabled:opacity-40"
              disabled={isPending}
              onClick={() => onRotate(db.id)}
              title="Rotate Password"
              type="button"
            >
              <RefreshCw size={14} />
            </button>
            <button
              className="grid h-11 w-11 place-items-center rounded text-slate-400 transition-colors hover:bg-white/[0.06] hover:text-red-200 disabled:opacity-40"
              disabled={isPending}
              onClick={() => onDelete(db.id)}
              title="Delete — supports ?force"
              type="button"
            >
              <Trash2 size={14} />
            </button>
          </div>
        </td>
      </tr>
      {isSelected && (backups.length > 0 || restores.length > 0) && (
        <tr>
          <td colSpan={7} className="px-4 pb-3">
            <div className="rounded-lg bg-white/[0.02] p-3 space-y-3">
              {backups.length > 0 && (
                <div>
                  <div className="mb-2 text-xs font-medium uppercase tracking-wider text-slate-400">Backups — GET /managed-databases/:id/backups</div>
                  <div className="space-y-1">
                    {backups.map((b) => (
                      <div key={b.id} className="flex items-center justify-between rounded bg-white/[0.02] px-3 py-2 text-xs">
                        <div className="flex items-center gap-2">
                          <Pill tone={b.status === "completed" ? "green" : b.status === "failed" ? "red" : "yellow"}>{b.status}</Pill>
                          <span className="text-slate-300">{b.name}</span>
                        </div>
                        <div className="flex items-center gap-2">
                          <span className="text-slate-400">{b.size > 0 ? `${(b.size / 1024 / 1024).toFixed(2)} MB` : "-"}</span>
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
              )}
              {restores.length > 0 && (
                <div>
                  <div className="mb-2 text-xs font-medium uppercase tracking-wider text-slate-400">Restores — GET /managed-databases/:id/restores</div>
                  <div className="space-y-1">
                    {restores.map((r) => (
                      <div key={r.id} className="flex items-center justify-between rounded bg-white/[0.02] px-3 py-2 text-xs">
                        <div className="flex items-center gap-2">
                          <Pill tone={r.status === "completed" ? "green" : r.status === "failed" ? "red" : "yellow"}>{r.status}</Pill>
                          <span className="text-slate-300">{r.id.slice(0,8)}</span>
                          {r.backupId && <span className="text-slate-400">backup:{r.backupId.slice(0,8)}</span>}
                        </div>
                        <div className="flex items-center gap-2">
                          {r.errorMessage && <span className="text-red-400 truncate max-w-[200px]">{r.errorMessage}</span>}
                          <span className="text-slate-500">{new Date(r.createdAt).toLocaleDateString()}</span>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}
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
            <p className="mt-1 text-[11px] text-slate-400">Min 64 MB. Default 256 MB.</p>
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
            <p className="mt-1 text-[11px] text-slate-400">Relative CPU weight. 0 = default (1024).</p>
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

function ManagedDBEditModal({ db, onClose, onSave, saving }: { db: ManagedDatabase; onClose: () => void; onSave: (patch: Partial<{ name: string; version: string; memoryMb: number; cpuShares: number }>) => void; saving: boolean }) {
  const [name, setName] = useState(db.name);
  const [version, setVersion] = useState(db.version);
  const [memoryMb, setMemoryMb] = useState(db.memoryMb);
  const [cpuShares, setCpuShares] = useState(db.cpuShares);
  return (
    <Modal title={<span className="text-base font-semibold text-slate-100">Edit Managed Database — PATCH /managed-databases/:id</span>} onClose={onClose} wide>
      <div className="space-y-5">
        <Input label="Name" value={name} onChange={setName} placeholder={db.name} />
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Version</label>
            <select className={selectStyle} value={version} onChange={(e) => setVersion(e.target.value)}>
              {(engineVersions[db.engine] ?? [db.version]).map((v) => (
                <option key={v} value={v}>{v}</option>
              ))}
              {!engineVersions[db.engine]?.includes(version) && <option value={version}>{version} (current)</option>}
            </select>
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Memory (MB)</label>
            <input type="number" className={selectStyle} value={memoryMb} onChange={(e) => setMemoryMb(Number(e.target.value))} min={64} step={64} />
          </div>
        </div>
        <div>
          <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">CPU Shares</label>
          <input type="number" className={selectStyle} value={cpuShares} onChange={(e) => setCpuShares(Number(e.target.value))} min={0} max={1024} />
        </div>
        <p className="text-[11px] text-slate-500">Wires <code className="font-mono">updateManagedDatabase</code> — PATCH /managed-databases/:id with name/version/memoryMb/cpuShares</p>
      </div>
      <ModalFooter onCancel={onClose} onConfirm={() => onSave({ name, version, memoryMb, cpuShares })} disabled={saving} confirmLabel={saving ? "Saving..." : "Save"} />
    </Modal>
  );
}
