"use client";

import { useQuery } from "@tanstack/react-query";
import { Activity, ShieldAlert, Server } from "lucide-react";
import { Card, CardHeader, Pill, AdminLoadingState, AdminErrorState, EmptyState } from "./admin-ui";
import { listUnhealthyTargets, type HealthCheckTarget } from "@/lib/api/health-checks";
import { cn } from "@/lib/utils";

function toneFor(status: string): "green" | "red" | "yellow" | "neutral" {
  switch (status) {
    case "unhealthy":
      return "red";
    case "suspected":
      return "yellow";
    case "healthy":
      return "green";
    default:
      return "neutral";
  }
}

// Surfaces the live health-check runner (probes feeding the reconciler).
// Read-only; the runner itself runs in the background and reacts to failures.
export function LiveHealthChecks() {
  const q = useQuery({
    queryKey: ["health-check-targets"],
    queryFn: listUnhealthyTargets,
    refetchInterval: 15_000,
    retry: false,
  });

  const targets: HealthCheckTarget[] = q.data ?? [];
  const unhealthy = targets.filter((t) => t.status === "unhealthy").length;
  const suspected = targets.filter((t) => t.status === "suspected").length;

  return (
    <Card>
      <CardHeader
        title="Active health checks"
        icon={Activity}
        action={
          !q.isPending && !q.isError ? (
            <div className="flex items-center gap-2">
              {unhealthy > 0 && <Pill tone="red"><ShieldAlert size={12} /> {unhealthy} unhealthy</Pill>}
              {suspected > 0 && <Pill tone="yellow">{suspected} suspected</Pill>}
              {unhealthy === 0 && suspected === 0 && <Pill tone="green">probing · no failures</Pill>}
            </div>
          ) : undefined
        }
      />
      {q.isLoading ? (
        <AdminLoadingState label="Loading health checks…" />
      ) : q.isError ? (
        <div className="p-4">
          <AdminErrorState message={q.error instanceof Error ? q.error.message : "Health checks unavailable"} retry={() => void q.refetch()} />
        </div>
      ) : targets.length === 0 ? (
        <EmptyState icon={Activity} title="No failing checks" message="Every monitored target is reporting healthy." />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                <th className="px-4 py-2.5">Target</th>
                <th className="px-4 py-2.5">Status</th>
                <th className="px-4 py-2.5">Fail / pass streak</th>
                <th className="px-4 py-2.5">Last check</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/[0.04]">
              {targets.map((t) => {
                const tone = toneFor(t.status);
                return (
                  <tr key={t.id} className="hover:bg-white/[0.02]">
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2 text-xs text-slate-200">
                        <Server size={13} className="text-slate-500" />
                        <span className="font-mono">{t.serverId || t.id}</span>
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <Pill tone={tone}>{t.status}</Pill>
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-400">
                      <span className={cn(t.consecutiveFailures > 0 && "text-red-300")}>{t.consecutiveFailures} fail</span>
                      {" / "}
                      <span className="text-emerald-300/80">{t.consecutiveSuccesses} pass</span>
                      <span className="text-slate-600"> · thr {t.unhealthyThreshold}</span>
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-500">{t.lastCheckAt ? new Date(t.lastCheckAt).toLocaleTimeString() : "—"}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}
