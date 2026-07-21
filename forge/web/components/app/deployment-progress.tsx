"use client";

import { useEffect, useState, useRef } from "react";
import { Check, X, Loader2, Clock, AlertTriangle } from "lucide-react";
import { cn } from "@/lib/utils";
import { fetchDeploymentSteps } from "@/lib/api/deployments";
import type { DeploymentStep } from "@/lib/api/deployments";

interface DeploymentProgressProps {
  deploymentId: string;
  onComplete?: () => void;
  onError?: (error: string) => void;
}

const stepLabels: Record<string, string> = {
  init: "Initialize",
  drain_old: "Drain Old Instances",
  provision: "Provision Resources",
  health_gate: "Health Check Gate",
  promote: "Promote to Active",
  drain_canary: "Drain Canary",
  complete: "Complete Deployment",
  cleanup: "Cleanup",
  rollback: "Rollback",
  scale_up: "Scale Up",
  scale_down: "Scale Down",
  verify: "Verify",
};

const statusConfig: Record<string, { icon: typeof Loader2; className: string }> = {
  pending: { icon: Clock, className: "text-slate-500" },
  in_progress: { icon: Loader2, className: "text-blue-400" },
  completed: { icon: Check, className: "text-emerald-400" },
  failed: { icon: X, className: "text-red-400" },
  cancelled: { icon: X, className: "text-slate-500" },
  skipped: { icon: AlertTriangle, className: "text-amber-400" },
};

export function DeploymentProgress({ deploymentId, onComplete, onError }: DeploymentProgressProps) {
  const [steps, setSteps] = useState<DeploymentStep[]>([]);
  const [loading, setLoading] = useState(true);
  const [expandedStep, setExpandedStep] = useState<string | null>(null);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const prevStatusRef = useRef<string>("");

  useEffect(() => {
    const load = async () => {
      try {
        const data = await fetchDeploymentSteps(deploymentId);
        setSteps(data);
        setLoading(false);
        const terminal = data.length > 0 && data.every(
          (s) => s.status === "completed" || s.status === "failed" || s.status === "cancelled" || s.status === "skipped",
        );
        if (terminal && intervalRef.current) {
          clearInterval(intervalRef.current);
          intervalRef.current = null;
        }
        const allCompleted = data.length > 0 && data.every((s) => s.status === "completed");
        const anyFailed = data.some((s) => s.status === "failed");
        if (allCompleted && prevStatusRef.current !== "completed") {
          prevStatusRef.current = "completed";
          onComplete?.();
        }
        if (anyFailed && prevStatusRef.current !== "failed") {
          const failedStep = data.find((s) => s.status === "failed");
          prevStatusRef.current = "failed";
          onError?.(failedStep?.error ?? "Deployment step failed");
        }
      } catch {
        setLoading(false);
      }
    };
    load();
    intervalRef.current = setInterval(load, 2000);
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [deploymentId, onComplete, onError]);

  const completed = steps.filter((s) => s.status === "completed").length;
  const total = steps.length;
  const pct = total > 0 ? Math.round((completed / total) * 100) : 0;

  if (loading) {
    return (
      <div className="flex items-center gap-2 py-4 text-sm text-slate-400">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading deployment progress...
      </div>
    );
  }

  if (steps.length === 0) {
    return (
      <div className="py-4 text-sm text-slate-500">No deployment steps available yet.</div>
    );
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-3">
        <div className="flex-1">
          <div className="h-2 w-full overflow-hidden rounded-full bg-white/[0.06]">
            <div
              className={cn(
                "h-full rounded-full transition-all duration-500",
                pct === 100 ? "bg-emerald-500" : "bg-blue-500",
              )}
              style={{ width: `${pct}%` }}
            />
          </div>
        </div>
        <span className="text-xs font-medium text-slate-400">{pct}%</span>
      </div>

      <div className="space-y-1">
        {steps.map((step, idx) => {
          const cfg = statusConfig[step.status] ?? { icon: Clock, className: "text-slate-500" };
          const Icon = cfg.icon;
          const isExpanded = expandedStep === step.id;
          const duration = step.startedAt && step.completedAt
            ? Math.round((new Date(step.completedAt).getTime() - new Date(step.startedAt).getTime()) / 1000)
            : null;

          return (
            <div key={step.id}>
              <button
                type="button"
                onClick={() => setExpandedStep(isExpanded ? null : step.id)}
                className={cn(
                  "flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-sm transition-colors hover:bg-white/[0.03]",
                  step.status === "in_progress" && "bg-blue-500/5",
                  step.status === "failed" && "bg-red-500/5",
                )}
              >
                <span className="flex h-6 w-6 shrink-0 items-center justify-center">
                  <Icon
                    className={cn(
                      "h-4 w-4",
                      cfg.className,
                      step.status === "in_progress" && "animate-spin",
                    )}
                  />
                </span>
                <span className="flex-1 font-medium text-slate-200">
                  {stepLabels[step.stepName] ?? step.stepName}
                </span>
                <span className="text-xs text-slate-500">
                  {idx + 1}/{total}
                </span>
                {duration !== null && (
                  <span className="text-xs text-slate-500">{duration}s</span>
                )}
                <span className={cn("text-xs font-medium", cfg.className)}>
                  {step.status === "in_progress" ? "Running" : step.status === "completed" ? "Done" : step.status}
                </span>
              </button>
              {isExpanded && step.error && (
                <div className="mx-3 mb-2 rounded-lg border border-red-500/20 bg-red-950/10 p-2 text-xs text-red-300">
                  {step.error}
                </div>
              )}
              {isExpanded && step.status === "in_progress" && (
                <div className="mx-3 mb-2 animate-pulse text-xs text-slate-500">
                  Executing {step.stepName}...
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
