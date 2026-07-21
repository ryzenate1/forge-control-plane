"use client";

import { useQuery } from "@tanstack/react-query";
import { Activity, Cpu, HardDrive, Network } from "lucide-react";
import { Card, CardHeader, SectionHeader } from "@/components/admin/admin-ui";
import { getNodeMetrics } from "@/lib/api/monitoring";

function miniBar(pct: number, color: string) {
  return (
    <div className="h-2 w-full overflow-hidden rounded-full bg-white/10">
      <div className={`h-full rounded-full ${color}`} style={{ width: `${Math.min(pct, 100)}%` }} />
    </div>
  );
}

function formatBytes(bytes: number) {
  if (bytes >= 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
  if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${bytes} B`;
}

function MetricCardSkeleton() {
  return (
    <Card>
      <CardHeader title="..." icon={Cpu} />
      <div className="p-4 space-y-3">
        <div className="h-9 w-20 animate-pulse rounded bg-white/5" />
        <div className="h-2 w-full animate-pulse rounded bg-white/5" />
        <div className="h-4 w-36 animate-pulse rounded bg-white/5" />
      </div>
    </Card>
  );
}

export function SystemMetrics() {
  const { data: metrics, isLoading, isError, error } = useQuery({
    queryKey: ["node-metrics"],
    queryFn: () => getNodeMetrics({ period: "5m" }),
    refetchInterval: 15_000,
  });

  if (isLoading) {
    return (
      <div className="space-y-6">
        <SectionHeader title="System Metrics" sub="Real-time system resource usage across all nodes" />
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          <MetricCardSkeleton />
          <MetricCardSkeleton />
          <MetricCardSkeleton />
          <MetricCardSkeleton />
        </div>
      </div>
    );
  }

  if (isError) {
    return (
      <div className="space-y-6">
        <SectionHeader title="System Metrics" sub="Real-time system resource usage across all nodes" />
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          {[
            { label: "CPU Usage", icon: Cpu },
            { label: "Memory", icon: Activity },
            { label: "Disk", icon: HardDrive },
            { label: "Network I/O", icon: Network },
          ].map((m) => (
            <Card key={m.label}>
              <CardHeader title={m.label} icon={m.icon} />
              <div className="p-4 text-sm text-red-400">Error loading data</div>
            </Card>
          ))}
        </div>
      </div>
    );
  }

  const latest = metrics && metrics.length > 0 ? metrics[metrics.length - 1] : null;

  const cpuAvg = metrics && metrics.length > 0
    ? metrics.reduce((acc, m) => acc + m.cpuPercent, 0) / metrics.length
    : 0;
  const memAvg = metrics && metrics.length > 0
    ? metrics.reduce((acc, m) => acc + m.memoryPercent, 0) / metrics.length
    : 0;

  const totalRx = metrics && metrics.length > 0 ? metrics.reduce((acc, m) => acc + m.networkRxBytes, 0) : 0;
  const totalTx = metrics && metrics.length > 0 ? metrics.reduce((acc, m) => acc + m.networkTxBytes, 0) : 0;

  return (
    <div className="space-y-6">
      <SectionHeader title="System Metrics" sub="Real-time system resource usage across all nodes" />

      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardHeader title="CPU Usage" icon={Cpu} />
          <div className="p-4">
            <p className="text-3xl font-bold text-slate-100">{cpuAvg.toFixed(1)}%</p>
            <div className="mt-2">{miniBar(cpuAvg, "bg-blue-500")}</div>
            <p className="mt-2 text-xs text-slate-500">Average across {metrics?.length || 0} data points</p>
          </div>
        </Card>

        <Card>
          <CardHeader title="Memory" icon={Activity} />
          <div className="p-4">
            <p className="text-3xl font-bold text-slate-100">{memAvg.toFixed(1)}%</p>
            <div className="mt-2">{miniBar(memAvg, "bg-emerald-500")}</div>
            {latest && (
              <p className="mt-2 text-xs text-slate-500">
                {formatBytes(latest.memoryUsedMb * 1024 * 1024)} / {formatBytes(latest.memoryTotalMb * 1024 * 1024)}
              </p>
            )}
          </div>
        </Card>

        <Card>
          <CardHeader title="Disk" icon={HardDrive} />
          <div className="p-4">
            {latest ? (
              <>
                <p className="text-3xl font-bold text-slate-100">{latest.diskPercent.toFixed(1)}%</p>
                <div className="mt-2">{miniBar(latest.diskPercent, "bg-amber-500")}</div>
                <p className="mt-2 text-xs text-slate-500">
                  {formatBytes(latest.diskUsedMb * 1024 * 1024)} / {formatBytes(latest.diskTotalMb * 1024 * 1024)}
                </p>
              </>
            ) : (
              <p className="text-sm text-slate-500">No data</p>
            )}
          </div>
        </Card>

        <Card>
          <CardHeader title="Network I/O" icon={Network} />
          <div className="p-4">
            <p className="text-sm text-slate-400">RX</p>
            <p className="text-xl font-bold text-slate-100">{formatBytes(totalRx)}</p>
            <p className="mt-2 text-sm text-slate-400">TX</p>
            <p className="text-xl font-bold text-slate-100">{formatBytes(totalTx)}</p>
            <p className="mt-2 text-xs text-slate-500">Last 5 minutes</p>
          </div>
        </Card>
      </div>
    </div>
  );
}
