"use client";

import { useQuery } from "@tanstack/react-query";
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { Card, CardHeader } from "@/components/admin/admin-ui";
import { getNodeMetrics, type NodeMetrics } from "@/lib/api/monitoring";
import { SpinnerPage } from "@/components/shared";

interface ResourceUsageBarProps {
  height?: number;
}

function ChartTooltip({ active, payload }: { active?: boolean; payload?: Array<{ payload: Record<string, number> }> }) {
  if (!active || !payload?.length) return null;
  const item = payload[0].payload;
  return (
    <div className="rounded-lg border border-white/10 bg-[#1e2536] p-3 shadow-xl">
      <p className="text-xs font-medium text-slate-200">{item.name}</p>
      {item.cpu !== undefined && (
        <p className="text-xs text-slate-400">CPU: {item.cpu.toFixed(1)}%</p>
      )}
      {item.memory !== undefined && (
        <p className="text-xs text-slate-400">Memory: {item.memory.toFixed(1)}%</p>
      )}
      {item.disk !== undefined && (
        <p className="text-xs text-slate-400">Disk: {item.disk.toFixed(1)}%</p>
      )}
    </div>
  );
}

export function ResourceUsageBar({ height = 300 }: ResourceUsageBarProps) {
  const { data, isLoading, isError } = useQuery({
    queryKey: ["resource-bar"],
    queryFn: () => getNodeMetrics(),
    refetchInterval: 30_000,
  });

  if (isLoading) {
    return (
      <Card>
        <CardHeader title="Resource Usage by Node" />
        <div style={{ height }}>
          <SpinnerPage message="Loading resource data..." />
        </div>
      </Card>
    );
  }

  if (isError) {
    return (
      <Card>
        <CardHeader title="Resource Usage by Node" />
        <div className="flex h-64 items-center justify-center text-sm text-red-400">
          Failed to load resource data
        </div>
      </Card>
    );
  }

  const chartData = (data ?? []).slice(0, 10).map((m: NodeMetrics) => ({
    name: m.nodeId.length > 12 ? `${m.nodeId.slice(0, 12)}…` : m.nodeId,
    cpu: m.cpuPercent,
    memory: m.memoryPercent,
    disk: m.diskPercent,
  }));

  if (chartData.length === 0) {
    return (
      <Card>
        <CardHeader title="Resource Usage by Node" />
        <div className="flex h-64 items-center justify-center text-sm text-slate-500">
          No resource data available
        </div>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader title="Resource Usage by Node" />
      <div className="p-4">
        <div className="mb-4 flex gap-4 text-xs text-slate-500">
          <span className="flex items-center gap-1">
            <span className="h-2.5 w-2.5 rounded-sm bg-blue-500" /> CPU
          </span>
          <span className="flex items-center gap-1">
            <span className="h-2.5 w-2.5 rounded-sm bg-emerald-500" /> Memory
          </span>
          <span className="flex items-center gap-1">
            <span className="h-2.5 w-2.5 rounded-sm bg-amber-500" /> Disk
          </span>
        </div>
        <div style={{ height }}>
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={chartData} margin={{ top: 5, right: 5, left: 0, bottom: 5 }} barGap={2}>
              <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.06)" />
              <XAxis
                dataKey="name"
                tick={{ fill: "#64748b", fontSize: 11 }}
                tickLine={false}
                axisLine={false}
              />
              <YAxis
                tick={{ fill: "#64748b", fontSize: 11 }}
                tickLine={false}
                axisLine={false}
                tickFormatter={(v) => `${v}%`}
                domain={[0, 100]}
                width={45}
              />
              <Tooltip content={<ChartTooltip />} />
              <Bar dataKey="cpu" fill="#3b82f6" radius={[2, 2, 0, 0]} maxBarSize={12} />
              <Bar dataKey="memory" fill="#10b981" radius={[2, 2, 0, 0]} maxBarSize={12} />
              <Bar dataKey="disk" fill="#f59e0b" radius={[2, 2, 0, 0]} maxBarSize={12} />
            </BarChart>
          </ResponsiveContainer>
        </div>
      </div>
    </Card>
  );
}
