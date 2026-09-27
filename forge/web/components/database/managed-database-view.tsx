"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Archive, Box, Database, Download, LayoutGrid, List, Plus, RefreshCw, Search, Server, Trash2 } from "lucide-react";
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
import { AdminConfirmDialog, Btn, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, cn } from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";
import { statusTone } from "@/lib/api/status";
import { StatusDot } from "@/components/ui/primitives";
import { DbStatCards } from "./databases-overview";

const selectStyle = "h-10 w-full rounded-lg border border-white/10 bg-surface-card-header px-3.5 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition hover:border-white/20 focus:border-[color-mix(in_srgb,var(--brand)_70%,transparent)] focus:ring-2 focus:ring-[color-mix(in_srgb,var(--brand)_15%,transparent)]";

const engineVersions: Record<string, string[]> = {
  postgresql: ["13", "14", "15", "16"],
  mysql: ["8.0", "8.1", "8.2", "8.3"],
  mariadb: ["10", "11"],
  redis: ["6", "7"],
  mongodb: ["6", "7"],
};

function fmtDate(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleDateString("en-GB", { day: "2-digit", month: "2-digit", year: "numeric" });
}

function ManagedStatusDot({ status }: { status: string }) {
  return <StatusDot status={status} tone={statusTone(status)} />;
}

const filterSelectCls = "h-10 cursor-pointer appearance-none rounded-lg border border-white/[0.08] bg-black/20 pl-3 pr-8 text-xs text-slate-200 outline-none";
const filterSelectWrap = "relative flex flex-col justify-center rounded-lg border border-white/[0.08] bg-black/20 px-3 py-1";

export function ManagedDatabaseView() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [selected, setSelected] = useState<string | null>(null);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const [forceDelete, setForceDelete] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [editingDb, setEditingDb] = useState<ManagedDatabase | null>(null);
  const [search, setSearch] = useState("");
  const [engineFilter, setEngineFilter] = useState("all");
  const [statusFilter, setStatusFilter] = useState("all");
  const [view, setView] = useState<"table" | "cards">("table");
  const [sort, setSort] = useState("name-asc");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const dbsQuery = useQuery({
    queryKey: ["managed-databases"],
    queryFn: () => listManagedDatabases(),
  });
  const dbs = useMemo(() => dbsQuery.data ?? [], [dbsQuery.data]);

  const engines = useMemo(() => [...new Set(dbs.map((db) => db.engine).filter(Boolean))].sort(), [dbs]);
  const statuses = useMemo(() => [...new Set(dbs.map((db) => db.status).filter(Boolean))].sort(), [dbs]);

  function resetPage(update: () => void) {
    setPage(1);
    update();
  }

  const filtered = useMemo(() => {
    const term = search.trim().toLowerCase();
    return dbs.filter((db) => {
      if (engineFilter !== "all" && db.engine !== engineFilter) return false;
      if (statusFilter !== "all" && db.status !== statusFilter) return false;
      if (!term) return true;
      return `${db.name} ${db.engine} ${db.version} ${db.status}`.toLowerCase().includes(term);
    });
  }, [dbs, search, engineFilter, statusFilter]);

  const sorted = useMemo(() => {
    const list = [...filtered];
    switch (sort) {
      case "name-desc": return list.sort((a, b) => b.name.localeCompare(a.name));
      case "status": return list.sort((a, b) => a.status.localeCompare(b.status) || a.name.localeCompare(b.name));
      case "engine": return list.sort((a, b) => a.engine.localeCompare(b.engine) || a.name.localeCompare(b.name));
      default: return list.sort((a, b) => a.name.localeCompare(b.name));
    }
  }, [filtered, sort]);

  const totalPages = Math.max(1, Math.ceil(sorted.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const visible = sorted.slice((currentPage - 1) * pageSize, currentPage * pageSize);
  const hasActiveFilters = Boolean(search.trim() || engineFilter !== "all" || statusFilter !== "all");

  const readyCount = useMemo(
    () => dbs.filter((db) => ["ready", "running"].includes((db.status ?? "").toLowerCase())).length,
    [dbs],
  );
  const failedCount = useMemo(
    () => dbs.filter((db) => ["failed", "error"].includes((db.status ?? "").toLowerCase())).length,
    [dbs],
  );
  const memoryTotal = useMemo(() => dbs.reduce((acc, db) => acc + (db.memoryMb ?? 0), 0), [dbs]);
  const statsLoading = dbsQuery.isLoading;

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
    mutationFn: async ({ id, force }: { id: string; force?: boolean }) => {
      const result = await deleteManagedDatabase(id, force);
      if (!result.ok) throw new Error("The server reported the database was not deleted.");
      return result;
    },
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

      <div className="space-y-4">
        <DbStatCards
          stats={[
            { key: "total", label: "Total Databases", icon: Database, tile: "border-white/[0.08] bg-white/[0.03] text-slate-300", value: statsLoading ? "…" : dbs.length },
            { key: "ready", label: "Ready", icon: Box, tile: "border-emerald-500/25 bg-emerald-500/10 text-emerald-300", value: statsLoading ? "…" : readyCount },
            { key: "failed", label: "Failed / Error", icon: Archive, tile: "border-red-500/25 bg-red-500/10 text-red-300", value: statsLoading ? "…" : failedCount },
            { key: "memory", label: "Memory Total", icon: Server, tile: "border-sky-500/25 bg-sky-500/10 text-sky-300", value: statsLoading ? "…" : `${memoryTotal} MB` },
          ]}
        />

        <div className="flex flex-col gap-2 xl:flex-row xl:items-center">
          <label className="flex min-w-52 flex-1 items-center gap-2 rounded-lg border border-white/[0.08] bg-black/20 px-2.5 py-2">
            <Search size={13} className="shrink-0 text-slate-500" />
            <input
              type="text"
              value={search}
              onChange={(e) => resetPage(() => setSearch(e.target.value))}
              placeholder="Search by name, engine, status…"
              aria-label="Search managed databases"
              className="w-full bg-transparent text-xs text-slate-200 outline-none placeholder:text-slate-600"
            />
          </label>
          <div className="flex flex-wrap items-center gap-2">
            <label className={filterSelectWrap}>
              <span className="text-[10px] leading-3 text-slate-500">Engine</span>
              <select aria-label="Filter by engine" value={engineFilter} onChange={(e) => resetPage(() => setEngineFilter(e.target.value))} className={filterSelectCls + " h-6 border-0 bg-transparent pl-0 text-xs"}>
                <option value="all">All</option>
                {engines.map((e) => <option key={e} value={e}>{e}</option>)}
              </select>
            </label>
            <label className={filterSelectWrap}>
              <span className="text-[10px] leading-3 text-slate-500">Status</span>
              <select aria-label="Filter by status" value={statusFilter} onChange={(e) => resetPage(() => setStatusFilter(e.target.value))} className={filterSelectCls + " h-6 border-0 bg-transparent pl-0 text-xs"}>
                <option value="all">All</option>
                {statuses.map((s) => <option key={s} value={s}>{s}</option>)}
              </select>
            </label>
            <div className="flex gap-1 rounded-lg border border-white/[0.08] bg-black/20 p-1" role="group" aria-label="View mode">
              <button type="button" aria-label="Table view" aria-pressed={view === "table"} onClick={() => setView("table")} className={cn("flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-semibold transition", view === "table" ? "border border-red-500/50 bg-red-500/10 text-red-200" : "text-slate-500 hover:text-slate-300")}>
                <List size={14} /> Table
              </button>
              <button type="button" aria-label="Cards view" aria-pressed={view === "cards"} onClick={() => setView("cards")} className={cn("flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-semibold transition", view === "cards" ? "border border-red-500/50 bg-red-500/10 text-red-200" : "text-slate-500 hover:text-slate-300")}>
                <LayoutGrid size={14} /> Cards
              </button>
            </div>
            {hasActiveFilters && (
              <button type="button" onClick={() => { setSearch(""); setEngineFilter("all"); setStatusFilter("all"); setPage(1); }} className="rounded-lg px-2.5 py-2 text-xs font-semibold text-slate-400 transition hover:text-white">
                Clear
              </button>
            )}
          </div>
        </div>

        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-sm font-bold text-slate-100">Managed Databases ({sorted.length})</h2>
          <label className="flex items-center gap-2 rounded-lg border border-white/[0.08] bg-black/20 px-2.5 py-1.5 text-xs text-slate-300">
            <span className="text-[11px] text-slate-500">Sort by</span>
            <select aria-label="Sort managed databases" value={sort} onChange={(e) => setSort(e.target.value)} className="cursor-pointer appearance-none bg-transparent pr-1 outline-none">
              <option value="name-asc">Name (A → Z)</option>
              <option value="name-desc">Name (Z → A)</option>
              <option value="status">Status</option>
              <option value="engine">Engine</option>
            </select>
          </label>
        </div>

        {dbsQuery.isLoading ? (
          <div className="py-10 text-center text-sm text-slate-300">Loading...</div>
        ) : dbsQuery.isError ? (
          <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
            <span>Failed to load: {dbsQuery.error.message}</span>
            <Btn size="sm" tone="ghost" onClick={() => void dbsQuery.refetch()}>Retry</Btn>
          </div>
        ) : dbs.length === 0 ? (
          <EmptyState icon={Database} message="No managed databases. Create one to get started." />
        ) : sorted.length === 0 ? (
          <EmptyState
            icon={Database}
            title="No matches"
            message="No managed databases match these filters."
          />
        ) : view === "cards" ? (
          <div className="space-y-3">
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
              {visible.map((db) => (
                <ManagedDBCard
                  key={db.id}
                  db={db}
                  isSelected={selected === db.id}
                  onSelect={() => setSelected(selected === db.id ? null : db.id)}
                  onBackup={(id) => backupMut.mutate(id)}
                  onRestore={(id, backupId) => restoreMut.mutate({ dbId: id, backupId })}
                  onRotate={(id) => rotateMut.mutate(id)}
                  onEdit={(item) => setEditingDb(item)}
                  onDelete={(id) => setConfirmDeleteId(id)}
                  backups={selected === db.id ? backups : []}
                  restores={selected === db.id ? restores : []}
                  isPending={backupMut.isPending || restoreMut.isPending || updateMut.isPending}
                />
              ))}
            </div>
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-white/[0.08] bg-[var(--surface)] px-4 py-3 text-xs text-slate-400">
              <span>Showing {(currentPage - 1) * pageSize + 1}–{Math.min(currentPage * pageSize, sorted.length)} of {sorted.length} databases</span>
              <div className="flex items-center gap-2">
                <button type="button" aria-label="Previous page" disabled={currentPage === 1} onClick={() => setPage((p) => Math.max(1, p - 1))} className="grid h-7 w-7 place-items-center rounded-lg border border-white/[0.08] transition hover:border-white/20 disabled:opacity-40">‹</button>
                <span className="grid h-7 min-w-7 place-items-center rounded-lg border border-red-500/40 bg-red-500/10 px-2 font-mono font-bold text-red-200">{currentPage}</span>
                <button type="button" aria-label="Next page" disabled={currentPage === totalPages} onClick={() => setPage((p) => Math.min(totalPages, p + 1))} className="grid h-7 w-7 place-items-center rounded-lg border border-white/[0.08] transition hover:border-white/20 disabled:opacity-40">›</button>
                <label className="ml-1 flex items-center gap-1.5 rounded-lg border border-white/[0.08] px-2 py-1.5">
                  <select aria-label="Rows per page" value={pageSize} onChange={(e) => { setPageSize(Number(e.target.value)); setPage(1); }} className="cursor-pointer appearance-none bg-transparent pr-1 font-mono outline-none">
                    <option value={10}>10 / page</option>
                    <option value={20}>20 / page</option>
                    <option value={50}>50 / page</option>
                  </select>
                </label>
              </div>
            </div>
          </div>
        ) : (
          <div className="overflow-hidden rounded-xl border border-white/[0.08] bg-[var(--surface)] shadow-sm">
            <div className="overflow-x-auto">
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-wider text-slate-500">
                    <th className="px-4 py-3 font-medium">Name</th>
                    <th className="px-2 py-3 font-medium">Engine / Version</th>
                    <th className="px-2 py-3 font-medium">Status</th>
                    <th className="px-2 py-3 font-medium">Host : Port</th>
                    <th className="px-2 py-3 font-medium">Resources</th>
                    <th className="px-2 py-3 text-right font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {visible.map((db) => (
                    <ManagedDBRow
                      key={db.id}
                      db={db}
                      isSelected={selected === db.id}
                      onSelect={() => setSelected(selected === db.id ? null : db.id)}
                      onBackup={(id) => backupMut.mutate(id)}
                      onRestore={(id, backupId) => restoreMut.mutate({ dbId: id, backupId })}
                      onRotate={(id) => rotateMut.mutate(id)}
                      onEdit={(item) => setEditingDb(item)}
                      onDelete={(id) => setConfirmDeleteId(id)}
                      backups={selected === db.id ? backups : []}
                      restores={selected === db.id ? restores : []}
                      isPending={backupMut.isPending || restoreMut.isPending || updateMut.isPending}
                    />
                  ))}
                </tbody>
              </table>
            </div>
            <div className="flex flex-wrap items-center justify-between gap-3 border-t border-white/[0.06] px-4 py-3 text-xs text-slate-400">
              <span>Showing {(currentPage - 1) * pageSize + 1}–{Math.min(currentPage * pageSize, sorted.length)} of {sorted.length} databases</span>
              <div className="flex items-center gap-2">
                <button type="button" aria-label="Previous page" disabled={currentPage === 1} onClick={() => setPage((p) => Math.max(1, p - 1))} className="grid h-7 w-7 place-items-center rounded-lg border border-white/[0.08] transition hover:border-white/20 disabled:opacity-40">‹</button>
                <span className="grid h-7 min-w-7 place-items-center rounded-lg border border-red-500/40 bg-red-500/10 px-2 font-mono font-bold text-red-200">{currentPage}</span>
                <button type="button" aria-label="Next page" disabled={currentPage === totalPages} onClick={() => setPage((p) => Math.min(totalPages, p + 1))} className="grid h-7 w-7 place-items-center rounded-lg border border-white/[0.08] transition hover:border-white/20 disabled:opacity-40">›</button>
                <label className="ml-1 flex items-center gap-1.5 rounded-lg border border-white/[0.08] px-2 py-1.5">
                  <select aria-label="Rows per page" value={pageSize} onChange={(e) => { setPageSize(Number(e.target.value)); setPage(1); }} className="cursor-pointer appearance-none bg-transparent pr-1 font-mono outline-none">
                    <option value={10}>10 / page</option>
                    <option value={20}>20 / page</option>
                    <option value={50}>50 / page</option>
                  </select>
                </label>
              </div>
            </div>
          </div>
        )}
      </div>

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
  const hostPort = db.host ? `${db.host}:${db.port}` : db.port > 0 ? String(db.port) : "—";

  return (
    <>
      <tr
        className="cursor-pointer transition hover:bg-white/[0.02]"
        onClick={onSelect}
      >
        <td className="px-4 py-3">
          <div className="flex items-center gap-2.5">
            <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg border border-white/[0.08] bg-white/[0.03] text-slate-400">
              <Database size={14} />
            </span>
            <span className="min-w-0">
              <span className="block max-w-44 truncate text-xs font-bold text-slate-100" title={db.name}>{db.name}</span>
              <span className="block font-mono text-[10px] text-slate-500">{fmtDate(db.createdAt)}</span>
            </span>
          </div>
        </td>
        <td className="px-2 py-3">
          <span className="block text-xs text-slate-200">{db.engine}</span>
          <span className="block font-mono text-[10px] text-slate-500">{db.version || "—"}</span>
        </td>
        <td className="px-2 py-3">
          <ManagedStatusDot status={db.status} />
        </td>
        <td className="px-2 py-3 font-mono text-[11px] text-slate-300">
          {hostPort}
        </td>
        <td className="px-2 py-3 text-[11px] text-slate-400">
          {db.memoryMb}MB / {db.cpuShares} CPU
        </td>
        <td className="px-2 py-3" onClick={(e) => e.stopPropagation()}>
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
          <td colSpan={6} className="px-4 pb-3">
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

function ManagedDBCard({
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
  const hostPort = db.host ? `${db.host}:${db.port}` : db.port > 0 ? String(db.port) : "—";
  return (
    <div className="rounded-xl border border-white/[0.08] bg-[var(--surface)] p-4 shadow-sm transition hover:border-white/20">
      <div className="flex cursor-pointer items-start gap-3" onClick={onSelect}>
        <span className="grid h-10 w-10 shrink-0 place-items-center rounded-xl border border-white/[0.08] bg-white/[0.03] text-slate-300">
          <Database size={18} />
        </span>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-bold text-slate-100" title={db.name}>{db.name}</p>
          <p className="font-mono text-[10px] text-slate-500">{fmtDate(db.createdAt)}</p>
        </div>
        <ManagedStatusDot status={db.status} />
      </div>
      <div className="mt-3 space-y-1 border-t border-white/[0.06] pt-3 font-mono text-[11px] text-slate-400">
        <p className="truncate">{db.engine}{db.version ? ` ${db.version}` : ""} · {hostPort}</p>
        <p className="truncate">{db.memoryMb}MB / {db.cpuShares} CPU</p>
      </div>
      <div className="mt-3 flex items-center gap-1" onClick={(e) => e.stopPropagation()}>
        <button
          className="grid h-9 flex-1 place-items-center rounded-lg border border-white/[0.08] text-slate-400 transition-colors hover:border-white/20 hover:text-[var(--brand)] disabled:opacity-40"
          disabled={isPending}
          onClick={() => onEdit(db)}
          title="Edit — PATCH /managed-databases/:id"
          type="button"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
        </button>
        <button
          className="grid h-9 flex-1 place-items-center rounded-lg border border-white/[0.08] text-slate-400 transition-colors hover:border-white/20 hover:text-amber-200 disabled:opacity-40"
          disabled={isPending}
          onClick={() => onBackup(db.id)}
          title="Backup"
          type="button"
        >
          <Archive size={14} />
        </button>
        <button
          className="grid h-9 flex-1 place-items-center rounded-lg border border-white/[0.08] text-slate-400 transition-colors hover:border-white/20 hover:text-blue-200 disabled:opacity-40"
          disabled={isPending}
          onClick={() => onRotate(db.id)}
          title="Rotate Password"
          type="button"
        >
          <RefreshCw size={14} />
        </button>
        <button
          className="grid h-9 flex-1 place-items-center rounded-lg border border-white/[0.08] text-slate-400 transition-colors hover:border-white/20 hover:text-red-200 disabled:opacity-40"
          disabled={isPending}
          onClick={() => onDelete(db.id)}
          title="Delete — supports ?force"
          type="button"
        >
          <Trash2 size={14} />
        </button>
      </div>
      {isSelected && (backups.length > 0 || restores.length > 0) && (
        <div className="mt-3 rounded-lg bg-white/[0.02] p-3 space-y-3">
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
      )}
    </div>
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
    <Modal title="Create Managed Database" onClose={onClose} wide>
      <div className="space-y-4">
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
            <p className="mt-1 text-xs text-slate-400">Min 64 MB. Default 256 MB.</p>
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
            <p className="mt-1 text-xs text-slate-400">Relative CPU weight. 0 = default (1024).</p>
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
    <Modal title="Edit Managed Database" description="PATCH /managed-databases/:id" onClose={onClose} wide>
      <div className="space-y-4">
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
        <p className="text-xs text-slate-400">Wires <code className="font-mono">updateManagedDatabase</code> — PATCH /managed-databases/:id with name/version/memoryMb/cpuShares</p>
      </div>
      <ModalFooter onCancel={onClose} onConfirm={() => onSave({ name, version, memoryMb, cpuShares })} disabled={saving} confirmLabel={saving ? "Saving..." : "Save"} />
    </Modal>
  );
}
