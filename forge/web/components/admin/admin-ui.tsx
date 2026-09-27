"use client";

/**
 * Shared admin UI primitives used across all admin panel sections.
 *
 * Every component here is a thin adapter over the canonical Forge system in
 * `components/ui/forge/*` and the `ui-*` classes in `app/globals.css`. Nothing
 * in this file picks its own geometry, colour or type scale: ~162 files import
 * these names, so the names and prop signatures are frozen while the rendering
 * delegates downward. Add new surfaces to `components/ui/forge`, not here.
 */

import { ArrowLeft, LockKeyhole, type LucideIcon } from "lucide-react";
import { usePathname } from "next/navigation";
import { useRef } from "react";
import {
  ForgeDrawer,
  ForgeGrid,
  ForgeIconButton,
  ForgeMetric,
  ForgeSparkline,
  ForgeSpinner,
} from "@/components/ui/forge";
import { resolveTone, toneStyles, type ToneInput } from "@/components/ui/forge/status";
import { Button, Dialog, EmptyState as SharedEmptyState, Input as SharedInput, Select as SharedSelect, Textarea as SharedTextarea } from "@/components/ui/primitives";
import { chart } from "@/lib/design-tokens";
import { cn } from "@/lib/utils";
import { findAdminPageGroup } from "./admin-registry";

export { cn };

/**
 * Historical tone names accepted by the admin primitives below. Colour words
 * (`green`) and meaning words (`success`) both appear across ~115 call sites, so
 * both keep working — but neither decides a colour here. Every one of them is
 * resolved through {@link resolveTone} onto the canonical Forge status
 * vocabulary in `components/ui/forge/status.ts`, which is the only place a
 * status colour is chosen. An unrecognised name resolves to `unknown` (grey,
 * dashed), never to `ok`.
 */
export type AdminTone = ToneInput;

export function Pill({ children, tone = "neutral", className }: { children: React.ReactNode; tone?: AdminTone; className?: string }) {
  return (
    <span className={cn("ui-badge", toneStyles[resolveTone(tone)].chip, className)}>
      {children}
    </span>
  );
}

/**
 * Small trend line for a header or table cell.
 *
 * Delegates to {@link ForgeSparkline}, which breaks the line at unreported
 * samples rather than drawing them as zero.
 */
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
  return <ForgeSparkline area className={className} color={color} height={34} points={data} strokeWidth={2} />;
}

/**
 * Labelled ratio bar for one subsystem.
 *
 * `percentage` is optional because a ratio is often genuinely unavailable — an
 * unreachable inventory, an empty one, a check that did not report. In that
 * case pass `null`/`undefined` and the track renders empty with a dashed edge.
 * It must not be given `0`, which claims a measured zero, and the defaults are
 * `unknown`/no-reading rather than the full green bar this component used to
 * draw for a caller that supplied nothing.
 */
export function SubsystemHealthMeter({
  label,
  valueText,
  icon: Icon,
  percentage,
  tone = "unknown",
  onClick,
}: {
  label: string;
  valueText: string;
  icon?: React.ElementType;
  percentage?: number | null;
  tone?: AdminTone;
  onClick?: () => void;
}) {
  const resolved = resolveTone(tone);
  const known = typeof percentage === "number" && Number.isFinite(percentage);
  const clampedPct = known ? Math.max(0, Math.min(100, percentage)) : null;

  return (
    <div
      className={cn(
        "flex items-center gap-3 text-xs",
        onClick && "-mx-1.5 cursor-pointer rounded-md px-1.5 py-1 transition-colors hover:bg-overlay-subtle focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
      )}
      onClick={onClick}
      onKeyDown={onClick ? (event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onClick();
        }
      } : undefined}
      role={onClick ? "button" : undefined}
      tabIndex={onClick ? 0 : undefined}
    >
      <div className="flex w-32 shrink-0 items-center gap-2 text-text-subtle">
        {Icon ? <Icon className="shrink-0 text-text-muted" size={14} /> : null}
        <span className="truncate font-medium">{label}</span>
      </div>
      <span className={cn("w-28 shrink-0 font-mono text-meta font-semibold", toneStyles[resolved].fg)}>
        {valueText}
      </span>
      {clampedPct === null ? (
        <div
          aria-label={`${label}: no reading available`}
          className="h-1.5 flex-1 rounded-full border border-dashed border-unknown-line bg-unknown-subtle"
          title="No reading available"
        />
      ) : (
        <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-overlay-strong">
          <div
            className={cn("h-full rounded-full transition-[width] duration-300", toneStyles[resolved].dot)}
            style={{ width: `${clampedPct}%` }}
          />
        </div>
      )}
    </div>
  );
}

export interface SectionHeaderProps {
  title: React.ReactNode;
  sub?: string;
  action?: React.ReactNode;
  breadcrumb?: React.ReactNode;
  hideBreadcrumb?: boolean;
  /**
   * Real freshness/provenance for the data on the page, rendered at the right
   * of the breadcrumb row.
   *
   * This slot replaced a hardcoded "Live · updated just now" badge that every
   * admin header rendered unconditionally — it claimed liveness for data that
   * may have been fetched minutes earlier, or have failed to load entirely.
   * Pass `<FreshnessBadge state={…}>` from `telemetry-ui` (which derives the
   * wording from the query's own `dataUpdatedAt`). A page with nothing truthful
   * to say here passes nothing and shows no badge.
   */
  status?: React.ReactNode;
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
  status,
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
          <div className="flex items-center gap-1.5">
            {parts.map((part, index) => {
              const isLast = index === parts.length - 1;
              return (
                <span className="flex items-center gap-1.5" key={index}>
                  {index > 0 && <span aria-hidden="true" className="select-none text-text-muted after:content-['/']" />}
                  <span className={cn(isLast ? "font-semibold text-text" : "text-text-muted")}>
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
    } else if (groupMatch && !pathname.startsWith("/admin")) {
      // Outside /admin nothing else renders a trail, so derive one.
      //
      // Inside /admin, `AdminShell` renders the full registry trail in the top
      // bar for every page. This header used to derive a second, shorter one
      // (group / page) a few pixels below it, so every admin screen showed two
      // breadcrumbs that disagreed on depth: the shell resolves aliases, walks
      // `parent` links for hidden pages and keeps deep sub-path segments, while
      // this one stopped at the group. One trail, from one source.
      breadcrumbContent = (
        <div className="flex items-center gap-1.5">
          <span>{groupMatch.groupTitle}</span>
          <span aria-hidden="true" className="select-none text-text-muted after:content-['/']" />
          <span className="font-semibold text-text">
            {groupMatch.pageLabel}
          </span>
        </div>
      );
    }
  }

  return (
    <div className={cn("space-y-2.5", className)}>
      {breadcrumbContent || status ? (
        <div className="t-meta flex items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            {backAction ? (
              <button
                className="inline-flex items-center gap-1.5 rounded border-r border-line pr-3 font-medium text-text-muted transition-colors hover:text-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
                onClick={backAction}
                type="button"
              >
                <ArrowLeft aria-hidden="true" size={13} />
                <span>{backLabel ?? "Back"}</span>
              </button>
            ) : null}
            {breadcrumbContent}
          </div>
          {status ? <div className="flex shrink-0 items-center gap-2">{status}</div> : null}
        </div>
      ) : backAction ? (
        <div className="flex items-center">
          <AdminBackButton label={backLabel} onClick={backAction} />
        </div>
      ) : null}

      <div className="flex flex-col gap-3 border-b border-line pb-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <h1 className="t-page flex min-w-0 items-center gap-2.5 break-words">
            {title}
          </h1>
          {sub ? (
            <p className="mt-1 max-w-prose text-xs leading-5 text-text-subtle">
              {sub}
            </p>
          ) : null}
        </div>
        {action ? (
          <div className="flex shrink-0 flex-wrap items-center gap-2">
            {action}
          </div>
        ) : null}
      </div>
    </div>
  );
}

/** Shared admin page frame. Use this for new and migrated screens rather than
 * rebuilding the header, action alignment, and responsive spacing per page.
 * Horizontal padding comes from `AdminShell`, so this only sets width and
 * vertical rhythm — the same rhythm `ForgePage` uses outside /admin. */
export function AdminPageLayout({ children, className }: { children: React.ReactNode; className?: string }) {
  return <div className={cn("mx-auto w-full max-w-page space-y-5", className)}>{children}</div>;
}

export function AdminBackButton({ onClick, label = "Back" }: { onClick: () => void; label?: string }) {
  return <button className="inline-flex items-center gap-1.5 text-xs font-medium text-text-subtle transition-colors hover:text-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" onClick={onClick} type="button"><ArrowLeft aria-hidden="true" size={14} />{label}</button>;
}

export function AdminPageHeader({
  title,
  description,
  action,
  backAction,
  backLabel,
  breadcrumb,
  hideBreadcrumb,
  status,
  className,
}: {
  title: string;
  description?: string;
  action?: React.ReactNode;
  backAction?: () => void;
  backLabel?: string;
  breadcrumb?: string;
  hideBreadcrumb?: boolean;
  status?: React.ReactNode;
  className?: string;
}) {
  return (
    <SectionHeader
      action={action}
      backAction={backAction}
      backLabel={backLabel}
      breadcrumb={breadcrumb}
      className={className}
      hideBreadcrumb={hideBreadcrumb}
      status={status}
      sub={description}
      title={title}
    />
  );
}

export function AdminSection({ children, title, description, action, className }: { children: React.ReactNode; title?: string; description?: string; action?: React.ReactNode; className?: string }) {
  return (
    <section className={cn("space-y-3", className)}>
      {title ? (
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 className="t-title">{title}</h2>
            {description ? <p className="mt-0.5 max-w-prose text-meta text-text-subtle">{description}</p> : null}
          </div>
          {action}
        </div>
      ) : null}
      {children}
    </section>
  );
}

export function Card({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <div className={cn("ui-card", className)}>
      {children}
    </div>
  );
}

export const AdminCard = Card;

/**
 * Header band for {@link Card}. The negative margins pull it back out to the
 * card's own `p-4 sm:p-5` padding so it reads as a bar, and the corner radius
 * matches `--radius` exactly — a rounder header on a square card was the most
 * visible seam in the old admin surfaces.
 */
export function CardHeader({ title, icon: Icon, action }: { title: string; icon?: React.ElementType; action?: React.ReactNode }) {
  return (
    <div className="ui-card-header -mx-4 -mt-4 mb-4 items-center rounded-t-lg text-xs font-semibold text-text sm:-mx-5 sm:-mt-5 sm:mb-5">
      {Icon ? <Icon aria-hidden="true" className="shrink-0 text-text-subtle" size={14} /> : null}
      {title}
      {action ? <div className="ml-auto flex items-center gap-2">{action}</div> : null}
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
  return <Button aria-label={ariaLabel} className={cn(tone === "warning" && toneStyles.warn.chip, tone === "success" && toneStyles.ok.chip, className)} disabled={disabled} loading={loading} onClick={onClick} size={size === "sm" ? "sm" : "default"} title={title} type={type} variant={variants[tone]}>{children}</Button>;
}

export function Input({ label, value, onChange, placeholder, type = "text", mono, required, readOnly, disabled, autoComplete }: {
  label?: string; value: string; onChange: (v: string) => void; placeholder?: string; type?: string; mono?: boolean; required?: boolean; readOnly?: boolean; disabled?: boolean; autoComplete?: string;
}) {
  return (
    <label className="block">
      {label ? <span className="ui-label mb-1.5">{label}</span> : null}
      <SharedInput
        autoComplete={autoComplete}
        className={cn(mono && "font-mono")}
        disabled={disabled}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        readOnly={readOnly}
        required={required}
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
    <label className="block">
      {label ? <span className="ui-label mb-1.5">{label}</span> : null}
      <SharedTextarea
        className="min-h-0 font-mono"
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        rows={rows}
        value={value}
      />
    </label>
  );
}

/**
 * Bare chip. Call sites supply their own tone classes, so this only carries the
 * canonical badge geometry; prefer {@link Pill} when the chip means a status.
 */
export function Badge({ children, className }: { children: React.ReactNode; className?: string }) {
  return <span className={cn("ui-badge border-transparent", className)}>{children}</span>;
}

export function Modal({ title, onClose, children, wide, className, description, maxWidth, open = true }: { title: React.ReactNode; onClose: () => void; children: React.ReactNode; wide?: boolean; className?: string; description?: string; maxWidth?: string; open?: boolean }) {
  return <Dialog className={cn(wide && "max-w-3xl", maxWidth && maxWidth, className)} closeAction={onClose} description={description} open={open} title={title}>{children}</Dialog>;
}

export function AdminSelect({ label, value, onChange, options, placeholder, mono, disabled }: {
  label?: string; value: string; onChange: (v: string) => void; options: Array<{ value: string; label: string }>; placeholder?: string; mono?: boolean; disabled?: boolean;
}) {
  return (
    <label className="block">
      {label ? <span className="ui-label mb-1.5">{label}</span> : null}
      <SharedSelect
        className={cn("w-full", mono && "font-mono")}
        disabled={disabled}
        onChange={(e) => onChange(e.target.value)}
        value={value}
      >
        {placeholder ? <option value="">{placeholder}</option> : null}
        {options.map((opt) => <option key={opt.value} value={opt.value}>{opt.label}</option>)}
      </SharedSelect>
    </label>
  );
}

export const AdminDialog = Modal;

export function AdminDrawer({ title, onClose, children }: { title: string; onClose: () => void; children: React.ReactNode }) {
  return <ForgeDrawer onClose={onClose} open title={title} width="lg">{children}</ForgeDrawer>;
}

export function AdminFormSection({ title, description, children }: { title: string; description?: string; children: React.ReactNode }) {
  return <fieldset className="space-y-3.5 border-b border-line pb-5 last:border-0 last:pb-0"><legend className="t-title">{title}</legend>{description ? <p className="-mt-1.5 max-w-prose text-meta text-text-subtle">{description}</p> : null}<div className="grid gap-3.5">{children}</div></fieldset>;
}

export function AdminFormField({ label, hint, error, children }: { label: string; hint?: string; error?: string; children: React.ReactNode }) {
  return <label className="block"><span className="ui-label mb-1.5">{label}</span>{children}{error ? <span className="ui-field-error mt-1.5 block">{error}</span> : hint ? <span className="ui-hint mt-1.5 block">{hint}</span> : null}</label>;
}

export function AdminToolbar({ children, className }: { children: React.ReactNode; className?: string }) { return <div className={cn("ui-toolbar sm:items-end", className)}>{children}</div>; }

export function AdminTable({ children, label, className }: { children: React.ReactNode; label?: string; className?: string }) {
  return <div className={cn("overflow-x-auto", className)}><table aria-label={label} className="ui-table">{children}</table></div>;
}
export function AdminTHead({ children }: { children?: React.ReactNode }) { return <thead><tr>{children}</tr></thead>; }
export function AdminTh({ children, className }: { children?: React.ReactNode; className?: string }) { return <th className={cn("ui-th text-left", className)}>{children}</th>; }
export function AdminTBody({ children }: { children?: React.ReactNode }) { return <tbody className="[&_tr:last-child_.ui-td]:border-0">{children}</tbody>; }
export function AdminTr({ children, onClick, className }: { children?: React.ReactNode; onClick?: () => void; className?: string }) {
  if (!onClick) return <tr className={cn(className)}>{children}</tr>;
  return (
    <tr
      className={cn("ui-tr-interactive focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[var(--focus)]", className)}
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
export function AdminTd({ children, className, title }: { children?: React.ReactNode; className?: string; title?: string }) { return <td className={cn("ui-td", className)} title={title}>{children}</td>; }

/** Class string for a bare `<select>`; identical to what `ForgeSelect` renders. */
export const selectStyle = "ui-input w-full cursor-pointer";

export function AdminLoadingState({ label = "Loading…" }: { label?: string }) {
  return (
    <div aria-busy="true" className="grid min-h-32 place-items-center rounded-lg border border-dashed border-line-strong bg-overlay-subtle p-6 text-xs text-text-subtle" role="status">
      <div className="flex items-center gap-2.5">
        <ForgeSpinner label={label} size="sm" />
        <span>{label}</span>
      </div>
    </div>
  );
}

export function AdminLoadingRows({ rows = 3, cols = 4, label = "Loading rows…" }: { rows?: number; cols?: number; label?: string }) {
  return <div aria-label={label} className="divide-y divide-line" role="status">{Array.from({ length: rows }, (_, i) => <div className="flex gap-3 px-3 py-2.5" key={i}>{Array.from({ length: cols }, (_, j) => <div className="ui-skeleton h-3.5 flex-1" key={j} />)}</div>)}</div>;
}

export function AdminErrorState({ message, retry }: { message: string; retry?: () => void }) { return <div className="ui-alert ui-alert-danger flex-wrap items-center justify-between gap-3" role="alert"><span>{message}</span>{retry ? <Btn onClick={retry} size="sm" tone="ghost">Retry</Btn> : null}</div>; }

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
  return <div aria-label={label} className="ui-tablist" role="tablist">
    {tabs.map((tab, index) => {
      const isActive = active === tab.id;
      const Icon = tab.icon;
      return <button
        aria-selected={isActive}
        className={cn("ui-tab", tab.danger && isActive && "border-danger text-danger")}
        key={tab.id}
        onClick={() => onChange(tab.id)}
        onKeyDown={(e) => handleTabKeyDown(e, index)}
        ref={(el) => { tabRefs.current[index] = el; }}
        role="tab"
        tabIndex={isActive ? 0 : -1}
        type="button"
      >
        {Icon ? <Icon aria-hidden="true" size={14} /> : null}
        {tab.label}
      </button>;
    })}
  </div>;
}

export function Kbd({ children }: { children: React.ReactNode }) {
  return <kbd className="ui-kbd">{children}</kbd>;
}

export function Separator({ className }: { className?: string }) {
  return <div className={cn("ui-separator", className)} role="separator" />;
}

export function DataTable({ headers, rows, label, className }: {
  headers: string[];
  rows: Array<Record<string, React.ReactNode>>;
  label: string;
  className?: string;
}) {
  return (
    <AdminTable className={className} label={label}>
      <AdminTHead>
        {headers.map((header) => <AdminTh key={header}>{header}</AdminTh>)}
      </AdminTHead>
      <AdminTBody>
        {rows.map((row, i) => (
          <AdminTr key={i}>
            {headers.map((header) => <AdminTd key={header}>{row[header] ?? <span className="text-text-muted">—</span>}</AdminTd>)}
          </AdminTr>
        ))}
      </AdminTBody>
    </AdminTable>
  );
}

export const AdminBadge = Pill;

export function AdminIconButton({ label, children, onClick, tone = "neutral", disabled }: { label: string; children: React.ReactNode; onClick: () => void; tone?: "neutral" | "danger"; disabled?: boolean }) {
  return <ForgeIconButton disabled={disabled} label={label} onClick={onClick} tone={tone === "danger" ? "danger" : "default"}>{children}</ForgeIconButton>;
}

export function AdminConfirmDialog({ title, description, confirmLabel = "Confirm", onCancel, onConfirm, loading = false, destructive = false, open = true }: { title: string; description: string; confirmLabel?: string; onCancel: () => void; onConfirm: () => void; loading?: boolean; destructive?: boolean; open?: boolean }) {
  return (
    <Modal description={description} onClose={onCancel} open={open} title={title}>
      <ModalFooter confirmLabel={confirmLabel} destructive={destructive} disabled={loading} onCancel={onCancel} onConfirm={onConfirm} />
    </Modal>
  );
}

/**
 * Footer bar for {@link Modal}. The negative margins cancel `.ui-dialog-body`'s
 * `px-5 py-4` so the bar meets the dialog edges, then `.ui-dialog-footer`
 * supplies the divider, tint and button alignment every Forge dialog uses.
 */
export function ModalFooter({ onCancel, onConfirm, confirmLabel = "Save", disabled, destructive = false }: {
  onCancel: () => void; onConfirm: () => void; confirmLabel?: string; disabled?: boolean; destructive?: boolean;
}) {
  return (
    <div className="ui-dialog-footer sticky bottom-0 z-10 -mx-5 -mb-4 mt-4">
      <Btn onClick={onCancel} tone="ghost">Cancel</Btn>
      <Btn disabled={disabled} loading={disabled} onClick={onConfirm} tone={destructive ? "danger" : "primary"}>{confirmLabel}</Btn>
    </div>
  );
}

export function EmptyState({ icon: Icon, message, title, sub }: { icon?: React.ElementType; message?: string; title?: string; sub?: string }) {
  return <SharedEmptyState description={message ?? sub ?? ""} icon={Icon ? <Icon size={20} strokeWidth={1.5} /> : undefined} title={title ?? "Nothing to show"} />;
}

export function PermissionDeniedState({ message }: { message?: string }) {
  return (
    <div className={cn("flex min-h-48 flex-col items-center justify-center rounded-lg border px-5 py-10 text-center", toneStyles.warn.border, toneStyles.warn.bg)}>
      <LockKeyhole className={cn("mb-3", toneStyles.warn.fg)} size={28} strokeWidth={1.5} />
      <h3 className={cn("t-section", toneStyles.warn.fg)}>Access denied</h3>
      <p className={cn("mt-1 max-w-prose text-xs leading-5", toneStyles.warn.fg)}>
        {message ?? "You don’t have permission to view this resource. Contact an administrator to request access."}
      </p>
    </div>
  );
}

export function StatsRow({ items }: { items: Array<{ label: string; value: string | number; icon?: LucideIcon; tone?: AdminTone }> }) {
  if (!items || items.length === 0) return null;
  return (
    <ForgeGrid className="mb-5" cols={4}>
      {items.map((item) => (
        <AdminStatCard icon={item.icon} key={item.label} label={item.label} tone={item.tone} value={item.value} />
      ))}
    </ForgeGrid>
  );
}

/**
 * Single stat tile. Delegates to {@link ForgeMetric}, so an absent reading
 * renders as `—` in the unknown tone rather than as `0`.
 */
export function AdminStatCard({ label, value, icon: Icon, tone = "neutral", className }: {
  label: string; value: string | number; icon?: LucideIcon; tone?: AdminTone; className?: string;
}) {
  return (
    <ForgeMetric
      className={className}
      icon={Icon ? <Icon aria-hidden="true" size={13} /> : undefined}
      label={label}
      tone={resolveTone(tone)}
      value={value}
    />
  );
}
