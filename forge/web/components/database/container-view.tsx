"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Archive, Box, Database, LayoutGrid, List, Plus, RotateCcw, Search, Server, Trash2 } from "lucide-react";
import { type DBContainer, listDBContainers, backupDBContainer, restartDBContainer, deprovisionDBContainer } from "@/lib/api/database-containers";
import { Btn, EmptyState, SectionHeader, AdminConfirmDialog, cn } from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";
import { DBContainerCreateModal } from "./container-create-modal";
import { DBContainerCredentialsModal } from "./container-credentials-modal";
import { statusTone } from "@/lib/api/status";
import { DbStatCards, type DbStat } from "./databases-overview";

function fmtDate(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleDateString("en-GB", { day: "2-digit", month: "2-digit", year: "numeric" });
}

function ContainerStatusDot({ status }: { status: string }) {
  const tone = statusTone(status);
  const color = tone === "green" ? "bg-emerald-400" : tone === "yellow" ? "bg-amber-400" : tone === "red" ? "bg-red-400" : tone === "blue" ? "bg-sky-400" : "bg-slate-500";
  const text = tone === "green" ? "text-emerald-300" : tone === "yellow" ? "text-amber-300" : tone === "red" ? "text-red-300" : tone === "blue" ? "text-sky-300" : "text-slate-300";
  return (
    <span className={`inline-flex items-center gap-1.5 text-xs font-semibold capitalize ${text}`}>
      <span className={`h-1.5 w-1.5 rounded-full ${color}`} />{status}
    </span>
  );
}

function ContainerActions({ db, onRestart, onBackup, onDelete, onShowCreds, isPending }: {
  db: DBContainer;
  onRestart: (id: string) => void;
  onBackup: (id: string) => void;
  onDelete: (id: string) => void;
  onShowCreds: (id: string) => void;
  isPending: boolean;
}) {
  const [confirmDelete, setConfirmDelete] = useState(false);

  return (
    <>
      <div className="flex items-center justify-end gap-1">
        {db.connectionString ? (
          <button
            type="button"
            onClick={() => onShowCreds(db.id)}
            className="rounded-lg border border-white/[0.08] px-3 py-1.5 text-[11px] font-bold text-slate-200 transition hover:border-white/20 hover:text-white"
          >
            View
          </button>
        ) : (
          <span className="px-2 text-xs text-slate-400">Pending</span>
        )}
        <button
          className="grid h-11 w-11 place-items-center rounded text-slate-400 transition-colors hover:bg-white/[0.06] hover:text-slate-200 disabled:opacity-40"
          disabled={isPending}
          onClick={() => onRestart(db.id)}
          title="Restart"
          type="button"
        >
          <RotateCcw size={14} />
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
          className="grid h-11 w-11 place-items-center rounded text-slate-400 transition-colors hover:bg-white/[0.06] hover:text-red-200 disabled:opacity-40"
          disabled={isPending || db.status === "provisioning"}
          onClick={() => setConfirmDelete(true)}
          title="Delete"
          type="button"
        >
          <Trash2 size={14} />
        </button>
      </div>
      <AdminConfirmDialog
        destructive
        loading={isPending}
        onCancel={() => setConfirmDelete(false)}
        onConfirm={() => { onDelete(db.id); setConfirmDelete(false); }}
        open={confirmDelete}
        title={`Delete DB container ${db.id.slice(0, 8)}?`}
        description="The database container and its data will be permanently removed. This cannot be undone."
      />
    </>
  );
}

export function DBContainerView() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [showCreate, setShowCreate] = useState(false);
  const [credsModal, setCredsModal] = useState<{ id: string; name: string } | null>(null);
  const [search, setSearch] = useState("");
  const [engineFilter, setEngineFilter] = useState("all");
  const [statusFilter, setStatusFilter] = useState("all");
  const [view, setView] = useState<"table" | "cards">("table");
  const [sort, setSort] = useState("name-asc");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  const containersQuery = useQuery({
    queryKey: ["db-containers"],
    queryFn: () => listDBContainers(),
  });
  const containers = useMemo(() => containersQuery.data ?? [], [containersQuery.data]);

  const invalidate = () => qc.invalidateQueries({ queryKey: ["db-containers"] });

  const restartMut = useMutation({
    mutationFn: async (id: string) => {
      const result = await restartDBContainer(id);
      if (!result.ok) throw new Error("The server reported the container restart did not complete.");
      return result;
    },
    onSuccess: () => { invalidate(); toast({ tone: "success", title: "Container restart initiated" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Restart failed", message: e.message }),
  });

  const backupMut = useMutation({
    mutationFn: async (id: string) => {
      const result = await backupDBContainer(id);
      if (!result.ok) throw new Error("The server reported the backup did not complete.");
      return result;
    },
    onSuccess: () => { invalidate(); toast({ tone: "success", title: "Backup initiated" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Backup failed", message: e.message }),
  });

  const deleteMut = useMutation({
    mutationFn: async (id: string) => {
      const result = await deprovisionDBContainer(id);
      if (!result.ok) throw new Error("The server reported the container was not deprovisioned.");
      return result;
    },
    onSuccess: () => { invalidate(); toast({ tone: "success", title: "Container deprovisioned" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Deletion failed", message: e.message }),
  });

  const isPending = restartMut.isPending || backupMut.isPending || deleteMut.isPending;
  const loading = containersQuery.isLoading;

  const stats: DbStat[] = useMemo(() => {
    const running = containers.filter((c) => ["running", "ready"].includes(c.status.toLowerCase())).length;
    const pending = containers.filter((c) => ["pending", "provisioning", "creating"].includes(c.status.toLowerCase())).length;
    const memorySum = containers.reduce((sum, c) => sum + (c.memoryMb ?? 0), 0);
    return [
      { key: "total", label: "Total", icon: Database, tile: "border-white/[0.08] bg-white/[0.03] text-slate-300", value: loading ? "…" : containers.length },
      { key: "running", label: "Running", icon: Box, tile: "border-emerald-500/25 bg-emerald-500/10 text-emerald-300", value: loading ? "…" : running },
      { key: "pending", label: "Pending", icon: Archive, tile: "border-amber-500/25 bg-amber-500/10 text-amber-300", value: loading ? "…" : pending },
      { key: "memory", label: "Memory", icon: Server, tile: "border-sky-500/25 bg-sky-500/10 text-sky-300", value: loading ? "…" : containers.length === 0 ? "—" : `${memorySum}MB` },
    ];
  }, [containers, loading]);

  const engines = useMemo(() => [...new Set(containers.map((c) => c.engine).filter(Boolean))].sort(), [containers]);
  const statuses = useMemo(() => [...new Set(containers.map((c) => c.status).filter(Boolean))].sort(), [containers]);

  function resetPage(update: () => void) {
    setPage(1);
    update();
  }

  const filtered = useMemo(() => {
    const term = search.trim().toLowerCase();
    return containers.filter((c) => {
      if (engineFilter !== "all" && c.engine !== engineFilter) return false;
      if (statusFilter !== "all" && c.status !== statusFilter) return false;
      if (!term) return true;
      return `${c.id} ${c.engine} ${c.version} ${c.status}`.toLowerCase().includes(term);
    });
  }, [containers, search, engineFilter, statusFilter]);

  const sorted = useMemo(() => {
    const list = [...filtered];
    switch (sort) {
      case "name-desc": return list.sort((a, b) => b.id.localeCompare(a.id));
      case "status": return list.sort((a, b) => a.status.localeCompare(b.status) || a.id.localeCompare(b.id));
      case "engine": return list.sort((a, b) => a.engine.localeCompare(b.engine) || a.id.localeCompare(b.id));
      default: return list.sort((a, b) => a.id.localeCompare(b.id));
    }
  }, [filtered, sort]);

  const totalPages = Math.max(1, Math.ceil(sorted.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const visible = sorted.slice((currentPage - 1) * pageSize, currentPage * pageSize);
  const hasActiveFilters = Boolean(search.trim() || engineFilter !== "all" || statusFilter !== "all");

  const selectCls = "h-10 cursor-pointer appearance-none rounded-lg border border-white/[0.08] bg-black/20 pl-3 pr-8 text-xs text-slate-200 outline-none";
  const selectWrap = "relative flex flex-col justify-center rounded-lg border border-white/[0.08] bg-black/20 px-3 py-1";

  const showCreds = (db: DBContainer) => setCredsModal({ id: db.id, name: `${db.engine}-${db.version}` });

  return (
    <div className="space-y-4">
      <SectionHeader
        title="DB Containers"
        sub="Managed database containers running on cluster nodes"
        action={<Btn onClick={() => setShowCreate(true)}><Plus size={14} /> Create Container</Btn>}
      />

      <DbStatCards stats={stats} />

      <div className="flex flex-col gap-2 xl:flex-row xl:items-center">
        <label className="flex min-w-52 flex-1 items-center gap-2 rounded-lg border border-white/[0.08] bg-black/20 px-2.5 py-2">
          <Search size={13} className="shrink-0 text-slate-500" />
          <input
            type="text"
            value={search}
            onChange={(e) => resetPage(() => setSearch(e.target.value))}
            placeholder="Search containers by id, engine, status…"
            aria-label="Search containers"
            className="w-full bg-transparent text-xs text-slate-200 outline-none placeholder:text-slate-600"
          />
        </label>
        <div className="flex flex-wrap items-center gap-2">
          <label className={selectWrap}>
            <span className="text-[10px] leading-3 text-slate-500">Engine</span>
            <select aria-label="Filter by engine" value={engineFilter} onChange={(e) => resetPage(() => setEngineFilter(e.target.value))} className={selectCls + " h-6 border-0 bg-transparent pl-0 text-xs"}>
              <option value="all">All</option>
              {engines.map((e) => <option key={e} value={e}>{e}</option>)}
            </select>
          </label>
          <label className={selectWrap}>
            <span className="text-[10px] leading-3 text-slate-500">Status</span>
            <select aria-label="Filter by status" value={statusFilter} onChange={(e) => resetPage(() => setStatusFilter(e.target.value))} className={selectCls + " h-6 border-0 bg-transparent pl-0 text-xs"}>
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
        <h2 className="text-sm font-bold text-slate-100">Containers ({sorted.length})</h2>
        <label className="flex items-center gap-2 rounded-lg border border-white/[0.08] bg-black/20 px-2.5 py-1.5 text-xs text-slate-300">
          <span className="text-[11px] text-slate-500">Sort by</span>
          <select aria-label="Sort containers" value={sort} onChange={(e) => setSort(e.target.value)} className="cursor-pointer appearance-none bg-transparent pr-1 outline-none">
            <option value="name-asc">Name (A → Z)</option>
            <option value="name-desc">Name (Z → A)</option>
            <option value="status">Status</option>
            <option value="engine">Engine</option>
          </select>
        </label>
      </div>

      {containersQuery.isLoading ? (
        <div className="py-10 text-center text-sm text-slate-300">Loading</div>
      ) : containersQuery.isError ? (
        <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
          <span>Could not load DB containers: {containersQuery.error.message}</span>
          <Btn size="sm" tone="ghost" onClick={() => void containersQuery.refetch()}>Retry</Btn>
        </div>
      ) : sorted.length === 0 ? (
        <EmptyState
          icon={Database}
          title={hasActiveFilters ? "No matches" : "No database containers"}
          message={hasActiveFilters ? "No containers match these filters." : "No database containers. Create one to get started."}
        />
      ) : view === "cards" ? (
        <div>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {visible.map((db) => (
              <div key={db.id} className="rounded-xl border border-white/[0.08] bg-[var(--surface)] p-4 shadow-sm transition hover:border-white/20">
                <div className="flex items-start gap-3">
                  <span className="grid h-10 w-10 shrink-0 place-items-center rounded-xl border border-white/[0.08] bg-white/[0.03] text-slate-300">
                    <Box size={18} />
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-mono text-sm font-bold text-slate-100" title={db.id}>{db.id.slice(0, 8)}</p>
                    <p className="font-mono text-[10px] text-slate-500">{fmtDate(db.createdAt)}</p>
                  </div>
                  <ContainerStatusDot status={db.status} />
                </div>
                <div className="mt-3 space-y-1 border-t border-white/[0.06] pt-3 font-mono text-[11px] text-slate-400">
                  <p className="truncate">{db.engine}{db.version ? ` ${db.version}` : ""} · {db.containerId ? `${db.containerId.slice(0, 12)}:${db.port}` : "—"}</p>
                  <p className="truncate">{db.memoryMb}MB / {db.cpuShares} CPU</p>
                </div>
                <div className="mt-3 border-t border-white/[0.06] pt-3">
                  <ContainerActions
                    db={db}
                    onRestart={(id) => restartMut.mutate(id)}
                    onBackup={(id) => backupMut.mutate(id)}
                    onDelete={(id) => deleteMut.mutate(id)}
                    onShowCreds={() => showCreds(db)}
                    isPending={isPending}
                  />
                </div>
              </div>
            ))}
          </div>
          <div className="mt-3 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-white/[0.08] bg-[var(--surface)] px-4 py-3 text-xs text-slate-400">
            <span>Showing {(currentPage - 1) * pageSize + 1}–{Math.min(currentPage * pageSize, sorted.length)} of {sorted.length} containers</span>
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
                  <tr key={db.id} className="transition hover:bg-white/[0.02]">
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2.5">
                        <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg border border-white/[0.08] bg-white/[0.03] text-slate-400">
                          <Box size={14} />
                        </span>
                        <span className="min-w-0">
                          <span className="block max-w-44 truncate font-mono text-xs font-bold text-slate-100" title={db.id}>{db.id.slice(0, 8)}</span>
                          <span className="block font-mono text-[10px] text-slate-500">{fmtDate(db.createdAt)}</span>
                        </span>
                      </div>
                    </td>
                    <td className="px-2 py-3">
                      <span className="block text-xs text-slate-200">{db.engine}</span>
                      <span className="block font-mono text-[10px] text-slate-500">{db.version || "—"}</span>
                    </td>
                    <td className="px-2 py-3"><ContainerStatusDot status={db.status} /></td>
                    <td className="px-2 py-3 font-mono text-[11px] text-slate-300">
                      {db.containerId ? `${db.containerId.slice(0, 12)}:${db.port}` : "-"}
                    </td>
                    <td className="px-2 py-3 text-[11px] text-slate-400">
                      {db.memoryMb}MB / {db.cpuShares} CPU
                    </td>
                    <td className="px-2 py-3">
                      <ContainerActions
                        db={db}
                        onRestart={(id) => restartMut.mutate(id)}
                        onBackup={(id) => backupMut.mutate(id)}
                        onDelete={(id) => deleteMut.mutate(id)}
                        onShowCreds={() => showCreds(db)}
                        isPending={isPending}
                      />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-white/[0.06] px-4 py-3 text-xs text-slate-400">
            <span>Showing {(currentPage - 1) * pageSize + 1}–{Math.min(currentPage * pageSize, sorted.length)} of {sorted.length} containers</span>
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
