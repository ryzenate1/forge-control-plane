"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Archive, Box, Database, LayoutGrid, LoaderCircle, List, Plus, RotateCcw, Search, Server, Trash2 } from "lucide-react";
import { type DBContainer, listDBContainers, backupDBContainer, restartDBContainer, deprovisionDBContainer } from "@/lib/api/database-containers";
import { Btn, EmptyState, AdminErrorState, AdminLoadingState, AdminConfirmDialog, cn } from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";
import { DBContainerCreateModal } from "./container-create-modal";
import { DBContainerCredentialsModal } from "./container-credentials-modal";
import { statusTone } from "@/lib/api/status";
import { StatusDot, Pagination } from "@/components/ui/primitives";
import { DbStatCards, resourcesLabel, type DbStat } from "./databases-overview";
import { isAvailable, isPartial, reportedTotal } from "@/lib/admin/telemetry";
import { resolveTone } from "@/components/ui/forge/status";
import { formatDate } from "@/lib/utils";

/** Shared formatter — this file had its own `en-GB` date layout. */
function fmtDate(iso?: string): string {
  return formatDate(iso, "—");
}

function ContainerStatusDot({ status }: { status: string }) {
  return <StatusDot status={status} tone={statusTone(status)} />;
}

function ContainerActions({ db, onRestart, onBackup, onDelete, onShowCreds, isPending, pendingAction }: {
  db: DBContainer;
  onRestart: (id: string) => void;
  onBackup: (id: string) => void;
  onDelete: (id: string) => void;
  onShowCreds: (id: string) => void;
  isPending: boolean;
  /** Which of this row's own operations is in flight, so the pressed button shows
   *  it rather than every row on the page looking busy. */
  pendingAction: "restart" | "backup" | "delete" | null;
}) {
  const [confirmDelete, setConfirmDelete] = useState(false);

  return (
    <>
      <div className="flex items-center justify-end gap-1">
        {db.connectionString ? (
          <button
            className="rounded-lg border border-line px-3 py-1.5 text-meta font-bold text-text transition hover:border-line-strong focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
            onClick={() => onShowCreds(db.id)}
            type="button"
          >
            View
          </button>
        ) : (
          <span className="px-2 text-xs text-text-subtle" title="The container has not reported a connection string yet">Credentials pending</span>
        )}
        <button
          aria-label={`Restart container ${db.id}`}
          className="grid h-11 w-11 place-items-center rounded text-text-subtle transition-colors hover:bg-overlay-subtle hover:text-text disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
          disabled={isPending}
          onClick={() => onRestart(db.id)}
          title={pendingAction === "restart" ? "Restarting…" : "Restart"}
          type="button"
        >
          {pendingAction === "restart" ? <LoaderCircle aria-hidden="true" className="animate-spin" size={14} /> : <RotateCcw aria-hidden="true" size={14} />}
        </button>
        <button
          aria-label={`Back up container ${db.id}`}
          className="grid h-11 w-11 place-items-center rounded text-text-subtle transition-colors hover:bg-overlay-subtle hover:text-text disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
          disabled={isPending}
          onClick={() => onBackup(db.id)}
          title={pendingAction === "backup" ? "Backing up…" : "Backup"}
          type="button"
        >
          {pendingAction === "backup" ? <LoaderCircle aria-hidden="true" className="animate-spin" size={14} /> : <Archive aria-hidden="true" size={14} />}
        </button>
        <button
          aria-label={`Delete container ${db.id}`}
          className="grid h-11 w-11 place-items-center rounded text-text-subtle transition-colors hover:bg-overlay-subtle hover:text-danger disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
          disabled={isPending || db.status === "provisioning"}
          onClick={() => setConfirmDelete(true)}
          title={db.status === "provisioning" ? "Cannot delete while the container is still provisioning" : `Delete container ${db.id}`}
          type="button"
        >
          <Trash2 aria-hidden="true" size={14} />
        </button>
      </div>
      <AdminConfirmDialog
        confirmLabel="Delete container"
        destructive
        description="The database container and its stored data are removed from the node. This cannot be undone."
        loading={pendingAction === "delete"}
        onCancel={() => setConfirmDelete(false)}
        onConfirm={() => { onDelete(db.id); setConfirmDelete(false); }}
        open={confirmDelete}
        title={`Delete DB container ${db.id.slice(0, 8)}?`}
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
  const containersReady = isAvailable(containersQuery);
  /** Which operation this row itself has in flight — `isPending` alone is page-wide. */
  const rowPendingAction = (id: string): "restart" | "backup" | "delete" | null => {
    if (restartMut.isPending && restartMut.variables === id) return "restart";
    if (backupMut.isPending && backupMut.variables === id) return "backup";
    if (deleteMut.isPending && deleteMut.variables === id) return "delete";
    return null;
  };

  /**
   * Sub-heading, not a second page `<h1>`. The Databases route owns the one page
   * title; this tab heads itself at `<h2>`.
   */
  const stats: DbStat[] = useMemo(() => {
    const running = containers.filter((c) => resolveTone(c.status) === "ok").length;
    const pending = containers.filter((c) => resolveTone(c.status) === "pending").length;
    // A container that reports no memory figure is not a 0MB container.
    const memory = reportedTotal(containers, (c) => c.memoryMb);
    const unknown = <span className="text-text-muted" title="The containers query has not reported">—</span>;
    return [
      { key: "total", label: "Total", icon: Database, hint: "Database containers on cluster nodes", value: containersReady ? containers.length : unknown },
      { key: "running", label: "Running", icon: Box, hint: "Containers the node reported as running", value: containersReady ? running : unknown },
      { key: "pending", label: "Pending", icon: Archive, hint: "Containers still being provisioned", value: containersReady ? pending : unknown },
      {
        key: "memory",
        label: "Memory",
        icon: Server,
        hint: "Sum of what the containers reported",
        value: containersReady
          ? (memory.total === 0 || memory.value === undefined
            ? <span className="text-text-muted" title="No container reported a memory figure">—</span>
            : (
              <span title={isPartial(memory) ? `Only ${memory.reported} of ${memory.total} containers reported memory` : undefined}>
                {`${memory.value}MB`}
                {isPartial(memory) ? <span className="ml-1 text-xs font-normal text-warn">partial</span> : null}
              </span>
            ))
          : unknown,
      },
    ];
  }, [containers, containersReady]);

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
  /** Names the filtered denominator instead of presenting a subset as the total. */
  const containersRangeLabel = sorted.length === 0
    ? "Nothing to show"
    : `Showing ${(currentPage - 1) * pageSize + 1}–${Math.min(currentPage * pageSize, sorted.length)} of ${hasActiveFilters ? `${sorted.length} matched of ${containers.length} containers (filtered)` : `${sorted.length} containers`}`;

  const selectCls = "ui-input w-full cursor-pointer sm:w-44";

  const showCreds = (db: DBContainer) => setCredsModal({ id: db.id, name: `${db.engine}-${db.version}` });

  return (
    <div className="space-y-4">
      {/* `<h2>` sub-heading: the Databases route owns the single `<h1>`, which it
          derives from the registry. This tab used to render `SectionHeader`, adding a
          second page title that changed whenever the tab changed. */}
      <div className="flex flex-wrap items-start justify-between gap-3 border-b border-line pb-3">
        <div className="min-w-0">
          <h2 className="t-title">DB Containers</h2>
          <p className="mt-0.5 max-w-prose text-meta text-text-subtle">
            Database containers running on cluster nodes. These are low-level runtime objects; a
            database created through another surface may appear here too.
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Btn ariaLabel="Refresh containers" disabled={containersQuery.isFetching} onClick={() => void containersQuery.refetch()} tone="ghost">
            <RotateCcw size={14} /> Refresh
          </Btn>
          <Btn onClick={() => setShowCreate(true)}><Plus size={14} /> Create Container</Btn>
        </div>
      </div>

      <DbStatCards stats={stats} />

      <div className="flex flex-col gap-2 xl:flex-row xl:items-center">
        <label className="flex min-w-52 flex-1 items-center gap-2 rounded-lg border border-line bg-overlay-subtle px-2.5 py-2 focus-within:border-line-strong">
          <Search aria-hidden="true" size={13} className="shrink-0 text-text-muted" />
          <input
            aria-label="Search containers"
            className="w-full bg-transparent text-xs text-text outline-none placeholder:text-text-muted"
            onChange={(e) => resetPage(() => setSearch(e.target.value))}
            placeholder="Search containers by id, engine, status…"
            type="search"
            value={search}
          />
        </label>
        <div className="flex flex-wrap items-center gap-2">
          <select aria-label="Filter containers by engine" className={selectCls} onChange={(e) => resetPage(() => setEngineFilter(e.target.value))} value={engineFilter}>
            <option value="all">Engine: all</option>
            {engines.map((e) => <option key={e} value={e}>{e}</option>)}
          </select>
          <select aria-label="Filter containers by status" className={selectCls} onChange={(e) => resetPage(() => setStatusFilter(e.target.value))} value={statusFilter}>
            <option value="all">Status: all</option>
            {statuses.map((s) => <option key={s} value={s}>{s}</option>)}
          </select>
          <div aria-label="View mode" className="flex gap-1 rounded-lg border border-line bg-overlay-subtle p-1" role="group">
            <button aria-pressed={view === "table"} className={cn("flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-semibold transition", view === "table" ? "border border-brand-line bg-brand-subtle text-text" : "border border-transparent text-text-muted hover:text-text")} onClick={() => setView("table")} type="button">
              <List aria-hidden="true" size={14} /> Table
            </button>
            <button aria-pressed={view === "cards"} className={cn("flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-semibold transition", view === "cards" ? "border border-brand-line bg-brand-subtle text-text" : "border border-transparent text-text-muted hover:text-text")} onClick={() => setView("cards")} type="button">
              <LayoutGrid aria-hidden="true" size={14} /> Cards
            </button>
          </div>
          {hasActiveFilters && (
            <button className="rounded-lg px-2.5 py-2 text-xs font-semibold text-text-subtle transition hover:text-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" onClick={() => { setSearch(""); setEngineFilter("all"); setStatusFilter("all"); setPage(1); }} type="button">
              Clear filters
            </button>
          )}
        </div>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <h3 className="t-title">
          Containers <span className="font-normal text-text-subtle">({hasActiveFilters ? `${sorted.length} of ${containers.length} (filtered)` : `${containers.length} container${containers.length === 1 ? "" : "s"}`})</span>
        </h3>
        <label className="flex items-center gap-2 rounded-lg border border-line bg-overlay-subtle px-2.5 py-1.5 text-xs text-text">
          <span className="text-text-subtle">Sort by</span>
          <select aria-label="Sort containers" className="cursor-pointer appearance-none bg-transparent pr-1 outline-none" onChange={(e) => setSort(e.target.value)} value={sort}>
            <option value="name-asc">Name (A → Z)</option>
            <option value="name-desc">Name (Z → A)</option>
            <option value="status">Status</option>
            <option value="engine">Engine</option>
          </select>
        </label>
      </div>

      {containersQuery.isPending ? (
        <AdminLoadingState label="Loading database containers…" />
      ) : containersQuery.isError ? (
        <AdminErrorState
          message={`Could not load DB containers: ${containersQuery.error.message}`}
          retry={() => void containersQuery.refetch()}
        />
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
              <div key={db.id} className="rounded-xl border border-line bg-overlay-subtle p-4 shadow-sm transition hover:border-line-strong">
                <div className="flex items-start gap-3">
                  <span className="grid h-10 w-10 shrink-0 place-items-center rounded-xl border border-line bg-overlay-subtle text-text-subtle">
                    <Box size={18} />
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-mono text-sm font-bold text-text" title={db.id}>{db.id.slice(0, 8)}</p>
                    <p className="font-mono text-meta text-text-muted">{fmtDate(db.createdAt)}</p>
                  </div>
                  <ContainerStatusDot status={db.status} />
                </div>
                <div className="mt-3 space-y-1 border-t border-line pt-3 font-mono text-meta text-text-subtle">
                  <p className="truncate">{db.engine}{db.version ? ` ${db.version}` : ""} · {db.containerId ? `${db.containerId.slice(0, 12)}:${db.port}` : "—"}</p>
                  <p className="truncate">{resourcesLabel(db.memoryMb, db.cpuShares)}</p>
                </div>
                <div className="mt-3 border-t border-line pt-3">
                  <ContainerActions
                    db={db}
                    onRestart={(id) => restartMut.mutate(id)}
                    onBackup={(id) => backupMut.mutate(id)}
                    onDelete={(id) => deleteMut.mutate(id)}
                    onShowCreds={() => showCreds(db)}
                    isPending={isPending}
                    pendingAction={rowPendingAction(db.id)}
                  />
                </div>
              </div>
            ))}
          </div>
          <div className="mt-3 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-overlay-subtle px-4 py-3 text-xs text-text-subtle">
            <span>{containersRangeLabel}</span>
            <div className="flex items-center gap-2">
              <Pagination label="Container pagination" onPageChange={setPage} page={currentPage} pageCount={totalPages} />
              <label className="flex items-center gap-1.5 rounded-lg border border-line px-2 py-1.5">
                <select aria-label="Rows per page" className="cursor-pointer appearance-none bg-transparent pr-1 font-mono outline-none" onChange={(e) => { setPageSize(Number(e.target.value)); setPage(1); }} value={pageSize}>
                  <option value={10}>10 / page</option>
                  <option value={20}>20 / page</option>
                  <option value={50}>50 / page</option>
                </select>
              </label>
            </div>
          </div>
        </div>
      ) : (
        <div className="overflow-hidden rounded-xl border border-line bg-overlay-subtle shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-line text-left text-meta uppercase tracking-wider text-text-muted">
                  <th className="px-4 py-3 font-medium">Name</th>
                  <th className="px-2 py-3 font-medium">Engine / Version</th>
                  <th className="px-2 py-3 font-medium">Status</th>
                  <th className="px-2 py-3 font-medium">Host : Port</th>
                  <th className="px-2 py-3 font-medium">Resources</th>
                  <th className="px-2 py-3 text-right font-medium">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line">
                {visible.map((db) => (
                  <tr key={db.id} className="transition hover:bg-overlay-subtle">
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2.5">
                        <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg border border-line bg-overlay-subtle text-text-subtle">
                          <Box size={14} />
                        </span>
                        <span className="min-w-0">
                          <span className="block max-w-44 truncate font-mono text-xs font-bold text-text" title={db.id}>{db.id.slice(0, 8)}</span>
                          <span className="block font-mono text-meta text-text-muted">{fmtDate(db.createdAt)}</span>
                        </span>
                      </div>
                    </td>
                    <td className="px-2 py-3">
                      <span className="block text-xs text-text">{db.engine}</span>
                      <span className="block font-mono text-meta text-text-muted">{db.version || "—"}</span>
                    </td>
                    <td className="px-2 py-3"><ContainerStatusDot status={db.status} /></td>
                    <td className="px-2 py-3 font-mono text-meta text-text-subtle">
                      {db.containerId ? `${db.containerId.slice(0, 12)}:${db.port}` : "-"}
                    </td>
                    <td className="px-2 py-3 text-meta text-text-subtle">
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
                    pendingAction={rowPendingAction(db.id)}
                      />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line px-4 py-3 text-xs text-text-subtle">
            <span>{containersRangeLabel}</span>
            <div className="flex items-center gap-2">
              <Pagination label="Container pagination" onPageChange={setPage} page={currentPage} pageCount={totalPages} />
              <label className="flex items-center gap-1.5 rounded-lg border border-line px-2 py-1.5">
                <select aria-label="Rows per page" className="cursor-pointer appearance-none bg-transparent pr-1 font-mono outline-none" onChange={(e) => { setPageSize(Number(e.target.value)); setPage(1); }} value={pageSize}>
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
