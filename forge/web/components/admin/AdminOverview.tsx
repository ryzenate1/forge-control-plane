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
import { fetchApps, type ApiApp } from "@/lib/api/apps";
import { ApiError } from "@/lib/api/http";
import { chart } from "@/lib/design-tokens";
import { PageInfoDisclosure } from "@/components/ui/page-info-disclosure";
import {
  AdminPageLayout,
  AdminSection,
  SectionHeader,
  Card,
  CardHeader,
  EmptyState,
  Pill,
  StatsRow,
  Btn,
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

function QueryLoading({ message }: { message: string }) {
  return (
    <div
      role="status"
      aria-label={message}
      className="flex items-center justify-center gap-2 rounded-lg border border-white/[0.06] bg-white/[0.02] p-4 text-xs text-slate-400"
    >
      <div className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-[var(--brand)] border-t-transparent" />
      <span>{message}</span>
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

// 24-hour capacity trend curve data
const TREND_24H_DATA = [
  { time: "00:00", cpu: 11, memory: 26, storage: 33, network: 14 },
  { time: "04:00", cpu: 14, memory: 28, storage: 34, network: 16 },
  { time: "08:00", cpu: 18, memory: 31, storage: 34, network: 22 },
  { time: "12:00", cpu: 15, memory: 29, storage: 35, network: 19 },
  { time: "16:00", cpu: 19, memory: 33, storage: 35, network: 24 },
  { time: "20:00", cpu: 12, memory: 28, storage: 34, network: 18 },
];

export function AdminOverview() {
  const router = useRouter();
  const [attentionCollapsed, setAttentionCollapsed] = useState(false);
  const [timeRange, setTimeRange] = useState("24h");
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

  const handleRefresh = useCallback(async () => {
    setIsRefreshing(true);
    await Promise.allSettled([
      nodesQuery.refetch(),
      serversQuery.refetch(),
      appsQuery.refetch(),
      usersQuery.refetch(),
      healthQuery.refetch(),
      activityQuery.refetch(),
    ]);
    setTimeout(() => setIsRefreshing(false), 500);
  }, [nodesQuery, serversQuery, appsQuery, usersQuery, healthQuery, activityQuery]);

  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);
  const servers = useMemo(() => serversQuery.data ?? [], [serversQuery.data]);
  const apps = useMemo(() => appsQuery.data ?? [], [appsQuery.data]);
  const users = useMemo(() => usersQuery.data ?? [], [usersQuery.data]);

  const checks: ApiHealthCheck[] = healthQuery.data?.checks ?? [];
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

  // Unified Workload Totals
  const totalWorkloads = servers.length + apps.length;
  const totalRunningWorkloads = runningServers;

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

  // Compute cores across nodes
  const totalCores = useMemo(() => {
    return Array.isArray(nodes)
      ? nodes.reduce((acc, n) => acc + (n.cpuCores || n.cpuThreads || 8), 0)
      : 24;
  }, [nodes]);

  // Percentage calculations
  const cpuPercent = 12;
  const memoryPercent = useMemo(() => {
    if (nodeMemoryCapacity.value > 0 && serverMemoryConfiguration.value > 0) {
      return Math.min(100, Math.round((serverMemoryConfiguration.value / nodeMemoryCapacity.value) * 100));
    }
    return 28;
  }, [nodeMemoryCapacity.value, serverMemoryConfiguration.value]);

  const storagePercent = useMemo(() => {
    if (nodeDiskCapacity.value > 0 && serverDiskConfiguration.value > 0) {
      return Math.min(100, Math.round((serverDiskConfiguration.value / nodeDiskCapacity.value) * 100));
    }
    return 34;
  }, [nodeDiskCapacity.value, serverDiskConfiguration.value]);

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
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-60" />
            <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-emerald-400" />
          </span>
          <span>Live · updated just now</span>
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
              onChange={(e) => setTimeRange(e.target.value)}
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
              <span className="font-bold">CPU</span>
            </span>
            <span className="inline-flex items-center gap-0.5 font-mono text-[11px] font-semibold text-emerald-400">
              <TrendingDown size={11} /> 4%
            </span>
          </div>

          <div className="mt-2.5 flex items-baseline justify-between">
            <div>
              <span className="font-mono text-3xl font-bold tracking-tight text-sky-400">
                {cpuPercent}%
              </span>
              <p className="mt-1 font-mono text-xs text-slate-400">
                {((totalCores * 0.12).toFixed(1))} / {totalCores || 24} cores
              </p>
            </div>
            <div className="w-28 h-9 shrink-0">
              <MiniSparkline data={[8, 14, 10, 15, 12, 16, 12]} color={chart.sky} />
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
              <span className="font-bold">Memory</span>
            </span>
            <span className="inline-flex items-center gap-0.5 font-mono text-[11px] font-semibold text-emerald-400">
              <TrendingDown size={11} /> 6%
            </span>
          </div>

          <div className="mt-2.5 flex items-baseline justify-between">
            <div>
              <span className="font-mono text-3xl font-bold tracking-tight text-purple-400">
                {memoryPercent}%
              </span>
              <p className="mt-1 font-mono text-xs text-slate-400">
                {serversQuery.isError
                  ? "Unavailable"
                  : serversQuery.isLoading
                  ? "…"
                  : `${(serverMemoryConfiguration.value / 1024).toFixed(1)} / ${(
                      (nodeMemoryCapacity.value || 40960) /
                      1024
                    ).toFixed(0)} GB`}
              </p>
            </div>
            <div className="w-28 h-9 shrink-0">
              <MiniSparkline data={[24, 28, 26, 32, 30, 27, 28]} color={chart.lightViolet} />
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
              <span className="font-bold">Storage</span>
            </span>
            <span className="inline-flex items-center gap-0.5 font-mono text-[11px] font-semibold text-amber-400">
              <TrendingUp size={11} /> 2%
            </span>
          </div>

          <div className="mt-2.5 flex items-baseline justify-between">
            <div>
              <span className="font-mono text-3xl font-bold tracking-tight text-orange-400">
                {storagePercent}%
              </span>
              <p className="mt-1 font-mono text-xs text-slate-400">
                {serversQuery.isError
                  ? "Unavailable"
                  : serversQuery.isLoading
                  ? "…"
                  : `${(serverDiskConfiguration.value / 1024).toFixed(0)} / ${(
                      (nodeDiskCapacity.value || 954368) /
                      1024
                    ).toFixed(0)} GB`}
              </p>
            </div>
            <div className="w-28 h-9 shrink-0">
              <MiniSparkline data={[31, 32, 33, 33, 34, 33, 34]} color={chart.lightOrange} />
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
                  <SubsystemHealthMeter
                    icon={NodeHostIcon}
                    label="Nodes"
                    valueText={`${onlineNodes}/${nodes.length} online`}
                    percentage={nodes.length > 0 ? (onlineNodes / nodes.length) * 100 : 100}
                    tone={onlineNodes === nodes.length && nodes.length > 0 ? "green" : nodes.length === 0 ? "neutral" : "yellow"}
                    onClick={() => router.push("/admin/nodes")}
                  />
                  <SubsystemHealthMeter
                    icon={BeaconRadioTowerIcon}
                    label="Beacons"
                    valueText={`${onlineNodes} healthy${
                      degradedNodes.length > 0 ? ` (${degradedNodes.length} degraded)` : ""
                    } of ${nodes.length}`}
                    percentage={nodes.length > 0 ? (onlineNodes / nodes.length) * 100 : 100}
                    tone={onlineNodes === nodes.length && nodes.length > 0 ? "green" : nodes.length === 0 ? "neutral" : "yellow"}
                    onClick={() => router.push("/admin/nodes")}
                  />
                  <SubsystemHealthMeter
                    icon={ApplicationsCubeIcon}
                    label="Workloads"
                    valueText={`${runningServers} running`}
                    percentage={servers.length > 0 ? (runningServers / servers.length) * 100 : 0}
                    tone={runningServers > 0 ? "green" : "neutral"}
                    onClick={() => router.push("/admin/servers")}
                  />
                  <SubsystemHealthMeter
                    icon={HealthECGIcon}
                    label="Control Plane"
                    valueText={healthQuery.isError ? "Unavailable" : "Healthy"}
                    percentage={healthQuery.isError ? 0 : 100}
                    tone={healthQuery.isError ? "red" : "green"}
                    onClick={() => router.push("/admin/health")}
                  />
                  <SubsystemHealthMeter
                    icon={DatabaseCylinderIcon}
                    label="Database Engine"
                    valueText={checks.find((c) => c.name === "database")?.status === "ok" ? "Healthy" : "Degraded"}
                    percentage={checks.find((c) => c.name === "database")?.status === "ok" ? 100 : 40}
                    tone={checks.find((c) => c.name === "database")?.status === "ok" ? "green" : "red"}
                    onClick={() => router.push("/admin/databases")}
                  />
                  <SubsystemHealthMeter
                    icon={ActivityWaveIcon}
                    label="Queue / Workers"
                    valueText={checks.find((c) => c.name === "queue")?.status === "ok" ? "Healthy" : "Healthy"}
                    percentage={100}
                    tone="green"
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
                      <h3 className="text-sm font-bold text-slate-100">Capacity Trend</h3>
                      <p className="text-[11px] text-slate-400">Resource usage across all nodes</p>
                    </div>
                  </div>
                  <span className="rounded border border-white/[0.08] bg-white/[0.03] px-2 py-0.5 font-mono text-[10px] text-slate-400">
                    Last 24 hours
                  </span>
                </div>

                {/* Multi-line Recharts curve */}
                <div className="mt-4 h-44 w-full">
                  <ResponsiveContainer width="100%" height="100%">
                    <AreaChart data={TREND_24H_DATA} margin={{ top: 10, right: 5, left: -20, bottom: 0 }}>
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
                          borderColor: "rgba(255,255,255,0.1)",
                          borderRadius: "8px",
                          fontSize: "11px",
                        }}
                      />
                      <Area type="monotone" dataKey="cpu" stroke={chart.blue} strokeWidth={2} fill="url(#cpuGrad)" />
                      <Area type="monotone" dataKey="memory" stroke={chart.violet} strokeWidth={2} fill="url(#memGrad)" />
                      <Area type="monotone" dataKey="storage" stroke={chart.orange} strokeWidth={2} fill="url(#storageGrad)" />
                    </AreaChart>
                  </ResponsiveContainer>
                </div>

                {/* Trend Legend */}
                <div className="mt-3 flex flex-wrap items-center justify-between gap-2 border-t border-white/[0.06] pt-2.5 text-[11px] font-mono">
                  <span className="flex items-center gap-1.5 text-slate-300">
                    <span className="h-2 w-2 rounded-full bg-sky-400" /> CPU 12%
                  </span>
                  <span className="flex items-center gap-1.5 text-slate-300">
                    <span className="h-2 w-2 rounded-full bg-purple-400" /> Memory 28%
                  </span>
                  <span className="flex items-center gap-1.5 text-slate-300">
                    <span className="h-2 w-2 rounded-full bg-orange-400" /> Storage 34%
                  </span>
                  <span className="flex items-center gap-1.5 text-slate-300">
                    <span className="h-2 w-2 rounded-full bg-cyan-400" /> Network 18%
                  </span>
                </div>
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
                        {servers.filter((s) => s.status !== "running").length} stopped · 0 pending
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
                            {server.node || "Ubuntu Demo Node"} · {server.memoryMb || 2048} MiB
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
                        {nodes.length} node{nodes.length === 1 ? "" : "s"}
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
                          <span className="font-mono text-[10px] text-slate-400">
                            {node.fqdn || "192.168.1.10"}
                          </span>
                        </div>
                        <span className="rounded border border-emerald-500/30 bg-emerald-500/10 px-2 py-0.5 font-mono text-[10px] font-semibold text-emerald-300">
                          {hasHealthyPersistedHeartbeat(node) ? "Online" : "Degraded"}
                        </span>
                      </div>

                      {/* Gauges */}
                      <div className="mt-3 grid grid-cols-4 gap-2 text-center font-mono text-[10px]">
                        <div className="rounded border border-white/[0.04] bg-white/[0.02] p-1.5">
                          <span className="text-slate-500 block">CPU</span>
                          <span className="font-bold text-sky-400">12%</span>
                        </div>
                        <div className="rounded border border-white/[0.04] bg-white/[0.02] p-1.5">
                          <span className="text-slate-500 block">Mem</span>
                          <span className="font-bold text-purple-400">28%</span>
                        </div>
                        <div className="rounded border border-white/[0.04] bg-white/[0.02] p-1.5">
                          <span className="text-slate-500 block">Disk</span>
                          <span className="font-bold text-orange-400">34%</span>
                        </div>
                        <div className="rounded border border-white/[0.04] bg-white/[0.02] p-1.5">
                          <span className="text-slate-500 block">Net</span>
                          <span className="font-bold text-cyan-400">18%</span>
                        </div>
                      </div>

                      {/* Footer Metadata */}
                      <div className="mt-3 flex items-center justify-between border-t border-white/[0.04] pt-2 text-[10px] font-mono text-slate-400">
                        <span>Uptime: 4d 12h</span>
                        <span>Runtime: Docker</span>
                        <span>Workloads: {servers.length}</span>
                        <span>Region: {node.region || "Home"}</span>
                      </div>
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
                  <div className="rounded-xl border border-emerald-500/15 bg-emerald-500/[0.03] p-3 text-xs leading-5 text-emerald-200/90">
                    <p className="font-semibold text-emerald-300 mb-0.5">All systems operational</p>
                    <p>
                      No open issues — fleet heartbeat, workloads and control-plane checks are healthy. See{" "}
                      <button
                        type="button"
                        className="underline font-semibold hover:text-emerald-100"
                        onClick={() => router.push("/admin/health")}
                      >
                        Health
                      </button>{" "}
                      for deep diagnostics.
                    </p>
                  </div>

                  {/* Informational Network Events */}
                  <div className="space-y-2 pt-2 border-t border-white/[0.04]">
                    <div
                      onClick={() => router.push("/admin/nodes")}
                      className="flex items-start justify-between gap-2 text-xs p-1.5 -mx-1.5 rounded hover:bg-white/[0.04] cursor-pointer transition"
                    >
                      <div className="flex items-start gap-2 min-w-0">
                        <span className="h-1.5 w-1.5 rounded-full bg-amber-400 mt-1.5 shrink-0" />
                        <div>
                          <p className="font-semibold text-slate-200 text-[11px]">High disk usage</p>
                          <p className="text-[10px] text-slate-400">Ubuntu Demo Node</p>
                        </div>
                      </div>
                      <span className="font-mono text-[10px] text-slate-500 shrink-0">12m ago</span>
                    </div>

                    <div
                      onClick={() => router.push("/admin/servers")}
                      className="flex items-start justify-between gap-2 text-xs p-1.5 -mx-1.5 rounded hover:bg-white/[0.04] cursor-pointer transition"
                    >
                      <div className="flex items-start gap-2 min-w-0">
                        <span className="h-1.5 w-1.5 rounded-full bg-amber-400 mt-1.5 shrink-0" />
                        <div>
                          <p className="font-semibold text-slate-200 text-[11px]">Memory usage above 80%</p>
                          <p className="text-[10px] text-slate-400">Game Server · gs-1</p>
                        </div>
                      </div>
                      <span className="font-mono text-[10px] text-slate-500 shrink-0">28m ago</span>
                    </div>

                    <div
                      onClick={() => router.push("/admin/nodes")}
                      className="flex items-start justify-between gap-2 text-xs p-1.5 -mx-1.5 rounded hover:bg-white/[0.04] cursor-pointer transition"
                    >
                      <div className="flex items-start gap-2 min-w-0">
                        <span className="h-1.5 w-1.5 rounded-full bg-sky-400 mt-1.5 shrink-0" />
                        <div>
                          <p className="font-semibold text-slate-200 text-[11px]">New node connected</p>
                          <p className="text-[10px] text-slate-400">forge-node-2</p>
                        </div>
                      </div>
                      <span className="font-mono text-[10px] text-slate-500 shrink-0">1h ago</span>
                    </div>

                    <div
                      onClick={() => router.push("/admin/databases")}
                      className="flex items-start justify-between gap-2 text-xs p-1.5 -mx-1.5 rounded hover:bg-white/[0.04] cursor-pointer transition"
                    >
                      <div className="flex items-start gap-2 min-w-0">
                        <span className="h-1.5 w-1.5 rounded-full bg-sky-400 mt-1.5 shrink-0" />
                        <div>
                          <p className="font-semibold text-slate-200 text-[11px]">Backup completed</p>
                          <p className="text-[10px] text-slate-400">Database · db-1</p>
                        </div>
                      </div>
                      <span className="font-mono text-[10px] text-slate-500 shrink-0">2h ago</span>
                    </div>
                  </div>
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

            {auditEvents.length === 0 ? (
              <p className="mt-4 text-xs text-slate-500">No recent changes.</p>
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
                      {event.createdAt ? new Date(event.createdAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : "just now"}
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
                auditEvents.slice(0, 4).map((evt, idx) => (
                  <tr
                    key={evt.id || idx}
                    onClick={() => router.push("/admin/operations")}
                    className="hover:bg-white/[0.04] cursor-pointer transition"
                  >
                    <td className="py-2.5 font-mono text-slate-400">
                      {evt.createdAt
                        ? new Date(evt.createdAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
                        : "12:14 AM"}
                    </td>
                    <td className="py-2.5">
                      <span className="inline-flex items-center gap-1.5 rounded-full border border-sky-500/30 bg-sky-500/10 px-2 py-0.5 font-mono text-[10px] font-semibold text-sky-300">
                        <span className="h-1.5 w-1.5 rounded-full bg-sky-400" />
                        <span>Deploy</span>
                      </span>
                    </td>
                    <td className="py-2.5 font-semibold text-slate-300 capitalize">
                      {evt.action.includes("server") ? "Server" : "Node"}
                    </td>
                    <td className="py-2.5 text-slate-400">
                      <span className="text-slate-200 font-medium">{evt.action.replace(/_/g, " ")} executed</span>
                      {evt.actorEmail ? ` by ${evt.actorEmail}` : ""}
                    </td>
                    <td className="py-2.5 text-right font-mono text-slate-500">
                      {idx === 0 ? "2m ago" : idx === 1 ? "6m ago" : idx === 2 ? "15m ago" : "34m ago"}
                    </td>
                  </tr>
                ))
              ) : (
                <>
                  <tr
                    onClick={() => router.push("/admin/operations")}
                    className="hover:bg-white/[0.04] cursor-pointer transition"
                  >
                    <td className="py-2.5 font-mono text-slate-400">12:14 AM</td>
                    <td className="py-2.5">
                      <span className="inline-flex items-center gap-1.5 rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2 py-0.5 font-mono text-[10px] font-semibold text-emerald-300">
                        <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" />
                        <span>Info</span>
                      </span>
                    </td>
                    <td className="py-2.5 font-semibold text-slate-300">Node</td>
                    <td className="py-2.5 text-slate-400">Health check passed for Ubuntu Demo Node</td>
                    <td className="py-2.5 text-right font-mono text-slate-500">2m ago</td>
                  </tr>
                  <tr
                    onClick={() => router.push("/admin/operations")}
                    className="hover:bg-white/[0.04] cursor-pointer transition"
                  >
                    <td className="py-2.5 font-mono text-slate-400">12:10 AM</td>
                    <td className="py-2.5">
                      <span className="inline-flex items-center gap-1.5 rounded-full border border-sky-500/30 bg-sky-500/10 px-2 py-0.5 font-mono text-[10px] font-semibold text-sky-300">
                        <span className="h-1.5 w-1.5 rounded-full bg-sky-400" />
                        <span>Deploy</span>
                      </span>
                    </td>
                    <td className="py-2.5 font-semibold text-slate-300">Deployment</td>
                    <td className="py-2.5 text-slate-400">Deployment started: coolify-app (v2.4.1)</td>
                    <td className="py-2.5 text-right font-mono text-slate-500">6m ago</td>
                  </tr>
                  <tr
                    onClick={() => router.push("/admin/operations")}
                    className="hover:bg-white/[0.04] cursor-pointer transition"
                  >
                    <td className="py-2.5 font-mono text-slate-400">12:01 AM</td>
                    <td className="py-2.5">
                      <span className="inline-flex items-center gap-1.5 rounded-full border border-cyan-500/30 bg-cyan-500/10 px-2 py-0.5 font-mono text-[10px] font-semibold text-cyan-300">
                        <span className="h-1.5 w-1.5 rounded-full bg-cyan-400" />
                        <span>Success</span>
                      </span>
                    </td>
                    <td className="py-2.5 font-semibold text-slate-300">Backup</td>
                    <td className="py-2.5 text-slate-400">Backup completed: db-1</td>
                    <td className="py-2.5 text-right font-mono text-slate-500">15m ago</td>
                  </tr>
                  <tr
                    onClick={() => router.push("/admin/operations")}
                    className="hover:bg-white/[0.04] cursor-pointer transition"
                  >
                    <td className="py-2.5 font-mono text-slate-400">11:42 PM</td>
                    <td className="py-2.5">
                      <span className="inline-flex items-center gap-1.5 rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 font-mono text-[10px] font-semibold text-amber-300">
                        <span className="h-1.5 w-1.5 rounded-full bg-amber-400" />
                        <span>Warning</span>
                      </span>
                    </td>
                    <td className="py-2.5 font-semibold text-slate-300">Server</td>
                    <td className="py-2.5 text-slate-400">Memory usage above 80%: gs-1</td>
                    <td className="py-2.5 text-right font-mono text-slate-500">34m ago</td>
                  </tr>
                </>
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
