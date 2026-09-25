"use client";

import { useMemo, useState, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Box, Database, LayoutGrid, List, MoreVertical, Network, Search, Server, X } from "lucide-react";
import { fetchDatabaseHosts, type ApiDatabaseHost } from "@/lib/api";
import { listDBContainers, listManagedDatabases, type DBContainer, type ManagedDatabase } from "@/lib/api/database-containers";
import { listDatabaseServices, type DatabaseService } from "@/lib/api/database-services";
import { fetchCatalogEntries, fetchCatalogInstances } from "@/lib/api/catalog";
import { fetchJSON } from "@/lib/api/http";
import type { ApiDatabase, ApiServer, PaginationMeta } from "@forge/shared-types";
import { EmptyState, Modal, Pill, cn } from "@/components/admin/admin-ui";
import type { PillTone } from "@/components/admin/dashboard-cards";

export type DatabaseTab = "overview" | "hosts" | "containers" | "managed" | "services";
type Origin = "host" | "container" | "managed" | "service" | "catalog" | "serverdb";

const ORIGIN_META: Record<Origin, { label: string; tone: PillTone; tab: DatabaseTab | null }> = {
  host: { label: "External Host", tone: "neutral", tab: "hosts" },
  container: { label: "Raw Container", tone: "blue", tab: "containers" },
  managed: { label: "Managed DB", tone: "green", tab: "managed" },
  service: { label: "Service", tone: "yellow", tab: "services" },
  catalog: { label: "Catalog", tone: "blue", tab: null },
  serverdb: { label: "Server DB", tone: "yellow", tab: null },
};

interface OverviewRow {
  key: string;
  origin: Origin;
  name: string;
  sub: string;
  engine: string;
  version: string;
  status: string;
  tone: PillTone;
  hostPort: string;
  resources: string;
  node: string;
  tab: DatabaseTab | null;
  /** Absolute admin href when the detail lives outside the Databases tabs. */
  href?: string;
  /** Short cross-reference shown under the name (runtime ref, server, user). */
  ref?: string;
}

function engineLabel(engine?: string): string {
  const map: Record<string, string> = {
    postgresql: "PostgreSQL", mysql: "MySQL", mariadb: "MariaDB", redis: "Redis", mongodb: "MongoDB",
  };
  if (!engine) return "—";
  return map[engine.toLowerCase()] ?? engine;
}

function statusTone(status?: string): { label: string; tone: PillTone } {
  if (!status) return { label: "—", tone: "neutral" };
  const s = status.toLowerCase();
  if (["running", "ready", "completed", "healthy", "online"].includes(s)) return { label: status, tone: "green" };
  if (["pending", "pending_payment", "provisioning", "installing", "starting", "creating", "restarting"].includes(s)) return { label: status, tone: "yellow" };
  if (["failed", "error", "crashed"].includes(s)) return { label: status, tone: "red" };
  return { label: status, tone: "neutral" };
}

function fmtDate(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleDateString("en-GB", { day: "2-digit", month: "2-digit", year: "numeric" });
}

function hostRow(h: ApiDatabaseHost): OverviewRow {
  const st = { label: "—", tone: "neutral" as PillTone };
  return {
    key: `host:${h.id}`, origin: "host", name: h.name, sub: h.id.slice(0, 8),
    engine: engineLabel(h.engine), version: "—", status: st.label, tone: st.tone,
    hostPort: `${h.host}:${h.port}`,
    resources: h.databases != null ? `${h.databases} dbs` : "—",
    node: h.nodeName ?? "—", tab: "hosts",
  };
}

function containerRow(c: DBContainer): OverviewRow {
  const st = statusTone(c.status);
  return {
    key: `container:${c.id}`, origin: "container", name: c.id.slice(0, 8), sub: fmtDate(c.createdAt),
    engine: engineLabel(c.engine), version: c.version || "—", status: st.label, tone: st.tone,
    hostPort: "—", resources: `${c.memoryMb}MB / ${c.cpuShares} CPU`, node: "—", tab: "containers",
  };
}

function managedRow(m: ManagedDatabase): OverviewRow {
  const st = statusTone(m.status);
  return {
    key: `managed:${m.id}`, origin: "managed", name: m.name || m.id.slice(0, 8), sub: fmtDate(m.createdAt),
    engine: engineLabel(m.engine), version: m.version || "—", status: st.label, tone: st.tone,
    hostPort: m.host ? `${m.host}:${m.port}` : "—",
    resources: `${m.memoryMb}MB / ${m.cpuShares} CPU`, node: "—", tab: "managed",
  };
}

function serviceRow(s: DatabaseService): OverviewRow {
  const st = statusTone(s.status);
  return {
    key: `service:${s.id}`, origin: "service", name: s.name || s.id.slice(0, 8), sub: fmtDate(s.createdAt),
    engine: engineLabel(s.type), version: s.version || "—", status: st.label, tone: st.tone,
    hostPort: s.host ? `${s.host}:${s.port}` : "—",
    resources: `${s.memoryMb}MB / ${s.cpuShares} CPU`, node: "—", tab: "services",
  };
}

function catalogRow(entryKey: string, inst: {
  id: string; kind?: string; version?: string; status?: string; host?: string; port?: number;
  connString?: string; refType?: string; instanceRef?: string; createdAt?: string;
}): OverviewRow {
  const st = statusTone(inst.status);
  const label = entryKey || inst.kind || "service";
  return {
    key: `catalog:${inst.id}`, origin: "catalog", name: inst.version ? `${label} ${inst.version}` : label, sub: fmtDate(inst.createdAt),
    engine: engineLabel(inst.kind || entryKey), version: inst.version || "—", status: st.label, tone: st.tone,
    hostPort: inst.host ? `${inst.host}:${inst.port}` : (inst.port ? `:${inst.port}` : "—"),
    resources: inst.refType === "db_container" ? "container runtime" : "compose runtime",
    node: "—", tab: null, href: "/admin/catalog",
    ref: inst.refType && inst.instanceRef ? `ref ${inst.refType}:${inst.instanceRef.slice(0, 8)}` : undefined,
  };
}

function serverDbRow(serverName: string, db: ApiDatabase): OverviewRow {
  const state = db.provisioningState || "unknown";
  const st = statusTone(state);
  const serverId = db.serverId;
  return {
    key: `serverdb:${db.id}`, origin: "serverdb", name: db.database || db.name || db.id.slice(0, 8), sub: `srv ${serverName}`,
    engine: engineLabel(db.engine), version: "—", status: st.label, tone: st.tone,
    hostPort: db.host && typeof db.port === "number" ? `${db.host}:${db.port}` : "—",
    resources: typeof db.maxConnections === "number" ? `${db.maxConnections} max conn` : "shared host",
    node: "—", tab: null, href: `/server/${encodeURIComponent(serverId)}/databases`,
    ref: db.username ? `user ${db.username}` : undefined,
  };
}

/** All catalog instances across entries, plus container IDs they back (folded, not double-counted). */
async function fetchCatalogOverview(): Promise<{ rows: OverviewRow[]; backedIds: Set<string>; failed: number }> {
  const entries = await fetchCatalogEntries();
  const list = Array.isArray(entries) ? entries : [];
  const perEntry = await Promise.allSettled(list.map((e) => fetchCatalogInstances(e.key).then((rows) => ({ key: e.key, rows }))));
  const rows: OverviewRow[] = [];
  const backedIds = new Set<string>();
  let failed = 0;
  for (const r of perEntry) {
    if (r.status !== "fulfilled") {
      failed += 1;
      continue;
    }
    for (const inst of Array.isArray(r.value.rows) ? r.value.rows : []) {
      if (inst.refType === "db_container" && inst.instanceRef) backedIds.add(inst.instanceRef);
      rows.push(catalogRow(r.value.key, inst));
    }
  }
  return { rows, backedIds, failed };
}

type ServersPage = { data?: ApiServer[]; meta?: { pagination?: PaginationMeta } } | ApiServer[];

/** All servers, handling both envelope shapes (mirrors lib/api fetchAllServers). */
async function fetchAllServersLocal(): Promise<ApiServer[]> {
  const first = await fetchJSON<ServersPage>("/servers?page=1&per_page=100");
  const firstData = Array.isArray(first) ? first : (first.data ?? []);
  const totalPages = !Array.isArray(first) ? first.meta?.pagination?.total : undefined;
  if (typeof totalPages !== "number" || totalPages <= 1) return firstData;
  const rest = await Promise.all(
    Array.from({ length: totalPages - 1 }, (_, i) =>
      fetchJSON<ServersPage>(`/servers?page=${i + 2}&per_page=100`).then((p) => (Array.isArray(p) ? p : (p.data ?? [])))),
  );
  return [...firstData, ...rest.flat()];
}

/** Per-server databases across all servers. */
async function fetchServerDbOverview(): Promise<{ rows: OverviewRow[]; failed: number }> {
  const servers = await fetchAllServersLocal();
  const perServer = await Promise.allSettled(
    servers.map((s) => fetchJSON<ApiDatabase[]>(`/servers/${encodeURIComponent(s.id)}/databases`).then((rows) => ({ server: s, rows }))),
  );
  const rows: OverviewRow[] = [];
  let failed = 0;
  for (const r of perServer) {
    if (r.status !== "fulfilled") {
      failed += 1;
      continue;
    }
    for (const db of Array.isArray(r.value.rows) ? r.value.rows : []) rows.push(serverDbRow(r.value.server.name || r.value.server.id.slice(0, 8), db));
  }
  return { rows, failed };
}

function OriginIcon({ origin, className }: { origin: Origin; className?: string }) {
  if (origin === "container") return <Box size={18} className={className} />;
  if (origin === "service") return <Server size={18} className={className} />;
  return <Database size={18} className={className} />;
}

function StatusDot({ row }: { row: OverviewRow }) {
  if (row.status === "—") return <span className="text-slate-500">—</span>;
  const color = row.tone === "green" ? "bg-emerald-400" : row.tone === "yellow" ? "bg-amber-400" : row.tone === "red" ? "bg-red-400" : row.tone === "blue" ? "bg-sky-400" : "bg-slate-500";
  const text = row.tone === "green" ? "text-emerald-300" : row.tone === "yellow" ? "text-amber-300" : row.tone === "red" ? "text-red-300" : row.tone === "blue" ? "text-sky-300" : "text-slate-300";
  return (
    <span className={`inline-flex items-center gap-1.5 text-xs font-semibold capitalize ${text}`}>
      <span className={`h-1.5 w-1.5 rounded-full ${color}`} />{row.status}
    </span>
  );
}

function openLabel(row: OverviewRow): string {
  if (row.href === "/admin/catalog") return "Open in Catalog";
  if (row.href) return "Open server databases";
  return `Open in ${row.tab === "hosts" ? "Database Hosts" : row.tab === "containers" ? "DB Containers" : row.tab === "managed" ? "Managed DBs" : "Services"}`;
}

function RowMenu({ row, onOpen }: { row: OverviewRow; onOpen: () => void }) {
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  async function copyHostPort() {
    if (row.hostPort === "—") return;
    try {
      await navigator.clipboard.writeText(row.hostPort);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch { /* clipboard unavailable */ }
  }
  return (
    <div className="relative">
      <button
        type="button"
        aria-label={`Actions for ${row.name}`}
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        className="grid h-8 w-8 place-items-center rounded-lg border border-white/[0.08] text-slate-400 transition hover:border-white/20 hover:text-white"
      >
        <MoreVertical size={15} />
      </button>
      {open && (
        <>
          <button type="button" aria-label="Close menu" className="fixed inset-0 z-10 cursor-default" onClick={() => setOpen(false)} />
          <div className="absolute right-0 z-20 mt-1 w-48 overflow-hidden rounded-lg border border-white/10 bg-[var(--surface-raised)] shadow-xl">
            <button type="button" onClick={() => { setOpen(false); onOpen(); }} className="block w-full px-3 py-2 text-left text-xs text-slate-200 transition hover:bg-white/[0.06]">
              {openLabel(row)}
            </button>
            <button
              type="button"
              disabled={row.hostPort === "—"}
              onClick={() => { setOpen(false); void copyHostPort(); }}
              className="block w-full px-3 py-2 text-left text-xs text-slate-200 transition hover:bg-white/[0.06] disabled:opacity-40"
            >
              {copied ? "Copied!" : "Copy host:port"}
            </button>
          </div>
        </>
      )}
    </div>
  );
}

const KPI_DEFS: Array<{ key: string; label: string; icon: typeof Database; tile: string; count: (c: { hosts: number; containers: number; managed: number; services: number; catalog: number; serverdb: number; total: number }) => number }> = [
  { key: "total", label: "Total Databases", icon: Database, tile: "border-white/[0.08] bg-white/[0.03] text-slate-300", count: (c) => c.total },
  { key: "managed", label: "Managed DBs", icon: Box, tile: "border-sky-500/25 bg-sky-500/10 text-sky-300", count: (c) => c.managed },
  { key: "containers", label: "DB Containers", icon: Box, tile: "border-amber-500/25 bg-amber-500/10 text-amber-300", count: (c) => c.containers },
  { key: "services", label: "Services", icon: Network, tile: "border-emerald-500/25 bg-emerald-500/10 text-emerald-300", count: (c) => c.services },
  { key: "serverdb", label: "Server DBs", icon: Database, tile: "border-yellow-500/25 bg-yellow-500/10 text-yellow-300", count: (c) => c.serverdb },
  { key: "catalog", label: "Catalog", icon: LayoutGrid, tile: "border-sky-500/25 bg-sky-500/10 text-sky-300", count: (c) => c.catalog },
  { key: "hosts", label: "External Hosts", icon: Server, tile: "border-white/[0.08] bg-white/[0.03] text-slate-300", count: (c) => c.hosts },
];

export interface DbStat {
  key: string;
  label: string;
  icon: typeof Database;
  tile: string;
  value: ReactNode;
}

export function DbStatCards({ stats }: { stats: DbStat[] }) {
  return (
    <div className={cn("grid grid-cols-2 gap-3", stats.length >= 5 ? "xl:grid-cols-5" : "xl:grid-cols-4")}>
      {stats.map((s) => (
        <div key={s.key} className="flex items-center gap-3 rounded-xl border border-white/[0.08] bg-[var(--surface)] p-4 shadow-sm">
          <span className={cn("grid h-10 w-10 shrink-0 place-items-center rounded-lg border", s.tile)}>
            <s.icon size={18} />
          </span>
          <span className="min-w-0">
            <span className="block truncate text-[11px] text-slate-400">{s.label}</span>
            <span className="block font-mono text-2xl font-bold text-slate-100">{s.value}</span>
          </span>
        </div>
      ))}
    </div>
  );
}

export function DatabasesOverview({ onOpenTab }: { onOpenTab: (tab: DatabaseTab) => void }) {
  const router = useRouter();
  const hostsQ = useQuery({ queryKey: ["database-hosts"], queryFn: fetchDatabaseHosts });
  const containersQ = useQuery({ queryKey: ["db-containers"], queryFn: () => listDBContainers() });
  const managedQ = useQuery({ queryKey: ["managed-databases"], queryFn: () => listManagedDatabases() });
  const servicesQ = useQuery({ queryKey: ["database-services"], queryFn: listDatabaseServices });
  const catalogQ = useQuery({ queryKey: ["databases-overview-catalog"], queryFn: fetchCatalogOverview, retry: false, staleTime: 30_000 });
  const serverDbQ = useQuery({ queryKey: ["databases-overview-serverdb"], queryFn: fetchServerDbOverview, retry: false, staleTime: 30_000 });

  function openRow(row: OverviewRow) {
    if (row.href) router.push(row.href);
    else if (row.tab) onOpenTab(row.tab);
  }

  const [bannerDismissed, setBannerDismissed] = useState(false);
  const [search, setSearch] = useState("");
  const [originFilter, setOriginFilter] = useState<"all" | Origin>("all");
  const [engineFilter, setEngineFilter] = useState("all");
  const [statusFilter, setStatusFilter] = useState("all");
  const [view, setView] = useState<"table" | "cards">("table");
  const [sort, setSort] = useState("name-asc");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  const rows = useMemo<OverviewRow[]>(() => {
    // Containers backing a catalog instance are folded into the catalog row
    // (same underlying DB, user-facing object is the instance).
    const backedIds = catalogQ.data?.backedIds ?? new Set<string>();
    return [
      ...((Array.isArray(hostsQ.data) ? hostsQ.data : []).map(hostRow)),
      ...((containersQ.data ?? []).filter((c) => !backedIds.has(c.id)).map(containerRow)),
      ...((managedQ.data ?? []).map(managedRow)),
      ...((servicesQ.data ?? []).map(serviceRow)),
      ...((catalogQ.data?.rows ?? []).map((r) => (r.ref ? { ...r, sub: `${r.sub} · ${r.ref}` } : r))),
      ...((serverDbQ.data?.rows ?? []).map((r) => (r.ref ? { ...r, sub: `${r.sub} · ${r.ref}` } : r))),
    ];
  }, [hostsQ.data, containersQ.data, managedQ.data, servicesQ.data, catalogQ.data, serverDbQ.data]);

  const counts = {
    hosts: Array.isArray(hostsQ.data) ? hostsQ.data.length : 0,
    containers: rows.filter((r) => r.origin === "container").length,
    managed: managedQ.data?.length ?? 0,
    services: servicesQ.data?.length ?? 0,
    catalog: catalogQ.data?.rows.length ?? 0,
    serverdb: serverDbQ.data?.rows.length ?? 0,
    total: 0,
  };
  counts.total = counts.hosts + counts.containers + counts.managed + counts.services + counts.catalog + counts.serverdb;
  const loadingCounts = hostsQ.isLoading || containersQ.isLoading || managedQ.isLoading || servicesQ.isLoading || catalogQ.isLoading || serverDbQ.isLoading;
  const loadError = hostsQ.isError || containersQ.isError || managedQ.isError || servicesQ.isError || catalogQ.isError || serverDbQ.isError;

  const engines = useMemo(() => [...new Set(rows.map((r) => r.engine).filter((e) => e !== "—"))].sort(), [rows]);
  const statuses = useMemo(() => [...new Set(rows.map((r) => r.status).filter((s) => s !== "—"))].sort(), [rows]);

  function resetPage(update: () => void) {
    setPage(1);
    update();
  }

  const filtered = useMemo(() => {
    const term = search.trim().toLowerCase();
    return rows.filter((r) => {
      if (originFilter !== "all" && r.origin !== originFilter) return false;
      if (engineFilter !== "all" && r.engine !== engineFilter) return false;
      if (statusFilter !== "all" && r.status !== statusFilter) return false;
      if (!term) return true;
      return `${r.name} ${r.sub} ${r.engine} ${r.version} ${r.status} ${r.hostPort}`.toLowerCase().includes(term);
    });
  }, [rows, search, originFilter, engineFilter, statusFilter]);

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
  const hasActiveFilters = Boolean(search.trim() || originFilter !== "all" || engineFilter !== "all" || statusFilter !== "all");

  const selectCls = "h-10 cursor-pointer appearance-none rounded-lg border border-white/[0.08] bg-black/20 pl-3 pr-8 text-xs text-slate-200 outline-none";
  const selectWrap = "relative flex flex-col justify-center rounded-lg border border-white/[0.08] bg-black/20 px-3 py-1";

  return (
    <div className="space-y-4">
      <DbStatCards stats={KPI_DEFS.map((kpi) => ({
        key: kpi.key, label: kpi.label, icon: kpi.icon, tile: kpi.tile,
        value: loadingCounts ? "…" : kpi.count(counts),
      }))} />

      {!bannerDismissed && (
        <div className="flex items-start gap-3 rounded-xl border border-white/[0.08] bg-white/[0.015] p-4">
          <span className="grid h-6 w-6 shrink-0 place-items-center rounded-full bg-sky-500/15 text-xs font-bold text-sky-300">i</span>
          <div className="min-w-0 flex-1 text-xs leading-5 text-slate-400">
            <span className="mr-3 font-bold text-slate-100">Where is my DB?</span>
            Every database appears here with an origin badge. <span className="font-semibold text-slate-200">Managed DB</span> — default one-click local DB.{" "}
            <span className="font-semibold text-slate-200">DB Container</span> — database running in a container on a node.{" "}
            <span className="font-semibold text-slate-200">External host</span> — shared host, no container.{" "}
            <span className="font-semibold text-slate-200">Service</span> — linkable database service instance.{" "}
            <span className="font-semibold text-slate-200">Server DB</span> — per-server database carved from a host.{" "}
            <span className="font-semibold text-slate-200">Catalog</span> — one-click service instance (SQL kinds reuse the container runtime, folded into one row).
          </div>
          <button type="button" aria-label="Dismiss" onClick={() => setBannerDismissed(true)} className="rounded p-1 text-slate-500 transition hover:text-white">
            <X size={14} />
          </button>
        </div>
      )}

      <div className="flex flex-col gap-2 xl:flex-row xl:items-center">
        <label className="flex min-w-52 flex-1 items-center gap-2 rounded-lg border border-white/[0.08] bg-black/20 px-2.5 py-2">
          <Search size={13} className="shrink-0 text-slate-500" />
          <input
            type="text"
            value={search}
            onChange={(e) => resetPage(() => setSearch(e.target.value))}
            placeholder="Search databases by name, engine, status…"
            aria-label="Search databases"
            className="w-full bg-transparent text-xs text-slate-200 outline-none placeholder:text-slate-600"
          />
        </label>
        <div className="flex flex-wrap items-center gap-2">
          <label className={selectWrap}>
            <span className="text-[10px] leading-3 text-slate-500">Origin</span>
            <select aria-label="Filter by origin" value={originFilter} onChange={(e) => resetPage(() => setOriginFilter(e.target.value as "all" | Origin))} className={selectCls + " h-6 border-0 bg-transparent pl-0 text-xs"}>
              <option value="all">All</option>
              {(Object.keys(ORIGIN_META) as Origin[]).map((o) => <option key={o} value={o}>{ORIGIN_META[o].label}</option>)}
            </select>
          </label>
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
            <button type="button" onClick={() => { setSearch(""); setOriginFilter("all"); setEngineFilter("all"); setStatusFilter("all"); setPage(1); }} className="rounded-lg px-2.5 py-2 text-xs font-semibold text-slate-400 transition hover:text-white">
              Clear
            </button>
          )}
        </div>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-sm font-bold text-slate-100">Databases ({sorted.length})</h2>
        <label className="flex items-center gap-2 rounded-lg border border-white/[0.08] bg-black/20 px-2.5 py-1.5 text-xs text-slate-300">
          <span className="text-[11px] text-slate-500">Sort by</span>
          <select aria-label="Sort databases" value={sort} onChange={(e) => setSort(e.target.value)} className="cursor-pointer appearance-none bg-transparent pr-1 outline-none">
            <option value="name-asc">Name (A → Z)</option>
            <option value="name-desc">Name (Z → A)</option>
            <option value="status">Status</option>
            <option value="engine">Engine</option>
          </select>
        </label>
      </div>

      {loadError && rows.length === 0 ? (
        <div className="rounded-xl border border-red-500/20 bg-red-950/10 p-4 text-sm text-red-200">Could not load database inventory. One or more sources are unreachable.</div>
      ) : sorted.length === 0 ? (
        <EmptyState
          icon={Database}
          title={hasActiveFilters ? "No matches" : "No databases yet"}
          message={hasActiveFilters ? "No databases match these filters." : "Create your first database to get started."}
        />
      ) : view === "cards" ? (
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {visible.map((r) => (
            <div key={r.key} className="rounded-xl border border-white/[0.08] bg-[var(--surface)] p-4 shadow-sm transition hover:border-white/20">
              <div className="flex items-start gap-3">
                <span className="grid h-10 w-10 shrink-0 place-items-center rounded-xl border border-white/[0.08] bg-white/[0.03] text-slate-300">
                  <OriginIcon origin={r.origin} />
                </span>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-bold text-slate-100" title={r.name}>{r.name}</p>
                  <p className="font-mono text-[10px] text-slate-500">{r.sub}</p>
                </div>
                <RowMenu row={r} onOpen={() => openRow(r)} />
              </div>
              <div className="mt-3 flex flex-wrap items-center gap-2">
                <Pill tone={ORIGIN_META[r.origin].tone}>{ORIGIN_META[r.origin].label}</Pill>
                <StatusDot row={r} />
              </div>
              <div className="mt-3 space-y-1 border-t border-white/[0.06] pt-3 font-mono text-[11px] text-slate-400">
                <p className="truncate">{r.engine}{r.version !== "—" ? ` ${r.version}` : ""} · {r.hostPort}</p>
                <p className="truncate">{r.resources}</p>
              </div>
              {r.href ? (
                <a href={r.href} className="mt-3 block w-full rounded-lg border border-white/[0.08] px-3 py-2 text-center text-xs font-bold text-slate-200 transition hover:border-white/20 hover:text-white">
                  Open
                </a>
              ) : (
                <button type="button" onClick={() => openRow(r)} className="mt-3 w-full rounded-lg border border-white/[0.08] px-3 py-2 text-xs font-bold text-slate-200 transition hover:border-white/20 hover:text-white">
                  Open
                </button>
              )}
            </div>
          ))}
        </div>
      ) : (
        <div className="overflow-hidden rounded-xl border border-white/[0.08] bg-[var(--surface)] shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-wider text-slate-500">
                  <th className="px-4 py-3 font-medium">Name</th>
                  <th className="px-2 py-3 font-medium">Origin</th>
                  <th className="px-2 py-3 font-medium">Engine / Version</th>
                  <th className="px-2 py-3 font-medium">Status</th>
                  <th className="px-2 py-3 font-medium">Host : Port</th>
                  <th className="px-2 py-3 font-medium">Resources</th>
                  <th className="px-2 py-3 font-medium">Node</th>
                  <th className="px-2 py-3 text-right font-medium">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {visible.map((r) => (
                  <tr key={r.key} className="transition hover:bg-white/[0.02]">
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2.5">
                        <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg border border-white/[0.08] bg-white/[0.03] text-slate-400">
                          <OriginIcon origin={r.origin} className="h-4 w-4" />
                        </span>
                        <span className="min-w-0">
                          <span className="block max-w-44 truncate text-xs font-bold text-slate-100" title={r.name}>{r.name}</span>
                          <span className="block font-mono text-[10px] text-slate-500">{r.sub}</span>
                        </span>
                      </div>
                    </td>
                    <td className="px-2 py-3"><Pill tone={ORIGIN_META[r.origin].tone}>{ORIGIN_META[r.origin].label}</Pill></td>
                    <td className="px-2 py-3">
                      <span className="block text-xs text-slate-200">{r.engine}</span>
                      <span className="block font-mono text-[10px] text-slate-500">{r.version}</span>
                    </td>
                    <td className="px-2 py-3"><StatusDot row={r} /></td>
                    <td className="px-2 py-3 font-mono text-[11px] text-slate-300">{r.hostPort}</td>
                    <td className="px-2 py-3 text-[11px] text-slate-400">{r.resources}</td>
                    <td className="px-2 py-3 text-[11px] text-slate-400">{r.node}</td>
                    <td className="px-2 py-3">
                      <div className="flex items-center justify-end gap-1.5">
                        {r.href ? (
                          <a href={r.href} className="rounded-lg border border-white/[0.08] px-3 py-1.5 text-[11px] font-bold text-slate-200 transition hover:border-white/20 hover:text-white">
                            Open
                          </a>
                        ) : (
                          <button type="button" onClick={() => openRow(r)} className="rounded-lg border border-white/[0.08] px-3 py-1.5 text-[11px] font-bold text-slate-200 transition hover:border-white/20 hover:text-white">
                            Open
                          </button>
                        )}
                        <RowMenu row={r} onOpen={() => openRow(r)} />
                      </div>
                    </td>
                  </tr>
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
  );
}

const CREATE_PATHS: Array<{ key: string; title: string; body: string; tab?: DatabaseTab; href?: string }> = [
  { key: "managed", title: "Managed Database", body: "Default one-click local DB with backups and restore.", tab: "managed" },
  { key: "services", title: "Database Service", body: "Provision a linkable service instance (PostgreSQL, MySQL, Redis…).", tab: "services" },
  { key: "serverdb", title: "Server Database", body: "Per-game-server MySQL/PG carved from a host — create on the server's Databases tab.", href: "/admin/servers" },
  { key: "catalog", title: "Catalog Service", body: "One-click postgres/mysql/redis/mongo, queues and caches.", href: "/admin/catalog" },
  { key: "containers", title: "DB Container", body: "Low-level container runtime for advanced setups.", tab: "containers" },
  { key: "hosts", title: "External Host", body: "Connect a shared external database host.", tab: "hosts" },
];

export function CreateDatabaseModal({ onClose, onSelect }: { onClose: () => void; onSelect: (tab: DatabaseTab) => void }) {
  const router = useRouter();
  function choose(p: (typeof CREATE_PATHS)[number]) {
    if (p.href) router.push(p.href);
    else if (p.tab) onSelect(p.tab);
    onClose();
  }
  return (
    <Modal title="Create Database" description="Pick one path — each opens where it is managed." onClose={onClose}>
      <div className="space-y-2">
        {CREATE_PATHS.map((p) => (
          <button
            key={p.key}
            type="button"
            onClick={() => choose(p)}
            className="flex w-full items-center justify-between gap-3 rounded-xl border border-white/[0.07] bg-white/[0.02] p-3.5 text-left transition hover:border-white/20 hover:bg-white/[0.05]"
          >
            <span className="min-w-0">
              <span className="block text-sm font-bold text-slate-100">{p.title}</span>
              <span className="mt-0.5 block text-xs text-slate-500">{p.body}</span>
            </span>
            <span className="shrink-0 text-slate-500">→</span>
          </button>
        ))}
      </div>
    </Modal>
  );
}
