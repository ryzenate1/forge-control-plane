"use client";

import { useQuery } from "@tanstack/react-query";
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { Cpu } from "lucide-react";
import { getNodeMetrics, type NodeMetrics } from "@/lib/api/monitoring";
import { Card, CardHeader } from "@/components/admin/admin-ui";
import { SpinnerPage } from "@/components/shared";

interface ServerCPUChartProps {
  nodeId?: string;
  height?: number;
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
          CPU: {Number(entry.value).toFixed(1)}%
        </p>
      ))}
    </div>
  );
}

export function ServerCPUChart({ nodeId, height = 300 }: ServerCPUChartProps) {
  const { data, isLoading, isError } = useQuery({
    queryKey: ["cpu-chart", nodeId],
    queryFn: () => getNodeMetrics({ nodeId, period: "1h" }),
    refetchInterval: 30_000,
  });

  if (isLoading) {
    return (
      <Card>
        <CardHeader title="CPU Usage" icon={Cpu} />
        <div style={{ height }}>
          <SpinnerPage message="Loading CPU metrics..." />
        </div>
      </Card>
    );
  }

  if (isError) {
    return (
      <Card>
        <CardHeader title="CPU Usage" icon={Cpu} />
        <div className="flex h-64 items-center justify-center text-sm text-red-400">
          Failed to load CPU metrics
        </div>
      </Card>
    );
  }

  const chartData = (data ?? []).map((m: NodeMetrics) => ({
    timestamp: m.observedAt,
    value: m.cpuPercent,
    load1: m.cpuLoad1m,
    load5: m.cpuLoad5m,
    load15: m.cpuLoad15m,
  }));

  if (chartData.length === 0) {
    return (
      <Card>
        <CardHeader title="CPU Usage" icon={Cpu} />
        <div className="flex h-64 items-center justify-center text-sm text-slate-500">
          No CPU data available
        </div>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader title="CPU Usage" icon={Cpu} />
      <div className="p-4">
        <div style={{ height }}>
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={chartData} margin={{ top: 5, right: 5, left: 0, bottom: 5 }}>
              <defs>
                <linearGradient id="cpuGradient" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.3} />
                  <stop offset="95%" stopColor="#3b82f6" stopOpacity={0} />
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
                tick={{ fill: "#64748b", fontSize: 11 }}
                tickLine={false}
                axisLine={false}
                tickFormatter={(v) => `${v}%`}
                domain={[0, 100]}
                width={45}
              />
              <Tooltip content={<ChartTooltip />} />
              <Area
                type="monotone"
                dataKey="value"
                stroke="#3b82f6"
                strokeWidth={2}
                fill="url(#cpuGradient)"
                dot={false}
                activeDot={{ r: 4, fill: "#3b82f6" }}
                name="CPU %"
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </div>
    </Card>
  );
}
