"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  Cpu,
  Database,
  HardDrive,
  HeartPulse,
  Network,
  RefreshCw,
  Server,
  Workflow,
} from "lucide-react";
import {
  fetchAdminActivity,
  fetchHealthStatus,
  fetchNodes,
  fetchRecoveryPlans,
  fetchReservations,
  fetchServers,
  type ApiHealthCheck,
} from "@/modules/infrastructure/api";
import { Btn, Card, CardHeader, EmptyState, Pill, SectionHeader, cn } from "./admin-ui";

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

const SECTION_LABELS: Record<MonitorSection, string> = {
  infrastructure: "Infrastructure",
  platform: "Platform",
  resources: "Resources",
  workloads: "Workloads",
  database: "Database Monitoring",
  cache: "Cache Monitoring",
  queue: "Queue Monitoring",
  api: "API Runtime Monitoring",
  daemon: "Daemon Monitoring",
  orchestration: "Orchestration Monitoring",
};

function mbLabel(value: number) {
  if (value >= 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} TB`;
  if (value >= 1024) return `${(value / 1024).toFixed(1)} GB`;
  return `${Math.round(value)} MB`;
}

function metricValue(value: unknown) {
  if (typeof value === "object" && value !== null) return JSON.stringify(value);
  return String(value);
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

function checkMessage(healthAvailable: boolean, check: ApiHealthCheck | undefined, fallback: string) {
  if (!healthAvailable) return "Health data unavailable";
  return check?.notificationMessage ?? fallback;
}

function queryErrorMessage(error: unknown) {
  return error instanceof Error && error.message ? error.message : "Unknown API error";
}

function statusTone(status?: string): "green" | "red" | "yellow" | "neutral" | "blue" {
  if (status === "ok" || status === "online") return "green";
  if (status === "failed" || status === "offline") return "red";
  if (status === "warning" || status === "degraded") return "yellow";
  return "neutral";
}

function SummaryCard({
  id,
  active,
  title,
  icon: Icon,
  primary,
  secondary,
  tone = "neutral",
  onClick,
}: {
  id: MonitorSection;
  active: boolean;
  title: string;
  icon: typeof Activity;
  primary: string;
  secondary: string;
  tone?: "green" | "red" | "yellow" | "blue" | "neutral";
  onClick: (id: MonitorSection) => void;
}) {
  const toneClasses = {
    green: "text-emerald-300",
    red: "text-red-300",
    yellow: "text-amber-300",
    blue: "text-blue-300",
    neutral: "text-slate-200",
  };
  return (
    <button
      className={cn(
        "rounded-xl border bg-[#1e2536] p-4 text-left shadow-lg transition hover:border-white/20",
        active ? "border-[#dc2626]/60" : "border-white/[0.08]",
      )}
      onClick={() => onClick(id)}
      type="button"
    >
      <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-widest text-slate-500">
        <Icon size={14} />
        {title}
      </div>
      <p className={cn("mt-3 text-2xl font-bold", toneClasses[tone])}>{primary}</p>
      <p className="mt-1 text-xs text-slate-500">{secondary}</p>
    </button>
  );
}

function MetricGrid({ rows }: { rows: Array<[string, unknown]> }) {
  const visible = rows.filter(([, value]) => value != null && value !== "");
  if (visible.length === 0) {
    return <EmptyState icon={Activity} message="No live metrics are available for this section." />;
  }
  return (
    <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
      {visible.map(([label, value]) => (
        <div key={label} className="rounded-lg border border-white/[0.06] bg-[#151b27] p-3">
          <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">{label}</p>
          <p className="mt-1 break-words text-sm font-semibold text-slate-200">{metricValue(value)}</p>
        </div>
      ))}
    </div>
  );
}

function healthByName(checks: ApiHealthCheck[], name: string) {
  return checks.find((check) => check.name === name);
}

export function AdminHealth({ initialSection = "infrastructure", overview = false }: { initialSection?: MonitorSection; overview?: boolean }) {
  const [selected, setSelected] = useState<MonitorSection>(initialSection);

  const healthQuery = useQuery({ queryKey: ["health"], queryFn: fetchHealthStatus, refetchInterval: 30_000 });
  const nodesQuery = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes, refetchInterval: 30_000 });
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: fetchServers, refetchInterval: 30_000 });
  const reservationsQuery = useQuery({ queryKey: ["reservations"], queryFn: fetchReservations, retry: false, refetchInterval: 30_000 });
  const recoveryQuery = useQuery({ queryKey: ["recovery"], queryFn: fetchRecoveryPlans, retry: false, refetchInterval: 30_000 });
  const activityQuery = useQuery({ queryKey: ["admin-activity", "monitoring"], queryFn: () => fetchAdminActivity({ limit: 1 }), retry: false, refetchInterval: 30_000 });

  const checks = healthQuery.data?.checks ?? [];
  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);
  const servers = useMemo(() => serversQuery.data ?? [], [serversQuery.data]);
  const reservations = reservationsQuery.data ?? [];
  const recoveryPlans = recoveryQuery.data ?? [];

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
  const api = healthByName(checks, "api");
  const memory = healthByName(checks, "memory");
  const system = healthByName(checks, "system");

  const summary = useMemo(() => {
    // Match the daemon check: both use persisted heartbeat evidence, not a live probe.
    const onlineNodes = nodes.filter((node) => node.heartbeatState === "healthy").length;
    const offlineNodes = nodes.length - onlineNodes;
    const runningServers = servers.filter((server) => server.status === "running").length;
    const stoppedServers = servers.filter((server) => server.status === "stopped").length;
    const suspendedServers = servers.filter((server) => server.suspended).length;
    const failedDeployments = servers.filter((server) => ["failed", "install_failed"].includes(server.status)).length;
    const configuredMemory = nodes.reduce((sum, node) => sum + (node.memoryMb ?? 0), 0);
    const configuredDisk = nodes.reduce((sum, node) => sum + (node.diskMb ?? 0), 0);
    const hasConfiguredMemory = nodes.some((node) => node.memoryMb != null);
    const hasConfiguredDisk = nodes.some((node) => node.diskMb != null);
    return {
      onlineNodes,
      offlineNodes,
      runningServers,
      stoppedServers,
      suspendedServers,
      failedDeployments,
      configuredMemory,
      configuredDisk,
      hasConfiguredMemory,
      hasConfiguredDisk,
    };
  }, [nodes, servers]);

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

  const activeReservations = reservations.filter((item) => !["completed", "cancelled", "canceled", "used", "expired", "failed"].includes(item.status)).length;
  const failedReservations = reservations.filter((item) => item.status === "failed").length;
  const activeRecoveries = recoveryPlans.filter((item) => !["completed", "cancelled", "canceled", "restored", "failed"].includes(item.status)).length;
  const failedRecoveries = recoveryPlans.filter((item) => item.status === "failed").length;
  const selectSection = (section: MonitorSection) => {
    setSelected(section);
    if (typeof window !== "undefined") {
      window.history.pushState(null, "", section === "infrastructure" && overview ? "/admin/monitoring" : `/admin/monitoring/${section}`);
    }
  };

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Monitoring Center"
        sub="Live platform monitoring for infrastructure, runtime, workloads, and orchestration."
        action={
          <Btn onClick={refresh} disabled={isFetching}>
            <RefreshCw className={isFetching ? "animate-spin" : ""} size={14} /> Refresh
          </Btn>
        }
      />

      {healthQuery.isError ? (
        <div className="rounded-lg border border-red-700/30 bg-red-900/10 p-3 text-sm text-red-300">
          Monitoring checks could not be loaded from the API: {queryErrorMessage(healthQuery.error)}
        </div>
      ) : null}
      {nodesQuery.isError ? (
        <div className="rounded-lg border border-red-700/30 bg-red-900/10 p-3 text-sm text-red-300">
          Nodes could not be loaded from the API: {queryErrorMessage(nodesQuery.error)}
        </div>
      ) : null}
      {serversQuery.isError ? (
        <div className="rounded-lg border border-red-700/30 bg-red-900/10 p-3 text-sm text-red-300">
          Servers could not be loaded from the API: {queryErrorMessage(serversQuery.error)}
        </div>
      ) : null}
      {reservationsQuery.isError ? (
        <div className="rounded-lg border border-amber-700/30 bg-amber-900/10 p-3 text-sm text-amber-200">
          Reservation data could not be loaded: {queryErrorMessage(reservationsQuery.error)}. Reservation counts are unavailable.
        </div>
      ) : null}
      {recoveryQuery.isError ? (
        <div className="rounded-lg border border-amber-700/30 bg-amber-900/10 p-3 text-sm text-amber-200">
          Recovery plan data could not be loaded: {queryErrorMessage(recoveryQuery.error)}. Recovery counts are unavailable.
        </div>
      ) : null}
      {activityQuery.isError ? (
        <div className="rounded-lg border border-amber-700/30 bg-amber-900/10 p-3 text-sm text-amber-200">
          Platform activity could not be loaded: {queryErrorMessage(activityQuery.error)}. Activity counts are unavailable.
        </div>
      ) : null}

      {healthQuery.data?.checkedAt ? (
        <p className="text-xs text-slate-500">Last checked {new Date(healthQuery.data.checkedAt).toLocaleString()}</p>
      ) : null}

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <SummaryCard
          id="infrastructure"
          active={selected === "infrastructure"}
          title="Infrastructure"
          icon={Network}
          primary={nodesQuery.isLoading ? "..." : nodesQuery.isError ? "Unavailable" : nodes.length === 0 ? "Setup required" : `${summary.onlineNodes}/${nodes.length} nodes`}
          secondary={nodesQuery.isError ? "API unreachable" : nodes.length === 0 ? "Register a node to begin hosting workloads" : `${summary.offlineNodes} without a healthy persisted heartbeat · check ${checkStatus(healthAvailable, daemon)}`}
          tone={nodesQuery.isError || nodesQuery.isLoading || nodes.length === 0 ? "neutral" : summary.offlineNodes ? "yellow" : "green"}
          onClick={selectSection}
        />
        <SummaryCard
          id="platform"
          active={selected === "platform"}
          title="Platform"
          icon={HeartPulse}
          primary={healthQuery.isLoading ? "..." : healthQuery.isError ? "Unavailable" : healthQuery.data?.status === "failed" ? "Failed" : nodesAvailable && nodes.length === 0 ? "Setup required" : healthQuery.data?.status === "warning" ? "Degraded" : "Operational"}
          secondary={healthAvailable ? healthQuery.data?.status === "failed" ? `System ${checkStatus(true, system)} — review failed checks` : nodesAvailable && nodes.length === 0 ? "Register a node before placing workloads" : `System ${checkStatus(true, system)} · ${checks.length} diagnostic checks` : "Health data unavailable"}
          tone={healthQuery.isLoading || healthQuery.isError || (nodesAvailable && nodes.length === 0 && healthQuery.data?.status !== "failed") ? "neutral" : statusTone(healthQuery.data?.status)}
          onClick={selectSection}
        />
        <SummaryCard
          id="resources"
          active={selected === "resources"}
          title="Resources"
          icon={HardDrive}
          primary={!nodesAvailable ? "Unavailable" : summary.hasConfiguredMemory ? mbLabel(summary.configuredMemory) : "Not reported"}
          secondary={!nodesAvailable ? "Node data unavailable" : summary.hasConfiguredDisk ? `${mbLabel(summary.configuredDisk)} configured disk capacity` : "Configured disk capacity not reported"}
          tone={nodesAvailable ? "blue" : "neutral"}
          onClick={selectSection}
        />
        <SummaryCard
          id="workloads"
          active={selected === "workloads"}
          title="Workloads"
          icon={Server}
          primary={serversQuery.isLoading ? "..." : serversQuery.isError ? "Unavailable" : `${summary.runningServers} running`}
          secondary={serversQuery.isError ? "Server data unavailable" : `${summary.stoppedServers} stopped · ${summary.suspendedServers} suspended · ${summary.failedDeployments} failed`}
          tone={!serversAvailable ? "neutral" : summary.failedDeployments ? "red" : "green"}
          onClick={selectSection}
        />
        <SummaryCard id="database" active={selected === "database"} title="Database" icon={Database} primary={!healthAvailable ? "Unavailable" : database?.status ?? "Not reported"} secondary={!healthAvailable ? "Health data unavailable" : database?.notificationMessage ?? "No database check reported"} tone={healthAvailable ? statusTone(database?.status) : "neutral"} onClick={selectSection} />
        <SummaryCard id="cache" active={selected === "cache"} title="Cache" icon={Cpu} primary={!healthAvailable ? "Unavailable" : cache?.status ?? "Not reported"} secondary={!healthAvailable ? "Health data unavailable" : cache?.notificationMessage ?? "No cache check reported"} tone={healthAvailable ? statusTone(cache?.status) : "neutral"} onClick={selectSection} />
        <SummaryCard id="queue" active={selected === "queue"} title="Queue" icon={Workflow} primary={!healthAvailable ? "Unavailable" : queue?.status ?? "Not reported"} secondary={!healthAvailable ? "Health data unavailable" : queue?.notificationMessage ?? "No queue check reported"} tone={healthAvailable ? statusTone(queue?.status) : "neutral"} onClick={selectSection} />
        <SummaryCard id="api" active={selected === "api"} title="System Runtime" icon={Activity} primary={checkStatus(healthAvailable, system)} secondary={checkMessage(healthAvailable, system, "No system runtime check reported")} tone={healthAvailable ? statusTone(system?.status) : "neutral"} onClick={selectSection} />
        <SummaryCard id="daemon" active={selected === "daemon"} title="Daemon Heartbeats" icon={Server} primary={checkStatus(healthAvailable, daemon)} secondary={checkMessage(healthAvailable, daemon, "No persisted-heartbeat check reported")} tone={healthAvailable ? statusTone(daemon?.status) : "neutral"} onClick={selectSection} />
        <SummaryCard id="orchestration" active={selected === "orchestration"} title="Orchestration" icon={Activity} primary={!reservationsAvailable || !recoveriesAvailable ? "Unavailable" : `${activeReservations + activeRecoveries} active`} secondary={!reservationsAvailable || !recoveriesAvailable ? "Reservation or recovery data unavailable" : `${failedReservations + failedRecoveries} failed jobs`} tone={!reservationsAvailable || !recoveriesAvailable ? "neutral" : failedReservations || failedRecoveries ? "red" : "green"} onClick={selectSection} />
      </div>

      <Card>
        <CardHeader title={SECTION_LABELS[selected]} icon={selected === "orchestration" ? Workflow : selected === "database" ? Database : HeartPulse} />
        <div className="space-y-5 p-4">
          {selected === "infrastructure" ? (
            <>
              <MetricGrid rows={[
                ["Nodes With Healthy Persisted Heartbeats", nodesAvailable ? summary.onlineNodes : "Unavailable"],
                ["Nodes Without Healthy Persisted Heartbeats", nodesAvailable ? summary.offlineNodes : "Unavailable"],
                ["Persisted Heartbeat Check", healthAvailable ? daemon?.status ?? "Not reported" : "Unavailable"],
                ["Heartbeat Check Message", healthAvailable ? daemon?.notificationMessage ?? "Not reported" : "Unavailable"],
                ["Nodes Reported by Heartbeat Check", healthAvailable ? detail(daemon, "onlineNodes") ?? "Not reported" : "Unavailable"],
                ["Oldest Persisted Heartbeat Age", healthAvailable ? detail(daemon, "oldestHeartbeatAgeSeconds") != null ? `${detail(daemon, "oldestHeartbeatAgeSeconds")}s` : undefined : "Unavailable"],
              ]} />
              {nodesAvailable ? <NodeTable nodes={nodes} /> : <EmptyState icon={Network} message="Node monitoring data is unavailable." />}
            </>
          ) : null}

          {selected === "platform" ? (
            <MetricGrid rows={[

              ["System Runtime", checkStatus(healthAvailable, system)],
              ["System Runtime Message", checkMessage(healthAvailable, system, "No system runtime check reported")],
              ["System Health", checkStatus(healthAvailable, system)],
              ["Queue Health", checkStatus(healthAvailable, queue)],
              ["Database Health", checkStatus(healthAvailable, database)],
              ["Cache Health", checkStatus(healthAvailable, cache)],
              ["Diagnostic Report", healthAvailable ? healthQuery.data?.status : "Unavailable"],
              ["Diagnostic Uptime", healthAvailable ? healthQuery.data?.uptime ?? "Not reported" : "Unavailable"],
              ["System Message", checkMessage(healthAvailable, system, "No system check reported")],
              ["Database Latency", healthAvailable && database?.latencyMs != null ? `${database.latencyMs} ms` : undefined],
              ["Recent Platform Activity", activityAvailable ? activityQuery.data?.total ?? 0 : "Unavailable"],
            ]} />
          ) : null}

          {selected === "resources" ? (
            <MetricGrid rows={[
              ["Configured Node Memory", !nodesAvailable ? "Unavailable" : summary.hasConfiguredMemory ? mbLabel(summary.configuredMemory) : "Not reported"],
              ["Configured Node Disk", !nodesAvailable ? "Unavailable" : summary.hasConfiguredDisk ? mbLabel(summary.configuredDisk) : "Not reported"],
              ["API Memory Check", checkStatus(healthAvailable, memory)],
              ["API Heap Allocated", healthAvailable ? bytesLabel(detail(memory, "heapAllocBytes")) ?? "Not reported" : "Unavailable"],
              ["Configured Heap Threshold", healthAvailable ? bytesLabel(detail(memory, "thresholdBytes")) ?? "Not configured" : "Unavailable"],
              ["System Runtime",  checkStatus(healthAvailable, system)],
              ["System Runtime Message", checkMessage(healthAvailable, system, "No system runtime check reported")],
              ["Runtime Heap Allocated", healthAvailable ? (detail(system, "heapAllocMb") != null ? `${detail(system, "heapAllocMb")} MB` : "Not reported") : "Unavailable"],
              ["Runtime Heap Reserved", healthAvailable ? (detail(system, "heapSysMb") != null ? `${detail(system, "heapSysMb")} MB` : "Not reported") : "Unavailable"],
              ["Goroutines", healthAvailable ? detail(system, "goroutines") ?? "Not reported" : "Unavailable"],
              ["System Uptime", healthAvailable ? secondsLabel(detail(system, "uptimeSeconds")) ?? "Not reported" : "Unavailable"],
            ]} />
          ) : null}

          {selected === "workloads" ? (
            <MetricGrid rows={[
              ["Running Servers", serversAvailable ? summary.runningServers : "Unavailable"],
              ["Stopped Servers", serversAvailable ? summary.stoppedServers : "Unavailable"],
              ["Suspended Servers", serversAvailable ? summary.suspendedServers : "Unavailable"],
              ["Failed Deployments", serversAvailable ? summary.failedDeployments : "Unavailable"],
              ["Total Servers", serversAvailable ? servers.length : "Unavailable"],
              ["Diagnostic Checks Reported", healthAvailable ? checks.length : "Unavailable"],
              ["Recent Platform Activity", activityAvailable ? activityQuery.data?.total ?? 0 : "Unavailable"],
            ]} />
          ) : null}

          {selected === "database" ? (
            <MetricGrid rows={[
              ["Connection Status", healthAvailable ? database?.status ?? "Not reported" : "Unavailable"],
              ["Active Connections", healthAvailable ? detail(database, "activeConnections") ?? "Not reported" : "Unavailable"],
              ["Query Latency", healthAvailable && database?.latencyMs != null ? `${database.latencyMs} ms` : undefined],
              ["Database Version", healthAvailable ? detail(database, "version") ?? "Not reported" : "Unavailable"],
              ["Schema Migrations", healthAvailable ? detail(database, "migrationCount") ?? "Not reported" : "Unavailable"],
            ]} />
          ) : null}

          {selected === "cache" ? (
            <MetricGrid rows={[
              ["Connection Status", healthAvailable ? cache?.status ?? "Not reported" : "Unavailable"],
              ["Response Latency", healthAvailable && cache?.latencyMs != null ? `${cache.latencyMs} ms` : undefined],
              ["Memory Usage", healthAvailable ? detail(cache, "used_memory_human") ?? "Not reported" : "Unavailable"],
              ["Connected Clients", healthAvailable ? detail(cache, "connected_clients") ?? "Not reported" : "Unavailable"],
            ]} />
          ) : null}

          {selected === "queue" ? (
            <MetricGrid rows={[
              ["Worker Status", healthAvailable ? queue?.status ?? "Not reported" : "Unavailable"],
              ["Active Workers", healthAvailable ? detail(queue, "activeWorkers") ?? "Not reported" : "Unavailable"],
              ["Active Reservations", reservationsAvailable ? activeReservations : "Unavailable"],
              ["Active Recoveries", recoveriesAvailable ? activeRecoveries : "Unavailable"],
              ["Failed Deployments", serversAvailable ? summary.failedDeployments : "Unavailable"],
            ]} />
          ) : null}

          {selected === "api" ? (
            <MetricGrid rows={[
              ["API Runtime Check", checkStatus(healthAvailable, api)],
              ["Runtime Message", checkMessage(healthAvailable, api, "No API runtime check reported")],
              ["API Heap Allocated", healthAvailable ? bytesLabel(detail(api, "heapAllocBytes")) ?? "Not reported" : "Unavailable"],
              ["API Uptime", healthAvailable ? secondsLabel(detail(api, "uptimeSeconds")) ?? "Not reported" : "Unavailable"],
              ["System Runtime Check", checkStatus(healthAvailable, system)],
              ["Heap Allocated", healthAvailable ? (detail(system, "heapAllocMb") != null ? `${detail(system, "heapAllocMb")} MB` : "Not reported") : "Unavailable"],
              ["Heap Reserved", healthAvailable ? (detail(system, "heapSysMb") != null ? `${detail(system, "heapSysMb")} MB` : "Not reported") : "Unavailable"],
              ["System Uptime", healthAvailable ? secondsLabel(detail(system, "uptimeSeconds")) ?? "Not reported" : "Unavailable"],
              ["Goroutine Count", healthAvailable ? detail(system, "goroutines") ?? "Not reported" : "Unavailable"],
              ["Go Version", healthAvailable ? detail(system, "goVersion") ?? "Not reported" : "Unavailable"],
              ["Runtime Platform", healthAvailable ? (detail(system, "goOS") && detail(system, "goArch") ? `${detail(system, "goOS")}/${detail(system, "goArch")}` : "Not reported") : "Unavailable"],
            ]} />
          ) : null}

          {selected === "daemon" ? (
            <MetricGrid rows={[
              ["Persisted Heartbeat Check", checkStatus(healthAvailable, daemon)],
              ["Heartbeat State Source", healthAvailable ? detail(daemon, "stateSource") ?? "Not reported" : "Unavailable"],
              ["Healthy Persisted Heartbeats", healthAvailable ? detail(daemon, "healthyHeartbeatNodes") ?? detail(daemon, "onlineNodes") ?? "Not reported" : "Unavailable"],
              ["Non-Healthy Persisted Heartbeats", healthAvailable ? detail(daemon, "nonHealthyHeartbeatNodes") ?? detail(daemon, "offlineNodes") ?? "Not reported" : "Unavailable"],
              ["Nodes Without a Persisted Heartbeat", healthAvailable ? detail(daemon, "nodesWithoutHeartbeat") ?? "Not reported" : "Unavailable"],
              ["Oldest Persisted Heartbeat", healthAvailable ? (secondsLabel(detail(daemon, "oldestHeartbeatAgeSeconds")) ? `${secondsLabel(detail(daemon, "oldestHeartbeatAgeSeconds"))} old` : "Not reported") : "Unavailable"],
              ["Nodes With Docker Status OK", nodesAvailable ? nodes.filter((node) => node.dockerStatus === "ok" || node.dockerStatus === "running").length : "Unavailable"],
              ["Recorded Heartbeat Errors", nodesAvailable ? nodes.filter((node) => node.heartbeatError).length : "Unavailable"],
              ["Recorded Heartbeat Recovery Attempts", nodesAvailable ? nodes.reduce((sum, node) => sum + (node.heartbeatRecoveryCount ?? 0), 0) : "Unavailable"],
            ]} />
          ) : null}

          {selected === "orchestration" ? (
            <MetricGrid rows={[
              ["Active Reservations", reservationsAvailable ? activeReservations : "Unavailable"],
              ["Completed Reservations", reservationsAvailable ? reservations.filter((item) => item.status === "completed").length : "Unavailable"],
              ["Expired Reservations", reservationsAvailable ? reservations.filter((item) => item.status === "expired").length : "Unavailable"],
              ["Failed Reservations", reservationsAvailable ? failedReservations : "Unavailable"],
              ["Active Recoveries", recoveriesAvailable ? activeRecoveries : "Unavailable"],
              ["Failed Recoveries", recoveriesAvailable ? failedRecoveries : "Unavailable"],
            ]} />
          ) : null}
        </div>
      </Card>
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
            <th className="px-3 py-2">Persisted Heartbeat</th>
            <th className="px-3 py-2">Docker</th>
            <th className="px-3 py-2">Recovery Attempts</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-white/[0.04]">
          {nodes.map((node) => (
            <tr key={node.id}>
              <td className="px-3 py-2 font-semibold text-slate-200">{node.name}</td>
              <td className="px-3 py-2"><Pill tone={statusTone(node.actualState)}>{node.actualState ?? "unknown"}</Pill></td>
              <td className="px-3 py-2 text-slate-400">{node.heartbeatState ?? "unknown"}</td>
              <td className="px-3 py-2 text-slate-400">{node.dockerStatus ?? "unknown"}</td>
              <td className="px-3 py-2 text-slate-400">{node.heartbeatRecoveryCount ?? 0}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
