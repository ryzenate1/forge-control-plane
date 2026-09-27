"use client";

/**
 * Shared admin UI primitives used across all admin panel sections.
 */

import { ArrowLeft, LoaderCircle, LockKeyhole, X, type LucideIcon } from "lucide-react";
import { useRef } from "react";
import { usePathname } from "next/navigation";
import { Button, Dialog, EmptyState as SharedEmptyState, Input as SharedInput, Select as SharedSelect, Textarea as SharedTextarea } from "@/components/ui/primitives";
import { findAdminPageGroup } from "./admin-registry";
import { chart } from "@/lib/design-tokens";
import { cn } from "@/lib/utils";

export * from "@/components/ui/forge-primitives";
export { cn };

export function Pill({ children, tone = "neutral", className }: { children: React.ReactNode; tone?: "neutral" | "green" | "red" | "yellow" | "blue" | "success" | "warning" | "danger" | "info"; className?: string }) {
  const tones: Record<string, string> = {
    neutral: "border-white/10 bg-white/[0.03] text-slate-300",
    green: "border-emerald-500/25 bg-emerald-500/10 text-emerald-300",
    red: "border-red-500/25 bg-red-500/10 text-red-300",
    yellow: "border-amber-500/25 bg-amber-500/10 text-amber-300",
    blue: "border-blue-500/25 bg-blue-500/10 text-blue-300",
    // Semantic aliases (Forge health vocabulary: success/warning/danger/info) map
    // onto the same palette so callers can speak in meaning, not raw colour.
    success: "border-emerald-500/25 bg-emerald-500/10 text-emerald-300",
    warning: "border-amber-500/25 bg-amber-500/10 text-amber-300",
    danger: "border-red-500/25 bg-red-500/10 text-red-300",
    info: "border-blue-500/25 bg-blue-500/10 text-blue-300",
  };
  return (
    <span className={cn("inline-flex items-center rounded-md border px-2 py-0.5 text-[11px] font-medium tracking-tight", tones[tone], className)}>
      {children}
    </span>
  );
}

export function MiniSparkline({
  data,
  color = chart.blue,
  className,
}: {
  data: number[];
  color?: string;
  className?: string;
}) {
  if (!data || data.length < 2) return null;
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
  const areaD = `${pathD} L ${width},${height} L 0,${height} Z`;
  const gradientId = `sparkline-grad-${color.replace(/[^a-zA-Z0-9]/g, "")}`;

  return (
    <svg
      viewBox={`0 0 ${width} ${height}`}
      className={cn("overflow-visible", className)}
      aria-hidden="true"
    >
      <defs>
        <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity={0.35} />
          <stop offset="100%" stopColor={color} stopOpacity={0.0} />
        </linearGradient>
      </defs>
      <path d={areaD} fill={`url(#${gradientId})`} />
      <path d={pathD} fill="none" stroke={color} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

export function SubsystemHealthMeter({
  label,
  valueText,
  icon: Icon,
  percentage = 100,
  tone = "green",
  onClick,
}: {
  label: string;
  valueText: string;
  icon?: React.ElementType;
  percentage?: number;
  tone?: "green" | "yellow" | "red" | "neutral";
  onClick?: () => void;
}) {
  const barColors = {
    green: "bg-emerald-400",
    yellow: "bg-amber-400",
    red: "bg-red-400",
    neutral: "bg-slate-600",
  };
  const textColors = {
    green: "text-emerald-300",
    yellow: "text-amber-300",
    red: "text-red-300",
    neutral: "text-slate-400",
  };
  const clampedPct = Math.max(0, Math.min(100, percentage));

  return (
    <div
      onClick={onClick}
      onKeyDown={onClick ? (event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onClick();
        }
      } : undefined}
      role={onClick ? "button" : undefined}
      tabIndex={onClick ? 0 : undefined}
      className={cn(
        "flex items-center gap-3 text-xs",
        onClick && "cursor-pointer hover:bg-white/[0.03] rounded-md px-1.5 -mx-1.5 py-1 transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
      )}
    >
      <div className="flex items-center gap-2 w-32 shrink-0 text-slate-300">
        {Icon ? <Icon size={14} className="text-slate-400 shrink-0" /> : null}
        <span className="truncate font-medium">{label}</span>
      </div>
      <span className={cn("w-28 shrink-0 font-mono font-medium text-[11px]", textColors[tone])}>
        {valueText}
      </span>
      <div className="flex-1 h-2 rounded-full overflow-hidden bg-white/[0.06]">
        <div
          className={cn("h-full rounded-full transition-all duration-300", barColors[tone])}
          style={{ width: `${clampedPct}%` }}
        />
      </div>
    </div>
  );
}

export interface SectionHeaderProps {
  title: React.ReactNode;
  sub?: string;
  action?: React.ReactNode;
  breadcrumb?: React.ReactNode;
  hideBreadcrumb?: boolean;
  hideLiveBadge?: boolean;
  backAction?: () => void;
  backLabel?: string;
  className?: string;
}

export function SectionHeader({
  title,
  sub,
  action,
  breadcrumb,
  hideBreadcrumb,
  hideLiveBadge,
  backAction,
  backLabel,
  className,
}: SectionHeaderProps) {
  const pathname = usePathname() || "";
  const groupMatch = findAdminPageGroup(pathname);

  // Determine breadcrumb content
  let breadcrumbContent: React.ReactNode = null;
  if (!hideBreadcrumb) {
    if (breadcrumb) {
      if (typeof breadcrumb === "string") {
        const parts = breadcrumb.split("/").map((s) => s.trim()).filter(Boolean);
        breadcrumbContent = (
          <div className="flex items-center gap-2">
            {parts.map((part, index) => {
              const isLast = index === parts.length - 1;
              return (
                <span key={index} className="flex items-center gap-2">
                  {index > 0 && <span aria-hidden="true" className="text-slate-600 select-none after:content-['/']" />}
                  <span className={cn(isLast ? "font-semibold text-slate-200" : "text-slate-400")}>
                    {part}
                  </span>
                </span>
              );
            })}
          </div>
        );
      } else {
        breadcrumbContent = breadcrumb;
      }
    } else if (groupMatch) {
      breadcrumbContent = (
        <div className="flex items-center gap-2">
          <span>{groupMatch.groupTitle}</span>
          <span aria-hidden="true" className="text-slate-600 select-none after:content-['/']" />
          <span className="font-semibold text-slate-200">
            {groupMatch.pageLabel}
          </span>
        </div>
      );
    } else if (pathname.startsWith("/admin")) {
      const parts = pathname.split("/").filter(Boolean);
      const lastPart = parts[parts.length - 1] ?? "Admin";
      const label = lastPart.charAt(0).toUpperCase() + lastPart.slice(1);
      breadcrumbContent = (
        <div className="flex items-center gap-2">
          <span>Admin</span>
          <span aria-hidden="true" className="text-slate-600 select-none after:content-['/']" />
          <span className="font-semibold text-slate-200">{label}</span>
        </div>
      );
    }
  }

  return (
    <div className={cn("space-y-3 mb-6", className)}>
      {breadcrumbContent ? (
        <div className="flex items-center justify-between text-xs text-slate-400">
          <div className="flex items-center gap-3">
            {backAction ? (
              <button
                type="button"
                onClick={backAction}
                className="inline-flex items-center gap-1.5 text-xs font-medium text-slate-400 hover:text-slate-200 transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] rounded pr-3 border-r border-[var(--line)]"
              >
                <ArrowLeft size={13} />
                <span>{backLabel ?? "Back"}</span>
              </button>
            ) : null}
            {breadcrumbContent}
          </div>
          {!hideLiveBadge && (
            <div className="flex items-center gap-1.5 font-mono text-[11px] text-slate-500 shrink-0">
              <span className="relative flex h-1.5 w-1.5">
                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-60" />
                <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-emerald-400" />
              </span>
              <span>Live · updated just now</span>
            </div>
          )}
        </div>
      ) : backAction ? (
        <div className="flex items-center text-xs text-slate-400">
          <AdminBackButton label={backLabel} onClick={backAction} />
        </div>
      ) : null}

      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between border-b border-[var(--line)] pb-5">
        <div className="min-w-0">
          <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-slate-100 flex items-center gap-2.5">
            {title}
          </h1>
          {sub ? (
            <p className="mt-1 text-xs sm:text-sm text-slate-400 leading-relaxed max-w-3xl">
              {sub}
            </p>
          ) : null}
        </div>
        {action ? (
          <div className="flex shrink-0 flex-wrap items-center gap-2 sm:self-center">
            {action}
          </div>
        ) : null}
      </div>
    </div>
  );
}

/** Shared admin page frame. Use this for new and migrated screens rather than
 * rebuilding the header, action alignment, and responsive spacing per page. */
export function AdminPageLayout({ children, className }: { children: React.ReactNode; className?: string }) {
 return <div className={cn("mx-auto w-full max-w-[1600px] space-y-6", className)}>{children}</div>;
}

export function AdminBackButton({ onClick, label = "Back" }: { onClick: () => void; label?: string }) {
 return <button className="inline-flex items-center gap-1.5 text-sm font-medium text-slate-400 transition hover:text-slate-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-400" onClick={onClick} type="button"><ArrowLeft size={15} />{label}</button>;
}

export function AdminPageHeader({
  title,
  description,
  action,
  backAction,
  backLabel,
  breadcrumb,
  hideBreadcrumb,
  hideLiveBadge,
  className,
}: {
  title: string;
  description?: string;
  action?: React.ReactNode;
  backAction?: () => void;
  backLabel?: string;
  breadcrumb?: string;
  hideBreadcrumb?: boolean;
  hideLiveBadge?: boolean;
  className?: string;
}) {
  return (
    <SectionHeader
      title={title}
      sub={description}
      action={action}
      backAction={backAction}
      backLabel={backLabel}
      breadcrumb={breadcrumb}
      hideBreadcrumb={hideBreadcrumb}
      hideLiveBadge={hideLiveBadge}
      className={className}
    />
  );
}

export function AdminSection({ children, title, description, action, className }: { children: React.ReactNode; title?: string; description?: string; action?: React.ReactNode; className?: string }) {
 return <section className={cn("space-y-3", className)}>{title ? <div className="flex flex-wrap items-start justify-between gap-3"><div><h2 className="text-sm font-semibold text-slate-100">{title}</h2>{description ? <p className="mt-1 text-sm leading-6 text-slate-400">{description}</p> : null}</div>{action}</div> : null}{children}</section>;
}

export function Card({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <div className={cn("ui-card", className)}>
      {children}
    </div>
  );
}

export const AdminCard = Card;

export function CardHeader({ title, icon: Icon, action }: { title: string; icon?: React.ElementType; action?: React.ReactNode }) {
 return (
 <div className="flex min-h-12 items-center gap-2 border-b border-white/[0.07] bg-white/[0.018] -mx-4 sm:-mx-5 -mt-4 sm:-mt-5 mb-4 sm:mb-5 px-4 text-xs font-semibold tracking-wide text-slate-300 sm:px-5 rounded-t-2xl">
 {Icon ? <Icon size={14} /> : null}
 {title}
 {action ? <div className="ml-auto flex items-center gap-2 normal-case tracking-normal">{action}</div> : null}
 </div>
 );
}

export function Btn({
  children,
  onClick,
  tone = "primary",
  disabled,
  size = "md",
  type = "button",
  loading,
  className,
  ariaLabel,
  title,
}: {
  children: React.ReactNode;
  onClick?: () => void;
  tone?: "primary" | "ghost" | "danger" | "subtle" | "warning" | "success";
  disabled?: boolean;
  size?: "sm" | "md";
  type?: "button" | "submit";
  loading?: boolean;
  className?: string;
  ariaLabel?: string;
  title?: string;
}) {
  const variants = { primary: "primary", danger: "danger", ghost: "secondary", subtle: "ghost", warning: "secondary", success: "secondary" } as const;
  return <Button aria-label={ariaLabel} className={cn(tone === "warning" && "border-amber-700/40 bg-amber-900/70 text-amber-200 hover:bg-amber-800", tone === "success" && "border-emerald-700/40 bg-emerald-900/70 text-emerald-200 hover:bg-emerald-800", className)} disabled={disabled} loading={loading} onClick={onClick} size={size === "sm" ? "sm" : "default"} title={title} type={type} variant={variants[tone]}>{children}</Button>;
}

export function Input({ label, value, onChange, placeholder, type = "text", mono, required, readOnly, disabled, autoComplete }: {
  label?: string; value: string; onChange: (v: string) => void; placeholder?: string; type?: string; mono?: boolean; required?: boolean; readOnly?: boolean; disabled?: boolean; autoComplete?: string;
}) {
 return (
 <label className="block text-sm font-medium text-slate-300">
 {label ? <span className="mb-1.5 block">{label}</span> : null}
 <SharedInput
 autoComplete={autoComplete}
 className={cn("min-h-9 bg-surface-card-header", mono && "font-mono text-xs")}
 onChange={(e) => onChange(e.target.value)}
 disabled={disabled}
 placeholder={placeholder}
 required={required}
 readOnly={readOnly}
 type={type}
 value={value}
 />
 </label>
 );
}

export function Textarea({ label, value, onChange, rows = 4, placeholder }: {
 label?: string; value: string; onChange: (v: string) => void; rows?: number; placeholder?: string;
}) {
 return (
 <label className="block text-sm font-medium text-slate-300">
 {label ? <span className="mb-1.5 block">{label}</span> : null}
 <SharedTextarea
 className="min-h-0 bg-surface-card-header font-mono text-xs"
 onChange={(e) => onChange(e.target.value)}
 rows={rows}
 value={value}
 placeholder={placeholder}
 />
 </label>
 );
}

export function Badge({ children, className }: { children: React.ReactNode; className?: string }) {
  return <span className={cn("inline-flex items-center rounded px-2 py-0.5 text-xs font-medium", className)}>{children}</span>;
}

export function Modal({ title, onClose, children, wide, className, description, maxWidth, open = true }: { title: React.ReactNode; onClose: () => void; children: React.ReactNode; wide?: boolean; className?: string; description?: string; maxWidth?: string; open?: boolean }) {
  return <Dialog className={cn("max-h-[90vh] overflow-y-auto", wide && "max-w-3xl", maxWidth && maxWidth, className)} closeAction={onClose} description={description} open={open} title={title}>{children}</Dialog>;
}

export function AdminSelect({ label, value, onChange, options, placeholder, mono, disabled }: {
  label?: string; value: string; onChange: (v: string) => void; options: Array<{ value: string; label: string }>; placeholder?: string; mono?: boolean; disabled?: boolean;
}) {
  return (
    <label className="block text-sm font-medium text-slate-300">
      {label ? <span className="mb-1.5 block">{label}</span> : null}
      <SharedSelect
        className={cn("h-10 w-full", mono && "font-mono text-xs")}
        disabled={disabled}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      >
        {placeholder ? <option value="">{placeholder}</option> : null}
        {options.map((opt) => <option key={opt.value} value={opt.value}>{opt.label}</option>)}
      </SharedSelect>
    </label>
  );
}

export const AdminDialog = Modal;

export function AdminDrawer({ title, onClose, children }: { title: string; onClose: () => void; children: React.ReactNode }) {
  return <div className="fixed inset-0 z-50 flex justify-end bg-black/65 p-0 backdrop-blur-sm" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}><aside aria-label={title} className="flex h-full w-full max-w-2xl flex-col border-l border-white/[0.1] bg-surface-card shadow-2xl"><div className="flex items-center justify-between border-b border-white/[0.08] px-5 py-4"><h2 className="text-base font-semibold text-slate-100">{title}</h2><button aria-label="Close panel" className="rounded-lg p-2 text-slate-400 hover:bg-white/[0.06] hover:text-white" onClick={onClose} type="button"><X size={17} /></button></div><div className="min-h-0 flex-1 overflow-y-auto p-5 sm:p-6">{children}</div></aside></div>;
}

export function AdminFormSection({ title, description, children }: { title: string; description?: string; children: React.ReactNode }) {
 return <fieldset className="space-y-4 border-b border-white/[0.07] pb-6 last:border-0 last:pb-0"><legend className="text-sm font-semibold text-slate-100">{title}</legend>{description ? <p className="-mt-2 text-sm leading-6 text-slate-400">{description}</p> : null}<div className="grid gap-4">{children}</div></fieldset>;
}

export function AdminFormField({ label, hint, error, children }: { label: string; hint?: string; error?: string; children: React.ReactNode }) {
 return <label className="block text-sm font-medium text-slate-200"><span className="mb-1.5 block">{label}</span>{children}{error ? <span className="mt-1.5 block text-xs text-red-300">{error}</span> : hint ? <span className="mt-1.5 block text-xs leading-5 text-slate-500">{hint}</span> : null}</label>;
}

export function AdminToolbar({ children, className }: { children: React.ReactNode; className?: string }) { return <div className={cn("flex flex-col gap-3 rounded-xl border border-white/[0.07] bg-white/[0.015] p-3 sm:flex-row sm:flex-wrap sm:items-end", className)}>{children}</div>; }

export function AdminTable({ children, label, className }: { children: React.ReactNode; label?: string; className?: string }) {
  return <div className={cn("overflow-x-auto", className)}><table aria-label={label} className="w-full text-sm">{children}</table></div>;
}
export function AdminTHead({ children }: { children?: React.ReactNode }) { return <thead><tr className="border-b border-white/[0.06] text-left text-xs uppercase tracking-wider text-slate-500">{children}</tr></thead>; }
export function AdminTh({ children, className }: { children?: React.ReactNode; className?: string }) { return <th className={cn("px-4 py-3 font-medium", className)}>{children}</th>; }
export function AdminTBody({ children }: { children?: React.ReactNode }) { return <tbody className="divide-y divide-white/[0.04]">{children}</tbody>; }
export function AdminTr({ children, onClick, className }: { children?: React.ReactNode; onClick?: () => void; className?: string }) {
  if (!onClick) return <tr className={cn(className)}>{children}</tr>;
  return (
    <tr
      className={cn(className, "cursor-pointer hover:bg-white/[0.02] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[var(--focus)]")}
      onClick={onClick}
      onKeyDown={(event) => {
        if (event.key === "Enter") onClick();
      }}
      tabIndex={0}
    >
      {children}
    </tr>
  );
}
// title carries a tooltip for cells that truncate their content, so the full
// value stays reachable when the column is too narrow to show it.
export function AdminTd({ children, className, title }: { children?: React.ReactNode; className?: string; title?: string }) { return <td className={cn("px-4 py-3 text-slate-200", className)} title={title}>{children}</td>; }

export const selectStyle = "h-10 w-full rounded-lg border border-white/10 bg-[var(--surface-input)] px-3 text-sm text-slate-100";

export function AdminLoadingState({ label = "Loading…" }: { label?: string }) { return <div className="grid min-h-32 place-items-center rounded-xl border border-dashed border-white/[0.1] bg-black/10 p-6 text-sm text-slate-400" role="status"><div className="flex flex-col items-center gap-2"><LoaderCircle size={20} className="animate-spin text-slate-500" /><span>{label}</span></div></div>; }

export function AdminLoadingRows({ rows = 3, cols = 4, label = "Loading rows…" }: { rows?: number; cols?: number; label?: string }) {
  return <div aria-label={label} className="divide-y divide-white/[0.04]" role="status">{Array.from({ length: rows }, (_, i) => <div key={i} className="flex gap-4 px-4 py-3">{Array.from({ length: cols }, (_, j) => <div key={j} className="h-4 flex-1 animate-pulse rounded bg-white/[0.06]" />)}</div>)}</div>;
}

export function AdminErrorState({ message, retry }: { message: string; retry?: () => void }) { return <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-red-500/25 bg-red-950/20 p-4 text-sm text-red-100" role="alert"><span>{message}</span>{retry ? <Btn size="sm" tone="ghost" onClick={retry}>Retry</Btn> : null}</div>; }

export interface AdminTab { id: string; label: string; icon?: LucideIcon; danger?: boolean; }
export function AdminTabs({ tabs, active, onChange, label = "Page sections" }: { tabs: AdminTab[]; active: string; onChange: (id: string) => void; label?: string }) {
  const tabRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const handleTabKeyDown = (e: React.KeyboardEvent<HTMLButtonElement>, index: number) => {
    let next: number | null = null;
    if (e.key === "ArrowRight") next = (index + 1) % tabs.length;
    else if (e.key === "ArrowLeft") next = (index - 1 + tabs.length) % tabs.length;
    else if (e.key === "Home") next = 0;
    else if (e.key === "End") next = tabs.length - 1;
    if (next === null) return;
    e.preventDefault();
    onChange(tabs[next].id);
    tabRefs.current[next]?.focus();
  };
  return <div aria-label={label} className="flex overflow-x-auto border-b border-white/[0.08]" role="tablist">
    {tabs.map((tab, index) => {
      const isActive = active === tab.id;
      const Icon = tab.icon;
      return <button
        aria-selected={isActive}
        className={cn(
          "flex shrink-0 items-center gap-1.5 border-b-2 px-3 py-2.5 text-sm font-medium transition whitespace-nowrap",
          tab.danger && isActive ? "border-red-500 text-red-300" :
          isActive ? "border-red-400 text-red-300" :
          "border-transparent text-slate-400 hover:text-slate-100"
        )}
        key={tab.id}
        onClick={() => onChange(tab.id)}
        onKeyDown={(e) => handleTabKeyDown(e, index)}
        ref={(el) => { tabRefs.current[index] = el; }}
        role="tab"
        tabIndex={isActive ? 0 : -1}
        type="button"
      >
        {Icon ? <Icon size={14} /> : null}
        {tab.label}
      </button>;
    })}
  </div>;
}

export function Kbd({ children }: { children: React.ReactNode }) {
  return <kbd className="inline-flex min-w-[1.5rem] items-center justify-center rounded-md border border-white/[0.12] bg-white/[0.04] px-1.5 py-0.5 text-[10px] font-bold tracking-wide text-slate-400 shadow-sm">{children}</kbd>;
}

export function Separator({ className }: { className?: string }) {
  return <div className={cn("h-px w-full bg-white/[0.07]", className)} role="separator" />;
}

export function DataTable({ headers, rows, label, className }: {
  headers: string[];
  rows: Array<Record<string, React.ReactNode>>;
  label: string;
  className?: string;
}) {
  return (
    <div className={cn("overflow-x-auto", className)}>
      <table aria-label={label} className="w-full text-sm">
        <thead>
          <tr className="border-b border-white/[0.06] text-left text-xs uppercase tracking-wider text-slate-500">
            {headers.map((header) => <th key={header} className="px-4 py-3 font-medium">{header}</th>)}
          </tr>
        </thead>
        <tbody className="divide-y divide-white/[0.04]">
          {rows.map((row, i) => (
            <tr key={i} className="hover:bg-white/[0.02]">
              {headers.map((header) => <td key={header} className="px-4 py-3 text-slate-200">{row[header] ?? <span className="text-slate-500">—</span>}</td>)}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export const AdminBadge = Pill;

export function AdminIconButton({ label, children, onClick, tone = "neutral", disabled }: { label: string; children: React.ReactNode; onClick: () => void; tone?: "neutral" | "danger"; disabled?: boolean }) { return <button aria-label={label} className={cn("inline-grid h-9 w-9 place-items-center rounded-lg transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-400", tone === "danger" ? "text-red-300 hover:bg-red-500/10" : "text-slate-400 hover:bg-white/[0.06] hover:text-slate-100")} disabled={disabled} onClick={onClick} title={label} type="button">{children}</button>; }

export function AdminConfirmDialog({ title, description, confirmLabel = "Confirm", onCancel, onConfirm, loading = false, destructive = false, open = true }: { title: string; description: string; confirmLabel?: string; onCancel: () => void; onConfirm: () => void; loading?: boolean; destructive?: boolean; open?: boolean }) { return <Modal open={open} title={title} description={description} onClose={onCancel}><div className="-mx-6 -mb-5 mt-5 flex flex-col-reverse gap-2 border-t border-[var(--line)] bg-white/[0.015] px-6 py-4 sm:flex-row sm:justify-end"><Btn onClick={onCancel} tone="ghost">Cancel</Btn><Btn disabled={loading} onClick={onConfirm} tone={destructive ? "danger" : "primary"}>{confirmLabel}</Btn></div></Modal>; }

export function ModalFooter({ onCancel, onConfirm, confirmLabel = "Save", disabled }: {
 onCancel: () => void; onConfirm: () => void; confirmLabel?: string; disabled?: boolean;
}) {
 return (
 <div className="sticky bottom-0 z-10 -mx-6 -mb-5 mt-5 flex flex-col-reverse gap-2 border-t border-[var(--line)] bg-white/[0.015] px-6 py-4 sm:flex-row sm:justify-end">
 <Btn onClick={onCancel} tone="ghost">Cancel</Btn>
 <Btn disabled={disabled} onClick={onConfirm}>{confirmLabel}</Btn>
 </div>
 );
}

export function EmptyState({ icon: Icon, message, title, sub }: { icon?: React.ElementType; message?: string; title?: string; sub?: string }) {
 return <SharedEmptyState description={message ?? sub ?? ""} icon={Icon ? <Icon size={20} strokeWidth={1.5} /> : undefined} title={title ?? "Nothing to show"} />;
}

export function PermissionDeniedState({ message }: { message?: string }) {
 return (
 <div className="flex min-h-[200px] flex-col items-center justify-center rounded-md border border-amber-500/20 bg-amber-500/5 p-6 text-center">
 <LockKeyhole size={32} className="mb-3 text-amber-400" strokeWidth={1.5} />
 <h3 className="text-base font-semibold text-amber-200">Access Denied</h3>
 <p className="mt-1 max-w-md text-sm text-amber-400/80">
 {message ?? "You don\u2019t have permission to view this resource. Contact an administrator to request access."}
 </p>
 </div>
 );
}

export function StatsRow({ items }: { items: Array<{ label: string; value: string | number; icon?: LucideIcon; tone?: "green" | "red" | "yellow" | "blue" | "neutral" }> }) {
  const tones: Record<string, string> = {
    green: "text-emerald-400",
    red: "text-red-400",
    yellow: "text-amber-400",
    blue: "text-blue-400",
    neutral: "text-slate-100",
  };
  if (!items || items.length === 0) return null;
  return (
    <div className="mb-6 grid grid-cols-2 gap-3.5 lg:grid-cols-4">
      {items.map((item) => (
        <div
          key={item.label}
          className="group relative overflow-hidden rounded-xl border border-white/[0.08] bg-[var(--surface)] p-4 shadow-sm transition-all duration-200 hover:border-white/[0.16] hover:bg-white/[0.02]"
        >
          <div className="flex items-center justify-between gap-2 text-[11px] font-semibold uppercase tracking-wider text-slate-400">
            <span className="truncate">{item.label}</span>
            {item.icon ? (
              <div className="grid h-6 w-6 place-items-center rounded-md border border-white/[0.08] bg-white/[0.03] text-slate-400 transition-colors group-hover:text-slate-200">
                <item.icon size={13} />
              </div>
            ) : null}
          </div>
          <div className={cn("mt-2 font-mono text-2xl font-bold tracking-tight", tones[item.tone ?? "neutral"])}>
            {item.value}
          </div>
        </div>
      ))}
    </div>
  );
}

export function AdminStatCard({ label, value, icon: Icon, tone = "neutral", className }: {
  label: string; value: string | number; icon?: LucideIcon; tone?: "green" | "red" | "yellow" | "blue" | "neutral"; className?: string;
}) {
  const valueTones: Record<string, string> = {
    green: "text-emerald-400",
    red: "text-red-400",
    yellow: "text-amber-400",
    blue: "text-blue-400",
    neutral: "text-slate-100",
  };
  return (
    <div className={cn("group relative overflow-hidden rounded-xl border border-white/[0.08] bg-[var(--surface)] p-4 shadow-sm transition-all duration-200 hover:border-white/[0.16]", className)}>
      <div className="flex items-center justify-between gap-2 text-[11px] font-semibold uppercase tracking-wider text-slate-400 mb-1">
        <span className="truncate">{label}</span>
        {Icon ? (
          <div className="grid h-6 w-6 place-items-center rounded-md border border-white/[0.08] bg-white/[0.03] text-slate-400 transition-colors group-hover:text-slate-200">
            <Icon size={13} />
          </div>
        ) : null}
      </div>
      <div className={cn("font-mono text-2xl font-bold tracking-tight", valueTones[tone])}>
        {value}
      </div>
    </div>
  );
}
