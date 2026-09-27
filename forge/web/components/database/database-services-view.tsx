"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Database, Plus, Trash2, RotateCcw, FlaskConical, Layers, Server, Search, List, LayoutGrid, Rows3, FileText, History, KeyRound, RefreshCw, Power } from "lucide-react";
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
import { Btn, EmptyState, Input, Modal, ModalFooter, SectionHeader, Pill, cn } from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { statusTone } from "@/lib/api/status";
import { StatusDot } from "@/components/ui/primitives";
import { formatBytes, formatDate } from "@/lib/utils";
import { DbStatCards, type DbStat } from "./databases-overview";

const selectCls = "h-10 w-full rounded-lg border border-white/10 bg-surface-card-header px-3.5 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition hover:border-white/20 focus:border-[color-mix(in_srgb,var(--brand)_70%,transparent)] focus:ring-2 focus:ring-[color-mix(in_srgb,var(--brand)_15%,transparent)]";

export function DatabaseServicesView() {
  const qc = useQueryClient();
  const { toast } = useToast();
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
  const runningCount = useMemo(() => services.filter((s) => s.status?.toLowerCase() === "running").length, [services]);
  const memorySum = useMemo(() => services.reduce((acc, s) => acc + (s.memoryMb ?? 0), 0), [services]);
  const statsLoading = servicesQ.isLoading || templatesQ.isLoading;

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

  const stats: DbStat[] = [
    { key: "total", label: "Services", icon: Database, tile: "border-white/[0.08] bg-white/[0.03] text-slate-300", value: statsLoading ? "…" : services.length },
    { key: "running", label: "Running", icon: Server, tile: "border-emerald-500/25 bg-emerald-500/10 text-emerald-300", value: statsLoading ? "…" : runningCount },
    { key: "templates", label: "Templates", icon: Layers, tile: "border-sky-500/25 bg-sky-500/10 text-sky-300", value: statsLoading ? "…" : templates.length },
    { key: "memory", label: "Memory", icon: Database, tile: "border-amber-500/25 bg-amber-500/10 text-amber-300", value: statsLoading ? "…" : `${memorySum} MB` },
  ];

  const filterSelectWrap = "relative flex flex-col justify-center rounded-lg border border-white/[0.08] bg-black/20 px-3 py-1";
  const filterSelectCls = "h-6 cursor-pointer appearance-none border-0 bg-transparent pl-0 pr-8 text-xs text-slate-200 outline-none";

  const restartMut = useMutation({
    mutationFn: async (id: string) => {
      const result = await restartDatabaseService(id);
      if (!result.ok) throw new Error("The server reported the service restart did not complete.");
      return result;
    },
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["database-services"] }); toast({ tone: "success", title: "Service restart initiated — POST /admin/database-services/:id/restart" }); },
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

  return (
    <div className="space-y-4">
      <SectionHeader
        title="Database Services"
        sub="Provisioned database services (PostgreSQL/MySQL/Redis/etc) — provision, restart, test connection, templates"
        action={
          <div className="flex flex-wrap gap-2">
            <Btn tone="ghost" onClick={() => setShowTest(true)}><FlaskConical size={14} /> Test Connection</Btn>
            <Btn tone="ghost" onClick={() => setShowTemplate(true)}><Layers size={14} /> New Template</Btn>
            <Btn onClick={() => setShowProvision(true)}><Plus size={14} /> Provision Service</Btn>
          </div>
        }
      />

      <DbStatCards stats={stats} />

      <div className="overflow-hidden rounded-xl border border-white/[0.08] bg-[var(--surface)] shadow-sm">
        <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
          <h2 className="flex items-center gap-2 text-sm font-bold text-slate-100"><Layers size={15} className="text-slate-400" /> Service Templates ({filteredTemplates.length})</h2>
          <div className="w-full max-w-64"><Input placeholder="Filter type/version..." value={templateFilter} onChange={setTemplateFilter} /></div>
        </div>
        {templatesQ.isLoading ? (
          <div className="py-6 text-center text-sm text-slate-400">Loading templates…</div>
        ) : templatesQ.isError ? (
          <div className="p-4 text-sm text-red-300">Failed to load templates: {(templatesQ.error as Error).message} <Btn size="sm" tone="ghost" onClick={() => void templatesQ.refetch()}>Retry</Btn></div>
        ) : filteredTemplates.length === 0 ? (
          <EmptyState icon={Layers} title="No templates" message="No service templates yet. Create one to define default images/ports. GET /admin/database-service-templates" />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead><tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-wider text-slate-500"><th className="px-4 py-3 font-medium">Type</th><th className="px-2 py-3 font-medium">Version</th><th className="px-2 py-3 font-medium">Image</th><th className="px-2 py-3 font-medium">Port</th><th className="px-2 py-3 font-medium">Min Mem</th></tr></thead>
              <tbody className="divide-y divide-white/[0.04]">
                {filteredTemplates.map((t) => (
                  <tr key={t.id} className="transition hover:bg-white/[0.02]"><td className="px-4 py-3"><Pill tone="blue">{t.type}</Pill></td><td className="px-2 py-3 font-mono text-[11px] text-slate-300">{t.version}</td><td className="px-2 py-3 font-mono text-[11px] text-slate-400">{t.dockerImage}</td><td className="px-2 py-3 font-mono text-[11px] text-slate-400">{t.defaultPort}</td><td className="px-2 py-3 text-[11px] text-slate-400">{t.minMemoryMb} MB</td></tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <p className="border-t border-white/[0.06] px-4 py-3 text-[11px] text-slate-500">Wires <code className="font-mono">listServiceTemplates</code> &amp; <code className="font-mono">createServiceTemplate</code> — POST /admin/database-service-templates</p>
      </div>

      <div className="space-y-4">
      <div className="flex flex-col gap-2 xl:flex-row xl:items-center">
        <label className="flex min-w-52 flex-1 items-center gap-2 rounded-lg border border-white/[0.08] bg-black/20 px-2.5 py-2">
          <Search size={13} className="shrink-0 text-slate-500" />
          <input
            type="text"
            value={search}
            onChange={(e) => resetPage(() => setSearch(e.target.value))}
            placeholder="Search services by name, type, status…"
            aria-label="Search services"
            className="w-full bg-transparent text-xs text-slate-200 outline-none placeholder:text-slate-600"
          />
        </label>
        <div className="flex flex-wrap items-center gap-2">
          <label className={filterSelectWrap}>
            <span className="text-[10px] leading-3 text-slate-500">Status</span>
            <select aria-label="Filter by status" value={statusFilter} onChange={(e) => resetPage(() => setStatusFilter(e.target.value))} className={filterSelectCls}>
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
          {hasActiveServiceFilters && (
            <button type="button" onClick={() => { setSearch(""); setStatusFilter("all"); setPage(1); }} className="rounded-lg px-2.5 py-2 text-xs font-semibold text-slate-400 transition hover:text-white">
              Clear
            </button>
          )}
        </div>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-sm font-bold text-slate-100">Database Services ({sortedServices.length})</h2>
        <label className="flex items-center gap-2 rounded-lg border border-white/[0.08] bg-black/20 px-2.5 py-1.5 text-xs text-slate-300">
          <span className="text-[11px] text-slate-500">Sort by</span>
          <select aria-label="Sort services" value={sort} onChange={(e) => setSort(e.target.value)} className="cursor-pointer appearance-none bg-transparent pr-1 outline-none">
            <option value="name-asc">Name (A → Z)</option>
            <option value="name-desc">Name (Z → A)</option>
            <option value="status">Status</option>
            <option value="memory">Memory</option>
          </select>
        </label>
      </div>

      {servicesQ.isLoading ? (
        <div className="rounded-xl border border-white/[0.08] bg-[var(--surface)] py-10 text-center text-sm text-slate-400 shadow-sm">Loading services…</div>
      ) : servicesQ.isError ? (
        <div className="rounded-xl border border-red-500/20 bg-red-950/10 p-4 text-sm text-red-200">Failed to load: {(servicesQ.error as Error).message} <Btn size="sm" tone="ghost" onClick={() => void servicesQ.refetch()}>Retry</Btn></div>
      ) : services.length === 0 ? (
        <EmptyState icon={Database} title="No services" message="No database services provisioned. Use Provision Service — POST /admin/database-services" />
      ) : sortedServices.length === 0 ? (
        <EmptyState icon={Database} title="No matches" message="No services match these filters." />
      ) : view === "cards" ? (
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {visibleServices.map((svc) => (
            <ServiceCard
              key={svc.id}
              svc={svc}
              onManage={() => setDetailSvc(svc)}
              onRestart={() => restartMut.mutate(svc.id)}
              onDelete={() => deleteMut.mutate(svc.id)}
              restartPending={restartMut.isPending}
              deletePending={deleteMut.isPending}
            />
          ))}
        </div>
      ) : (
        <div className="overflow-hidden rounded-xl border border-white/[0.08] bg-[var(--surface)] shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-wider text-slate-500">
                  <th className="px-4 py-3 font-medium">Name</th>
                  <th className="px-2 py-3 font-medium">Type / Version</th>
                  <th className="px-2 py-3 font-medium">Status</th>
                  <th className="px-2 py-3 font-medium">Host : Port</th>
                  <th className="px-2 py-3 font-medium">Memory</th>
                  <th className="px-2 py-3 text-right font-medium">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {visibleServices.map((svc) => (
                  <tr key={svc.id} className="transition hover:bg-white/[0.02]">
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2.5">
                        <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg border border-white/[0.08] bg-white/[0.03] text-slate-400">
                          <Database size={14} />
                        </span>
                        <span className="min-w-0">
                          <span className="block max-w-44 truncate text-xs font-bold text-slate-100" title={svc.name || svc.id}>{svc.name || svc.id.slice(0, 8)}</span>
                          <span className="block font-mono text-[10px] text-slate-500">{svc.id.slice(0, 8)}</span>
                        </span>
                      </div>
                    </td>
                    <td className="px-2 py-3">
                      <span className="block text-xs text-slate-200">{svc.type}</span>
                      <span className="block font-mono text-[10px] text-slate-500">{svc.version}</span>
                    </td>
                    <td className="px-2 py-3"><ServiceStatusDot status={svc.status} /></td>
                    <td className="px-2 py-3 font-mono text-[11px] text-slate-300">{svc.host}:{svc.port}</td>
                    <td className="px-2 py-3 text-[11px] text-slate-400">{svc.memoryMb} MB</td>
                    <td className="px-2 py-3">
                      <div className="flex items-center justify-end gap-1">
                        <button disabled={false} onClick={() => setDetailSvc(svc)} className="grid h-9 w-9 place-items-center rounded text-slate-400 hover:bg-white/[0.06] hover:text-sky-200" title="Manage — logs, backups, credentials" type="button"><Rows3 size={14} /></button>
                        <button disabled={restartMut.isPending} onClick={() => restartMut.mutate(svc.id)} className="grid h-9 w-9 place-items-center rounded text-slate-400 hover:bg-white/[0.06] hover:text-amber-200 disabled:opacity-40" title="Restart — POST /admin/database-services/:id/restart" type="button"><RotateCcw size={14} /></button>
                        <button disabled={deleteMut.isPending} onClick={() => deleteMut.mutate(svc.id)} className="grid h-9 w-9 place-items-center rounded text-slate-400 hover:bg-white/[0.06] hover:text-red-200 disabled:opacity-40" title="Delete" type="button"><Trash2 size={14} /></button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-white/[0.06] px-4 py-3 text-xs text-slate-400">
            <span>Showing {(currentPage - 1) * pageSize + 1}–{Math.min(currentPage * pageSize, sortedServices.length)} of {sortedServices.length} services</span>
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
      <p className="text-[11px] text-slate-500">Wires <code className="font-mono">provisionDatabaseService</code>, <code className="font-mono">restartDatabaseService</code>, <code className="font-mono">testConnection</code>; Manage opens <code className="font-mono">getServiceLogs</code>, <code className="font-mono">listServiceBackups</code>/<code className="font-mono">restoreServiceBackup</code>, and the credentials section</p>
      </div>

      {detailSvc && <ServiceDetailModal svc={detailSvc} onClose={() => setDetailSvc(null)} />}
      {showProvision && <ProvisionModal onClose={() => setShowProvision(false)} onDone={() => { setShowProvision(false); qc.invalidateQueries({ queryKey: ["database-services"] }); }} />}
      {showTemplate && <TemplateModal onClose={() => setShowTemplate(false)} onDone={() => { setShowTemplate(false); qc.invalidateQueries({ queryKey: ["service-templates"] }); }} />}
      {showTest && <TestConnectionModal onClose={() => setShowTest(false)} />}
    </div>
  );
}

function ServiceStatusDot({ status }: { status: string }) {
  return <StatusDot status={status} tone={statusTone(status)} />;
}

function ServiceCard({ svc, onManage, onRestart, onDelete, restartPending, deletePending }: {
  svc: DatabaseService;
  onManage: () => void;
  onRestart: () => void;
  onDelete: () => void;
  restartPending: boolean;
  deletePending: boolean;
}) {
  return (
    <div className="rounded-xl border border-white/[0.08] bg-[var(--surface)] p-4 shadow-sm transition hover:border-white/20">
      <div className="flex items-start gap-3">
        <span className="grid h-10 w-10 shrink-0 place-items-center rounded-xl border border-white/[0.08] bg-white/[0.03] text-slate-300">
          <Database size={18} />
        </span>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-bold text-slate-100" title={svc.name || svc.id}>{svc.name || svc.id.slice(0, 8)}</p>
          <p className="font-mono text-[10px] text-slate-500">{svc.id.slice(0, 8)}</p>
        </div>
        <ServiceStatusDot status={svc.status} />
      </div>
      <div className="mt-3 space-y-1 border-t border-white/[0.06] pt-3 font-mono text-[11px] text-slate-400">
        <p className="truncate">{svc.type}{svc.version ? ` ${svc.version}` : ""} · {svc.host}:{svc.port}</p>
        <p className="truncate">{svc.memoryMb} MB</p>
      </div>
      <div className="mt-3 flex gap-2">
        <button type="button" onClick={onManage} title="Manage — logs, backups, credentials" className="flex flex-1 items-center justify-center gap-1.5 rounded-lg border border-white/[0.08] px-3 py-2 text-xs font-bold text-slate-200 transition hover:border-white/20 hover:text-sky-200">
          <Rows3 size={14} /> Manage
        </button>
        <button type="button" disabled={restartPending} onClick={onRestart} title="Restart — POST /admin/database-services/:id/restart" className="flex flex-1 items-center justify-center gap-1.5 rounded-lg border border-white/[0.08] px-3 py-2 text-xs font-bold text-slate-200 transition hover:border-white/20 hover:text-amber-200 disabled:opacity-40">
          <RotateCcw size={14} /> Restart
        </button>
        <button type="button" disabled={deletePending} onClick={onDelete} title="Delete" className="flex flex-1 items-center justify-center gap-1.5 rounded-lg border border-white/[0.08] px-3 py-2 text-xs font-bold text-slate-200 transition hover:border-white/20 hover:text-red-200 disabled:opacity-40">
          <Trash2 size={14} /> Delete
        </button>
      </div>
    </div>
  );
}

function ProvisionModal({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const { toast } = useToast();
  const [name, setName] = useState("");
  const [type, setType] = useState("postgresql");
  const [version, setVersion] = useState("16");
  const [memoryMb, setMemoryMb] = useState(256);
  const templatesQ = useQuery({ queryKey: ["service-templates"], queryFn: listServiceTemplates });

  const mut = useMutation({
    mutationFn: () => provisionDatabaseService({ name: name || `db-${Date.now()}`, type, version, memoryMb }),
    onSuccess: () => { toast({ tone: "success", title: "Database service provisioned — POST /admin/database-services" }); onDone(); },
    onError: (e: Error) => toast({ tone: "error", title: "Provision failed", message: e.message }),
  });

  return (
    <Modal title="Provision Database Service" description="POST /admin/database-services" onClose={onClose} wide>
      <div className="space-y-4">
        <Input label="Name (optional, auto if blank)" value={name} onChange={setName} placeholder="my-db-service" />
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Type</label>
            <select className={selectCls} value={type} onChange={(e) => { setType(e.target.value); const t = templatesQ.data?.filter((x) => x.type===e.target.value); if (t && t.length) setVersion(t[t.length-1].version); }}>
              <option value="postgresql">postgresql</option>
              <option value="mysql">mysql</option>
              <option value="mariadb">mariadb</option>
              <option value="redis">redis</option>
              <option value="mongodb">mongodb</option>
            </select>
          </div>
          <Input label="Version" value={version} onChange={setVersion} placeholder="16" />
        </div>
        <div>
          <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Memory MB</label>
          <input type="number" className={selectCls} value={memoryMb} onChange={(e) => setMemoryMb(Number(e.target.value))} min={64} step={64} />
        </div>
        {templatesQ.data && templatesQ.data.length > 0 && (
          <div className="rounded-lg border border-white/[0.06] bg-white/[0.02] p-3">
            <p className="mb-2 text-xs font-semibold uppercase tracking-wider text-slate-400">Templates selector — pick to autofill version/image</p>
            <div className="flex flex-wrap gap-2">
              {templatesQ.data.map((t) => (
                <button key={t.id} type="button" onClick={() => { setType(t.type); setVersion(t.version); }} className={`rounded-full border px-3 py-1 text-xs ${type===t.type && version===t.version ? "border-[var(--brand)] bg-[color-mix(in_srgb,var(--brand)_20%,transparent)] text-white" : "border-white/10 bg-white/[0.04] text-slate-300 hover:bg-white/[0.06]"}`}>{t.type}:{t.version} → {t.dockerImage}</button>
              ))}
            </div>
          </div>
        )}
      </div>
      <ModalFooter onCancel={onClose} onConfirm={() => mut.mutate()} disabled={mut.isPending || !type || !version} confirmLabel={mut.isPending ? "Provisioning…" : "Provision"} />
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
    onSuccess: () => { toast({ tone: "success", title: "Template created — POST /admin/database-service-templates" }); onDone(); },
    onError: (e: Error) => toast({ tone: "error", title: "Create template failed", message: e.message }),
  });

  return (
    <Modal title="Create Service Template" description="POST /admin/database-service-templates" onClose={onClose} wide>
      <div className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div><label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Type</label><select className={selectCls} value={type} onChange={(e) => setType(e.target.value)}><option value="postgresql">postgresql</option><option value="mysql">mysql</option><option value="mariadb">mariadb</option><option value="redis">redis</option><option value="mongodb">mongodb</option></select></div>
          <Input label="Version" value={version} onChange={setVersion} placeholder="16" />
        </div>
        <Input label="Docker Image" value={dockerImage} onChange={setDockerImage} placeholder="postgres:16-alpine" />
        <div className="grid gap-4 sm:grid-cols-3">
          <div><label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Default Port</label><input type="number" className={selectCls} value={defaultPort} onChange={(e) => setDefaultPort(Number(e.target.value))} /></div>
          <Input label="Default DB" value={defaultDatabase} onChange={setDefaultDatabase} placeholder="postgres" />
          <div><label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Min Memory MB</label><input type="number" className={selectCls} value={minMemoryMb} onChange={(e) => setMinMemoryMb(Number(e.target.value))} /></div>
        </div>
      </div>
      <ModalFooter onCancel={onClose} onConfirm={() => mut.mutate()} disabled={mut.isPending || !type || !version || !dockerImage} confirmLabel={mut.isPending ? "Creating…" : "Create Template"} />
    </Modal>
  );
}

function TestConnectionModal({ onClose }: { onClose: () => void }) {
  const { toast } = useToast();
  const [host, setHost] = useState("127.0.0.1");
  const [port, setPort] = useState("5432");
  const [engine, setEngine] = useState("postgresql");
  const [username, setUsername] = useState("gamepanel");
  const [password, setPassword] = useState("");
  const [databaseName, setDatabaseName] = useState("postgres");

  const mut = useMutation({
    mutationFn: async () => {
      const result = await testConnection({ host, port: Number(port), engine, username, password, databaseName });
      if (!result.ok) throw new Error(result.message || "The server reported the connection test did not succeed.");
      return result;
    },
    onSuccess: (res) => toast({ tone: "success", title: res.message || "Connection successful — POST /admin/database-services/test-connection" }),
    onError: (e: Error) => toast({ tone: "error", title: "Test failed", message: e.message }),
  });

  return (
    <Modal title="Test Connection" description="POST /admin/database-services/test-connection" onClose={onClose} wide>
      <div className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <Input label="Host" value={host} onChange={setHost} placeholder="127.0.0.1" />
          <div><label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Port</label><input className={selectCls} value={port} onChange={(e) => setPort(e.target.value)} placeholder="5432" /></div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div><label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Engine</label><select className={selectCls} value={engine} onChange={(e) => setEngine(e.target.value)}><option value="postgresql">postgresql</option><option value="mysql">mysql</option><option value="mariadb">mariadb</option><option value="redis">redis</option><option value="mongodb">mongodb</option></select></div>
          <Input label="Username" value={username} onChange={setUsername} />
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <Input label="Password" value={password} onChange={setPassword} type="password" placeholder="••••••••" />
          <Input label="Database" value={databaseName} onChange={setDatabaseName} placeholder="postgres" />
        </div>
        <p className="text-xs text-slate-400">Tests <code className="font-mono">POST /admin/database-services/test-connection</code> — host/engine/username required, port defaults via defaultPortForEngine.</p>
      </div>
      <ModalFooter onCancel={onClose} onConfirm={() => mut.mutate()} disabled={mut.isPending || !host || !engine || !username} confirmLabel={mut.isPending ? "Testing…" : "Test Connection"} />
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
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["db-service-backups", svc.id] }); toast({ tone: "success", title: "Backup created — POST /admin/database-services/:id/backups" }); },
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
      toast({ tone: "success", title: "Credential created — POST …/credentials" });
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
    <Modal title={`Manage ${svc.name || svc.id.slice(0, 8)}`} description={`${svc.type} ${svc.version} · ${svc.host}:${svc.port} — logs, backups, credentials`} onClose={onClose} wide>
      <div className="mb-4 flex gap-1 border-b border-white/[0.06]">
        {detailTabs.map(({ id: tId, label, icon: Icon }) => (
          <button key={tId} type="button" onClick={() => setSection(tId)} className={cn("-mb-px flex items-center gap-1.5 border-b-2 px-3 py-2 text-xs font-medium transition", section === tId ? "border-[var(--brand)] text-[var(--brand)]" : "border-transparent text-slate-500 hover:text-slate-300")}>
            <Icon size={12} /> {label}
          </button>
        ))}
      </div>

      {section === "logs" && (
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <p className="text-[11px] text-slate-500">GET /admin/database-services/:id/logs — last 50 lines</p>
            <Btn size="sm" tone="ghost" onClick={() => void logsQ.refetch()} disabled={logsQ.isFetching}><RefreshCw size={12} className={logsQ.isFetching ? "animate-spin" : ""} /> Refresh</Btn>
          </div>
          {logsQ.isLoading ? (
            <div className="py-8 text-center text-sm text-slate-500">Loading logs…</div>
          ) : logsQ.isError ? (
            <div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-300">{(logsQ.error as Error).message}</div>
          ) : logs.length === 0 ? (
            <EmptyState icon={FileText} title="No logs" message="Container produced no recent log lines." />
          ) : (
            <pre className="max-h-96 overflow-auto rounded-lg border border-white/[0.06] bg-[var(--canvas)] p-3 font-mono text-xs text-slate-400 whitespace-pre-wrap">{logs.join("\n")}</pre>
          )}
        </div>
      )}

      {section === "backups" && (
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <p className="text-[11px] text-slate-500">GET/POST /admin/database-services/:id/backups · restore via POST …/backups/:backupId/restore</p>
            <div className="flex gap-2">
              <Btn size="sm" tone="ghost" onClick={() => void backupsQ.refetch()} disabled={backupsQ.isFetching}><RefreshCw size={12} className={backupsQ.isFetching ? "animate-spin" : ""} /> Refresh</Btn>
              <Btn size="sm" onClick={() => createBackupMut.mutate()} disabled={createBackupMut.isPending}>{createBackupMut.isPending ? "Creating…" : "Create Backup"}</Btn>
            </div>
          </div>
          {backupsQ.isLoading ? (
            <div className="py-8 text-center text-sm text-slate-500">Loading backups…</div>
          ) : backupsQ.isError ? (
            <div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-300">{(backupsQ.error as Error).message}</div>
          ) : backups.length === 0 ? (
            <EmptyState icon={History} title="No backups" message="No backup history for this service yet." />
          ) : (
            <div className="overflow-x-auto rounded-lg border border-white/[0.06]">
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-wider text-slate-500">
                    <th className="px-3 py-2 font-medium">Backup</th><th className="px-3 py-2 font-medium">Status</th><th className="px-3 py-2 font-medium">Size</th><th className="px-3 py-2 font-medium">Created</th><th className="px-3 py-2 text-right font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {backups.map((b) => (
                    <tr key={b.id}>
                      <td className="px-3 py-2 font-mono text-[11px] text-slate-300" title={b.filePath || b.id}>{b.filePath ? b.filePath.split("/").pop() : b.id.slice(0, 8)}</td>
                      <td className="px-3 py-2"><Pill tone={b.status === "completed" ? "green" : b.status === "failed" ? "red" : b.status === "running" || b.status === "creating" ? "blue" : "neutral"}>{b.status}</Pill></td>
                      <td className="px-3 py-2 text-[11px] text-slate-400">{b.sizeBytes ? formatBytes(b.sizeBytes) : "—"}</td>
                      <td className="px-3 py-2 text-[11px] text-slate-500">{formatDate(b.createdAt)}</td>
                      <td className="px-3 py-2 text-right">
                        <Btn size="sm" tone="ghost" disabled={b.status !== "completed" || restoreMut.isPending} onClick={() => { void (async () => { if (await confirm({ title: `Restore backup ${b.id.slice(0, 8)}?`, description: `Current data in ${svc.name || svc.id} will be overwritten by this backup.`, danger: true, confirmLabel: "Restore" })) restoreMut.mutate(b.id); })(); }}>
                          <Power size={12} /> Restore
                        </Btn>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {section === "credentials" && (
        <div className="space-y-4">
          <div className="rounded-lg border border-white/[0.06] bg-white/[0.02] p-3">
            <p className="mb-2 text-xs font-semibold uppercase tracking-wider text-slate-400">Create credential — POST …/credentials</p>
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <Input label="Username" value={credUser} onChange={setCredUser} placeholder="app_user" />
              <Input label="Password" value={credPass} onChange={setCredPass} type="password" placeholder="••••••••" />
              <Input label="Database (optional grant)" value={credDb} onChange={setCredDb} placeholder={svc.databaseName || "db"} />
              <div>
                <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Permissions</label>
                <select className={selectCls} value={credPerms} onChange={(e) => setCredPerms(e.target.value)}>
                  <option value="read-write">read-write</option>
                  <option value="read-only">read-only</option>
                </select>
              </div>
            </div>
            <div className="mt-3 flex justify-end">
              <Btn size="sm" onClick={() => createCredMut.mutate({ username: credUser.trim(), password: credPass, database: credDb.trim() || undefined, permissions: credPerms })} disabled={createCredMut.isPending || !credUser.trim() || !credPass}>
                {createCredMut.isPending ? "Creating…" : "Create Credential"}
              </Btn>
            </div>
          </div>
          <div className="flex items-center justify-between">
            <p className="text-[11px] text-slate-500">GET /admin/database-services/:id/credentials — issued credentials (passwords stay server-side)</p>
            <Btn size="sm" tone="ghost" onClick={() => void credsQ.refetch()} disabled={credsQ.isFetching}><RefreshCw size={12} className={credsQ.isFetching ? "animate-spin" : ""} /> Refresh</Btn>
          </div>
          {credsQ.isLoading ? (
            <div className="py-8 text-center text-sm text-slate-500">Loading credentials…</div>
          ) : credsQ.isError ? (
            <div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-300">{(credsQ.error as Error).message}</div>
          ) : creds.length === 0 ? (
            <EmptyState icon={KeyRound} title="No credentials" message="No credentials issued for this service yet." />
          ) : (
            <div className="overflow-x-auto rounded-lg border border-white/[0.06]">
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-wider text-slate-500">
                    <th className="px-3 py-2 font-medium">Username</th><th className="px-3 py-2 font-medium">Database</th><th className="px-3 py-2 font-medium">Permissions</th><th className="px-3 py-2 font-medium">Created</th><th className="px-3 py-2 font-medium">State</th><th className="px-3 py-2 text-right font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {creds.map((cred) => (
                    <tr key={cred.id}>
                      <td className="px-3 py-2 font-mono text-[11px] text-slate-200">{cred.username}</td>
                      <td className="px-3 py-2 font-mono text-[11px] text-slate-400">{cred.databaseName || "—"}</td>
                      <td className="px-3 py-2"><Pill tone={cred.permissions === "read-only" ? "blue" : "yellow"}>{cred.permissions}</Pill></td>
                      <td className="px-3 py-2 text-[11px] text-slate-500">{formatDate(cred.createdAt)}</td>
                      <td className="px-3 py-2"><Pill tone={cred.revokedAt ? "neutral" : "green"}>{cred.revokedAt ? "revoked" : "active"}</Pill></td>
                      <td className="px-3 py-2 text-right">
                        {!cred.revokedAt && (
                          <Btn size="sm" tone="danger" disabled={revokeCredMut.isPending} onClick={() => { void (async () => { if (await confirm({ title: `Revoke ${cred.username}?`, description: "Apps relying on this credential will lose database access.", danger: true, confirmLabel: "Revoke" })) revokeCredMut.mutate(cred.id); })(); }}>
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
