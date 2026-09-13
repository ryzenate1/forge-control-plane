"use client";

/**
 * Cards, panels and metric tiles.
 *
 * Surface hierarchy is strictly four levels — canvas, surface, raised, well —
 * and a hairline border does the separating. There is no drop shadow on a card
 * and no nesting a card inside a card: a panel inside a panel uses `ForgeSubPanel`,
 * which steps the background rather than adding another frame.
 */

import * as React from "react";
import { cn } from "@/lib/utils";
import { type ForgeTone, toneStyles } from "./status";

/* -------------------------------------------------------------------------- */
/* Card / Panel                                                               */
/* -------------------------------------------------------------------------- */

export type ForgeCardProps = {
  title?: React.ReactNode;
  description?: React.ReactNode;
  eyebrow?: React.ReactNode;
  icon?: React.ReactNode;
  /** Header-right slot: filters, links, overflow menus. */
  actions?: React.ReactNode;
  footer?: React.ReactNode;
  /** Set false when the body is a table or list that should sit flush. */
  padded?: boolean;
  /** Left accent rail. Use sparingly — one per screen at most. */
  tone?: ForgeTone;
  className?: string;
  bodyClassName?: string;
  children?: React.ReactNode;
  id?: string;
};

export function ForgeCard({
  title,
  description,
  eyebrow,
  icon,
  actions,
  footer,
  padded = true,
  tone,
  className,
  bodyClassName,
  children,
  id,
}: ForgeCardProps) {
  const hasHeader = Boolean(title || description || eyebrow || actions || icon);
  return (
    <section
      className={cn(
        "ui-surface overflow-hidden shadow-card",
        tone && cn("border-l-[3px]", toneStyles[tone].border),
        className
      )}
      id={id}
    >
      {hasHeader ? (
        <div className="ui-card-header">
          <div className="flex min-w-0 items-start gap-2.5">
            {icon ? <span className="mt-px shrink-0 text-text-subtle">{icon}</span> : null}
            <div className="min-w-0 space-y-0.5">
              {eyebrow ? <p className="t-eyebrow">{eyebrow}</p> : null}
              {title ? <h3 className="t-section">{title}</h3> : null}
              {description ? (
                <p className="max-w-prose text-meta text-text-subtle">{description}</p>
              ) : null}
            </div>
          </div>
          {actions ? (
            <div className="flex shrink-0 items-center gap-1.5">{actions}</div>
          ) : null}
        </div>
      ) : null}
      {children !== undefined && children !== null ? (
        <div className={cn(padded && "p-4 sm:p-5", bodyClassName)}>{children}</div>
      ) : null}
      {footer ? (
        <div className="flex items-center justify-between gap-3 border-t border-line bg-overlay-subtle px-4 py-2.5 sm:px-5">
          {footer}
        </div>
      ) : null}
    </section>
  );
}

/** Alias. `Panel` reads better for a persistent region, `Card` for a tile. */
export const ForgePanel = ForgeCard;

/**
 * A nested region inside a card. Steps the background instead of drawing a
 * second frame, so nested content never looks like a card in a card.
 */
export function ForgeSubPanel({
  title,
  actions,
  className,
  bodyClassName,
  children,
}: {
  title?: React.ReactNode;
  actions?: React.ReactNode;
  className?: string;
  bodyClassName?: string;
  children?: React.ReactNode;
}) {
  return (
    <div className={cn("rounded-lg border border-line bg-overlay-subtle", className)}>
      {title || actions ? (
        <div className="flex items-center justify-between gap-3 border-b border-line px-3 py-2">
          {title ? <p className="t-eyebrow">{title}</p> : <span />}
          {actions ? <div className="flex items-center gap-1.5">{actions}</div> : null}
        </div>
      ) : null}
      <div className={cn("p-3", bodyClassName)}>{children}</div>
    </div>
  );
}

/** Inset well — logs, diffs, read-only payloads. Darkest surface level. */
export function ForgeWell({
  className,
  children,
  ...rest
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={cn("ui-well p-3", className)} {...rest}>
      {children}
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* Metric                                                                     */
/* -------------------------------------------------------------------------- */

export type ForgeMetricProps = {
  label: React.ReactNode;
  /**
   * The reading. `null`/`undefined` renders an explicit unknown dash in the
   * unknown tone — it is never coerced to 0, because a value we do not have is
   * not a value of zero.
   */
  value: React.ReactNode;
  unit?: React.ReactNode;
  /** Secondary line: context, comparison, or last-observed time. */
  sub?: React.ReactNode;
  tone?: ForgeTone;
  icon?: React.ReactNode;
  /** Trend or delta chip shown beside the label. */
  delta?: React.ReactNode;
  /** Sparkline or gauge rendered under the readout. */
  chart?: React.ReactNode;
  className?: string;
};

function isMissing(value: React.ReactNode): boolean {
  return value === null || value === undefined || value === "";
}

export function ForgeMetric({
  label,
  value,
  unit,
  sub,
  tone,
  icon,
  delta,
  chart,
  className,
}: ForgeMetricProps) {
  const missing = isMissing(value);
  const effectiveTone: ForgeTone = missing ? "unknown" : (tone ?? "neutral");
  const valueTone = missing || effectiveTone === "neutral" ? undefined : toneStyles[effectiveTone].fg;

  return (
    <div className={cn("ui-surface p-3.5 shadow-card", className)}>
      <div className="flex items-start justify-between gap-2">
        <p className="t-eyebrow min-w-0 truncate">{label}</p>
        <div className="flex shrink-0 items-center gap-1.5">
          {delta}
          {icon ? <span className="text-text-muted">{icon}</span> : null}
        </div>
      </div>
      <p className={cn("t-readout mt-2", missing ? "text-unknown" : "text-text", valueTone)}>
        {missing ? (
          <span aria-label="No reading available" title="No reading available">
            —
          </span>
        ) : (
          value
        )}
        {!missing && unit ? (
          <span className="ml-1 font-sans text-meta font-semibold text-text-muted">{unit}</span>
        ) : null}
      </p>
      {sub ? <p className="mt-1 text-meta text-text-subtle">{sub}</p> : null}
      {chart ? <div className="mt-2.5">{chart}</div> : null}
    </div>
  );
}

/** Alias used by dashboards that read "stat" rather than "metric". */
export const ForgeStatTile = ForgeMetric;

/* -------------------------------------------------------------------------- */
/* Key / value                                                                */
/* -------------------------------------------------------------------------- */

export type ForgeKeyValueItem = {
  label: React.ReactNode;
  value: React.ReactNode;
  /** Renders the value in mono — ids, paths, versions, hashes. */
  mono?: boolean;
  /** Full-width row inside a two-column list. */
  wide?: boolean;
};

/**
 * Definition list for detail panels. A missing value renders an explicit dash
 * rather than an empty cell, so "not set" is never indistinguishable from a
 * layout bug.
 */
export function ForgeKeyValue({
  items,
  columns = 2,
  className,
}: {
  items: readonly ForgeKeyValueItem[];
  columns?: 1 | 2 | 3;
  className?: string;
}) {
  const colClass = { 1: "", 2: "sm:grid-cols-2", 3: "sm:grid-cols-3" }[columns];
  return (
    <dl className={cn("grid grid-cols-1 gap-x-6 gap-y-3", colClass, className)}>
      {items.map((item, index) => (
        <div className={cn("min-w-0", item.wide && "sm:col-span-full")} key={index}>
          <dt className="t-eyebrow">{item.label}</dt>
          <dd
            className={cn(
              "mt-0.5 break-words text-xs text-text",
              item.mono && "font-mono text-meta"
            )}
          >
            {isMissing(item.value) ? <span className="text-text-muted">—</span> : item.value}
          </dd>
        </div>
      ))}
    </dl>
  );
}
