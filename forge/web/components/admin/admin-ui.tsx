"use client";

/**
 * Shared admin UI primitives used across all admin panel sections.
 */

import { ArrowLeft, LoaderCircle, LockKeyhole, X, type LucideIcon } from "lucide-react";
import { useRef } from "react";
import { Button, Dialog, EmptyState as SharedEmptyState, Input as SharedInput, Select as SharedSelect, Textarea as SharedTextarea } from "@/components/ui/primitives";
import { cn } from "@/lib/utils";

export { cn };

export function Pill({ children, tone = "neutral", className }: { children: React.ReactNode; tone?: "neutral" | "green" | "red" | "yellow" | "blue"; className?: string }) {
 const tones: Record<string, string> = {
  neutral: "border border-white/10 bg-white/[0.03] text-slate-300",
  green: "border border-emerald-500/30 bg-emerald-900/30 text-emerald-300",
  red: "border border-red-500/30 bg-red-900/30 text-red-300",
  yellow: "border border-amber-500/30 bg-amber-900/30 text-amber-300",
  blue: "border border-blue-500/30 bg-blue-900/30 text-blue-300",
};
 return (
 <span className={cn("inline-flex items-center rounded-full px-2.5 py-1 text-[11px] font-semibold tracking-wide", tones[tone], className)}>
 {children}
 </span>
 );
}

export function SectionHeader({ title, sub, action }: { title: string; sub?: string; action?: React.ReactNode }) {
  return (
  <div className="relative mb-7 flex flex-col gap-4 border-b border-white/[0.08] pb-5 sm:flex-row sm:items-start sm:justify-between">
  <div>
  <div className="mb-2 h-1 w-10 rounded-full bg-brand" aria-hidden="true" />
  <h1 className="text-[clamp(1.4rem,2vw,1.85rem)] font-semibold tracking-[-0.025em] text-slate-100">{title}</h1>
  {sub ? <p className="mt-1.5 max-w-2xl text-sm leading-6 text-slate-400">{sub}</p> : null}
  </div>
  {action ? <div className="flex shrink-0 flex-wrap gap-2">{action}</div> : null}
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

export function AdminPageHeader({ title, description, action, backAction, backLabel, breadcrumb }: { title: string; description?: string; action?: React.ReactNode; backAction?: () => void; backLabel?: string; breadcrumb?: string }) {
 return <div>{(backAction || breadcrumb) ? <div className="mb-3 flex flex-wrap items-center gap-3 text-xs text-slate-500">{backAction ? <AdminBackButton label={backLabel} onClick={backAction} /> : null}{breadcrumb ? <span>{breadcrumb}</span> : null}</div> : null}<SectionHeader title={title} sub={description} action={action} /></div>;
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

export function CardHeader({ title, icon: Icon, action }: { title: string; icon?: LucideIcon; action?: React.ReactNode }) {
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
export function AdminTHead({ children }: { children: React.ReactNode }) { return <thead><tr className="border-b border-white/[0.06] text-left text-xs uppercase tracking-wider text-slate-500">{children}</tr></thead>; }
export function AdminTh({ children, className }: { children: React.ReactNode; className?: string }) { return <th className={cn("px-4 py-3 font-medium", className)}>{children}</th>; }
export function AdminTBody({ children }: { children: React.ReactNode }) { return <tbody className="divide-y divide-white/[0.04]">{children}</tbody>; }
export function AdminTr({ children, onClick, className }: { children: React.ReactNode; onClick?: () => void; className?: string }) {
  return <tr className={cn(className, onClick && "cursor-pointer hover:bg-white/[0.02]")} onClick={onClick}>{children}</tr>;
}
export function AdminTd({ children, className }: { children: React.ReactNode; className?: string }) { return <td className={cn("px-4 py-3 text-slate-200", className)}>{children}</td>; }

export const selectStyle = "h-10 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100";

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

export function AdminConfirmDialog({ title, description, confirmLabel = "Confirm", onCancel, onConfirm, loading = false, destructive = false, open = true }: { title: string; description: string; confirmLabel?: string; onCancel: () => void; onConfirm: () => void; loading?: boolean; destructive?: boolean; open?: boolean }) { return <Modal open={open} title={title} onClose={onCancel}><p className="text-sm leading-6 text-slate-400">{description}</p><div className="mt-5 flex flex-col-reverse gap-2 border-t border-white/[0.06] pt-4 sm:flex-row sm:justify-end"><Btn onClick={onCancel} tone="ghost">Cancel</Btn><Btn disabled={loading} onClick={onConfirm} tone={destructive ? "danger" : "primary"}>{confirmLabel}</Btn></div></Modal>; }

export function ModalFooter({ onCancel, onConfirm, confirmLabel = "Save", disabled }: {
 onCancel: () => void; onConfirm: () => void; confirmLabel?: string; disabled?: boolean;
}) {
 return (
 <div className="sticky bottom-0 z-10 mt-5 flex flex-col-reverse gap-2 border-t border-white/[0.06] bg-surface-card pt-4 sm:flex-row sm:justify-end">
 <Btn onClick={onCancel} tone="ghost">Cancel</Btn>
 <Btn disabled={disabled} onClick={onConfirm}>{confirmLabel}</Btn>
 </div>
 );
}

export function EmptyState({ icon: Icon, message, title, sub }: { icon?: LucideIcon; message?: string; title?: string; sub?: string }) {
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
  neutral: "text-slate-300",
  };
  if (!items || items.length === 0) return null;
  return (
  <div className="mb-6 grid grid-cols-2 gap-3 md:grid-cols-4">
  {items.map((item) => (
  <Card key={item.label}>
  <div className="flex items-center gap-2 text-xs text-slate-500 uppercase tracking-wider mb-1">
  {item.icon ? <item.icon size={12} /> : null}
  {item.label}
  </div>
  <div className={cn("text-2xl font-bold", tones[item.tone ?? "neutral"])}>
  {item.value}
  </div>
  </Card>
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
    <div className={cn("rounded-xl border border-white/[0.09] bg-[#111722] p-4", className)}>
      <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-slate-500 mb-0.5">
        {Icon ? <Icon size={12} /> : null}
        {label}
      </div>
      <div className={cn("text-2xl font-bold tracking-tight", valueTones[tone])}>
        {value}
      </div>
    </div>
  );
}
