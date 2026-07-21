"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Cpu, HardDrive, Info, Network, Server } from "lucide-react";
import { Card, CardHeader, SectionHeader } from "@/components/admin/admin-ui";
import {
  fetchHostInfo,
  fetchHostDisk,
  fetchHostMemory,
  fetchHostNetwork,
  fetchHostProcesses,
  type DiskPartition,
  type HostInfo,
  type MemoryInfo,
  type NetworkInterface,
  type ProcessEntry,
} from "@/lib/api/host";

type Tab = "info" | "disk" | "memory" | "network" | "processes";

const TABS: { id: Tab; label: string; icon: typeof Info }[] = [
  { id: "info", label: "Info", icon: Info },
  { id: "disk", label: "Disk", icon: HardDrive },
  { id: "memory", label: "Memory", icon: Cpu },
  { id: "network", label: "Network", icon: Network },
  { id: "processes", label: "Processes", icon: Server },
];

function usageBar(pct: number) {
  const color = pct > 80 ? "bg-red-500" : pct > 60 ? "bg-amber-500" : "bg-emerald-500";
  return (
    <div className="h-2 w-full overflow-hidden rounded-full bg-white/10">
      <div className={`h-full rounded-full ${color}`} style={{ width: `${Math.min(pct, 100)}%` }} />
    </div>
  );
}

function InfoTab() {
  const { data, isLoading } = useQuery({ queryKey: ["host-info"], queryFn: fetchHostInfo });
  if (isLoading) return <div className="text-sm text-slate-500">Loading...</div>;
  if (!data) return <div className="text-sm text-slate-500">No data available</div>;

  return (
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
  );
}

function StatRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between rounded-lg border border-white/[0.06] bg-[#151b27] px-4 py-2">
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

function DiskTab() {
  const { data, isLoading } = useQuery({ queryKey: ["host-disk"], queryFn: fetchHostDisk });
  if (isLoading) return <div className="text-sm text-slate-500">Loading...</div>;
  if (!data || data.length === 0) return <div className="text-sm text-slate-500">No disk data</div>;

  return (
    <div className="space-y-3">
      {data.map((part: DiskPartition) => (
        <div key={part.mountPoint} className="rounded-lg border border-white/[0.06] bg-[#151b27] p-4">
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
  );
}

function MemoryTab() {
  const { data, isLoading } = useQuery({ queryKey: ["host-memory"], queryFn: fetchHostMemory });
  if (isLoading) return <div className="text-sm text-slate-500">Loading...</div>;
  if (!data) return <div className="text-sm text-slate-500">No memory data</div>;

  return (
    <div className="space-y-4">
      <div className="rounded-lg border border-white/[0.06] bg-[#151b27] p-4">
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
  );
}

function NetworkTab() {
  const { data, isLoading } = useQuery({ queryKey: ["host-network"], queryFn: fetchHostNetwork });
  if (isLoading) return <div className="text-sm text-slate-500">Loading...</div>;
  if (!data || data.length === 0) return <div className="text-sm text-slate-500">No network data</div>;

  return (
    <div className="grid gap-3 md:grid-cols-2">
      {data.map((iface: NetworkInterface) => (
        <div key={iface.name} className="rounded-lg border border-white/[0.06] bg-[#151b27] p-4">
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
  );
}

function ProcessesTab() {
  const [sortBy, setSortBy] = useState<"cpu" | "mem">("cpu");
  const { data, isLoading } = useQuery({ queryKey: ["host-processes"], queryFn: fetchHostProcesses, refetchInterval: 10_000 });

  if (isLoading) return <div className="text-sm text-slate-500">Loading...</div>;
  if (!data || data.length === 0) return <div className="text-sm text-slate-500">No process data</div>;

  const sorted = [...data].sort((a, b) => {
    if (sortBy === "cpu") return b.cpuPercent - a.cpuPercent;
    return b.memoryPercent - a.memoryPercent;
  });

  return (
    <div>
      <div className="mb-3 flex gap-2">
        <button
          className={`rounded px-3 py-1 text-xs font-medium ${sortBy === "cpu" ? "bg-blue-900/50 text-blue-300" : "bg-white/5 text-slate-500"}`}
          onClick={() => setSortBy("cpu")}
          type="button"
        >
          Sort by CPU
        </button>
        <button
          className={`rounded px-3 py-1 text-xs font-medium ${sortBy === "mem" ? "bg-blue-900/50 text-blue-300" : "bg-white/5 text-slate-500"}`}
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
    </div>
  );
}

export default function AdminHost() {
  const [activeTab, setActiveTab] = useState<Tab>("info");

  const tabComponents: Record<Tab, () => React.ReactNode> = {
    info: InfoTab,
    disk: DiskTab,
    memory: MemoryTab,
    network: NetworkTab,
    processes: ProcessesTab,
  };

  const ActiveComponent = tabComponents[activeTab];

  return (
    <div className="space-y-6">
      <SectionHeader title="Host Management" sub="View detailed host system information" />

      <div className="flex gap-1 rounded-lg border border-white/[0.06] bg-[#151b27] p-1">
        {TABS.map((tab) => (
          <button
            key={tab.id}
            className={`flex items-center gap-2 rounded-md px-3 py-2 text-xs font-medium transition ${
              activeTab === tab.id
                ? "bg-blue-900/50 text-blue-300"
                : "text-slate-500 hover:text-slate-300"
            }`}
            onClick={() => setActiveTab(tab.id)}
            type="button"
          >
            <tab.icon size={14} />
            {tab.label}
          </button>
        ))}
      </div>

      <Card>
        <CardHeader title={TABS.find((t) => t.id === activeTab)?.label ?? ""} icon={TABS.find((t) => t.id === activeTab)?.icon} />
        <div className="p-4">
          <ActiveComponent />
        </div>
      </Card>
    </div>
  );
}
