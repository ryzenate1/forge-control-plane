"use client";

import React, {
  forwardRef,
  useId,
  useState,
  useEffect,
  useRef,
  type ReactNode,
  type ButtonHTMLAttributes,
  type InputHTMLAttributes,
  type SelectHTMLAttributes,
  type TextareaHTMLAttributes,
} from "react";
import {
  AlertCircle,
  AlertTriangle,
  ArrowLeft,
  Check,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Copy,
  ExternalLink,
  HelpCircle,
  Info,
  Loader2,
  RefreshCw,
  Search,
  X,
  type LucideIcon,
} from "lucide-react";
import { cn } from "@/lib/utils";

// ============================================================================
// 1. PAGE LAYOUT & HEADERS
// ============================================================================

export interface ForgePageProps {
  children: ReactNode;
  className?: string;
  maxWidth?: "default" | "wide" | "full";
}

export function ForgePage({
  children,
  className,
  maxWidth = "default",
}: ForgePageProps) {
  const maxClass = {
    default: "max-w-[1440px]",
    wide: "max-w-[1680px]",
    full: "max-w-full",
  }[maxWidth];

  return (
    <div className={cn("mx-auto w-full px-4 py-6 sm:px-6 lg:px-8 space-y-6", maxClass, className)}>
      {children}
    </div>
  );
}

export interface ForgePageHeaderProps {
  title: string;
  description?: string;
  badge?: ReactNode;
  actions?: ReactNode;
  backAction?: () => void;
  backLabel?: string;
  breadcrumb?: ReactNode;
  className?: string;
}

export function ForgePageHeader({
  title,
  description,
  badge,
  actions,
  backAction,
  backLabel = "Back",
  breadcrumb,
  className,
}: ForgePageHeaderProps) {
  return (
    <header className={cn("border-b border-line pb-5 space-y-3", className)}>
      {(backAction || breadcrumb) && (
        <nav aria-label="Breadcrumbs" className="flex items-center gap-2 text-xs text-text-subtle">
          {backAction && (
            <button
              type="button"
              onClick={backAction}
              className="inline-flex items-center gap-1.5 font-medium text-text-subtle transition-colors hover:text-text focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-focus"
            >
              <ArrowLeft className="h-3.5 w-3.5" />
              <span>{backLabel}</span>
            </button>
          )}
          {backAction && breadcrumb && <span className="text-line-strong">/</span>}
          {breadcrumb && <div className="flex items-center gap-2">{breadcrumb}</div>}
        </nav>
      )}

      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-1 min-w-0">
          <div className="flex items-center gap-3 flex-wrap">
            <h1 className="text-xl sm:text-2xl font-bold tracking-tight text-text">
              {title}
            </h1>
            {badge}
          </div>
          {description && (
            <p className="text-sm leading-relaxed text-text-subtle max-w-3xl">
              {description}
            </p>
          )}
        </div>

        {actions && (
          <div className="flex shrink-0 flex-wrap items-center gap-2 pt-1 sm:pt-0">
            {actions}
          </div>
        )}
      </div>
    </header>
  );
}

// ============================================================================
// 2. SECTIONS & CONTEXTUAL INFO DISCLOSURES
// ============================================================================

export interface ForgeSectionProps {
  title?: string;
  description?: string;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
}

export function ForgeSection({
  title,
  description,
  action,
  children,
  className,
}: ForgeSectionProps) {
  return (
    <section className={cn("space-y-3.5", className)}>
      {(title || action) && (
        <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
          <div>
            {title && <h2 className="text-sm font-semibold text-text">{title}</h2>}
            {description && <p className="text-xs text-text-subtle leading-5">{description}</p>}
          </div>
          {action && <div className="shrink-0">{action}</div>}
        </div>
      )}
      {children}
    </section>
  );
}

export interface ForgeInfoProps {
  title: string;
  summary: string;
  details?: ReactNode;
  docHref?: string;
  className?: string;
}

export function ForgeInfo({
  title,
  summary,
  details,
  docHref,
  className,
}: ForgeInfoProps) {
  const [open, setOpen] = useState(false);

  return (
    <div
      className={cn(
        "rounded-lg border border-line bg-surface/60 p-3.5 text-xs text-text-subtle transition-colors",
        open && "bg-surface border-line-strong",
        className
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-start gap-2.5 min-w-0 flex-1">
          <Info className="h-4 w-4 shrink-0 text-info mt-0.5" aria-hidden="true" />
          <div className="min-w-0">
            <span className="font-semibold text-text mr-1.5">{title}:</span>
            <span className="leading-relaxed">{summary}</span>
          </div>
        </div>

        <div className="flex items-center gap-2 shrink-0">
          {docHref && (
            <a
              href={docHref}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 text-[11px] font-medium text-text-subtle hover:text-text transition-colors"
            >
              <span>Docs</span>
              <ExternalLink className="h-3 w-3" />
            </a>
          )}
          {details && (
            <button
              type="button"
              onClick={() => setOpen(!open)}
              className="inline-flex items-center gap-1 text-[11px] font-medium text-text hover:text-brand transition-colors"
              aria-expanded={open}
            >
              <span>{open ? "Less" : "Explain"}</span>
              <ChevronDown className={cn("h-3 w-3 transition-transform", open && "rotate-180")} />
            </button>
          )}
        </div>
      </div>

      {open && details && (
        <div className="mt-3 border-t border-line/60 pt-2.5 text-text leading-relaxed">
          {details}
        </div>
      )}
    </div>
  );
}

// ============================================================================
// 3. CARDS & PANELS
// ============================================================================

export interface ForgeCardProps {
  title?: ReactNode;
  subtitle?: ReactNode;
  icon?: LucideIcon;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
  contentClassName?: string;
  tone?: "default" | "raised" | "ghost";
}

export function ForgeCard({
  title,
  subtitle,
  icon: Icon,
  action,
  children,
  className,
  contentClassName,
  tone = "default",
}: ForgeCardProps) {
  const bg = {
    default: "bg-surface border-line",
    raised: "bg-surface-raised border-line-strong",
    ghost: "bg-transparent border-line/50",
  }[tone];

  return (
    <div className={cn("rounded-xl border shadow-card transition-shadow", bg, className)}>
      {(title || action) && (
        <div className="flex items-center justify-between gap-3 border-b border-line/80 px-4 py-3 sm:px-5">
          <div className="flex items-center gap-2.5 min-w-0">
            {Icon && <Icon className="h-4 w-4 shrink-0 text-text-subtle" aria-hidden="true" />}
            <div>
              {typeof title === "string" ? (
                <h3 className="text-xs font-semibold uppercase tracking-wider text-text">{title}</h3>
              ) : (
                title
              )}
              {subtitle && <p className="text-[11px] text-text-subtle mt-0.5">{subtitle}</p>}
            </div>
          </div>
          {action && <div className="shrink-0">{action}</div>}
        </div>
      )}
      <div className={cn("p-4 sm:p-5", contentClassName)}>{children}</div>
    </div>
  );
}

export const ForgePanel = ForgeCard;

// ============================================================================
// 4. METRICS & OPERATIONAL TELEMETRY
// ============================================================================

export interface ForgeMetricProps {
  label: string;
  value: ReactNode;
  unit?: string;
  context?: ReactNode;
  freshness?: string;
  status?: "healthy" | "warning" | "danger" | "neutral" | "stale" | "unknown";
  icon?: LucideIcon;
  className?: string;
}

export function ForgeMetric({
  label,
  value,
  unit,
  context,
  freshness,
  status = "neutral",
  icon: Icon,
  className,
}: ForgeMetricProps) {
  const statusColor = {
    healthy: "text-emerald-400",
    warning: "text-amber-400",
    danger: "text-rose-400",
    neutral: "text-text",
    stale: "text-text-subtle opacity-75",
    unknown: "text-text-subtle",
  }[status];

  return (
    <div className={cn("rounded-xl border border-line bg-surface p-4 shadow-card space-y-2", className)}>
      <div className="flex items-center justify-between text-xs text-text-subtle">
        <div className="flex items-center gap-2">
          {Icon && <Icon className="h-3.5 w-3.5 text-text-subtle" />}
          <span className="font-medium tracking-wide uppercase text-[11px]">{label}</span>
        </div>
        {freshness && (
          <span className="text-[10px] text-text-subtle/80 font-mono" title="Last observed freshness">
            {freshness}
          </span>
        )}
      </div>

      <div className="flex items-baseline gap-1.5">
        <span className={cn("font-mono text-2xl font-bold tracking-tight", statusColor)}>
          {value}
        </span>
        {unit && <span className="text-xs text-text-subtle font-mono">{unit}</span>}
      </div>

      {context && <div className="text-xs text-text-subtle pt-1 border-t border-line/40">{context}</div>}
    </div>
  );
}

// ============================================================================
// 5. TRUTHFUL STATUS & BADGES
// ============================================================================

export type ForgeStatusType =
  | "healthy"
  | "running"
  | "stopped"
  | "degraded"
  | "unhealthy"
  | "offline"
  | "deploying"
  | "queued"
  | "retrying"
  | "failed"
  | "unknown"
  | "stale"
  | "connection_lost";

export interface ForgeStatusProps {
  status: ForgeStatusType | string;
  desired?: string;
  actual?: string;
  freshness?: string;
  showText?: boolean;
  className?: string;
}

export function ForgeStatus({
  status,
  desired,
  actual,
  freshness,
  showText = true,
  className,
}: ForgeStatusProps) {
  const normalized = String(status).toLowerCase();

  let dotColor = "bg-slate-400";
  let label = status;

  if (["healthy", "running", "live", "active", "done", "succeeded"].includes(normalized)) {
    dotColor = "bg-emerald-400";
    label = "Running";
  } else if (["degraded", "warning", "awaiting_health", "queued", "pending"].includes(normalized)) {
    dotColor = "bg-amber-400";
  } else if (["failed", "unhealthy", "error", "crash"].includes(normalized)) {
    dotColor = "bg-rose-500";
  } else if (["deploying", "building", "cloning", "starting"].includes(normalized)) {
    dotColor = "bg-sky-400 animate-pulse";
  } else if (["stopped", "offline", "canceled", "cancelled"].includes(normalized)) {
    dotColor = "bg-slate-500";
  } else if (["unknown", "stale", "connection_lost", "unavailable"].includes(normalized)) {
    dotColor = "bg-zinc-600";
    label = normalized === "connection_lost" ? "Connection Lost" : "Unknown";
  }

  return (
    <span
      className={cn(
        "inline-flex items-center gap-2 rounded-full border border-line bg-surface-raised/80 px-2.5 py-0.5 text-xs font-medium text-text",
        className
      )}
      title={
        desired && actual
          ? `Desired: ${desired} | Actual: ${actual}${freshness ? ` (${freshness})` : ""}`
          : freshness
          ? `Observed: ${freshness}`
          : undefined
      }
    >
      <span className={cn("h-2 w-2 rounded-full shrink-0", dotColor)} aria-hidden="true" />
      {showText && <span className="capitalize">{label}</span>}
      {desired && actual && desired !== actual && (
        <span className="text-[10px] text-text-subtle font-mono border-l border-line pl-1.5">
          {actual} ➔ {desired}
        </span>
      )}
    </span>
  );
}

export interface ForgeBadgeProps {
  children: ReactNode;
  tone?: "neutral" | "success" | "warning" | "danger" | "info" | "brand";
  size?: "sm" | "md";
  className?: string;
}

export function ForgeBadge({
  children,
  tone = "neutral",
  size = "md",
  className,
}: ForgeBadgeProps) {
  const tones = {
    neutral: "border-line bg-surface-raised text-text-subtle",
    success: "border-emerald-500/30 bg-emerald-950/30 text-emerald-300",
    warning: "border-amber-500/30 bg-amber-950/30 text-amber-300",
    danger: "border-rose-500/30 bg-rose-950/30 text-rose-300",
    info: "border-sky-500/30 bg-sky-950/30 text-sky-300",
    brand: "border-red-500/30 bg-red-950/30 text-red-300",
  }[tone];

  const sizes = {
    sm: "px-2 py-0.5 text-[10px]",
    md: "px-2.5 py-1 text-xs",
  }[size];

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-md border font-medium font-mono uppercase tracking-wider",
        tones,
        sizes,
        className
      )}
    >
      {children}
    </span>
  );
}

// ============================================================================
// 6. BUTTONS & INTERACTIVE CONTROLS
// ============================================================================

export interface ForgeButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: "primary" | "secondary" | "danger" | "ghost" | "outline";
  size?: "sm" | "md" | "lg";
  loading?: boolean;
  icon?: LucideIcon;
  iconPosition?: "left" | "right";
}

export const ForgeButton = forwardRef<HTMLButtonElement, ForgeButtonProps>(
  function ForgeButton(
    {
      children,
      variant = "primary",
      size = "md",
      loading = false,
      disabled,
      icon: Icon,
      iconPosition = "left",
      className,
      type = "button",
      ...props
    },
    ref
  ) {
    const variants = {
      primary:
        "bg-brand text-white border-brand hover:bg-brand-hover shadow-sm shadow-brand/20 active:translate-y-px",
      secondary:
        "bg-surface-raised border-line text-text hover:bg-surface-hover hover:border-line-strong active:translate-y-px",
      danger:
        "bg-rose-600/10 border-rose-500/30 text-rose-300 hover:bg-rose-600/20 active:translate-y-px",
      ghost:
        "bg-transparent border-transparent text-text-subtle hover:text-text hover:bg-white/[0.06]",
      outline:
        "bg-transparent border-line text-text hover:bg-surface-raised hover:border-line-strong",
    }[variant];

    const sizes = {
      sm: "h-8 px-3 text-xs gap-1.5 rounded-lg",
      md: "h-9 px-4 text-xs font-semibold gap-2 rounded-lg",
      lg: "h-11 px-5 text-sm font-semibold gap-2.5 rounded-xl",
    }[size];

    return (
      <button
        ref={ref}
        type={type}
        disabled={disabled || loading}
        aria-busy={loading || undefined}
        className={cn(
          "inline-flex items-center justify-center border font-medium transition-[background-color,border-color,box-shadow,transform] duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus disabled:pointer-events-none disabled:opacity-50",
          variants,
          sizes,
          className
        )}
        {...props}
      >
        {loading ? (
          <Loader2 className="h-4 w-4 animate-spin shrink-0" aria-hidden="true" />
        ) : Icon && iconPosition === "left" ? (
          <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
        ) : null}
        <span>{children}</span>
        {!loading && Icon && iconPosition === "right" ? (
          <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
        ) : null}
      </button>
    );
  }
);

export interface ForgeIconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  icon: LucideIcon;
  label: string;
  variant?: "ghost" | "secondary" | "danger";
  size?: "sm" | "md";
}

export const ForgeIconButton = forwardRef<HTMLButtonElement, ForgeIconButtonProps>(
  function ForgeIconButton(
    { icon: Icon, label, variant = "ghost", size = "md", className, disabled, ...props },
    ref
  ) {
    const variants = {
      ghost: "text-text-subtle hover:text-text hover:bg-white/[0.06] border-transparent",
      secondary: "bg-surface-raised border-line text-text hover:bg-surface-hover hover:border-line-strong",
      danger: "text-rose-400 hover:bg-rose-500/10 border-transparent",
    }[variant];

    const sizes = {
      sm: "h-7 w-7 rounded-md",
      md: "h-9 w-9 rounded-lg",
    }[size];

    return (
      <button
        ref={ref}
        type="button"
        title={label}
        aria-label={label}
        disabled={disabled}
        className={cn(
          "inline-grid place-items-center border transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus disabled:pointer-events-none disabled:opacity-40",
          variants,
          sizes,
          className
        )}
        {...props}
      >
        <Icon className="h-4 w-4" aria-hidden="true" />
      </button>
    );
  }
);

// ============================================================================
// 7. FORM INPUTS & TEXT AREAS
// ============================================================================

export interface ForgeInputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  hint?: ReactNode;
  error?: string;
  mono?: boolean;
}

export const ForgeInput = forwardRef<HTMLInputElement, ForgeInputProps>(
  function ForgeInput({ label, hint, error, mono, id: providedId, className, ...props }, ref) {
    const generatedId = useId();
    const id = providedId || generatedId;

    return (
      <div className="space-y-1.5 w-full">
        {label && (
          <label htmlFor={id} className="block text-xs font-semibold uppercase tracking-wider text-text-subtle">
            {label}
          </label>
        )}
        <input
          ref={ref}
          id={id}
          className={cn(
            "block h-9 w-full rounded-lg border border-line bg-surface-input px-3 text-xs text-text shadow-inner shadow-black/10 outline-none transition placeholder:text-text-subtle/50 hover:border-line-strong focus:border-brand focus:ring-1 focus:ring-brand disabled:cursor-not-allowed disabled:opacity-50",
            mono && "font-mono",
            error && "border-rose-500/70 focus:border-rose-500 focus:ring-rose-500",
            className
          )}
          aria-invalid={error ? "true" : undefined}
          aria-describedby={error ? `${id}-error` : hint ? `${id}-hint` : undefined}
          {...props}
        />
        {error ? (
          <p id={`${id}-error`} role="alert" className="text-xs text-rose-400">
            {error}
          </p>
        ) : hint ? (
          <p id={`${id}-hint`} className="text-xs text-text-subtle">
            {hint}
          </p>
        ) : null}
      </div>
    );
  }
);

export interface ForgeSelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  label?: string;
  hint?: ReactNode;
  error?: string;
}

export const ForgeSelect = forwardRef<HTMLSelectElement, ForgeSelectProps>(
  function ForgeSelect({ label, hint, error, id: providedId, className, children, ...props }, ref) {
    const generatedId = useId();
    const id = providedId || generatedId;

    return (
      <div className="space-y-1.5 w-full">
        {label && (
          <label htmlFor={id} className="block text-xs font-semibold uppercase tracking-wider text-text-subtle">
            {label}
          </label>
        )}
        <div className="relative">
          <select
            ref={ref}
            id={id}
            className={cn(
              "block h-9 w-full appearance-none rounded-lg border border-line bg-surface-input px-3 pr-8 text-xs text-text outline-none transition hover:border-line-strong focus:border-brand focus:ring-1 focus:ring-brand disabled:cursor-not-allowed disabled:opacity-50",
              error && "border-rose-500/70 focus:border-rose-500",
              className
            )}
            aria-invalid={error ? "true" : undefined}
            {...props}
          >
            {children}
          </select>
          <ChevronDown className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-text-subtle" />
        </div>
        {error && <p className="text-xs text-rose-400">{error}</p>}
        {hint && !error && <p className="text-xs text-text-subtle">{hint}</p>}
      </div>
    );
  }
);

// ============================================================================
// 8. CODE & TECHNICAL VALUES
// ============================================================================

export interface ForgeCodeProps {
  children: string;
  block?: boolean;
  copyable?: boolean;
  className?: string;
}

export function ForgeCode({ children, block = false, copyable = false, className }: ForgeCodeProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    navigator.clipboard.writeText(children);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  if (!block) {
    return (
      <span
        className={cn(
          "inline-flex items-center gap-1.5 rounded bg-surface-raised px-1.5 py-0.5 font-mono text-[11px] text-text border border-line/60",
          className
        )}
      >
        <span>{children}</span>
        {copyable && (
          <button
            type="button"
            onClick={handleCopy}
            className="text-text-subtle hover:text-text transition-colors"
            title={copied ? "Copied" : "Copy"}
            aria-label="Copy code value"
          >
            {copied ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
          </button>
        )}
      </span>
    );
  }

  return (
    <div className={cn("relative rounded-lg border border-line bg-surface-input p-3 font-mono text-xs text-text", className)}>
      <pre className="overflow-x-auto whitespace-pre">{children}</pre>
      {copyable && (
        <button
          type="button"
          onClick={handleCopy}
          className="absolute right-2.5 top-2.5 inline-flex items-center gap-1 rounded border border-line bg-surface-raised px-2 py-1 text-[10px] text-text-subtle hover:text-text"
        >
          {copied ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
          <span>{copied ? "Copied" : "Copy"}</span>
        </button>
      )}
    </div>
  );
}

// ============================================================================
// 9. DIALOGS & CONFIRMATION
// ============================================================================

export interface ForgeDialogProps {
  open: boolean;
  onClose: () => void;
  title: string;
  description?: string;
  children: ReactNode;
  actions?: ReactNode;
  maxWidth?: "sm" | "md" | "lg" | "xl";
}

export function ForgeDialog({
  open,
  onClose,
  title,
  description,
  children,
  actions,
  maxWidth = "md",
}: ForgeDialogProps) {
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && open) {
        onClose();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [open, onClose]);

  if (!open) return null;

  const maxW = {
    sm: "max-w-sm",
    md: "max-w-md",
    lg: "max-w-lg",
    xl: "max-w-2xl",
  }[maxWidth];

  return (
    <div className="fixed inset-0 z-50 grid place-items-center overflow-y-auto bg-black/70 p-4 backdrop-blur-md sm:p-6">
      <div
        className="fixed inset-0 bg-black/70 backdrop-blur-md transition-opacity"
        onClick={onClose}
        aria-hidden="true"
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="dialog-title"
        className={cn(
          "relative flex max-h-[88vh] w-full flex-col overflow-hidden rounded-2xl border border-[var(--line)] bg-[var(--surface-raised)] shadow-[var(--shadow-dialog)]",
          maxW
        )}
      >
        <div className="flex items-start justify-between gap-4 border-b border-[var(--line)] bg-white/[0.015] px-6 pb-4 pt-5">
          <div className="min-w-0">
            <h2 id="dialog-title" className="text-[15px] font-bold tracking-tight text-[var(--text)]">
              {title}
            </h2>
            {description && (
              <p className="mt-1 text-[13px] leading-5 text-[var(--text-subtle)]">{description}</p>
            )}
          </div>
          <button
            type="button"
            onClick={onClose}
            className="ui-icon-button shrink-0"
            aria-label="Close dialog"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5 text-sm leading-6 text-[var(--text)]">{children}</div>

        {actions && (
          <div className="flex items-center justify-end gap-2 border-t border-[var(--line)] bg-white/[0.015] px-6 py-4">
            {actions}
          </div>
        )}
      </div>
    </div>
  );
}

export interface ForgeConfirmDialogProps {
  open: boolean;
  onClose: () => void;
  onConfirm: () => void | Promise<void>;
  title: string;
  description: string;
  confirmLabel?: string;
  cancelLabel?: string;
  tone?: "danger" | "primary";
  loading?: boolean;
}

export function ForgeConfirmDialog({
  open,
  onClose,
  onConfirm,
  title,
  description,
  confirmLabel = "Confirm",
  cancelLabel = "Cancel",
  tone = "primary",
  loading = false,
}: ForgeConfirmDialogProps) {
  return (
    <ForgeDialog
      open={open}
      onClose={onClose}
      title={title}
      description={description}
      maxWidth="sm"
      actions={
        <>
          <ForgeButton variant="ghost" onClick={onClose} disabled={loading}>
            {cancelLabel}
          </ForgeButton>
          <ForgeButton
            variant={tone === "danger" ? "danger" : "primary"}
            onClick={onConfirm}
            loading={loading}
          >
            {confirmLabel}
          </ForgeButton>
        </>
      }
    >
      <div className="py-1" />
    </ForgeDialog>
  );
}

// ============================================================================
// 10. EMPTY, LOADING & ERROR STATES
// ============================================================================

export interface ForgeEmptyStateProps {
  icon?: LucideIcon;
  title: string;
  description: string;
  action?: ReactNode;
  className?: string;
}

export function ForgeEmptyState({
  icon: Icon = HelpCircle,
  title,
  description,
  action,
  className,
}: ForgeEmptyStateProps) {
  return (
    <div
      className={cn(
        "flex flex-col items-center justify-center rounded-xl border border-dashed border-line bg-surface/40 p-8 text-center",
        className
      )}
    >
      <div className="grid h-12 w-12 place-items-center rounded-full bg-surface-raised border border-line text-text-subtle mb-3">
        <Icon className="h-6 w-6" aria-hidden="true" />
      </div>
      <h3 className="text-sm font-semibold text-text">{title}</h3>
      <p className="mt-1 text-xs text-text-subtle max-w-sm leading-relaxed">{description}</p>
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}

export interface ForgeErrorStateProps {
  title?: string;
  message: string;
  onRetry?: () => void;
  className?: string;
}

export function ForgeErrorState({
  title = "Failed to load operational data",
  message,
  onRetry,
  className,
}: ForgeErrorStateProps) {
  return (
    <div
      role="alert"
      className={cn(
        "rounded-xl border border-rose-500/30 bg-rose-950/20 p-4 text-xs text-rose-200 flex items-start gap-3",
        className
      )}
    >
      <AlertTriangle className="h-5 w-5 text-rose-400 shrink-0 mt-0.5" />
      <div className="flex-1 space-y-1">
        <p className="font-semibold text-rose-100">{title}</p>
        <p className="text-rose-300/90 leading-relaxed font-mono text-[11px]">{message}</p>
      </div>
      {onRetry && (
        <ForgeButton size="sm" variant="danger" onClick={onRetry} icon={RefreshCw}>
          Retry
        </ForgeButton>
      )}
    </div>
  );
}

export function ForgeLoadingState({ message = "Loading operational telemetry..." }: { message?: string }) {
  return (
    <div className="flex flex-col items-center justify-center p-12 text-center space-y-3">
      <Loader2 className="h-6 w-6 animate-spin text-brand" />
      <p className="text-xs text-text-subtle font-mono">{message}</p>
    </div>
  );
}
