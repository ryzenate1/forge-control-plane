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
import { Btn, EmptyState, Input, Modal, ModalFooter, AdminErrorState, AdminLoadingState, Pill, cn, AdminTable, AdminTHead, AdminTh, AdminTBody } from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { statusTone } from "@/lib/api/status";
import { StatusDot, Pagination } from "@/components/ui/primitives";
import { DbStatCards, resourcesLabel, type DbStat } from "./databases-overview";
import { isAvailable, isPartial, reportedTotal } from "@/lib/admin/telemetry";
import { formatBytes, formatDate } from "@/lib/utils";

const selectStyle = "ui-input w-full";

const engineVersions: Record<string, string[]> = {
  postgresql: ["13", "14", "15", "16"],
  mysql: ["8.0", "8.1", "8.2", "8.3"],
  mariadb: ["10", "11"],
  redis: ["6", "7"],
  mongodb: ["6", "7"],
};

/** Shared formatter — this file had its own `en-GB` date layout. */
function fmtDate(iso?: string): string {
  return formatDate(iso, "—");
}

/**
 * Status as text + dot, coloured by the database status vocabulary — the same
 * table the tiles count with, so a row and a tile cannot disagree.
 */
function ManagedStatusDot({ status }: { status: string }) {
  if (!status) return <span className="border-b border-dashed border-unknown-line text-unknown" title="The database reported no status">not reported</span>;
  return <StatusDot status={status} tone={statusTone(status, "database")} />;
}

/** `host:port` as reported; a database that has no host yet has no endpoint. */
function endpointLabel(db: ManagedDatabase): string {
  if (!db.host) return "Endpoint not reported";
  return db.port ? `${db.host}:${db.port}` : db.host;
}

/**
 * Backup/restore outcome as a tone. `completed`, `running` and `failed` are the
 * words the API uses; anything it cannot interpret resolves to `unknown` rather
 * than a plausible amber.
 */
function operationTone(status: string) {
  return statusTone(status, "deployment");
}

/**
 * Bytes as reported. A backup still in flight has no size, and `-`/`0 MB` would
 * both read as a measured empty backup.
 */
function backupSizeLabel(bytes: number, done: boolean): string {
  if (bytes > 0) return formatBytes(bytes);
  return done ? "0 B" : "Not reported";
}

const filterSelectCls = "ui-input w-full cursor-pointer sm:w-44";

export function ManagedDatabaseView() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
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
  /** Names the filtered denominator rather than presenting a subset as the total. */
  const managedRangeLabel = sorted.length === 0
    ? "Nothing to show"
    : `Showing ${(currentPage - 1) * pageSize + 1}–${Math.min(currentPage * pageSize, sorted.length)} of ${hasActiveFilters ? `${sorted.length} matched of ${dbs.length} databases (filtered)` : `${sorted.length} databases`}`;

  const statsLoading = !isAvailable(dbsQuery);

  /**
   * Tiles for the Managed tab.
   *
   * `memoryTotal` was `dbs.reduce((acc, db) => acc + (db.memoryMb ?? 0), 0)`, so a
   * database that never reported a memory figure still contributed a confident 0
   * and the tile read as a measured total. It now sums only what was reported and
   * marks the figure partial. Every tile renders `—` until the query settles, and
   * the buckets come from the same database status vocabulary the row pill uses,
   * so a tile and a row cannot disagree about what "ready" means.
   */
  const dbStats: DbStat[] = useMemo(() => {
    const ready = dbs.filter((db) => statusTone(db.status, "database") === "ok").length;
    const failed = dbs.filter((db) => statusTone(db.status, "database") === "danger").length;
    const memory = reportedTotal(dbs, (db) => db.memoryMb);
    const unknown = <span className="text-text-muted" title="The managed databases query has not reported">—</span>;
    return [
      { key: "total", label: "Total Databases", icon: Database, hint: "Databases managed by this panel", value: !statsLoading ? dbs.length : unknown },
      { key: "ready", label: "Ready", icon: Box, hint: "Reported as running by the node", value: !statsLoading ? ready : unknown },
      { key: "failed", label: "Failed / Error", icon: Archive, hint: "Reported as failed or in error; the row says what was attempted", value: !statsLoading ? failed : unknown },
      {
        key: "memory",
        label: "Memory Total",
        icon: Server,
        hint: "Sum of what the databases reported",
        value: statsLoading
          ? unknown
          : (memory.value === undefined || memory.total === 0
            ? <span className="text-text-muted" title="No managed database reported a memory figure">—</span>
            : (
              <span title={isPartial(memory) ? `Only ${memory.reported} of ${memory.total} reported memory` : undefined}>
                {`${memory.value} MB`}
                {isPartial(memory) ? <span className="ml-1 text-xs font-normal text-warn">partial</span> : null}
              </span>
            )),
      },
    ];
  }, [dbs, statsLoading]);

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
    onSuccess: () => { invalidate(); toast({ tone: "success", title: "Restore started", message: "The database is replaced with the backup and restarted." }); },
    onError: (e: Error) => toast({ tone: "error", title: "Restore failed", message: e.message }),
  });

  /**
   * A restore overwrites live data, so it is acknowledged first. It used to fire
   * straight from an unlabeled download icon in the expanded row.
   */
  async function requestRestore(dbId: string, backupId: string) {
    const db = dbs.find((d) => d.id === dbId);
    const backup = backups.find((b) => b.id === backupId);
    const confirmed = await confirm({
      title: `Restore ${backup?.name || backupId.slice(0, 8)}?`,
      description: `Everything currently in ${db?.name || dbId.slice(0, 8)} is replaced by this backup, and the database is restarted while the restore runs. This cannot be undone.`,
      danger: true,
      confirmLabel: "Restore backup",
    });
    if (confirmed) restoreMut.mutate({ dbId, backupId });
  }

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
    <div className="space-y-4">
      {/* `<h2>` sub-heading, not a second `<h1>`. The Databases route owns the page
          title (derived from the registry); this tab used to render `SectionHeader`,
          which put a second page heading on the same route. */}
      <div className="flex flex-wrap items-start justify-between gap-3 border-b border-line pb-3">
        <div className="min-w-0">
          <h2 className="t-title">Managed Databases</h2>
          <p className="mt-0.5 max-w-prose text-meta text-text-subtle">
            Databases this panel provisions and operates: created on a node you choose, with backup
            and restore managed here. This is the create form — the “Add database…” button on the
            page header only routes you to a surface like this one.
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Btn ariaLabel="Refresh managed databases" disabled={dbsQuery.isFetching} onClick={() => void dbsQuery.refetch()} tone="ghost">
            <RefreshCw size={14} /> Refresh
          </Btn>
          <Btn onClick={() => setShowCreate(true)}><Plus size={14} /> Create Database</Btn>
        </div>
      </div>

      {confirmDeleteId ? (() => {
        const target = dbs.find((db) => db.id === confirmDeleteId);
        return (
          <Modal
            description={undefined}
            onClose={() => { setConfirmDeleteId(null); setForceDelete(false); }}
            title={target ? `Delete ${target.name || target.id.slice(0, 8)}?` : "Delete this database?"}
          >
            <div className="space-y-3">
              <p className="text-sm text-text">
                The database and all data stored in it are permanently removed.
                {target?.host ? <span className="block mt-1 text-text-subtle">Endpoint being torn down: <code className="ui-code-inline">{target.host}{target.port ? `:${target.port}` : ""}</code>.</span> : null}
                This cannot be undone.
              </p>
              {/* The force option lives *inside* the dialog the operator acknowledges.
                  It used to render below the dialog as a loose checkbox, so the
                  escalating variant of the deletion could be confirmed without the
                  operator ever having read it — and its label was a query string. */}
              <label className="flex items-start gap-2 rounded-lg border border-line bg-overlay-subtle p-3 text-xs text-text-subtle">
                <input
                  checked={forceDelete}
                  className="mt-0.5"
                  id="forceDeleteChk"
                  onChange={(e) => setForceDelete(e.target.checked)}
                  type="checkbox"
                />
                <span>
                  <span className="block font-semibold text-text">Force the deletion</span>
                  Use this only if the database has already been removed on the node and the panel
                  record needs clearing. Forced deletion skips the step that shuts the container down
                  remotely, so the data may still exist on the machine afterwards.
                </span>
              </label>
              {deleteMut.isError ? (
                <p className="ui-alert ui-alert-danger" role="alert">
                  Deletion failed: {deleteMut.error.message}
                </p>
              ) : null}
            </div>
            <ModalFooter
              confirmLabel={forceDelete ? "Force delete" : "Delete database"}
              destructive
              disabled={deleteMut.isPending}
              onCancel={() => { setConfirmDeleteId(null); setForceDelete(false); }}
              onConfirm={() => { deleteMut.mutate({ id: confirmDeleteId, force: forceDelete }); setConfirmDeleteId(null); setForceDelete(false); }}
            />
          </Modal>
        );
      })() : null}

      <div className="space-y-4">
        <DbStatCards
          stats={dbStats}
        />

        <div className="flex flex-col gap-2 xl:flex-row xl:items-center">
          <label className="flex min-w-52 flex-1 items-center gap-2 rounded-lg border border-line bg-overlay-subtle px-2.5 py-2 focus-within:border-line-strong">
            <Search aria-hidden="true" size={13} className="shrink-0 text-text-muted" />
            <input
              aria-label="Search managed databases"
              className="w-full bg-transparent text-xs text-text outline-none placeholder:text-text-muted"
              onChange={(e) => resetPage(() => setSearch(e.target.value))}
              placeholder="Search by name, engine, status…"
              type="search"
              value={search}
            />
          </label>
          <div className="flex flex-wrap items-center gap-2">
            <select aria-label="Filter managed databases by engine" className={filterSelectCls} onChange={(e) => resetPage(() => setEngineFilter(e.target.value))} value={engineFilter}>
              <option value="all">Engine: all</option>
              {engines.map((e) => <option key={e} value={e}>{e}</option>)}
            </select>
            <select aria-label="Filter managed databases by status" className={filterSelectCls} onChange={(e) => resetPage(() => setStatusFilter(e.target.value))} value={statusFilter}>
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
            Managed databases <span className="font-normal text-text-subtle">({hasActiveFilters ? `${sorted.length} of ${dbs.length} (filtered)` : `${dbs.length} database${dbs.length === 1 ? "" : "s"}`})</span>
          </h3>
          <label className="flex items-center gap-2 rounded-lg border border-line bg-overlay-subtle px-2.5 py-1.5 text-xs text-text-subtle">
            <span className="text-meta text-text-muted">Sort by</span>
            <select aria-label="Sort managed databases" value={sort} onChange={(e) => setSort(e.target.value)} className="cursor-pointer appearance-none bg-transparent pr-1 outline-none">
              <option value="name-asc">Name (A → Z)</option>
              <option value="name-desc">Name (Z → A)</option>
              <option value="status">Status</option>
              <option value="engine">Engine</option>
            </select>
          </label>
        </div>

        {dbsQuery.isPending ? (
          <AdminLoadingState label="Loading managed databases…" />
        ) : dbsQuery.isError ? (
          <AdminErrorState
            message={`Could not load managed databases: ${dbsQuery.error.message}`}
            retry={() => void dbsQuery.refetch()}
          />
        ) : dbs.length === 0 ? (
          <EmptyState
            icon={Database}
            message="No managed databases have been created in this panel yet."
            title="No managed databases"
          />
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
                  onRestore={(id, backupId) => void requestRestore(id, backupId)}
                  onRotate={(id) => rotateMut.mutate(id)}
                  onEdit={(item) => setEditingDb(item)}
                  onDelete={(id) => setConfirmDeleteId(id)}
                  backups={selected === db.id ? backups : []}
                  restores={selected === db.id ? restores : []}
                  isPending={backupMut.isPending || restoreMut.isPending || updateMut.isPending}
                />
              ))}
            </div>
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-overlay-subtle px-4 py-3 text-xs text-text-subtle">
              <span>{managedRangeLabel}</span>
              <div className="flex items-center gap-2">
                <Pagination label="Managed database pagination" onPageChange={setPage} page={currentPage} pageCount={totalPages} />
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
            <AdminTable label="Managed databases">
              <AdminTHead>
                <AdminTh>Name</AdminTh>
                <AdminTh>Engine / Version</AdminTh>
                <AdminTh>Status</AdminTh>
                <AdminTh>Host : Port</AdminTh>
                <AdminTh>Resources</AdminTh>
                <AdminTh className="text-right">Actions</AdminTh>
              </AdminTHead>
              <AdminTBody>
                {visible.map((db) => (
                  <ManagedDBRow
                    key={db.id}
                    db={db}
                    isSelected={selected === db.id}
                    onSelect={() => setSelected(selected === db.id ? null : db.id)}
                    onBackup={(id) => backupMut.mutate(id)}
                    onRestore={(id, backupId) => void requestRestore(id, backupId)}
                    onRotate={(id) => rotateMut.mutate(id)}
                    onEdit={(item) => setEditingDb(item)}
                    onDelete={(id) => setConfirmDeleteId(id)}
                    backups={selected === db.id ? backups : []}
                    restores={selected === db.id ? restores : []}
                    isPending={backupMut.isPending || restoreMut.isPending || updateMut.isPending}
                  />
                ))}
              </AdminTBody>
            </AdminTable>
            <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line px-4 py-3 text-xs text-text-subtle">
              <span>{managedRangeLabel}</span>
              <div className="flex items-center gap-2">
                <Pagination label="Managed database pagination" onPageChange={setPage} page={currentPage} pageCount={totalPages} />
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
      </div>

      {renderConfirm()}
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
  /** Backup and password rotation act on the live container, so they need one. */
  const offlineReason = db.status === "running" ? null : `Cannot run this against a database that is ${db.status || "not reporting a status"}`;

  return (
    <>
      <tr
        aria-expanded={isSelected}
        className="cursor-pointer transition hover:bg-overlay-subtle focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[var(--focus)]"
        onClick={onSelect}
        onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onSelect(); } }}
        tabIndex={0}
      >
        <td className="px-4 py-3">
          <div className="flex items-center gap-2.5">
            <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg border border-line bg-overlay-subtle text-text-subtle">
              <Database aria-hidden="true" size={14} />
            </span>
            <span className="min-w-0">
              {/* Whole name on screen; the tooltip is not its only copy. */}
              <span className="block break-words text-xs font-bold text-text">{db.name || db.id.slice(0, 8)}</span>
              <span className="block font-mono text-meta text-text-muted">{fmtDate(db.createdAt)}</span>
            </span>
          </div>
        </td>
        <td className="px-2 py-3">
          <span className="block text-xs text-text">{db.engine}</span>
          <span className="block font-mono text-meta text-text-muted">{db.version || "—"}</span>
        </td>
        <td className="px-2 py-3">
          <ManagedStatusDot status={db.status} />
        </td>
        <td className="px-2 py-3 font-mono text-meta text-text-subtle">
          {endpointLabel(db)}
        </td>
        <td className="px-2 py-3 text-meta text-text-subtle">
          {resourcesLabel(db.memoryMb || undefined, db.cpuShares || undefined)}
        </td>
        <td className="px-2 py-3" onClick={(e) => e.stopPropagation()}>
           <div className="flex items-center justify-end gap-1">
            <button
              aria-label={`Edit ${db.name || db.id.slice(0, 8)}`}
              className="grid h-11 w-11 place-items-center rounded text-text-subtle transition-colors hover:bg-overlay-subtle hover:text-[var(--brand)] disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
              disabled={isPending}
              onClick={() => onEdit(db)}
              title={isPending ? "Another change is in flight" : `Edit ${db.name || db.id.slice(0, 8)} — name, version, memory and CPU weight`}
              type="button"
            >
              <svg aria-hidden="true" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
            </button>
            <button
              aria-label={`Back up ${db.name || db.id.slice(0, 8)}`}
              className="grid h-11 w-11 place-items-center rounded text-text-subtle transition-colors hover:bg-overlay-subtle hover:text-text disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
              disabled={isPending || offlineReason !== null}
              onClick={() => onBackup(db.id)}
              title={offlineReason ?? (isPending ? "Another change is in flight" : "Start a backup")}
              type="button"
            >
              <Archive aria-hidden="true" size={14} />
            </button>
            <button
              aria-label={`Rotate the password of ${db.name || db.id.slice(0, 8)}`}
              className="grid h-11 w-11 place-items-center rounded text-text-subtle transition-colors hover:bg-overlay-subtle hover:text-[var(--brand)] disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
              disabled={isPending || offlineReason !== null}
              onClick={() => onRotate(db.id)}
              title={offlineReason ?? (isPending ? "Another change is in flight" : "Rotate the password — apps holding the old one will need it")}
              type="button"
            >
              <RefreshCw aria-hidden="true" size={14} />
            </button>
            <button
              aria-label={`Delete ${db.name || db.id.slice(0, 8)}`}
              className="grid h-11 w-11 place-items-center rounded text-text-subtle transition-colors hover:bg-overlay-subtle hover:text-danger disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
              disabled={isPending}
              onClick={() => onDelete(db.id)}
              title={isPending ? "Another change is in flight" : `Delete ${db.name || db.id.slice(0, 8)}`}
              type="button"
            >
              <Trash2 aria-hidden="true" size={14} />
            </button>
          </div>
        </td>
      </tr>
      {isSelected && (backups.length > 0 || restores.length > 0) && (
        <tr>
          <td colSpan={6} className="px-4 pb-3">
            <div className="space-y-3 rounded-lg bg-overlay-subtle p-3">
              {backups.length > 0 && (
                <div>
                  <h4 className="mb-2 text-xs font-semibold uppercase tracking-wider text-text-subtle">Backups</h4>
                  <div className="space-y-1">
                    {backups.map((b) => (
                      <div key={b.id} className="flex flex-wrap items-center justify-between gap-2 rounded bg-overlay-subtle px-3 py-2 text-xs">
                        <div className="flex min-w-0 items-center gap-2">
                          <Pill tone={operationTone(b.status)}>{b.status || "not reported"}</Pill>
                          <span className="break-words text-text">{b.name || b.id}</span>
                        </div>
                        <div className="flex items-center gap-2">
                          <span className="text-text-subtle">{backupSizeLabel(b.size, b.status === "completed")}</span>
                          <span className="text-text-muted">{fmtDate(b.createdAt)}</span>
                          {b.status === "completed" ? (
                            <button
                              aria-label={`Restore ${b.name || b.id} into ${db.name || db.id.slice(0, 8)}`}
                              className="grid h-9 w-9 place-items-center rounded text-text-subtle transition-colors hover:bg-overlay hover:text-[var(--brand)] disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
                              disabled={isPending}
                              onClick={() => onRestore(db.id, b.id)}
                              title={isPending ? "Another change is in flight" : `Restore ${b.name || b.id} — replaces the current data`}
                              type="button"
                            >
                              <Download aria-hidden="true" size={14} />
                            </button>
                          ) : (
                            <span className="text-meta text-text-muted" title={`Only a completed backup can be restored; this one is ${b.status || "not reporting a status"}`}>Not restorable</span>
                          )}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}
              {restores.length > 0 && (
                <div>
                  <h4 className="mb-2 text-xs font-semibold uppercase tracking-wider text-text-subtle">Restores</h4>
                  <div className="space-y-1">
                    {restores.map((r) => (
                      <div key={r.id} className="flex flex-wrap items-center justify-between gap-2 rounded bg-overlay-subtle px-3 py-2 text-xs">
                        <div className="flex min-w-0 items-center gap-2">
                          <Pill tone={operationTone(r.status)}>{r.status || "not reported"}</Pill>
                          <span className="font-mono text-text-subtle">{r.id.slice(0, 8)}</span>
                          {r.backupId && <span className="font-mono text-text-muted">backup {r.backupId.slice(0, 8)}</span>}
                        </div>
                        <div className="flex min-w-0 flex-col items-end gap-1">
                          <span className="text-text-muted">{fmtDate(r.createdAt)}</span>
                          {/* The failure text is the only explanation of a failed restore,
                              so it is rendered in full rather than clipped into a tooltip. */}
                          {r.errorMessage ? <span className="max-w-prose break-words text-danger">{r.errorMessage}</span> : null}
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
  const hostPort = endpointLabel(db);
  return (
    <div className="rounded-xl border border-line bg-overlay-subtle p-4 shadow-sm transition hover:border-line-strong">
      <div className="flex cursor-pointer items-start gap-3" onClick={onSelect}>
        <span className="grid h-10 w-10 shrink-0 place-items-center rounded-xl border border-line bg-overlay-subtle text-text-subtle">
          <Database size={18} />
        </span>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-bold text-text" title={db.name}>{db.name}</p>
          <p className="font-mono text-meta text-text-muted">{fmtDate(db.createdAt)}</p>
        </div>
        <ManagedStatusDot status={db.status} />
      </div>
      <div className="mt-3 space-y-1 border-t border-line pt-3 font-mono text-meta text-text-subtle">
        <p className="truncate">{db.engine}{db.version ? ` ${db.version}` : ""} · {hostPort}</p>
        <p className="truncate">{resourcesLabel(db.memoryMb, db.cpuShares)}</p>
      </div>
      <div className="mt-3 flex items-center gap-1" onClick={(e) => e.stopPropagation()}>
        <button
          className="grid h-9 flex-1 place-items-center rounded-lg border border-line text-text-subtle transition-colors hover:border-line-strong hover:text-[var(--brand)] disabled:opacity-40"
          disabled={isPending}
          onClick={() => onEdit(db)}
          title="Edit — PATCH /managed-databases/:id"
          type="button"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
        </button>
        <button
          className="grid h-9 flex-1 place-items-center rounded-lg border border-line text-text-subtle transition-colors hover:border-line-strong hover:text-text disabled:opacity-40"
          disabled={isPending}
          onClick={() => onBackup(db.id)}
          title="Backup"
          type="button"
        >
          <Archive size={14} />
        </button>
        <button
          className="grid h-9 flex-1 place-items-center rounded-lg border border-line text-text-subtle transition-colors hover:border-line-strong hover:text-info disabled:opacity-40"
          disabled={isPending}
          onClick={() => onRotate(db.id)}
          title="Rotate Password"
          type="button"
        >
          <RefreshCw size={14} />
        </button>
        <button
          className="grid h-9 flex-1 place-items-center rounded-lg border border-line text-text-subtle transition-colors hover:border-line-strong hover:text-danger disabled:opacity-40"
          disabled={isPending}
          onClick={() => onDelete(db.id)}
          title="Delete — supports ?force"
          type="button"
        >
          <Trash2 size={14} />
        </button>
      </div>
      {isSelected && (backups.length > 0 || restores.length > 0) && (
        <div className="mt-3 rounded-lg bg-overlay-subtle p-3 space-y-3">
          {backups.length > 0 && (
            <div>
              <div className="mb-2 text-xs font-medium uppercase tracking-wider text-text-subtle">Backups</div>
              <div className="space-y-1">
                {backups.map((b) => (
                  <div key={b.id} className="flex items-center justify-between rounded bg-overlay-subtle px-3 py-2 text-xs">
                    <div className="flex items-center gap-2">
                      <Pill tone={b.status === "completed" ? "green" : b.status === "failed" ? "red" : "yellow"}>{b.status}</Pill>
                      <span className="text-text-subtle">{b.name}</span>
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="text-text-subtle">{backupSizeLabel(b.size, b.status === "completed")}</span>
                      {b.status === "completed" && (
                        <button
                          className="text-text-subtle transition-colors hover:text-info"
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
              <div className="mb-2 text-xs font-medium uppercase tracking-wider text-text-subtle">Restores</div>
              <div className="space-y-1">
                {restores.map((r) => (
                  <div key={r.id} className="flex items-center justify-between rounded bg-overlay-subtle px-3 py-2 text-xs">
                    <div className="flex items-center gap-2">
                      <Pill tone={r.status === "completed" ? "green" : r.status === "failed" ? "red" : "yellow"}>{r.status}</Pill>
                      <span className="text-text-subtle">{r.id.slice(0,8)}</span>
                      {r.backupId && <span className="text-text-subtle">backup:{r.backupId.slice(0,8)}</span>}
                    </div>
                    <div className="flex items-center gap-2">
                      {r.errorMessage && <span className="text-danger truncate max-w-[200px]">{r.errorMessage}</span>}
                      <span className="text-text-muted">{new Date(r.createdAt).toLocaleDateString()}</span>
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
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">Engine</label>
            <select className={selectStyle} value={engine} onChange={(e) => { setEngine(e.target.value as ManagedDatabaseEngine); setVersion(engineVersions[e.target.value]?.[engineVersions[e.target.value].length - 1] ?? "latest"); }}>
              <option value="postgresql">PostgreSQL</option>
              <option value="mysql">MySQL</option>
              <option value="mariadb">MariaDB</option>
              <option value="redis">Redis</option>
              <option value="mongodb">MongoDB</option>
            </select>
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">Version</label>
            <select className={selectStyle} value={version} onChange={(e) => setVersion(e.target.value)}>
              {(engineVersions[engine] ?? []).map((v) => (
                <option key={v} value={v}>{v}</option>
              ))}
            </select>
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">Memory (MB)</label>
            <input
              type="number"
              className={selectStyle}
              value={memoryMb}
              onChange={(e) => setMemoryMb(Number(e.target.value))}
              min={64}
              step={64}
            />
            <p className="mt-1 text-xs text-text-subtle">Min 64 MB. Default 256 MB.</p>
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">CPU Shares</label>
            <input
              type="number"
              className={selectStyle}
              value={cpuShares}
              onChange={(e) => setCpuShares(Number(e.target.value))}
              min={0}
              max={1024}
            />
            <p className="mt-1 text-xs text-text-subtle">Relative CPU weight. 0 = default (1024).</p>
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
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">Version</label>
            <select className={selectStyle} value={version} onChange={(e) => setVersion(e.target.value)}>
              {(engineVersions[db.engine] ?? [db.version]).map((v) => (
                <option key={v} value={v}>{v}</option>
              ))}
              {!engineVersions[db.engine]?.includes(version) && <option value={version}>{version} (current)</option>}
            </select>
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">Memory (MB)</label>
            <input type="number" className={selectStyle} value={memoryMb} onChange={(e) => setMemoryMb(Number(e.target.value))} min={64} step={64} />
          </div>
        </div>
        <div>
          <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">CPU Shares</label>
          <input type="number" className={selectStyle} value={cpuShares} onChange={(e) => setCpuShares(Number(e.target.value))} min={0} max={1024} />
        </div>
        <p className="text-xs text-text-subtle">Wires <code className="font-mono">updateManagedDatabase</code> — PATCH /managed-databases/:id with name/version/memoryMb/cpuShares</p>
      </div>
      <ModalFooter onCancel={onClose} onConfirm={() => onSave({ name, version, memoryMb, cpuShares })} disabled={saving} confirmLabel={saving ? "Saving..." : "Save"} />
    </Modal>
  );
}
