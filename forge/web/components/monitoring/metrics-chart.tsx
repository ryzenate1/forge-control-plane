"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { Activity, Cpu, HardDrive, Network } from "lucide-react";
import { Card, CardHeader } from "@/components/admin/admin-ui";
import { getNodeMetrics, type NodeMetrics } from "@/lib/api/monitoring";

type MetricKey = "cpuPercent" | "memoryPercent" | "diskPercent" | "networkRxBytes";
type Period = "5m" | "15m" | "1h" | "6h" | "24h";

const METRICS: { key: MetricKey; label: string; color: string; icon: typeof Cpu }[] = [
  { key: "cpuPercent", label: "CPU %", color: "#3b82f6", icon: Cpu },
  { key: "memoryPercent", label: "Memory %", color: "#10b981", icon: Activity },
  { key: "diskPercent", label: "Disk %", color: "#f59e0b", icon: HardDrive },
  { key: "networkRxBytes", label: "Network RX", color: "#8b5cf6", icon: Network },
];

const PERIODS: { value: Period; label: string }[] = [
  { value: "5m", label: "5 min" },
  { value: "15m", label: "15 min" },
  { value: "1h", label: "1 hour" },
  { value: "6h", label: "6 hours" },
  { value: "24h", label: "24 hours" },
];

function formatValue(key: MetricKey, value: number) {
  if (key === "networkRxBytes") {
    const bytes = value;
    if (bytes >= 1e9) return `${(bytes / 1e9).toFixed(1)} GB`;
    if (bytes >= 1e6) return `${(bytes / 1e6).toFixed(1)} MB`;
    if (bytes >= 1e3) return `${(bytes / 1e3).toFixed(1)} KB`;
    return `${bytes} B`;
  }
  return `${value.toFixed(1)}%`;
}

function ChartTooltip({ active, payload, label, metric }: {
  active?: boolean;
  payload?: Array<{ name: string; value: number }>;
  label?: string;
  metric: MetricKey;
}) {
  if (!active || !payload?.length) return null;
  return (
    <div className="rounded-lg border border-white/10 bg-[#1e2536] p-3 shadow-xl">
      <p className="text-xs text-slate-400">{new Date(label ?? "").toLocaleTimeString()}</p>
      {payload.map((entry) => (
        <p key={entry.name} className="text-sm font-semibold text-slate-200">
          {entry.value.toFixed(1)}{metric === "networkRxBytes" ? " bytes" : "%"}
        </p>
      ))}
    </div>
  );
}

export function MetricsChart() {
  const [selectedMetric, setSelectedMetric] = useState<MetricKey>("cpuPercent");
  const [selectedPeriod, setSelectedPeriod] = useState<Period>("5m");

  const metric = METRICS.find((m) => m.key === selectedMetric)!;

  const { data, isLoading, error } = useQuery({
    queryKey: ["metrics-chart", selectedPeriod],
    queryFn: () => getNodeMetrics({ period: selectedPeriod }),
    refetchInterval: selectedPeriod === "5m" ? 10_000 : 60_000,
  });

  const chartData = data?.map((m: NodeMetrics) => ({
    timestamp: m.observedAt,
    value: m[selectedMetric],
  })) ?? [];

  return (
    <Card>
      <CardHeader
        title={`Metrics: ${metric.label}`}
        icon={metric.icon}
        action={
          <div className="flex gap-1">
            {METRICS.map((m) => (
              <button
                key={m.key}
                className={`rounded px-2 py-1 text-xs font-medium transition ${
                  selectedMetric === m.key
                    ? "bg-blue-900/50 text-blue-300"
                    : "text-slate-500 hover:text-slate-300"
                }`}
                onClick={() => setSelectedMetric(m.key)}
                type="button"
              >
                {m.label}
              </button>
            ))}
          </div>
        }
      />
      <div className="p-4">
        <div className="mb-4 flex gap-1">
          {PERIODS.map((p) => (
            <button
              key={p.value}
              className={`rounded px-3 py-1 text-xs font-medium transition ${
                selectedPeriod === p.value
                  ? "bg-white/10 text-slate-200"
                  : "text-slate-600 hover:text-slate-400"
              }`}
              onClick={() => setSelectedPeriod(p.value)}
              type="button"
            >
              {p.label}
            </button>
          ))}
        </div>

        {isLoading ? (
          <div className="flex h-64 items-center justify-center text-sm text-slate-500">
            <div className="h-8 w-8 animate-spin rounded-full border-2 border-slate-600 border-t-blue-500" />
          </div>
        ) : error ? (
          <div className="flex h-64 items-center justify-center text-sm text-red-400">
            Failed to load metrics
          </div>
        ) : chartData.length === 0 ? (
          <div className="flex h-64 items-center justify-center text-sm text-slate-500">
            No metric data available
          </div>
        ) : (
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={chartData} margin={{ top: 5, right: 5, left: 0, bottom: 5 }}>
                <defs>
                  <linearGradient id="colorValue" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor={metric.color} stopOpacity={0.3} />
                    <stop offset="95%" stopColor={metric.color} stopOpacity={0} />
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
                  tickFormatter={(v) => formatValue(selectedMetric, v)}
                  width={60}
                />
                <Tooltip content={<ChartTooltip metric={selectedMetric} />} />
                <Area
                  type="monotone"
                  dataKey="value"
                  stroke={metric.color}
                  strokeWidth={2}
                  fill="url(#colorValue)"
                  dot={false}
                  activeDot={{ r: 4, fill: metric.color }}
                />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        )}
      </div>
    </Card>
  );
}
