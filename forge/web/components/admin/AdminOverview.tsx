"use client";

import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, Layers, Network, Server, Shield, Activity, HardDrive, Cpu } from "lucide-react";
import { fetchAdminAudit, fetchAllServers, fetchHealthStatus, fetchNodes, fetchUsers, type ApiAdminAuditEvent, type ApiHealthCheck, type ApiNode, type ApiServer } from "@/lib/api";
import { Card, CardHeader, EmptyState, SectionHeader, StatsRow, Pill } from "./admin-ui";

function SimplePieChart({ data, total }: { data: { label: string; value: number; color: string }[]; total: number }) {
  let cumulativePercent = 0;
  const safeTotal = total > 0 ? total : 1;
  return (
    <div className="flex items-center gap-4">
      <div className="relative h-20 w-20 shrink-0">
        <svg viewBox="0 0 36 36" className="h-full w-full transform -rotate-90">
          {data.map((item) => {
            const percent = (item.value / safeTotal) * 100;
            const strokeDasharray = `${percent} 100`;
            const strokeDashoffset = -cumulativePercent;
            cumulativePercent += percent;
            return (
              <circle
                key={item.label}
                cx="18"
                cy="18"
                r="15.9155"
                fill="transparent"
                stroke={item.color}
                strokeWidth="4"
                strokeDasharray={strokeDasharray}
                strokeDashoffset={strokeDashoffset}
              />
            );
          })}
        </svg>
      </div>
      <div className="space-y-1">
        {data.map((item) => (
          <div key={item.label} className="flex items-center gap-2 text-xs">
            <div className="h-2 w-2 rounded-full" style={{ backgroundColor: item.color }} />
            <span className="text-slate-400">{item.label}: {item.value} ({((item.value / safeTotal) * 100).toFixed(1)}%)</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function SimpleBarChart({ data, title }: { data: { id: string; label: string; value: number; max: number }[]; title: string }) {
  const maxValue = Math.max(0, ...data.map((item) => item.max));
  return (
    <div>
      <h4 className="text-sm font-semibold text-slate-200 mb-3">{title}</h4>
      <div className="space-y-2">
        {data.map((item) => (
          <div key={item.id} className="flex items-center gap-2">
            <span className="w-20 text-xs text-slate-400">{item.label}</span>
            <div className="flex-1 h-4 rounded-full bg-white/[0.05] overflow-hidden">
              <div 
                className="h-full rounded-full bg-blue-500 transition-all"
                style={{ width: `${maxValue > 0 ? Math.min(100, (item.value / maxValue) * 100) : 0}%` }}
              />
            </div>
            <span className="w-16 text-xs text-slate-400 text-right">{item.value}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function QueryError({ message }: { message: string }) {
  return (
    <div className="rounded-lg border border-red-700/30 bg-red-900/10 p-4 text-center text-sm text-red-300">{message}</div>
  );
}

function QueryLoading({ message }: { message: string }) {
  return (
    <div className="rounded-lg border border-white/[0.06] bg-white/[0.02] p-4 text-center text-sm text-slate-400">{message}</div>
  );
}

function reportedTotal(records: Array<ApiNode | ApiServer>, field: "memoryMb" | "diskMb") {
  const values = records
    .map((record) => record[field])
    .filter((value): value is number => typeof value === "number" && Number.isFinite(value));
  return { value: values.reduce((sum, value) => sum + value, 0), reported: values.length, total: records.length };
}

function hasHealthyPersistedHeartbeat(node: ApiNode) {
  return node.heartbeatState === "healthy";
}

export function AdminOverview() {
  const nodesQuery = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes, refetchInterval: 30_000, retry: 3 });
  const serversQuery = useQuery({ queryKey: ["servers", "all"], queryFn: fetchAllServers, refetchInterval: 30_000, retry: 3 });
  const usersQuery = useQuery({ queryKey: ["users"], queryFn: fetchUsers, refetchInterval: 60_000, retry: 2 });
  const healthQuery = useQuery<{ checks?: ApiHealthCheck[] }>({ queryKey: ["health"], queryFn: fetchHealthStatus, retry: 2, refetchInterval: 30_000 });
  const activityQuery = useQuery<ApiAdminAuditEvent[]>({ queryKey: ["admin-audit"], queryFn: fetchAdminAudit, retry: 2, refetchInterval: 15_000 });

  const nodes = nodesQuery.data ?? [];
  const servers = serversQuery.data ?? [];
  const users = usersQuery.data ?? [];

  const onlineNodes = nodes.filter(hasHealthyPersistedHeartbeat).length;
  const heartbeatReportedNodes = nodes.filter((node) => Boolean(node.heartbeatState)).length;
  const runningServers = servers.filter((server) => server.status === "running").length;
  const failures = [
    ...(nodesQuery.isError ? [] : nodes.filter((node) => !hasHealthyPersistedHeartbeat(node) || node.heartbeatError).map((node) => ({ id: `node-${node.id}`, label: node.name, detail: node.heartbeatError ?? `Persisted heartbeat is ${node.heartbeatState ?? "unreported"}` }))),
    ...(serversQuery.isError ? [] : servers.filter((server) => server.status === "failed" || server.status === "install_failed" || server.transferError).map((server) => ({ id: `server-${server.id}`, label: server.name, detail: server.transferError ?? `Server is ${server.status}` }))),
    ...(healthQuery.isError ? [] : (healthQuery.data?.checks ?? []).filter((check) => check.status !== "ok").map((check) => ({ id: `health-${check.name}`, label: check.label ?? check.name, detail: check.notificationMessage }))),
  ];
  const nodeMemoryCapacity = reportedTotal(nodes, "memoryMb");
  const nodeDiskCapacity = reportedTotal(nodes, "diskMb");
  const serverMemoryConfiguration = reportedTotal(servers, "memoryMb");
  const serverDiskConfiguration = reportedTotal(servers, "diskMb");

  const serverStatusData = [
    { label: "Running", value: servers.filter((server) => server.status === "running").length, color: "#22c55e" },
    { label: "Stopped", value: servers.filter((server) => server.status === "stopped").length, color: "#64748b" },
    { label: "Other", value: servers.filter((server) => server.status !== "running" && server.status !== "stopped").length, color: "#eab308" },
  ];

  const nodeResourceData = nodes
    .filter((node): node is ApiNode & { memoryMb: number } => typeof node.memoryMb === "number" && Number.isFinite(node.memoryMb))
    .map((node) => ({ id: node.id, label: node.name, value: node.memoryMb, max: node.memoryMb }));

  return (
  <div>
    <SectionHeader title="Overview" sub="Inventory, configured capacity, and persisted node health." />

    <div className="mb-4 flex flex-wrap gap-2 text-xs">
      <Pill tone={nodesQuery.isError ? "red" : nodesQuery.isLoading ? "yellow" : "green"}>Nodes: {nodesQuery.isError ? "unavailable" : nodesQuery.isLoading ? "loading" : "available"}</Pill>
      <Pill tone={serversQuery.isError ? "red" : serversQuery.isLoading ? "yellow" : "green"}>All servers: {serversQuery.isError ? "unavailable" : serversQuery.isLoading ? "loading" : "available"}</Pill>
      <Pill tone={usersQuery.isError ? "red" : usersQuery.isLoading ? "yellow" : "green"}>Users: {usersQuery.isError ? "unavailable" : usersQuery.isLoading ? "loading" : "available"}</Pill>
      <Pill tone={healthQuery.isError ? "red" : healthQuery.isLoading ? "yellow" : "green"}>Health checks: {healthQuery.isError ? "unavailable" : healthQuery.isLoading ? "loading" : "available"}</Pill>
      <Pill tone={activityQuery.isError ? "red" : activityQuery.isLoading ? "yellow" : "green"}>Activity: {activityQuery.isError ? "unavailable" : activityQuery.isLoading ? "loading" : "available"}</Pill>
    </div>

    {(nodesQuery.isError || serversQuery.isError || usersQuery.isError || healthQuery.isError || activityQuery.isError) && (
      <div className="mb-4 space-y-2">
        {nodesQuery.isError && <QueryError message={`Nodes are unavailable; node capacity and persisted-heartbeat coverage cannot be shown. (${nodesQuery.error?.message ?? "unknown error"})`} />}
        {serversQuery.isError && <QueryError message={`All-server inventory is unavailable; server totals, status, and configured capacity cannot be shown. (${serversQuery.error?.message ?? "unknown error"})`} />}
        {usersQuery.isError && <QueryError message={`Users are unavailable; the user total cannot be shown. (${usersQuery.error?.message ?? "unknown error"})`} />}
        {healthQuery.isError && <QueryError message={`Health checks are unavailable; the actionable-failures list is incomplete. (${healthQuery.error?.message ?? "unknown error"})`} />}
        {activityQuery.isError && <QueryError message={`Administrative activity is unavailable. (${activityQuery.error?.message ?? "unknown error"})`} />}
      </div>
    )}

    <StatsRow items={[
      { label: "Nodes", value: nodesQuery.isError ? "Unavailable" : nodesQuery.isLoading ? "Loading…" : nodes.length, icon: Network, tone: nodesQuery.isError || nodesQuery.isLoading || nodes.length === 0 ? "neutral" : onlineNodes === nodes.length ? "green" : "yellow" },
      { label: "All servers", value: serversQuery.isError ? "Unavailable" : serversQuery.isLoading ? "Loading…" : servers.length, icon: Layers, tone: serversQuery.isError || serversQuery.isLoading ? "neutral" : "blue" },
      { label: "Running", value: serversQuery.isError ? "Unavailable" : serversQuery.isLoading ? "Loading…" : runningServers, icon: Server, tone: serversQuery.isError || serversQuery.isLoading ? "neutral" : "green" },
      { label: "Users", value: usersQuery.isError ? "Unavailable" : usersQuery.isLoading ? "Loading…" : users.length, icon: Shield, tone: "neutral" },
    ]} />

    <div className="mb-6 grid gap-3 sm:grid-cols-2">
      <div className="rounded-xl border border-white/[0.06] bg-[#1e2536] p-4">
        <p className="text-xs font-medium uppercase tracking-wider text-slate-500">Configured server memory</p>
        <p className="mt-1 text-lg font-bold text-slate-100">{serversQuery.isError ? "Unavailable" : serversQuery.isLoading ? "Loading…" : serverMemoryConfiguration.reported ? `${serverMemoryConfiguration.value.toLocaleString()} MiB` : "Not reported"}</p>
        <p className="mt-1 text-xs text-slate-500">{serversQuery.isError ? "All-server inventory is unavailable." : serversQuery.isLoading ? "Waiting for the all-server inventory." : `Reported by ${serverMemoryConfiguration.reported} of ${serverMemoryConfiguration.total} servers.`}</p>
        <p className="mt-2 border-t border-white/[0.06] pt-2 text-xs text-slate-400">Node configured capacity: {nodesQuery.isError ? "unavailable" : nodesQuery.isLoading ? "loading…" : nodeMemoryCapacity.reported ? `${nodeMemoryCapacity.value.toLocaleString()} MiB across ${nodeMemoryCapacity.reported}/${nodeMemoryCapacity.total} nodes` : "not reported"}</p>
      </div>
      <div className="rounded-xl border border-white/[0.06] bg-[#1e2536] p-4">
        <p className="text-xs font-medium uppercase tracking-wider text-slate-500">Configured server disk</p>
        <p className="mt-1 text-lg font-bold text-slate-100">{serversQuery.isError ? "Unavailable" : serversQuery.isLoading ? "Loading…" : serverDiskConfiguration.reported ? `${serverDiskConfiguration.value.toLocaleString()} MiB` : "Not reported"}</p>
        <p className="mt-1 text-xs text-slate-500">{serversQuery.isError ? "All-server inventory is unavailable." : serversQuery.isLoading ? "Waiting for the all-server inventory." : `Reported by ${serverDiskConfiguration.reported} of ${serverDiskConfiguration.total} servers.`}</p>
        <p className="mt-2 border-t border-white/[0.06] pt-2 text-xs text-slate-400">Node configured capacity: {nodesQuery.isError ? "unavailable" : nodesQuery.isLoading ? "loading…" : nodeDiskCapacity.reported ? `${nodeDiskCapacity.value.toLocaleString()} MiB across ${nodeDiskCapacity.reported}/${nodeDiskCapacity.total} nodes` : "not reported"}</p>
      </div>
    </div>
    <p className="-mt-3 mb-6 text-xs text-slate-500">Capacity is configured allocation, not live resource usage. Coverage counts identify records that supplied each configured value.</p>

    <div className="mb-6 grid gap-6 md:grid-cols-2">
      {serversQuery.isError ? (
        <Card>
          <CardHeader title="All-server status distribution" icon={Activity} />
          <QueryError message="All-server inventory is unavailable." />
        </Card>
      ) : serversQuery.isLoading ? (
        <Card>
          <CardHeader title="All-server status distribution" icon={Activity} />
          <QueryLoading message="Loading the complete server inventory…" />
        </Card>
      ) : (
        <Card>
          <CardHeader title="All-server status distribution" icon={Activity} />
          {servers.length === 0 ? (
            <EmptyState icon={Activity} message="No servers yet." />
          ) : (
            <SimplePieChart data={serverStatusData} total={servers.length} />
          )}
        </Card>
      )}

      {nodesQuery.isError ? (
        <Card>
          <CardHeader title="Node configured memory capacity" icon={Cpu} />
          <QueryError message="Nodes are unavailable." />
        </Card>
      ) : nodesQuery.isLoading ? (
        <Card>
          <CardHeader title="Node configured memory capacity" icon={Cpu} />
          <QueryLoading message="Loading node capacity…" />
        </Card>
      ) : (
        <Card>
          <CardHeader title="Node configured memory capacity" icon={Cpu} />
          {nodes.length === 0 ? (
            <EmptyState icon={Cpu} message="No nodes configured." />
          ) : nodeResourceData.length === 0 ? (
            <EmptyState icon={Cpu} message="Memory capacity is not reported by nodes." />
          ) : (
            <>
              <SimpleBarChart data={nodeResourceData} title="Configured memory capacity per node (MiB)" />
              <p className="mt-4 text-xs text-slate-500">Coverage: {nodeResourceData.length} of {nodes.length} nodes reported configured memory capacity. This is not live memory usage.</p>
            </>
          )}
        </Card>
      )}
    </div>

    <div className="grid gap-6 md:grid-cols-2">
      {nodesQuery.isError ? (
        <Card>
          <CardHeader title="Persisted node heartbeat" icon={Network} />
          <QueryError message="Nodes are unavailable." />
        </Card>
      ) : nodesQuery.isLoading ? (
        <Card>
          <CardHeader title="Persisted node heartbeat" icon={Network} />
          <QueryLoading message="Loading persisted node heartbeat state…" />
        </Card>
      ) : (
        <Card>
          <CardHeader title="Persisted node heartbeat" icon={Network} />
          {nodes.length === 0 ? (
            <EmptyState icon={Network} message="No nodes configured." />
          ) : (
            <>
            <div className="border-b border-white/[0.04] px-4 py-3 text-xs text-slate-400">
              {onlineNodes} healthy · {heartbeatReportedNodes}/{nodes.length} nodes have a persisted heartbeat state
            </div>
            <ul className="divide-y divide-white/[0.04]">
              {nodes.map((node) => (
                <li key={node.id} className="flex items-center justify-between px-4 py-3">
                  <div>
                    <p className="text-sm font-medium text-slate-200">{node.name}</p>
                    <p className="text-xs text-slate-500">{node.fqdn ?? node.region}</p>
                  </div>
                  <Pill tone={hasHealthyPersistedHeartbeat(node) ? "green" : node.heartbeatState === "degraded" ? "yellow" : node.heartbeatState ? "red" : "neutral"}>
                    {hasHealthyPersistedHeartbeat(node) ? "persisted: healthy" : `persisted: ${node.heartbeatState ?? "unreported"}`}
                  </Pill>
                </li>
              ))}
            </ul>
            </>
          )}
        </Card>
      )}

      {serversQuery.isError ? (
        <Card>
          <CardHeader title="Server inventory" icon={Layers} />
          <QueryError message="All-server inventory is unavailable." />
        </Card>
      ) : serversQuery.isLoading ? (
        <Card>
          <CardHeader title="Server inventory" icon={Layers} />
          <QueryLoading message="Loading the complete server inventory…" />
        </Card>
      ) : (
        <Card>
          <CardHeader title="Server inventory" icon={Layers} />
          {servers.length === 0 ? (
            <EmptyState icon={Layers} message="No servers yet." />
          ) : (
            <>
            <div className="border-b border-white/[0.04] px-4 py-3 text-xs text-slate-400">Showing {Math.min(8, servers.length)} of {servers.length} servers returned by the complete inventory.</div>
            <ul className="divide-y divide-white/[0.04]">
              {servers.slice(0, 8).map((server) => (
                <li key={server.id} className="flex items-center justify-between px-4 py-3">
                  <div>
                    <p className="text-sm font-medium text-slate-200">{server.name}</p>
                    <p className="text-xs text-slate-500">{server.node}</p>
                  </div>
                  <Pill tone={server.status === "running" ? "green" : server.status === "stopped" ? "neutral" : "yellow"}>
                    {server.suspended ? "suspended" : server.status}
                  </Pill>
                </li>
              ))}
            </ul>
            </>
          )}
        </Card>
      )}
    </div>

    <div className="mt-6 grid gap-6 md:grid-cols-2">
      <Card>
        <CardHeader title="Configured capacity coverage" icon={HardDrive} />
        {serversQuery.isError ? (
          <QueryError message="All-server inventory is unavailable; configured server capacity cannot be calculated." />
        ) : serversQuery.isLoading ? (
          <QueryLoading message="Loading configured server capacity…" />
        ) : (
          <div className="space-y-3 p-4 text-sm">
            <div>
              <p className="text-xs uppercase text-slate-500">Memory configuration</p>
              <p className="mt-1 font-semibold text-slate-200">{serverMemoryConfiguration.reported ? `${serverMemoryConfiguration.value.toLocaleString()} MiB across ${serverMemoryConfiguration.reported} of ${serverMemoryConfiguration.total} servers` : `No memory configuration reported by ${serverMemoryConfiguration.total} servers.`}</p>
            </div>
            <div>
              <p className="text-xs uppercase text-slate-500">Disk configuration</p>
              <p className="mt-1 font-semibold text-slate-200">{serverDiskConfiguration.reported ? `${serverDiskConfiguration.value.toLocaleString()} MiB across ${serverDiskConfiguration.reported} of ${serverDiskConfiguration.total} servers` : `No disk configuration reported by ${serverDiskConfiguration.total} servers.`}</p>
            </div>
            <p className="border-t border-white/[0.06] pt-3 text-xs text-slate-500">Totals include only finite configured values returned by the all-server inventory. They do not represent current usage.</p>
          </div>
        )}
      </Card>

      <Card>
        <CardHeader title="Actionable failures" icon={AlertTriangle} />
        {(nodesQuery.isError || serversQuery.isError || healthQuery.isError) ? (
          <>
            <QueryError message={`This list is incomplete: ${[nodesQuery.isError && "node heartbeat", serversQuery.isError && "server inventory", healthQuery.isError && "health checks"].filter(Boolean).join(", ")} data is unavailable.`} />
            {failures.length > 0 && (
              <ul className="divide-y divide-white/[0.04]">
                {failures.slice(0, 10).map((failure) => (
                  <li className="p-4" key={failure.id}>
                    <p className="font-semibold text-red-300">{failure.label}</p>
                    <p className="text-xs text-slate-400">{failure.detail}</p>
                  </li>
                ))}
              </ul>
            )}
          </>
        ) : (nodesQuery.isLoading || serversQuery.isLoading || healthQuery.isLoading) ? (
          <QueryLoading message="Loading node heartbeat, server inventory, and health checks…" />
        ) : failures.length === 0 ? (
          <EmptyState icon={AlertTriangle} message="No failures reported." />
        ) : (
          <ul className="divide-y divide-white/[0.04]">
            {failures.slice(0, 10).map((failure) => (
              <li className="p-4" key={failure.id}>
                <p className="font-semibold text-red-300">{failure.label}</p>
                <p className="text-xs text-slate-400">{failure.detail}</p>
              </li>
            ))}
          </ul>
        )}
      </Card>

      {activityQuery.isError ? (
        <Card>
          <CardHeader title="Recent admin activity" icon={Shield} />
          <QueryError message="Activity API unavailable." />
        </Card>
      ) : activityQuery.isLoading ? (
        <Card>
          <CardHeader title="Recent admin activity" icon={Shield} />
          <QueryLoading message="Loading administrative activity…" />
        </Card>
      ) : (
        <Card>
          <CardHeader title="Recent admin activity" icon={Shield} />
          {(activityQuery.data ?? []).length === 0 ? (
            <EmptyState icon={Shield} message="No administrative activity." />
          ) : (
            <ul className="divide-y divide-white/[0.04]">
              {(activityQuery.data ?? []).slice(0, 8).map((event) => (
                <li className="p-4" key={event.id}>
                  <div className="flex justify-between gap-3">
                    <p className="text-sm font-semibold text-slate-200">{event.action}</p>
                    <time className="text-xs text-slate-500">{new Date(event.createdAt).toLocaleString()}</time>
                  </div>
                  <p className="text-xs text-slate-500">{event.actorEmail ?? "system"} · {event.targetType}{event.targetId ? `:${event.targetId}` : ""}</p>
                </li>
              ))}
            </ul>
          )}
        </Card>
      )}
    </div>
  </div>
);
}