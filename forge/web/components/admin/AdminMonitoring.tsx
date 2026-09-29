"use client";

import { useMemo, useState, useCallback, type ComponentType } from "react";
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
  type MetricPeriod,
  type NodeMetrics,
} from "@/lib/api/monitoring";
import {
  REFRESH,
  checkVerdict,
  findCheck,
  isAvailable,
  sourceState,
  summaryIsTrustworthy,
  useActivityQuery,
  useHealthQuery,
  useLatestNodeMetricsQuery,
  useMonitoringSummaryQuery,
  useNodeMetricsHistoryQuery,
  useNodesQuery,
  useServersQuery,
  worstSourceState,
} from "@/lib/admin/telemetry";
import { AdminPageToolbar } from "./admin-page-toolbar";
import { AdminPageLayout, AdminTable, AdminTBody, AdminTd, AdminTh, AdminTHead, AdminTr, SectionHeader, Btn, Card, Pill, cn } from "@/components/admin/admin-ui";
import { FreshnessBadge } from "./telemetry-ui";
import { resolveTone, toneStyles, type ToneInput } from "@/components/ui/forge/status";
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

// No `network` key. The control plane hardcodes network_rx_bytes and
// network_tx_bytes to 0 when it writes node_metrics (see
// forge/api/internal/services/observability/service.go collectNodeMetrics), so
// a network series would be a flat zero line dressed up as traffic.
type MetricKey = "cpu" | "memory" | "disk";

const METRIC_TABS: { value: MetricKey; label: string }[] = [
  { value: "cpu", label: "CPU" },
  { value: "memory", label: "Memory" },
  { value: "disk", label: "Disk" },
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

/**
 * KPI card sparkline, plotted from the card's own samples.
 *
 * This used to draw one fixed decorative wave on every card, which read as a
 * trend while being the same hand-written path regardless of the data — and it
 * was drawn even when no node had reported anything. It now renders only when
 * there are at least two real points, so a card with no history shows no line.
 */
function Sparkline({ color, points }: { color: string; points: number[] }) {
  if (points.length < 2) return null;
  const id = `spark-${color.replace(/[^a-zA-Z0-9]/g, "")}`;
  const w = 120;
  const h = 34;
  // Percent metrics: pin the scale to 0–100 so cards stay comparable and a
  // flat-but-low series does not look like a full one.
  const y = (v: number) => h - (Math.max(0, Math.min(100, v)) / 100) * (h - 2) - 1;
  const step = w / (points.length - 1);
  const line = points.map((v, i) => `${i === 0 ? "M" : "L"} ${(i * step).toFixed(2)},${y(v).toFixed(2)}`).join(" ");
  return (
    <svg viewBox={`0 0 ${w} ${h}`} className="h-full w-full" aria-hidden="true">
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity={0.28} />
          <stop offset="100%" stopColor={color} stopOpacity={0} />
        </linearGradient>
      </defs>
      <path d={`${line} L ${w},${h} L 0,${h} Z`} fill={`url(#${id})`} />
      <path d={line} fill="none" stroke={color} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" />
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

  const periodLabel = PERIODS.find((p) => p.value === period)?.label ?? "1 hour";

  const nodesQuery = useNodesQuery();
  const serversQuery = useServersQuery();
  // See the note in AdminOverview: one canonical key for the health report.
  const healthQuery = useHealthQuery();
  const activityQuery = useActivityQuery(8);
  const summaryQuery = useMonitoringSummaryQuery();
  const summaryTrustworthy = summaryIsTrustworthy(summaryQuery.data, {
    nodeCount: nodesQuery.data?.length,
    serverCount: serversQuery.data?.length,
  });

  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);
  const servers = useMemo(() => serversQuery.data ?? [], [serversQuery.data]);
  const nodeById = useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes]);

  const latestQuery = useLatestNodeMetricsQuery();
  const latestByNode = useMemo(() => {
    const map = new Map<string, NodeMetrics>();
    for (const m of latestQuery.data ?? []) map.set(m.nodeId, m);
    return map;
  }, [latestQuery.data]);

  // The fan-out reports which nodes failed and which were never queried instead
  // of swallowing per-node errors into an empty series. A window with no rows
  // is missing history, not an idle fleet, and every-node-failed is an error.
  const historyQuery = useNodeMetricsHistoryQuery(
    period,
    selectedNode,
    nodes,
    selectedNode != null || nodes.length > 0,
  );

  const history = useMemo(() => historyQuery.data?.rows ?? [], [historyQuery.data]);
  const historyFailedNodes = historyQuery.data?.failedNodeIds ?? [];
  const historySkippedNodes = historyQuery.data?.skippedNodeIds ?? [];
  const hasTelemetry = history.length > 0;
  const historyLoading = historyQuery.isPending;

  const series: HistoryPoint[] = useMemo(() => {
    if (history.length === 0) return [];
    const buckets = new Map<number, { cpu: number; memory: number; disk: number; count: number }>();
    const sorted = [...history].sort(
      (a, b) => new Date(a.observedAt).getTime() - new Date(b.observedAt).getTime(),
    );
    for (const m of sorted) {
      const t = new Date(m.observedAt).getTime();
      if (Number.isNaN(t)) continue;
      const bucket = buckets.get(t) ?? { cpu: 0, memory: 0, disk: 0, count: 0 };
      bucket.cpu += m.cpuPercent;
      bucket.memory += m.memoryPercent;
      bucket.disk += m.diskPercent;
      bucket.count += 1;
      buckets.set(t, bucket);
    }
    // Each bucket is divided by how many nodes actually reported at that
    // timestamp. The previous divisor was the fleet size, which silently scaled
    // every average down whenever a node had not reported — a node that said
    // nothing was counted as a node at 0%.
    return [...buckets.entries()]
      .map(([t, b]) => ({
        t,
        iso: new Date(t).toISOString(),
        cpu: Math.min(100, b.cpu / b.count),
        memory: Math.min(100, b.memory / b.count),
        disk: Math.min(100, b.disk / b.count),
      }))
      .sort((a, b) => a.t - b.t);
  }, [history]);

  const kpis = useMemo(() => {
    if (series.length === 0) return null;
    const avg = (pick: (p: HistoryPoint) => number) =>
      series.reduce((s, p) => s + pick(p), 0) / series.length;
    return { cpu: avg((p) => p.cpu), memory: avg((p) => p.memory), disk: avg((p) => p.disk) };
  }, [series]);

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

  // The verdict is about the platform's reported health, so it is only stated
  // once health has been read. Missing metric history is a gap in this page's
  // own data and no longer counts as a degraded platform — the coverage notice
  // under the chart says so instead.
  const platform = useMemo(() => {
    if (healthQuery.isError || nodesQuery.isError) return { label: "Platform Unavailable", tone: "red" as const };
    if (healthQuery.isPending || nodesQuery.isPending) return { label: "Reading Platform Health…", tone: "neutral" as const };
    if (failedChecks.length > 0 || health?.status === "failed") return { label: "Platform Critical", tone: "red" as const };
    if (checks.some((c) => c.status === "warning") || health?.status === "warning" || onlineNodes < nodes.length)
      return { label: "Platform Degraded", tone: "yellow" as const };
    return { label: "Platform Operational", tone: "green" as const };
  }, [healthQuery.isError, healthQuery.isPending, nodesQuery.isError, nodesQuery.isPending, failedChecks.length, health?.status, checks, onlineNodes, nodes.length]);

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

  const pageState = useMemo(
    () =>
      worstSourceState([
        sourceState(nodesQuery, REFRESH.inventory),
        sourceState(serversQuery, REFRESH.inventory),
        sourceState(healthQuery, REFRESH.health),
        sourceState(latestQuery, REFRESH.telemetry),
      ]),
    [nodesQuery, serversQuery, healthQuery, latestQuery],
  );

  const kpiCards = [
    { key: "cpu" as MetricKey, title: "CPU ALLOCATED", icon: CpuKpiChipIcon, color: chart.sky, note: "allocation" },
    { key: "memory" as MetricKey, title: "MEMORY ALLOCATED", icon: MemoryRamStickIcon, color: chart.violet, note: "allocation" },
    { key: "disk" as MetricKey, title: "STORAGE ALLOCATED", icon: HardDrive, color: chart.lightOrange, note: "allocation" },
  ];

  return (
    <AdminPageLayout>
      <div className="flex items-center justify-end">
        <FreshnessBadge state={pageState} />
      </div>

      <SectionHeader
        title="Monitoring"
        sub="Platform, node and workload health dashboards"
        info={{
          title: "Fleet allocation over time",
          triggerLabel: "About Monitoring",
          eyebrow: "Architecture & Semantics",
          description: "Monitoring shows what happens over time. The control plane derives each node_metrics row from that node's capacity snapshot, so the CPU / memory / disk series are allocated shares of capacity, not measured host load. Charts stay empty (never zero) until rows exist for the selected window.",
          sections: [
            {
              title: "Where data comes from",
              icon: BarChart3,
              content:
                "GET /monitoring/nodes/metrics returns per-node history (nodeId + limit + since) or the latest row per node. Fleet charts fan out across nodes and merge by timestamp, and say when a node is missing from the merge. Network byte counters are written as zero by the collector, so no network series is shown. Summary, health and activity come from /monitoring/summary, /health and /admin/activity.",
            },
            {
              title: "Monitoring vs Health vs Overview",
              icon: HeartPulse,
              content:
                "Monitoring = trends over time. Health = what is wrong right now. Overview = fleet snapshot and capacity. Use the time selector to adjust the telemetry window.",
            },
          ],
        }}
        action={<AdminPageToolbar range={{ value: period, onChange: (value) => setPeriod(value as MetricPeriod), options: PERIODS.map((item) => ({ value: item.value, label: `Last ${item.label}` })) }} onRefresh={handleRefresh} refreshing={isRefreshing} refreshLabel="Refresh monitoring data">
            <Btn size="sm" onClick={handleExport} disabled={!hasTelemetry}><Download size={14} /> Export</Btn>
          </AdminPageToolbar>}
      />

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
            <p className="text-sm font-semibold text-amber-300">No samples recorded for {periodLabel}</p>
            <p className="mt-0.5 text-xs leading-5 text-amber-200/80">
              Charts stay empty rather than flat at zero. The control plane&apos;s observability collector writes one{" "}
              <code className="rounded bg-black/30 px-1 font-mono">node_metrics</code> row per node from that node&apos;s
              capacity snapshot, so a window with no rows means the collector did not run or the nodes were not
              reachable — check that nodes are heartbeating in{" "}
              <button type="button" onClick={() => router.push("/admin/nodes")} className="underline hover:text-amber-100">
                Nodes
              </button>
              .
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
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-red-500/25 bg-red-950/20 p-3 text-xs text-red-200" role="alert">
          <span>Telemetry query failed: {(historyQuery.error as Error)?.message ?? "unknown error"} — latest state below still loads from its own queries.</span>
          <Btn size="sm" tone="ghost" onClick={() => void historyQuery.refetch()}>Retry</Btn>
        </div>
      ) : null}

      {/* Allocation KPI cards */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
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
                <span className="rounded border border-white/10 bg-white/[0.04] px-1.5 py-0.5 font-mono text-[11px] text-slate-400">
                  {card.note}
                </span>
              </div>
              <div className="mt-2 flex items-baseline justify-between gap-2">
                <div>
                  <p className="font-mono text-2xl font-bold tracking-tight text-slate-100">
                    {value != null ? `${value.toFixed(1)}%` : "—"}
                  </p>
                  <p className="mt-0.5 font-mono text-xs text-slate-400">
                    {value != null
                      ? `${periodLabel} aggregate`
                      : historyQuery.isError
                        ? "Series unavailable"
                        : historyLoading
                          ? "Reading series…"
                          : "No node reported this metric"}
                  </p>
                </div>
                <div className="h-8 w-24 shrink-0 overflow-hidden">
                  {/* No line at all when there is nothing to plot. */}
                  <Sparkline color={card.color} points={series.map((p) => p[card.key])} />
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
                <h3 className="text-sm font-bold text-slate-100">Allocation Over Time</h3>
                <p className="text-[11px] text-slate-400">
                  Allocated share of node capacity, averaged across the nodes that reported — not measured host load
                </p>
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
                  Reading allocation series…
                </span>
              </div>
            ) : historyQuery.isError ? (
              <div className="grid h-full place-items-center px-6 text-center">
                <div>
                  <AlertTriangle size={26} className="mx-auto mb-2 text-amber-400" strokeWidth={1.5} />
                  <p className="text-sm font-semibold text-slate-200">Allocation history unavailable</p>
                  <p className="mx-auto mt-1 max-w-md text-xs text-slate-500">
                    {(historyQuery.error as Error)?.message ?? "The metric history query failed."} Nothing is plotted
                    here — this is not a fleet at zero.
                  </p>
                  <button
                    type="button"
                    onClick={() => void historyQuery.refetch()}
                    className="mt-3 rounded-lg border border-white/15 px-3 py-1.5 text-xs font-semibold text-slate-200 transition hover:bg-white/[0.06]"
                  >
                    Retry
                  </button>
                </div>
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
                  {/* The banner above already states that no samples exist in
                      this window; the overlay says what that means for the
                      plot rather than repeating the same sentence. */}
                  <p className="text-sm font-semibold text-slate-200">
                    {nodes.length === 0 ? "No nodes are registered" : "Nothing to plot in this window"}
                  </p>
                  <p className="mx-auto mt-0.5 max-w-md text-xs text-slate-500">
                    {nodes.length === 0
                      ? "There is nothing to plot until a node is enrolled."
                      : "The control plane holds no allocation samples in this window. This is missing history, not idle hosts."}
                  </p>
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
                    <linearGradient id="mainDisk" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor={chart.lightOrange} stopOpacity={0.3} />
                      <stop offset="95%" stopColor={chart.lightOrange} stopOpacity={0} />
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
                    stroke={metric === "cpu" ? chart.dangerBright : metric === "memory" ? chart.violet : chart.lightOrange}
                    strokeWidth={2}
                    fill={metric === "cpu" ? "url(#mainCpu)" : metric === "memory" ? "url(#mainMem)" : "url(#mainDisk)"}
                    dot={false}
                    stackId={stacked ? "fleet" : undefined}
                  />
                </AreaChart>
              </ResponsiveContainer>
            )}
          </div>

          {/* Coverage. History is served per node, so the fleet view is a
              fan-out: say which nodes are missing from the line above rather
              than letting a partial average pass for the whole fleet. */}
          {hasTelemetry && (historyFailedNodes.length > 0 || historySkippedNodes.length > 0) ? (
            <p className="mt-3 text-[11px] leading-5 text-amber-300/80">
              {historyFailedNodes.length > 0
                ? `${historyFailedNodes.length} node${historyFailedNodes.length === 1 ? "" : "s"} did not return history and ${historyFailedNodes.length === 1 ? "is" : "are"} excluded from this average. `
                : ""}
              {historySkippedNodes.length > 0
                ? `${historySkippedNodes.length} further node${historySkippedNodes.length === 1 ? " was" : "s were"} not queried (fan-out cap).`
                : ""}
            </p>
          ) : null}
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
                {platform.tone === "green"
                  ? "Operational"
                  : platform.tone === "yellow"
                    ? "Degraded"
                    : platform.tone === "red"
                      ? "Critical"
                      : "Unknown"}
              </p>
              <p className="mt-0.5 text-[11px] text-slate-400">
                {healthQuery.isError
                  ? "The health report could not be read, so the platform's state is unknown."
                  : nodesQuery.isError
                    ? "The node list could not be read, so beacon state is unknown."
                    : nodesQuery.isPending || serversQuery.isPending
                      ? "Reading nodes and workloads…"
                      : `${onlineNodes}/${nodes.length} nodes healthy · ${runningServers}/${servers.length} workloads running`}
              </p>
            </div>
            {/* Each row states what its own source reported. A check the report
                does not contain is "Not reported", never "Healthy" — the
                Database row used to claim health while the report was still
                loading, and Queue / Workers was hardcoded to Healthy. */}
            <div className="mt-2 divide-y divide-white/[0.04] text-xs">
              {(() => {
                const healthAvailable = isAvailable(healthQuery);
                const reported = (count: number, noun: string, query: { isError: boolean; isPending: boolean }) =>
                  query.isError
                    ? { state: "Unavailable", tone: "neutral" as const }
                    : query.isPending
                      ? { state: "Reading…", tone: "neutral" as const }
                      : count === 0
                        ? { state: "None", tone: "neutral" as const }
                        : { state: `${count} ${noun}`, tone: "ok" as const };
                const rows: {
                  label: string;
                  icon: ComponentType<{ size?: number; className?: string }>;
                  state: string;
                  tone: ToneInput;
                }[] = [
                  {
                    label: "Control Plane",
                    icon: Box,
                    ...(healthQuery.isError
                      ? { state: "Unreachable", tone: "danger" as const }
                      : healthQuery.isPending
                        ? { state: "Reading…", tone: "neutral" as const }
                        : { state: "Answering", tone: "ok" as const }),
                  },
                  (() => {
                    const v = checkVerdict(healthAvailable, findCheck(health, "database"));
                    return { label: "Database", icon: Database, state: v.label, tone: v.tone };
                  })(),
                  (() => {
                    const v = checkVerdict(healthAvailable, findCheck(health, "queue"));
                    return { label: "Queue / Workers", icon: RefreshCw, state: v.label, tone: v.tone };
                  })(),
                  {
                    label: "Nodes online",
                    icon: Server,
                    ...(nodesQuery.isError
                      ? { state: "Unavailable", tone: "neutral" as const }
                      : nodesQuery.isPending
                        ? { state: "Reading…", tone: "neutral" as const }
                        : nodes.length === 0
                          ? { state: "None enrolled", tone: "neutral" as const }
                          : {
                              state: `${onlineNodes}/${nodes.length}`,
                              tone: onlineNodes === nodes.length ? ("ok" as const) : ("warn" as const),
                            }),
                  },
                  { label: "Nodes tracked", icon: HardDrive, ...reported(nodes.length, "tracked", nodesQuery) },
                  { label: "Workloads running", icon: CpuKpiChipIcon, ...reported(runningServers, "running", serversQuery) },
                ];
                return rows.map((row) => {
                  const tone = toneStyles[resolveTone(row.tone)];
                  return (
                    <div className="flex items-center gap-2 py-1.5" key={row.label}>
                      <row.icon className="shrink-0 text-text-muted" size={13} />
                      <span className="text-text-subtle">{row.label}</span>
                      <span className="ml-auto flex items-center gap-1.5 font-medium">
                        <span className={cn("h-1.5 w-1.5 shrink-0 rounded-full", tone.dot)} />
                        <span className={tone.fg}>{row.state}</span>
                      </span>
                    </div>
                  );
                });
              })()}
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
              {/* An unreadable audit feed is not a quiet one. */}
              {activityQuery.isError ? (
                <p className="text-xs text-amber-300/80">
                  The audit feed could not be read, so recent activity is unknown.
                </p>
              ) : activityQuery.isPending ? (
                <p className="text-xs text-slate-500">Reading audit feed…</p>
              ) : activity.length === 0 ? (
                <p className="text-xs text-slate-500">No audit events recorded.</p>
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
              <h3 className="text-sm font-bold text-slate-100">Node Allocation</h3>
              <p className="text-[11px] text-slate-400">
                Latest recorded sample per node · allocated share of capacity, not measured host load
              </p>
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
              <p className="text-sm font-semibold text-slate-200">
                {nodesQuery.isError ? "Node list unavailable" : "No nodes registered"}
              </p>
              <p className="mt-0.5 text-xs text-slate-500">
                {nodesQuery.isError
                  ? "The node list could not be read, so this is not a statement about the fleet."
                  : selectedWorkload
                    ? "No node matches the selected workload."
                    : "Enroll a node to see its allocation."}
              </p>
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
                        {/* No network column: the collector writes 0 bytes for
                            every sample, so the figure carried no information. */}
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
              <h3 className="text-sm font-bold text-slate-100">Workloads</h3>
              {/* Not "Top Workloads": nothing here is ranked by usage, because
                  the metrics API reports no per-workload usage at all. */}
              <p className="text-[11px] text-slate-400">
                Configured limits and current status · per-workload usage is not reported
              </p>
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
              <p className="text-sm font-semibold text-slate-200">
                {serversQuery.isError ? "Workload list unavailable" : "No workloads"}
              </p>
              <p className="mt-0.5 text-xs text-slate-500">
                {serversQuery.isError
                  ? "The workload list could not be read, so this is not a statement about what is deployed."
                  : selectedNode || selectedWorkload
                    ? "No workload matches the current filters."
                    : "Deploy a workload to see it here."}
              </p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-wider text-slate-500">
                    <th className="px-2 py-2 font-medium">Name</th>
                    <th className="px-2 py-2 font-medium">Type</th>
                    <th className="px-2 py-2 text-right font-medium">CPU used</th>
                    <th className="px-2 py-2 text-right font-medium">Memory limit</th>
                    <th className="px-2 py-2 text-right font-medium">Disk limit</th>
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
        Every series on this page comes from <code className="font-mono">node_metrics</code>, which the control-plane
        collector derives from each node&apos;s capacity snapshot — it is allocation, not measured host load. Live host
        CPU and network series are not reported by beacons today, so no such series is shown. Point-in-time failures
        live in{" "}
        <button type="button" onClick={() => router.push("/admin/health")} className="underline hover:text-slate-400">
          Health
        </button>
        .
      </p>

      {summaryTrustworthy && summaryQuery.data?.unacknowledgedAlerts ? (
        <p className="text-[11px] text-amber-400/80">
          {summaryQuery.data.unacknowledgedAlerts} unacknowledged alert{(summaryQuery.data.unacknowledgedAlerts ?? 0) > 1 ? "s" : ""} in the alert pipeline.
        </p>
      ) : null}
    </AdminPageLayout>
  );
}

