"use client";

import { useQuery } from "@tanstack/react-query";
import { Activity } from "lucide-react";
import { Card, CardHeader } from "@/components/admin/admin-ui";
import { getSystemInfo } from "@/lib/api/monitoring";

export function SystemHealthGauge() {
  const { data, isLoading, isError } = useQuery({
    queryKey: ["system-health"],
    queryFn: getSystemInfo,
    refetchInterval: 30_000,
  });

  if (isLoading) {
    return (
      <Card>
        <CardHeader title="System Health" icon={Activity} />
        <div className="p-4">
          <div className="mx-auto h-32 w-32 animate-pulse rounded-full bg-white/5" />
        </div>
      </Card>
    );
  }

  if (isError) {
    return (
      <Card>
        <CardHeader title="System Health" icon={Activity} />
        <div className="p-4 text-sm text-red-400">Failed to load health data</div>
      </Card>
    );
  }

  const nodeCount = data?.nodes?.length ?? 0;
  const healthyNodes = data?.nodes?.filter((n) => n.cpuPercent < 90 && n.memoryPercent < 90 && n.diskPercent < 90).length ?? 0;
  const healthScore = nodeCount > 0 ? Math.round((healthyNodes / nodeCount) * 100) : 0;
  const unacknowledged = data?.unacknowledgedAlerts ?? 0;

  const getColor = () => {
    if (healthScore >= 90) return { stroke: "#10b981", text: "text-emerald-400", label: "Healthy" };
    if (healthScore >= 70) return { stroke: "#f59e0b", text: "text-amber-400", label: "Degraded" };
    return { stroke: "#ef4444", text: "text-red-400", label: "Critical" };
  };

  const color = getColor();
  const circumference = 2 * Math.PI * 54;
  const offset = circumference - (healthScore / 100) * circumference;

  return (
    <Card>
      <CardHeader title="System Health" icon={Activity} />
      <div className="flex flex-col items-center p-6">
        <div className="relative h-32 w-32">
          <svg className="h-full w-full -rotate-90" viewBox="0 0 120 120">
            <circle cx="60" cy="60" r="54" fill="none" stroke="rgba(255,255,255,0.06)" strokeWidth="8" />
            <circle
              cx="60"
              cy="60"
              r="54"
              fill="none"
              stroke={color.stroke}
              strokeWidth="8"
              strokeLinecap="round"
              strokeDasharray={circumference}
              strokeDashoffset={offset}
              className="transition-all duration-1000"
            />
          </svg>
          <div className="absolute inset-0 flex flex-col items-center justify-center">
            <span className={`text-2xl font-bold ${color.text}`}>{healthScore}%</span>
          </div>
        </div>
        <p className={`mt-2 text-sm font-semibold ${color.text}`}>{color.label}</p>
        <div className="mt-3 flex gap-4 text-xs text-slate-500">
          <span>{nodeCount} nodes</span>
          {unacknowledged > 0 && <span className="text-red-400">{unacknowledged} alerts</span>}
        </div>
      </div>
    </Card>
  );
}
