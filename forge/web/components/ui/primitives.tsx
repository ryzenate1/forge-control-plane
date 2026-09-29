"use client";

import {
  AlertCircle, Check, CheckCircle2, ChevronLeft, ChevronRight, Copy, Info, LoaderCircle, Search, TriangleAlert,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import {
  cloneElement, forwardRef, Fragment, isValidElement, useId, useState,
  type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes, type TextareaHTMLAttributes,
} from "react";
import { cn } from "@/lib/utils";
import { useToast } from "@/components/ui/toast";
import { ForgeDialog } from "./forge/overlay";

import { Button as ButtonBase } from "./button";
import { Input as InputBase } from "./input";
import { Badge as BadgeBase } from "./badge";
import { resolveTone, toneStyles, type ForgeTone, type ToneInput } from "./forge/status";
export {
  CardHeader, CardTitle, CardDescription, CardContent, CardFooter,
} from "./card";
export {
  TableHeader, TableBody, TableRow, TableHead, TableCell,
} from "./table";

const variantMap: Record<string, "default" | "destructive" | "secondary" | "ghost"> = {
  primary: "default", secondary: "secondary", danger: "destructive", ghost: "ghost",
};

export const Button = forwardRef<HTMLButtonElement, ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "danger" | "ghost";
  size?: "default" | "sm";
  loading?: boolean;
}>(function Button({ children, disabled, loading = false, type = "button", variant = "primary", size = "default", ...props }, ref) {
  return (
    <ButtonBase ref={ref} disabled={disabled || loading} aria-busy={loading || undefined} type={type} variant={variantMap[variant]} size={size} {...props}>
      {loading ? <LoaderCircle aria-hidden="true" className="h-4 w-4 animate-spin" /> : null}
      {children}
    </ButtonBase>
  );
});

export function Field({ id, label, hint, error, children }: { id: string; label: string; hint?: ReactNode; error?: string; children: ReactNode }) {
  const describedBy = error ? `${id}-error` : hint ? `${id}-hint` : undefined;
  let wiredChildren = children;
  if (describedBy && isValidElement(children)) {
    const child = children as React.ReactElement<{ "aria-describedby"?: string; "aria-invalid"?: boolean | string }>;
    // Fragments accept only `key` — cloning aria props onto one warns and wires
    // nothing, so fragment children are left alone (see ForgeField). The symbol
    // check is the whole guard: a fragment's element type is a symbol at
    // runtime, but comparing it to Symbol.for("react.fragment") does not
    // typecheck, because ReactElement.type is declared as
    // string | JSXElementConstructor and so never overlaps symbol.
    if (typeof child.type !== "symbol") {
      wiredChildren = cloneElement(child, {
        "aria-describedby": [child.props["aria-describedby"], describedBy].filter(Boolean).join(" ") || undefined,
        "aria-invalid": error ? true : child.props["aria-invalid"],
      });
    }
  }
  return (
    <div className="space-y-1.5">
      <label className="ui-label" htmlFor={id}>{label}</label>
      {wiredChildren}
      {error ? <p className="ui-field-error" id={`${id}-error`} role="alert">{error}</p> : hint ? <p className="ui-hint" id={`${id}-hint`}>{hint}</p> : null}
    </div>
  );
}

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement> & { invalid?: boolean }>(function Input({ invalid, ...props }, ref) {
  return <InputBase ref={ref} aria-invalid={invalid || undefined} {...props} />;
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
  return (
    <div className={cn("ui-alert", `ui-alert-${tone}`, className)} role={tone === "error" || tone === "warning" ? "alert" : "status"}>
      <Icon aria-hidden="true" className="mt-px size-4 shrink-0" />
      <div className="min-w-0 flex-1">
        {title ? <p className="font-semibold text-current">{title}</p> : null}
        <div className={cn(title && "mt-0.5")}>{children}</div>
      </div>
      {actions ? <div className="shrink-0">{actions}</div> : null}
    </div>
  );
}

export function Card({ title, description, icon, badge, children, className, contentClassName }: { title?: string; description?: string; icon?: ReactNode; badge?: ReactNode; children: ReactNode; className?: string; contentClassName?: string }) {
  // NOTE: the section is `ui-surface`, not `ui-card` — `ui-card` already
  // carries p-4/sm:p-5, and the content slot below adds its own padding, so
  // combining them doubled every inset. Header/content own their padding here.
  return (
    <section className={cn("ui-surface shadow-card", className)}>
      {title ? (
        <div className="ui-card-header">
          <div className="flex min-w-0 items-start gap-3">
            {icon ? <span className="mt-px shrink-0 text-text-subtle">{icon}</span> : null}
            <div className="min-w-0">
              <h2 className="t-section">{title}</h2>
              {description ? <p className="mt-0.5 max-w-prose text-meta text-text-subtle">{description}</p> : null}
            </div>
          </div>
          {badge}
        </div>
      ) : null}
      <div className={cn("p-4 sm:p-5", contentClassName)}>{children}</div>
    </section>
  );
}

const toneBadgeMap: Record<ForgeTone, "default" | "destructive" | "secondary" | "outline"> = {
  ok: "default", warn: "secondary", danger: "destructive", info: "secondary",
  pending: "secondary", neutral: "outline", unknown: "outline",
};

/**
 * Tone names these components accept. Re-exported under the local name these
 * call sites already use; the vocabulary itself is defined once in
 * `components/ui/forge/status.ts`, the single source of status meaning.
 */
export type Tone = ToneInput;

export function Badge({ tone = "neutral", children, className }: { tone?: Tone; children: ReactNode; className?: string }) {
  return <BadgeBase className={className} variant={toneBadgeMap[resolveTone(tone)]}>{children}</BadgeBase>;
}

/**
 * CSS suffix for the `ui-status-pill-*` / `ui-badge-*` families in
 * `app/globals.css`. `pending` shares the `info` treatment; `unknown` has its
 * own grey so a reading we do not have never renders as a healthy one.
 */
const tonePillClass: Record<ForgeTone, string> = {
  ok: "success", warn: "warning", danger: "danger", info: "info",
  pending: "info", neutral: "neutral", unknown: "unknown",
};

export function StatusPill({ children, tone = "neutral", pulse = false }: { children: ReactNode; tone?: Tone; pulse?: boolean }) {
  return (
    <span className={cn("ui-status-pill", `ui-status-pill-${tonePillClass[resolveTone(tone)]}`)}>
      <span aria-hidden="true" className={cn("h-1.5 w-1.5 rounded-full bg-current", pulse && "animate-pulse")} />
      {children}
    </span>
  );
}

/**
 * Coloured dot plus the status word, with no pill chrome.
 *
 * For dense rows where a full {@link StatusPill} is too heavy. `tone` is
 * derived from `status` unless given explicitly, so passing a raw status string
 * off the wire is enough — one it does not recognise reads as unknown.
 */
export function StatusDot({ status, tone, className }: { status: string; tone?: Tone; className?: string }) {
  const style = toneStyles[resolveTone(tone ?? status)];
  return (
    <span className={cn("inline-flex items-center gap-1.5 text-xs font-semibold capitalize", style.fg, className)}>
      <span aria-hidden="true" className={cn("h-1.5 w-1.5 rounded-full", style.dot)} />
      {status}
    </span>
  );
}

export function SearchInput({ label = "Search", className, ...props }: InputHTMLAttributes<HTMLInputElement> & { label?: string }) {
  return (
    <label className={cn("relative block", className)}>
      <span className="sr-only">{label}</span>
      <Search aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-muted" />
      <InputBase {...props} className="pl-9" type="search" />
    </label>
  );
}

/**
 * Legacy switch. Kept for the two admin call sites that pass
 * `onCheckedChange`; new code must use `ForgeSwitch` from
 * `@/components/ui/forge`, which is the single switch implementation.
 *
 * Both `onChange` and the legacy `onCheckedChange` are accepted so call sites
 * can migrate to the `ForgeSwitch` prop naming (`onChange`) without a flag
 * day; when both are given `onChange` wins.
 *
 * @deprecated Use `ForgeSwitch` (`@/components/ui/forge`) for new code.
 */
export function Switch({ checked, onCheckedChange, onChange, label, disabled = false }: { checked: boolean; onCheckedChange?: (checked: boolean) => void; onChange?: (checked: boolean) => void; label: ReactNode; disabled?: boolean }) {
  const id = useId();
  const labelId = `${id}-label`;
  const handleChange = onChange ?? onCheckedChange ?? (() => undefined);
  return (
    <div className={cn("inline-flex items-center gap-2.5 text-xs", disabled ? "cursor-not-allowed opacity-50" : "cursor-pointer")}>
      <span className="t-eyebrow text-text-subtle" id={labelId}>{label}</span>
      <button aria-checked={checked} aria-labelledby={labelId} className={cn("relative h-5 w-9 rounded-full border transition-colors", checked ? "border-transparent bg-brand" : "border-line bg-overlay-strong")} disabled={disabled} id={id} onClick={() => handleChange(!checked)} role="switch" type="button">
        <span className={cn("absolute left-0.5 top-0.5 h-4 w-4 rounded-full bg-white shadow transition-transform", checked && "translate-x-4")} />
      </button>
    </div>
  );
}

/**
 * Horizontal fill bar.
 *
 * A non-numeric or non-finite `value` is an absent reading, not zero: the bar
 * renders an empty dashed track with `aria-valuenow` omitted, so assistive tech
 * and the eye both see "no reading" rather than "0%". Filling it to 0% would
 * claim a measurement that was never taken.
 *
 * Pass `tone` when the bar's meaning comes from a status rather than from how
 * full it is (a failed pipeline run is red at 40%). Without it the fill is
 * derived from `alarmAt`.
 */
export function ProgressBar({ value, label, className, alarmAt = 90, tone }: { value?: number | null; label: string; className?: string; alarmAt?: number; tone?: Tone }) {
  const known = typeof value === "number" && Number.isFinite(value);
  if (!known) {
    return (
      <div
        aria-label={`${label}: no reading available`}
        aria-valuemax={100}
        aria-valuemin={0}
        className={cn("h-1.5 rounded-full border border-dashed border-unknown-line bg-unknown-subtle", className)}
        role="progressbar"
        title="No reading available"
      />
    );
  }
  const percent = Math.min(100, Math.max(0, value));
  const alarm = percent >= alarmAt;
  const fill = tone ? toneStyles[resolveTone(tone)].dot : alarm ? "bg-danger" : "bg-ok";
  return (
    <div aria-label={label} aria-valuemax={100} aria-valuemin={0} aria-valuenow={Math.round(percent)} className={cn("h-1.5 overflow-hidden rounded-full bg-overlay-strong", className)} role="progressbar">
      <div className={cn("h-full rounded-full transition-all", fill)} style={{ width: `${percent}%` }} />
    </div>
  );
}

/**
 * Usage readout for a resource with a limit.
 *
 * Four states are kept distinct, because collapsing any two of them would
 * assert something we were not told:
 *
 * - no `limit` reading at all → "Unavailable" (callers that have no usage data
 *   pass nothing and rely on this — see `app/servers/page.tsx`)
 * - `limit === 0` → "Unlimited" (Forge's convention for an unbounded quota)
 * - a real limit but no `current` reading → unknown track and a dash, never 0%
 * - both readings present → a percentage
 */
export function ResourceBar({ icon: Icon, label, current, limit, unit = "" }: { icon: LucideIcon; label: string; current?: number | null; limit?: number | null; unit?: string }) {
  const limitKnown = typeof limit === "number" && Number.isFinite(limit) && limit >= 0;
  const unlimited = limitKnown && limit === 0;
  const max = limitKnown && !unlimited ? (limit as number) : null;
  const used = typeof current === "number" && Number.isFinite(current) ? current : null;
  const percent = max !== null && used !== null ? Math.min(100, (used / max) * 100) : null;
  const alarm = percent !== null && percent >= 90;
  const limitText = !limitKnown ? "unavailable" : unlimited ? "Unlimited" : `${max}${unit}`;
  return (
    <div className="flex items-center gap-2" title={`${label}: ${used === null ? "no reading" : `${used}${unit}`} / ${limitText}`}>
      <Icon aria-hidden="true" className={cn("h-3.5 w-3.5 shrink-0", alarm ? "text-danger" : "text-text-muted")} />
      {!limitKnown ? (
        <span className="text-meta text-text-muted">Unavailable</span>
      ) : unlimited ? (
        <span className="text-meta text-text-muted">Unlimited</span>
      ) : (
        <div className="flex flex-1 items-center gap-2">
          <ProgressBar className="flex-1" label={`${label} usage`} value={percent} />
          {percent === null ? (
            <span className="min-w-[3ch] text-right font-mono text-meta text-unknown" title="No reading available">—</span>
          ) : (
            <span className={cn("min-w-[3ch] text-right font-mono text-meta", alarm ? "font-semibold text-danger" : "text-text-subtle")}>{Math.round(percent)}%</span>
          )}
        </div>
      )}
    </div>
  );
}

export function Table({ children, label, className }: { children: ReactNode; label: string; className?: string }) {
  return (
    <div className="overflow-x-auto">
      <table aria-label={label} className={cn("ui-table", className)}>{children}</table>
    </div>
  );
}

export function Pagination({ page, pageCount, onPageChange, label = "Pagination" }: { page: number; pageCount: number; onPageChange: (page: number) => void; label?: string }) {
  const current = Math.min(Math.max(page, 1), Math.max(pageCount, 1));
  if (pageCount <= 1) return null;
  return (
    <nav aria-label={label} className="mt-4 flex items-center justify-between rounded-lg border border-line bg-overlay-subtle px-2.5 py-2">
      <Button disabled={current <= 1} onClick={() => onPageChange(current - 1)} size="sm" variant="ghost"><ChevronLeft className="size-3.5" />Previous</Button>
      <span className="text-meta text-text-subtle">Page {current} of {pageCount}</span>
      <Button disabled={current >= pageCount} onClick={() => onPageChange(current + 1)} size="sm" variant="ghost">Next<ChevronRight className="size-3.5" /></Button>
    </nav>
  );
}

export function CopyButton({ value, label = "Copy", className }: { value: string; label?: string; className?: string }) {
  const { toast } = useToast();
  const [copied, setCopied] = useState(false);
  return (
    <Button className={className} onClick={async () => {
      try { await navigator.clipboard.writeText(value); setCopied(true); window.setTimeout(() => setCopied(false), 1500); toast({ tone: "success", title: "Copied to clipboard" }); }
      catch { toast({ tone: "error", title: "Could not copy", message: "Select and copy the value manually." }); }
    }} type="button" variant="secondary">
      {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
      {copied ? "Copied" : label}
    </Button>
  );
}

export function EmptyState({ icon, title, description, action }: { icon?: ReactNode; title: string; description: string; action?: ReactNode }) {
  return (
    <div className="ui-empty">
      <div className="ui-empty-icon">{icon}</div>
      <h3 className="t-section mt-3">{title}</h3>
      <p className="mt-1 max-w-prose text-xs leading-5 text-text-subtle">{description}</p>
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  );
}

/**
 * Modal shell. Delegates to {@link ForgeDialog}, so this and every other Forge
 * dialog share one focus trap, one scroll lock, one Escape/backdrop contract
 * and one set of chrome. `closeAction` is kept as the prop name because ~40 call
 * sites pass it.
 */
export function Dialog({ open, title, description, children, closeAction, className }: {
  open: boolean; title: ReactNode; description?: string; children: ReactNode; closeAction: () => void; className?: string;
}) {
  return (
    <ForgeDialog className={className} description={description} onClose={closeAction} open={open} title={title}>
      {children}
    </ForgeDialog>
  );
}

export function ConfirmDialog({ open, title, description, confirmLabel = "Confirm", destructive = false, loading = false, closeAction, confirmAction }: {
  open: boolean; title: string; description: string; confirmLabel?: string; destructive?: boolean; loading?: boolean; closeAction: () => void; confirmAction: () => void;
}) {
  // While the mutation is in flight the dialog must stay put: Escape and
  // backdrop clicks are disabled so the operator cannot orphan a pending
  // mutation and double-submit on retry (mirrors ForgeConfirmDialog).
  const dismiss = loading ? () => undefined : closeAction;
  return (
    <ForgeDialog
      description={description}
      footer={
        <>
          <Button disabled={loading} onClick={closeAction} variant="ghost">Cancel</Button>
          <Button loading={loading} onClick={confirmAction} variant={destructive ? "danger" : "primary"}>{confirmLabel}</Button>
        </>
      }
      hideClose={loading}
      onClose={dismiss}
      open={open}
      size="sm"
      static={loading}
      title={title}
    />
  );
}
