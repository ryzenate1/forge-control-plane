"use client";

import { useState, useCallback, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { Cpu, HardDrive, Info, Network, Server } from "lucide-react";
import { AdminLoadingState, AdminTabs, Card, CardHeader, EmptyState, SectionHeader, AdminPageLayout, AdminErrorState } from "@/components/admin/admin-ui";
import { NodeSelect, pickDefaultNode } from "@/components/admin/node-select";
import { fetchNodes } from "@/lib/api";
import {
  fetchHostInfo,
  fetchHostDisk,
  fetchHostMemory,
  fetchHostNetwork,
  fetchHostProcesses,
  type DiskPartition,
  type NetworkInterface,
  type ProcessEntry,
} from "@/lib/api/host";
import { ApiError } from "@/lib/api/http";

type Tab = "info" | "disk" | "memory" | "network" | "processes";

const TABS: { id: Tab; label: string; icon: typeof Info }[] = [
  { id: "info", label: "Info", icon: Info },
  { id: "disk", label: "Disk", icon: HardDrive },
  { id: "memory", label: "Memory", icon: Cpu },
  { id: "network", label: "Network", icon: Network },
  { id: "processes", label: "Processes", icon: Server },
];

function useHostQuery<T>(
  queryKey: string[],
  fetchFn: (init?: RequestInit) => Promise<T>,
  refetchInterval: number | false = 30_000,
) {
  return useQuery({
    queryKey,
    queryFn: ({ signal }) => {
      const timeout = AbortSignal.timeout(15000);
      const combined = signal ? AbortSignal.any([signal, timeout]) : timeout;
      return fetchFn({ signal: combined });
    },
    retry: 1,
    refetchInterval,
    placeholderData: (prev) => prev,
    staleTime: 10_000,
  });
}

function fmtTime(ts: Date | null): string {
  if (!ts) return "";
  const delta = Math.floor((Date.now() - ts.getTime()) / 1000);
  if (delta < 10) return "just now";
  if (delta < 60) return `${delta}s ago`;
  if (delta < 3600) return `${Math.floor(delta / 60)}m ${delta % 60}s ago`;
  return `${Math.floor(delta / 3600)}h ago`;
}

function formatError(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 504) return "Connection timeout — the Beacon daemon did not respond in time";
    if (error.status === 502) return "Node offline — Beacon daemon is unreachable";
    if (error.status === 404) return "No node found — register a node first";
    if (error.status === 503) return "Service unavailable — daemon proxy is not ready";
    return error.message;
  }
  return "An unexpected error occurred";
}

function InfoTab({ nodeId }: { nodeId: string }) {
  const { data, isLoading, error, refetch, dataUpdatedAt } = useHostQuery(
    ["host-info", nodeId],
    fetchHostInfo,
    30_000,
  );
  const lastUpdated = dataUpdatedAt ? new Date(dataUpdatedAt) : null;

  if (isLoading && !data) return <AdminLoadingState label="Loading host information…" />;
  if (error && !data) return (
    <div>
      <AdminErrorState message={formatError(error)} retry={() => refetch()} />
    </div>
  );
  if (!data) return <EmptyState title="Host information unavailable" message="The host did not return system information." />;

  return (
    <div>
      <div className="grid gap-4 md:grid-cols-2">
        <div className="space-y-3">
          <StatRow label="Hostname" value={data.hostname} />
          <StatRow label="OS" value={data.os} />
          <StatRow label="Kernel" value={data.kernel} />
          <StatRow label="Architecture" value={data.arch} />
        </div>
        <div className="space-y-3">
          <StatRow label="CPU Model" value={data.cpuModel} />
          <StatRow label="CPU Cores" value={String(data.cpuCores)} />
          <StatRow label="Uptime" value={fmtUptime(data.uptimeSeconds)} />
          <StatRow label="Time" value={new Date(data.time).toLocaleString()} />
        </div>
      </div>
      {lastUpdated && <p className="mt-4 text-right text-[11px] text-slate-500">Updated {fmtTime(lastUpdated)}</p>}
    </div>
  );
}

function DiskTab({ nodeId }: { nodeId: string }) {
  const { data, isLoading, error, refetch, dataUpdatedAt } = useHostQuery(
    ["host-disk", nodeId],
    fetchHostDisk,
    30_000,
  );
  const lastUpdated = dataUpdatedAt ? new Date(dataUpdatedAt) : null;

  if (isLoading && !data) return <AdminLoadingState label="Loading disk usage…" />;
  if (error && !data) return (
    <AdminErrorState message={formatError(error)} retry={() => refetch()} />
  );
  if (!data || data.length === 0) return <EmptyState title="No disk data" message="The host did not report any mounted disk partitions." />;

  return (
    <div>
      <div className="space-y-3">
        {data.map((part: DiskPartition) => (
          <div key={part.mountPoint} className="rounded-xl border border-white/[0.07] bg-white/[0.018] p-4">
            <div className="mb-2 flex items-center justify-between">
              <span className="text-sm font-medium text-slate-200">{part.mountPoint}</span>
              <span className="text-xs text-slate-500">{part.device}</span>
            </div>
            {usageBar(part.usedPercent)}
            <div className="mt-2 flex items-center justify-between text-xs text-slate-500">
              <span>{part.usedPercent.toFixed(1)}% used</span>
              <span>{fmtMB(part.usedMb)} / {fmtMB(part.totalMb)}</span>
            </div>
          </div>
        ))}
      </div>
      {lastUpdated && <p className="mt-4 text-right text-[11px] text-slate-500">Updated {fmtTime(lastUpdated)}</p>}
    </div>
  );
}

function MemoryTab({ nodeId }: { nodeId: string }) {
  const { data, isLoading, error, refetch, dataUpdatedAt } = useHostQuery(
    ["host-memory", nodeId],
    fetchHostMemory,
    30_000,
  );
  const lastUpdated = dataUpdatedAt ? new Date(dataUpdatedAt) : null;

  if (isLoading && !data) return <AdminLoadingState label="Loading memory usage…" />;
  if (error && !data) return (
    <div>
      <AdminErrorState message={formatError(error)} retry={() => refetch()} />
    </div>
  );
  if (!data) return <EmptyState title="No memory data" message="The host did not return memory usage." />;

  return (
    <div>
      <div className="space-y-4">
        <div className="rounded-xl border border-white/[0.07] bg-white/[0.018] p-4">
          <div className="mb-2 flex items-center justify-between">
            <span className="text-sm font-medium text-slate-200">Memory Usage</span>
            <span className="text-sm text-slate-400">{data.usedPercent.toFixed(1)}%</span>
          </div>
          {usageBar(data.usedPercent)}
          <div className="mt-2 flex items-center justify-between text-xs text-slate-500">
            <span>{fmtMB(data.usedMb)} used</span>
            <span>{fmtMB(data.freeMb)} free</span>
            <span>{fmtMB(data.totalMb)} total</span>
          </div>
        </div>
      </div>
      {lastUpdated && <p className="mt-4 text-right text-[11px] text-slate-500">Updated {fmtTime(lastUpdated)}</p>}
    </div>
  );
}

function NetworkTab({ nodeId }: { nodeId: string }) {
  const { data, isLoading, error, refetch, dataUpdatedAt } = useHostQuery(
    ["host-network", nodeId],
    fetchHostNetwork,
    30_000,
  );
  const lastUpdated = dataUpdatedAt ? new Date(dataUpdatedAt) : null;

  if (isLoading && !data) return <AdminLoadingState label="Loading network interfaces…" />;
  if (error && !data) return (
    <div>
      <AdminErrorState message={formatError(error)} retry={() => refetch()} />
    </div>
  );
  if (!data || data.length === 0) return <EmptyState title="No network data" message="The host did not report network interfaces." />;

  return (
    <div>
      <div className="grid gap-3 md:grid-cols-2">
        {data.map((iface: NetworkInterface) => (
          <div key={iface.name} className="rounded-xl border border-white/[0.07] bg-white/[0.018] p-4">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium text-slate-200">{iface.name}</span>
              <span className={`text-xs ${iface.status === "up" ? "text-emerald-400" : "text-red-400"}`}>
                {iface.status}
              </span>
            </div>
            {iface.ips && <p className="mt-1 text-xs text-slate-500">{iface.ips}</p>}
            {iface.mac && <p className="text-xs text-slate-500">MAC: {iface.mac}</p>}
            {iface.speedMbps > 0 && <p className="text-xs text-slate-500">{iface.speedMbps} Mbps</p>}
          </div>
        ))}
      </div>
      {lastUpdated && <p className="mt-4 text-right text-[11px] text-slate-500">Updated {fmtTime(lastUpdated)}</p>}
    </div>
  );
}

function ProcessesTab({ nodeId }: { nodeId: string }) {
  const [sortBy, setSortBy] = useState<"cpu" | "mem">("cpu");
  const { data, isLoading, error, refetch, dataUpdatedAt } = useHostQuery(
    ["host-processes", nodeId],
    fetchHostProcesses,
    10_000,
  );
  const lastUpdated = dataUpdatedAt ? new Date(dataUpdatedAt) : null;

  if (isLoading && !data) return <AdminLoadingState label="Loading processes…" />;
  if (error && !data) return (
    <AdminErrorState message={formatError(error)} retry={() => refetch()} />
  );
  if (!data || data.length === 0) return <EmptyState title="No process data" message="The host did not return any running processes." />;

  const sorted = [...data].sort((a, b) => {
    if (sortBy === "cpu") return b.cpuPercent - a.cpuPercent;
    return b.memoryPercent - a.memoryPercent;
  });

  return (
    <div>
      <div className="mb-3 flex gap-2">
        <button
          className={`rounded-lg border px-3 py-1.5 text-xs font-medium ${sortBy === "cpu" ? "border-red-400/50 bg-red-500/10 text-red-200" : "border-transparent bg-white/[0.04] text-slate-400"}`}
          onClick={() => setSortBy("cpu")}
          type="button"
        >
          Sort by CPU
        </button>
        <button
          className={`rounded-lg border px-3 py-1.5 text-xs font-medium ${sortBy === "mem" ? "border-red-400/50 bg-red-500/10 text-red-200" : "border-transparent bg-white/[0.04] text-slate-400"}`}
          onClick={() => setSortBy("mem")}
          type="button"
        >
          Sort by Memory
        </button>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-white/[0.06] text-left text-xs font-semibold uppercase tracking-widest text-slate-500">
              <th className="px-4 py-2">PID</th>
              <th className="px-4 py-2">Name</th>
              <th className="px-4 py-2">CPU %</th>
              <th className="px-4 py-2">Memory %</th>
              <th className="px-4 py-2">State</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-white/[0.06]">
            {sorted.slice(0, 100).map((proc: ProcessEntry) => (
              <tr key={proc.pid} className="hover:bg-white/[0.03]">
                <td className="px-4 py-2 font-mono text-xs text-slate-400">{proc.pid}</td>
                <td className="px-4 py-2 font-medium text-slate-200">{proc.name}</td>
                <td className="px-4 py-2 text-slate-300">{proc.cpuPercent.toFixed(1)}</td>
                <td className="px-4 py-2 text-slate-300">{proc.memoryPercent.toFixed(1)}</td>
                <td className="px-4 py-2 text-slate-400">{proc.state}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {lastUpdated && <p className="mt-4 text-right text-[11px] text-slate-500">Updated {fmtTime(lastUpdated)}</p>}
    </div>
  );
}

function usageBar(pct: number) {
  const color = pct > 80 ? "bg-red-500" : pct > 60 ? "bg-amber-500" : "bg-emerald-500";
  return (
    <div className="h-2 w-full overflow-hidden rounded-full bg-white/10">
      <div className={`h-full rounded-full ${color}`} style={{ width: `${Math.min(pct, 100)}%` }} />
    </div>
  );
}

function StatRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4 rounded-xl border border-white/[0.07] bg-white/[0.018] px-4 py-3">
      <span className="text-xs font-semibold uppercase tracking-widest text-slate-500">{label}</span>
      <span className="text-sm font-medium text-slate-200">{value || "-"}</span>
    </div>
  );
}

function fmtUptime(seconds: number) {
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function fmtMB(mb: number) {
  if (mb >= 1024 * 1024) return `${(mb / 1024 / 1024).toFixed(1)} TB`;
  if (mb >= 1024) return `${(mb / 1024).toFixed(1)} GB`;
  return `${Math.round(mb)} MB`;
}

export default function AdminHost() {
  const [activeTab, setActiveTab] = useState<Tab>("info");
  const [nodeId, setNodeId] = useState("");

  const nodesQuery = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });

  useEffect(() => {
    if (!nodeId && nodesQuery.data && nodesQuery.data.length > 0) {
      setNodeId(pickDefaultNode(nodesQuery.data));
    }
  }, [nodeId, nodesQuery.data]);

  const handleNodeChange = useCallback((id: string) => {
    setNodeId(id);
  }, []);

  const nodesLoading = nodesQuery.isLoading;
  const noNodes = nodesQuery.data && nodesQuery.data.length === 0;

  const sub = nodeId
    ? "Real-time system data from the selected Beacon daemon node"
    : "Select a node to view its system information";

  return (
    <AdminPageLayout>
      <SectionHeader
        title="Host Management"
        sub={sub}
        action={
          nodesLoading ? null : noNodes ? null : (
            <NodeSelect value={nodeId} onChange={handleNodeChange} />
          )
        }
      />

      {nodesLoading ? (
        <AdminLoadingState label="Loading nodes…" />
      ) : noNodes ? (
        <Card>
          <CardHeader title="No Beacon Registered" icon={Server} />
          <div className="p-4">
            <EmptyState
              title="No nodes available"
              message="No Beacon daemon nodes are registered. Add a node in the Infrastructure &gt; Nodes section first."
            />
          </div>
        </Card>
      ) : !nodeId ? (
        <Card>
          <CardHeader title="Select a Node" icon={Server} />
          <div className="p-4">
            <EmptyState
              title="No node selected"
              message="Select a node from the dropdown above to view its host information."
            />
          </div>
        </Card>
      ) : (
        <>
          <AdminTabs active={activeTab} label="Host sections" onChange={(id) => setActiveTab(id as Tab)} tabs={TABS} />

          <Card>
            <CardHeader
              title={TABS.find((t) => t.id === activeTab)?.label ?? ""}
              icon={TABS.find((t) => t.id === activeTab)?.icon}
            />
            <div className="p-4">
              {activeTab === "info" && <InfoTab nodeId={nodeId} />}
              {activeTab === "disk" && <DiskTab nodeId={nodeId} />}
              {activeTab === "memory" && <MemoryTab nodeId={nodeId} />}
              {activeTab === "network" && <NetworkTab nodeId={nodeId} />}
              {activeTab === "processes" && <ProcessesTab nodeId={nodeId} />}
            </div>
          </Card>
        </>
      )}
    </AdminPageLayout>
  );
}
