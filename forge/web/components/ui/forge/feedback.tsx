"use client";

/**
 * Badges, statuses, alerts, and the empty/loading/error triad.
 *
 * Every state a surface can be in is represented here, so a page never has to
 * invent its own "no data" or "failed to load" treatment. Loading states are
 * skeletons shaped like the content they replace, not centred spinners, so the
 * layout does not jump when data arrives.
 */

import * as React from "react";
import {
  AlertTriangle,
  CheckCircle2,
  CircleHelp,
  Info,
  LoaderCircle,
  OctagonAlert,
  RefreshCw,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { type ForgeTone, toneStyles } from "./status";

/* -------------------------------------------------------------------------- */
/* Badge                                                                      */
/* -------------------------------------------------------------------------- */

export function ForgeBadge({
  tone = "neutral",
  children,
  icon,
  className,
  ...rest
}: React.HTMLAttributes<HTMLSpanElement> & { tone?: ForgeTone; icon?: React.ReactNode }) {
  return (
    <span className={cn("ui-badge", toneStyles[tone].chip, className)} {...rest}>
      {icon}
      {children}
    </span>
  );
}

/** Brand-tinted badge for counts and plan tiers. Not a status. */
export function ForgeCountBadge({
  children,
  className,
  ...rest
}: React.HTMLAttributes<HTMLSpanElement>) {
  return (
    <span className={cn("ui-badge ui-badge-brand", className)} {...rest}>
      {children}
    </span>
  );
}

/* -------------------------------------------------------------------------- */
/* Status                                                                     */
/* -------------------------------------------------------------------------- */

export function ForgeStatusDot({
  tone = "unknown",
  pulse = false,
  className,
}: {
  tone?: ForgeTone;
  /** Only for states that are genuinely live and changing. */
  pulse?: boolean;
  className?: string;
}) {
  if (pulse) {
    return (
      <span className={cn("ui-pulse", toneStyles[tone].fg, className)} role="presentation" />
    );
  }
  return (
    <span
      className={cn("size-1.5 shrink-0 rounded-full", toneStyles[tone].dot, className)}
      role="presentation"
    />
  );
}

export function ForgeStatusBadge({
  tone = "unknown",
  label,
  pulse = false,
  className,
}: {
  tone?: ForgeTone;
  /** Defaults to the tone's own wording so a bare tone still reads correctly. */
  label?: React.ReactNode;
  pulse?: boolean;
  className?: string;
}) {
  return (
    <span className={cn("ui-status-pill", toneStyles[tone].chip, className)}>
      <ForgeStatusDot pulse={pulse} tone={tone} />
      {label ?? toneStyles[tone].label}
    </span>
  );
}

/**
 * Desired-vs-actual state. Renders the drift explicitly rather than showing
 * only the desired state, because a server that *should* be running is not the
 * same as one that *is*.
 */
export function ForgeStatus({
  actual,
  desired,
  tone,
  observedLabel,
  className,
}: {
  actual?: string | null;
  desired?: string | null;
  tone?: ForgeTone;
  /** e.g. "12s ago". Shown dimmed after the state. */
  observedLabel?: React.ReactNode;
  className?: string;
}) {
  const actualLabel = actual && actual.trim() !== "" ? actual : null;
  const effectiveTone: ForgeTone = tone ?? (actualLabel ? "neutral" : "unknown");
  const drifted = Boolean(desired && actualLabel && desired !== actualLabel);

  return (
    <span className={cn("inline-flex items-center gap-1.5", className)}>
      <ForgeStatusBadge
        label={actualLabel ?? "Unknown"}
        tone={drifted ? "warn" : effectiveTone}
      />
      {drifted ? (
        <span className="inline-flex items-center gap-1 text-meta text-text-muted">
          <span aria-label="target state">➔</span>
          <span className="font-mono">{desired}</span>
        </span>
      ) : null}
      {observedLabel ? (
        <span className="text-meta text-text-muted">{observedLabel}</span>
      ) : null}
    </span>
  );
}

/* -------------------------------------------------------------------------- */
/* Alert                                                                      */
/* -------------------------------------------------------------------------- */

const alertIcons: Record<ForgeTone, React.ComponentType<{ className?: string }>> = {
  ok: CheckCircle2,
  warn: AlertTriangle,
  danger: OctagonAlert,
  info: Info,
  pending: LoaderCircle,
  neutral: Info,
  unknown: CircleHelp,
};

const alertClass: Record<ForgeTone, string> = {
  ok: "ui-alert-success",
  warn: "ui-alert-warning",
  danger: "ui-alert-error",
  info: "ui-alert-info",
  pending: "ui-alert-info",
  neutral: "border-line bg-overlay-subtle text-text-subtle",
  unknown: "border-dashed border-unknown-line bg-unknown-subtle text-unknown",
};

export function ForgeAlert({
  tone = "info",
  title,
  children,
  actions,
  icon,
  className,
  ...rest
}: React.HTMLAttributes<HTMLDivElement> & {
  tone?: ForgeTone;
  title?: React.ReactNode;
  actions?: React.ReactNode;
  icon?: React.ReactNode;
}) {
  const Icon = alertIcons[tone];
  return (
    <div
      className={cn("ui-alert", alertClass[tone], className)}
      role={tone === "danger" || tone === "warn" ? "alert" : "status"}
      {...rest}
    >
      <span className="mt-px shrink-0">
        {icon ?? <Icon className={cn("size-4", tone === "pending" && "animate-spin")} />}
      </span>
      <div className="min-w-0 flex-1">
        {title ? <p className="font-semibold">{title}</p> : null}
        {children ? <div className={cn(title && "mt-0.5", "text-text-subtle")}>{children}</div> : null}
      </div>
      {actions ? <div className="shrink-0">{actions}</div> : null}
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* Empty                                                                      */
/* -------------------------------------------------------------------------- */

export function ForgeEmptyState({
  title,
  description,
  icon,
  action,
  className,
}: {
  title: React.ReactNode;
  description?: React.ReactNode;
  icon?: React.ReactNode;
  action?: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("ui-empty", className)}>
      {icon ? <span className="ui-empty-icon mb-3">{icon}</span> : null}
      <p className="t-section">{title}</p>
      {description ? (
        <p className="mt-1 max-w-prose text-meta text-text-subtle">{description}</p>
      ) : null}
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* Loading                                                                    */
/* -------------------------------------------------------------------------- */

export function ForgeSkeleton({ className }: { className?: string }) {
  return <div aria-hidden="true" className={cn("ui-skeleton h-4 w-full", className)} />;
}

export function ForgeSpinner({
  size = "md",
  label = "Loading",
  className,
}: {
  size?: "sm" | "md" | "lg";
  label?: string;
  className?: string;
}) {
  const dimension = { sm: "size-3.5", md: "size-5", lg: "size-7" }[size];
  return (
    <span className="inline-flex items-center justify-center" role="status">
      <LoaderCircle
        aria-hidden="true"
        className={cn("animate-spin text-text-muted", dimension, className)}
      />
      <span className="sr-only">{label}</span>
    </span>
  );
}

/** Row skeletons sized to a table body, so the page does not reflow on load. */
export function ForgeLoadingRows({
  rows = 5,
  columns = 4,
  className,
}: {
  rows?: number;
  columns?: number;
  className?: string;
}) {
  return (
    <div aria-label="Loading" className={cn("divide-y divide-line", className)} role="status">
      {Array.from({ length: rows }).map((_, rowIndex) => (
        <div className="flex items-center gap-4 px-3 py-2.5" key={rowIndex}>
          {Array.from({ length: columns }).map((__, colIndex) => (
            <ForgeSkeleton
              className={cn("h-3", colIndex === 0 ? "w-1/4" : "flex-1")}
              key={colIndex}
            />
          ))}
        </div>
      ))}
    </div>
  );
}

export function ForgeLoadingState({
  label = "Loading",
  rows = 3,
  className,
}: {
  label?: string;
  rows?: number;
  className?: string;
}) {
  return (
    <div aria-busy="true" className={cn("space-y-2.5", className)} role="status">
      <span className="sr-only">{label}</span>
      <ForgeSkeleton className="h-3 w-40" />
      {Array.from({ length: rows }).map((_, index) => (
        <ForgeSkeleton className={index % 2 === 0 ? "h-3 w-full" : "h-3 w-3/4"} key={index} />
      ))}
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* Error                                                                      */
/* -------------------------------------------------------------------------- */

/**
 * Failure surface. The message is always shown — a failed load is reported,
 * never silently rendered as an empty list.
 */
export function ForgeErrorState({
  title = "Couldn't load this",
  message,
  onRetry,
  retryLabel = "Retry",
  className,
}: {
  title?: React.ReactNode;
  message?: React.ReactNode;
  onRetry?: () => void;
  retryLabel?: string;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "flex flex-col items-start gap-2.5 rounded-lg border border-danger-line bg-danger-subtle p-4 sm:flex-row sm:items-center",
        className
      )}
      role="alert"
    >
      <OctagonAlert aria-hidden="true" className="size-4 shrink-0 text-danger" />
      <div className="min-w-0 flex-1">
        <p className="text-xs font-semibold text-danger">{title}</p>
        {message ? (
          <p className="mt-0.5 break-words text-meta text-text-subtle">{message}</p>
        ) : null}
      </div>
      {onRetry ? (
        <button className="ui-button ui-button-secondary h-8 shrink-0" onClick={onRetry} type="button">
          <RefreshCw aria-hidden="true" className="size-3.5" />
          {retryLabel}
        </button>
      ) : null}
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* Progress                                                                   */
/* -------------------------------------------------------------------------- */

/**
 * Determinate bar. Pass `value={null}` when the figure is unknown — the track
 * renders striped and empty rather than pretending the value is 0%.
 */
export function ForgeProgress({
  value,
  max = 100,
  tone = "info",
  label,
  showValue = false,
  className,
}: {
  value: number | null | undefined;
  max?: number;
  tone?: ForgeTone;
  label?: React.ReactNode;
  showValue?: boolean;
  className?: string;
}) {
  const known = typeof value === "number" && Number.isFinite(value);
  const pct = known ? Math.min(100, Math.max(0, (value / max) * 100)) : 0;

  return (
    <div className={cn("space-y-1", className)}>
      {label || showValue ? (
        <div className="flex items-baseline justify-between gap-2">
          {label ? <span className="t-eyebrow">{label}</span> : <span />}
          {showValue ? (
            <span
              className={cn(
                "font-mono text-meta tabular-nums",
                known ? "text-text" : "text-unknown"
              )}
            >
              {known ? `${pct.toFixed(0)}%` : "—"}
            </span>
          ) : null}
        </div>
      ) : null}
      <div
        aria-valuemax={max}
        aria-valuemin={0}
        aria-valuenow={known ? value : undefined}
        aria-valuetext={known ? undefined : "Unknown"}
        className={cn(
          "h-1.5 w-full overflow-hidden rounded-full bg-overlay-strong",
          !known && "border border-dashed border-unknown-line bg-transparent"
        )}
        role="progressbar"
      >
        {known ? (
          <div
            className={cn("h-full rounded-full transition-[width] duration-300", toneStyles[tone].dot)}
            style={{ width: `${pct}%` }}
          />
        ) : null}
      </div>
    </div>
  );
}

/** Progress bar with a resource label and reading. Used for CPU/memory/disk. */
export function ForgeResourceBar({
  label,
  used,
  total,
  tone,
  formatter,
  className,
}: {
  label: React.ReactNode;
  used: number | null | undefined;
  total: number | null | undefined;
  tone?: ForgeTone;
  formatter?: (value: number) => string;
  className?: string;
}) {
  const known =
    typeof used === "number" &&
    Number.isFinite(used) &&
    typeof total === "number" &&
    Number.isFinite(total) &&
    total > 0;
  const pct = known ? (used / total) * 100 : null;
  const autoTone: ForgeTone = !known
    ? "unknown"
    : pct! >= 90
      ? "danger"
      : pct! >= 75
        ? "warn"
        : "ok";
  const format = formatter ?? ((value: number) => String(value));

  return (
    <div className={cn("space-y-1", className)}>
      <div className="flex items-baseline justify-between gap-2">
        <span className="t-eyebrow">{label}</span>
        <span
          className={cn("font-mono text-meta tabular-nums", known ? "text-text" : "text-unknown")}
        >
          {known ? `${format(used)} / ${format(total)}` : "—"}
        </span>
      </div>
      <ForgeProgress tone={tone ?? autoTone} value={pct} />
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* Tooltip                                                                    */
/* -------------------------------------------------------------------------- */

/**
 * CSS-only tooltip — hover and keyboard focus, no portal, no positioning
 * library. Content is short by contract; anything longer belongs in a
 * disclosure or a dialog.
 */
export function ForgeTooltip({
  content,
  side = "top",
  children,
  className,
}: {
  content: React.ReactNode;
  side?: "top" | "bottom" | "left" | "right";
  children: React.ReactNode;
  className?: string;
}) {
  const position = {
    top: "bottom-full left-1/2 mb-1.5 -translate-x-1/2",
    bottom: "top-full left-1/2 mt-1.5 -translate-x-1/2",
    left: "right-full top-1/2 mr-1.5 -translate-y-1/2",
    right: "left-full top-1/2 ml-1.5 -translate-y-1/2",
  }[side];

  return (
    <span className={cn("group/tooltip relative inline-flex", className)} tabIndex={-1}>
      {children}
      <span
        className={cn(
          "pointer-events-none absolute z-50 hidden w-max max-w-64 rounded-md border border-line",
          "bg-surface-raised px-2 py-1 text-meta text-text shadow-popover",
          "group-hover/tooltip:block group-focus-within/tooltip:block",
          position
        )}
        role="tooltip"
      >
        {content}
      </span>
    </span>
  );
}
