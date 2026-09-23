"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import {
  AlertTriangle,
  ArrowUpRight,
  CheckCircle,
  ChevronDown,
  ChevronRight,
  ExternalLink,
  MinusCircle,
  RefreshCw,
  Wrench,
  XCircle,
} from "lucide-react";
import {
  HealthECGIcon,
  NodeHostIcon,
  ServerRackIcon,
  ApplicationsCubeIcon,
  DatabaseCylinderIcon,
  ActivityWaveIcon,
  MemoryRamStickIcon,
  PipelineFlowIcon,
  SystemHealthOperationalIcon,
  SystemHealthAlertIcon,
} from "@/components/ui/forge-icons";
import { PageInfoDisclosure } from "@/components/ui/page-info-disclosure";
import {
  fetchAdminActivity,
  fetchHealthStatus,
  fetchNodes,
  fetchRecoveryPlans,
  fetchReservations,
  fetchServers,
  type ApiHealthCheck,
} from "@/lib/api";
import { Btn, EmptyState, Pill, cn } from "./admin-ui";

type MonitorSection =
  | "infrastructure"
  | "platform"
  | "resources"
  | "workloads"
  | "database"
  | "cache"
  | "queue"
  | "api"
  | "daemon"
  | "orchestration";

export type { MonitorSection };

function mbLabel(value: number) {
  if (value >= 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} TB`;
  if (value >= 1024) return `${(value / 1024).toFixed(1)} GB`;
  return `${Math.round(value)} MB`;
}

function detail(check: ApiHealthCheck | undefined, key: string) {
  return check?.details?.[key];
}

function bytesLabel(value: unknown) {
  const bytes = Number(value);
  if (!Number.isFinite(bytes) || bytes < 0) return undefined;
  if (bytes >= 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`;
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${Math.round(bytes)} B`;
}

function secondsLabel(value: unknown) {
  const seconds = Number(value);
  if (!Number.isFinite(seconds) || seconds < 0) return undefined;
  if (seconds >= 86_400) return `${(seconds / 86_400).toFixed(1)} days`;
  if (seconds >= 3_600) return `${(seconds / 3_600).toFixed(1)} hours`;
  if (seconds >= 60) return `${Math.round(seconds / 60)} min`;
  return `${Math.round(seconds)}s`;
}

function checkStatus(healthAvailable: boolean, check: ApiHealthCheck | undefined) {
  if (!healthAvailable) return "Unavailable";
  return check?.status ?? "Not reported";
}

function queryErrorMessage(error: unknown) {
  return error instanceof Error && error.message ? error.message : "Unknown API error";
}

function statusIcon(status?: string, size = 16) {
  if (status === "ok" || status === "online") return <CheckCircle size={size} className="text-emerald-400" />;
  if (status === "failed" || status === "offline") return <XCircle size={size} className="text-red-400" />;
  if (status === "warning" || status === "degraded") return <AlertTriangle size={size} className="text-amber-400" />;
  return <MinusCircle size={size} className="text-slate-500" />;
}

function healthByName(checks: ApiHealthCheck[], name: string) {
  return Array.isArray(checks) ? checks.find((check) => check.name === name) : undefined;
}

function remediationFor(check: ApiHealthCheck | undefined): string | null {
  if (!check || check.status === "ok") return null;
  const name = check.name;
  if (!name) return null;
  const msg = check.notificationMessage?.toLowerCase() ?? "";
  if (name === "database") {
    if (msg.includes("connect") || msg.includes("reachable"))
      return "Check database credentials and ensure the database server is running. Verify network connectivity between the API and database host.";
    if (msg.includes("migration"))
      return "Database migrations are pending. Run the migration command to apply pending schema changes.";
    return "Review database connection settings and server logs for details.";
  }
  if (name === "cache") {
    return "Verify Redis connection settings and that the Redis server is running. Check for authentication or network issues.";
  }
  if (name === "queue") {
    return "The queue worker may not be running. Restart the queue worker process and check worker logs for startup errors.";
  }
  if (name === "daemon" || name === "heartbeat") {
    return "Nodes without recent heartbeats may be offline, network-isolated, or running an incompatible agent version. Check node connectivity and review the node log.";
  }
  if (name === "api" || name === "system") {
    return "System resource constraints (memory, goroutine leaks) may cause instability. Review API runtime metrics and consider a restart.";
  }
  if (name === "memory") {
    return "High memory usage may cause OOM kills. Increase available memory or reduce workload allocation.";
  }
  return null;
}

function MetricTile({
  label,
  value,
  status,
  onClick,
}: {
  label: string;
  value: string;
  status?: string;
  onClick?: () => void;
}) {
  return (
    <div
      onClick={onClick}
      className={cn(
        "rounded-xl border border-white/[0.08] bg-[var(--surface)] p-4 shadow-sm transition",
        onClick && "cursor-pointer hover:border-white/20 hover:bg-white/[0.02]"
      )}
    >
      <p className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">{label}</p>
      <div className="mt-1.5 flex items-center justify-between gap-2">
        <p className="font-mono text-sm sm:text-base font-bold text-slate-100 tabular-nums truncate">{value}</p>
        {status && <span className="shrink-0">{statusIcon(status, 14)}</span>}
      </div>
    </div>
  );
}

function HealthSection({
  title,
  icon: Icon,
  children,
  defaultOpen = true,
}: {
  title: string;
  icon: React.ElementType;
  children: React.ReactNode;
  defaultOpen?: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <div className="rounded-2xl border border-white/[0.08] bg-[var(--surface)] shadow-sm overflow-hidden transition-all">
      <button
        className="flex w-full items-center justify-between px-5 py-4 text-left transition hover:bg-white/[0.02] cursor-pointer"
        onClick={() => setOpen(!open)}
        type="button"
        aria-expanded={open}
      >
        <div className="flex items-center gap-2.5 text-sm font-bold text-slate-100">
          <div className="grid h-7 w-7 place-items-center rounded-lg border border-white/[0.08] bg-white/[0.03] text-slate-400">
            <Icon size={15} />
          </div>
          <span>{title}</span>
        </div>
        <div className="flex items-center gap-2 text-xs font-semibold text-slate-400">
          <span>{open ? "Collapse" : "Expand"}</span>
          {open ? <ChevronDown size={14} className="text-slate-400" /> : <ChevronRight size={14} className="text-slate-400" />}
        </div>
      </button>
      {open && <div className="border-t border-white/[0.06] p-5 space-y-4 bg-black/10">{children}</div>}
    </div>
  );
}

export function AdminHealth({
  initialSection = "infrastructure",
  overview = false,
}: {
  initialSection?: MonitorSection;
  overview?: boolean;
}) {
  const router = useRouter();
  const [selected, setSelected] = useState<MonitorSection>(initialSection);
  const lastRefreshedRef = useRef<Date | null>(null);

  const poll = { refetchInterval: 30_000, refetchIntervalInBackground: false } as const;
  const healthQuery = useQuery({ queryKey: ["health"], queryFn: fetchHealthStatus, ...poll });
  const nodesQuery = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes, ...poll });
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: fetchServers, ...poll });
  const reservationsQuery = useQuery({ queryKey: ["reservations"], queryFn: fetchReservations, retry: false, ...poll });
  const recoveryQuery = useQuery({ queryKey: ["recovery"], queryFn: fetchRecoveryPlans, retry: false, ...poll });
  const activityQuery = useQuery({
    queryKey: ["admin-activity", "monitoring"],
    queryFn: () => fetchAdminActivity({ limit: 1 }),
    retry: false,
    ...poll,
  });

  const queryClient = useQueryClient();
  useEffect(() => {
    const refresh = () => {
      if (document.visibilityState === "visible") {
        void queryClient.invalidateQueries({ queryKey: ["health"] });
        void queryClient.invalidateQueries({ queryKey: ["nodes"] });
        void queryClient.invalidateQueries({ queryKey: ["servers"] });
        void queryClient.invalidateQueries({ queryKey: ["reservations"] });
        void queryClient.invalidateQueries({ queryKey: ["recovery"] });
        void queryClient.invalidateQueries({ queryKey: ["admin-activity"] });
      }
    };
    document.addEventListener("visibilitychange", refresh);
    return () => document.removeEventListener("visibilitychange", refresh);
  }, [queryClient]);

  const checks = useMemo(() => healthQuery.data?.checks ?? [], [healthQuery.data?.checks]);
  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);
  const servers = useMemo(() => serversQuery.data ?? [], [serversQuery.data]);
  const reservations = useMemo(() => reservationsQuery.data ?? [], [reservationsQuery.data]);
  const recoveryPlans = useMemo(() => recoveryQuery.data ?? [], [recoveryQuery.data]);

  const nodesAvailable = !nodesQuery.isLoading && !nodesQuery.isError && nodesQuery.data !== undefined;
  const serversAvailable = !serversQuery.isLoading && !serversQuery.isError && serversQuery.data !== undefined;
  const reservationsAvailable = !reservationsQuery.isLoading && !reservationsQuery.isError && reservationsQuery.data !== undefined;
  const recoveriesAvailable = !recoveryQuery.isLoading && !recoveryQuery.isError && recoveryQuery.data !== undefined;
  const activityAvailable = !activityQuery.isLoading && !activityQuery.isError && activityQuery.data !== undefined;
  const healthAvailable = !healthQuery.isLoading && !healthQuery.isError && healthQuery.data !== undefined;

  const database = healthByName(checks, "database");
  const cache = healthByName(checks, "cache");
  const queue = healthByName(checks, "queue");
  const daemon = healthByName(checks, "daemon");
  const memory = healthByName(checks, "memory");
  const system = healthByName(checks, "system");

  const summary = useMemo(() => {
    const healthyNodes = nodes.filter((node) => node.heartbeatState === "healthy").length;
    const degradedNodes = nodes.filter(
      (node) => node.heartbeatState === "degraded" || node.heartbeatState === "suspected"
    ).length;
    const expectedOfflineNodes = nodes.filter((node) => node.maintenanceMode).length;
    const unexpectedOfflineNodes = nodes.filter(
      (node) =>
        !node.maintenanceMode &&
        node.heartbeatState !== "healthy" &&
        node.heartbeatState !== "degraded" &&
        node.heartbeatState !== "suspected" &&
        node.heartbeatState !== "unknown"
    ).length;
    const runningServers = servers.filter((server) => server.status === "running").length;
    const stoppedServers = servers.filter((server) => server.status === "stopped").length;
    const suspendedServers = servers.filter((server) => server.suspended).length;
    const failedDeployments = servers.filter((server) => ["failed", "install_failed"].includes(server.status)).length;
    const configuredMemory = nodes.reduce((sum, node) => sum + (node.memoryMb ?? 0), 0);
    const configuredDisk = nodes.reduce((sum, node) => sum + (node.diskMb ?? 0), 0);
    const hasConfiguredMemory = nodes.some((node) => node.memoryMb != null);
    const hasConfiguredDisk = nodes.some((node) => node.diskMb != null);
    const failedChecks = checks.filter((c) => c.status === "failed");
    const warningChecks = checks.filter((c) => c.status === "warning");
    return {
      healthyNodes,
      degradedNodes,
      expectedOfflineNodes,
      unexpectedOfflineNodes,
      totalNodes: nodes.length,
      runningServers,
      stoppedServers,
      suspendedServers,
      failedDeployments,
      totalServers: servers.length,
      configuredMemory,
      configuredDisk,
      hasConfiguredMemory,
      hasConfiguredDisk,
      failedChecks,
      warningChecks,
    };
  }, [nodes, servers, checks]);

  const lastRefreshed = healthQuery.data?.checkedAt
    ? new Date(healthQuery.data.checkedAt)
    : nodesQuery.data?.[0]?.lastSeenAt
    ? new Date(nodesQuery.data[0].lastSeenAt)
    : lastRefreshedRef.current;

  function refresh() {
    lastRefreshedRef.current = new Date();
    void healthQuery.refetch();
    void nodesQuery.refetch();
    void serversQuery.refetch();
    void reservationsQuery.refetch();
    void recoveryQuery.refetch();
    void activityQuery.refetch();
  }

  const isFetching =
    healthQuery.isFetching ||
    nodesQuery.isFetching ||
    serversQuery.isFetching ||
    reservationsQuery.isFetching ||
    recoveryQuery.isFetching ||
    activityQuery.isFetching;

  const activeReservations = Array.isArray(reservations)
    ? reservations.filter((item) => !["completed", "cancelled", "canceled", "used", "expired", "failed"].includes(item.status)).length
    : 0;
  const failedReservations = Array.isArray(reservations)
    ? reservations.filter((item) => item.status === "failed").length
    : 0;
  const activeRecoveries = Array.isArray(recoveryPlans)
    ? recoveryPlans.filter((item) => !["completed", "cancelled", "canceled", "restored", "failed"].includes(item.status)).length
    : 0;
  const failedRecoveries = Array.isArray(recoveryPlans)
    ? recoveryPlans.filter((item) => item.status === "failed").length
    : 0;

  const overallStatus = healthAvailable ? healthQuery.data.status : "unknown";
  const hasFailures =
    summary.failedChecks.length > 0 ||
    summary.failedDeployments > 0 ||
    failedReservations > 0 ||
    failedRecoveries > 0;
  const hasWarnings = summary.warningChecks.length > 0 || summary.degradedNodes > 0;

  const selectSection = (section: MonitorSection) => {
    setSelected(section);
    if (typeof window !== "undefined") {
      window.history.pushState(
        null,
        "",
        section === "infrastructure" && overview ? "/admin/monitoring" : `/admin/monitoring/${section}`
      );
    }
  };

  const queryErrors = [
    {
      title: "Health",
      message: "Monitoring checks could not be loaded",
      refetch: () => {
        void healthQuery.refetch();
      },
      isError: healthQuery.isError,
      tone: "red" as const,
    },
    {
      title: "Nodes",
      message: "Nodes could not be loaded",
      refetch: () => {
        void nodesQuery.refetch();
      },
      isError: nodesQuery.isError,
      tone: "red" as const,
    },
    {
      title: "Servers",
      message: "Servers could not be loaded",
      refetch: () => {
        void serversQuery.refetch();
      },
      isError: serversQuery.isError,
      tone: "red" as const,
    },
    {
      title: "Reservations",
      message: "Reservation data could not be loaded; counts unavailable",
      refetch: () => {
        void reservationsQuery.refetch();
      },
      isError: reservationsQuery.isError,
      tone: "amber" as const,
    },
    {
      title: "Recovery",
      message: "Recovery plan data could not be loaded; counts unavailable",
      refetch: () => {
        void recoveryQuery.refetch();
      },
      isError: recoveryQuery.isError,
      tone: "amber" as const,
    },
    {
      title: "Activity",
      message: "Platform activity could not be loaded; counts unavailable",
      refetch: () => {
        void activityQuery.refetch();
      },
      isError: activityQuery.isError,
      tone: "amber" as const,
    },
  ].filter((e) => e.isError);

  return (
    <div className="space-y-6 max-w-[1440px] mx-auto">
      {/* ========================================================================= */}
      {/* ZONE 1: MISSION CONTROL HEADER WITH REPUTATION BADGES & ACTION TOOLS      */}
      {/* ========================================================================= */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 border-b border-white/[0.08] pb-5">
        <div>
          <div className="flex items-center gap-2">
            <span className="text-xs font-bold uppercase tracking-wider text-[var(--brand)]">Control Plane</span>
            <span className="text-slate-600">/</span>
            <span className="text-xs font-semibold text-slate-400">Diagnostics</span>
          </div>
          <h1 className="mt-1 text-2xl sm:text-3xl font-bold tracking-tight text-slate-100 flex items-center gap-2.5">
            <span>Health</span>
            <PageInfoDisclosure
              title="Health Diagnostics & Verification"
              eyebrow="Control Plane Verification"
              description="Live evaluation of control-plane dependencies, fleet heartbeats, database availability, and workload health."
              sections={[
                {
                  title: "Active Verification vs. Passive Telemetry",
                  icon: HealthECGIcon,
                  content:
                    "Health performs active connectivity, latency, and heartbeat checks against dependencies. For historical resource telemetry over time, consult Monitoring.",
                },
                {
                  title: "Beacon Fleet Heartbeats",
                  icon: NodeHostIcon,
                  content:
                    "Per-host daemons check in periodically. Missing heartbeats transition through suspected and degraded states before being marked unexpectedly offline.",
                },
                {
                  title: "Dependency Isolation",
                  icon: DatabaseCylinderIcon,
                  content:
                    "Postgres and Redis are checked independently with millisecond round-trip probes and migration state verification.",
                },
                {
                  title: "Remediation & Action Guidance",
                  icon: Wrench,
                  content:
                    "When checks fail, actionable instructions guide credential verification, service restarts, or capacity reallocations.",
                },
              ]}
            />
          </h1>
          <p className="mt-1 text-xs sm:text-sm text-slate-400">
            What&apos;s wrong — failures, degraded subsystems and remediation steps. For what happens over time see Monitoring; for what to know now see Overview.
          </p>
        </div>

        {/* Global Toolbar Controls */}
        <div className="flex items-center gap-2.5 shrink-0">
          {lastRefreshed && (
            <span className="hidden md:inline font-mono text-[11px] text-slate-500">
              Last checked {lastRefreshed.toLocaleString()}
            </span>
          )}
          <Btn onClick={refresh} disabled={isFetching}>
            <RefreshCw className={isFetching ? "animate-spin" : ""} size={14} /> Refresh
          </Btn>
        </div>
      </div>

      {queryErrors.length > 0 && (
        <div className="space-y-2">
          {queryErrors.map((e) => (
            <div
              key={e.title}
              className={cn(
                "flex items-start justify-between gap-4 rounded-xl border p-3.5 text-sm shadow-sm",
                e.tone === "red"
                  ? "border-red-500/25 bg-red-950/20 text-red-200"
                  : "border-amber-500/25 bg-amber-950/20 text-amber-200"
              )}
            >
              <span>
                {e.message}:{" "}
                {(() => {
                  const err =
                    e.title === "Health"
                      ? healthQuery.error
                      : e.title === "Nodes"
                      ? nodesQuery.error
                      : e.title === "Servers"
                      ? serversQuery.error
                      : e.title === "Reservations"
                      ? reservationsQuery.error
                      : e.title === "Recovery"
                      ? recoveryQuery.error
                      : activityQuery.error;
                  return queryErrorMessage(err);
                })()}
              </span>
              <Btn size="sm" tone="ghost" onClick={e.refetch}>
                Retry
              </Btn>
            </div>
          ))}
        </div>
      )}

      {/* ========================================================================= */}
      {/* ZONE 2: OVERALL PLATFORM OPERATIONAL STATE CARD                           */}
      {/* ========================================================================= */}
      <div
        className={cn(
          "rounded-2xl border p-5 shadow-sm transition-all relative overflow-hidden",
          overallStatus === "ok"
            ? "border-emerald-500/25 bg-emerald-500/[0.03]"
            : overallStatus === "warning"
            ? "border-amber-500/25 bg-amber-500/[0.03]"
            : "border-red-500/25 bg-red-500/[0.03]"
        )}
      >
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
          <div className="flex items-center gap-4">
            <div
              onClick={() => router.push("/admin/overview")}
              className="shrink-0 transition-transform hover:scale-105 cursor-pointer"
              title="Overview"
            >
              {overallStatus === "ok" ? (
                <SystemHealthOperationalIcon size={44} />
              ) : (
                <SystemHealthAlertIcon size={44} />
              )}
            </div>
            <div>
              <div className="text-[11px] font-semibold uppercase tracking-wider text-slate-400">
                Control Plane Status
              </div>
              <p
                className={cn(
                  "text-xl font-bold tracking-tight",
                  overallStatus === "ok"
                    ? "text-emerald-300"
                    : overallStatus === "warning"
                    ? "text-amber-300"
                    : "text-red-300"
                )}
              >
                {healthQuery.isLoading
                  ? "Loading..."
                  : healthQuery.isError
                  ? "Unavailable"
                  : overallStatus === "ok"
                  ? "All Systems Operational"
                  : overallStatus === "warning"
                  ? "Degraded Performance"
                  : "System Issues Detected"}
              </p>
              <p className="mt-0.5 text-xs text-slate-400 font-mono">
                {summary.totalNodes} nodes · {summary.totalServers} workloads
                {healthQuery.data?.uptime ? ` · Uptime ${secondsLabel(healthQuery.data.uptime)}` : ""}
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2">
            {healthQuery.isLoading ? (
              <Pill tone="neutral">Checking...</Pill>
            ) : (
              <>
                {hasFailures && (
                  <Pill tone="red">
                    {summary.failedChecks.length +
                      summary.failedDeployments +
                      failedReservations +
                      failedRecoveries}{" "}
                    failures
                  </Pill>
                )}
                {hasWarnings && !hasFailures && (
                  <Pill tone="yellow">
                    {summary.warningChecks.length + summary.degradedNodes} warnings
                  </Pill>
                )}
                {!hasFailures && !hasWarnings && <Pill tone="green">All healthy</Pill>}
              </>
            )}
            <button
              type="button"
              onClick={() => router.push("/admin/overview")}
              className="hidden sm:inline-flex items-center gap-1 rounded-lg border border-white/[0.08] bg-white/[0.04] px-2.5 py-1 text-xs font-semibold text-slate-300 hover:bg-white/[0.08] hover:text-white transition cursor-pointer"
            >
              <span>Overview</span>
              <ArrowUpRight size={13} />
            </button>
          </div>
        </div>
      </div>

      {/* Refetching indicator */}
      {isFetching && (
        <div className="flex items-center gap-2 rounded-xl border border-sky-500/20 bg-sky-950/20 p-2.5 text-xs text-sky-300">
          <RefreshCw size={13} className="animate-spin" />
          <span>Refreshing health check diagnostics...</span>
        </div>
      )}

      {/* ========================================================================= */}
      {/* ZONE 3: SUBSYSTEM STATUS TILES (Clickable, Informative, Grouped)          */}
      {/* ========================================================================= */}
      <div className="grid gap-3.5 sm:grid-cols-2 lg:grid-cols-4">
        {/* Infrastructure */}
        <button
          className={cn(
            "rounded-xl border p-4 text-left transition-all cursor-pointer relative overflow-hidden group",
            selected === "infrastructure"
              ? "border-[var(--brand)]/50 bg-[var(--surface-raised)] ring-1 ring-[var(--brand)]/30 shadow-md"
              : "border-white/[0.08] bg-[var(--surface)] hover:border-white/20 hover:bg-white/[0.02]"
          )}
          onClick={() => selectSection("infrastructure")}
          type="button"
        >
          <div className="flex items-center justify-between">
            <span className="flex items-center gap-1.5 text-[11px] font-bold uppercase tracking-wider text-slate-400 group-hover:text-slate-300">
              <ServerRackIcon size={14} className="text-sky-400" />
              <span>Infrastructure</span>
            </span>
            {statusIcon(nodes.length === 0 ? undefined : summary.unexpectedOfflineNodes > 0 ? "offline" : "ok", 14)}
          </div>
          <p className="mt-2.5 font-mono text-xl font-bold tracking-tight text-slate-100">
            {nodesQuery.isLoading ? "..." : nodesQuery.isError ? "Unavailable" : `${summary.healthyNodes}/${summary.totalNodes} nodes`}
          </p>
          <p className="mt-1 text-xs text-slate-400">
            {nodesQuery.isError
              ? "API unreachable"
              : summary.totalNodes === 0
              ? "Register a node to begin hosting workloads"
              : summary.expectedOfflineNodes > 0
              ? `${summary.expectedOfflineNodes} in maintenance`
              : summary.unexpectedOfflineNodes > 0
              ? `${summary.unexpectedOfflineNodes} offline unexpectedly`
              : summary.degradedNodes > 0
              ? `${summary.degradedNodes} degraded`
              : "All nodes healthy"}
          </p>
        </button>

        {/* Workloads */}
        <button
          className={cn(
            "rounded-xl border p-4 text-left transition-all cursor-pointer relative overflow-hidden group",
            selected === "workloads"
              ? "border-[var(--brand)]/50 bg-[var(--surface-raised)] ring-1 ring-[var(--brand)]/30 shadow-md"
              : "border-white/[0.08] bg-[var(--surface)] hover:border-white/20 hover:bg-white/[0.02]"
          )}
          onClick={() => selectSection("workloads")}
          type="button"
        >
          <div className="flex items-center justify-between">
            <span className="flex items-center gap-1.5 text-[11px] font-bold uppercase tracking-wider text-slate-400 group-hover:text-slate-300">
              <ApplicationsCubeIcon size={14} className="text-purple-400" />
              <span>Workloads</span>
            </span>
            {statusIcon(serversAvailable ? (summary.failedDeployments > 0 ? "failed" : "ok") : undefined, 14)}
          </div>
          <p className="mt-2.5 font-mono text-xl font-bold tracking-tight text-slate-100">
            {serversQuery.isLoading ? "..." : serversQuery.isError ? "Unavailable" : `${summary.runningServers} running`}
          </p>
          <p className="mt-1 text-xs text-slate-400">
            {serversQuery.isError
              ? "Server data unavailable"
              : `${summary.stoppedServers} stopped · ${summary.suspendedServers} suspended${
                  summary.failedDeployments > 0 ? ` · ${summary.failedDeployments} failed` : ""
                }`}
          </p>
        </button>

        {/* API & Queue */}
        <button
          className={cn(
            "rounded-xl border p-4 text-left transition-all cursor-pointer relative overflow-hidden group",
            selected === "platform"
              ? "border-[var(--brand)]/50 bg-[var(--surface-raised)] ring-1 ring-[var(--brand)]/30 shadow-md"
              : "border-white/[0.08] bg-[var(--surface)] hover:border-white/20 hover:bg-white/[0.02]"
          )}
          onClick={() => selectSection("platform")}
          type="button"
        >
          <div className="flex items-center justify-between">
            <span className="flex items-center gap-1.5 text-[11px] font-bold uppercase tracking-wider text-slate-400 group-hover:text-slate-300">
              <ActivityWaveIcon size={14} className="text-emerald-400" />
              <span>API & Queue</span>
            </span>
            {statusIcon(overallStatus, 14)}
          </div>
          <p className="mt-2.5 font-mono text-xl font-bold tracking-tight text-slate-100">
            {checkStatus(healthAvailable, system)}
          </p>
          <p className="mt-1 text-xs text-slate-400">
            Queue {checkStatus(healthAvailable, queue)} · {checks.length} checks
          </p>
        </button>

        {/* Database & Cache */}
        <button
          className={cn(
            "rounded-xl border p-4 text-left transition-all cursor-pointer relative overflow-hidden group",
            selected === "database"
              ? "border-[var(--brand)]/50 bg-[var(--surface-raised)] ring-1 ring-[var(--brand)]/30 shadow-md"
              : "border-white/[0.08] bg-[var(--surface)] hover:border-white/20 hover:bg-white/[0.02]"
          )}
          onClick={() => selectSection("database")}
          type="button"
        >
          <div className="flex items-center justify-between">
            <span className="flex items-center gap-1.5 text-[11px] font-bold uppercase tracking-wider text-slate-400 group-hover:text-slate-300">
              <DatabaseCylinderIcon size={14} className="text-orange-400" />
              <span>Database & Cache</span>
            </span>
            {statusIcon(
              database?.status === "ok" && cache?.status === "ok"
                ? "ok"
                : database?.status === "failed"
                ? "failed"
                : cache?.status === "failed"
                ? "failed"
                : undefined,
              14
            )}
          </div>
          <p className="mt-2.5 font-mono text-xl font-bold tracking-tight text-slate-100">
            {!healthAvailable ? "Unavailable" : `${database?.status ?? "?"}`}
          </p>
          <p className="mt-1 text-xs text-slate-400">
            Cache {!healthAvailable ? "Unavailable" : cache?.status ?? "?"}{" "}
            {database?.latencyMs != null ? `· ${database.latencyMs}ms` : ""}
          </p>
        </button>
      </div>

      {/* ========================================================================= */}
      {/* ZONE 4: ACTIONABLE FAILURES (Incident Remediation Cards)                  */}
      {/* ========================================================================= */}
      {summary.failedChecks.length > 0 && (
        <div className="rounded-2xl border border-red-500/30 bg-red-500/[0.04] p-5 shadow-sm">
          <div className="flex items-center justify-between gap-3 mb-4">
            <h3 className="flex items-center gap-2 text-sm font-bold text-red-300">
              <XCircle size={17} className="text-red-400 stroke-[2.2]" />
              <span>Actionable Failures</span>
              <span className="rounded-full bg-red-500/20 px-2 py-0.5 font-mono text-[10px] font-semibold text-red-300">
                {summary.failedChecks.length} issue{summary.failedChecks.length === 1 ? "" : "s"}
              </span>
            </h3>
            <button
              type="button"
              onClick={() => void healthQuery.refetch()}
              disabled={isFetching}
              className="inline-flex items-center gap-1.5 rounded-lg border border-red-500/30 bg-red-500/10 px-2.5 py-1 text-xs font-semibold text-red-200 hover:bg-red-500/20 transition cursor-pointer"
            >
              <RefreshCw size={12} className={cn(isFetching && "animate-spin")} />
              <span>Retry all</span>
            </button>
          </div>

          <div className="space-y-3">
            {summary.failedChecks.map((check) => {
              const remediation = remediationFor(check);
              return (
                <div
                  key={check.name}
                  className="rounded-xl border border-red-500/20 bg-[var(--surface)] p-4 shadow-sm"
                >
                  <div className="flex flex-col sm:flex-row sm:items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <span className="h-2 w-2 rounded-full bg-red-400 shrink-0" />
                        <p className="text-sm font-bold text-red-200 truncate">
                          {check.name} — {check.notificationMessage ?? "Failed"}
                        </p>
                      </div>
                      {remediation && (
                        <div className="mt-2.5 rounded-lg border border-red-500/15 bg-red-950/20 p-3 text-xs text-slate-300 leading-relaxed">
                          <span className="font-semibold text-red-300 block mb-0.5">Recommended Remediation:</span>
                          {remediation}
                        </div>
                      )}
                    </div>
                    <div className="flex items-center gap-2 shrink-0 self-end sm:self-start">
                      <Btn size="sm" tone="ghost" onClick={() => void healthQuery.refetch()}>
                        <RefreshCw size={12} /> Retry
                      </Btn>
                      {check.name === "database" && (
                        <button
                          type="button"
                          onClick={() => router.push("/admin/databases")}
                          className="inline-flex items-center gap-1 rounded-lg bg-[var(--brand)] px-2.5 py-1 text-xs font-bold text-white hover:bg-[var(--brand-hover)] transition cursor-pointer"
                        >
                          <span>Databases</span>
                          <ExternalLink size={12} />
                        </button>
                      )}
                      {check.name === "daemon" && (
                        <button
                          type="button"
                          onClick={() => router.push("/admin/nodes")}
                          className="inline-flex items-center gap-1 rounded-lg bg-[var(--brand)] px-2.5 py-1 text-xs font-bold text-white hover:bg-[var(--brand-hover)] transition cursor-pointer"
                        >
                          <span>Nodes</span>
                          <ExternalLink size={12} />
                        </button>
                      )}
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* ========================================================================= */}
      {/* ZONE 5: WARNINGS                                                          */}
      {/* ========================================================================= */}
      {summary.warningChecks.length > 0 && (
        <div className="rounded-2xl border border-amber-500/30 bg-amber-500/[0.04] p-5 shadow-sm">
          <div className="flex items-center justify-between gap-3 mb-3">
            <h3 className="flex items-center gap-2 text-sm font-bold text-amber-300">
              <AlertTriangle size={17} className="text-amber-400 stroke-[2.2]" />
              <span>Warnings</span>
              <span className="rounded-full bg-amber-500/20 px-2 py-0.5 font-mono text-[10px] font-semibold text-amber-300">
                {summary.warningChecks.length} warning{summary.warningChecks.length === 1 ? "" : "s"}
              </span>
            </h3>
            <button
              type="button"
              onClick={() => void healthQuery.refetch()}
              className="inline-flex items-center gap-1 text-xs font-semibold text-amber-300 hover:text-amber-200 transition cursor-pointer"
            >
              <RefreshCw size={12} />
              <span>Re-check</span>
            </button>
          </div>

          <div className="space-y-2.5">
            {summary.warningChecks.map((check) => (
              <div
                key={check.name}
                className="flex items-start justify-between gap-3 rounded-xl border border-amber-500/20 bg-[var(--surface)] p-3.5 shadow-sm"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="h-2 w-2 rounded-full bg-amber-400 shrink-0" />
                    <p className="text-sm font-bold text-amber-200">{check.label ?? check.name}</p>
                  </div>
                  {check.notificationMessage && (
                    <p className="mt-1 text-xs text-slate-400 pl-4">{check.notificationMessage}</p>
                  )}
                </div>
                <button
                  type="button"
                  onClick={() => void healthQuery.refetch()}
                  className="inline-flex items-center gap-1 rounded-lg border border-white/[0.08] bg-white/[0.04] px-2.5 py-1 text-xs font-semibold text-slate-300 hover:bg-white/[0.08] hover:text-white transition cursor-pointer shrink-0"
                >
                  <RefreshCw size={11} />
                  <span>Check</span>
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* ========================================================================= */}
      {/* ZONE 6: DETAILED DIAGNOSTIC SECTIONS WITH BESPOKE FORGE ICONS             */}
      {/* ========================================================================= */}
      <HealthSection
        title="Infrastructure Details"
        icon={NodeHostIcon}
        defaultOpen={selected === "infrastructure"}
      >
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          <MetricTile
            label="Healthy Heartbeats"
            value={nodesAvailable ? String(summary.healthyNodes) : "Unavailable"}
            status={summary.healthyNodes > 0 ? "ok" : nodesAvailable && summary.totalNodes > 0 ? "failed" : undefined}
            onClick={() => router.push("/admin/nodes")}
          />
          <MetricTile
            label="In Maintenance"
            value={nodesAvailable ? String(summary.expectedOfflineNodes) : "Unavailable"}
            onClick={() => router.push("/admin/nodes")}
          />
          <MetricTile
            label="Unexpectedly Offline"
            value={nodesAvailable ? String(summary.unexpectedOfflineNodes) : "Unavailable"}
            status={summary.unexpectedOfflineNodes > 0 ? "failed" : undefined}
            onClick={() => router.push("/admin/nodes")}
          />
          <MetricTile
            label="Degraded"
            value={nodesAvailable ? String(summary.degradedNodes) : "Unavailable"}
            status={summary.degradedNodes > 0 ? "warning" : undefined}
            onClick={() => router.push("/admin/nodes")}
          />
          <MetricTile
            label="Daemon Check"
            value={checkStatus(healthAvailable, daemon)}
            status={healthAvailable ? daemon?.status : undefined}
            onClick={() => router.push("/admin/nodes")}
          />
          <MetricTile
            label="Heartbeat Message"
            value={checkStatus(healthAvailable, daemon)}
            status={healthAvailable ? daemon?.status : undefined}
            onClick={() => router.push("/admin/nodes")}
          />
        </div>
        {nodesAvailable && summary.totalNodes > 0 && <NodeTable nodes={nodes} />}
        <div className="flex items-center justify-between border-t border-white/[0.06] pt-3 text-xs">
          <span className="text-slate-400">Heartbeats are transmitted by Beacon host agents over HTTP</span>
          <button
            type="button"
            onClick={() => router.push("/admin/nodes")}
            className="flex items-center gap-1 font-semibold text-slate-300 hover:text-white transition"
          >
            <span>Manage Beacon Nodes</span>
            <ArrowUpRight size={13} />
          </button>
        </div>
      </HealthSection>

      <HealthSection
        title="Database & Cache"
        icon={DatabaseCylinderIcon}
        defaultOpen={selected === "database"}
      >
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          <MetricTile
            label="Database Status"
            value={checkStatus(healthAvailable, database)}
            status={healthAvailable ? database?.status : undefined}
            onClick={() => router.push("/admin/databases")}
          />
          <MetricTile
            label="Cache Status"
            value={checkStatus(healthAvailable, cache)}
            status={healthAvailable ? cache?.status : undefined}
            onClick={() => router.push("/admin/databases")}
          />
          <MetricTile
            label="Database Latency"
            value={healthAvailable && database?.latencyMs != null ? `${database.latencyMs} ms` : "Not reported"}
            onClick={() => router.push("/admin/databases")}
          />
          <MetricTile
            label="Active Connections"
            value={healthAvailable ? String(detail(database, "activeConnections") ?? "Not reported") : "Unavailable"}
            onClick={() => router.push("/admin/databases")}
          />
          <MetricTile
            label="Database Version"
            value={healthAvailable ? String(detail(database, "version") ?? "Not reported") : "Unavailable"}
            onClick={() => router.push("/admin/databases")}
          />
          <MetricTile
            label="Cache Memory"
            value={healthAvailable ? String(detail(cache, "used_memory_human") ?? "Not reported") : "Unavailable"}
            onClick={() => router.push("/admin/databases")}
          />
        </div>
        <div className="flex items-center justify-between border-t border-white/[0.06] pt-3 text-xs">
          <span className="text-slate-400">PostgreSQL store and Redis cache instances</span>
          <button
            type="button"
            onClick={() => router.push("/admin/databases")}
            className="flex items-center gap-1 font-semibold text-slate-300 hover:text-white transition"
          >
            <span>Manage Databases & Backups</span>
            <ArrowUpRight size={13} />
          </button>
        </div>
      </HealthSection>

      <HealthSection
        title="Control-Plane Services"
        icon={HealthECGIcon}
        defaultOpen={selected === "platform"}
      >
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          <MetricTile
            label="API Runtime"
            value={checkStatus(healthAvailable, system)}
            status={healthAvailable ? system?.status : undefined}
            onClick={() => router.push("/admin/settings")}
          />
          <MetricTile
            label="Queue Health"
            value={checkStatus(healthAvailable, queue)}
            status={healthAvailable ? queue?.status : undefined}
            onClick={() => router.push("/admin/activity")}
          />
          <MetricTile
            label="Overall Health"
            value={healthAvailable ? healthQuery.data.status : "Unavailable"}
            status={overallStatus}
          />
          <MetricTile
            label="API Uptime"
            value={healthAvailable ? secondsLabel(healthQuery.data?.uptime) ?? "Not reported" : "Unavailable"}
          />
          <MetricTile
            label="Memory Check"
            value={checkStatus(healthAvailable, memory)}
            status={healthAvailable ? memory?.status : undefined}
          />
          <MetricTile
            label="Active Workers"
            value={healthAvailable ? String(detail(queue, "activeWorkers") ?? "Not reported") : "Unavailable"}
            onClick={() => router.push("/admin/activity")}
          />
        </div>
        <div className="flex items-center justify-between border-t border-white/[0.06] pt-3 text-xs">
          <span className="text-slate-400">Forge Fiber API server and asynchronous queue workers</span>
          <button
            type="button"
            onClick={() => router.push("/admin/operations")}
            className="flex items-center gap-1 font-semibold text-slate-300 hover:text-white transition"
          >
            <span>Platform Operations & Audit</span>
            <ArrowUpRight size={13} />
          </button>
        </div>
      </HealthSection>

      <HealthSection
        title="Workloads"
        icon={ApplicationsCubeIcon}
        defaultOpen={selected === "workloads"}
      >
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          <MetricTile
            label="Running"
            value={serversAvailable ? String(summary.runningServers) : "Unavailable"}
            status={summary.runningServers > 0 ? "ok" : undefined}
            onClick={() => router.push("/admin/servers")}
          />
          <MetricTile
            label="Stopped"
            value={serversAvailable ? String(summary.stoppedServers) : "Unavailable"}
            onClick={() => router.push("/admin/servers")}
          />
          <MetricTile
            label="Suspended"
            value={serversAvailable ? String(summary.suspendedServers) : "Unavailable"}
            onClick={() => router.push("/admin/servers")}
          />
          <MetricTile
            label="Failed"
            value={serversAvailable ? String(summary.failedDeployments) : "Unavailable"}
            status={summary.failedDeployments > 0 ? "failed" : undefined}
            onClick={() => router.push("/admin/servers")}
          />
          <MetricTile
            label="Total Servers"
            value={serversAvailable ? String(summary.totalServers) : "Unavailable"}
            onClick={() => router.push("/admin/servers")}
          />
          <MetricTile
            label="Platform Activity"
            value={activityAvailable ? String(activityQuery.data?.total ?? 0) : "Unavailable"}
            onClick={() => router.push("/admin/audit")}
          />
        </div>
        {summary.failedDeployments > 0 && (
          <div className="rounded-xl border border-red-500/20 bg-red-950/15 p-3.5 text-xs text-red-200">
            <span className="font-semibold block mb-0.5">{summary.failedDeployments} workload(s) are in a failed state.</span>
            Check workload logs for deployment errors and verify that the target node is online and healthy.
          </div>
        )}
        <div className="flex items-center justify-between border-t border-white/[0.06] pt-3 text-xs">
          <span className="text-slate-400">Container apps, game servers, and services across all nodes</span>
          <button
            type="button"
            onClick={() => router.push("/admin/servers")}
            className="flex items-center gap-1 font-semibold text-slate-300 hover:text-white transition"
          >
            <span>Manage All Workloads</span>
            <ArrowUpRight size={13} />
          </button>
        </div>
      </HealthSection>

      <HealthSection
        title="Runtime & Resources"
        icon={MemoryRamStickIcon}
        defaultOpen={selected === "resources"}
      >
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          <MetricTile
            label="Configured Memory"
            value={!nodesAvailable ? "Unavailable" : summary.hasConfiguredMemory ? mbLabel(summary.configuredMemory) : "Not reported"}
            onClick={() => router.push("/admin/monitoring")}
          />
          <MetricTile
            label="Configured Disk"
            value={!nodesAvailable ? "Unavailable" : summary.hasConfiguredDisk ? mbLabel(summary.configuredDisk) : "Not reported"}
            onClick={() => router.push("/admin/monitoring")}
          />
          <MetricTile
            label="Heap Allocated"
            value={
              healthAvailable
                ? bytesLabel(detail(system, "heapAllocMb") != null ? `${detail(system, "heapAllocMb")} MB` : detail(system, "heapAllocBytes")) ??
                  "Not reported"
                : "Unavailable"
            }
            onClick={() => router.push("/admin/monitoring")}
          />
          <MetricTile
            label="Goroutines"
            value={healthAvailable ? String(detail(system, "goroutines") ?? "Not reported") : "Unavailable"}
            onClick={() => router.push("/admin/monitoring")}
          />
          <MetricTile
            label="Go Version"
            value={healthAvailable ? String(detail(system, "goVersion") ?? "Not reported") : "Unavailable"}
          />
          <MetricTile
            label="Platform"
            value={
              healthAvailable
                ? detail(system, "goOS") && detail(system, "goArch")
                  ? `${detail(system, "goOS")}/${detail(system, "goArch")}`
                  : "Not reported"
                : "Unavailable"
            }
          />
        </div>
        <div className="flex items-center justify-between border-t border-white/[0.06] pt-3 text-xs">
          <span className="text-slate-400">Go control-plane runtime heap, goroutines, and OS architecture</span>
          <button
            type="button"
            onClick={() => router.push("/admin/monitoring")}
            className="flex items-center gap-1 font-semibold text-slate-300 hover:text-white transition"
          >
            <span>Open Real-time Monitoring</span>
            <ArrowUpRight size={13} />
          </button>
        </div>
      </HealthSection>

      {selected === "orchestration" && (
        <HealthSection title="Orchestration" icon={PipelineFlowIcon}>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            <MetricTile
              label="Active Reservations"
              value={reservationsAvailable ? String(activeReservations) : "Unavailable"}
              onClick={() => router.push("/admin/reconciliation")}
            />
            <MetricTile
              label="Failed Reservations"
              value={reservationsAvailable ? String(failedReservations) : "Unavailable"}
              status={failedReservations > 0 ? "failed" : undefined}
              onClick={() => router.push("/admin/reconciliation")}
            />
            <MetricTile
              label="Active Recoveries"
              value={recoveriesAvailable ? String(activeRecoveries) : "Unavailable"}
              onClick={() => router.push("/admin/reconciliation")}
            />
            <MetricTile
              label="Failed Recoveries"
              value={recoveriesAvailable ? String(failedRecoveries) : "Unavailable"}
              status={failedRecoveries > 0 ? "failed" : undefined}
              onClick={() => router.push("/admin/reconciliation")}
            />
          </div>
          {(failedReservations > 0 || failedRecoveries > 0) && (
            <div className="rounded-xl border border-red-500/20 bg-red-950/15 p-3.5 text-xs text-red-200">
              {failedReservations > 0 && `Failed reservation jobs indicate resource contention or unavailable nodes. Review node capacity and retry failed reservations.`}
              {failedRecoveries > 0 && ` Failed recovery plans require manual intervention. Check node connectivity and recovery plan configuration.`}
            </div>
          )}
          <div className="flex items-center justify-between border-t border-white/[0.06] pt-3 text-xs">
            <span className="text-slate-400">Resource reservations, allocation locks, and failover recovery plans</span>
            <button
              type="button"
              onClick={() => router.push("/admin/reconciliation")}
              className="flex items-center gap-1 font-semibold text-slate-300 hover:text-white transition"
            >
              <span>Reconciliation Engine</span>
              <ArrowUpRight size={13} />
            </button>
          </div>
        </HealthSection>
      )}
    </div>
  );
}

function NodeTable({ nodes }: { nodes: Awaited<ReturnType<typeof fetchNodes>> }) {
  const router = useRouter();
  if (nodes.length === 0) {
    return <EmptyState icon={NodeHostIcon} message="No nodes are registered; node monitoring will begin after setup." />;
  }
  return (
    <div className="overflow-x-auto rounded-xl border border-white/[0.08] bg-[var(--surface)] shadow-sm">
      <table className="w-full text-left text-xs">
        <thead className="border-b border-white/[0.06] bg-white/[0.02] text-[10px] font-bold uppercase tracking-wider text-slate-400">
          <tr>
            <th className="px-4 py-3">Node</th>
            <th className="px-4 py-3">Status</th>
            <th className="px-4 py-3">Heartbeat</th>
            <th className="px-4 py-3">Docker</th>
            <th className="px-4 py-3 text-right">Actions</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-white/[0.04]">
          {nodes.map((node) => (
            <tr
              key={node.id}
              onClick={() => router.push(`/admin/nodes/${node.id}`)}
              className={cn(
                "hover:bg-white/[0.03] cursor-pointer transition",
                node.maintenanceMode && "opacity-60"
              )}
            >
              <td className="px-4 py-3 font-semibold text-slate-200">
                <div className="flex items-center gap-2">
                  {node.maintenanceMode && <Wrench size={13} className="text-amber-400 shrink-0" />}
                  <span className="font-medium text-slate-100">{node.name}</span>
                </div>
              </td>
              <td className="px-4 py-3">
                <span className="inline-flex items-center gap-1.5 font-mono text-[11px]">
                  {statusIcon(node.actualState, 12)}
                  <span
                    className={
                      node.actualState === "online"
                        ? "text-emerald-300 font-semibold"
                        : node.actualState === "degraded"
                        ? "text-amber-300 font-semibold"
                        : node.maintenanceMode
                        ? "text-amber-400 font-semibold"
                        : "text-slate-400"
                    }
                  >
                    {node.maintenanceMode ? "maintenance" : node.actualState ?? "unknown"}
                  </span>
                </span>
              </td>
              <td className="px-4 py-3 text-slate-400">
                <span className="inline-flex items-center gap-1.5 font-mono text-[11px]">
                  {statusIcon(node.heartbeatState, 12)}
                  <span>{node.heartbeatState ?? "unknown"}</span>
                </span>
              </td>
              <td className="px-4 py-3 font-mono text-[11px] text-slate-400">{node.dockerStatus ?? "unknown"}</td>
              <td className="px-4 py-3 text-right">
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation();
                    router.push(`/admin/nodes/${node.id}`);
                  }}
                  className="inline-flex items-center gap-1 text-slate-400 hover:text-white transition font-semibold cursor-pointer"
                >
                  <span>View</span>
                  <ExternalLink size={12} />
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}