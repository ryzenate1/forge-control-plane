"use client";

import { useQuery } from "@tanstack/react-query";
import { CheckCircle2, Circle, Clock, LoaderCircle, XCircle } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { Card, CardHeader } from "@/components/admin/admin-ui";
import { fetchDeploymentSteps } from "@/lib/api/deployments";
import { cn } from "@/lib/utils";

interface DeploymentTimelineProps {
  deploymentId: string;
}

const statusConfig: Record<string, { icon: LucideIcon; color: string; bg: string }> = {
  completed: { icon: CheckCircle2, color: "text-emerald-400", bg: "bg-emerald-500/20" },
  in_progress: { icon: LoaderCircle, color: "text-blue-400", bg: "bg-blue-500/20" },
  failed: { icon: XCircle, color: "text-red-400", bg: "bg-red-500/20" },
  cancelled: { icon: XCircle, color: "text-slate-400", bg: "bg-slate-500/20" },
  skipped: { icon: Circle, color: "text-slate-500", bg: "bg-slate-500/10" },
  pending: { icon: Clock, color: "text-slate-500", bg: "bg-slate-500/10" },
};

export function DeploymentTimeline({ deploymentId }: DeploymentTimelineProps) {
  const { data: steps, isLoading, isError } = useQuery({
    queryKey: ["deployment-steps", deploymentId],
    queryFn: () => fetchDeploymentSteps(deploymentId),
    refetchInterval: (query) => {
      const hasActive = query.state.data?.some(
        (s) => s.status === "in_progress" || s.status === "pending",
      );
      return hasActive ? 5000 : false;
    },
  });

  if (isLoading) {
    return (
      <Card>
        <CardHeader title="Deployment Timeline" />
        <div className="space-y-3 p-4">
          {[1, 2, 3, 4, 5].map((i) => (
            <div key={`step-skeleton-${i}`} className="flex items-start gap-3">
              <div className="h-6 w-6 animate-pulse rounded-full bg-white/5" />
              <div className="flex-1 space-y-1">
                <div className="h-4 w-32 animate-pulse rounded bg-white/5" />
                <div className="h-3 w-48 animate-pulse rounded bg-white/5" />
              </div>
            </div>
          ))}
        </div>
      </Card>
    );
  }

  if (isError) {
    return (
      <Card>
        <CardHeader title="Deployment Timeline" />
        <div className="p-4 text-sm text-red-400">Failed to load deployment steps</div>
      </Card>
    );
  }

  if (!steps || steps.length === 0) {
    return (
      <Card>
        <CardHeader title="Deployment Timeline" />
        <div className="p-4 text-sm text-slate-500">No deployment steps available</div>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader title={`Deployment Timeline (${steps.length} steps)`} />
      <div className="p-4">
        <div className="relative">
          {steps.map((step, index) => {
            const config = statusConfig[step.status] ?? statusConfig.pending;
            const Icon = config.icon;
            const isLast = index === steps.length - 1;

            return (
              <div key={step.id} className="relative flex items-start gap-3 pb-6 last:pb-0">
                {!isLast && (
                  <div className="absolute left-[11px] top-7 bottom-0 w-px bg-white/[0.08]" />
                )}
                <div className={cn("relative z-10 flex h-6 w-6 items-center justify-center rounded-full", config.bg)}>
                  <Icon className={cn("h-3.5 w-3.5", config.color, step.status === "in_progress" && "animate-spin")} />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-slate-200">{step.stepName}</p>
                  <div className="flex items-center gap-2 text-xs text-slate-500">
                    {step.startedAt && (
                      <span>{new Date(step.startedAt).toLocaleTimeString()}</span>
                    )}
                    {step.completedAt && (
                      <span>→ {new Date(step.completedAt).toLocaleTimeString()}</span>
                    )}
                  </div>
                  {step.error && (
                    <p className="mt-1 text-xs text-red-400">{step.error}</p>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </Card>
  );
}
