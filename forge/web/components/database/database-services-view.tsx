"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Database, Plus, Trash2, RotateCcw, FlaskConical, Layers, Server, Search, List, LayoutGrid } from "lucide-react";
import {
  listDatabaseServices,
  provisionDatabaseService,
  deleteDatabaseService,
  restartDatabaseService,
  testConnection,
  listServiceTemplates,
  createServiceTemplate,
  type DatabaseService,
} from "@/lib/api/database-services";
import { Btn, EmptyState, Input, Modal, ModalFooter, SectionHeader, Pill, cn } from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";
import { statusTone } from "@/lib/api/status";
import { DbStatCards, type DbStat } from "./databases-overview";

const selectCls = "h-10 w-full rounded-lg border border-white/10 bg-surface-card-header px-3.5 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition hover:border-white/20 focus:border-[var(--brand)]/70 focus:ring-2 focus:ring-[var(--brand)]/15";

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
      <p className="text-[11px] text-slate-500">Wires <code className="font-mono">provisionDatabaseService</code>, <code className="font-mono">restartDatabaseService</code>, <code className="font-mono">testConnection</code></p>
      </div>

      {showProvision && <ProvisionModal onClose={() => setShowProvision(false)} onDone={() => { setShowProvision(false); qc.invalidateQueries({ queryKey: ["database-services"] }); }} />}
      {showTemplate && <TemplateModal onClose={() => setShowTemplate(false)} onDone={() => { setShowTemplate(false); qc.invalidateQueries({ queryKey: ["service-templates"] }); }} />}
      {showTest && <TestConnectionModal onClose={() => setShowTest(false)} />}
    </div>
  );
}

function ServiceStatusDot({ status }: { status: string }) {
  const tone = statusTone(status);
  const color = tone === "green" ? "bg-emerald-400" : tone === "yellow" ? "bg-amber-400" : tone === "red" ? "bg-red-400" : tone === "blue" ? "bg-sky-400" : "bg-slate-500";
  const text = tone === "green" ? "text-emerald-300" : tone === "yellow" ? "text-amber-300" : tone === "red" ? "text-red-300" : tone === "blue" ? "text-sky-300" : "text-slate-300";
  return (
    <span className={`inline-flex items-center gap-1.5 text-xs font-semibold capitalize ${text}`}>
      <span className={`h-1.5 w-1.5 rounded-full ${color}`} />{status}
    </span>
  );
}

function ServiceCard({ svc, onRestart, onDelete, restartPending, deletePending }: {
  svc: DatabaseService;
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
                <button key={t.id} type="button" onClick={() => { setType(t.type); setVersion(t.version); }} className={`rounded-full border px-3 py-1 text-xs ${type===t.type && version===t.version ? "border-[var(--brand)] bg-[var(--brand)]/20 text-white" : "border-white/10 bg-white/[0.04] text-slate-300 hover:bg-white/[0.06]"}`}>{t.type}:{t.version} → {t.dockerImage}</button>
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
