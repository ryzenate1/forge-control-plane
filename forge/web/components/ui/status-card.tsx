"use client";

import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

type StatusTone = "ok" | "warning" | "danger" | "neutral";

const toneBorder = {
  ok: "border-l-emerald-500",
  warning: "border-l-amber-500",
  danger: "border-l-red-500",
  neutral: "border-l-slate-600",
} satisfies Record<StatusTone, string>;

const toneIcon = {
  ok: "text-emerald-400",
  warning: "text-amber-400",
  danger: "text-red-400",
  neutral: "text-slate-400",
} satisfies Record<StatusTone, string>;

export function StatusCard({ tone = "neutral", title, sub, icon, children, className }: {
  tone?: StatusTone;
  title: ReactNode;
  sub?: ReactNode;
  icon?: ReactNode;
  children?: ReactNode;
  className?: string;
}) {
  return (
    <section className={cn("ui-card border-l-[3px]", toneBorder[tone], className)}>
      <div className="flex items-start gap-3">
        {icon ? <span className={cn("mt-0.5 shrink-0", toneIcon[tone])}>{icon}</span> : null}
        <div className="min-w-0 flex-1">
          <h2 className="flex items-center gap-2 font-bold text-white">{title}</h2>
          {sub ? <p className="mt-1 text-sm text-slate-400">{sub}</p> : null}
        </div>
      </div>
      {children ? <div className="mt-4">{children}</div> : null}
    </section>
  );
}
