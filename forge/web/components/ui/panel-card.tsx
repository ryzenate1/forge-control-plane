"use client";

import { cn } from "@/lib/utils";
import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

export function PanelCard({ title, icon: Icon, children, className }: { title: string; icon?: LucideIcon; children: ReactNode; className?: string }) {
  return <section className={cn("ui-card", className)}><div className="ui-card-header items-center"><div className="flex items-center gap-2 text-sm font-semibold text-slate-200">{Icon ? <Icon aria-hidden="true" className="text-slate-400" size={17} /> : null}{title}</div></div>{children}</section>;
}

export function PanelSection({ title, description, action, children, className }: { title: string; description?: string; action?: ReactNode; children: ReactNode; className?: string }) {
  return <section className={cn("space-y-4", className)}><div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between"><div><h2 className="text-lg font-semibold text-slate-100">{title}</h2>{description ? <p className="mt-1 text-sm text-slate-400">{description}</p> : null}</div>{action ? <div className="shrink-0">{action}</div> : null}</div>{children}</section>;
}
