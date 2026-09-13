"use client";

import { Check, X } from "lucide-react";
import type { LifecycleMachine, StepStatus } from "./lifecycle-machines";
import { stepStatuses } from "./lifecycle-machines";
import { useT } from "@/components/TranslationProvider";
import { cn } from "@/lib/utils";

const STEP_DOT: Record<StepStatus, string> = {
  done: "bg-emerald-400 border-emerald-400/40",
  active: "bg-violet-400 border-violet-400/60 animate-pulse",
  pending: "bg-white/[0.07] border-white/10",
  error: "bg-rose-400 border-rose-400/40",
  skipped: "bg-slate-700 border-slate-500/40",
};

const STEP_TEXT: Record<StepStatus, string> = {
  done: "text-emerald-300",
  active: "text-violet-200",
  pending: "text-slate-400",
  error: "text-rose-300",
  skipped: "text-slate-400",
};

const STEP_BAR: Record<StepStatus, string> = {
  done: "bg-emerald-400/50",
  active: "bg-violet-400/40",
  pending: "bg-white/[0.06]",
  error: "bg-rose-400/50",
  skipped: "bg-slate-700/50",
};

function tr(t: (key: string) => string, key: string, fallback: string) {
  const value = t(key);
  return value === key ? fallback : value;
}

export function StateMachine({ machine, state, className }: { machine: LifecycleMachine; state: string; className?: string }) {
  const t = useT();
  const statuses = stepStatuses(machine, state);
  const label = machine.label(state);

  return (
    <div className={cn("space-y-3", className)}>
      <div className="flex items-center gap-2">
        <span className="text-xs font-bold uppercase tracking-[0.12em] text-slate-400">{machine.fallbackName}</span>
        <span
          className={cn(
            "rounded-full px-2 py-0.5 text-[10px] font-bold",
            statuses.includes("error") ? "bg-rose-500/15 text-rose-300"
            : statuses.includes("active") ? "bg-violet-500/15 text-violet-200"
            : statuses.includes("skipped") ? "bg-slate-700/30 text-slate-300 border border-slate-600/30"
            : statuses.every((s) => s === "done") ? "bg-emerald-500/15 text-emerald-300"
            : "bg-white/[0.05] text-slate-300",
          )}
        >
          {label}
        </span>
      </div>
      <ol className="flex items-center" aria-label={`${machine.fallbackName} pipeline`}>
        {machine.steps.map((step, index) => {
          const status = statuses[index];
          return (
            <li key={step.id} className={cn("flex items-center", index < machine.steps.length - 1 && "flex-1")}>
              <div className="flex flex-col items-center gap-1.5">
                <span
                  className={cn(
                    "flex h-5 w-5 items-center justify-center rounded-full border",
                    STEP_DOT[status],
                    status === "done" || status === "error" ? "text-slate-950" : status === "skipped" ? "text-slate-400" : "",
                  )}
                >
                  {status === "done" ? <Check size={11} strokeWidth={3} /> : status === "error" ? <X size={11} strokeWidth={3} /> : null}
                </span>
                <span className={cn("text-[10px] font-medium", STEP_TEXT[status])}>{tr(t, step.labelKey, step.fallback)}</span>
              </div>
              {index < machine.steps.length - 1 ? (
                <span aria-hidden className={cn("mx-2 mb-4 h-0.5 min-w-4 flex-1 rounded-full", STEP_BAR[status])} />
              ) : null}
            </li>
          );
        })}
      </ol>
    </div>
  );
}