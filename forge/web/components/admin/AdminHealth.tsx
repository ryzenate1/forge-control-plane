"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  AlertTriangle,
  CheckCircle,
  ChevronDown,
  ChevronRight,
  Cpu,
  Database,
  ExternalLink,
  MinusCircle,
  Network,
  RefreshCw,
  Server,
  Wrench,
  XCircle,
} from "lucide-react";
import {
  fetchAdminActivity,
  fetchHealthStatus,
  fetchNodes,
  fetchRecoveryPlans,
  fetchReservations,
  fetchServers,
  type ApiHealthCheck,
} from "@/lib/api";
import { Btn, EmptyState, Pill, SectionHeader, cn } from "./admin-ui";

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
    if (msg.includes("connect") || msg.includes("reachable")) return "Check database credentials and ensure the database server is running. Verify network connectivity between the API and database host.";
    if (msg.includes("migration")) return "Database migrations are pending. Run the migration command to apply pending schema changes.";
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

function MetricTile({ label, value, status }: { label: string; value: string; status?: string }) {
  return (
    <div className="rounded-xl border border-white/[0.06] bg-[#111722] p-3.5">
      <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">{label}</p>
      <div className="mt-1 flex items-center gap-2">
        {status && statusIcon(status, 14)}
        <p className="text-sm font-semibold text-slate-200">{value}</p>
      </div>
    </div>
  );
}

function HealthSection({ title, icon: Icon, children, defaultOpen = true }: {
  title: string;
  icon: typeof Activity;
  children: React.ReactNode;
  defaultOpen?: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <div className="rounded-xl border border-white/[0.07] bg-white/[0.018]">
      <button
        className="flex w-full items-center justify-between px-4 py-3 text-left transition hover:bg-white/[0.03]"
        onClick={() => setOpen(!open)}
        type="button"
      >
        <div className="flex items-center gap-2 text-sm font-semibold text-slate-200">
          <Icon size={16} className="text-slate-400" />
          {title}
        </div>
        {open ? <ChevronDown size={14} className="text-slate-500" /> : <ChevronRight size={14} className="text-slate-500" />}
      </button>
      {open && <div className="border-t border-white/[0.06] p-4 space-y-4">{children}</div>}
    </div>
  );
}

export function AdminHealth({ initialSection = "infrastructure", overview = false }: { initialSection?: MonitorSection; overview?: boolean }) {
  const [selected, setSelected] = useState<MonitorSection>(initialSection);
  const lastRefreshedRef = useRef<Date | null>(null);

  const poll = { refetchInterval: 30_000, refetchIntervalInBackground: false } as const;
  const healthQuery = useQuery({ queryKey: ["health"], queryFn: fetchHealthStatus, ...poll });
  const nodesQuery = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes, ...poll });
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: fetchServers, ...poll });
  const reservationsQuery = useQuery({ queryKey: ["reservations"], queryFn: fetchReservations, retry: false, ...poll });
  const recoveryQuery = useQuery({ queryKey: ["recovery"], queryFn: fetchRecoveryPlans, retry: false, ...poll });
  const activityQuery = useQuery({ queryKey: ["admin-activity", "monitoring"], queryFn: () => fetchAdminActivity({ limit: 1 }), retry: false, ...poll });

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
    const degradedNodes = nodes.filter((node) =>
      node.heartbeatState && node.heartbeatState !== "healthy" && node.heartbeatState !== "unknown"
    ).length;
    const expectedOfflineNodes = nodes.filter((node) => node.maintenanceMode).length;
    const unexpectedOfflineNodes = nodes.length - healthyNodes - expectedOfflineNodes - degradedNodes;
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
      healthyNodes, degradedNodes, expectedOfflineNodes, unexpectedOfflineNodes, totalNodes: nodes.length,
      runningServers, stoppedServers, suspendedServers, failedDeployments, totalServers: servers.length,
      configuredMemory, configuredDisk, hasConfiguredMemory, hasConfiguredDisk,
      failedChecks, warningChecks,
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

  const activeReservations = Array.isArray(reservations) ? reservations.filter((item) => !["completed", "cancelled", "canceled", "used", "expired", "failed"].includes(item.status)).length : 0;
  const failedReservations = Array.isArray(reservations) ? reservations.filter((item) => item.status === "failed").length : 0;
  const activeRecoveries = Array.isArray(recoveryPlans) ? recoveryPlans.filter((item) => !["completed", "cancelled", "canceled", "restored", "failed"].includes(item.status)).length : 0;
  const failedRecoveries = Array.isArray(recoveryPlans) ? recoveryPlans.filter((item) => item.status === "failed").length : 0;

  const overallStatus = healthAvailable ? healthQuery.data.status : "unknown";
  const hasFailures = summary.failedChecks.length > 0 || summary.failedDeployments > 0 || failedReservations > 0 || failedRecoveries > 0;
  const hasWarnings = summary.warningChecks.length > 0 || summary.degradedNodes > 0;

  const selectSection = (section: MonitorSection) => {
    setSelected(section);
    if (typeof window !== "undefined") {
      window.history.pushState(null, "", section === "infrastructure" && overview ? "/admin/monitoring" : `/admin/monitoring/${section}`);
    }
  };

  const queryErrors = [
    { title: "Health", message: "Monitoring checks could not be loaded", refetch: () => { healthQuery.refetch(); }, isError: healthQuery.isError, tone: "red" as const },
    { title: "Nodes", message: "Nodes could not be loaded", refetch: () => { nodesQuery.refetch(); }, isError: nodesQuery.isError, tone: "red" as const },
    { title: "Servers", message: "Servers could not be loaded", refetch: () => { serversQuery.refetch(); }, isError: serversQuery.isError, tone: "red" as const },
    { title: "Reservations", message: "Reservation data could not be loaded; counts unavailable", refetch: () => { reservationsQuery.refetch(); }, isError: reservationsQuery.isError, tone: "amber" as const },
    { title: "Recovery", message: "Recovery plan data could not be loaded; counts unavailable", refetch: () => { recoveryQuery.refetch(); }, isError: recoveryQuery.isError, tone: "amber" as const },
    { title: "Activity", message: "Platform activity could not be loaded; counts unavailable", refetch: () => { activityQuery.refetch(); }, isError: activityQuery.isError, tone: "amber" as const },
  ].filter((e) => e.isError);

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Monitoring Center"
        sub="Live platform monitoring for infrastructure, runtime, workloads, and orchestration."
        action={
          <div className="flex items-center gap-3">
            {lastRefreshed && (
              <span className="text-xs text-slate-500">Last checked {lastRefreshed.toLocaleString()}</span>
            )}
            <Btn onClick={refresh} disabled={isFetching}>
              <RefreshCw className={isFetching ? "animate-spin" : ""} size={14} /> Refresh
            </Btn>
          </div>
        }
      />

      {queryErrors.length > 0 && (
        <div className="space-y-2">
          {queryErrors.map((e) => (
            <div key={e.title} className={cn(
              "flex items-start justify-between gap-4 rounded-lg border p-3 text-sm",
              e.tone === "red" ? "border-red-500/20 bg-red-950/10 text-red-200" : "border-amber-500/20 bg-amber-950/10 text-amber-200"
            )}>
              <span>{e.message}: {(() => {
                const err =
                  e.title === "Health" ? healthQuery.error :
                  e.title === "Nodes" ? nodesQuery.error :
                  e.title === "Servers" ? serversQuery.error :
                  e.title === "Reservations" ? reservationsQuery.error :
                  e.title === "Recovery" ? recoveryQuery.error :
                  activityQuery.error;
                return queryErrorMessage(err);
              })()}</span>
              <Btn size="sm" tone="ghost" onClick={e.refetch}>Retry</Btn>
            </div>
          ))}
        </div>
      )}

      {/* Overall Status */}
      <div className="rounded-2xl border border-white/[0.08] bg-[#111722] p-5">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            {statusIcon(overallStatus, 24)}
            <div>
              <p className="text-lg font-bold text-slate-100">
                {healthQuery.isLoading ? "Loading..." : healthQuery.isError ? "Unavailable" : overallStatus === "ok" ? "All Systems Operational" : overallStatus === "warning" ? "Degraded Performance" : "System Issues Detected"}
              </p>
              <p className="text-xs text-slate-500">
                {summary.totalNodes} nodes · {summary.totalServers} workloads
                {healthQuery.data?.uptime ? ` · Uptime ${secondsLabel(healthQuery.data.uptime)}` : ""}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            {hasFailures && <Pill tone="red">{summary.failedChecks.length + summary.failedDeployments + failedReservations + failedRecoveries} failures</Pill>}
            {hasWarnings && !hasFailures && <Pill tone="yellow">{summary.warningChecks.length + summary.degradedNodes} warnings</Pill>}
            {!hasFailures && !hasWarnings && <Pill tone="green">All healthy</Pill>}
          </div>
        </div>
      </div>

      {/* Error banners - keep visible data during refetch */}
      {isFetching && (
        <div className="flex items-center gap-2 rounded-lg border border-blue-500/20 bg-blue-950/10 p-2 text-xs text-blue-300">
          <RefreshCw size={12} className="animate-spin" />
          Refreshing data...
        </div>
      )}

      {/* Summary Grouped Grid */}
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        <button
          className={cn(
            "rounded-xl border bg-[#111722] p-4 text-left transition hover:border-white/20",
            selected === "infrastructure" ? "border-slate-500/50 ring-1 ring-slate-500/20" : "border-white/[0.07]"
          )}
          onClick={() => selectSection("infrastructure")}
          type="button"
        >
          <div className="flex items-center justify-between">
            <span className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Infrastructure</span>
            {statusIcon(nodes.length === 0 ? undefined : summary.unexpectedOfflineNodes > 0 ? "offline" : "ok", 14)}
          </div>
          <p className="mt-2 text-lg font-bold text-slate-100">
            {nodesQuery.isLoading ? "..." : nodesQuery.isError ? "Unavailable" : `${summary.healthyNodes}/${summary.totalNodes} nodes`}
          </p>
          <p className="mt-0.5 text-xs text-slate-500">
            {nodesQuery.isError ? "API unreachable" :
             summary.totalNodes === 0 ? "Register a node to begin hosting workloads" :
             summary.expectedOfflineNodes > 0 ? `${summary.expectedOfflineNodes} in maintenance` :
             summary.unexpectedOfflineNodes > 0 ? `${summary.unexpectedOfflineNodes} offline unexpectedly` :
             "All nodes healthy"}
          </p>
        </button>

        <button
          className={cn(
            "rounded-xl border bg-[#111722] p-4 text-left transition hover:border-white/20",
            selected === "workloads" ? "border-slate-500/50 ring-1 ring-slate-500/20" : "border-white/[0.07]"
          )}
          onClick={() => selectSection("workloads")}
          type="button"
        >
          <div className="flex items-center justify-between">
            <span className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Workloads</span>
            {statusIcon(serversAvailable ? (summary.failedDeployments > 0 ? "failed" : "ok") : undefined, 14)}
          </div>
          <p className="mt-2 text-lg font-bold text-slate-100">
            {serversQuery.isLoading ? "..." : serversQuery.isError ? "Unavailable" : `${summary.runningServers} running`}
          </p>
          <p className="mt-0.5 text-xs text-slate-500">
            {serversQuery.isError ? "Server data unavailable" :
             `${summary.stoppedServers} stopped · ${summary.suspendedServers} suspended${summary.failedDeployments > 0 ? ` · ${summary.failedDeployments} failed` : ""}`}
          </p>
        </button>

        <button
          className={cn(
            "rounded-xl border bg-[#111722] p-4 text-left transition hover:border-white/20",
            selected === "platform" ? "border-slate-500/50 ring-1 ring-slate-500/20" : "border-white/[0.07]"
          )}
          onClick={() => selectSection("platform")}
          type="button"
        >
          <div className="flex items-center justify-between">
            <span className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">API & Queue</span>
            {statusIcon(overallStatus, 14)}
          </div>
          <p className="mt-2 text-lg font-bold text-slate-100">{checkStatus(healthAvailable, system)}</p>
          <p className="mt-0.5 text-xs text-slate-500">
            Queue {checkStatus(healthAvailable, queue)} · {checks.length} checks
          </p>
        </button>

        <button
          className={cn(
            "rounded-xl border bg-[#111722] p-4 text-left transition hover:border-white/20",
            selected === "database" ? "border-slate-500/50 ring-1 ring-slate-500/20" : "border-white/[0.07]"
          )}
          onClick={() => selectSection("database")}
          type="button"
        >
          <div className="flex items-center justify-between">
            <span className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Database & Cache</span>
            {statusIcon(database?.status === "ok" && cache?.status === "ok" ? "ok" : database?.status === "failed" ? "failed" : cache?.status === "failed" ? "failed" : undefined, 14)}
          </div>
          <p className="mt-2 text-lg font-bold text-slate-100">
            {!healthAvailable ? "Unavailable" : `${database?.status ?? "?"}`}
          </p>
          <p className="mt-0.5 text-xs text-slate-500">
            Cache {!healthAvailable ? "Unavailable" : cache?.status ?? "?"} {database?.latencyMs != null ? `· ${database.latencyMs}ms` : ""}
          </p>
        </button>
      </div>

      {/* Actionable Failures */}
      {summary.failedChecks.length > 0 && (
        <div className="rounded-xl border border-red-500/20 bg-red-950/5 p-4">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-red-300">
            <XCircle size={16} /> Actionable Failures
          </h3>
          <div className="mt-3 space-y-3">
            {summary.failedChecks.map((check) => {
              const remediation = remediationFor(check);
              return (
                <div key={check.name} className="rounded-lg border border-red-500/15 bg-[#111722] p-3">
                  <div className="flex items-start justify-between gap-2">
                    <div>
                      <p className="text-sm font-medium text-red-200">{check.name} — {check.notificationMessage ?? "Failed"}</p>
                      {remediation && <p className="mt-1 text-xs text-slate-400">{remediation}</p>}
                    </div>
                    <div className="flex items-center gap-1 shrink-0">
                      <Btn size="sm" tone="ghost" onClick={() => void healthQuery.refetch()}><RefreshCw size={12} /> Retry</Btn>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Warning Checks */}
      {summary.warningChecks.length > 0 && (
        <div className="rounded-xl border border-amber-500/20 bg-amber-950/5 p-4">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-amber-300">
            <AlertTriangle size={16} /> Warnings
          </h3>
          <div className="mt-3 space-y-2">
            {summary.warningChecks.map((check) => (
              <div key={check.name} className="flex items-start justify-between gap-2 rounded-lg border border-amber-500/15 bg-[#111722] p-3">
                <div>
                  <p className="text-sm font-medium text-amber-200">{check.label ?? check.name}</p>
                  {check.notificationMessage && <p className="mt-0.5 text-xs text-slate-400">{check.notificationMessage}</p>}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Detailed Sections */}
      <HealthSection title="Infrastructure Details" icon={Network} defaultOpen={selected === "infrastructure"}>
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          <MetricTile label="Healthy Heartbeats" value={nodesAvailable ? String(summary.healthyNodes) : "Unavailable"} status={summary.healthyNodes > 0 ? "ok" : nodesAvailable && summary.totalNodes > 0 ? "failed" : undefined} />
          <MetricTile label="In Maintenance" value={nodesAvailable ? String(summary.expectedOfflineNodes) : "Unavailable"} />
          <MetricTile label="Unexpectedly Offline" value={nodesAvailable ? String(summary.unexpectedOfflineNodes) : "Unavailable"} status={summary.unexpectedOfflineNodes > 0 ? "failed" : undefined} />
          <MetricTile label="Degraded" value={nodesAvailable ? String(summary.degradedNodes) : "Unavailable"} status={summary.degradedNodes > 0 ? "warning" : undefined} />
          <MetricTile label="Daemon Check" value={checkStatus(healthAvailable, daemon)} status={healthAvailable ? daemon?.status : undefined} />
          <MetricTile label="Heartbeat Message" value={checkStatus(healthAvailable, daemon)} status={healthAvailable ? daemon?.status : undefined} />
        </div>
        {nodesAvailable && summary.totalNodes > 0 && <NodeTable nodes={nodes} />}
      </HealthSection>

      <HealthSection title="Database & Cache" icon={Database} defaultOpen={selected === "database"}>
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          <MetricTile label="Database Status" value={checkStatus(healthAvailable, database)} status={healthAvailable ? database?.status : undefined} />
          <MetricTile label="Cache Status" value={checkStatus(healthAvailable, cache)} status={healthAvailable ? cache?.status : undefined} />
          <MetricTile label="Database Latency" value={healthAvailable && database?.latencyMs != null ? `${database.latencyMs} ms` : "Not reported"} />
          <MetricTile label="Active Connections" value={healthAvailable ? String(detail(database, "activeConnections") ?? "Not reported") : "Unavailable"} />
          <MetricTile label="Database Version" value={healthAvailable ? String(detail(database, "version") ?? "Not reported") : "Unavailable"} />
          <MetricTile label="Cache Memory" value={healthAvailable ? String(detail(cache, "used_memory_human") ?? "Not reported") : "Unavailable"} />
        </div>
      </HealthSection>

      <HealthSection title="Control-Plane Services" icon={Activity} defaultOpen={selected === "platform"}>
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          <MetricTile label="API Runtime" value={checkStatus(healthAvailable, system)} status={healthAvailable ? system?.status : undefined} />
          <MetricTile label="Queue Health" value={checkStatus(healthAvailable, queue)} status={healthAvailable ? queue?.status : undefined} />
          <MetricTile label="Overall Health" value={healthAvailable ? healthQuery.data.status : "Unavailable"} status={overallStatus} />
          <MetricTile label="API Uptime" value={healthAvailable ? secondsLabel(healthQuery.data?.uptime) ?? "Not reported" : "Unavailable"} />
          <MetricTile label="Memory Check" value={checkStatus(healthAvailable, memory)} status={healthAvailable ? memory?.status : undefined} />
          <MetricTile label="Active Workers" value={healthAvailable ? String(detail(queue, "activeWorkers") ?? "Not reported") : "Unavailable"} />
        </div>
      </HealthSection>

      <HealthSection title="Workloads" icon={Server} defaultOpen={selected === "workloads"}>
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          <MetricTile label="Running" value={serversAvailable ? String(summary.runningServers) : "Unavailable"} status={summary.runningServers > 0 ? "ok" : undefined} />
          <MetricTile label="Stopped" value={serversAvailable ? String(summary.stoppedServers) : "Unavailable"} />
          <MetricTile label="Suspended" value={serversAvailable ? String(summary.suspendedServers) : "Unavailable"} />
          <MetricTile label="Failed" value={serversAvailable ? String(summary.failedDeployments) : "Unavailable"} status={summary.failedDeployments > 0 ? "failed" : undefined} />
          <MetricTile label="Total Servers" value={serversAvailable ? String(summary.totalServers) : "Unavailable"} />
          <MetricTile label="Platform Activity" value={activityAvailable ? String(activityQuery.data?.total ?? 0) : "Unavailable"} />
        </div>
        {summary.failedDeployments > 0 && (
          <div className="rounded-lg border border-red-500/15 bg-red-950/5 p-3 text-xs text-slate-400">
            {summary.failedDeployments} workload(s) are in a failed state. Check the workload logs for deployment errors and verify that the target node is online and healthy.
          </div>
        )}
      </HealthSection>

      <HealthSection title="Runtime & Resources" icon={Cpu} defaultOpen={selected === "resources"}>
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          <MetricTile label="Configured Memory" value={!nodesAvailable ? "Unavailable" : summary.hasConfiguredMemory ? mbLabel(summary.configuredMemory) : "Not reported"} />
          <MetricTile label="Configured Disk" value={!nodesAvailable ? "Unavailable" : summary.hasConfiguredDisk ? mbLabel(summary.configuredDisk) : "Not reported"} />
          <MetricTile label="Heap Allocated" value={healthAvailable ? bytesLabel(detail(system, "heapAllocMb") != null ? `${detail(system, "heapAllocMb")} MB` : detail(system, "heapAllocBytes")) ?? "Not reported" : "Unavailable"} />
          <MetricTile label="Goroutines" value={healthAvailable ? String(detail(system, "goroutines") ?? "Not reported") : "Unavailable"} />
          <MetricTile label="Go Version" value={healthAvailable ? String(detail(system, "goVersion") ?? "Not reported") : "Unavailable"} />
          <MetricTile label="Platform" value={healthAvailable ? (detail(system, "goOS") && detail(system, "goArch") ? `${detail(system, "goOS")}/${detail(system, "goArch")}` : "Not reported") : "Unavailable"} />
        </div>
      </HealthSection>

      {selected === "orchestration" && (
        <HealthSection title="Orchestration" icon={Wrench}>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            <MetricTile label="Active Reservations" value={reservationsAvailable ? String(activeReservations) : "Unavailable"} />
            <MetricTile label="Failed Reservations" value={reservationsAvailable ? String(failedReservations) : "Unavailable"} status={failedReservations > 0 ? "failed" : undefined} />
            <MetricTile label="Active Recoveries" value={recoveriesAvailable ? String(activeRecoveries) : "Unavailable"} />
            <MetricTile label="Failed Recoveries" value={recoveriesAvailable ? String(failedRecoveries) : "Unavailable"} status={failedRecoveries > 0 ? "failed" : undefined} />
          </div>
          {(failedReservations > 0 || failedRecoveries > 0) && (
            <div className="rounded-lg border border-red-500/15 bg-red-950/5 p-3 text-xs text-slate-400">
              {failedReservations > 0 && `Failed reservation jobs indicate resource contention or unavailable nodes. Review node capacity and retry failed reservations.`}
              {failedRecoveries > 0 && `Failed recovery plans require manual intervention. Check node connectivity and recovery plan configuration.`}
            </div>
          )}
        </HealthSection>
      )}
    </div>
  );
}

function NodeTable({ nodes }: { nodes: Awaited<ReturnType<typeof fetchNodes>> }) {
  if (nodes.length === 0) {
    return <EmptyState icon={Network} message="No nodes are registered; node monitoring will begin after setup." />;
  }
  return (
    <div className="overflow-x-auto rounded-lg border border-white/[0.06]">
      <table className="w-full text-left text-xs">
        <thead className="border-b border-white/[0.06] bg-[#161b28] text-slate-500">
          <tr>
            <th className="px-3 py-2">Node</th>
            <th className="px-3 py-2">Status</th>
            <th className="px-3 py-2">Heartbeat</th>
            <th className="px-3 py-2">Docker</th>
            <th className="px-3 py-2">Actions</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-white/[0.04]">
          {nodes.map((node) => (
            <tr key={node.id} className={node.maintenanceMode ? "opacity-60" : ""}>
              <td className="px-3 py-2 font-semibold text-slate-200">
                <div className="flex items-center gap-1.5">
                  {node.maintenanceMode && <Wrench size={12} className="text-amber-400" />}
                  {node.name}
                </div>
              </td>
              <td className="px-3 py-2">
                <span className="inline-flex items-center gap-1">
                  {statusIcon(node.actualState, 12)}
                  <span className={
                    node.actualState === "online" ? "text-emerald-300" :
                    node.actualState === "degraded" ? "text-amber-300" :
                    node.maintenanceMode ? "text-amber-400" :
                    "text-slate-400"
                  }>{node.maintenanceMode ? "maintenance" : node.actualState ?? "unknown"}</span>
                </span>
              </td>
              <td className="px-3 py-2 text-slate-400">
                <span className="inline-flex items-center gap-1">
                  {statusIcon(node.heartbeatState, 12)}
                  {node.heartbeatState ?? "unknown"}
                </span>
              </td>
              <td className="px-3 py-2 text-slate-400">{node.dockerStatus ?? "unknown"}</td>
              <td className="px-3 py-2">
                <a
                  href={`/admin/nodes/${node.id}`}
                  className="inline-flex items-center gap-1 text-slate-500 hover:text-slate-200 transition"
                >
                  <ExternalLink size={12} /> View
                </a>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}