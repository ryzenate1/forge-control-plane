"use client";

/**
 * Composite panels shared by the admin Overview and Health experiences.
 *
 * Each of these existed two or three times over — Overview, Monitoring and
 * Health all rendered their own "System Health" card, their own "Recent
 * Activity" list and their own node table, and they disagreed both visually
 * and in what they claimed. These are the canonical implementations; page
 * components compose them rather than re-deriving them.
 */

import React from "react";
import Link from "next/link";
import { Activity, HeartPulse, ServerCog, ShieldAlert } from "lucide-react";

import {
  AdminTBody,
  AdminTable,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
  Btn,
  cn,
} from "@/components/admin/admin-ui";
import {
  DataState,
  PanelCard,
  Reading,
  StatusIcon,
  StatusPill,
  type NamedSource,
} from "@/components/admin/telemetry-ui";
import {
  absoluteTime,
  checkVerdict,
  isAvailable,
  latencyLabel,
  nodeStatus,
  partitionOffline,
  relativeTime,
  remediationFor,
  type SourceState,
  type StatusTone,
} from "@/lib/admin/telemetry";
import type { ApiActivityLog, ApiHealthCheck, ApiHealthReport, ApiNode } from "@/lib/api/types";
import { toneStyles } from "@/components/ui/forge/status";

/* ------------------------------------------------------------------ *
 * Platform verdict
 * ------------------------------------------------------------------ */

export type PlatformVerdict = {
  tone: StatusTone;
  headline: string;
  detail: string;
  /** Concrete, individually actionable problems. Empty when there are none. */
  issues: string[];
  /** Sources we could not read, and therefore cannot vouch for. */
  blindSpots: string[];
};

/**
 * The single platform-status judgement used by every page in this area.
 *
 * The important property is that it refuses to claim health it cannot see.
 * "All systems operational" requires *both* a readable /health report with no
 * failing checks *and* a readable node inventory with nothing unexpectedly
 * offline. If either source is loading, restricted or erroring, the verdict is
 * "incomplete", not "operational" — an unreadable source is a blind spot, and
 * a blind spot is not good news.
 *
 * It also refuses the opposite error: absence of telemetry is not degradation.
 * Nodes that have never reported metrics do not make the platform degraded;
 * only observed failures and observed-offline nodes do.
 */
export function derivePlatformVerdict(input: {
  healthState: SourceState;
  report: ApiHealthReport | undefined;
  nodesState: SourceState;
  nodes: ApiNode[] | undefined;
}): PlatformVerdict {
  const { healthState, report, nodesState, nodes } = input;
  const issues: string[] = [];
  const blindSpots: string[] = [];

  const healthReadable = healthState.status === "ready" && report !== undefined;
  const nodesReadable = nodesState.status === "ready" && nodes !== undefined;

  if (!healthReadable) {
    blindSpots.push(
      healthState.status === "restricted"
        ? "Control-plane diagnostics are not visible to your account"
        : healthState.status === "error"
          ? `Control-plane diagnostics unavailable: ${healthState.message ?? "request failed"}`
          : "Control-plane diagnostics are still loading",
    );
  }
  if (!nodesReadable) {
    blindSpots.push(
      nodesState.status === "restricted"
        ? "Node inventory is not visible to your account"
        : nodesState.status === "error"
          ? `Node inventory unavailable: ${nodesState.message ?? "request failed"}`
          : "Node inventory is still loading",
    );
  }

  const failedChecks = healthReadable ? report!.checks.filter((check) => check.status === "failed") : [];
  const warningChecks = healthReadable ? report!.checks.filter((check) => check.status === "warning") : [];

  for (const check of failedChecks) {
    issues.push(`${check.label || check.name} failed${check.critical ? " (critical)" : ""}`);
  }
  for (const check of warningChecks) {
    issues.push(`${check.label || check.name} reported a warning`);
  }

  let unexpectedOffline: ApiNode[] = [];
  let degradedNodes: ApiNode[] = [];
  if (nodesReadable) {
    unexpectedOffline = partitionOffline(nodes!).unexpected;
    degradedNodes = nodes!.filter((node) => {
      const observed = (node.actualState ?? node.status ?? "").toLowerCase();
      return observed === "degraded" && !node.maintenanceMode && !node.draining;
    });
    if (unexpectedOffline.length > 0) {
      issues.push(
        `${unexpectedOffline.length} ${unexpectedOffline.length === 1 ? "node is" : "nodes are"} offline unexpectedly`,
      );
    }
    for (const node of degradedNodes) {
      issues.push(`Node ${node.name} is degraded`);
    }
  }

  const hasFailure = failedChecks.length > 0 || unexpectedOffline.length > 0;
  const hasWarning = warningChecks.length > 0 || degradedNodes.length > 0;

  if (hasFailure) {
    return {
      tone: "danger",
      headline: "Issues detected",
      detail: `${issues.length} ${issues.length === 1 ? "problem" : "problems"} need attention.`,
      issues,
      blindSpots,
    };
  }
  if (hasWarning) {
    return {
      tone: "warn",
      headline: "Degraded",
      detail: "The platform is serving, but some subsystems are not fully healthy.",
      issues,
      blindSpots,
    };
  }
  if (blindSpots.length > 0) {
    const loadingOnly = healthState.status === "loading" || nodesState.status === "loading";
    return {
      // Not `neutral`: a source we could not read is not a quiet, healthy
      // system. `unknown` draws the dashed edge that says so.
      tone: "unknown",
      headline: loadingOnly ? "Reading platform status" : "Platform status incomplete",
      detail: loadingOnly
        ? "Waiting for the control plane to respond."
        : "No failures were observed, but not every source could be read — this is not a clean bill of health.",
      issues,
      blindSpots,
    };
  }
  return {
    tone: "ok",
    headline: "All systems operational",
    detail: `${report!.checks.length} control-plane ${report!.checks.length === 1 ? "check" : "checks"} passing across ${nodes!.length} ${nodes!.length === 1 ? "node" : "nodes"}.`,
    issues,
    blindSpots,
  };
}

/** Banner surface for a tone: the shared border and background wash. */
function bannerSurface(tone: StatusTone): string {
  return cn(toneStyles[tone].border, toneStyles[tone].bg);
}

/**
 * The page-level status banner. Issues and blind spots are listed, not
 * summarised into a single reassuring word.
 */
export function PlatformStatusBanner({
  verdict,
  action,
  className,
}: {
  verdict: PlatformVerdict;
  action?: React.ReactNode;
  className?: string;
}) {
  return (
    <section
      aria-label="Platform status"
      className={cn("rounded-xl border p-4 sm:p-5", bannerSurface(verdict.tone), className)}
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <span className="mt-0.5">
            <StatusIcon size={20} tone={verdict.tone} />
          </span>
          <div className="min-w-0">
            <h2 className="text-base font-semibold text-slate-100">{verdict.headline}</h2>
            <p className="mt-0.5 text-sm leading-6 text-slate-400">{verdict.detail}</p>
          </div>
        </div>
        {action ? <div className="shrink-0">{action}</div> : null}
      </div>

      {verdict.issues.length > 0 ? (
        <ul className="mt-3 space-y-1 border-t border-white/[0.07] pt-3">
          {verdict.issues.map((issue) => (
            <li className="flex items-start gap-2 text-sm text-slate-300" key={issue}>
              <ShieldAlert size={13} className="mt-1 shrink-0 text-slate-500" aria-hidden="true" />
              {issue}
            </li>
          ))}
        </ul>
      ) : null}

      {verdict.blindSpots.length > 0 ? (
        <ul className="mt-3 space-y-1 border-t border-white/[0.07] pt-3">
          {verdict.blindSpots.map((spot) => (
            <li className="text-xs leading-5 text-amber-300/90" key={spot}>
              Not verified: {spot}
            </li>
          ))}
        </ul>
      ) : null}
    </section>
  );
}

/* ------------------------------------------------------------------ *
 * Control-plane checks
 * ------------------------------------------------------------------ */

/**
 * The control-plane diagnostic report, rendered check by check.
 *
 * Only checks the report actually contains are listed. The former
 * implementation hardcoded rows — a "Queue / Workers: Healthy" row that was a
 * string literal, and a database row that said "Healthy" whenever the report
 * had not loaded — so the panel asserted health for subsystems it had never
 * asked about.
 */
export function ControlPlaneChecksPanel({
  state,
  report,
  onRetry,
  footer,
}: {
  state: SourceState;
  report: ApiHealthReport | undefined;
  onRetry?: () => void;
  footer?: React.ReactNode;
}) {
  const available = state.status === "ready" && report !== undefined;
  const checks = report?.checks ?? [];

  return (
    <PanelCard
      description={
        available
          ? `Reported by GET /health at ${absoluteTime(report!.checkedAt) ?? "an unknown time"}`
          : "GET /health"
      }
      footer={footer}
      icon={HeartPulse}
      title="Control-plane checks"
    >
      <DataState
        emptyMessage="The control plane returned a report with no checks in it. Nothing can be asserted about subsystem health."
        emptyTitle="No checks reported"
        isEmpty={checks.length === 0}
        loadingLabel="Reading control-plane diagnostics…"
        onRetry={onRetry}
        state={state}
      >
        <ul className="divide-y divide-white/[0.05]">
          {checks.map((check) => (
            <CheckRow available={available} check={check} key={check.name} />
          ))}
        </ul>
      </DataState>
    </PanelCard>
  );
}

function CheckRow({ available, check }: { available: boolean; check: ApiHealthCheck }) {
  const verdict = checkVerdict(available, check);
  const remediation = remediationFor(check);
  return (
    <li className="flex flex-wrap items-start justify-between gap-x-4 gap-y-1.5 py-2.5 first:pt-0">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <StatusIcon tone={verdict.tone} />
          <span className="truncate text-sm font-medium text-slate-100">{check.label || check.name}</span>
          {check.critical ? <span className="text-[10px] uppercase tracking-wider text-slate-500">critical</span> : null}
        </div>
        {check.status !== "ok" && check.notificationMessage ? (
          <p className="mt-1 text-xs leading-5 text-slate-400">
            {check.name} — {check.notificationMessage}
          </p>
        ) : null}
        {remediation ? <p className="mt-1 text-xs leading-5 text-slate-500">{remediation}</p> : null}
        {check.consecutiveFailures ? (
          <p className="mt-1 font-mono text-[11px] text-red-300/90">
            {check.consecutiveFailures} consecutive {check.consecutiveFailures === 1 ? "failure" : "failures"}
          </p>
        ) : null}
      </div>
      <div className="flex shrink-0 items-center gap-3">
        <span className="text-right">
          <Reading
            className="text-[11px] text-slate-400"
            reason="Latency not reported for this check"
            value={latencyLabel(check.latencyMs)}
          />
        </span>
        <StatusPill
          label={verdict.label}
          title={check.lastChecked ? `Last checked ${absoluteTime(check.lastChecked)}` : undefined}
          tone={verdict.tone}
        />
      </div>
    </li>
  );
}

/* ------------------------------------------------------------------ *
 * Node inventory summary
 * ------------------------------------------------------------------ */

/**
 * Node population counted by observed state.
 *
 * "Offline (expected)" and "Offline (unexpected)" are separate rows because
 * collapsing them turns routine maintenance into an alarm and hides real
 * outages inside a maintenance window.
 */
export function NodePopulationPanel({
  state,
  nodes,
  onRetry,
}: {
  state: SourceState;
  nodes: ApiNode[] | undefined;
  onRetry?: () => void;
}) {
  const list = nodes ?? [];
  const { expected, unexpected } = partitionOffline(list);
  const unexpectedIds = new Set(unexpected.map((node) => node.id));
  const byLabel = new Map<string, { count: number; tone: StatusTone }>();
  for (const node of list) {
    // Offline splits into two populations, so the unexpected half is labelled
    // here rather than taking nodeStatus's single "Offline". Every other state
    // — including maintenance/draining, which cover the expected half — comes
    // straight from nodeStatus, the one reading of node status.
    const status = unexpectedIds.has(node.id)
      ? { label: "Offline (unexpected)", tone: "danger" as StatusTone }
      : nodeStatus(node);
    const entry = byLabel.get(status.label) ?? { count: 0, tone: status.tone };
    byLabel.set(status.label, { count: entry.count + 1, tone: status.tone });
  }

  return (
    <PanelCard description="GET /nodes" icon={ServerCog} title="Node population">
      <DataState
        emptyMessage="No nodes are registered with this control plane yet."
        emptyTitle="No nodes"
        isEmpty={list.length === 0}
        loadingLabel="Reading node inventory…"
        onRetry={onRetry}
        state={state}
      >
        <ul className="space-y-1.5">
          {[...byLabel.entries()].map(([label, entry]) => (
            <li className="flex items-center justify-between gap-3" key={label}>
              <span className="flex items-center gap-2 text-sm text-slate-300">
                <StatusIcon tone={entry.tone} />
                {label}
              </span>
              <span className="font-mono text-sm text-slate-100">{entry.count}</span>
            </li>
          ))}
        </ul>
        {expected.length > 0 ? (
          <p className="mt-3 border-t border-white/[0.06] pt-2.5 text-xs leading-5 text-slate-500">
            {expected.length} {expected.length === 1 ? "node is" : "nodes are"} down by operator intent
            (maintenance or draining) and are not counted as incidents.
          </p>
        ) : null}
        {unexpected.length > 0 ? (
          <p className="mt-2 text-xs leading-5 text-red-300/90">
            Offline without a maintenance flag: {unexpected.map((node) => node.name).join(", ")}
          </p>
        ) : null}
      </DataState>
    </PanelCard>
  );
}

/* ------------------------------------------------------------------ *
 * Activity feed
 * ------------------------------------------------------------------ */

/**
 * Recent control-plane events.
 *
 * Every column comes from the event: its own `event`/`action`, its own
 * `timestamp`. The previous implementation labelled every row "Deploy" and
 * derived each row's age from its array index ("2m ago", "6m ago", …), then
 * invented four rows when the feed was empty. An empty feed now says so.
 */
export function ActivityFeedPanel({
  state,
  events,
  onRetry,
  title = "Recent activity",
  href = "/admin/activity",
  limit = 8,
}: {
  state: SourceState;
  events: ApiActivityLog[] | undefined;
  onRetry?: () => void;
  title?: string;
  href?: string;
  limit?: number;
}) {
  const rows = (events ?? []).slice(0, limit);
  return (
    <PanelCard
      action={
        <Link
          className="text-[11px] font-medium text-slate-400 transition hover:text-slate-100"
          href={href}
        >
          View all
        </Link>
      }
      description="GET /admin/activity"
      icon={Activity}
      title={title}
    >
      <DataState
        emptyMessage="The control plane has not recorded any activity in this window."
        emptyTitle="No recent activity"
        isEmpty={rows.length === 0}
        loadingLabel="Reading activity log…"
        onRetry={onRetry}
        state={state}
      >
        <AdminTable label="Recent control-plane activity">
          <AdminTHead>
            <AdminTh>Event</AdminTh>
            <AdminTh>Subject</AdminTh>
            <AdminTh>Actor</AdminTh>
            <AdminTh className="text-right">When</AdminTh>
          </AdminTHead>
          <AdminTBody>
            {rows.map((row) => (
              <AdminTr key={row.id}>
                <AdminTd>
                  <span className="font-mono text-[12px] text-slate-200">
                    {row.event || row.action || "—"}
                  </span>
                  {row.description ? (
                    <div className="truncate text-[11px] text-slate-500" title={row.description}>
                      {row.description}
                    </div>
                  ) : null}
                </AdminTd>
                <AdminTd>
                  <Reading
                    className="text-[12px] text-slate-300"
                    reason="No subject recorded for this event"
                    value={
                      row.subjectType
                        ? `${row.subjectType}${row.subjectId ? `/${row.subjectId}` : ""}`
                        : (row.resource ?? undefined)
                    }
                  />
                </AdminTd>
                <AdminTd>
                  <Reading
                    className="text-[12px] text-slate-300"
                    reason="No actor recorded for this event"
                    value={row.actorEmail ?? row.userId ?? undefined}
                  />
                </AdminTd>
                <AdminTd className="text-right" title={absoluteTime(row.timestamp ?? row.createdAt)}>
                  <Reading
                    className="text-[12px] text-slate-400"
                    reason="No timestamp recorded for this event"
                    value={relativeTime(row.timestamp ?? row.createdAt)}
                  />
                </AdminTd>
              </AdminTr>
            ))}
          </AdminTBody>
        </AdminTable>
      </DataState>
    </PanelCard>
  );
}

/* ------------------------------------------------------------------ *
 * Shared header action
 * ------------------------------------------------------------------ */

export function RefreshAction({ onRefresh, busy }: { onRefresh: () => void; busy?: boolean }) {
  return (
    <Btn loading={busy} onClick={onRefresh} size="sm" tone="ghost">
      Refresh
    </Btn>
  );
}

export { isAvailable };
export type { NamedSource };
