"use client";

/**
 * Presentational half of the admin operational data layer.
 *
 * `lib/admin/telemetry.ts` decides what is known; this file decides how a
 * known / unknown / unavailable / stale / restricted reading looks. Keeping
 * both halves in one place is what makes the Overview, Monitoring and Health
 * pages agree: a "—" means the same thing on all three, an amber pill means
 * the same thing, and a chart with no series looks like no data rather than
 * like a flat line at zero.
 *
 * Rules encoded here:
 *   - `undefined` renders as an em dash with a reason, never as `0`.
 *   - Loading renders as a spinner/skeleton, never as an empty result.
 *   - An error renders as an error with a retry, never as an empty result.
 *   - A 403 renders as "Permission restricted" in amber, distinct from failure.
 *   - A stale reading keeps its value but is dimmed and labelled with its age.
 */

import React from "react";
import {
  Area,
  AreaChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import {
  AlertTriangle,
  CheckCircle2,
  CircleSlash,
  HelpCircle,
  LoaderCircle,
  LockKeyhole,
  MinusCircle,
  RefreshCw,
  XCircle,
} from "lucide-react";

import {
  AdminTBody,
  AdminTable,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
  Btn,
  Card,
  EmptyState,
  Pill,
  cn,
} from "@/components/admin/admin-ui";
import { chart } from "@/lib/design-tokens";
import {
  absoluteTime,
  isPartial,
  hasDrift,
  nodeStatus,
  percentLabel,
  relativeTime,
  type ReportedTotal,
  type SourceState,
  type StatusTone,
} from "@/lib/admin/telemetry";
import type { ApiNode } from "@/lib/api/types";
import type { NodeMetrics } from "@/lib/api/monitoring";
import { toneStyles } from "@/components/ui/forge/status";

/* ------------------------------------------------------------------ *
 * Atoms
 * ------------------------------------------------------------------ */

/**
 * The single rendering of "we do not have this value".
 *
 * `reason` is mandatory. An unexplained dash invites the reader to assume the
 * value is zero; naming why it is missing is the whole point.
 */
export function NotReported({ reason, className }: { reason: string; className?: string }) {
  return (
    <span className={cn("font-mono text-text-subtle", className)} title={reason}>
      —
    </span>
  );
}

/** Value-or-dash. The one helper every cell in this feature area goes through. */
export function Reading({
  value,
  reason = "Not reported",
  className,
}: {
  value: string | number | null | undefined;
  reason?: string;
  className?: string;
}) {
  if (value === null || value === undefined || value === "") {
    return <NotReported reason={reason} className={className} />;
  }
  return <span className={cn("font-mono", className)}>{value}</span>;
}

/**
 * One glyph per tone. `unknown` gets its own — a question mark, not the minus
 * that means "inactive" — so a subsystem we could not read is distinguishable
 * at a glance from one that is deliberately off.
 */
const toneIcon: Record<StatusTone, typeof CheckCircle2> = {
  ok: CheckCircle2,
  warn: AlertTriangle,
  danger: XCircle,
  info: RefreshCw,
  pending: LoaderCircle,
  neutral: MinusCircle,
  unknown: HelpCircle,
};

export function StatusIcon({ tone, size = 14 }: { tone: StatusTone; size?: number }) {
  const Icon = toneIcon[tone];
  return <Icon size={size} className={toneStyles[tone].fg} aria-hidden="true" />;
}

export function StatusPill({
  tone,
  label,
  title,
}: {
  tone: StatusTone;
  label: string;
  title?: string;
}) {
  return (
    <span title={title}>
      <Pill tone={tone}>{label}</Pill>
    </span>
  );
}

/* ------------------------------------------------------------------ *
 * Freshness
 * ------------------------------------------------------------------ */

/**
 * Real freshness, derived from the query's own `dataUpdatedAt`.
 *
 * This replaced the decorative "Live · updated just now" badge that the shared
 * `SectionHeader` used to render unconditionally — that badge claimed liveness
 * for data that might have been fetched minutes ago, or have failed entirely.
 * `SectionHeader` now has no badge of its own: pages pass this component into
 * its `status` slot, so the only freshness claim on screen is one derived from
 * a real `dataUpdatedAt`.
 */
export function FreshnessBadge({ state, className }: { state: SourceState; className?: string }) {
  if (state.status === "loading") {
    return (
      <span className={cn("inline-flex items-center gap-1.5 font-mono text-[11px] text-slate-500", className)}>
        <LoaderCircle size={11} className="animate-spin" aria-hidden="true" />
        Loading
      </span>
    );
  }
  if (state.status === "restricted") {
    return (
      <span className={cn("inline-flex items-center gap-1.5 font-mono text-[11px] text-amber-300", className)}>
        <LockKeyhole size={11} aria-hidden="true" />
        Permission restricted
      </span>
    );
  }
  if (state.status === "error") {
    return (
      <span
        className={cn("inline-flex items-center gap-1.5 font-mono text-[11px] text-red-300", className)}
        title={state.message}
      >
        <XCircle size={11} aria-hidden="true" />
        Unavailable
      </span>
    );
  }

  const age = relativeTime(state.updatedAt);
  const absolute = absoluteTime(state.updatedAt);
  if (state.stale) {
    return (
      <span
        className={cn("inline-flex items-center gap-1.5 font-mono text-[11px] text-amber-300", className)}
        title={absolute ? `Last successful read ${absolute}. Older than the refresh interval.` : undefined}
      >
        <AlertTriangle size={11} aria-hidden="true" />
        Stale · {age ?? "age unknown"}
      </span>
    );
  }
  return (
    <span
      className={cn("inline-flex items-center gap-1.5 font-mono text-[11px] text-slate-500", className)}
      title={absolute ? `Last read ${absolute}` : undefined}
    >
      {state.refreshing ? (
        <LoaderCircle size={11} className="animate-spin" aria-hidden="true" />
      ) : (
        <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" aria-hidden="true" />
      )}
      {state.refreshing ? "Refreshing" : `Read ${age ?? "just now"}`}
    </span>
  );
}

export type NamedSource = { label: string; state: SourceState };

/**
 * Per-source provenance strip. Every panel that mixes sources names them and
 * their individual states, so a reader can tell which half of a panel is real
 * when the other half failed.
 */
export function SourceRibbon({ sources, className }: { sources: NamedSource[]; className?: string }) {
  if (sources.length === 0) return null;
  return (
    <div
      className={cn(
        "flex flex-wrap items-center gap-x-4 gap-y-1.5 rounded-lg border border-white/[0.06] bg-white/[0.012] px-3 py-2",
        className,
      )}
    >
      <span className="text-[11px] font-semibold uppercase tracking-wider text-slate-500">Sources</span>
      {sources.map((source) => (
        <span key={source.label} className="inline-flex items-center gap-1.5">
          <span className="text-[11px] text-slate-400">{source.label}</span>
          <FreshnessBadge state={source.state} />
        </span>
      ))}
    </div>
  );
}

/* ------------------------------------------------------------------ *
 * State gate
 * ------------------------------------------------------------------ */

/**
 * The one place loading / error / restricted / empty are decided.
 *
 * `isEmpty` is evaluated only in the `ready` state, which is what stops an
 * error from being rendered as "nothing here".
 */
export function DataState({
  state,
  isEmpty,
  emptyTitle = "Nothing to show",
  emptyMessage,
  loadingLabel = "Loading…",
  onRetry,
  children,
}: {
  state: SourceState;
  isEmpty?: boolean;
  emptyTitle?: string;
  emptyMessage?: string;
  loadingLabel?: string;
  onRetry?: () => void;
  children: React.ReactNode;
}) {
  if (state.status === "loading") {
    return (
      <div
        className="grid min-h-28 place-items-center rounded-xl border border-dashed border-white/[0.1] bg-black/10 p-6 text-sm text-slate-400"
        role="status"
      >
        <div className="flex flex-col items-center gap-2">
          <LoaderCircle size={18} className="animate-spin text-slate-500" aria-hidden="true" />
          <span>{loadingLabel}</span>
        </div>
      </div>
    );
  }

  if (state.status === "restricted") {
    return (
      <div
        className="flex min-h-28 flex-col items-center justify-center gap-2 rounded-xl border border-amber-500/20 bg-amber-500/[0.05] p-6 text-center"
        role="status"
      >
        <LockKeyhole size={20} className="text-amber-400" strokeWidth={1.5} aria-hidden="true" />
        <p className="text-sm font-medium text-amber-200">Permission restricted</p>
        <p className="max-w-md text-xs text-amber-400/80">
          Your account cannot read this source. The data is not missing — it is not visible to you.
        </p>
      </div>
    );
  }

  if (state.status === "error") {
    return (
      <div
        className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-red-500/25 bg-red-950/20 p-4 text-sm text-red-100"
        role="alert"
      >
        <span className="flex items-center gap-2">
          <XCircle size={15} aria-hidden="true" />
          {state.message ?? "This source is unavailable."}
        </span>
        {onRetry ? (
          <Btn size="sm" tone="ghost" onClick={onRetry}>
            Retry
          </Btn>
        ) : null}
      </div>
    );
  }

  if (isEmpty) {
    return <EmptyState icon={CircleSlash} title={emptyTitle} message={emptyMessage} />;
  }

  return <>{children}</>;
}

/* ------------------------------------------------------------------ *
 * Metrics
 * ------------------------------------------------------------------ */

export type MetricKind = "live" | "configured" | "allocated" | "derived";

const kindCopy: Record<MetricKind, { label: string; hint: string }> = {
  live: { label: "Live", hint: "Reported by the host agent" },
  configured: { label: "Configured", hint: "Operator-configured capacity, not live usage" },
  allocated: { label: "Allocated", hint: "Quota committed to workloads, not live usage" },
  derived: { label: "Derived", hint: "Computed from other readings on this page" },
};

/**
 * A single metric.
 *
 * `kind` is required and rendered, because the most dangerous confusion in this
 * area is reading configured capacity as live utilisation. A tile never carries
 * a trend, delta or sparkline unless real series data is supplied — the former
 * implementation attached static "4% ▾" pills and a fixed decorative wave path
 * to every KPI, which is fabricated telemetry wearing the costume of a chart.
 */
export function MetricTile({
  label,
  value,
  unit,
  kind,
  context,
  state,
  tone,
  missingReason = "Not reported",
  className,
}: {
  label: string;
  value: string | number | null | undefined;
  unit?: string;
  kind: MetricKind;
  context?: string;
  state?: SourceState;
  tone?: StatusTone;
  missingReason?: string;
  className?: string;
}) {
  const meta = kindCopy[kind];
  const missing = value === null || value === undefined || value === "";
  const stale = state?.stale ?? false;
  const loading = state?.status === "loading";

  return (
    <div
      className={cn(
        "rounded-xl border border-white/[0.08] bg-[var(--surface)] p-4",
        stale && "opacity-75",
        className,
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <span className="text-[11px] font-semibold uppercase tracking-wider text-slate-400">{label}</span>
        <span
          className="shrink-0 text-[10px] font-medium uppercase tracking-wider text-slate-500"
          title={meta.hint}
        >
          {meta.label}
        </span>
      </div>

      <div className="mt-2 flex items-baseline gap-1.5">
        {loading ? (
          <span className="inline-block h-7 w-20 animate-pulse rounded bg-white/[0.06]" aria-label="Loading value" />
        ) : missing ? (
          <span className="font-mono text-2xl font-bold tracking-tight text-text-subtle" title={missingReason}>
            —
          </span>
        ) : (
          <>
            <span
              className={cn(
                "font-mono text-2xl font-bold tracking-tight",
                tone ? toneStyles[tone].fg : "text-text",
              )}
            >
              {value}
            </span>
            {unit ? <span className="font-mono text-xs text-slate-500">{unit}</span> : null}
          </>
        )}
      </div>

      <p className="mt-1.5 text-xs leading-5 text-slate-500">
        {loading ? "Reading…" : missing ? missingReason : context}
      </p>

      {stale ? (
        <p className="mt-1 text-[11px] text-amber-300/90">
          Last read {relativeTime(state?.updatedAt) ?? "an unknown time"} ago — older than the refresh interval.
        </p>
      ) : null}
    </div>
  );
}

/**
 * A metric summed from records where some records may not have reported the
 * field. Renders "N of M reported" so a floor is never read as a total.
 */
export function AggregateTile({
  label,
  total,
  format,
  unit,
  kind,
  noun,
  state,
  className,
}: {
  label: string;
  total: ReportedTotal;
  format: (value: number) => string | undefined;
  unit?: string;
  kind: MetricKind;
  noun: string;
  state?: SourceState;
  className?: string;
}) {
  const partial = isPartial(total);
  const context = partial
    ? `${total.reported} of ${total.total} ${noun} reported this field`
    : `${total.total} ${noun}`;
  return (
    <MetricTile
      className={className}
      context={context}
      kind={kind}
      label={label}
      missingReason={
        total.total === 0 ? `No ${noun} to sum` : `No ${noun} reported this field`
      }
      state={state}
      tone={partial ? "warn" : undefined}
      unit={unit}
      value={total.value === undefined ? undefined : format(total.value)}
    />
  );
}

/* ------------------------------------------------------------------ *
 * Panels
 * ------------------------------------------------------------------ */

export function PanelCard({
  title,
  icon: Icon,
  description,
  action,
  footer,
  children,
  className,
}: {
  title: string;
  icon?: React.ElementType;
  description?: string;
  action?: React.ReactNode;
  footer?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <Card className={cn("flex flex-col", className)}>
      <div className="flex min-h-12 items-center gap-2 border-b border-white/[0.07] bg-white/[0.018] -mx-4 sm:-mx-5 -mt-4 sm:-mt-5 mb-4 sm:mb-5 rounded-t-2xl px-4 sm:px-5">
        {Icon ? <Icon size={14} className="text-slate-400" aria-hidden="true" /> : null}
        <div className="min-w-0">
          <h3 className="truncate text-xs font-semibold tracking-wide text-slate-200">{title}</h3>
          {description ? <p className="truncate text-[11px] text-slate-500">{description}</p> : null}
        </div>
        {action ? <div className="ml-auto flex shrink-0 items-center gap-2">{action}</div> : null}
      </div>
      <div className="min-w-0 flex-1">{children}</div>
      {footer ? (
        <div className="-mx-4 -mb-4 mt-4 border-t border-white/[0.06] px-4 py-2.5 text-[11px] leading-5 text-slate-500 sm:-mx-5 sm:-mb-5 sm:px-5">
          {footer}
        </div>
      ) : null}
    </Card>
  );
}

/* ------------------------------------------------------------------ *
 * Node telemetry table — one implementation, used by Overview,
 * Monitoring and Health instead of three divergent copies.
 * ------------------------------------------------------------------ */

export function NodeTelemetryTable({
  nodes,
  latestByNode,
  telemetryState,
  onSelect,
  selectedNodeId,
}: {
  nodes: ApiNode[];
  latestByNode: Map<string, NodeMetrics>;
  telemetryState: SourceState;
  onSelect?: (nodeId: string) => void;
  selectedNodeId?: string | null;
}) {
  const telemetryUnavailable = telemetryState.status !== "ready";
  const telemetryReason =
    telemetryState.status === "restricted"
      ? "Telemetry is not visible to your account"
      : telemetryState.status === "error"
        ? "Telemetry source is unavailable"
        : telemetryState.status === "loading"
          ? "Telemetry is still loading"
          : "This node has not reported telemetry";

  return (
    <AdminTable label="Nodes and their reported telemetry">
      <AdminTHead>
        <AdminTh>Node</AdminTh>
        <AdminTh>Status</AdminTh>
        <AdminTh className="text-right">CPU</AdminTh>
        <AdminTh className="text-right">Memory</AdminTh>
        <AdminTh className="text-right">Disk</AdminTh>
        <AdminTh className="text-right">Containers</AdminTh>
        <AdminTh className="text-right">Observed</AdminTh>
      </AdminTHead>
      <AdminTBody>
        {nodes.map((node) => {
          const verdict = nodeStatus(node);
          const metrics = telemetryUnavailable ? undefined : latestByNode.get(node.id);
          const selected = selectedNodeId === node.id;
          return (
            <AdminTr
              className={cn(selected && "bg-white/[0.03]")}
              key={node.id}
              onClick={onSelect ? () => onSelect(node.id) : undefined}
            >
              <AdminTd>
                <div className="min-w-0">
                  <div className="truncate font-medium text-slate-100">{node.name}</div>
                  {/* No invented address: a node without an fqdn simply does not show one. */}
                  {node.fqdn ? (
                    <div className="truncate font-mono text-[11px] text-slate-500">{node.fqdn}</div>
                  ) : (
                    <div className="font-mono text-[11px] text-text-subtle" title="No FQDN configured">
                      no fqdn configured
                    </div>
                  )}
                </div>
              </AdminTd>
              <AdminTd>
                <span className="inline-flex items-center gap-1.5">
                  <StatusIcon tone={verdict.tone} />
                  <StatusPill
                    label={verdict.label}
                    title={
                      // Desired and observed states are different vocabularies
                      // ("active/maintenance/draining" vs "online/offline/…"),
                      // so drift is decided by hasDrift, not by comparing them.
                      hasDrift(node)
                        ? `Desired ${node.desiredState ?? "unknown"}, observed ${node.actualState ?? "unknown"}`
                        : undefined
                    }
                    tone={verdict.tone}
                  />
                </span>
              </AdminTd>
              <AdminTd className="text-right">
                <Reading reason={telemetryReason} value={percentLabel(metrics?.cpuPercent, 1)} />
              </AdminTd>
              <AdminTd className="text-right">
                <Reading reason={telemetryReason} value={percentLabel(metrics?.memoryPercent, 1)} />
              </AdminTd>
              <AdminTd className="text-right">
                <Reading reason={telemetryReason} value={percentLabel(metrics?.diskPercent, 1)} />
              </AdminTd>
              <AdminTd className="text-right">
                <Reading
                  reason={telemetryReason}
                  value={
                    metrics ? `${metrics.containerRunning} / ${metrics.containerTotal}` : undefined
                  }
                />
              </AdminTd>
              <AdminTd className="text-right">
                <Reading
                  reason={telemetryReason}
                  value={metrics ? relativeTime(metrics.observedAt) : undefined}
                />
              </AdminTd>
            </AdminTr>
          );
        })}
      </AdminTBody>
    </AdminTable>
  );
}

/* ------------------------------------------------------------------ *
 * Metric series chart
 * ------------------------------------------------------------------ */

export type SeriesKey = "cpuPercent" | "memoryPercent" | "diskPercent";

export const seriesMeta: Record<SeriesKey, { label: string; color: string }> = {
  cpuPercent: { label: "CPU", color: chart.cpu },
  memoryPercent: { label: "Memory", color: chart.memory },
  diskPercent: { label: "Disk", color: chart.disk },
};

export type SeriesPoint = { t: number; label: string } & Partial<Record<SeriesKey, number>>;

/**
 * Percentage time series.
 *
 * When there are no points the axes still render but no series is drawn and an
 * overlay explains why. Plotting zeros for missing data would draw a flat
 * healthy-looking line for a fleet that is reporting nothing at all.
 */
export function MetricSeriesChart({
  points,
  series,
  height = 300,
  emptyTitle = "No telemetry reported",
  emptyMessage,
}: {
  points: SeriesPoint[];
  series: SeriesKey[];
  height?: number;
  emptyTitle?: string;
  emptyMessage?: string;
}) {
  const empty = points.length === 0;
  return (
    <div className="relative" style={{ height }}>
      <ResponsiveContainer height="100%" width="100%">
        <AreaChart data={points} margin={{ top: 8, right: 12, bottom: 0, left: -12 }}>
          <defs>
            {series.map((key) => (
              <linearGradient id={`grad-${key}`} key={key} x1="0" x2="0" y1="0" y2="1">
                <stop offset="0%" stopColor={seriesMeta[key].color} stopOpacity={0.28} />
                <stop offset="100%" stopColor={seriesMeta[key].color} stopOpacity={0.02} />
              </linearGradient>
            ))}
          </defs>
          <CartesianGrid stroke={chart.grid} vertical={false} />
          <XAxis
            dataKey="label"
            minTickGap={28}
            stroke={chart.axis}
            tick={{ fontSize: 11 }}
            tickLine={false}
          />
          <YAxis
            domain={[0, 100]}
            stroke={chart.axis}
            tick={{ fontSize: 11 }}
            tickFormatter={(value: number) => `${value}%`}
            tickLine={false}
            width={48}
          />
          <Tooltip
            contentStyle={{
              background: chart.panel,
              border: `1px solid ${chart.panelBorder}`,
              borderRadius: 8,
              fontSize: 12,
            }}
            formatter={(value: number, name: string) => [`${Number(value).toFixed(1)}%`, name]}
          />
          <Legend wrapperStyle={{ fontSize: 11 }} />
          {empty
            ? null
            : series.map((key) => (
                <Area
                  dataKey={key}
                  fill={`url(#grad-${key})`}
                  key={key}
                  name={seriesMeta[key].label}
                  stroke={seriesMeta[key].color}
                  strokeWidth={1.75}
                  type="monotone"
                />
              ))}
        </AreaChart>
      </ResponsiveContainer>

      {empty ? (
        <div className="pointer-events-none absolute inset-0 grid place-items-center">
          <div className="pointer-events-auto max-w-sm rounded-lg border border-dashed border-white/[0.12] bg-[var(--surface)]/90 px-4 py-3 text-center">
            <p className="text-sm font-medium text-slate-200">{emptyTitle}</p>
            {emptyMessage ? <p className="mt-1 text-xs leading-5 text-slate-500">{emptyMessage}</p> : null}
          </div>
        </div>
      ) : null}
    </div>
  );
}

/** Warn when a fleet-wide series was assembled from an incomplete fan-out. */
export function PartialFleetNotice({
  failedNodeIds,
  skippedNodeIds,
  nodeNames,
}: {
  failedNodeIds: string[];
  skippedNodeIds: string[];
  nodeNames: Map<string, string>;
}) {
  if (failedNodeIds.length === 0 && skippedNodeIds.length === 0) return null;
  const name = (id: string) => nodeNames.get(id) ?? id;
  return (
    <div
      className="flex flex-wrap items-start gap-2 rounded-lg border border-amber-500/25 bg-amber-500/[0.06] px-3 py-2 text-xs leading-5 text-amber-200"
      role="status"
    >
      <AlertTriangle size={14} className="mt-0.5 shrink-0" aria-hidden="true" />
      <div className="space-y-0.5">
        {failedNodeIds.length > 0 ? (
          <p>
            This series is incomplete: history could not be read for{" "}
            <span className="font-mono">{failedNodeIds.map(name).join(", ")}</span>.
          </p>
        ) : null}
        {skippedNodeIds.length > 0 ? (
          <p>
            {skippedNodeIds.length} further {skippedNodeIds.length === 1 ? "node was" : "nodes were"} not
            queried — select a node to read its history directly.
          </p>
        ) : null}
      </div>
    </div>
  );
}
