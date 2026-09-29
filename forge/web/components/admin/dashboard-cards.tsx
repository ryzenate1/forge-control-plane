"use client";

import { useId, type ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { Pill, cn } from "./admin-ui";
import { toneStyles } from "@/components/ui/forge/status";
import { chart } from "@/lib/design-tokens";

export type PillTone = "neutral" | "green" | "red" | "yellow" | "blue";

/* ------------------------------------------------------------------ */
/* Dashboard header — icon tile, eyebrow, title + status, tags, meta   */
/* ------------------------------------------------------------------ */

export interface DashMetaItem {
  label: string;
  value: ReactNode;
  title?: string;
}

export function DashHeader({ icon: Icon, eyebrow, title, pill, description, tags, meta, actions }: {
  icon: LucideIcon;
  eyebrow: string;
  title: string;
  pill?: { tone: PillTone; label: string };
  description?: string;
  tags?: string[];
  meta?: DashMetaItem[];
  actions?: ReactNode;
}) {
  return (
    <div className="overflow-hidden rounded-2xl border border-line bg-overlay-subtle">
      <div className="flex flex-wrap items-start gap-4 p-5">
        <span aria-hidden="true" className="grid h-14 w-14 shrink-0 place-items-center rounded-xl border border-line bg-overlay-strong text-text-subtle">
          <Icon size={26} />
        </span>
        <div className="min-w-0 flex-1">
          <p className="t-meta font-bold uppercase tracking-[0.14em] text-text-subtle">{eyebrow}</p>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <h2 className="text-xl font-bold tracking-tight text-text">{title}</h2>
            {pill ? <Pill tone={pill.tone}>{pill.label}</Pill> : null}
          </div>
          {description ? <p className="mt-1 max-w-2xl text-xs leading-5 text-text-subtle">{description}</p> : null}
          {tags && tags.length > 0 ? (
            <div className="mt-2 flex flex-wrap gap-1.5">
              {tags.map((t) => (
                <span key={t} className="rounded-md border border-line bg-overlay-subtle px-2 py-0.5 font-mono text-[10px] text-text-subtle" title={t}>{t}</span>
              ))}
            </div>
          ) : null}
          {meta && meta.length > 0 ? (
            <dl className="mt-3 flex flex-wrap gap-x-6 gap-y-1.5 text-[11px]">
              {meta.map((m) => (
                <div key={m.label} className="flex items-center gap-1.5">
                  <dt className="text-text-muted">{m.label}</dt>
                  <dd className="font-semibold text-text" title={m.title}>{m.value}</dd>
                </div>
              ))}
            </dl>
          ) : null}
        </div>
        {actions ? <div className="flex shrink-0 flex-wrap items-center gap-1.5">{actions}</div> : null}
      </div>
    </div>
  );
}

export function DashActionButton({ label, icon: Icon, onClick, disabled, pending, href, tone = "neutral" }: {
  label: string;
  icon: LucideIcon;
  onClick?: () => void;
  disabled?: boolean;
  pending?: boolean;
  href?: string;
  tone?: "neutral" | "brand";
}) {
  const cls = cn(
    "inline-flex items-center gap-1.5 rounded-lg border px-3 py-2 text-[11px] font-bold transition disabled:cursor-not-allowed disabled:opacity-40",
    tone === "brand"
      ? "border-[color-mix(in_srgb,var(--brand)_40%,transparent)] bg-[color-mix(in_srgb,var(--brand)_7%,transparent)] text-text hover:border-[color-mix(in_srgb,var(--brand)_60%,transparent)] hover:bg-[color-mix(in_srgb,var(--brand)_12%,transparent)]"
      : "border-line bg-overlay-subtle text-text hover:bg-[var(--surface-hover)]"
  );
  const inner = <><Icon size={12} />{pending ? "…" : label}</>;
  if (href) return <a className={cls} href={href} target="_blank" rel="noreferrer">{inner}</a>;
  return <button type="button" className={cls} disabled={disabled} onClick={onClick}>{inner}</button>;
}

/* ------------------------------------------------------------------ */
/* KPI cards — icon, colored value, sparkline, capacity bar            */
/* ------------------------------------------------------------------ */

export interface KpiDatum {
  key: string;
  title: string;
  icon: LucideIcon;
  iconClass: string;
  valueClass?: string;
  color: string;
  value: string | null;
  sub: string;
  live?: boolean;
  trend?: number[];
  bar?: number | null;
}

export function Sparkline({ data, color }: { data: number[]; color: string }) {
  const uid = useId().replace(/[^a-zA-Z0-9]/g, "");
  const gid = `dash-${uid}`;
  // A series needs two points before it has a shape. Draw a flat rule rather
  // than a curve: an earlier version emitted a hardcoded rising path here, so
  // every KPI with no history looked like it was climbing.
  if (data.length < 2) {
    return (
      <svg viewBox="0 0 120 34" className="h-full w-full" aria-hidden="true">
        <line x1="0" y1="26" x2="120" y2="26" stroke={color} strokeWidth={1.5} strokeDasharray="4 4" opacity={0.35} />
      </svg>
    );
  }
  const min = Math.min(...data);
  const max = Math.max(...data);
  const range = max - min || 1;
  const width = 120;
  const height = 34;
  const points = data.map((val, idx) => {
    const x = (idx / (data.length - 1)) * width;
    const y = height - ((val - min) / range) * (height - 8) - 4;
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  });
  const pathD = `M ${points.join(" L ")}`;
  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="h-full w-full" aria-hidden="true">
      <defs>
        <linearGradient id={gid} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity={0.35} />
          <stop offset="100%" stopColor={color} stopOpacity={0} />
        </linearGradient>
      </defs>
      <path d={`${pathD} L ${width},${height} L 0,${height} Z`} fill={`url(#${gid})`} />
      <path d={pathD} fill="none" stroke={color} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

export function KpiCard({ kpi }: { kpi: KpiDatum }) {
  return (
    <div className="rounded-xl border border-line bg-[var(--surface)] p-4 shadow-sm">
      <div className="flex items-center gap-2 text-xs font-semibold text-text">
        <kpi.icon size={15} className={kpi.iconClass} />
        <span>{kpi.title}</span>
        {kpi.live ? (
          <span className={cn("ml-auto flex items-center gap-1 font-mono text-[10px] font-normal", toneStyles.ok.fg)}>
            <span className={cn("h-1.5 w-1.5 animate-pulse rounded-full", toneStyles.ok.dot)} />live
          </span>
        ) : null}
      </div>
      <div className="mt-2 flex items-end justify-between gap-2">
        <div className="min-w-0">
          <p className={cn("font-mono text-xl font-bold", kpi.value ? (kpi.valueClass ?? "text-text") : toneStyles.unknown.fg)}>{kpi.value ?? "Not reported"}</p>
          <p className="mt-0.5 max-w-36 truncate text-[11px] text-text-muted" title={kpi.sub}>{kpi.sub}</p>
        </div>
        {kpi.trend ? (
          <div className="h-9 w-24 shrink-0 overflow-hidden sm:w-28">
            <Sparkline data={kpi.trend} color={kpi.color} />
          </div>
        ) : null}
      </div>
      {kpi.bar != null ? (
        <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-overlay-strong">
          <div className="h-full rounded-full transition-all" style={{ width: `${Math.min(100, Math.max(0, kpi.bar))}%`, backgroundColor: kpi.color }} />
        </div>
      ) : null}
    </div>
  );
}

export function KpiGrid({ kpis, columns = 4 }: { kpis: KpiDatum[]; columns?: 2 | 4 }) {
  return (
    <div className={cn("grid grid-cols-2 gap-3", columns === 4 && "xl:grid-cols-4")}>
      {kpis.map((kpi) => <KpiCard key={kpi.key} kpi={kpi} />)}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* Info card — labeled rows                                            */
/* ------------------------------------------------------------------ */

export function InfoCard({ icon: Icon, title, rows, action, wide }: {
  icon: LucideIcon;
  title: string;
  rows: Array<[string, ReactNode]>;
  action?: ReactNode;
  wide?: boolean;
}) {
  return (
    <div className={cn("rounded-xl border border-line bg-[var(--surface)] p-5 shadow-sm", wide && "xl:col-span-3")}>
      <h3 className="flex items-center gap-2 text-sm font-bold text-text">
        <Icon size={15} className="text-text-subtle" /> {title}
        {action ? <span className="ml-auto">{action}</span> : null}
      </h3>
      <dl className="mt-2 divide-y divide-line">
        {rows.map(([label, value]) => (
          <div key={label} className="flex items-start justify-between gap-3 py-2">
            <dt className="shrink-0 text-xs text-text-muted">{label}</dt>
            <dd className="min-w-0 text-right text-xs">{value}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* Quick actions grid                                                  */
/* ------------------------------------------------------------------ */

export interface QuickAction {
  label: string;
  hint: string;
  icon: LucideIcon;
  href?: string;
  onSelect?: () => void;
  highlight?: boolean;
}

export function QuickActionsCard({ icon: Icon, title, actions, wide }: {
  icon: LucideIcon;
  title: string;
  actions: QuickAction[];
  wide?: boolean;
}) {
  return (
    <div className={cn("rounded-xl border border-line bg-[var(--surface)] p-5 shadow-sm", wide && "xl:col-span-2")}>
      <h3 className="flex items-center gap-2 text-sm font-bold text-text"><Icon size={15} className="text-text-subtle" /> {title}</h3>
      <div className="mt-3 grid grid-cols-2 gap-2">
        {actions.map(({ label, hint, icon: ActionIcon, href, onSelect, highlight }) => (
          href ? (
            <a
              key={label}
              href={href}
              className={cn(
                "group flex items-center gap-2.5 rounded-xl border p-3 transition hover:bg-[var(--surface-hover)]",
                highlight
                  ? "border-[color-mix(in_srgb,var(--brand)_40%,transparent)] bg-[color-mix(in_srgb,var(--brand)_7%,transparent)] hover:border-[color-mix(in_srgb,var(--brand)_60%,transparent)]"
                  : "border-line bg-overlay-subtle hover:border-line-strong"
              )}
            >
              <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg border border-line bg-overlay-subtle text-text-subtle transition group-hover:text-text">
                <ActionIcon size={15} />
              </span>
              <span className="min-w-0">
                <span className="block truncate text-xs font-bold text-text">{label}</span>
                <span className="block truncate text-[10px] text-text-muted">{hint}</span>
              </span>
            </a>
          ) : (
            <button
              key={label}
              type="button"
              onClick={onSelect}
              className="group flex items-center gap-2.5 rounded-xl border border-line bg-overlay-subtle p-3 text-left transition hover:border-line-strong hover:bg-[var(--surface-hover)]"
            >
              <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg border border-line bg-overlay-subtle text-text-subtle transition group-hover:text-text">
                <ActionIcon size={15} />
              </span>
              <span className="min-w-0">
                <span className="block truncate text-xs font-bold text-text">{label}</span>
                <span className="block truncate text-[10px] text-text-muted">{hint}</span>
              </span>
            </button>
          )
        ))}
      </div>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* Multi-series trend chart from real sampled values                   */
/* ------------------------------------------------------------------ */

export interface TrendSeries {
  key: string;
  color: string;
  label: string;
  display: string;
  values: number[];
}

export function TrendChart({ icon: Icon, title, subtitle, live, series, emptyHint, loading }: {
  icon: LucideIcon;
  title: string;
  subtitle?: string;
  live?: boolean;
  series: TrendSeries[];
  emptyHint: string;
  loading?: boolean;
}) {
  const ready = series.some((s) => s.values.length >= 2);
  const max = Math.max(1, ...series.flatMap((s) => s.values));
  return (
    <div className="rounded-xl border border-line bg-[var(--surface)] p-5 shadow-sm xl:col-span-3">
      <h3 className="flex items-center gap-2 text-sm font-bold text-text">
        <Icon size={15} className="text-text-subtle" /> {title}
        {subtitle ? <span className="font-normal text-text-muted">{subtitle}</span> : null}
        {live ? (
          <span className={cn("ml-auto flex items-center gap-1 font-mono text-[10px] font-normal", toneStyles.ok.fg)}>
            <span className={cn("h-1.5 w-1.5 animate-pulse rounded-full", toneStyles.ok.dot)} />live
          </span>
        ) : null}
      </h3>
      {ready ? (
        <div>
          <svg viewBox="0 0 300 90" className="mt-3 h-32 w-full" role="img" aria-label={`${title} chart`}>
            {[18, 44, 70].map((y) => (
              <line key={y} x1="0" y1={y} x2="300" y2={y} stroke={chart.gridSlate} strokeWidth="1" />
            ))}
            {series.filter((s) => s.values.length >= 2).map(({ key, color, values }) => {
              const pts = values.map((v, i) => `${((i / (values.length - 1)) * 300).toFixed(1)},${(86 - (Math.max(0, v) / max) * 78).toFixed(1)}`).join(" L ");
              return <path key={key} d={`M ${pts}`} fill="none" stroke={color} strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" />;
            })}
          </svg>
          <div className="mt-2 flex flex-wrap gap-x-5 gap-y-1 font-mono text-[11px]">
            {series.map((s) => (
              <span key={s.key} className="flex items-center gap-1.5 text-text-subtle">
                <span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: s.color }} />{s.label} {s.display}
              </span>
            ))}
          </div>
        </div>
      ) : (
        <p className="mt-3 rounded-lg border border-dashed border-line bg-overlay-subtle p-6 text-center text-xs text-text-muted">
          {loading ? "Connecting to live telemetry…" : emptyHint}
        </p>
      )}
    </div>
  );
}
