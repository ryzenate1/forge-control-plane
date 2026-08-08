"use client";

import { useState } from "react";
import dynamic from "next/dynamic";
import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, RefreshCw } from "lucide-react";
import { AdminPageLayout, Btn, Card, CardHeader, Pill, SectionHeader } from "@/components/admin/admin-ui";
import { ErrorBoundary } from "@/components/ui/error-boundary";
import { MetricsChart } from "@/components/monitoring/metrics-chart";
import { SystemMetrics } from "@/components/monitoring/system-metrics";
import { NodeList } from "@/components/monitoring/node-list";
import { getAlertHistory, type AlertEvent } from "@/lib/api/monitoring";

const ServerCPUChart = dynamic(() => import("@/components/charts/ServerCPUChart").then((m) => m.ServerCPUChart), { ssr: false });
const ServerMemoryChart = dynamic(() => import("@/components/charts/ServerMemoryChart").then((m) => m.ServerMemoryChart), { ssr: false });
const ServerDiskChart = dynamic(() => import("@/components/charts/ServerDiskChart").then((m) => m.ServerDiskChart), { ssr: false });
const ServerNetworkChart = dynamic(() => import("@/components/charts/ServerNetworkChart").then((m) => m.ServerNetworkChart), { ssr: false });
const ResourceUsageBar = dynamic(() => import("@/components/charts/ResourceUsageBar").then((m) => m.ResourceUsageBar), { ssr: false });
const SystemHealthGauge = dynamic(() => import("@/components/charts/SystemHealthGauge").then((m) => m.SystemHealthGauge), { ssr: false });

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

  const nodeFilter = selectedNode ?? undefined;

  return (
    <AdminPageLayout>
      <SectionHeader
        title="Monitoring Dashboard"
        sub="Real-time system monitoring and alerting"
        action={selectedNode ? <div className="flex items-center gap-2 rounded-lg border border-white/10 bg-white/[0.03] px-3 py-1.5 text-xs text-slate-300"><span className="font-semibold text-slate-200">Node: {selectedNode}</span><button aria-label="Clear node filter" className="text-slate-500 hover:text-white" onClick={() => setSelectedNode(null)} type="button">✕</button></div> : undefined}
      />

      {/* Summary Cards */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <ErrorBoundary><SystemHealthGauge /></ErrorBoundary>
        <ErrorBoundary><ServerCPUChart height={160} nodeId={nodeFilter} /></ErrorBoundary>
        <ErrorBoundary><ServerMemoryChart height={160} nodeId={nodeFilter} /></ErrorBoundary>
        <ErrorBoundary><ServerDiskChart height={160} nodeId={nodeFilter} /></ErrorBoundary>
      </div>

      {/* System Metrics */}
      <ErrorBoundary><SystemMetrics /></ErrorBoundary>

      {/* Resource Usage Bar */}
      <ErrorBoundary><ResourceUsageBar height={300} /></ErrorBoundary>

      {/* Node List & Network Chart */}
      <div className="grid gap-4 lg:grid-cols-2 xl:grid-cols-3">
        <div className="xl:col-span-2">
          <ErrorBoundary><NodeList onNodeSelect={(id) => setSelectedNode(id === selectedNode ? null : id)} /></ErrorBoundary>
        </div>
        <ErrorBoundary><ServerNetworkChart height={300} nodeId={nodeFilter} /></ErrorBoundary>
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
          <ErrorBoundary><MetricsChart /></ErrorBoundary>
          <div className="grid gap-4 lg:grid-cols-2">
            <ErrorBoundary><ServerCPUChart height={350} nodeId={nodeFilter} /></ErrorBoundary>
            <ErrorBoundary><ServerMemoryChart height={350} nodeId={nodeFilter} /></ErrorBoundary>
          </div>
          <div className="grid gap-4 lg:grid-cols-2">
            <ErrorBoundary><ServerDiskChart height={350} nodeId={nodeFilter} /></ErrorBoundary>
            <ErrorBoundary><ServerNetworkChart height={350} nodeId={nodeFilter} /></ErrorBoundary>
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
    </AdminPageLayout>
  );
}
