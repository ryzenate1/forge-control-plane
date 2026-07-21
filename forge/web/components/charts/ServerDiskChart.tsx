"use client";

import { useQuery } from "@tanstack/react-query";
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { HardDrive } from "lucide-react";
import { getNodeMetrics, type NodeMetrics } from "@/lib/api/monitoring";
import { Card, CardHeader } from "@/components/admin/admin-ui";
import { SpinnerPage } from "@/components/shared";

interface ServerDiskChartProps {
  nodeId?: string;
  height?: number;
}

function formatMb(mb: number) {
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
          {entry.name}: {entry.name === "Used %" ? `${Number(entry.value).toFixed(1)}%` : formatMb(Number(entry.value))}
        </p>
      ))}
    </div>
  );
}

export function ServerDiskChart({ nodeId, height = 300 }: ServerDiskChartProps) {
  const { data, isLoading, isError } = useQuery({
    queryKey: ["disk-chart", nodeId],
    queryFn: () => getNodeMetrics({ nodeId, period: "1h" }),
    refetchInterval: 30_000,
  });

  if (isLoading) {
    return (
      <Card>
        <CardHeader title="Disk Usage" icon={HardDrive} />
        <div style={{ height }}>
          <SpinnerPage message="Loading disk metrics..." />
        </div>
      </Card>
    );
  }

  if (isError) {
    return (
      <Card>
        <CardHeader title="Disk Usage" icon={HardDrive} />
        <div className="flex h-64 items-center justify-center text-sm text-red-400">
          Failed to load disk metrics
        </div>
      </Card>
    );
  }

  const chartData = (data ?? []).map((m: NodeMetrics) => ({
    timestamp: m.observedAt,
    usedPercent: m.diskPercent,
    usedMb: m.diskUsedMb,
    totalMb: m.diskTotalMb,
  }));

  if (chartData.length === 0) {
    return (
      <Card>
        <CardHeader title="Disk Usage" icon={HardDrive} />
        <div className="flex h-64 items-center justify-center text-sm text-slate-500">
          No disk data available
        </div>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader title="Disk Usage" icon={HardDrive} />
      <div className="p-4">
        <div style={{ height }}>
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={chartData} margin={{ top: 5, right: 5, left: 0, bottom: 5 }}>
              <defs>
                <linearGradient id="diskGradient" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="#f59e0b" stopOpacity={0.3} />
                  <stop offset="95%" stopColor="#f59e0b" stopOpacity={0} />
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
                tickFormatter={(v) => formatMb(v)}
                width={60}
              />
              <Tooltip content={<ChartTooltip />} />
              <Area
                yAxisId="percent"
                type="monotone"
                dataKey="usedPercent"
                stroke="#f59e0b"
                strokeWidth={2}
                fill="url(#diskGradient)"
                dot={false}
                activeDot={{ r: 4, fill: "#f59e0b" }}
                name="Used %"
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </div>
    </Card>
  );
}
