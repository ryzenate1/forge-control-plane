"use client";

/**
 * Charts.
 *
 * Hand-rolled SVG, no chart library, because every chart in Forge is one of
 * three shapes: a sparkline, a small area series, or a bar ramp.
 *
 * The rule that matters here: a missing sample is a **gap**, not a zero. Every
 * series type below accepts `null` in its data and breaks the line rather than
 * drawing a value the host never reported.
 */

import * as React from "react";
import { cn } from "@/lib/utils";
import { chart as chartTokens } from "@/lib/design-tokens";
import { type ForgeTone, toneStyles } from "./status";

/** A single sample. `null` means "not reported" and renders as a gap. */
export type ForgePoint = number | null | undefined;

const seriesColor: Record<ForgeTone, string> = {
  ok: chartTokens.success,
  warn: chartTokens.warning,
  danger: chartTokens.critical,
  info: chartTokens.sky,
  pending: chartTokens.sky,
  neutral: chartTokens.axisStrong,
  unknown: chartTokens.unknown,
};

/** Resolves a plot colour for a series. Tones map to the status ramp. */
export function forgeSeriesColor(tone: ForgeTone = "info"): string {
  return seriesColor[tone];
}

function finiteValues(points: readonly ForgePoint[]): number[] {
  return points.filter((point): point is number => typeof point === "number" && Number.isFinite(point));
}

/**
 * Splits a series into runs of consecutive reported samples so each run can be
 * drawn as its own path and the gaps stay visibly empty.
 */
function runs(points: readonly ForgePoint[]): { index: number; value: number }[][] {
  const out: { index: number; value: number }[][] = [];
  let current: { index: number; value: number }[] = [];
  points.forEach((point, index) => {
    if (typeof point === "number" && Number.isFinite(point)) {
      current.push({ index, value: point });
    } else if (current.length > 0) {
      out.push(current);
      current = [];
    }
  });
  if (current.length > 0) out.push(current);
  return out;
}

/* -------------------------------------------------------------------------- */
/* Chart frame                                                                */
/* -------------------------------------------------------------------------- */

/**
 * Titled container for a plot, with slots for a legend and a range picker.
 * Use it so every chart on a page has the same header rhythm.
 */
export function ForgeChartFrame({
  title,
  description,
  actions,
  legend,
  /** Rendered instead of the plot when there is nothing to show. */
  empty,
  height = 180,
  className,
  bodyClassName,
  children,
}: {
  title?: React.ReactNode;
  description?: React.ReactNode;
  actions?: React.ReactNode;
  legend?: React.ReactNode;
  empty?: React.ReactNode;
  height?: number | string;
  className?: string;
  bodyClassName?: string;
  children?: React.ReactNode;
}) {
  const hasHeader = Boolean(title || description || actions);
  return (
    <div className={cn("ui-surface overflow-hidden shadow-card", className)}>
      {hasHeader ? (
        <div className="ui-card-header">
          <div className="min-w-0 space-y-0.5">
            {title ? <h3 className="t-section">{title}</h3> : null}
            {description ? <p className="text-meta text-text-subtle">{description}</p> : null}
          </div>
          {actions ? <div className="flex shrink-0 items-center gap-1.5">{actions}</div> : null}
        </div>
      ) : null}
      <div className={cn("p-3.5", bodyClassName)}>
        {empty ? (
          <div className="flex items-center justify-center" style={{ height }}>
            {empty}
          </div>
        ) : (
          <div style={{ height }}>{children}</div>
        )}
        {legend ? <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1.5">{legend}</div> : null}
      </div>
    </div>
  );
}

export function ForgeChartLegendItem({
  color,
  label,
  value,
}: {
  color: string;
  label: React.ReactNode;
  /** Current or latest reading. Omit when unknown — do not pass 0. */
  value?: React.ReactNode;
}) {
  return (
    <span className="inline-flex items-center gap-1.5 text-meta text-text-subtle">
      <span
        aria-hidden="true"
        className="size-2 shrink-0 rounded-sm"
        style={{ backgroundColor: color }}
      />
      {label}
      {value !== undefined && value !== null ? (
        <span className="font-mono tabular-nums text-text">{value}</span>
      ) : null}
    </span>
  );
}

/* -------------------------------------------------------------------------- */
/* Sparkline                                                                  */
/* -------------------------------------------------------------------------- */

export type ForgeSparklineProps = {
  points: readonly ForgePoint[];
  tone?: ForgeTone;
  /** Overrides the tone colour. Pass a token from `lib/design-tokens.chart`. */
  color?: string;
  /** Fills the area under the line. */
  area?: boolean;
  /** Fixed scale. Defaults to the observed min/max of the series. */
  min?: number;
  max?: number;
  height?: number;
  strokeWidth?: number;
  className?: string;
  /** Accessible summary. Defaults to a sample count and range. */
  label?: string;
};

/**
 * Compact trend line. Scales to its container width via `viewBox`, so the same
 * component works in a metric tile and in a table cell.
 */
export function ForgeSparkline({
  points,
  tone = "info",
  color,
  area = false,
  min,
  max,
  height = 32,
  strokeWidth = 1.5,
  className,
  label,
}: ForgeSparklineProps) {
  const stroke = color ?? forgeSeriesColor(tone);
  const values = finiteValues(points);
  const gradientId = React.useId();

  if (values.length === 0) {
    return (
      <div
        aria-label="No data reported"
        className={cn(
          "flex items-center justify-center rounded-sm border border-dashed border-line",
          className
        )}
        role="img"
        style={{ height }}
      >
        <span className="font-mono text-eyebrow text-text-muted">NO DATA</span>
      </div>
    );
  }

  const lo = min ?? Math.min(...values);
  const hi = max ?? Math.max(...values);
  const span = hi - lo || 1;
  const width = 100;
  const denominator = Math.max(points.length - 1, 1);

  const x = (index: number) => (index / denominator) * width;
  const y = (value: number) => height - ((value - lo) / span) * (height - strokeWidth) - strokeWidth / 2;

  const segments = runs(points);

  return (
    <svg
      aria-label={label ?? `Trend across ${values.length} samples`}
      className={cn("block w-full overflow-visible", className)}
      height={height}
      preserveAspectRatio="none"
      role="img"
      viewBox={`0 0 ${width} ${height}`}
    >
      {area ? (
        <defs>
          <linearGradient id={gradientId} x1="0" x2="0" y1="0" y2="1">
            <stop offset="0%" stopColor={stroke} stopOpacity={0.28} />
            <stop offset="100%" stopColor={stroke} stopOpacity={0} />
          </linearGradient>
        </defs>
      ) : null}
      {segments.map((segment, segmentIndex) => {
        const line = segment
          .map((point, i) => `${i === 0 ? "M" : "L"}${x(point.index).toFixed(2)},${y(point.value).toFixed(2)}`)
          .join(" ");
        return (
          <React.Fragment key={segmentIndex}>
            {area && segment.length > 1 ? (
              <path
                d={`${line} L${x(segment[segment.length - 1].index).toFixed(2)},${height} L${x(segment[0].index).toFixed(2)},${height} Z`}
                fill={`url(#${gradientId})`}
              />
            ) : null}
            {segment.length === 1 ? (
              <circle
                cx={x(segment[0].index)}
                cy={y(segment[0].value)}
                fill={stroke}
                r={strokeWidth}
              />
            ) : (
              <path
                d={line}
                fill="none"
                stroke={stroke}
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={strokeWidth}
                vectorEffect="non-scaling-stroke"
              />
            )}
          </React.Fragment>
        );
      })}
    </svg>
  );
}

/* -------------------------------------------------------------------------- */
/* Bar series                                                                 */
/* -------------------------------------------------------------------------- */

/**
 * Discrete bar ramp — request counts, per-day totals, check results. Missing
 * samples render as a dashed placeholder so a reporting gap cannot be misread
 * as a quiet period.
 */
export function ForgeBarSeries({
  points,
  tone = "info",
  color,
  max,
  height = 48,
  gap = 2,
  className,
  label,
}: {
  points: readonly ForgePoint[];
  tone?: ForgeTone;
  color?: string;
  max?: number;
  height?: number;
  gap?: number;
  className?: string;
  label?: string;
}) {
  const fill = color ?? forgeSeriesColor(tone);
  const values = finiteValues(points);
  const hi = max ?? (values.length > 0 ? Math.max(...values) : 0);

  return (
    <div
      aria-label={label ?? `${values.length} of ${points.length} intervals reported`}
      className={cn("flex w-full items-end", className)}
      role="img"
      style={{ height, gap }}
    >
      {points.map((point, index) => {
        const reported = typeof point === "number" && Number.isFinite(point);
        if (!reported) {
          return (
            <div
              className="min-w-px flex-1 rounded-sm border border-dashed border-line"
              key={index}
              style={{ height: Math.max(height * 0.18, 4) }}
              title="Not reported"
            />
          );
        }
        const ratio = hi > 0 ? point / hi : 0;
        return (
          <div
            className="min-w-px flex-1 rounded-sm"
            key={index}
            style={{
              backgroundColor: fill,
              height: Math.max(ratio * height, 2),
              opacity: 0.55 + ratio * 0.45,
            }}
            title={String(point)}
          />
        );
      })}
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* Gauge                                                                      */
/* -------------------------------------------------------------------------- */

/**
 * Single-value radial gauge for a utilisation percentage. `value == null`
 * renders an empty dashed track and reads as "Unknown".
 */
export function ForgeGauge({
  value,
  label,
  size = 96,
  tone,
  className,
}: {
  value: number | null | undefined;
  label?: React.ReactNode;
  size?: number;
  /** Overrides the automatic ok/warn/danger threshold tone. */
  tone?: ForgeTone;
  className?: string;
}) {
  const known = typeof value === "number" && Number.isFinite(value);
  const pct = known ? Math.min(Math.max(value, 0), 100) : 0;
  const effectiveTone: ForgeTone = !known
    ? "unknown"
    : (tone ?? (pct >= 90 ? "danger" : pct >= 75 ? "warn" : "ok"));
  const stroke = 7;
  const radius = (size - stroke) / 2;
  const circumference = 2 * Math.PI * radius;

  return (
    <div className={cn("inline-flex flex-col items-center gap-1.5", className)}>
      <div className="relative" style={{ width: size, height: size }}>
        <svg
          aria-label={known ? `${pct.toFixed(0)} percent` : "Unknown"}
          height={size}
          role="img"
          viewBox={`0 0 ${size} ${size}`}
          width={size}
        >
          <circle
            cx={size / 2}
            cy={size / 2}
            fill="none"
            r={radius}
            stroke={chartTokens.gridSlate}
            strokeDasharray={known ? undefined : "3 4"}
            strokeWidth={stroke}
          />
          {known ? (
            <circle
              cx={size / 2}
              cy={size / 2}
              fill="none"
              r={radius}
              stroke={forgeSeriesColor(effectiveTone)}
              strokeDasharray={`${(pct / 100) * circumference} ${circumference}`}
              strokeLinecap="round"
              strokeWidth={stroke}
              transform={`rotate(-90 ${size / 2} ${size / 2})`}
            />
          ) : null}
        </svg>
        <div className="absolute inset-0 flex flex-col items-center justify-center">
          <span
            className={cn(
              "font-mono text-lg font-bold leading-none tabular-nums",
              toneStyles[effectiveTone].fg
            )}
          >
            {known ? `${pct.toFixed(0)}%` : "—"}
          </span>
        </div>
      </div>
      {label ? <p className="t-eyebrow">{label}</p> : null}
    </div>
  );
}
