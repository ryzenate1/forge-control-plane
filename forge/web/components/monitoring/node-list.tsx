"use client";

import { useQuery } from "@tanstack/react-query";
import { Server } from "lucide-react";
import { Card, CardHeader } from "@/components/admin/admin-ui";
import { getNodeMetrics } from "@/lib/api/monitoring";

function usageBar(pct: number, color: string) {
  return (
    <div className="h-1.5 w-full overflow-hidden rounded-full bg-white/10">
      <div className={`h-full rounded-full ${color}`} style={{ width: `${Math.min(pct, 100)}%` }} />
    </div>
  );
}

function NodeRowSkeleton() {
  return (
    <div className="flex flex-col gap-2 px-4 py-3">
      <div className="flex items-center justify-between">
        <div className="h-4 w-32 animate-pulse rounded bg-white/5" />
        <div className="h-3 w-20 animate-pulse rounded bg-white/5" />
      </div>
      <div className="grid grid-cols-3 gap-4">
        {[1, 2, 3].map((i) => (
          <div key={i}>
            <div className="mb-1 h-3 w-8 animate-pulse rounded bg-white/5" />
            <div className="h-1.5 w-full animate-pulse rounded bg-white/5" />
          </div>
        ))}
      </div>
    </div>
  );
}

export function NodeList({ onNodeSelect }: { onNodeSelect?: (nodeId: string) => void }) {
  const { data: metrics, isLoading, isError } = useQuery({
    queryKey: ["node-metrics-list"],
    queryFn: () => getNodeMetrics(),
    refetchInterval: 15_000,
  });

  if (isLoading) {
    return (
      <Card>
        <CardHeader title="Nodes" icon={Server} />
        <div className="divide-y divide-white/[0.06]">
          {[1, 2, 3].map((i) => (
            <NodeRowSkeleton key={i} />
          ))}
        </div>
      </Card>
    );
  }

  if (isError) {
    return (
      <Card>
        <CardHeader title="Nodes" icon={Server} />
        <div className="p-4 text-sm text-red-400">Failed to load node metrics</div>
      </Card>
    );
  }

  if (!metrics || metrics.length === 0) {
    return (
      <Card>
        <CardHeader title="Nodes" icon={Server} />
        <div className="p-4 text-sm text-slate-500">No nodes registered</div>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader title={`Nodes (${metrics.length})`} icon={Server} />
      <div className="divide-y divide-white/[0.06]">
        {metrics.map((node) => (
          <button
            key={node.nodeId}
            className="flex w-full flex-col gap-2 px-4 py-3 text-left transition hover:bg-white/[0.03]"
            onClick={() => onNodeSelect?.(node.nodeId)}
            type="button"
          >
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium text-slate-200">{node.nodeId}</span>
              <span className="text-xs text-slate-500">
                {node.containerRunning}/{node.containerTotal} containers
              </span>
            </div>
            <div className="grid grid-cols-3 gap-4">
              <div>
                <div className="flex items-center justify-between text-xs text-slate-500">
                  <span>CPU</span>
                  <span>{node.cpuPercent.toFixed(1)}%</span>
                </div>
                {usageBar(node.cpuPercent, "bg-blue-500")}
              </div>
              <div>
                <div className="flex items-center justify-between text-xs text-slate-500">
                  <span>Memory</span>
                  <span>{node.memoryPercent.toFixed(1)}%</span>
                </div>
                {usageBar(node.memoryPercent, "bg-emerald-500")}
              </div>
              <div>
                <div className="flex items-center justify-between text-xs text-slate-500">
                  <span>Disk</span>
                  <span>{node.diskPercent.toFixed(1)}%</span>
                </div>
                {usageBar(node.diskPercent, "bg-amber-500")}
              </div>
            </div>
          </button>
        ))}
      </div>
    </Card>
  );
}
