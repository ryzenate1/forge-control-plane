"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import {
  AlertTriangle, Ban, Box, ChevronDown, ChevronLeft, ChevronRight, Cpu, Database, ExternalLink, HardDrive, Info,
  KeyRound, Layers, LayoutGrid, List, MoreVertical, Network, Play, Plus, RefreshCw, Search, Server, Square, Trash2, Zap,
} from "lucide-react";
import {
  type ApiServer, type ApiNode, type ApiAllocation, type ApiEgg,
  type ApiUser, type ApiMount, type ApiRegion,
  fetchServer, fetchServers, fetchNodes, fetchEggs, fetchAllocations, fetchUsers,
  fetchTemplates, fetchRegions, fetchMounts, fetchServerMounts, fetchServerDatabases, fetchServerStartup,
  assignServerAllocation, assignServerMount, fetchServerAllocations, removeServerMount, searchUsers, createServer, createServerDatabase,
  rotateServerDatabasePasswordByBody, deleteServerDatabaseWithSuffix, setPrimaryServerAllocation, unassignServerAllocation, updateServerStartupVariable,
  cancelServerTransfer, deleteServer, fetchServerTransferStatus, suspendServer, transferServer, unsuspendServer, reinstallServer, updateServer,
  sendPowerSignal,
} from "@/lib/api";
import { PageInfoDisclosure } from "@/components/ui/page-info-disclosure";
import { AdminTabs, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, Textarea, cn } from "./admin-ui";

type ServerTab = "about" | "details" | "build" | "startup" | "allocations" | "database" | "mounts" | "manage" | "delete";
const SERVER_TABS: Array<{ id: ServerTab; label: string; danger?: boolean }> = [
  { id: "about", label: "About" },
  { id: "details", label: "Details" },
  { id: "build", label: "Build Configuration" },
  { id: "startup", label: "Startup" },
  { id: "allocations", label: "Allocations" },
  { id: "database", label: "Database" },
  { id: "mounts", label: "Mounts" },
  { id: "manage", label: "Manage" },
  { id: "delete", label: "Delete", danger: true },
];

export function AdminServers() {
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: fetchServers });
  const servers = useMemo(() => Array.isArray(serversQuery.data) ? serversQuery.data : [], [serversQuery.data]);
  const isLoading = serversQuery.isLoading;
  const isError = serversQuery.isError;
  const error = serversQuery.error;
  const refetch = serversQuery.refetch;
  const usersQuery = useQuery({ queryKey: ["users"], queryFn: fetchUsers });
  const users = useMemo(() => Array.isArray(usersQuery.data) ? usersQuery.data : [], [usersQuery.data]);
  const nodesQuery = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });
  const nodes = useMemo(() => Array.isArray(nodesQuery.data) ? nodesQuery.data : [], [nodesQuery.data]);
  const allocsQuery = useQuery({ queryKey: ["allocations"], queryFn: fetchAllocations });
  const allocations = useMemo(() => Array.isArray(allocsQuery.data) ? allocsQuery.data : [], [allocsQuery.data]);
  const eggsQuery = useQuery<ApiEgg[]>({
    queryKey: ["eggs"],
    queryFn: () => fetchEggs("*"),
  });
  const eggs = useMemo(() => Array.isArray(eggsQuery.data) ? eggsQuery.data : [], [eggsQuery.data]);
  const templatesQuery = useQuery({ queryKey: ["templates"], queryFn: fetchTemplates });
  const templates = useMemo(() => Array.isArray(templatesQuery.data) ? templatesQuery.data : [], [templatesQuery.data]);
  const regionsQuery = useQuery({ queryKey: ["regions"], queryFn: fetchRegions });
  const regions = useMemo(() => Array.isArray(regionsQuery.data) ? regionsQuery.data : [], [regionsQuery.data]);
  const mountsQuery = useQuery({ queryKey: ["mounts"], queryFn: fetchMounts });
  const mounts = useMemo(() => Array.isArray(mountsQuery.data) ? mountsQuery.data : [], [mountsQuery.data]);
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [nodeFilter, setNodeFilter] = useState("");
  const [templateFilter, setTemplateFilter] = useState("");
  const [sort, setSort] = useState("name-asc");
  const [view, setView] = useState<"list" | "grid">("list");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [selected, setSelected] = useState<string[]>([]);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [bulkPending, setBulkPending] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [selectedServerId, setSelectedServerId] = useState<string | null>(null);
  const [tab, setTab] = useState<ServerTab>("about");
  const { toast } = useToast();

  const nodeById = useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes]);
  const allocationById = useMemo(() => new Map(allocations.map((a) => [a.id, a])), [allocations]);
  const eggByName = useMemo(() => new Map(eggs.map((e) => [e.name, e])), [eggs]);
  const templateNames = useMemo(() => [...new Set(servers.map((s) => s.template).filter((t): t is string => Boolean(t)))].sort(), [servers]);

  const statusOf = (s: ApiServer): "running" | "stopped" | "error" | "suspended" | "installing" | "other" => {
    if (s.suspended) return "suspended";
    if (s.status === "crashed") return "error";
    if (s.status === "running") return "running";
    if (s.status === "installing" || s.status === "starting") return "installing";
    if (s.status === "stopped" || s.status === "offline") return "stopped";
    return "other";
  };

  const runningCount = servers.filter((s) => statusOf(s) === "running").length;
  const stoppedCount = servers.filter((s) => statusOf(s) === "stopped").length;
  const errorCount = servers.filter((s) => statusOf(s) === "error").length;
  const pct = (n: number) => (servers.length > 0 ? `${Math.round((n / servers.length) * 100)}% of total` : "0% of total");

  function resetPage(update: () => void) {
    setPage(1);
    update();
  }

  const filtered = useMemo(() => {
    const term = search.trim().toLowerCase();
    return servers.filter((s) => {
      if (statusFilter !== "all" && statusOf(s) !== statusFilter) return false;
      if (nodeFilter && s.nodeId !== nodeFilter) return false;
      if (templateFilter && s.template !== templateFilter) return false;
      if (!term) return true;
      const nodeName = (s.nodeId && nodeById.get(s.nodeId)?.name) ?? s.node ?? "";
      return [s.name, s.id, s.uuid ?? "", nodeName, s.owner ?? s.ownerEmail ?? "", s.template ?? ""]
        .join(" ").toLowerCase().includes(term);
    });
  }, [servers, search, statusFilter, nodeFilter, templateFilter, nodeById]);

  const sorted = useMemo(() => {
    const list = [...filtered];
    const created = (s: ApiServer) => (s.createdAt ? new Date(s.createdAt).getTime() : 0);
    switch (sort) {
      case "name-desc": return list.sort((a, b) => b.name.localeCompare(a.name));
      case "status": return list.sort((a, b) => statusOf(a).localeCompare(statusOf(b)) || a.name.localeCompare(b.name));
      case "newest": return list.sort((a, b) => created(b) - created(a));
      case "oldest": return list.sort((a, b) => created(a) - created(b));
      case "node": return list.sort((a, b) => (a.node ?? "").localeCompare(b.node ?? "") || a.name.localeCompare(b.name));
      default: return list.sort((a, b) => a.name.localeCompare(b.name));
    }
  }, [filtered, sort]);

  const totalPages = Math.max(1, Math.ceil(sorted.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const visible = sorted.slice((currentPage - 1) * pageSize, currentPage * pageSize);
  const hasActiveFilters = Boolean(search.trim() || statusFilter !== "all" || nodeFilter || templateFilter);

  function clearFilters() {
    setSearch("");
    setStatusFilter("all");
    setNodeFilter("");
    setTemplateFilter("");
    setPage(1);
  }

  async function handleRefresh() {
    setIsRefreshing(true);
    await Promise.allSettled([serversQuery.refetch(), nodesQuery.refetch()]);
    setTimeout(() => setIsRefreshing(false), 500);
  }

  function toggleSelect(id: string) {
    setSelected((prev) => (prev.includes(id) ? prev.filter((v) => v !== id) : [...prev, id]));
  }

  function toggleSelectPage() {
    const ids = visible.map((s) => s.id);
    setSelected((prev) => (ids.every((id) => prev.includes(id)) ? prev.filter((id) => !ids.includes(id)) : [...new Set([...prev, ...ids])]));
  }

  async function bulkPower(signal: "start" | "stop" | "restart") {
    if (selected.length === 0) return;
    setBulkPending(signal);
    const settled = await Promise.allSettled(selected.map((id) => sendPowerSignal(id, signal)));
    const failed = settled.filter((r) => r.status === "rejected").length;
    if (failed > 0) toast({ tone: "error", title: "Bulk action incomplete", message: `${failed} of ${selected.length} servers failed to ${signal}.` });
    await serversQuery.refetch();
    setSelected([]);
    setBulkPending(null);
  }

  function openDetails(id: string) {
    setSelectedServerId(id);
    setTab("about");
  }

  return (
    <div className="space-y-6">
      <SectionHeader
        title={
          <span className="flex items-center gap-2">
            <span>Servers</span>
            <PageInfoDisclosure
              title="Game servers"
              eyebrow="Architecture & Semantics"
              description="Game server workloads running across your Forge infrastructure. Counts, filters and exports reflect live API data."
              sections={[
                {
                  title: "Where data comes from",
                  icon: Layers,
                  content:
                    "Servers, nodes, allocations and eggs load from /servers, /nodes, /allocations and /eggs. Status pills derive from desired vs actual state; resource usage needs live workload telemetry, so configured limits are shown with used values left blank.",
                },
                {
                  title: "Filtering & paging",
                  icon: Info,
                  content:
                    "Search, status, node and template filters plus sorting apply over the full loaded dataset, then paginate locally. Bulk power actions send one request per selected server.",
                },
              ]}
            />
          </span>
        }
        sub="Game server workloads running across your Forge infrastructure."
        action={
          <Btn tone="primary" onClick={() => setShowCreate(true)}>
            <Plus size={14} /> Create Server
          </Btn>
        }
      />

      <div className="grid grid-cols-2 gap-4 xl:grid-cols-4">
        {[
          { label: "Total Servers", value: servers.length, sub: `${runningCount} running • ${stoppedCount} stopped • ${errorCount} error`, icon: Box, tile: "border-white/[0.08] bg-white/[0.03] text-slate-300" },
          { label: "Running", value: runningCount, sub: pct(runningCount), icon: Play, tile: "border-emerald-500/25 bg-emerald-500/10 text-emerald-300" },
          { label: "Stopped", value: stoppedCount, sub: pct(stoppedCount), icon: Square, tile: "border-white/[0.08] bg-white/[0.03] text-slate-300" },
          { label: "Error", value: errorCount, sub: pct(errorCount), icon: AlertTriangle, tile: "border-red-500/25 bg-red-500/10 text-red-300" },
        ].map((kpi) => (
          <div key={kpi.label} className="rounded-xl border border-white/[0.08] bg-[var(--surface)] p-4 shadow-sm">
            <div className="flex items-center gap-2">
              <span className={cn("grid h-7 w-7 place-items-center rounded-lg border", kpi.tile)}>
                <kpi.icon size={14} />
              </span>
              <span className="text-xs font-semibold text-slate-200">{kpi.label}</span>
            </div>
            <p className="mt-2.5 font-mono text-3xl font-bold tracking-tight text-slate-100">
              {isLoading ? "…" : kpi.value}
            </p>
            <p className="mt-1 text-[11px] text-slate-500">{kpi.sub}</p>
          </div>
        ))}
      </div>

      <div className="flex flex-col gap-2 rounded-xl border border-white/[0.07] bg-white/[0.015] p-3 xl:flex-row xl:items-center">
        <label className="flex min-w-52 flex-1 items-center gap-2 rounded-lg border border-white/[0.08] bg-black/20 px-2.5 py-2">
          <Search size={13} className="shrink-0 text-slate-500" />
          <input
            type="text"
            value={search}
            onChange={(e) => resetPage(() => setSearch(e.target.value))}
            placeholder="Search servers by name, UUID, or node…"
            aria-label="Search servers"
            className="w-full bg-transparent text-xs text-slate-200 outline-none placeholder:text-slate-600"
          />
        </label>
        <div className="flex flex-wrap items-center gap-2">
          <label className="flex items-center gap-2 rounded-lg border border-white/[0.08] bg-black/20 px-2.5 py-2 text-xs text-slate-300">
            <span className="text-[11px] text-slate-500">Status</span>
            <select aria-label="Filter by status" value={statusFilter} onChange={(e) => resetPage(() => setStatusFilter(e.target.value))} className="cursor-pointer appearance-none bg-transparent pr-1 outline-none">
              <option value="all">All</option>
              <option value="running">Running</option>
              <option value="stopped">Stopped</option>
              <option value="error">Error</option>
              <option value="suspended">Suspended</option>
              <option value="installing">Installing</option>
            </select>
          </label>
          <label className="flex items-center gap-2 rounded-lg border border-white/[0.08] bg-black/20 px-2.5 py-2 text-xs text-slate-300">
            <span className="text-[11px] text-slate-500">Node</span>
            <select aria-label="Filter by node" value={nodeFilter} onChange={(e) => resetPage(() => setNodeFilter(e.target.value))} className="max-w-36 cursor-pointer appearance-none bg-transparent pr-1 outline-none">
              <option value="">All</option>
              {nodes.map((n) => <option key={n.id} value={n.id}>{n.name}</option>)}
            </select>
          </label>
          <label className="flex items-center gap-2 rounded-lg border border-white/[0.08] bg-black/20 px-2.5 py-2 text-xs text-slate-300">
            <span className="text-[11px] text-slate-500">Game / Template</span>
            <select aria-label="Filter by template" value={templateFilter} onChange={(e) => resetPage(() => setTemplateFilter(e.target.value))} className="max-w-40 cursor-pointer appearance-none bg-transparent pr-1 outline-none">
              <option value="">All</option>
              {templateNames.map((t) => <option key={t} value={t}>{t}</option>)}
            </select>
          </label>
          <button
            type="button"
            aria-label="Refresh servers"
            onClick={handleRefresh}
            className="grid h-9 w-9 place-items-center rounded-lg border border-white/[0.08] bg-white/[0.02] text-slate-400 transition hover:border-white/20 hover:text-white"
          >
            <RefreshCw size={14} className={isRefreshing ? "animate-spin text-sky-400" : ""} />
          </button>
          <label className="flex items-center gap-2 rounded-lg border border-white/[0.08] bg-black/20 px-2.5 py-2 text-xs text-slate-300">
            <span className="text-[11px] text-slate-500">Sort by</span>
            <select aria-label="Sort servers" value={sort} onChange={(e) => setSort(e.target.value)} className="cursor-pointer appearance-none bg-transparent pr-1 outline-none">
              <option value="name-asc">Name (A → Z)</option>
              <option value="name-desc">Name (Z → A)</option>
              <option value="status">Status</option>
              <option value="newest">Newest</option>
              <option value="oldest">Oldest</option>
              <option value="node">Node</option>
            </select>
          </label>
          <div className="flex gap-1 rounded-lg border border-white/[0.08] bg-black/20 p-1" role="group" aria-label="View mode">
            <button type="button" aria-label="List view" aria-pressed={view === "list"} onClick={() => setView("list")} className={cn("rounded-md p-1.5 transition", view === "list" ? "bg-white/[0.08] text-white" : "text-slate-500 hover:text-slate-300")}>
              <List size={14} />
            </button>
            <button type="button" aria-label="Grid view" aria-pressed={view === "grid"} onClick={() => setView("grid")} className={cn("rounded-md p-1.5 transition", view === "grid" ? "bg-white/[0.08] text-white" : "text-slate-500 hover:text-slate-300")}>
              <LayoutGrid size={14} />
            </button>
          </div>
          {hasActiveFilters && (
            <button type="button" onClick={clearFilters} className="rounded-lg px-2.5 py-2 text-xs font-semibold text-slate-400 transition hover:text-white">
              Clear
            </button>
          )}
        </div>
      </div>

      {selected.length > 0 && (
        <div className="flex flex-wrap items-center gap-2 rounded-xl border border-sky-500/25 bg-sky-500/[0.06] px-4 py-2.5 text-xs">
          <span className="font-semibold text-sky-200">{selected.length} selected</span>
          {(["start", "stop", "restart"] as const).map((signal) => (
            <button
              key={signal}
              type="button"
              disabled={bulkPending !== null}
              onClick={() => { void bulkPower(signal); }}
              className="rounded-lg border border-white/15 px-2.5 py-1 font-semibold capitalize text-slate-200 transition hover:bg-white/[0.06] disabled:opacity-40"
            >
              {bulkPending === signal ? "…" : signal}
            </button>
          ))}
          <button type="button" onClick={() => setSelected([])} className="ml-auto font-semibold text-slate-400 hover:text-white">Clear</button>
        </div>
      )}

      <Card>
        {isLoading ? (
          <div className="space-y-0 divide-y divide-white/[0.04]" role="status" aria-label="Loading servers">
            {Array.from({ length: 5 }, (_, i) => (
              <div key={i} className="flex gap-4 px-4 py-4">
                <div className="h-9 w-9 animate-pulse rounded-lg bg-white/[0.06]" />
                <div className="h-4 w-40 animate-pulse rounded bg-white/[0.06]" />
                <div className="h-4 w-24 animate-pulse rounded bg-white/[0.06]" />
                <div className="h-4 flex-1 animate-pulse rounded bg-white/[0.06]" />
              </div>
            ))}
          </div>
        ) : isError ? (
          <div className="p-4"><div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200"><span>Could not load servers: {error?.message ?? "Unknown error"}</span><Btn size="sm" tone="ghost" onClick={() => void refetch()}>Retry</Btn></div></div>
        ) : sorted.length === 0 ? (
          <EmptyState
            icon={Layers}
            title={hasActiveFilters ? "No matches" : "No servers yet"}
            message={hasActiveFilters ? "No servers match these filters." : "Create your first game server to get started."}
          />
        ) : view === "grid" ? (
          <div className="grid gap-3 p-4 sm:grid-cols-2 xl:grid-cols-3">
            {visible.map((s) => <ServerGridCard key={s.id} server={s} node={s.nodeId ? nodeById.get(s.nodeId) : undefined} allocation={primaryAllocationFor(s, allocations, allocationById)} egg={s.template ? eggByName.get(s.template) : undefined} selected={selected.includes(s.id)} onToggle={() => toggleSelect(s.id)} onOpen={() => openDetails(s.id)} />)}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-wider text-slate-500">
                  <th className="w-8 px-3 py-3"><input type="checkbox" aria-label="Select all servers on this page" checked={visible.length > 0 && visible.every((s) => selected.includes(s.id))} onChange={toggleSelectPage} className="h-3.5 w-3.5 accent-red-500" /></th>
                  <th className="px-2 py-3 font-medium">Server</th>
                  <th className="px-2 py-3 font-medium">Status</th>
                  <th className="px-2 py-3 font-medium">Node</th>
                  <th className="px-2 py-3 font-medium">Resources</th>
                  <th className="px-2 py-3 font-medium">Game / Template</th>
                  <th className="px-2 py-3 font-medium">Created</th>
                  <th className="px-2 py-3 font-medium">Updated</th>
                  <th className="px-2 py-3 text-right font-medium">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {visible.map((s) => (
                  <ServerRow
                    key={s.id}
                    server={s}
                    node={s.nodeId ? nodeById.get(s.nodeId) : undefined}
                    allocation={primaryAllocationFor(s, allocations, allocationById)}
                    egg={s.template ? eggByName.get(s.template) : undefined}
                    selected={selected.includes(s.id)}
                    onToggle={() => toggleSelect(s.id)}
                    onOpen={() => openDetails(s.id)}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
        {!isLoading && !isError && sorted.length > 0 && (
          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-white/[0.06] px-4 py-3 text-xs text-slate-400">
            <span>{sorted.length} of {servers.length} servers{hasActiveFilters ? " (filtered)" : ""}</span>
            <div className="flex items-center gap-2">
              <button type="button" aria-label="Previous page" disabled={currentPage === 1} onClick={() => setPage((p) => Math.max(1, p - 1))} className="grid h-7 w-7 place-items-center rounded-lg border border-white/[0.08] transition hover:border-white/20 disabled:opacity-40"><ChevronLeft size={13} /></button>
              <span className="grid h-7 min-w-7 place-items-center rounded-lg border border-red-500/40 bg-red-500/10 px-2 font-mono font-bold text-red-200">{currentPage}</span>
              <button type="button" aria-label="Next page" disabled={currentPage === totalPages} onClick={() => setPage((p) => Math.min(totalPages, p + 1))} className="grid h-7 w-7 place-items-center rounded-lg border border-white/[0.08] transition hover:border-white/20 disabled:opacity-40"><ChevronRight size={13} /></button>
              <label className="ml-1 flex items-center gap-1.5 rounded-lg border border-white/[0.08] px-2 py-1.5">
                <select aria-label="Rows per page" value={pageSize} onChange={(e) => { setPageSize(Number(e.target.value)); setPage(1); }} className="cursor-pointer appearance-none bg-transparent pr-1 font-mono outline-none">
                  <option value={10}>10 / page</option>
                  <option value={20}>20 / page</option>
                  <option value={50}>50 / page</option>
                </select>
                <ChevronDown size={12} className="text-slate-500" />
              </label>
            </div>
          </div>
        )}
      </Card>

      {showCreate && (
        <CreateServerModal
          users={users}
          nodes={nodes}
          allocations={allocations}
          templates={templates}
          eggs={eggs}
          regions={regions}
          onClose={() => setShowCreate(false)}
        />
      )}

      {selectedServerId && (
        <Modal title="Server" onClose={() => setSelectedServerId(null)} wide>
          <ServerDetailContent
            serverId={selectedServerId}
            tab={tab}
            setTab={setTab}
            users={users}
            nodes={nodes}
            allocations={allocations}
            mounts={mounts}
            onClose={() => setSelectedServerId(null)}
          />
        </Modal>
      )}
    </div>
  );
}

function primaryAllocationFor(server: ApiServer, allocations: ApiAllocation[], byId: Map<string, ApiAllocation>): ApiAllocation | undefined {
  const direct = (server.primaryAllocationId && byId.get(server.primaryAllocationId))
    ?? (server.allocationId && byId.get(server.allocationId));
  if (direct) return direct;
  return allocations.find((a) => a.isPrimary || a.primary);
}

function statusSubtext(server: ApiServer): string {
  if (server.suspended) return "Suspended by admin";
  if (server.status === "crashed") return server.transferError ?? "Crashed";
  if (server.status === "installing" || server.status === "starting") return "Installing…";
  if (server.status === "running") return "Running";
  if (server.status === "stopped" || server.status === "offline") {
    return server.desiredState === "running" ? "Start requested" : "Manual stop";
  }
  return server.transferring ? "Transferring…" : (server.status || "Unknown");
}

function statusPillTone(server: ApiServer): "green" | "red" | "yellow" | "neutral" {
  if (server.suspended || server.status === "crashed") return "red";
  if (server.status === "running") return "green";
  if (server.status === "installing" || server.status === "starting") return "yellow";
  return "neutral";
}

function formatDateTime(iso?: string): { date: string; time: string } {  if (!iso) return { date: "—", time: "" };
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return { date: "—", time: "" };
  return {
    date: d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" }),
    time: d.toLocaleTimeString("en-US", { hour: "numeric", minute: "2-digit", hour12: true }),
  };
}

function shortImage(image?: string): string {
  if (!image) return "—";
  const noTag = image.split("@")[0];
  const parts = noTag.split("/");
  return parts[parts.length - 1] || image;
}

function RowMenu({ server, onOpen }: { server: ApiServer; onOpen: () => void }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="relative">
      <button
        type="button"
        aria-label={`Actions for ${server.name}`}
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        className="grid h-8 w-8 place-items-center rounded-lg border border-white/[0.08] text-slate-400 transition hover:border-white/20 hover:text-white"
      >
        <MoreVertical size={15} />
      </button>
      {open && (
        <>
          <button type="button" aria-label="Close menu" className="fixed inset-0 z-10 cursor-default" onClick={() => setOpen(false)} />
          <div className="absolute right-0 z-20 mt-1 w-44 overflow-hidden rounded-lg border border-white/10 bg-[var(--surface-raised)] shadow-xl">
            {[
              { label: "Open overview", href: `/server/${server.id}` },
              { label: "Open console", href: `/server/${server.id}/console` },
              { label: "Manage", action: onOpen },
            ].map((item) => (
              item.href ? (
                <a key={item.label} href={item.href} className="block px-3 py-2 text-left text-xs text-slate-200 transition hover:bg-white/[0.06]">
                  {item.label}
                </a>
              ) : (
                <button key={item.label} type="button" onClick={() => { setOpen(false); item.action?.(); }} className="block w-full px-3 py-2 text-left text-xs text-slate-200 transition hover:bg-white/[0.06]">
                  {item.label}
                </button>
              )
            ))}
          </div>
        </>
      )}
    </div>
  );
}

function ServerRow({ server, node, allocation, egg, selected, onToggle, onOpen }: {
  server: ApiServer;
  node?: ApiNode;
  allocation?: ApiAllocation;
  egg?: ApiEgg;
  selected: boolean;
  onToggle: () => void;
  onOpen: () => void;
}) {
  const created = formatDateTime(server.createdAt);
  // updatedAt is not part of the typed contract yet — read defensively so the
  // column fills in if/when the API starts sending it, without faking a value.
  const updated = formatDateTime((server as unknown as { updatedAt?: string }).updatedAt);
  const nodeIp = node?.fqdn ?? (allocation ? allocation.ip : undefined) ?? "—";
  const gameName = egg?.nestName ?? server.template ?? "—";
  const gameSub = egg ? egg.name : shortImage(server.dockerImage);
  const canStart = server.status !== "running";
  return (
    <tr className={cn("transition hover:bg-white/[0.02]", selected && "bg-sky-500/[0.04]")}>
      <td className="px-3 py-3"><input type="checkbox" aria-label={`Select ${server.name}`} checked={selected} onChange={onToggle} className="h-3.5 w-3.5 accent-red-500" /></td>
      <td className="px-2 py-3">
        <div className="flex items-center gap-2.5">
          <span className="grid h-9 w-9 shrink-0 place-items-center rounded-lg border border-white/[0.08] bg-white/[0.03] text-slate-400">
            <Box size={16} />
          </span>
          <span className="min-w-0">
            <button type="button" onClick={onOpen} className="block max-w-44 truncate text-left text-xs font-bold text-slate-100 hover:text-white" title={server.name}>
              {server.name}
            </button>
            <span className="block font-mono text-[10px] text-slate-500">{server.id.slice(0, 8)}…</span>
          </span>
        </div>
      </td>
      <td className="px-2 py-3">
        <Pill tone={statusPillTone(server)}>{server.suspended ? "Suspended" : server.status}</Pill>
        <span className="mt-1 block text-[10px] text-slate-500">{statusSubtext(server)}</span>
      </td>
      <td className="px-2 py-3">
        <span className="block max-w-36 truncate text-xs text-slate-200" title={node?.name ?? server.node}>{node?.name ?? server.node ?? "—"}</span>
        <span className="block font-mono text-[10px] text-slate-500">{nodeIp}</span>
      </td>
      <td className="px-2 py-3 font-mono text-[11px] text-slate-300">
        <span className="block">— / {typeof server.memoryMb === "number" ? `${server.memoryMb.toLocaleString()} MiB` : "—"}</span>
        <span className="block">— / {typeof server.diskMb === "number" ? `${server.diskMb.toLocaleString()} MiB` : "—"}</span>
      </td>
      <td className="px-2 py-3">
        <div className="flex items-center gap-2">
          <span className="grid h-7 w-7 shrink-0 place-items-center rounded-md border border-white/[0.08] bg-white/[0.03] text-slate-400">
            <Box size={13} />
          </span>
          <span className="min-w-0">
            <span className="block max-w-32 truncate text-xs text-slate-200" title={gameName}>{gameName}</span>
            <span className="block max-w-32 truncate text-[10px] text-slate-500" title={gameSub}>{gameSub}</span>
          </span>
        </div>
      </td>
      <td className="px-2 py-3 text-[11px] text-slate-300">
        <span className="block whitespace-nowrap">{created.date}</span>
        <span className="block whitespace-nowrap text-slate-500">{created.time}</span>
      </td>
      <td className="px-2 py-3 text-[11px] text-slate-300">
        <span className="block whitespace-nowrap">{updated.date}</span>
        <span className="block whitespace-nowrap text-slate-500">{updated.time}</span>
      </td>
      <td className="px-2 py-3">
        <div className="flex items-center justify-end gap-1.5">
          <a
            href={canStart ? `/server/${server.id}/console` : `/server/${server.id}`}
            aria-label={canStart ? `Start ${server.name}` : `Open ${server.name}`}
            title={canStart ? `Start ${server.name}` : `Open ${server.name}`}
            className="grid h-8 w-8 place-items-center rounded-lg border border-white/[0.08] text-slate-300 transition hover:border-white/20 hover:text-white"
          >
            <Play size={14} />
          </a>
          <RowMenu server={server} onOpen={onOpen} />
        </div>
      </td>
    </tr>
  );
}

function ServerGridCard({ server, node, allocation, egg, selected, onToggle, onOpen }: {
  server: ApiServer;
  node?: ApiNode;
  allocation?: ApiAllocation;
  egg?: ApiEgg;
  selected: boolean;
  onToggle: () => void;
  onOpen: () => void;
}) {
  const created = formatDateTime(server.createdAt);
  const nodeIp = node?.fqdn ?? (allocation ? allocation.ip : undefined) ?? "—";
  return (
    <div className={cn("rounded-xl border bg-[var(--surface)] p-4 shadow-sm transition hover:border-white/20", selected ? "border-sky-500/40" : "border-white/[0.08]")}>
      <div className="flex items-start gap-3">
        <input type="checkbox" aria-label={`Select ${server.name}`} checked={selected} onChange={onToggle} className="mt-1 h-3.5 w-3.5 shrink-0 accent-red-500" />
        <span className="grid h-10 w-10 shrink-0 place-items-center rounded-xl border border-white/[0.08] bg-white/[0.03] text-slate-300">
          <Box size={18} />
        </span>
        <div className="min-w-0 flex-1">
          <button type="button" onClick={onOpen} className="block max-w-full truncate text-left text-sm font-bold text-slate-100 hover:text-white" title={server.name}>
            {server.name}
          </button>
          <p className="font-mono text-[10px] text-slate-500">{server.id.slice(0, 8)}… · {created.date}</p>
        </div>
        <RowMenu server={server} onOpen={onOpen} />
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <Pill tone={statusPillTone(server)}>{server.suspended ? "Suspended" : server.status}</Pill>
        <span className="text-[11px] text-slate-500">{statusSubtext(server)}</span>
      </div>
      <div className="mt-3 space-y-1 border-t border-white/[0.06] pt-3 font-mono text-[11px] text-slate-400">
        <p className="truncate">Node: <span className="text-slate-200">{node?.name ?? server.node ?? "—"}</span> · {nodeIp}</p>
        <p>Mem: <span className="text-slate-200">— / {typeof server.memoryMb === "number" ? `${server.memoryMb.toLocaleString()} MiB` : "—"}</span></p>
        <p className="truncate">Game: <span className="text-slate-200">{egg?.nestName ?? server.template ?? "—"}</span></p>
      </div>
    </div>
  );
}

function CreateServerModal({ users, nodes, allocations, templates, eggs, regions, onClose }: {
  users: ApiUser[];
  nodes: ApiNode[];
  allocations: ApiAllocation[];
  templates: ApiEgg[];
  eggs: ApiEgg[];
  regions: ApiRegion[];
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [name, setName] = useState("");
  const [ownerId, setOwnerId] = useState("");
  const [nodeId, setNodeId] = useState("");
  const [regionId, setRegionId] = useState("");
  const [templateId, setTemplateId] = useState("");
  const [allocationId, setAllocationId] = useState("");
  const [memoryMb, setMemoryMb] = useState("2048");
  const [cpuShares, setCpuShares] = useState("1024");
  const [diskMb, setDiskMb] = useState("10240");
  const availableAllocations = allocations.filter((allocation) => !allocation.server && (!nodeId || allocation.node === nodeId));
  const templateOptions = [
    ...templates.map((template) => ({
      id: template.id,
      label: eggs.find((egg) => egg.id === template.id)?.name ?? template.name,
    })),
    ...eggs.filter((egg) => !templates.some((template) => template.id === egg.id)).map((egg) => ({
      id: egg.id,
      label: egg.nestName ? `${egg.nestName} / ${egg.name}` : egg.name,
    })),
  ];
  const selectedAllocation = availableAllocations.find((allocation) => allocation.id === allocationId);
  const validationError = !name.trim()
    ? "Server name is required."
    : !ownerId
      ? "Owner is required."
      : !templateId
        ? "Template is required."
        : !nodeId
          ? "Node is required."
          : !allocationId || !selectedAllocation
            ? "An available allocation is required."
            : selectedAllocation.node !== nodeId
              ? "The allocation must belong to the selected node."
              : [memoryMb, cpuShares, diskMb].some((value) => !Number.isFinite(Number(value)) || Number(value) <= 0)
                ? "Memory, CPU shares, and disk must be positive numbers."
                : null;
  const createMut = useMutation({
    mutationFn: () => createServer({
      name: name.trim(),
      ownerId,
      nodeId,
      regionId: regionId || undefined,
      templateId,
      allocationId,
      memoryMb: Number(memoryMb),
      cpuShares: Number(cpuShares),
      diskMb: Number(diskMb),
    }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["servers"] });
      void qc.invalidateQueries({ queryKey: ["allocations"] });
      onClose();
    },
    onError: (error) => toast({ tone: "error", title: "Create failed", message: error instanceof Error ? error.message : "Could not create server" }),
  });

  const inputBase = "h-10 w-full rounded-lg border border-white/10 bg-[var(--surface)] text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15";
  const selectBase = "h-10 w-full rounded-lg border border-white/10 bg-[var(--surface)] text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15";

  const iconClasses = "pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-500";

  return (
    <Modal title={<span className="text-base font-semibold text-slate-100">Create Server</span>} onClose={onClose} wide>
      <form className="space-y-6" onSubmit={(event) => { event.preventDefault(); if (!validationError) createMut.mutate(); }}>
        <div>
          <label className="mb-3 block text-xs font-semibold uppercase tracking-wider text-slate-400">Server Identity</label>
          <div className="grid gap-5 sm:grid-cols-2">
            <div>
              <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Server Name</label>
              <div className="relative">
                <Server size={14} className={iconClasses} strokeWidth={1.5} />
                <input className={cn(inputBase, "pl-9")} value={name} onChange={(e) => setName(e.target.value)} placeholder="My Game Server" required />
              </div>
            </div>
            <div>
              <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Owner</label>
              <select className={selectBase + " px-3"} value={ownerId} onChange={(event) => setOwnerId(event.target.value)} required>
                <option value="">Select owner&hellip;</option>
                {users.map((user) => <option key={user.id} value={user.id}>{user.email}</option>)}
              </select>
            </div>
          </div>
        </div>

        <div>
          <label className="mb-3 block text-xs font-semibold uppercase tracking-wider text-slate-400">Deployment</label>
          <div className="grid gap-5 sm:grid-cols-2">
            <div>
              <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Node</label>
              <select className={selectBase + " px-3"} value={nodeId} onChange={(event) => { setNodeId(event.target.value); setAllocationId(""); }}>
                <option value="">Select node&hellip;</option>
                {nodes.filter((node) => !regionId || node.regionId === regionId || node.region === regions.find((region) => region.id === regionId)?.slug).map((node) => <option key={node.id} value={node.id}>{node.name}</option>)}
              </select>
            </div>
            <div>
              <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Region</label>
              <select className={selectBase + " px-3"} value={regionId} onChange={(event) => { setRegionId(event.target.value); setNodeId(""); setAllocationId(""); }}>
                <option value="">Select region&hellip;</option>
                {regions.filter((region) => region.enabled).map((region) => <option key={region.id} value={region.id}>{region.name}</option>)}
              </select>
            </div>
            <div>
              <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Template / Egg</label>
              <select className={selectBase + " px-3"} value={templateId} onChange={(event) => setTemplateId(event.target.value)} required>
                <option value="">Select template&hellip;</option>
                {templateOptions.map((template) => <option key={template.id} value={template.id}>{template.label}</option>)}
              </select>
            </div>
            <div>
              <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Allocation</label>
              <select className={selectBase + " px-3"} value={allocationId} onChange={(event) => setAllocationId(event.target.value)} required>
                <option value="">Select allocation&hellip;</option>
                {availableAllocations.map((allocation) => <option key={allocation.id} value={allocation.id}>{allocation.ip}:{allocation.port}</option>)}
              </select>
            </div>
          </div>
        </div>

        <div>
          <label className="mb-3 block text-xs font-semibold uppercase tracking-wider text-slate-400">Resources</label>
          <div className="grid gap-5 sm:grid-cols-3">
            <div>
              <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">
                Memory <span className="font-normal normal-case text-slate-500">(MiB)</span>
              </label>
              <div className="relative">
                <HardDrive size={14} className={iconClasses} strokeWidth={1.5} />
                <input className={cn(inputBase, "pl-9 [&::-webkit-inner-spin-button]:appearance-none")} type="number" min={64} step={64} value={memoryMb} onChange={(e) => setMemoryMb(e.target.value)} required />
              </div>
              <p className="mt-1 text-[11px] text-slate-500">Minimum 64 MiB</p>
            </div>
            <div>
              <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">
                CPU <span className="font-normal normal-case text-slate-500">(shares)</span>
              </label>
              <div className="relative">
                <Cpu size={14} className={iconClasses} strokeWidth={1.5} />
                <input className={cn(inputBase, "pl-9 [&::-webkit-inner-spin-button]:appearance-none")} type="number" min={1} value={cpuShares} onChange={(e) => setCpuShares(e.target.value)} required />
              </div>
              <p className="mt-1 text-[11px] text-slate-500">Relative CPU weight (default 1024)</p>
            </div>
            <div>
              <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">
                Disk <span className="font-normal normal-case text-slate-500">(MiB)</span>
              </label>
              <div className="relative">
                <Database size={14} className={iconClasses} strokeWidth={1.5} />
                <input className={cn(inputBase, "pl-9 [&::-webkit-inner-spin-button]:appearance-none")} type="number" min={64} step={64} value={diskMb} onChange={(e) => setDiskMb(e.target.value)} required />
              </div>
              <p className="mt-1 text-[11px] text-slate-500">Minimum 64 MiB</p>
            </div>
          </div>
        </div>

        <div className="rounded-xl border border-white/[0.06] bg-white/[0.02] p-4">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-slate-800/50">
              <Server className="h-5 w-5 text-slate-400" strokeWidth={1.5} />
            </div>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium text-slate-200">{name || "Unnamed Server"}</p>
              <p className="text-xs text-slate-500">{memoryMb || "2048"} MiB &middot; {cpuShares || "1024"} CPU &middot; {diskMb || "10240"} MiB disk{templateId ? ` \u00b7 ${templateOptions.find((t) => t.id === templateId)?.label ?? ""}` : ""}</p>
            </div>
          </div>
        </div>

        {validationError && <div className="flex items-start gap-2.5 rounded-lg border border-amber-500/20 bg-amber-950/10 p-3.5 text-sm text-amber-200"><span>{validationError}</span></div>}
        {createMut.error && <div className="flex items-start gap-2.5 rounded-lg border border-red-500/20 bg-red-950/10 p-3.5 text-sm text-red-200"><span>{createMut.error instanceof Error ? createMut.error.message : "Server creation failed."}</span></div>}

        <ModalFooter onCancel={onClose} onConfirm={() => { if (!validationError) createMut.mutate(); }} confirmLabel={createMut.isPending ? "Creating..." : "Create Server"} disabled={Boolean(validationError) || createMut.isPending} />
      </form>
    </Modal>
  );
}

function ServerDetailContent({ serverId, tab, setTab, users, nodes, allocations, mounts, onClose }: {
  serverId: string; tab: ServerTab; setTab: (t: ServerTab) => void;
  users: ApiUser[]; nodes: ApiNode[]; allocations: ApiAllocation[];
  mounts: ApiMount[]; onClose: () => void;
}) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const { data: server, isLoading } = useQuery({ queryKey: ["server", serverId], queryFn: () => fetchServer(serverId) });
  const deleteMut = useMutation({ mutationFn: () => deleteServer(serverId, false), onSuccess: () => { qc.invalidateQueries({ queryKey: ["servers"] }); onClose(); }, onError: (error) => toast({ tone: "error", title: "Delete failed", message: error instanceof Error ? error.message : "Could not delete server" }) });
  const forceDeleteMut = useMutation({ mutationFn: () => deleteServer(serverId, true), onSuccess: () => { qc.invalidateQueries({ queryKey: ["servers"] }); onClose(); }, onError: (error) => toast({ tone: "error", title: "Force delete failed", message: error instanceof Error ? error.message : "Could not force delete server" }) });
  const suspendMut = useMutation({ mutationFn: async () => { const result = await suspendServer(serverId); if (!result.ok) throw new Error("The server reported the suspend action did not complete."); return result; }, onSuccess: () => qc.invalidateQueries({ queryKey: ["server", serverId] }), onError: (error) => toast({ tone: "error", title: "Suspend failed", message: error instanceof Error ? error.message : "Could not suspend server" }) });
  const unsuspendMut = useMutation({ mutationFn: async () => { const result = await unsuspendServer(serverId); if (!result.ok) throw new Error("The server reported the unsuspend action did not complete."); return result; }, onSuccess: () => qc.invalidateQueries({ queryKey: ["server", serverId] }), onError: (error) => toast({ tone: "error", title: "Unsuspend failed", message: error instanceof Error ? error.message : "Could not unsuspend server" }) });
  const reinstallMut = useMutation({ mutationFn: () => reinstallServer(serverId), onSuccess: () => qc.invalidateQueries({ queryKey: ["server", serverId] }), onError: (error) => toast({ tone: "error", title: "Reinstall failed", message: error instanceof Error ? error.message : "Could not reinstall server" }) });

  if (isLoading || !server) return <div className="p-8 text-center text-sm text-slate-500">Loading…</div>;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-bold text-white">{server.name}</h2>
          <p className="text-xs text-slate-400">{server.description ?? ""}</p>
        </div>
        <a className="flex items-center gap-1 text-xs text-sky-400 hover:underline" href={`/server/${server.id}/console`} target="_blank" rel="noreferrer">
          Open console <ExternalLink size={12} />
        </a>
      </div>
      <AdminTabs tabs={SERVER_TABS} active={tab} onChange={(id) => setTab(id as ServerTab)} label="Server sections" />
      {tab === "about" && <ServerAboutTab server={server} users={users} nodes={nodes} allocations={allocations} />}
      {tab === "details" && <ServerDetailsTab server={server} users={users} />}
      {tab === "build" && <ServerBuildTab server={server} users={users} allocations={allocations} />}
      {tab === "startup" && <ServerStartupTab server={server} />}
      {tab === "allocations" && <ServerAllocationsTab server={server} allocations={allocations} />}
      {tab === "database" && <ServerDatabaseTab serverId={serverId} />}
      {tab === "mounts" && <ServerMountsTab server={server} mounts={mounts} />}
      {tab === "manage" && (
        <ServerManageTab
          server={server}
          reinstallMut={reinstallMut}
          suspendMut={suspendMut}
          unsuspendMut={unsuspendMut}
          nodes={nodes}
          allocations={allocations}
        />
      )}
      {tab === "delete" && <ServerDeleteTab deleteMut={deleteMut} forceDeleteMut={forceDeleteMut} serverName={server.name} />}
    </div>
  );
}

function ServerAboutTab({ server, users, nodes, allocations }: { server: ApiServer; users: ApiUser[]; nodes: ApiNode[]; allocations: ApiAllocation[] }) {
  const owner = users.find((u) => u.id === server.owner)?.email ?? server.owner ?? "—";
  const node = nodes.find((n) => n.id === server.node)?.name ?? server.node ?? "—";
  const alloc = allocations.find((a) => a.id === server.allocation);
  const fieldClasses = "h-10 w-full rounded-lg border border-white/10 bg-[var(--surface)] text-sm leading-10 shadow-inner shadow-black/10";
  const iconClasses = "pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-500";
  return (
    <div className="space-y-5">
      <div className="rounded-xl border border-white/[0.06] bg-white/[0.02] p-5">
        <label className="mb-4 block text-xs font-semibold uppercase tracking-wider text-slate-400">Server Identity</label>
        <div className="grid gap-5 sm:grid-cols-2">
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Internal ID</label>
            <div className="relative">
              <Info size={14} className={iconClasses} strokeWidth={1.5} />
              <div className={cn(fieldClasses, "pl-9 font-mono text-slate-400")}>{server.id.slice(0, 8)}&hellip;</div>
            </div>
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">UUID</label>
            <div className="relative">
              <KeyRound size={14} className={iconClasses} strokeWidth={1.5} />
              <div className={cn(fieldClasses, "pl-9 font-mono text-slate-400 truncate")}>{server.uuid ?? server.id}</div>
            </div>
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Server Name</label>
            <div className="relative">
              <Server size={14} className={iconClasses} strokeWidth={1.5} />
              <div className={cn(fieldClasses, "pl-9 text-slate-100")}>{server.name}</div>
            </div>
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Default Connection</label>
            <div className="relative">
              <Network size={14} className={iconClasses} strokeWidth={1.5} />
              <div className={cn(fieldClasses, "pl-9 font-mono text-slate-400")}>{alloc ? `${alloc.ip}:${alloc.port}` : "\u2014"}</div>
            </div>
          </div>
        </div>
      </div>

      <div className="rounded-xl border border-white/[0.06] bg-white/[0.02] p-5">
        <label className="mb-4 block text-xs font-semibold uppercase tracking-wider text-slate-400">Resources</label>
        <div className="grid gap-5 sm:grid-cols-2">
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">
              Memory <span className="font-normal normal-case text-slate-500">(MiB)</span>
            </label>
            <div className="relative">
              <HardDrive size={14} className={iconClasses} strokeWidth={1.5} />
              <div className={cn(fieldClasses, "pl-9 text-slate-100")}>{server.memoryMb != null ? `${server.memoryMb} MiB` : "\u2014"}</div>
            </div>
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">
              Disk <span className="font-normal normal-case text-slate-500">(MiB)</span>
            </label>
            <div className="relative">
              <Database size={14} className={iconClasses} strokeWidth={1.5} />
              <div className={cn(fieldClasses, "pl-9 text-slate-100")}>{server.diskMb != null ? `${server.diskMb} MiB` : "\u2014"}</div>
            </div>
          </div>
        </div>
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        <div className="rounded-xl border border-white/[0.06] bg-white/[0.02] p-4">
          <label className="mb-2 block text-[10px] font-bold uppercase tracking-widest text-slate-500">Status</label>
          <div className="flex items-center gap-2">
            <span className={cn("inline-block h-2 w-2 rounded-full", server.suspended ? "bg-red-400" : server.status === "running" ? "bg-emerald-400" : server.status === "installing" ? "bg-amber-400" : "bg-slate-500")} />
            <span className="text-sm font-semibold text-slate-200">{server.suspended ? "Suspended" : server.status}</span>
          </div>
        </div>
        <div className="rounded-xl border border-white/[0.06] bg-white/[0.02] p-4">
          <label className="mb-2 block text-[10px] font-bold uppercase tracking-widest text-slate-500">Owner</label>
          <span className="text-sm font-semibold text-slate-200">{owner}</span>
        </div>
        <div className="rounded-xl border border-white/[0.06] bg-white/[0.02] p-4">
          <label className="mb-2 block text-[10px] font-bold uppercase tracking-widest text-slate-500">Node</label>
          <span className="text-sm font-semibold text-slate-200">{node}</span>
        </div>
      </div>
    </div>
  );
}

function ServerDetailsTab({ server, users }: { server: ApiServer; users: ApiUser[] }) {
  const qc = useQueryClient();
  const currentOwnerId = server.ownerId ?? users.find((user) => user.id === server.owner || user.email === server.owner)?.id ?? "";
  const [name, setName] = useState(server.name);
  const [description, setDescription] = useState(server.description ?? "");
  const [ownerId, setOwnerId] = useState(currentOwnerId);
  const [userSearch, setUserSearch] = useState("");
  const { data: userResults } = useQuery<ApiUser[]>({
    queryKey: ["user-search", userSearch],
    queryFn: () => searchUsers(userSearch),
    enabled: userSearch.length > 0,
  });
  const canSafelyUpdate = Boolean(ownerId && server.memoryMb !== undefined && server.cpuShares !== undefined && server.diskMb !== undefined);
  const { toast } = useToast();
  const saveMut = useMutation({
    mutationFn: () => {
      if (!canSafelyUpdate) throw new Error("Current server resource values are unavailable.");
      return updateServer(server.id, {
        name: name.trim(),
        description,
        ownerId,
        memoryMb: server.memoryMb!,
        cpuShares: server.cpuShares!,
        diskMb: server.diskMb!,
      });
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["server", server.id] }); void qc.invalidateQueries({ queryKey: ["servers"] }); },
    onError: (error) => toast({ tone: "error", title: "Update failed", message: error instanceof Error ? error.message : "Could not update server details" }),
  });
  return (
    <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); if (canSafelyUpdate && name.trim()) saveMut.mutate(); }}>
      <Card>
        <CardHeader title="Base Information" icon={Info} />
        <div className="space-y-3 p-4">
          <Input label="Server Name" value={name} onChange={setName} required />
          <Textarea label="Description" value={description} onChange={setDescription} rows={3} />
          <Input label="Owner Email Search" value={userSearch} onChange={setUserSearch} placeholder="Search by email or username…" />
          {userResults && userResults.length > 0 && (
            <ul className="max-h-40 overflow-y-auto rounded border border-white/10 bg-[var(--canvas)] p-2 text-sm">
              {userResults.map((u) => (
                <li key={u.id}>
                  <button
                    type="button"
                    className={cn("w-full rounded px-2 py-1 text-left hover:bg-white/5", ownerId === u.id && "bg-[var(--brand)]/20")}
                    onClick={() => { setOwnerId(u.id); setUserSearch(""); }}
                  >
                    {u.email} <span className="text-xs text-slate-400">({u.username})</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
          {ownerId && <div className="text-xs text-slate-400">Selected owner ID: <span className="font-mono">{ownerId}</span></div>}
        </div>
      </Card>
      {!canSafelyUpdate && <p className="text-sm text-amber-300">Details cannot be updated safely because the API response is missing the current owner or resource baseline required by the backend PATCH endpoint.</p>}
      {saveMut.error && <p className="text-sm text-red-300">{saveMut.error instanceof Error ? saveMut.error.message : "Server update failed."}</p>}
      <div className="flex justify-end">
        <Btn tone="primary" type="submit" disabled={!canSafelyUpdate || !name.trim() || saveMut.isPending}>
          {saveMut.isPending ? "Saving…" : "Update Details"}
        </Btn>
      </div>
    </form>
  );
}

function ServerBuildTab({ server, users, allocations }: { server: ApiServer; users: ApiUser[]; allocations: ApiAllocation[] }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const ownerId = server.ownerId ?? users.find((user) => user.id === server.owner || user.email === server.owner)?.id ?? "";
  const [memory, setMemory] = useState(server.memoryMb === undefined ? "" : String(server.memoryMb));
  const [disk, setDisk] = useState(server.diskMb === undefined ? "" : String(server.diskMb));
  const [cpuShares, setCpuShares] = useState(server.cpuShares === undefined ? "" : String(server.cpuShares));
  const serverAllocs = allocations.filter((allocation) => allocation.server === server.id);
  const currentAllocationId = server.allocationId ?? (serverAllocs.some((allocation) => allocation.id === server.allocation) ? server.allocation : "") ?? "";
  const [allocationId, setAllocationId] = useState(currentAllocationId);
  const numericValues = [memory, disk, cpuShares].map(Number);
  const canSafelyUpdate = Boolean(ownerId && server.name.trim() && numericValues.every((value) => Number.isFinite(value) && value > 0));
  const saveMut = useMutation({
    mutationFn: () => {
      if (!canSafelyUpdate) throw new Error("Current owner and positive resource values are required.");
      return updateServer(server.id, {
        name: server.name,
        description: server.description ?? "",
        ownerId,
        memoryMb: Number(memory),
        cpuShares: Number(cpuShares),
        diskMb: Number(disk),
        primaryAllocationId: allocationId || undefined,
      });
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["server", server.id] }); void qc.invalidateQueries({ queryKey: ["servers"] }); },
    onError: (error) => toast({ tone: "error", title: "Build update failed", message: error instanceof Error ? error.message : "Could not update build configuration" }),
  });
  return (
    <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); if (canSafelyUpdate) saveMut.mutate(); }}>
      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader title="Resource Management" icon={Cpu} />
          <div className="space-y-3 p-4">
            <Input label="CPU Shares" value={cpuShares} onChange={setCpuShares} type="number" />
            <Input label="Memory (MiB)" value={memory} onChange={setMemory} type="number" />
            <Input label="Disk Space (MiB)" value={disk} onChange={setDisk} type="number" />
            <p className="text-xs text-amber-300">CPU percentage limits are unavailable: the backend accepts cpuLimit but does not persist it.</p>
          </div>
        </Card>
        <div className="space-y-4">
          <Card>
            <CardHeader title="Application Feature Limits" icon={Layers} />
            <p className="p-4 text-sm text-amber-300">Database, backup, and allocation limits are read-only here because the current backend PATCH endpoint does not persist them.</p>
          </Card>
          <Card>
            <CardHeader title="Allocation Management" icon={Network} />
            <div className="space-y-3 p-4">
              <label className="block text-sm">
                <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-slate-400">Default Allocation</span>
                <select className="h-10 w-full rounded-lg border border-white/10 bg-[var(--surface)] px-3 text-slate-100" value={allocationId} onChange={(event) => setAllocationId(event.target.value)}>
                  <option value="">Keep current allocation</option>
                  {serverAllocs.map((allocation) => <option key={allocation.id} value={allocation.id}>{allocation.ip}:{allocation.port}</option>)}
                </select>
              </label>
              <p className="text-xs text-slate-400">Only allocations already assigned to this server can become primary.</p>
            </div>
          </Card>
        </div>
      </div>
      {!canSafelyUpdate && <p className="text-sm text-amber-300">Build settings cannot be updated until the current owner and resource baseline are available.</p>}
      {saveMut.error && <p className="text-sm text-red-300">{saveMut.error instanceof Error ? saveMut.error.message : "Build update failed."}</p>}
      <div className="flex justify-end">
        <Btn tone="primary" type="submit" disabled={!canSafelyUpdate || saveMut.isPending}>
          {saveMut.isPending ? "Saving…" : "Update Build Configuration"}
        </Btn>
      </div>
    </form>
  );
}

function ServerStartupTab({ server }: { server: ApiServer }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const { data: startup } = useQuery({ queryKey: ["server-startup", server.id], queryFn: () => fetchServerStartup(server.id) });
  const [vars, setVars] = useState<Record<string, string>>({});
  const imageEntries = Object.entries(startup?.docker_images ?? {}) as Array<[string, string]>;
  const updateVar = (name: string, value: string) => setVars((current) => ({ ...current, [name]: value }));
  const saveMut = useMutation({
    mutationFn: async () => {
      const changedVariables = Object.entries(vars);
      if (changedVariables.length === 0) throw new Error("No startup variable changes to save.");
      for (const [key, value] of changedVariables) await updateServerStartupVariable(server.id, key, value);
    },
    onSuccess: () => {
      setVars({});
      void qc.invalidateQueries({ queryKey: ["server-startup", server.id] });
    },
    onError: (error) => toast({ tone: "error", title: "Startup update failed", message: error instanceof Error ? error.message : "Could not update startup variables" }),
  });
  return (
    <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); if (Object.keys(vars).length > 0) saveMut.mutate(); }}>
      <Card>
        <CardHeader title="Startup Command" icon={Zap} />
        <div className="space-y-3 p-4">
          <label className="block text-sm font-medium text-slate-300">Resolved Startup Command<input className="mt-1.5 h-9 w-full rounded-lg border border-white/10 bg-[var(--surface-input)] px-3 font-mono text-xs text-slate-400" readOnly value={startup?.startup_command ?? ""} /></label>
          <label className="block text-sm font-medium text-slate-300">Raw Startup Command<input className="mt-1.5 h-9 w-full rounded-lg border border-white/10 bg-[var(--surface-input)] px-3 font-mono text-xs text-slate-400" readOnly value={startup?.raw_startup_command ?? ""} /></label>
          <p className="text-xs text-amber-300">Startup command editing is unavailable because the current backend PATCH endpoint does not persist it safely.</p>
        </div>
      </Card>
      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader title="Template" icon={Box} />
          <div className="space-y-3 p-4">
            <p className="text-sm text-slate-300">{server.template || "—"}</p>
          </div>
        </Card>
        <Card>
          <CardHeader title="Available Docker Images" icon={Box} />
          <div className="space-y-2 p-4 text-sm text-slate-300">
            {imageEntries.length === 0 ? <p className="text-slate-500">No Docker images were reported by this template.</p> : imageEntries.map(([label, image]) => <div key={image} className="rounded border border-white/[0.06] bg-[var(--surface-input)] px-3 py-2"><span className="block text-xs text-slate-500">{label || "Image"}</span><code className="break-all text-xs">{image}</code></div>)}
          </div>
        </Card>
      </div>
      {startup?.variables && Array.isArray(startup.variables) && startup.variables.length > 0 && (
        <Card>
          <CardHeader title="Service Variables" icon={KeyRound} />
          <div className="space-y-3 p-4">
            {startup.variables.map((variable) => {
              const envVarKey = variable.env_variable || variable.envVariable || "";
              const isEditable = variable.is_editable ?? variable.isEditable ?? false;
              const serverValue = variable.server_value ?? variable.serverValue ?? "";
              return (
                <label className="block text-sm font-medium text-slate-300" key={envVarKey}>
                  <span className="mb-1.5 block">{variable.name} ({envVarKey})</span>
                  <input
                    className="h-9 w-full rounded-lg border border-white/10 bg-[var(--surface-input)] px-3 text-slate-100 disabled:cursor-not-allowed disabled:opacity-60"
                    disabled={!isEditable}
                    onChange={(event) => updateVar(envVarKey, event.target.value)}
                    value={vars[envVarKey] ?? serverValue}
                  />
                  {variable.description && <span className="mt-1 block text-xs text-slate-500">{variable.description}</span>}
                </label>
              );
            })}
          </div>
        </Card>
      )}
      {saveMut.error && <p className="text-sm text-red-300">{saveMut.error instanceof Error ? saveMut.error.message : "Startup variable update failed."}</p>}
      <div className="flex justify-end">
        <Btn tone="primary" type="submit" disabled={Object.keys(vars).length === 0 || saveMut.isPending}>
          {saveMut.isPending ? "Saving…" : "Save Variable Changes"}
        </Btn>
      </div>
    </form>
  );
}

function ServerAllocationsTab({ server, allocations }: { server: ApiServer; allocations: ApiAllocation[] }) {
  const qc = useQueryClient();
  const query = useQuery({ queryKey: ["server-allocations", server.id], queryFn: () => fetchServerAllocations(server.id) });
  const assigned = query.data ?? [];
  const assignedIds = new Set(assigned.map((allocation) => allocation.id));
  const available = allocations.filter((allocation) => !allocation.server && !assignedIds.has(allocation.id) && allocation.node === server.node);
  const [allocationId, setAllocationId] = useState("");
  const refresh = () => { void qc.invalidateQueries({ queryKey: ["server-allocations", server.id] }); void qc.invalidateQueries({ queryKey: ["allocations"] }); void qc.invalidateQueries({ queryKey: ["server", server.id] }); };
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const assignMut = useMutation({ mutationFn: () => assignServerAllocation(server.id, allocationId), onSuccess: () => { setAllocationId(""); refresh(); }, onError: (error) => toast({ tone: "error", title: "Assign failed", message: error instanceof Error ? error.message : "Could not assign allocation" }) });
  const unassignMut = useMutation({ mutationFn: (id: string) => unassignServerAllocation(server.id, id), onSuccess: refresh, onError: (error) => toast({ tone: "error", title: "Unassign failed", message: error instanceof Error ? error.message : "Could not unassign allocation" }) });
  const primaryMut = useMutation({ mutationFn: (id: string) => setPrimaryServerAllocation(server.id, id), onSuccess: refresh, onError: (error) => toast({ tone: "error", title: "Set primary failed", message: error instanceof Error ? error.message : "Could not set primary allocation" }) });
  const primaryId = server.primaryAllocationId ?? server.allocationId;
  return <div className="space-y-4"><Card><CardHeader title="Assigned Allocations" icon={Network}/>{assigned.length === 0 ? <EmptyState icon={Network} message="No allocations assigned."/> : <div className="overflow-x-auto"><table className="w-full text-sm"><tbody className="divide-y divide-white/[0.04]">{assigned.map((allocation) => { const primary = allocation.id === primaryId || allocation.primary || allocation.isPrimary; return <tr key={allocation.id}><td className="px-4 py-3 font-mono text-xs">{allocation.ip}:{allocation.port}</td><td className="px-4 py-3">{primary ? <Pill tone="green">Primary</Pill> : <Btn size="sm" tone="ghost" onClick={() => primaryMut.mutate(allocation.id)}>Make primary</Btn>}</td><td className="px-4 py-3 text-right"><Btn size="sm" tone="danger" disabled={Boolean(primary) || unassignMut.isPending} onClick={() => { void (async () => { if (await confirm({ title: `Unassign ${allocation.ip}:${allocation.port}?`, description: "The allocation will be released from this server.", danger: true, confirmLabel: "Unassign" })) unassignMut.mutate(allocation.id); })(); }}>Unassign</Btn></td></tr>; })}</tbody></table></div>}</Card><Card><CardHeader title="Assign Allocation" icon={Plus}/><div className="flex flex-col gap-3 p-4 sm:flex-row"><select className="h-9 flex-1 rounded border border-white/10 bg-[var(--surface-input)] px-3 text-sm" value={allocationId} onChange={(event) => setAllocationId(event.target.value)}><option value="">Select an unassigned allocation…</option>{available.map((allocation) => <option key={allocation.id} value={allocation.id}>{allocation.ip}:{allocation.port}</option>)}</select><Btn disabled={!allocationId || assignMut.isPending} onClick={() => assignMut.mutate()}>Assign</Btn></div></Card>{renderConfirm()}</div>;
}

function ServerDatabaseTab({ serverId }: { serverId: string }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const dbsQuery = useQuery({ queryKey: ["server-dbs", serverId], queryFn: () => fetchServerDatabases(serverId) });
  const dbs = dbsQuery.data ?? [];
  const [dbName, setDbName] = useState("");
  const [remote, setRemote] = useState("%");
  const createMut = useMutation({
    mutationFn: () => createServerDatabase(serverId, { database: dbName.trim(), remote: remote.trim() || "%" }),
    onSuccess: () => { setDbName(""); void qc.invalidateQueries({ queryKey: ["server-dbs", serverId] }); },
    onError: (error) => toast({ tone: "error", title: "Create failed", message: error instanceof Error ? error.message : "Could not create database" }),
  });
  const rotateMut = useMutation({ mutationFn: (dbId: string) => rotateServerDatabasePasswordByBody(serverId, dbId), onSuccess: () => qc.invalidateQueries({ queryKey: ["server-dbs", serverId] }), onError: (error) => toast({ tone: "error", title: "Rotate failed", message: error instanceof Error ? error.message : "Could not rotate database password" }) });
  const deleteMut = useMutation({ mutationFn: (dbId: string) => deleteServerDatabaseWithSuffix(serverId, dbId), onSuccess: () => qc.invalidateQueries({ queryKey: ["server-dbs", serverId] }), onError: (error) => toast({ tone: "error", title: "Delete failed", message: error instanceof Error ? error.message : "Could not delete database" }) });
  const [confirm, renderConfirm] = useConfirm();
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader title="Active Databases" icon={Database} />
        {dbs.length === 0 ? (
          <EmptyState icon={Database} message="No databases for this server." />
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/[0.06] bg-[var(--surface-input)] text-left text-[10px] uppercase tracking-widest text-slate-500">
                <th className="px-4 py-2">Database</th>
                <th className="px-4 py-2">Username</th>
                <th className="px-4 py-2">Host</th>
                <th className="px-4 py-2">Remote</th>
                <th className="px-4 py-2 text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {dbs.map((d) => (
                <tr key={d.id} className="border-b border-white/[0.04]">
                  <td className="px-4 py-2 font-mono text-xs">{d.database}</td>
                  <td className="px-4 py-2 font-mono text-xs">{d.username}</td>
                  <td className="px-4 py-2 font-mono text-xs">{d.host ?? "—"}</td>
                  <td className="px-4 py-2 text-xs">{d.remote}</td>
                  <td className="px-4 py-2 text-right space-x-2">
                    <Btn tone="ghost" onClick={() => { void (async () => { if (await confirm({ title: `Rotate password for ${d.database}?`, description: "Existing clients will stop connecting with the old password.", confirmLabel: "Rotate" })) rotateMut.mutate(d.id); })(); }}>
                      Rotate
                    </Btn>
                    <Btn tone="danger" onClick={() => { void (async () => { if (await confirm({ title: `Delete database ${d.database}?`, description: "The database and its data will be removed. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteMut.mutate(d.id); })(); }}>
                      Delete
                    </Btn>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
      <Card>
        <CardHeader title="Create New Database" icon={Plus} />
        <form
          className="space-y-3 p-4"
          onSubmit={(e) => { e.preventDefault(); createMut.mutate(); }}
        >
          <p className="text-xs text-slate-400">The backend selects the database host automatically; server database creation does not accept a host ID.</p>
          <Input label="Database Name" value={dbName} onChange={setDbName} placeholder={`s${serverId.slice(0, 6)}_name`} required />
          <Input label="Connections From" value={remote} onChange={setRemote} />
          {createMut.error && <p className="text-sm text-red-300">{createMut.error instanceof Error ? createMut.error.message : "Database creation failed."}</p>}
          <Btn tone="primary" type="submit" disabled={!dbName.trim() || createMut.isPending}>
            {createMut.isPending ? "Creating…" : "Create Database"}
          </Btn>
        </form>
      </Card>
      {renderConfirm()}
    </div>
  );
}

function ServerMountsTab({ server, mounts }: { server: ApiServer; mounts: ApiMount[] }) {
  const qc = useQueryClient();
  const serverId = server.id;
  const smQuery = useQuery({ queryKey: ["server-mounts", serverId], queryFn: () => fetchServerMounts(serverId) });
  const serverMounts = smQuery.data ?? [];
  const isMountsError = smQuery.isError;
  const mountsError = smQuery.error;
  const refetchMounts = smQuery.refetch;
  const mountIds = new Set(serverMounts.map((mount) => mount.id));
  const nodeId = server.nodeId;
  const eggId = server.template;
  const eligibleMounts = nodeId && eggId
    ? mounts.filter((mount) => (mount.nodeIds ?? []).includes(nodeId) && (mount.templateIds ?? []).includes(eggId))
    : [];
  const [syncNotice, setSyncNotice] = useState<string | null>(null);
  const refreshMountState = () => {
    void qc.invalidateQueries({ queryKey: ["server-mounts", serverId] });
    void qc.invalidateQueries({ queryKey: ["server", serverId] });
  };
  const attachMut = useMutation({
    mutationFn: (mountId: string) => assignServerMount(serverId, mountId),
    onSuccess: (result) => { setSyncNotice(result.runtimeSynchronized ? "Mount assignment synchronized with the runtime." : "Mount assignment is pending runtime synchronization."); refreshMountState(); },
    onError: refreshMountState,
  });
  const detachMut = useMutation({
    mutationFn: (mountId: string) => removeServerMount(serverId, mountId),
    onSuccess: (result) => { setSyncNotice(result.runtimeSynchronized ? "Mount removal synchronized with the runtime." : "Mount removal is pending runtime synchronization."); refreshMountState(); },
    onError: refreshMountState,
  });
  const actionError = attachMut.error ?? detachMut.error;
  const errorText = (error: unknown, fallback: string) => error instanceof Error ? error.message : fallback;
  const eligibilityGuidance = !server.nodeId || !server.template
    ? "This server is missing node or egg information, so no mounts can be selected. Assign both before attaching a mount."
    : "Create a mount, then attach this server's node and egg to it before it can be assigned here.";

  return (
    <Card>
      <CardHeader title="Eligible Mounts" icon={HardDrive} />
      <div className="border-b border-white/[0.06] px-4 py-3 text-xs text-slate-400">
        Only mounts attached to this server&apos;s node and egg are shown. Assignment makes the mount available to the server; it does not confirm a runtime mount.
      </div>
      {server.configSyncPending ? <div className="m-4 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-100" role="status">Mount configuration is pending runtime synchronization{server.configSyncError ? `: ${server.configSyncError}` : "."}</div> : null}
      {syncNotice ? <div className="m-4 rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-sm text-emerald-100" role="status">{syncNotice}</div> : null}
      {isMountsError ? <div className="mx-4 mb-4 flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200" role="alert"><span>Could not load mount assignments: {errorText(mountsError, "Server mount assignments could not be loaded.")}</span><Btn size="sm" tone="ghost" onClick={() => void refetchMounts()}>Retry</Btn></div> : null}
      {actionError ? <div className="m-4 rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-200" role="alert">{errorText(actionError, "Mount assignment failed. The persisted change may be pending runtime synchronization.")}</div> : null}
      {eligibleMounts.length === 0 ? (
        <div className="p-6 text-sm text-slate-400">
          <p className="font-medium text-slate-200">No eligible mounts</p>
          <p className="mt-1">{eligibilityGuidance}</p>
        </div>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-white/[0.06] bg-[var(--surface-input)] text-left text-[10px] uppercase tracking-widest text-slate-500">
              <th className="px-4 py-2">ID</th>
              <th className="px-4 py-2">Name</th>
              <th className="px-4 py-2">Source</th>
              <th className="px-4 py-2">Target</th>
              <th className="px-4 py-2">Assignment</th>
              <th className="px-4 py-2 text-right">Action</th>
            </tr>
          </thead>
          <tbody>
            {eligibleMounts.map((mount) => {
              const assigned = mountIds.has(mount.id);
              return (
                <tr key={mount.id} className="border-b border-white/[0.04]">
                  <td className="px-4 py-2 font-mono text-xs">{mount.id.slice(0, 8)}…</td>
                  <td className="px-4 py-2 font-semibold">{mount.name}</td>
                  <td className="px-4 py-2 font-mono text-xs">{mount.source}</td>
                  <td className="px-4 py-2 font-mono text-xs">{mount.target}</td>
                  <td className="px-4 py-2">
                    {assigned ? <Pill tone="green">Assigned</Pill> : <Pill tone="neutral">Not assigned</Pill>}
                  </td>
                  <td className="px-4 py-2 text-right">
                    {assigned ? (
                      <Btn tone="danger" disabled={detachMut.isPending} onClick={() => detachMut.mutate(mount.id)}>{detachMut.isPending ? "Unassigning…" : "Unassign"}</Btn>
                    ) : (
                      <Btn tone="primary" disabled={attachMut.isPending} onClick={() => attachMut.mutate(mount.id)}>{attachMut.isPending ? "Assigning…" : "Assign"}</Btn>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </Card>
  );
}

function ServerManageTab({ server, reinstallMut, suspendMut, unsuspendMut, nodes, allocations }: { server: ApiServer; reinstallMut: { mutate: () => void; isPending: boolean }; suspendMut: { mutate: () => void; isPending: boolean }; unsuspendMut: { mutate: () => void; isPending: boolean }; nodes: ApiNode[]; allocations: ApiAllocation[] }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const transferQuery = useQuery({ queryKey: ["server-transfer", server.id], queryFn: () => fetchServerTransferStatus(server.id), retry: false, refetchInterval: server.transferring ? 5000 : false });
  const [targetNodeId, setTargetNodeId] = useState(server.transferTargetNodeId ?? "");
  const [primaryAllocationId, setPrimaryAllocationId] = useState("");
  const targetAllocations = allocations.filter((allocation) => allocation.node === targetNodeId && !allocation.server);
  const transferMut = useMutation({ mutationFn: () => transferServer(server.id, targetNodeId, primaryAllocationId || undefined), onSuccess: () => { void transferQuery.refetch(); void qc.invalidateQueries({ queryKey: ["server", server.id] }); }, onError: (error) => toast({ tone: "error", title: "Transfer failed", message: error instanceof Error ? error.message : "Could not transfer server" }) });
  const cancelMut = useMutation({ mutationFn: async () => { const result = await cancelServerTransfer(server.id); if (!result.ok) throw new Error("The server reported the transfer was not cancelled."); return result; }, onSuccess: () => { void transferQuery.refetch(); void qc.invalidateQueries({ queryKey: ["server", server.id] }); }, onError: (error) => toast({ tone: "error", title: "Cancel failed", message: error instanceof Error ? error.message : "Could not cancel transfer" }) });
  const transfer = transferQuery.data;
  return (
    <div className="grid gap-4 md:grid-cols-2">
      <Card>
        <CardHeader title="Reinstall Server" icon={RefreshCw} />
        <div className="space-y-3 p-4">
          <p className="text-xs text-slate-400">Re-runs the egg install script. Container will be destroyed and recreated.</p>
          <Btn tone="danger" onClick={() => { void (async () => { if (await confirm({ title: `Reinstall ${server.name ?? server.id.slice(0, 8)}?`, description: "The container will be destroyed and recreated, and the egg install script will re-run. This can overwrite server files.", danger: true, confirmLabel: "Reinstall" })) reinstallMut.mutate(); })(); }} disabled={reinstallMut.isPending}>
            {reinstallMut.isPending ? "Reinstalling…" : "Reinstall Server"}
          </Btn>
        </div>
      </Card>
      <Card>
        <CardHeader title="Suspension" icon={Ban} />
        <div className="space-y-3 p-4">
          <p className="text-xs text-slate-400">Suspending stops the container and blocks user access until unsuspended.</p>
          {server.suspended ? (
            <Btn tone="primary" onClick={() => unsuspendMut.mutate()} disabled={unsuspendMut.isPending}>
              {unsuspendMut.isPending ? "Unsuspending…" : "Unsuspend Server"}
            </Btn>
          ) : (
            <Btn tone="warning" onClick={() => suspendMut.mutate()} disabled={suspendMut.isPending}>
              {suspendMut.isPending ? "Suspending…" : "Suspend Server"}
            </Btn>
          )}
        </div>
      </Card>
      <Card>
        <CardHeader title="Transfer" icon={Network} action={transfer ? <Pill tone={transfer.error ? "red" : transfer.transferring ? "yellow" : "blue"}>{transfer.state}</Pill> : undefined}/>
        <div className="space-y-3 p-4">
          {transfer?.error ? <p className="text-xs text-red-300">{transfer.error}</p> : null}
          <label className="block text-xs text-slate-400">Target node<select className="mt-1 h-9 w-full rounded border border-white/10 bg-[var(--surface-input)] px-3 text-sm text-slate-100" value={targetNodeId} onChange={(event) => { setTargetNodeId(event.target.value); setPrimaryAllocationId(""); }}><option value="">Select…</option>{nodes.filter((node) => node.id !== server.nodeId && node.name !== server.node).map((node) => <option key={node.id} value={node.id}>{node.name}</option>)}</select></label>
          <label className="block text-xs text-slate-400">Primary allocation<select className="mt-1 h-9 w-full rounded border border-white/10 bg-[var(--surface-input)] px-3 text-sm text-slate-100" value={primaryAllocationId} onChange={(event) => setPrimaryAllocationId(event.target.value)}><option value="">Select…</option>{targetAllocations.map((allocation) => <option key={allocation.id} value={allocation.id}>{allocation.ip}:{allocation.port}</option>)}</select></label>
          {transfer?.transferring ? <Btn tone="danger" disabled={cancelMut.isPending} onClick={() => { void (async () => { if (await confirm({ title: "Cancel this transfer?", description: "The in-progress server transfer will be aborted.", danger: true, confirmLabel: "Cancel Transfer" })) cancelMut.mutate(); })(); }}>Cancel Transfer</Btn> : <Btn disabled={!targetNodeId || !primaryAllocationId || transferMut.isPending} onClick={() => { void (async () => { if (await confirm({ title: "Start this server transfer?", description: `The server will move to the selected node. Services may be interrupted.`, confirmLabel: "Start Transfer" })) transferMut.mutate(); })(); }}>Start Transfer</Btn>}
        </div>
      </Card>
      {renderConfirm()}
    </div>
  );
}

function ServerDeleteTab({ deleteMut, forceDeleteMut, serverName }: { deleteMut: { mutate: () => void; isPending: boolean }; forceDeleteMut: { mutate: () => void; isPending: boolean }; serverName: string }) {
  const [confirm, renderConfirm] = useConfirm();
  return (
    <div className="grid gap-4 md:grid-cols-2">
      <Card>
        <CardHeader title="Safely Delete" icon={Trash2} />
        <div className="space-y-3 p-4">
          <p className="text-xs text-slate-400">Removes the server record. Container and files are cleaned up by the daemon asynchronously.</p>
          <Btn tone="danger" onClick={() => { void (async () => { if (await confirm({ title: `Safely delete ${serverName}?`, description: "The server record will be removed and the container cleaned up by the daemon. This cannot be undone.", danger: true, confirmLabel: "Safely Delete" })) deleteMut.mutate(); })(); }} disabled={deleteMut.isPending}>
            {deleteMut.isPending ? "Deleting…" : "Safely Delete"}
          </Btn>
        </div>
      </Card>
      <Card>
        <CardHeader title="Force Delete" icon={AlertTriangle} action={<Pill tone="red">Danger</Pill>} />
        <div className="space-y-3 p-4">
          <p className="text-xs text-red-400">Bypasses daemon cleanup. Files may be orphaned on the node.</p>
          <Btn tone="danger" onClick={() => { void (async () => { if (await confirm({ title: `Force delete ${serverName}?`, description: "Daemon cleanup is bypassed. Files may be orphaned on the node. This cannot be undone.", danger: true, confirmLabel: "Force Delete" })) forceDeleteMut.mutate(); })(); }} disabled={forceDeleteMut.isPending}>
            {forceDeleteMut.isPending ? "Deleting…" : "Forcibly Delete"}
          </Btn>
        </div>
      </Card>
      {renderConfirm()}
    </div>
  );
}
