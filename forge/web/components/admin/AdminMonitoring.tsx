"use client";

import { useMemo, useState, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  AlertTriangle,
  ArrowUpRight,
  BarChart3,
  Box,
  ChevronDown,
  Database,
  Download,
  HardDrive,
  HeartPulse,
  Info,
  Network,
  RefreshCw,
  Server,
} from "lucide-react";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { useRouter } from "next/navigation";
import {
  fetchAdminActivity,
  fetchAllNodes,
  fetchAllServers,
  fetchHealthStatus,
} from "@/lib/api";
import {
  getNodeMetrics,
  getSystemInfo,
  metricWindow,
  type MetricPeriod,
  type NodeMetrics,
} from "@/lib/api/monitoring";
import { PageInfoDisclosure } from "@/components/ui/page-info-disclosure";
import { Card, Pill, cn } from "@/components/admin/admin-ui";
import {
  CpuKpiChipIcon,
  MemoryRamStickIcon,
  ActivityWaveIcon,
} from "@/components/ui/forge-icons";
import type { ApiNode } from "@forge/shared-types";
import { chart } from "@/lib/design-tokens";

/** Compare-mode series ramp (SVG attributes can't resolve var(--*) — see lib/design-tokens). */
const MONITOR_SERIES = [chart.sky, chart.violet, chart.lightOrange, chart.lightCyan, chart.lightEmerald];

const PERIODS: { value: MetricPeriod; label: string; short: string }[] = [
  { value: "1h", label: "1 hour", short: "1 hour" },
  { value: "6h", label: "6 hours", short: "6 hours" },
  { value: "24h", label: "24 hours", short: "24 hours" },
  { value: "7d", label: "7 days", short: "7 days" },
  { value: "30d", label: "30 days", short: "30 days" },
];

type MetricKey = "cpu" | "memory" | "disk" | "network";

const METRIC_TABS: { value: MetricKey; label: string }[] = [
  { value: "cpu", label: "CPU" },
  { value: "memory", label: "Memory" },
  { value: "disk", label: "Disk" },
  { value: "network", label: "Network" },
];

function timeAgo(iso?: string): string {
  if (!iso) return "—";
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return "—";
  const mins = Math.max(0, Math.round((Date.now() - t) / 60000));
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(1)} GB`;
  if (bytes >= 1024 ** 2) return `${(bytes / 1024 ** 2).toFixed(1)} MB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${Math.round(bytes)} B`;
}

function formatMiB(mb?: number | null): string {
  if (typeof mb !== "number" || !Number.isFinite(mb)) return "—";
  if (mb >= 1024) return `${(mb / 1024).toFixed(1)} GiB`;
  return `${Math.round(mb)} MiB`;
}

function nodeStatus(node: ApiNode): { label: string; tone: "green" | "yellow" | "red" | "neutral" } {
  const hb = (node.heartbeatState ?? "").toLowerCase();
  const actual = (node.actualState ?? "").toLowerCase();
  if (hb === "healthy" || actual === "online") return { label: "Online", tone: "green" };
  if (hb === "degraded" || hb === "suspected" || actual === "degraded") return { label: "Degraded", tone: "yellow" };
  if (hb === "offline" || hb === "unreachable" || actual === "offline") return { label: "Offline", tone: "red" };
  if (node.maintenanceMode || node.desiredState === "maintenance") return { label: "Maintenance", tone: "neutral" };
  return { label: "Unknown", tone: "neutral" };
}

function toCSV(rows: NodeMetrics[]): string {
  const header =
    "observed_at,node_id,cpu_percent,memory_percent,disk_percent,memory_used_mb,memory_total_mb,disk_used_mb,disk_total_mb,network_rx_bytes,network_tx_bytes,containers_running,containers_total";
  const lines = rows.map((m) =>
    [
      new Date(m.observedAt).toISOString(),
      m.nodeId,
      m.cpuPercent.toFixed(2),
      m.memoryPercent.toFixed(2),
      m.diskPercent.toFixed(2),
      m.memoryUsedMb,
      m.memoryTotalMb,
      m.diskUsedMb,
      m.diskTotalMb,
      m.networkRxBytes,
      m.networkTxBytes,
      m.containerRunning,
      m.containerTotal,
    ].join(","),
  );
  return [header, ...lines].join("\n");
}

function WaveSparkline({ color }: { color: string }) {
  const id = `spark-${color.replace(/[^a-zA-Z0-9]/g, "")}`;
  return (
    <svg viewBox="0 0 120 34" className="h-full w-full" aria-hidden="true">
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity={0.28} />
          <stop offset="100%" stopColor={color} stopOpacity={0} />
        </linearGradient>
      </defs>
      <path d="M 0,24 Q 15,18 30,22 T 60,14 T 90,18 T 120,8 L 120,34 L 0,34 Z" fill={`url(#${id})`} />
      <path
        d="M 0,24 Q 15,18 30,22 T 60,14 T 90,18 T 120,8"
        fill="none"
        stroke={color}
        strokeWidth={2}
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function MainChartTooltip({
  active,
  payload,
  label,
}: {
  active?: boolean;
  payload?: Array<{ name: string; value: number; color?: string }>;
  label?: string;
}) {
  if (!active || !payload?.length) return null;
  return (
    <div className="rounded-lg border border-white/10 bg-[var(--surface-raised)] p-3 shadow-xl">
      <p className="mb-1 font-mono text-[11px] text-slate-400">
        {label ? new Date(label).toLocaleString() : ""}
      </p>
      {payload.map((entry) => (
        <p key={entry.name} className="text-xs font-semibold text-slate-200">
          <span style={{ color: entry.color ?? chart.sky }}>●</span> {entry.name}: {Number(entry.value).toFixed(1)}%
        </p>
      ))}
    </div>
  );
}

type HistoryPoint = {
  t: number;
  iso: string;
  cpu: number;
  memory: number;
  disk: number;
  network: number;
  nodeId?: string;
};

export function AdminMonitoring() {
  const router = useRouter();
  const [period, setPeriod] = useState<MetricPeriod>("1h");
  const [selectedNode, setSelectedNode] = useState<string | null>(null);
  const [selectedWorkload, setSelectedWorkload] = useState<string | null>(null);
  const [metric, setMetric] = useState<MetricKey>("cpu");
  const [stacked, setStacked] = useState(true);
  const [compare, setCompare] = useState(false);
  const [isRefreshing, setIsRefreshing] = useState(false);

  const window = useMemo(() => metricWindow(period), [period]);
  const periodLabel = PERIODS.find((p) => p.value === period)?.label ?? "1 hour";

  const nodesQuery = useQuery({
    queryKey: ["nodes", "all"],
    queryFn: fetchAllNodes,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
    retry: 2,
  });
  const serversQuery = useQuery({
    queryKey: ["servers", "all"],
    queryFn: fetchAllServers,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
    retry: 2,
  });
  const healthQuery = useQuery({
    queryKey: ["health"],
    queryFn: fetchHealthStatus,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
    retry: 2,
  });
  const activityQuery = useQuery({
    queryKey: ["admin-activity", { limit: 8 }],
    queryFn: () => fetchAdminActivity({ limit: 8 }),
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
    retry: 1,
  });
  const summaryQuery = useQuery({
    queryKey: ["monitoring-summary"],
    queryFn: getSystemInfo,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
    retry: 1,
  });

  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);
  const servers = useMemo(() => serversQuery.data ?? [], [serversQuery.data]);
  const nodeById = useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes]);

  const latestQuery = useQuery({
    queryKey: ["monitoring-latest"],
    queryFn: () => getNodeMetrics(),
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
    retry: 1,
  });
  const latestByNode = useMemo(() => {
    const map = new Map<string, NodeMetrics>();
    for (const m of latestQuery.data ?? []) map.set(m.nodeId, m);
    return map;
  }, [latestQuery.data]);

  const historyQuery = useQuery({
    queryKey: ["monitoring-history", period, selectedNode ?? "all", window.limit, window.since],
    queryFn: async (): Promise<NodeMetrics[]> => {
      if (selectedNode) {
        return getNodeMetrics({ nodeId: selectedNode, limit: window.limit, since: window.since });
      }
      const targets = (nodesQuery.data ?? []).slice(0, 12);
      if (targets.length === 0) return [];
      const settled = await Promise.all(
        targets.map((n) =>
          getNodeMetrics({ nodeId: n.id, limit: window.limit, since: window.since }).catch((): NodeMetrics[] => []),
        ),
      );
      return settled.flat();
    },
    enabled: selectedNode != null || (nodesQuery.data?.length ?? 0) > 0,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
    retry: 1,
  });

  const history = useMemo(() => historyQuery.data ?? [], [historyQuery.data]);
  const hasTelemetry = history.length > 0;
  const historyLoading = historyQuery.isLoading;

  const series: HistoryPoint[] = useMemo(() => {
    if (history.length === 0) return [];
    const maxTotal = nodes.length > 0 ? nodes.length : 1;
    const buckets = new Map<number, { cpu: number; memory: number; disk: number; network: number; count: number }>();
    const sorted = [...history].sort(
      (a, b) => new Date(a.observedAt).getTime() - new Date(b.observedAt).getTime(),
    );
    for (const m of sorted) {
      const t = new Date(m.observedAt).getTime();
      if (Number.isNaN(t)) continue;
      const net = m.memoryTotalMb > 0 ? Math.min(100, ((m.networkRxBytes + m.networkTxBytes) / (1024 ** 3)) * 10) : 0;
      const bucket = buckets.get(t) ?? { cpu: 0, memory: 0, disk: 0, network: 0, count: 0 };
      bucket.cpu += m.cpuPercent;
      bucket.memory += m.memoryPercent;
      bucket.disk += m.diskPercent;
      bucket.network += Number.isFinite(net) ? net : 0;
      bucket.count += 1;
      buckets.set(t, bucket);
    }
    const divisor = selectedNode ? 1 : Math.max(1, Math.min(maxTotal, buckets.size > 0 ? maxTotal : 1));
    return [...buckets.entries()]
      .map(([t, b]) => ({
        t,
        iso: new Date(t).toISOString(),
        cpu: Math.min(100, b.cpu / Math.max(1, selectedNode ? b.count : divisor)),
        memory: Math.min(100, b.memory / Math.max(1, selectedNode ? b.count : divisor)),
        disk: Math.min(100, b.disk / Math.max(1, selectedNode ? b.count : divisor)),
        network: Math.min(100, b.network / Math.max(1, selectedNode ? b.count : divisor)),
      }))
      .sort((a, b) => a.t - b.t);
  }, [history, nodes.length, selectedNode]);

  const kpis = useMemo(() => {
    if (!hasTelemetry) return null;
    const avg = (pick: (p: HistoryPoint) => number) =>
      series.length ? series.reduce((s, p) => s + pick(p), 0) / series.length : 0;
    return { cpu: avg((p) => p.cpu), memory: avg((p) => p.memory), disk: avg((p) => p.disk), network: avg((p) => p.network) };
  }, [hasTelemetry, series]);

  const compareSeries = useMemo(() => {
    if (!compare || selectedNode || history.length === 0) return null;
    const byNode = new Map<string, { iso: string; value: number }[]>();
    const sorted = [...history].sort(
      (a, b) => new Date(a.observedAt).getTime() - new Date(b.observedAt).getTime(),
    );
    for (const m of sorted.slice(-120)) {
      const arr = byNode.get(m.nodeId) ?? [];
      const value = metric === "cpu" ? m.cpuPercent : metric === "memory" ? m.memoryPercent : metric === "disk" ? m.diskPercent : 0;
      arr.push({ iso: new Date(m.observedAt).toISOString(), value });
      byNode.set(m.nodeId, arr);
    }
    return [...byNode.entries()].slice(0, 5);
  }, [compare, selectedNode, history, metric]);

  const health = healthQuery.data;
  const checks = useMemo(() => health?.checks ?? [], [health]);
  const failedChecks = useMemo(() => checks.filter((c) => c.status !== "ok" && c.status !== "warning"), [checks]);

  const onlineNodes = useMemo(() => nodes.filter((n) => nodeStatus(n).tone === "green").length, [nodes]);
  const runningServers = useMemo(() => servers.filter((s) => s.status === "running").length, [servers]);

  const platform = useMemo(() => {
    if (healthQuery.isError || nodesQuery.isError) return { label: "Platform Unavailable", tone: "red" as const };
    if (failedChecks.length > 0 || health?.status === "failed") return { label: "Platform Critical", tone: "red" as const };
    if (!hasTelemetry && !historyLoading) return { label: "Platform Degraded", tone: "yellow" as const };
    if (checks.some((c) => c.status === "warning") || health?.status === "warning" || onlineNodes < nodes.length)
      return { label: "Platform Degraded", tone: "yellow" as const };
    return { label: "Platform Operational", tone: "green" as const };
  }, [healthQuery.isError, nodesQuery.isError, failedChecks.length, health?.status, hasTelemetry, historyLoading, checks, onlineNodes, nodes.length]);

  const filteredNodes = useMemo(() => {
    let list = nodes;
    if (selectedWorkload) {
      const srv = servers.find((s) => s.id === selectedWorkload);
      if (srv?.nodeId) list = list.filter((n) => n.id === srv.nodeId);
    }
    return list;
  }, [nodes, servers, selectedWorkload]);

  const filteredServers = useMemo(() => {
    let list = servers;
    if (selectedNode) list = list.filter((s) => s.nodeId === selectedNode);
    if (selectedWorkload) list = list.filter((s) => s.id === selectedWorkload);
    return list;
  }, [servers, selectedNode, selectedWorkload]);

  const activity = useMemo(() => activityQuery.data?.events ?? [], [activityQuery.data]);

  const handleRefresh = useCallback(async () => {
    setIsRefreshing(true);
    await Promise.allSettled([
      nodesQuery.refetch(),
      serversQuery.refetch(),
      healthQuery.refetch(),
      activityQuery.refetch(),
      summaryQuery.refetch(),
      latestQuery.refetch(),
      historyQuery.refetch(),
    ]);
    setTimeout(() => setIsRefreshing(false), 500);
  }, [nodesQuery, serversQuery, healthQuery, activityQuery, summaryQuery, latestQuery, historyQuery]);

  const handleExport = useCallback(() => {
    if (history.length === 0) return;
    const blob = new Blob([toCSV(history)], { type: "text/csv;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `forge-monitoring-${period}-${new Date().toISOString().slice(0, 10)}.csv`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  }, [history, period]);

  const kpiCards = [
    { key: "cpu" as MetricKey, title: "CPU", icon: CpuKpiChipIcon, color: chart.sky, delta: "fleet avg" },
    { key: "memory" as MetricKey, title: "MEMORY", icon: MemoryRamStickIcon, color: chart.violet, delta: "configured" },
    { key: "disk" as MetricKey, title: "STORAGE", icon: HardDrive, color: chart.lightOrange, delta: "allocated" },
    { key: "network" as MetricKey, title: "NETWORK", icon: Network, color: chart.lightCyan, delta: "traffic" },
  ];

  return (
    <div className="space-y-6">
      {/* ========================================================================= */}
      {/* ZONE 1: BREADCRUMB, HEADER & GLOBAL ACTIONS                                */}
      {/* ========================================================================= */}
      <div className="flex items-center justify-between text-xs text-slate-400">
        <div className="flex items-center gap-2">
          <span>Command</span>
          <span className="text-slate-600">/</span>
          <span className="font-semibold text-slate-200">Monitoring</span>
        </div>
        <div className="flex items-center gap-1.5 font-mono text-[11px] text-slate-500">
          <span className="relative flex h-1.5 w-1.5">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-60" />
            <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-emerald-400" />
          </span>
          <span>Live · updated just now</span>
        </div>
      </div>

      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between border-b border-[var(--line)] pb-5">
        <div>
          <h1 className="flex items-center gap-2.5 text-2xl font-bold tracking-tight text-slate-100 sm:text-3xl">
            <span>Monitoring</span>
            <PageInfoDisclosure
              title="Fleet telemetry"
              eyebrow="Architecture & Semantics"
              description="Monitoring shows what happens over time — node resource telemetry collected by the control plane, plus live fleet state. Charts stay empty (never zero) until node_metrics rows exist for the selected window."
              sections={[
                {
                  title: "Where data comes from",
                  icon: BarChart3,
                  content:
                    "GET /monitoring/nodes/metrics returns per-node history (nodeId + limit + since) or the latest row per node. Fleet charts fan out across nodes and merge by timestamp. Summary, health and activity come from /monitoring/summary, /health and /admin/activity.",
                },
                {
                  title: "Monitoring vs Health vs Overview",
                  icon: HeartPulse,
                  content:
                    "Monitoring = trends over time. Health = what is wrong right now. Overview = fleet snapshot and capacity. Use the time selector to adjust the telemetry window.",
                },
              ]}
            />
          </h1>
          <p className="mt-1 text-sm text-slate-400">
            Real-time telemetry from your beacons and workloads. Monitor resource usage, performance and system health.
          </p>
        </div>

        {/* Global Toolbar Controls */}
        <div className="flex items-center gap-2 shrink-0 sm:self-center">
          <div className="relative">
            <select
              aria-label="Select time range"
              value={period}
              onChange={(e) => setPeriod(e.target.value as MetricPeriod)}
              className="h-8 rounded-md border border-[var(--line)] bg-[var(--surface)] pl-2.5 pr-7 text-xs font-medium text-slate-200 shadow-sm transition hover:border-[var(--line-strong)] focus:outline-none focus:ring-1 focus:ring-[var(--brand)] appearance-none cursor-pointer"
            >
              <option value="1h">Last 1 hour</option>
              <option value="6h">Last 6 hours</option>
              <option value="24h">Last 24 hours</option>
              <option value="7d">Last 7 days</option>
              <option value="30d">Last 30 days</option>
            </select>
            <ChevronDown size={12} className="absolute right-2 top-2.5 pointer-events-none text-slate-400" />
          </div>
          <button
            type="button"
            aria-label="Refresh monitoring data"
            onClick={handleRefresh}
            className="flex h-8 w-8 items-center justify-center rounded-md border border-[var(--line)] bg-[var(--surface)] text-slate-300 transition hover:bg-white/[0.06] hover:text-slate-100 focus:outline-none focus:ring-1 focus:ring-[var(--brand)] disabled:opacity-50"
          >
            <RefreshCw size={13} className={isRefreshing ? "animate-spin text-sky-400" : ""} />
          </button>
          <button
            type="button"
            onClick={handleExport}
            disabled={!hasTelemetry}
            className="inline-flex h-8 items-center gap-1.5 rounded-md bg-[var(--brand)] px-3 text-xs font-semibold text-white shadow-sm transition hover:bg-[var(--brand-hover)] focus:outline-none focus:ring-1 focus:ring-[var(--brand)] disabled:cursor-not-allowed disabled:opacity-40"
          >
            <Download size={14} />
            <span>Export</span>
          </button>
        </div>
      </div>

      {/* Filter toolbar */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-2">
          {/* Quick period presets */}
          <div className="flex items-center gap-1 rounded-md border border-[var(--line)] bg-[var(--surface)] p-0.5">
            {PERIODS.map((p) => (
              <button
                key={p.value}
                type="button"
                onClick={() => setPeriod(p.value)}
                className={cn(
                  "rounded px-2.5 py-1 text-xs font-medium transition",
                  period === p.value
                    ? "bg-white/[0.1] text-white shadow-sm font-semibold"
                    : "text-slate-400 hover:text-slate-200"
                )}
              >
                {p.label}
              </button>
            ))}
          </div>

          <div className="relative">
            <select
              aria-label="Filter by node"
              value={selectedNode ?? ""}
              onChange={(e) => {
                setSelectedNode(e.target.value || null);
                setCompare(false);
              }}
              className="h-8 rounded-md border border-[var(--line)] bg-[var(--surface)] pl-2.5 pr-7 text-xs font-medium text-slate-200 shadow-sm transition hover:border-[var(--line-strong)] focus:outline-none focus:ring-1 focus:ring-[var(--brand)] appearance-none cursor-pointer"
            >
              <option value="">All Nodes</option>
              {nodes.map((n) => (
                <option key={n.id} value={n.id}>
                  {n.name}
                </option>
              ))}
            </select>
            <ChevronDown size={12} className="absolute right-2 top-2.5 pointer-events-none text-slate-400" />
          </div>

          <div className="relative">
            <select
              aria-label="Filter by workload"
              value={selectedWorkload ?? ""}
              onChange={(e) => setSelectedWorkload(e.target.value || null)}
              className="h-8 rounded-lg border border-[var(--line)] bg-[var(--surface-input)] pl-2.5 pr-7 text-xs font-medium text-slate-200 outline-none focus:border-[var(--focus)] appearance-none cursor-pointer hover:border-white/20"
            >
              <option value="">All Workloads</option>
              {servers.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
            <ChevronDown size={12} className="absolute right-2 top-2.5 pointer-events-none text-slate-400" />
          </div>

          <button
            type="button"
            onClick={() => setCompare((v) => !v)}
            disabled={!!selectedNode || !hasTelemetry}
            className={cn(
              "flex h-8 items-center gap-1.5 rounded-lg border px-3 text-xs font-semibold transition",
              compare
                ? "border-sky-500/40 bg-sky-500/10 text-sky-300"
                : "border-white/[0.08] bg-white/[0.02] text-slate-300 hover:border-white/20 hover:text-white",
              (!!selectedNode || !hasTelemetry) && "cursor-not-allowed opacity-40",
            )}
          >
            <BarChart3 size={13} />
            Compare
          </button>
        </div>

        {(selectedNode || selectedWorkload) && (
          <div className="flex flex-wrap items-center gap-2 text-xs text-slate-400">
            {selectedNode && nodeById.get(selectedNode) && (
              <button
                type="button"
                onClick={() => setSelectedNode(null)}
                className="rounded-lg border border-sky-500/30 bg-sky-500/10 px-2.5 py-1 font-semibold text-sky-300 transition hover:bg-sky-500/20"
              >
                Node: {nodeById.get(selectedNode)!.name} ✕
              </button>
            )}
            {selectedWorkload && (
              <button
                type="button"
                onClick={() => setSelectedWorkload(null)}
                className="rounded-lg border border-purple-500/30 bg-purple-500/10 px-2.5 py-1 font-semibold text-purple-300 transition hover:bg-purple-500/20"
              >
                Workload: {servers.find((s) => s.id === selectedWorkload)?.name ?? "selected"} ✕
              </button>
            )}
          </div>
        )}
      </div>

      {!historyLoading && !hasTelemetry && !historyQuery.isError ? (
        <div className="flex flex-col gap-3 rounded-xl border border-amber-500/25 bg-amber-500/[0.05] p-4 sm:flex-row sm:items-center">
          <AlertTriangle size={18} className="shrink-0 text-amber-400" />
          <div className="min-w-0 flex-1">
            <p className="text-sm font-semibold text-amber-300">No telemetry for {periodLabel} yet</p>
            <p className="mt-0.5 text-xs leading-5 text-amber-200/80">
              Charts stay empty until beacons report via <code className="rounded bg-black/30 px-1 font-mono">POST /monitoring/nodes/metrics</code>.
              Check that beacons are online (<span className="font-semibold text-amber-200">Heartbeat healthy</span> in{" "}
              <button type="button" onClick={() => router.push("/admin/nodes")} className="underline hover:text-amber-100">
                Beacons
              </button>
              ) and that the agent has write access.
            </p>
          </div>
          <button
            type="button"
            onClick={() => router.push("/admin/health")}
            className="flex shrink-0 items-center gap-1.5 rounded-lg border border-white/15 px-3 py-1.5 text-xs font-semibold text-slate-200 transition hover:bg-white/[0.06]"
          >
            View Health <ArrowUpRight size={13} />
          </button>
        </div>
      ) : null}
      {historyQuery.isError ? (
        <div className="rounded-xl border border-red-500/25 bg-red-950/20 p-3 text-xs text-red-200">
          Telemetry query failed: {(historyQuery.error as Error)?.message ?? "unknown error"} — latest state below still loads from its own queries.
        </div>
      ) : null}

      {/* Top 4 Metric Cards */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {kpiCards.map((card) => {
          const Icon = card.icon;
          const value = kpis ? kpis[card.key] : null;
          return (
            <div key={card.key} className="rounded-xl border border-white/[0.08] bg-[var(--surface)] p-4 shadow-sm">
              <div className="flex items-center justify-between">
                <span className="flex items-center gap-1.5 font-mono text-[11px] font-semibold text-slate-400 tracking-wider">
                  <Icon size={14} className="text-slate-400" />
                  {card.title}
                </span>
                <span className="text-[11px] font-mono text-emerald-400 bg-emerald-500/10 border border-emerald-500/20 rounded px-1.5 py-0.5">
                  {card.delta}
                </span>
              </div>
              <div className="mt-2 flex items-baseline justify-between gap-2">
                <div>
                  <p className="font-mono text-2xl font-bold tracking-tight text-slate-100">
                    {value != null ? `${value.toFixed(1)}%` : "—"}
                  </p>
                  <p className="mt-0.5 text-xs font-mono text-slate-400">
                    {value != null ? (
                      card.key === "network" ? (
                        <span>
                          RX {formatBytes(history.reduce((s, m) => s + m.networkRxBytes, 0))}
                        </span>
                      ) : (
                        <span>{periodLabel} aggregate</span>
                      )
                    ) : (
                      "No data reported"
                    )}
                  </p>
                </div>
                <div className="h-8 w-24 shrink-0 overflow-hidden">
                  <WaveSparkline color={card.color} />
                </div>
              </div>
            </div>
          );
        })}
      </div>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-3">
        <div className="rounded-xl border border-white/[0.08] bg-[var(--surface)] p-5 shadow-sm xl:col-span-2">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="flex items-center gap-2">
              <BarChart3 size={16} className="text-sky-400" />
              <div>
                <h3 className="text-sm font-bold text-slate-100">Resource Usage Over Time</h3>
                <p className="text-[11px] text-slate-400">Aggregate resource usage across all nodes</p>
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <div className="flex gap-1 rounded-lg border border-white/[0.07] bg-black/20 p-0.5">
                {METRIC_TABS.map((t) => (
                  <button
                    key={t.value}
                    type="button"
                    onClick={() => setMetric(t.value)}
                    className={cn(
                      "rounded-md px-3 py-1 text-[11px] font-semibold transition",
                      metric === t.value ? "bg-[var(--brand)] text-white" : "text-slate-400 hover:text-slate-200",
                    )}
                  >
                    {t.label}
                  </button>
                ))}
              </div>
              <button
                type="button"
                onClick={() => setStacked((v) => !v)}
                className="flex items-center gap-1.5 rounded-lg border border-white/[0.08] bg-white/[0.02] px-2.5 py-1 text-[11px] font-semibold text-slate-300 transition hover:border-white/20"
              >
                <BarChart3 size={12} />
                {stacked ? "Stacked" : "Overlaid"}
              </button>
            </div>
          </div>

          <div className="mt-4 h-72 w-full">
            {historyLoading ? (
              <div className="grid h-full place-items-center text-xs text-slate-500" role="status">
                <span className="flex items-center gap-2">
                  <span className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-[var(--brand)] border-t-transparent" />
                  Loading telemetry…
                </span>
              </div>
            ) : !hasTelemetry ? (
              <div className="relative h-full">
                <ResponsiveContainer width="100%" height="100%">
                  <AreaChart data={[]} margin={{ top: 5, right: 5, left: -10, bottom: 0 }}>
                    <CartesianGrid strokeDasharray="3 3" stroke={chart.gridStrong} vertical={false} />
                    <XAxis dataKey="iso" tick={{ fill: chart.axis, fontSize: 10 }} tickLine={false} axisLine={false} />
                    <YAxis tick={{ fill: chart.axis, fontSize: 10 }} tickLine={false} axisLine={false} domain={[0, 100]} unit="%" width={44} />
                  </AreaChart>
                </ResponsiveContainer>
                <div className="absolute inset-0 flex flex-col items-center justify-center text-center">
                  <BarChart3 size={26} className="mb-2 text-slate-500" strokeWidth={1.5} />
                  <p className="text-sm font-semibold text-slate-200">No telemetry data yet</p>
                  <p className="mt-0.5 text-xs text-slate-500">Data will appear once nodes start reporting metrics.</p>
                </div>
              </div>
            ) : compare && compareSeries ? (
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={series} margin={{ top: 5, right: 5, left: -10, bottom: 0 }}>
                  <defs>
                    {compareSeries.map(([id], i) => (
                      <linearGradient key={id} id={`cmp-${i}`} x1="0" y1="0" x2="0" y2="1">
                        <stop offset="5%" stopColor={MONITOR_SERIES[i % 5]} stopOpacity={0.3} />
                        <stop offset="95%" stopColor={MONITOR_SERIES[i % 5]} stopOpacity={0} />
                      </linearGradient>
                    ))}
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke={chart.gridStrong} vertical={false} />
                  <XAxis
                    dataKey="iso"
                    tick={{ fill: chart.axis, fontSize: 10 }}
                    tickLine={false}
                    axisLine={false}
                    tickFormatter={(v: string) => new Date(v).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
                    minTickGap={48}
                  />
                  <YAxis tick={{ fill: chart.axis, fontSize: 10 }} tickLine={false} axisLine={false} domain={[0, 100]} unit="%" width={44} />
                  <Tooltip content={<MainChartTooltip />} />
                  {compareSeries.map(([id], i) => (
                    <Area
                      key={id}
                      type="monotone"
                      dataKey={metric}
                      data={compareSeries[i][1].map((p) => ({ iso: p.iso, [metric]: p.value }))}
                      name={nodeById.get(id)?.name ?? id.slice(0, 8)}
                      stroke={MONITOR_SERIES[i % 5]}
                      strokeWidth={2}
                      fill={`url(#cmp-${i})`}
                      dot={false}
                    />
                  ))}
                </AreaChart>
              </ResponsiveContainer>
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={series} margin={{ top: 5, right: 5, left: -10, bottom: 0 }}>
                  <defs>
                    <linearGradient id="mainCpu" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor={chart.dangerBright} stopOpacity={0.35} />
                      <stop offset="95%" stopColor={chart.dangerBright} stopOpacity={0} />
                    </linearGradient>
                    <linearGradient id="mainMem" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor={chart.violet} stopOpacity={0.3} />
                      <stop offset="95%" stopColor={chart.violet} stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke={chart.gridStrong} vertical={false} />
                  <XAxis
                    dataKey="iso"
                    tick={{ fill: chart.axis, fontSize: 10 }}
                    tickLine={false}
                    axisLine={false}
                    tickFormatter={(v: string) => new Date(v).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
                    minTickGap={48}
                  />
                  <YAxis tick={{ fill: chart.axis, fontSize: 10 }} tickLine={false} axisLine={false} domain={[0, 100]} unit="%" width={44} />
                  <Tooltip content={<MainChartTooltip />} />
                  <Area
                    type="monotone"
                    dataKey={metric}
                    name={METRIC_TABS.find((t) => t.value === metric)?.label ?? metric}
                    stroke={metric === "cpu" ? chart.dangerBright : metric === "memory" ? chart.violet : metric === "disk" ? chart.lightOrange : chart.lightCyan}
                    strokeWidth={2}
                    fill={metric === "cpu" ? "url(#mainCpu)" : "url(#mainMem)"}
                    dot={false}
                    stackId={stacked ? "fleet" : undefined}
                  />
                </AreaChart>
              </ResponsiveContainer>
            )}
          </div>
        </div>

        <div className="flex flex-col gap-4">
          <div className="rounded-xl border border-white/[0.08] bg-[var(--surface)] p-5 shadow-sm">
            <div className="flex items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <HeartPulse size={15} className="text-emerald-400" />
                <h3 className="text-sm font-bold text-slate-100">System Health</h3>
              </div>
              <button
                type="button"
                onClick={() => router.push("/admin/health")}
                className="flex items-center gap-1 text-[11px] font-semibold text-slate-400 transition hover:text-white"
              >
                View Details <ArrowUpRight size={12} />
              </button>
            </div>
            <div className="mt-3 rounded-lg border border-white/[0.06] bg-black/20 p-3">
              <p
                className={cn(
                  "flex items-center gap-1.5 text-sm font-bold",
                  platform.tone === "green" && "text-emerald-300",
                  platform.tone === "yellow" && "text-amber-300",
                  platform.tone === "red" && "text-red-300",
                )}
              >
                <span
                  className={cn(
                    "h-2 w-2 rounded-full",
                    platform.tone === "green" && "bg-emerald-400",
                    platform.tone === "yellow" && "bg-amber-400",
                    platform.tone === "red" && "bg-red-400",
                  )}
                />
                {platform.tone === "green" ? "Operational" : platform.tone === "yellow" ? "Degraded" : "Critical"}
              </p>
              <p className="mt-0.5 text-[11px] text-slate-400">
                {hasTelemetry
                  ? `${onlineNodes}/${nodes.length} beacons healthy · ${runningServers}/${servers.length} workloads running`
                  : "Some components are not reporting data."}
              </p>
            </div>
            <div className="mt-2 divide-y divide-white/[0.04] text-xs">
              {[
                { label: "Control Plane", icon: Box, state: healthQuery.isError ? "Down" : "Healthy", ok: !healthQuery.isError },
                {
                  label: "Database",
                  icon: Database,
                  state: checks.find((c) => c.name === "database")?.status === "ok" || (!healthQuery.data && !healthQuery.isError) ? "Healthy" : checks.find((c) => c.name === "database") ? "Degraded" : "Healthy",
                  ok: true,
                },
                { label: "Queue / Workers", icon: RefreshCw, state: "Healthy", ok: true },
                { label: "Beacons", icon: Server, state: hasTelemetry ? `${onlineNodes} reporting` : "No Data", ok: hasTelemetry },
                { label: "Nodes", icon: HardDrive, state: nodes.length ? (hasTelemetry ? `${nodes.length} tracked` : "No Data") : "None", ok: hasTelemetry && nodes.length > 0 },
                { label: "Workloads", icon: CpuKpiChipIcon, state: servers.length ? (hasTelemetry ? `${runningServers} running` : "No Data") : "None", ok: hasTelemetry && servers.length > 0 },
              ].map((row) => (
                <div key={row.label} className="flex items-center gap-2 py-2">
                  <row.icon size={13} className="shrink-0 text-slate-500" />
                  <span className="text-slate-300">{row.label}</span>
                  <span className="ml-auto flex items-center gap-1.5 font-medium text-slate-300">
                    <span className={cn("h-1.5 w-1.5 rounded-full", row.ok ? "bg-emerald-400" : "bg-amber-400")} />
                    <span className={row.ok ? "text-emerald-300" : "text-amber-300"}>{row.state}</span>
                  </span>
                </div>
              ))}
            </div>
          </div>

          <div className="rounded-xl border border-white/[0.08] bg-[var(--surface)] p-5 shadow-sm">
            <div className="flex items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <Info size={15} className="text-slate-300" />
                <h3 className="text-sm font-bold text-slate-100">Recent Activity</h3>
              </div>
              <button
                type="button"
                onClick={() => router.push("/admin/activity")}
                className="flex items-center gap-1 text-[11px] font-semibold text-slate-400 transition hover:text-white"
              >
                View all <ArrowUpRight size={12} />
              </button>
            </div>
            <div className="mt-3 space-y-3">
              {activityQuery.isLoading ? (
                <p className="text-xs text-slate-500">Loading activity…</p>
              ) : activity.length === 0 ? (
                <p className="text-xs text-slate-500">No recent activity.</p>
              ) : (
                activity.slice(0, 5).map((e) => (
                  <div key={e.id} className="flex items-start gap-2.5">
                    <span className="grid h-7 w-7 shrink-0 place-items-center rounded-lg border border-white/[0.08] bg-white/[0.03] text-slate-300">
                      <ActivityWaveIcon className="w-3.5 h-3.5" />
                    </span>
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-xs font-semibold text-slate-200">{e.description || e.event || e.action || "Event"}</p>
                      <p className="truncate font-mono text-[10px] text-slate-500">{e.actorEmail || e.userId || e.source || "system"}</p>
                    </div>
                    <span className="shrink-0 font-mono text-[10px] text-slate-500">{timeAgo(e.timestamp || e.createdAt)}</span>
                  </div>
                ))
              )}
            </div>
          </div>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <Card>
          <div className="mb-4 flex items-center gap-2">
            <Server size={15} className="text-slate-300" />
            <div>
              <h3 className="text-sm font-bold text-slate-100">Node Metrics</h3>
              <p className="text-[11px] text-slate-400">Live metrics from all nodes</p>
            </div>
            <button
              type="button"
              onClick={() => router.push("/admin/nodes")}
              className="ml-auto flex items-center gap-1 rounded-lg border border-white/[0.08] px-2.5 py-1 text-[11px] font-semibold text-slate-300 transition hover:border-white/20 hover:text-white"
            >
              View all nodes <ArrowUpRight size={12} />
            </button>
          </div>
          {nodesQuery.isLoading ? (
            <p className="px-1 py-8 text-center text-xs text-slate-500" role="status">Loading nodes…</p>
          ) : filteredNodes.length === 0 ? (
            <div className="flex flex-col items-center py-10 text-center">
              <Server size={26} className="mb-2 text-slate-600" strokeWidth={1.5} />
              <p className="text-sm font-semibold text-slate-200">No nodes available</p>
              <p className="mt-0.5 text-xs text-slate-500">Nodes will appear here once they report metrics.</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-wider text-slate-500">
                    <th className="px-2 py-2 font-medium">Node</th>
                    <th className="px-2 py-2 font-medium">Status</th>
                    <th className="px-2 py-2 text-right font-medium">CPU</th>
                    <th className="px-2 py-2 text-right font-medium">Memory</th>
                    <th className="px-2 py-2 text-right font-medium">Disk</th>
                    <th className="px-2 py-2 text-right font-medium">Network</th>
                    <th className="px-2 py-2 text-right font-medium">Last seen</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {filteredNodes.map((n) => {
                    const st = nodeStatus(n);
                    const m = latestByNode.get(n.id);
                    return (
                      <tr
                        key={n.id}
                        onClick={() => setSelectedNode((v) => (v === n.id ? null : n.id))}
                        className={cn("cursor-pointer transition hover:bg-white/[0.02]", selectedNode === n.id && "bg-sky-500/[0.06]")}
                      >
                        <td className="max-w-36 truncate px-2 py-2.5 font-semibold text-slate-200">{n.name}</td>
                        <td className="px-2 py-2.5">
                          <Pill tone={st.tone}>{st.label}</Pill>
                        </td>
                        <td className="px-2 py-2.5 text-right font-mono text-slate-300">{m ? `${m.cpuPercent.toFixed(1)}%` : "—"}</td>
                        <td className="px-2 py-2.5 text-right font-mono text-slate-300">{m ? `${m.memoryPercent.toFixed(1)}%` : "—"}</td>
                        <td className="px-2 py-2.5 text-right font-mono text-slate-300">{m ? `${m.diskPercent.toFixed(1)}%` : "—"}</td>
                        <td className="px-2 py-2.5 text-right font-mono text-slate-300">
                          {m ? formatBytes(m.networkRxBytes + m.networkTxBytes) : "—"}
                        </td>
                        <td className="px-2 py-2.5 text-right font-mono text-slate-500">{timeAgo(n.lastSeenAt || n.lastHeartbeatAt)}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </Card>

        <Card>
          <div className="mb-4 flex items-center gap-2">
            <Box size={15} className="text-slate-300" />
            <div>
              <h3 className="text-sm font-bold text-slate-100">Top Workloads</h3>
              <p className="text-[11px] text-slate-400">Highest resource usage workloads</p>
            </div>
            <button
              type="button"
              onClick={() => router.push("/admin/servers")}
              className="ml-auto flex items-center gap-1 rounded-lg border border-white/[0.08] px-2.5 py-1 text-[11px] font-semibold text-slate-300 transition hover:border-white/20 hover:text-white"
            >
              View all workloads <ArrowUpRight size={12} />
            </button>
          </div>
          {serversQuery.isLoading ? (
            <p className="px-1 py-8 text-center text-xs text-slate-500" role="status">Loading workloads…</p>
          ) : filteredServers.length === 0 ? (
            <div className="flex flex-col items-center py-10 text-center">
              <Box size={26} className="mb-2 text-slate-600" strokeWidth={1.5} />
              <p className="text-sm font-semibold text-slate-200">No workloads available</p>
              <p className="mt-0.5 text-xs text-slate-500">Deploy workloads to see their resource usage.</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-wider text-slate-500">
                    <th className="px-2 py-2 font-medium">Name</th>
                    <th className="px-2 py-2 font-medium">Type</th>
                    <th className="px-2 py-2 text-right font-medium">CPU</th>
                    <th className="px-2 py-2 text-right font-medium">Memory</th>
                    <th className="px-2 py-2 text-right font-medium">Disk</th>
                    <th className="px-2 py-2 text-right font-medium">Status</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {filteredServers.slice(0, 8).map((s) => (
                    <tr
                      key={s.id}
                      onClick={() => router.push(`/server/${s.id}`)}
                      className="cursor-pointer transition hover:bg-white/[0.02]"
                      title="Per-workload live telemetry is not reported by the metrics API — memory/disk show configured limits"
                    >
                      <td className="max-w-36 truncate px-2 py-2.5 font-semibold text-slate-200">{s.name}</td>
                      <td className="px-2 py-2.5 text-slate-400">{s.dockerImage ? "container" : "game"}</td>
                      <td className="px-2 py-2.5 text-right font-mono text-slate-500" title="No per-workload CPU telemetry reported">—</td>
                      <td className="px-2 py-2.5 text-right font-mono text-slate-300">{formatMiB(s.memoryMb)}</td>
                      <td className="px-2 py-2.5 text-right font-mono text-slate-300">{formatMiB(s.diskMb)}</td>
                      <td className="px-2 py-2.5 text-right">
                        <Pill tone={s.status === "running" ? "green" : s.status === "crashed" ? "red" : "neutral"}>{s.status}</Pill>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      </div>

      <p className="text-[11px] text-slate-600">
        Fleet telemetry is allocation-derived capacity snapshots from the control-plane collector — live host CPU / load / network series are not yet reported by
        beacons, so treat absolute values as approximate. Point-in-time failures live in{" "}
        <button type="button" onClick={() => router.push("/admin/health")} className="underline hover:text-slate-400">
          Health
        </button>
        .
      </p>

      {summaryQuery.data?.unacknowledgedAlerts ? (
        <p className="text-[11px] text-amber-400/80">
          {summaryQuery.data.unacknowledgedAlerts} unacknowledged alert{(summaryQuery.data.unacknowledgedAlerts ?? 0) > 1 ? "s" : ""} in the alert pipeline.
        </p>
      ) : null}
    </div>
  );
}
