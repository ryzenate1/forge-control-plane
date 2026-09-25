"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowRight,
  ArrowUpRight,
  BarChart3,
  Box,
  Check,
  ChevronDown,
  ChevronUp,
  Clock,
  Cpu,
  Database,
  HardDrive,
  Info,
  Layers,
  LayoutGrid,
  Leaf,
  List,
  ListOrdered,
  MapPin,
  MemoryStick,
  Package,
  Plus,
  Rabbit,
  RefreshCw,
  Search,
  Server,
  Settings,
  X,
  Zap,
  type LucideIcon,
} from "lucide-react";
import {
  fetchCatalogEntries,
  fetchCatalogEntry,
  provisionCatalog,
  fetchCatalogInstances,
  fetchCatalogRetention,
  setCatalogRetention,
  runCatalogRetention,
  type CatalogEntry,
} from "@/lib/api/catalog";
import { fetchNodes, fetchRegions } from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import { OfflineBanner } from "@/components/shared/states-offline";
import {
  AdminPageLayout,
  Btn,
  Pill,
  AdminLoadingState,
  AdminErrorState,
  EmptyState,
  Input,
  Modal,
  Card,
  CardHeader,
} from "./admin-ui";
import { DashHeader } from "./dashboard-cards";
import { cn } from "@/lib/utils";

type ViewMode = "cards" | "list";
type SortKey = "name-asc" | "name-desc" | "category" | "recent";

const SORT_OPTIONS: Array<{ value: SortKey; label: string }> = [
  { value: "name-asc", label: "Name (A → Z)" },
  { value: "name-desc", label: "Name (Z → A)" },
  { value: "category", label: "Category" },
  { value: "recent", label: "Recently updated" },
];

type GroupMeta = { id: string; title: string; description: string; icon: LucideIcon };

const GROUP_ORDER = ["databases", "cache", "document", "queue", "message", "storage"];

function groupMetaFor(id: string): GroupMeta {
  switch (id) {
    case "databases":
      return { id, title: "Databases", description: "Persistent, relational and document databases.", icon: Database };
    case "cache":
      return { id, title: "Cache", description: "In-memory data stores, caching and session storage.", icon: Layers };
    case "document":
      return { id, title: "Document Databases", description: "Flexible document-oriented databases.", icon: Database };
    case "queue":
      return { id, title: "Queues", description: "Durable job queues and message buffering.", icon: ListOrdered };
    case "message":
      return { id, title: "Messaging", description: "Lightweight high-throughput pub/sub messaging.", icon: Zap };
    case "storage":
      return { id, title: "Storage", description: "Persistent object and block storage engines.", icon: HardDrive };
    default: {
      const title = id.charAt(0).toUpperCase() + id.slice(1);
      return { id, title, description: `${title} services.`, icon: Package };
    }
  }
}

function groupIdFor(entry: CatalogEntry): string {
  const key = entry.key.toLowerCase();
  if (key === "mongodb") return "document";
  const cat = (entry.category || "").toLowerCase();
  if (cat === "database") return "databases";
  if (cat === "cache") return "cache";
  if (cat === "queue") return "queue";
  if (cat === "message" || cat === "messaging") return "message";
  if (cat === "storage") return "storage";
  if (cat) return cat;
  return "other";
}

const LOGO_STYLE: Record<string, { color: string; bg: string; icon: LucideIcon }> = {
  postgres: { color: "#60a5fa", bg: "rgba(96,165,250,0.12)", icon: Database },
  mysql: { color: "#38bdf8", bg: "rgba(56,189,248,0.12)", icon: Database },
  mariadb: { color: "#d2a679", bg: "rgba(210,166,121,0.12)", icon: Database },
  redis: { color: "#f87171", bg: "rgba(248,113,113,0.12)", icon: Layers },
  valkey: { color: "#a78bfa", bg: "rgba(167,139,250,0.14)", icon: Layers },
  mongodb: { color: "#4ade80", bg: "rgba(74,222,128,0.12)", icon: Leaf },
  "redis-queue": { color: "#f87171", bg: "rgba(248,113,113,0.12)", icon: ListOrdered },
  rabbitmq: { color: "#fb923c", bg: "rgba(251,146,60,0.12)", icon: Rabbit },
  clickhouse: { color: "#facc15", bg: "rgba(250,204,21,0.12)", icon: BarChart3 },
  nats: { color: "#34d399", bg: "rgba(52,211,153,0.12)", icon: Zap },
  memcached: { color: "#94a3b8", bg: "rgba(148,163,184,0.14)", icon: HardDrive },
};

function logoFor(key: string): { color: string; bg: string; icon: LucideIcon } {
  return LOGO_STYLE[key.toLowerCase()] ?? { color: "#94a3b8", bg: "rgba(148,163,184,0.12)", icon: Package };
}

function CatalogLogo({ entryKey, size = "md" }: { entryKey: string; size?: "md" | "sm" }) {
  const { color, bg, icon: Icon } = logoFor(entryKey);
  const box = size === "sm" ? "h-9 w-9" : "h-11 w-11";
  return (
    <div
      aria-hidden="true"
      className={cn("grid shrink-0 place-items-center rounded-xl", box)}
      style={{ backgroundColor: bg, color }}
    >
      <Icon size={size === "sm" ? 18 : 24} strokeWidth={1.8} />
    </div>
  );
}

function VersionPills({ entry }: { entry: CatalogEntry }) {
  const versions = entry.versions ?? [];
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <span className="inline-flex items-center rounded-md border border-blue-500/30 bg-blue-500/10 px-2 py-0.5 text-[11px] font-medium capitalize text-blue-300">
        {entry.category}
      </span>
      {versions.slice(0, 3).map((v) =>
        v === entry.defaultVersion ? (
          <span
            key={v}
            className="inline-flex items-center rounded-md border border-red-500/40 bg-red-500/10 px-2 py-0.5 font-mono text-[11px] font-medium text-red-300"
          >
            {v}&nbsp;&nbsp;·&nbsp;&nbsp;default
          </span>
        ) : (
          <span
            key={v}
            className="inline-flex items-center rounded-md border border-white/10 bg-white/[0.04] px-2 py-0.5 font-mono text-[11px] text-slate-400"
          >
            {v}
          </span>
        ),
      )}
      {versions.length > 3 && <span className="text-[11px] text-[var(--text-subtle)]">+{versions.length - 3}</span>}
    </div>
  );
}

function RequiresLine({ entry, className }: { entry: CatalogEntry; className?: string }) {
  if (!entry.requires?.length) return null;
  return (
    <p className={cn("text-[11px] leading-4 text-[var(--text-subtle)]", className)}>
      Requires: {entry.requires.join(", ")}
    </p>
  );
}

function CardActions({
  entry,
  onDetail,
  onProvision,
  compact,
}: {
  entry: CatalogEntry;
  onDetail: () => void;
  onProvision: () => void;
  compact?: boolean;
}) {
  return (
    <div className={cn("flex gap-2", compact && "sm:w-56 sm:shrink-0")}>
      <button
        type="button"
        onClick={onDetail}
        className="inline-flex h-9 flex-1 items-center justify-center rounded-lg border border-white/10 bg-white/[0.03] px-3 text-xs font-medium text-slate-200 transition hover:bg-white/[0.07]"
      >
        Details
      </button>
      <button
        type="button"
        disabled={!entry.enabled}
        onClick={onProvision}
        className="inline-flex h-9 flex-1 items-center justify-center gap-1.5 rounded-lg border border-red-500/60 bg-[var(--brand)] px-3 text-xs font-semibold text-white transition hover:bg-[var(--brand-hover)] disabled:pointer-events-none disabled:opacity-40"
      >
        <Plus size={14} strokeWidth={2.5} />
        Provision
      </button>
    </div>
  );
}

export function AdminCatalog() {
  const { toast } = useToast();
  const qc = useQueryClient();
  const [search, setSearch] = useState("");
  const [category, setCategory] = useState("all");
  const [runtime, setRuntime] = useState("all");
  const [sort, setSort] = useState<SortKey>("name-asc");
  const [view, setView] = useState<ViewMode>("cards");
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [showProvision, setShowProvision] = useState<CatalogEntry | null>(null);

  const catalogQ = useQuery({ queryKey: ["admin-catalog"], queryFn: fetchCatalogEntries, retry: false });
  const retentionQ = useQuery({ queryKey: ["admin-catalog-retention"], queryFn: fetchCatalogRetention, retry: false });

  const entries = useMemo(() => catalogQ.data ?? [], [catalogQ.data]);

  const groups = useMemo(() => {
    const byId = new Map<string, CatalogEntry[]>();
    for (const e of entries) {
      const gid = groupIdFor(e);
      if (!byId.has(gid)) byId.set(gid, []);
      byId.get(gid)!.push(e);
    }
    return [...byId.entries()]
      .map(([id, items]) => ({ meta: groupMetaFor(id), items: [...items].sort((a, b) => a.displayName.localeCompare(b.displayName)) }))
      .sort((a, b) => {
        const ai = GROUP_ORDER.indexOf(a.meta.id);
        const bi = GROUP_ORDER.indexOf(b.meta.id);
        if (ai !== -1 || bi !== -1) return (ai === -1 ? 99 : ai) - (bi === -1 ? 99 : bi);
        return a.meta.title.localeCompare(b.meta.title);
      });
  }, [entries]);

  const filtered = useMemo(() => {
    const s = search.trim().toLowerCase();
    let out = entries.filter((e) => {
      if (runtime === "kubernetes") return false;
      if (category !== "all" && groupIdFor(e) !== category) return false;
      if (!s) return true;
      return (
        e.key.toLowerCase().includes(s) ||
        e.displayName.toLowerCase().includes(s) ||
        (e.description || "").toLowerCase().includes(s)
      );
    });
    out = [...out].sort((a, b) => {
      switch (sort) {
        case "name-desc":
          return b.displayName.localeCompare(a.displayName);
        case "category":
          return groupIdFor(a).localeCompare(groupIdFor(b)) || a.displayName.localeCompare(b.displayName);
        case "recent":
          return new Date(b.updatedAt || 0).getTime() - new Date(a.updatedAt || 0).getTime();
        default:
          return a.displayName.localeCompare(b.displayName);
      }
    });
    return out;
  }, [entries, search, category, runtime, sort]);

  const visibleGroups = useMemo(() => {
    const keys = new Set(filtered.map((e) => e.key));
    return groups
      .map((g) => ({ ...g, items: g.items.filter((e) => keys.has(e.key)) }))
      .filter((g) => g.items.length > 0);
  }, [groups, filtered]);

  const runRetentionMut = useMutation({
    mutationFn: runCatalogRetention,
    onSuccess: (data) => {
      toast({ tone: "success", title: `Retention run — ${data.deleted} pruned` });
      void qc.invalidateQueries({ queryKey: ["admin-catalog-retention"] });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Retention run failed", message: e.message }),
  });

  const toggleGroup = (id: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const selectClass =
    "h-9 w-full appearance-none rounded-lg border border-[var(--line)] bg-[var(--surface-input)] pl-2.5 pr-7 text-xs text-slate-200 outline-none transition hover:border-[var(--line-strong)] focus:border-[var(--brand)]";

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={() => { void catalogQ.refetch(); void retentionQ.refetch(); }} />

      {/* Header — breadcrumb / title / actions */}
      <div className="space-y-3">
        <div className="flex items-center gap-2 text-xs">
          <span className="text-slate-400">Workloads</span>
          <span className="text-slate-600">/</span>
          <span className="font-semibold text-slate-200">Catalog</span>
        </div>
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0">
            <h1 className="flex items-center gap-2.5 text-[28px] font-bold tracking-tight text-slate-100">
              Service Catalog
              <span className="grid h-5 w-5 place-items-center rounded-full border border-white/15 text-slate-500">
                <Info size={12} />
              </span>
            </h1>
            <p className="mt-1.5 max-w-3xl text-xs leading-5 text-slate-400">
              One-click service catalog and provisioning. GET /catalog, GET /catalog/:key, POST /admin/catalog/provision
              (phase3_registrar.go:22).
              <br />
              Categories: database, cache, queue, storage.
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Btn size="sm" tone="ghost" onClick={() => { void catalogQ.refetch(); void retentionQ.refetch(); }}>
              <RefreshCw size={14} /> Refresh
            </Btn>
            <Btn size="sm" tone="ghost" onClick={() => runRetentionMut.mutate()} loading={runRetentionMut.isPending}>
              <Clock size={14} /> Run retention
            </Btn>
          </div>
        </div>
      </div>

      {/* Filter bar */}
      <div className="flex flex-col gap-2 rounded-xl border border-[var(--line)] bg-[var(--surface)] p-2 xl:flex-row xl:items-center">
        <div className="relative min-w-0 flex-1">
          <Search size={15} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search services by name, key or description…"
            aria-label="Search services by name, key or description"
            className="h-10 w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] pl-9 pr-3 text-xs text-slate-200 outline-none transition placeholder:text-slate-500 hover:border-[var(--line-strong)] focus:border-[var(--brand)]"
          />
        </div>
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-3 xl:flex xl:items-center">
          <label className="block xl:w-40">
            <span className="mb-1 block text-[10px] leading-none text-slate-500">
              Category <span className="text-slate-600">- All</span>
            </span>
            <span className="relative block">
              <select aria-label="Category" className={selectClass} value={category} onChange={(e) => setCategory(e.target.value)}>
                <option value="all">All</option>
                {groups.map((g) => (
                  <option key={g.meta.id} value={g.meta.id}>{g.meta.title}</option>
                ))}
              </select>
              <ChevronDown size={14} className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 text-slate-500" />
            </span>
          </label>
          <label className="block xl:w-40">
            <span className="mb-1 block text-[10px] leading-none text-slate-500">
              Provider / Runtime <span className="text-slate-600">- All</span>
            </span>
            <span className="relative block">
              <select aria-label="Provider or runtime" className={selectClass} value={runtime} onChange={(e) => setRuntime(e.target.value)}>
                <option value="all">All</option>
                <option value="docker">Docker</option>
                <option value="kubernetes">Kubernetes</option>
              </select>
              <ChevronDown size={14} className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 text-slate-500" />
            </span>
          </label>
          <label className="block xl:w-48">
            <span className="mb-1 block text-[10px] leading-none text-slate-500">
              Sort by <span className="text-slate-600">- {SORT_OPTIONS.find((o) => o.value === sort)?.label}</span>
            </span>
            <span className="relative block">
              <select aria-label="Sort by" className={selectClass} value={sort} onChange={(e) => setSort(e.target.value as SortKey)}>
                {SORT_OPTIONS.map((o) => (
                  <option key={o.value} value={o.value}>{o.label}</option>
                ))}
              </select>
              <ChevronDown size={14} className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 text-slate-500" />
            </span>
          </label>
        </div>
        <div
          role="group"
          aria-label="View mode"
          className="flex items-center gap-1 self-start rounded-lg border border-[var(--line)] bg-[var(--surface-input)] p-1 xl:self-center"
        >
          <button
            type="button"
            aria-pressed={view === "cards"}
            onClick={() => setView("cards")}
            className={cn(
              "inline-flex h-8 items-center gap-1.5 rounded-md border px-3 text-xs font-medium transition",
              view === "cards"
                ? "border-red-500/60 bg-red-500/10 text-red-300"
                : "border-transparent text-slate-400 hover:text-slate-200",
            )}
          >
            <LayoutGrid size={14} />
            Cards
          </button>
          <button
            type="button"
            aria-pressed={view === "list"}
            onClick={() => setView("list")}
            className={cn(
              "inline-flex h-8 items-center gap-1.5 rounded-md border px-3 text-xs font-medium transition",
              view === "list"
                ? "border-red-500/60 bg-red-500/10 text-red-300"
                : "border-transparent text-slate-400 hover:text-slate-200",
            )}
          >
            <List size={14} />
            List
          </button>
        </div>
      </div>

      {/* Count + helper */}
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <p className="pt-0.5 text-xs text-slate-400">
          {filtered.length} service{filtered.length === 1 ? "" : "s"}
          {search || category !== "all" || runtime !== "all" ? ` · filtered from ${entries.length}` : ""}
        </p>
        <div className="flex items-start justify-end gap-2 text-right">
          <Info size={14} className="mt-0.5 shrink-0 text-slate-500" />
          <p className="max-w-xl text-[11px] leading-5 text-slate-500">
            Catalog entries are provisioning templates and capabilities from your Forge backend.
            <br />
            Available versions and providers are fetched in real-time from the control plane.
          </p>
        </div>
      </div>

      {/* Body */}
      {catalogQ.isLoading ? (
        <AdminLoadingState label="Loading catalog…" />
      ) : catalogQ.isError ? (
        <AdminErrorState message={(catalogQ.error as Error).message} retry={() => void catalogQ.refetch()} />
      ) : filtered.length === 0 ? (
        <EmptyState
          icon={Package}
          title={search || category !== "all" || runtime !== "all" ? "No matches" : "Catalog empty"}
          message={
            search || category !== "all" || runtime !== "all"
              ? "No services match the current filters."
              : "No catalog entries — seed the catalog via migrations or the admin seeder."
          }
        />
      ) : (
        <div className="space-y-4">
          {visibleGroups.map((group) => {
            const isCollapsed = collapsed.has(group.meta.id);
            const GroupIcon = group.meta.icon;
            const gridClass =
              group.items.length === 1
                ? "grid gap-4 p-4 grid-cols-1"
                : group.items.length === 2
                  ? "grid gap-4 p-4 sm:grid-cols-2"
                  : "grid gap-4 p-4 sm:grid-cols-2 xl:grid-cols-3";
            return (
              <section key={group.meta.id} className="overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--surface)]">
                <button
                  type="button"
                  onClick={() => toggleGroup(group.meta.id)}
                  aria-expanded={!isCollapsed}
                  className="flex w-full items-center gap-3 px-4 py-3.5 text-left transition hover:bg-white/[0.02]"
                >
                  <GroupIcon size={18} className="shrink-0 text-slate-400" />
                  <span className="min-w-0 flex-1">
                    <span className="mr-2.5 text-sm font-bold text-slate-100">{group.meta.title}</span>
                    <span className="text-xs text-slate-500">{group.meta.description}</span>
                  </span>
                  <span className="inline-flex shrink-0 items-center rounded-md border border-white/10 bg-white/[0.03] px-2 py-1 text-[11px] text-slate-400">
                    {group.items.length} service{group.items.length === 1 ? "" : "s"}
                  </span>
                  {isCollapsed ? (
                    <ChevronDown size={16} className="shrink-0 text-slate-500" />
                  ) : (
                    <ChevronUp size={16} className="shrink-0 text-slate-400" />
                  )}
                </button>
                {!isCollapsed && (
                  view === "cards" ? (
                    <div className={gridClass}>
                      {group.items.map((entry) =>
                        group.items.length === 1 ? (
                          <WideCatalogCard
                            key={entry.key}
                            entry={entry}
                            onDetail={() => setSelectedKey(entry.key)}
                            onProvision={() => setShowProvision(entry)}
                          />
                        ) : (
                          <CatalogCard
                            key={entry.key}
                            entry={entry}
                            onDetail={() => setSelectedKey(entry.key)}
                            onProvision={() => setShowProvision(entry)}
                          />
                        ),
                      )}
                    </div>
                  ) : (
                    <div className="divide-y divide-[var(--line)] border-t border-[var(--line)]">
                      {group.items.map((entry) => (
                        <CatalogListRow
                          key={entry.key}
                          entry={entry}
                          onDetail={() => setSelectedKey(entry.key)}
                          onProvision={() => setShowProvision(entry)}
                        />
                      ))}
                    </div>
                  )
                )}
              </section>
            );
          })}
        </div>
      )}

      {retentionQ.data && retentionQ.data.length > 0 && (
        <RetentionCard policies={retentionQ.data} onRefresh={() => void retentionQ.refetch()} />
      )}

      {selectedKey && <CatalogDetailDrawer entryKey={selectedKey} onClose={() => setSelectedKey(null)} onProvision={(e) => { setSelectedKey(null); setShowProvision(e); }} />}

      {showProvision && <ProvisionModal entry={showProvision} onClose={() => setShowProvision(null)} onDone={() => { setShowProvision(null); void catalogQ.refetch(); }} />}
    </AdminPageLayout>
  );
}

function EnabledPill({ enabled }: { enabled: boolean }) {
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center rounded-full border px-2.5 py-0.5 text-[11px] font-medium",
        enabled
          ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-300"
          : "border-red-500/30 bg-red-500/10 text-red-300",
      )}
    >
      {enabled ? "Enabled" : "Disabled"}
    </span>
  );
}

function CatalogCard({ entry, onProvision, onDetail }: { entry: CatalogEntry; onProvision: () => void; onDetail: () => void }) {
  return (
    <article className="flex flex-col rounded-xl border border-[var(--line)] bg-white/[0.015] p-4 transition hover:border-white/[0.14]">
      <div className="flex items-start gap-3">
        <CatalogLogo entryKey={entry.key} />
        <div className="min-w-0 flex-1">
          <div className="flex items-start justify-between gap-2">
            <div className="min-w-0">
              <h3 className="truncate text-[13px] font-semibold text-slate-100">{entry.displayName}</h3>
              <p className="truncate font-mono text-[11px] text-rose-400/90">{entry.key}</p>
            </div>
            <EnabledPill enabled={entry.enabled} />
          </div>
          <p className="mt-1.5 line-clamp-2 min-h-8 text-xs leading-5 text-slate-400">{entry.description || "—"}</p>
        </div>
      </div>
      <div className="mt-3">
        <VersionPills entry={entry} />
      </div>
      <RequiresLine entry={entry} className="mt-2.5" />
      <div className="mt-3.5">
        <CardActions entry={entry} onDetail={onDetail} onProvision={onProvision} />
      </div>
    </article>
  );
}

function WideCatalogCard({ entry, onProvision, onDetail }: { entry: CatalogEntry; onProvision: () => void; onDetail: () => void }) {
  return (
    <article className="flex flex-col gap-4 rounded-xl border border-[var(--line)] bg-white/[0.015] p-4 transition hover:border-white/[0.14] lg:flex-row lg:items-center">
      <div className="flex min-w-0 flex-1 items-start gap-3">
        <CatalogLogo entryKey={entry.key} />
        <div className="min-w-0">
          <h3 className="text-[13px] font-semibold text-slate-100">{entry.displayName}</h3>
          <p className="font-mono text-[11px] text-rose-400/90">{entry.key}</p>
          <p className="mt-1.5 max-w-xl text-xs leading-5 text-slate-400">{entry.description || "—"}</p>
        </div>
      </div>
      <div className="shrink-0 space-y-2.5 lg:w-72">
        <div className="flex items-center justify-between gap-2 lg:justify-end">
          <span className="lg:hidden" />
          <EnabledPill enabled={entry.enabled} />
        </div>
        <div className="flex lg:justify-end">
          <VersionPills entry={entry} />
        </div>
        <RequiresLine entry={entry} className="lg:text-right" />
        <CardActions entry={entry} onDetail={onDetail} onProvision={onProvision} />
      </div>
    </article>
  );
}

function CatalogListRow({ entry, onProvision, onDetail }: { entry: CatalogEntry; onProvision: () => void; onDetail: () => void }) {
  return (
    <div className="flex flex-col gap-3 px-4 py-3.5 sm:flex-row sm:items-center">
      <div className="flex min-w-0 flex-1 items-start gap-3">
        <CatalogLogo entryKey={entry.key} size="sm" />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span className="text-[13px] font-semibold text-slate-100">{entry.displayName}</span>
            <span className="font-mono text-[11px] text-rose-400/90">{entry.key}</span>
            <EnabledPill enabled={entry.enabled} />
          </div>
          <p className="mt-1 line-clamp-1 text-xs text-slate-500">{entry.description || "—"}</p>
          <div className="mt-2 flex flex-wrap items-center gap-1.5">
            <VersionPills entry={entry} />
            {entry.requires?.length ? (
              <span className="text-[11px] text-slate-500">Requires: {entry.requires.join(", ")}</span>
            ) : null}
          </div>
        </div>
      </div>
      <CardActions entry={entry} onDetail={onDetail} onProvision={onProvision} compact />
    </div>
  );
}

function CatalogDetailDrawer({ entryKey, onClose, onProvision }: { entryKey: string; onClose: () => void; onProvision: (e: CatalogEntry) => void }) {
  const entryQ = useQuery({ queryKey: ["admin-catalog-entry", entryKey], queryFn: () => fetchCatalogEntry(entryKey), retry: false });
  const instancesQ = useQuery({ queryKey: ["admin-catalog-instances", entryKey], queryFn: () => fetchCatalogInstances(entryKey), retry: false });

  return (
    <Modal title={entryQ.data ? entryQ.data.displayName : `Catalog — ${entryKey}`} description={entryQ.data ? `${entryQ.data.key} · ${entryQ.data.category}` : entryKey} onClose={onClose} wide className="max-w-6xl">
      {entryQ.isLoading ? <AdminLoadingState label="Loading entry…" /> : entryQ.isError ? <AdminErrorState message={(entryQ.error as Error).message} retry={() => void entryQ.refetch()} /> : entryQ.data ? (
        <div className="space-y-4">
          <DashHeader
            icon={Package}
            eyebrow="Catalog entry"
            title={entryQ.data.displayName}
            pill={{ tone: entryQ.data.enabled ? "green" : "red", label: entryQ.data.enabled ? "Enabled" : "Disabled" }}
            description={entryQ.data.description}
            tags={[entryQ.data.key, entryQ.data.category].filter(Boolean)}
            meta={(entryQ.data.versions ?? []).length > 0 ? [{ label: "Default version", value: <span key="dv" className="font-mono">{entryQ.data.defaultVersion}</span> }] : undefined}
            actions={(
              <>
                <Btn size="sm" tone="primary" onClick={() => onProvision(entryQ.data!)}><Plus size={12} /> Provision</Btn>
                <Btn size="sm" tone="ghost" onClick={() => void instancesQ.refetch()}><RefreshCw size={12} /> Refresh instances</Btn>
              </>
            )}
          />

          <Card className="border border-[var(--line)] bg-[var(--surface)]">
            <CardHeader title={`Instances — GET /catalog/${entryKey}/instances`} icon={Database} action={<span className="text-xs text-[var(--text-subtle)]">{instancesQ.data?.length ?? 0}</span>} />
            {instancesQ.isLoading ? <AdminLoadingState label="Loading instances…" /> : instancesQ.isError ? <div className="p-3 text-xs text-red-300">{(instancesQ.error as Error).message}</div> : (instancesQ.data?.length ?? 0) === 0 ? <EmptyState icon={Server} title="No instances" message="No provisioned instances for this entry yet." /> : (
              <div className="divide-y divide-[var(--line)]">
                {(instancesQ.data ?? []).map((inst) => (
                  <div key={inst.id} className="flex items-center justify-between gap-3 px-4 py-3">
                    <div className="min-w-0">
                      <div className="font-mono text-xs font-medium text-[var(--text)]">{inst.id.slice(0, 12)}… · {inst.kind}:{inst.version}</div>
                      <div className="font-mono text-[11px] text-[var(--text-subtle)]">{inst.host ? `${inst.host}:${inst.port}` : `:${inst.port}`} · {inst.status} · node {inst.nodeId?.slice(0, 8) ?? "—"}</div>
                      {inst.connString && <div className="mt-1 truncate font-mono text-[11px] text-emerald-300">{inst.connString.slice(0, 80)}…</div>}
                    </div>
                    <Pill tone={inst.status === "ready" || inst.status === "running" ? "green" : inst.status === "error" ? "red" : "yellow"}>{inst.status}</Pill>
                  </div>
                ))}
              </div>
            )}
          </Card>

          <pre className="overflow-auto rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] p-3 text-xs leading-5 text-[var(--text-subtle)]">{JSON.stringify(entryQ.data, null, 2)}</pre>
        </div>
      ) : null}
    </Modal>
  );
}

const PROVISION_STEPS = [
  { title: "Version", sub: "Choose image" },
  { title: "Environment", sub: "Attach target" },
  { title: "Placement", sub: "Node & region" },
  { title: "Resources", sub: "Compute & storage" },
  { title: "Review", sub: "Confirm & deploy" },
];

const IMAGE_FOR: Record<string, string> = {
  postgres: "postgres",
  mysql: "mysql",
  mariadb: "mariadb",
  redis: "redis",
  "redis-queue": "redis",
  valkey: "valkey/valkey",
  mongodb: "mongo",
  rabbitmq: "rabbitmq",
  clickhouse: "clickhouse/clickhouse-server",
  nats: "nats",
  memcached: "memcached",
};

const PORTS_FOR: Record<string, string> = {
  postgres: "5432 (PostgreSQL)",
  mysql: "3306 (MySQL)",
  mariadb: "3306 (MySQL)",
  redis: "6379 (Redis)",
  "redis-queue": "6379 (Redis)",
  valkey: "6379 (Valkey)",
  mongodb: "27017 (MongoDB)",
  rabbitmq: "5672 (AMQP), 15672 (Mgmt)",
  clickhouse: "8123 (HTTP), 9000 (Native)",
  nats: "4222 (Client), 8222 (Monitor)",
  memcached: "11211 (Memcached)",
};

const TAGS_FOR: Record<string, string[]> = {
  postgres: ["Database", "Relational", "SQL"],
  mysql: ["Database", "Relational", "SQL"],
  mariadb: ["Database", "Relational", "SQL"],
  redis: ["Cache", "In-Memory", "Sessions"],
  valkey: ["Cache", "In-Memory", "Sessions"],
  "redis-queue": ["Queue", "Jobs", "Redis"],
  mongodb: ["Database", "Document", "NoSQL"],
  rabbitmq: ["Queue", "Messaging", "AMQP"],
  clickhouse: ["Database", "Analytics", "Columnar", "High Performance"],
  nats: ["Messaging", "Pub/Sub", "Lightweight"],
  memcached: ["Cache", "In-Memory", "Key-Value"],
};

function provisionNoun(entry: CatalogEntry): string {
  switch (groupIdFor(entry)) {
    case "cache":
      return "cache cluster";
    case "queue":
      return "queue cluster";
    case "message":
      return "messaging cluster";
    case "document":
      return "document database cluster";
    case "storage":
      return "storage cluster";
    default:
      return "database cluster";
  }
}

function WizardSection({
  index,
  title,
  sub,
  action,
  innerRef,
  children,
}: {
  index: number;
  title: string;
  sub: string;
  action?: React.ReactNode;
  innerRef?: (el: HTMLDivElement | null) => void;
  children: React.ReactNode;
}) {
  return (
    <div ref={innerRef} className="scroll-mt-2 rounded-xl border border-white/[0.07] bg-white/[0.015] p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-start gap-2.5">
          <span className="grid h-5 w-5 shrink-0 place-items-center rounded-md bg-[var(--brand)] font-mono text-[11px] font-bold text-white">
            {index}
          </span>
          <div>
            <h3 className="text-[13px] font-bold text-slate-100">
              {index}. {title}
            </h3>
            <p className="mt-0.5 text-xs text-slate-500">{sub}</p>
          </div>
        </div>
        {action ? <div className="shrink-0">{action}</div> : null}
      </div>
      <div className="mt-3.5 space-y-3">{children}</div>
    </div>
  );
}

function WizardNote({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex items-start gap-2.5 rounded-lg border border-white/[0.07] bg-white/[0.02] p-3 text-xs leading-5 text-slate-400">
      <Info size={15} className="mt-0.5 shrink-0 text-sky-400" />
      <div className="min-w-0">{children}</div>
    </div>
  );
}

function SummaryRow({ label, value, mono }: { label: string; value: React.ReactNode; mono?: boolean }) {
  return (
    <div className="flex items-start justify-between gap-3 py-[7px]">
      <dt className="shrink-0 text-xs text-slate-500">{label}</dt>
      <dd className={cn("min-w-0 text-right text-xs text-slate-200", mono && "break-all font-mono text-[11px]")}>{value}</dd>
    </div>
  );
}

const wizardFieldClass =
  "h-10 w-full rounded-lg border border-white/10 bg-[var(--surface-input)] px-3 text-[13px] text-slate-100 outline-none transition placeholder:text-slate-600 hover:border-white/20 focus:border-red-500/60";

const wizardSelectWrap = "relative block";
const wizardSelectIcon = "pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-slate-500";
const wizardChevron = "pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-slate-500";

function ProvisionModal({ entry, onClose, onDone }: { entry: CatalogEntry; onClose: () => void; onDone: () => void }) {
  const { toast } = useToast();
  const [step, setStep] = useState(1);
  const [version, setVersion] = useState(entry.defaultVersion || entry.versions?.[0] || "");
  const [nodeId, setNodeId] = useState("");
  const [regionId, setRegionId] = useState("");
  const [envId, setEnvId] = useState("");
  const [memoryMb, setMemoryMb] = useState("512");
  const [cpuShares, setCpuShares] = useState("1024");
  const [diskMb, setDiskMb] = useState("2048");
  const [showAllVersions, setShowAllVersions] = useState(false);
  const sectionRefs = useRef<Array<HTMLDivElement | null>>([]);

  const nodesQ = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes, retry: false });
  const regionsQ = useQuery({ queryKey: ["regions"], queryFn: fetchRegions, retry: false });

  const nodes = useMemo(() => nodesQ.data ?? [], [nodesQ.data]);
  const regions = useMemo(() => (regionsQ.data ?? []).filter((r) => r.enabled !== false), [regionsQ.data]);
  const visibleNodes = useMemo(
    () =>
      regionId
        ? nodes.filter((n) => n.regionId === regionId || n.region === regionId || n.region === regions.find((r) => r.id === regionId)?.name || n.region === regions.find((r) => r.id === regionId)?.slug)
        : nodes,
    [nodes, regionId, regions],
  );
  const picked = nodes.find((n) => n.id === nodeId);
  const recommended = useMemo(() => {
    if (nodes.length === 0) return null;
    const scored = nodes.map((n) => ({
      node: n,
      score: (n.heartbeatState === "healthy" ? 40 : 0) + (n.memoryMb ?? 0) / 1024 - (n.diskMb ?? 0) / 16384,
    }));
    scored.sort((a, b) => b.score - a.score);
    return scored[0]?.node ?? null;
  }, [nodes]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("keydown", onKey);
      document.body.style.overflow = prev;
    };
  }, [onClose]);

  const goTo = (n: number) => {
    const clamped = Math.min(5, Math.max(1, n));
    setStep(clamped);
    sectionRefs.current[clamped - 1]?.scrollIntoView({ behavior: "smooth", block: "start" });
  };

  const mut = useMutation({
    mutationFn: () =>
      provisionCatalog({
        kind: entry.key,
        version: version || undefined,
        envId: envId.trim() || undefined,
        nodeId,
        resources: { memoryMb: Number(memoryMb) || 0, cpuShares: Number(cpuShares) || 0, diskMb: Number(diskMb) || 0 },
      }),
    onSuccess: (data) => {
      toast({ tone: "success", title: `Provisioned ${data.kind} → ${data.id.slice(0, 8)}…` });
      onDone();
    },
    onError: (e: Error) => toast({ tone: "error", title: "Provision failed", message: e.message }),
  });

  const key = entry.key.toLowerCase();
  const image = IMAGE_FOR[key] ?? key;
  const ports = PORTS_FOR[key] ?? "—";
  const tags = TAGS_FOR[key] ?? [entry.category.charAt(0).toUpperCase() + entry.category.slice(1)];
  const versions = entry.versions ?? [];
  const versionLabel = version ? `${version}${version === entry.defaultVersion ? " (default)" : ""}` : "—";
  const otherVersions = versions.filter((v) => v !== entry.defaultVersion).slice(0, 2);
  const pickedStatus = picked ? (picked.heartbeatState === "healthy" ? "Online" : (picked.heartbeatState ?? picked.status ?? "unknown")) : null;
  const regionName = regionId ? (regions.find((r) => r.id === regionId)?.name ?? "Default region") : "Default region";

  const nodeLabel = (n: (typeof nodes)[number]) => {
    const host = n.fqdn || n.region || "?";
    const status = n.heartbeatState === "healthy" ? "Online" : (n.heartbeatState ?? n.status ?? "unknown");
    return `${n.name} · ${host} · ${status}${recommended?.id === n.id ? " · ★ recommended" : ""}`;
  };

  return (
    <div
      className="fixed inset-0 z-50 overflow-y-auto bg-black/70 p-4 backdrop-blur-sm sm:p-6"
      role="presentation"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label={`Provision ${entry.displayName}`}
        className="mx-auto flex max-h-[92vh] w-full max-w-[920px] flex-col overflow-hidden rounded-2xl border border-white/10 bg-[var(--surface)] shadow-2xl"
      >
        {/* Header */}
        <div className="flex items-start justify-between gap-4 border-b border-white/[0.07] px-5 pb-4 pt-5 sm:px-6">
          <div className="flex min-w-0 items-start gap-3.5">
            <div className="grid h-12 w-12 shrink-0 place-items-center rounded-xl border border-white/10 bg-white/[0.03] text-violet-300">
              <Box size={24} strokeWidth={1.8} />
            </div>
            <div className="min-w-0">
              <h2 className="text-lg font-bold tracking-tight text-white">Provision {entry.displayName}</h2>
              <p className="mt-0.5 text-xs leading-5 text-slate-400">
                Deploy a {entry.displayName} {provisionNoun(entry)} with automatic infrastructure, networking and configuration.
              </p>
              <div className="mt-2 flex flex-wrap gap-1.5">
                {tags.map((t) => (
                  <span key={t} className="inline-flex items-center rounded-md border border-white/[0.08] bg-white/[0.03] px-2 py-0.5 text-[11px] text-slate-400">
                    {t}
                  </span>
                ))}
              </div>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close dialog"
            className="grid h-8 w-8 shrink-0 place-items-center rounded-lg text-slate-400 transition hover:bg-white/[0.06] hover:text-white"
          >
            <X size={16} />
          </button>
        </div>

        {/* Stepper */}
        <div className="border-b border-white/[0.07] px-5 sm:px-6">
          <ol className="flex items-start gap-1 overflow-x-auto py-4">
            {PROVISION_STEPS.map((s, i) => {
              const n = i + 1;
              const active = n === step;
              const done = n < step;
              return (
                <li key={s.title} className="flex min-w-0 flex-1 items-start last:flex-none">
                  <button type="button" onClick={() => goTo(n)} className="flex min-w-0 items-start gap-2 text-left" aria-current={active ? "step" : undefined}>
                    <span
                      className={cn(
                        "grid h-6 w-6 shrink-0 place-items-center rounded-full text-[11px] font-bold transition",
                        active && "bg-[var(--brand)] text-white",
                        done && "bg-red-500/15 text-red-300",
                        !active && !done && "border border-white/10 bg-white/[0.04] text-slate-400",
                      )}
                    >
                      {done ? <Check size={13} strokeWidth={3} /> : n}
                    </span>
                    <span className="min-w-0 leading-tight">
                      <span className={cn("block truncate text-xs font-semibold", active ? "text-red-400" : "text-slate-300")}>{s.title}</span>
                      <span className="block truncate text-[11px] text-slate-500">{s.sub}</span>
                    </span>
                  </button>
                  {n < PROVISION_STEPS.length && <span className={cn("mx-2 mt-3 h-px min-w-4 flex-1", n < step ? "bg-red-500/40" : "bg-white/10")} aria-hidden="true" />}
                </li>
              );
            })}
          </ol>
          {step >= 1 && <div className="h-0.5 w-24 -translate-y-px rounded-full bg-[var(--brand)]" aria-hidden="true" />}
        </div>

        {/* Body */}
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4 sm:px-6">
          <div className="grid items-start gap-4 lg:grid-cols-[minmax(0,1fr)_290px]">
            <div className="min-w-0 space-y-4">
              <WizardSection
                index={1}
                title="Select Version"
                sub={`Choose the ${entry.displayName} image version to deploy.`}
                innerRef={(el) => {
                  sectionRefs.current[0] = el;
                }}
                action={
                  versions.length > 1 ? (
                    <button
                      type="button"
                      onClick={() => setShowAllVersions((v) => !v)}
                      className="inline-flex h-8 items-center gap-1 rounded-lg border border-white/10 bg-white/[0.02] px-2.5 text-[11px] font-medium text-slate-300 transition hover:bg-white/[0.06] hover:text-white"
                    >
                      View all versions
                      <ArrowUpRight size={12} />
                    </button>
                  ) : undefined
                }
              >
                <label className="block">
                  <span className="mb-1.5 block text-xs text-slate-400">Version</span>
                  <span className={wizardSelectWrap}>
                    <select aria-label="Version" className={wizardFieldClass} value={version} onChange={(e) => setVersion(e.target.value)}>
                      {versions.map((v) => (
                        <option key={v} value={v}>
                          {v}
                          {v === entry.defaultVersion ? " (default)" : ""}
                        </option>
                      ))}
                    </select>
                    <ChevronDown size={14} className={wizardChevron} />
                  </span>
                </label>
                {showAllVersions && versions.length > 1 && (
                  <div className="flex flex-wrap gap-1.5" role="radiogroup" aria-label="All versions">
                    {versions.map((v) => (
                      <button
                        key={v}
                        type="button"
                        role="radio"
                        aria-checked={v === version}
                        onClick={() => setVersion(v)}
                        className={cn(
                          "inline-flex items-center rounded-md border px-2.5 py-1 font-mono text-[11px] transition",
                          v === version
                            ? "border-red-500/60 bg-red-500/10 text-red-200"
                            : "border-white/10 bg-white/[0.02] text-slate-400 hover:text-slate-200",
                        )}
                      >
                        {v}
                        {v === entry.defaultVersion ? " · default" : ""}
                      </button>
                    ))}
                  </div>
                )}
                <WizardNote>
                  This will use the {versionLabel} stable {entry.displayName} image from the configured registry.
                  {otherVersions.length > 0 && (
                    <>
                      {" "}
                      You can also choose a specific version (e.g. {otherVersions.join(", ")}) if you need stability.
                    </>
                  )}
                </WizardNote>
              </WizardSection>

              <WizardSection
                index={2}
                title="Environment"
                sub="Attach connection variables to an environment (optional)."
                innerRef={(el) => {
                  sectionRefs.current[1] = el;
                }}
              >
                <label className="block">
                  <span className="mb-1.5 block text-xs text-slate-400">Environment ID</span>
                  <input
                    value={envId}
                    onChange={(e) => setEnvId(e.target.value)}
                    placeholder="env_… or leave blank"
                    spellCheck={false}
                    className={cn(wizardFieldClass, "font-mono text-xs")}
                  />
                </label>
                <WizardNote>
                  When set, the provisioned service attaches its connection variables to this environment automatically (
                  <code className="font-mono text-[11px] text-slate-300">POST /catalog/:key/instances/:id/attach</code>).
                  Leave blank to provision a standalone instance.
                </WizardNote>
              </WizardSection>

              <WizardSection
                index={3}
                title="Placement"
                sub="Choose where to deploy this database."
                innerRef={(el) => {
                  sectionRefs.current[2] = el;
                }}
              >
                <div className="grid gap-3 sm:grid-cols-2">
                  <label className="block min-w-0">
                    <span className="mb-1.5 block text-xs text-slate-400">Node</span>
                    <span className={wizardSelectWrap}>
                      <Box size={15} className={wizardSelectIcon} />
                      <select
                        aria-label="Node"
                        className={cn(wizardFieldClass, "appearance-none pl-9 pr-8")}
                        value={nodeId}
                        onChange={(e) => setNodeId(e.target.value)}
                      >
                        <option value="">Select node…</option>
                        {visibleNodes.map((n) => (
                          <option key={n.id} value={n.id}>
                            {nodeLabel(n)}
                          </option>
                        ))}
                      </select>
                      <ChevronDown size={14} className={wizardChevron} />
                    </span>
                    {picked && (
                      <span className="mt-1.5 block truncate text-[11px] text-slate-500">
                        {picked.fqdn ?? picked.region ?? ""} · <span className={pickedStatus === "Online" ? "text-emerald-400" : "text-amber-300"}>{pickedStatus}</span>
                      </span>
                    )}
                  </label>
                  <label className="block min-w-0">
                    <span className="mb-1.5 block text-xs text-slate-400">Region (optional)</span>
                    <span className={wizardSelectWrap}>
                      <MapPin size={15} className={wizardSelectIcon} />
                      <select
                        aria-label="Region"
                        className={cn(wizardFieldClass, "appearance-none pl-9 pr-8")}
                        value={regionId}
                        onChange={(e) => {
                          setRegionId(e.target.value);
                          setNodeId("");
                        }}
                      >
                        <option value="">Default region</option>
                        {regions.map((r) => (
                          <option key={r.id} value={r.id}>
                            {r.name}
                          </option>
                        ))}
                      </select>
                      <ChevronDown size={14} className={wizardChevron} />
                    </span>
                  </label>
                </div>
                {nodesQ.isError && <p className="text-xs text-red-300">{(nodesQ.error as Error).message}</p>}
                {regionsQ.isError && <p className="text-xs text-red-300">{(regionsQ.error as Error).message}</p>}
                {regionId && visibleNodes.length === 0 && !nodesQ.isLoading && (
                  <p className="text-xs text-amber-300">No nodes report into this region yet — clear the region to see every node.</p>
                )}
                {!nodeId && recommended && (
                  <div className="flex flex-wrap items-center gap-2">
                    <button
                      type="button"
                      onClick={() => setNodeId(recommended.id)}
                      className="inline-flex h-8 items-center rounded-lg border border-white/10 bg-white/[0.03] px-3 text-xs font-medium text-slate-200 transition hover:bg-white/[0.07]"
                    >
                      Use recommended: {recommended.name}
                    </button>
                    <span className="text-[11px] text-slate-500">Highest placement score from heartbeat, memory and disk headroom.</span>
                  </div>
                )}
                {picked && (
                  <div className="rounded-xl border border-emerald-500/20 bg-emerald-500/[0.04] p-3">
                    <div className="flex items-center justify-between gap-2">
                      <p className="text-[11px] font-bold uppercase tracking-widest text-emerald-300">
                        Placement · {recommended?.id === picked.id ? "Recommended" : "Available"}
                      </p>
                      <Pill tone={picked.heartbeatState === "healthy" ? "green" : picked.heartbeatState === "degraded" ? "yellow" : "red"}>
                        {picked.heartbeatState ?? picked.status ?? "unknown"}
                      </Pill>
                    </div>
                    <ul className="mt-2 grid gap-1 text-xs leading-5 text-[var(--text-subtle)] sm:grid-cols-2">
                      <li className="flex items-center gap-1.5">
                        <span className={picked.heartbeatState === "healthy" ? "text-emerald-400" : "text-red-400"}>
                          {picked.heartbeatState === "healthy" ? "✓" : "✗"}
                        </span>{" "}
                        Heartbeat {picked.heartbeatState ?? "unknown"}
                      </li>
                      <li className="flex items-center gap-1.5">
                        <span className="text-emerald-400">✓</span>{" "}
                        {(picked.memoryMb ?? 0) >= Number(memoryMb || 0)
                          ? `Memory available ${picked.memoryMb} MiB ≥ ${memoryMb} MiB`
                          : `Memory short ${picked.memoryMb ?? "?"} MiB < ${memoryMb} MiB`}
                      </li>
                      <li className="flex items-center gap-1.5">
                        <span className={picked.diskMb != null && picked.diskMb >= Number(diskMb || 0) ? "text-emerald-400" : "text-amber-400"}>
                          {picked.diskMb != null && picked.diskMb >= Number(diskMb || 0) ? "✓" : "○"}
                        </span>{" "}
                        Disk {picked.diskMb ?? "?"} MiB
                      </li>
                      <li className="flex items-center gap-1.5">
                        <span className="text-emerald-400">✓</span> Region {picked.region || "—"}
                      </li>
                    </ul>
                  </div>
                )}
                <WizardNote>
                  The database will be deployed on the selected node. Make sure the node has enough available resources.
                </WizardNote>
              </WizardSection>

              <WizardSection
                index={4}
                title="Resources"
                sub="Compute and storage reserved for this instance."
                innerRef={(el) => {
                  sectionRefs.current[3] = el;
                }}
              >
                <div className="grid gap-3 sm:grid-cols-3">
                  <label className="block">
                    <span className="mb-1.5 block text-xs text-slate-400">Memory MB</span>
                    <input type="number" min={0} value={memoryMb} onChange={(e) => setMemoryMb(e.target.value)} className={cn(wizardFieldClass, "font-mono text-xs")} />
                  </label>
                  <label className="block">
                    <span className="mb-1.5 block text-xs text-slate-400">CPU shares</span>
                    <input type="number" min={0} value={cpuShares} onChange={(e) => setCpuShares(e.target.value)} className={cn(wizardFieldClass, "font-mono text-xs")} />
                  </label>
                  <label className="block">
                    <span className="mb-1.5 block text-xs text-slate-400">Disk MB</span>
                    <input type="number" min={0} value={diskMb} onChange={(e) => setDiskMb(e.target.value)} className={cn(wizardFieldClass, "font-mono text-xs")} />
                  </label>
                </div>
              </WizardSection>

              <WizardSection
                index={5}
                title="Review"
                sub="Confirm the configuration before deploying."
                innerRef={(el) => {
                  sectionRefs.current[4] = el;
                }}
              >
                <dl className="divide-y divide-white/[0.06] rounded-lg border border-white/[0.07] bg-white/[0.015] px-3.5">
                  <SummaryRow label="Service" value={entry.displayName} />
                  <SummaryRow label="Version" value={versionLabel} mono />
                  <SummaryRow label="Image" value={image} mono />
                  <SummaryRow label="Node" value={picked ? picked.name : "Not selected"} />
                  <SummaryRow label="Region" value={regionName} />
                  <SummaryRow label="Environment" value={envId.trim() ? envId.trim() : "Standalone (no attach)"} mono />
                  <SummaryRow label="Resources" value={`${memoryMb || 0} MiB · ${cpuShares || 0} shares · ${diskMb || 0} MiB disk`} mono />
                </dl>
                {!nodeId && <p className="text-xs text-amber-300">Select a node in Placement to enable deploy.</p>}
                {mut.isError && (
                  <p role="alert" className="rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-300">
                    {(mut.error as Error).message}
                  </p>
                )}
              </WizardSection>
            </div>

            {/* Sidebar */}
            <aside className="min-w-0 space-y-4 lg:sticky lg:top-0">
              <div className="rounded-xl border border-white/[0.07] bg-white/[0.015] p-4">
                <div className="flex items-center gap-2.5">
                  <span className="grid h-7 w-7 place-items-center rounded-lg border border-violet-500/25 bg-violet-500/10 text-violet-300">
                    <Database size={15} />
                  </span>
                  <h3 className="text-[13px] font-bold text-slate-100">Service Summary</h3>
                </div>
                <dl className="mt-2 divide-y divide-white/[0.05]">
                  <SummaryRow label="Service" value={entry.displayName} />
                  <SummaryRow label="Category" value={entry.category.charAt(0).toUpperCase() + entry.category.slice(1)} />
                  <SummaryRow label="Version" value={versionLabel} mono />
                  <SummaryRow label="Image" value={image} mono />
                  <SummaryRow label="Port(s)" value={ports} mono />
                  <SummaryRow label="Requires" value={entry.requires?.length ? entry.requires.join(", ") : "none"} mono />
                  <SummaryRow label="Deployment" value="New instance" />
                </dl>
              </div>

              <div className="rounded-xl border border-white/[0.07] bg-white/[0.015] p-4">
                <div className="flex items-center gap-2.5">
                  <span className="grid h-7 w-7 place-items-center rounded-lg border border-sky-500/25 bg-sky-500/10 text-sky-300">
                    <Box size={15} />
                  </span>
                  <h3 className="text-[13px] font-bold text-slate-100">Resource Configuration</h3>
                </div>
                <div className="mt-3 space-y-2">
                  <div className="flex items-center gap-2.5 rounded-lg border border-white/[0.07] bg-white/[0.02] px-3 py-2.5">
                    <MemoryStick size={14} className="shrink-0 text-slate-400" />
                    <span className="flex-1 text-xs text-slate-400">Memory</span>
                    <span className="font-mono text-xs text-slate-200">{memoryMb || 0} MiB</span>
                  </div>
                  <div className="flex items-center gap-2.5 rounded-lg border border-white/[0.07] bg-white/[0.02] px-3 py-2.5">
                    <Cpu size={14} className="shrink-0 text-slate-400" />
                    <span className="flex-1 text-xs text-slate-400">CPU Shares</span>
                    <span className="font-mono text-xs text-slate-200">{cpuShares || 0}</span>
                  </div>
                  <div className="flex items-center gap-2.5 rounded-lg border border-white/[0.07] bg-white/[0.02] px-3 py-2.5">
                    <HardDrive size={14} className="shrink-0 text-amber-300/80" />
                    <span className="flex-1 text-xs text-slate-400">Disk</span>
                    <span className="font-mono text-xs text-slate-200">{diskMb || 0} MiB</span>
                  </div>
                </div>
                <div className="mt-3 flex items-start gap-2 rounded-lg border border-white/[0.07] bg-white/[0.02] p-3 text-[11px] leading-4 text-slate-500">
                  <Info size={13} className="mt-0.5 shrink-0" />
                  <p>These resources can be adjusted in the next step based on the selected node&apos;s capacity.</p>
                </div>
              </div>
            </aside>
          </div>
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between gap-2 border-t border-white/[0.07] bg-white/[0.01] px-5 py-3.5 sm:px-6">
          <button
            type="button"
            onClick={onClose}
            className="inline-flex h-9 items-center rounded-lg border border-white/10 bg-white/[0.02] px-4 text-xs font-medium text-slate-300 transition hover:bg-white/[0.06] hover:text-white"
          >
            Cancel
          </button>
          <div className="flex items-center gap-2">
            {step > 1 && (
              <button
                type="button"
                onClick={() => goTo(step - 1)}
                className="inline-flex h-9 items-center rounded-lg border border-white/10 bg-white/[0.02] px-4 text-xs font-medium text-slate-300 transition hover:bg-white/[0.06] hover:text-white"
              >
                Back
              </button>
            )}
            <button
              type="button"
              onClick={() => (step < 5 ? goTo(step + 1) : mut.mutate())}
              disabled={step === 5 && (!nodeId || mut.isPending)}
              className="inline-flex h-9 items-center gap-1.5 rounded-lg border border-red-500/60 bg-[var(--brand)] px-4 text-xs font-semibold text-white transition hover:bg-[var(--brand-hover)] disabled:pointer-events-none disabled:opacity-50"
            >
              {mut.isPending ? (
                "Provisioning…"
              ) : step < 5 ? (
                <>
                  Next: {PROVISION_STEPS[step].title}
                  <ArrowRight size={14} />
                </>
              ) : (
                <>Deploy {entry.displayName}</>
              )}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

function RetentionCard({ policies, onRefresh }: { policies: import("@/lib/api/catalog").BackupRetention[]; onRefresh: () => void }) {
  const { toast } = useToast();
  const qc = useQueryClient();
  const [editingKind, setEditingKind] = useState<string | null>(null);
  const [days, setDays] = useState("30");
  const [max, setMax] = useState("8");
  const [enabled, setEnabled] = useState(true);

  const saveMut = useMutation({
    mutationFn: () => setCatalogRetention({ kind: editingKind!, retentionDays: Number(days), retentionMax: Number(max), enabled }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["admin-catalog-retention"] });
      setEditingKind(null);
      toast({ tone: "success", title: "Retention policy saved" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Save failed", message: e.message }),
  });

  return (
    <Card className="border border-[var(--line)] bg-[var(--surface)]">
      <CardHeader title="Backup retention — GET /catalog/backups/retention" icon={Clock} action={<Btn size="sm" tone="ghost" onClick={onRefresh}><RefreshCw size={12} /> Reload</Btn>} />
      {policies.length === 0 ? (
        <EmptyState icon={Clock} title="No retention policies" message="No per-kind backup retention yet. Create one per catalog kind (e.g. postgres, redis)." />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--line)] bg-[var(--surface-raised)] text-left text-[10px] uppercase tracking-[0.12em] text-[var(--text-subtle)]">
                <th className="px-4 py-3">Kind</th>
                <th className="px-4 py-3">Enabled</th>
                <th className="px-4 py-3">Days</th>
                <th className="px-4 py-3">Max</th>
                <th className="px-4 py-3">Updated</th>
                <th className="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--line)]">
              {policies.map((p) => (
                <tr key={p.kind} className="hover:bg-[var(--surface-hover)]">
                  <td className="px-4 py-3 font-mono text-xs font-medium text-[var(--text)]">{p.kind}</td>
                  <td className="px-4 py-3"><Pill tone={p.enabled ? "green" : "red"}>{p.enabled ? "enabled" : "disabled"}</Pill></td>
                  <td className="px-4 py-3 font-mono text-xs">{p.retentionDays}</td>
                  <td className="px-4 py-3 font-mono text-xs">{p.retentionMax}</td>
                  <td className="px-4 py-3 font-mono text-xs text-[var(--text-subtle)]">{p.updatedAt ? new Date(p.updatedAt).toLocaleString() : "—"}</td>
                  <td className="px-4 py-3 text-right">
                    <Btn size="sm" tone="ghost" onClick={() => { setEditingKind(p.kind); setDays(String(p.retentionDays)); setMax(String(p.retentionMax)); setEnabled(p.enabled); }}>
                      <Settings size={12} /> Edit
                    </Btn>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {editingKind && (
        <div className="border-t border-[var(--line)] p-4">
          <div className="mb-2 text-xs font-semibold uppercase tracking-widest text-[var(--text-subtle)]">Edit retention — PUT /catalog/backups/retention · {editingKind}</div>
          <div className="grid gap-3 sm:grid-cols-3">
            <Input label="Retention days" value={days} onChange={setDays} type="number" mono />
            <Input label="Retention max" value={max} onChange={setMax} type="number" mono />
            <label className="flex items-center gap-2 pt-6 text-sm">
              <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} className="accent-[var(--brand)]" /> Enabled
            </label>
          </div>
          <div className="mt-3 flex gap-2">
            <Btn size="sm" tone="primary" loading={saveMut.isPending} onClick={() => saveMut.mutate()}>Save</Btn>
            <Btn size="sm" tone="ghost" onClick={() => setEditingKind(null)}>Cancel</Btn>
          </div>
          {saveMut.isError && <div className="mt-2 text-xs text-red-300">{(saveMut.error as Error).message}</div>}
        </div>
      )}
    </Card>
  );
}
