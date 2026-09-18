"use client";

/**
 * Live health-check runner state.
 *
 * Two endpoints, deliberately both read:
 *
 *   - `/target-health/targets` returns only targets that are NOT healthy, so an
 *     empty response on its own cannot tell "every probe is passing" from
 *     "nothing is being probed". The previous version of this panel claimed the
 *     former.
 *   - `/target-health/metrics` carries the denominator (total / healthy /
 *     suspected / unhealthy), which is what makes the empty state honest.
 *
 * The route group is only registered when the runner service exists, so a 404
 * means "not enabled on this control plane" — reported as unavailable, not as
 * an absence of failures.
 */

import { Activity, ShieldAlert, Server } from "lucide-react";

import { Card, CardHeader, Pill } from "./admin-ui";
import { DataState, Reading, StatusPill } from "./telemetry-ui";
import {
  REFRESH,
  absoluteTime,
  relativeTime,
  sourceState,
  useHealthCheckMetricsQuery,
  useHealthCheckTargetsQuery,
} from "@/lib/admin/telemetry";
import type { HealthCheckTarget } from "@/lib/api/health-checks";
import type { ForgeTone } from "@/components/ui/forge/status";
import { cn } from "@/lib/utils";

/** An unrecognised probe status is unknown, not healthy and not merely plain. */
function toneFor(status: string): ForgeTone {
  switch (status) {
    case "unhealthy":
      return "danger";
    case "suspected":
      return "warn";
    case "healthy":
      return "ok";
    case "pending":
      return "pending";
    default:
      return "unknown";
  }
}

export function LiveHealthChecks() {
  const targetsQuery = useHealthCheckTargetsQuery();
  const metricsQuery = useHealthCheckMetricsQuery();

  const targetsState = sourceState(targetsQuery, REFRESH.telemetry);
  const counts = metricsQuery.data;
  const targets: HealthCheckTarget[] = targetsQuery.data ?? [];
  const unhealthy = targets.filter((target) => target.status === "unhealthy").length;
  const suspected = targets.filter((target) => target.status === "suspected").length;

  const monitored = counts?.total;
  const emptyMessage =
    monitored === undefined
      ? "No target is currently failing. The runner did not report how many targets it monitors, so this is not a statement about coverage."
      : monitored === 0
        ? "The runner has no targets registered, so nothing is being probed. This is an absence of coverage, not a clean bill of health."
        : `All ${monitored.toLocaleString()} monitored targets are passing their probes.`;

  return (
    <Card>
      <CardHeader
        action={
          targetsState.status === "ready" ? (
            <div className="flex items-center gap-2">
              {unhealthy > 0 ? (
                <Pill tone="red">
                  <ShieldAlert size={12} /> {unhealthy} unhealthy
                </Pill>
              ) : null}
              {suspected > 0 ? <Pill tone="yellow">{suspected} suspected</Pill> : null}
              {unhealthy === 0 && suspected === 0 ? (
                <Pill tone={monitored ? "green" : "neutral"}>
                  {monitored ? `${monitored.toLocaleString()} targets passing` : "no targets registered"}
                </Pill>
              ) : null}
            </div>
          ) : undefined
        }
        icon={Activity}
        title="Active health checks"
      />
      <div className="px-4 pb-4">
        <DataState
          emptyMessage={emptyMessage}
          emptyTitle={monitored === 0 ? "Nothing is being probed" : "No failing checks"}
          isEmpty={targets.length === 0}
          loadingLabel="Reading health-check runner…"
          onRetry={() => {
            void targetsQuery.refetch();
            void metricsQuery.refetch();
          }}
          state={targetsState}
        >
          <div className="-mx-4 overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-2.5">Target</th>
                  <th className="px-4 py-2.5">Status</th>
                  <th className="px-4 py-2.5">Fail / pass streak</th>
                  <th className="px-4 py-2.5">Last check</th>
                  <th className="px-4 py-2.5">Last success</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {targets.map((target) => (
                  <tr className="hover:bg-white/[0.02]" key={target.id}>
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2 text-xs text-slate-200">
                        <Server className="text-slate-500" size={13} />
                        <span className="font-mono">{target.serverId || target.id}</span>
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <StatusPill label={target.status} tone={toneFor(target.status)} />
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-400">
                      <span className={cn(target.consecutiveFailures > 0 && "text-red-300")}>
                        {target.consecutiveFailures} fail
                      </span>
                      {" / "}
                      <span className="text-emerald-300/80">{target.consecutiveSuccesses} pass</span>
                      <span className="text-slate-600">
                        {" "}
                        · fails at {target.unhealthyThreshold}, recovers at {target.healthyThreshold}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-500" title={absoluteTime(target.lastCheckAt)}>
                      <Reading reason="Never probed" value={relativeTime(target.lastCheckAt)} />
                    </td>
                    <td
                      className="px-4 py-3 text-xs text-slate-500"
                      title={absoluteTime(target.lastSuccessAt ?? undefined)}
                    >
                      <Reading reason="Never succeeded" value={relativeTime(target.lastSuccessAt ?? undefined)} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {monitored !== undefined ? (
            <p className="mt-3 text-[11px] leading-5 text-slate-500">
              {targets.length.toLocaleString()} of {monitored.toLocaleString()} monitored targets are failing. Only
              failing targets are listed; passing ones are counted but not enumerated by the runner.
            </p>
          ) : null}
        </DataState>
      </div>
    </Card>
  );
}
