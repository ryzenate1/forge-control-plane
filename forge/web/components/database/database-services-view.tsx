"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Database, Plus, Trash2, RotateCcw, Layers, Server, Search, List, LayoutGrid, Rows3, FileText, History, KeyRound, RefreshCw, Power, LoaderCircle } from "lucide-react";
import {
  listDatabaseServices,
  provisionDatabaseService,
  deleteDatabaseService,
  restartDatabaseService,
  testConnection,
  listServiceTemplates,
  createServiceTemplate,
  getServiceLogs,
  listServiceBackups,
  restoreServiceBackup,
  createServiceBackup,
  createServiceCredential,
  listServiceCredentials,
  revokeServiceCredential,
  type DatabaseService,
  type DatabaseServiceBackup,
  type DatabaseServiceCredential,
} from "@/lib/api/database-services";
import { fetchDBEngines } from "@/lib/api/database-containers";
import {
  AdminErrorState,
  AdminLoadingRows,
  AdminLoadingState,
  Btn,
  EmptyState,
  Input,
  Modal,
  ModalFooter,
  Pill,
  cn,
} from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { statusTone } from "@/lib/api/status";
import { Pagination, StatusDot } from "@/components/ui/primitives";
import { isAvailable, isPartial, reportedTotal } from "@/lib/admin/telemetry";
import { formatBytes, formatDate } from "@/lib/utils";
import { DbStatCards, resourcesLabel, type DbStat } from "./databases-overview";

const selectCls = "ui-input w-full";
const labelCls = "ui-label mb-1.5 block";
const filterSelectWrap = "flex flex-col justify-center rounded-lg border border-line bg-overlay-subtle px-3 py-1";
const filterSelectCls = "h-6 cursor-pointer appearance-none border-0 bg-transparent pl-0 pr-8 text-xs text-text outline-none";

/** `host:port` as reported — a stopped service reports an empty host and port 0. */
function endpointLabel(svc: DatabaseService): string {
  if (!svc.host) return "Endpoint not reported";
  return svc.port ? `${svc.host}:${svc.port}` : svc.host;
}

/**
 * A 0 read from Go means "never configured", not "configured as zero", so it is
 * passed on as absent and {@link resourcesLabel} renders it as not reported.
 */
function serviceResources(svc: DatabaseService): string {
  return resourcesLabel(svc.memoryMb || undefined, svc.cpuShares || undefined);
}

export function DatabaseServicesView() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [showProvision, setShowProvision] = useState(false);
  const [showTemplate, setShowTemplate] = useState(false);
  const [showTest, setShowTest] = useState(false);
  const [templateFilter, setTemplateFilter] = useState("");
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [view, setView] = useState<"table" | "cards">("table");
  const [sort, setSort] = useState("name-asc");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [detailSvc, setDetailSvc] = useState<DatabaseService | null>(null);

  const servicesQ = useQuery({ queryKey: ["database-services"], queryFn: listDatabaseServices });
  const templatesQ = useQuery({ queryKey: ["service-templates"], queryFn: listServiceTemplates });

  const services = useMemo(() => servicesQ.data ?? [], [servicesQ.data]);
  const templates = templatesQ.data ?? [];
  const filteredTemplates = templateFilter ? templates.filter((t) => t.type.toLowerCase().includes(templateFilter.toLowerCase()) || t.version.includes(templateFilter)) : templates;

  const statuses = useMemo(() => [...new Set(services.map((s) => s.status).filter(Boolean))].sort(), [services]);
  /** Running as the status vocabulary reads it — the same rule as the row pill. */
  const runningCount = useMemo(() => services.filter((s) => statusTone(s.status, "database") === "ok").length, [services]);
  const servicesReady = isAvailable(servicesQ);
  const templatesReady = isAvailable(templatesQ);

  function resetPage(update: () => void) {
    setPage(1);
    update();
  }

  const filteredServices = useMemo(() => {
    const term = search.trim().toLowerCase();
    return services.filter((s) => {
      if (statusFilter !== "all" && s.status !== statusFilter) return false;
      if (!term) return true;
      return `${s.name} ${s.id} ${s.type} ${s.version} ${s.status}`.toLowerCase().includes(term);
    });
  }, [services, search, statusFilter]);

  const sortedServices = useMemo(() => {
    const list = [...filteredServices];
    switch (sort) {
      case "name-desc": return list.sort((a, b) => (b.name || b.id).localeCompare(a.name || a.id));
      case "status": return list.sort((a, b) => a.status.localeCompare(b.status) || (a.name || a.id).localeCompare(b.name || b.id));
      case "memory": return list.sort((a, b) => (b.memoryMb ?? 0) - (a.memoryMb ?? 0));
      default: return list.sort((a, b) => (a.name || a.id).localeCompare(b.name || b.id));
    }
  }, [filteredServices, sort]);

  const totalPages = Math.max(1, Math.ceil(sortedServices.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const visibleServices = sortedServices.slice((currentPage - 1) * pageSize, currentPage * pageSize);
  const hasActiveServiceFilters = Boolean(search.trim() || statusFilter !== "all");
  /** Names the filtered denominator instead of presenting a subset as the total. */
  const servicesRangeLabel = sortedServices.length === 0
    ? "Nothing to show"
    : `Showing ${(currentPage - 1) * pageSize + 1}–${Math.min(currentPage * pageSize, sortedServices.length)} of ${hasActiveServiceFilters ? `${sortedServices.length} matched of ${services.length} services (filtered)` : `${sortedServices.length} services`}`;

  /**
   * Tiles for the Services tab.
   *
   * `DbStat` has no `tile` field any more — `DbStatCards` renders one neutral token
   * treatment for every tile, so the per-tile palette classes that were passing
   * `tile:` here are gone and each tile carries a `hint` explaining the figure
   * instead. Values are honest about their source: every tile renders `—` until the
   * query it reads has reported, so an unreachable API can never show a confident
   * zero, and memory sums only what services actually reported rather than
   * counting an unmeasured service as 0 MB.
   */
  const stats: DbStat[] = useMemo(() => {
    const memory = reportedTotal(services, (s) => s.memoryMb);
    const servicesUnknown = <span className="text-text-muted" title="The database services query has not reported">—</span>;
    const templatesUnknown = <span className="text-text-muted" title="The service templates query has not reported">—</span>;
    return [
      { key: "total", label: "Services", icon: Database, hint: "Database services known to this panel", value: servicesReady ? services.length : servicesUnknown },
      { key: "running", label: "Running", icon: Server, hint: "Services the node reported as running", value: servicesReady ? runningCount : servicesUnknown },
      { key: "templates", label: "Templates", icon: Layers, hint: "Templates a service can be provisioned from", value: templatesReady ? templates.length : templatesUnknown },
      {
        key: "memory",
        label: "Memory Total",
        icon: Database,
        hint: "Sum of what the services reported",
        value: servicesReady
          ? (memory.value === undefined || memory.total === 0
            ? <span className="text-text-muted" title="No database service reported a memory figure">—</span>
            : (
              <span title={isPartial(memory) ? `Only ${memory.reported} of ${memory.total} services reported memory` : undefined}>
                {`${memory.value} MB`}
                {isPartial(memory) ? <span className="ml-1 text-xs font-normal text-warn">partial</span> : null}
              </span>
            ))
          : servicesUnknown,
      },
    ];
  }, [servicesReady, templatesReady, services, templates, runningCount]);

  const restartMut = useMutation({
    mutationFn: async (id: string) => {
      const result = await restartDatabaseService(id);
      if (!result.ok) throw new Error("The server reported the service restart did not complete.");
      return result;
    },
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["database-services"] }); toast({ tone: "success", title: "Service restart initiated", message: "The container is stopped and started again; connections drop while it does." }); },
    onError: (e: Error) => toast({ tone: "error", title: "Restart failed", message: e.message }),
  });
  const deleteMut = useMutation({
    mutationFn: async (id: string) => {
      const result = await deleteDatabaseService(id);
      if (!result.ok) throw new Error("The server reported the database service was not deleted.");
      return result;
    },
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["database-services"] }); toast({ tone: "success", title: "Service deleted" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Delete failed", message: e.message }),
  });

  /**
   * Deletion is destructive and irreversible, so it is acknowledged first and the
   * dialog says what is torn down. It used to fire straight from the icon button.
   */
  async function requestDelete(svc: DatabaseService) {
    const name = svc.name || svc.id.slice(0, 8);
    const confirmed = await confirm({
      title: `Delete ${name}?`,
      description: `The ${svc.type} ${svc.version} service is stopped and its container, data volume and issued credentials are removed${svc.serverId ? ", and it is unlinked from the server it is attached to" : ""}. This cannot be undone.`,
      danger: true,
      confirmLabel: "Delete service",
    });
    if (confirmed) deleteMut.mutate(svc.id);
  }

  /** Restart cannot run on a service that is mid-provision or being deleted. */
  function restartBlockReason(svc: DatabaseService): string | null {
    if (svc.status === "provisioning") return "Cannot restart while the service is still provisioning";
    if (svc.status === "deleting") return "Cannot restart — this service is being deleted";
    return null;
  }

  /** Which operation this row has in flight, so only that row shows it. */
  function rowPendingAction(id: string): "restart" | "delete" | null {
    if (restartMut.isPending && restartMut.variables === id) return "restart";
    if (deleteMut.isPending && deleteMut.variables === id) return "delete";
    return null;
  }

  return (
    <div className="space-y-4">
      {/* `<h2>` sub-heading, not a second `<h1>`. The Databases route owns the page
          title. Also one primary action rather than three co-equal buttons: Provision
          Service is the purpose of this tab, so "New template" moves to the template
          card and "Test connection" to the filters row, and `FlaskConical` is no
          longer used as an action glyph (the registry reserves it elsewhere). */}
      <div className="flex flex-wrap items-start justify-between gap-3 border-b border-line pb-3">
        <div className="min-w-0">
          <h2 className="t-title">Database Services</h2>
          <p className="mt-0.5 max-w-prose text-meta text-text-subtle">
            Service instances provisioned from a template — PostgreSQL, MySQL, Redis and the rest.
            Each instance gets its own credentials, backups and logs here.
          </p>
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-2">
          <Btn ariaLabel="Refresh database services" disabled={servicesQ.isFetching} onClick={() => void servicesQ.refetch()} tone="ghost">
            <RefreshCw size={14} /> Refresh
          </Btn>
          <Btn onClick={() => setShowProvision(true)}><Plus size={14} /> Provision Service</Btn>
        </div>
      </div>

      <DbStatCards stats={stats} />

      <div className="overflow-hidden rounded-xl border border-line bg-overlay-subtle">
        <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
          <h3 className="t-title flex items-center gap-2">
            <Layers aria-hidden="true" size={15} className="text-text-subtle" /> Service Templates{" "}
            <span className="font-normal text-text-subtle">
              {templatesQ.data
                ? `${filteredTemplates.length}${filteredTemplates.length !== templatesQ.data.length ? ` of ${templatesQ.data.length} (filtered)` : ""}`
                : "—"}
            </span>
          </h3>
          <div className="flex items-center gap-2">
            <div className="w-full max-w-64"><Input label="Filter templates" onChange={setTemplateFilter} placeholder="Type or version…" value={templateFilter} /></div>
            <Btn ariaLabel="Create a service template" onClick={() => setShowTemplate(true)} tone="ghost">
              <Plus size={14} /> New template
            </Btn>
          </div>
        </div>
        {templatesQ.isPending ? (
          <AdminLoadingState label="Loading service templates…" />
        ) : templatesQ.isError ? (
          <div className="p-4">
            <AdminErrorState
              message={`Could not load service templates: ${(templatesQ.error as Error).message}`}
              retry={() => void templatesQ.refetch()}
            />
          </div>
        ) : filteredTemplates.length === 0 ? (
          <EmptyState
            icon={Layers}
            message={templateFilter.trim()
              ? `No templates match “${templateFilter.trim()}”. A template fixes the default image, port and resource floor for a type and version.`
              : "No service templates defined yet. A template is the type and version a service is provisioned from — add one before provisioning."}
            title={templateFilter.trim() ? "No matching templates" : "No service templates"}
          />
        ) : (
          <div className="overflow-x-auto">
            <table aria-label="Database service templates" className="ui-table w-full text-xs">
              <thead><tr className="border-b border-line-strong text-left"><th className="ui-th px-4 py-3 font-medium">Type</th><th className="ui-th px-2 py-3 font-medium">Version</th><th className="ui-th px-2 py-3 font-medium">Image</th><th className="ui-th px-2 py-3 font-medium">Port</th><th className="ui-th px-2 py-3 font-medium">Min memory</th></tr></thead>
              <tbody className="divide-y divide-line">
                {filteredTemplates.map((t) => (
                  <tr className="transition hover:bg-overlay-subtle" key={t.id}>
                    <td className="px-4 py-3"><Pill tone="info">{t.type}</Pill></td>
                    <td className="px-2 py-3 font-mono text-meta text-text">{t.version}</td>
                    <td className="px-2 py-3 font-mono text-meta text-text-subtle">{t.dockerImage || "—"}</td>
                    <td className="px-2 py-3 font-mono text-meta text-text-subtle">{t.defaultPort ?? "—"}</td>
                    <td className="px-2 py-3 text-meta text-text-subtle">{typeof t.minMemoryMb === "number" ? `${t.minMemoryMb} MB` : "Not reported"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <p className="border-t border-line px-4 py-3 text-meta text-text-muted">
          A template is a default, not a running service. Editing one never changes services already
          provisioned from it.
        </p>
      </div>

      <div className="space-y-4">
      <div className="flex flex-col gap-2 xl:flex-row xl:items-center">
        <label className="flex min-w-52 flex-1 items-center gap-2 rounded-lg border border-line bg-overlay-subtle px-2.5 py-2 focus-within:border-line-strong">
          <Search aria-hidden="true" size={13} className="shrink-0 text-text-muted" />
          <input
            type="search"
            value={search}
            onChange={(e) => resetPage(() => setSearch(e.target.value))}
            placeholder="Search services by name, type, status…"
            aria-label="Search services"
            className="w-full bg-transparent text-xs text-text outline-none placeholder:text-text-muted"
          />
        </label>
        <div className="flex flex-wrap items-center gap-2">
          <label className={filterSelectWrap}>
            <span className="text-meta leading-3 text-text-muted">Status</span>
            <select aria-label="Filter services by status" value={statusFilter} onChange={(e) => resetPage(() => setStatusFilter(e.target.value))} className={filterSelectCls}>
              <option value="all">All</option>
              {statuses.map((s) => <option key={s} value={s}>{s}</option>)}
            </select>
          </label>
          <div className="flex gap-1 rounded-lg border border-line bg-overlay-subtle p-1" role="group" aria-label="View mode">
            <button type="button" aria-pressed={view === "table"} onClick={() => setView("table")} className={cn("flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-semibold transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]", view === "table" ? "border border-brand-line bg-brand-subtle text-text" : "border border-transparent text-text-muted hover:text-text")}>
              <List aria-hidden="true" size={14} /> Table
            </button>
            <button type="button" aria-pressed={view === "cards"} onClick={() => setView("cards")} className={cn("flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-semibold transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]", view === "cards" ? "border border-brand-line bg-brand-subtle text-text" : "border border-transparent text-text-muted hover:text-text")}>
              <LayoutGrid aria-hidden="true" size={14} /> Cards
            </button>
          </div>
          <Btn ariaLabel="Test a database connection" onClick={() => setShowTest(true)} tone="ghost">Test connection</Btn>
          {hasActiveServiceFilters && (
            <button type="button" onClick={() => { setSearch(""); setStatusFilter("all"); setPage(1); }} className="rounded-lg px-2.5 py-2 text-xs font-semibold text-text-subtle transition hover:text-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">
              Clear filters
            </button>
          )}
        </div>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <h3 className="t-title">
          Database services{" "}
          <span className="font-normal text-text-subtle">
            {servicesReady
              ? (hasActiveServiceFilters ? `${sortedServices.length} of ${services.length} (filtered)` : `${services.length} service${services.length === 1 ? "" : "s"}`)
              : "not reported"}
          </span>
        </h3>
        <label className="flex items-center gap-2 rounded-lg border border-line bg-overlay-subtle px-2.5 py-1.5 text-xs text-text">
          <span className="text-text-subtle">Sort by</span>
          <select aria-label="Sort services" value={sort} onChange={(e) => setSort(e.target.value)} className="cursor-pointer appearance-none bg-transparent pr-1 outline-none">
            <option value="name-asc">Name (A → Z)</option>
            <option value="name-desc">Name (Z → A)</option>
            <option value="status">Status</option>
            <option value="memory">Memory</option>
          </select>
        </label>
      </div>

      {servicesQ.isPending ? (
        <AdminLoadingState label="Loading database services…" />
      ) : servicesQ.isError ? (
        <AdminErrorState
          message={`Could not load database services: ${servicesQ.error.message}`}
          retry={() => void servicesQ.refetch()}
        />
      ) : services.length === 0 ? (
        <EmptyState icon={Database} title="No services" message="No database services have been provisioned yet. Provision one to create a linkable database with its own credentials, backups and logs." />
      ) : sortedServices.length === 0 ? (
        <EmptyState icon={Database} title="No matches" message={`No service matches these filters out of ${services.length} read.`} />
      ) : view === "cards" ? (
        <div className="space-y-3">
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {visibleServices.map((svc) => (
              <ServiceCard
                key={svc.id}
                svc={svc}
                onManage={() => setDetailSvc(svc)}
                onRestart={() => void restartMut.mutate(svc.id)}
                onDelete={() => void requestDelete(svc)}
                pendingAction={rowPendingAction(svc.id)}
                restartBlockReason={restartBlockReason(svc)}
              />
            ))}
          </div>
          <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-overlay-subtle px-4 py-3 text-xs text-text-subtle">
            <span>{servicesRangeLabel}</span>
            <div className="flex items-center gap-2">
              <Pagination label="Database services pagination" onPageChange={setPage} page={currentPage} pageCount={totalPages} />
              <select aria-label="Rows per page" value={pageSize} onChange={(e) => { setPageSize(Number(e.target.value)); setPage(1); }} className="cursor-pointer appearance-none rounded border border-line bg-overlay px-2 py-1 font-mono outline-none">
                <option value={10}>10 / page</option>
                <option value={20}>20 / page</option>
                <option value={50}>50 / page</option>
              </select>
            </div>
          </div>
        </div>
      ) : (
        <div className="overflow-hidden rounded-xl border border-line bg-overlay-subtle">
          <div className="overflow-x-auto">
            <table aria-label="Database services" className="ui-table w-full">
              <thead>
                <tr className="border-b border-line-strong text-left">
                  <th className="ui-th px-4 py-3 font-medium">Name</th>
                  <th className="ui-th px-2 py-3 font-medium">Type / Version</th>
                  <th className="ui-th px-2 py-3 font-medium">Status</th>
                  <th className="ui-th px-2 py-3 font-medium">Host : Port</th>
                  <th className="ui-th px-2 py-3 font-medium">Resources</th>
                  <th className="ui-th px-2 py-3 text-right font-medium">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line">
                {visibleServices.map((svc) => (
                  <tr key={svc.id} className="transition hover:bg-overlay-subtle">
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2.5">
                        <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg border border-line bg-overlay text-text-subtle">
                          <Database aria-hidden="true" size={14} />
                        </span>
                        <span className="min-w-0">
                          {/* The whole name is on screen; the id line is not its only copy. */}
                          <span className="block break-words text-xs font-bold text-text">{svc.name || "Unnamed service"}</span>
                          <span className="block font-mono text-meta text-text-muted">{svc.id.slice(0, 8)}</span>
                        </span>
                      </div>
                    </td>
                    <td className="px-2 py-3">
                      <span className="block text-xs text-text">{svc.type}</span>
                      <span className="block font-mono text-meta text-text-muted">{svc.version || "—"}</span>
                    </td>
                    <td className="px-2 py-3"><ServiceStatusDot status={svc.status} /></td>
                    <td className="px-2 py-3 font-mono text-meta text-text-subtle">{endpointLabel(svc)}</td>
                    <td className="px-2 py-3 text-meta text-text-subtle">{serviceResources(svc)}</td>
                    <td className="px-2 py-3">
                      <ServiceActions
                        svc={svc}
                        onManage={() => setDetailSvc(svc)}
                        onRestart={() => void restartMut.mutate(svc.id)}
                        onDelete={() => void requestDelete(svc)}
                        pendingAction={rowPendingAction(svc.id)}
                        restartBlockReason={restartBlockReason(svc)}
                      />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line px-4 py-3 text-xs text-text-subtle">
            <span>{servicesRangeLabel}</span>
            <div className="flex items-center gap-2">
              <Pagination label="Database services pagination" onPageChange={setPage} page={currentPage} pageCount={totalPages} />
              <select aria-label="Rows per page" value={pageSize} onChange={(e) => { setPageSize(Number(e.target.value)); setPage(1); }} className="cursor-pointer appearance-none rounded border border-line bg-overlay px-2 py-1 font-mono outline-none">
                <option value={10}>10 / page</option>
                <option value={20}>20 / page</option>
                <option value={50}>50 / page</option>
              </select>
            </div>
          </div>
        </div>
      )}
      <p className="text-meta text-text-muted">
        Manage opens the service’s logs, backups and credentials. Provisioning asks a node to create
        the container, so a service can report <code className="ui-code-inline">failed</code> if the
        node could not run it.
      </p>
      </div>

      {detailSvc && <ServiceDetailModal svc={detailSvc} onClose={() => setDetailSvc(null)} />}
      {showProvision && <ProvisionModal onClose={() => setShowProvision(false)} onDone={() => { setShowProvision(false); qc.invalidateQueries({ queryKey: ["database-services"] }); }} />}
      {showTemplate && <TemplateModal onClose={() => setShowTemplate(false)} onDone={() => { setShowTemplate(false); qc.invalidateQueries({ queryKey: ["service-templates"] }); }} />}
      {showTest && <TestConnectionModal onClose={() => setShowTest(false)} />}
      {renderConfirm()}
    </div>
  );
}

function ServiceStatusDot({ status }: { status: string }) {
  if (!status) return <span className="border-b border-dashed border-unknown-line text-unknown" title="The service reported no status">not reported</span>;
  return <StatusDot status={status} tone={statusTone(status, "database")} />;
}

/**
 * Row and card actions. Icon-only buttons carry an accessible name; the control
 * that cannot run is disabled with the reason in its description rather than
 * hidden; only the row with work in flight shows a spinner.
 */
function ServiceActions({ svc, onManage, onRestart, onDelete, pendingAction, restartBlockReason, layout = "row" }: {
  svc: DatabaseService;
  onManage: () => void;
  onRestart: () => void;
  onDelete: () => void;
  pendingAction: "restart" | "delete" | null;
  restartBlockReason: string | null;
  layout?: "row" | "card";
}) {
  const name = svc.name || svc.id.slice(0, 8);
  const base = "grid h-11 place-items-center rounded text-text-subtle transition-colors hover:bg-overlay-subtle focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] disabled:cursor-not-allowed disabled:opacity-40";
  const sizing = layout === "card" ? "flex-1" : "w-11";
  return (
    <div className={cn("flex items-center gap-1", layout === "card" ? "justify-between" : "justify-end")}>
      <button
        aria-label={`Manage ${name} — logs, backups and credentials`}
        className={cn(base, sizing, "hover:text-[var(--brand)]")}
        onClick={onManage}
        title={`Manage ${name} — logs, backups and credentials`}
        type="button"
      >
        <Rows3 aria-hidden="true" size={14} />
      </button>
      <button
        aria-label={`Restart ${name}`}
        className={cn(base, sizing, "hover:text-text")}
        disabled={pendingAction !== null || restartBlockReason !== null}
        onClick={onRestart}
        title={restartBlockReason ?? (pendingAction === "restart" ? "Restarting…" : `Restart ${name} — the container is stopped, then started`)}
        type="button"
      >
        {pendingAction === "restart" ? <LoaderCircle aria-hidden="true" className="animate-spin" size={14} /> : <RotateCcw aria-hidden="true" size={14} />}
      </button>
      <button
        aria-label={`Delete ${name}`}
        className={cn(base, sizing, "hover:text-danger")}
        disabled={pendingAction !== null}
        onClick={onDelete}
        title={pendingAction === "delete" ? "Deleting…" : `Delete ${name}`}
        type="button"
      >
        <Trash2 aria-hidden="true" size={14} />
      </button>
    </div>
  );
}

function ServiceCard({ svc, onManage, onRestart, onDelete, pendingAction, restartBlockReason }: {
  svc: DatabaseService;
  onManage: () => void;
  onRestart: () => void;
  onDelete: () => void;
  pendingAction: "restart" | "delete" | null;
  restartBlockReason: string | null;
}) {
  return (
    <div className="rounded-xl border border-line bg-overlay-subtle p-4 transition hover:border-line-strong">
      <div className="flex items-start gap-3">
        <span className="grid h-10 w-10 shrink-0 place-items-center rounded-xl border border-line bg-overlay text-text-subtle">
          <Database aria-hidden="true" size={18} />
        </span>
        <div className="min-w-0 flex-1">
          <p className="break-words text-sm font-bold text-text">{svc.name || "Unnamed service"}</p>
          <p className="font-mono text-meta text-text-muted">{svc.id.slice(0, 8)}</p>
        </div>
        <ServiceStatusDot status={svc.status} />
      </div>
      <div className="mt-3 space-y-1 border-t border-line pt-3 font-mono text-meta text-text-subtle">
        <p className="break-words">{svc.type}{svc.version ? ` ${svc.version}` : ""} · {endpointLabel(svc)}</p>
        <p className="break-words">{serviceResources(svc)}</p>
      </div>
      <div className="mt-3 border-t border-line pt-3">
        <ServiceActions
          layout="card"
          onManage={onManage}
          onDelete={onDelete}
          onRestart={onRestart}
          pendingAction={pendingAction}
          restartBlockReason={restartBlockReason}
          svc={svc}
        />
      </div>
    </div>
  );
}

function ProvisionModal({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const { toast } = useToast();
  const [name, setName] = useState("");
  const [type, setType] = useState("");
  const [version, setVersion] = useState("");
  const [memoryMb, setMemoryMb] = useState(256);
  const templatesQ = useQuery({ queryKey: ["service-templates"], queryFn: listServiceTemplates });
  const templates = templatesQ.data ?? [];
  const templatesReady = isAvailable(templatesQ);
  /**
   * Engines and versions come from the API rather than a list written here,
   * because `POST /admin/database-services` validates the pair against the same
   * table (`store.SupportedDBEngines`) and rejects anything else.
   */
  const enginesQ = useQuery({ queryKey: ["db-engines"], queryFn: fetchDBEngines, staleTime: 300_000 });
  const enginesReady = isAvailable(enginesQ);
  const engineNames = useMemo(() => Object.keys(enginesQ.data ?? {}).sort(), [enginesQ.data]);
  const activeType = type || engineNames[0] || "";
  const versions = enginesQ.data?.[activeType] ?? [];
  const activeVersion = version || versions[versions.length - 1] || "";

  const mut = useMutation({
    mutationFn: () => provisionDatabaseService({ name: name.trim(), type: activeType, version: activeVersion, memoryMb }),
    onSuccess: () => { toast({ tone: "success", title: "Provisioning started", message: "The service reports running once the node has created its container." }); onDone(); },
    onError: (e: Error) => toast({ tone: "error", title: "Provision failed", message: e.message }),
  });

  return (
    <Modal description="Creates the service record, then asks a node to run the container" onClose={onClose} title="Provision Database Service" wide>
      <div className="space-y-4">
        {!enginesQ.isPending && enginesQ.isError ? (
          <AdminErrorState
            message={`Could not read the supported engine list: ${enginesQ.error.message}. Provisioning needs it, because the server validates the engine and version you submit.`}
            retry={() => void enginesQ.refetch()}
          />
        ) : !enginesReady ? (
          <AdminLoadingState label="Reading the supported engine list…" />
        ) : engineNames.length === 0 ? (
          <AdminErrorState message="The API reported no supported database engines, so there is nothing to provision." retry={() => void enginesQ.refetch()} />
        ) : (
          <>
            <Input label="Name (optional)" onChange={setName} placeholder="my-db-service" value={name} />
            <div className="grid gap-4 sm:grid-cols-2">
              <label className="block">
                <span className={labelCls}>Type</span>
                <select aria-label="Database type" className={cn(selectCls, "cursor-pointer")} onChange={(e) => { setType(e.target.value); setVersion(""); }} value={activeType}>
                  {engineNames.map((e) => <option key={e} value={e}>{e}</option>)}
                </select>
              </label>
              <label className="block">
                <span className={labelCls}>Version</span>
                <select aria-label="Database version" className={cn(selectCls, "cursor-pointer")} onChange={(e) => setVersion(e.target.value)} value={activeVersion}>
                  {versions.map((v) => <option key={v} value={v}>{v}</option>)}
                </select>
              </label>
            </div>
            <label className="block">
              <span className={labelCls}>Memory (MB)</span>
              <input aria-label="Memory in megabytes" className={selectCls} min={64} onChange={(e) => setMemoryMb(Number(e.target.value))} step={64} type="number" value={memoryMb} />
              <span className="mt-1 block text-meta text-text-muted">A negative figure is rejected; a missing one defaults to 256 MB.</span>
            </label>
            {templatesReady && templates.length > 0 && (
              <div className="rounded-lg border border-line bg-overlay-subtle p-3">
                <p className={labelCls}>Templates — pick one to use its type and version</p>
                <div className="flex flex-wrap gap-2">
                  {templates.map((t) => (
                    <button
                      className={cn("rounded-full border px-3 py-1 text-meta transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]", activeType === t.type && activeVersion === t.version ? "border-brand-line bg-brand-subtle text-text" : "border-line bg-overlay text-text-subtle hover:border-line-strong hover:text-text")}
                      key={t.id}
                      onClick={() => { setType(t.type); setVersion(t.version); }}
                      type="button"
                    >
                      {t.type}:{t.version} → {t.dockerImage}
                    </button>
                  ))}
                </div>
              </div>
            )}
          </>
        )}
      </div>
      <ModalFooter
        confirmLabel={mut.isPending ? "Provisioning…" : "Provision"}
        disabled={mut.isPending || !enginesReady || !activeType || !activeVersion || !Number.isFinite(memoryMb) || memoryMb < 0}
        onCancel={onClose}
        onConfirm={() => mut.mutate()}
      />
    </Modal>
  );
}

function TemplateModal({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const { toast } = useToast();
  const [type, setType] = useState("postgresql");
  const [version, setVersion] = useState("16");
  const [dockerImage, setDockerImage] = useState("postgres:16-alpine");
  const [defaultPort, setDefaultPort] = useState(5432);
  const [defaultDatabase, setDefaultDatabase] = useState("postgres");
  const [minMemoryMb, setMinMemoryMb] = useState(256);

  const mut = useMutation({
    mutationFn: () => createServiceTemplate({ type, version, dockerImage, defaultPort, defaultDatabase, minMemoryMb }),
    onSuccess: () => { toast({ tone: "success", title: "Template created" }); onDone(); },
    onError: (e: Error) => toast({ tone: "error", title: "Create template failed", message: e.message }),
  });
  /** The server rejects these outright, so say so before the attempt, not after. */
  const missing = [!type && "type", !version.trim() && "version", !dockerImage.trim() && "docker image", !(defaultPort > 0) && "default port"].filter(Boolean) as string[];

  return (
    <Modal description="A template fixes the image, port and minimum memory a provision request starts from" onClose={onClose} title="Create Service Template" wide>
      <div className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="block">
            <span className={labelCls}>Type</span>
            <select aria-label="Template type" className={cn(selectCls, "cursor-pointer")} onChange={(e) => setType(e.target.value)} value={type}>
              <option value="postgresql">postgresql</option>
              <option value="mysql">mysql</option>
              <option value="mariadb">mariadb</option>
              <option value="redis">redis</option>
              <option value="mongodb">mongodb</option>
            </select>
          </label>
          <Input label="Version" onChange={setVersion} placeholder="16" value={version} />
        </div>
        <Input label="Docker image" onChange={setDockerImage} placeholder="postgres:16-alpine" value={dockerImage} />
        <div className="grid gap-4 sm:grid-cols-3">
          <label className="block">
            <span className={labelCls}>Default port</span>
            <input aria-label="Default port" className={selectCls} min={1} onChange={(e) => setDefaultPort(Number(e.target.value))} type="number" value={defaultPort} />
          </label>
          <Input label="Default database" onChange={setDefaultDatabase} placeholder="postgres" value={defaultDatabase} />
          <label className="block">
            <span className={labelCls}>Min memory (MB)</span>
            <input aria-label="Minimum memory in megabytes" className={selectCls} min={0} onChange={(e) => setMinMemoryMb(Number(e.target.value))} type="number" value={minMemoryMb} />
            <span className="mt-1 block text-meta text-text-muted">0 is accepted and defaults to 256 MB.</span>
          </label>
        </div>
        {missing.length > 0 && (
          <p className="ui-alert ui-alert-warning" role="status">Required before this can be saved: {missing.join(", ")}.</p>
        )}
      </div>
      <ModalFooter
        confirmLabel={mut.isPending ? "Creating…" : "Create Template"}
        disabled={mut.isPending || missing.length > 0}
        onCancel={onClose}
        onConfirm={() => mut.mutate()}
      />
    </Modal>
  );
}

function TestConnectionModal({ onClose }: { onClose: () => void }) {
  const { toast } = useToast();
  const [host, setHost] = useState("127.0.0.1");
  const [port, setPort] = useState("");
  const [engine, setEngine] = useState("postgresql");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [databaseName, setDatabaseName] = useState("postgres");

  const mut = useMutation({
    mutationFn: async () => {
      const result = await testConnection({ host, port: Number(port) || 0, engine, username, password, databaseName });
      if (!result.ok) throw new Error(result.message || "The server reported the connection test did not succeed.");
      return result;
    },
    onSuccess: (res) => toast({ tone: "success", title: res.message || "Connection successful" }),
    onError: (e: Error) => toast({ tone: "error", title: "Test failed", message: e.message }),
  });
  const missing = [!host.trim() && "host", !engine && "engine", !username.trim() && "username"].filter(Boolean) as string[];

  return (
    <Modal description="Connects with the credentials you enter and reports the result; nothing is saved" onClose={onClose} title="Test Connection" wide>
      <div className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <Input label="Host" onChange={setHost} placeholder="127.0.0.1" value={host} />
          <label className="block">
            <span className={labelCls}>Port</span>
            <input aria-label="Port" className={selectCls} onChange={(e) => setPort(e.target.value.replace(/[^0-9]/g, ""))} placeholder="Blank uses the engine default" value={port} />
          </label>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="block">
            <span className={labelCls}>Engine</span>
            <select aria-label="Engine" className={cn(selectCls, "cursor-pointer")} onChange={(e) => setEngine(e.target.value)} value={engine}>
              <option value="postgresql">postgresql</option>
              <option value="mysql">mysql</option>
              <option value="mariadb">mariadb</option>
              <option value="redis">redis</option>
              <option value="mongodb">mongodb</option>
            </select>
          </label>
          <Input label="Username" onChange={setUsername} value={username} />
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <Input label="Password" onChange={setPassword} type="password" value={password} />
          <Input label="Database" onChange={setDatabaseName} placeholder="postgres" value={databaseName} />
        </div>
        <p className="text-meta text-text-muted">
          Host, engine and username are required. A blank port uses the engine default: postgresql
          5432, mysql and mariadb 3306, redis 6379, mongodb 27017.
        </p>
        {missing.length > 0 && (
          <p className="ui-alert ui-alert-warning" role="status">Required before testing: {missing.join(", ")}.</p>
        )}
      </div>
      <ModalFooter
        confirmLabel={mut.isPending ? "Testing…" : "Test Connection"}
        disabled={mut.isPending || missing.length > 0}
        onCancel={onClose}
        onConfirm={() => mut.mutate()}
      />
    </Modal>
  );
}

const detailTabs = [
  { id: "logs", label: "Logs", icon: FileText },
  { id: "backups", label: "Backups", icon: History },
  { id: "credentials", label: "Credentials", icon: KeyRound },
] as const;

type DetailTabId = (typeof detailTabs)[number]["id"];

function ServiceDetailModal({ svc, onClose }: { svc: DatabaseService; onClose: () => void }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [section, setSection] = useState<DetailTabId>("logs");
  const [credUser, setCredUser] = useState("");
  const [credPass, setCredPass] = useState("");
  const [credDb, setCredDb] = useState(svc.databaseName ?? "");
  const [credPerms, setCredPerms] = useState("read-write");

  const logsQ = useQuery({
    queryKey: ["db-service-logs", svc.id],
    queryFn: () => getServiceLogs(svc.id),
    enabled: section === "logs",
    refetchInterval: 15_000,
  });
  const backupsQ = useQuery({
    queryKey: ["db-service-backups", svc.id],
    queryFn: () => listServiceBackups(svc.id),
    enabled: section === "backups",
  });
  const credsQ = useQuery({
    queryKey: ["db-service-credentials", svc.id],
    queryFn: () => listServiceCredentials(svc.id),
    enabled: section === "credentials",
  });

  const createBackupMut = useMutation({
    mutationFn: () => createServiceBackup(svc.id),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["db-service-backups", svc.id] }); toast({ tone: "success", title: "Backup started", message: "It appears as running until the dump completes." }); },
    onError: (e: Error) => toast({ tone: "error", title: "Backup failed", message: e.message }),
  });
  const restoreMut = useMutation({
    mutationFn: async (backupId: string) => {
      const result = await restoreServiceBackup(svc.id, backupId);
      if (!result.ok) throw new Error("The server reported the restore did not complete.");
      return result;
    },
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["db-service-backups", svc.id] }); toast({ tone: "success", title: "Restore started" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Restore failed", message: e.message }),
  });
  const createCredMut = useMutation({
    mutationFn: (data: { username: string; password: string; database?: string; permissions?: string }) => createServiceCredential(svc.id, data),
    onSuccess: () => {
      setCredUser(""); setCredPass("");
      qc.invalidateQueries({ queryKey: ["db-service-credentials", svc.id] });
      toast({ tone: "success", title: "Credential created" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Create credential failed", message: e.message }),
  });
  const revokeCredMut = useMutation({
    mutationFn: async (credId: string) => {
      const result = await revokeServiceCredential(svc.id, credId);
      if (!result.ok) throw new Error("The server reported the credential was not revoked.");
      return result;
    },
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["db-service-credentials", svc.id] }); toast({ tone: "success", title: "Credential revoked" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Revoke failed", message: e.message }),
  });

  const logs = logsQ.data?.logs ?? [];
  const backups: DatabaseServiceBackup[] = backupsQ.data ?? [];
  const creds: DatabaseServiceCredential[] = credsQ.data ?? [];

  return (
    <Modal description={`${svc.type} ${svc.version} · ${endpointLabel(svc)} — logs, backups and credentials`} onClose={onClose} title={`Manage ${svc.name || svc.id.slice(0, 8)}`} wide>
      <div className="mb-4 flex gap-1 border-b border-line" role="tablist" aria-label="Service sections">
        {detailTabs.map(({ id: tId, label, icon: Icon }) => (
          <button key={tId} type="button" role="tab" aria-selected={section === tId} onClick={() => setSection(tId)} className={cn("-mb-px flex items-center gap-1.5 border-b-2 px-3 py-2 text-xs font-medium transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]", section === tId ? "border-[var(--brand)] text-[var(--brand)]" : "border-transparent text-text-muted hover:text-text")}>
            <Icon aria-hidden="true" size={12} /> {label}
          </button>
        ))}
      </div>

      {section === "logs" && (
        <div className="space-y-3">
          <div className="flex items-center justify-between gap-3">
            <p className="text-meta text-text-muted">The last 50 lines the container produced; refreshed every 15 s.</p>
            <Btn ariaLabel="Reload logs" size="sm" tone="ghost" onClick={() => void logsQ.refetch()} disabled={logsQ.isFetching}><RefreshCw aria-hidden="true" size={12} className={logsQ.isFetching ? "animate-spin" : undefined} /> Refresh</Btn>
          </div>
          {logsQ.isPending ? (
            <AdminLoadingState label="Loading logs…" />
          ) : logsQ.isError ? (
            <AdminErrorState message={`Could not read the logs: ${logsQ.error.message}`} retry={() => void logsQ.refetch()} />
          ) : logs.length === 0 ? (
            <EmptyState icon={FileText} title="No logs" message="The service has produced no log lines recently." />
          ) : (
            <pre className="max-h-96 overflow-auto rounded-lg border border-line bg-overlay p-3 font-mono text-xs whitespace-pre-wrap text-text-subtle">{logs.join("\n")}</pre>
          )}
        </div>
      )}

      {section === "backups" && (
        <div className="space-y-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-meta text-text-muted">A restore replaces the data the service holds now.</p>
            <div className="flex gap-2">
              <Btn ariaLabel="Reload backups" size="sm" tone="ghost" onClick={() => void backupsQ.refetch()} disabled={backupsQ.isFetching}><RefreshCw aria-hidden="true" size={12} className={backupsQ.isFetching ? "animate-spin" : undefined} /> Refresh</Btn>
              <Btn size="sm" onClick={() => createBackupMut.mutate()} disabled={createBackupMut.isPending}>{createBackupMut.isPending ? "Starting…" : "Create Backup"}</Btn>
            </div>
          </div>
          {backupsQ.isPending ? (
            <AdminLoadingState label="Loading backups…" />
          ) : backupsQ.isError ? (
            <AdminErrorState message={`Could not read backups: ${backupsQ.error.message}`} retry={() => void backupsQ.refetch()} />
          ) : backups.length === 0 ? (
            <EmptyState icon={History} title="No backups" message="No backup has been taken for this service yet." />
          ) : (
            <div className="overflow-x-auto rounded-lg border border-line">
              <table aria-label="Service backups" className="ui-table w-full">
                <thead>
                  <tr className="border-b border-line-strong text-left">
                    <th className="ui-th px-3 py-2 font-medium">Backup</th><th className="ui-th px-3 py-2 font-medium">Status</th><th className="ui-th px-3 py-2 font-medium">Size</th><th className="ui-th px-3 py-2 font-medium">Created</th><th className="ui-th px-3 py-2 text-right font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-line">
                  {backups.map((b) => {
                    const restorable = b.status === "completed";
                    const label = b.filePath ? b.filePath.split("/").pop() : b.id;
                    return (
                      <tr key={b.id}>
                        <td className="px-3 py-2 break-words font-mono text-meta text-text">{label}</td>
                        <td className="px-3 py-2"><Pill tone={statusTone(b.status, "deployment")}>{b.status || "not reported"}</Pill></td>
                        {/* An in-flight backup has no size yet; that is not a 0 B backup. */}
                        <td className="px-3 py-2 text-meta text-text-subtle">{b.sizeBytes > 0 ? formatBytes(b.sizeBytes) : restorable ? "0 B" : "Not reported"}</td>
                        <td className="px-3 py-2 text-meta text-text-muted">{formatDate(b.createdAt, "—")}</td>
                        <td className="px-3 py-2 text-right">
                          <Btn size="sm" tone="ghost" title={restorable ? `Restore ${label}` : `Only a completed backup can be restored — this one is ${b.status || "not reported"}`} disabled={!restorable || restoreMut.isPending} onClick={() => { void (async () => { if (await confirm({ title: `Restore ${label}?`, description: `Everything currently in ${svc.name || svc.id.slice(0, 8)} is replaced by this backup, and the service is restarted while the restore runs.`, danger: true, confirmLabel: "Restore backup" })) restoreMut.mutate(b.id); })(); }}>
                            <Power aria-hidden="true" size={12} /> Restore
                          </Btn>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {section === "credentials" && (
        <div className="space-y-4">
          <div className="rounded-lg border border-line bg-overlay-subtle p-3">
            <p className={labelCls}>Create credential</p>
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <Input label="Username" value={credUser} onChange={setCredUser} placeholder="app_user" />
              <Input label="Password" value={credPass} onChange={setCredPass} type="password" />
              <Input label="Database (optional grant)" value={credDb} onChange={setCredDb} placeholder={svc.databaseName || "db"} />
              <label className="block">
                <span className={labelCls}>Permissions</span>
                <select aria-label="Credential permissions" className={cn(selectCls, "cursor-pointer")} value={credPerms} onChange={(e) => setCredPerms(e.target.value)}>
                  <option value="read-write">read-write</option>
                  <option value="read-only">read-only</option>
                </select>
              </label>
            </div>
            <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
              <p className="text-meta text-text-muted">
                {!credUser.trim() || !credPass
                  ? "Username and password are required — the server rejects the request without them."
                  : credDb.trim() ? `Granted ${credPerms} on ${credDb.trim()}.` : `Created with ${credPerms} and no database grant.`}
              </p>
              <Btn size="sm" onClick={() => createCredMut.mutate({ username: credUser.trim(), password: credPass, database: credDb.trim() || undefined, permissions: credPerms })} disabled={createCredMut.isPending || !credUser.trim() || !credPass}>
                {createCredMut.isPending ? "Creating…" : "Create Credential"}
              </Btn>
            </div>
          </div>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-meta text-text-muted">Passwords are never returned once issued — rotate rather than try to retrieve one.</p>
            <Btn ariaLabel="Reload credentials" size="sm" tone="ghost" onClick={() => void credsQ.refetch()} disabled={credsQ.isFetching}><RefreshCw aria-hidden="true" size={12} className={credsQ.isFetching ? "animate-spin" : undefined} /> Refresh</Btn>
          </div>
          {credsQ.isPending ? (
            <AdminLoadingState label="Loading credentials…" />
          ) : credsQ.isError ? (
            <AdminErrorState message={`Could not read credentials: ${credsQ.error.message}`} retry={() => void credsQ.refetch()} />
          ) : creds.length === 0 ? (
            <EmptyState icon={KeyRound} title="No credentials" message="No credential has been issued for this service yet." />
          ) : (
            <div className="overflow-x-auto rounded-lg border border-line">
              <table aria-label="Service credentials" className="ui-table w-full">
                <thead>
                  <tr className="border-b border-line-strong text-left">
                    <th className="ui-th px-3 py-2 font-medium">Username</th><th className="ui-th px-3 py-2 font-medium">Database</th><th className="ui-th px-3 py-2 font-medium">Permissions</th><th className="ui-th px-3 py-2 font-medium">Created</th><th className="ui-th px-3 py-2 font-medium">State</th><th className="ui-th px-3 py-2 text-right font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-line">
                  {creds.map((cred) => (
                    <tr key={cred.id}>
                      <td className="px-3 py-2 break-words font-mono text-meta text-text">{cred.username}</td>
                      <td className="px-3 py-2 font-mono text-meta text-text-subtle">{cred.databaseName || "—"}</td>
                      <td className="px-3 py-2"><Pill tone={cred.permissions === "read-only" ? "info" : "neutral"}>{cred.permissions || "not reported"}</Pill></td>
                      <td className="px-3 py-2 text-meta text-text-muted">{formatDate(cred.createdAt, "—")}</td>
                      <td className="px-3 py-2">
                        {cred.revokedAt
                          ? <Pill tone="neutral">revoked {formatDate(cred.revokedAt, "")}</Pill>
                          : <Pill tone="ok">active</Pill>}
                      </td>
                      <td className="px-3 py-2 text-right">
                        {cred.revokedAt ? (
                          <span className="text-meta text-text-muted">Revoked</span>
                        ) : (
                          <Btn size="sm" tone="danger" disabled={revokeCredMut.isPending} onClick={() => { void (async () => { if (await confirm({ title: `Revoke ${cred.username}?`, description: `Any app using this credential loses database access to ${cred.databaseName || "the service"} immediately; live connections may fail rather than reconnect.`, danger: true, confirmLabel: "Revoke credential" })) revokeCredMut.mutate(cred.id); })(); }}>
                            Revoke
                          </Btn>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
      {renderConfirm()}
    </Modal>
  );
}
