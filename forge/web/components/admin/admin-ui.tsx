"use client";

/**
 * Shared admin UI primitives used across all admin panel sections.
 */

import { LockKeyhole, type LucideIcon } from "lucide-react";
import { Button, Dialog, EmptyState as SharedEmptyState, Input as SharedInput, Textarea as SharedTextarea } from "@/components/ui/primitives";
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
 <span className={cn("rounded px-2 py-0.5 text-xs font-semibold", tones[tone], className)}>
 {children}
 </span>
 );
}

export function SectionHeader({ title, sub, action }: { title: string; sub?: string; action?: React.ReactNode }) {
 return (
 <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
 <div>
 <h1 className="text-2xl font-semibold text-slate-100">{title}</h1>
 {sub ? <p className="mt-1 text-sm text-slate-400">{sub}</p> : null}
 </div>
 {action ? <div className="flex shrink-0 flex-wrap gap-2">{action}</div> : null}
 </div>
 );
}

export function Card({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <div className={cn("overflow-hidden rounded-xl border border-white/[0.08] bg-[#111722] shadow-xl shadow-black/10", className)}>
      {children}
    </div>
  );
}

export function CardHeader({ title, icon: Icon, action }: { title: string; icon?: LucideIcon; action?: React.ReactNode }) {
 return (
 <div className="flex h-11 items-center gap-2 border-b border-white/[0.06] bg-[#161b28] px-4 text-xs font-semibold uppercase tracking-widest text-slate-400">
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
}: {
  children: React.ReactNode;
  onClick?: () => void;
  tone?: "primary" | "ghost" | "danger" | "subtle" | "warning" | "success";
  disabled?: boolean;
  size?: "sm" | "md";
  type?: "button" | "submit";
  loading?: boolean;
  className?: string;
}) {
  const variants = { primary: "primary", danger: "danger", ghost: "secondary", subtle: "ghost", warning: "secondary", success: "secondary" } as const;
  return <Button className={cn(tone === "warning" && "border-amber-700/40 bg-amber-900/70 text-amber-200 hover:bg-amber-800", tone === "success" && "border-emerald-700/40 bg-emerald-900/70 text-emerald-200 hover:bg-emerald-800", className)} disabled={disabled} loading={loading} onClick={onClick} size={size === "sm" ? "sm" : "default"} type={type} variant={variants[tone]}>{children}</Button>;
}

export function Input({ label, value, onChange, placeholder, type = "text", mono, required }: {
  label?: string; value: string; onChange: (v: string) => void; placeholder?: string; type?: string; mono?: boolean; required?: boolean;
}) {
 return (
 <label className="block text-sm font-medium text-slate-300">
 {label ? <span className="mb-1.5 block">{label}</span> : null}
 <SharedInput
 className={cn("min-h-9 bg-surface-card-header", mono && "font-mono text-xs")}
 onChange={(e) => onChange(e.target.value)}
 placeholder={placeholder}
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

export function Modal({ title, onClose, children, wide, className }: { title: React.ReactNode; onClose: () => void; children: React.ReactNode; wide?: boolean; className?: string }) {
 return <Dialog className={cn("max-h-[92vh] overflow-y-auto", wide && "max-w-3xl", className)} closeAction={onClose} open title={title}>{children}</Dialog>;
}

export function ModalFooter({ onCancel, onConfirm, confirmLabel = "Save", disabled }: {
 onCancel: () => void; onConfirm: () => void; confirmLabel?: string; disabled?: boolean;
}) {
 return (
 <div className="mt-5 flex flex-col-reverse gap-2 border-t border-white/[0.06] pt-4 sm:flex-row sm:justify-end">
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
 return (
 <div className="mb-6 grid grid-cols-2 gap-3 md:grid-cols-4">
 {items.map((item) => (
 <Card key={item.label} className="p-4">
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
