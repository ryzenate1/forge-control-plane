"use client";

import { useQuery } from "@tanstack/react-query";
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { Network } from "lucide-react";
import { getNodeMetrics, type NodeMetrics } from "@/lib/api/monitoring";
import { Card, CardHeader } from "@/components/admin/admin-ui";
import { SpinnerPage } from "@/components/shared";

interface ServerNetworkChartProps {
  nodeId?: string;
  height?: number;
}

function formatBytes(bytes: number) {
  if (bytes >= 1e9) return `${(bytes / 1e9).toFixed(1)} GB`;
  if (bytes >= 1e6) return `${(bytes / 1e6).toFixed(1)} MB`;
  if (bytes >= 1e3) return `${(bytes / 1e3).toFixed(1)} KB`;
  return `${bytes} B`;
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
          {entry.name}: {formatBytes(Number(entry.value))}
        </p>
      ))}
    </div>
  );
}

export function ServerNetworkChart({ nodeId, height = 300 }: ServerNetworkChartProps) {
  const { data, isLoading, isError } = useQuery({
    queryKey: ["network-chart", nodeId],
    queryFn: () => getNodeMetrics({ nodeId, period: "1h" }),
    refetchInterval: 30_000,
  });

  if (isLoading) {
    return (
      <Card>
        <CardHeader title="Network I/O" icon={Network} />
        <div style={{ height }}>
          <SpinnerPage message="Loading network metrics..." />
        </div>
      </Card>
    );
  }

  if (isError) {
    return (
      <Card>
        <CardHeader title="Network I/O" icon={Network} />
        <div className="flex h-64 items-center justify-center text-sm text-red-400">
          Failed to load network metrics
        </div>
      </Card>
    );
  }

  const chartData = (data ?? []).map((m: NodeMetrics) => ({
    timestamp: m.observedAt,
    rx: m.networkRxBytes,
    tx: m.networkTxBytes,
  }));

  if (chartData.length === 0) {
    return (
      <Card>
        <CardHeader title="Network I/O" icon={Network} />
        <div className="flex h-64 items-center justify-center text-sm text-slate-500">
          No network data available
        </div>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader title="Network I/O" icon={Network} />
      <div className="p-4">
        <div style={{ height }}>
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={chartData} margin={{ top: 5, right: 5, left: 0, bottom: 5 }}>
              <defs>
                <linearGradient id="rxGradient" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="#8b5cf6" stopOpacity={0.3} />
                  <stop offset="95%" stopColor="#8b5cf6" stopOpacity={0} />
                </linearGradient>
                <linearGradient id="txGradient" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="#06b6d4" stopOpacity={0.3} />
                  <stop offset="95%" stopColor="#06b6d4" stopOpacity={0} />
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
                tickFormatter={(v) => formatBytes(v)}
                width={60}
              />
              <Tooltip content={<ChartTooltip />} />
              <Area
                type="monotone"
                dataKey="rx"
                stroke="#8b5cf6"
                strokeWidth={2}
                fill="url(#rxGradient)"
                dot={false}
                activeDot={{ r: 4, fill: "#8b5cf6" }}
                name="RX"
              />
              <Area
                type="monotone"
                dataKey="tx"
                stroke="#06b6d4"
                strokeWidth={2}
                fill="url(#txGradient)"
                dot={false}
                activeDot={{ r: 4, fill: "#06b6d4" }}
                name="TX"
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </div>
    </Card>
  );
}
