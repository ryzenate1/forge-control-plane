"use client";

import { AlertCircle, Check, CheckCircle2, ChevronLeft, ChevronRight, Copy, Info, LoaderCircle, Search, TriangleAlert, X } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { forwardRef, useEffect, useId, useRef, useState, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes, type TextareaHTMLAttributes } from "react";
import { cn } from "@/lib/utils";
import { useToast } from "@/components/ui/toast";

export const Button = forwardRef<HTMLButtonElement, ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "primary" | "secondary" | "danger" | "ghost"; size?: "default" | "sm"; loading?: boolean }>(function Button({ className, children, disabled, loading = false, type = "button", variant = "primary", size = "default", ...props }, ref) {
  return <button ref={ref} className={cn("ui-button", `ui-button-${variant}`, size === "sm" ? "px-3 py-1.5 min-h-8 text-xs rounded-md" : "", className)} disabled={disabled || loading} aria-busy={loading || undefined} {...props} type={type}>{loading ? <LoaderCircle aria-hidden="true" className="h-4 w-4 animate-spin" /> : null}{children}</button>;
});

export function Field({ id, label, hint, error, children }: { id: string; label: string; hint?: ReactNode; error?: string; children: ReactNode }) {
  return <div className="space-y-1.5"><label className="ui-label" htmlFor={id}>{label}</label>{children}{error ? <p className="ui-field-error" id={`${id}-error`} role="alert">{error}</p> : hint ? <p className="ui-hint" id={`${id}-hint`}>{hint}</p> : null}</div>;
}

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement> & { invalid?: boolean }>(function Input({ className, invalid, ...props }, ref) {
  return <input ref={ref} className={cn("ui-input", className)} aria-invalid={invalid || undefined} {...props} />;
});

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaHTMLAttributes<HTMLTextAreaElement> & { invalid?: boolean }>(function Textarea({ className, invalid, ...props }, ref) {
  return <textarea ref={ref} className={cn("ui-input min-h-24 resize-y py-3", className)} aria-invalid={invalid || undefined} {...props} />;
});

export const Select = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement> & { invalid?: boolean }>(function Select({ className, invalid, ...props }, ref) {
  return <select ref={ref} className={cn("ui-input appearance-auto", className)} aria-invalid={invalid || undefined} {...props} />;
});

const alertIcons = { error: AlertCircle, warning: TriangleAlert, success: CheckCircle2, info: Info };
export function Alert({ tone = "info", title, children, actions, className }: { tone?: keyof typeof alertIcons; title?: string; children: ReactNode; actions?: ReactNode; className?: string }) {
  const Icon = alertIcons[tone];
  return <div className={cn("ui-alert", `ui-alert-${tone}`, className)} role={tone === "error" || tone === "warning" ? "alert" : "status"}><Icon aria-hidden="true" className="mt-0.5 h-5 w-5 shrink-0" /><div className="min-w-0 flex-1">{title ? <p className="font-semibold text-current">{title}</p> : null}<div className={cn("text-sm", title && "mt-1")}>{children}</div></div>{actions ? <div className="shrink-0">{actions}</div> : null}</div>;
}

export function Card({ title, description, icon, badge, children, className, contentClassName }: { title?: string; description?: string; icon?: ReactNode; badge?: ReactNode; children: ReactNode; className?: string; contentClassName?: string }) {
  return <section className={cn("ui-card", className)}>{title ? <div className="ui-card-header"><div className="flex min-w-0 items-start gap-3">{icon ? <span className="mt-0.5 text-slate-400">{icon}</span> : null}<div><h2 className="text-base font-semibold text-slate-100">{title}</h2>{description ? <p className="mt-1 text-sm leading-6 text-slate-400">{description}</p> : null}</div></div>{badge}</div> : null}<div className={cn("p-5 sm:p-6", contentClassName)}>{children}</div></section>;
}

export function Badge({ tone = "neutral", children }: { tone?: "neutral" | "success" | "warning" | "danger"; children: ReactNode }) {
  return <span className={cn("ui-badge", `ui-badge-${tone}`)}>{children}</span>;
}

export function StatusPill({ children, tone = "neutral", pulse = false }: { children: ReactNode; tone?: "neutral" | "success" | "warning" | "danger" | "info"; pulse?: boolean }) {
  return <span className={cn("ui-status-pill", `ui-status-pill-${tone}`)}><span aria-hidden="true" className={cn("h-1.5 w-1.5 rounded-full bg-current", pulse && "animate-pulse")} />{children}</span>;
}

export function SearchInput({ label = "Search", className, ...props }: InputHTMLAttributes<HTMLInputElement> & { label?: string }) {
  return <label className={cn("relative block", className)}><span className="sr-only">{label}</span><Search aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500" /><Input {...props} className="pl-9" type="search" /></label>;
}

export function Switch({ checked, onCheckedChange, label, disabled = false }: { checked: boolean; onCheckedChange: (checked: boolean) => void; label: ReactNode; disabled?: boolean }) {
  const id = useId();
  return <label className={cn("inline-flex items-center gap-2.5 text-xs", disabled ? "cursor-not-allowed opacity-50" : "cursor-pointer")} htmlFor={id}><span className="uppercase tracking-wide text-slate-400">{label}</span><button aria-checked={checked} aria-label={typeof label === "string" ? label : undefined} className={cn("relative h-5 w-9 rounded-full transition-colors", checked ? "bg-red-600" : "bg-slate-600")} disabled={disabled} id={id} onClick={() => onCheckedChange(!checked)} role="switch" type="button"><span className={cn("absolute left-0.5 top-0.5 h-4 w-4 rounded-full bg-white shadow transition-transform", checked && "translate-x-4")} /></button></label>;
}

export function ProgressBar({ value, label, className, alarmAt = 90 }: { value: number; label: string; className?: string; alarmAt?: number }) {
  const percent = Math.min(100, Math.max(0, Number.isFinite(value) ? value : 0));
  const alarm = percent >= alarmAt;
  return <div aria-label={label} aria-valuemax={100} aria-valuemin={0} aria-valuenow={Math.round(percent)} className={cn("h-1.5 overflow-hidden rounded-full bg-slate-700/70", className)} role="progressbar"><div className={cn("h-full rounded-full transition-all", alarm ? "bg-red-500" : "bg-emerald-500/70")} style={{ width: `${percent}%` }} /></div>;
}

export function ResourceBar({ icon: Icon, label, current, limit, unit = "" }: { icon: LucideIcon; label: string; current?: number | null; limit?: number | null; unit?: string }) {
  const used = typeof current === "number" ? current : 0;
  const max = typeof limit === "number" && limit > 0 ? limit : 0;
  const percent = max > 0 ? Math.min(100, (used / max) * 100) : 0;
  const alarm = max > 0 && percent >= 90;
  return <div className="flex items-center gap-2" title={`${label}: ${used}${unit} / ${max > 0 ? `${max}${unit}` : "Unlimited"}`}><Icon aria-hidden="true" className={cn("h-3.5 w-3.5 shrink-0", alarm ? "text-red-400" : "text-slate-500")} />{max === 0 ? <span className="text-[10px] text-slate-500">Unavailable</span> : <div className="flex flex-1 items-center gap-2"><ProgressBar className="flex-1" label={`${label} usage`} value={percent} /><span className={cn("min-w-[3ch] text-right font-mono text-[10px]", alarm ? "font-bold text-red-300" : "text-slate-400")}>{Math.round(percent)}%</span></div>}</div>;
}

export function Table({ children, label, className }: { children: ReactNode; label: string; className?: string }) {
  return <div className="overflow-x-auto"><table aria-label={label} className={cn("w-full text-left text-sm", className)}>{children}</table></div>;
}

export function Pagination({ page, pageCount, onPageChange, label = "Pagination" }: { page: number; pageCount: number; onPageChange: (page: number) => void; label?: string }) {
  const current = Math.min(Math.max(page, 1), Math.max(pageCount, 1));
  if (pageCount <= 1) return null;
  return <nav aria-label={label} className="mt-6 flex items-center justify-between rounded-lg border border-white/[0.06] bg-surface-card p-3"><Button disabled={current <= 1} onClick={() => onPageChange(current - 1)} size="sm" variant="ghost"><ChevronLeft className="h-4 w-4" />Previous</Button><span className="text-sm text-slate-400">Page {current} of {pageCount}</span><Button disabled={current >= pageCount} onClick={() => onPageChange(current + 1)} size="sm" variant="ghost">Next<ChevronRight className="h-4 w-4" /></Button></nav>;
}

export function CopyButton({ value, label = "Copy", className }: { value: string; label?: string; className?: string }) {
  const { toast } = useToast();
  const [copied, setCopied] = useState(false);
  return <Button className={className} onClick={async () => { try { await navigator.clipboard.writeText(value); setCopied(true); window.setTimeout(() => setCopied(false), 1500); toast({ tone: "success", title: "Copied to clipboard" }); } catch { toast({ tone: "error", title: "Could not copy", message: "Select and copy the value manually." }); } }} type="button" variant="secondary">{copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}{copied ? "Copied" : label}</Button>;
}

export function EmptyState({ icon, title, description, action }: { icon?: ReactNode; title: string; description: string; action?: ReactNode }) {
  return <div className="ui-empty"><div className="ui-empty-icon">{icon}</div><h3 className="mt-3 text-sm font-semibold text-slate-200">{title}</h3><p className="mt-1 max-w-md text-sm leading-6 text-slate-400">{description}</p>{action ? <div className="mt-4">{action}</div> : null}</div>;
}

export function Dialog({ open, title, description, children, closeAction, className }: { open: boolean; title: ReactNode; description?: string; children: ReactNode; closeAction: () => void; className?: string }) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const closeRef = useRef(closeAction);
  closeRef.current = closeAction;
  useEffect(() => {
    if (!open) return;
    const previouslyFocused = document.activeElement as HTMLElement | null;
    const dialog = dialogRef.current;
    dialog?.querySelector<HTMLElement>("button:not([disabled]), input:not([disabled]), textarea:not([disabled]), [href]")?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") closeRef.current();
      if (event.key !== "Tab" || !dialog) return;
      const focusable = Array.from(dialog.querySelectorAll<HTMLElement>("button:not([disabled]), input:not([disabled]), textarea:not([disabled]), [href]"));
      if (!focusable.length) return;
      const first = focusable[0]; const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => { document.removeEventListener("keydown", onKeyDown); previouslyFocused?.focus(); };
  }, [open]);
  if (!open) return null;
  return (
    <div className="ui-dialog-layer" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) closeAction(); }}>
      <div ref={dialogRef} aria-describedby={description ? "dialog-description" : undefined} aria-labelledby="dialog-title" aria-modal="true" className={cn("ui-dialog", className)} role="dialog">
        <div className="flex items-start justify-between gap-4">
          <div><h2 className="text-lg font-semibold text-white" id="dialog-title">{title}</h2>{description ? <p className="mt-1 text-sm text-slate-400" id="dialog-description">{description}</p> : null}</div>
          <button aria-label="Close dialog" className="ui-icon-button" onClick={closeAction} type="button"><X className="h-4 w-4" /></button>
        </div>
        <div className="mt-5">{children}</div>
      </div>
    </div>
  );
}

export function ConfirmDialog({ open, title, description, confirmLabel = "Confirm", destructive = false, loading = false, closeAction, confirmAction }: { open: boolean; title: string; description: string; confirmLabel?: string; destructive?: boolean; loading?: boolean; closeAction: () => void; confirmAction: () => void }) {
  return <Dialog closeAction={closeAction} description={description} open={open} title={title}><div className="flex justify-end gap-2"><Button disabled={loading} onClick={closeAction} variant="ghost">Cancel</Button><Button loading={loading} onClick={confirmAction} variant={destructive ? "danger" : "primary"}>{confirmLabel}</Button></div></Dialog>;
}
