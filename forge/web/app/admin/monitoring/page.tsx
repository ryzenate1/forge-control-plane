"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, RefreshCw } from "lucide-react";
import { Btn, Card, CardHeader, Pill, SectionHeader } from "@/components/admin/admin-ui";
import { MetricsChart } from "@/components/monitoring/metrics-chart";
import { SystemMetrics } from "@/components/monitoring/system-metrics";
import { NodeList } from "@/components/monitoring/node-list";
import { ServerCPUChart } from "@/components/charts/ServerCPUChart";
import { ServerMemoryChart } from "@/components/charts/ServerMemoryChart";
import { ServerDiskChart } from "@/components/charts/ServerDiskChart";
import { ServerNetworkChart } from "@/components/charts/ServerNetworkChart";
import { ResourceUsageBar } from "@/components/charts/ResourceUsageBar";
import { SystemHealthGauge } from "@/components/charts/SystemHealthGauge";
import { getAlertHistory, type AlertEvent } from "@/lib/api/monitoring";

function AlertRow({ alert }: { alert: AlertEvent }) {
  const severityTone = alert.severity === "critical" ? "red" : alert.severity === "warning" ? "yellow" : "blue";
  return (
    <div className="flex items-center justify-between border-b border-white/[0.06] px-4 py-2 text-sm">
      <div className="flex items-center gap-2">
        <Pill tone={severityTone}>{alert.severity}</Pill>
        <span className="text-slate-200">{alert.message}</span>
      </div>
      <span className="text-xs text-slate-500">{new Date(alert.createdAt).toLocaleString()}</span>
    </div>
  );
}

function AlertSkeleton() {
  return (
    <div className="space-y-2 p-4">
      {[1, 2, 3].map((i) => (
        <div key={`alert-skeleton-${i}`} className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <div className="h-5 w-16 animate-pulse rounded bg-white/5" />
            <div className="h-4 w-48 animate-pulse rounded bg-white/5" />
          </div>
          <div className="h-4 w-24 animate-pulse rounded bg-white/5" />
        </div>
      ))}
    </div>
  );
}

export default function AdminMonitoring() {
  const [selectedNode, setSelectedNode] = useState<string | null>(null);
  const [showAdvanced, setShowAdvanced] = useState(false);

  const { data: alerts, isLoading: alertsLoading, isError: alertsError, refetch: refetchAlerts } = useQuery({
    queryKey: ["alerts"],
    queryFn: () => getAlertHistory({ limit: 20 }),
    refetchInterval: 30_000,
  });

  return (
    <div className="space-y-6">
      <SectionHeader title="Monitoring Dashboard" sub="Real-time system monitoring and alerting" />

      {/* Summary Cards */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <SystemHealthGauge />
        <ServerCPUChart height={160} />
        <ServerMemoryChart height={160} />
        <ServerDiskChart height={160} />
      </div>

      {/* System Metrics */}
      <SystemMetrics />

      {/* Resource Usage Bar */}
      <ResourceUsageBar height={300} />

      {/* Node List & Network Chart */}
      <div className="grid gap-4 lg:grid-cols-2 xl:grid-cols-3">
        <div className="xl:col-span-2">
          <NodeList onNodeSelect={(id) => setSelectedNode(id === selectedNode ? null : id)} />
        </div>
        <ServerNetworkChart height={300} />
      </div>

      {/* Advanced Charts Toggle */}
      <div className="flex items-center gap-3">
        <button
          className="text-sm text-slate-400 hover:text-slate-200 transition"
          onClick={() => setShowAdvanced(!showAdvanced)}
          type="button"
        >
          {showAdvanced ? "Hide" : "Show"} detailed charts
        </button>
      </div>

      {showAdvanced && (
        <div className="space-y-6">
          <MetricsChart />
          <div className="grid gap-4 lg:grid-cols-2">
            <ServerCPUChart height={350} />
            <ServerMemoryChart height={350} />
          </div>
          <div className="grid gap-4 lg:grid-cols-2">
            <ServerDiskChart height={350} />
            <ServerNetworkChart height={350} />
          </div>
        </div>
      )}

      {/* Alerts */}
      <Card>
        <CardHeader
          title="Recent Alerts"
          icon={AlertTriangle}
          action={
            <Btn size="sm" tone="ghost" onClick={() => refetchAlerts()}>
              <RefreshCw size={12} />
            </Btn>
          }
        />
        {alertsLoading ? (
          <AlertSkeleton />
        ) : alertsError ? (
          <div className="p-4 text-sm text-red-400">Failed to load alerts</div>
        ) : !alerts || alerts.length === 0 ? (
          <div className="p-4 text-sm text-slate-500">No recent alerts</div>
        ) : (
          <div className="divide-y divide-white/[0.06]">
            {alerts.map((alert) => (
              <AlertRow key={alert.id} alert={alert} />
            ))}
          </div>
        )}
      </Card>
    </div>
  );
}
