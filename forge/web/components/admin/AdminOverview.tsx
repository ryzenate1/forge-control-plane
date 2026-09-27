"use client";

import { useMemo, useState, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  AlertTriangle,
  ArrowUpRight,
  ChevronDown,
  Clock,
  Plus,
  RefreshCw,
  RotateCcw,
  Shield,
  ShieldCheck,
  TrendingDown,
  TrendingUp,
  User,
  Zap,
} from "lucide-react";
import {
  CpuKpiChipIcon,
  MemoryRamStickIcon,
  StoragePlattersIcon,
  SystemHealthOperationalIcon,
  SystemHealthAlertIcon,
  NodeHostIcon,
  BeaconRadioTowerIcon,
  ApplicationsCubeIcon,
  DatabaseCylinderIcon,
  ActivityWaveIcon,
  ServerRackIcon,
  HealthECGIcon,
} from "@/components/ui/forge-icons";
import { useRouter } from "next/navigation";
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
} from "recharts";
import {
  fetchAdminAudit,
  fetchAllNodes,
  fetchAllServers,
  fetchHealthStatus,
  fetchUsers,
  type ApiAdminAuditEvent,
  type ApiHealthCheck,
  type ApiNode,
  type ApiServer,
} from "@/lib/api";
import { fetchApps } from "@/lib/api/apps";
import type { MetricPeriod, NodeMetrics } from "@/lib/api/monitoring";
import {
  checkVerdict,
  findCheck,
  isAvailable,
  relativeTime,
  useLatestNodeMetricsQuery,
  useNodeMetricsHistoryQuery,
} from "@/lib/admin/telemetry";
import { ApiError } from "@/lib/api/http";
import { chart } from "@/lib/design-tokens";
import { PageInfoDisclosure } from "@/components/ui/page-info-disclosure";
import {
  AdminPageLayout,
  AdminSection,
  Card,
  CardHeader,
  Pill,
  MiniSparkline,
  SubsystemHealthMeter,
  cn,
} from "./admin-ui";

function SimpleBarChart({
  data,
  title,
}: {
  data: { id: string; label: string; value: number; max: number; note?: string }[];
  title: string;
}) {
  const maxValue = Math.max(0, ...data.map((item) => item.max));
  return (
    <div>
      <h4 className="mb-3 text-xs font-semibold uppercase tracking-wider text-slate-400">
        {title}
      </h4>
      <div className="space-y-2.5">
        {data.map((item) => {
          const pct =
            maxValue > 0
              ? Math.min(100, Math.round((item.value / maxValue) * 100))
              : item.value > 0
              ? 100
              : 0;
          return (
            <div key={item.id} className="min-w-0 space-y-1">
              <div className="flex items-center justify-between text-xs">
                <span
                  className="truncate font-medium text-slate-300"
                  title={item.label}
                >
                  {`host · ${item.label}`}
                </span>
                <span className="font-mono text-slate-400 tabular-nums">
                  {item.value > 0 ? `${item.value.toLocaleString()} MiB` : "Unmetered / Dynamic"}
                </span>
              </div>
              <div className="flex h-2 w-full overflow-hidden rounded-full bg-white/[0.05]">
                {item.value > 0 ? (
                  <div
                    className="h-full rounded-full bg-sky-500/80 transition-all"
                    style={{ width: `${pct}%` }}
                  />
                ) : (
                  <div
                    className="h-full w-full rounded-full bg-sky-500/20 opacity-50"
                  />
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function isPermissionError(error: unknown): boolean {
  return error instanceof ApiError && error.status === 403;
}

function QueryError({
  message,
  onRetry,
  error,
}: {
  message: string;
  onRetry?: () => void;
  error?: unknown;
}) {
  const isPermission = isPermissionError(error);
  return (
    <div
      className={`flex items-center justify-between gap-3 rounded-lg border p-3.5 text-xs ${
        isPermission
          ? "border-amber-500/30 bg-amber-950/20 text-amber-300"
          : "border-red-500/30 bg-red-950/20 text-red-300"
      }`}
    >
      <div className="flex items-center gap-2 min-w-0">
        {isPermission ? (
          <Shield size={14} className="shrink-0 text-amber-400" />
        ) : (
          <AlertTriangle size={14} className="shrink-0 text-red-400" />
        )}
        <span className="truncate">
          {isPermission ? "Permission restricted" : message}
        </span>
      </div>
      {onRetry && !isPermission ? (
        <button
          type="button"
          onClick={onRetry}
          className="shrink-0 font-semibold underline hover:text-red-100"
        >
          Retry
        </button>
      ) : null}
    </div>
  );
}

function reportedTotal(
  records: Array<ApiNode | ApiServer>,
  field: "memoryMb" | "diskMb"
) {
  const values = records
    .map((record) => record[field])
    .filter((value): value is number => typeof value === "number" && Number.isFinite(value));
  return {
    value: values.reduce((sum, value) => sum + value, 0),
    reported: values.length,
    total: records.length,
  };
}

function hasHealthyPersistedHeartbeat(node: ApiNode) {
  return node.heartbeatState === "healthy";
}

/* ------------------------------------------------------------------ *
 * Subsystem meter readings
 *
 * Each of these returns `null` rather than `0` when there is no ratio to
 * report. A meter that has not been read draws an empty dashed track; a meter
 * showing a measured 0% is a different fact and looks different.
 * ------------------------------------------------------------------ */

/** Percentage of `total` that is `part`, or `null` when there is no ratio. */
function ratioPct(part: number, total: number, available: boolean): number | null {
  if (!available || total <= 0) return null;
  return (part / total) * 100;
}

/**
 * Tone for an inventory ratio: a failed query is an error, an unread or empty
 * inventory is unknown, and a full complement is the only healthy reading.
 */
function inventoryTone(
  query: { isError: boolean },
  total: number,
  healthy: number
): "danger" | "unknown" | "ok" | "warn" {
  if (query.isError) return "danger";
  if (total === 0) return "unknown";
  return healthy === total ? "ok" : "warn";
}

/**
 * A control-plane check has no percentage of its own — it either passed or it
 * did not. Report a full bar for a pass and an empty one for a failure; report
 * no bar at all when the check was never read.
 */
function verdictPct(verdict: { tone: string; known: boolean }): number | null {
  if (!verdict.known) return null;
  return verdict.tone === "ok" ? 100 : 0;
}

/**
 * Mean of a reported metric field across rows, or `undefined` when no row
 * reports it. Deliberately not `0`: an unreported reading is not a zero one.
 */
function fleetMean(rows: NodeMetrics[], read: (row: NodeMetrics) => number | undefined): number | undefined {
  let sum = 0;
  let count = 0;
  for (const row of rows) {
    const value = read(row);
    if (typeof value === "number" && Number.isFinite(value)) {
      sum += value;
      count += 1;
    }
  }
  return count > 0 ? sum / count : undefined;
}

/** Sum of a reported field, plus how many rows actually reported it. */
function fleetSum(
  rows: NodeMetrics[],
  read: (row: NodeMetrics) => number | undefined
): { value: number; reported: number } {
  let value = 0;
  let reported = 0;
  for (const row of rows) {
    const raw = read(row);
    if (typeof raw === "number" && Number.isFinite(raw)) {
      value += raw;
      reported += 1;
    }
  }
  return { value, reported };
}

type TrendPoint = { time: string; cpu?: number; memory?: number; storage?: number };

/**
 * Fleet trend series built from real metric history.
 *
 * Rows arrive per node, so they are bucketed by observation time and averaged
 * across whichever nodes reported inside each bucket. Buckets with no reading
 * are not emitted — a gap in the series is left as a gap.
 */
function buildTrendSeries(rows: NodeMetrics[], buckets: number): TrendPoint[] {
  const stamped = rows
    .map((row) => ({ row, at: Date.parse(row.observedAt) }))
    .filter((entry) => Number.isFinite(entry.at))
    .sort((a, b) => a.at - b.at);
  if (stamped.length === 0) return [];

  const first = stamped[0].at;
  const last = stamped[stamped.length - 1].at;
  const span = Math.max(1, last - first);
  const width = span / Math.max(1, buckets);
  const grouped = new Map<number, NodeMetrics[]>();

  for (const entry of stamped) {
    const index = Math.min(buckets - 1, Math.floor((entry.at - first) / width));
    const existing = grouped.get(index);
    if (existing) existing.push(entry.row);
    else grouped.set(index, [entry.row]);
  }

  return [...grouped.entries()]
    .sort((a, b) => a[0] - b[0])
    .map(([index, group]) => ({
      time: new Date(first + index * width).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
      cpu: fleetMean(group, (row) => row.cpuPercent),
      memory: fleetMean(group, (row) => row.memoryPercent),
      storage: fleetMean(group, (row) => row.diskPercent),
    }));
}

function roundPercent(value: number | undefined): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? Math.round(value) : undefined;
}

/** Sparkline input: the series values that exist, or `null` when too short to draw. */
function sparkSeries(series: TrendPoint[], read: (point: TrendPoint) => number | undefined): number[] | null {
  const values = series
    .map(read)
    .filter((value): value is number => typeof value === "number" && Number.isFinite(value));
  return values.length >= 2 ? values : null;
}

/** First-to-last change over a series, or `undefined` when it cannot be computed. */
function seriesDelta(values: number[] | null): number | undefined {
  if (!values || values.length < 2) return undefined;
  return values[values.length - 1] - values[0];
}

function DeltaPill({ delta }: { delta: number | undefined }) {
  if (delta === undefined) return null;
  const rounded = Math.round(delta);
  if (rounded === 0) {
    return <span className="font-mono text-[11px] font-semibold text-slate-500">no change</span>;
  }
  const rising = rounded > 0;
  return (
    <span
      className={cn(
        "inline-flex items-center gap-0.5 font-mono text-[11px] font-semibold",
        rising ? "text-amber-400" : "text-emerald-400"
      )}
    >
      {rising ? <TrendingUp size={11} /> : <TrendingDown size={11} />} {Math.abs(rounded)}%
    </span>
  );
}

/** Shared placeholder for a KPI number we have no reading for. */
function NoReading({ className }: { className?: string }) {
  return (
    <span className={cn("font-mono text-sm font-semibold text-slate-500", className)} title="No reading reported">
      Not reported
    </span>
  );
}

export function AdminOverview() {
  const router = useRouter();
  const [attentionCollapsed, setAttentionCollapsed] = useState(false);
  const [timeRange, setTimeRange] = useState<MetricPeriod>("24h");
  const [isRefreshing, setIsRefreshing] = useState(false);

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

  const appsQuery = useQuery({
    queryKey: ["apps"],
    queryFn: fetchApps,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
    retry: 2,
  });

  const usersQuery = useQuery({
    queryKey: ["users"],
    queryFn: fetchUsers,
    refetchInterval: 60_000,
    refetchIntervalInBackground: false,
    retry: 2,
  });

  const healthQuery = useQuery({
    queryKey: ["health"],
    queryFn: fetchHealthStatus,
    retry: 2,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
  });

  const activityQuery = useQuery<ApiAdminAuditEvent[]>({
    queryKey: ["admin-audit"],
    queryFn: fetchAdminAudit,
    retry: 2,
    refetchInterval: 15_000,
    refetchIntervalInBackground: false,
  });

  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);

  // The node_metrics series. Its cpu/memory/disk percentages are derived by the
  // control plane from NodeCapacitySnapshot (see
  // forge/api/internal/services/observability/service.go collectNodeMetrics),
  // so they are ALLOCATION over time, not measured host load — they are labelled
  // that way throughout this page. The same source hardcodes network bytes to 0
  // and sets containerRunning == containerTotal, so neither is surfaced here.
  // `latestMetricsQuery` is the newest row per node; `trendQuery` is the real
  // per-node history fanned out across the fleet. Neither is substituted for
  // when it reports nothing.
  const latestMetricsQuery = useLatestNodeMetricsQuery();
  const trendQuery = useNodeMetricsHistoryQuery(timeRange, null, nodes, nodes.length > 0);

  const handleRefresh = useCallback(async () => {
    setIsRefreshing(true);
    await Promise.allSettled([
      nodesQuery.refetch(),
      serversQuery.refetch(),
      appsQuery.refetch(),
      usersQuery.refetch(),
      healthQuery.refetch(),
      activityQuery.refetch(),
      latestMetricsQuery.refetch(),
      trendQuery.refetch(),
    ]);
    setTimeout(() => setIsRefreshing(false), 500);
  }, [
    nodesQuery,
    serversQuery,
    appsQuery,
    usersQuery,
    healthQuery,
    activityQuery,
    latestMetricsQuery,
    trendQuery,
  ]);

  const servers = useMemo(() => serversQuery.data ?? [], [serversQuery.data]);

  const checks: ApiHealthCheck[] = useMemo(() => healthQuery.data?.checks ?? [], [healthQuery.data?.checks]);
  const failedChecks = useMemo(
    () => checks.filter((c: ApiHealthCheck) => c.status !== "ok" && c.status !== "warning"),
    [checks]
  );
  const warningChecks = useMemo(
    () => checks.filter((c: ApiHealthCheck) => c.status === "warning"),
    [checks]
  );

  // Fleet Heartbeat & State Classifications
  const onlineNodes = useMemo(
    () => (Array.isArray(nodes) ? nodes.filter(hasHealthyPersistedHeartbeat).length : 0),
    [nodes]
  );
  const offlineNodes = useMemo(
    () =>
      Array.isArray(nodes)
        ? nodes.filter(
            (node) =>
              node.heartbeatState === "offline" ||
              node.heartbeatState === "unreachable" ||
              node.actualState === "offline"
          )
        : [],
    [nodes]
  );
  const degradedNodes = useMemo(
    () =>
      Array.isArray(nodes)
        ? nodes.filter(
            (node) =>
              node.heartbeatState === "degraded" ||
              node.heartbeatState === "suspected" ||
              node.actualState === "degraded"
          )
        : [],
    [nodes]
  );

  // Workload Health & State Classifications
  const runningServers = useMemo(
    () => (Array.isArray(servers) ? servers.filter((server) => server.status === "running").length : 0),
    [servers]
  );
  const crashedServers = useMemo(
    () =>
      Array.isArray(servers)
        ? servers.filter(
            (server) =>
              server.status === "crashed" ||
              server.actualState === "crashed" ||
              (server.desiredState === "running" && server.status === "offline") ||
              Boolean(server.transferError)
          )
        : [],
    [servers]
  );

  // Actionable Failures & Attention Lane
  const failures = useMemo(() => {
    const list: Array<{ id: string; label: string; detail: string; href?: string }> = [];

    // Offline or Degraded Nodes
    if (!nodesQuery.isError && Array.isArray(nodes)) {
      for (const node of nodes) {
        if (!hasHealthyPersistedHeartbeat(node) || node.heartbeatError) {
          list.push({
            id: `node-${node.id}`,
            label: node.name,
            detail: node.heartbeatError
              ? `Node heartbeat failure: ${node.heartbeatError}`
              : `Node heartbeat failure — beacon is ${node.heartbeatState ?? "offline"}`,
            href: "/admin/nodes",
          });
        }
      }
    }

    // Crashed or Divergent Servers
    if (!serversQuery.isError && Array.isArray(servers)) {
      for (const server of servers) {
        if (
          server.status === "crashed" ||
          server.actualState === "crashed" ||
          (server.desiredState === "running" && server.status === "offline") ||
          server.transferError
        ) {
          list.push({
            id: `server-${server.id}`,
            label: server.name,
            detail:
              server.transferError ??
              (server.desiredState && server.actualState && server.desiredState !== server.actualState
                ? `Desired state is ${server.desiredState}, actual state is ${server.actualState}`
                : `Server is ${server.status}`),
            href: `/admin/servers`,
          });
        }
      }
    }

    // Diagnostic Health Checks
    if (!healthQuery.isError && Array.isArray(failedChecks)) {
      for (const check of failedChecks) {
        list.push({
          id: `health-${check.name}`,
          label: check.label ?? check.name,
          detail: check.notificationMessage ?? `Status is ${check.status}`,
          href: "/admin/health",
        });
      }
    }

    return list;
  }, [
    nodes,
    servers,
    nodesQuery.isError,
    serversQuery.isError,
    healthQuery.isError,
    failedChecks,
  ]);

  const pendingAttention = failures.length;

  // Overall Status Truth Engine
  const { overallStatus, overallTitle, overallTone } = useMemo(() => {
    if (healthQuery.isError || nodesQuery.isError || serversQuery.isError) {
      return {
        overallStatus: "unavailable",
        overallTitle: "Control plane partially unreachable",
        overallTone: "red" as const,
      };
    }
    if (
      failedChecks.length > 0 ||
      offlineNodes.length > 0 ||
      crashedServers.length > 0 ||
      healthQuery.data?.status === "failed"
    ) {
      return {
        overallStatus: "failed",
        overallTitle:
          offlineNodes.length > 0
            ? `${offlineNodes.length} nodes offline`
            : "Attention required",
        overallTone: "red" as const,
      };
    }
    if (
      warningChecks.length > 0 ||
      degradedNodes.length > 0 ||
      healthQuery.data?.status === "warning" ||
      (healthQuery.data?.status as string) === "degraded"
    ) {
      return {
        overallStatus: "degraded",
        overallTitle: "Platform degraded",
        overallTone: "yellow" as const,
      };
    }
    return {
      overallStatus: "ok",
      overallTitle: "All systems operational",
      overallTone: "green" as const,
    };
  }, [
    healthQuery.isError,
    nodesQuery.isError,
    serversQuery.isError,
    failedChecks.length,
    warningChecks.length,
    offlineNodes.length,
    degradedNodes.length,
    crashedServers.length,
    healthQuery.data?.status,
  ]);

  // Capacity calculations
  const nodeMemoryCapacity = useMemo(() => reportedTotal(nodes, "memoryMb"), [nodes]);
  const nodeDiskCapacity = useMemo(() => reportedTotal(nodes, "diskMb"), [nodes]);
  const serverMemoryConfiguration = useMemo(() => reportedTotal(servers, "memoryMb"), [servers]);
  const serverDiskConfiguration = useMemo(() => reportedTotal(servers, "diskMb"), [servers]);

  // Cores across nodes. Only nodes that report a core count are summed, and
  // the reporting coverage travels with the number so the UI can say so.
  const cores = useMemo(() => {
    const values = nodes
      .map((node) => node.cpuCores)
      .filter((value): value is number => typeof value === "number" && Number.isFinite(value));
    return {
      value: values.reduce((sum, value) => sum + value, 0),
      reported: values.length,
      total: nodes.length,
    };
  }, [nodes]);

  /* ----------------------------------------------------------------- *
   * Live utilization — from /monitoring/nodes/metrics, never invented.
   * ----------------------------------------------------------------- */

  const latestMetrics = useMemo(() => latestMetricsQuery.data ?? [], [latestMetricsQuery.data]);
  const metricsAvailable = isAvailable(latestMetricsQuery);

  const cpuPercent = useMemo(
    () => roundPercent(fleetMean(latestMetrics, (row) => row.cpuPercent)),
    [latestMetrics]
  );
  const memoryPercent = useMemo(
    () => roundPercent(fleetMean(latestMetrics, (row) => row.memoryPercent)),
    [latestMetrics]
  );
  const storagePercent = useMemo(
    () => roundPercent(fleetMean(latestMetrics, (row) => row.diskPercent)),
    [latestMetrics]
  );

  const memoryUsed = useMemo(() => fleetSum(latestMetrics, (row) => row.memoryUsedMb), [latestMetrics]);
  const memoryTotal = useMemo(() => fleetSum(latestMetrics, (row) => row.memoryTotalMb), [latestMetrics]);
  const diskUsed = useMemo(() => fleetSum(latestMetrics, (row) => row.diskUsedMb), [latestMetrics]);
  const diskTotal = useMemo(() => fleetSum(latestMetrics, (row) => row.diskTotalMb), [latestMetrics]);

  /** Latest reported row for one node, or `undefined` when that node has none. */
  const metricsByNode = useMemo(() => {
    const map = new Map<string, NodeMetrics>();
    for (const row of latestMetrics) {
      const existing = map.get(row.nodeId);
      if (!existing || Date.parse(row.observedAt) > Date.parse(existing.observedAt)) {
        map.set(row.nodeId, row);
      }
    }
    return map;
  }, [latestMetrics]);

  /* ----------------------------------------------------------------- *
   * Trend series — real history, with gaps left as gaps.
   * ----------------------------------------------------------------- */

  const trendSeries = useMemo(() => buildTrendSeries(trendQuery.data?.rows ?? [], 12), [trendQuery.data]);
  const cpuSpark = useMemo(() => sparkSeries(trendSeries, (point) => point.cpu), [trendSeries]);
  const memorySpark = useMemo(() => sparkSeries(trendSeries, (point) => point.memory), [trendSeries]);
  const storageSpark = useMemo(() => sparkSeries(trendSeries, (point) => point.storage), [trendSeries]);
  const trendHasData = trendSeries.some(
    (point) => point.cpu !== undefined || point.memory !== undefined || point.storage !== undefined
  );

  const timeRangeLabel =
    timeRange === "1h"
      ? "Last 1 hour"
      : timeRange === "6h"
        ? "Last 6 hours"
        : timeRange === "7d"
          ? "Last 7 days"
          : "Last 24 hours";

  /* ----------------------------------------------------------------- *
   * Control-plane checks. `checkVerdict` keeps "Unavailable",
   * "Not reported" and a real status from collapsing into "Healthy".
   * ----------------------------------------------------------------- */

  const healthAvailable = isAvailable(healthQuery);
  const databaseVerdict = checkVerdict(healthAvailable, findCheck(healthQuery.data, "database"));
  const queueVerdict = checkVerdict(healthAvailable, findCheck(healthQuery.data, "queue"));


  // Per-node memory allocation chart data
  const nodeResourceData = useMemo(
    () =>
      Array.isArray(nodes)
        ? nodes
            .filter(
              (node): node is ApiNode & { memoryMb: number } =>
                typeof node.memoryMb === "number" && Number.isFinite(node.memoryMb)
            )
            .map((node) => ({
              id: node.id,
              label: node.name,
              value: node.memoryMb,
              max: node.memoryMb,
            }))
        : [],
    [nodes]
  );

  const isDataStale = useCallback(
    (query: { dataUpdatedAt: number; isSuccess: boolean }, refetchIntervalMs: number) =>
      query.isSuccess &&
      query.dataUpdatedAt > 0 &&
      Date.now() - query.dataUpdatedAt > refetchIntervalMs * 2,
    []
  );

  const anyStale =
    isDataStale(nodesQuery, 30_000) ||
    isDataStale(serversQuery, 30_000) ||
    isDataStale(appsQuery, 30_000) ||
    isDataStale(usersQuery, 60_000) ||
    isDataStale(healthQuery, 30_000) ||
    isDataStale(activityQuery, 15_000);

  const auditEvents = activityQuery.data ?? [];

  // Freshness badge: the newest successful read across every source on the
  // page. Zero means nothing has landed yet, which is not "just now".
  const newestUpdate = Math.max(
    nodesQuery.isSuccess ? nodesQuery.dataUpdatedAt : 0,
    serversQuery.isSuccess ? serversQuery.dataUpdatedAt : 0,
    healthQuery.isSuccess ? healthQuery.dataUpdatedAt : 0,
    latestMetricsQuery.isSuccess ? latestMetricsQuery.dataUpdatedAt : 0,
    activityQuery.isSuccess ? activityQuery.dataUpdatedAt : 0
  );

  return (
    <AdminPageLayout className="space-y-6">
      {/* ========================================================================= */}
      {/* ZONE 1: BREADCRUMB, HEADER & GLOBAL ACTIONS                                */}
      {/* ========================================================================= */}
      <div className="flex items-center justify-between text-xs text-slate-400">
        <div className="flex items-center gap-2">
          <span>Command</span>
          <span className="text-slate-600">/</span>
          <span className="text-slate-200 font-semibold">Overview</span>
        </div>
        <div className="flex items-center gap-1.5 font-mono text-[11px] text-slate-500">
          <span className="relative flex h-1.5 w-1.5">
            {newestUpdate > 0 && !anyStale ? (
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-60" />
            ) : null}
            <span
              className={cn(
                "relative inline-flex h-1.5 w-1.5 rounded-full",
                newestUpdate === 0 ? "bg-slate-500" : anyStale ? "bg-amber-400" : "bg-emerald-400"
              )}
            />
          </span>
          <span title={newestUpdate > 0 ? new Date(newestUpdate).toLocaleString() : undefined}>
            {newestUpdate === 0
              ? "Waiting for first read"
              : anyStale
                ? `Stale · last read ${relativeTime(newestUpdate)}`
                : `Live · updated ${relativeTime(newestUpdate)}`}
          </span>
        </div>
      </div>

      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between border-b border-[var(--line)] pb-5">
        <div>
          <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-slate-100 flex items-center gap-2.5">
            <span>Overview</span>
            <PageInfoDisclosure
              title="Forge Mission Control"
              eyebrow="Architecture & Semantics"
              description="Overview provides a live, verified snapshot of your infrastructure fleet, workload instances, control-plane health, and active operations."
              sections={[
                {
                  title: "Desired vs. Actual State",
                  icon: ApplicationsCubeIcon,
                  content:
                    "Forge explicitly separates user intent (Desired State) from per-host runtime observation (Observed Actual State). Workloads and Beacons are only considered healthy when active verification succeeds.",
                },
                {
                  title: "Beacon Fleet Telemetry",
                  icon: NodeHostIcon,
                  content:
                    "Host daemons transmit periodic heartbeats. A healthy beacon reports within its scheduled window; expired heartbeats transition into suspected, unreachable, or offline states.",
                },
                {
                  title: "Capacity vs. Live Observability",
                  icon: ActivityWaveIcon,
                  content:
                    "Overview displays configured resource allocations (memory/disk limits). For live CPU, load, and network time-series graphs, navigate to the dedicated Monitoring section.",
                },
                {
                  title: "Governance & Auditing",
                  icon: ShieldCheck,
                  content:
                    "Every mutation in the control plane is immutably logged with actor identity, target resource, and timestamps.",
                },
              ]}
            />
          </h1>
          <p className="mt-1 text-xs sm:text-sm text-slate-400">
            Your infrastructure at a glance. Live state, workloads, capacity and recent activity.
          </p>
        </div>

        {/* Global Toolbar Controls */}
        <div className="flex items-center gap-2 shrink-0">
          {/* Time range selector */}
          <div className="relative">
            <select
              aria-label="Select time range"
              value={timeRange}
              onChange={(e) => setTimeRange(e.target.value as MetricPeriod)}
              className="h-8 rounded-lg border border-[var(--line)] bg-[var(--surface-input)] pl-2.5 pr-7 text-xs font-medium text-slate-200 outline-none focus:border-[var(--focus)] appearance-none cursor-pointer hover:border-white/20"
            >
              <option value="1h">Last 1 hour</option>
              <option value="6h">Last 6 hours</option>
              <option value="24h">Last 24 hours</option>
              <option value="7d">Last 7 days</option>
            </select>
            <ChevronDown size={12} className="absolute right-2 top-2.5 pointer-events-none text-slate-400" />
          </div>

          {/* Quick refresh button */}
          <button
            type="button"
            aria-label="Refresh data"
            onClick={handleRefresh}
            className="grid h-8 w-8 place-items-center rounded-lg border border-[var(--line)] bg-[var(--surface-input)] text-slate-400 hover:text-white hover:border-white/20 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
          >
            <RotateCcw size={13} className={isRefreshing ? "animate-spin text-sky-400" : ""} />
          </button>

          {/* Primary Deploy CTA */}
          <button
            type="button"
            onClick={() => router.push("/admin/servers")}
            className="flex h-8 items-center gap-1.5 rounded-lg bg-[var(--brand)] px-3.5 text-xs font-bold text-white shadow-sm hover:bg-[var(--brand-hover)] transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] cursor-pointer"
          >
            <Plus size={14} />
            <span>Deploy</span>
          </button>
        </div>
      </div>

      {/* ========================================================================= */}
      {/* ZONE 2: TOP SUMMARY KPI CARDS (CPU, Memory, Storage)                      */}
      {/* ========================================================================= */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {/* CPU Card */}
        <div
          onClick={() => router.push("/admin/monitoring")}
          className="relative overflow-hidden rounded-xl border border-white/[0.08] bg-[var(--surface)] p-4 shadow-sm transition-all hover:border-sky-500/50 hover:bg-white/[0.02] cursor-pointer"
        >
          <div className="flex items-center justify-between gap-2 text-xs font-semibold uppercase tracking-wider text-slate-400">
            <span className="flex items-center gap-1.5 text-sky-400">
              <CpuKpiChipIcon size={16} />
              <span className="font-bold">CPU Allocated</span>
            </span>
            <DeltaPill delta={seriesDelta(cpuSpark)} />
          </div>

          <div className="mt-2.5 flex items-baseline justify-between">
            <div>
              {latestMetricsQuery.isError ? (
                <NoReading className="text-base" />
              ) : cpuPercent === undefined ? (
                <span className="font-mono text-3xl font-bold tracking-tight text-slate-500">
                  {metricsAvailable ? "—" : "…"}
                </span>
              ) : (
                <span className="font-mono text-3xl font-bold tracking-tight text-sky-400">{cpuPercent}%</span>
              )}
              <p className="mt-1 font-mono text-xs text-slate-400">
                {latestMetricsQuery.isError
                  ? "Allocation series unavailable"
                  : cpuPercent === undefined
                    ? metricsAvailable
                      ? "No node reported CPU allocation"
                      : "Reading allocation series…"
                    : `Mean across ${latestMetrics.length} node${latestMetrics.length === 1 ? "" : "s"}${
                        cores.reported > 0
                          ? ` · ${cores.value} core${cores.value === 1 ? "" : "s"}${
                              cores.reported < cores.total ? ` (${cores.reported}/${cores.total} reported)` : ""
                            }`
                          : " · cores not reported"
                      }`}
              </p>
            </div>
            <div className="w-28 h-9 shrink-0">
              {cpuSpark ? <MiniSparkline data={cpuSpark} color={chart.sky} /> : null}
            </div>
          </div>
          <div className="absolute inset-x-0 bottom-0 h-0.5 bg-gradient-to-r from-sky-500/40 via-sky-400/80 to-transparent" />
        </div>

        {/* Memory Card */}
        <div
          onClick={() => router.push("/admin/monitoring")}
          className="relative overflow-hidden rounded-xl border border-white/[0.08] bg-[var(--surface)] p-4 shadow-sm transition-all hover:border-purple-500/50 hover:bg-white/[0.02] cursor-pointer"
        >
          <div className="flex items-center justify-between gap-2 text-xs font-semibold uppercase tracking-wider text-slate-400">
            <span className="flex items-center gap-1.5 text-purple-400">
              <MemoryRamStickIcon size={16} />
              <span className="font-bold">Memory Allocated</span>
            </span>
            <DeltaPill delta={seriesDelta(memorySpark)} />
          </div>

          <div className="mt-2.5 flex items-baseline justify-between">
            <div>
              {latestMetricsQuery.isError ? (
                <NoReading className="text-base" />
              ) : memoryPercent === undefined ? (
                <span className="font-mono text-3xl font-bold tracking-tight text-slate-500">
                  {metricsAvailable ? "—" : "…"}
                </span>
              ) : (
                <span className="font-mono text-3xl font-bold tracking-tight text-purple-400">{memoryPercent}%</span>
              )}
              <p className="mt-1 font-mono text-xs text-slate-400">
                {latestMetricsQuery.isError
                  ? "Allocation series unavailable"
                  : memoryTotal.reported > 0
                    ? `${(memoryUsed.value / 1024).toFixed(1)} / ${(memoryTotal.value / 1024).toFixed(0)} GB allocated`
                    : metricsAvailable
                      ? "No node reported memory allocation"
                      : "Reading allocation series…"}
              </p>
            </div>
            <div className="w-28 h-9 shrink-0">
              {memorySpark ? <MiniSparkline data={memorySpark} color={chart.lightViolet} /> : null}
            </div>
          </div>
          <div className="absolute inset-x-0 bottom-0 h-0.5 bg-gradient-to-r from-purple-500/40 via-purple-400/80 to-transparent" />
        </div>

        {/* Storage Card */}
        <div
          onClick={() => router.push("/admin/nodes")}
          className="relative overflow-hidden rounded-xl border border-white/[0.08] bg-[var(--surface)] p-4 shadow-sm transition-all hover:border-orange-500/50 hover:bg-white/[0.02] cursor-pointer"
        >
          <div className="flex items-center justify-between gap-2 text-xs font-semibold uppercase tracking-wider text-slate-400">
            <span className="flex items-center gap-1.5 text-orange-400">
              <StoragePlattersIcon size={16} />
              <span className="font-bold">Storage Allocated</span>
            </span>
            <DeltaPill delta={seriesDelta(storageSpark)} />
          </div>

          <div className="mt-2.5 flex items-baseline justify-between">
            <div>
              {latestMetricsQuery.isError ? (
                <NoReading className="text-base" />
              ) : storagePercent === undefined ? (
                <span className="font-mono text-3xl font-bold tracking-tight text-slate-500">
                  {metricsAvailable ? "—" : "…"}
                </span>
              ) : (
                <span className="font-mono text-3xl font-bold tracking-tight text-orange-400">{storagePercent}%</span>
              )}
              <p className="mt-1 font-mono text-xs text-slate-400">
                {latestMetricsQuery.isError
                  ? "Allocation series unavailable"
                  : diskTotal.reported > 0
                    ? `${(diskUsed.value / 1024).toFixed(0)} / ${(diskTotal.value / 1024).toFixed(0)} GB allocated`
                    : metricsAvailable
                      ? "No node reported disk allocation"
                      : "Reading allocation series…"}
              </p>
            </div>
            <div className="w-28 h-9 shrink-0">
              {storageSpark ? <MiniSparkline data={storageSpark} color={chart.lightOrange} /> : null}
            </div>
          </div>
          <div className="absolute inset-x-0 bottom-0 h-0.5 bg-gradient-to-r from-orange-500/40 via-orange-400/80 to-transparent" />
        </div>
      </div>

      {/* ========================================================================= */}
      {/* ZONE 3: DUAL MISSION CORE (Left 2-Cols) & RIGHT COLUMN (Alerts & Activity) */}
      {/* ========================================================================= */}
      <div className="grid grid-cols-1 xl:grid-cols-3 gap-5">
        {/* Left Column (Span 2) */}
        <div className="xl:col-span-2 space-y-5">
          {/* Sub-grid: System Health & Capacity Trend */}
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">
            {/* System Health Card */}
            <div className="rounded-xl border border-white/[0.08] bg-[var(--surface)] p-5 shadow-sm flex flex-col justify-between">
              <div>
                <div className="flex items-start justify-between gap-3">
                  <div className="flex items-center gap-3">
                    <div
                      onClick={() => router.push("/admin/health")}
                      className="shrink-0 cursor-pointer transition-transform hover:scale-105"
                      title="View System Health"
                    >
                      {overallTone === "green" ? (
                        <SystemHealthOperationalIcon size={38} />
                      ) : (
                        <SystemHealthAlertIcon size={38} />
                      )}
                    </div>
                    <div>
                      <div className="text-[11px] font-semibold uppercase tracking-wider text-slate-400">
                        System Health
                      </div>
                      <h2
                        className={cn(
                          "text-base font-bold tracking-tight",
                          overallTone === "green"
                            ? "text-emerald-300"
                            : overallTone === "yellow"
                            ? "text-amber-300"
                            : "text-red-300"
                        )}
                      >
                        {healthQuery.isLoading
                          ? "Loading control-plane state…"
                          : overallTitle}
                      </h2>
                    </div>
                  </div>

                  <button
                    type="button"
                    onClick={() => router.push("/admin/health")}
                    className="flex items-center gap-1 text-xs font-semibold text-slate-400 hover:text-white transition-colors"
                  >
                    <span>View details</span>
                    <ArrowUpRight size={13} />
                  </button>
                </div>

                <p className="mt-2 text-xs text-slate-400">
                  {healthQuery.isLoading
                    ? "Checking health, nodes and workloads…"
                    : healthQuery.isError
                    ? String(healthQuery.error?.message ?? "Health API unavailable")
                    : `${onlineNodes}/${nodes.length} beacons healthy · ${runningServers}/${servers.length} workloads running · ${checks.length - failedChecks.length}/${checks.length} checks passing`}
                </p>

                {/* Subsystem Health Progress Meters */}
                <div className="mt-5 space-y-2.5 pt-3 border-t border-white/[0.06]">
                  {/* An empty or unreachable inventory is not a full meter:
                      with no beacons there is no ratio to report. */}
                  <SubsystemHealthMeter
                    icon={NodeHostIcon}
                    label="Nodes"
                    valueText={
                      nodesQuery.isError
                        ? "Unavailable"
                        : !isAvailable(nodesQuery)
                          ? "Loading…"
                          : nodes.length === 0
                            ? "No nodes registered"
                            : `${onlineNodes}/${nodes.length} online`
                    }
                    percentage={ratioPct(onlineNodes, nodes.length, isAvailable(nodesQuery))}
                    tone={inventoryTone(nodesQuery, nodes.length, onlineNodes)}
                    onClick={() => router.push("/admin/nodes")}
                  />
                  <SubsystemHealthMeter
                    icon={BeaconRadioTowerIcon}
                    label="Beacons"
                    valueText={
                      nodesQuery.isError
                        ? "Unavailable"
                        : !isAvailable(nodesQuery)
                          ? "Loading…"
                          : nodes.length === 0
                            ? "No beacons enrolled"
                            : `${onlineNodes} healthy${
                                degradedNodes.length > 0 ? ` (${degradedNodes.length} degraded)` : ""
                              } of ${nodes.length}`
                    }
                    percentage={ratioPct(onlineNodes, nodes.length, isAvailable(nodesQuery))}
                    tone={inventoryTone(nodesQuery, nodes.length, onlineNodes)}
                    onClick={() => router.push("/admin/nodes")}
                  />
                  <SubsystemHealthMeter
                    icon={ApplicationsCubeIcon}
                    label="Workloads"
                    valueText={
                      serversQuery.isError
                        ? "Unavailable"
                        : !isAvailable(serversQuery)
                          ? "Loading…"
                          : servers.length === 0
                            ? "No workloads"
                            : `${runningServers}/${servers.length} running`
                    }
                    percentage={ratioPct(runningServers, servers.length, isAvailable(serversQuery))}
                    tone={inventoryTone(serversQuery, servers.length, runningServers)}
                    onClick={() => router.push("/admin/servers")}
                  />
                  <SubsystemHealthMeter
                    icon={HealthECGIcon}
                    label="Control Plane"
                    valueText={
                      healthQuery.isError ? "Unavailable" : !healthAvailable ? "Checking…" : "Reachable"
                    }
                    percentage={healthQuery.isError ? null : healthAvailable ? 100 : null}
                    tone={healthQuery.isError ? "danger" : healthAvailable ? "ok" : "unknown"}
                    onClick={() => router.push("/admin/health")}
                  />
                  {/* Both of these report their own absence: an unreachable
                      /health and a /health that omits the check are distinct
                      from a passing check, and neither may read as healthy. */}
                  <SubsystemHealthMeter
                    icon={DatabaseCylinderIcon}
                    label="Database Engine"
                    valueText={databaseVerdict.label}
                    percentage={verdictPct(databaseVerdict)}
                    tone={databaseVerdict.tone}
                    onClick={() => router.push("/admin/databases")}
                  />
                  <SubsystemHealthMeter
                    icon={ActivityWaveIcon}
                    label="Queue / Workers"
                    valueText={queueVerdict.label}
                    percentage={verdictPct(queueVerdict)}
                    tone={queueVerdict.tone}
                    onClick={() => router.push("/admin/activity")}
                  />
                </div>
              </div>
            </div>

            {/* Capacity Trend Card */}
            <div className="rounded-xl border border-white/[0.08] bg-[var(--surface)] p-5 shadow-sm flex flex-col justify-between">
              <div>
                <div className="flex items-center justify-between gap-3">
                  <div className="flex items-center gap-2">
                    <TrendingUp size={16} className="text-sky-400" />
                    <div>
                      <h3 className="text-sm font-bold text-slate-100">Allocation Trend</h3>
                      <p className="text-[11px] text-slate-400">
                        Allocated share of node capacity, averaged across nodes — not measured host load
                      </p>
                    </div>
                  </div>
                  <span className="rounded border border-white/[0.08] bg-white/[0.03] px-2 py-0.5 font-mono text-[10px] text-slate-400">
                    {timeRangeLabel}
                  </span>
                </div>

                {/* Multi-line Recharts curve over real metric history */}
                <div className="mt-4 h-44 w-full">
                  {trendQuery.isError ? (
                    <div className="flex h-full flex-col items-center justify-center gap-1 rounded-lg border border-red-500/20 bg-red-500/[0.04] px-4 text-center">
                      <p className="text-xs font-semibold text-red-300">Metric history unavailable</p>
                      <p className="text-[11px] text-slate-400">
                        {trendQuery.error instanceof Error ? trendQuery.error.message : "The series could not be read."}
                      </p>
                    </div>
                  ) : nodes.length === 0 ? (
                    <div className="flex h-full items-center justify-center rounded-lg border border-white/[0.06] bg-white/[0.01] px-4 text-center text-[11px] text-slate-500">
                      No nodes are registered, so there is no allocation history to plot.
                    </div>
                  ) : trendQuery.isPending ? (
                    <div className="flex h-full items-center justify-center rounded-lg border border-white/[0.06] bg-white/[0.01] px-4 text-center text-[11px] text-slate-500">
                      Reading metric history…
                    </div>
                  ) : !trendHasData ? (
                    <div className="flex h-full flex-col items-center justify-center gap-1 rounded-lg border border-white/[0.06] bg-white/[0.01] px-4 text-center">
                      <p className="text-xs font-semibold text-slate-300">No readings in this window</p>
                      <p className="text-[11px] text-slate-500">
                        The control plane recorded no allocation samples for {timeRangeLabel.toLowerCase()}. This is
                        missing history, not idle hosts.
                      </p>
                    </div>
                  ) : (
                  <ResponsiveContainer width="100%" height="100%">
                    <AreaChart data={trendSeries} margin={{ top: 10, right: 5, left: -20, bottom: 0 }}>
                      <defs>
                        <linearGradient id="cpuGrad" x1="0" y1="0" x2="0" y2="1">
                          <stop offset="5%" stopColor={chart.blue} stopOpacity={0.3} />
                          <stop offset="95%" stopColor={chart.blue} stopOpacity={0} />
                        </linearGradient>
                        <linearGradient id="memGrad" x1="0" y1="0" x2="0" y2="1">
                          <stop offset="5%" stopColor={chart.violet} stopOpacity={0.3} />
                          <stop offset="95%" stopColor={chart.violet} stopOpacity={0} />
                        </linearGradient>
                        <linearGradient id="storageGrad" x1="0" y1="0" x2="0" y2="1">
                          <stop offset="5%" stopColor={chart.orange} stopOpacity={0.3} />
                          <stop offset="95%" stopColor={chart.orange} stopOpacity={0} />
                        </linearGradient>
                      </defs>
                      <XAxis dataKey="time" stroke={chart.axisStrong} fontSize={10} tickLine={false} />
                      <YAxis stroke={chart.axisStrong} fontSize={10} tickLine={false} domain={[0, 100]} unit="%" />
                      <Tooltip
                        contentStyle={{
                          backgroundColor: chart.panel,
                          borderColor: chart.panelBorder,
                          borderRadius: "8px",
                          fontSize: "11px",
                        }}
                      />
                      <Area type="monotone" dataKey="cpu" stroke={chart.blue} strokeWidth={2} fill="url(#cpuGrad)" />
                      <Area type="monotone" dataKey="memory" stroke={chart.violet} strokeWidth={2} fill="url(#memGrad)" />
                      <Area type="monotone" dataKey="storage" stroke={chart.orange} strokeWidth={2} fill="url(#storageGrad)" />
                    </AreaChart>
                  </ResponsiveContainer>
                  )}
                </div>

                {/* Legend carries the latest recorded allocation, or a dash when
                    no node reported that metric. */}
                <div className="mt-3 flex flex-wrap items-center justify-between gap-2 border-t border-white/[0.06] pt-2.5 text-[11px] font-mono">
                  <span className="flex items-center gap-1.5 text-slate-300">
                    <span className="h-2 w-2 rounded-full bg-sky-400" /> CPU{" "}
                    {cpuPercent === undefined ? <span className="text-slate-500">—</span> : `${cpuPercent}%`}
                  </span>
                  <span className="flex items-center gap-1.5 text-slate-300">
                    <span className="h-2 w-2 rounded-full bg-purple-400" /> Memory{" "}
                    {memoryPercent === undefined ? <span className="text-slate-500">—</span> : `${memoryPercent}%`}
                  </span>
                  <span className="flex items-center gap-1.5 text-slate-300">
                    <span className="h-2 w-2 rounded-full bg-orange-400" /> Storage{" "}
                    {storagePercent === undefined ? <span className="text-slate-500">—</span> : `${storagePercent}%`}
                  </span>
                </div>

                {/* The fleet series is a client fan-out; incomplete coverage is
                    stated rather than silently averaged away. */}
                {trendQuery.data &&
                (trendQuery.data.failedNodeIds.length > 0 || trendQuery.data.skippedNodeIds.length > 0) ? (
                  <p className="mt-2 text-[11px] leading-5 text-amber-300/80">
                    Partial series:{" "}
                    {trendQuery.data.failedNodeIds.length > 0
                      ? `${trendQuery.data.failedNodeIds.length} node${
                          trendQuery.data.failedNodeIds.length === 1 ? "" : "s"
                        } failed to return history`
                      : null}
                    {trendQuery.data.failedNodeIds.length > 0 && trendQuery.data.skippedNodeIds.length > 0
                      ? "; "
                      : null}
                    {trendQuery.data.skippedNodeIds.length > 0
                      ? `${trendQuery.data.skippedNodeIds.length} node${
                          trendQuery.data.skippedNodeIds.length === 1 ? "" : "s"
                        } beyond the fan-out cap were not queried`
                      : null}
                    .
                  </p>
                ) : null}
              </div>
            </div>
          </div>

          {/* Sub-grid: Workloads & Nodes Floor */}
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">
            {/* Workloads Card */}
            <div className="rounded-xl border border-white/[0.08] bg-[var(--surface)] p-5 shadow-sm flex flex-col justify-between">
              <div>
                <div className="flex items-center justify-between gap-3">
                  <div className="flex items-center gap-2">
                    <ApplicationsCubeIcon size={16} className="text-slate-400" />
                    <div>
                      <h3 className="text-sm font-bold text-slate-100">Workloads</h3>
                      <p className="text-[11px] text-slate-400">
                        {serversQuery.isError
                          ? "Inventory unavailable"
                          : `${servers.length} total · ${
                              servers.filter((s) => s.status !== "running").length
                            } not running`}
                      </p>
                    </div>
                  </div>
                  <button
                    type="button"
                    onClick={() => router.push("/admin/servers")}
                    className="flex items-center gap-1 text-xs font-semibold text-slate-400 hover:text-white transition-colors"
                  >
                    <span>View workloads</span>
                    <ArrowUpRight size={13} />
                  </button>
                </div>

                {/* Workloads Content */}
                {servers.length === 0 ? (
                  <div className="mt-8 mb-4 flex flex-col items-center justify-center text-center">
                    <div className="grid h-12 w-12 place-items-center rounded-2xl border border-white/[0.08] bg-white/[0.03] text-slate-400 shadow-inner mb-3">
                      <ApplicationsCubeIcon size={24} />
                    </div>
                    <h4 className="text-sm font-semibold text-slate-200">No workloads yet</h4>
                    <p className="mt-1 max-w-xs text-xs text-slate-400">
                      Deploy applications, game servers, databases and more.
                    </p>
                    <button
                      type="button"
                      onClick={() => router.push("/admin/servers")}
                      className="mt-4 flex items-center gap-1.5 rounded-lg bg-[var(--brand)] px-4 py-2 text-xs font-bold text-white shadow-sm hover:bg-[var(--brand-hover)] transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
                    >
                      <Plus size={14} />
                      <span>Deploy your first workload</span>
                    </button>
                  </div>
                ) : (
                  <div className="mt-4 divide-y divide-white/[0.04]">
                    {servers.slice(0, 3).map((server) => (
                      <div
                        key={server.id}
                        onClick={() => router.push(`/server/${server.id}`)}
                        className="flex items-center justify-between py-2.5 hover:bg-white/[0.02] cursor-pointer rounded px-2 -mx-2 transition"
                      >
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-xs font-semibold text-slate-200">{server.name}</p>
                          <p className="truncate text-[10px] font-mono text-slate-500">
                            {server.node || "node not reported"} ·{" "}
                            {typeof server.memoryMb === "number" && Number.isFinite(server.memoryMb)
                              ? `${server.memoryMb} MiB`
                              : "memory not set"}
                          </p>
                        </div>
                        <Pill
                          tone={
                            server.status === "running"
                              ? "green"
                              : server.status === "crashed"
                              ? "red"
                              : "neutral"
                          }
                        >
                          {server.status}
                        </Pill>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>

            {/* Nodes Card */}
            <div className="rounded-xl border border-white/[0.08] bg-[var(--surface)] p-5 shadow-sm flex flex-col justify-between">
              <div>
                <div className="flex items-center justify-between gap-3">
                  <div className="flex items-center gap-2">
                    <ServerRackIcon size={16} className="text-slate-400" />
                    <div>
                      <div className="flex items-center gap-1.5">
                        <h3 className="text-sm font-bold text-slate-100">Nodes</h3>
                        <span className="text-[10px] font-mono text-slate-500 uppercase tracking-wider">Infrastructure</span>
                      </div>
                      <p className="text-[11px] text-slate-400">
                        {nodesQuery.isError
                          ? "Inventory unavailable"
                          : !isAvailable(nodesQuery)
                            ? "Loading…"
                            : `${nodes.length} node${nodes.length === 1 ? "" : "s"}`}
                      </p>
                    </div>
                  </div>
                  <button
                    type="button"
                    onClick={() => router.push("/admin/nodes")}
                    className="flex items-center gap-1 text-xs font-semibold text-slate-400 hover:text-white transition-colors"
                  >
                    <span>View nodes</span>
                    <ArrowUpRight size={13} />
                  </button>
                </div>

                {/* Nodes Content */}
                <div className="mt-4 space-y-3">
                  {nodesQuery.isError ? (
                    <p className="text-xs text-red-300">
                      Beacon inventory unavailable — node state cannot be shown.
                    </p>
                  ) : !isAvailable(nodesQuery) ? (
                    <p className="text-xs text-slate-500">Reading beacon inventory…</p>
                  ) : nodes.length === 0 ? (
                    <p className="text-xs text-slate-500">No beacons are enrolled yet.</p>
                  ) : null}
                  {nodes.slice(0, 1).map((node) => (
                    <div
                      key={node.id}
                      onClick={() => router.push("/admin/nodes")}
                      className="rounded-xl border border-white/[0.08] bg-black/25 p-3.5 hover:border-white/20 cursor-pointer transition"
                    >
                      <div className="flex items-center justify-between gap-2">
                        <div className="flex items-center gap-2 min-w-0">
                          <span
                            className={cn(
                              "h-2 w-2 rounded-full shrink-0",
                              hasHealthyPersistedHeartbeat(node) ? "bg-emerald-400" : "bg-amber-400"
                            )}
                          />
                          <span className="truncate text-xs font-bold text-slate-200">{node.name}</span>
                          {node.fqdn ? (
                            <span className="truncate font-mono text-[10px] text-slate-400">{node.fqdn}</span>
                          ) : null}
                        </div>
                        <span
                          className={cn(
                            "rounded border px-2 py-0.5 font-mono text-[10px] font-semibold",
                            hasHealthyPersistedHeartbeat(node)
                              ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-300"
                              : "border-amber-500/30 bg-amber-500/10 text-amber-300"
                          )}
                        >
                          {hasHealthyPersistedHeartbeat(node) ? "Online" : node.heartbeatState || "Unknown"}
                        </span>
                      </div>

                      {/* Gauges — this node's own latest metric row, which the
                          control plane derives from its capacity snapshot. These
                          are allocated shares, not measured load. A node that has
                          not reported shows a dash, not a number. */}
                      {(() => {
                        const row = metricsByNode.get(node.id);
                        const gauges = [
                          { label: "CPU", value: roundPercent(row?.cpuPercent), tint: "text-sky-400" },
                          { label: "Mem", value: roundPercent(row?.memoryPercent), tint: "text-purple-400" },
                          { label: "Disk", value: roundPercent(row?.diskPercent), tint: "text-orange-400" },
                        ];
                        return (
                          <>
                            <p className="mt-3 font-mono text-[10px] uppercase tracking-wider text-slate-500">
                              Allocated
                            </p>
                            <div className="mt-1.5 grid grid-cols-3 gap-2 text-center font-mono text-[10px]">
                              {gauges.map((gauge) => (
                                <div
                                  className="rounded border border-white/[0.04] bg-white/[0.02] p-1.5"
                                  key={gauge.label}
                                >
                                  <span className="block text-slate-500">{gauge.label}</span>
                                  <span className={cn("font-bold", gauge.value === undefined ? "text-slate-500" : gauge.tint)}>
                                    {gauge.value === undefined ? "—" : `${gauge.value}%`}
                                  </span>
                                </div>
                              ))}
                            </div>

                            {/* Footer carries only what the control plane
                                reports for this node. */}
                            <div className="mt-3 flex flex-wrap items-center justify-between gap-x-3 gap-y-1 border-t border-white/[0.04] pt-2 text-[10px] font-mono text-slate-400">
                              <span>
                                Workloads: {servers.filter((server) => server.nodeId === node.id).length}
                              </span>
                              <span>Region: {node.region || "not set"}</span>
                              <span className="text-slate-500">
                                {row
                                  ? `Metrics ${relativeTime(row.observedAt)}`
                                  : latestMetricsQuery.isError
                                    ? "Metrics unavailable"
                                    : "No metrics reported"}
                              </span>
                            </div>
                          </>
                        );
                      })()}
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* Right Column (Span 1) — Network/Alerts & Recent Activity Stack */}
        <div className="xl:col-span-1 space-y-5">
          {/* Active Alerts / Needs Attention Card */}
          <div
            className={cn(
              "rounded-xl border p-5 shadow-sm transition-all",
              pendingAttention > 0
                ? "border-amber-500/25 bg-amber-500/[0.04]"
                : "border-white/[0.08] bg-[var(--surface)]"
            )}
          >
            <div className="flex items-center justify-between gap-2">
              <button
                type="button"
                onClick={() => setAttentionCollapsed((v) => !v)}
                className="flex items-center gap-2 text-left text-sm font-semibold text-slate-200 hover:text-white transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-brand/50 rounded"
                aria-label="Needs attention"
                aria-expanded={!attentionCollapsed}
              >
                <AlertTriangle
                  size={15}
                  className={pendingAttention > 0 ? "text-amber-400" : "text-emerald-400"}
                />
                <span>Needs attention</span>
                <span className="text-xs font-normal text-slate-400">
                  ({attentionCollapsed ? "Show" : "Hide"})
                </span>
              </button>
              <button
                type="button"
                onClick={() => router.push("/admin/health")}
                className="flex items-center gap-1 text-xs font-semibold text-slate-400 hover:text-white transition-colors"
              >
                <span>View all</span>
                <ArrowUpRight size={13} />
              </button>
            </div>

            {!attentionCollapsed ? (
              pendingAttention === 0 ? (
                <div className="mt-4 space-y-3">
                  {/* An empty failure list only means "nothing operational" when
                      every source that could report a failure was readable. */}
                  {overallStatus === "unavailable" ? (
                    <div className="rounded-xl border border-amber-500/20 bg-amber-500/[0.04] p-3 text-xs leading-5 text-amber-100/90">
                      <p className="mb-0.5 font-semibold text-amber-300">Nothing to report — and nothing verified</p>
                      <p>
                        One or more sources could not be read, so this list is not evidence of a healthy fleet. See{" "}
                        <button
                          className="font-semibold underline hover:text-amber-100"
                          onClick={() => router.push("/admin/health")}
                          type="button"
                        >
                          Health
                        </button>{" "}
                        for which checks are missing.
                      </p>
                    </div>
                  ) : (
                    <div className="rounded-xl border border-emerald-500/15 bg-emerald-500/[0.03] p-3 text-xs leading-5 text-emerald-200/90">
                      <p className="mb-0.5 font-semibold text-emerald-300">No open issues</p>
                      <p>
                        Fleet heartbeat, workloads and the control-plane checks that reported are all passing. See{" "}
                        <button
                          className="font-semibold underline hover:text-emerald-100"
                          onClick={() => router.push("/admin/health")}
                          type="button"
                        >
                          Health
                        </button>{" "}
                        for deep diagnostics.
                      </p>
                    </div>
                  )}

                  {/* Warnings are not failures, but they are real and belong
                      here. Nothing is listed when nothing is reported. */}
                  {warningChecks.length > 0 ? (
                    <div className="space-y-2 border-t border-white/[0.04] pt-2">
                      {warningChecks.slice(0, 4).map((check) => (
                        <div
                          className="-mx-1.5 flex items-start justify-between gap-2 rounded p-1.5 text-xs transition hover:bg-white/[0.04] cursor-pointer"
                          key={check.name}
                          onClick={() => router.push("/admin/health")}
                        >
                          <div className="flex min-w-0 items-start gap-2">
                            <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-amber-400" />
                            <div className="min-w-0">
                              <p className="truncate text-[11px] font-semibold text-slate-200 capitalize">
                                {check.name.replace(/_/g, " ")}
                              </p>
                              <p className="truncate text-[10px] text-slate-400">
                                {check.notificationMessage || check.label || "Reported a warning without a message."}
                              </p>
                            </div>
                          </div>
                          <span className="shrink-0 font-mono text-[10px] text-amber-300/80">warning</span>
                        </div>
                      ))}
                    </div>
                  ) : null}
                </div>
              ) : (
                <ul className="mt-4 max-h-[220px] divide-y divide-white/[0.06] overflow-auto rounded-xl border border-white/[0.08] bg-black/30">
                  {failures.slice(0, 5).map((f) => (
                    <li
                      key={f.id}
                      onClick={() => f.href && router.push(f.href)}
                      className={`p-3 transition ${
                        f.href ? "cursor-pointer hover:bg-white/[0.04]" : ""
                      }`}
                    >
                      <p className="truncate text-xs font-semibold text-amber-200">{f.label}</p>
                      <p className="truncate text-xs text-slate-400">{f.detail}</p>
                    </li>
                  ))}
                </ul>
              )
            ) : null}

            {!attentionCollapsed && pendingAttention > 5 ? (
              <button
                type="button"
                onClick={() => router.push("/admin/health")}
                className="mt-3 text-xs font-medium text-amber-300 hover:text-amber-200 transition"
              >
                View all {pendingAttention} issues →
              </button>
            ) : null}
          </div>

          {/* Recent Activity Card */}
          <div className="rounded-xl border border-white/[0.08] bg-[var(--surface)] p-5 shadow-sm">
            <div className="flex items-center justify-between gap-3">
              <div className="flex items-center gap-2">
                <Clock size={15} className="text-slate-400" />
                <h3 className="text-sm font-bold text-slate-100">Recent Activity</h3>
              </div>
              <button
                type="button"
                onClick={() => router.push("/admin/operations")}
                className="flex items-center gap-1 text-xs font-semibold text-slate-400 hover:text-white transition-colors"
              >
                <span>View all</span>
                <ArrowUpRight size={13} />
              </button>
            </div>

            {activityQuery.isError ? (
              <p className="mt-4 text-xs text-amber-300/80">
                The audit feed could not be read, so recent changes are unknown.
              </p>
            ) : activityQuery.isPending ? (
              <p className="mt-4 text-xs text-slate-500">Reading audit feed…</p>
            ) : auditEvents.length === 0 ? (
              <p className="mt-4 text-xs text-slate-500">No audit events recorded.</p>
            ) : (
              <div className="mt-4 divide-y divide-white/[0.04]">
                {auditEvents.slice(0, 4).map((event) => (
                  <div
                    key={event.id}
                    onClick={() => router.push("/admin/operations")}
                    className="flex items-start justify-between gap-2 py-2.5 px-2 -mx-2 rounded hover:bg-white/[0.03] cursor-pointer transition"
                  >
                    <div className="flex items-start gap-2.5 min-w-0">
                      <div className="grid h-6 w-6 place-items-center rounded-full bg-white/[0.04] text-slate-400 shrink-0 mt-0.5">
                        <User size={12} />
                      </div>
                      <div className="min-w-0">
                        <p className="truncate text-xs font-semibold text-slate-200">{event.action.replace(/_/g, " ")}</p>
                        <p className="truncate text-[10px] font-mono text-slate-500">
                          {event.actorEmail || "system"}
                        </p>
                      </div>
                    </div>
                    <span className="font-mono text-[10px] text-slate-500 shrink-0">
                      {event.createdAt
                        ? new Date(event.createdAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
                        : "—"}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* ========================================================================= */}
      {/* ZONE 4: CAPACITY QUOTAS SECTION (Deduplicated, Clean, Truthful)           */}
      {/* ========================================================================= */}
      <AdminSection
        title="Capacity"
        description="Configured compute allocations across workloads and host pools."
      >
        <div className="grid gap-6 md:grid-cols-2">
          <Card className="min-h-[160px]">
            <CardHeader title="Memory Allocation" icon={MemoryRamStickIcon} />
            <p className="text-xs font-medium uppercase tracking-wider text-slate-500">
              Configured server memory — sum of Workload resources.memory
            </p>
            <p className="mt-1 font-mono text-2xl font-bold text-slate-100 tabular-nums">
              {serversQuery.isError
                ? "Unavailable"
                : serversQuery.isLoading
                ? "…"
                : serverMemoryConfiguration.reported > 0
                ? `${serverMemoryConfiguration.value.toLocaleString()} MiB`
                : "Not reported"}
            </p>
            <p className="mt-1 text-xs text-slate-400">
              {serversQuery.isError
                ? "Workload inventory is unavailable."
                : serversQuery.isLoading
                ? "Waiting for workload inventory."
                : `Allocated across ${serverMemoryConfiguration.reported} of ${serverMemoryConfiguration.total} workloads.`}
            </p>
            <div className="mt-3 border-t border-white/[0.06] pt-2.5 text-xs text-slate-400">
              <span className="text-slate-500">Beacon host allocatable pool:</span>{" "}
              {nodesQuery.isError
                ? "unavailable"
                : nodesQuery.isLoading
                ? "…"
                : nodeMemoryCapacity.reported > 0
                ? `${nodeMemoryCapacity.value.toLocaleString()} MiB across ${nodeMemoryCapacity.reported}/${nodeMemoryCapacity.total} beacons`
                : "not reported"}
            </div>
            <p className="mt-2 text-[11px] leading-relaxed text-slate-500">
              Capacity is configured quota allocation, not live memory utilization. See Monitoring for real-time telemetry over time.
            </p>
          </Card>

          <Card className="min-h-[160px]">
            <CardHeader title="Storage Allocation" icon={StoragePlattersIcon} />
            <p className="text-xs font-medium uppercase tracking-wider text-slate-500">
              Configured server disk — sum of Workload resources.disk
            </p>
            <p className="mt-1 font-mono text-2xl font-bold text-slate-100 tabular-nums">
              {serversQuery.isError
                ? "Unavailable"
                : serversQuery.isLoading
                ? "…"
                : serverDiskConfiguration.reported > 0
                ? `${serverDiskConfiguration.value.toLocaleString()} MiB`
                : "Not reported"}
            </p>
            <p className="mt-1 text-xs text-slate-400">
              {serversQuery.isError
                ? "Workload inventory is unavailable."
                : serversQuery.isLoading
                ? "Waiting for workload inventory."
                : `Allocated across ${serverDiskConfiguration.reported} of ${serverDiskConfiguration.total} workloads.`}
            </p>
            <div className="mt-3 border-t border-white/[0.06] pt-2.5 text-xs text-slate-400">
              <span className="text-slate-500">Beacon storage pool:</span>{" "}
              {nodesQuery.isError
                ? "unavailable"
                : nodesQuery.isLoading
                ? "…"
                : nodeDiskCapacity.reported > 0
                ? `${nodeDiskCapacity.value.toLocaleString()} MiB across ${nodeDiskCapacity.reported}/${nodeDiskCapacity.total} beacons`
                : "not reported"}
            </div>
            <p className="mt-2 text-[11px] leading-relaxed text-slate-500">
              Disk capacity represents maximum allocation bounds. Live disk usage is monitored per container via Beacon.
            </p>
          </Card>
        </div>

        {/* Per-Node Memory Capacity Bar Chart */}
        {nodeResourceData.length > 0 && (
          <div className="mt-4 rounded-xl border border-white/[0.08] bg-[var(--surface)] p-5">
            <SimpleBarChart
              data={nodeResourceData}
              title="Configured memory capacity per beacon (MiB)"
            />
          </div>
        )}
      </AdminSection>

      {/* ========================================================================= */}
      {/* ZONE 5: BOTTOM INFRASTRUCTURE EVENTS TABLE                                */}
      {/* ========================================================================= */}
      <div className="rounded-xl border border-white/[0.08] bg-[var(--surface)] p-5 shadow-sm">
        <div className="flex items-center justify-between gap-3 mb-4">
          <div>
            <h3 className="text-sm font-bold text-slate-100 flex items-center gap-2">
              <Zap size={14} className="text-amber-400" />
              <span>Infrastructure Events</span>
            </h3>
            <p className="text-xs text-slate-400 mt-0.5">
              Recent operational events across your Forge installation.
            </p>
          </div>
          <button
            type="button"
            onClick={() => router.push("/admin/operations")}
            className="flex items-center gap-1 text-xs font-semibold text-slate-400 hover:text-white transition-colors"
          >
            <span>View all</span>
            <ArrowUpRight size={13} />
          </button>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead>
              <tr className="border-b border-white/[0.06] text-[10px] font-bold uppercase tracking-wider text-slate-500">
                <th className="pb-2.5 font-semibold">Time</th>
                <th className="pb-2.5 font-semibold">Type</th>
                <th className="pb-2.5 font-semibold">Resource</th>
                <th className="pb-2.5 font-semibold">Message</th>
                <th className="pb-2.5 font-semibold text-right">Age</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/[0.04]">
              {auditEvents.length > 0 ? (
                auditEvents.slice(0, 6).map((evt, idx) => (
                  <tr
                    key={evt.id || idx}
                    onClick={() => router.push("/admin/operations")}
                    className="hover:bg-white/[0.04] cursor-pointer transition"
                  >
                    <td className="py-2.5 font-mono text-slate-400">
                      {evt.createdAt
                        ? new Date(evt.createdAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
                        : "—"}
                    </td>
                    <td className="py-2.5">
                      {/* Category comes from the action, not from a fixed label. */}
                      <span className="inline-flex items-center gap-1.5 rounded-full border border-slate-500/30 bg-slate-500/10 px-2 py-0.5 font-mono text-[10px] font-semibold text-slate-300">
                        <span className="h-1.5 w-1.5 rounded-full bg-slate-400" />
                        <span>{evt.action.split(/[._]/)[0] || "audit"}</span>
                      </span>
                    </td>
                    <td className="py-2.5 font-semibold text-slate-300 capitalize">
                      {evt.targetType || evt.resource || "—"}
                    </td>
                    <td className="py-2.5 text-slate-400">
                      <span className="text-slate-200 font-medium">{evt.action.replace(/_/g, " ")}</span>
                      {evt.actorEmail ? ` by ${evt.actorEmail}` : ""}
                    </td>
                    <td className="py-2.5 text-right font-mono text-slate-500">
                      {relativeTime(evt.createdAt) ?? "—"}
                    </td>
                  </tr>
                ))
              ) : (
                /* One row, stating which of the three it is. Never sample events. */
                <tr>
                  <td className="py-4 text-xs text-slate-500" colSpan={5}>
                    {activityQuery.isError
                      ? "The audit feed could not be read. Recent infrastructure events are unknown — this is not an idle fleet."
                      : activityQuery.isPending
                      ? "Reading audit feed…"
                      : "No audit events have been recorded."}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* ========================================================================= */}
      {/* ZONE 6: TELEMETRY FRESHNESS RIBBON ("SOURCES")                            */}
      {/* ========================================================================= */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-white/[0.06] bg-white/[0.015] px-3.5 py-2">
        <div className="flex flex-wrap items-center gap-2 text-xs">
          <span className="font-mono text-[10px] font-bold uppercase tracking-wider text-slate-500">
            Sources:
          </span>
          {anyStale ? (
            <Pill tone="yellow" className="gap-1">
              <RefreshCw size={10} className="animate-spin" /> Data may be stale
            </Pill>
          ) : null}
          {([
            { label: "Nodes", query: nodesQuery },
            { label: "Servers", query: serversQuery },
            { label: "Users", query: usersQuery },
            { label: "Health", query: healthQuery },
            { label: "Activity", query: activityQuery },
          ] as const).map(({ label, query }) => (
            <Pill
              key={label}
              tone={
                query.isError
                  ? isPermissionError(query.error)
                    ? "yellow"
                    : "red"
                  : query.isLoading
                  ? "yellow"
                  : "green"
              }
            >
              {label}:{" "}
              {query.isError
                ? isPermissionError(query.error)
                  ? "restricted"
                  : "unavailable"
                : query.isLoading
                ? "loading"
                : "available"}
              {query.isSuccess && query.dataUpdatedAt > 0 ? (
                <span className="ml-1 font-mono text-[10px] text-slate-500 tabular-nums">
                  {new Date(query.dataUpdatedAt).toLocaleTimeString([], {
                    hour: "2-digit",
                    minute: "2-digit",
                    second: "2-digit",
                  })}
                </span>
              ) : null}
            </Pill>
          ))}
        </div>

        <div className="flex items-center gap-2 text-[10px] text-slate-500 font-mono">
          <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 animate-pulse" />
          <span>Live Telemetry Polling 30s</span>
        </div>
      </div>

      {/* Error Banners if queries fail */}
      {(nodesQuery.isError ||
        serversQuery.isError ||
        usersQuery.isError ||
        healthQuery.isError ||
        activityQuery.isError) && (
        <div className="space-y-2">
          {nodesQuery.isError && (
            <QueryError
              message={`Beacons unavailable — fleet and capacity coverage incomplete. (${
                nodesQuery.error?.message ?? "unknown error"
              })`}
              onRetry={() => nodesQuery.refetch()}
              error={nodesQuery.error}
            />
          )}
          {serversQuery.isError && (
            <QueryError
              message={`Workload inventory unavailable — status and capacity incomplete. (${
                serversQuery.error?.message ?? "unknown error"
              })`}
              onRetry={() => serversQuery.refetch()}
              error={serversQuery.error}
            />
          )}
          {usersQuery.isError && (
            <QueryError
              message={`Users unavailable. (${
                usersQuery.error?.message ?? "unknown error"
              })`}
              onRetry={() => usersQuery.refetch()}
              error={usersQuery.error}
            />
          )}
          {healthQuery.isError && (
            <QueryError
              message={`Control-plane health unavailable. (${
                healthQuery.error?.message ?? "unknown error"
              })`}
              onRetry={() => healthQuery.refetch()}
              error={healthQuery.error}
            />
          )}
          {activityQuery.isError && (
            <QueryError
              message={`Activity unavailable. (${
                activityQuery.error?.message ?? "unknown error"
              })`}
              onRetry={() => activityQuery.refetch()}
              error={activityQuery.error}
            />
          )}
        </div>
      )}
    </AdminPageLayout>
  );
}

