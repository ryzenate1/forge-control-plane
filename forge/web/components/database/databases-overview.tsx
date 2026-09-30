"use client";

import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Box, Database, DatabaseBackup, HardDrive, LayoutGrid, List, MoreVertical, Network, Search, Server, X } from "lucide-react";
import { fetchDatabaseHosts, type ApiDatabaseHost } from "@/lib/api";
import { listDBContainers, listManagedDatabases, type DBContainer, type ManagedDatabase } from "@/lib/api/database-containers";
import { listDatabaseServices, type DatabaseService } from "@/lib/api/database-services";
import { fetchCatalogEntries, fetchCatalogInstances } from "@/lib/api/catalog";
import { fetchJSON } from "@/lib/api/http";
import type { ApiDatabase, ApiServer, PaginationMeta } from "@forge/shared-types";
import {
  AdminErrorState,
  AdminLoadingState,
  Btn,
  EmptyState,
  Modal,
  Pill,
  cn,
  type AdminTone,
} from "@/components/admin/admin-ui";
import { Pagination } from "@/components/ui/primitives";
import { resolveTone, toneStyles } from "@/components/ui/forge/status";
import { formatDate } from "@/lib/utils";
import { isAvailable } from "@/lib/admin/telemetry";

export type DatabaseTab = "overview" | "hosts" | "containers" | "managed" | "services";
type Origin = "host" | "container" | "managed" | "service" | "catalog" | "serverdb";

/** Dismissing the origin explainer used to be plain `useState`, so it returned on
 *  every navigation and the operator could never get rid of it for good. */
const BANNER_DISMISSED_KEY = "forge.databases.overview-origin-guide-dismissed";

const ORIGIN_META: Record<Origin, { label: string; tone: AdminTone; tab: DatabaseTab | null }> = {
  host: { label: "External Host", tone: "neutral", tab: "hosts" },
  container: { label: "Raw Container", tone: "info", tab: "containers" },
  managed: { label: "Managed DB", tone: "ok", tab: "managed" },
  service: { label: "Service", tone: "pending", tab: "services" },
  catalog: { label: "Catalog", tone: "info", tab: null },
  serverdb: { label: "Server DB", tone: "pending", tab: null },
};

interface OverviewRow {
  key: string;
  origin: Origin;
  name: string;
  sub: string;
  engine: string;
  version: string;
  /** Raw status word as the source reported it, or "" when it reported nothing.
   *  Colour is derived from it by {@link resolveTone}, never stored alongside it. */
  status: string;
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

/**
 * Status wording for a row.
 *
 * This used to be a local `statusTone()` with its own word list returning
 * `PillTone` from the legacy frame, so the same slice held two rival status
 * vocabularies that disagreed about what "we don't know" means. The word list is
 * gone: the raw status is rendered as reported and {@link resolveTone} decides
 * the colour in one place. An unrecognised or absent status resolves to
 * `unknown` (dashed grey), never to `ok` and never to `neutral`.
 */
function statusLabel(status?: string): string {
  if (!status) return "—";
  const s = status.toLowerCase();
  if (["unknown", "none", "null"].includes(s)) return "Not reported";
  return status;
}

function fmtDate(iso?: string): string {
  // Shared formatter: page-local date formats were the third one in this slice.
  return formatDate(iso, "—");
}

function hostRow(h: ApiDatabaseHost): OverviewRow {
  return {
    key: `host:${h.id}`, origin: "host", name: h.name, sub: h.id.slice(0, 8),
    engine: engineLabel(h.engine), version: "—", status: "",
    hostPort: `${h.host}:${h.port}`,
    resources: h.databases != null ? `${h.databases} dbs` : "—",
    node: h.nodeName ?? "—", tab: "hosts",
  };
}

function containerRow(c: DBContainer): OverviewRow {
  return {
    key: `container:${c.id}`, origin: "container", name: c.id.slice(0, 8), sub: fmtDate(c.createdAt),
    engine: engineLabel(c.engine), version: c.version || "—", status: statusLabel(c.status),
    hostPort: "—", resources: resourcesLabel(c.memoryMb, c.cpuShares), node: "—", tab: "containers",
  };
}

function managedRow(m: ManagedDatabase): OverviewRow {
  return {
    key: `managed:${m.id}`, origin: "managed", name: m.name || m.id.slice(0, 8), sub: fmtDate(m.createdAt),
    engine: engineLabel(m.engine), version: m.version || "—", status: statusLabel(m.status),
    hostPort: m.host ? `${m.host}:${m.port}` : "—",
    resources: resourcesLabel(m.memoryMb, m.cpuShares), node: "—", tab: "managed",
  };
}

function serviceRow(s: DatabaseService): OverviewRow {
  return {
    key: `service:${s.id}`, origin: "service", name: s.name || s.id.slice(0, 8), sub: fmtDate(s.createdAt),
    engine: engineLabel(s.type), version: s.version || "—", status: statusLabel(s.status),
    hostPort: s.host ? `${s.host}:${s.port}` : "—",
    resources: resourcesLabel(s.memoryMb, s.cpuShares), node: "—", tab: "services",
  };
}

/**
 * Memory/CPU as reported. A missing figure is not `0MB` and not `0 CPU` — it was
 * never measured, so it renders as unknown rather than as a configured zero.
 */
export function resourcesLabel(memoryMb?: number, cpuShares?: number): string {
  const hasMemory = typeof memoryMb === "number" && Number.isFinite(memoryMb);
  const hasCpu = typeof cpuShares === "number" && Number.isFinite(cpuShares);
  if (!hasMemory && !hasCpu) return "Resources not reported";
  const memory = hasMemory ? `${memoryMb}MB` : "memory not reported";
  const cpu = hasCpu ? `${cpuShares} CPU` : "CPU not reported";
  return `${memory} / ${cpu}`;
}

function catalogRow(entryKey: string, inst: {
  id: string; kind?: string; version?: string; status?: string; host?: string; port?: number;
  connString?: string; refType?: string; instanceRef?: string; createdAt?: string;
}): OverviewRow {
  const label = entryKey || inst.kind || "service";
  return {
    key: `catalog:${inst.id}`, origin: "catalog", name: inst.version ? `${label} ${inst.version}` : label, sub: fmtDate(inst.createdAt),
    engine: engineLabel(inst.kind || entryKey), version: inst.version || "—", status: statusLabel(inst.status),
    hostPort: inst.host ? `${inst.host}:${inst.port}` : (inst.port ? `:${inst.port}` : "—"),
    resources: inst.refType === "db_container" ? "container runtime" : "compose runtime",
    node: "—", tab: null, href: "/admin/catalog",
    ref: inst.refType && inst.instanceRef ? `ref ${inst.refType}:${inst.instanceRef.slice(0, 8)}` : undefined,
  };
}

function serverDbRow(serverName: string, db: ApiDatabase): OverviewRow {
  const state = db.provisioningState || "";
  const serverId = db.serverId;
  return {
    key: `serverdb:${db.id}`, origin: "serverdb", name: db.database || db.name || db.id.slice(0, 8), sub: `srv ${serverName}`,
    engine: engineLabel(db.engine), version: "—", status: statusLabel(state),
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
const SERVERS_OVERVIEW_MAX_PAGES = 10;
const SERVER_DB_FANOUT_CAP = 50;

async function fetchAllServersLocal(): Promise<ApiServer[]> {
  const first = await fetchJSON<ServersPage>("/servers?page=1&per_page=100");
  const firstData = Array.isArray(first) ? first : (first.data ?? []);
  const totalPages = !Array.isArray(first) ? first.meta?.pagination?.total : undefined;
  if (typeof totalPages !== "number" || totalPages <= 1) return firstData.slice(0, SERVER_DB_FANOUT_CAP);
  const pages = Math.min(totalPages, SERVERS_OVERVIEW_MAX_PAGES);
  const rest = await Promise.all(
    Array.from({ length: pages - 1 }, (_, i) =>
      fetchJSON<ServersPage>(`/servers?page=${i + 2}&per_page=100`).then((p) => (Array.isArray(p) ? p : (p.data ?? [])))),
  );
  return [...firstData, ...rest.flat()].slice(0, SERVER_DB_FANOUT_CAP);
}

/** Per-server databases across all servers. */
async function fetchServerDbOverview(): Promise<{ rows: OverviewRow[]; failed: number; failedIds: string[] }> {
  const servers = await fetchAllServersLocal();
  const perServer = await Promise.allSettled(
    servers.map((s) => fetchJSON<ApiDatabase[]>(`/servers/${encodeURIComponent(s.id)}/databases`).then((rows) => ({ server: s, rows }))),
  );
  const rows: OverviewRow[] = [];
  let failed = 0;
  const failedIds: string[] = [];
  perServer.forEach((r, index) => {
    if (r.status !== "fulfilled") {
      failed += 1;
      failedIds.push(servers[index]?.id ?? `index-${index}`);
      return;
    }
    for (const db of Array.isArray(r.value.rows) ? r.value.rows : []) rows.push(serverDbRow(r.value.server.name || r.value.server.id.slice(0, 8), db));
  });
  return { rows, failed, failedIds };
}

function OriginIcon({ origin, className }: { origin: Origin; className?: string }) {
  if (origin === "container") return <Box aria-hidden="true" className={className} size={18} />;
  if (origin === "service") return <Server aria-hidden="true" className={className} size={18} />;
  return <Database aria-hidden="true" className={className} size={18} />;
}

/**
 * Status as text + dot. The colour comes from {@link resolveTone} over the raw
 * status word, never from a class chosen here, so a status this page cannot
 * interpret reads as the dashed `unknown` treatment instead of a plausible green.
 */
function StatusDot({ row }: { row: OverviewRow }) {
  if (!row.status || row.status === "—") {
    return <span className="text-text-muted" title="No status reported">—</span>;
  }
  const tone = resolveTone(row.status);
  const styles = toneStyles[tone];
  return (
    <span className={cn("inline-flex items-center gap-1.5 text-xs font-semibold capitalize", styles.fg, tone === "unknown" && "border-b border-dashed border-unknown-line")}>
      <span aria-hidden="true" className={cn("h-1.5 w-1.5 rounded-full", styles.dot)} />
      {row.status}
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
        aria-expanded={open}
        aria-haspopup="menu"
        aria-label={`Actions for ${row.name}`}
        className="grid h-8 w-8 place-items-center rounded-lg border border-line text-text-subtle transition hover:border-line-strong hover:bg-overlay-subtle hover:text-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
        onClick={() => setOpen((v) => !v)}
        onKeyDown={(event) => { if (event.key === "Escape") setOpen(false); }}
        type="button"
      >
        <MoreVertical aria-hidden="true" size={15} />
      </button>
      {open && (
        <>
          <button aria-label="Close menu" className="fixed inset-0 z-10 cursor-default" onClick={() => setOpen(false)} type="button" />
          <div className="absolute right-0 z-20 mt-1 w-48 overflow-hidden rounded-lg border border-line-strong bg-overlay-strong shadow-xl" role="menu">
            <button
              className="block w-full px-3 py-2 text-left text-xs text-text transition hover:bg-overlay-subtle focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[var(--focus)]"
              onClick={() => { setOpen(false); onOpen(); }}
              role="menuitem"
              type="button"
            >
              {openLabel(row)}
            </button>
            <button
              className="block w-full px-3 py-2 text-left text-xs text-text transition hover:bg-overlay-subtle disabled:cursor-not-allowed disabled:text-text-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[var(--focus)]"
              disabled={row.hostPort === "—"}
              onClick={() => { setOpen(false); void copyHostPort(); }}
              role="menuitem"
              title={row.hostPort === "—" ? "No host or port was reported for this database" : undefined}
              type="button"
            >
              {copied ? "Copied" : "Copy host:port"}
            </button>
          </div>
        </>
      )}
    </div>
  );
}

/**
 * Inventory tiles.
 *
 * `count` returns `number | undefined`: `undefined` means that source has not
 * reported (still loading, or it failed), and the tile renders `—`. The previous
 * version built these counts with `?? 0`, so a source that errored contributed a
 * confident zero and the "Total Databases" tile silently under-counted. A missing
 * reading is not a measured zero.
 *
 * Tile decoration is one neutral token treatment rather than seven hand-picked
 * palette classes, and each tile has its own glyph: `Box` previously labelled both
 * "Managed DBs" and "DB Containers", and `Database` both "Total Databases" and
 * "Server DBs".
 */
type SourceCounts = Record<"hosts" | "containers" | "managed" | "services" | "catalog" | "serverdb", number | undefined>;

const KPI_DEFS: Array<{ key: string; label: string; icon: typeof Database; hint: string; count: (c: SourceCounts & { total?: number | undefined }) => number | undefined }> = [
  { key: "total", label: "Total Databases", icon: Database, hint: "Every source that reported; partial while any source is unreachable", count: (c) => c.total },
  { key: "managed", label: "Managed DBs", icon: DatabaseBackup, hint: "Managed databases created in this panel", count: (c) => c.managed },
  { key: "containers", label: "DB Containers", icon: Box, hint: "Database containers running on a node", count: (c) => c.containers },
  { key: "services", label: "Services", icon: Network, hint: "Linkable database service instances", count: (c) => c.services },
  { key: "serverdb", label: "Server DBs", icon: HardDrive, hint: "Per-game-server databases carved from a host", count: (c) => c.serverdb },
  { key: "catalog", label: "Catalog", icon: LayoutGrid, hint: "Instances provisioned from the service catalog", count: (c) => c.catalog },
  { key: "hosts", label: "External Hosts", icon: Server, hint: "External MySQL and PostgreSQL connections", count: (c) => c.hosts },
];

export interface DbStat {
  key: string;
  label: string;
  icon: typeof Database;
  hint: string;
  value: ReactNode;
}

export function DbStatCards({ stats }: { stats: DbStat[] }) {
  return (
    <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
      {stats.map((s) => (
        <div key={s.key} className="flex items-center gap-3 rounded-xl border border-line bg-overlay-subtle p-4">
          <span className="grid h-10 w-10 shrink-0 place-items-center rounded-lg border border-line bg-overlay text-text-subtle">
            <s.icon aria-hidden="true" size={18} />
          </span>
          <span className="min-w-0">
            <span className="block truncate text-xs text-text-subtle" title={s.hint}>{s.label}</span>
            <span className="block font-mono text-2xl font-bold text-text">{s.value}</span>
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
  useEffect(() => {
    try {
      if (window.localStorage.getItem(BANNER_DISMISSED_KEY) === "1") setBannerDismissed(true);
    } catch { /* storage unavailable (private mode) — keep the banner visible */ }
  }, []);
  const dismissBanner = useCallback(() => {
    setBannerDismissed(true);
    try { window.localStorage.setItem(BANNER_DISMISSED_KEY, "1"); } catch { /* not persistable */ }
  }, []);
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

  /**
   * Per-source counts, with *unavailable* kept distinct from *zero*.
   *
   * This block used to be `hostsQ.data … : 0` / `managedQ.data?.length ?? 0`, and
   * only an all-sources failure surfaced as an error — so a partial failure (for
   * example `catalogQ`, which runs with `retry: false`) rendered "Catalog 0" and a
   * silently under-counted "Total Databases" as measured facts. A source that has
   * not reported, or that failed, now yields `undefined`, renders `—`, and makes
   * the total read as partial.
   */
  const sourceAvailable = {
    hosts: isAvailable(hostsQ) && Array.isArray(hostsQ.data),
    containers: isAvailable(containersQ),
    managed: isAvailable(managedQ),
    services: isAvailable(servicesQ),
    catalog: isAvailable(catalogQ),
    serverdb: isAvailable(serverDbQ),
  } as const;

  const counts = useMemo<SourceCounts>(() => ({
    hosts: sourceAvailable.hosts ? (hostsQ.data as ApiDatabaseHost[]).length : undefined,
    containers: sourceAvailable.containers
      ? rows.filter((r) => r.origin === "container").length
      : undefined,
    managed: sourceAvailable.managed ? (managedQ.data?.length ?? undefined) : undefined,
    services: sourceAvailable.services ? (servicesQ.data?.length ?? undefined) : undefined,
    catalog: sourceAvailable.catalog ? (catalogQ.data?.rows.length ?? undefined) : undefined,
    serverdb: sourceAvailable.serverdb ? (serverDbQ.data?.rows.length ?? undefined) : undefined,
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }), [rows, hostsQ.data, containersQ.data, managedQ.data, servicesQ.data, catalogQ.data, serverDbQ.data]);

  const reportedSources = Object.values(sourceAvailable).filter(Boolean).length;
  const allSourcesReported = reportedSources === Object.keys(sourceAvailable).length;
  const total: number | undefined = allSourcesReported
    ? (counts.hosts ?? 0) + (counts.containers ?? 0) + (counts.managed ?? 0) +
      (counts.services ?? 0) + (counts.catalog ?? 0) + (counts.serverdb ?? 0)
    : undefined;

  const loadingCounts = hostsQ.isPending || containersQ.isPending || managedQ.isPending || servicesQ.isPending || catalogQ.isPending || serverDbQ.isPending;
  const loadError = hostsQ.isError || containersQ.isError || managedQ.isError || servicesQ.isError || catalogQ.isError || serverDbQ.isError;
  const unavailableSources = (Object.keys(sourceAvailable) as Array<keyof typeof sourceAvailable>)
    .filter((k) => !sourceAvailable[k])
    .map((k) => KPI_DEFS.find((d) => d.key === k)?.label ?? k);

  const engines = useMemo(() => [...new Set(rows.map((r) => r.engine).filter((e) => e !== "—"))].sort(), [rows]);
  const statuses = useMemo(() => [...new Set(rows.map((r) => r.status).filter((s) => s && s !== "—"))].sort(), [rows]);

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

  /**
   * Nothing had rendered while these six sources were in flight — the toolbar and
   * an empty table appeared over an un-populated `rows` array. Loading, then
   * error, then empty, in that precedence: a source that failed must never be able
   * to fall through to "No databases yet".
   */
  if (loadingCounts && rows.length === 0) {
    return <AdminLoadingState label="Loading database inventory from all sources…" />;
  }

  if (loadError && rows.length === 0) {
    return (
      <AdminErrorState
        message={`Could not load the database inventory. ${unavailableSources.length ? `Unreachable or unreported: ${unavailableSources.join(", ")}.` : "One or more sources are unreachable."}`}
        retry={() => {
          void hostsQ.refetch(); void containersQ.refetch(); void managedQ.refetch();
          void servicesQ.refetch(); void catalogQ.refetch(); void serverDbQ.refetch();
        }}
      />
    );
  }

  const selectCls = "ui-input w-full cursor-pointer";

  /** Total as read, with the filtered denominator named — the Servers grammar. */
  const totalsLabel = hasActiveFilters
    ? `${sorted.length} of ${rows.length} databases (filtered)`
    : `${rows.length} database${rows.length === 1 ? "" : "s"}`;

  return (
    <div className="space-y-4">
      <DbStatCards stats={KPI_DEFS.map((kpi) => {
        const value = kpi.count({ ...counts, total });
        return {
          key: kpi.key,
          label: kpi.label,
          icon: kpi.icon,
          hint: kpi.hint,
          // `undefined` means the source has not reported — render `—`, never a
          // zero, and never the ellipsis over a source that already answered.
          value: value === undefined ? (
            <span className="text-text-muted" title={kpi.key === "total" && !allSourcesReported ? `Not every source reported; ${unavailableSources.join(", ") || "one source"} is unavailable` : `Not reported by ${kpi.label}`}>—</span>
          ) : (
            <>
              {value.toLocaleString()}
              {kpi.key === "total" && !allSourcesReported ? (
                <span className="ml-1 text-xs font-normal text-warn" title={`Partial: ${unavailableSources.join(", ")} did not report`}>partial</span>
              ) : null}
            </>
          ),
        };
      })} />

      {!bannerDismissed && (
        <div className="flex items-start gap-3 rounded-xl border border-line bg-overlay-subtle p-4">
          <span aria-hidden="true" className="grid h-6 w-6 shrink-0 place-items-center rounded-full border border-line bg-overlay font-mono text-xs font-bold text-text-subtle">i</span>
          <div className="min-w-0 flex-1 text-xs leading-5 text-text-subtle">
            <span className="mr-3 font-bold text-text">Where is my DB?</span>
            Every database appears here with an origin badge.{" "}
            <strong className="font-semibold text-text">Managed DB</strong> — a database created and managed in this panel.{" "}
            <strong className="font-semibold text-text">DB Container</strong> — a database running in a container on a node.{" "}
            <strong className="font-semibold text-text">External Host</strong> — a shared host this panel connects to but does not run.{" "}
            <strong className="font-semibold text-text">Service</strong> — a linkable database service instance.{" "}
            <strong className="font-semibold text-text">Server DB</strong> — a per-game-server database carved from a host.{" "}
            <strong className="font-semibold text-text">Catalog</strong> — a provisioned catalog service; SQL kinds reuse the container runtime and are folded into one row.
          </div>
          <button aria-label="Dismiss the origin explainer" className="rounded p-1 text-text-muted transition hover:text-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" onClick={dismissBanner} type="button">
            <X aria-hidden="true" size={14} />
          </button>
        </div>
      )}

      <div className="flex flex-col gap-2 xl:flex-row xl:items-center">
        <label className="flex min-w-52 flex-1 items-center gap-2 rounded-lg border border-line bg-overlay-subtle px-2.5 py-2 focus-within:border-line-strong">
          <Search aria-hidden="true" size={13} className="shrink-0 text-text-muted" />
          <input
            aria-label="Search databases"
            className="w-full bg-transparent text-xs text-text outline-none placeholder:text-text-muted"
            onChange={(e) => resetPage(() => setSearch(e.target.value))}
            placeholder="Search databases by name, engine, status…"
            type="search"
            value={search}
          />
        </label>
        <div className="flex flex-wrap items-center gap-2">
          <select aria-label="Filter by origin" className={selectCls} onChange={(e) => resetPage(() => setOriginFilter(e.target.value as "all" | Origin))} value={originFilter}>
            <option value="all">Origin: all</option>
            {(Object.keys(ORIGIN_META) as Origin[]).map((o) => <option key={o} value={o}>{ORIGIN_META[o].label}</option>)}
          </select>
          <select aria-label="Filter by engine" className={selectCls} onChange={(e) => resetPage(() => setEngineFilter(e.target.value))} value={engineFilter}>
            <option value="all">Engine: all</option>
            {engines.map((e) => <option key={e} value={e}>{e}</option>)}
          </select>
          <select aria-label="Filter by status" className={selectCls} onChange={(e) => resetPage(() => setStatusFilter(e.target.value))} value={statusFilter}>
            <option value="all">Status: all</option>
            {statuses.map((s) => <option key={s} value={s}>{s}</option>)}
          </select>
          <div aria-label="View mode" className="flex gap-1 rounded-lg border border-line bg-overlay-subtle p-1" role="group">
            <button aria-label="Table view" aria-pressed={view === "table"} className={cn("flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-semibold transition", view === "table" ? "border border-brand-line bg-brand-subtle text-text" : "border border-transparent text-text-muted hover:text-text")} onClick={() => setView("table")} type="button">
              <List aria-hidden="true" size={14} /> Table
            </button>
            <button aria-label="Cards view" aria-pressed={view === "cards"} className={cn("flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-semibold transition", view === "cards" ? "border border-brand-line bg-brand-subtle text-text" : "border border-transparent text-text-muted hover:text-text")} onClick={() => setView("cards")} type="button">
              <LayoutGrid aria-hidden="true" size={14} /> Cards
            </button>
          </div>
          {hasActiveFilters && (
            <button className="rounded-lg px-2.5 py-2 text-xs font-semibold text-text-subtle transition hover:text-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" onClick={() => { setSearch(""); setOriginFilter("all"); setEngineFilter("all"); setStatusFilter("all"); setPage(1); }} type="button">
              Clear filters
            </button>
          )}
        </div>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="t-title">Databases <span className="font-normal text-text-subtle">({totalsLabel})</span></h2>
        <label className="flex items-center gap-2 rounded-lg border border-line bg-overlay-subtle px-2.5 py-1.5 text-xs text-text">
          <span className="text-text-subtle">Sort by</span>
          <select aria-label="Sort databases" className="cursor-pointer appearance-none bg-transparent pr-1 outline-none" onChange={(e) => setSort(e.target.value)} value={sort}>
            <option value="name-asc">Name (A → Z)</option>
            <option value="name-desc">Name (Z → A)</option>
            <option value="status">Status</option>
            <option value="engine">Engine</option>
          </select>
        </label>
      </div>

      {(serverDbQ.data && serverDbQ.data.failed > 0) || (catalogQ.data && catalogQ.data.failed > 0) || !allSourcesReported ? (
        <div className="rounded-xl border border-warn-line bg-warn-subtle p-3 text-xs text-warn" role="status">
          Partial inventory: {serverDbQ.data?.failed ?? 0} server{(serverDbQ.data?.failed ?? 0) === 1 ? "" : "s"} and {catalogQ.data?.failed ?? 0} catalog {(catalogQ.data?.failed ?? 0) === 1 ? "entry" : "entries"} could not be read
          {serverDbQ.data?.failedIds?.length ? ` (${serverDbQ.data.failedIds.slice(0, 5).join(", ")}${serverDbQ.data.failedIds.length > 5 ? ` +${serverDbQ.data.failedIds.length - 5} more` : ""})` : ""}
          {unavailableSources.length ? `. Sources not reported: ${unavailableSources.join(", ")}` : ""}. Counts and totals below cover only what was read.
        </div>
      ) : null}

      {sorted.length === 0 ? (
        hasActiveFilters ? (
          <EmptyState
            icon={Database}
            message={`No databases match these filters across ${rows.length} database${rows.length === 1 ? "" : "s"} read.`}
            title="No matches"
          />
        ) : (
          <EmptyState
            icon={Database}
            message="No databases have been created or connected yet. Use “Add database…” to choose where to create one."
            title="No databases"
          />
        )
      ) : view === "cards" ? (
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {visible.map((r) => (
            <article key={r.key} className="rounded-xl border border-line bg-overlay-subtle p-4 transition hover:border-line-strong">
              <div className="flex items-start gap-3">
                <span className="grid h-10 w-10 shrink-0 place-items-center rounded-xl border border-line bg-overlay text-text-subtle">
                  <OriginIcon origin={r.origin} />
                </span>
                <div className="min-w-0 flex-1">
                  <p className="break-words text-sm font-bold text-text">{r.name}</p>
                  <p className="font-mono text-meta text-text-muted">{r.sub}</p>
                </div>
                <RowMenu row={r} onOpen={() => openRow(r)} />
              </div>
              <div className="mt-3 flex flex-wrap items-center gap-2">
                <Pill tone={ORIGIN_META[r.origin].tone}>{ORIGIN_META[r.origin].label}</Pill>
                <StatusDot row={r} />
              </div>
              <div className="mt-3 space-y-1 border-t border-line pt-3 font-mono text-meta text-text-subtle">
                <p className="break-words">{r.engine}{r.version !== "—" ? ` ${r.version}` : ""} · {r.hostPort}</p>
                <p className="break-words">{r.resources}</p>
              </div>
              {r.href ? (
                <Btn className="mt-3 w-full" onClick={() => router.push(r.href!)} tone="ghost">
                  {openLabel(r)}
                </Btn>
              ) : (
                <Btn className="mt-3 w-full" onClick={() => openRow(r)} tone="ghost">
                  {openLabel(r)}
                </Btn>
              )}
            </article>
          ))}
        </div>
      ) : (
        <div className="rounded-xl border border-line bg-overlay-subtle">
          <div className="overflow-x-auto">
            <table aria-label="Databases across every origin" className="ui-table w-full text-xs">
              <thead>
                <tr className="border-b border-line-strong text-left">
                  <th className="ui-th px-4 py-3 font-medium">Name</th>
                  <th className="ui-th px-2 py-3 font-medium">Origin</th>
                  <th className="ui-th px-2 py-3 font-medium">Engine / Version</th>
                  <th className="ui-th px-2 py-3 font-medium">Status</th>
                  <th className="ui-th px-2 py-3 font-medium">Host : Port</th>
                  <th className="ui-th px-2 py-3 font-medium">Resources</th>
                  <th className="ui-th px-2 py-3 font-medium">Node</th>
                  <th className="ui-th px-2 py-3 text-right font-medium">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line">
                {visible.map((r) => (
                  <tr className="transition hover:bg-overlay-subtle" key={r.key}>
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2.5">
                        <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg border border-line bg-overlay text-text-subtle">
                          <OriginIcon origin={r.origin} className="h-4 w-4" />
                        </span>
                        <span className="min-w-0">
                          {/* Full name rendered, not clipped into a `title` tooltip:
                              the tooltip was the only place the name existed whole. */}
                          <span className="block break-words text-xs font-bold text-text">{r.name}</span>
                          <span className="block font-mono text-meta text-text-muted">{r.sub}</span>
                        </span>
                      </div>
                    </td>
                    <td className="px-2 py-3"><Pill tone={ORIGIN_META[r.origin].tone}>{ORIGIN_META[r.origin].label}</Pill></td>
                    <td className="px-2 py-3">
                      <span className="block text-xs text-text">{r.engine}</span>
                      <span className="block font-mono text-meta text-text-muted">{r.version}</span>
                    </td>
                    <td className="px-2 py-3"><StatusDot row={r} /></td>
                    <td className="px-2 py-3 font-mono text-meta text-text-subtle">{r.hostPort}</td>
                    <td className="px-2 py-3 text-meta text-text-subtle">{r.resources}</td>
                    <td className="px-2 py-3 text-meta text-text-subtle">{r.node}</td>
                    <td className="px-2 py-3">
                      <div className="flex items-center justify-end gap-1.5">
                        {r.href ? (
                          <Btn onClick={() => router.push(r.href!)} size="sm" tone="ghost">{openLabel(r)}</Btn>
                        ) : (
                          <Btn onClick={() => openRow(r)} size="sm" tone="ghost">{openLabel(r)}</Btn>
                        )}
                        <RowMenu row={r} onOpen={() => openRow(r)} />
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line px-4 py-3 text-xs text-text-subtle">
            <span>
              {sorted.length === 0
                ? "Nothing to show"
                : hasActiveFilters
                  ? `Showing ${(currentPage - 1) * pageSize + 1}–${Math.min(currentPage * pageSize, sorted.length)} of ${sorted.length} matched of ${rows.length} read (filtered)`
                  : `Showing ${(currentPage - 1) * pageSize + 1}–${Math.min(currentPage * pageSize, sorted.length)} of ${sorted.length} databases`}
            </span>
            <div className="flex items-center gap-2">
              <select aria-label="Rows per page" className="cursor-pointer appearance-none rounded border border-line bg-overlay px-2 py-1 font-mono outline-none" onChange={(e) => { setPageSize(Number(e.target.value)); setPage(1); }} value={pageSize}>
                <option value={10}>10 / page</option>
                <option value={20}>20 / page</option>
                <option value={50}>50 / page</option>
              </select>
            </div>
          </div>
          <div className="px-4 pb-4">
            <Pagination label="Database inventory pagination" onPageChange={setPage} page={currentPage} pageCount={totalPages} />
          </div>
        </div>
      )}
    </div>
  );
}

/**
 * Where each kind of database is actually created.
 *
 * `destination` is stated explicitly because three of these six do not open a form
 * here — they navigate to the surface that owns that kind (the Servers page, the
 * Catalog, or another tab of this page). The old copy said "Pick one path — each
 * opens where it is managed" while the button was labelled "Create Database", so
 * the operator reasonably expected a database to be created.
 */
const CREATE_PATHS: Array<{ key: string; title: string; body: string; destination: string; tab?: DatabaseTab; href?: string }> = [
  { key: "managed", title: "Managed Database", body: "Local database with backups and restore.", destination: "Opens the create form in the Managed DBs tab", tab: "managed" },
  { key: "services", title: "Database Service", body: "Linkable service instance (PostgreSQL, MySQL, Redis…).", destination: "Opens the Services tab, where a template is chosen and provisioned", tab: "services" },
  { key: "serverdb", title: "Server Database", body: "Per-game-server MySQL/PostgreSQL carved from a host.", destination: "Leaves Databases — created on a game server’s Databases page", href: "/admin/servers" },
  { key: "catalog", title: "Catalog Service", body: "Postgres, MySQL, Redis, Mongo, queues and caches.", destination: "Leaves Databases — provisioned through the Service Catalog wizard", href: "/admin/catalog" },
  { key: "containers", title: "DB Container", body: "Low-level container runtime for advanced setups.", destination: "Opens the DB Containers tab, where a container is created on a chosen node", tab: "containers" },
  { key: "hosts", title: "External Host", body: "Connect a database host this panel does not run.", destination: "Opens the Database Hosts tab, where the connection is registered", tab: "hosts" },
];

export function CreateDatabaseModal({ onClose, onSelect }: { onClose: () => void; onSelect: (tab: DatabaseTab) => void }) {
  const router = useRouter();
  function choose(p: (typeof CREATE_PATHS)[number]) {
    if (p.href) router.push(p.href);
    else if (p.tab) onSelect(p.tab);
    onClose();
  }
  return (
    <Modal description="This page does not create the database itself. Choose the kind — you will be taken to the surface that creates and manages it." onClose={onClose} title="Add a database">
      <ul className="list-none space-y-2">
        {CREATE_PATHS.map((p) => (
          <li key={p.key}>
            <button
              className="flex w-full items-center justify-between gap-3 rounded-xl border border-line bg-overlay-subtle p-3.5 text-left transition hover:border-line-strong hover:bg-overlay focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
              onClick={() => choose(p)}
              type="button"
            >
              <span className="min-w-0">
                <span className="block text-sm font-bold text-text">{p.title}</span>
                <span className="mt-0.5 block text-xs text-text-subtle">{p.body}</span>
                <span className="mt-1 block text-meta text-text-muted">{p.destination}</span>
              </span>
              <span aria-hidden="true" className="shrink-0 text-text-muted">→</span>
            </button>
          </li>
        ))}
      </ul>
    </Modal>
  );
}
