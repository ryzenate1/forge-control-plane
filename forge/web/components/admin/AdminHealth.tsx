"use client";

import { useEffect, useMemo, useState } from "react";
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
import {
  fetchAdminActivity,
  fetchRecoveryPlans,
  fetchReservations,
  fetchServers,
  type ApiHealthCheck,
  type ApiNode,
} from "@/lib/api";
import { Btn, EmptyState, Pill, SectionHeader, cn } from "./admin-ui";
import { LiveHealthChecks } from "./LiveHealthChecks";
import { relativeTime, useHealthQuery, useNodesQuery } from "@/lib/admin/telemetry";

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
  if (status === "ok" || status === "online") return <CheckCircle size={size} className="text-ok" />;
  if (status === "failed" || status === "offline") return <XCircle size={size} className="text-danger" />;
  if (status === "warning" || status === "degraded") return <AlertTriangle size={size} className="text-warn" />;
  return <MinusCircle size={size} className="text-text-muted" />;
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
  hint,
  onClick,
}: {
  label: string;
  value: string;
  status?: string;
  /** What the source actually said, when there is something to say. */
  hint?: string;
  onClick?: () => void;
}) {
  return (
    <div
      onClick={onClick}
      className={cn(
        "rounded-xl border border-line bg-[var(--surface)] p-4 shadow-sm transition",
        onClick && "cursor-pointer hover:border-line-strong hover:bg-overlay-subtle"
      )}
    >
      <p className="text-[10px] font-semibold uppercase tracking-wider text-text-subtle">{label}</p>
      <div className="mt-1.5 flex items-center justify-between gap-2">
        <p className="font-mono text-sm sm:text-base font-bold text-text tabular-nums truncate">{value}</p>
        {status && <span className="shrink-0">{statusIcon(status, 14)}</span>}
      </div>
      {hint ? <p className="mt-1 line-clamp-2 text-[11px] leading-4 text-text-muted">{hint}</p> : null}
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
    <div className="rounded-2xl border border-line bg-[var(--surface)] shadow-sm overflow-hidden transition-all">
      <button
        className="flex w-full items-center justify-between px-5 py-4 text-left transition hover:bg-overlay-subtle cursor-pointer"
        onClick={() => setOpen(!open)}
        type="button"
        aria-expanded={open}
      >
        <div className="flex items-center gap-2.5 text-sm font-bold text-text">
          <div className="grid h-7 w-7 place-items-center rounded-lg border border-line bg-overlay text-text-subtle">
            <Icon size={15} />
          </div>
          <span>{title}</span>
        </div>
        <div className="flex items-center gap-2 text-xs font-semibold text-text-subtle">
          <span>{open ? "Collapse" : "Expand"}</span>
          {open ? <ChevronDown size={14} className="text-text-subtle" /> : <ChevronRight size={14} className="text-text-subtle" />}
        </div>
      </button>
      {open && <div className="border-t border-line p-5 space-y-4 bg-overlay-subtle">{children}</div>}
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

  const poll = { refetchInterval: 30_000, refetchIntervalInBackground: false } as const;
  // Canonical health report — see the note in AdminOverview. The bare
  // ["health"] key this used cached separately from queryKeys.health.report().
  const healthQuery = useHealthQuery();
  const nodesQuery = useNodesQuery();
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


  function refresh() {
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

  // Real freshness for the header badge, which previously read
  // "Live · updated just now" at all times — including before the first fetch
  // returned and after it failed.
  const pageFresh = ((): { label: string; tone: "live" | "stale" | "loading" | "error" } => {
    const sources = [healthQuery, nodesQuery, serversQuery];
    if (sources.every((q) => q.isError)) return { label: "No source responding", tone: "error" };
    const newest = sources.reduce((max, q) => (q.dataUpdatedAt > max ? q.dataUpdatedAt : max), 0);
    if (newest === 0) return { label: "Loading…", tone: "loading" };
    const rel = relativeTime(newest) ?? "at an unknown time";
    if (sources.some((q) => q.isError)) return { label: `Partial · read ${rel}`, tone: "stale" };
    // Sources poll every 30s; past twice that the page is showing stale reads.
    const stale = Date.now() - newest > 75_000;
    return { label: stale ? `Stale · read ${rel}` : `Live · read ${rel}`, tone: stale ? "stale" : "live" };
  })();

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
    <div className="space-y-6">
      <SectionHeader
        title="Diagnostics"
        sub="What's wrong — failures, degraded subsystems and remediation steps. For what happens over time see Monitoring; for what to know now see Overview."
        info={{
          title: "Health Diagnostics & Verification",
          eyebrow: "Control Plane Verification",
          description: "Live evaluation of control-plane dependencies, fleet heartbeats, database availability, and workload health.",
          sections: [
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
          ],
        }}
        status={
          <span className="flex items-center gap-1.5 font-mono text-[11px] text-text-muted">
            <span className="relative flex h-1.5 w-1.5">
              {pageFresh.tone === "live" ? (
                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-ok opacity-60" />
              ) : null}
              <span
                className={cn(
                  "relative inline-flex h-1.5 w-1.5 rounded-full",
                  pageFresh.tone === "live" && "bg-ok",
                  pageFresh.tone === "stale" && "bg-warn",
                  pageFresh.tone === "error" && "bg-danger",
                  pageFresh.tone === "loading" && "bg-text-muted",
                )}
              />
            </span>
            <span>{pageFresh.label}</span>
          </span>
        }
        action={
          <>
            {/* Subsystem filter dropdown */}
            <div className="relative">
              <select
                aria-label="Filter subsystem"
                value={selected}
                onChange={(e) => selectSection(e.target.value as MonitorSection)}
                className="h-8 rounded-md border border-[var(--line)] bg-[var(--surface)] pl-2.5 pr-7 text-xs font-medium text-text shadow-sm transition hover:border-[var(--line-strong)] focus:outline-none focus:ring-1 focus:ring-[var(--brand)] appearance-none cursor-pointer"
              >
                <option value="infrastructure">Infrastructure</option>
                <option value="workloads">Workloads</option>
                {/* "platform", not "api": the Control-Plane Services section
                    opens on `selected === "platform"`, so the old value picked
                    a section that nothing renders. */}
                <option value="platform">API &amp; Queue</option>
                <option value="database">Database &amp; Cache</option>
                <option value="resources">Runtime Resources</option>
                <option value="orchestration">Orchestration</option>
              </select>
              <ChevronDown size={12} className="absolute right-2 top-2.5 pointer-events-none text-text-subtle" />
            </div>

            {/* Quick refresh button */}
            <button
              type="button"
              aria-label="Refresh health checks"
              onClick={refresh}
              className="grid h-8 w-8 place-items-center rounded-lg border border-line bg-overlay-subtle text-text-subtle transition hover:border-line-strong hover:text-text"
            >
              <RefreshCw size={13} className={isFetching ? "animate-spin text-info" : ""} />
            </button>

            {/* Primary Action Button */}
            <button
              type="button"
              onClick={refresh}
              className="flex h-8 items-center gap-1.5 rounded-lg bg-[var(--brand)] px-3.5 text-xs font-bold text-white shadow-sm transition hover:bg-[var(--brand-hover)]"
            >
              <ActivityWaveIcon className="w-3.5 h-3.5" />
              <span>Re-check</span>
            </button>
          </>
        }
      />

      <LiveHealthChecks />

      {queryErrors.length > 0 && (
        <div className="space-y-2">
          {queryErrors.map((e) => (
            <div
              key={e.title}
              className={cn(
                "flex items-start justify-between gap-4 rounded-xl border p-3.5 text-sm shadow-sm",
                e.tone === "red"
                  ? "border-danger-line bg-danger-subtle text-danger"
                  : "border-warn-line bg-warn-subtle text-warn"
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
      {/* `overallStatus` is "unknown" until the report is read, so the card
          used to sit in an alarm-red frame while still loading. Unknown gets
          a neutral frame; red is reserved for a status the report stated. */}
      <div
        className={cn(
          "rounded-2xl border p-5 shadow-sm transition-all relative overflow-hidden",
          overallStatus === "ok"
            ? "border-ok-line bg-ok-subtle"
            : overallStatus === "warning"
            ? "border-warn-line bg-warn-subtle"
            : overallStatus === "unknown"
            ? "border-line bg-overlay-subtle"
            : "border-danger-line bg-danger/[0.03]"
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
              ) : overallStatus === "unknown" ? (
                // Not an alert — nothing has reported a problem, the report
                // simply has not been read yet.
                <span className="opacity-30">
                  <SystemHealthAlertIcon size={44} />
                </span>
              ) : (
                <SystemHealthAlertIcon size={44} />
              )}
            </div>
            <div>
              <div className="text-[11px] font-semibold uppercase tracking-wider text-text-subtle">
                Control Plane Status
              </div>
              {/* Loading is not a failure and unavailable is not a verdict:
                  both used to be printed in red, which read as a detected
                  problem rather than an unread source. */}
              <p
                className={cn(
                  "text-xl font-bold tracking-tight",
                  healthQuery.isLoading
                    ? "text-text"
                    : healthQuery.isError
                    ? "text-text-subtle"
                    : overallStatus === "ok"
                    ? "text-ok"
                    : overallStatus === "warning"
                    ? "text-warn"
                    : "text-danger"
                )}
              >
                {healthQuery.isLoading
                  ? "Reading health report…"
                  : healthQuery.isError
                  ? "Status Unavailable"
                  : overallStatus === "ok"
                  ? "All Systems Operational"
                  : overallStatus === "warning"
                  ? "Degraded Performance"
                  : "System Issues Detected"}
              </p>
              {/* Counts only where the list behind them was actually read —
                  an unread node list is not a fleet of zero nodes. */}
              <p className="mt-0.5 text-xs text-text-subtle font-mono">
                {nodesQuery.isError
                  ? "nodes unavailable"
                  : nodesQuery.isPending
                  ? "reading nodes…"
                  : `${summary.totalNodes} nodes`}
                {" · "}
                {serversQuery.isError
                  ? "workloads unavailable"
                  : serversQuery.isPending
                  ? "reading workloads…"
                  : `${summary.totalServers} workloads`}
                {healthQuery.data?.uptime ? ` · Uptime ${secondsLabel(healthQuery.data.uptime)}` : ""}
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2">
            {/* "All healthy" is a claim about every source that feeds this
                count, so it is only made when every one of them answered.
                It previously appeared whenever health had finished loading,
                even if the nodes, reservations or recovery reads had failed
                and their failure counts were therefore zero by default. */}
            {healthQuery.isLoading ? (
              <Pill tone="neutral">Reading…</Pill>
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
                {!hasFailures && !hasWarnings ? (
                  queryErrors.length > 0 ? (
                    <Pill tone="neutral">
                      {queryErrors.length} source{queryErrors.length === 1 ? "" : "s"} unread
                    </Pill>
                  ) : (
                    <Pill tone="green">All healthy</Pill>
                  )
                ) : null}
              </>
            )}
            <button
              type="button"
              onClick={() => router.push("/admin/overview")}
              className="hidden sm:inline-flex items-center gap-1 rounded-lg border border-line bg-overlay px-2.5 py-1 text-xs font-semibold text-text hover:bg-overlay-strong hover:text-text transition cursor-pointer"
            >
              <span>Overview</span>
              <ArrowUpRight size={13} />
            </button>
          </div>
        </div>
      </div>

      {/* Refetching indicator */}
      {isFetching && (
        <div className="flex items-center gap-2 rounded-xl border border-info-line bg-info-subtle p-2.5 text-xs text-info">
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
              ? "border-[color-mix(in_srgb,var(--brand)_50%,transparent)] bg-[var(--surface-raised)] ring-1 ring-[color-mix(in_srgb,var(--brand)_30%,transparent)] shadow-md"
              : "border-line bg-[var(--surface)] hover:border-line-strong hover:bg-overlay-subtle"
          )}
          onClick={() => selectSection("infrastructure")}
          type="button"
        >
          <div className="flex items-center justify-between">
            <span className="flex items-center gap-1.5 text-[11px] font-bold uppercase tracking-wider text-text-subtle group-hover:text-text">
              <ServerRackIcon size={14} className="text-text-subtle" />
              <span>Infrastructure</span>
            </span>
            {/* Degraded nodes are not "ok": the icon only went amber for
                unexpected offline nodes, so a degraded fleet showed green. */}
            {statusIcon(
              !nodesAvailable || nodes.length === 0
                ? undefined
                : summary.unexpectedOfflineNodes > 0
                ? "offline"
                : summary.degradedNodes > 0
                ? "degraded"
                : "ok",
              14
            )}
          </div>
          <p className="mt-2.5 font-mono text-xl font-bold tracking-tight text-text">
            {nodesQuery.isLoading ? "..." : nodesQuery.isError ? "Unavailable" : `${summary.healthyNodes}/${summary.totalNodes} nodes`}
          </p>
          {/* An unread list is not an empty fleet — "Register a node" used to
              show while the request was still in flight. */}
          <p className="mt-1 text-xs text-text-subtle">
            {nodesQuery.isError
              ? "API unreachable"
              : nodesQuery.isPending
              ? "Reading node list…"
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
              ? "border-[color-mix(in_srgb,var(--brand)_50%,transparent)] bg-[var(--surface-raised)] ring-1 ring-[color-mix(in_srgb,var(--brand)_30%,transparent)] shadow-md"
              : "border-line bg-[var(--surface)] hover:border-line-strong hover:bg-overlay-subtle"
          )}
          onClick={() => selectSection("workloads")}
          type="button"
        >
          <div className="flex items-center justify-between">
            <span className="flex items-center gap-1.5 text-[11px] font-bold uppercase tracking-wider text-text-subtle group-hover:text-text">
              <ApplicationsCubeIcon size={14} className="text-text-subtle" />
              <span>Workloads</span>
            </span>
            {statusIcon(serversAvailable ? (summary.failedDeployments > 0 ? "failed" : "ok") : undefined, 14)}
          </div>
          <p className="mt-2.5 font-mono text-xl font-bold tracking-tight text-text">
            {serversQuery.isLoading ? "..." : serversQuery.isError ? "Unavailable" : `${summary.runningServers} running`}
          </p>
          <p className="mt-1 text-xs text-text-subtle">
            {serversQuery.isError
              ? "Server data unavailable"
              : serversQuery.isPending
              ? "Reading workloads…"
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
              ? "border-[color-mix(in_srgb,var(--brand)_50%,transparent)] bg-[var(--surface-raised)] ring-1 ring-[color-mix(in_srgb,var(--brand)_30%,transparent)] shadow-md"
              : "border-line bg-[var(--surface)] hover:border-line-strong hover:bg-overlay-subtle"
          )}
          onClick={() => selectSection("platform")}
          type="button"
        >
          <div className="flex items-center justify-between">
            <span className="flex items-center gap-1.5 text-[11px] font-bold uppercase tracking-wider text-text-subtle group-hover:text-text">
              <ActivityWaveIcon size={14} className="text-ok" />
              <span>API & Queue</span>
            </span>
            {statusIcon(overallStatus, 14)}
          </div>
          <p className="mt-2.5 font-mono text-xl font-bold tracking-tight text-text">
            {checkStatus(healthAvailable, system)}
          </p>
          <p className="mt-1 text-xs text-text-subtle">
            {/* Only count checks once the report is in hand: "0 checks" was
                being printed while it was still being read. */}
            Queue {checkStatus(healthAvailable, queue)} ·{" "}
            {healthAvailable ? `${checks.length} checks` : "checks unread"}
          </p>
        </button>

        {/* Database & Cache */}
        <button
          className={cn(
            "rounded-xl border p-4 text-left transition-all cursor-pointer relative overflow-hidden group",
            selected === "database"
              ? "border-[color-mix(in_srgb,var(--brand)_50%,transparent)] bg-[var(--surface-raised)] ring-1 ring-[color-mix(in_srgb,var(--brand)_30%,transparent)] shadow-md"
              : "border-line bg-[var(--surface)] hover:border-line-strong hover:bg-overlay-subtle"
          )}
          onClick={() => selectSection("database")}
          type="button"
        >
          <div className="flex items-center justify-between">
            <span className="flex items-center gap-1.5 text-[11px] font-bold uppercase tracking-wider text-text-subtle group-hover:text-text">
              <DatabaseCylinderIcon size={14} className="text-text-subtle" />
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
          <p className="mt-2.5 font-mono text-xl font-bold tracking-tight text-text">
            {!healthAvailable ? "Unavailable" : `${database?.status ?? "?"}`}
          </p>
          <p className="mt-1 text-xs text-text-subtle">
            Cache {!healthAvailable ? "Unavailable" : cache?.status ?? "?"}{" "}
            {database?.latencyMs != null ? `· ${database.latencyMs}ms` : ""}
          </p>
        </button>
      </div>

      {/* ========================================================================= */}
      {/* ZONE 4: ACTIONABLE FAILURES (Incident Remediation Cards)                  */}
      {/* ========================================================================= */}
      {summary.failedChecks.length > 0 && (
        <div className="rounded-2xl border border-danger-line bg-danger-subtle p-5 shadow-sm">
          <div className="flex items-center justify-between gap-3 mb-4">
            <h3 className="flex items-center gap-2 text-sm font-bold text-danger">
              <XCircle size={17} className="text-danger stroke-[2.2]" />
              <span>Actionable Failures</span>
              <span className="rounded-full bg-danger-subtle px-2 py-0.5 font-mono text-[10px] font-semibold text-danger">
                {summary.failedChecks.length} issue{summary.failedChecks.length === 1 ? "" : "s"}
              </span>
            </h3>
            <button
              type="button"
              onClick={() => void healthQuery.refetch()}
              disabled={isFetching}
              className="inline-flex items-center gap-1.5 rounded-lg border border-danger-line bg-danger-subtle px-2.5 py-1 text-xs font-semibold text-danger hover:bg-danger-subtle transition cursor-pointer"
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
                  className="rounded-xl border border-danger-line bg-[var(--surface)] p-4 shadow-sm"
                >
                  <div className="flex flex-col sm:flex-row sm:items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <span className="h-2 w-2 rounded-full bg-danger shrink-0" />
                        <p className="text-sm font-bold text-danger truncate">
                          {check.name} — {check.notificationMessage ?? "Failed"}
                        </p>
                      </div>
                      {remediation && (
                        <div className="mt-2.5 rounded-lg border border-danger-line bg-danger-subtle p-3 text-xs text-text leading-relaxed">
                          <span className="font-semibold text-danger block mb-0.5">Recommended Remediation:</span>
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
        <div className="rounded-2xl border border-warn-line bg-warn-subtle p-5 shadow-sm">
          <div className="flex items-center justify-between gap-3 mb-3">
            <h3 className="flex items-center gap-2 text-sm font-bold text-warn">
              <AlertTriangle size={17} className="text-warn stroke-[2.2]" />
              <span>Warnings</span>
              <span className="rounded-full bg-warn-subtle px-2 py-0.5 font-mono text-[10px] font-semibold text-warn">
                {summary.warningChecks.length} warning{summary.warningChecks.length === 1 ? "" : "s"}
              </span>
            </h3>
            <button
              type="button"
              onClick={() => void healthQuery.refetch()}
              className="inline-flex items-center gap-1 text-xs font-semibold text-warn hover:text-warn transition cursor-pointer"
            >
              <RefreshCw size={12} />
              <span>Re-check</span>
            </button>
          </div>

          <div className="space-y-2.5">
            {summary.warningChecks.map((check) => (
              <div
                key={check.name}
                className="flex items-start justify-between gap-3 rounded-xl border border-warn-line bg-[var(--surface)] p-3.5 shadow-sm"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="h-2 w-2 rounded-full bg-warn shrink-0" />
                    <p className="text-sm font-bold text-warn">{check.label ?? check.name}</p>
                  </div>
                  {check.notificationMessage && (
                    <p className="mt-1 text-xs text-text-subtle pl-4">{check.notificationMessage}</p>
                  )}
                </div>
                <button
                  type="button"
                  onClick={() => void healthQuery.refetch()}
                  className="inline-flex items-center gap-1 rounded-lg border border-line bg-overlay px-2.5 py-1 text-xs font-semibold text-text hover:bg-overlay-strong hover:text-text transition cursor-pointer shrink-0"
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
          {/* Healthy out of total: a bare count read as "all good" when it was
              1 of 10, so the denominator is shown and the tile is only green
              when every node is heartbeating. */}
          <MetricTile
            label="Healthy Heartbeats"
            value={nodesAvailable ? `${summary.healthyNodes} / ${summary.totalNodes}` : "Unavailable"}
            status={
              !nodesAvailable || summary.totalNodes === 0
                ? undefined
                : summary.healthyNodes === summary.totalNodes
                  ? "ok"
                  : summary.healthyNodes === 0
                    ? "failed"
                    : "warning"
            }
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
          {/* One tile, not two. "Heartbeat Message" used to render the exact
              same value as "Daemon Check" from the same check, so the message
              the check reported is now shown here instead. */}
          <MetricTile
            hint={healthAvailable ? daemon?.notificationMessage : undefined}
            label="Daemon Check"
            onClick={() => router.push("/admin/nodes")}
            status={healthAvailable ? daemon?.status : undefined}
            value={checkStatus(healthAvailable, daemon)}
          />
        </div>
        {nodesAvailable && summary.totalNodes > 0 && <NodeTable nodes={nodes} />}
        <div className="flex items-center justify-between border-t border-line pt-3 text-xs">
          <span className="text-text-subtle">Heartbeats are transmitted by Beacon host agents over HTTP</span>
          <button
            type="button"
            onClick={() => router.push("/admin/nodes")}
            className="flex items-center gap-1 font-semibold text-text hover:text-text transition"
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
        <div className="flex items-center justify-between border-t border-line pt-3 text-xs">
          <span className="text-text-subtle">PostgreSQL store and Redis cache instances</span>
          <button
            type="button"
            onClick={() => router.push("/admin/databases")}
            className="flex items-center gap-1 font-semibold text-text hover:text-text transition"
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
        <div className="flex items-center justify-between border-t border-line pt-3 text-xs">
          <span className="text-text-subtle">Forge Fiber API server and asynchronous queue workers</span>
          <button
            type="button"
            onClick={() => router.push("/admin/operations")}
            className="flex items-center gap-1 font-semibold text-text hover:text-text transition"
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
            onClick={() => router.push("/admin/activity")}
          />
        </div>
        {summary.failedDeployments > 0 && (
          <div className="rounded-xl border border-danger-line bg-danger-subtle p-3.5 text-xs text-danger">
            <span className="font-semibold block mb-0.5">{summary.failedDeployments} workload(s) are in a failed state.</span>
            Check workload logs for deployment errors and verify that the target node is online and healthy.
          </div>
        )}
        <div className="flex items-center justify-between border-t border-line pt-3 text-xs">
          <span className="text-text-subtle">Container apps, game servers, and services across all nodes</span>
          <button
            type="button"
            onClick={() => router.push("/admin/servers")}
            className="flex items-center gap-1 font-semibold text-text hover:text-text transition"
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
        <div className="flex items-center justify-between border-t border-line pt-3 text-xs">
          <span className="text-text-subtle">Go control-plane runtime heap, goroutines, and OS architecture</span>
          <button
            type="button"
            onClick={() => router.push("/admin/monitoring")}
            className="flex items-center gap-1 font-semibold text-text hover:text-text transition"
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
            <div className="rounded-xl border border-danger-line bg-danger-subtle p-3.5 text-xs text-danger">
              {failedReservations > 0 && `Failed reservation jobs indicate resource contention or unavailable nodes. Review node capacity and retry failed reservations.`}
              {failedRecoveries > 0 && ` Failed recovery plans require manual intervention. Check node connectivity and recovery plan configuration.`}
            </div>
          )}
          <div className="flex items-center justify-between border-t border-line pt-3 text-xs">
            <span className="text-text-subtle">Resource reservations, allocation locks, and failover recovery plans</span>
            <button
              type="button"
              onClick={() => router.push("/admin/reconciliation")}
              className="flex items-center gap-1 font-semibold text-text hover:text-text transition"
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

function NodeTable({ nodes }: { nodes: ApiNode[] }) {
  const router = useRouter();
  if (nodes.length === 0) {
    return <EmptyState icon={NodeHostIcon} message="No nodes are registered; node monitoring will begin after setup." />;
  }
  return (
    <div className="overflow-x-auto rounded-xl border border-line bg-[var(--surface)] shadow-sm">
      <table className="w-full text-left text-xs">
        <thead className="border-b border-line bg-overlay-subtle text-[10px] font-bold uppercase tracking-wider text-text-subtle">
          <tr>
            <th className="px-4 py-3">Node</th>
            <th className="px-4 py-3">Status</th>
            <th className="px-4 py-3">Heartbeat</th>
            <th className="px-4 py-3">Docker</th>
            <th className="px-4 py-3 text-right">Actions</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-line">
          {nodes.map((node) => (
            <tr
              key={node.id}
              onClick={() => router.push(`/admin/nodes/${node.id}`)}
              className={cn(
                "hover:bg-overlay cursor-pointer transition",
                node.maintenanceMode && "opacity-60"
              )}
            >
              <td className="px-4 py-3 font-semibold text-text">
                <div className="flex items-center gap-2">
                  {node.maintenanceMode && <Wrench size={13} className="text-warn shrink-0" />}
                  <span className="font-medium text-text">{node.name}</span>
                </div>
              </td>
              <td className="px-4 py-3">
                <span className="inline-flex items-center gap-1.5 font-mono text-[11px]">
                  {statusIcon(node.actualState, 12)}
                  <span
                    className={
                      node.actualState === "online"
                        ? "text-ok font-semibold"
                        : node.actualState === "degraded"
                        ? "text-warn font-semibold"
                        : node.maintenanceMode
                        ? "text-warn font-semibold"
                        : "text-text-subtle"
                    }
                  >
                    {node.maintenanceMode ? "maintenance" : node.actualState ?? "unknown"}
                  </span>
                </span>
              </td>
              <td className="px-4 py-3 text-text-subtle">
                <span className="inline-flex items-center gap-1.5 font-mono text-[11px]">
                  {statusIcon(node.heartbeatState, 12)}
                  <span>{node.heartbeatState ?? "unknown"}</span>
                </span>
              </td>
              <td className="px-4 py-3 font-mono text-[11px] text-text-subtle">{node.dockerStatus ?? "unknown"}</td>
              <td className="px-4 py-3 text-right">
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation();
                    router.push(`/admin/nodes/${node.id}`);
                  }}
                  className="inline-flex items-center gap-1 text-text-subtle hover:text-text transition font-semibold cursor-pointer"
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
