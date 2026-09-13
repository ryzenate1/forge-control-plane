"use client";

import { CheckCircle2, Clock, XCircle, AlertTriangle } from "lucide-react";
import type { MonitoringHealthRecord } from "@/lib/api/monitoring";
import { healthLevel } from "./health-status-gauge";
import { Card, CardHeader } from "@/components/admin/admin-ui";
import { EmptyState } from "@/components/ui/primitives";
import { cn } from "@/lib/utils";

function relativeTime(iso: string | undefined) {
  if (!iso || !Number.isFinite(Date.parse(iso))) return "Time unreported";
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(iso).getTime()) / 1000));
  if (seconds < 60) return `${seconds}s ago`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  return `${Math.floor(seconds / 86400)}d ago`;
}

export function HealthSummaryRow({ check }: { check: MonitoringHealthRecord }) {
  const level = healthLevel(check);
  const message = check.error ?? check.message;
  const Icon = level === "healthy" ? CheckCircle2 : level === "degraded" ? AlertTriangle : level === "critical" ? XCircle : Clock;
  return (
    <div className="flex items-start gap-3 px-4 py-3">
      <Icon
        size={16}
        className={cn(
          "mt-0.5 shrink-0",
          level === "healthy" ? "text-emerald-400" : level === "degraded" ? "text-amber-400" : level === "critical" ? "text-rose-400" : "text-slate-400",
        )}
      />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline justify-between gap-3">
          <p className="truncate text-sm font-medium text-slate-200">
            {check.endpointId ?? check.checkName ?? "Unknown check"}
            {check.version ? <span className="ml-2 text-xs font-normal text-slate-400">v{check.version}</span> : null}
          </p>
          <span className="shrink-0 text-xs text-slate-400">{relativeTime(check.observedAt)}</span>
        </div>
        <p className="mt-0.5 text-xs text-slate-400">
          {level === "healthy"
            ? check.containers != null && check.images != null && check.volumes != null
              ? `${check.containers} containers · ${check.images} images · ${check.volumes} volumes`
              : "Check passed"
            : check.reachable === false ? "Unreachable" : level === "degraded" ? "Degraded" : level === "critical" ? "Check failed" : "Status unreported"}
        </p>
        {message ? <p className={cn("mt-0.5 truncate text-xs", level === "critical" || level === "degraded" ? "text-rose-300/80" : "text-slate-400")}>{message}</p> : null}
      </div>
    </div>
  );
}

export function HealthSummary({ checks }: { checks: MonitoringHealthRecord[] }) {
  return (
    <Card>
      <CardHeader
        title="Endpoint Health Checks"
        icon={Clock}
        action={<span className="text-xs text-slate-400">{checks.length} recorded</span>}
      />
      {checks.length === 0 ? (
        <EmptyState
          icon={<Clock size={20} />}
          title="No health checks yet"
          description="Endpoint health records will appear here once the monitoring loop runs."
        />
      ) : (
        <div className="divide-y divide-white/[0.06]">
          {checks.map((check, index) => (
            <HealthSummaryRow key={check.id ?? `${check.endpointId ?? check.checkName}-${check.observedAt}-${index}`} check={check} />
          ))}
        </div>
      )}
    </Card>
  );
}
