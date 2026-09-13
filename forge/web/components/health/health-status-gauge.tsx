"use client";

import type { MonitoringHealthRecord } from "@/lib/api/monitoring";
import { cn } from "@/lib/utils";

export type HealthLevel = "healthy" | "degraded" | "critical" | "unknown";

export function healthLevel(check: MonitoringHealthRecord): HealthLevel {
  if (check.reachable === false) return "critical";
  if (check.status === "healthy" || check.status === "degraded" || check.status === "critical") return check.status;
  return "unknown";
}

export function aggregateHealth(checks: MonitoringHealthRecord[]): {
  level: HealthLevel;
  score: number | null;
  reachable: number;
  total: number;
} {
  if (checks.length === 0) return { level: "unknown", score: null, reachable: 0, total: 0 };
  const reachable = checks.filter((check) => check.reachable === true).length;
  const scores = checks.map((check) => check.healthScore).filter((score): score is number => typeof score === "number" && Number.isFinite(score) && score >= 0 && score <= 100);
  const score = scores.length === checks.length ? scores.reduce((sum, value) => sum + value, 0) / scores.length : null;
  const levels = checks.map(healthLevel);
  const level: HealthLevel = levels.every((value) => value === "healthy") ? "healthy"
    : levels.every((value) => value === "critical") ? "critical"
    : levels.some((value) => value === "critical" || value === "degraded") ? "degraded" : "unknown";
  return { level, score, reachable, total: checks.length };
}

const LEVEL_STYLES: Record<HealthLevel, { stroke: string; label: string; text: string; dot: string }> = {
  healthy: { stroke: "text-emerald-400", label: "Healthy", text: "text-emerald-300", dot: "bg-emerald-400" },
  degraded: { stroke: "text-amber-400", label: "Degraded", text: "text-amber-300", dot: "bg-amber-400" },
  critical: { stroke: "text-rose-400", label: "Critical", text: "text-rose-300", dot: "bg-rose-400" },
  unknown: { stroke: "text-slate-400", label: "No data", text: "text-slate-400", dot: "bg-slate-500" },
};

export function HealthStatusGauge({ checks, className }: { checks: MonitoringHealthRecord[]; className?: string }) {
  const { level, score, reachable, total } = aggregateHealth(checks);
  const styles = LEVEL_STYLES[level];
  const pct = score != null ? Math.max(0, Math.min(100, score)) : 0;
  const radius = 52;
  const circumference = 2 * Math.PI * radius;

  return (
    <div className={cn("flex items-center gap-5 rounded-xl border border-white/[0.07] bg-white/[0.018] p-5", className)}>
      <div className="relative h-32 w-32 shrink-0">
        <svg className="h-32 w-32 -rotate-90" viewBox="0 0 128 128">
          <circle cx="64" cy="64" r={radius} fill="none" strokeWidth="10" className="stroke-white/[0.07]" />
          <circle
            cx="64" cy="64" r={radius} fill="none" strokeWidth="10" strokeLinecap="round"
            className={cn("transition-all duration-700", styles.stroke)}
            strokeDasharray={circumference}
            strokeDashoffset={circumference - (circumference * pct) / 100}
          />
        </svg>
        <div className="absolute inset-0 flex flex-col items-center justify-center">
          <span className={cn("text-2xl font-bold", styles.text)}>{score != null ? `${Math.round(score)}%` : "—"}</span>
          <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-slate-400">Health</span>
        </div>
      </div>
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <span className={cn("h-2 w-2 rounded-full", styles.dot)} />
          <p className="text-sm font-bold text-slate-200">{styles.label}</p>
        </div>
        <p className="mt-1 text-xs text-slate-400">
          {total === 0
            ? "No endpoint health checks recorded yet."
            : checks.every((check) => typeof check.reachable === "boolean")
              ? `${reachable} of ${total} endpoints reachable`
              : `${checks.filter((check) => healthLevel(check) === "healthy").length} of ${total} checks healthy; reachability unreported`}
        </p>
        {level !== "unknown" ? (
          <p className="mt-3 text-xs text-slate-400">
            {score === null ? "Health score not reported. Status reflects recorded checks only." : "Aggregate score across the last recorded health checks."}
          </p>
        ) : null}
      </div>
    </div>
  );
}
