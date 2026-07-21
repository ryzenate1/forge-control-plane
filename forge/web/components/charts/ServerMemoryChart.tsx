"use client";

import { useQuery } from "@tanstack/react-query";
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { Activity } from "lucide-react";
import { getNodeMetrics, type NodeMetrics } from "@/lib/api/monitoring";
import { Card, CardHeader } from "@/components/admin/admin-ui";
import { SpinnerPage } from "@/components/shared";

interface ServerMemoryChartProps {
  nodeId?: string;
  height?: number;
}

function formatBytes(mb: number) {
  if (mb >= 1024) return `${(mb / 1024).toFixed(1)} GB`;
  return `${mb.toFixed(0)} MB`;
}

interface TooltipPayloadEntry {
  name: string;
  value: number;
  color: string;
}

function ChartTooltip({ active, payload, label }: { active?: boolean; payload?: TooltipPayloadEntry[]; label?: string }) {
  if (!active || !payload?.length) return null;
  return (
    <div className="rounded-lg border border-white/10 bg-[#1e2536] p-3 shadow-xl">
      <p className="text-xs text-slate-400">{label ? new Date(label).toLocaleTimeString() : ""}</p>
      {payload.map((entry) => (
        <p key={entry.name} className="text-sm font-semibold text-slate-200">
          {entry.name}: {entry.name === "Used %" ? `${Number(entry.value).toFixed(1)}%` : formatBytes(Number(entry.value))}
        </p>
      ))}
    </div>
  );
}

export function ServerMemoryChart({ nodeId, height = 300 }: ServerMemoryChartProps) {
  const { data, isLoading, isError } = useQuery({
    queryKey: ["memory-chart", nodeId],
    queryFn: () => getNodeMetrics({ nodeId, period: "1h" }),
    refetchInterval: 30_000,
  });

  if (isLoading) {
    return (
      <Card>
        <CardHeader title="Memory Usage" icon={Activity} />
        <div style={{ height }}>
          <SpinnerPage message="Loading memory metrics..." />
        </div>
      </Card>
    );
  }

  if (isError) {
    return (
      <Card>
        <CardHeader title="Memory Usage" icon={Activity} />
        <div className="flex h-64 items-center justify-center text-sm text-red-400">
          Failed to load memory metrics
        </div>
      </Card>
    );
  }

  const chartData = (data ?? []).map((m: NodeMetrics) => ({
    timestamp: m.observedAt,
    usedPercent: m.memoryPercent,
    usedMb: m.memoryUsedMb,
    totalMb: m.memoryTotalMb,
  }));

  if (chartData.length === 0) {
    return (
      <Card>
        <CardHeader title="Memory Usage" icon={Activity} />
        <div className="flex h-64 items-center justify-center text-sm text-slate-500">
          No memory data available
        </div>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader title="Memory Usage" icon={Activity} />
      <div className="p-4">
        <div style={{ height }}>
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={chartData} margin={{ top: 5, right: 5, left: 0, bottom: 5 }}>
              <defs>
                <linearGradient id="memGradient" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="#10b981" stopOpacity={0.3} />
                  <stop offset="95%" stopColor="#10b981" stopOpacity={0} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.06)" />
              <XAxis
                dataKey="timestamp"
                tick={{ fill: "#64748b", fontSize: 11 }}
                tickLine={false}
                axisLine={false}
                tickFormatter={(v) => new Date(v).toLocaleTimeString()}
              />
              <YAxis
                yAxisId="percent"
                orientation="left"
                tick={{ fill: "#64748b", fontSize: 11 }}
                tickLine={false}
                axisLine={false}
                tickFormatter={(v) => `${v}%`}
                domain={[0, 100]}
                width={45}
              />
              <YAxis
                yAxisId="mb"
                orientation="right"
                tick={{ fill: "#64748b", fontSize: 11 }}
                tickLine={false}
                axisLine={false}
                tickFormatter={(v) => formatBytes(v)}
                width={60}
              />
              <Tooltip content={<ChartTooltip />} />
              <Area
                yAxisId="percent"
                type="monotone"
                dataKey="usedPercent"
                stroke="#10b981"
                strokeWidth={2}
                fill="url(#memGradient)"
                dot={false}
                activeDot={{ r: 4, fill: "#10b981" }}
                name="Used %"
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </div>
    </Card>
  );
}
