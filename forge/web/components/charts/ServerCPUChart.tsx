"use client";

import { useQuery } from "@tanstack/react-query";
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { Cpu } from "lucide-react";
import { getNodeMetrics, type MetricPeriod, type NodeMetrics } from "@/lib/api/monitoring";
import { Card, CardHeader } from "@/components/admin/admin-ui";
import { chart } from "@/lib/design-tokens";
import { SpinnerPage } from "@/components/shared";

interface ServerCPUChartProps {
  nodeId?: string;
  height?: number;
  period?: MetricPeriod;
}

interface TooltipPayloadEntry {
  name: string;
  value: number;
  color: string;
}

function ChartTooltip({ active, payload, label }: { active?: boolean; payload?: TooltipPayloadEntry[]; label?: string }) {
  if (!active || !payload?.length) return null;
  return (
    <div className="rounded-lg border border-white/10 bg-[var(--surface-raised)] p-3 shadow-xl">
      <p className="text-xs text-slate-400">{label ? new Date(label).toLocaleTimeString() : ""}</p>
      {payload.map((entry) => (
        <p key={entry.name} className="text-sm font-semibold text-slate-200">
          CPU: {Number(entry.value).toFixed(1)}%
        </p>
      ))}
    </div>
  );
}

export function ServerCPUChart({ nodeId, height = 300, period = "1h" }: ServerCPUChartProps) {
  const { data, isLoading, isError } = useQuery({
    queryKey: ["cpu-chart", nodeId, period],
    queryFn: () => getNodeMetrics({ nodeId, period }),
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
                  <stop offset="5%" stopColor={chart.cpu} stopOpacity={0.3} />
                  <stop offset="95%" stopColor={chart.cpu} stopOpacity={0} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke={chart.grid} />
              <XAxis
                dataKey="timestamp"
                tick={{ fill: chart.axis, fontSize: 11 }}
                tickLine={false}
                axisLine={false}
                tickFormatter={(v) => new Date(v).toLocaleTimeString()}
              />
              <YAxis
                tick={{ fill: chart.axis, fontSize: 11 }}
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
                stroke={chart.cpu}
                strokeWidth={2}
                fill="url(#cpuGradient)"
                dot={false}
                activeDot={{ r: 4, fill: chart.cpu }}
                name="CPU %"
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </div>
    </Card>
  );
}
